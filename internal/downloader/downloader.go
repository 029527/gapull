package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/schollz/progressbar/v3"
)

const (
	defaultChunkSize = 5 * 1024 * 1024 // 5MB
	defaultWorkers   = 8
	maxRetries       = 5
)

// Options 下载选项
type Options struct {
	OutputDir string
	ChunkSize int64
	Workers   int
	ProxyURL  string
}

func DefaultOptions(outputDir string) *Options {
	opts := &Options{
		OutputDir: outputDir,
		ChunkSize: defaultChunkSize,
		Workers:   defaultWorkers,
		ProxyURL:  os.Getenv("HTTPS_PROXY"),
	}
	if opts.ProxyURL == "" {
		opts.ProxyURL = os.Getenv("HTTP_PROXY")
	}
	if opts.ProxyURL == "" {
		opts.Workers = 16
	}
	return opts
}

type chunk struct {
	index int
	start int64
	end   int64
}

// Download 多线程下载单个文件，进度条原地刷新，返回本地文件路径
func Download(ctx context.Context, name, rawURL string, totalSize int64, opts *Options) (string, error) {
	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		return "", err
	}
	outPath := filepath.Join(opts.OutputDir, name)

	transport := buildTransport(opts.ProxyURL)
	client := &http.Client{Transport: transport, Timeout: 0}

	if totalSize <= 0 {
		size, rangeOK, err := probe(ctx, client, rawURL)
		if err != nil {
			return "", err
		}
		totalSize = size
		if !rangeOK || totalSize <= 0 {
			return downloadSingle(ctx, client, name, rawURL, outPath)
		}
	}

	chunks := splitChunks(totalSize, opts.ChunkSize)

	f, err := os.OpenFile(outPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return "", err
	}
	if err := f.Truncate(totalSize); err != nil {
		f.Close()
		return "", err
	}
	f.Close()

	// 单一进度条，由专属 goroutine 驱动，避免多线程并发写终端
	bar := progressbar.NewOptions64(
		totalSize,
		progressbar.OptionSetDescription(fmt.Sprintf("  %-40s", name)),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(35),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionUseANSICodes(true),
		progressbar.OptionOnCompletion(func() { fmt.Fprint(os.Stderr, "\n") }),
		progressbar.OptionSetWriter(os.Stderr),
	)

	var downloaded int64
	doneCh := make(chan struct{})

	// 唯一写终端的 goroutine
	go func() {
		ticker := time.NewTicker(150 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = bar.Set64(atomic.LoadInt64(&downloaded))
			case <-doneCh:
				_ = bar.Set64(atomic.LoadInt64(&downloaded))
				return
			}
		}
	}()

	var wg sync.WaitGroup
	errCh := make(chan error, len(chunks))
	sem := make(chan struct{}, opts.Workers)

	for _, ck := range chunks {
		wg.Add(1)
		sem <- struct{}{}
		go func(ck chunk) {
			defer wg.Done()
			defer func() { <-sem }()
			n, err := downloadChunk(ctx, client, rawURL, outPath, ck)
			if err != nil {
				errCh <- fmt.Errorf("chunk %d 失败: %w", ck.index, err)
				return
			}
			atomic.AddInt64(&downloaded, n)
		}(ck)
	}

	wg.Wait()
	close(doneCh)
	close(errCh)

	if err := <-errCh; err != nil {
		os.Remove(outPath)
		return "", err
	}

	return outPath, nil
}

func downloadChunk(ctx context.Context, client *http.Client, rawURL, outPath string, ck chunk) (int64, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(1<<uint(attempt)) * time.Second
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(wait):
			}
		}
		n, err := writeChunk(ctx, client, rawURL, outPath, ck)
		if err == nil {
			return n, nil
		}
		lastErr = err
	}
	return 0, fmt.Errorf("重试 %d 次后仍失败: %w", maxRetries, lastErr)
}

func writeChunk(ctx context.Context, client *http.Client, rawURL, outPath string, ck chunk) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", ck.start, ck.end))

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("服务器返回非 206 状态: %d", resp.StatusCode)
	}

	f, err := os.OpenFile(outPath, os.O_WRONLY, 0644)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	if _, err := f.Seek(ck.start, io.SeekStart); err != nil {
		return 0, err
	}

	return io.Copy(f, resp.Body)
}

func downloadSingle(ctx context.Context, client *http.Client, name, rawURL, outPath string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	f, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	bar := progressbar.NewOptions64(
		resp.ContentLength,
		progressbar.OptionSetDescription(fmt.Sprintf("  %-40s", name)),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(35),
		progressbar.OptionUseANSICodes(true),
		progressbar.OptionOnCompletion(func() { fmt.Fprint(os.Stderr, "\n") }),
		progressbar.OptionSetWriter(os.Stderr),
	)
	_, err = io.Copy(io.MultiWriter(f, bar), resp.Body)
	return outPath, err
}

func probe(ctx context.Context, client *http.Client, rawURL string) (int64, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, rawURL, nil)
	if err != nil {
		return 0, false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, false, err
	}
	resp.Body.Close()
	return resp.ContentLength, resp.Header.Get("Accept-Ranges") == "bytes", nil
}

func splitChunks(totalSize, chunkSize int64) []chunk {
	var chunks []chunk
	var i int
	for start := int64(0); start < totalSize; start += chunkSize {
		end := start + chunkSize - 1
		if end >= totalSize {
			end = totalSize - 1
		}
		chunks = append(chunks, chunk{index: i, start: start, end: end})
		i++
	}
	return chunks
}

func buildTransport(proxyURL string) *http.Transport {
	t := &http.Transport{
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 30 * time.Second,
	}
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			t.Proxy = http.ProxyURL(u)
		}
	}
	return t
}
