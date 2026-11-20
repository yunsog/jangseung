package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"time"
)

// KV : 상세 표의 한 줄. V 는 이미 이스케이프한 HTML (링크 포함)
type KV struct {
	K string
	V template.HTML
}

func esc(s string) string { return template.HTMLEscapeString(s) }

// objLink : 객체 화면 링크. CA / TA 인증서는 CA 화면
func objLink(uri string, ca bool) template.HTML {
	if uri == "" {
		return ""
	}
	page := "/object"
	if ca {
		page = "/ca"
	}
	return template.HTML(fmt.Sprintf(`<a href="%s?uri=%s" class="mono">%s</a>`, page, url.QueryEscape(uri), esc(strings.TrimPrefix(uri, "rsync://"))))
}

func isCA(o *Obj) bool { return o.Type == "cer" && (o.Summary.Kind == "CA" || o.Summary.Kind == "TA") }

func fmtT(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.In(kst).Format("2006-01-02 15:04") + " KST (UTC+9)"
}

func daysLeft(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return ""
	}
	d := int(time.Until(t).Hours() / 24)
	if time.Until(t) < 0 {
		return " (expired)"
	}
	return fmt.Sprintf(" (D-%d)", d)
}

// overlap : 자원 항목 ("a/n" 또는 "a-b") 이 target 과 겹치는지
func overlap(item string, target netip.Prefix) bool {
	if p, err := netip.ParsePrefix(item); err == nil {
		return p.Overlaps(target)
	}
	if a, b, ok := strings.Cut(item, "-"); ok {
		lo, e1 := netip.ParseAddr(strings.TrimSpace(a))
		hi, e2 := netip.ParseAddr(strings.TrimSpace(b))
		return e1 == nil && e2 == nil && lo.Compare(lastAddr(target)) <= 0 && hi.Compare(target.Masked().Addr()) >= 0
	}
	return false
}

// resList : 자원 목록 (종류마다 앞 n 개 + target 과 겹치는 것은 굵게, 겹치는 것은 n 개 밖이어도 보임)
func resList(res map[string][]any, target netip.Prefix, n int) template.HTML {
	var b strings.Builder
	for _, k := range []string{"ipv4", "ipv6", "asn"} {
		items := make([]string, 0, len(res[k]))
		for _, x := range res[k] {
			items = append(items, fmt.Sprint(x))
		}
		if len(items) == 0 {
			continue
		}
		same := target.IsValid() && ((k == "ipv4") == target.Addr().Is4()) && k != "asn"
		var show []string
		shown := 0
		for i, x := range items {
			hit := same && overlap(x, target)
			if i < n || hit {
				t := esc(x)
				if hit {
					t = "<b>" + t + "</b>"
				}
				show = append(show, t)
				shown++
			}
		}
		more := ""
		if shown < len(items) {
			more = fmt.Sprintf(` <span class="mut">+%d</span>`, len(items)-shown)
		}
		fmt.Fprintf(&b, "<div>%s %d : %s%s</div>", k, len(items), strings.Join(show, " "), more)
	}
	return template.HTML(b.String())
}

// detailRows : 객체 한 개의 중요한 정보 (해태 객체 상세 API 의 요약 범위)
func detailRows(o *Obj, target netip.Prefix) []KV {
	s := &o.Summary
	st := template.HTML(esc(o.Status))
	if o.Reason != "" {
		st += template.HTML(" " + esc(o.Reason))
	}
	rows := []KV{{"Path", template.HTML(`<span class="mono">` + esc(o.URI) + `</span>`)}, {"Status", st}, {"TA", template.HTML(esc(o.TA))}}
	if o.Type == "mft" || o.Type == "crl" { // thisUpdate / nextUpdate
		rows = append(rows, KV{"thisUpdate ~ nextUpdate", template.HTML(esc(fmtT(o.NotBefore) + " ~ " + fmtT(o.NotAfter) + daysLeft(o.NotAfter)))})
	} else {
		rows = append(rows, KV{"Valid", template.HTML(esc(fmtT(o.NotBefore) + " ~ " + fmtT(o.NotAfter) + daysLeft(o.NotAfter)))})
	}
	switch o.Type {
	case "mft":
		rows = append(rows, KV{"Number", template.HTML(esc(s.Number))}, KV{"Files", template.HTML(fmt.Sprint(s.Files))},
			KV{"CA", objLink(o.Parent, true)})
	case "crl":
		rows = append(rows, KV{"Revoked", template.HTML(fmt.Sprint(s.Revoked))}, KV{"CA", objLink(o.Parent, true)})
	case "roa":
		ps := roaPrefixHTML(s.Prefixes, target)
		rows = append(rows, KV{"AS", template.HTML(fmt.Sprintf(`<a href="/explorer?q=AS%d">AS%d</a>`, s.ASN, s.ASN))},
			KV{fmt.Sprintf("Prefixes (%d)", len(s.Prefixes)), ps}, KV{"Parent CA", objLink(o.Parent, true)})
	case "cer":
		rows = append(rows, KV{"Kind", template.HTML(esc(s.Kind))})
		if s.Kind == "router" { // BGPsec 라우터 인증서 : AS / SKI / 공개키 (공개키는 전체 목록에서)
			var as []string
			for _, a := range s.ASNs {
				as = append(as, fmt.Sprintf(`<a href="/explorer?q=AS%v">AS%v</a>`, a, a))
			}
			rows = append(rows, KV{"AS", template.HTML(strings.Join(as, " "))},
				KV{"SKI", template.HTML(`<span class="mono">` + esc(s.SKI) + `</span>`)})
			if ds := curData.Load(); ds != nil {
				for _, k := range ds.Keys {
					if k.SKI == strings.ToUpper(s.SKI) {
						rows = append(rows, KV{"Public key", template.HTML(`<span class="mono">` + esc(k.Pub) + `</span>`)})
					}
				}
			}
		}
		if o.Parent != "" {
			rows = append(rows, KV{"Parent", objLink(o.Parent, true)})
		}
		if s.Repository != "" {
			r := esc(s.Repository)
			if s.Notify != "" {
				r += "<br>RRDP " + esc(s.Notify)
			}
			rows = append(rows, KV{"Repository", template.HTML(r)})
		}
		if len(s.Resources) > 0 {
			counts := []string{}
			for _, k := range []string{"ipv4", "ipv6", "asn"} {
				if n := len(s.Resources[k]); n > 0 {
					counts = append(counts, fmt.Sprintf("%s %d", k, n))
				}
			}
			rows = append(rows, KV{"Resources", resList(s.Resources, target, 12) + template.HTML(`<div class="mut">`+strings.Join(counts, ", ")+`</div>`)})
		}
		if s.Repository != "" { // GBR 연락처
			if g := getPub(o.URI, s.Repository, "gbr"); g != nil {
				rows = append(rows, KV{"GBR", objLink(g.URI, false)})
			}
		}
	}
	rows = append(rows, KV{"sha256", template.HTML(`<span class="mono">` + esc(o.SHA256) + `</span>`)},
		KV{"Page", objLink(o.URI, isCA(o))})
	return rows
}

// ---- 객체 화면 / CA 화면 ----

func okURI(u string) bool {
	return len(u) < 500 && (strings.HasPrefix(u, "rsync://") || strings.HasPrefix(u, "https://"))
}

func objectPage(w http.ResponseWriter, r *http.Request) {
	p := newPage(w, r, "")
	u := r.URL.Query().Get("uri")
	type data struct {
		URI  string
		Rows []KV
		Raw  string
	}
	d := &data{URI: u}
	p.Data = d
	switch {
	case u == "":
	case !okURI(u):
		p.Err = p.T("bad_uri")
	default:
		if o := getObj(u); o == nil {
			p.Err = p.T("no_object")
		} else {
			d.Rows = detailRows(o, netip.Prefix{})
			b, _ := json.MarshalIndent(o, "", " ")
			d.Raw = string(b)
		}
	}
	render(w, r, p, "object")
}

type CAResp struct {
	URI        string           `json:"uri"`
	TA         string           `json:"ta"`
	Parent     string           `json:"parent"`
	Status     string           `json:"status"`
	Reason     string           `json:"reason"`
	Kind       string           `json:"kind"`
	SHA256     string           `json:"sha256"`
	NotBefore  string           `json:"notBefore"`
	NotAfter   string           `json:"notAfter"`
	Resources  map[string][]any `json:"resources"`
	Repository string           `json:"repository"`
	Notify     string           `json:"notify"`
	RepoResult string           `json:"repo_result"`
	Children   []struct {
		URI    string `json:"uri"`
		Type   string `json:"type"`
		Kind   string `json:"kind"`
		Status string `json:"status"`
	} `json:"children"`
	ChildrenTruncated bool           `json:"children_truncated"`
	Counts            map[string]int `json:"counts"`
}

type kvInt struct {
	K string
	N int
}

func caPage(w http.ResponseWriter, r *http.Request) {
	p := newPage(w, r, "")
	u := r.URL.Query().Get("uri")
	type child struct {
		Type, Status string
		Link         template.HTML
	}
	type data struct {
		URI      string
		CA       *CAResp
		Info     []KV
		Counts   []kvInt
		Children []child
		NChild   int
		Cut      bool
		Res      template.HTML
	}
	d := &data{URI: u}
	p.Data = d
	switch {
	case u == "":
	case !okURI(u):
		p.Err = p.T("bad_uri")
	default:
		var c CAResp
		if _, err := fetchT("/api/v1/ca?uri="+url.QueryEscape(u), &c, objTTL); err != nil {
			p.Err = p.T("no_object")
			break
		}
		d.CA = &c
		st := esc(c.Status)
		if c.Reason != "" {
			st += " " + esc(c.Reason)
		}
		repo := esc(c.Repository)
		if c.Notify != "" {
			repo += "<br>RRDP " + esc(c.Notify)
		}
		if c.RepoResult != "" {
			repo += " <b>" + esc(c.RepoResult) + "</b>"
		}
		parent := objLink(c.Parent, true)
		if parent == "" {
			parent = "(TA)"
		}
		d.Info = []KV{{"CA", template.HTML(`<span class="mono">` + esc(c.URI) + `</span>`)}, {"Status", template.HTML(st)}, {"TA", template.HTML(esc(c.TA))},
			{"Kind", template.HTML(esc(c.Kind))}, {"Parent", parent},
			{"Valid", template.HTML(esc(fmtT(c.NotBefore) + " ~ " + fmtT(c.NotAfter) + daysLeft(c.NotAfter)))},
			{"Repository", template.HTML(repo)}, {"sha256", template.HTML(`<span class="mono">` + esc(c.SHA256) + `</span>`)}}
		for k, n := range c.Counts {
			d.Counts = append(d.Counts, kvInt{k, n})
		}
		sort.Slice(d.Counts, func(i, j int) bool { return d.Counts[i].K < d.Counts[j].K })
		d.Res = resList(c.Resources, netip.Prefix{}, 300)
		sort.SliceStable(c.Children, func(i, j int) bool { // CA 먼저, 거부된 것 먼저
			a, b := c.Children[i], c.Children[j]
			if (a.Type == "cer") != (b.Type == "cer") {
				return a.Type == "cer"
			}
			if (a.Status == "valid") != (b.Status == "valid") {
				return a.Status != "valid"
			}
			return a.URI < b.URI
		})
		d.NChild, d.Cut = len(c.Children), c.ChildrenTruncated
		for i, ch := range c.Children {
			if i == conf().MaxRows {
				break
			}
			t := ch.Type
			if ch.Kind != "" {
				t += " " + ch.Kind
			}
			d.Children = append(d.Children, child{t, ch.Status, objLink(ch.URI, ch.Type == "cer" && ch.Kind != "router")})
		}
	}
	render(w, r, p, "ca")
}

// roaPrefixHTML : ROA 의 prefix 가 수천 개일 수 있음 -> target 과 겹치는 것만 먼저 보이고, 나머지는 접어 둠 (펼치면 전체)
func roaPrefixHTML(all []roaPfx, target netip.Prefix) template.HTML {
	const keep = 8 // 겹치는 것이 없을 때 앞에서 보이는 개수
	var hit, rest []string
	for _, p := range all {
		t := fmt.Sprintf(`<a href="/explorer?prefix=%s">%s</a>-%d`, url.QueryEscape(p.Prefix), esc(p.Prefix), p.MaxLength)
		if pp, err := netip.ParsePrefix(p.Prefix); err == nil && target.IsValid() && pp.Overlaps(target) {
			hit = append(hit, "<b>"+t+"</b>")
		} else {
			rest = append(rest, t)
		}
	}
	for len(hit) == 0 && len(hit) < keep && len(rest) > 0 { // 겹치는 것이 없으면 앞쪽 몇 개만
		hit = append(hit, rest[0])
		rest = rest[1:]
	}
	out := strings.Join(hit, " ")
	if len(rest) > 0 {
		out += fmt.Sprintf(` <details><summary>+%d</summary><div class="pfx">%s</div></details>`, len(rest), strings.Join(rest, " "))
	}
	return template.HTML(out)
}
