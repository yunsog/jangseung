package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// 전체 목록 (VRP / ASPA / 라우터 키) : 해태 /json-delta 전체 (약 85 MB, 2 초) 를 한 번 받아 메모리에 줄여 둠.
// VRP 약 104 만 개 = 12 바이트씩 약 12 MB. 새 회차가 나오면 (10 분마다) 뒤에서 다시 받고, 그동안은 옛 것을 보임.
// 해태 쪽 제한 : 전체는 주소마다 60 초에 한 번 -> 최소 5 분 간격.

type v4rec struct {
	Addr     uint32
	Len, Max uint8
	ASN      uint32
}

type v6rec struct {
	Addr     [16]byte
	Len, Max uint8
	ASN      uint32
}

type AspaRec struct {
	Customer  uint32
	Providers []uint32
}

type KeyRec struct {
	ASN uint32
	SKI string // 16 진수 40 자 (대문자)
	Pub string // base64 (SubjectPublicKeyInfo)
}

type Dataset struct {
	Serial  int
	Session string    // 해태 세션 (변경분 요청에 필요)
	Full    time.Time // 전체를 받은 시각 (오래되면 다시 받아 바로잡음)
	Time    string    // 데이터 시각 (RFC3339)
	V4      []v4rec
	V6      []v6rec
	ASPA    []AspaRec
	Keys    []KeyRec
	At      time.Time
}

var (
	curData atomic.Pointer[Dataset]
	loadMu  sync.Mutex
	loading bool
	lastTry time.Time
	loadErr string
)

// getData : 지금 있는 목록 (없으면 nil) 과 불러오는 중인지.
// 처음 = 전체를 받음. 그 뒤 새 회차가 나오면 "바뀐 것만" 받아 반영 (수 KB). 변경분을 못 받거나 (세션 바뀜 / 기록 밖) 6 시간이 지나면 전체를 다시 받음
func getData(latestSerial int) (*Dataset, bool) {
	d := curData.Load()
	loadMu.Lock()
	defer loadMu.Unlock()
	need := d == nil || (latestSerial > d.Serial) || time.Since(d.Full) > fullEvery
	wait := 5 * time.Second // 변경분은 가볍다
	if d == nil || time.Since(d.Full) > fullEvery {
		wait = conf().Refresh // 전체는 해태가 주소마다 60 초에 한 번만 줌
	}
	if need && !loading && time.Since(lastTry) > wait && time.Since(lastTry) > retryAfter() {
		loading, lastTry = true, time.Now()
		go func() {
			t0 := time.Now()
			nd, err := refresh(d)
			loadMu.Lock()
			loading = false
			if err != nil {
				loadErr = err.Error()
			} else {
				loadErr = ""
				curData.Store(nd)
				if d == nil {
					log.Printf("목록 준비 serial %d : VRP %d, ASPA %d, 라우터 키 %d (%.1f 초)", nd.Serial, len(nd.V4)+len(nd.V6), len(nd.ASPA), len(nd.Keys), time.Since(t0).Seconds())
				}
			}
			loadMu.Unlock()
		}()
	}
	return d, loading
}

// retryAfter : 마지막 시도가 429 (너무 잦음) 였으면 65 초 뒤에 다시
func retryAfter() time.Duration {
	if strings.Contains(loadErr, "429") {
		return 65 * time.Second
	}
	return 0
}

const fullEvery = 6 * time.Hour

// refresh : 가진 것이 없으면 전체, 있으면 변경분 (안 되면 전체)
func refresh(old *Dataset) (*Dataset, error) {
	if old != nil && time.Since(old.Full) < fullEvery {
		nd, err := applyDelta(old)
		if err == nil {
			log.Printf("목록 변경분 반영 serial %d -> %d (VRP %d -> %d)", old.Serial, nd.Serial, len(old.V4)+len(old.V6), len(nd.V4)+len(nd.V6))
			return nd, nil
		}
		log.Printf("목록 변경분 반영 실패 (%v) -> 전체를 다시 받음", err)
	}
	return loadData()
}

type deltaItem struct {
	Type      string   `json:"type"`
	ASN       string   `json:"asn"`
	Prefix    string   `json:"prefix"`
	MaxLength int      `json:"maxLength"`
	Customer  string   `json:"customer"`
	Providers []string `json:"providers"`
	SKI       string   `json:"SKI"`
	Pub       string   `json:"routerPublicKey"`
}

func asnNum(s string) uint32 {
	n, _ := strconv.ParseUint(strings.TrimPrefix(strings.ToUpper(s), "AS"), 10, 32)
	return uint32(n)
}

// loadData : /json-delta 전체를 줄 단위가 아니라 토큰으로 훑어 (메모리에 85 MB 를 올리지 않음) 줄여 담음
func loadData() (*Dataset, error) {
	resp, err := client2.Get(conf().HaetaeURL + "/json-delta")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 { // 해태 : 전체는 주소마다 60 초에 한 번
		return nil, fmt.Errorf("json-delta: busy (429), retry after 65s")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("json-delta: %s", resp.Status)
	}
	return parseFull(resp.Body)
}

// parseFull : /json-delta 전체 본문을 토큰으로 훑어 줄여 담음 (시험에서 파일도 읽을 수 있게 분리)
func parseFull(r io.Reader) (*Dataset, error) {
	dec := json.NewDecoder(r)
	d := &Dataset{At: time.Now(), Full: time.Now()}
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, fmt.Errorf("json-delta: not an object")
	}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := kt.(string)
		switch key {
		case "announced":
			if err := readItems(dec, d); err != nil {
				return nil, err
			}
		case "serial":
			dec.Decode(&d.Serial)
		case "session":
			dec.Decode(&d.Session)
		case "generatedTime":
			dec.Decode(&d.Time)
		default:
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return nil, err
			}
		}
	}
	if len(d.V4)+len(d.V6) == 0 {
		return nil, fmt.Errorf("json-delta: empty")
	}
	sort.Slice(d.V4, func(i, j int) bool { return lessV4(d.V4[i], d.V4[j]) })
	sort.Slice(d.V6, func(i, j int) bool { return lessV6(d.V6[i], d.V6[j]) })
	sort.Slice(d.ASPA, func(i, j int) bool { return d.ASPA[i].Customer < d.ASPA[j].Customer })
	sort.Slice(d.Keys, func(i, j int) bool {
		if d.Keys[i].ASN != d.Keys[j].ASN {
			return d.Keys[i].ASN < d.Keys[j].ASN
		}
		return d.Keys[i].SKI < d.Keys[j].SKI
	})
	return d, nil
}

func readItems(dec *json.Decoder, d *Dataset) error {
	if t, err := dec.Token(); err != nil || t != json.Delim('[') {
		return fmt.Errorf("announced: not an array")
	}
	for dec.More() {
		var it deltaItem
		if err := dec.Decode(&it); err != nil {
			return err
		}
		switch it.Type {
		case "routeOrigin":
			p, err := netip.ParsePrefix(it.Prefix)
			if err != nil {
				continue
			}
			if p.Addr().Is4() {
				a := p.Addr().As4()
				d.V4 = append(d.V4, v4rec{binary.BigEndian.Uint32(a[:]), uint8(p.Bits()), uint8(it.MaxLength), asnNum(it.ASN)})
			} else {
				d.V6 = append(d.V6, v6rec{p.Addr().As16(), uint8(p.Bits()), uint8(it.MaxLength), asnNum(it.ASN)})
			}
		case "aspa":
			r := AspaRec{Customer: asnNum(it.Customer)}
			for _, x := range it.Providers {
				r.Providers = append(r.Providers, asnNum(x))
			}
			d.ASPA = append(d.ASPA, r)
		case "routerKey":
			d.Keys = append(d.Keys, KeyRec{asnNum(it.ASN), strings.ToUpper(it.SKI), it.Pub})
		}
	}
	_, err := dec.Token() // ]
	return err
}

func lessV4(a, b v4rec) bool {
	if a.Addr != b.Addr {
		return a.Addr < b.Addr
	}
	if a.Len != b.Len {
		return a.Len < b.Len
	}
	if a.Max != b.Max {
		return a.Max < b.Max
	}
	return a.ASN < b.ASN
}

func lessV6(a, b v6rec) bool {
	if c := strings.Compare(string(a.Addr[:]), string(b.Addr[:])); c != 0 {
		return c < 0
	}
	if a.Len != b.Len {
		return a.Len < b.Len
	}
	if a.Max != b.Max {
		return a.Max < b.Max
	}
	return a.ASN < b.ASN
}

// ---- 검색 ----

type VRPRow struct {
	Prefix string
	Max    uint8
	ASN    uint32
}

func (r v4rec) prefix() string {
	var a [4]byte
	binary.BigEndian.PutUint32(a[:], r.Addr)
	return netip.PrefixFrom(netip.AddrFrom4(a), int(r.Len)).String()
}

func (r v6rec) prefix() string {
	return netip.PrefixFrom(netip.AddrFrom16(r.Addr), int(r.Len)).String()
}

// matchPrefix : 레코드 (addr, len) 이 target 과 어떤 관계인지 mode (exact / lspec / mspec / any) 에 맞는지
func matchBits(mode string, rl, tl int, contains func() bool, containedBy func() bool) bool {
	switch mode {
	case "exact":
		return rl == tl && contains()
	case "lspec": // 레코드가 target 을 덮음
		return rl <= tl && contains()
	case "mspec": // 레코드가 target 안쪽
		return rl >= tl && containedBy()
	}
	return (rl <= tl && contains()) || (rl >= tl && containedBy())
}

// searchVRP : AS (0 = 조건 없음) 와 prefix (유효하면) 로 거름. offset 부터 limit 개 + 전체 개수
func (d *Dataset) searchVRP(asn uint32, pf netip.Prefix, hasPrefix bool, mode string, offset, limit int) (rows []VRPRow, total int) {
	add := func(match bool, p func() string, max uint8, a uint32) {
		if !match {
			return
		}
		if total >= offset && len(rows) < limit {
			rows = append(rows, VRPRow{p(), max, a})
		}
		total++
	}
	if !hasPrefix || pf.Addr().Is4() {
		var t4 uint32
		if hasPrefix {
			a := pf.Addr().As4()
			t4 = binary.BigEndian.Uint32(a[:])
		}
		for i := range d.V4 {
			r := &d.V4[i]
			if asn != 0 && r.ASN != asn {
				continue
			}
			ok := true
			if hasPrefix {
				tl, rl := pf.Bits(), int(r.Len)
				mask := func(n int) uint32 {
					if n == 0 {
						return 0
					}
					return ^uint32(0) << (32 - n)
				}
				ok = matchBits(mode, rl, tl,
					func() bool { return r.Addr&mask(rl) == t4&mask(rl) },
					func() bool { return r.Addr&mask(tl) == t4&mask(tl) })
			}
			add(ok, r.prefix, r.Max, r.ASN)
		}
	}
	if !hasPrefix || pf.Addr().Is6() {
		for i := range d.V6 {
			r := &d.V6[i]
			if asn != 0 && r.ASN != asn {
				continue
			}
			ok := true
			if hasPrefix {
				rp := netip.PrefixFrom(netip.AddrFrom16(r.Addr), int(r.Len))
				ok = matchBits(mode, int(r.Len), pf.Bits(),
					func() bool { return rp.Contains(pf.Addr()) },
					func() bool { return pf.Contains(rp.Addr()) })
			}
			add(ok, r.prefix, r.Max, r.ASN)
		}
	}
	return
}

// searchASPA : AS 가 customer 이거나 provider 인 것 (0 = 전체)
func (d *Dataset) searchASPA(asn uint32, offset, limit int) (rows []AspaRec, total int) {
	for _, r := range d.ASPA {
		if asn != 0 && r.Customer != asn {
			found := false
			for _, p := range r.Providers {
				if p == asn {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if total >= offset && len(rows) < limit {
			rows = append(rows, r)
		}
		total++
	}
	return
}

// searchKeys : AS (0 = 조건 없음) 와 SKI (일부 일치, 대소문자 무관) 로 거름
func (d *Dataset) searchKeys(asn uint32, ski string, offset, limit int) (rows []KeyRec, total int) {
	ski = strings.ToUpper(ski)
	for _, k := range d.Keys {
		if (asn != 0 && k.ASN != asn) || (ski != "" && !strings.Contains(k.SKI, ski)) {
			continue
		}
		if total >= offset && len(rows) < limit {
			rows = append(rows, k)
		}
		total++
	}
	return
}

// ---- 변경분 반영 ----

type deltaResp struct {
	Reset     bool        `json:"reset"`
	Serial    int         `json:"serial"`
	Time      string      `json:"generatedTime"`
	Announced []deltaItem `json:"announced"`
	Withdrawn []deltaItem `json:"withdrawn"`
}

// applyDelta : /json-delta?session=&serial= (그 뒤 지금까지 바뀐 것) 을 옛 목록에 반영한 새 목록.
// 옛 목록은 바꾸지 않음 (읽는 쪽이 잠금 없이 쓰므로 복사해서 바꾸고 포인터만 바꿈). reset / 오류면 에러 -> 전체를 다시 받음
func applyDelta(old *Dataset) (*Dataset, error) {
	if old.Session == "" {
		return nil, fmt.Errorf("no session")
	}
	body, err := httpGet(fmt.Sprintf("/json-delta?session=%s&serial=%d", url.QueryEscape(old.Session), old.Serial))
	if err != nil {
		return nil, err
	}
	var dr deltaResp
	if err := json.Unmarshal(body, &dr); err != nil {
		return nil, err
	}
	if dr.Reset {
		return nil, fmt.Errorf("reset")
	}
	nd := &Dataset{Serial: dr.Serial, Session: old.Session, Full: old.Full, Time: dr.Time, At: time.Now(),
		V4: append([]v4rec(nil), old.V4...), V6: append([]v6rec(nil), old.V6...),
		ASPA: append([]AspaRec(nil), old.ASPA...), Keys: append([]KeyRec(nil), old.Keys...)}
	if dr.Time == "" {
		nd.Time = old.Time
	}
	for _, it := range dr.Withdrawn { // 지운 것 먼저, 추가는 나중 (바뀐 항목 = 지우고 다시 추가)
		nd.apply(it, false)
	}
	for _, it := range dr.Announced {
		nd.apply(it, true)
	}
	return nd, nil
}

// insertSorted / removeSorted : 정렬된 조각에서 이진 탐색으로 넣고 뺌
func insertSorted[T any](s []T, x T, less func(a, b T) bool) []T {
	i := sort.Search(len(s), func(i int) bool { return !less(s[i], x) })
	if i < len(s) && !less(x, s[i]) { // 이미 있음
		return s
	}
	s = append(s, x)
	copy(s[i+1:], s[i:])
	s[i] = x
	return s
}

func removeSorted[T any](s []T, x T, less func(a, b T) bool) []T {
	i := sort.Search(len(s), func(i int) bool { return !less(s[i], x) })
	if i < len(s) && !less(x, s[i]) {
		return append(s[:i], s[i+1:]...)
	}
	return s
}

func (d *Dataset) apply(it deltaItem, add bool) {
	switch it.Type {
	case "routeOrigin":
		p, err := netip.ParsePrefix(it.Prefix)
		if err != nil {
			return
		}
		if p.Addr().Is4() {
			a := p.Addr().As4()
			r := v4rec{binary.BigEndian.Uint32(a[:]), uint8(p.Bits()), uint8(it.MaxLength), asnNum(it.ASN)}
			if add {
				d.V4 = insertSorted(d.V4, r, lessV4)
			} else {
				d.V4 = removeSorted(d.V4, r, lessV4)
			}
		} else {
			r := v6rec{p.Addr().As16(), uint8(p.Bits()), uint8(it.MaxLength), asnNum(it.ASN)}
			if add {
				d.V6 = insertSorted(d.V6, r, lessV6)
			} else {
				d.V6 = removeSorted(d.V6, r, lessV6)
			}
		}
	case "aspa": // customer 하나에 ASPA 하나
		c := asnNum(it.Customer)
		d.ASPA = removeSorted(d.ASPA, AspaRec{Customer: c}, func(a, b AspaRec) bool { return a.Customer < b.Customer })
		if add {
			r := AspaRec{Customer: c}
			for _, x := range it.Providers {
				r.Providers = append(r.Providers, asnNum(x))
			}
			d.ASPA = insertSorted(d.ASPA, r, func(a, b AspaRec) bool { return a.Customer < b.Customer })
		}
	case "routerKey":
		k := KeyRec{asnNum(it.ASN), strings.ToUpper(it.SKI), it.Pub}
		lt := func(a, b KeyRec) bool {
			if a.ASN != b.ASN {
				return a.ASN < b.ASN
			}
			return a.SKI < b.SKI
		}
		if add {
			d.Keys = insertSorted(d.Keys, k, lt)
		} else {
			d.Keys = removeSorted(d.Keys, k, lt)
		}
	}
}

// keepFresh : 서비스가 시작하면 바로 목록을 받아 두고, 그 뒤로 사람이 접속하지 않아도 새 회차마다 변경분을 반영함.
// -> 첫 접속자가 기다리지 않음 (재시작 직후만 몇 초). 해태가 멈춰도 마지막 목록을 그대로 둠
func keepFresh() {
	for {
		var h Health
		if _, err := fetch("/healthz", &h); err == nil {
			getData(h.Serial)
		} else {
			getData(0) // 해태 응답이 없어도 (처음이면) 목록은 시도
		}
		time.Sleep(30 * time.Second)
	}
}
