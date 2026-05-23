//go:build linux
// +build linux

package main

import (
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

func getLinuxOptions() *linux.Options {
	return &linux.Options{
		IconData: nil, // Uses system default icon
	}
}
