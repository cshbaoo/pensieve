package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/core/memory"
	"github.com/cshbaoo/pensieve/internal/core/ops"
	"github.com/cshbaoo/pensieve/internal/core/retrieve"
	pensievesync "github.com/cshbaoo/pensieve/internal/core/sync"
	"github.com/cshbaoo/pensieve/internal/llm"
)

var staleMark bool

var staleCmd = &cobra.Command{
	Use:   "stale",
	Short: "巡检代码锚点失活的记忆(文件被删或在你记忆后有大改动)",
	Long: `巡检记忆库中 active 记忆的 code 锚点:
  - 锚点文件已不存在       → 强烈建议标 stale
  - 锚点文件在记忆创建后又有改动 → 建议复核结论是否仍然成立
本命令只产出嫌疑列表,不自动改任何状态;确认后用 update --status stale 落状态,
或用 --mark 一键批量标记。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		store := memory.NewStore(cfg.Core.RepoDir)
		suspects, err := ops.StaleSuspectsFromCwd(ctx, store, cwd)
		if err != nil {
			return err
		}
		if len(suspects) == 0 {
			fmt.Println("✔ 无锚点失活嫌疑(或当前目录不在 git 仓库/无活跃 code 锚点记忆)")
			return nil
		}
		fmt.Printf("发现 %d 条锚点疑似失活的记忆:\n\n", len(suspects))
		for _, s := range suspects {
			fmt.Printf("- %s (%s)\n  锚点 %s — %s\n", s.Title, s.MemID, s.Anchor, s.Reason)
		}
		if !staleMark {
			fmt.Println("\n复核确认过期后:\n  pensieve stale --mark        # 批量标记 stale\n  pensieve update <id> --status stale   # 逐条标记")
			return nil
		}

		idx, err := index.Open(cfg.Core.IndexDir)
		if err != nil {
			return err
		}
		defer idx.Close()
		client := llm.New(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Timeout)
		for _, s := range suspects {
			m, err := store.GetByID(s.MemID)
			if err != nil {
				return err
			}
			m.Status = "stale"
			if err := store.Write(m); err != nil {
				return err
			}
			// stale 仍以 0.3 权重召回,向量行必须保留——重新 embed 而非传 nil
			var vec []float32
			if client.Enabled() {
				if v, err := client.Embed(ctx, cfg.LLM.EmbedModel, retrieve.MakeEmbedText(m.Title, m.Body, m.Tags, m.Entities)); err == nil {
					vec = v
				}
			}
			if err := idx.Upsert(ctx, m, vec, cfg.LLM.EmbedModel); err != nil {
				return err
			}
			fmt.Printf("✔ 已标记 stale: %s (%s)\n", s.Title, s.MemID)
		}
		gitCommit(cfg.Core.RepoDir, fmt.Sprintf("memory-update: 锚点失活巡检标记 stale ×%d", len(suspects)))
		if cfg.Sync.Mode == "auto" {
			pensievesync.New(cfg.Core.RepoDir).PushAsync()
		}
		return nil
	},
}

func init() {
	staleCmd.Flags().BoolVar(&staleMark, "mark", false, "把列出的嫌疑记忆批量标记为 stale(默认只列出)")
	rootCmd.AddCommand(staleCmd)
}
