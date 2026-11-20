package main

import (
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// 전체 목록 화면 : /list?kind=vrp|aspa|keys&q=&p= . 데이터 = data.go (메모리에 줄여 둔 /json-delta 전체)

const listPage = 100

func latestSerial(p *Page) int {
	if p.Health != nil {
		return p.Health.Serial
	}
	return 0
}

func listPageHandler(w http.ResponseWriter, r *http.Request) {
	p := newPage(w, r, "list")
	q := r.URL.Query()
	kind := q.Get("kind")
	if kind != "aspa" && kind != "keys" {
		kind = "vrp"
	}
	p.Sub = kind
	type data struct {
		Kind, Q, Match string
		Total, Page    int
		Pages          int
		Bar            *PageBar
		VRPs           []VRPRow
		ASPAs          []AspaRec
		Keys           []KeyRec
		Snapshot       string
		Loading        bool
	}
	d := &data{Kind: kind, Q: strings.TrimSpace(q.Get("q")), Match: q.Get("prefixMatch")}
	if d.Match != "exact" && d.Match != "lspec" && d.Match != "mspec" {
		d.Match = "any"
	}
	p.Data = d
	qq := parseQuery(d.Q)
	if qq.Err != "" {
		p.Err = p.T(qq.Err) + " : " + qq.BadToken
		render(w, r, p, "list")
		return
	}
	ds, loading := getData(latestSerial(p))
	if ds == nil {
		d.Loading, p.Refresh = true, true
		loadMu.Lock()
		if loadErr != "" && !loading {
			p.Err = loadErr
		}
		loadMu.Unlock()
		render(w, r, p, "list")
		return
	}
	_ = loading
	d.Snapshot = "serial " + strconv.Itoa(ds.Serial) + " · " + fmtTime(ds.Time)
	var asn uint32
	if len(qq.ASNs) > 0 {
		asn = uint32(qq.ASNs[0])
	}
	page := pageParam(q)
	for try := 0; try < 2; try++ { // 쪽 번호가 끝을 넘으면 마지막 쪽으로 한 번 더
		off := page * listPage
		switch kind {
		case "vrp":
			d.VRPs, d.Total = ds.searchVRP(asn, qq.Prefix, qq.HasPrefix, d.Match, off, listPage)
		case "aspa":
			d.ASPAs, d.Total = ds.searchASPA(asn, off, listPage)
		case "keys":
			d.Keys, d.Total = ds.searchKeys(asn, qq.SKI, off, listPage)
		}
		d.Pages = (d.Total + listPage - 1) / listPage
		if page < d.Pages || d.Pages == 0 {
			break
		}
		page = d.Pages - 1
	}
	d.Page = page
	d.Bar = pageBar(p, page, d.Pages)
	render(w, r, p, "list")
}

var _ = netip.Prefix{}

// PageLink : 쪽 번호 막대의 한 칸. Gap = "…", Cur = 지금 쪽 (링크 아님). N 은 1 부터 (주소의 p 는 0 부터)
type PageLink struct {
	N        int
	URL      string
	Cur, Gap bool
}

// PageBar : 쪽 번호 막대 하나 (목록 / 객체 검색 공용). Keep = "쪽 번호로 이동" 폼이 그대로 실어 보낼 나머지 입력
type PageBar struct {
	Nav         []PageLink
	Prev, Next  string
	Page, Pages int
	Keep        []Pair
	Go          string
}

type Pair struct{ K, V string }

// pageParam : 주소의 쪽 번호 (0 부터). pg = 사람이 입력한 번호 (1 부터, 우선), p = 막대 링크의 번호 (0 부터)
func pageParam(q url.Values) int {
	if g, err := strconv.Atoi(strings.TrimSpace(q.Get("pg"))); err == nil && g >= 1 {
		return g - 1
	}
	if n, err := strconv.Atoi(q.Get("p")); err == nil && n > 0 {
		return n
	}
	return 0
}

// pageBar : 쪽이 하나뿐이면 nil (막대를 그리지 않음)
func pageBar(p *Page, cur, pages int) *PageBar {
	nav := pageNav(p, cur, pages)
	if nav == nil {
		return nil
	}
	b := &PageBar{Nav: nav, Page: cur, Pages: pages, Go: p.T("goto")}
	if cur > 0 {
		b.Prev = p.With("p", strconv.Itoa(cur-1))
	}
	if cur+1 < pages {
		b.Next = p.With("p", strconv.Itoa(cur+1))
	}
	keys := make([]string, 0, len(p.Query))
	for k := range p.Query {
		if k != "p" && k != "pg" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range p.Query[k] {
			b.Keep = append(b.Keep, Pair{k, v})
		}
	}
	return b
}

// pageNav : 1 2 … 8 9 [10] 11 12 … 99 모양. 처음 / 끝 쪽과 지금 쪽 앞뒤 2 쪽을 보이고, 빠진 구간은 …
// (빠진 쪽이 하나뿐이면 … 대신 그 쪽을 보임). 쪽이 하나면 빈 목록
func pageNav(p *Page, cur, pages int) []PageLink {
	if pages < 2 {
		return nil
	}
	link := func(i int) PageLink {
		l := PageLink{N: i + 1, Cur: i == cur}
		if i != cur {
			l.URL = p.With("p", strconv.Itoa(i))
		}
		return l
	}
	var out []PageLink
	prev := -1 // 마지막으로 낸 쪽 (0 부터)
	for i := 0; i < pages; i++ {
		if i != 0 && i != pages-1 && (i < cur-2 || i > cur+2) {
			continue // 보이지 않는 쪽
		}
		switch i - prev {
		case 1: // 이어짐
		case 2: // 빠진 쪽이 하나뿐 -> … 대신 그 쪽
			out = append(out, link(i-1))
		default:
			out = append(out, PageLink{Gap: true})
		}
		out = append(out, link(i))
		prev = i
	}
	return out
}
