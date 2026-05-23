package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"aclivehelper/backend"
)

type App struct {
	ctx           context.Context
	cancel        context.CancelFunc
	backendPort   int
	overlayServer *http.Server
	overlayURL    string
	// overlay 样式实时同步：BroadcastOverlayStyle 把最新样式 JSON 缓存到 overlayStyle，
	// 同时通过 SSE 推送给所有连入 /events 的 overlay 客户端，避免 OBS 必须手动刷新浏览器源。
	overlayStyleMu   sync.RWMutex
	overlayStyle     string
	overlayClientsMu sync.Mutex
	overlayClients   map[*overlaySSEClient]struct{}
	logFile          *os.File
	logPath          string
	logMu            sync.Mutex
	sysStatsMu       sync.Mutex
	cpuPercent       float64
	memPercent       float64
	stopStatsChan    chan struct{}
	isMini           bool
}

type overlaySSEClient struct {
	ch chan string
}

func NewApp(isMini bool) *App {
	ctx, cancel := context.WithCancel(context.Background())
	return &App{
		ctx:         ctx,
		cancel:      cancel,
		backendPort: envInt("ACLIVE_BACKEND_PORT", backend.DefaultPort),
		isMini:      isMini,
	}
}

// ========== 共享业务逻辑：日志 ==========

func (a *App) setupLogFile() error {
	a.logMu.Lock()
	defer a.logMu.Unlock()

	if a.logFile != nil {
		return nil
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	logDir := filepath.Join(dir, "ACFun Live Helper", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	a.logPath = filepath.Join(logDir, "app.log")
	f, err := os.OpenFile(a.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	a.logFile = f
	log.SetOutput(io.MultiWriter(f, os.Stderr))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	return nil
}

func (a *App) closeLogFile() {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	if a.logFile != nil {
		_ = a.logFile.Close()
		a.logFile = nil
	}
}

// GetLogPath 返回日志文件绝对路径。
func (a *App) GetLogPath() string {
	if a.logPath == "" {
		if err := a.setupLogFile(); err != nil {
			log.Printf("failed to setup log file: %v", err)
		}
	}
	return a.logPath
}

// AppendLog 写一行前端日志到同一个日志文件。
func (a *App) AppendLog(message string) error {
	text := strings.TrimSpace(message)
	if text == "" {
		return nil
	}
	if a.logPath == "" || a.logFile == nil {
		if err := a.setupLogFile(); err != nil {
			return err
		}
	}
	log.Printf("[frontend] %s", text)
	return nil
}

// OpenLogFolder 在文件管理器中打开日志文件夹。
func (a *App) OpenLogFolder() error {
	if a.logPath == "" {
		return errors.New("log file is not available")
	}
	folder := filepath.Dir(a.logPath)
	if runtime.GOOS == "windows" {
		return exec.Command("explorer", folder).Start()
	}
	if runtime.GOOS == "darwin" {
		return exec.Command("open", folder).Start()
	}
	return exec.Command("xdg-open", folder).Start()
}

// ========== 共享业务逻辑：主题 & 悬浮窗状态 ==========

func (a *App) SetSharedTheme(theme string) error {
	value := "light"
	if theme == "dark" {
		value = "dark"
	}
	return os.WriteFile(sharedThemePath(), []byte(value), 0o644)
}

func (a *App) GetSharedTheme() string {
	data, err := os.ReadFile(sharedThemePath())
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(data))
	if value != "dark" && value != "light" {
		return ""
	}
	return value
}

func (a *App) SetSharedFloatState(payload string) error {
	return os.WriteFile(sharedFloatStatePath(), []byte(payload), 0o600)
}

func (a *App) GetSharedFloatState() string {
	data, err := os.ReadFile(sharedFloatStatePath())
	if err != nil {
		return ""
	}
	return string(data)
}

func sharedThemePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "aclivehelper-theme")
	}
	path := filepath.Join(dir, "ACFun Live Helper")
	_ = os.MkdirAll(path, 0o755)
	return filepath.Join(path, "theme")
}

func sharedFloatStatePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "aclivehelper-float-state.json")
	}
	path := filepath.Join(dir, "ACFun Live Helper")
	_ = os.MkdirAll(path, 0o755)
	return filepath.Join(path, "float-state.json")
}

// ========== 共享业务逻辑：封面图 ==========

func (a *App) ReadCoverFile(filePath string) (string, error) {
	resolvedPath, err := filepath.Abs(strings.TrimSpace(filePath))
	if err != nil {
		return "", err
	}
	if resolvedPath == "" {
		return "", nil
	}

	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	mimeType := mime.TypeByExtension(strings.ToLower(filepath.Ext(resolvedPath)))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(content)), nil
}

func (a *App) SaveCoverImage(dataURL string) (string, error) {
	mimeType, payload, ok := strings.Cut(strings.TrimSpace(dataURL), ";base64,")
	if !ok || !strings.HasPrefix(mimeType, "data:image/") {
		return "", errors.New("invalid cover image data")
	}

	content, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", err
	}

	ext := imageExtension(strings.TrimPrefix(mimeType, "data:"))
	userConfigDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	coverDir := filepath.Join(userConfigDir, "ACFun Live Helper", "covers")
	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		return "", err
	}

	filePath := filepath.Join(coverDir, fmt.Sprintf("cover-%d%s", time.Now().UnixMilli(), ext))
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		return "", err
	}

	return filePath, nil
}

// ========== 共享业务逻辑：后端管理 ==========

func (a *App) GetBackendPort() int {
	return a.backendPort
}

func (a *App) startBackend() error {
	if isPortOpen(a.backendPort) {
		log.Printf("acfunlive-backend port %d already in use; skipping embedded start", a.backendPort)
		return nil
	}

	opts := backend.Options{
		Port:   a.backendPort,
		Debug:  true,
		TCP:    envBool("ACLIVE_BACKEND_TCP"),
		LogAll: envBool("ACLIVE_BACKEND_LOGALL"),
	}
	if err := backend.Start(opts); err != nil {
		return err
	}

	for index := 0; index < 20; index++ {
		if isPortOpen(a.backendPort) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

// ========== 共享业务逻辑：Overlay 弹幕服务器（SSE） ==========

const overlayPreferredPort = 15370

func (a *App) GetOverlayBaseUrl() (string, error) {
	if a.overlayURL == "" {
		if err := a.startOverlayServer(); err != nil {
			return "", err
		}
	}
	return a.overlayURL + "/danmaku-overlay.html", nil
}

func (a *App) startOverlayServer() error {
	if a.overlayServer != nil && a.overlayURL != "" {
		return nil
	}

	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(overlayPreferredPort)))
	if err != nil {
		log.Printf("danmaku overlay preferred port %d unavailable, fallback to random: %v", overlayPreferredPort, err)
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return err
		}
	}

	a.overlayClientsMu.Lock()
	if a.overlayClients == nil {
		a.overlayClients = make(map[*overlaySSEClient]struct{})
	}
	a.overlayClientsMu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/events", a.serveOverlayEvents)
	mux.HandleFunc("/style.json", a.serveOverlayStyle)
	mux.HandleFunc("/", a.serveOverlayAsset)

	a.overlayURL = "http://" + listener.Addr().String()
	a.overlayServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := a.overlayServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("danmaku overlay server stopped: %v", err)
		}
	}()

	return nil
}

func (a *App) stopOverlayServer(parent context.Context) {
	if a.overlayServer == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	if err := a.overlayServer.Shutdown(ctx); err != nil {
		log.Printf("failed to stop danmaku overlay server: %v", err)
	}
	a.overlayServer = nil
	a.overlayURL = ""
}

func (a *App) serveOverlayEvents(response http.ResponseWriter, request *http.Request) {
	flusher, ok := response.(http.Flusher)
	if !ok {
		http.Error(response, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	response.Header().Set("Content-Type", "text/event-stream")
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("Connection", "keep-alive")
	response.Header().Set("X-Accel-Buffering", "no")
	response.Header().Set("Access-Control-Allow-Origin", "*")

	client := &overlaySSEClient{ch: make(chan string, 8)}

	a.overlayClientsMu.Lock()
	if a.overlayClients == nil {
		a.overlayClients = make(map[*overlaySSEClient]struct{})
	}
	a.overlayClients[client] = struct{}{}
	a.overlayClientsMu.Unlock()

	defer func() {
		a.overlayClientsMu.Lock()
		delete(a.overlayClients, client)
		a.overlayClientsMu.Unlock()
	}()

	a.overlayStyleMu.RLock()
	current := a.overlayStyle
	a.overlayStyleMu.RUnlock()
	if current != "" {
		writeSSE(response, "style", current)
		flusher.Flush()
	}

	ctx := request.Context()
	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-client.ch:
			if !ok {
				return
			}
			writeSSE(response, "style", msg)
			flusher.Flush()
		case <-keepalive.C:
			_, _ = io.WriteString(response, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func writeSSE(w io.Writer, event, payload string) {
	cleaned := strings.ReplaceAll(payload, "\r", "")
	cleaned = strings.ReplaceAll(cleaned, "\n", " ")
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, cleaned)
}

func (a *App) serveOverlayStyle(response http.ResponseWriter, request *http.Request) {
	a.overlayStyleMu.RLock()
	payload := a.overlayStyle
	a.overlayStyleMu.RUnlock()
	response.Header().Set("Content-Type", "application/json")
	response.Header().Set("Cache-Control", "no-store")
	if payload == "" {
		_, _ = response.Write([]byte("{}"))
		return
	}
	_, _ = response.Write([]byte(payload))
}

// BroadcastOverlayStyle 由前端在样式变化时调用：缓存最新样式 JSON 字符串并推给所有 overlay 客户端，
// 让 OBS 浏览器源无需手动刷新即可应用新设置。
func (a *App) BroadcastOverlayStyle(payload string) error {
	if payload == "" {
		return nil
	}
	a.overlayStyleMu.Lock()
	a.overlayStyle = payload
	a.overlayStyleMu.Unlock()

	a.overlayClientsMu.Lock()
	defer a.overlayClientsMu.Unlock()
	for c := range a.overlayClients {
		select {
		case c.ch <- payload:
		default:
		}
	}
	return nil
}

func (a *App) serveOverlayAsset(response http.ResponseWriter, request *http.Request) {
	assetPath := strings.TrimPrefix(path.Clean("/"+request.URL.Path), "/")
	if assetPath == "" {
		assetPath = "danmaku-overlay.html"
	}

	response.Header().Set("Cache-Control", "no-store")

	if served := serveDiskAsset(response, request, filepath.Join("public", assetPath)); served {
		return
	}
	if served := serveDiskAsset(response, request, filepath.Join("dist", assetPath)); served {
		return
	}

	http.NotFound(response, request)
}

func serveDiskAsset(response http.ResponseWriter, request *http.Request, filePath string) bool {
	resolvedPath, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}
	info, err := os.Stat(resolvedPath)
	if err != nil || info.IsDir() {
		return false
	}
	http.ServeFile(response, request, resolvedPath)
	return true
}

// ========== 共享业务逻辑：系统信息 ==========

type SystemStatsResult struct {
	CPU    float64 `json:"cpu"`
	Memory float64 `json:"memory"`
}

func (a *App) GetSystemStats() SystemStatsResult {
	a.sysStatsMu.Lock()
	defer a.sysStatsMu.Unlock()
	return SystemStatsResult{
		CPU:    a.cpuPercent,
		Memory: a.memPercent,
	}
}

func (a *App) GetNetworkDelay() int {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", "live.acfun.cn:443", 1500*time.Millisecond)
	if err != nil {
		conn, err = net.DialTimeout("tcp", "www.acfun.cn:80", 1500*time.Millisecond)
		if err != nil {
			return -1
		}
	}
	defer conn.Close()
	return int(time.Since(start).Milliseconds())
}

func (a *App) trackSystemStats() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	state := newSysStatsState()

	for {
		select {
		case <-a.stopStatsChan:
			return
		case <-ticker.C:
			cpu, mem := state.collect()
			a.sysStatsMu.Lock()
			a.cpuPercent = cpu
			a.memPercent = mem
			a.sysStatsMu.Unlock()
		}
	}
}

func (a *App) GetSystemFonts() []string {
	if runtime.GOOS == "windows" {
		return getWindowsFonts()
	}
	return getLinuxFonts()
}

// ========== 共享业务逻辑：录播下载辅助 ==========

func newPlaybackHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			ResponseHeaderTimeout: 60 * time.Second,
		},
	}
}

func playbackGet(ctx context.Context, client *http.Client, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (ACFunLiveHelper)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("远程返回 HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func parseM3U8Segments(body []byte, base *url.URL) ([]string, error) {
	var segments []string
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		u, err := url.Parse(line)
		if err != nil {
			return nil, err
		}
		abs := base.ResolveReference(u).String()
		segments = append(segments, abs)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return segments, nil
}

func looksLikeM3U8URL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return strings.HasSuffix(strings.ToLower(parsed.Path), ".m3u8")
}

func swapPlaybackExt(name, newExt string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return name + newExt
	}
	return strings.TrimSuffix(name, ext) + newExt
}

func sanitizePlaybackFileName(name string) string {
	const forbidden = "<>:\"/\\|?*"
	mapped := strings.Map(func(r rune) rune {
		if r < 0x20 {
			return '_'
		}
		if strings.ContainsRune(forbidden, r) {
			return '_'
		}
		return r
	}, name)
	mapped = strings.TrimSpace(mapped)
	if mapped == "" {
		mapped = "playback.mp4"
	}
	if len([]rune(mapped)) > 120 {
		runes := []rune(mapped)
		mapped = string(runes[:120])
	}
	return mapped
}

// ========== 工具函数 ==========

func isPortOpen(port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func imageExtension(mimeType string) string {
	switch strings.ToLower(mimeType) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}

func contentType(assetPath string) string {
	mimeType := mime.TypeByExtension(filepath.Ext(assetPath))
	if mimeType != "" {
		return mimeType
	}
	return "application/octet-stream"
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name)))
	if err != nil {
		return fallback
	}
	return value
}

func envBool(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return value == "1" || value == "true" || value == "yes"
}
