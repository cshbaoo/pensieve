package export

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cshbaoo/pensieve/internal/core/memory"
)

func TestRefreshManaged(t *testing.T) {
	// 初始化一个临时 git 仓库作为"工作区"
	dir := t.TempDir()
	for _, args := range [][]string{{"init"}, {"config", "user.name", "t"}, {"config", "user.email", "t@t"}, {"remote", "add", "origin", "https://github.com/a/b.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	// 工作区 AGENTS.md 带托管区
	sec := RenderSection(nil)
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(Upsert("# 手写\n", sec)), 0o644); err != nil {
		t.Fatal(err)
	}
	// 记忆仓库放一条高票 gotcha
	mrepo := t.TempDir()
	store := memory.NewStore(mrepo)
	g := mem("m1", "gotcha", "a/b", "active", 5, time.Now())
	g.ID = "mem_20260814_a1b2" // store.Walk 只认 mem_ 前缀文件名
	g.Body = "正文"
	if err := store.Write(g); err != nil {
		t.Fatal(err)
	}

	changed, err := RefreshManaged(dir, store, DefaultOptions(), time.Now())
	if err != nil || !changed {
		t.Fatalf("应发生写入: changed=%v err=%v", changed, err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(string(data), "标题-m1") || !strings.Contains(string(data), "# 手写") {
		t.Fatalf("应含 gotcha 与手写内容:\n%s", data)
	}
	// 第二次运行记忆无变化 → 不写入
	changed, err = RefreshManaged(dir, store, DefaultOptions(), time.Now())
	if err != nil || changed {
		t.Fatalf("应幂等不写入: changed=%v err=%v", changed, err)
	}
	// AGENTS.md 无托管区 → 不接管
	dir2 := t.TempDir()
	os.WriteFile(filepath.Join(dir2, "AGENTS.md"), []byte("# 纯手写\n"), 0o644)
	changed, err = RefreshManaged(dir2, store, DefaultOptions(), time.Now())
	if changed || err == nil {
		t.Fatalf("非 git 仓库应报 git 错误, got changed=%v err=%v", changed, err)
	}
}

func mem(id, typ, project, status string, votes int, created time.Time) *memory.Memory {
	return &memory.Memory{
		ID: id, Type: typ, Title: "标题-" + id, Project: project,
		Status: status, Votes: votes, Created: created,
	}
}

func TestCollectFiltersAndSorts(t *testing.T) {
	now := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	mems := []*memory.Memory{
		mem("m1", "gotcha", "a/b", "active", 5, now.AddDate(0, -2, 0)),  // 高票,入选
		mem("m2", "gotcha", "a/b", "active", 0, now.AddDate(0, 0, -1)),  // 新近,入选
		mem("m3", "gotcha", "a/b", "active", 1, now.AddDate(0, -6, 0)),  // 低票且旧,淘汰
		mem("m4", "pattern", "a/b", "active", 9, now.AddDate(0, 0, -3)), // 非 gotcha,淘汰
		mem("m5", "gotcha", "a/b", "superseded", 9, now.AddDate(0, 0, -3)), // 非 active,淘汰
		mem("m6", "gotcha", "c/d", "active", 8, now.AddDate(0, -1, 0)),  // 别的项目,淘汰
		mem("m7", "gotcha", "a/b", "active", 9, now.AddDate(-1, 0, 0)),  // 最高票,排第一
	}
	got := Collect(mems, Options{Project: "a/b", MinVotes: 2, SinceDays: 30, MaxItems: 10}, now)
	ids := []string{}
	for _, m := range got {
		ids = append(ids, m.ID)
	}
	want := []string{"m7", "m1", "m2"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", ids, want)
	}
}

func TestCollectWithoutProjectAndMaxTruncation(t *testing.T) {
	now := time.Now()
	var mems []*memory.Memory
	for i := 0; i < 15; i++ {
		mems = append(mems, mem(string(rune('a'+i)), "gotcha", "x/y", "active", i, now))
	}
	got := Collect(mems, Options{MinVotes: 2, SinceDays: 30, MaxItems: 3}, now)
	if len(got) != 3 || got[0].Votes != 14 || got[1].Votes != 13 || got[2].Votes != 12 {
		t.Fatalf("截断或排序错误: %+v", got)
	}
}

func TestRenderSectionIsSingleLineAndDeterministic(t *testing.T) {
	m := mem("m1", "gotcha", "a/b", "active", 1, time.Now())
	m.Title = "第一行\n第二行"
	sec1 := RenderSection([]*memory.Memory{m})
	if !strings.Contains(sec1, "- 第一行\n") || strings.Contains(sec1, "第二行") {
		t.Fatalf("标题应为单行: %q", sec1)
	}
	sec2 := RenderSection([]*memory.Memory{m})
	if sec1 != sec2 {
		t.Fatalf("同输入应输出一致:\n%s\n---\n%s", sec1, sec2)
	}
	if !strings.HasPrefix(sec1, Begin) || !strings.HasSuffix(sec1, End) {
		t.Fatalf("必须被 begin/end 包裹")
	}
}

func TestRenderSectionEmpty(t *testing.T) {
	sec := RenderSection(nil)
	if !strings.Contains(sec, "暂无高频坑记录") {
		t.Fatalf("空列表应有占位文案")
	}
}

func TestUpsertAppendAndReplace(t *testing.T) {
	sec := RenderSection(nil)
	// 无标记:追加
	full := Upsert("# 手写内容\n", sec)
	if !strings.Contains(full, "# 手写内容") || !strings.Contains(full, Begin) {
		t.Fatalf("追加失败")
	}
	// 幂等:再跑一次完全不变
	full2 := Upsert(full, sec)
	if full != full2 {
		t.Fatalf("应为幂等:\n%s\n---\n%s", full, full2)
	}
	// 有标记:只换中间,前后手写区保留
	sec2 := RenderSection([]*memory.Memory{mem("m9", "gotcha", "a/b", "active", 3, time.Now())})
	full3 := Upsert("# 手写头\n\n"+full+"\n# 手写尾\n", sec2)
	if !strings.Contains(full3, "# 手写头") || !strings.Contains(full3, "# 手写尾") || !strings.Contains(full3, "标题-m9") {
		t.Fatalf("标记区替换失败:\n%s", full3)
	}
}

func TestProjectFromGitRemote(t *testing.T) {
	// 当前仓库设置过 origin 时应能解析出 owner/repo 形态;无 origin 时返回空不报错
	p := ProjectFromGitRemote(t.TempDir())
	if p != "" {
		t.Fatalf("非 git 目录应返回空,得到 %q", p)
	}
}
