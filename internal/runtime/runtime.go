// Package runtime: schedules multi-agent turns for rooms.
package runtime

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
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
	stop     chan struct{}
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

// ── routines (P2#4): cron 5-field parser + 60s-Ticker ───────────────

// parseCronField: * | */n | a,b,c | a-b → matches? (min/max je Feld)
func parseCronField(field string, lo, hi int, val int) bool {
	if field == "*" {
		return true
	}
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "*/") {
			n, err := strconv.Atoi(part[2:])
			if err != nil || n <= 0 {
				continue
			}
			if val%lo == 0 { // */n relativ zur Feld-0-Basis (hour/dom/month: Basis 0)
				// hour: 0-basiert; day/month: 1-basiert mit Offset
				if lo == 0 && val%n == 0 {
					return true
				}
				if lo > 0 && (val-lo)%n == 0 {
					return true
				}
			}
			continue
		}
		if strings.Contains(part, "-") {
			ab := strings.SplitN(part, "-", 2)
			a2, e1 := strconv.Atoi(ab[0])
			b2, e2 := strconv.Atoi(ab[1])
			if e1 != nil || e2 != nil {
				continue
			}
			if val >= a2 && val <= b2 {
				return true
			}
			continue
		}
		if v, err := strconv.Atoi(part); err == nil && v == val {
			return true
		}
	}
	return false
}

// cronMatches: minute hour dom month dow (lokale Zeit des Servers).
func cronMatches(spec string, at time.Time) bool {
	f := strings.Fields(strings.TrimSpace(spec))
	if len(f) != 5 {
		return false
	}
	minute, _ := strconv.Atoi(f[0])
	_ = minute
	if !parseCronField(f[0], 0, 59, at.Minute()) {
		return false
	}
	if !parseCronField(f[1], 0, 23, at.Hour()) {
		return false
	}
	if !parseCronField(f[2], 1, 31, at.Day()) {
		return false
	}
	if !parseCronField(f[3], 1, 12, int(at.Month())) {
		return false
	}
	if !parseCronField(f[4], 0, 6, int(at.Weekday())) {
		return false
	}
	return true
}

// nextCronTime: nächste Ausführungszeit nach spec (Brute-Scan ≤ 1 Jahr
// minute-genau; korrekt für v1-Formen).
func nextCronTime(spec string, from time.Time) string {
	t := from.Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 366*24*60; i++ { // max 1 Jahr Minuten
		if cronMatches(spec, t) {
			return t.Format("2006-01-02T15:04:05.000000000Z07:00")
		}
		t = t.Add(time.Minute)
	}
	return from.Add(24 * time.Hour).Format("2006-01-02T15:04:05.000000000Z07:00")
}

// startRoutineTicker: 60 s-Tick — fällige Feuern als User-Post mit Prompt
// (Mention-Pfad; Agent-Antworten laufen über die normale enqueueTurn-Kette).
// StartRoutines: public Einstieg (main.go ruft es nach Runtime-Start).
func (r *Runtime) StartRoutines() {
	r.startRoutineTicker()
}

func (r *Runtime) startRoutineTicker() {
	go func() {
		tick := time.NewTicker(60 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-r.stop:
				return
			case <-tick.C:
				now := time.Now()
				nowISO := now.Format("2006-01-02T15:04:05.000000000Z07:00")
				due, err := r.st.DueRoutines(nowISO)
				if err != nil {
					continue
				}
				for _, t := range due {
					room := t.RoomID
					if room == "" {
						continue
					}
					prompt := t.Prompt
					if t.AgentID != "" {
						if ag, err := r.st.GetAgent(t.AgentID); err == nil && ag != nil {
							prompt = "@" + ag.Name + " " + prompt
						}
					}
					// P3-6b (opencode-Audit): Routine-Trigger MUSS das Modell
					// erreichen — als human-artige Trigger-Nachricht (history()
					// nimmt nur human/agent; ein system-Prompt verliert Modell-Kontakt)
					m := &store.Message{
						ID:         store.NewID("msg"),
						RoomID:     room,
						SenderName: "⏰ " + t.Name,
						SenderKind: "human", // Trigger-Route (Agent sieht den Prompt)
						Content:    prompt,
						Status:     "complete",
						CreatedAt:  nowISO,
						UpdatedAt:  nowISO,
					}
					if err := r.st.InsertMessage(m); err != nil {
						continue
					}
					r.publish("message.created", map[string]any{"message": m})
					// Agent-Turn mit Routine-Message (ParentID = System-Route:
					// die Routine-Message läuft als User-Ersatz)
					userMsg := m
					go func(t2 *store.Routine) {
						if t2.AgentID != "" {
							if ag, err := r.st.GetAgent(t2.AgentID); err == nil && ag != nil {
								r.enqueueTurn(ag, userMsg)
							}
						} else {
							// kein Agent-Pin: alle Raum-Members (OnUserMessage-Pfad)
							agents, _ := r.st.RoomAgents(room)
							for i, ag2 := range agents {
								ag3 := ag2
								delay := time.Duration(i) * 900 * time.Millisecond
								go func() {
									select {
									case <-time.After(delay):
									case <-r.stop:
										return
									}
									r.enqueueTurn(ag3, userMsg)
								}()
							}
						}
					}(t)
					next := nextCronTime(t.Cron, now)
					_ = r.st.RoutineFired(t.ID, nowISO, next)
				}
			}
		}
	}()
}

// ResumeApproval (P2#2): Freigabe re-enqueued den Original-Turn —
// OnUserMessage-Weg (Mention-Treffer identisch zur Original-Auslösung,
// enqueueTurn-Kette latest-wins bleibt intakt).
func (r *Runtime) ResumeApproval(ap *store.Approval) error {
	m, err := r.st.GetMessage(ap.UserMsgID)
	if err != nil || m == nil {
		return fmt.Errorf("original message %s nicht gefunden", ap.UserMsgID)
	}
	// Codex-Audit H1: Raum-Membership des Agents prüfen (raum kann gelöscht/
	// roster-geändert sein — kein Turn für ex-Members) + Token ist noch offen?
	ok, _ := r.st.ConsumeApprovalFor(ap.AgentID, ap.UserMsgID)
	if !ok {
		return fmt.Errorf("Grant-Token bereits konsumiert (Doppel-Resume-Schutz)")
	}
	agents, _ := r.st.RoomAgents(m.RoomID)
	stillMember := false
	for _, a := range agents {
		if a.ID == ap.AgentID {
			stillMember = true
			break
		}
	}
	if !stillMember {
		return fmt.Errorf("agent %s ist nicht mehr Mitglied des Raums", ap.AgentID)
	}
	if a, _ := r.st.GetAgent(ap.AgentID); a != nil && a.Enabled {
		r.enqueueTurn(a, m) // Codex-Audit P2-3: NUR der begutachtete Agent (kein Raum-Fanout, kein Treadmill)
	} else {
		return fmt.Errorf("agent %s nicht gefunden oder aus", ap.AgentID)
	}
	return nil
}

// OnUserMessage is called after a human message is persisted.
func (r *Runtime) OnUserMessage(m *store.Message) {
	agents, err := r.st.RoomAgents(m.RoomID)
	if err != nil || len(agents) == 0 {
		return
	}

	// P1#9 (Codex-Matrix): "@all" expandiert zu ALLEN Raum-Members;
	// sonst Mention-Match; ohne jeden Treffer = alle (bisheriges Verhalten).
	var targets []*store.Agent
	if strings.Contains(strings.ToLower(m.Content), "@all") {
		targets = agents
	} else {
		targets = mentioned(m.Content, agents)
		if len(targets) == 0 {
			targets = agents
		}
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
	// P2#3: gebundene Skills als Systemprompt-Anhang (Paperclip-Muster:
	// agent-skills hängen an Agents; Instructions = wiederverwendbare Blöcke)
	if skills, err := r.st.AgentsSkills(a.ID); err == nil && len(skills) > 0 {
		extra := "\n\n# Gebundene Skills\n"
		for _, sk := range skills {
			extra += fmt.Sprintf("\n## %s (v%d)\n%s\n", sk.Name, sk.Version, sk.Instructions)
		}
		cfg.SystemPrompt += extra
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

	// Audit (P3#4): Turn-Start als Register-Fall — Receipt zeigt WANN/WER/
	// warum-Weg (Adapter + sessionKey), nicht den Chat-Inhalt.
	r.audit("agent.turn.started", a.Name, "agent", userMsg.RoomID, map[string]any{
		"agentId": a.ID, "msgId": userMsg.ID,
		"adapter": a.Adapter, "sessionKey": cfg.SessionKey,
	})

	// stream placeholder row first, so ordering in UI is stable
	// P1#4: Agent-Antwort erbt parent_id vom auslösenden Turn — der
	// Agent-Turn bleibt im selben Thread wie die Frage.
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
		ParentID:   userMsg.ParentID,
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
		r.audit("agent.turn.error", a.Name, "agent", userMsg.RoomID, map[string]any{"msgId": msgID, "phase": "validate"})
		_ = r.st.UpdateMessage(msgID, finish, "error")
		r.publish("agent.finished", map[string]any{
			"message": updated(r.st, msgID),
		})
		return nil
	}

	// P2#2 (Codex-Matrix 2.2, paperclip approvals.ts-Muster): Approval-Gate —
	// bei Agent-Flag approval_gate hält der Turn VOR dem Stream an:
	// pending-Approval persistieren, WS approval.required, Turn sauber
	// beenden; die Freigabe re-enqueued den Original-Turn über die
	// bestehende enqueueTurn-Kette (latest-wins bleibt intakt).
	if a.ApprovalGate {
		// P2#2 + Befund 1 (Codex-Audit): ATOMARER Grant-Konsum — true =
		// dieser Turn führt aus (Resume-Weg); false = Token schon verbrannt
		// (Doppel-Resume-Schutz) → Turn still skippen, KEIN Gate-Re-Trigger.
		runs, _ := r.st.ConsumeApprovalFor(a.ID, userMsg.ID)
		if runs != true {
			ap := &store.Approval{
				ID:        store.NewID("appr"),
				AgentID:   a.ID,
				UserMsgID: userMsg.ID,
				Reason:    fmt.Sprintf("Turn angehalten — Freigabe für @%s-Antwort", a.Name),
				Status:    "pending",
				CreatedAt: nowRFC3339(),
			}
			if err := r.st.InsertApproval(ap); err != nil {
				return err
			}
			r.publish("approval.required", map[string]any{"approval": ap})
			return nil
		}
	}

	history := r.history(cfg.ID, userMsg.RoomID)
	chunks, err := adapt.Stream(cfg, history)
	if err != nil {
		finish := fmt.Sprintf("⚠️ %v", err)
		r.audit("agent.turn.error", a.Name, "agent", userMsg.RoomID, map[string]any{"msgId": msgID, "phase": "stream"})
		_ = r.st.UpdateMessage(msgID, finish, "error")
		r.publish("agent.finished", map[string]any{"message": updated(r.st, msgID)})
		return nil
	}

	var acc strings.Builder
	for c := range chunks {
		if c.Err != nil {
			r.audit("agent.turn.error", a.Name, "agent", userMsg.RoomID, map[string]any{"msgId": msgID, "phase": "chunk"})
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
	// Audit (P3#4): Turn-Terminus — Status + Antwortgröße (Bytes, nicht der
	// Text: Inhalt bleibt im messages-Log; das Register beweist Ereignisse).
	r.audit("agent.turn.finished", a.Name, "agent", userMsg.RoomID, map[string]any{
		"msgId": msgID, "status": status, "bytes": len(final),
	})
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
		if v != "" {
			return v
		}
	}
	return ""
}

// audit (P3#4): append one Relay event to the hash-chain register.
// Failure is logged, never fatal — bookkeeping must not break turns
// (Register ist Beweis-Schicht, kein Steuerungs-Abhängigkeit).
func (r *Runtime) audit(action, actor, actorKind, subject string, detail map[string]any) {
	e := &store.AuditEvent{
		Action: action, Actor: actor, ActorKind: actorKind,
		Subject: subject, Detail: auditJSON(detail),
	}
	if err := r.st.AppendAudit(e); err != nil {
		log.Printf("[audit] append fehlgeschlagen (%s): %v", action, err)
	}
}

// auditJSON encodes detail deterministically (encoding/json sorts map keys)
// so the chain verification reproduces the exact stored bytes; nil → "{}"
// (codex P3-5 — nie "null" in der Kette).
func auditJSON(m map[string]any) string {
	if m == nil {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}
