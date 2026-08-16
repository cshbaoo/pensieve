package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/cshbaoo/pensieve/internal/core/stats"
)

var (
	statsDays   int
	statsExport string
	statsFrom   []string
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "使用埋点报表（本地数据,主动导出的除外）",
	RunE: func(cmd *cobra.Command, args []string) error {
		// 导出模式：把本机埋点打包成 JSON,可发给同事/负责人查看
		if statsExport != "" {
			r, err := stats.ExportReport(cfg.Core.IndexDir)
			if err != nil {
				return err
			}
			if err := stats.SaveReport(r, statsExport); err != nil {
				return err
			}
			fmt.Printf("✅ 已导出 %d 条事件 → %s\n", len(r.Events), statsExport)
			fmt.Println("仅含行为计数（动作/来源/项目名/记忆 id),不含搜索词与记忆内容;发送前可自行打开检查。")
			return nil
		}

		// 查看模式：渲染别人导出的报表（支持多人合并对比）
		if len(statsFrom) > 0 {
			return renderReports(statsFrom, statsDays)
		}

		// 本地报表
		events, err := stats.LoadAll(cfg.Core.IndexDir)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			fmt.Println("还没有埋点数据——用起来,过两天来看。")
			return nil
		}
		renderSummary("本机", events, statsDays)
		return nil
	},
}

// renderReports 渲染一个或多个导出报表：逐个展示,并给出合并视图
func renderReports(paths []string, days int) error {
	var merged []stats.Event
	for _, path := range paths {
		r, err := stats.LoadReportFile(path)
		if err != nil {
			return err
		}
		title := r.Exporter
		if title == "" {
			title = path
		}
		title = fmt.Sprintf("%s（导出于 %s）", title, time.Unix(r.ExportedAt, 0).Format("01-02 15:04"))
		renderSummary(title, r.Events, days)
		merged = append(merged, r.Events...)
	}
	if len(paths) > 1 {
		fmt.Println()
		renderSummary("多人合计", merged, days)
	}
	return nil
}

// renderSummary 把事件聚合并打印报表
func renderSummary(title string, events []stats.Event, days int) {
	if len(events) == 0 {
		fmt.Printf("📊 %s:没有埋点数据\n", title)
		return
	}
	s := stats.Summarize(events, days)

	period := "全部"
	if days > 0 {
		period = fmt.Sprintf("近 %d 天", days)
	}
	fmt.Printf("📊 Pensieve 使用报表（%s · %s）\n\n", title, period)
	fmt.Printf("事件总数: %d  |  活跃天数: %d  |  起始: %s  |  最近: %s\n\n",
		s.TotalEvents, s.ActiveDays,
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
		fmt.Println("⭐ 复用率达标:记忆被读取 ≥3 次,记忆真在被用起来")
	} else {
		fmt.Printf("🌱 复用率观察:记忆被读取 %d 次(达标≥3/周沉积才会真正飞轮)\n", reuseEvents)
	}
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) * 100 / float64(b)
}

func init() {
	statsCmd.Flags().IntVarP(&statsDays, "days", "d", 7, "统计近几天(0=全部)")
	statsCmd.Flags().StringVarP(&statsExport, "export", "o", "", "导出埋点数据到 JSON 文件(可发给他人查看)")
	statsCmd.Flags().StringSliceVarP(&statsFrom, "from", "f", nil, "查看他人导出的 stats JSON(可多次指定,自动合并)")
	rootCmd.AddCommand(statsCmd)
}
