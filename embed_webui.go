//go:build !wails

package main

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
//go:embed all:public
var webuiAssets embed.FS

var (
	webuiDistFS   = mustSubFS(webuiAssets, "dist")
	webuiPublicFS = mustSubFS(webuiAssets, "public")
)

func mustSubFS(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic("embed: missing directory " + dir + ": " + err.Error())
	}
	return sub
}

// serveEmbeddedAsset attempts to serve an asset from the embedded FS.
// It checks public/ first (overlay HTML, medals), then dist/ (frontend build).
// Returns true if the asset was found and served.
func serveEmbeddedAsset(w http.ResponseWriter, r *http.Request, assetPath string) bool {
	for _, fsys := range []http.FileSystem{http.FS(webuiPublicFS), http.FS(webuiDistFS)} {
		f, err := fsys.Open(assetPath)
		if err != nil {
			continue
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			continue
		}
		http.ServeContent(w, r, assetPath, stat.ModTime(), f)
		return true
	}
	return false
}

// webuiRootHandler returns an http.Handler that serves the embedded dist/
// frontend (Vue build output) with SPA fallback.
// For client-side routing paths, it serves index.html so the Vue app can
// determine the route client-side.
func webuiRootHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Explicit /mini path is handled separately; for anything else, serve
		// the exact asset or fall back to index.html (SPA).
		if r.URL.Path == "/mini" {
			serveMiniHTML(w, r)
			return
		}
		// Try to serve the exact asset from public/ or dist/
		if serveEmbeddedAsset(w, r, r.URL.Path) {
			return
		}
		// SPA fallback: serve index.html for client-side routes
		serveEmbeddedAsset(w, r, "/index.html")
	})
}

// serveMiniHTML serves dist/index.html with an injected script that mocks
// Wails' IsMiniMode() to return true. In Wails mode the mini window is a
// separate process with isMini=true; in WebUI mode we simulate this by
// monkey-patching window.go before the Vue app loads, so the app renders
// the mini floating-danmaku UI instead of the main control panel.
func serveMiniHTML(w http.ResponseWriter, r *http.Request) {
	content, err := fs.ReadFile(webuiDistFS, "index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusInternalServerError)
		return
	}
	// Place the injected <script> before the first <script type="module"
	// so it runs before the Vue app bootstrap.
	html := strings.Replace(string(content),
		`<script type="module"`,
		`<script>window.go={main:{App:{IsMiniMode:function(){return!0}}}}</script><script type="module"`,
		1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(html))
}
