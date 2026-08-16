package cmd

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/cshbaoo/pensieve/internal/core/memory"
)

var (
	listProject string
	listLimit   int
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "列出最近的记忆",
	RunE: func(cmd *cobra.Command, args []string) error {
		store := memory.NewStore(cfg.Core.RepoDir)
		var all []*memory.Memory
		if err := store.Walk(func(m *memory.Memory) error {
			if listProject == "" || m.Project == listProject {
				all = append(all, m)
			}
			return nil
		}); err != nil {
			return err
		}
		sort.Slice(all, func(i, j int) bool { return all[i].Created.After(all[j].Created) })
		if listLimit > 0 && len(all) > listLimit {
			all = all[:listLimit]
		}
		for _, m := range all {
			status := m.Status
			if status == "active" {
				status = "" // active 是默认态,不显示,保持表干净
			}
			fmt.Printf("%-22s %-9s %-11s %-9s %s\n", m.ID, m.Type, m.Project, status, m.Title)
		}
		fmt.Printf("\n共 %d 条\n", len(all))
		return nil
	},
}

func init() {
	listCmd.Flags().StringVarP(&listProject, "project", "p", "", "限定项目")
	listCmd.Flags().IntVarP(&listLimit, "limit", "n", 20, "最多显示条数")
}
