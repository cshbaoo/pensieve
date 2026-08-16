package ops

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/core/memory"
	"github.com/cshbaoo/pensieve/internal/llm"
)

func writeMem(t *testing.T, dir string, m *memory.Memory) {
	t.Helper()
	s := memory.NewStore(dir)
	if err := s.Write(m); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func memOf(id, title string) *memory.Memory {
	return &memory.Memory{
		ID: id, Title: title, Body: "body of " + title, Project: "p",
		Type: "pattern", Status: "active", Confidence: "human", Source: "t",
		Sensitivity: "normal", Created: time.Now().UTC(),
	}
}

func TestRebuildIncremental(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	idx, err := index.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	client := &llm.Client{} // 无 key → 纯本地,跳过 embedding

	// 第一次:两条全新
	writeMem(t, dir, memOf("mem_a", "甲"))
	writeMem(t, dir, memOf("mem_b", "乙"))
	res, err := Rebuild(ctx, memory.NewStore(dir), idx, client, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if res.IndexedTotal != 2 || res.Upserted != 2 {
		t.Fatalf("首次: %+v", res)
	}

	// 第二次:未变动 → 0 upsert
	res, err = Rebuild(ctx, memory.NewStore(dir), idx, client, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Upserted != 0 {
		t.Fatalf("未变动不应重复写入: %+v", res)
	}

	// 改一个文件内容(mtime 变) → 1 upsert
	time.Sleep(1100 * time.Millisecond) // 保证 mtime 不同
	m := memOf("mem_a", "甲-修改后")
	writeMem(t, dir, m)
	res, err = Rebuild(ctx, memory.NewStore(dir), idx, client, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Upserted != 1 {
		t.Fatalf("仅一条变动应只 upsert 1: %+v", res)
	}

	// 删除一个文件 → removed=1,total=1
	if err := os.Remove(filepath.Join(dir, m.Path)); err != nil {
		t.Fatal(err)
	}
	res, err = Rebuild(ctx, memory.NewStore(dir), idx, client, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 1 || res.IndexedTotal != 1 {
		t.Fatalf("删除感知: %+v", res)
	}
}

func TestRebuildForceFull(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	idx, _ := index.Open(t.TempDir())
	defer idx.Close()
	client := &llm.Client{}

	writeMem(t, dir, memOf("mem_a", "甲"))
	Rebuild(ctx, memory.NewStore(dir), idx, client, "", false)
	res, err := Rebuild(ctx, memory.NewStore(dir), idx, client, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Upserted != 1 {
		t.Fatalf("全量强制: %+v", res)
	}
}
