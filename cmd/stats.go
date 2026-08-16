package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/cshbaoo/pensieve/internal/core/stats"
)

var statsDays int

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "使用埋点报表（本地数据,从不上传）",
	RunE: func(cmd *cobra.Command, args []string) error {
		events, err := stats.LoadAll(cfg.Core.IndexDir)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			fmt.Println("还没有埋点数据——用起来,过两天来看。")
			return nil
		}
		s := stats.Summarize(events, statsDays)

		period := "全部"
		if statsDays > 0 {
			period = fmt.Sprintf("近 %d 天", statsDays)
		}
		fmt.Printf("📊 Pensieve 使用报表（%s）\n\n", period)
		fmt.Printf("活跃天数: %d  |  起始: %s  |  最近: %s\n\n",
			s.ActiveDays,
			time.Unix(s.FirstTs, 0).Format("01-02 15:04"),
			time.Unix(s.LastTs, 0).Format("01-02 15:04"))

		fmt.Println("== 行为分布 ==")
		fmt.Printf("  搜索 %d 次（命中 %d,命中率 %.0f%%）\n",
			s.SearchTotal, s.SearchWithHit, pct(s.SearchWithHit, s.SearchTotal))
		fmt.Printf("  新增 %d  |  读取 %d  |  更新 %d  |  简报 %d\n\n",
			s.ByEvent["save"], s.ByEvent["get"], s.ByEvent["update"], s.ByEvent["context"])

		fmt.Println("== 来源分布 ==")
		for src, n := range s.BySource {
			fmt.Printf("  %-6s %d 次\n", src, n)
		}

		if len(s.GetTopIDs) > 0 {
			fmt.Println("\n== 最常被读取的记忆 top5 ==")
			for i, ic := range s.GetTopIDs {
				if i >= 5 {
					break
				}
				fmt.Printf("  %d. %s ×%d\n", i+1, ic.ID, ic.Count)
			}
		}

		if len(s.SaveTopTypes) > 0 {
			fmt.Println("\n== 沉淀类型分布 ==")
			for t, n := range s.SaveTopTypes {
				fmt.Printf("  %-9s %d 条\n", t, n)
			}
		}

		// 北极星提示
		fmt.Println()
		reuseEvents := s.ByEvent["get"]
		if reuseEvents >= 3 {
			fmt.Println("⭐ 复用率达标:本周/期记忆被读取 ≥3 次,记忆真在被用起来")
		} else {
			fmt.Printf("🌱 复用率观察:记忆被读取 %d 次(达标≥3/周沉积才会真正飞轮)\n", reuseEvents)
		}
		return nil
	},
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) * 100 / float64(b)
}

func init() {
	statsCmd.Flags().IntVarP(&statsDays, "days", "d", 7, "统计近几天(0=全部)")
	rootCmd.AddCommand(statsCmd)
}
