package memory

import (
	"strings"
	"testing"
	"time"
)

func TestApplyReviewPolicyDecisionDefault(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.Local)
	m := &Memory{Type: "decision", Created: now}
	ApplyReviewPolicy(m)
	want := now.Add(DefaultDecisionReview)
	if !m.ReviewAt.Equal(want) {
		t.Fatalf("decision 缺省复核期应为 60 天: %v", m.ReviewAt)
	}
}

func TestApplyReviewPolicySkipsOthers(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.Local)
	for _, typ := range []string{"pattern", "gotcha", "api", "bugfix"} {
		m := &Memory{Type: typ, Created: now}
		ApplyReviewPolicy(m)
		if !m.ReviewAt.IsZero() {
			t.Errorf("%s 不应自动设 review_at", typ)
		}
	}
	// 显式设置的不被覆盖
	custom := now.Add(10 * 24 * time.Hour)
	m := &Memory{Type: "decision", Created: now, ReviewAt: custom}
	ApplyReviewPolicy(m)
	if !m.ReviewAt.Equal(custom) {
		t.Fatalf("显式 review_at 被覆盖: %v", m.ReviewAt)
	}
	// 未设 Created 时不拍脑袋
	m = &Memory{Type: "decision"}
	ApplyReviewPolicy(m)
	if !m.ReviewAt.IsZero() {
		t.Error("Created 未设时不应填 review_at")
	}
}

func TestOverdueForReview(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.Local)
	past := &Memory{Type: "decision", Status: "active", ReviewAt: now.Add(-time.Hour)}
	future := &Memory{Type: "decision", Status: "active", ReviewAt: now.Add(time.Hour)}
	super := &Memory{Type: "decision", Status: "superseded", ReviewAt: now.Add(-time.Hour)}
	unset := &Memory{Type: "decision", Status: "active"}
	if !past.OverdueForReview(now) {
		t.Error("过期 active 应命中")
	}
	if future.OverdueForReview(now) {
		t.Error("未到期不命中")
	}
	if super.OverdueForReview(now) {
		t.Error("superseded 不命中(已由读取兜底遮蔽)")
	}
	if unset.OverdueForReview(now) {
		t.Error("未设 review_at 不命中")
	}
}

// review_at 序列化/解析 roundtrip(进文件的事实源契约)
func TestReviewAtRoundtrip(t *testing.T) {
	m := &Memory{ID: "mem_t", Type: "decision", Title: "t", Project: "global",
		Status: "active", Confidence: "human", Source: "test", Sensitivity: "normal",
		Created: time.Now().Truncate(0), Body: "b"}
	m.ReviewAt = m.Created.AddDate(0, 2, 0)
	data, err := Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !got.ReviewAt.Equal(m.ReviewAt) {
		t.Fatalf("roundtrip drift: %v != %v", got.ReviewAt, m.ReviewAt)
	}
	// 未设时 frontmatter 里不出现该字段
	m2 := &Memory{ID: "mem_t2", Type: "pattern", Title: "t", Project: "global",
		Status: "active", Confidence: "human", Source: "test", Sensitivity: "normal",
		Created: time.Now().Truncate(0), Body: "b"}
	d2, _ := Marshal(m2)
	if strings.Contains(string(d2), "review_at") {
		t.Fatal("未设 review_at 时不应序列化该字段")
	}
}
