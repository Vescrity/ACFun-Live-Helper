package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:dist
var assets embed.FS

func main() {
	isMini := consumeMiniLaunchToken()

	if isMini && runtime.GOOS == "windows" {
		go watchParentProcess()
	}

	app := NewApp(isMini)

	appTitle := "AcFun Live Helper"
	appWidth := 1320
	appHeight := 860
	appMinWidth := 360
	appMinHeight := 640
	alwaysOnTop := false

	if isMini {
		appTitle = "AcFun Live Helper - 桌面悬浮弹幕"
		appWidth = 360
		appHeight = 580
		appMinWidth = 240
		appMinHeight = 320
		alwaysOnTop = true
	}

	var bgColour *options.RGBA
	if isMini {
		bgColour = &options.RGBA{R: 0, G: 0, B: 0, A: 0}
	} else {
		bgColour = &options.RGBA{R: 248, G: 243, B: 245, A: 255}
	}
	webviewDataPath := sharedWebviewDataPath("main")
	if isMini {
		webviewDataPath = sharedWebviewDataPath("mini")
	}

	appOptions := &options.App{
		Title:            appTitle,
		Width:            appWidth,
		Height:           appHeight,
		MinWidth:         appMinWidth,
		MinHeight:        appMinHeight,
		AlwaysOnTop:      alwaysOnTop,
		Frameless:        true,
		BackgroundColour: bgColour,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		EnableDefaultContextMenu: true,
	}

	// Platform-specific configuration
	if runtime.GOOS == "windows" {
		appOptions.Windows = getWindowsOptions(webviewDataPath, isMini)
	} else if runtime.GOOS == "linux" {
		appOptions.Linux = getLinuxOptions()
	} else if runtime.GOOS == "darwin" {
		appOptions.Mac = getMacOptions()
	}

	err := wails.Run(appOptions)
	if err != nil {
		log.Fatal(err)
	}
}

func sharedWebviewDataPath(profile string) string {
	var configDir string
	var err error

	if runtime.GOOS == "linux" {
		// Use XDG_CONFIG_HOME on Linux, fallback to ~/.config
		configDir = os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return ""
			}
			configDir = filepath.Join(home, ".config")
		}
	} else {
		// Windows and macOS use standard config directory
		configDir, err = os.UserConfigDir()
		if err != nil {
			return ""
		}
	}

	path := filepath.Join(configDir, "ACFun Live Helper", "webview", profile)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return ""
	}
	return path
}

func consumeMiniLaunchToken() bool {
	if os.Getenv("ACLIVE_MINI_WINDOW") != "1" {
		return false
	}

	token := os.Getenv("ACLIVE_MINI_TOKEN")
	tokenFile := os.Getenv("ACLIVE_MINI_TOKEN_FILE")
	if token == "" || tokenFile == "" {
		return false
	}

	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return false
	}
	_ = os.Remove(tokenFile)
	if string(data) != token {
		return false
	}

	index := strings.LastIndex(token, "-")
	if index < 0 {
		return false
	}
	createdAt, err := strconv.ParseInt(token[index+1:], 10, 64)
	if err != nil {
		return false
	}
	age := time.Since(time.Unix(0, createdAt))
	return age >= 0 && age <= 2*time.Minute
}
