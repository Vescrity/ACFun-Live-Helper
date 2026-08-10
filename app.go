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

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"aclivehelper/backend"
)

type App struct {
	ctx           context.Context
	cancel        context.CancelFunc
	backendPort   int
	overlayServer *http.Server
	overlayURL    string
	// overlay 实时同步：按 channel 缓存最新 JSON，并通过 SSE 推给对应 OBS 浏览器源。
	overlayPayloadsMu sync.RWMutex
	overlayPayloads   map[string]string
	overlayClientsMu  sync.Mutex
	overlayClients    map[*overlaySSEClient]struct{}
	logFile           *os.File
	logPath           string
	logMu             sync.Mutex
	sysStatsMu        sync.Mutex
	cpuPercent        float64
	memPercent        float64
	stopStatsChan     chan struct{}
	isMini            bool
	miniCmd           *exec.Cmd
}

type overlaySSEClient struct {
	channel string
	ch      chan string
}

func NewApp(isMini bool) *App {
	ctx, cancel := context.WithCancel(context.Background())
	return &App{
		ctx:           ctx,
		cancel:        cancel,
		backendPort:   envInt("ACLIVE_BACKEND_PORT", backend.DefaultPort),
		isMini:        isMini,
		stopStatsChan: make(chan struct{}),
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

func (a *App) SetSharedState(payload string) error {
	return os.WriteFile(sharedStatePath(), []byte(payload), 0o600)
}

func (a *App) GetSharedState() string {
	data, err := os.ReadFile(sharedStatePath())
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

func sharedStatePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "aclivehelper-state.json")
	}
	path := filepath.Join(dir, "ACFun Live Helper")
	_ = os.MkdirAll(path, 0o755)
	return filepath.Join(path, "state.json")
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

	// 封面文件是临时性的（传给直播 API 用），放到系统临时目录，不污染用户配置空间。
	coverDir := filepath.Join(os.TempDir(), "aclivehelper-covers")
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

func (a *App) OpenExternalURL(rawURL string) error {
	if a.ctx == nil {
		return errors.New("application is not ready")
	}
	target := strings.TrimSpace(rawURL)
	if target == "" {
		return errors.New("url is empty")
	}
	wailsRuntime.BrowserOpenURL(a.ctx, target)
	return nil
}

// DownloadPlaybackToFile 弹出保存对话框，让用户选择路径后流式下载远程录播文件到本地。
// AcFun 录播 URL 实际是 HLS m3u8 索引（几 KB 文本），需要解析后逐段下载 ts 拼接；
// 若拿到的不是 m3u8（比如直链 mp4/flv），就直接 io.Copy 写盘。
func (a *App) DownloadPlaybackToFile(rawURL string, suggestedName string) (string, error) {
	if a.ctx == nil {
		return "", errors.New("application is not ready")
	}
	target := strings.TrimSpace(rawURL)
	if target == "" {
		return "", errors.New("url is empty")
	}
	defaultName := strings.TrimSpace(suggestedName)
	if defaultName == "" {
		defaultName = "playback.ts"
	}
	defaultName = sanitizePlaybackFileName(defaultName)
	// AcFun 录播多为 HLS（拼接后得到 .ts），把预填名后缀改为 .ts，避免误导
	if looksLikeM3U8URL(target) {
		defaultName = swapPlaybackExt(defaultName, ".ts")
	}
	savePath, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "保存录播",
		DefaultFilename: defaultName,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "视频文件 (*.ts;*.mp4;*.flv)", Pattern: "*.ts;*.mp4;*.flv"},
			{DisplayName: "所有文件 (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return "", err
	}
	if savePath == "" {
		return "", nil
	}
	client := newPlaybackHTTPClient()
	// 先 HEAD 不一定可靠，直接 GET 原始 URL 看响应类型
	resp, err := playbackGet(a.ctx, client, target)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	contentType := resp.Header.Get("Content-Type")
	// 读前 8KB 用于判断是否 m3u8 文本
	peek, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}
	isM3U8 := strings.Contains(strings.ToLower(contentType), "mpegurl") ||
		bytes.HasPrefix(bytes.TrimSpace(peek), []byte("#EXTM3U")) ||
		looksLikeM3U8URL(target)
	if isM3U8 {
		// 把已读 peek + 剩余 body 拼回完整内容
		rest, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("读取 m3u8 索引失败: %w", err)
		}
		playlistBody := append([]byte{}, peek...)
		playlistBody = append(playlistBody, rest...)
		baseURL, err := url.Parse(resp.Request.URL.String())
		if err != nil {
			return "", fmt.Errorf("解析 m3u8 base url 失败: %w", err)
		}
		segments, err := parseM3U8Segments(playlistBody, baseURL)
		if err != nil {
			return "", fmt.Errorf("解析 m3u8 失败: %w", err)
		}
		if len(segments) == 0 {
			return "", errors.New("m3u8 索引内没有可下载的视频段")
		}
		out, err := os.Create(savePath)
		if err != nil {
			return "", fmt.Errorf("创建本地文件失败: %w", err)
		}
		writer := bufio.NewWriterSize(out, 1<<20)
		for index, segURL := range segments {
			if err := a.ctx.Err(); err != nil {
				out.Close()
				os.Remove(savePath)
				return "", fmt.Errorf("已取消: %w", err)
			}
			segResp, err := playbackGet(a.ctx, client, segURL)
			if err != nil {
				out.Close()
				os.Remove(savePath)
				return "", fmt.Errorf("下载第 %d/%d 段失败: %w", index+1, len(segments), err)
			}
			if _, copyErr := io.Copy(writer, segResp.Body); copyErr != nil {
				segResp.Body.Close()
				out.Close()
				os.Remove(savePath)
				return "", fmt.Errorf("写入第 %d/%d 段失败: %w", index+1, len(segments), copyErr)
			}
			segResp.Body.Close()
		}
		if err := writer.Flush(); err != nil {
			out.Close()
			os.Remove(savePath)
			return "", fmt.Errorf("刷新缓冲失败: %w", err)
		}
		if err := out.Close(); err != nil {
			return "", fmt.Errorf("关闭文件失败: %w", err)
		}
		return savePath, nil
	}
	// 非 m3u8：直接把 peek + 剩余 body 写入
	out, err := os.Create(savePath)
	if err != nil {
		return "", fmt.Errorf("创建本地文件失败: %w", err)
	}
	if _, err := out.Write(peek); err != nil {
		out.Close()
		os.Remove(savePath)
		return "", fmt.Errorf("写入本地文件失败: %w", err)
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		os.Remove(savePath)
		return "", fmt.Errorf("写入本地文件失败: %w", err)
	}
	if err := out.Close(); err != nil {
		return "", fmt.Errorf("关闭文件失败: %w", err)
	}
	return savePath, nil
}

// GetBackendPort returns the loopback port that the embedded acfunlive-backend
// WebSocket server listens on. Useful for the frontend store on first launch.
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

func (a *App) GetSongRequestOverlayUrl() (string, error) {
	if a.overlayURL == "" {
		if err := a.startOverlayServer(); err != nil {
			return "", err
		}
	}
	return a.overlayURL + "/song-request-overlay.html", nil
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

// serveOverlayEvents 升级请求为 Server-Sent Events 流，把对应 channel 的最新 payload 推给 overlay 客户端。
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

	channel := overlayChannel(request.URL.Query().Get("channel"))
	client := &overlaySSEClient{channel: channel, ch: make(chan string, 8)}

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

	a.overlayPayloadsMu.RLock()
	current := ""
	if a.overlayPayloads != nil {
		current = a.overlayPayloads[channel]
	}
	a.overlayPayloadsMu.RUnlock()
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

func overlayChannel(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "song":
		return "song"
	default:
		return "danmaku"
	}
}

func writeSSE(w io.Writer, event, payload string) {
	cleaned := strings.ReplaceAll(payload, "\r", "")
	cleaned = strings.ReplaceAll(cleaned, "\n", " ")
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, cleaned)
}

func (a *App) serveOverlayStyle(response http.ResponseWriter, request *http.Request) {
	channel := overlayChannel(request.URL.Query().Get("channel"))
	a.overlayPayloadsMu.RLock()
	payload := ""
	if a.overlayPayloads != nil {
		payload = a.overlayPayloads[channel]
	}
	a.overlayPayloadsMu.RUnlock()
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
	return a.broadcastOverlayPayload("danmaku", payload)
}

func (a *App) BroadcastSongRequestOverlay(payload string) error {
	return a.broadcastOverlayPayload("song", payload)
}

func (a *App) broadcastOverlayPayload(channel string, payload string) error {
	if payload == "" {
		return nil
	}
	channel = overlayChannel(channel)
	a.overlayPayloadsMu.Lock()
	if a.overlayPayloads == nil {
		a.overlayPayloads = make(map[string]string)
	}
	a.overlayPayloads[channel] = payload
	a.overlayPayloadsMu.Unlock()

	a.overlayClientsMu.Lock()
	defer a.overlayClientsMu.Unlock()
	for c := range a.overlayClients {
		if c.channel != channel {
			continue
		}
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

	// Try embedded assets first (WebUI mode — compiled into the binary)
	if serveEmbeddedAsset(response, request, assetPath) {
		return
	}

	// Fallback: disk directories (development or Wails mode)
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
	return getPlatformFonts()
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

// SetAlwaysOnTop 设置窗口置顶
func (a *App) SetAlwaysOnTop(enabled bool) {
	wailsRuntime.WindowSetAlwaysOnTop(a.ctx, enabled)
}

// SetWindowSize 设置窗口大小
func (a *App) SetWindowSize(width, height int) {
	wailsRuntime.WindowSetSize(a.ctx, width, height)
}

// IsMiniMode 返回当前是否是精简悬浮进程
func (a *App) IsMiniMode() bool {
	return a.isMini
}

// LaunchMiniWindow 原生拉起全新的带置顶的悬浮独立弹幕窗口进程
func (a *App) LaunchMiniWindow() error {
	a.sysStatsMu.Lock()
	if a.miniCmd != nil {
		a.sysStatsMu.Unlock()
		log.Printf("[Wails] Mini window is already running, skip launching duplicate instance")
		return nil
	}
	a.sysStatsMu.Unlock()

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	token := fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	tokenFile, err := os.CreateTemp("", "aclive-mini-*.token")
	if err != nil {
		return err
	}
	if _, err := tokenFile.WriteString(token); err != nil {
		_ = tokenFile.Close()
		_ = os.Remove(tokenFile.Name())
		return err
	}
	if err := tokenFile.Close(); err != nil {
		_ = os.Remove(tokenFile.Name())
		return err
	}

	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(),
		"ACLIVE_MINI_WINDOW=1",
		"ACLIVE_MINI_TOKEN="+token,
		"ACLIVE_MINI_TOKEN_FILE="+tokenFile.Name(),
		"ACLIVE_PARENT_PID="+strconv.Itoa(os.Getpid()),
	)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(tokenFile.Name())
		return err
	}

	a.sysStatsMu.Lock()
	a.miniCmd = cmd
	a.sysStatsMu.Unlock()

	// 开启进程生命周期监控协程，退出后自动重置为 nil
	go func() {
		_ = cmd.Wait()
		a.sysStatsMu.Lock()
		if a.miniCmd == cmd {
			a.miniCmd = nil
		}
		a.sysStatsMu.Unlock()
		log.Printf("[Wails] Mini window process exited")
	}()

	return nil
}
