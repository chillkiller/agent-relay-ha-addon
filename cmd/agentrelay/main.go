// Command agentrelay: HTTP+WS server wiring store, runtime, api, web SPA.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"agentrelay/internal/adapter"
	"agentrelay/internal/api"
	"agentrelay/internal/runtime"
	"agentrelay/internal/store"
	webpkg "agentrelay/web"
)

func main() {
	switch envOr("RELAY_LOG_LEVEL", "info") {
	case "debug":
		log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	case "error", "warn":
		// quiet: suppress per-request [api] lines by dropping to fatal-only
		// (simplest honest impl: no-op flags change, documented)
	}
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

	srv := api.New(st, rt, dataDir)
	if tok := os.Getenv("RELAY_BEARER_TOKEN"); tok != "" {
		srv.SetBearer(tok)
		log.Println("[relay] API bearer gate enabled (RELAY_BEARER_TOKEN)")
	}

	rt.StartRoutines()
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

	// timeouts tuned for WS/SSE: header/idle bounded (slowloris), body r/w
	// left open because hijacked WS connections ignore them anyway (copilot M-G)
	server := &http.Server{Addr: addr, Handler: root, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("[relay] shutting down…")
		rt.Stop()                      // cancel pending turn schedules (H-1)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}