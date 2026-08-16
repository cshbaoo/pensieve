package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var reindexFull bool

var reindexCmd = &cobra.Command{
	Use:   "reindex",
	Short: "重建索引（默认增量;--full 删库全量重建）",
	RunE: func(cmd *cobra.Command, args []string) error {
		if reindexFull {
			dbPath := filepath.Join(cfg.Core.IndexDir, "index.db")
			if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("删除旧索引失败: %w", err)
			}
		}
		n, err := reindex(reindexFull)
		if err != nil {
			return err
		}
		fmt.Printf("✔ 索引重建完成: %d 条记忆\n", n)
		return nil
	},
}

func init() {
	reindexCmd.Flags().BoolVar(&reindexFull, "full", false, "删掉整个索引库全量重建（平时增量即可）")
	rootCmd.AddCommand(reindexCmd)
}
