package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	pensievesync "github.com/cshbaoo/pensieve/internal/core/sync"
	pensievemcp "github.com/cshbaoo/pensieve/internal/mcp"
)

var serveCmd = &cobra.Command{
	Use:     "serve",
	Aliases: []string{"mcp-serve"},
	Short:   "启动 MCP server（stdio），供 OpenCode / pi / Cursor 等 AI 工具接入",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 启动即拉取一次远程（sync.mode=auto），随后后台定时拉
		if cfg.Sync.Mode == "auto" {
			s := pensievesync.New(cfg.Core.RepoDir)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				if err := s.Pull(ctx); err != nil {
					fmt.Fprintf(os.Stderr, "⚠ 启动拉取失败(继续使用本地记忆): %v\n", err)
					return
				}
				if n, err := reindexAll(); err == nil && n > 0 {
					fmt.Fprintf(os.Stderr, "✔ 远程记忆已同步,索引更新至 %d 条\n", n)
				}
				interval, _ := time.ParseDuration(cfg.Sync.Interval)
				s.AutoLoop(context.Background(), interval)
			}()
		}
		return pensievemcp.New(cfg).Serve()
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
