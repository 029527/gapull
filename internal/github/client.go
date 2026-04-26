package github

import (
	"context"
	"fmt"
	"strings"
	"time"

	gh "github.com/google/go-github/v62/github"
	"golang.org/x/oauth2"
)

// WorkflowType 区分 artifact 和 release 两种产物类型
type WorkflowType string

const (
	TypeArtifact WorkflowType = "artifact"
	TypeRelease  WorkflowType = "release"
)

// Arch 目标架构
type Arch string

const (
	ArchAMD64 Arch = "amd64"
	ArchARM64 Arch = "arm64"
	ArchARM32 Arch = "arm32"
)

// workflowFile 根据架构和类型映射到实际文件名
func workflowFile(arch Arch, wt WorkflowType) (string, string) {
	switch wt {
	case TypeRelease:
		switch arch {
		case ArchAMD64:
			return "release-amd64.yml", "DockerTarBuilder-AMD64"
		case ArchARM64:
			return "release-arm64.yml", "DockerTarBuilder-ARM64"
		case ArchARM32:
			return "release-arm32.yml", "DockerTarBuilder-ARM32"
		}
	default:
		switch arch {
		case ArchAMD64:
			return "artifact-amd64.yml", ""
		case ArchARM64:
			return "artifact-arm64.yml", ""
		case ArchARM32:
			return "artifact-arm32.yml", ""
		}
	}
	return "", ""
}

// Client 封装所有 GitHub API 操作
type Client struct {
	gh    *gh.Client
	owner string
	repo  string
}

func NewClient(token, owner, repo string) *Client {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	return &Client{
		gh:    gh.NewClient(tc),
		owner: owner,
		repo:  repo,
	}
}

// TriggerResult 触发后的上下文信息
type TriggerResult struct {
	WorkflowFile string
	ReleaseTag   string
	WorkflowType WorkflowType
	TriggeredAt  time.Time
}

// Trigger 触发 workflow dispatch，images 为逗号分隔的镜像列表
func (c *Client) Trigger(ctx context.Context, images string, arch Arch, wt WorkflowType) (*TriggerResult, error) {
	file, tag := workflowFile(arch, wt)
	if file == "" {
		return nil, fmt.Errorf("不支持的架构或类型: %s/%s", arch, wt)
	}

	event := gh.CreateWorkflowDispatchEventRequest{
		Ref:    "main",
		Inputs: map[string]interface{}{"images": images},
	}

	triggeredAt := time.Now().UTC().Add(-5 * time.Second) // 留 5s 余量应对时钟偏差
	_, err := c.gh.Actions.CreateWorkflowDispatchEventByFileName(ctx, c.owner, c.repo, file, event)
	if err != nil {
		return nil, fmt.Errorf("触发 Workflow 失败: %w", err)
	}

	return &TriggerResult{
		WorkflowFile: file,
		ReleaseTag:   tag,
		WorkflowType: wt,
		TriggeredAt:  triggeredAt,
	}, nil
}

// PollRun 轮询直到找到触发后产生的 Run ID，超时返回错误
func (c *Client) PollRun(ctx context.Context, tr *TriggerResult, statusCh chan<- string) (int64, error) {
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}

		opts := &gh.ListWorkflowRunsOptions{
			Created: ">=" + tr.TriggeredAt.Format("2006-01-02T15:04:05Z"),
			ListOptions: gh.ListOptions{PerPage: 10},
		}
		runs, _, err := c.gh.Actions.ListWorkflowRunsByFileName(ctx, c.owner, c.repo, tr.WorkflowFile, opts)
		if err != nil {
			time.Sleep(10 * time.Second)
			continue
		}
		if runs.GetTotalCount() > 0 {
			run := runs.WorkflowRuns[0]
			statusCh <- fmt.Sprintf("找到 Run #%d，状态: %s", run.GetID(), run.GetStatus())
			return run.GetID(), nil
		}
		statusCh <- "等待 Workflow 启动..."
		time.Sleep(10 * time.Second)
	}
	return 0, fmt.Errorf("等待 Workflow 启动超时（3 分钟）")
}

// WaitComplete 轮询 Run 直到完成，返回 conclusion
func (c *Client) WaitComplete(ctx context.Context, runID int64, statusCh chan<- string) (string, error) {
	deadline := time.Now().Add(30 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		run, _, err := c.gh.Actions.GetWorkflowRunByID(ctx, c.owner, c.repo, runID)
		if err != nil {
			time.Sleep(15 * time.Second)
			continue
		}

		status := run.GetStatus()
		conclusion := run.GetConclusion()
		statusCh <- fmt.Sprintf("Run #%d 状态: %s", runID, status)

		if status == "completed" {
			return conclusion, nil
		}
		time.Sleep(15 * time.Second)
	}
	return "", fmt.Errorf("等待 Workflow 完成超时（30 分钟）")
}

// AssetURL 是一个下载资源的描述
type AssetURL struct {
	Name        string
	DownloadURL string
	Size        int64
}

// ListReleaseAssets 列出 Release Tag 下的所有资产，按镜像名过滤
func (c *Client) ListReleaseAssets(ctx context.Context, tag string, images []string) ([]*AssetURL, error) {
	release, _, err := c.gh.Repositories.GetReleaseByTag(ctx, c.owner, c.repo, tag)
	if err != nil {
		return nil, fmt.Errorf("获取 Release 失败: %w", err)
	}

	var assets []*AssetURL
	for _, a := range release.Assets {
		name := a.GetName()
		if matchesAnyImage(name, images) {
			assets = append(assets, &AssetURL{
				Name:        name,
				DownloadURL: a.GetBrowserDownloadURL(),
				Size:        int64(a.GetSize()),
			})
		}
	}
	return assets, nil
}

// ListArtifactAssets 列出指定 Run 下的所有 artifact 下载 URL
// 不按名称过滤——dispatch 触发的 Run 里只会有本次构建的产物
func (c *Client) ListArtifactAssets(ctx context.Context, runID int64, _ []string) ([]*AssetURL, error) {
	arts, _, err := c.gh.Actions.ListWorkflowRunArtifacts(ctx, c.owner, c.repo, runID, nil)
	if err != nil {
		return nil, fmt.Errorf("获取 Artifact 列表失败: %w", err)
	}

	var assets []*AssetURL
	for _, a := range arts.Artifacts {
		url, _, err := c.gh.Actions.DownloadArtifact(ctx, c.owner, c.repo, a.GetID(), 5)
		if err != nil {
			continue
		}
		assets = append(assets, &AssetURL{
			Name:        a.GetName() + ".zip",
			DownloadURL: url.String(),
			Size:        0,
		})
	}
	return assets, nil
}

// matchesAnyImage 检查文件名是否对应任一目标镜像
func matchesAnyImage(filename string, images []string) bool {
	for _, img := range images {
		// 规则与 Workflow 中一致：/ → _ ，: → _
		normalized := strings.ReplaceAll(img, "/", "_")
		normalized = strings.ReplaceAll(normalized, ":", "_")
		if strings.HasPrefix(filename, normalized) {
			return true
		}
	}
	return false
}
