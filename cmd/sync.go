package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	pensievesync "github.com/cshbaoo/pensieve/internal/core/sync"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "立即与远程记忆仓库双向同步（pull --rebase 然后 push）",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		s := pensievesync.New(cfg.Core.RepoDir)
		if !s.HasRemote(ctx) {
			fmt.Println("未配置远程仓库。先用: git -C " + cfg.Core.RepoDir + " remote add origin <url>")
			return nil
		}
		fmt.Println("拉取远程...")
		if err := s.Pull(ctx); err != nil {
			return err
		}
		fmt.Println("推送本地...")
		if err := s.Push(ctx); err != nil {
			return err
		}
		fmt.Println("✔ 同步完成")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
