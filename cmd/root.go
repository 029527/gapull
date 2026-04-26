package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "gapull",
	Short: "通过 GitHub Action 在云端拉取 Docker 镜像并下载到本地",
	Long: `gapull 通过触发 GitHub Action 在云端拉取 Docker 镜像，
打包为 .tar.gz 后下载到本地。支持 amd64 / arm64 / arm32 架构，
并发分块下载，自动适配代理环境。`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(configCmd)
}
