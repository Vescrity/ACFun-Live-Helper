//go:build !wails

package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
)

func registerWebUIHandlers(mux *http.ServeMux, app *App) {
	mux.HandleFunc("/api/stats", jsonHandler(func() any { return app.GetSystemStats() }))
	mux.HandleFunc("/api/delay", jsonHandler(func() any { return map[string]int{"delay": app.GetNetworkDelay()} }))
	mux.HandleFunc("/api/fonts", jsonHandler(func() any { return app.GetSystemFonts() }))
	mux.HandleFunc("/api/backend-port", jsonHandler(func() any { return map[string]int{"port": app.GetBackendPort()} }))
	mux.HandleFunc("/api/overlay-url", jsonHandler(func() any { return jsonOrError(app.GetOverlayBaseUrl()) }))

	mux.HandleFunc("/api/state", stateHandler(app))

	mux.HandleFunc("/api/log-path", jsonHandler(func() any { return map[string]string{"path": app.GetLogPath()} }))
	mux.HandleFunc("/api/theme", themeHandler(app))
	mux.HandleFunc("/api/float-state", floatStateHandler(app))

	mux.HandleFunc("/api/copy-text", methodHandler("POST", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		if runtime.GOOS == "linux" {
			// 尝试 xclip；回退到什么都不做（浏览器端已 fallback 到 clipboard API）
			cmd := exec.Command("xclip", "-selection", "clipboard")
			cmd.Stdin = strings.NewReader(string(body))
			_ = cmd.Run()
		}
		w.WriteHeader(http.StatusOK)
	}))

	mux.HandleFunc("/api/open-url", methodHandler("POST", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	mux.HandleFunc("/api/log", methodHandler("POST", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = app.AppendLog(string(body))
		w.WriteHeader(http.StatusOK)
	}))

	mux.HandleFunc("/api/log-folder", methodHandler("POST", func(w http.ResponseWriter, r *http.Request) {
		_ = app.OpenLogFolder()
		w.WriteHeader(http.StatusOK)
	}))

	mux.HandleFunc("/api/overlay-style", methodHandler("POST", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = app.BroadcastOverlayStyle(string(body))
		w.WriteHeader(http.StatusOK)
	}))

	mux.HandleFunc("/api/launch-mini", methodHandler("POST", func(w http.ResponseWriter, r *http.Request) {
		// WebUI 模式下前端直接 window.open('/mini')，不再经过后端。
		// 保留此端点仅用于兼容旧版前端。
		log.Printf("[WebUI] Launch-mini requested (frontend now opens directly)")
		w.WriteHeader(http.StatusOK)
	}))

	mux.HandleFunc("/api/cover/save", methodHandler("POST", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		path, err := app.SaveCoverImage(string(body))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]string{"path": path})
	}))

	mux.HandleFunc("/api/cover/read", methodHandler("GET", func(w http.ResponseWriter, r *http.Request) {
		filePath := r.URL.Query().Get("path")
		dataURL, err := app.ReadCoverFile(filePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]string{"dataUrl": dataURL})
	}))
}

// ========== 辅助 ==========

func jsonHandler(fn func() any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, fn())
	}
}

func jsonOrError(val any, err error) any {
	if err != nil {
		return map[string]string{"error": err.Error()}
	}
	return val
}

func writeJSON(w http.ResponseWriter, val any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(val)
}

func methodHandler(method string, fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		fn(w, r)
	}
}

func themeHandler(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			writeJSON(w, map[string]string{"theme": app.GetSharedTheme()})
		case "POST":
			body, _ := io.ReadAll(r.Body)
			_ = app.SetSharedTheme(string(body))
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func floatStateHandler(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			writeJSON(w, map[string]string{"state": app.GetSharedFloatState()})
		case "POST":
			body, _ := io.ReadAll(r.Body)
			_ = app.SetSharedFloatState(string(body))
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func stateHandler(app *App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			writeJSON(w, map[string]string{"state": app.GetSharedState()})
		case "POST":
			body, _ := io.ReadAll(r.Body)
			_ = app.SetSharedState(string(body))
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func openBrowserOnLinux(url string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("cmd", "/c", "start", url).Start()
	case "darwin":
		_ = exec.Command("open", url).Start()
	default:
		_ = exec.Command("xdg-open", url).Start()
	}
}
