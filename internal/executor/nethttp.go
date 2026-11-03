package executor

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// runNetHTTP implements `kopsc net http`: start a simple HTTP file server.
func runNetHTTP(ctx context.Context, req BuiltinRequest) error {
	opts, err := parseNetHTTPArgs(req.Args)
	if err != nil {
		return err
	}

	dir := opts.dir
	if dir == "" {
		dir = "."
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("无法解析路径 %q: %w", dir, err)
	}

	if info, err := os.Stat(absDir); err != nil {
		return fmt.Errorf("目录 %q 不存在: %w", dir, err)
	} else if !info.IsDir() {
		return fmt.Errorf("%q 不是目录", dir)
	}

	port := opts.port
	addr := fmt.Sprintf(":%d", port)

	// Check if port is available
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("端口 %d 已被占用: %w", port, err)
	}

	// Get local IP addresses
	ips := getLocalIPs()

	fmt.Fprintf(req.Stdout, "=== HTTP 文件服务器 ===\n\n")
	fmt.Fprintf(req.Stdout, "目录: %s\n", absDir)
	fmt.Fprintf(req.Stdout, "地址:\n")
	fmt.Fprintf(req.Stdout, "  http://localhost:%d\n", port)
	for _, ip := range ips {
		fmt.Fprintf(req.Stdout, "  http://%s:%d\n", ip, port)
	}
	if opts.upload {
		fmt.Fprintf(req.Stdout, "\n上传功能已启用\n")
		fmt.Fprintf(req.Stdout, "POST /upload 可上传文件\n")
	}
	fmt.Fprintf(req.Stdout, "\n按 Ctrl+C 停止\n")

	mux := http.NewServeMux()

	if opts.upload {
		mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "请使用 POST 方法上传", http.StatusMethodNotAllowed)
				return
			}
			handleUpload(w, r, absDir, req)
		})
	}

	// File server with directory listing
	fileServer := http.FileServer(http.Dir(absDir))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Log request
		fmt.Fprintf(req.Stdout, "[%s] %s %s\n", time.Now().Format("15:04:05"), r.Method, r.URL.Path)
		fileServer.ServeHTTP(w, r)
	}))

	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// Handle context cancellation
	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

type netHTTPOptions struct {
	dir    string
	port   int
	upload bool
}

func parseNetHTTPArgs(args []string) (netHTTPOptions, error) {
	opts := netHTTPOptions{
		port: 8000, // default port
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			// Ignore everything after --
			return opts, nil
		case arg == "--dir" || arg == "-d":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少目录参数", arg)
			}
			i++
			opts.dir = args[i]
		case strings.HasPrefix(arg, "--dir="):
			opts.dir = strings.TrimPrefix(arg, "--dir=")
		case arg == "--port" || arg == "-p":
			if i+1 >= len(args) {
				return opts, fmt.Errorf("%s 缺少端口参数", arg)
			}
			i++
			port, err := strconv.Atoi(args[i])
			if err != nil || port < 1 || port > 65535 {
				return opts, fmt.Errorf("无效的端口号: %q", args[i])
			}
			opts.port = port
		case strings.HasPrefix(arg, "--port="):
			portStr := strings.TrimPrefix(arg, "--port=")
			port, err := strconv.Atoi(portStr)
			if err != nil || port < 1 || port > 65535 {
				return opts, fmt.Errorf("无效的端口号: %q", portStr)
			}
			opts.port = port
		case arg == "--upload" || arg == "-u":
			opts.upload = true
		case strings.HasPrefix(arg, "-") && arg != "-":
			return opts, fmt.Errorf("未知选项 %q", arg)
		default:
			// Treat as directory
			opts.dir = arg
		}
	}

	return opts, nil
}

func getLocalIPs() []string {
	var ips []string

	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}

	for _, iface := range ifaces {
		// Skip down interfaces and loopback
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() {
				continue
			}

			// Only IPv4 for simplicity
			if ip.To4() != nil {
				ips = append(ips, ip.String())
			}
		}
	}

	return ips
}

func handleUpload(w http.ResponseWriter, r *http.Request, dir string, req BuiltinRequest) {
	// Parse multipart form (max 100MB)
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		http.Error(w, fmt.Sprintf("解析表单失败: %v", err), http.StatusBadRequest)
		return
	}

	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		http.Error(w, "没有选择文件", http.StatusBadRequest)
		return
	}

	uploaded := 0
	for _, fileHeader := range files {
		file, err := fileHeader.Open()
		if err != nil {
			http.Error(w, fmt.Sprintf("打开文件失败: %v", err), http.StatusInternalServerError)
			return
		}
		defer file.Close()

		// Sanitize filename
		filename := filepath.Base(fileHeader.Filename)
		destPath := filepath.Join(dir, filename)

		// Check if file exists
		if _, err := os.Stat(destPath); err == nil {
			// Add suffix to avoid overwrite
			ext := filepath.Ext(filename)
			base := strings.TrimSuffix(filename, ext)
			for i := 1; ; i++ {
				newName := fmt.Sprintf("%s_%d%s", base, i, ext)
				destPath = filepath.Join(dir, newName)
				if _, err := os.Stat(destPath); os.IsNotExist(err) {
					break
				}
			}
		}

		dest, err := os.Create(destPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("创建文件失败: %v", err), http.StatusInternalServerError)
			return
		}

		if _, err := io.Copy(dest, file); err != nil {
			dest.Close()
			http.Error(w, fmt.Sprintf("写入文件失败: %v", err), http.StatusInternalServerError)
			return
		}
		dest.Close()

		fmt.Fprintf(req.Stdout, "[上传] %s -> %s\n", fileHeader.Filename, destPath)
		uploaded++
	}

	// Set content type based on Accept header
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "uploaded": %d}`, uploaded)
	} else {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<html><body><h2>上传成功</h2><p>已上传 %d 个文件</p><a href='/'>返回</a></body></html>", uploaded)
	}
}

// detectContentType tries to detect the MIME type of a file
func detectContentType(path string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return "application/octet-stream"
	}

	mimeType := mime.TypeByExtension(ext)
	if mimeType != "" {
		return mimeType
	}

	return "application/octet-stream"
}
