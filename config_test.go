package main

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func parse(s string) (*Config, error) { return parseConf(bufio.NewScanner(strings.NewReader(s))) }

func TestConfOK(t *testing.T) {
	c, err := parse("[haetae]\nurl = http://h:5002/  # 끝 / 는 뗌\ntimeout = 5s\n[listen]\naddr = 0.0.0.0:80\n[web]\ntrust_proxy = 10.0.0.0/8, ::1\nlang = ko\n[cache]\nmax_rows = 100\n")
	if err != nil {
		t.Fatal(err)
	}
	if c.HaetaeURL != "http://h:5002" || c.Timeout != 5*time.Second || c.Listen != "0.0.0.0:80" || c.MaxRows != 100 || c.Lang != "ko" {
		t.Fatalf("%+v", c)
	}
	if !c.inTrusted("10.1.2.3") || !c.inTrusted("::1") || c.inTrusted("11.0.0.1") || c.inTrusted("garbage") {
		t.Fatal("trust_proxy 판정")
	}
}

func TestConfErrors(t *testing.T) {
	for _, s := range []string{
		"[haetae]\nurll = x\n",           // 모르는 키
		"[haetae]\nurl = a\nurl = b\n",   // 두 번
		"[haetae]\ntimeout = abc\n",      // 값
		"[listen]\naddr = 8330\n",        // 주소 모양
		"[web]\nbase_path = rpki\n",      // / 로 시작해야
		"[web]\ntrust_proxy = nothost\n", // 주소 아님
		"[cache]\nmax_rows = 5\n",        // 범위
		"[internal]\nenabled = maybe\n",  // yes / no
		"그냥 글자\n",                        // 키 = 값 아님
	} {
		if _, err := parse(s); err == nil {
			t.Errorf("오류여야 함 : %q", s)
		}
	}
}

func TestConfValidate(t *testing.T) {
	c := defaultConfig()
	if c.validate() == nil {
		t.Fatal("해태 주소 없으면 오류여야 함")
	}
	c.HaetaeURL = "http://x"
	c.InternalOn = true
	if c.validate() == nil {
		t.Fatal("key_file 없으면 오류여야 함")
	}
	c.InternalOn = false
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRequireLogin(t *testing.T) {
	c := defaultConfig()
	c.HaetaeURL, c.RequireLogin = "http://x", true
	if c.validate() == nil {
		t.Fatal("require_login 인데 key_file 이 없으면 오류여야 함")
	}
	if cc, err := parse("[web]\nrequire_login = yes\n"); err != nil || !cc.RequireLogin {
		t.Fatalf("require_login 읽기 : %v", err)
	}
	old := conf()
	defer curConf.Store(old)
	c.Key = "0123456789abcdef"
	curConf.Store(c)
	h := loginGate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, tc := range []struct {
		path, key string
		want      int
	}{
		{"/", "", 403}, {"/", "wrong", 403}, {"/", "0123456789abcdef", 204},
		{"/jangseung-healthz", "", 204}, {"/static/style.css", "", 403},
	} {
		req := httptest.NewRequest("GET", tc.path, nil)
		if tc.key != "" {
			req.Header.Set(c.KeyHeader, tc.key)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s key=%q : %d (원하는 값 %d)", tc.path, tc.key, rec.Code, tc.want)
		}
	}
}

func TestUpstreamLimit(t *testing.T) {
	c := defaultConfig()
	c.UpstreamRPS = 3
	old := conf()
	defer curConf.Store(old)
	curConf.Store(c)
	upMu.Lock()
	upTokens, upLast = 0, time.Now().Add(-time.Second) // 1 초 지난 것으로 = 3 개 채움
	upMu.Unlock()
	n := 0
	for i := 0; i < 10; i++ {
		if allowUpstream() {
			n++
		}
	}
	if n < 3 || n > 4 {
		t.Fatalf("1 초에 허용 %d 개 (3 개 안팎이어야 함)", n)
	}
}

func TestPageNav(t *testing.T) {
	p := &Page{Query: map[string][]string{}}
	show := func(cur, pages int) string {
		var b []string
		for _, l := range pageNav(p, cur, pages) {
			switch {
			case l.Gap:
				b = append(b, "…")
			case l.Cur:
				b = append(b, "["+strconv.Itoa(l.N)+"]")
			default:
				b = append(b, strconv.Itoa(l.N))
			}
		}
		return strings.Join(b, " ")
	}
	for _, tc := range []struct {
		cur, pages int
		want       string
	}{
		{0, 1, ""}, {0, 2, "[1] 2"}, {0, 7, "[1] 2 3 … 7"}, {4, 9, "1 2 3 4 [5] 6 7 8 9"},
	} {
		if got := show(tc.cur, tc.pages); got != tc.want {
			t.Errorf("cur=%d pages=%d : %q (원하는 값 %q)", tc.cur, tc.pages, got, tc.want)
		}
	}
	if got, want := show(9, 20), "1 … 8 9 [10] 11 12 … 20"; got != want {
		t.Errorf("%q != %q", got, want)
	}
	if got, want := show(19, 20), "1 … 18 19 [20]"; got != want {
		t.Errorf("%q != %q", got, want)
	}
}

func TestPageParam(t *testing.T) {
	for _, tc := range []struct {
		q    string
		want int
	}{{"", 0}, {"p=6", 6}, {"pg=3", 2}, {"pg=3&p=9", 2}, {"pg=0", 0}, {"pg=abc&p=2", 2}, {"p=-4", 0}, {"pg=+7+", 6}} {
		v, _ := url.ParseQuery(tc.q)
		if got := pageParam(v); got != tc.want {
			t.Errorf("%q : %d (원하는 값 %d)", tc.q, got, tc.want)
		}
	}
}
