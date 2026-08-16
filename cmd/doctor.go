package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/core/memory"
	"github.com/cshbaoo/pensieve/internal/core/ops"
	pensievesync "github.com/cshbaoo/pensieve/internal/core/sync"
	"github.com/cshbaoo/pensieve/internal/llm"
)

type check struct {
	name string
	run  func(ctx context.Context) (string, error)
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "环境自检：git/仓库/索引/LLM 端点/远程连通性",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		okAll := true

		checks := []check{
			{"git 可用", func(ctx context.Context) (string, error) {
				out, err := exec.CommandContext(ctx, "git", "version").CombinedOutput()
				return string(out), err
			}},
			{"仓库完整", func(ctx context.Context) (string, error) {
				if _, err := os.Stat(filepath.Join(cfg.Core.RepoDir, ".git")); err != nil {
					return "", fmt.Errorf("缺少 .git: %v", err)
				}
				out, err := exec.CommandContext(ctx, "git", "-C", cfg.Core.RepoDir, "fsck", "--no-progress").CombinedOutput()
				return fmt.Sprintf("repo=%s\nfsck: %s", cfg.Core.RepoDir, lastLine(string(out))), err
			}},
			{"索引健康", func(ctx context.Context) (string, error) {
				idx, err := index.Open(cfg.Core.IndexDir)
				if err != nil {
					return "", err
				}
				defer idx.Close()
				n, err := idx.Count(ctx)
				return fmt.Sprintf("%d 条记忆 (FTS5+向量+实体表)", n), err
			}},
			{"LLM chat 端点", func(ctx context.Context) (string, error) {
				c := llm.New(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Timeout)
				if !c.Enabled() {
					return "未配置 api_key(跳过,离线模式可用)", nil
				}
				c2, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
				if _, err := c.Chat(c2, cfg.LLM.ChatModel, "回复一个字: 好", "测试连通"); err != nil {
					return "", err
				}
				return fmt.Sprintf("%s @ %s", cfg.LLM.ChatModel, cfg.LLM.BaseURL), nil
			}},
			{"embedding 端点", func(ctx context.Context) (string, error) {
				c := llm.New(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Timeout)
				if !c.Enabled() {
					return "未配置 api_key(跳过)", nil
				}
				c2, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
				v, err := c.Embed(c2, cfg.LLM.EmbedModel, "doctor 测试")
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("%s (dim=%d)", cfg.LLM.EmbedModel, len(v)), nil
			}},
			{"远程仓库", func(ctx context.Context) (string, error) {
				s := pensievesync.New(cfg.Core.RepoDir)
				if !s.HasRemote(ctx) {
					return "未配置 origin(纯本地模式)", nil
				}
				c2, cancel := context.WithTimeout(ctx, 20*time.Second)
				defer cancel()
				out, err := exec.CommandContext(c2, "git", "-C", cfg.Core.RepoDir, "ls-remote", "origin", "HEAD").CombinedOutput()
				return fmt.Sprintf("origin 可达: %s", lastLine(string(out))), err
			}},
			{"锚点失活巡检", func(ctx context.Context) (string, error) {
				cwd, err := os.Getwd()
				if err != nil {
					return "", err
				}
				suspects, err := ops.StaleSuspectsFromCwd(ctx, memory.NewStore(cfg.Core.RepoDir), cwd)
				if err != nil {
					return "", err
				}
				if len(suspects) == 0 {
					return "无疑似失活锚点", nil
				}
				var sb strings.Builder
				for _, s := range suspects {
					fmt.Fprintf(&sb, "  %s (%s): %s——%s\n", s.Title, s.MemID, s.Anchor, s.Reason)
				}
				return fmt.Sprintf("%d 条嫌疑(见下),复核后 pensieve stale --mark 或 update --status stale", len(suspects)),
					fmt.Errorf("锚点失活嫌疑 %d 条:\n%s", len(suspects), sb.String())
			}},
			{"决策复核到期", func(ctx context.Context) (string, error) {
				overdue := ops.DecisionReviewDue(memory.NewStore(cfg.Core.RepoDir), time.Now())
				if len(overdue) == 0 {
					return "无超期决策", nil
				}
				var sb strings.Builder
				for _, m := range overdue {
					fmt.Fprintf(&sb, "  %s (%s, 复核期 %s)\n", m.Title, m.ID, m.ReviewAt.Format("2006-01-02"))
				}
				// 提示级:决策超期不是健康失败,只提醒复核,doctor 整体仍判绿
				return fmt.Sprintf("⚠ %d 条超期决复核(提示级,非失败):\n%s  update <id> --review-at 顺延 / --status stale 过期", len(overdue), sb.String()), nil
			}},
			{"记忆巡检", func(ctx context.Context) (string, error) {
				// ECC 借鉴的 doctor 思路:坏链 + 重复 ID 扫一遍
				store := memory.NewStore(cfg.Core.RepoDir)
				count := map[string]int{}
				links := map[string][]string{}
				n := 0
				if err := store.Walk(func(m *memory.Memory) error {
					n++
					count[m.ID]++
					for _, lk := range m.Links {
						links[m.ID] = append(links[m.ID], lk.ID)
					}
					return nil
				}); err != nil {
					return "", err
				}
				var broken []string
				var dups []string
				for from, tos := range links {
					for _, to := range tos {
						if count[to] == 0 {
							broken = append(broken, fmt.Sprintf("%s→%s", from, to))
						}
					}
				}
				for id, c := range count {
					if c > 1 {
						dups = append(dups, id)
					}
				}
				summary := fmt.Sprintf("%d 条记忆, %d 条链接", n, len(links))
				if len(broken) == 0 && len(dups) == 0 {
					return summary + ",干净", nil
				}
				var sb strings.Builder
				sb.WriteString(summary + "\n")
				for _, b := range broken {
					fmt.Fprintf(&sb, "  坏链: %s\n", b)
				}
				for _, d := range dups {
					fmt.Fprintf(&sb, "  重复ID: %s\n", d)
				}
				return summary, fmt.Errorf("巡检发现 %d 条坏链 / %d 个重复 ID\n%s", len(broken), len(dups), sb.String())
			}},
		}

		for _, c := range checks {
			result, err := c.run(ctx)
			if err != nil {
				okAll = false
				fmt.Printf("✘ %-14s %v\n", c.name, err)
				if result != "" {
					fmt.Printf("  %s\n", result)
				}
			} else {
				fmt.Printf("✔ %-14s %s\n", c.name, result)
			}
		}
		fmt.Println()
		if okAll {
			fmt.Println("全部正常 🎉  (GOOS:", runtime.GOOS+")")
		} else {
			return fmt.Errorf("存在失败项，请按上面提示修复后重跑")
		}
		return nil
	},
}

func lastLine(s string) string {
	for len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '\n' {
			return s[i+1:]
		}
	}
	return s
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
