package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cshbaoo/pensieve/internal/core/index"
	"github.com/cshbaoo/pensieve/internal/core/memory"
	"github.com/cshbaoo/pensieve/internal/core/stats"
)

var getCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "按 id 读取记忆全文",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		store := memory.NewStore(cfg.Core.RepoDir)
		m, err := store.GetByID(args[0])
		if err != nil {
			return err
		}
		stats.Track(cfg.Core.IndexDir, "get", "cli", m.Project, map[string]any{"id": m.ID})
		data, err := memory.Marshal(m)
		if err != nil {
			return err
		}
		// 读取兜底:过时记忆先出横幅,防漂移结论被采信
		out := store.FreshnessNotice(m) + string(data)

		// backlinks:哪些记忆链接到了这条(反向引用索引,ECC 借鉴)
		if idx, err := index.Open(cfg.Core.IndexDir); err == nil {
			defer idx.Close()
			if parents, err := idx.Parents(ctx, m.ID); err == nil && len(parents) > 0 {
				out += "\n---\n\n🔁 被这些记忆引用:\n\n"
				for _, p := range parents {
					rel := p.Rel
					if rel == "" {
						rel = "linked"
					}
					out += fmt.Sprintf("- [%s] %s (%s, rel=%s)\n", p.Type, p.Title, p.ID, rel)
				}
			}
		}
		fmt.Println(out)
		return nil
	},
}
