package memory

import (
	"strings"
	"testing"
	"time"
)

func TestSuccessorID(t *testing.T) {
	m := &Memory{Links: Links{
		{ID: "mem_a", Rel: "related"},
		{ID: "mem_b", Rel: "superseded-by"},
	}}
	if got := SuccessorID(m); got != "mem_b" {
		t.Fatalf("successor: %q", got)
	}
	if got := SuccessorID(&Memory{}); got != "" {
		t.Fatalf("无链应空: %q", got)
	}
}

func TestFreshnessBanner(t *testing.T) {
	old := &Memory{Status: "superseded", Links: Links{{ID: "mem_new", Rel: "superseded-by"}}}
	b := FreshnessBanner(old, "新结论 [mem_new]", "2026-08-15")
	if !strings.Contains(b, "已被取代") || !strings.Contains(b, "mem_new") || !strings.Contains(b, "2026-08-15") {
		t.Fatalf("superseded 横幅: %q", b)
	}
	// 继任者信息缺失时仍出横幅
	b = FreshnessBanner(old, "", "")
	if !strings.Contains(b, "已被取代") {
		t.Fatalf("缺继任者也应出横幅: %q", b)
	}
	b = FreshnessBanner(&Memory{Status: "stale"}, "", "")
	if !strings.Contains(b, "stale") {
		t.Fatalf("stale 横幅: %q", b)
	}
	if b := FreshnessBanner(&Memory{Status: "active"}, "", ""); b != "" {
		t.Fatalf("active 不应有横幅: %q", b)
	}
}

func TestStoreFreshnessNotice(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	now := time.Now()
	succ := &Memory{ID: "mem_new", Type: "pattern", Title: "新结论", Project: "global",
		Status: "active", Confidence: "human", Source: "test", Sensitivity: "normal",
		Created: now, Body: "b"}
	old := &Memory{ID: "mem_old", Type: "pattern", Title: "旧结论", Project: "global",
		Status: "superseded", Confidence: "human", Source: "test", Sensitivity: "normal",
		Created: now, Body: "b", Links: Links{{ID: "mem_new", Rel: "superseded-by"}}}
	for _, m := range []*Memory{succ, old} {
		if err := s.Write(m); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	notice := s.FreshnessNotice(old)
	if !strings.Contains(notice, "新结论 [mem_new]") {
		t.Fatalf("横幅应含继任者标题: %q", notice)
	}
	// active 记忆无横幅
	if n := s.FreshnessNotice(succ); n != "" {
		t.Fatalf("active 应无横幅: %q", n)
	}
}
