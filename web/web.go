// Package web: embedded SPA + static assets.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed public
var publicFS embed.FS

// SPA serves the UI; unknown paths fall back to index.html (client routing).
// All asset references inside the HTML are relative, so the UI also works
// under a Home Assistant Ingress subpath without rewriting.
func SPA(next http.Handler) http.Handler {
	sub, err := fs.Sub(publicFS, "public")
	if err != nil {
		panic(err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(r.URL.Path, "/")
		if clean != "" && !strings.HasSuffix(clean, "/") {
			if data, err := fs.ReadFile(sub, clean); err == nil {
				w.Header().Set("Content-Type", contentType(clean))
				_, _ = w.Write(data)
				return
			}
		}
		if data, err := fs.ReadFile(sub, "index.html"); err == nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(data)
			return
		}
		if next != nil {
			next.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
}

func contentType(p string) string {
	switch {
	case strings.HasSuffix(p, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(p, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(p, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(p, ".svg"):
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}