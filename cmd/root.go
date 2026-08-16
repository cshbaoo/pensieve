package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cshbaoo/pensieve/internal/config"
)

var (
	flagConfig string
	flagJSON   bool
	cfg        *config.Config
)

var rootCmd = &cobra.Command{
	Use:           "pensieve",
	Short:         "Pensieve — 编程记忆系统：把每次踩坑、决策、发现沉淀为可复用资产",
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		path := flagConfig
		if path == "" {
			var err error
			path, err = config.DefaultPath()
			if err != nil {
				return err
			}
		}
		var err error
		cfg, err = config.Load(path)
		return err
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", "", "配置文件路径（默认 ~/.pensieve/config.toml)\"")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "JSON 输出")

	rootCmd.AddCommand(initCmd, addCmd, searchCmd, getCmd, listCmd)
}
