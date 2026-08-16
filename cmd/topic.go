package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/core/memory"
	"github.com/cshbaoo/pensieve/internal/core/topic"
	"github.com/cshbaoo/pensieve/internal/llm"
)

var (
	topicProject string
	topicYes     bool
)

var topicCmd = &cobra.Command{
	Use:   "topic <主题>",
	Short: "把某主题相关的记忆聚成一张卷宗卡片（topic 类型记忆）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		theme := args[0]
		client := llm.New(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Timeout)
		if !client.Enabled() {
			return fmt.Errorf("topic 需要配置 llm.api_key")
		}

		idx, err := index.Open(cfg.Core.IndexDir)
		if err != nil {
			return err
		}
		defer idx.Close()

		// 候选:该项目（或全部）的活跃记忆
		cands, err := loadCandidates(ctx, idx, topicProject)
		if err != nil {
			return err
		}
		if len(cands) == 0 {
			fmt.Println("没有候选记忆。")
			return nil
		}

		draft, skipped, err := topic.Generate(ctx, client, cfg.LLM.ChatModel, theme, cands)
		if err != nil {
			return err
		}
		if skipped {
			fmt.Printf("没有找到与主题 %q 相关的记忆,未生成卷宗。\n", theme)
			return nil
		}

		// 已存在同主题 topic → 提示
		if topics, _ := idx.Topics(ctx, topicProject); len(topics) > 0 {
			for _, t := range topics {
				fmt.Printf("已存在卷宗: %s (%s)\n", t.Title, t.ID)
			}
		}

		m := &memory.Memory{
			ID:      memory.NewID(draft.Title, time.Now()),
			Type:    "topic",
			Title:   draft.Title,
			Project: topicProject,
			Tags:    draft.Keywords,
			Status:  "active",
			Confidence: "ai-inferred",
			Source:     "cli-topic",
			Sensitivity: "normal",
			Created:  time.Now(),
			Body:     draft.Summary,
		}

		// LLM 选出的子记忆 id → 过滤幻觉 id + 打上 rel=contains
		validIDs := keepExisting(ctx, idx, draft.Links)
		m.Links = make(memory.Links, 0, len(validIDs))
		for _, id := range validIDs {
			m.Links = append(m.Links, memory.Link{ID: id, Rel: "contains"})
		}

		if !topicYes {
			fmt.Println("\n── 卷宗草稿 ──")
			fmt.Printf("标题: %s\n链接 %d 条子记忆:\n", m.Title, len(m.Links))
			for _, lk := range m.Links {
				if row, _ := idx.GetRow(ctx, lk.ID); row != nil {
					fmt.Printf("  - %s (%s)\n", row.Title, lk.ID)
				}
			}
			fmt.Printf("说明: %s\n", m.Body)
			fmt.Print("\n创建? [y/N] ")
			answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			if !strings.EqualFold(strings.TrimSpace(answer), "y") {
				fmt.Println("已取消")
				return nil
			}
		}

		if err := memory.NewStore(cfg.Core.RepoDir).Write(m); err != nil {
			return err
		}
		if err := idx.Upsert(ctx, m, nil, ""); err != nil {
			return err
		}
		gitCommit(cfg.Core.RepoDir, "memory: [topic] "+m.Title)

		fmt.Printf("✔ 卷宗已创建: %s (%d 条子记忆)\n", m.ID, len(m.Links))
		return nil
	},
}

func loadCandidates(ctx context.Context, idx *index.Index, project string) ([]index.Row, error) {
	return idx.ListActive(ctx, project)
}

// keepExisting 过滤掉 LLM 幻觉编造的不存在 id
func keepExisting(ctx context.Context, idx *index.Index, ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if row, _ := idx.GetRow(ctx, id); row != nil {
			out = append(out, id)
		}
	}
	return out
}

func init() {
	topicCmd.Flags().StringVarP(&topicProject, "project", "p", "", "只在该项目范围内选子记忆(缺省全库)")
	topicCmd.Flags().BoolVarP(&topicYes, "yes", "y", false, "跳过确认")
	rootCmd.AddCommand(topicCmd)
}
