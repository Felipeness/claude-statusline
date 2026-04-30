package main

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
)

//go:embed all:web/dist
var webDist embed.FS

var webStatic http.Handler = func() http.Handler {
	sub, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		return nil
	}
	if entries, err := fs.ReadDir(sub, "."); err != nil || len(entries) == 0 {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "frontend não buildado — rode `cd web && bun run build` antes de `go build`", http.StatusServiceUnavailable)
		})
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hasExt(r.URL.Path) {
			fileServer.ServeHTTP(w, r)
			return
		}
		f, err := sub.Open("index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.Copy(w, f)
	})
}()

func hasExt(path string) bool {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return true
		}
		if path[i] == '/' {
			return false
		}
	}
	return false
}
