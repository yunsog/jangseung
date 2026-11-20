package main

import (
	"os"
	"testing"
)

// 변경분 반영 시험 (해태에 접속하므로 평소에는 건너뜀).
//
//	JANGSEUNG_TEST_FULL=<옛 전체 /json-delta 파일> go test -run TestDelta -v
//
// 옛 전체에 변경분을 적용한 결과가, 지금 새로 받은 전체와 한 항목도 다르지 않아야 함
func TestDelta(t *testing.T) {
	path := os.Getenv("JANGSEUNG_TEST_FULL")
	if path == "" {
		t.Skip("JANGSEUNG_TEST_FULL 없음")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	old, err := parseFull(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("옛 전체 serial %d : v4 %d v6 %d aspa %d keys %d", old.Serial, len(old.V4), len(old.V6), len(old.ASPA), len(old.Keys))
	nd, err := applyDelta(old)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := loadData()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("변경분 적용 serial %d / 새 전체 serial %d", nd.Serial, fresh.Serial)
	if nd.Serial != fresh.Serial {
		t.Skip("시험 중 회차가 바뀜, 다시 실행")
	}
	if len(nd.V4) != len(fresh.V4) || len(nd.V6) != len(fresh.V6) || len(nd.ASPA) != len(fresh.ASPA) || len(nd.Keys) != len(fresh.Keys) {
		t.Fatalf("개수 다름 v4 %d/%d v6 %d/%d aspa %d/%d keys %d/%d", len(nd.V4), len(fresh.V4), len(nd.V6), len(fresh.V6), len(nd.ASPA), len(fresh.ASPA), len(nd.Keys), len(fresh.Keys))
	}
	for i := range fresh.V4 {
		if nd.V4[i] != fresh.V4[i] {
			t.Fatalf("v4 %d 다름", i)
		}
	}
	for i := range fresh.V6 {
		if nd.V6[i] != fresh.V6[i] {
			t.Fatalf("v6 %d 다름", i)
		}
	}
	for i := range fresh.ASPA {
		a, b := nd.ASPA[i], fresh.ASPA[i]
		if a.Customer != b.Customer || len(a.Providers) != len(b.Providers) {
			t.Fatalf("aspa %d 다름 %v %v", i, a, b)
		}
		for j := range a.Providers {
			if a.Providers[j] != b.Providers[j] {
				t.Fatalf("aspa %d provider 다름", i)
			}
		}
	}
	for i := range fresh.Keys {
		if nd.Keys[i] != fresh.Keys[i] {
			t.Fatalf("key %d 다름", i)
		}
	}
	t.Logf("일치 : v4 %d v6 %d aspa %d keys %d", len(fresh.V4), len(fresh.V6), len(fresh.ASPA), len(fresh.Keys))
}
