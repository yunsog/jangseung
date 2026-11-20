package main

import (
	"net/http"
	"strings"
)

// 문구 사전 두 벌 (키 -> 문구). RFC 용어 / API 값은 번역하지 않음.
var dict = map[string]map[string]string{
	"ko": {
		"title": "장승 - RPKI", "nav_status": "현황", "nav_route": "경로 판정", "nav_as": "AS 조회", "nav_explorer": "탐색기",
		"ok": "정상", "bad": "이상", "serial": "시리얼", "data_time": "데이터 시각", "last_run": "마지막 회차",
		"took": "걸린 시간", "applied": "반영", "not_applied": "반영 보류", "per_ta": "TA 별 VRP 수", "total": "합계",
		"changes": "직전 회차 변화", "added": "추가", "removed": "삭제", "rejected": "거부된 객체",
		"asn": "AS 번호", "prefix": "Prefix", "check": "판정", "search": "조회",
		"route_help": "BGP 경로 (origin AS + prefix) 가 RPKI 로 유효한지 (RFC 6811) 확인합니다.",
		"state":      "판정", "matched": "일치한 VRP", "unmatched_as": "AS 가 달라 불일치", "unmatched_length": "maxLength 초과로 불일치",
		"none": "없음", "max_length": "maxLength", "ta": "TA", "expires": "만료", "roa": "출처 ROA",
		"as_vrps": "이 AS 의 VRP", "aspa": "ASPA (provider)", "provider_of": "이 AS 를 provider 로 둔 곳", "router_keys": "라우터 키",
		"covering": "덮는 VRP (lspec)", "more_specific": "안쪽 VRP (mspec)", "exact": "같은 prefix",
		"match": "범위", "m_any": "전체", "m_exact": "정확히 같음", "m_lspec": "덮는 것 (less specific)", "m_mspec": "안쪽 (more specific)",
		"vstate": "판정 필터", "v_any": "전체", "tab_list": "목록",
		"truncated": "너무 많아 일부만 보여 줍니다.", "no_match": "해당 안 됨", "goto": "이동", "no_match_why": "조회한 /%[1]d 에는 해당 안 됨 : maxLength %[2]d < %[1]d (RFC 6811)", "serial_out": "serial %d 의 변경은 해태가 보관하지 않습니다 (보관 범위 %d ~ %d). 최신 회차를 보여 줍니다.", "shown": "표시",
		"bgp_note":  "BGP 에 실제로 보이는 경로 기준의 Valid / Invalid 필터는 BGP 데이터 연동 (예정) 뒤에 됩니다. 지금은 RPKI 의 VRP 목록입니다.",
		"asn_check": "AS 를 같이 넣으면 그 AS 가 이 prefix 를 광고할 때의 판정도 보여 줍니다.",
		"bad_asn":   "AS 번호가 올바르지 않습니다.", "bad_prefix": "prefix 가 올바르지 않습니다 (예 211.44.103.0/24, 네트워크 주소여야 함).",
		"api_down": "해태 응답이 없습니다. 마지막 데이터를 보여 줍니다.", "api_none": "해태 응답이 없고 저장된 데이터도 없습니다.",
		"busy": "요청이 많습니다. 잠시 뒤 다시 시도하세요.", "source": "출처 해태 (Haetae)", "as_of": "기준",
		"explain_valid": "맞는 VRP 가 있음", "explain_invalid": "덮는 VRP 는 있으나 AS 또는 길이가 다름", "explain_not-found": "덮는 VRP 가 없음",
		"stale": "오래된 데이터", "age": "데이터 나이", "sec": "초", "no_aspa": "ASPA 없음",
		"nav_aspa": "ASPA 검사", "nav_history": "추이", "nav_changes": "최근 변경", "bad_path": "AS_PATH 가 올바르지 않습니다 (숫자를 공백으로, 예 64503 64502 64501).",
		"aspa_help": "AS_PATH 를 보이는 그대로 (왼쪽 = 이웃, 오른쪽 = origin) 넣으면 ASPA 로 경로 누출을 검사합니다. 받은 세션에 맞는 쪽을 보세요: upstream = 고객 / peer 에서, downstream = provider 에서 받은 경로.",
		"neighbor":  "이웃 AS", "history_help": "VRP 수 추이. 빨간 점 = 반영 보류 회차.", "held_n": "반영 보류 회차", "time": "시각", "older": "이전", "newer": "다음",
		"chain_title": "인증서 경로 (TA → CA → ROA)", "chain_help": "VRP 를 만든 ROA 가 어떤 인증서 단계를 거쳐 TA 까지 이어지는지 위 (TA) 에서 아래 (ROA) 순서로 보여 줍니다. 각 CA 아래는 그 CA 의 Manifest / CRL. Path 를 누르면 해시 / 유효기간 / 저장소를 펼칩니다.",
		"chain_cut": "경로가 많아 앞의 일부만 보여 줍니다.", "no_roa": "ROA 객체를 찾지 못함 :", "no_ta": "TA 까지 올라가지 못함 (부모 객체를 찾지 못함)",
		"vrp_expiry": "VRP 만료 (체인에서 가장 이른 만료)", "step": "단계", "content": "내용 (CA = 이 prefix 를 덮는 자원)", "earliest": "VRP 만료 기준",
		"nav_internal": "내부", "nav_ops": "운영", "i_home": "요약", "i_repos": "저장소 상태", "i_rejects": "거부된 객체", "i_expiring": "만료 임박", "i_objects": "객체 검색", "i_unsafe": "unsafe VRP", "i_rtr": "RTR 라우터",
		"i_title": "내부 화면", "i_note": "로그인 뒤", "i_problems": "문제", "i_showall": "전체 보기", "i_onlyprob": "문제만", "i_fail": "연속 실패", "i_lastok": "마지막 성공", "i_next": "다음 시도", "i_error": "오류",
		"i_filter": "찾기", "i_reasons": "거부 이유별", "i_list": "목록", "i_reason": "이유", "i_days": "며칠 안", "i_content": "내용", "i_unsafe_help": "거부된 CA 의 자원과 겹치는 VRP (경고만, VRP 는 그대로 나감). 그 CA 의 세부 ROA 가 사라지고 다른 곳의 넓은 ROA 때문에 세부 경로가 invalid 가 될 수 있음.",
		"i_rejca": "거부된 CA", "i_remote": "원격 주소", "i_ver": "버전", "i_since": "연결", "i_lastq": "마지막 질의", "i_trusted": "신뢰", "i_handover": "무중단 판 올리기", "i_prefix_match": "앞부분 일치", "i_catree": "CA 트리 (TA 에서 시작) :", "i_parent": "부모 CA",
		"q_desc": "ROA, AS, VRP, ASPA, 라우터 키를 조회하고 검증할 수 있습니다.", "nav_list": "목록", "held_help": "검증 결과가 이상해 이전 데이터를 그대로 둔 회차", "held_drop": "VRP 감소", "held_reason": "이유", "aspa_prov": "provider", "loading": "전체 목록을 불러오는 중입니다 (처음 한 번, 수 초). 잠시 뒤 자동으로 다시 읽습니다.", "pubkey": "공개키", "list_q": "검색 (prefix / AS / SKI)", "q_label": "prefix / AS / AS_PATH", "q_ph": "192.0.2.0/24 · AS64496 · AS64496 192.0.2.0/24 · 64497 64498 64496",
		"q_h_prefix": "prefix -> 이 prefix 의 VRP 목록과 ROA 별 인증서 경로", "q_h_as": "AS -> AS 조회 (VRP, ASPA, provider, 라우터 키)", "q_h_route": "AS + prefix -> 경로 판정 (valid / invalid / not-found) + VRP 목록",
		"q_h_path": "AS 둘 이상 (AS_PATH, 왼쪽 = 이웃, 오른쪽 = origin) -> ASPA 검사", "many_prefix": "prefix 는 하나만 넣을 수 있습니다", "bad_token": "알 수 없는 입력",
		"theme_auto": "브라우저 설정을 따름", "sec_input": "조회", "sec_vrps": "VRP 목록", "sec_roa": "ROA 정보", "sub_chain": "인증서 경로 (TA → CA → ROA)", "optional": "선택", "click_detail": "줄을 누르면 상세 정보를 펼칩니다", "bad_uri": "URI 는 rsync:// 또는 https:// 로 시작해야 합니다.", "no_object": "객체를 찾지 못했습니다.",
		"counts": "객체 수", "resources": "자원", "children": "자식", "type": "종류",
	},
	"en": {
		"title": "Jangseung - RPKI", "nav_status": "Status", "nav_route": "Route validity", "nav_as": "AS lookup", "nav_explorer": "Explorer",
		"ok": "OK", "bad": "Problem", "serial": "Serial", "data_time": "Data time", "last_run": "Last run",
		"took": "Duration", "applied": "Applied", "not_applied": "Not applied", "per_ta": "VRPs per TA", "total": "Total",
		"changes": "Changes in last run", "added": "Added", "removed": "Withdrawn", "rejected": "Rejected objects",
		"asn": "AS number", "prefix": "Prefix", "check": "Check", "search": "Search",
		"route_help": "Check whether a BGP route (origin AS + prefix) is valid under RPKI (RFC 6811).",
		"state":      "State", "matched": "Matching VRPs", "unmatched_as": "Different AS", "unmatched_length": "maxLength exceeded",
		"none": "none", "max_length": "maxLength", "ta": "TA", "expires": "Expires", "roa": "Source ROA",
		"as_vrps": "VRPs of this AS", "aspa": "ASPA (providers)", "provider_of": "Registered this AS as provider", "router_keys": "Router keys",
		"covering": "Covering VRPs (lspec)", "more_specific": "More specific VRPs (mspec)", "exact": "Same prefix",
		"match": "Match", "m_any": "Any", "m_exact": "Exact", "m_lspec": "Less specific (covering)", "m_mspec": "More specific",
		"vstate": "Validity filter", "v_any": "Any", "tab_list": "List",
		"truncated": "Too many rows; showing part of them.", "no_match": "no match", "goto": "Go", "no_match_why": "Does not match the queried /%[1]d: maxLength %[2]d < %[1]d (RFC 6811)", "serial_out": "Changes for serial %d are no longer kept by Haetae (available %d ~ %d). Showing the latest run.", "shown": "shown",
		"bgp_note":  "Valid / Invalid filtering of routes actually seen in BGP needs a BGP data integration (planned). This page lists RPKI VRPs.",
		"asn_check": "Add an AS to see the validity of that AS originating this prefix.",
		"bad_asn":   "Invalid AS number.", "bad_prefix": "Invalid prefix (e.g. 211.44.103.0/24; must be a network address).",
		"api_down": "Haetae is not responding. Showing the last data.", "api_none": "Haetae is not responding and nothing is cached.",
		"busy": "Too many requests. Try again shortly.", "source": "Source: Haetae", "as_of": "as of",
		"explain_valid": "a matching VRP exists", "explain_invalid": "covered by VRPs but AS or length differs", "explain_not-found": "no covering VRP",
		"stale": "stale data", "age": "Data age", "sec": "s", "no_aspa": "no ASPA",
		"nav_aspa": "ASPA check", "nav_history": "History", "nav_changes": "Changes", "bad_path": "Invalid AS_PATH (numbers separated by spaces, e.g. 64503 64502 64501).",
		"aspa_help": "Enter the AS_PATH as seen (left = neighbor, right = origin) to check for route leaks with ASPA. Use the side matching the session: upstream = from customer / peer, downstream = from provider.",
		"neighbor":  "Neighbor AS", "history_help": "VRP count over time. Red dot = run not applied.", "held_n": "Runs not applied", "time": "Time", "older": "Previous", "newer": "Next",
		"chain_title": "Certification path (TA → CA → ROA)", "chain_help": "For each VRP, the certificates from the TA down to the ROA that produced it. Each CA is followed by its Manifest / CRL. Click a path to show hash, validity and repository.",
		"chain_cut": "Too many paths; showing the first ones.", "no_roa": "ROA object not found:", "no_ta": "Could not reach a TA (parent object not found)",
		"vrp_expiry": "VRP expires (earliest expiry in the chain)", "step": "Step", "content": "Content (CA = resource covering this prefix)", "earliest": "decides VRP expiry",
		"nav_internal": "Internal", "nav_ops": "Operations", "i_home": "Summary", "i_repos": "Repositories", "i_rejects": "Rejected objects", "i_expiring": "Expiring", "i_objects": "Object search", "i_unsafe": "Unsafe VRPs", "i_rtr": "RTR routers",
		"i_title": "Internal", "i_note": "behind login", "i_problems": "problems", "i_showall": "show all", "i_onlyprob": "problems only", "i_fail": "Failures", "i_lastok": "Last success", "i_next": "Next try", "i_error": "Error",
		"i_filter": "Filter", "i_reasons": "By reason", "i_list": "List", "i_reason": "Reason", "i_days": "Within days", "i_content": "Content", "i_unsafe_help": "VRPs overlapping resources of a rejected CA (warning only; the VRPs are still served). Fine-grained ROAs of that CA vanish and broader ROAs elsewhere can make specific routes invalid.",
		"i_rejca": "Rejected CA", "i_remote": "Remote", "i_ver": "Version", "i_since": "Since", "i_lastq": "Last query", "i_trusted": "Trusted", "i_handover": "Zero-downtime upgrade", "i_prefix_match": "prefix match", "i_catree": "CA tree (from TA):", "i_parent": "Parent CA",
		"q_desc": "Look up and validate ROAs, ASes, VRPs, ASPA and router keys.", "nav_list": "Lists", "held_help": "Runs whose result looked wrong, so the previous data was kept", "held_drop": "VRP drop", "held_reason": "Reason", "aspa_prov": "providers", "loading": "Loading the full lists (first time only, a few seconds). This page reloads automatically.", "pubkey": "Public key", "list_q": "Search (prefix / AS / SKI)", "q_label": "Prefix / AS / AS_PATH", "q_ph": "192.0.2.0/24 · AS64496 · AS64496 192.0.2.0/24 · 64497 64498 64496",
		"q_h_prefix": "prefix -> VRPs of the prefix and certification path per ROA", "q_h_as": "AS -> AS lookup (VRPs, ASPA, providers, router keys)", "q_h_route": "AS + prefix -> route validity (valid / invalid / not-found) + VRP list",
		"q_h_path": "two or more ASes (AS_PATH, left = neighbor, right = origin) -> ASPA check", "many_prefix": "Only one prefix is allowed", "bad_token": "Unrecognized input",
		"theme_auto": "Follow browser setting", "sec_input": "Lookup", "sec_vrps": "VRPs", "sec_roa": "ROA details", "sub_chain": "Certification path (TA → CA → ROA)", "optional": "optional", "click_detail": "Click a row to expand details", "bad_uri": "URI must start with rsync:// or https://.", "no_object": "Object not found.",
		"counts": "Object counts", "resources": "Resources", "children": "Children", "type": "Type",
	},
}

// 언어 고르기 : 쿠키 > ?lang > Accept-Language (ko 가 앞이면 국문, 아니면 영문)
func pickLang(r *http.Request) string {
	if l := r.URL.Query().Get("lang"); l == "ko" || l == "en" {
		return l
	}
	if c, err := r.Cookie("lang"); err == nil && (c.Value == "ko" || c.Value == "en") {
		return c.Value
	}
	al := strings.ToLower(r.Header.Get("Accept-Language"))
	ko, en := strings.Index(al, "ko"), strings.Index(al, "en")
	if lg := conf().Lang; lg != "auto" { // 설정 [web] lang 이 ko / en 이면 브라우저 언어보다 이것 (쿠키 / ?lang 은 위에서 이미 우선)
		return lg
	}
	if ko >= 0 && (en < 0 || ko < en) {
		return "ko"
	}
	return "en"
}
