package ops

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cshbaoo/pensieve/internal/core/memory"
)

// StaleSuspect 锚点失活嫌疑:记忆的代码锚点相对代码仓库已漂移
type StaleSuspect struct {
	MemID  string
	Title  string
	Anchor string // 出问题的锚点目标
	Reason string // 人类可读原因
}

const staleScanMaxAnchors = 200 // 扫描预算上限(ECC 借鉴):防巨型库把 context/doctor 拖慢

// DecisionReviewDue 返回已过复核期(review_at < now)且仍 active 的决策类记忆。
// 决策的前提会变,超期≠失效——只浮出人工复核,red line 一致:不落状态由人拍板。
func DecisionReviewDue(store *memory.Store, now time.Time) []*memory.Memory {
	var out []*memory.Memory
	_ = store.Walk(func(m *memory.Memory) error {
		if m.OverdueForReview(now) {
			out = append(out, m)
		}
		return nil
	})
	return out
}

// stripAnchorLine 去掉锚点里的行号后缀(path/to.go:525 → path/to.go)
func stripAnchorLine(target string) string {
	if i := strings.LastIndex(target, ":"); i > 0 {
		if _, err := strconv.Atoi(target[i+1:]); err == nil {
			return target[:i]
		}
	}
	return target
}

// codeRootOf 从工作目录推导代码仓库根(非 git 仓库返回空,静默跳过巡检)
func codeRootOf(cwd string) string {
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// looksLikePath 锚点像不像文件路径(符号名/实体名无法落盘校验,跳过防误报)
func looksLikePath(p string) bool {
	if strings.ContainsAny(p, `/\`) {
		return true
	}
	return strings.Contains(p, ".") // 有扩展名,如 README.md
}

// StaleSuspects 巡检:把活跃记忆的 code 锚点与代码仓库比对(文件不存在 + 记忆创建后有改动)。
// 只产出嫌疑列表,永不自动改状态——落状态永远由人确认,守住质量闸门红线。
//
// 防误报两道闸:
//  1. project 非空时只巡检该项目挂的记忆(跨项目锚点在当前仓库必然"不存在");
//  2. 只校验"长得像路径"的锚点(符号名/实体名跳过)。
//
// 成本可控:全库最多 staleScanMaxAnchors 个锚点,git log 只调用一次。
func StaleSuspects(ctx context.Context, store *memory.Store, codeRoot, project string) ([]StaleSuspect, error) {
	if codeRoot == "" {
		return nil, nil // 非 git 工作区,无法巡检,静默跳过
	}

	// 收集活跃记忆的 code 锚点
	type ref struct {
		m      *memory.Memory
		anchor string
	}
	var refs []ref
	var mems []*memory.Memory
	seenAnchor := map[string]bool{}
	var anchorPaths []string
	err := store.Walk(func(m *memory.Memory) error {
		if m.Status != "active" {
			return nil
		}
		if project != "" && m.Project != project {
			return nil
		}
		hasCodeAnchor := false
		for _, a := range m.Anchors {
			if a.Kind != "code" {
				continue
			}
			p := stripAnchorLine(a.Target)
			if p == "" || !looksLikePath(p) {
				continue
			}
			if !seenAnchor[p] {
				seenAnchor[p] = true
				anchorPaths = append(anchorPaths, p)
			}
			refs = append(refs, ref{m, p})
			hasCodeAnchor = true
		}
		if hasCodeAnchor {
			mems = append(mems, m)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(mems) == 0 {
		return nil, nil
	}
	if len(anchorPaths) > staleScanMaxAnchors {
		anchorPaths = anchorPaths[:staleScanMaxAnchors]
	}

	// 一次 git 调用拿到指定路径自最早记忆创建时间以来的最后修改时间
	var minCreated time.Time
	for _, m := range mems {
		if minCreated.IsZero() || m.Created.Before(minCreated) {
			minCreated = m.Created
		}
	}
	changedAt := lastCommitTimes(ctx, codeRoot, anchorPaths, minCreated)

	// 判定:文件不存在 → 锚点失活;文件最后提交晚于记忆创建 → 疑似漂移。
	// 「有改动」设同日宽限:与记忆创建同一天的改动常见(写完记忆顺手再改代码),不算漂移信号;
	// 跨自然日的改动才是"结论建立在外部世界已变的前提上"。
	var out []StaleSuspect
	missing := map[string]bool{}
	changedReason := map[string]string{}
	for _, p := range anchorPaths {
		if _, err := os.Stat(filepath.Join(codeRoot, filepath.FromSlash(p))); err != nil {
			missing[p] = true
		} else if ts, ok := changedAt[p]; ok {
			changedReason[p] = time.Unix(ts, 0).Format("2006-01-02")
		}
	}
	reported := map[string]bool{} // 每条记忆只报一次(取最严重原因:不存在 > 漂移)
	for _, r := range refs {
		if reported[r.m.ID] {
			continue
		}
		if missing[r.anchor] {
			out = append(out, StaleSuspect{MemID: r.m.ID, Title: r.m.Title, Anchor: r.anchor, Reason: "锚点文件已不存在"})
			reported[r.m.ID] = true
		} else if day, ok := changedReason[r.anchor]; ok && day > r.m.Created.Format("2006-01-02") {
			out = append(out, StaleSuspect{MemID: r.m.ID, Title: r.m.Title, Anchor: r.anchor, Reason: fmt.Sprintf("锚点文件在 %s 又有改动", day)})
			reported[r.m.ID] = true
		}
	}
	return out, nil
}

// StaleSuspectsFromCwd 便捷入口:从工作目录推导代码仓库根与项目名再巡检。
// 项目名取自 git remote(owner/repo);无法识别时 project 传空退化为仅路径形态过滤。
func StaleSuspectsFromCwd(ctx context.Context, store *memory.Store, cwd string) ([]StaleSuspect, error) {
	root := codeRootOf(cwd)
	if root == "" {
		return nil, nil
	}
	return StaleSuspects(ctx, store, root, projectFromRemote(root))
}

// projectFromRemote 从仓库 origin 推导 owner/repo(与 export.ProjectFromGitRemote 同算法,
// 不引 export 包防循环:export 依赖 memory,保持 ops 自闭)
func projectFromRemote(root string) string {
	out, err := exec.Command("git", "-C", root, "remote", "get-url", "origin").CombinedOutput()
	if err != nil {
		return ""
	}
	u := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(string(out)), ".git"), "/")
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

// lastCommitTimes 用一条 git log 拿指定路径集合里每个路径的最后提交时间(unix 秒)。
// 输出格式:@<unix> 后跟若干路径行;--name-only 下 merge commit 路径行为受 -m 影响,这里用 --no-merges 简化语义。
func lastCommitTimes(ctx context.Context, repoRoot string, paths []string, since time.Time) map[string]int64 {
	if len(paths) == 0 {
		return nil
	}
	sinceArg := since.Format("2006-01-02T15:04:05")
	args := []string{"-C", repoRoot, "log", "--no-merges", "--format=@%ct", "--name-only",
		"--since=" + sinceArg, "--"}
	args = append(args, paths...)
	c2, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(c2, "git", args...).CombinedOutput()
	if err != nil {
		return nil // 巡检失败静默降级,不影响主流程
	}
	res := map[string]int64{}
	var cur int64
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "@") {
			cur, _ = strconv.ParseInt(line[1:], 10, 64)
			continue
		}
		if _, ok := res[line]; !ok && cur > 0 {
			res[line] = cur // git log 按时间倒序,首次出现即最新
		}
	}
	return res
}
