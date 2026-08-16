package retrieve

import (
	"context"
	"testing"
	"time"

	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/core/memory"
)

// 纯向量且余弦低于地板(0.55) → 不召回
func TestNoiseFiltering(t *testing.T) {
	idx := openTempIndex(t)
	ctx := context.Background()
	// 完全无关的记忆
	addMem(t, idx, "mem_unl", "明天午饭吃什么", "食堂还是外卖", "active", []float32{0, 0, 1})

	results, err := Search(ctx, idx, Request{
		Query: "缓存性能", QueryVec: []float32{1, 0, 0}, Limit: 5,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("无关内容应被过滤,实际: %+v", results)
	}
}

// 纯向量但高相似 → 保留
func TestHighCosineKept(t *testing.T) {
	idx := openTempIndex(t)
	ctx := context.Background()
	addMem(t, idx, "mem_rel", "缓存列表优化", "详述", "active", []float32{1, 0, 0})

	results, err := Search(ctx, idx, Request{
		Query: "cache list", QueryVec: []float32{1, 0, 0}, Limit: 5,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || results[0].ID != "mem_rel" {
		t.Fatalf("高相似应保留: %+v", results)
	}
}

// 低余弦但有实体命中 → 仍保留（双通道冗余设计）
func TestEntityRescuesLowCosine(t *testing.T) {
	idx := openTempIndex(t)
	ctx := context.Background()
	m := &memory.Memory{
		ID: "mem_ent", Title: "旧功能", Body: "内容无关此查询",
		Project: "p", Type: "pattern", Status: "active", Confidence: "human",
		Source: "t", Sensitivity: "normal",
		Entities: []string{"XYZService"},
		Created:  time.Now().UTC(), Path: "x/mem_ent.md",
	}
	if err := idx.Upsert(ctx, m, []float32{0, 0, 1}, "test"); err != nil {
		t.Fatal(err)
	}
	results, err := Search(ctx, idx, Request{
		Query: "XYZService 相关问题", QueryVec: []float32{1, 0, 0}, Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != "mem_ent" {
		t.Fatalf("实体命中应保留: %+v", results)
	}
}

func openTempIndex2(t *testing.T) *index.Index {
	return openTempIndex(t)
}
