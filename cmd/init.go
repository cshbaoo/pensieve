package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/cshbaoo/pensieve/internal/config"
)

var initFrom string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "初始化 Pensieve（创建目录/仓库/索引；可用 --from 克隆远程仓库）",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := config.HomeDir()
		if err != nil {
			return err
		}
		cfgPath, _ := config.DefaultPath()

		// 1. 目录
		for _, d := range []string{cfg.Core.RepoDir, cfg.Core.IndexDir} {
			if err := os.MkdirAll(d, 0o755); err != nil {
				return err
			}
		}
		fmt.Println("✔ 目录就绪:", home)

		// 2. 配置
		if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
			if err := config.WriteDefault(cfgPath, home); err != nil {
				return err
			}
			fmt.Println("✔ 配置生成:", cfgPath, "（如需 LLM 能力请填入 llm.api_key）")
		} else {
			fmt.Println("= 配置已存在:", cfgPath)
		}

		// 3. git 仓库
		gitDir := filepath.Join(cfg.Core.RepoDir, ".git")
		if _, err := os.Stat(gitDir); os.IsNotExist(err) {
			if initFrom != "" {
				if out, err := exec.Command("git", "clone", initFrom, cfg.Core.RepoDir).CombinedOutput(); err != nil {
					return fmt.Errorf("克隆失败: %w\n%s", err, out)
				}
				fmt.Println("✔ 已从远程克隆记忆仓库:", initFrom)
			} else {
				if out, err := exec.Command("git", "-C", cfg.Core.RepoDir, "init").CombinedOutput(); err != nil {
					return fmt.Errorf("git init 失败: %w\n%s", err, out)
				}
				fmt.Println("✔ 记忆仓库已初始化:", cfg.Core.RepoDir)
			}
		} else {
			fmt.Println("= 仓库已存在:", cfg.Core.RepoDir)
		}

		// 4. 全量建索引
		n, err := reindexAll()
		if err != nil {
			return fmt.Errorf("建索引失败: %w", err)
		}
		fmt.Printf("✔ 索引构建完成: %d 条记忆\n", n)
		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&initFrom, "from", "", "远程记忆仓库 URL（多设备/团队共享）")
}
