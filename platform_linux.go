//go:build linux

package main

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

func setPlatformOptions(appOptions *options.App, webviewDataPath string, isMini bool) {
	_ = webviewDataPath
	appOptions.Linux = &linux.Options{
		WindowIsTranslucent: isMini,
		// Enable GPU acceleration by default on Linux.
		// Set to WebviewGpuPolicyNever to disable if causing issues.
		WebviewGpuPolicy: linux.WebviewGpuPolicyAlways,
	}
}
