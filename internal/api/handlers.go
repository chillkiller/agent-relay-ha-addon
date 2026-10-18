// Package api: HTTP + WebSocket surface for Agent Relay.
package api

import (
	"bytes"
	"context"
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

type Server struct {
	st  *store.DB
	rt  *runtime.Runtime
	mu  sync.RWMutex
	subs map[*websocket.Conn]bool
}

func New(st *store.DB, rt *runtime.Runtime) *Server {
	s := &Server{st: st, rt: rt, subs: map[*websocket.Conn]bool{}}
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

func (s *Server) broadcast(data map[string]any) {
	b, _ := json.Marshal(data)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for c := range s.subs {
		if err := c.Write(context.Background(), websocket.MessageText, b); err != nil {
			go func(c *websocket.Conn) { _ = c.Close(websocket.StatusInternalError, "write failed") }(c)
		}
	}
}

// ── routes ──────────────────────────────────────────────────────────

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

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
	mux.HandleFunc("POST /api/rooms/{id}/members", s.addMember)
	mux.HandleFunc("DELETE /api/rooms/{id}/members/{agentId}", s.removeMember)

	mux.HandleFunc("GET /api/tasks", s.listTasks)
	mux.HandleFunc("POST /api/tasks", s.createTask)
	mux.HandleFunc("PATCH /api/tasks/{id}", s.patchTask)
	mux.HandleFunc("DELETE /api/tasks/{id}", s.deleteTask)

	mux.HandleFunc("GET /ws", s.ws)
	return logMiddleware(mux)
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("[api] %s %s (%s)", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func (s *Server) Handler() http.Handler { return s.routes() }

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
	writeJSON(w, 200, s.st.GetSettings())
}

func (s *Server) patchSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	if err := decode(r, &body); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	for k, v := range body {
		if _, ok := map[string]bool{"relayName": true, "theme": true, "openclawBaseUrl": true, "openclawToken": true}[k]; ok {
			s.st.UpdateSetting(k, v)
		}
	}
	writeJSON(w, 200, s.st.GetSettings())
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
	if b.Model != "" || hasJSON(r, "model") {
		existing.Model = b.Model
	}
	if b.BaseURL != "" || hasJSON(r, "baseUrl") {
		existing.BaseURL = b.BaseURL
	}
	if b.APIKey != "" || hasJSON(r, "apiKey") {
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

// hasJSON inspects the raw body for a key (cheap presence check).
func hasJSON(r *http.Request, key string) bool {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

// ── ... continuation in next file ───────────────────────────────────