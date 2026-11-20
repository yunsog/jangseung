package main

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"
)

// 인증서 경로 : VRP 의 출처 ROA 에서 부모 CA 를 따라 TA 까지 (해태 /api/v1/objects).
// 객체는 잘 안 바뀌므로 10 분 캐시 (CA 는 prefix 가 달라도 공유 -> 해태 부담이 작음)

const (
	objTTL   = 10 * time.Minute
	chainMax = 12 // 단계 상한 (무한 고리 방지)
	maxCards = 10 // 한 화면에 그리는 경로 수 (VRP x ROA)
)

// getObj : uri 가 정확히 같은 객체 하나 (objects?uri= 는 앞부분 일치라 걸러야 함)
func getObj(uri string) *Obj {
	var l ObjList
	if _, err := fetchT("/api/v1/objects?limit=5&uri="+url.QueryEscape(uri), &l, objTTL); err != nil {
		return nil
	}
	for i := range l.Objects {
		if l.Objects[i].URI == uri {
			return &l.Objects[i]
		}
	}
	return nil
}

// getPub : CA (caURI) 의 저장소 (repo) 바로 안의 Manifest / CRL / GBR (typ).
// objects?uri= 는 앞부분 일치라 큰 CA (TA / RIR) 는 아래 CA 들의 것이 섞여 1 만 개가 넘음
// -> 1 만 개씩 쪽을 넘기며 "저장소 바로 아래 + 부모가 이 CA" 인 것을 찾음 (결과는 10 분 기억, 없다는 것도)
func getPub(caURI, repo, typ string) *Obj {
	key := caURI + "|" + typ
	pubMu.Lock()
	m, ok := pubMemo[key]
	pubMu.Unlock()
	if ok && time.Since(m.at) < objTTL {
		return m.o
	}
	o := findPub(caURI, repo, typ)
	pubMu.Lock()
	if len(pubMemo) > 5000 {
		pubMemo = map[string]pubEntry{}
	}
	pubMemo[key] = pubEntry{o, time.Now()}
	pubMu.Unlock()
	return o
}

type pubEntry struct {
	o  *Obj
	at time.Time
}

var (
	pubMu   sync.Mutex
	pubMemo = map[string]pubEntry{}
)

func findPub(caURI, repo, typ string) *Obj {
	for off := 0; off < 50000; off += 10000 { // 큰 응답은 캐시에 두지 않고 바로 훑음
		body, err := httpGet(fmt.Sprintf("/api/v1/objects?limit=10000&offset=%d&type=%s&uri=%s", off, typ, url.QueryEscape(repo)))
		if err != nil {
			return nil
		}
		var l struct {
			Total   int   `json:"total"`
			Objects []Obj `json:"objects"`
		}
		if json.Unmarshal(body, &l) != nil {
			return nil
		}
		for i := range l.Objects {
			o := &l.Objects[i]
			if o.Parent == caURI && !strings.Contains(strings.TrimPrefix(o.URI, repo), "/") {
				return o
			}
		}
		if off+10000 >= l.Total {
			break
		}
	}
	return nil
}

type Step struct {
	Depth    int
	Name     string // TA / CA / Manifest / CRL / ROA
	O        *Obj
	Muted    bool   // Manifest / CRL (CA 의 부속)
	Content  string // 이 단계가 담은 자원 / 내용
	Earliest bool   // VRP 만료를 정하는 단계
	Detail   []KV   // 행을 눌렀을 때 펼치는 내용
}

type Card struct {
	VRP       VRP
	ROA       string
	Steps     []Step
	NoTA      bool   // TA 까지 못 올라감
	Missing   bool   // ROA 자체를 못 찾음
	Expire    string // VRP 만료 (체인에서 가장 이른 notAfter)
	ROADetail []KV   // ROA 내용 (인증서 경로 위에 바로 보임)
}

func parseT2(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t }

// buildCard : ROA -> ... -> TA 를 위에서부터 (TA 먼저) 줄로
func buildCard(v VRP, roa string) Card {
	c := Card{VRP: v, ROA: roa}
	var chain []*Obj
	for u := roa; u != "" && len(chain) < chainMax; {
		o := getObj(u)
		if o == nil {
			break
		}
		chain = append(chain, o)
		u = o.Parent
	}
	if len(chain) == 0 || chain[0].Type != "roa" {
		c.Missing = true
		return c
	}
	target, _ := netip.ParsePrefix(v.Prefix)
	for d, i := 0, len(chain)-1; i >= 0; d, i = d+1, i-1 { // TA 가 깊이 0
		o := chain[i]
		name := o.Summary.Kind
		if o.Type == "roa" {
			name = "ROA"
		} else if name == "" {
			name = o.Type
		}
		c.Steps = append(c.Steps, Step{Depth: d, Name: name, O: o, Content: content(o, target), Detail: detailRows(o, target)})
		if o.Type == "cer" && o.Summary.Repository != "" { // CA 아래에 Manifest / CRL
			for _, t := range [][2]string{{"mft", "Manifest"}, {"crl", "CRL"}} {
				if p := getPub(o.URI, o.Summary.Repository, t[0]); p != nil {
					c.Steps = append(c.Steps, Step{Depth: d + 1, Name: t[1], O: p, Muted: true, Content: content(p, target), Detail: detailRows(p, target)})
				}
			}
		}
	}
	c.ROADetail = detailRows(chain[0], target)
	top := chain[len(chain)-1]
	c.NoTA = top.Summary.Kind != "TA"
	// VRP 만료 = 이 단계들의 notAfter (Manifest / CRL 은 다음 갱신) 중 가장 이른 것
	var first time.Time
	for _, s := range c.Steps {
		if t := parseT2(s.O.NotAfter); !t.IsZero() && (first.IsZero() || t.Before(first)) {
			first = t
		}
	}
	for i := range c.Steps {
		c.Steps[i].Earliest = !first.IsZero() && parseT2(c.Steps[i].O.NotAfter).Equal(first)
	}
	if !first.IsZero() {
		c.Expire = fmtT(first.UTC().Format(time.RFC3339))
	}
	return c
}

type pair struct {
	v   VRP
	roa string
}

// buildCards : ROA 하나당 카드 한 장 (같은 ROA 에 든 VRP 가 여럿이어도 ROA 정보는 한 번만).
// 대표 VRP = 그 ROA 의 VRP 중 조회한 prefix 에 해당하는 것 (모두 해당 안 되면 첫 번째). 앞에서 maxCards 개까지 나란히 받아 옴
// (해태 호출은 sem 이 동시 6 개로 제한)
func buildCards(rows []VRP) (cards []Card, cut bool) {
	var order []string // ROA 가 처음 나온 순서
	rep := map[string]VRP{}
	for _, v := range rows {
		for _, r := range v.ROAs {
			old, ok := rep[r]
			if !ok {
				order = append(order, r)
			}
			if !ok || (old.NoMatch && !v.NoMatch) {
				rep[r] = v
			}
		}
	}
	var ps []pair
	for _, r := range order {
		if len(ps) == maxCards {
			cut = true
			break
		}
		ps = append(ps, pair{rep[r], r})
	}
	cards = make([]Card, len(ps))
	var wg sync.WaitGroup
	for i, p := range ps {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cards[i] = buildCard(p.v, p.roa)
		}()
	}
	wg.Wait()
	return cards, cut
}

// content : 단계가 담은 내용 (CA = target 을 덮는 자원, ROA = prefix 목록, Manifest / CRL = 개수)
func content(o *Obj, target netip.Prefix) string {
	s := &o.Summary
	switch o.Type {
	case "mft":
		return fmt.Sprintf("#%s, %d files", s.Number, s.Files)
	case "crl":
		return fmt.Sprintf("%d revoked", s.Revoked)
	case "roa":
		// 수천 개일 수 있음 -> target 과 겹치는 것만 (없으면 앞 6 개) 보이고 나머지는 개수로
		var b []string
		for _, p := range s.Prefixes {
			if pp, err := netip.ParsePrefix(p.Prefix); err == nil && target.IsValid() && pp.Overlaps(target) {
				b = append(b, fmt.Sprintf("%s-%d", p.Prefix, p.MaxLength))
			}
		}
		if len(b) == 0 {
			for _, p := range s.Prefixes[:min(6, len(s.Prefixes))] {
				b = append(b, fmt.Sprintf("%s-%d", p.Prefix, p.MaxLength))
			}
		}
		if n := len(s.Prefixes) - len(b); n > 0 {
			b = append(b, fmt.Sprintf("(+%d)", n))
		}
		return fmt.Sprintf("AS%d: %s", s.ASN, strings.Join(b, " "))
	}
	return resHit(s.Resources, target)
}

// resHit : CA 의 자원 목록에서 target 을 덮는 항목 (prefix 또는 a-b 범위)
func resHit(res map[string][]any, target netip.Prefix) string {
	key := "ipv6"
	if target.Addr().Is4() {
		key = "ipv4"
	}
	list := res[key]
	for _, x := range list {
		e := fmt.Sprint(x)
		if p, err := netip.ParsePrefix(e); err == nil {
			if p.Bits() <= target.Bits() && p.Contains(target.Addr()) {
				return e
			}
		} else if a, b, ok := strings.Cut(e, "-"); ok {
			lo, e1 := netip.ParseAddr(a)
			hi, e2 := netip.ParseAddr(b)
			if e1 == nil && e2 == nil && lo.Compare(target.Addr()) <= 0 && hi.Compare(lastAddr(target)) >= 0 {
				return e
			}
		}
	}
	return fmt.Sprintf("%s: %d entries", key, len(list))
}

// lastAddr : prefix 의 마지막 주소
func lastAddr(p netip.Prefix) netip.Addr {
	b := p.Masked().Addr().AsSlice()
	for i := p.Bits(); i < len(b)*8; i++ {
		b[i/8] |= 0x80 >> (i % 8)
	}
	a, _ := netip.AddrFromSlice(b)
	return a
}
