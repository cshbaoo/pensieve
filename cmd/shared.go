package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/core/memory"
	"github.com/cshbaoo/pensieve/internal/core/ops"
	"github.com/cshbaoo/pensieve/internal/llm"
)

// gitCommit 提交记忆仓库（失败仅告警，不阻塞主流程；无变更时跳过）
func gitCommit(repoDir, msg string) {
	out, err := exec.Command("git", "-C", repoDir, "status", "--porcelain").CombinedOutput()
	if err != nil || len(out) == 0 {
		return
	}
	if _, err := exec.Command("git", "-C", repoDir, "add", "-A").CombinedOutput(); err != nil {
		fmt.Fprintln(os.Stderr, "⚠ git add 失败:", err)
		return
	}
	if out, err := exec.Command("git", "-C", repoDir, "-c", "user.name=pensieve", "-c", "user.email=pensieve@local",
		"commit", "-q", "-m", msg).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "⚠ git commit: %s\n%s", err, out)
	}
}

// ensureLocalGuardCommitted 若 local-only 守卫刚建则立刻单独提交,
// 防止守卫混进下一次记忆写入的 commit 被 undo 误删
func ensureLocalGuardCommitted(repoDir string) {
	created, err := memory.NewStore(repoDir).EnsureLocalGitignore()
	if err == nil && created {
		gitCommit(repoDir, "chore: 建立 local-only 守卫(.gitignore /local/)")
	}
}

// reindexAll 索引重建统一入口（增量；init/serve 共用）
// 日志必须走 stderr——serve 模式下 stdout 是 JSON-RPC 通道，打印会破坏协议
func reindexAll() (int, error) {
	return reindex(false)
}

func reindex(full bool) (int, error) {
	ctx := context.Background()
	idx, err := index.Open(cfg.Core.IndexDir)
	if err != nil {
		return 0, err
	}
	defer idx.Close()
	client := llm.New(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Timeout)
	res, err := ops.Rebuild(ctx, memory.NewStore(cfg.Core.RepoDir), idx, client, cfg.LLM.EmbedModel, full)
	if err != nil {
		return 0, err
	}
	if res.Removed > 0 {
		fmt.Fprintf(os.Stderr, "清理已删除文件残留索引 %d 条\n", res.Removed)
	}
	return res.IndexedTotal, nil
}
