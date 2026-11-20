package main

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---- 추이 그래프 ----

type histData struct {
	Hours int
	Runs  []Run // 표 (최근 것 먼저, 최대 50)
	Chart template.HTML
	Held  []HeldRow // 반영 보류 회차 (모두, 최근 것 먼저)
}

func historyPage(w http.ResponseWriter, r *http.Request) {
	p := newPage(w, r, "history")
	h, _ := strconv.Atoi(r.URL.Query().Get("hours"))
	switch h {
	case 24, 72, 168, 720:
	default:
		h = 24
	}
	d := &histData{Hours: h}
	p.Data = d
	var hr HistoryResp
	if _, err := fetch(fmt.Sprintf("/api/v1/history?hours=%d", h), &hr); err != nil {
		p.apiErr(err)
	} else {
		d.Chart = lineChart(hr.Runs)
		lastOK := 0
		for _, x := range hr.Runs {
			if !x.Applied {
				d.Held = append(d.Held, HeldRow{Run: x, Drop: lastOK - x.VRPs})
			} else {
				lastOK = x.VRPs
			}
		}
		for i, j := 0, len(d.Held)-1; i < j; i, j = i+1, j-1 { // 최근 것 먼저
			d.Held[i], d.Held[j] = d.Held[j], d.Held[i]
		}
		for i := len(hr.Runs) - 1; i >= 0 && len(d.Runs) < 50; i-- {
			d.Runs = append(d.Runs, hr.Runs[i])
		}
	}
	render(w, r, p, "history")
}

// lineChart : VRP 수 선 그래프 (SVG). 반영된 회차만 선, 보류 회차는 빨간 점
func lineChart(runs []Run) template.HTML {
	if len(runs) < 2 {
		return ""
	}
	const W, H, L, B, T = 900.0, 220.0, 70.0, 22.0, 10.0
	lo, hi := 0, 0
	t0, t1 := parseT(runs[0].Start), parseT(runs[len(runs)-1].Start)
	for _, x := range runs { // 세로 범위 = 반영된 회차만. 보류 회차 (값이 크게 낮을 수 있음) 는 아래에 걸쳐 그림
		if !x.Applied {
			continue
		}
		if lo == 0 || x.VRPs < lo {
			lo = x.VRPs
		}
		if x.VRPs > hi {
			hi = x.VRPs
		}
	}
	if hi == 0 { // 반영된 회차가 없으면 전체로
		lo, hi = runs[0].VRPs, runs[0].VRPs
	}
	if hi == lo { // 평평한 선도 보이게
		hi++
	}
	pad := (hi - lo) / 10
	lo, hi = lo-pad, hi+pad
	X := func(t time.Time) float64 {
		if t1.Equal(t0) {
			return L
		}
		return L + (W-L-8)*float64(t.Sub(t0))/float64(t1.Sub(t0))
	}
	Y := func(v int) float64 { return T + (H-T-B)*(1-float64(v-lo)/float64(hi-lo)) }
	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %g %g" width="100%%" role="img" style="max-width:%gpx">`, W, H, W)
	for i := 0; i <= 3; i++ { // 가로 눈금
		v := lo + (hi-lo)*i/3
		y := Y(v)
		fmt.Fprintf(&b, `<line x1="%g" x2="%g" y1="%.1f" y2="%.1f" stroke="var(--line)"/><text x="%g" y="%.1f" font-size="11" text-anchor="end" fill="var(--mut)">%s</text>`, L, W-8, y, y, L-6, y+4, fmtNum(v))
	}
	for i := 0; i <= 4; i++ { // 시각 눈금 (KST)
		t := t0.Add(t1.Sub(t0) * time.Duration(i) / 4)
		fmt.Fprintf(&b, `<text x="%.1f" y="%g" font-size="11" text-anchor="middle" fill="var(--mut)">%s</text>`, X(t), H-6, t.In(kst).Format("01-02 15:04"))
	}
	var pts []string
	for _, x := range runs {
		if x.Applied {
			pts = append(pts, fmt.Sprintf("%.1f,%.1f", X(parseT(x.Start)), Y(x.VRPs)))
		}
	}
	fmt.Fprintf(&b, `<polyline fill="none" stroke="var(--acc)" stroke-width="1.6" points="%s"/>`, strings.Join(pts, " "))
	for _, x := range runs { // 보류 회차 : 세로 점선 (어느 범위에서도 보임) + 점 (범위 밖이면 위 / 아래 끝에), 마우스를 올리면 값과 이유
		if x.Applied {
			continue
		}
		xx, y := X(parseT(x.Start)), Y(x.VRPs)
		if y > H-B {
			y = H - B
		} else if y < T {
			y = T
		}
		tip := template.HTMLEscapeString(fmt.Sprintf("serial %d · %s · VRP %s · %s", x.Serial, fmtTime(x.Start), fmtNum(x.VRPs), x.Reason))
		fmt.Fprintf(&b, `<line x1="%.1f" x2="%.1f" y1="%g" y2="%g" stroke="var(--bad)" stroke-width="1.2" stroke-dasharray="4 3" opacity=".6"/><circle cx="%.1f" cy="%.1f" r="5" fill="var(--bad)" stroke="var(--panel)" stroke-width="1.5"><title>%s</title></circle>`, xx, xx, T, H-B, xx, y, tip)
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String()) // 숫자 / 고정 문자만 넣었으므로 안전
}

func parseT(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t }

// ---- 최근 변경 (회차 하나의 추가 / 삭제) ----

type changeData struct {
	Serial, Prev     int
	Added, Removed   []DeltaItem
	AddCut, RemCut   bool
	AddN, RemN       int
	Newest, Oldest   int
	HasPrev, HasNext bool
	Time             string // 이 회차의 데이터 시각
}

func changesPage(w http.ResponseWriter, r *http.Request) {
	p := newPage(w, r, "changes")
	d := &changeData{}
	p.Data = d
	if p.Status == nil {
		render(w, r, p, "changes")
		return
	}
	d.Newest = p.Status.Serial
	d.Oldest = d.Newest - 195 // 해태는 200 회차 보관 (실측 : 최신 -198 까지) -> 여유 있게
	d.Serial = d.Newest
	if n, err := strconv.Atoi(r.URL.Query().Get("serial")); err == nil {
		if n <= d.Newest && n > d.Oldest {
			d.Serial = n
		} else { // 보관 범위 밖 : 말없이 최신을 보이면 헷갈림 -> 알림
			p.Err = fmt.Sprintf(p.T("serial_out"), n, d.Oldest+1, d.Newest)
		}
	}
	d.Prev = d.Serial - 1
	d.HasPrev, d.HasNext = d.Prev > d.Oldest, d.Serial < d.Newest
	var dr DeltaResp
	u := fmt.Sprintf("/json-delta?session=%d&serial=%d&to=%d", p.Status.Session, d.Prev, d.Serial)
	if _, err := fetch(u, &dr); err != nil {
		p.apiErr(err)
	} else {
		d.Time = runTime(d.Serial)
		d.AddN, d.RemN = len(dr.Announced), len(dr.Withdrawn)
		d.Added, d.AddCut = capItems(dr.Announced)
		d.Removed, d.RemCut = capItems(dr.Withdrawn)
	}
	render(w, r, p, "changes")
}

func capItems(a []DeltaItem) ([]DeltaItem, bool) {
	if len(a) > conf().MaxRows {
		return a[:conf().MaxRows], true
	}
	return a, false
}

// runTime : 그 회차 (serial) 의 시각. /json-delta 의 generatedTime 은 회차마다 다르지 않아 /api/v1/history 의 start 를 씀
// (history 는 최근 30 일, 변경 화면은 최근 150 회차 = 약 25 시간이라 72 시간이면 충분). 못 찾으면 빈 값
func runTime(serial int) string {
	var hr HistoryResp
	if _, err := fetch("/api/v1/history?hours=72", &hr); err != nil {
		return ""
	}
	for _, x := range hr.Runs {
		if x.Serial == serial {
			return fmtTime(x.Start)
		}
	}
	return ""
}

// HeldRow : 반영 보류 회차 한 줄. Drop = 직전에 반영된 회차보다 줄어든 VRP 수
type HeldRow struct {
	Run
	Drop int
}
