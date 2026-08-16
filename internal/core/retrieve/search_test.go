package retrieve

import (
	"context"
	"math"
	"testing"

	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/core/memory"
	"time"
)

func TestCosine(t *testing.T) {
	if got := cosine([]float32{1, 0}, []float32{1, 0}); math.Abs(got-1) > 1e-9 {
		t.Fatalf("identical: %v", got)
	}
	if got := cosine([]float32{1, 0}, []float32{0, 1}); math.Abs(got) > 1e-9 {
		t.Fatalf("orthogonal: %v", got)
	}
	if got := cosine([]float32{}, []float32{1}); got != 0 {
		t.Fatalf("empty: %v", got)
	}
	if got := cosine([]float32{0, 0}, []float32{1, 0}); got != 0 {
		t.Fatalf("zero norm: %v", got)
	}
}

func TestNormalize(t *testing.T) {
	out := normalize(map[string]float64{"a": 10, "b": 5})
	if out["a"] != 1.0 || out["b"] != 0.5 {
		t.Fatalf("normalize: %v", out)
	}
	out = normalize(map[string]float64{})
	if len(out) != 0 {
		t.Fatal("空 map 应返回空")
	}
	// 全零不 panic
	out = normalize(map[string]float64{"a": 0})
	if out["a"] != 0 {
		t.Fatalf("全零: %v", out)
	}
}

func openTempIndex(t *testing.T) *index.Index {
	t.Helper()
	idx, err := index.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func addMem(t *testing.T, idx *index.Index, id, title, body, status string, vec []float32) {
	t.Helper()
	m := &memory.Memory{
		ID: id, Title: title, Body: body, Project: "myteam/my-service",
		Type: "bugfix", Status: status, Confidence: "human", Source: "test",
		Sensitivity: "normal", Entities: []string{"X" + id},
		Created: time.Now().UTC(), Path: "x/" + id + ".md",
	}
	if err := idx.Upsert(context.Background(), m, vec, "test"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
}

func TestSearchFusionAndStatusWeight(t *testing.T) {
	idx := openTempIndex(t)
	ctx := context.Background()
	addMem(t, idx, "mem_hit", "通知列表性能优化", "fetchPage N+1 全量遍历", "active", []float32{1, 0, 0})
	addMem(t, idx, "mem_other", "别的", "毫不相关的内容天气", "active", []float32{0, 1, 0})
	addMem(t, idx, "mem_stale", "通知列表性能优化", "条目相似但已过时", "stale", []float32{1, 0, 0})
	addMem(t, idx, "mem_arch", "通知列表性能优化", "归档不该出现", "archived", []float32{1, 0, 0})

	results, err := Search(ctx, idx, Request{
		Query: "通知 列表 性能", Limit: 5, QueryVec: []float32{1, 0, 0},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("无结果")
	}
	if results[0].ID != "mem_hit" {
		t.Fatalf("top1 应为 active 高分: %v", results[0])
	}
	for _, r := range results {
		if r.ID == "mem_arch" {
			t.Fatal("archived 不应被召回")
		}
	}
	// stale 降权：相同内容应远低于 active
	var active, stale float64
	for _, r := range results {
		if r.ID == "mem_hit" {
			active = r.Score
		}
		if r.ID == "mem_stale" {
			stale = r.Score
		}
	}
	if stale >= active {
		t.Fatalf("stale(%v) 应低于 active(%v)", stale, active)
	}
}

func TestSearchNoVectorFallback(t *testing.T) {
	idx := openTempIndex(t)
	ctx := context.Background()
	addMem(t, idx, "mem_hit", "电池保养指南", "避免过充过放", "active", nil)
	results, err := Search(ctx, idx, Request{Query: "电池 保养", Limit: 5})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 || results[0].ID != "mem_hit" {
		t.Fatalf("纯 FTS 检索失败: %v", results)
	}
}

func TestDedupCheck(t *testing.T) {
	idx := openTempIndex(t)
	ctx := context.Background()
	addMem(t, idx, "mem_a", "标题A", "内容A", "active", []float32{1, 0, 0})

	// 高相似度 → 命中
	hit, err := DedupCheck(ctx, idx, []float32{0.9999, 0.0001, 0}, 0.85)
	if err != nil || hit == nil {
		t.Fatalf("应查重命中: hit=%v err=%v", hit, err)
	}
	if hit.ID != "mem_a" || hit.Title != "标题A" {
		t.Fatalf("查重字段: %+v", hit)
	}
	// 低相似度 → 不命中
	hit, _ = DedupCheck(ctx, idx, []float32{0, 1, 0}, 0.85)
	if hit != nil {
		t.Fatalf("不应命中: %+v", hit)
	}
	// 无向量 → 直接跳过
	hit, _ = DedupCheck(ctx, idx, nil, 0.85)
	if hit != nil {
		t.Fatal("nil vec 应跳过")
	}
}

// superseded 默认不召回(防漂移);显式 IncludeSuperseded 时以 0.2 降权召回
func TestSearchSupersededExclusion(t *testing.T) {
	idx := openTempIndex(t)
	ctx := context.Background()
	addMem(t, idx, "mem_new", "通知列表性能优化", "连接池方案 v2", "active", []float32{1, 0, 0})
	addMem(t, idx, "mem_old", "通知列表性能优化", "连接池方案 v1(已被取代)", "superseded", []float32{1, 0, 0})

	// 默认:superseded 不出现
	results, err := Search(ctx, idx, Request{Query: "通知 列表 性能", Limit: 5, QueryVec: []float32{1, 0, 0}})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	for _, r := range results {
		if r.ID == "mem_old" {
			t.Fatal("superseded 默认不应被召回")
		}
	}
	foundNew := false
	for _, r := range results {
		if r.ID == "mem_new" {
			foundNew = true
		}
	}
	if !foundNew {
		t.Fatal("active 继任者应被召回")
	}

	// 显式溯源:superseded 召回且分数显著低于 active
	results, err = Search(ctx, idx, Request{
		Query: "通知 列表 性能", Limit: 5, QueryVec: []float32{1, 0, 0},
		IncludeSuperseded: true,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	var oldScore, newScore float64
	for _, r := range results {
		if r.ID == "mem_old" {
			oldScore = r.Score
		}
		if r.ID == "mem_new" {
			newScore = r.Score
		}
	}
	if oldScore == 0 {
		t.Fatal("IncludeSuperseded=true 时应召回 superseded")
	}
	if oldScore >= newScore {
		t.Fatalf("superseded(%v) 应低于 active(%v)", oldScore, newScore)
	}
}

func TestDedupScan(t *testing.T) {
	idx := openTempIndex(t)
	ctx := context.Background()
	addMem(t, idx, "mem_a", "标题A", "内容A", "active", []float32{1, 0, 0})
	addMem(t, idx, "mem_b", "标题B", "内容B", "active", []float32{0.98, 0.2, 0})
	addMem(t, idx, "mem_far", "无关", "完全不同的主题天气", "active", []float32{0, 1, 0})

	// 阈 0.5:两条高相关的都进候选,低相关的被过滤;按相似度降序,
	// 且每条带上标题(冲突判定的展示前提)
	hits, err := DedupScan(ctx, idx, []float32{1, 0, 0}, 0.5, 5)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("应召回 2 条候选: %+v", hits)
	}
	if hits[0].Score < hits[1].Score {
		t.Fatal("应按相似度降序")
	}
	if hits[0].Title == "" {
		t.Fatal("候选应回填标题")
	}

	// limit 截断
	hits, _ = DedupScan(ctx, idx, []float32{1, 0, 0}, 0.0, 1)
	if len(hits) != 1 {
		t.Fatalf("limit=1 应截断: %+v", hits)
	}
}
