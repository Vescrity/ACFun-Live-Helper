//go:build !wails

package main

import (
	"embed"
	"io/fs"
	"net/http"
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
// frontend (Vue build output).
func webuiRootHandler() http.Handler {
	return http.FileServer(http.FS(webuiDistFS))
}
