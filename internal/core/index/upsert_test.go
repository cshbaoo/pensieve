package index

import (
	"context"
	"testing"
	"time"

	"github.com/cshbaoo/pensieve/internal/core/memory"
)

func openTemp(t *testing.T) *Index {
	t.Helper()
	idx, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { idx.Close() })
	return idx
}

func mem(id, title, body, project, mtype string, entities []string) *memory.Memory {
	return &memory.Memory{
		ID: id, Title: title, Body: body, Project: project,
		Type: mtype, Status: "active", Confidence: "human", Source: "test",
		Sensitivity: "normal", Entities: entities,
		Created: time.Now().UTC(), Path: project + "/2026/08/" + id + ".md",
	}
}

func TestUpsertAndMeta(t *testing.T) {
	idx := openTemp(t)
	ctx := context.Background()
	m := mem("mem_a", "标题甲", "正文包邮", "myteam/my-api", "gotcha", []string{"impl.go"})

	if err := idx.Upsert(ctx, m, []float32{1, 0, 0}, "test-embed"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	n, _ := idx.Count(ctx)
	if n != 1 {
		t.Fatalf("count: %d", n)
	}
	// 重复 upsert 不应报错且数量不变
	m2 := mem("mem_a", "标题甲-改", "正文改", "myteam/my-api", "gotcha", []string{"impl.go", "main.go"})
	if err := idx.Upsert(ctx, m2, []float32{1, 0, 0}, "test-embed"); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	if n, _ := idx.Count(ctx); n != 1 {
		t.Fatalf("重复 upsert count: %d", n)
	}
	row, _ := idx.GetRow(ctx, "mem_a")
	if row.Title != "标题甲-改" {
		t.Fatalf("未更新: %q", row.Title)
	}

	// meta 读写
	if err := idx.SetMeta(ctx, "k", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := idx.SetMeta(ctx, "k", "v2"); err != nil {
		t.Fatal(err)
	}
	if v, _ := idx.GetMeta(ctx, "k"); v != "v2" {
		t.Fatalf("meta: %q", v)
	}
}

func TestFTSAndEntity(t *testing.T) {
	idx := openTemp(t)
	ctx := context.Background()
	_ = idx.Upsert(ctx, mem("mem_a", "缓存列表接口优化", "refreshCacheStats 有 N+1 问题", "myteam/my-service", "bugfix", []string{"refreshCacheStats"}), nil, "")
	_ = idx.Upsert(ctx, mem("mem_b", "完全不同的主题", "今天天气不错多云转晴", "global", "pattern", []string{"天气"}), nil, "")

	hits, err := idx.FTSSearch(ctx, "缓存 列表 N+1", "", "", 10)
	if err != nil {
		t.Fatalf("fts: %v", err)
	}
	if _, ok := hits["mem_a"]; !ok {
		t.Fatalf("应命中 mem_a: %v", hits)
	}
	// project 过滤
	hits, _ = idx.FTSSearch(ctx, "天气", "myteam/my-service", "", 10)
	if len(hits) != 0 {
		t.Fatalf("project 过滤失效: %v", hits)
	}
	// 短词 LIKE 兜底
	like, _ := idx.LikeSearch(ctx, "天气", "", 10)
	if _, ok := like["mem_b"]; !ok {
		t.Fatalf("LIKE 兜底未命中: %v", like)
	}
	// 实体
	boost, _ := idx.EntityBoost(ctx, "refreshCacheStats 的问题该看哪里")
	if boost["mem_a"] != 1.0 {
		t.Fatalf("entity boost: %v", boost)
	}
	if _, ok := boost["mem_b"]; ok {
		t.Fatalf("不应命中 mem_b")
	}
}

func TestVectorCandidatesAndDelete(t *testing.T) {
	idx := openTemp(t)
	ctx := context.Background()
	_ = idx.Upsert(ctx, mem("mem_a", "A", "a", "myteam/my-service", "bugfix", nil), []float32{1, 0, 0}, "m")
	_ = idx.Upsert(ctx, mem("mem_b", "B", "b", "global", "pattern", nil), []float32{0, 1, 0}, "m")

	cands, err := idx.VecCandidates(ctx, "", "")
	if err != nil || len(cands) != 2 {
		t.Fatalf("candidates: %v %d", err, len(cands))
	}
	// project 过滤
	cands, _ = idx.VecCandidates(ctx, "global", "")
	if len(cands) != 1 {
		t.Fatalf("project 过滤向量: %d", len(cands))
	}

	// 删除全链路清理
	if err := idx.Delete(ctx, "mem_a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if n, _ := idx.Count(ctx); n != 1 {
		t.Fatalf("delete 后 count: %d", n)
	}
	if fts, _ := idx.FTSSearch(ctx, "A", "", "", 10); len(fts) != 0 {
		t.Fatalf("FTA 未清理: %v", fts)
	}
	cands, _ = idx.VecCandidates(ctx, "", "")
	if len(cands) != 1 {
		t.Fatalf("vec 未清理: %d", len(cands))
	}
	if row, _ := idx.GetRow(ctx, "mem_a"); row != nil {
		t.Fatal("元数据未清理")
	}
}
