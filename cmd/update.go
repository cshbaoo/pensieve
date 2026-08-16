package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/core/memory"
	"github.com/cshbaoo/pensieve/internal/core/retrieve"
	"github.com/cshbaoo/pensieve/internal/llm"
)

var (
	updStatus      string
	updSupersedeBy string
	updVote        bool
)

var updateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "更新记忆状态 / 标记被取代 / 投票",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		store := memory.NewStore(cfg.Core.RepoDir)
		m, err := store.GetByID(args[0])
		if err != nil {
			return err
		}
		if updStatus != "" {
			m.Status = updStatus
		}
		if updSupersedeBy != "" {
			m.Status = "superseded"
			m.Links = append(m.Links, memory.Link{ID: updSupersedeBy, Rel: "superseded-by"})
		}
		if updVote {
			m.Votes++
		}
		if err := store.Write(m); err != nil {
			return err
		}

		var vec []float32
		client := llm.New(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Timeout)
		if client.Enabled() {
			if v, err := client.Embed(ctx, cfg.LLM.EmbedModel, retrieve.MakeEmbedText(m.Title, m.Body, m.Tags, m.Entities)); err == nil {
				vec = v
			}
		}
		idx, err := index.Open(cfg.Core.IndexDir)
		if err != nil {
			return err
		}
		defer idx.Close()
		if err := idx.Upsert(ctx, m, vec, cfg.LLM.EmbedModel); err != nil {
			return err
		}
		gitCommit(cfg.Core.RepoDir, "memory-update: "+m.Title)
		fmt.Printf("✔ 已更新: %s (status=%s, votes=%d)\n", m.ID, m.Status, m.Votes)
		return nil
	},
}

func init() {
	updateCmd.Flags().StringVar(&updStatus, "status", "", "active|stale|superseded|archived")
	updateCmd.Flags().StringVar(&updSupersedeBy, "by", "", "取代它的新记忆 id")
	updateCmd.Flags().BoolVar(&updVote, "vote", false, "给这条记忆 +1 票")
	rootCmd.AddCommand(updateCmd)
}
