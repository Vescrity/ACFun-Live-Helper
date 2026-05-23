//go:build wails

package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// ========== Wails 生命周期 ==========

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.cancel = func() {} // Wails manages lifecycle

	if err := a.setupLogFile(); err != nil {
		log.Printf("failed to setup log file: %v", err)
	}
	log.Printf("==== ACFun Live Helper started; os=%s arch=%s ====", runtime.GOOS, runtime.GOARCH)
	if err := a.startOverlayServer(); err != nil {
		log.Printf("failed to start danmaku overlay server: %v", err)
	}
	if err := a.startBackend(); err != nil {
		log.Printf("failed to start embedded acfunlive-backend: %v", err)
	}

	a.stopStatsChan = make(chan struct{})
	go a.trackSystemStats()

	// 仅悬浮 mini 进程注册全局热键（默认 Ctrl+Alt+Shift+G），用于切换鼠标穿透模式。
	if a.isMini {
		startGlobalHotkey(uintptr(modCtrl|modAlt|modShift), uintptr(vkG), func() {
			wailsRuntime.EventsEmit(a.ctx, "mini:click-through-toggle")
		})
	}
}

func (a *App) shutdown(ctx context.Context) {
	if a.stopStatsChan != nil {
		close(a.stopStatsChan)
	}
	a.closeMiniWindow()
	a.stopOverlayServer(ctx)
	a.closeLogFile()
}

// ========== Wails UI 操作 ==========

func (a *App) CopyText(text string) error {
	if a.ctx == nil {
		return errors.New("application is not ready")
	}
	return wailsRuntime.ClipboardSetText(a.ctx, text)
}

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

func (a *App) SetAlwaysOnTop(enabled bool) {
	wailsRuntime.WindowSetAlwaysOnTop(a.ctx, enabled)
}

func (a *App) SetWindowSize(width, height int) {
	wailsRuntime.WindowSetSize(a.ctx, width, height)
}

func (a *App) IsMiniMode() bool {
	return a.isMini
}

// ========== Wails 文件选择框 ==========

func (a *App) OpenCoverFile() (string, error) {
	if a.ctx == nil {
		return "", errors.New("application is not ready")
	}

	return wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "选择直播封面",
		Filters: []wailsRuntime.FileFilter{
			{
				DisplayName: "图片文件 (*.jpg;*.jpeg;*.png;*.webp;*.gif)",
				Pattern:     "*.jpg;*.jpeg;*.png;*.webp;*.gif",
			},
		},
	})
}

// ========== Wails 录播下载（弹出保存对话框） ==========

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
	resp, err := playbackGet(a.ctx, client, target)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	contentType := resp.Header.Get("Content-Type")
	peek, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}
	isM3U8 := strings.Contains(strings.ToLower(contentType), "mpegurl") ||
		bytes.HasPrefix(bytes.TrimSpace(peek), []byte("#EXTM3U")) ||
		looksLikeM3U8URL(target)
	if isM3U8 {
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

// ========== Wails Mini 窗口管理 ==========

var miniCmd *exec.Cmd

func (a *App) closeMiniWindow() {
	if miniCmd == nil || miniCmd.Process == nil {
		return
	}
	if err := miniCmd.Process.Kill(); err != nil {
		log.Printf("[Wails] failed to close mini window process: %v", err)
		return
	}
	log.Printf("[Wails] Mini window process killed on main shutdown")
}

func (a *App) LaunchMiniWindow() error {
	if miniCmd != nil {
		log.Printf("[Wails] Mini window is already running, skip launching duplicate instance")
		return nil
	}

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

	miniCmd = cmd

	go func() {
		_ = cmd.Wait()
		miniCmd = nil
		log.Printf("[Wails] Mini window process exited")
	}()

	return nil
}

// ========== Wails 鼠标穿透 & 全局热键 ==========

func (a *App) SetMouseClickThrough(enable bool) error {
	if !a.isMini {
		return nil
	}
	hwnd := findOwnVisibleHWND()
	if hwnd == 0 {
		return errors.New("mini window handle not found")
	}
	return applyMouseClickThrough(hwnd, enable)
}

func (a *App) SetMouseClickThroughHotkey(mods uint32, vk uint32) error {
	if !a.isMini {
		return nil
	}
	if vk == 0 {
		return errors.New("invalid virtual key code")
	}
	return updateGlobalHotkey(uintptr(mods), uintptr(vk))
}
