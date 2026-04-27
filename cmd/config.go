package cmd

import (
	"context"
	"fmt"

	"github.com/029527/gapull/internal/config"
	"github.com/029527/gapull/internal/github"
	"github.com/spf13/cobra"
)

var (
	flagToken string
	flagOwner string
	flagRepo  string
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "管理工具配置",
}

var configSetCmd = &cobra.Command{
	Use:   "set",
	Short: "设置配置项（持久化到 ~/.config/docker-pull-proxy/config.json）",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if flagToken != "" {
			cfg.Token = flagToken
		}
		if flagOwner != "" {
			cfg.Owner = flagOwner
		} else if flagToken != "" {
			// 未显式指定 owner，根据 token 自动查询归属账号
			fmt.Println("正在通过 Token 获取 GitHub 用户名...")
			client := github.NewClientWithToken(cfg.Token)
			login, err := client.GetAuthenticatedUser(context.Background())
			if err != nil {
				fmt.Printf("警告: 无法自动获取 GitHub 用户名（%v），请手动指定 --owner\n", err)
			} else {
				cfg.Owner = login
				fmt.Printf("自动设置 owner 为: %s\n", login)
			}
		}
		if flagRepo != "" {
			cfg.Repo = flagRepo
		}
		if err := config.Save(cfg); err != nil {
			return fmt.Errorf("保存配置失败: %w", err)
		}
		fmt.Println("配置已保存:")
		fmt.Println(cfg)
		return nil
	},
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "显示当前配置",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		fmt.Println(cfg)
		return nil
	},
}

func init() {
	configSetCmd.Flags().StringVar(&flagToken, "token", "", "GitHub Personal Access Token（需要 workflow 权限）")
	configSetCmd.Flags().StringVar(&flagOwner, "owner", "", "仓库 Owner（默认: its029527）")
	configSetCmd.Flags().StringVar(&flagRepo, "repo", "", "仓库名（默认: DockerTarBuilder）")

	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configShowCmd)
}
