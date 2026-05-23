//go:build !wails

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

func main() {
	port := flag.Int("port", 15369, "HTTP server port")
	flag.Parse()

	app := NewApp(false)

	if err := app.setupLogFile(); err != nil {
		log.Printf("failed to setup log file: %v", err)
	}
	log.Printf("==== ACFun Live Helper (WebUI) started; os=%s arch=%s ====", runtime.GOOS, runtime.GOARCH)

	if err := app.startOverlayServer(); err != nil {
		log.Printf("failed to start danmaku overlay server: %v", err)
	}
	if err := app.startBackend(); err != nil {
		log.Printf("failed to start embedded acfunlive-backend: %v", err)
	}

	app.stopStatsChan = make(chan struct{})
	go app.trackSystemStats()

	// HTTP 服务：前端静态文件 + API
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(*port)))
	if err != nil {
		log.Printf("port %d unavailable, fallback to random: %v", *port, err)
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			log.Fatal(err)
		}
	}

	mux := http.NewServeMux()
	registerWebUIHandlers(mux, app)
	// Serve embedded dist/ frontend (built via npm run build, embedded at compile time).
	// No runtime dependency on disk directories.
	mux.Handle("/", webuiRootHandler())

	addr := listener.Addr().String()
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("WebUI started at http://%s", addr)
	openBrowser(fmt.Sprintf("http://%s", addr))

	// 优雅退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Shutting down...")
		app.cancel()
		if app.stopStatsChan != nil {
			close(app.stopStatsChan)
		}
		app.stopOverlayServer(context.Background())
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
		app.closeLogFile()
	}()

	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		// Linux: try xdg-open, then sensible-browser, then www-browser
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("failed to open browser: %v; please open %s manually", err, url)
	}
}

// sharedWebviewDataPath is only needed by Wails mode.
func sharedWebviewDataPath(profile string) string {
	return ""
}

// consumeMiniLaunchToken is only needed by Wails mini process.
func consumeMiniLaunchToken() bool {
	return false
}
