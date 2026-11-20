package main

import (
	"fmt"
	"net/netip"
	"strings"
)

// Query : prefix 탐색기의 한 입력난을 해석한 것.
//
//	prefix 만              -> VRP 목록 + ROA 경로
//	AS 하나만              -> AS 조회 (VRP, ASPA ...)
//	AS 둘 이상 (AS_PATH)   -> ASPA 검사
//	prefix + AS            -> 경로 판정 (origin = 마지막 AS). AS 가 둘 이상이면 ASPA 검사도
type Query struct {
	Raw       string
	Prefix    netip.Prefix
	HasPrefix bool
	ASNs      []int
	SKI       string // 라우터 키의 SKI (16 진수 40 자)
	Path      string // ASPA API 에 줄 AS_PATH ({ } 는 AS_SET 으로 그대로)
	Err       string // 번역 키 (bad_prefix / bad_asn / many_prefix / bad_token)
	BadToken  string
}

// parseQuery : 공백 / 쉼표로 나눈 토큰마다 prefix (/ : . 포함) 인지 AS 번호인지 가름
func parseQuery(raw string) Query {
	q := Query{Raw: raw}
	var path []string
	for _, tok := range strings.FieldsFunc(raw, func(c rune) bool { return c == ' ' || c == ',' || c == '\t' || c == '>' }) {
		core := strings.Trim(tok, "{}")
		switch {
		case isSKI(core):
			q.SKI = strings.ToUpper(core)
		case strings.ContainsAny(core, "/:."):
			p, err := netip.ParsePrefix(core)
			switch {
			case q.HasPrefix:
				q.Err, q.BadToken = "many_prefix", tok
			case err != nil || p != p.Masked():
				q.Err, q.BadToken = "bad_prefix", tok
			default:
				q.Prefix, q.HasPrefix = p, true
			}
		default:
			n, ok := parseASN(core)
			if !ok {
				q.Err, q.BadToken = "bad_token", tok
				break
			}
			q.ASNs = append(q.ASNs, n)
			lead := len(tok) - len(strings.TrimLeft(tok, "{"))
			trail := len(tok) - len(strings.TrimRight(tok, "}"))
			path = append(path, strings.Repeat("{", lead)+fmt.Sprint(n)+strings.Repeat("}", trail))
		}
		if q.Err != "" {
			return q
		}
	}
	q.Path = strings.Join(path, " ")
	if len(q.ASNs) > 64 {
		q.Err = "bad_path"
	}
	return q
}

// isSKI : 16 진수 40 자 (라우터 인증서의 Subject Key Identifier)
func isSKI(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}
