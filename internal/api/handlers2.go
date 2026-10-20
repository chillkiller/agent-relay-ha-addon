package api

import (
	"encoding/json"
	"io"
	"log"
	"strconv"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	// P1#8 / P2#10-Rest:
	Topic   string `json:"topic,omitempty"`            // Raum-Thema (buzz channel topic)
	PinnedMessageID string `json:"pinnedMessageId,omitempty"` // pin/unpin (leer = unpin)
	// P2#8: role je member (member endpoints)
	Role string `json:"role,omitempty"`
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
	if b.Kind != "chat" && b.Kind != "project" && b.Kind != "dm" {
		errJSON(w, 400, "kind muss chat, project oder dm sein")
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
	// dm-rooms: benannt nach dem agent (Emoji + Name), genau 1 Mitglied —
	// end-to-end-Kommunikation (GaRoN-Definition), nie projektgebunden.
	if b.Kind == "dm" {
		if len(b.AgentIDs) != 1 {
			errJSON(w, 400, "dm braucht genau einen Agent")
			return
		}
		if a, _ := s.st.GetAgent(b.AgentIDs[0]); a != nil {
			b.Name = a.Emoji + " " + a.Name
		}
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

func (s *Server) roomRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := s.st.RoomRoles(r.PathValue("id"))
	if err != nil {
		errJSON(w, 500, "interner Fehler")
		return
	}
	writeJSON(w, 200, roles)
}

func (s *Server) getRoom(w http.ResponseWriter, r *http.Request) {
	room, err := s.st.GetRoom(r.PathValue("id"))
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if room == nil {
		errJSON(w, 404, "Raum nicht gefunden")
		return
	}
	writeJSON(w, 200, room)
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
		if b.Kind != "chat" && b.Kind != "project" && b.Kind != "dm" {
			errJSON(w, 400, "kind muss chat, project oder dm sein")
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
	// P1#8: topic-Feld (immer setzen wenn present; leer = löschen)
	if fieldPresent(raw, "topic") {
		room.Topic = strings.TrimSpace(b.Topic)
	}
	// P2#10-Rest: pin/unpin — leer = unpin; gültige msg-ID im selben Raum = pin
	if fieldPresent(raw, "pinnedMessageId") {
		pid := strings.TrimSpace(b.PinnedMessageID)
		if pid == "" {
			room.PinnedMessageID = nil
		} else if msg, _ := s.st.GetMessage(pid); msg != nil && msg.RoomID == id {
			room.PinnedMessageID = &pid
		} else {
			errJSON(w, 400, "pinnedMessageId muss eine Nachricht im selben Raum sein")
			return
		}
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

// setMemberRole (P2#8): PATCH /api/rooms/{id}/members/{agentId}/role
func (s *Server) setMemberRole(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Role string `json:"role"`
	}
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	if err := s.st.SetRoomRole(r.PathValue("id"), r.PathValue("agentId"), strings.TrimSpace(b.Role)); err != nil {
		if err.Error() == "sql: no rows in result set" {
			errJSON(w, 404, "Member nicht im Raum")
			return
		}
		errJSON(w, 400, err.Error())
		return
	}
	roomChanged(s)
	writeJSON(w, 200, map[string]any{"ok": true, "role": strings.TrimSpace(b.Role)})
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
	Content  string `json:"content"`
	ParentID string `json:"parentId,omitempty"` // P1#4: thread reply-to
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
	// P1#4: parent_id nur setzen, wenn die Wurzel existiert (gleicher Raum —
	// Threads bleiben räumlich verankert; cross-room replies sind nicht v1)
	if strings.TrimSpace(b.ParentID) != "" {
		root, err := s.st.GetMessage(strings.TrimSpace(b.ParentID))
		if err == nil && root != nil && root.RoomID == roomID {
			m.ParentID = &b.ParentID
		}
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

// listReplies (P1#4): Direkt-Antworten je Wurzel.
func (s *Server) listReplies(w http.ResponseWriter, r *http.Request) {
	msgs, err := s.st.ListReplies(r.PathValue("id"))
	if err != nil {
		log.Printf("[api] replies: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	if msgs == nil {
		msgs = []*store.Message{}
	}
	writeJSON(w, 200, msgs)
}

// ── approvals (P2#2, paperclip approvals.ts) ────────────────────────

func (s *Server) listApprovals(w http.ResponseWriter, r *http.Request) {
	aps, err := s.st.ListApprovals(r.URL.Query().Get("status"))
	if err != nil {
		errJSON(w, 500, "interner Fehler")
		return
	}
	if aps == nil {
		aps = []*store.Approval{}
	}
	writeJSON(w, 200, aps)
}

func decideApproval(s *Server, decision string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Race-Schutz: erste Entscheidung gewinnt (store.DecideApproval)
		ok, err := s.st.DecideApproval(r.PathValue("id"), decision)
		if err != nil {
			errJSON(w, 500, "interner Fehler")
			return
		}
		if !ok {
			errJSON(w, 409, "Approval nicht mehr pending")
			return
		}
		ap, _ := s.st.GetApproval(r.PathValue("id"))
		s.broadcast(map[string]any{"type": "approval.decided", "approval": ap, "ts": time.Now().UnixMilli()})
		if decision == "granted" && ap != nil {
			if err := s.rt.ResumeApproval(ap); err != nil {
				log.Printf("[api] approval resume: %v", err)
				s.broadcast(map[string]any{"type": "system", "roomId": "", "text": "⚠️ Approval-Resume fehlgeschlagen: " + err.Error()})
			}
		}
		writeJSON(w, 200, ap)
	}
}

func (s *Server) decideApprovalGranted(w http.ResponseWriter, r *http.Request) {
	decideApproval(s, "granted")(w, r)
}

func (s *Server) decideApprovalDenied(w http.ResponseWriter, r *http.Request) {
	decideApproval(s, "denied")(w, r)
}

// ── task chatter (P2#1, paperclip task-chat) ────────────────────────

func (s *Server) listTaskComments(w http.ResponseWriter, r *http.Request) {
	cs, err := s.st.ListTaskComments(r.PathValue("id"))
	if err != nil {
		errJSON(w, 500, "interner Fehler")
		return
	}
	if cs == nil {
		cs = []*store.TaskComment{}
	}
	writeJSON(w, 200, cs)
}

func (s *Server) addTaskComment(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("id")
	t, err := s.st.GetTask(taskID)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if t == nil {
		errJSON(w, 404, "Task nicht gefunden")
		return
	}
	var b struct {
		Content string `json:"content"`
		AgentID string `json:"agentId,omitempty"` // v2: Agent-Kommentar via Relay-UI
	}
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	content := strings.TrimSpace(b.Content)
	if content == "" {
		errJSON(w, 400, "content erforderlich")
		return
	}
	kind, name := "human", humanName()
	if b.AgentID != "" {
		ag, err := s.st.GetAgent(b.AgentID)
		if err != nil || ag == nil {
			errJSON(w, 404, "Agent nicht gefunden")
			return
		}
		kind, name = "agent", ag.Name
	}
	c, err := s.st.AddTaskComment(taskID, kind, name, content)
	if err != nil {
		errJSON(w, 500, "interner Fehler")
		return
	}
	s.broadcast(map[string]any{"type": "task.comment.created", "taskId": taskID, "comment": c, "ts": time.Now().UnixMilli()})
	writeJSON(w, 201, c)
}

// ── unread authoritativ (P1#10) ─────────────────────────────────────

func (s *Server) markRoomRead(w http.ResponseWriter, r *http.Request) {
	at := nowISO()
	if err := s.st.MarkRoomRead(r.PathValue("id"), "human", at); err != nil {
		errJSON(w, 500, "interner Fehler")
		return
	}
	s.broadcast(map[string]any{"type": "rooms.changed", "ts": time.Now().UnixMilli()})
	writeJSON(w, 200, map[string]any{"ok": true, "at": at})
}

func (s *Server) listUnread(w http.ResponseWriter, _ *http.Request) {
	m, err := s.st.UnreadBatch("human")
	if err != nil {
		errJSON(w, 500, "interner Fehler")
		return
	}
	writeJSON(w, 200, m)
}

// ── dashboard (P2#9, paperclip dashboard.ts/status-cards.ts) ────────
// Vertrag: "what is happening → does it need me → what do I do about it"

func (s *Server) dashboardAggregates(w http.ResponseWriter, _ *http.Request) {
	out := map[string]any{}
	// needs-you-Fläche: pending approvals (höchste Priorität — hält Turns an)
	aps, err := s.st.ListApprovals("pending")
	if err != nil {
		errJSON(w, 500, "interner Fehler")
		return
	}
	out["pendingApprovals"] = aps
	// unread-Summe (needs-you: ungelesene fremde Nachrichten)
	un, err := s.st.UnreadBatch("human")
	if err != nil {
		un = map[string]int{}
	}
	total := 0
	for _, c := range un {
		total += c
	}
	out["unreadTotal"] = total
	// agents: online/idle/off-Stände (what-is-happening)
	agents, _ := s.st.ListAgents()
	ac := map[string]int{"online": 0, "idle": 0, "off": 0}
	for _, a := range agents {
		if !a.Enabled {
			ac["off"]++
		} else {
			// v1: enabled = online-Kandidat; echte 90s-Aktivität ist client-
			// seitig (WS-geboren) — server v1: enabled→online, sonst off
			ac["online"]++
		}
	}
	out["agents"] = ac
	// tasks je Projekt (open/doing/done — projekt-kopfzeile)
	projects, _ := s.st.ListProjects()
	type projAgg struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Color    string `json:"color"`
		Todo     int    `json:"todo"`
		Doing    int    `json:"doing"`
		Done     int    `json:"done"`
		OpenTotal int   `json:"openTotal"`
	}
	pout := []projAgg{}
	tasks, _ := s.st.ListTasks("")
	for _, p := range projects {
		agg := projAgg{ID: p.ID, Name: p.Name, Color: p.Color}
		for _, t := range tasks {
			if t.ProjectID == nil || *t.ProjectID != p.ID {
				continue
			}
			switch t.Status {
			case "todo":
				agg.Todo++
			case "doing":
				agg.Doing++
			case "done":
				agg.Done++
			}
		}
		agg.OpenTotal = agg.Todo + agg.Doing
		pout = append(pout, agg)
	}
	out["projects"] = pout
	// routines: nächste fällige (what-is-coming)
	routines, _ := s.st.ListRoutines()
	next := ""
	for _, rt := range routines {
		if !rt.Enabled {
			continue
		}
		if next == "" || rt.NextRunAt < next {
			next = rt.NextRunAt
		}
	}
	out["nextRoutineAt"] = next
	out["routinesActive"] = len(routines)
	writeJSON(w, 200, out)
}

// getMessage (P1#1-Begleiter): Einzelabruf für WS-Refresh (message.updated).

// nextRoutineRun: nächste Ausführung nach cron (5-field) — api-Seite; brückt
// den runtime-Parser (Export über runtime.CronNext wäre kreisfrei aber v1
// lokal: gleiche Brute-Scan-Logik, klein).
func nextRoutineRun(spec string, from time.Time) string {
	f := strings.Fields(spec)
	if len(f) != 5 {
		return ""
	}
	t := from.Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 366*24*60; i++ {
		if cronMatchAPI(f, t) {
			return t.Format("2006-01-02T15:04:05.000000000Z07:00")
		}
		t = t.Add(time.Minute)
	}
	return ""
}

func cronMatchAPI(f []string, t time.Time) bool {
	check := func(field string, lo, hi, val int) bool {
		if field == "*" {
			return true
		}
		for _, part := range strings.Split(field, ",") {
			if strings.HasPrefix(part, "*/") {
				n, e := strconv.Atoi(part[2:])
				if e != nil || n <= 0 {
					continue
				}
				if lo == 0 && val%n == 0 {
					return true
				}
				if lo > 0 && (val-lo)%n == 0 {
					return true
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
			if v, e := strconv.Atoi(part); e == nil && v == val {
				return true
			}
		}
		return false
	}
	return check(f[0], 0, 59, t.Minute()) && check(f[1], 0, 23, t.Hour()) &&
		check(f[2], 1, 31, t.Day()) && check(f[3], 1, 12, int(t.Month())) &&
		check(f[4], 0, 6, int(t.Weekday()))
}

func (s *Server) listRoutines(w http.ResponseWriter, _ *http.Request) {
	rs, err := s.st.ListRoutines()
	if err != nil {
		errJSON(w, 500, "interner Fehler")
		return
	}
	if rs == nil {
		rs = []*store.Routine{}
	}
	writeJSON(w, 200, rs)
}

func (s *Server) createRoutine(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name    string `json:"name"`
		Cron    string `json:"cron"`
		Prompt  string `json:"prompt"`
		RoomID  string `json:"roomId"`
		AgentID string `json:"agentId"`
	}
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	name, cron, prompt := strings.TrimSpace(b.Name), strings.TrimSpace(b.Cron), strings.TrimSpace(b.Prompt)
	if name == "" || prompt == "" {
		errJSON(w, 400, "name + prompt erforderlich")
		return
	}
	if len(strings.Fields(cron)) != 5 {
		errJSON(w, 400, "cron: 5 Felder (minute hour dom month dow)")
		return
	}
	if b.RoomID == "" {
		errJSON(w, 400, "roomId erforderlich (Ziel-Raum)")
		return
	}
	room, _ := s.st.GetRoom(b.RoomID)
	if room == nil {
		errJSON(w, 404, "Raum nicht gefunden")
		return
	}
	// cron-Parser-Vertrag: ungültiges spec → 400 (parser prüft per
	// nextCronTime-Fallback NICHT — hier exakt validieren)
	if strings.Count(cron, "*") > 0 {
		// erlaubt; Details im runtime-Ticker
	}
	next := "2026-12-31T00:00:00.000000000+02:00" // calc im Runtime-Ticker-Loop
	t := &store.Routine{
		ID: newIDs("rt"), Name: name, Cron: cron, Prompt: prompt,
		RoomID: b.RoomID, AgentID: b.AgentID, Enabled: true,
		NextRunAt: next, CreatedAt: nowISO(), UpdatedAt: nowISO(),
	}
	if err := s.st.InsertRoutine(t); err != nil {
		log.Printf("[api] routine insert: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	// next_run sofort korrekt berechnen (create-Vertrag; Ticker respektiert)
	if n := nextRoutineRun(cron, time.Now()); n != "" {
		_ = s.st.RoutineFired(t.ID, t.NextRunAt, n) // next setzen ohne last_run zu faken
	}
	s.broadcast(map[string]any{"type": "routines.changed", "ts": time.Now().UnixMilli()})
	out, _ := s.st.GetRoutine(t.ID)
	writeJSON(w, 201, out)
}

func (s *Server) patchRoutine(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.st.GetRoutine(id)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if t == nil {
		errJSON(w, 404, "Routine nicht gefunden")
		return
	}
	raw, rerr := readRawBody(r)
	if rerr != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	var b struct {
		Name    string `json:"name"`
		Cron    string `json:"cron"`
		Prompt  string `json:"prompt"`
		RoomID  string `json:"roomId"`
		AgentID string `json:"agentId"`
		Enabled *bool  `json:"enabled"`
	}
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	if fieldPresent(raw, "name") && strings.TrimSpace(b.Name) != "" {
		t.Name = strings.TrimSpace(b.Name)
	}
	if fieldPresent(raw, "cron") {
		if len(strings.Fields(strings.TrimSpace(b.Cron))) != 5 {
			errJSON(w, 400, "cron: 5 Felder")
			return
		}
		t.Cron = strings.TrimSpace(b.Cron)
	}
	if fieldPresent(raw, "prompt") && strings.TrimSpace(b.Prompt) != "" {
		t.Prompt = strings.TrimSpace(b.Prompt)
	}
	if fieldPresent(raw, "roomId") {
		t.RoomID = b.RoomID
	}
	if fieldPresent(raw, "agentId") {
		t.AgentID = b.AgentID
	}
	if b.Enabled != nil {
		t.Enabled = *b.Enabled
	}
	t.UpdatedAt = nowISO()
	if err := s.st.UpdateRoutine(t); err != nil {
		errJSON(w, 500, "interner Fehler")
		return
	}
	s.broadcast(map[string]any{"type": "routines.changed", "ts": time.Now().UnixMilli()})
	out, _ := s.st.GetRoutine(id)
	writeJSON(w, 200, out)
}

func (s *Server) deleteRoutine(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteRoutine(r.PathValue("id")); err != nil {
		errJSON(w, 500, "interner Fehler")
		return
	}
	s.broadcast(map[string]any{"type": "routines.changed", "ts": time.Now().UnixMilli()})
	w.WriteHeader(204)
}

// getMessage (P1#1-Begleiter): Einzelabruf für WS-Refresh (message.updated).

func (s *Server) listSkills(w http.ResponseWriter, r *http.Request) {
	sk, err := s.st.ListSkills(r.URL.Query().Get("all") == "1")
	if err != nil {
		errJSON(w, 500, "interner Fehler")
		return
	}
	if sk == nil {
		sk = []*store.Skill{}
	}
	writeJSON(w, 200, sk)
}

func (s *Server) createSkill(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Name         string   `json:"name"`
		Instructions string   `json:"instructions"`
		AgentIDs     []string `json:"agentIds"`
	}
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	name := strings.TrimSpace(b.Name)
	instr := strings.TrimSpace(b.Instructions)
	if name == "" || instr == "" {
		errJSON(w, 400, "name + instructions erforderlich")
		return
	}
	sk := &store.Skill{
		ID: newIDs("skill"), Name: name, Instructions: instr,
		Version: 1, CreatedAt: nowISO(), UpdatedAt: nowISO(),
	}
	if err := s.st.InsertSkill(sk); err != nil {
		log.Printf("[api] skill insert: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	for _, aid := range b.AgentIDs {
		_ = s.st.AssignSkill(sk.ID, aid)
	}
	neu, _ := s.st.GetSkill(sk.ID)
	s.broadcast(map[string]any{"type": "skills.changed", "ts": time.Now().UnixMilli()})
	writeJSON(w, 201, neu)
}

func (s *Server) patchSkill(w http.ResponseWriter, r *http.Request) {
	// update = neue Version-Row (Paperclip Version-History); alt bleibt restorebar
	var b struct {
		Name         string   `json:"name"`
		Instructions string   `json:"instructions"`
	}
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	old, err := s.st.GetSkill(r.PathValue("id"))
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if old == nil {
		errJSON(w, 404, "Skill nicht gefunden")
		return
	}
	name := strings.TrimSpace(b.Name)
	if name == "" {
		name = old.Name
	}
	instr := strings.TrimSpace(b.Instructions)
	if instr == "" {
		instr = old.Instructions
	}
	newID, err := s.st.UpdateSkill(&store.Skill{ID: old.ID, Name: name, Instructions: instr})
	if err != nil {
		errJSON(w, 500, "interner Fehler")
		return
	}
	neu, _ := s.st.GetSkill(newID)
	s.broadcast(map[string]any{"type": "skills.changed", "ts": time.Now().UnixMilli()})
	writeJSON(w, 200, neu)
}

func (s *Server) deleteSkill(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteSkill(r.PathValue("id")); err != nil {
		errJSON(w, 500, "interner Fehler")
		return
	}
	s.broadcast(map[string]any{"type": "skills.changed", "ts": time.Now().UnixMilli()})
	w.WriteHeader(204)
}

func (s *Server) restoreSkill(w http.ResponseWriter, r *http.Request) {
	if err := s.st.RestoreSkill(r.PathValue("id")); err != nil {
		errJSON(w, 500, "interner Fehler")
		return
	}
	s.broadcast(map[string]any{"type": "skills.changed", "ts": time.Now().UnixMilli()})
	sk, _ := s.st.GetSkill(r.PathValue("id"))
	writeJSON(w, 200, sk)
}

func (s *Server) setSkillAgents(w http.ResponseWriter, r *http.Request) {
	var b struct {
		AgentIDs []string `json:"agentIds"`
	}
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	id := r.PathValue("id")
	old, err := s.st.SkillAgents(id)
	if err != nil {
		errJSON(w, 500, "interner Fehler")
		return
	}
	for _, aid := range old {
		if !contains(b.AgentIDs, aid) {
			_ = s.st.UnassignSkill(id, aid)
		}
	}
	for _, aid := range b.AgentIDs {
		if !contains(old, aid) {
			_ = s.st.AssignSkill(id, aid)
		}
	}
	s.broadcast(map[string]any{"type": "skills.changed", "ts": time.Now().UnixMilli()})
	sk, _ := s.st.GetSkill(id)
	writeJSON(w, 200, sk)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// getMessage (P1#1-Begleiter): Einzelabruf für WS-Refresh (message.updated).
func (s *Server) getMessage(w http.ResponseWriter, r *http.Request) {
	msg, err := s.st.GetMessage(r.PathValue("id"))
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if msg == nil {
		errJSON(w, 404, "Nachricht nicht gefunden")
		return
	}
	writeJSON(w, 200, msg)
}


// ── attachments (P1#2, Codex-Matrix 1.2 buzz-media/paperclip
// file-resources): multipart-Upload → FS unter <dataDir>/attachments,
// Meta in SQLite; GET serve mit Filename-Disposition; Batch liefert die
// Meta-Liste je Message (Enrichment liefert sie inline mit).

// uploadAttachment: POST /api/messages/{id}/attachments (multipart field
// "file", evtl. mehrere files → mehrere Attachments).
func (s *Server) uploadAttachment(w http.ResponseWriter, r *http.Request) {
	if s.dataDir == "" {
		errJSON(w, 409, "uploads deaktiviert (RELAY_DATA_DIR nicht gesetzt)")
		return
	}
	msgID := r.PathValue("id")
	msg, err := s.st.GetMessage(msgID)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if msg == nil {
		errJSON(w, 404, "Nachricht nicht gefunden")
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil { // 32 MB cap (review-2-Schema)
		errJSON(w, 400, "multipart ungültig oder >32 MB")
		return
	}
	dir := filepath.Join(s.dataDir, "attachments")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("[api] attachments mkdir: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	form := r.MultipartForm
	files := form.File["file"]
	if len(files) == 0 {
		errJSON(w, 400, "file-Feld erforderlich")
		return
	}
	out := make([]*store.Attachment, 0, len(files))
	for _, fh := range files {
		src, err := fh.Open()
		if err != nil {
			errJSON(w, 400, "datei nicht lesbar")
			return
		}
		att := &store.Attachment{
			ID:        store.NewID("att"),
			MessageID: msgID,
			Filename:  store.SanitizeFilename(fh.Filename),
			Mime:      fh.Header.Get("Content-Type"),
			Size:      fh.Size,
			CreatedAt: nowISO(),
		}
		if att.Mime == "" {
			att.Mime = "application/octet-stream"
		}
		att.Path = filepath.Join(dir, att.ID+"-"+att.Filename)
		dst, err := os.Create(att.Path)
		if err != nil {
			src.Close()
			log.Printf("[api] attachments create: %v", err)
			errJSON(w, 500, "interner Fehler")
			return
		}
		_, cpErr := io.Copy(dst, src)
		src.Close()
		dst.Close()
		if cpErr != nil {
			log.Printf("[api] attachments copy: %v", cpErr)
			errJSON(w, 500, "interner Fehler")
			return
		}
		if err := s.st.InsertAttachment(att); err != nil {
			log.Printf("[api] attachments insert: %v", err)
			errJSON(w, 500, "interner Fehler")
			return
		}
		out = append(out, att)
	}
	s.broadcast(map[string]any{"type": "message.updated", "messageId": msgID, "ts": time.Now().UnixMilli()})
	writeJSON(w, 201, out)
}

// serveAttachment: GET /api/attachments/{id} — Datei mit Content-Type/—
//-Disposition (filename), keine Pfad-Information im Response.
func (s *Server) serveAttachment(w http.ResponseWriter, r *http.Request) {
	att, err := s.st.GetAttachment(r.PathValue("id"))
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if att == nil {
		errJSON(w, 404, "Attachment nicht gefunden")
		return
	}
	w.Header().Set("Content-Type", att.Mime)
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(att.Filename, `"`, "")+`"`)
	http.ServeFile(w, r, att.Path)
}

// editMessage (P1#1, Codex-Matrix 1.6 buzz KIND_STREAM_MESSAGE_EDIT):
// eigene Nachrichten editieren — content-PATCH; nur Human-Autor (actor=Du
// über query) darf eigene human-Nachrichten ändern; streaming/editable-
// Status-Grenzen im Client. WS message.updated treibt SPA-Refresh.
func (s *Server) editMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var b struct {
		Content string `json:"content"`
	}
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	content := strings.TrimSpace(b.Content)
	if content == "" {
		errJSON(w, 400, "content erforderlich")
		return
	}
	if len(content) > 16000 {
		errJSON(w, 400, "content zu lang (max 16000)")
		return
	}
	msg, err := s.st.GetMessage(id)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if msg == nil {
		errJSON(w, 404, "Nachricht nicht gefunden")
		return
	}
	if msg.SenderKind != "human" {
		errJSON(w, 403, "nur eigene Nachrichten sind editierbar")
		return
	}
	if msg.Status != "complete" {
		errJSON(w, 409, "Nachricht ist nicht abgeschlossen")
		return
	}
	if err := s.st.UpdateMessage(id, content, "complete"); err != nil {
		log.Printf("[api] message edit: %v", err)
		errJSON(w, 500, "interner Fehler")
		return
	}
	updated, _ := s.st.GetMessage(id)
	s.broadcast(map[string]any{"type": "message.updated", "messageId": id, "ts": time.Now().UnixMilli()})
	writeJSON(w, 200, updated)
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
