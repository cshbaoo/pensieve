package ops

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/cshbaoo/pensieve/internal/core/memory"
)

func TestDecisionReviewDue(t *testing.T) {
	dir := t.TempDir()
	store := memory.NewStore(dir)
	now := time.Now()

	mk := func(id string, typ string, status string, reviewAgo time.Duration) *memory.Memory {
		m := &memory.Memory{
			ID: id, Type: typ, Title: id, Project: "global",
			Status: status, Confidence: "human", Source: "test", Sensitivity: "normal",
			Created: now.Add(-72 * time.Hour), Body: "b",
		}
		if reviewAgo != 0 {
			m.ReviewAt = now.Add(-reviewAgo)
		}
		if err := store.Write(m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	mk("mem_overdue", "decision", "active", time.Hour)   // 已超期 → 命中
	mk("mem_fresh", "decision", "active", -time.Hour)    // 未到期
	mk("mem_gotcha", "gotcha", "active", 0)              // 非 decision 且无显式复核期
	mk("mem_super", "decision", "superseded", time.Hour) // 已被取代不再复核

	got := DecisionReviewDue(store, now)
	if len(got) != 1 || got[0].ID != "mem_overdue" {
		ids := []string{}
		for _, m := range got {
			ids = append(ids, m.ID)
		}
		t.Fatalf("应只命中 mem_overdue,实际: %v", ids)
	}
}

// StaleSuspects 的 git 无关路径:非 git 工作区应静默返回空(不挂现场)
func TestStaleSuspectsNonGitWorkdir(t *testing.T) {
	dir := t.TempDir()
	store := memory.NewStore(dir)
	m := &memory.Memory{
		ID: "mem_x", Type: "gotcha", Title: "t", Project: "global",
		Status: "active", Confidence: "human", Source: "test", Sensitivity: "normal",
		Created: time.Now(), Body: "b",
		Anchors: []memory.Anchor{{Kind: "code", Target: "internal/foo/bar.go"}},
	}
	if err := store.Write(m); err != nil {
		t.Fatal(err)
	}
	// 一个非 git 目录
	notGit := t.TempDir()
	sus, err := StaleSuspectsFromCwd(t.Context(), store, notGit)
	if err != nil || len(sus) != 0 {
		t.Fatalf("非 git 目录应静默跳过: sus=%v err=%v", sus, err)
	}
}

// StaleSuspects 项目过滤:只在当前 git 仓库的项目才检查锚点
func TestStaleSuspectsProjectFilter(t *testing.T) {
	dir := t.TempDir()
	store := memory.NewStore(dir)
	now := time.Now()
	mk := func(id, proj string) {
		m := &memory.Memory{
			ID: id, Type: "gotcha", Title: id, Project: proj,
			Status: "active", Confidence: "human", Source: "test", Sensitivity: "normal",
			Created: now, Body: "b",
			Anchors: []memory.Anchor{{Kind: "code", Target: "不存在/这个文件.go"}},
		}
		if err := store.Write(m); err != nil {
			t.Fatal(err)
		}
	}
	mk("mem_thisrepo", "acme/demo")
	mk("mem_otherrepo", "other/thing")

	// 造一个假 git 仓库,远端 origin = acme/demo
	repo := t.TempDir()
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s %v", args, out, err)
		}
	}
	runGit("init", "-q")
	runGit("remote", "add", "origin", "https://github.com/acme/demo.git")
	if err := os.WriteFile(filepath.Join(repo, "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", ".")

	sus, err := StaleSuspects(t.Context(), store, repo, "acme/demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(sus) != 1 || sus[0].MemID != "mem_thisrepo" {
		t.Fatalf("应只检查当前项目的记忆: %+v", sus)
	}
	if sus[0].Reason == "" {
		t.Error("应附原因")
	}
}

func TestLooksLikePath(t *testing.T) {
	yes := []string{"internal/foo/bar.go", "README.md", `cmd\stale.go`, "a/b/c"}
	no := []string{"EnsureLocalGitignore", "DoSomething", "", "golang"}
	for _, p := range yes {
		if !looksLikePath(p) {
			t.Errorf("%q 应判为路径", p)
		}
	}
	for _, p := range no {
		if looksLikePath(p) {
			t.Errorf("%q 不应判为路径", p)
		}
	}
}
