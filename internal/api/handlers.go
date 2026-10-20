// Package api: HTTP + WebSocket surface for Agent Relay.
package api

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"agentrelay/internal/runtime"
	"agentrelay/internal/store"
)

// sub is one connected websocket client with its own serialized write pump.
// coder/websocket requires sequential Writes per connection; a dedicated
// goroutine also keeps slow clients from blocking the hub (review C-1/M-5).
type sub struct {
	conn *websocket.Conn
	out  chan []byte
	done chan struct{}
}

type Server struct {
	st     *store.DB
	rt     *runtime.Runtime
	mu     sync.RWMutex
	subs   map[*sub]bool
	bearer string // optional API gate (RELAY_BEARER_TOKEN); empty = open LAN mode
}

func New(st *store.DB, rt *runtime.Runtime) *Server {
	s := &Server{st: st, rt: rt, subs: map[*sub]bool{}}
	broadcast = s.broadcast
	rt.SetBus(hubAdapter{})
	return s
}

// hubAdapter adapts the Server to runtime.Bus.
type hubAdapter struct{}

func (hubAdapter) Publish(event string, payload map[string]any) {
	wrapped := map[string]any{"type": event, "ts": time.Now().UnixMilli()}
	for k, v := range payload {
		wrapped[k] = v
	}
	broadcast(wrapped)
}

// broadcast is bound in New after Server construction; hubAdapter publishes
// through it (the two are decoupled so runtime keeps no api import).
var broadcast func(map[string]any)

// broadcast enqueues for every client; per-sub write pumps own the writes.
// Never blocks on slow peers; a full queue closes that client (slow-consumer
// isolation) instead of stalling the hub.
func (s *Server) broadcast(data map[string]any) {
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	s.mu.RLock()
	targets := make([]*sub, 0, len(s.subs))
	for sub := range s.subs {
		targets = append(targets, sub)
	}
	s.mu.RUnlock()
	for _, sub := range targets {
		select {
		case sub.out <- b:
		default:
			// slow consumer: drop it from the hub outside any read path
			go s.dropSub(sub, "slow consumer")
		}
	}
}

// dropSub unregisters and closes a client exactly once.
func (s *Server) dropSub(sub *sub, reason string) {
	s.mu.Lock()
	_, present := s.subs[sub]
	delete(s.subs, sub)
	s.mu.Unlock()
	if !present {
		return
	}
	close(sub.done) // write pump exits on done
	_ = sub.conn.Close(websocket.StatusNormalClosure, reason)
}

// writePump serializes writes for one connection and exits on first failure.
func (s *Server) writePump(sub *sub) {
	for {
		select {
		case <-sub.done:
			return
		case msg := <-sub.out:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := sub.conn.Write(ctx, websocket.MessageText, msg); err != nil {
				cancel()
				s.dropSub(sub, "write failed")
				return
			}
			cancel()
		}
	}
}

// ── routes ──────────────────────────────────────────────────────────

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	_ = s.bearer

	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("PATCH /api/settings", s.patchSettings)

	mux.HandleFunc("GET /api/agents", s.listAgents)
	mux.HandleFunc("POST /api/agents", s.createAgent)
	mux.HandleFunc("GET /api/agents/{id}", s.getAgent)
	mux.HandleFunc("PATCH /api/agents/{id}", s.patchAgent)
	mux.HandleFunc("DELETE /api/agents/{id}", s.deleteAgent)

	mux.HandleFunc("GET /api/projects", s.listProjects)
	mux.HandleFunc("POST /api/projects", s.createProject)
	mux.HandleFunc("PATCH /api/projects/{id}", s.patchProject)
	mux.HandleFunc("DELETE /api/projects/{id}", s.deleteProject)

	mux.HandleFunc("GET /api/rooms", s.listRooms)
	mux.HandleFunc("POST /api/rooms", s.createRoom)
	mux.HandleFunc("PATCH /api/rooms/{id}", s.patchRoom)
	mux.HandleFunc("DELETE /api/rooms/{id}", s.deleteRoom)
	mux.HandleFunc("GET /api/rooms/{id}/messages", s.listMessages)
	mux.HandleFunc("POST /api/rooms/{id}/messages", s.postMessage)
	mux.HandleFunc("POST /api/rooms/reorder", s.reorderRooms)
	mux.HandleFunc("POST /api/rooms/{id}/members", s.addMember)
	mux.HandleFunc("DELETE /api/rooms/{id}/members/{agentId}", s.removeMember)

	mux.HandleFunc("POST /api/reactions/batch", s.reactionsBatch)
	mux.HandleFunc("GET /api/messages/{id}", s.getMessage)
	mux.HandleFunc("PATCH /api/messages/{id}", s.editMessage)
	mux.HandleFunc("POST /api/messages/{id}/reactions", s.addReaction)
	mux.HandleFunc("DELETE /api/messages/{id}/reactions/{emoji}", s.removeReaction)
	mux.HandleFunc("GET /api/rooms/{id}/search", s.searchMessages)
	mux.HandleFunc("GET /api/tasks", s.listTasks)
	mux.HandleFunc("POST /api/tasks", s.createTask)
	mux.HandleFunc("PATCH /api/tasks/{id}", s.patchTask)
	mux.HandleFunc("DELETE /api/tasks/{id}", s.deleteTask)

	mux.HandleFunc("GET /ws", s.ws)
	return mux
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("[api] %s %s (%s)", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func (s *Server) SetBearer(tok string) { s.bearer = tok }

// authorized: constant-time bearer check via header (REST) or ?token= (WS).
func (s *Server) authorized(r *http.Request) bool {
	if s.bearer == "" {
		return true
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") && subtle.ConstantTimeCompare([]byte(auth[7:]), []byte(s.bearer)) == 1 {
		return true
	}
	if tok := r.URL.Query().Get("token"); tok != "" && subtle.ConstantTimeCompare([]byte(tok), []byte(s.bearer)) == 1 {
		return true
	}
	return false
}

// Handler composes: optional bearer gate → routes → logging.
// /api/health stays open so the Supervisor watchdog can probe.
func (s *Server) Handler() http.Handler {
	mux := s.routes()
	var h http.Handler = mux
	if s.bearer != "" {
		h = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/health" || s.authorized(r) {
				mux.ServeHTTP(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(401)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "auth required"})
		})
	}
	return logMiddleware(h)
}

// ── helpers ─────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func errJSON(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func decode(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func newIDs(prefix string) string { return store.NewID(prefix) }

func nowISO() string { return time.Now().UTC().Format(time.RFC3339) }

func roomChanged(s *Server) {
	s.broadcast(map[string]any{"type": "rooms.changed", "ts": time.Now().UnixMilli()})
}

// ── handlers: health/settings ───────────────────────────────────────

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	agents, _ := s.st.ListAgents()
	rooms, _ := s.st.ListRooms()
	writeJSON(w, 200, map[string]any{
		"ok":       true,
		"adapters": s.rt.AdapterIDs(),
		"agents":   len(agents),
		"rooms":    len(rooms),
	})
}

func (s *Server) getSettings(w http.ResponseWriter, _ *http.Request) {
	out := s.st.GetSettings()
	// write-only token: never return the stored secret (review C-2)
	if tok, ok := out["openclawToken"].(string); ok && tok != "" {
		out["openclawToken"] = "##MASK##"
	} else {
		out["openclawToken"] = ""
	}
	writeJSON(w, 200, out)
}

func (s *Server) patchSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	if err := decode(r, &body); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	for k, v := range body {
		if _, ok := map[string]bool{"relayName": true, "theme": true, "openclawBaseUrl": true, "openclawToken": true}[k]; ok {
			if k == "openclawToken" && (v == "##MASK##" || strings.TrimSpace(v) == "") {
				continue // placeholder or cleared field never overwrites the secret
			}
			if err := s.st.UpdateSetting(k, v); err != nil {
				errJSON(w, 500, err.Error())
				return
			}
		}
	}
	out := s.st.GetSettings()
	// never echo the secret back on PATCH either (review-2 copilot H-1)
	if tok, ok := out["openclawToken"].(string); ok && tok != "" {
		out["openclawToken"] = "##MASK##"
	} else {
		out["openclawToken"] = ""
	}
	writeJSON(w, 200, out)
}

// ── handlers: agents ────────────────────────────────────────────────

type agentBody struct {
	Name         string `json:"name"`
	Emoji        string `json:"emoji"`
	Color        string `json:"color"`
	SystemPrompt string `json:"systemPrompt"`
	Adapter      string `json:"adapter"`
	Model        string `json:"model"`
	BaseURL      string `json:"baseUrl"`
	APIKey       string `json:"apiKey"`
	Enabled      *bool  `json:"enabled"`
}

func (s *Server) listAgents(w http.ResponseWriter, _ *http.Request) {
	agents, err := s.st.ListAgents()
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if agents == nil {
		agents = []*store.Agent{}
	}
	writeJSON(w, 200, agents)
}

func (s *Server) getAgent(w http.ResponseWriter, r *http.Request) {
	a, err := s.st.GetAgent(r.PathValue("id"))
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if a == nil {
		errJSON(w, 404, "Agent nicht gefunden")
		return
	}
	writeJSON(w, 200, a)
}

func (s *Server) createAgent(w http.ResponseWriter, r *http.Request) {
	var b agentBody
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	if strings.TrimSpace(b.Name) == "" {
		errJSON(w, 400, "name erforderlich")
		return
	}
	enabled := true
	if b.Enabled != nil {
		enabled = *b.Enabled
	}
	a := &store.Agent{
		ID:           newIDs("agent"),
		Name:         b.Name,
		Emoji:        fallback(b.Emoji, "🤖"),
		Color:        fallback(b.Color, "#6ea8fe"),
		SystemPrompt: b.SystemPrompt,
		Adapter:      fallback(b.Adapter, "echo"),
		Model:        b.Model,
		BaseURL:      b.BaseURL,
		APIKey:       b.APIKey,
		Enabled:      enabled,
		CreatedAt:    nowISO(),
		UpdatedAt:    nowISO(),
	}
	if err := s.st.InsertAgent(a); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, a)
}

func (s *Server) patchAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.st.GetAgent(id)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if existing == nil {
		errJSON(w, 404, "Agent nicht gefunden")
		return
	}
	raw, rerr := readRawBody(r)
	if rerr != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	var b agentBody
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	if b.Name != "" {
		existing.Name = b.Name
	}
	if b.Emoji != "" {
		existing.Emoji = b.Emoji
	}
	if b.Color != "" {
		existing.Color = b.Color
	}
	if b.SystemPrompt != "" {
		existing.SystemPrompt = b.SystemPrompt
	}
	if b.Adapter != "" {
		existing.Adapter = b.Adapter
	}
	if fieldPresent(raw, "model") {
		existing.Model = b.Model
	}
	if fieldPresent(raw, "baseUrl") {
		existing.BaseURL = b.BaseURL
	}
	if fieldPresent(raw, "apiKey") {
		existing.APIKey = b.APIKey
	}
	if b.Enabled != nil {
		existing.Enabled = *b.Enabled
	}
	existing.UpdatedAt = nowISO()
	if err := s.st.UpdateAgent(existing); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	// Sprint-4 (Matrix-4): Agent-Roster-Änderungen (Enable-Toggle) als
	// WS-Event — SPA aktualisiert Presence-Dots ohne Poll.
	broadcast(map[string]any{"type": "agents.changed", "agentId": id, "enabled": existing.Enabled, "ts": time.Now().UnixMilli()})
	writeJSON(w, 200, existing)
}

func (s *Server) deleteAgent(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteAgent(r.PathValue("id")); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

func fallback(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// fieldPresent reports whether the request body contains a key. Requires
// readRawBody (below) to have loaded the body — decode()-based handlers call
// it via bodyFields (review-2 copilot H-3: hasJSON alone is dead code —
// decode() consumes the body first).
func fieldPresent(raw map[string]json.RawMessage, key string) bool {
	_, ok := raw[key]
	return ok
}

// readRawBody reads the request body ONCE into a map and restores a
// replacement reader, so a later decode() still works.
func readRawBody(r *http.Request) (map[string]json.RawMessage, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	if len(bytes.TrimSpace(body)) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// ── ... continuation in next file ───────────────────────────────────