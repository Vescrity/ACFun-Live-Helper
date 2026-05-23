//go:build wails

package main

import (
	"net/http"
)

// serveEmbeddedAsset is a no-op in Wails mode; the overlay server reads
// from disk (public/ and dist/) as before.
func serveEmbeddedAsset(_ http.ResponseWriter, _ *http.Request, _ string) bool {
	return false
}
