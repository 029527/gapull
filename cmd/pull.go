package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/029527/gapull/internal/config"
	"github.com/029527/gapull/internal/downloader"
	gh "github.com/029527/gapull/internal/github"
	"github.com/spf13/cobra"
)

var (
	flagArch    string
	flagType    string
	flagOutput  string
	flagWorkers int
)

var pullCmd = &cobra.Command{
	Use:   "pull <image>[,image2,...]",
	Short: "通过 GitHub Action 拉取并下载 Docker 镜像 tar 包",
	Example: `  gapull pull nginx:latest
  gapull pull nginx:latest,redis:7 --arch arm64
  gapull pull alpine:latest --type artifact --output ./images`,
	Args: cobra.ExactArgs(1),
	RunE: runPull,
}

func init() {
	pullCmd.Flags().StringVar(&flagArch, "arch", "", "目标架构: amd64 | arm64 | arm32（默认自动检测当前系统架构）")
	pullCmd.Flags().StringVar(&flagType, "type", "release", "产物类型: release | artifact")
	pullCmd.Flags().StringVar(&flagOutput, "output", "", "本地保存目录（默认为当前目录）")
	pullCmd.Flags().IntVar(&flagWorkers, "workers", 0, "并发下载线程数（0=自动）")
}

// nativeArch 将 runtime.GOARCH 映射到 workflow 架构名
func nativeArch() gh.Arch {
	switch runtime.GOARCH {
	case "arm64":
		return gh.ArchARM64
	case "arm":
		return gh.ArchARM32
	default:
		return gh.ArchAMD64
	}
}

func runPull(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	// 架构：用户未指定时自动检测
	arch := nativeArch()
	if flagArch != "" {
		arch = gh.Arch(flagArch)
	}

	// 输出目录：用户未指定时使用当前工作目录
	outputDir := flagOutput
	if outputDir == "" {
		outputDir, err = os.Getwd()
		if err != nil {
			outputDir = "."
		}
	}
	outputDir, _ = filepath.Abs(outputDir)

	images := strings.Split(args[0], ",")
	for i, img := range images {
		images[i] = strings.TrimSpace(img)
	}

	wt := gh.WorkflowType(flagType)
	client := gh.NewClient(cfg.Token, cfg.Owner, cfg.Repo)
	ctx := context.Background()

	fmt.Printf("触发 Workflow（架构=%s，类型=%s）...\n", arch, wt)
	fmt.Printf("镜像列表: %s\n", strings.Join(images, ", "))

	tr, err := client.Trigger(ctx, strings.Join(images, ","), arch, wt)
	if err != nil {
		return err
	}
	fmt.Println("触发成功，等待 Workflow 启动...")

	statusCh := make(chan string, 32)
	go func() {
		for msg := range statusCh {
			fmt.Println(" ", msg)
		}
	}()

	runID, err := client.PollRun(ctx, tr, statusCh)
	if err != nil {
		close(statusCh)
		return err
	}

	conclusion, err := client.WaitComplete(ctx, runID, statusCh)
	close(statusCh)
	if err != nil {
		return err
	}
	if conclusion != "success" {
		return fmt.Errorf("Workflow 结束，结论: %s（非 success）", conclusion)
	}
	fmt.Println("Workflow 运行成功，准备下载...")

	var assets []*gh.AssetURL
	if wt == gh.TypeRelease {
		assets, err = client.ListReleaseAssets(ctx, tr.ReleaseTag, images)
	} else {
		assets, err = client.ListArtifactAssets(ctx, runID, images)
	}
	if err != nil {
		return err
	}
	if len(assets) == 0 {
		return fmt.Errorf("未找到匹配的下载资产，请检查镜像名称是否正确")
	}

	opts := downloader.DefaultOptions(outputDir)
	if flagWorkers > 0 {
		opts.Workers = flagWorkers
	}
	if opts.ProxyURL != "" {
		fmt.Printf("使用代理: %s，workers=%d\n", opts.ProxyURL, opts.Workers)
	} else {
		fmt.Printf("无代理直连，高并发模式 workers=%d\n", opts.Workers)
	}

	var failed []string
	for _, asset := range assets {
		fmt.Printf("\n下载 %s  (%s)\n", asset.Name, formatBytes(asset.Size))
		path, err := downloader.Download(ctx, asset.Name, asset.DownloadURL, asset.Size, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "下载失败: %v\n", err)
			failed = append(failed, asset.Name)
			continue
		}
		fmt.Printf("已保存: %s\n", path)
	}

	if len(failed) > 0 {
		return fmt.Errorf("以下文件下载失败: %s", strings.Join(failed, ", "))
	}

	fmt.Println("\n全部完成！加载镜像：")
	for _, asset := range assets {
		fmt.Printf("  docker load -i %s\n", filepath.Join(outputDir, asset.Name))
	}
	return nil
}

func formatBytes(b int64) string {
	if b <= 0 {
		return "未知大小"
	}
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
