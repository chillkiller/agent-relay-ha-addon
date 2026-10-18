// Package runtime: schedules multi-agent turns for rooms.
package runtime

import (
	"context"
	"fmt"
	"log"
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
	st        *store.DB
	bus       Bus
	adapters  map[string]adapter.Adapter
	mu        sync.Mutex
	lastUser  map[string]*store.Message // roomID -> last user message snapshot (for history anchor)
	stop      chan struct{}
}

func New(st *store.DB, bus Bus) *Runtime {
	return &Runtime{
		st:       st,
		bus:      bus,
		adapters: map[string]adapter.Adapter{},
		lastUser: map[string]*store.Message{},
		stop:     make(chan struct{}),
	}
}

func (r *Runtime) Register(a adapter.Adapter) { r.adapters[a.ID()] = a }

// SetBus attaches the event sink lazily (api wires it after construction).
func (r *Runtime) SetBus(b Bus) {
	r.mu.Lock()
	r.bus = b
	r.mu.Unlock()
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
			if err := r.turn(agent, m); err != nil {
				log.Printf("[runtime] turn failed (%s): %v", agent.Name, err)
				r.bus.Publish("system", map[string]any{
					"roomId": m.RoomID,
					"text":   fmt.Sprintf("⚠️ %s: %v", agent.Name, err),
				})
			}
		}()
	}
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
		cfg.BaseURL, _ = settings["openclawBaseUrl"].(string)
	}
	if cfg.APIKey == "" {
		settings := r.st.GetSettings()
		cfg.APIKey, _ = settings["openclawToken"].(string)
	}

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
	r.bus.Publish("agent.started", map[string]any{"message": placeholder})
	r.bus.Publish("typing", map[string]any{
		"roomId": userMsg.RoomID, "agentId": a.ID, "agentName": a.Name,
	})

	if err := adapt.Validate(cfg); err != nil {
		finish := fmt.Sprintf("⚠️ %v", err)
		_ = r.st.UpdateMessage(msgID, finish, "error")
		r.bus.Publish("agent.finished", map[string]any{
			"message": updated(r.st, msgID),
		})
		return nil
	}

	history := r.history(cfg.ID, userMsg.RoomID)
	chunks, err := adapt.Stream(cfg, history)
	if err != nil {
		finish := fmt.Sprintf("⚠️ %v", err)
		_ = r.st.UpdateMessage(msgID, finish, "error")
		r.bus.Publish("agent.finished", map[string]any{"message": updated(r.st, msgID)})
		return nil
	}

	var acc strings.Builder
	for c := range chunks {
		if c.Err != nil {
			_ = r.st.UpdateMessage(msgID, acc.String()+fmt.Sprintf("\n⚠️ %v", c.Err), "error")
			r.bus.Publish("agent.finished", map[string]any{"message": updated(r.st, msgID)})
			return nil
		}
		acc.WriteString(c.Text)
		r.bus.Publish("agent.token", map[string]any{"messageId": msgID, "delta": c.Text})
	}

	final := strings.TrimSpace(acc.String())
	status := "complete"
	if final == "" {
		final, status = "⚠️ Leere Antwort vom Adapter", "error"
	}
	_ = r.st.UpdateMessage(msgID, final, status)
	r.bus.Publish("agent.finished", map[string]any{"message": updated(r.st, msgID)})
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
			// include other agents' messages as assistant context with name hints
			role := "assistant"
			name := m.SenderName
			_ = agentID
			out = append(out, adapter.Message{Role: role, Name: name, Content: m.Content})
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

var _ = context.Background // reserve for future ctx plumbing