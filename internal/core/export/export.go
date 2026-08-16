// Package export 把记忆库蒸馏为 AGENTS.md 托管区:
// 内容只放"纪律指令 + 高频坑一句话清单"(路标),详情永远留在 Pensieve 记忆库里。
package export

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cshbaoo/pensieve/internal/core/memory"
)

// 托管区标记:rendered 内容被这两行包裹,pensieve export 只重写标记之间的部分
const (
	Begin = "<!-- pensieve:begin -->"
	End   = "<!-- pensieve:end -->"
)

// Options gotcha 清单的入选与截断规则
type Options struct {
	Project   string // 限定项目;空 = 不限项目
	MinVotes  int    // 入选条件一:投票数 >= MinVotes
	SinceDays int    // 入选条件二:近 SinceDays 天新建
	MaxItems  int    // 最多条数(0 = 不限);按 votes 降序、Created 降序排序后截断
}

// Collect 从全部记忆中筛出"高频坑"清单
func Collect(mems []*memory.Memory, opts Options, now time.Time) []*memory.Memory {
	cutoff := now.AddDate(0, 0, -opts.SinceDays)
	var out []*memory.Memory
	for _, m := range mems {
		if m.Type != "gotcha" || m.Status != "active" {
			continue
		}
		if opts.Project != "" && m.Project != opts.Project {
			continue
		}
		if m.Votes >= opts.MinVotes || m.Created.After(cutoff) {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Votes != out[j].Votes {
			return out[i].Votes > out[j].Votes
		}
		return out[i].Created.After(out[j].Created)
	})
	if opts.MaxItems > 0 && len(out) > opts.MaxItems {
		out = out[:opts.MaxItems]
	}
	return out
}

// RenderSection 渲染托管区内容(含 begin/end 标记,确定性输出:同一天同一记忆库字节一致)
func RenderSection(mems []*memory.Memory) string {
	var sb strings.Builder
	sb.WriteString(Begin + "\n")
	sb.WriteString("<!-- 本节由 pensieve export 自动生成,请勿手改;更新方式:pensieve export(Claude Code 等用 pensieve export --out CLAUDE.md) -->\n")
	sb.WriteString("## 工程记忆(Pensieve)\n\n")
	sb.WriteString("本项目使用 Pensieve 管理工程记忆(踩坑/决策/接口细节)。工作纪律:\n")
	sb.WriteString("- 开始排查问题前,先检索记忆:memory_search \"<问题关键词>\"\n")
	sb.WriteString("- 解决问题/做完决策后,沉淀为记忆:memory_save(先出草稿给用户确认)\n\n")
	sb.WriteString("## 高频坑(gotcha)\n\n")
	if len(mems) == 0 {
		sb.WriteString("暂无高频坑记录;随记忆沉淀自动更新。\n")
	} else {
		for _, m := range mems {
			sb.WriteString("- " + oneLine(m.Title) + "\n")
		}
	}
	sb.WriteString(End)
	return sb.String()
}

// Upsert 把托管区写入既有文本:有标记则原位替换,无标记则追加到文末
func Upsert(existing, section string) string {
	bi := strings.Index(existing, Begin)
	ei := strings.Index(existing, End)
	if bi >= 0 && ei > bi {
		return existing[:bi] + section + existing[ei+len(End):]
	}
	existing = strings.TrimRight(existing, "\n")
	if existing == "" {
		return section + "\n"
	}
	return existing + "\n\n" + section + "\n"
}

// ProjectFromGitRemote 从 git remote 推导项目名 owner/repo(与 MCP serve 的检测逻辑一致)
func ProjectFromGitRemote(dir string) string {
	out, err := exec.Command("git", "-C", dir, "remote", "get-url", "origin").CombinedOutput()
	if err != nil {
		return ""
	}
	u := strings.TrimSpace(string(out))
	u = strings.TrimSuffix(strings.TrimSuffix(u, ".git"), "/")
	if i := strings.LastIndex(u, "/"); i >= 0 {
		repo := u[i+1:]
		u2 := u[:i]
		sep := "/"
		if strings.Contains(u2, ":") && !strings.Contains(u2, "//") {
			sep = ":"
		}
		if j := strings.LastIndex(u2, sep); j >= 0 {
			return u2[j+1:] + "/" + repo
		}
		return repo
	}
	return ""
}

// DefaultOptions 与 CLI flag 默认值保持一致
func DefaultOptions() Options {
	return Options{MinVotes: 2, SinceDays: 30, MaxItems: 10}
}

// RefreshManaged 若 dir 所在 git 仓库根的 AGENTS.md 含托管区标记,则就地重导出。
// memory_save/update 成功后调用,让托管区永远跟记忆库同步。返回是否发生写入。
func RefreshManaged(dir string, store *memory.Store, opts Options, now time.Time) (bool, error) {
	root, err := gitRoot(dir)
	if err != nil {
		return false, err
	}
	path := filepath.Join(root, "AGENTS.md")
	existing, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // 没有 AGENTS.md:不替用户创建文件
		}
		return false, err
	}
	if !strings.Contains(string(existing), Begin) {
		return false, nil // 有 AGENTS.md 但无托管区:不接管
	}
	project := ProjectFromGitRemote(root)
	var mems []*memory.Memory
	if err := store.Walk(func(m *memory.Memory) error {
		mems = append(mems, m)
		return nil
	}); err != nil {
		return false, err
	}
	opts.Project = project
	section := RenderSection(Collect(mems, opts, now))
	desired := Upsert(string(existing), section)
	if desired == string(existing) {
		return false, nil
	}
	if err := os.WriteFile(path, []byte(desired), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// gitRoot 返回 dir 所在仓库的根目录
func gitRoot(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("不在 git 仓库中: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSuffix(strings.TrimSpace(string(out)), "/"), nil
}

// oneLine 取标题首行并压成单行(gotcha 清单每条一行)
func oneLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

