package ops

import (
	"context"
	"testing"
	"time"

	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/core/memory"
)

// MarkSuperseded:旧记忆被置 superseded + 追加 superseded-by 链接,事实源与索引同步
func TestMarkSuperseded(t *testing.T) {
	dir := t.TempDir()
	store := memory.NewStore(dir)
	ctx := context.Background()

	old := &memory.Memory{
		ID: "mem_old", Type: "decision", Title: "旧结论", Project: "global",
		Status: "active", Confidence: "human", Source: "test",
		Sensitivity: "normal", Created: time.Now(),
		Body: "v1 方案",
	}
	if err := store.Write(old); err != nil {
		t.Fatalf("write: %v", err)
	}
	idx, err := index.Open(t.TempDir())
	if err != nil {
		t.Fatalf("index open: %v", err)
	}
	defer idx.Close()
	if err := idx.Upsert(ctx, old, nil, "test"); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// 无 LLM(离线路径):client=nil 也应完成
	if err := MarkSuperseded(ctx, store, idx, nil, "test", "mem_old", "mem_new"); err != nil {
		t.Fatalf("mark: %v", err)
	}
	got, err := store.GetByID("mem_old")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != "superseded" {
		t.Fatalf("status: %s", got.Status)
	}
	if memory.SuccessorID(got) != "mem_new" {
		t.Fatalf("link: %+v", got.Links)
	}
	// 幂等:重复标记不重复加链
	if err := MarkSuperseded(ctx, store, idx, nil, "test", "mem_old", "mem_new"); err != nil {
		t.Fatal(err)
	}
	got, _ = store.GetByID("mem_old")
	n := 0
	for _, lk := range got.Links {
		if lk.Rel == "superseded-by" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("重复标记导致链接重复: %+v", got.Links)
	}
	// 索引同步:状态已进入索引
	row, _ := idx.GetRow(ctx, "mem_old")
	if row == nil || row.Status != "superseded" {
		t.Fatalf("索引状态未同步: %+v", row)
	}

	// 自我取代应被拒绝
	if err := MarkSuperseded(ctx, store, idx, nil, "test", "mem_old", "mem_old"); err == nil {
		t.Fatal("自我取代应报错")
	}
	// 旧记忆不存在
	if err := MarkSuperseded(ctx, store, idx, nil, "test", "mem_gone", "mem_new"); err == nil {
		t.Fatal("不存在的旧记忆应报错")
	}
}

func TestStripAnchorLine(t *testing.T) {
	cases := map[string]string{
		"services/x/impl.go:525": "services/x/impl.go",
		"internal/core/foo.go":   "internal/core/foo.go",
		"a/b.go:12:34":           "a/b.go:12", // 只去最后一截行号
		"README.md":              "README.md",
		"noext:abc":              "noext:abc", // 非数字后缀不去
	}
	for in, want := range cases {
		if got := stripAnchorLine(in); got != want {
			t.Errorf("stripAnchorLine(%q) = %q, want %q", in, got, want)
		}
	}
}
