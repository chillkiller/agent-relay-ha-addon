// Package runtime: schedules multi-agent turns for rooms.
package runtime

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"agentrelay/internal/adapter"
	"agentrelay/internal/store"
)

// Bus is the event sink (usually a websocket hub).
type Bus interface {
	Publish(event string, payload map[string]any)
}

type Runtime struct {
	st       *store.DB
	bus      Bus
	mu       sync.RWMutex
	adapters map[string]adapter.Adapter
	stop    chan struct{}
	// per-agent turn serialization (review-2 copilot M-B)
	busy    map[string]bool
	pending map[string]*store.Message // agentID -> latest queued user msg
}

func New(st *store.DB, bus Bus) *Runtime {
	return &Runtime{
		st:       st,
		bus:      bus,
		adapters: map[string]adapter.Adapter{},
		stop:     make(chan struct{}),
		busy:     map[string]bool{},
		pending:  map[string]*store.Message{},
	}
}

// Stop revokes all pending turn schedules and marks the runtime as stopping
// (running turns finish; scheduled delays are cancelled). Idempotent.
func (r *Runtime) Stop() {
	r.mu.Lock()
	select {
	case <-r.stop:
		// already stopped
	default:
		close(r.stop)
	}
	r.mu.Unlock()
}

func (r *Runtime) Register(a adapter.Adapter) { r.adapters[a.ID()] = a }

// SetBus attaches the event sink lazily (api wires it after construction).
func (r *Runtime) SetBus(b Bus) {
	r.mu.Lock()
	r.bus = b
	r.mu.Unlock()
}

// publish captures the bus under RLock (review-2 codex H-2: racing SetBus).
func (r *Runtime) publish(event string, payload map[string]any) {
	r.mu.RLock()
	b := r.bus
	r.mu.RUnlock()
	if b != nil {
		b.Publish(event, payload)
	}
}

func (r *Runtime) AdapterIDs() []string {
	ids := make([]string, 0, len(r.adapters))
	for id := range r.adapters {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// OnUserMessage is called after a human message is persisted.
func (r *Runtime) OnUserMessage(m *store.Message) {
	agents, err := r.st.RoomAgents(m.RoomID)
	if err != nil || len(agents) == 0 {
		return
	}

	targets := mentioned(m.Content, agents)
	hits := len(targets)
	if hits == 0 {
		targets = agents
	}

	for i, a := range targets {
		agent := a
		delay := time.Duration(i) * 900 * time.Millisecond
		go func() {
			select {
			case <-time.After(delay):
			case <-r.stop:
				return
			}
			r.enqueueTurn(agent, m)
		}()
	}
}

// enqueueTurn runs the agent's turn now if idle, otherwise queues it
// (latest-wins) and runs it when the current turn finishes
// (review-2 copilot M-B: no parallel turns of the same harness session).
func (r *Runtime) enqueueTurn(agent *store.Agent, m *store.Message) {
	r.mu.Lock()
	if r.busy[agent.ID] {
		r.pending[agent.ID] = m // latest wins
		r.mu.Unlock()
		return
	}
	r.busy[agent.ID] = true
	r.mu.Unlock()

	var run func(msg *store.Message)
	run = func(msg *store.Message) {
		// next queued turn runs after this one (deferred chain)
		defer func() {
			r.mu.Lock()
			next := r.pending[agent.ID]
			delete(r.pending, agent.ID)
			if next == nil {
				delete(r.busy, agent.ID)
			}
			r.mu.Unlock()
			if next != nil {
				run(next)
			}
		}()
		select {
		case <-r.stop:
			return
		default:
		}
		if err := r.turn(agent, msg); err != nil {
			log.Printf("[runtime] turn failed (%s): %v", agent.Name, err)
			r.publish("system", map[string]any{
				"roomId": msg.RoomID,
				"text":   fmt.Sprintf("⚠️ %s: %v", agent.Name, err),
			})
		}
	}
	run(m)
}

func mentioned(text string, agents []*store.Agent) []*store.Agent {
	lower := strings.ToLower(text)
	matched := func(cand string) bool {
		c := strings.ToLower(cand)
		if c == "" {
			return false
		}
		pattern := "@" + regexp.QuoteMeta(c)
		for start := strings.Index(lower, pattern); start != -1; {
			end := start + len(pattern)
			// word boundary right of the mention (Go RE2 has no lookahead)
			if end >= len(lower) {
				return true
			}
			next := lower[end]
			if next != '_' && next != '-' && !(next >= 'a' && next <= 'z') && !(next >= '0' && next <= '9') && next < 0x80 {
				return true
			}
			// non-ascii (e.g. emoji/umlaut suffix) also breaks the word
			if next >= 0x80 {
				return true
			}
			idx := strings.Index(lower[end:], pattern)
			if idx == -1 {
				return false
			}
			start = end + idx
		}
		return false
	}
	var out []*store.Agent
	for _, a := range agents {
		if matched(a.Name) || matched(a.ID) {
			out = append(out, a)
		}
	}
	return out
}

// turn: run one full agent response into the room.
func (r *Runtime) turn(a *store.Agent, userMsg *store.Message) error {
	adapt := r.adapters[a.Adapter]
	if adapt == nil {
		return fmt.Errorf("unbekannter Adapter %q", a.Adapter)
	}

	room, err := r.st.GetRoom(userMsg.RoomID)
	if err != nil || room == nil {
		return fmt.Errorf("raum nicht gefunden")
	}

	cfg := adapter.AgentConfig{
		ID:           a.ID,
		Name:         a.Name,
		SystemPrompt: a.SystemPrompt,
		Model:        a.Model,
		BaseURL:      a.BaseURL,
		APIKey:       a.APIKey,
	}
	if cfg.BaseURL == "" {
		settings := r.st.GetSettings()
		sv, _ := settings["openclawBaseUrl"].(string)
		// env fallback (review H-2): the add-on injects options; UI wins,
		// env fills gaps so operators can configure either way.
		cfg.BaseURL = firstNonEmpty(sv, os.Getenv("RELAY_OPENAI_BASE_URL"))
	}
	if cfg.APIKey == "" {
		settings := r.st.GetSettings()
		tk, _ := settings["openclawToken"].(string)
		cfg.APIKey = firstNonEmpty(tk, os.Getenv("RELAY_OPENAI_API_KEY"))
	}
	cfg.IdempotencyKey = userMsg.ID
	// gateway-agent namespace (review: 'agent "coordinator" does not match
	// session key agent "agent_…"'): derive from the configured agent target
	// (model field), not the relay-DB row id. Room suffix keeps rooms apart.
	target := strings.TrimPrefix(strings.TrimPrefix(a.Model, "openclaw/"), "agent:")
	if target == "" || target == "openclaw" {
		target = "main" // gateway default agent
	}
	cfg.SessionKey = fmt.Sprintf("agent:%s:relay:%s", target, userMsg.RoomID)

	// stream placeholder row first, so ordering in UI is stable
	msgID := store.NewID("msg")
	placeholder := &store.Message{
		ID:         msgID,
		RoomID:     userMsg.RoomID,
		AgentID:    a.ID,
		SenderName: a.Name,
		SenderKind: "agent",
		Content:    "",
		Status:     "streaming",
		CreatedAt:  nowRFC3339(),
		UpdatedAt:  nowRFC3339(),
	}
	if err := r.st.InsertMessage(placeholder); err != nil {
		return err
	}
	r.publish("agent.started", map[string]any{"message": placeholder})
	r.publish("typing", map[string]any{
		"roomId": userMsg.RoomID, "agentId": a.ID, "agentName": a.Name,
	})

	if err := adapt.Validate(cfg); err != nil {
		finish := fmt.Sprintf("⚠️ %v", err)
		_ = r.st.UpdateMessage(msgID, finish, "error")
		r.publish("agent.finished", map[string]any{
			"message": updated(r.st, msgID),
		})
		return nil
	}

	history := r.history(cfg.ID, userMsg.RoomID)
	chunks, err := adapt.Stream(cfg, history)
	if err != nil {
		finish := fmt.Sprintf("⚠️ %v", err)
		_ = r.st.UpdateMessage(msgID, finish, "error")
		r.publish("agent.finished", map[string]any{"message": updated(r.st, msgID)})
		return nil
	}

	var acc strings.Builder
	for c := range chunks {
		if c.Err != nil {
			_ = r.st.UpdateMessage(msgID, acc.String()+fmt.Sprintf("\n⚠️ %v", c.Err), "error")
			r.publish("agent.finished", map[string]any{"message": updated(r.st, msgID)})
			return nil
		}
		acc.WriteString(c.Text)
		r.publish("agent.token", map[string]any{"messageId": msgID, "delta": c.Text})
	}

	final := strings.TrimSpace(acc.String())
	status := "complete"
	if final == "" {
		final, status = "⚠️ Leere Antwort vom Adapter", "error"
	}
	_ = r.st.UpdateMessage(msgID, final, status)
	r.publish("agent.finished", map[string]any{"message": updated(r.st, msgID)})
	return nil
}

// history builds the adapter-facing history (full room context).
func (r *Runtime) history(agentID, roomID string) []adapter.Message {
	msgs, err := r.st.ListMessages(roomID, 60)
	if err != nil {
		return nil
	}
	out := make([]adapter.Message, 0, len(msgs))
	for _, m := range msgs {
		switch m.SenderKind {
		case "human":
			out = append(out, adapter.Message{Role: "user", Name: m.SenderName, Content: m.Content})
		case "agent":
			// include agents' messages as assistant context with stable ids
			out = append(out, adapter.Message{Role: "assistant", Name: m.SenderName, Content: m.Content, AgentID: m.AgentID})
		}
	}
	// The last entry should be the user's message (guaranteed by DB ordering
	// because the placeholder is streamed AFTER it was inserted, but the
	// placeholder is 'agent' kind and empty → skip empty contents).
	clean := out[:0]
	for _, m := range out {
		if strings.TrimSpace(m.Content) != "" {
			clean = append(clean, m)
		}
	}
	return clean
}

func updated(st *store.DB, id string) any {
	m, _ := st.GetMessage(id)
	return m
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" { return v }
	}
	return ""
}
