package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// 해태 API 클라이언트 + 캐시. 방문자가 많아도 해태가 받는 요청은 경로마다 ttl 에 한 번.
// 해태가 실패하면 마지막 캐시를 돌려주고 stale=true 로 알림.

var (
	client  = &http.Client{Timeout: 20 * time.Second} // 시작할 때 설정의 timeout 으로 다시 만듦 (initClients)
	client2 = &http.Client{Timeout: 3 * time.Minute}  // 전체 목록 (85 MB) 용
)

var errBusy = errors.New("busy")

type entry struct {
	mu   sync.Mutex // 같은 경로를 동시에 받지 않게 (한 요청만 해태로 감)
	body []byte
	at   time.Time
}

var (
	cacheMu sync.Mutex
	cache   = map[string]*entry{}
)

const maxCache = 2000 // 경로 수 상한 (입력이 제각각이라 무한히 늘 수 있음)

func getEntry(path string) *entry {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	e := cache[path]
	if e == nil {
		if len(cache) >= maxCache { // 단순하게 전부 비움
			cache = map[string]*entry{}
		}
		e = &entry{}
		cache[path] = e
	}
	return e
}

// fetch : path 의 JSON 을 out 에 풀어 넣음. stale = 해태 실패로 옛 캐시를 썼는지
func fetch(path string, out any) (stale bool, err error) { return fetchT(path, out, conf().TTL) }

// fetchT : JSON 응답을 out 에. 캐시 유지 시간을 직접 지정 (객체 조회는 잘 안 바뀌어 길게)
func fetchT(path string, out any, keep time.Duration) (stale bool, err error) {
	body, stale, err := fetchBytes(path, keep)
	if err != nil {
		return false, err
	}
	return stale, json.Unmarshal(body, out)
}

// fetchBytes : 응답 본문 그대로 (캐시 / 해태 실패 시 옛 캐시)
func fetchBytes(path string, keep time.Duration) (body []byte, stale bool, err error) {
	e := getEntry(path)
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.body == nil || time.Since(e.at) > keep {
		if !allowUpstream() { // 해태 보호 : 새 요청이 너무 잦음
			if e.body == nil {
				return nil, false, errBusy
			}
			return e.body, true, nil
		}
		b, ferr := httpGet(path)
		if ferr == nil {
			e.body, e.at = b, time.Now()
		} else if e.body == nil {
			return nil, false, ferr
		} else {
			stale = true
		}
	}
	return e.body, stale, nil
}

// 해태로 가는 새 요청 (캐시에 없는 것) 의 초당 상한 : 방문자가 prefix 를 제각각 바꿔 가며 눌러도 해태가 받는 양이 일정하게.
// 넘으면 마지막 캐시 (있으면) 또는 "바쁨" 안내. 캐시에서 답하는 요청은 세지 않음
var (
	upMu     sync.Mutex
	upTokens float64
	upLast   = time.Now()
)

func allowUpstream() bool {
	upMu.Lock()
	defer upMu.Unlock()
	rps := float64(conf().UpstreamRPS)
	upTokens += time.Since(upLast).Seconds() * rps
	upLast = time.Now()
	if upTokens > rps { // 한꺼번에 쓸 수 있는 양 = 1 초치
		upTokens = rps
	}
	if upTokens < 1 {
		return false
	}
	upTokens--
	return true
}

var sem = make(chan struct{}, 6) // 해태로 가는 동시 요청 상한

func httpGet(path string) ([]byte, error) {
	sem <- struct{}{}
	defer func() { <-sem }()
	resp, err := client.Get(conf().HaetaeURL + path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 {
		return nil, errBusy
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("haetae %s: %s", path, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

// ---- 응답 모양. 필요한 칸만 선언 -> rtr_clients / config / mem_mb 는 아예 받지 않음 (공개 화면 보호)

type Status struct {
	Serial   int    `json:"serial"`
	Session  int    `json:"session"`
	DataTime string `json:"data_time"`
	LastRun  struct {
		Start      string         `json:"start"`
		DurationS  float64        `json:"duration_s"`
		Applied    bool           `json:"applied"`
		Reason     string         `json:"reason"`
		VRPs       int            `json:"vrps"`
		VAPs       int            `json:"vaps"`
		RouterKeys int            `json:"router_keys"`
		PerTA      map[string]int `json:"vrps_per_ta"`
		Announced  int            `json:"announced"`
		Withdrawn  int            `json:"withdrawn"`
		Unsafe     int            `json:"unsafe_vrps"`
	} `json:"last_run"`
}

type Health struct {
	Status   string `json:"status"`
	Serial   int    `json:"serial"`
	RtrOpen  bool   `json:"rtr_open"`
	DataAgeS int    `json:"data_age_s"`
}

type VRP struct {
	ASN       int      `json:"asn"`
	Prefix    string   `json:"prefix"`
	MaxLength int      `json:"maxLength"`
	TA        string   `json:"ta"`
	Expires   int64    `json:"expires"`
	ROAs      []string `json:"roas"`
	// 탐색기가 prefix 를 조회할 때만 채움 : 이 VRP 가 조회한 prefix 를 덮지만 maxLength 가 모자라 판정에는 안 쓰임 (RFC 6811)
	NoMatch   bool `json:"-"`
	TargetLen int  `json:"-"` // 조회한 prefix 의 길이
}

type PrefixResp struct {
	Covering      []VRP `json:"covering"`
	MoreSpecific  []VRP `json:"more_specific"`
	MoreTruncated bool  `json:"more_specific_truncated"`
}

type ASResp struct {
	ASN  int `json:"asn"`
	ASPA *struct {
		Providers []int `json:"providers"`
	} `json:"aspa"`
	ProviderOf []int             `json:"provider_of"`
	RouterKeys []json.RawMessage `json:"router_keys"` // 공개 화면은 개수만
	VRPs       []VRP             `json:"vrps"`
}

type VRPShort struct {
	ASN       string `json:"asn"`
	Prefix    string `json:"prefix"`
	MaxLength int    `json:"max_length"`
}

type Validity struct {
	Route struct {
		Route struct {
			Origin string `json:"origin_asn"`
			Prefix string `json:"prefix"`
		} `json:"route"`
		Validity struct {
			VRPs struct {
				Matched   []VRPShort `json:"matched"`
				UnmAS     []VRPShort `json:"unmatched_as"`
				UnmLength []VRPShort `json:"unmatched_length"`
			} `json:"VRPs"`
			Description string `json:"description"`
			State       string `json:"state"`
		} `json:"validity"`
	} `json:"validated_route"`
}

type AspaResp struct {
	Result struct {
		Path       []int                          `json:"path"`
		AsSet      bool                           `json:"as_set"`
		Neighbor   int                            `json:"neighbor"`
		Upstream   struct{ State, Reason string } `json:"upstream"`
		Downstream struct{ State, Reason string } `json:"downstream"`
		Hops       []struct {
			From int    `json:"from"`
			To   int    `json:"to"`
			Up   string `json:"up"`
			Down string `json:"down"`
		} `json:"hops"`
	} `json:"result"`
}

// History : 회차마다 한 줄. mem_mb 는 내부 정보라 선언하지 않음
type Run struct {
	Start     string  `json:"start"`
	DurationS float64 `json:"duration_s"`
	Applied   bool    `json:"applied"`
	Serial    int     `json:"serial"`
	VRPs      int     `json:"vrps"`
	VAPs      int     `json:"vaps"`
	Announced int     `json:"announced"`
	Withdrawn int     `json:"withdrawn"`
	Reason    string  `json:"reason"` // 반영하지 않은 이유 (예 "TA failed [ripe], keep previous")
}

type HistoryResp struct {
	Runs []Run `json:"runs"`
}

type DeltaItem struct {
	Type      string   `json:"type"`
	ASN       string   `json:"asn"`
	Prefix    string   `json:"prefix"`
	MaxLength int      `json:"maxLength"`
	Customer  string   `json:"customer"`
	Providers []string `json:"providers"`
}

type DeltaResp struct {
	Serial      int         `json:"serial"`
	FromSerial  int         `json:"fromSerial"`
	GeneratedAt string      `json:"generatedTime"`
	Announced   []DeltaItem `json:"announced"`
	Withdrawn   []DeltaItem `json:"withdrawn"`
}

// Obj : /api/v1/objects 의 객체 하나 (인증서 경로 화면용)
type Obj struct {
	Type      string `json:"type"`
	URI       string `json:"uri"`
	SHA256    string `json:"sha256"`
	TA        string `json:"ta"`
	Parent    string `json:"parent"`
	NotBefore string `json:"notBefore"`
	NotAfter  string `json:"notAfter"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
	Summary   struct {
		Kind       string           `json:"kind"`
		Repository string           `json:"repository"`
		Notify     string           `json:"notify"`
		Number     string           `json:"number"`
		Files      int              `json:"files"`
		Revoked    int              `json:"revoked"`
		ASN        int              `json:"asn"`
		Prefixes   []roaPfx         `json:"prefixes"`
		Resources  map[string][]any `json:"resources"`
		Customer   any              `json:"customer"`
		Providers  []any            `json:"providers"`
		ASNs       []any            `json:"asns"`
		SKI        string           `json:"ski"`
		VCard      map[string]any   `json:"vcard"`
	} `json:"summary"`
}

type ObjList struct {
	Objects []Obj `json:"objects"`
}

// ---- 내부 화면 (nginx 로그인 뒤) 응답 ----

type Repo struct {
	URI      string `json:"uri"`
	Type     string `json:"type"`
	Result   string `json:"result"`
	Action   string `json:"action"`
	Serial   int    `json:"serial"`
	Error    string `json:"error"`
	Failures int    `json:"consecutive_failures"`
	LastOK   string `json:"last_success"`
	NextTry  string `json:"next_try"`
	Skipped  bool   `json:"skipped"`
	Start    string `json:"start"`
	DurNS    int64  `json:"duration_ns"`
}

type RTRClient struct {
	Remote    string `json:"remote"`
	Transport string `json:"transport"`
	Version   int    `json:"version"`
	Since     string `json:"since"`
	LastQuery string `json:"last_query"`
	Serial    int    `json:"serial"`
	Trusted   bool   `json:"trusted"`
}

type IStatus struct {
	TAs []struct {
		Name string `json:"name"`
		URI  string `json:"uri"`
	} `json:"tas"`
	RTR      []RTRClient `json:"rtr_clients"`
	Handover struct {
		Count   int    `json:"count"`
		LastAt  string `json:"last_at"`
		From    string `json:"from"`
		PauseMS int    `json:"pause_ms"`
		Conns   int    `json:"rtr_conns"`
		Err     string `json:"last_error"`
	} `json:"handover"`
	Version string `json:"version"`
	Started string `json:"started"`
	Stale   bool   `json:"stale"`
	LastRun struct {
		Reason string `json:"reason"`
	} `json:"last_run"`
}

type UnsafeResp struct {
	Total int `json:"total"`
	VRPs  []struct {
		ASN        int    `json:"asn"`
		Prefix     string `json:"prefix"`
		MaxLength  int    `json:"maxLength"`
		TA         string `json:"ta"`
		RejectedCA string `json:"rejected_ca"`
	} `json:"vrps"`
}

type ObjPage struct {
	Total   int   `json:"total"`
	Offset  int   `json:"offset"`
	Objects []Obj `json:"objects"`
}

// roaPfx : ROA 안의 prefix 한 개
type roaPfx struct {
	Prefix    string `json:"prefix"`
	MaxLength int    `json:"maxLength"`
}
