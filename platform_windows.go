//go:build windows

package main

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

func setPlatformOptions(appOptions *options.App, webviewDataPath string, isMini bool) {
	appOptions.Windows = &windows.Options{
		WebviewUserDataPath:              webviewDataPath,
		WebviewGpuIsDisabled:             !isMini,
		Theme:                            windows.SystemDefault,
		WebviewIsTransparent:             isMini,
		WindowIsTranslucent:              isMini,
		BackdropType:                     windows.None,
		DisableFramelessWindowDecorations: isMini,
	}
}
