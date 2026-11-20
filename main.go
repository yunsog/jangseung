// 장승 (jangseung) : 해태 (RPKI RP) HTTP API 로 보여 주는 공개 웹 화면.
// 127.0.0.1 한 포트만 듣고, 바깥은 nginx 가 맡음.
package main

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/signal"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ---- 입력 검사 ----

// parseASN : "13335" / "AS13335" -> 13335
func parseASN(s string) (int, bool) {
	s = strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(s)), "AS")
	n, err := strconv.ParseUint(s, 10, 32)
	return int(n), err == nil
}

// parsePrefix : 네트워크 주소만 허용 (1.1.1.1/24 는 거절 = 해태도 400)
func parsePrefix(s string) (netip.Prefix, bool) {
	p, err := netip.ParsePrefix(strings.TrimSpace(s))
	if err != nil || p != p.Masked() {
		return p, false
	}
	return p, true
}

// ---- 화면 공통 ----

type Page struct {
	Lang    string
	Theme   string     // "day" / "night" / "" (= 브라우저 설정을 따름)
	Refresh bool       // 목록을 불러오는 중이면 몇 초 뒤 다시 읽음
	Sub     string     // 내부 화면의 소메뉴
	Query   url.Values // 지금 주소의 쿼리 (언어 / 테마 링크가 나머지 입력을 지키려고)
	Active  string
	Err     string // 사용자에게 보일 오류 (입력 / 해태)
	Stale   bool   // 해태가 멈춰 옛 캐시
	Health  *Health
	Status  *Status
	T       func(string) string
	Data    any
}

// With : 지금 쿼리에 k=v 만 바꾼 링크 ("?..."). 언어 / 테마를 바꿔도 입력한 prefix 등이 남음
func (p *Page) With(k, v string) string {
	q := url.Values{}
	for kk, vv := range p.Query {
		q[kk] = vv
	}
	q.Set(k, v)
	return "?" + q.Encode()
}

// isHTTPS : 이 요청이 https 로 왔는가 (직접 TLS 는 없으므로 trust_proxy 의 프록시가 알려 준 X-Forwarded-Proto 만 믿음)
func isHTTPS(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	return err == nil && conf().inTrusted(host) && r.Header.Get("X-Forwarded-Proto") == "https"
}

// pickTheme : ?theme= > 쿠키 > 없음 (브라우저 설정). ?theme=auto 는 쿠키를 지움
func pickTheme(w http.ResponseWriter, r *http.Request) string {
	set := func(v string, age int) {
		http.SetCookie(w, &http.Cookie{Name: "theme", Value: v, Path: "/", MaxAge: age, SameSite: http.SameSiteLaxMode, HttpOnly: true, Secure: isHTTPS(r)})
	}
	switch q := r.URL.Query().Get("theme"); q {
	case "day", "night":
		set(q, 365*86400)
		return q
	case "auto":
		set("", -1)
		return ""
	}
	if c, err := r.Cookie("theme"); err == nil && (c.Value == "day" || c.Value == "night") {
		return c.Value
	}
	return ""
}

func (p *Page) Other() string {
	if p.Lang == "ko" {
		return "en"
	}
	return "ko"
}

var kst = time.FixedZone("KST", 9*3600)

// fmtTime : RFC3339 -> "2026-11-19 23:48 KST (UTC+9)"
func fmtTime(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.In(kst).Format("2006-01-02 15:04") + " KST (UTC+9)"
}

func fmtEpoch(e int64) string {
	if e == 0 {
		return "-"
	}
	return time.Unix(e, 0).UTC().Format("2006-01-02")
}

func fmtNum(n int) string { // 천 단위 쉼표
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

var funcs = template.FuncMap{
	"time": fmtTime, "epoch": fmtEpoch, "num": fmtNum,
	"asn": func(n int) string { return "AS" + strconv.Itoa(n) },
	"roaName": func(u string) string { // ROA 주소는 끝 파일 이름만 (전체는 title)
		if i := strings.LastIndex(u, "/"); i >= 0 {
			return u[i+1:]
		}
		return u
	},
	"list2": func(t func(string) string, k string, l []VRPShort) map[string]any {
		return map[string]any{"T": t, "K": k, "L": l}
	},
	// rows : "vrps" 조각에 줄과 번역 함수를 같이 넘김
	"list4": func() []int { return []int{24, 72, 168, 720} },
	"inc":   func(n int) int { return n + 1 },
	"dl": func(t func(string) string, k string, items []DeltaItem, n int, cut bool) map[string]any {
		return map[string]any{"T": t, "K": k, "Items": items, "N": n, "Cut": cut}
	},
	"cc": func(t func(string) string, cards []Card, cut bool) map[string]any {
		return map[string]any{"T": t, "Cards": cards, "Cut": cut}
	},
	"date": func(s string) string {
		if len(s) >= 10 {
			return s[:10]
		}
		return s
	},
	"days": func(s string) string { // 남은 날 (D-n)
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return ""
		}
		return fmt.Sprintf("D%+d", int(time.Until(t).Hours()/24))
	},
	"pad":   func(n int) int { return n * 16 },
	"path":  func(u string) string { return strings.TrimPrefix(u, "rsync://") },
	"ago":   ago,
	"list7": func() []int { return []int{1, 3, 7, 14, 30, 60, 90} },
	"list5": func() []string { return []string{"", "roa", "cer", "asa", "gbr"} },
	"list9": func() []string { return []string{"", "cer", "crl", "mft", "roa", "asa", "gbr", "spl", "tak", "router"} },
	"list3": func() []string { return []string{"", "valid", "rejected"} },
	"list6": func() []string { return []string{"", "afrinic", "apnic", "arin", "lacnic", "ripe"} },
	"kt":    func(t func(string) string, k []KeyRec) map[string]any { return map[string]any{"T": t, "Keys": k} },
	"cutn": func(s string, n int) string {
		if len(s) <= n {
			return s
		}
		return s[:n] + "…"
	},
	"cut":   cutPath,
	"asset": assetURL,
	"vb":    func(t func(string) string, v *Validity) map[string]any { return map[string]any{"T": t, "V": v} },
	"ab":    func(t func(string) string, r *AspaResp) map[string]any { return map[string]any{"T": t, "R": r} },
	"rows":  func(t func(string) string, r []VRP) map[string]any { return map[string]any{"T": t, "Rows": r} },
}

var tmpl = template.Must(template.Must(template.New("").Funcs(funcs).Parse(templateText)).Parse(templateText2))
var _ = template.Must(tmpl.Parse(templateText3))
var _ = template.Must(tmpl.Parse(templateText4))
var _ = template.Must(tmpl.Parse(templateText5))
var _ = template.Must(tmpl.Parse(templateText6))

// newPage : 모든 화면 공통 준비 (언어, 해태 상태 = 아래 "기준" 표시용)
func newPage(w http.ResponseWriter, r *http.Request, active string) *Page {
	lang := pickLang(r)
	if q := r.URL.Query().Get("lang"); q == lang {
		http.SetCookie(w, &http.Cookie{Name: "lang", Value: lang, Path: "/", MaxAge: 365 * 86400, SameSite: http.SameSiteLaxMode, HttpOnly: true, Secure: isHTTPS(r)})
	}
	d := dict[lang]
	p := &Page{Lang: lang, Theme: pickTheme(w, r), Query: r.URL.Query(), Active: active, T: func(k string) string {
		if v, ok := d[k]; ok {
			return v
		}
		return k
	}}
	var h Health
	var st Status
	_, e1 := fetch("/healthz", &h)
	s2, e2 := fetch("/api/v1/status", &st)
	if e1 == nil {
		p.Health = &h
	}
	if e2 == nil {
		p.Status, p.Stale = &st, s2
	}
	if e1 != nil && e2 != nil {
		p.Err = p.T("api_none")
	} else if p.Stale {
		p.Err = p.T("api_down")
	}
	return p
}

// basePath : 프록시가 /rpki/ 같은 아래 경로로 줄 때 X-Forwarded-Prefix: /rpki 를 붙임. 설정 [web] base_path :
//
//	auto = 그 헤더를 믿음 (단, trust_proxy 에 든 주소에서 온 요청만 : 인터넷에 직접 열어도 헤더 위조로 링크를 바꾸지 못함)
//	/이름 = 고정, 비움 = 없음. 모양이 이상하면 무시 (링크 주입 방지)
var prefixRe = regexp.MustCompile(`^/[a-z0-9_-]{1,32}$`)

func basePath(r *http.Request) string {
	c := conf()
	if c.BasePath != "auto" {
		return c.BasePath
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || !c.inTrusted(host) {
		return ""
	}
	if p := r.Header.Get("X-Forwarded-Prefix"); prefixRe.MatchString(p) {
		return p
	}
	return ""
}

func render(w http.ResponseWriter, r *http.Request, p *Page, name string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Vary", "Accept-Language, Cookie")
	w.Header().Set("Cache-Control", "no-cache")
	if conf().RequireLogin { // 로그인 뒤의 내용은 공용 캐시에 남기지 않음
		w.Header().Set("Cache-Control", "private, no-store")
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, p); err != nil {
		log.Println("render:", err)
		return
	}
	out := buf.Bytes()
	if b := basePath(r); b != "" { // 모든 절대 링크 / 폼 앞에 /rpki 를 붙임
		out = bytes.ReplaceAll(out, []byte(`href="/`), []byte(`href="`+b+`/`))
		out = bytes.ReplaceAll(out, []byte(`action="/`), []byte(`action="`+b+`/`))
		out = bytes.ReplaceAll(out, []byte(`srcset="/`), []byte(`srcset="`+b+`/`))
		out = bytes.ReplaceAll(out, []byte(`src="/`), []byte(`src="`+b+`/`))
	}
	w.Write(out)
}

// apiErr : 해태 오류를 사용자 문구로
func (p *Page) apiErr(err error) {
	if errors.Is(err, errBusy) {
		p.Err = p.T("busy")
	} else {
		p.Err = p.T("api_none")
	}
}

// ---- 화면들 ----

func statusPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	p := newPage(w, r, "status")
	type row struct {
		TA string
		N  int
	}
	var rows []row
	if p.Status != nil {
		for k, v := range p.Status.LastRun.PerTA {
			rows = append(rows, row{strings.ToUpper(k), v})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].TA < rows[j].TA })
	}
	p.Data = rows
	render(w, r, p, "status")
}

// explorerPage : Cloudflare RPKI Explorer 의 prefix 검색.
// prefixMatch = exact / lspec (덮는 것) / mspec (안쪽) / 비우면 전체
func explorerPage(w http.ResponseWriter, r *http.Request) {
	p := newPage(w, r, "explorer")
	qv := r.URL.Query()
	type data struct {
		Q, Match    string
		Rows        []VRP
		Cut, Trunc  bool
		Cards       []Card
		CardCut     bool
		Total       int
		Prefix      string
		V           *Validity
		Aspa        *AspaResp
		AS          *ASResp
		ASCut       bool
		ASTotal     int
		ShowVRPs    bool
		Keys        []KeyRec
		KeysTotal   int
		KeysWait    bool
		KeysLoading bool
	}
	d := &data{Q: strings.TrimSpace(qv.Get("q")), Match: qv.Get("prefixMatch")}
	if d.Q == "" { // 옛 주소 (Cloudflare 와 같은 prefix= / asn=) 도 받음
		d.Q = strings.TrimSpace(qv.Get("prefix") + " " + qv.Get("asn"))
	}
	if d.Match != "exact" && d.Match != "lspec" && d.Match != "mspec" {
		d.Match = "any"
	}
	p.Data = d
	if d.Q == "" {
		render(w, r, p, "explorer")
		return
	}
	q := parseQuery(d.Q)
	if q.Err != "" {
		p.Err = p.T(q.Err) + " : " + q.BadToken
		render(w, r, p, "explorer")
		return
	}
	if q.SKI != "" || (len(q.ASNs) == 1 && !q.HasPrefix) { // 라우터 키 : SKI 로 찾기, AS 하나면 그 AS 의 키도
		var asn uint32
		if len(q.ASNs) > 0 {
			asn = uint32(q.ASNs[0])
		}
		if ds, _ := getData(latestSerial(p)); ds != nil {
			d.Keys, d.KeysTotal = ds.searchKeys(asn, q.SKI, 0, listPage)
		} else {
			d.KeysWait = q.SKI != ""
			d.KeysLoading = true
			p.Refresh = true
		}
	}
	origin := 0
	if n := len(q.ASNs); n > 0 {
		origin = q.ASNs[n-1] // 경로 판정의 origin = AS_PATH 의 오른쪽 끝
	}
	if q.HasPrefix {
		d.Prefix = q.Prefix.String()
		d.ShowVRPs = true
		var pr PrefixResp
		if _, err := fetch("/api/v1/prefix/"+d.Prefix, &pr); err != nil {
			p.apiErr(err)
		} else {
			d.Rows, d.Trunc = pickRows(&pr, q.Prefix, d.Match)
			d.Total = len(d.Rows)
			d.Cards, d.CardCut = buildCards(d.Rows)
			if len(d.Rows) > conf().MaxRows {
				d.Rows, d.Cut = d.Rows[:conf().MaxRows], true
			}
		}
		if len(q.ASNs) > 0 {
			var v Validity
			if _, err := fetch(fmt.Sprintf("/api/v1/validity/AS%d/%s", origin, q.Prefix), &v); err == nil {
				d.V = &v
			}
		}
	} else if len(q.ASNs) == 1 { // AS 하나 = AS 조회
		var a ASResp
		if _, err := fetch(fmt.Sprintf("/api/v1/as/%d", q.ASNs[0]), &a); err != nil {
			p.apiErr(err)
		} else {
			sort.Slice(a.VRPs, func(i, j int) bool { return a.VRPs[i].Prefix < a.VRPs[j].Prefix })
			d.ASTotal = len(a.VRPs)
			if len(a.VRPs) > conf().MaxRows {
				a.VRPs, d.ASCut = a.VRPs[:conf().MaxRows], true
			}
			d.AS = &a
		}
	}
	if len(q.ASNs) >= 2 { // AS_PATH = ASPA 검사 (이웃 = 맨 왼쪽 AS)
		var a AspaResp
		u := "/api/v1/aspa/verify?path=" + url.QueryEscape(q.Path) + "&neighbor=" + strconv.Itoa(q.ASNs[0])
		if _, err := fetch(u, &a); err != nil {
			p.apiErr(err)
		} else {
			d.Aspa = &a
		}
	}
	render(w, r, p, "explorer")
}

// pickRows : 범위에 맞는 VRP 만 골라 prefix 순으로
func pickRows(pr *PrefixResp, pf netip.Prefix, match string) (rows []VRP, trunc bool) {
	switch match {
	case "lspec":
		rows = append(rows, pr.Covering...)
	case "mspec":
		rows, trunc = append(rows, pr.MoreSpecific...), pr.MoreTruncated
	case "exact":
		for _, v := range append(append([]VRP{}, pr.Covering...), pr.MoreSpecific...) {
			if v.Prefix == pf.String() {
				rows = append(rows, v)
			}
		}
	default:
		rows = append(append(rows, pr.Covering...), pr.MoreSpecific...)
		trunc = pr.MoreTruncated
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, _ := netip.ParsePrefix(rows[i].Prefix)
		b, _ := netip.ParsePrefix(rows[j].Prefix)
		if a != b {
			if c := a.Addr().Compare(b.Addr()); c != 0 {
				return c < 0
			}
			return a.Bits() < b.Bits()
		}
		return rows[i].ASN < rows[j].ASN
	})
	for i := range rows { // 조회한 prefix 를 덮는데 (더 짧거나 같음) maxLength 가 그보다 짧으면 해당 안 됨
		if vp, err := netip.ParsePrefix(rows[i].Prefix); err == nil && vp.Bits() <= pf.Bits() && vp.Contains(pf.Addr()) && rows[i].MaxLength < pf.Bits() {
			rows[i].NoMatch, rows[i].TargetLen = true, pf.Bits()
		}
	}
	return
}

//go:embed static
var staticFS embed.FS // 로고 (실행 파일 안에 들어감)

// serveStatic / serveAsset : 임베드 파일은 수정 시각이 없어 브라우저가 매번 다시 받음
// -> 내용 해시를 ETag 로, Cache-Control 로 하루 동안 다시 묻지 않게 (바뀌면 ETag 가 달라 새로 받음)
func serveStatic(w http.ResponseWriter, r *http.Request) {
	serveAsset(w, r, "static/"+strings.TrimPrefix(r.URL.Path, "/static/"))
}

var etags sync.Map // 파일 이름 -> ETag (실행 파일 안 내용은 안 바뀜)

func serveAsset(w http.ResponseWriter, r *http.Request, name string) {
	data, err := staticFS.ReadFile(name)
	if err != nil || strings.Contains(name, "..") {
		http.NotFound(w, r)
		return
	}
	et, ok := etags.Load(name)
	if !ok {
		sum := sha256.Sum256(data)
		et = `"` + hex.EncodeToString(sum[:8]) + `"`
		etags.Store(name, et)
	}
	h := w.Header()
	h.Set("ETag", et.(string))
	h.Set("Cache-Control", "public, max-age=86400")
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
		h.Set("Content-Type", ct)
	}
	if r.Header.Get("If-None-Match") == et.(string) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Write(data)
}

func healthz(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "ok") }

// version : 장승 판 번호 (바꾸면 README / 슬라이드 / git 태그도)
const version = "0.1.0"

func main() {
	confPath := flag.String("c", "", "설정 파일 (없으면 "+defaultConfPath+" 가 있을 때만)")
	listen := flag.String("listen", "", "듣는 주소 (설정의 [listen] addr 보다 우선)")
	haetae := flag.String("haetae", "", "해태 HTTP API 주소 (설정의 [haetae] url 보다 우선)")
	check := flag.Bool("check", false, "설정만 검사하고 끝냄")
	showVer := flag.Bool("version", false, "판 번호를 쓰고 끝냄")
	flag.Parse()
	if *showVer {
		fmt.Println("jangseung", version)
		return
	}
	c, path, err := loadConf(*confPath)
	if err == nil {
		if *listen == "" && flag.NArg() > 0 { // 옛 방식 : jangseung 127.0.0.1:8330
			*listen = flag.Arg(0)
		}
		if *listen != "" {
			c.Listen = *listen
		}
		if *haetae != "" {
			c.HaetaeURL = strings.TrimRight(*haetae, "/")
		}
		err = c.validate()
	}
	if err != nil {
		log.Fatal("설정 오류 : ", err)
	}
	if *check {
		fmt.Println("설정 정상", path)
		return
	}
	curConf.Store(c)
	initClients(c)
	go reloadOnHUP(path, *listen, *haetae)
	addr := c.Listen
	mux := http.NewServeMux()
	mux.HandleFunc("/", statusPage)
	mux.HandleFunc("/route", oldLink)
	mux.HandleFunc("/as", oldLink)
	mux.HandleFunc("/as/", oldLink)
	mux.HandleFunc("/aspa", oldLink)
	mux.HandleFunc("/explorer", explorerPage)
	mux.HandleFunc("/history", historyPage)
	mux.HandleFunc("/changes", changesPage)
	mux.HandleFunc("/object", objectPage)
	mux.HandleFunc("/list", listPageHandler)
	mux.HandleFunc("/internal/", internalOnly(iHome))
	mux.HandleFunc("/internal/ping", internalOnly(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })) // 메뉴가 로그인 여부를 알아봄
	mux.HandleFunc("/repos", iRepos)
	mux.HandleFunc("/internal/repos", movedOps) // 옛 주소 -> 공개 주소
	mux.HandleFunc("/rejects", iRejects)
	mux.HandleFunc("/internal/rejects", movedOps) // 옛 주소 -> 공개 주소
	mux.HandleFunc("/expiring", iExpiring)
	mux.HandleFunc("/internal/expiring", movedOps) // 옛 주소 -> 공개 주소
	mux.HandleFunc("/unsafe", iUnsafe)
	mux.HandleFunc("/internal/unsafe", movedOps) // 옛 주소 -> 공개 주소
	mux.HandleFunc("/internal/slurm", internalOnly(iSlurm))
	mux.HandleFunc("/internal/rtr", internalOnly(iRTR))
	mux.HandleFunc("/objects", iObjects)
	mux.HandleFunc("/internal/objects", movedOps) // 옛 주소 -> 공개 주소
	mux.HandleFunc("/ca", caPage)
	mux.HandleFunc("/static/", serveStatic)
	mux.HandleFunc("/favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		serveAsset(w, r, "static/jangseung_mark.svg")
	})
	mux.HandleFunc("/jangseung-healthz", healthz)
	srv := &http.Server{Addr: addr, Handler: secHeaders(loginGate(mux)), MaxHeaderBytes: 32 << 10, ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout: 60 * time.Second, IdleTimeout: 2 * time.Minute}
	ln, err := net.Listen("tcp", addr) // 포트를 연 뒤에 READY (systemd Type=notify)
	if err != nil {
		log.Fatal(err)
	}
	go keepFresh()
	log.Printf("장승 %s 시작 %s (해태 %s, 내부 화면 %v, 로그인 필수 %v, 설정 %s)", version, addr, c.HaetaeURL, c.InternalOn, c.RequireLogin, orNone(path))
	sdNotify("READY=1")
	go watchdog(addr)
	log.Fatal(srv.Serve(ln))
}

func orNone(s string) string {
	if s == "" {
		return "없음 (기본값)"
	}
	return s
}

// initClients : 설정의 timeout 으로 해태 API 클라이언트를 만듦 (바뀌면 restart)
func initClients(c *Config) {
	client = &http.Client{Timeout: c.Timeout}
	client2 = &http.Client{Timeout: 3 * time.Minute}
}

// reloadOnHUP : systemctl reload = SIGHUP -> 설정을 다시 읽음. 틀리면 지금 값 유지.
// listen / timeout 은 restart 해야 바뀜 (바뀐 파일이면 지금 값을 유지하고 경고)
func reloadOnHUP(path, listenFlag, haetaeFlag string) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)
	for range ch {
		sdNotify("RELOADING=1\nMONOTONIC_USEC=" + strconv.FormatInt(time.Now().UnixMicro(), 10))
		nc, _, err := loadConf(path)
		if err == nil {
			if haetaeFlag != "" {
				nc.HaetaeURL = strings.TrimRight(haetaeFlag, "/")
			}
			err = nc.validate()
		}
		old := conf()
		if err != nil {
			log.Println("설정 다시 읽기 실패, 지금 값 유지 :", err)
		} else {
			if (nc.Listen != old.Listen && listenFlag == "") || nc.Timeout != old.Timeout {
				log.Println("listen / timeout 은 restart 해야 바뀜 : 지금 값 유지")
			}
			nc.Listen, nc.Timeout = old.Listen, old.Timeout
			curConf.Store(nc)
			log.Println("설정 다시 읽음 :", path)
		}
		sdNotify("READY=1")
	}
}

// cutPath : 긴 URI 를 앞 / 뒤만 남기고 가운데를 줄임 (rsync:// 는 뺌, 뒤 = 파일 이름이 보이게). 전체는 title 에
func cutPath(u string) string {
	u = strings.TrimPrefix(u, "rsync://")
	const head, tail = 28, 30
	r := []rune(u)
	if len(r) <= head+tail+1 {
		return u
	}
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}

// assetURL : 정적 파일 주소 + 내용 해시 (?v=) -> 파일이 바뀌면 주소가 바뀌어 옛 캐시를 안 씀
func assetURL(name string) string {
	data, err := staticFS.ReadFile("static/" + name)
	if err != nil {
		return "/static/" + name
	}
	sum := sha256.Sum256(data)
	return "/static/" + name + "?v=" + hex.EncodeToString(sum[:4])
}

// oldLink : 경로 판정 / AS 조회 / ASPA 검사는 prefix 탐색기 하나로 합침. 옛 주소 (/route /as /as/N /aspa) 는 탐색기로 보냄
func oldLink(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var parts []string
	for _, k := range []string{"asn", "prefix", "path"} {
		if v := strings.TrimSpace(q.Get(k)); v != "" {
			parts = append(parts, v)
		}
	}
	if n := strings.TrimPrefix(r.URL.Path, "/as/"); n != r.URL.Path && n != "" {
		parts = append(parts, n)
	}
	to := "/explorer"
	if len(parts) > 0 {
		to += "?q=" + url.QueryEscape(strings.Join(parts, " "))
	}
	if b := basePath(r); b != "" {
		to = b + to
	}
	http.Redirect(w, r, to, http.StatusMovedPermanently)
}

// movedOps : 옛 /internal/<화면> 은 공개 /<화면> 으로 옮김 (북마크 유지). 쿼리도 그대로
func movedOps(w http.ResponseWriter, r *http.Request) {
	u := basePath(r) + strings.TrimPrefix(r.URL.Path, "/internal")
	if r.URL.RawQuery != "" {
		u += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, u, http.StatusMovedPermanently)
}
