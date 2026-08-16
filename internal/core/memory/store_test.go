package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sampleMemory() *Memory {
	return &Memory{
		ID:          "mem_20260813_abcd",
		Type:        "gotcha",
		Title:       "测试记忆标题",
		Project:     "myteam/my-api",
		Tags:        []string{"sqlite", "中文标签"},
		Status:      "active",
		Confidence:  "human",
		Entities:    []string{"impl.go"},
		Anchors:     []Anchor{{Kind: "code", Target: "internal/foo/bar.go:123"}},
		Source:      "test",
		Votes:       3,
		Sensitivity: "normal",
		Created:     time.Date(2026, 8, 13, 10, 0, 0, 0, time.Local),
		Body:        "这里是正文。\n多行。\n中文内容。",
	}
}

func TestMarshalParseRoundtrip(t *testing.T) {
	m := sampleMemory()
	data, err := Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.ID != m.ID || got.Type != m.Type || got.Title != m.Title || got.Project != m.Project {
		t.Fatalf("标量字段不等: %+v", got)
	}
	if got.Votes != 3 || got.Status != "active" || got.Sensitivity != "normal" {
		t.Fatalf("数值/状态字段不等: %+v", got)
	}
	if len(got.Tags) != 2 || got.Tags[1] != "中文标签" {
		t.Fatalf("tags: %+v", got.Tags)
	}
	if len(got.Anchors) != 1 || got.Anchors[0].Target != "internal/foo/bar.go:123" {
		t.Fatalf("anchors: %+v", got.Anchors)
	}
	if !strings.Contains(got.Body, "多行。") || !strings.Contains(got.Body, "中文内容。") {
		t.Fatalf("body 丢失: %q", got.Body)
	}
}

func TestParseEmptyBody(t *testing.T) {
	m := sampleMemory()
	m.Body = ""
	data, _ := Marshal(m)
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.ID != m.ID {
		t.Fatalf("id: %v", got.ID)
	}
}

func TestParseInvalid(t *testing.T) {
	if _, err := Parse([]byte("没有 frontmatter 的内容")); err == nil {
		t.Fatal("应报错")
	}
	if _, err := Parse([]byte("---\n未闭合")); err == nil {
		t.Fatal("未闭合应报错")
	}
}

func TestStoreWriteGetWalk(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)

	m1 := sampleMemory()
	m2 := sampleMemory()
	m2.ID = "mem_20260813_ef01"
	m2.Project = "global"
	m2.Title = "另一条"

	if err := s.Write(m1); err != nil {
		t.Fatalf("write m1: %v", err)
	}
	if err := s.Write(m2); err != nil {
		t.Fatalf("write m2: %v", err)
	}

	// 路径布局 {project}/{yyyy}/{mm}/{id}.md
	want1 := filepath.Join("myteam", "my-api", "2026", "08", "mem_20260813_abcd.md")
	if m1.Path != want1 {
		t.Fatalf("路径布局: got %q want %q", m1.Path, want1)
	}
	if _, err := os.Stat(filepath.Join(dir, want1)); err != nil {
		t.Fatalf("文件不存在: %v", err)
	}

	got, err := s.GetByID("mem_20260813_ef01")
	if err != nil {
		t.Fatalf("getByID: %v", err)
	}
	if got.Title != "另一条" {
		t.Fatalf("取错: %q", got.Title)
	}
	if _, err := s.GetByID("mem_00000000_0000"); err == nil {
		t.Fatal("不存在的 id 应报错")
	}

	n := 0
	if err := s.Walk(func(*Memory) error { n++; return nil }); err != nil {
		t.Fatalf("walk: %v", err)
	}
	if n != 2 {
		t.Fatalf("walk 数量: %d", n)
	}
}

// local-only 记忆隔离到 local/ 子树,且 .gitignore fail-closed 自动创建
func TestLocalOnlyIsolation(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	m := &Memory{
		ID: "mem_20260814_l001", Title: "敏感记忆", Project: "myteam/my-api",
		Type: "gotcha", Status: "active", Confidence: "human", Source: "manual",
		Sensitivity: "local-only", Votes: 0,
		Created: time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC),
		Body:    "不可推远程的内容",
	}
	if err := s.Write(m); err != nil {
		t.Fatalf("write: %v", err)
	}
	want := filepath.Join("local", "myteam", "my-api", "2026", "08", "mem_20260814_l001.md")
	if m.Path != want {
		t.Fatalf("local-only 路径应隔离进 local/: got %q want %q", m.Path, want)
	}
	// fail-closed:.gitignore 必须含 /local/
	gi, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("local-only 写入后必须自动建 .gitignore: %v", err)
	}
	if !strings.Contains(string(gi), "/local/") {
		t.Fatalf(".gitignore 必须含 /local/: %q", gi)
	}
	// 幂等:再写一条不应重复追加
	m2 := *m
	m2.ID = "mem_20260814_l002"
	m2.Title = "另一条"
	if err := s.Write(&m2); err != nil {
		t.Fatal(err)
	}
	gi2, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if strings.Count(string(gi2), "/local/") != 1 {
		t.Fatalf("gitignore 幂等失败: %q", gi2)
	}
	// Walk 依然能找到 local-only 记忆(读取不受影响)
	got, err := s.GetByID("mem_20260814_l001")
	if err != nil || got.Title != "敏感记忆" {
		t.Fatalf("local-only 读取: err=%v got=%+v", err, got)
	}
}

// 短项目名归一到唯一 owner/repo 全名,防止命名空间分裂(brainpp → basemind/brainpp)
func TestNormalizeProject(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir)
	m1 := sampleMemory() // myteam/my-api
	m2 := sampleMemory()
	m2.ID = "mem_20260813_ef01"
	m2.Project = "basemind/brainpp"
	for _, m := range []*Memory{m1, m2} {
		if err := s.Write(m); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	cases := map[string]string{
		"brainpp":       "basemind/brainpp",
		"my-api":        "myteam/my-api",
		"global":        "global",
		"local":         "local",
		"basemind/x":    "basemind/x", // 已有完整 owner 前缀,不动
		"newrepo":       "newrepo",    // 无匹配,原样返回(允许新建项目)
		"":              "",
	}
	for in, want := range cases {
		if got := s.NormalizeProject(in); got != want {
			t.Fatalf("NormalizeProject(%q)=%q want %q", in, got, want)
		}
	}
	// 歧义:两个 owner 都有同名 repo 时原样返回
	m3 := sampleMemory()
	m3.ID = "mem_20260813_ef02"
	m3.Project = "otherteam/my-api"
	if err := s.Write(m3); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := s.NormalizeProject("my-api"); got != "my-api" {
		t.Fatalf("歧义场景不应强猜: got %q", got)
	}
}

func TestNewIDFormatAndUnique(t *testing.T) {
	now := time.Now()
	a := NewID("标题", now)
	b := NewID("标题", now.Add(time.Nanosecond))
	if !strings.HasPrefix(a, "mem_") || !strings.Contains(a, now.Format("20060102")) {
		t.Fatalf("id 格式: %q", a)
	}
	if a == b {
		t.Fatal("相同标题不同时间应生成不同 id")
	}
}
