package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
		errJSON(w, 500, err.Error())
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
		errJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, p)
}

func (s *Server) patchProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.st.GetProject(id)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if p == nil {
		errJSON(w, 404, "Projekt nicht gefunden")
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
	if b.Description != "" || hasJSON(r, "description") {
		p.Description = b.Description
	}
	if b.Color != "" {
		p.Color = b.Color
	}
	p.UpdatedAt = nowISO()
	if err := s.st.UpdateProject(p); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteProject(r.PathValue("id")); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

// ── handlers: rooms ─────────────────────────────────────────────────

type roomBody struct {
	Name      string   `json:"name"`
	ProjectID *string  `json:"projectId"`
	AgentIDs  []string `json:"agentIds"`
	// member endpoints:
	AgentID string `json:"agentId"`
}

func (s *Server) listRooms(w http.ResponseWriter, _ *http.Request) {
	rooms, err := s.st.ListRooms()
	if err != nil {
		errJSON(w, 500, err.Error())
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
	room := &store.Room{
		ID:             newIDs("room"),
		Name:           b.Name,
		ProjectID:      b.ProjectID,
		MemberAgentIDs: b.AgentIDs,
		CreatedAt:      nowISO(),
		UpdatedAt:      nowISO(),
	}
	if err := s.st.InsertRoom(room); err != nil {
		errJSON(w, 500, err.Error())
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
		errJSON(w, 500, err.Error())
		return
	}
	if room == nil {
		errJSON(w, 404, "Raum nicht gefunden")
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
	if hasJSON(r, "projectId") {
		room.ProjectID = b.ProjectID
	}
	room.UpdatedAt = nowISO()
	if err := s.st.UpdateRoom(room); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	updated, _ := s.st.GetRoom(id)
	roomChanged(s)
	writeJSON(w, 200, updated)
}

func (s *Server) deleteRoom(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteRoom(r.PathValue("id")); err != nil {
		errJSON(w, 500, err.Error())
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
		errJSON(w, 500, err.Error())
		return
	}
	roomChanged(s)
	writeJSON(w, 201, map[string]any{"ok": true})
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	if err := s.st.RemoveRoomMember(r.PathValue("id"), r.PathValue("agentId")); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	roomChanged(s)
	w.WriteHeader(204)
}

// ── handlers: messages ──────────────────────────────────────────────

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	limit := 300
	if v := r.URL.Query().Get("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	msgs, err := s.st.ListMessages(r.PathValue("id"), limit)
	if err != nil {
		errJSON(w, 500, err.Error())
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
		errJSON(w, 500, err.Error())
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
		errJSON(w, 500, err.Error())
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
		errJSON(w, 500, err.Error())
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
		errJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, t)
}

func (s *Server) patchTask(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.st.GetTask(id)
	if err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	if existing == nil {
		errJSON(w, 404, "Task nicht gefunden")
		return
	}
	var b taskBody
	if err := decode(r, &b); err != nil {
		errJSON(w, 400, "ungültiges JSON")
		return
	}
	if hasJSON(r, "projectId") {
		existing.ProjectID = b.ProjectID
	}
	if b.Title != "" {
		existing.Title = b.Title
	}
	if hasJSON(r, "notes") {
		existing.Notes = b.Notes
	}
	if b.Status != "" {
		existing.Status = b.Status
	}
	if hasJSON(r, "assignee") {
		existing.Assignee = b.Assignee
	}
	if b.Position != nil {
		existing.Position = *b.Position
	}
	existing.UpdatedAt = nowISO()
	if err := s.st.UpdateTask(existing); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, existing)
}

func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DeleteTask(r.PathValue("id")); err != nil {
		errJSON(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
}

// ── websocket ───────────────────────────────────────────────────────

func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.subs[conn] = true
	s.mu.Unlock()

	hello, _ := json.Marshal(map[string]any{"type": "hello", "ts": time.Now().UnixMilli()})
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	_ = conn.Write(ctx, websocket.MessageText, hello)
	cancel()

	// keep read pump for close/Pings; we never expect data frames
	for {
		if _, _, err := conn.Read(r.Context()); err != nil {
			break
		}
	}
	s.mu.Lock()
	delete(s.subs, conn)
	s.mu.Unlock()
	_ = conn.Close(websocket.StatusNormalClosure, "bye")
}