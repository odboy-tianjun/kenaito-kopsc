package executor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// runNetSpeed implements `kopsc net speed`: simple network speed test.
func runNetSpeed(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseNetSpeedArgs(req.Args)
	if err != nil {
		return err
	}

	fmt.Fprint(req.Stdout, "=== 网络速度测试 ===\n")

	// Download test
	if !opts.uploadOnly {
		fmt.Fprintln(req.Stdout, "测试下载速度...")
		downloadSpeed, err := testDownload(ctx, opts.size)
		if err != nil {
			fmt.Fprintf(req.Stderr, "下载测试失败: %v\n", err)
		} else {
			fmt.Fprintf(req.Stdout, "下载速度: %s/s\n", humanSize(downloadSpeed))
		}
		fmt.Fprintln(req.Stdout)
	}

	// Upload test
	if !opts.downloadOnly {
		fmt.Fprintln(req.Stdout, "测试上传速度...")
		uploadSpeed, err := testUpload(ctx, opts.size)
		if err != nil {
			fmt.Fprintf(req.Stderr, "上传测试失败: %v\n", err)
		} else {
			fmt.Fprintf(req.Stdout, "上传速度: %s/s\n", humanSize(uploadSpeed))
		}
	}

	return nil
}

type netSpeedOptions struct {
	size         int64
	downloadOnly bool
	uploadOnly   bool
}

func parseNetSpeedArgs(args []string) (netSpeedOptions, error) {
	opts := netSpeedOptions{size: 10 * 1024 * 1024} // 10MB default

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			remaining := args[i+1:]
			if len(remaining) > 0 {
				n, err := parseSizeArg(remaining[0])
				if err != nil {
					return opts, fmt.Errorf("大小参数无效: %w", err)
				}
				opts.size = n
			}
			i = len(args)
		case arg == "--size" || arg == "-s":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少大小参数", arg)
			}
			i++
			n, err := parseSizeArg(args[i])
			if err != nil {
				return opts, fmt.Errorf("--size: %w", err)
			}
			opts.size = n
		case strings.HasPrefix(arg, "--size="):
			n, err := parseSizeArg(strings.TrimPrefix(arg, "--size="))
			if err != nil {
				return opts, fmt.Errorf("--size: %w", err)
			}
			opts.size = n
		case arg == "--download" || arg == "-d":
			opts.downloadOnly = true
		case arg == "--upload" || arg == "-u":
			opts.uploadOnly = true
		default:
			return opts, fmt.Errorf("未知选项 %q", arg)
		}
	}

	if opts.downloadOnly && opts.uploadOnly {
		return opts, fmt.Errorf("--download 和 --upload 不能同时使用")
	}

	return opts, nil
}

func testDownload(ctx context.Context, size int64) (int64, error) {
	// Use Cloudflare's speed test endpoint
	url := fmt.Sprintf("https://speed.cloudflare.com/__down?bytes=%d", size)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, err
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return 0, err
	}

	elapsed := time.Since(start).Seconds()
	if elapsed == 0 {
		elapsed = 0.001
	}
	return int64(float64(n) / elapsed), nil
}

func testUpload(ctx context.Context, size int64) (int64, error) {
	// Use httpbin.org for upload test
	url := "https://httpbin.org/post"

	// Create a reader that generates data on the fly
	data := make([]byte, size)
	for i := range data {
		data[i] = 'A'
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(data)))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	elapsed := time.Since(start).Seconds()
	if elapsed == 0 {
		elapsed = 0.001
	}
	return int64(float64(size) / elapsed), nil
}
