package main

// 설정 파일 (/etc/jangseung/jangseung.conf). 형식은 해태와 같음 :
//   [구역] 아래 "키 = 값", # 뒤는 주석. 없는 키 = 기본값. 모르는 키 / 같은 키 두 번 / 잘못된 값이 있으면 시작하지 않음
//   (오타로 허용 목록이 비는 사고 방지). 명령줄 옵션이 파일보다 우선.
// systemctl reload (SIGHUP) 로 다시 읽음 : listen / timeout 만 restart 가 필요 (바뀌면 지금 값 유지 + 경고).

import (
	"bufio"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

const defaultConfPath = "/etc/jangseung/jangseung.conf"

// Config : 읽은 설정. 한 번 만들면 바꾸지 않고, 다시 읽으면 통째로 바꿔 끼움 (conf())
type Config struct {
	HaetaeURL string        // [haetae] url (필수)
	Timeout   time.Duration // [haetae] timeout (restart)
	Listen    string        // [listen] addr (restart)

	InternalOn   bool   // [internal] enabled
	KeyFile      string // [internal] key_file
	KeyHeader    string // [internal] header
	Key          string // key_file 의 내용 (설정에는 없음)
	RequireLogin bool   // [web] require_login : 공개 화면도 프록시가 로그인 확인 뒤 붙인 비밀 헤더가 있을 때만
	BasePath     string // [web] base_path : "auto" = 프록시의 X-Forwarded-Prefix, 아니면 고정값 (예 /rpki), "" = 없음
	TrustProxy   []netip.Prefix
	Lang         string // [web] lang : auto | ko | en
	TTL          time.Duration
	Refresh      time.Duration
	MaxRows      int
	UpstreamRPS  int // [cache] upstream_rps : 캐시에 없어 해태로 새로 가는 요청의 초당 상한
}

var curConf atomic.Pointer[Config]

func conf() *Config { return curConf.Load() }

// 시작 전 / 시험에서도 conf() 가 nil 이 아니게 (main 이 설정을 읽어 바꿔 끼움)
func init() { curConf.Store(defaultConfig()) }

func defaultConfig() *Config {
	return &Config{
		Timeout: 20 * time.Second, Listen: "127.0.0.1:8330",
		KeyHeader: "X-Jangseung-Key", BasePath: "auto", Lang: "auto",
		TrustProxy: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32"), netip.MustParsePrefix("::1/128")},
		TTL:        60 * time.Second, Refresh: 5 * time.Minute, MaxRows: 500, UpstreamRPS: 20,
	}
}

// parseConf : 파일 내용 -> Config. 오류 메시지에 줄 번호를 붙임
func parseConf(r *bufio.Scanner) (*Config, error) {
	c := defaultConfig()
	sec, n := "", 0
	seen := map[string]bool{}
	for r.Scan() {
		n++
		line := strings.TrimSpace(r.Text())
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sec = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%d 번째 줄 : '키 = 값' 이 아님", n)
		}
		key := sec + "." + strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if seen[key] {
			return nil, fmt.Errorf("%d 번째 줄 : %s 가 두 번 나옴", n, key)
		}
		seen[key] = true
		if err := c.set(key, v); err != nil {
			return nil, fmt.Errorf("%d 번째 줄 %s : %v", n, key, err)
		}
	}
	return c, r.Err()
}

// set : 키 하나를 값에 맞게 넣음. 모르는 키는 오류
func (c *Config) set(key, v string) (err error) {
	switch key {
	case "haetae.url":
		c.HaetaeURL = strings.TrimRight(v, "/")
	case "haetae.timeout":
		c.Timeout, err = parseDur(v)
	case "listen.addr":
		_, err = netip.ParseAddrPort(v)
		c.Listen = v
	case "internal.enabled":
		c.InternalOn, err = parseBool(v)
	case "internal.key_file":
		c.KeyFile = v
	case "internal.header":
		c.KeyHeader = v
	case "web.require_login":
		c.RequireLogin, err = parseBool(v)
	case "web.base_path":
		if v != "auto" && v != "" && !prefixRe.MatchString(v) {
			err = fmt.Errorf("auto / 비움 / /이름 중 하나여야 함")
		}
		c.BasePath = v
	case "web.trust_proxy":
		c.TrustProxy, err = parsePrefixes(v)
	case "web.lang":
		if v != "auto" && v != "ko" && v != "en" {
			err = fmt.Errorf("auto / ko / en 중 하나여야 함")
		}
		c.Lang = v
	case "cache.ttl":
		c.TTL, err = parseDur(v)
	case "cache.refresh":
		c.Refresh, err = parseDur(v)
	case "cache.upstream_rps":
		c.UpstreamRPS, err = strconv.Atoi(v)
		if err == nil && (c.UpstreamRPS < 1 || c.UpstreamRPS > 1000) {
			err = fmt.Errorf("1 ~ 1000")
		}
	case "cache.max_rows":
		c.MaxRows, err = strconv.Atoi(v)
		if err == nil && (c.MaxRows < 10 || c.MaxRows > 100000) {
			err = fmt.Errorf("10 ~ 100000")
		}
	default:
		err = fmt.Errorf("모르는 키")
	}
	return err
}

func parseDur(v string) (time.Duration, error) {
	d, err := time.ParseDuration(v)
	if err == nil && d <= 0 {
		err = fmt.Errorf("0 보다 커야 함")
	}
	return d, err
}

func parseBool(v string) (bool, error) {
	switch strings.ToLower(v) {
	case "yes", "true", "on", "1":
		return true, nil
	case "no", "false", "off", "0":
		return false, nil
	}
	return false, fmt.Errorf("yes / no")
}

// parsePrefixes : "127.0.0.1, 10.0.0.0/8, ::1" -> 주소 범위 목록. 주소만 쓰면 그 주소 하나
func parsePrefixes(v string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, s := range strings.Split(v, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if p, err := netip.ParsePrefix(s); err == nil {
			out = append(out, p.Masked())
		} else if a, err := netip.ParseAddr(s); err == nil {
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
		} else {
			return nil, fmt.Errorf("%q 는 주소 / 범위가 아님", s)
		}
	}
	return out, nil
}

// validate : 파일을 다 읽은 뒤 필수 / 서로 맞아야 하는 것 확인 + key_file 읽기
func (c *Config) validate() error {
	if c.HaetaeURL == "" {
		return fmt.Errorf("[haetae] url 이 없음 (해태 HTTP API 주소, 예 http://127.0.0.1:5002)")
	}
	if !strings.HasPrefix(c.HaetaeURL, "http://") && !strings.HasPrefix(c.HaetaeURL, "https://") {
		return fmt.Errorf("[haetae] url 은 http:// 또는 https:// 로 시작해야 함")
	}
	if c.InternalOn || c.RequireLogin {
		if c.KeyFile == "" {
			return fmt.Errorf("[internal] enabled = yes 또는 [web] require_login = yes 인데 key_file 이 없음")
		}
		b, err := os.ReadFile(c.KeyFile)
		if err != nil {
			return fmt.Errorf("[internal] key_file 을 읽지 못함 : %v", err)
		}
		c.Key = strings.TrimSpace(string(b))
		if len(c.Key) < 16 {
			return fmt.Errorf("[internal] key_file 의 키가 너무 짧음 (16 자 이상, 예 openssl rand -hex 24)")
		}
	}
	return nil
}

// loadConf : 파일 (path 가 "" 이면 기본 위치가 있을 때만) 을 읽어 검사한 Config. 파일이 없고 path 도 없으면 기본값만
func loadConf(path string) (*Config, string, error) {
	if path == "" {
		if _, err := os.Stat(defaultConfPath); err != nil {
			return defaultConfig(), "", nil
		}
		path = defaultConfPath
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, path, err
	}
	defer f.Close()
	c, err := parseConf(bufio.NewScanner(f))
	if err != nil {
		return nil, path, fmt.Errorf("%s : %v", path, err)
	}
	return c, path, nil
}

// inTrusted : 이 주소 (RemoteAddr 의 host 부분) 가 trust_proxy 안인가
func (c *Config) inTrusted(host string) bool {
	a, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	a = a.Unmap()
	for _, p := range c.TrustProxy {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
