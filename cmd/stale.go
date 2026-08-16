package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

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
	Short: "巡检失活记忆:代码锚点漂移 + 决策复核期到期",
	Long: `巡检记忆库中 active 记忆的两类"过期信号":
  - code 锚点:文件被删,或记忆创建后跨自然日又有改动
  - decision 复核期:写入时设定的 review_at 到期,前提可能已变化
本命令只产出嫌疑列表,不自动改任何状态;确认后:
  update --status stale / stale --mark(锚点浮出)    # 落状态
  update --review-at <期限> (决策复核完仍成立时顺延)`,
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
		overdue := ops.DecisionReviewDue(store, time.Now())
		if len(overdue) > 0 {
			fmt.Printf("⚠ %d 条决策已过复核期(记忆时设定的 review_at 到期):\n\n", len(overdue))
			for _, m := range overdue {
				fmt.Printf("- %s (%s)\n  复核期: %s — 前提可能已变化,请重读决策是否仍然成立\n", m.Title, m.ID, m.ReviewAt.Format("2006-01-02"))
			}
			fmt.Println("\n仍然成立 → pensieve update <id> --review-at <新期限>\n已经过时 → pensieve update <id> --status stale(或写新结论并 supersede)")
		}
		if len(suspects) == 0 {
			if len(overdue) > 0 {
				return nil
			}
			fmt.Println("✔ 无锚点失活嫌疑(或当前目录不在 git 仓库/无活跃 code 锚点记忆),无决策超期")
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
