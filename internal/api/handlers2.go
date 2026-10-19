package api

import (
	"encoding/json"
	"log"
	"strconv"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/coder/websocket"

	"agentrelay/internal/store"
)

// ── handlers: projects ──────────────────────────────────────────────

type projectBody struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
}

func (s *Server) listProjects(w http.ResponseWriter, _ *http.Request) {
	projects, err := s.st.ListProjects()
	if err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	if projects == nil {
		projects = []*store.Project{}
	}
	writeJSON(w, 200, projects)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var b projectBody
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	if strings.TrimSpace(b.Name) == "" {
		errJSON(w, 400, "name erforderlich")
		return
	}
	p := &store.Project{
		ID:          newIDs("proj"),
		Name:        b.Name,
		Description: b.Description,
		Color:       fallback(b.Color, "#8a7ff0"),
		CreatedAt:   nowISO(),
		UpdatedAt:   nowISO(),
	}
	if err := s.st.InsertProject(p); err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	writeJSON(w, 201, p)
}

func (s *Server) patchProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.st.GetProject(id)
	if err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	if p == nil {
		errJSON(w, 404, "Projekt nicht gefunden")
		return
	}
	raw, rerr := readRawBody(r)
	if rerr != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	var b projectBody
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	if b.Name != "" {
		p.Name = b.Name
	}
	if fieldPresent(raw, "description") {
		p.Description = b.Description
	}
	if b.Color != "" {
		p.Color = b.Color
	}
	p.UpdatedAt = nowISO()
	if err := s.st.UpdateProject(p); err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteProject(r.PathValue("id")); err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	w.WriteHeader(204)
}

// ── handlers: rooms ─────────────────────────────────────────────────

type roomBody struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"` // chat | project
	HumanMember *bool  `json:"humanMember"` // operator in/out
	ProjectID *string  `json:"projectId"`
	AgentIDs  []string `json:"agentIds"`
	// member endpoints:
	AgentID string `json:"agentId"`
}

func (s *Server) listRooms(w http.ResponseWriter, _ *http.Request) {
	rooms, err := s.st.ListRooms()
	if err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	if rooms == nil {
		rooms = []*store.Room{}
	}
	writeJSON(w, 200, rooms)
}

func (s *Server) createRoom(w http.ResponseWriter, r *http.Request) {
	var b roomBody
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	if strings.TrimSpace(b.Name) == "" {
		errJSON(w, 400, "name erforderlich")
		return
	}
	if len(b.AgentIDs) == 0 {
		b.AgentIDs = []string{}
	}
	if b.Kind == "" {
		b.Kind = "chat"
	}
	if b.Kind != "chat" && b.Kind != "project" {
		errJSON(w, 400, "kind muss chat oder project sein")
		return
	}
	// project rooms are the project's home: auto-create the linked board (UX)
	if b.Kind == "project" && (b.ProjectID == nil || *b.ProjectID == "") {
		proj := &store.Project{ID: newIDs("proj"), Name: b.Name, CreatedAt: nowISO(), UpdatedAt: nowISO()}
		if err := s.st.InsertProject(proj); err != nil {
			log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
			return
		}
		b.ProjectID = &proj.ID
	}
	room := &store.Room{
		ID:             newIDs("room"),
		Name:           b.Name,
		Kind:           b.Kind,
		HumanMember:    b.HumanMember == nil || *b.HumanMember,
		ProjectID:      b.ProjectID,
		MemberAgentIDs: b.AgentIDs,
		CreatedAt:      nowISO(),
		UpdatedAt:      nowISO(),
	}
	if err := s.st.InsertRoom(room); err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	created, _ := s.st.GetRoom(room.ID)
	roomChanged(s)
	writeJSON(w, 201, created)
}

func (s *Server) patchRoom(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	room, err := s.st.GetRoom(id)
	if err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	if room == nil {
		errJSON(w, 404, "Raum nicht gefunden")
		return
	}
	raw, rerr := readRawBody(r)
	if rerr != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	var b roomBody
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	if b.Name != "" {
		room.Name = b.Name
	}
	if b.Kind != "" {
		if b.Kind != "chat" && b.Kind != "project" {
			errJSON(w, 400, "kind muss chat oder project sein")
			return
		}
		// chat → project: auto-create the linked board if none (UX)
		if b.Kind == "project" && room.Kind != "project" && (room.ProjectID == nil || *room.ProjectID == "") {
			name := b.Name
			if name == "" {
				name = room.Name
			}
			proj := &store.Project{ID: newIDs("proj"), Name: name, CreatedAt: nowISO(), UpdatedAt: nowISO()}
			if err := s.st.InsertProject(proj); err != nil {
				log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
				return
			}
			room.ProjectID = &proj.ID
		}
		room.Kind = b.Kind
	}
	if fieldPresent(raw, "projectId") {
		room.ProjectID = b.ProjectID
	}
	if fieldPresent(raw, "humanMember") && b.HumanMember != nil {
		room.HumanMember = *b.HumanMember // owner in/out (du-drag)
	}
	// roster replacement on PATCH (review-2 M-1): explicit agentIds wins,
	// otherwise the loaded roster is preserved
	if fieldPresent(raw, "agentIds") && b.AgentIDs != nil {
		room.MemberAgentIDs = b.AgentIDs
	}
	room.UpdatedAt = nowISO()
	if err := s.st.UpdateRoom(room); err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	updated, _ := s.st.GetRoom(id)
	roomChanged(s)
	writeJSON(w, 200, updated)
}

func (s *Server) deleteRoom(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteRoom(r.PathValue("id")); err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	roomChanged(s)
	w.WriteHeader(204)
}

func (s *Server) addMember(w http.ResponseWriter, r *http.Request) {
	var b roomBody
	if err := decode(r, &b); err != nil || b.AgentID == "" {
		errJSON(w, 400, "agentId erforderlich")
		return
	}
	if err := s.st.AddRoomMember(r.PathValue("id"), b.AgentID); err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	roomChanged(s)
	writeJSON(w, 201, map[string]any{"ok": true})
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	if err := s.st.RemoveRoomMember(r.PathValue("id"), r.PathValue("agentId")); err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	roomChanged(s)
	w.WriteHeader(204)
}

// ── handlers: messages ──────────────────────────────────────────────

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	limit := 300
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			limit = n
		} // negative/NaN/garbage → default (review-2 codex M-5)
	}
	msgs, err := s.st.ListMessages(r.PathValue("id"), limit)
	if err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	if msgs == nil {
		msgs = []*store.Message{}
	}
	writeJSON(w, 200, msgs)
}

type messageBody struct {
	Content string `json:"content"`
}

func (s *Server) postMessage(w http.ResponseWriter, r *http.Request) {
	roomID := r.PathValue("id")
	room, err := s.st.GetRoom(roomID)
	if err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	if room == nil {
		errJSON(w, 404, "Raum nicht gefunden")
		return
	}
	var b messageBody
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	content := strings.TrimSpace(b.Content)
	if content == "" {
		errJSON(w, 400, "content erforderlich")
		return
	}
	m := &store.Message{
		ID:         newIDs("msg"),
		RoomID:     roomID,
		SenderName: humanName(),
		SenderKind: "human",
		Content:    content,
		Status:     "complete",
		CreatedAt:  nowISO(),
		UpdatedAt:  nowISO(),
	}
	if err := s.st.InsertMessage(m); err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	writeJSON(w, 201, m)

	// fan out to websocket + schedule agent turns
	writeEvt := map[string]any{"type": "message.created", "message": m}
	writeEvt["ts"] = time.Now().UnixMilli()
	s.broadcast(writeEvt)
	s.rt.OnUserMessage(m)
}

func humanName() string {
	name := env("RELAY_USER_NAME", "gaRON")
	return name
}

// env reads an env var with fallback.
func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}


func (s *Server) reorderRooms(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Order []string `json:"order"`
	}
	if err := decode(r, &body); err != nil || len(body.Order) == 0 {
		errJSON(w, 400, "order (Array von room-ids) erforderlich")
		return
	}
	if err := s.st.ReorderRooms(body.Order); err != nil {
		log.Printf("[api] reorder failed: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	roomChanged(s)
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ── handlers: tasks ─────────────────────────────────────────────────

type taskBody struct {
	ProjectID *string `json:"projectId"`
	Title     string  `json:"title"`
	Notes     string  `json:"notes"`
	Status    string  `json:"status"`
	Assignee  string  `json:"assignee"`
	Position  *int    `json:"position"`
}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.st.ListTasks(r.URL.Query().Get("projectId"))
	if err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	if tasks == nil {
		tasks = []*store.Task{}
	}
	writeJSON(w, 200, tasks)
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	var b taskBody
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	if strings.TrimSpace(b.Title) == "" {
		errJSON(w, 400, "title erforderlich")
		return
	}
	if b.Status == "" {
		b.Status = "todo"
	}
	pos := 0
	if b.Position != nil {
		pos = *b.Position
	}
	t := &store.Task{
		ID:        newIDs("task"),
		ProjectID: b.ProjectID,
		Title:     b.Title,
		Notes:     b.Notes,
		Status:    b.Status,
		Assignee:  b.Assignee,
		Position:  pos,
		CreatedAt: nowISO(),
		UpdatedAt: nowISO(),
	}
	if err := s.st.InsertTask(t); err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	writeJSON(w, 201, t)
}

func (s *Server) patchTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.st.GetTask(id)
	if err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	if existing == nil {
		errJSON(w, 404, "Task nicht gefunden")
		return
	}
	raw, rerr := readRawBody(r)
	if rerr != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	var b taskBody
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	if fieldPresent(raw, "projectId") {
		existing.ProjectID = b.ProjectID
	}
	if b.Title != "" {
		existing.Title = b.Title
	}
	if fieldPresent(raw, "notes") {
		existing.Notes = b.Notes
	}
	if b.Status != "" {
		existing.Status = b.Status
	}
	if fieldPresent(raw, "assignee") {
		existing.Assignee = b.Assignee
	}
	if b.Position != nil {
		existing.Position = *b.Position
	}
	existing.UpdatedAt = nowISO()
	if err := s.st.UpdateTask(existing); err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	writeJSON(w, 200, existing)
}

func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteTask(r.PathValue("id")); err != nil {
		log.Printf("[api] store error: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	w.WriteHeader(204)
}

// ── websocket ───────────────────────────────────────────────────────

func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	// Origin check, env-gated: browser WS-hijacking defense (review-2 M-4/H-5).
	// Default OFF because HA-Ingress rewrites Host (Origin != r.Host there);
	// set RELAY_WS_ORIGIN_REQUIRED=1 for plain-LAN hardening.
	if os.Getenv("RELAY_WS_ORIGIN_REQUIRED") == "1" && strings.HasPrefix(r.Header.Get("Origin"), "http") {
		origHost := func(raw string) string {
			u, err := url.Parse(raw)
			if err != nil {
				return raw
			}
			return u.Host
		}
		o := origHost(r.Header.Get("Origin"))
		if o != r.Host && o != (r.Header.Get("X-Forwarded-Host")) {
			http.Error(w, "forbidden origin", 403)
			return
		}
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	sub := &sub{conn: conn, out: make(chan []byte, 64), done: make(chan struct{})}
	s.mu.Lock()
	s.subs[sub] = true
	s.mu.Unlock()

	go s.writePump(sub)

	hello, _ := json.Marshal(map[string]any{"type": "hello", "ts": time.Now().UnixMilli()})
	select {
	case sub.out <- hello:
	default: // queue full right after connect: treat as dead client
		s.dropSub(sub, "queue full")
		return
	}

	// read pump: block until the client disconnects / gets dropped.
	for {
		if _, _, err := conn.Read(r.Context()); err != nil {
			break
		}
	}
	s.mu.Lock()
	_, present := s.subs[sub]
	delete(s.subs, sub)
	s.mu.Unlock()
	if present {
		close(sub.done)
		_ = conn.Close(websocket.StatusNormalClosure, "bye")
	}
}
// ── quick-wins: reactions + search ──────────────────────────────────

func (s *Server) addReaction(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Actor string `json:"actor"`
		Emoji string `json:"emoji"`
	}
	if err := decode(r, &b); err != nil || b.Actor == "" || b.Emoji == "" {
		errJSON(w, 400, "actor + emoji erforderlich")
		return
	}
	if err := s.st.AddReaction(r.PathValue("id"), b.Actor, b.Emoji); err != nil {
		log.Printf("[api] reaction add: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	s.broadcast(map[string]any{"type": "message.updated", "messageId": r.PathValue("id"), "ts": time.Now().UnixMilli()})
	writeJSON(w, 201, map[string]any{"ok": true})
}

func (s *Server) removeReaction(w http.ResponseWriter, r *http.Request) {
	actor := r.URL.Query().Get("actor")
	if err := s.st.RemoveReaction(r.PathValue("id"), actor, r.PathValue("emoji")); err != nil {
		log.Printf("[api] reaction remove: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	writeJSON(w, 204, nil)
}

func (s *Server) reactionsBatch(w http.ResponseWriter, r *http.Request) {
	var b struct {
		MessageIds []string `json:"messageIds"`
	}
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	out, err := s.st.ListReactions(b.MessageIds)
	if err != nil {
		log.Printf("[api] reactions batch: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	writeJSON(w, 200, out)
}

func (s *Server) searchMessages(w http.ResponseWriter, r *http.Request) {
	msgs, err := s.st.SearchMessages(r.PathValue("id"), r.URL.Query().Get("q"), 40)
	if err != nil {
		log.Printf("[api] search: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	if msgs == nil {
		msgs = []*store.Message{}
	}
	writeJSON(w, 200, msgs)
}
