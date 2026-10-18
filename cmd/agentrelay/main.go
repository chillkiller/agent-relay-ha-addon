// Command agentrelay: HTTP+WS server wiring store, runtime, api, web SPA.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"agentrelay/internal/adapter"
	"agentrelay/internal/api"
	"agentrelay/internal/runtime"
	"agentrelay/internal/store"
	webpkg "agentrelay/web"
)

func main() {
	port := envOr("PORT", "8099")
	host := envOr("HOST", "0.0.0.0")
	dataDir := envOr("RELAY_DATA_DIR", "")
	if dataDir == "" {
		// local dev default: home dir (SQLite needs real fcntl locks — SMB/NFS
		// mounts don't provide them). In the HA add-on, run.sh sets RELAY_DATA_DIR=/data.
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".agent-relay")
	}

	st, err := store.Open(dataDir)
	if err != nil {
		log.Fatalf("[relay] db: %v", err)
	}
	st.FailStaleStreaming()

	rt := runtime.New(st, nil)
	rt.Register(adapter.Echo{})
	rt.Register(adapter.OpenAI{})
	rt.Register(adapter.OpenClaw{})

	srv := api.New(st, rt)

	apiHandler := srv.Handler()
	spaHandler := webpkg.SPA(nil)

	root := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/ws" {
			apiHandler.ServeHTTP(w, r)
			return
		}
		spaHandler.ServeHTTP(w, r)
	})

	addr := fmt.Sprintf("%s:%s", host, port)
	log.Printf("[relay] listening on http://%s (data: %s)", addr, dataDir)
	if err := http.ListenAndServe(addr, root); err != nil {
		log.Fatal(err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}