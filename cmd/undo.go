package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

var undoYes bool

var undoCmd = &cobra.Command{
	Use:   "undo",
	Short: "撤销最近一次记忆写入（回滚最新的 memory 提交）",
	RunE: func(cmd *cobra.Command, args []string) error {
		repo := cfg.Core.RepoDir

		// 查看最近提交
		out, err := exec.Command("git", "-C", repo, "log", "-1", "--oneline", "--format=%s").CombinedOutput()
		if err != nil {
			return fmt.Errorf("读取最近提交失败: %w", err)
		}
		msg := strings.TrimSpace(string(out))
		if !strings.HasPrefix(msg, "memory:") && !strings.HasPrefix(msg, "memory-update:") && !strings.HasPrefix(msg, "migrate:") {
			return fmt.Errorf("最近一次提交不是记忆写入（%q）,为避免误操作已中止", msg)
		}
		fmt.Printf("将撤销最近的提交: %s\n", msg)
		fmt.Println("该提交的文件改动会被丢弃（受保护:只会动本提交内的记忆文件）")

		if !undoYes {
			fmt.Print("确认撤销? [y/N] ")
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			if !strings.EqualFold(strings.TrimSpace(answer), "y") {
				fmt.Println("已取消")
				return nil
			}
		}

		// 找出这次提交影响的文件,逐个删除
		filesOut, err := exec.Command("git", "-C", repo, "show", "--name-only", "--format=", "HEAD").CombinedOutput()
		if err != nil {
			return fmt.Errorf("查询提交文件失败: %w", err)
		}
		for _, f := range strings.Split(strings.TrimSpace(string(filesOut)), "\n") {
			f = strings.TrimSpace(f)
			if f != "" {
				p := repo + string(os.PathSeparator) + strings.ReplaceAll(f, "/", string(os.PathSeparator))
				_ = os.Remove(p)
				fmt.Println("  删除文件:", f)
			}
		}

		// 回滚提交本身
		if out, err := exec.Command("git", "-C", repo, "reset", "--hard", "HEAD~1").CombinedOutput(); err != nil {
			return fmt.Errorf("git reset 失败: %w\n%s", err, out)
		}

		// 重建索引(增量会检测到文件缺失并清理残留)
		n, err := reindexAll()
		if err != nil {
			return err
		}
		fmt.Printf("✔ 已撤销,当前索引 %d 条\n", n)
		return nil
	},
}

func init() {
	undoCmd.Flags().BoolVarP(&undoYes, "yes", "y", false, "跳过确认")
	rootCmd.AddCommand(undoCmd)
}
