package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 내부 화면 (/internal/...). 프록시 (nginx auth_request 등) 가 로그인을 통과시킨 요청에만 비밀 헤더 X-Jangseung-Key 를 붙임.
// 장승은 이 헤더가 맞을 때만 답함 -> nginx 를 건너뛰거나 공개 경로로 와도 열리지 않음. 키가 설정 안 되면 내부 화면 전체가 꺼짐.

func internalOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := conf()
		got := r.Header.Get(c.KeyHeader)
		if !c.InternalOn || c.Key == "" || subtle.ConstantTimeCompare([]byte(got), []byte(c.Key)) != 1 {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store") // 로그인 뒤 내용은 공용 캐시에 남기지 않음
		h(w, r)
	}
}

// secHeaders : 모든 응답의 보안 헤더 (프록시가 없어도 / 프록시가 안 붙여도 동작하도록 장승이 직접).
// CSP : 스크립트는 같은 출처의 app.js 만 (인라인 스크립트 없음), 스타일은 인라인 style 속성이 있어 'unsafe-inline', 다른 곳의 iframe 에 못 들어감
func secHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// loginGate : [web] require_login = yes 이면 모든 화면 (systemd 워치독의 자기 확인용 /jangseung-healthz 만 빼고) 이 같은 비밀 헤더를 요구함.
// 장승은 로그인을 직접 하지 않음 : 프록시가 로그인을 확인한 요청에만 헤더를 붙이고, 헤더 없이 (프록시를 건너뛰어) 오면 403
func loginGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c := conf(); c.RequireLogin && r.URL.Path != "/jangseung-healthz" {
			if c.Key == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get(c.KeyHeader)), []byte(c.Key)) != 1 {
				w.Header().Set("Cache-Control", "no-store")
				http.Error(w, "403 로그인이 필요합니다 / login required", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isub(w http.ResponseWriter, r *http.Request, sub string) *Page {
	p := newPage(w, r, "internal")
	p.Sub = sub
	return p
}

// psub : 공개 화면 (저장소 / 거부 / 만료 / 객체 / unsafe) 의 소메뉴 쪽. 위 메뉴는 "운영"
func psub(w http.ResponseWriter, r *http.Request, sub string) *Page {
	p := newPage(w, r, "ops")
	p.Sub = sub
	return p
}

func ago(s string) string { // 지난 / 남은 시간을 짧게 (언어 무관)
	t, err := time.Parse(time.RFC3339, s)
	if err != nil || s == "" {
		return "-"
	}
	d := time.Since(t)
	pre, suf := "", " ago"
	if d < 0 {
		d, pre, suf = -d, "in ", ""
	}
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%s%dm%s", pre, int(d.Minutes()), suf)
	case d < 48*time.Hour:
		return fmt.Sprintf("%s%dh%s", pre, int(d.Hours()), suf)
	}
	return fmt.Sprintf("%s%dd%s", pre, int(d.Hours()/24), suf)
}

// ---- 홈 ----

func iHome(w http.ResponseWriter, r *http.Request) {
	p := isub(w, r, "home")
	type data struct{ Problems, Rejects, Unsafe, Clients int }
	d := &data{}
	var repos []Repo
	if _, err := fetch("/repos.json", &repos); err == nil {
		for _, x := range repos {
			if x.Result != "ok" {
				d.Problems++
			}
		}
	}
	if b, _, err := fetchBytes("/rejects.txt", conf().TTL); err == nil {
		d.Rejects = strings.Count(string(b), "\n")
	}
	var u UnsafeResp
	if _, err := fetch("/api/v1/unsafe", &u); err == nil {
		d.Unsafe = u.Total
	}
	var st IStatus
	if _, err := fetch("/api/v1/status", &st); err == nil {
		d.Clients = len(st.RTR)
	}
	p.Data = d
	render(w, r, p, "i_home")
}

// ---- 저장소 상태 ----

func iRepos(w http.ResponseWriter, r *http.Request) {
	p := psub(w, r, "repos")
	type row struct {
		Repo
		Dur string
	}
	type data struct {
		Rows         []row
		Total, Bad   int
		OnlyProblems bool
	}
	d := &data{OnlyProblems: r.URL.Query().Get("all") != "1"}
	p.Data = d
	var repos []Repo
	if _, err := fetch("/repos.json", &repos); err != nil {
		p.apiErr(err)
		render(w, r, p, "i_repos")
		return
	}
	d.Total = len(repos)
	rank := func(x Repo) int { // 실패 > 대체 > ok
		switch x.Result {
		case "failed":
			return 0
		case "ok":
			return 2
		}
		return 1
	}
	sort.SliceStable(repos, func(i, j int) bool {
		if ri, rj := rank(repos[i]), rank(repos[j]); ri != rj {
			return ri < rj
		}
		return repos[i].Failures > repos[j].Failures
	})
	for _, x := range repos {
		if x.Result != "ok" {
			d.Bad++
		} else if d.OnlyProblems {
			continue
		}
		d.Rows = append(d.Rows, row{x, time.Duration(x.DurNS).Round(time.Millisecond).String()})
	}
	render(w, r, p, "i_repos")
}

// ---- 거부된 객체 ----

func iRejects(w http.ResponseWriter, r *http.Request) {
	p := psub(w, r, "rejects")
	type row struct{ URI, Reason string }
	type data struct {
		Rows    []row
		Reasons []kvInt
		Filter  string
	}
	d := &data{Filter: strings.TrimSpace(r.URL.Query().Get("q"))}
	p.Data = d
	b, _, err := fetchBytes("/rejects.txt", conf().TTL)
	if err != nil {
		p.apiErr(err)
		render(w, r, p, "i_rejects")
		return
	}
	cnt := map[string]int{}
	for _, line := range strings.Split(string(b), "\n") {
		u, reason, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		cnt[reason]++
		if d.Filter == "" || strings.Contains(strings.ToLower(u+" "+reason), strings.ToLower(d.Filter)) {
			d.Rows = append(d.Rows, row{u, reason})
		}
	}
	for k, n := range cnt {
		d.Reasons = append(d.Reasons, kvInt{k, n})
	}
	sort.Slice(d.Reasons, func(i, j int) bool { return d.Reasons[i].N > d.Reasons[j].N })
	render(w, r, p, "i_rejects")
}

// ---- 만료 임박 ----

func iExpiring(w http.ResponseWriter, r *http.Request) {
	p := psub(w, r, "expiring")
	q := r.URL.Query()
	days, _ := strconv.Atoi(q.Get("days"))
	switch days {
	case 1, 3, 7, 14, 30, 60, 90:
	default:
		days = 30
	}
	type row struct {
		O    Obj
		Left string
	}
	type data struct {
		Days, Total int
		ASN         string
		Type        string
		Rows        []row
		Cut         bool
	}
	d := &data{Days: days, ASN: q.Get("as"), Type: q.Get("type")}
	p.Data = d
	v := url.Values{"days": {strconv.Itoa(days)}, "limit": {strconv.Itoa(conf().MaxRows)}}
	if d.ASN != "" {
		n, ok := parseASN(d.ASN)
		if !ok {
			p.Err = p.T("bad_asn")
			render(w, r, p, "i_expiring")
			return
		}
		v.Set("as", strconv.Itoa(n))
	}
	switch d.Type {
	case "roa", "cer", "asa", "gbr":
		v.Set("type", d.Type)
	default:
		d.Type = ""
	}
	var e struct {
		Total   int   `json:"total"`
		Objects []Obj `json:"objects"`
	}
	if _, err := fetch("/api/v1/expiring?"+v.Encode(), &e); err != nil {
		p.apiErr(err)
	} else {
		d.Total, d.Cut = e.Total, e.Total > len(e.Objects)
		for _, o := range e.Objects {
			d.Rows = append(d.Rows, row{o, ago(o.NotAfter)})
		}
	}
	render(w, r, p, "i_expiring")
}

// ---- unsafe VRP ----

func iUnsafe(w http.ResponseWriter, r *http.Request) {
	p := psub(w, r, "unsafe")
	var u UnsafeResp
	if _, err := fetch("/api/v1/unsafe", &u); err != nil {
		p.apiErr(err)
	}
	if len(u.VRPs) > conf().MaxRows*2 {
		u.VRPs = u.VRPs[:conf().MaxRows*2]
	}
	p.Data = &u
	render(w, r, p, "i_unsafe")
}

// ---- SLURM ----

func iSlurm(w http.ResponseWriter, r *http.Request) {
	p := isub(w, r, "slurm")
	b, _, err := fetchBytes("/api/v1/slurm", conf().TTL)
	if err != nil {
		p.apiErr(err)
	}
	var v any
	if json.Unmarshal(b, &v) == nil {
		out, _ := json.MarshalIndent(v, "", " ")
		p.Data = string(out)
	}
	render(w, r, p, "i_slurm")
}

// ---- RTR 에 붙은 라우터 ----

func iRTR(w http.ResponseWriter, r *http.Request) {
	p := isub(w, r, "rtr")
	var st IStatus
	if _, err := fetch("/api/v1/status", &st); err != nil {
		p.apiErr(err)
	}
	sort.Slice(st.RTR, func(i, j int) bool { return st.RTR[i].Remote < st.RTR[j].Remote })
	p.Data = &st
	render(w, r, p, "i_rtr")
}

// ---- 객체 검색 / CA 트리 시작점 ----

const objPage = 100

func iObjects(w http.ResponseWriter, r *http.Request) {
	p := psub(w, r, "objects")
	q := r.URL.Query()
	type row struct {
		O       Obj
		Summary template.HTML
		Link    template.HTML
		Parent  template.HTML
	}
	type data struct {
		Type, Status, TA, AS, Prefix, URI string
		Rows                              []row
		Total, Page, Pages                int
		Bar                               *PageBar
		TAs                               []struct{ Name, URI string }
	}
	d := &data{Type: q.Get("type"), Status: q.Get("status"), TA: q.Get("ta"), AS: strings.TrimSpace(q.Get("as")),
		Prefix: strings.TrimSpace(q.Get("prefix")), URI: strings.TrimSpace(q.Get("uri"))}
	p.Data = d
	var st IStatus
	if _, err := fetch("/api/v1/status", &st); err == nil {
		for _, t := range st.TAs {
			d.TAs = append(d.TAs, struct{ Name, URI string }{t.Name, t.URI})
		}
	}
	v := url.Values{}
	has := false
	add := func(k, val string) {
		if val != "" {
			v.Set(k, val)
			has = true
		}
	}
	oneOf := func(val string, ok ...string) string {
		for _, o := range ok {
			if val == o {
				return val
			}
		}
		return ""
	}
	add("type", oneOf(d.Type, "cer", "crl", "mft", "roa", "asa", "gbr", "spl", "tak"))
	add("status", oneOf(d.Status, "valid", "rejected"))
	add("ta", oneOf(d.TA, "afrinic", "apnic", "arin", "lacnic", "ripe"))
	if d.Type == "router" || q.Get("kind") == "router" {
		add("kind", "router")
	}
	if d.AS != "" {
		if n, ok := parseASN(d.AS); ok {
			add("as", strconv.Itoa(n))
		} else {
			p.Err = p.T("bad_asn")
		}
	}
	if d.Prefix != "" {
		if pf, ok := parsePrefix(d.Prefix); ok {
			add("prefix", pf.String())
		} else {
			p.Err = p.T("bad_prefix")
		}
	}
	if d.URI != "" {
		if okURI(d.URI) {
			add("uri", d.URI)
		} else {
			p.Err = p.T("bad_uri")
		}
	}
	if has && p.Err == "" {
		page := pageParam(q)
		v.Set("limit", strconv.Itoa(objPage))
		var op ObjPage
		var err error
		for try := 0; try < 2; try++ { // 쪽 번호가 끝을 넘으면 마지막 쪽으로 한 번 더
			v.Set("offset", strconv.Itoa(page*objPage))
			if _, err = fetchT("/api/v1/objects?"+v.Encode(), &op, objTTL); err != nil {
				break
			}
			if pages := (op.Total + objPage - 1) / objPage; page < pages || pages == 0 {
				break
			} else {
				page = pages - 1
			}
		}
		if err != nil {
			p.apiErr(err)
		} else {
			d.Total, d.Page = op.Total, page
			d.Pages = (op.Total + objPage - 1) / objPage
			d.Bar = pageBar(p, page, d.Pages)
			for i := range op.Objects {
				o := op.Objects[i]
				d.Rows = append(d.Rows, row{o, objSummary(&o), objLink(o.URI, isCA(&o)), objLink(o.Parent, true)})
			}
		}
	}
	render(w, r, p, "i_objects")
}

// objSummary : 객체 목록의 "내용" 칸
func objSummary(o *Obj) template.HTML {
	s := &o.Summary
	switch o.Type {
	case "roa":
		var b []string
		for _, x := range s.Prefixes {
			b = append(b, fmt.Sprintf("%s-%d", esc(x.Prefix), x.MaxLength))
		}
		if len(b) > 6 {
			b = append(b[:6], fmt.Sprintf("+%d", len(s.Prefixes)-6))
		}
		return template.HTML(fmt.Sprintf(`<a href="/explorer?q=AS%d">AS%d</a> %s`, s.ASN, s.ASN, strings.Join(b, " ")))
	case "asa":
		return template.HTML(esc(fmt.Sprintf("AS%v → providers %v", s.Customer, s.Providers)))
	case "mft":
		return template.HTML(fmt.Sprintf("#%s, %d files", esc(s.Number), s.Files))
	case "crl":
		return template.HTML(fmt.Sprintf("%d revoked", s.Revoked))
	case "gbr":
		var b []string
		for _, k := range []string{"FN", "ORG", "EMAIL", "TEL"} {
			if x, ok := s.VCard[k]; ok {
				b = append(b, fmt.Sprint(x))
			}
		}
		return template.HTML(esc(strings.Join(b, " / ")))
	case "cer":
		if s.Kind == "router" {
			return template.HTML(esc(fmt.Sprintf("router ASNs %v", s.ASNs)))
		}
		return template.HTML(esc(s.Kind + " " + strings.TrimPrefix(s.Repository, "rsync://")))
	}
	return ""
}
