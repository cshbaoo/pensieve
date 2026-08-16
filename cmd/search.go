package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/core/retrieve"
	"github.com/cshbaoo/pensieve/internal/core/stats"
	"github.com/cshbaoo/pensieve/internal/llm"
)

var (
	searchProject           string
	searchType              string
	searchLimit             int
	searchIncludeSuperseded bool
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "混合检索记忆（FTS 全文 + 向量语义 + 实体）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		idx, err := index.Open(cfg.Core.IndexDir)
		if err != nil {
			return err
		}
		defer idx.Close()

		// 有 API key 则查询也向量化，启用语义召回
		var qvec []float32
		client := llm.New(cfg.LLM.BaseURL, cfg.LLM.APIKey, cfg.LLM.Timeout)
		if client.Enabled() {
			if v, err := client.Embed(ctx, cfg.LLM.EmbedModel, args[0]); err == nil {
				qvec = v
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "⚠ 查询向量化失败（退化全文检索）: %v\n", err)
			}
		}

		r := retrieve.Request{
			Query: args[0], Project: searchProject, Type: searchType,
			Limit: searchLimit, QueryVec: qvec,
			IncludeSuperseded: searchIncludeSuperseded,
		}
		if cfg.LLM.RerankEnabled && cfg.LLM.RerankModel != "" {
			r.Reranker = client
			r.RerankModel = cfg.LLM.RerankModel
		}
		results, err := retrieve.Search(ctx, idx, r)
		if err != nil {
			return err
		}
		stats.Track(cfg.Core.IndexDir, "search", "cli", searchProject, map[string]any{"hits": len(results)})

		if flagJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(results)
		}

		if len(results) == 0 {
			fmt.Println("（无匹配记忆）")
			return nil
		}
		for _, r := range results {
			marker := ""
			if r.Status != "active" {
				marker = " [" + r.Status + "]"
			}
			fmt.Printf("%.2f  %-22s %-8s %s%s\n     %s\n", r.Score, r.ID, r.Type, r.Title, marker, r.Project)
		}
		return nil
	},
}

func init() {
	searchCmd.Flags().StringVarP(&searchProject, "project", "p", "", "限定项目")
	searchCmd.Flags().StringVarP(&searchType, "type", "t", "", "限定类型")
	searchCmd.Flags().IntVarP(&searchLimit, "limit", "n", 5, "返回条数")
	searchCmd.Flags().BoolVarP(&searchIncludeSuperseded, "include-superseded", "A", false, "包含已被取代的记忆(默认不召回,溯源用)")
}
