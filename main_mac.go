//go:build darwin
// +build darwin

package main

import (
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

func getMacOptions() *mac.Options {
	return &mac.Options{
		TitleBar: mac.TitleBarDefault(),
	}
}
