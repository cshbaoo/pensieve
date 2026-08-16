package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cshbaoo/pensieve/internal/core/export"
	"github.com/cshbaoo/pensieve/internal/core/memory"
)

var (
	exportOut      string
	exportCheck    bool
	exportProject  string
	exportMinVotes int
	exportDays     int
	exportMax      int
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "把记忆库的高频坑与使用纪律导出为 AGENTS.md(供任意 agent 读取)",
	Long: `在当前仓库生成/更新 AGENTS.md 的 pensieve 托管区:
内容只有"工作纪律指令 + 高频坑一句话清单"(路标),详情永远留在记忆库。

托管区由 <!-- pensieve:begin/end --> 包裹,重复执行幂等,不覆盖手写的其余内容。
配合 --check 可挂在 CI 上,保证摘要与记忆库同步。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		rootOut, err := exec.Command("git", "rev-parse", "--show-toplevel").CombinedOutput()
		if err != nil {
			return fmt.Errorf("当前目录不在 git 仓库中: %s", strings.TrimSpace(string(rootOut)))
		}
		root := strings.TrimSpace(string(rootOut))

		project := exportProject
		if project == "" {
			project = export.ProjectFromGitRemote(root)
		}

		var mems []*memory.Memory
		if err := memory.NewStore(cfg.Core.RepoDir).Walk(func(m *memory.Memory) error {
			mems = append(mems, m)
			return nil
		}); err != nil {
			return err
		}

		gotchas := export.Collect(mems, export.Options{
			Project:   project,
			MinVotes:  exportMinVotes,
			SinceDays: exportDays,
			MaxItems:  exportMax,
		}, time.Now())
		section := export.RenderSection(gotchas)

		path := filepath.Join(strings.TrimSuffix(root, "/"), exportOut)
		var existing []byte
		if data, err := os.ReadFile(path); err == nil {
			existing = data
		} else if !os.IsNotExist(err) {
			return err
		}
		desired := export.Upsert(string(existing), section)

		if exportCheck {
			if string(existing) != desired {
				return fmt.Errorf("%s 与记忆库不同步,请运行 pensieve export 更新", path)
			}
			fmt.Printf("✔ %s 已同步\n", path)
			return nil
		}

		if string(existing) == desired {
			fmt.Printf("✔ %s 无变化,无需写入\n", exportOut)
			return nil
		}
		if err := os.WriteFile(path, []byte(desired), 0o644); err != nil {
			return err
		}
		fmt.Printf("✔ 已导出 %s(项目 %q,高频坑 %d 条)\n", path, projectOrAll(project), len(gotchas))
		return nil
	},
}

func projectOrAll(p string) string {
	if p == "" {
		return "全部"
	}
	return p
}

func init() {
	exportCmd.Flags().StringVar(&exportOut, "out", "AGENTS.md", "输出文件名(CLAUDE.md 等)")
	exportCmd.Flags().BoolVar(&exportCheck, "check", false, "只检查是否同步,不写入(不同步时退出码非零,适合 CI)")
	exportCmd.Flags().StringVar(&exportProject, "project", "", "限定项目;缺省从当前仓库 git remote 自动检测")
	exportCmd.Flags().IntVar(&exportMinVotes, "min-votes", 2, "高频坑入选条件:投票数不低于该值")
	exportCmd.Flags().IntVar(&exportDays, "days", 30, "高频坑入选条件:近 N 天新建")
	exportCmd.Flags().IntVar(&exportMax, "max", 10, "高频坑清单最多条数")
	rootCmd.AddCommand(exportCmd)
}
