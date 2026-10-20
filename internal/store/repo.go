package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// ── settings ────────────────────────────────────────────────────────

var defaultSettings = map[string]any{
	"relayName": "Agent Relay",
	"theme":     "dark",
	// OpenClaw / LLM gateway defaults (OpenAI-compatible endpoint).
	"openclawBaseUrl": "", // e.g. http://127.0.0.1:18789/v1
	"openclawToken":   "",
}

type Settings map[string]any

func (d *DB) GetSettings() Settings {
	out := map[string]any{"relayName": nil, "theme": nil, "openclawBaseUrl": nil, "openclawToken": nil}
	for k, v := range defaultSettings {
		out[k] = v
	}
	rows, err := d.db.Query(`SELECT key, value FROM settings`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var k string
			var raw string
			if rows.Scan(&k, &raw) == nil {
				if _, ok := out[k]; ok {
					out[k] = raw
				}
			}
		}
	}
	return Settings(out)
}

func (d *DB) UpdateSetting(key string, value string) error {
	_, err := d.db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err // surfaced to the caller (review L-3)
}

// ── agents ──────────────────────────────────────────────────────────

func scanAgent(r rowScanner) (*Agent, error) {
	a := &Agent{}
	var model, baseURL, apiKey sql.NullString
	if err := r.Scan(&a.ID, &a.Name, &a.Emoji, &a.Color, &a.SystemPrompt, &a.Adapter,
		&model, &baseURL, &apiKey, &a.Enabled, &a.ApprovalGate, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}
	a.Model, a.BaseURL, a.APIKey = model.String, baseURL.String, apiKey.String
	return a, nil
}

func (d *DB) ListAgents() ([]*Agent, error) {
	rows, err := d.db.Query(`SELECT id,name,emoji,color,system_prompt,adapter,model,base_url,api_key,enabled,approval_gate,created_at,updated_at FROM agents ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (d *DB) GetAgent(id string) (*Agent, error) {
	r := d.db.QueryRow(`SELECT id,name,emoji,color,system_prompt,adapter,model,base_url,api_key,enabled,approval_gate,created_at,updated_at FROM agents WHERE id=?`, id)
	a, err := scanAgent(r)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func agentInsertValues(a *Agent) []any {
	return []any{a.ID, a.Name, a.Emoji, a.Color, a.SystemPrompt, a.Adapter,
		nullable(a.Model), nullable(a.BaseURL), nullable(a.APIKey), boolInt(a.Enabled), boolInt(a.ApprovalGate), a.CreatedAt, a.UpdatedAt}
}

func (d *DB) InsertAgent(a *Agent) error {
	_, err := d.db.Exec(`INSERT INTO agents
	(id,name,emoji,color,system_prompt,adapter,model,base_url,api_key,enabled,approval_gate,created_at,updated_at)
	VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, agentInsertValues(a)...)
	return err
}

func (d *DB) UpdateAgent(a *Agent) error {
	_, err := d.db.Exec(`UPDATE agents SET name=?,emoji=?,color=?,system_prompt=?,adapter=?,model=?,base_url=?,api_key=?,enabled=?,approval_gate=?,updated_at=? WHERE id=?`,
		a.Name, a.Emoji, a.Color, a.SystemPrompt, a.Adapter, nullable(a.Model), nullable(a.BaseURL), nullable(a.APIKey), boolInt(a.Enabled), boolInt(a.ApprovalGate), now(), a.ID)
	return err
}

func (d *DB) DeleteAgent(id string) error {
	_, err := d.db.Exec(`DELETE FROM agents WHERE id=?`, id)
	return err
}

// ── reactions (quick-win 1, audit P1) ────────────────────────────────

func (d *DB) AddReaction(messageID, actor, emoji string) error {
	_, err := d.db.Exec(`INSERT OR IGNORE INTO reactions (id,message_id,actor,emoji,created_at) VALUES (?,?,?,?,?)`,
		NewID("react"), messageID, actor, emoji, now())
	return err
}

func (d *DB) RemoveReaction(messageID, actor, emoji string) error {
	_, err := d.db.Exec(`DELETE FROM reactions WHERE message_id=? AND actor=? AND emoji=?`,
		messageID, actor, emoji)
	return err
}

type ReactionSummary struct {
	Emoji  string   `json:"emoji"`
	Count  int      `json:"count"`
	Actors []string `json:"actors"`
}

func (d *DB) ListReactions(messageIDs []string) (map[string][]ReactionSummary, error) {
	out := map[string][]ReactionSummary{}
	if len(messageIDs) == 0 {
		return out, nil
	}
	ph := make([]string, len(messageIDs))
	args := make([]any, 0, len(messageIDs))
	for i, id := range messageIDs {
		ph[i] = "?"
		args = append(args, id)
	}
	rows, err := d.db.Query(`SELECT message_id, emoji, actor FROM reactions
		WHERE message_id IN (`+strings.Join(ph, ",")+`) ORDER BY created_at`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type pr struct{ actor, emoji string }
	byMsg := map[string][]pr{}
	for rows.Next() {
		var mid, emoji, actor string
		if err := rows.Scan(&mid, &emoji, &actor); err != nil {
			return nil, err
		}
		byMsg[mid] = append(byMsg[mid], pr{actor, emoji})
	}
	for mid, prs := range byMsg {
		idx := map[string]int{}
		var sums []ReactionSummary
		for _, pg := range prs {
			i, exists := idx[pg.emoji]
			if !exists {
				i = len(sums)
				idx[pg.emoji] = i
				sums = append(sums, ReactionSummary{Emoji: pg.emoji})
			}
			sums[i].Count++
			sums[i].Actors = append(sums[i].Actors, pg.actor)
		}
		out[mid] = sums
	}
	return out, nil
}

// SearchMessages: LIKE-Volltextuche im Raum (quick-win 2; robust ohne FTS5-Abhängigkeit).
func (d *DB) SearchMessages(roomID, query string, limit int) ([]*Message, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	rows, err := d.db.Query(`
		SELECT id,room_id,agent_id,sender_name,sender_kind,content,status,created_at,updated_at
		FROM messages
		WHERE room_id = ? AND content LIKE ?
		ORDER BY created_at DESC LIMIT ?`,
		roomID, "%"+q+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ReorderRooms persists manual room order (drag&drop, review-7 B3).
func (d *DB) ReorderRooms(ids []string) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE rooms SET position=?, updated_at=? WHERE id=?`, i, now(), id); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// ── projects ────────────────────────────────────────────────────────

func (d *DB) ListProjects() ([]*Project, error) {
	rows, err := d.db.Query(`SELECT id,name,description,color,created_at,updated_at FROM projects ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Project
	for rows.Next() {
		p := &Project{}
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Color, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) GetProject(id string) (*Project, error) {
	p := &Project{}
	r := d.db.QueryRow(`SELECT id,name,description,color,created_at,updated_at FROM projects WHERE id=?`, id)
	err := r.Scan(&p.ID, &p.Name, &p.Description, &p.Color, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (d *DB) InsertProject(p *Project) error {
	_, err := d.db.Exec(`INSERT INTO projects (id,name,description,color,created_at,updated_at) VALUES (?,?,?,?,?,?)`,
		p.ID, p.Name, p.Description, p.Color, p.CreatedAt, p.UpdatedAt)
	return err
}

func (d *DB) UpdateProject(p *Project) error {
	_, err := d.db.Exec(`UPDATE projects SET name=?,description=?,color=?,updated_at=? WHERE id=?`,
		p.Name, p.Description, p.Color, now(), p.ID)
	return err
}

func (d *DB) DeleteProject(id string) error {
	_, err := d.db.Exec(`DELETE FROM projects WHERE id=?`, id)
	return err
}

// ── rooms ───────────────────────────────────────────────────────────

func (d *DB) ListRooms() ([]*Room, error) {
	rows, err := d.db.Query(`SELECT id,name,kind,human_member,position,pinned_message_id,topic,project_id,created_at,updated_at,
			COALESCE((SELECT content FROM messages m WHERE m.room_id = rooms.id AND m.status != 'deleted' ORDER BY created_at DESC LIMIT 1), '')
			FROM rooms ORDER BY position,created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Room
	byID := map[string]*Room{}
	for rows.Next() {
		r := &Room{}
		var pid, pinned sql.NullString
		var hm, pos int
		var lastMsg, topic string
		if err := rows.Scan(&r.ID, &r.Name, &r.Kind, &hm, &pos, &pinned, &topic, &pid, &r.CreatedAt, &r.UpdatedAt, &lastMsg); err != nil {
			return nil, err
		}
		r.HumanMember = hm == 1
		r.Position = pos
		r.Topic = topic
		r.LastMessage = lastMsg // Row-Subtitle-Digest (Matrix-22)
		if pinned.Valid {
			r.PinnedMessageID = &pinned.String
		}
		if pid.Valid {
			r.ProjectID = &pid.String
		}
		r.MemberAgentIDs = []string{}
		byID[r.ID] = r
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	mrows, err := d.db.Query(`SELECT room_id,agent_id FROM room_members`)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var rid, aid string
		if err := mrows.Scan(&rid, &aid); err != nil {
			return nil, err
		}
		if r := byID[rid]; r != nil {
			r.MemberAgentIDs = append(r.MemberAgentIDs, aid)
		}
	}
	crows, err := d.db.Query(`SELECT room_id, COUNT(*) FROM messages GROUP BY room_id`)
	if err != nil {
		return nil, err
	}
	defer crows.Close()
	for crows.Next() {
		var rid string
		var c int
		if err := crows.Scan(&rid, &c); err != nil {
			return nil, err
		}
		if r := byID[rid]; r != nil {
			r.MessageCount = c
		}
	}
	return out, nil
}

func (d *DB) GetRoom(id string) (*Room, error) {
	r := &Room{}
	var pid, pinned sql.NullString
	var hm, pos int
	var topic string
	err := d.db.QueryRow(`SELECT id,name,kind,human_member,position,pinned_message_id,topic,project_id,created_at,updated_at FROM rooms WHERE id=?`, id).
		Scan(&r.ID, &r.Name, &r.Kind, &hm, &pos, &pinned, &topic, &pid, &r.CreatedAt, &r.UpdatedAt)
	r.HumanMember = hm == 1
	r.Position = pos
	r.Topic = topic
	if pinned.Valid {
		r.PinnedMessageID = &pinned.String
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if pid.Valid {
		r.ProjectID = &pid.String
	}
	r.MemberAgentIDs = []string{}
	mrows, err := d.db.Query(`SELECT agent_id FROM room_members WHERE room_id=?`, id)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var aid string
		if err := mrows.Scan(&aid); err != nil {
			return nil, err
		}
		r.MemberAgentIDs = append(r.MemberAgentIDs, aid)
	}
	return r, nil
}

func (d *DB) InsertRoom(r *Room) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	kind := r.Kind
	if kind == "" {
		kind = "chat"
	}
	hm := 1
	if !r.HumanMember {
		hm = 0
	}
	if _, err := tx.Exec(`INSERT INTO rooms (id,name,kind,human_member,position,topic,pinned_message_id,project_id,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.Name, kind, hm, r.Position, r.Topic, nil, r.CreatedAt, r.UpdatedAt); err != nil {
		tx.Rollback()
		return err
	}
	for _, aid := range r.MemberAgentIDs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO room_members (room_id,agent_id) VALUES (?,?)`, r.ID, aid); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) UpdateRoom(r *Room) error {
	var pid any
	if r.ProjectID != nil && *r.ProjectID != "" {
		pid = *r.ProjectID
	}
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	hm := 1
	if !r.HumanMember {
		hm = 0
	}
	var pm any
	if r.PinnedMessageID != nil && *r.PinnedMessageID != "" {
		pm = *r.PinnedMessageID
	}
	if _, err := tx.Exec(`UPDATE rooms SET name=?, kind=?, human_member=?, position=?, topic=?, pinned_message_id=?, project_id=?, updated_at=? WHERE id=?`,
		r.Name, r.Kind, hm, r.Position, r.Topic, pm, pid, now(), r.ID); err != nil {
		tx.Rollback()
		return err
	}
	// full member sync (review H-4): caller always supplies the complete
	// member list (createRoom + patchRoom rosters)
	if _, err := tx.Exec(`DELETE FROM room_members WHERE room_id=?`, r.ID); err != nil {
		tx.Rollback()
		return err
	}
	for _, aid := range r.MemberAgentIDs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO room_members (room_id,agent_id) VALUES (?,?)`, r.ID, aid); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) DeleteRoom(id string) error {
	_, err := d.db.Exec(`DELETE FROM rooms WHERE id=?`, id)
	return err
}

func (d *DB) AddRoomMember(roomID, agentID string) error {
	_, err := d.db.Exec(`INSERT OR IGNORE INTO room_members (room_id,agent_id) VALUES (?,?)`, roomID, agentID)
	return err
}

func (d *DB) RemoveRoomMember(roomID, agentID string) error {
	_, err := d.db.Exec(`DELETE FROM room_members WHERE room_id=? AND agent_id=?`, roomID, agentID)
	return err
}

func (d *DB) RoomAgents(roomID string) ([]*Agent, error) {
	rows, err := d.db.Query(`
		SELECT a.id,a.name,a.emoji,a.color,a.system_prompt,a.adapter,a.model,a.base_url,a.api_key,a.enabled,a.approval_gate,a.created_at,a.updated_at
		FROM agents a JOIN room_members m ON m.agent_id=a.id
		WHERE m.room_id=? AND a.enabled=1 ORDER BY a.created_at`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ── messages ────────────────────────────────────────────────────────

func scanMessage(r rowScanner) (*Message, error) {
	m := &Message{}
	var agentID, parentID sql.NullString
	if err := r.Scan(&m.ID, &m.RoomID, &agentID, &m.SenderName, &m.SenderKind, &m.Content, &m.Status, &m.CreatedAt, &m.UpdatedAt, &parentID); err != nil {
		return nil, err
	}
	m.AgentID = agentID.String
	if parentID.Valid {
		m.ParentID = &parentID.String
	}
	return m, nil
}

const msgCols = `id,room_id,agent_id,sender_name,sender_kind,content,status,created_at,updated_at,parent_id`

func (d *DB) ListMessages(roomID string, limit int) ([]*Message, error) {
	if limit <= 0 || limit > 1000 {
		limit = 300
	}
	rows, err := d.db.Query(`
		SELECT `+msgCols+` FROM (
			SELECT * FROM messages WHERE room_id=? ORDER BY created_at DESC LIMIT ?
		) ORDER BY created_at ASC`, roomID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	d.enrichAttachments(out)
	return out, nil
}

// GetMessage: Einzelabruf (P1#1 WS-refresh + P1#2 enrichment).
func (d *DB) GetMessage(id string) (*Message, error) {
	r := d.db.QueryRow(`SELECT `+msgCols+` FROM messages WHERE id=?`, id)
	m, err := scanMessage(r)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.enrichAttachments([]*Message{m})
	return m, nil
}

// enrichAttachments (P1#2): füllt Message.Attachments je Batch —
// 1 Query für alle IDs (kein N+1, review-2-Stil).
func (d *DB) enrichAttachments(msgs []*Message) {
	if len(msgs) == 0 {
		return
	}
	ids := make([]string, len(msgs))
	byID := map[string]*Message{}
	for i, m := range msgs {
		ids[i] = m.ID
		byID[m.ID] = m
	}
	q := `SELECT id,message_id,filename,mime,size,created_at FROM attachments WHERE message_id IN (` +
		strings.Repeat("?,", len(ids)-1) + `?) ORDER BY created_at`
	args := make([]any, len(ids))
	for i, v := range ids {
		args[i] = v
	}
	rows, err := d.db.Query(q, args...)
	if err != nil {
		return // attachments optional — leere Liste ist der ehrliche Fallback
	}
	defer rows.Close()
	for rows.Next() {
		a := &Attachment{}
		if err := rows.Scan(&a.ID, &a.MessageID, &a.Filename, &a.Mime, &a.Size, &a.CreatedAt); err != nil {
			continue
		}
		if m := byID[a.MessageID]; m != nil {
			m.Attachments = append(m.Attachments, a)
		}
	}
}

// SanitizeFilename (P1#2 security): basename only — strip Pfade/Controls,
// kein Traversal (…/..), lesbare Zeichen bleiben. Exportiert: API-Handler
// nutzen es vor dem FS-Write.
func SanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || strings.ContainsRune(`<>:"|?*`, r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "datei"
	}
	return name
}

// ── attachments (P1#2, Codex-Matrix 1.2) ────────────────────────────

func (d *DB) InsertAttachment(a *Attachment) error {
	_, err := d.db.Exec(`INSERT INTO attachments (id,message_id,filename,mime,size,path,created_at) VALUES (?,?,?,?,?,?,?)`,
		a.ID, a.MessageID, a.Filename, a.Mime, a.Size, a.Path, a.CreatedAt)
	return err
}

func (d *DB) AttachMessage(id, messageID string) error {
	res, err := d.db.Exec(`UPDATE attachments SET message_id=? WHERE id=?`, messageID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (d *DB) ListAttachments(messageID string) ([]*Attachment, error) {
	rows, err := d.db.Query(`SELECT id,message_id,filename,mime,size,path,created_at FROM attachments WHERE message_id=? ORDER BY created_at`, messageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Attachment
	for rows.Next() {
		a := &Attachment{}
		if err := rows.Scan(&a.ID, &a.MessageID, &a.Filename, &a.Mime, &a.Size, &a.Path, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (d *DB) GetAttachment(id string) (*Attachment, error) {
	r := d.db.QueryRow(`SELECT id,message_id,filename,mime,size,path,created_at FROM attachments WHERE id=?`, id)
	a := &Attachment{}
	if err := r.Scan(&a.ID, &a.MessageID, &a.Filename, &a.Mime, &a.Size, &a.Path, &a.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return a, nil
}

// ── approvals (P2#2, paperclip approvals.ts-Muster) ─────────────────

func (d *DB) InsertApproval(a *Approval) error {
	_, err := d.db.Exec(`INSERT INTO approvals (id,agent_id,user_msg_id,reason,status,created_at,decided_at) VALUES (?,?,?,?,?,?,?)`,
		a.ID, a.AgentID, a.UserMsgID, a.Reason, a.Status, a.CreatedAt, a.DecidedAt)
	return err
}

// FindGrantedApproval: offener Grant-Token für das Turn-Paar
// (agent, userMsg) — wird beim Resume-Turn konsumiert.
func (d *DB) FindGrantedApproval(agentID, userMsgID string) (*Approval, error) {
	row := d.db.QueryRow(`SELECT id,agent_id,user_msg_id,reason,status,created_at,decided_at,consumed_at FROM approvals
		WHERE agent_id=? AND user_msg_id=? AND status='granted' AND consumed_at IS NULL
		ORDER BY decided_at DESC LIMIT 1`, agentID, userMsgID)
	a := &Approval{}
	var decided, consumed sql.NullString
	if err := row.Scan(&a.ID, &a.AgentID, &a.UserMsgID, &a.Reason, &a.Status, &a.CreatedAt, &decided, &consumed); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	a.DecidedAt = decided.String
	a.ConsumedAt = consumed.String
	return a, nil
}

// ConsumeApprovalFor (Befund 1, Codex-Audit): atomarer Grant-Konsum für
// das Turn-Paar — einzelne UPDATE mit Subselect; true = DIESER Turn führt
// aus, false = Token schon verbrannt (anderer Worker; Turn skippen).
func (d *DB) ConsumeApprovalFor(agentID, userMsgID string) (bool, error) {
	res, err := d.db.Exec(`UPDATE approvals SET consumed_at=? WHERE id=(
		SELECT id FROM approvals WHERE agent_id=? AND user_msg_id=? AND status='granted' AND consumed_at IS NULL
		ORDER BY decided_at DESC LIMIT 1) AND consumed_at IS NULL`,
		now(), agentID, userMsgID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ConsumeApproval: Grant-Token verbrennen (Execution-Paar; Audit bleibt).
func (d *DB) ConsumeApproval(id string) error {
	_, err := d.db.Exec(`UPDATE approvals SET consumed_at=? WHERE id=? AND consumed_at IS NULL`, now(), id)
	return err
}

// ListApprovals: status="" = alle (neueste zuerst), sonst Filter.
func (d *DB) AddTaskComment(taskID, authorKind, authorName, content string) (*TaskComment, error) {
	c := &TaskComment{
		ID: NewID("tc"), TaskID: taskID, AuthorKind: authorKind,
		AuthorName: authorName, Content: content, CreatedAt: now(),
	}
	_, err := d.db.Exec(`INSERT INTO task_comments (id,task_id,author_kind,author_name,content,created_at) VALUES (?,?,?,?,?,?)`,
		c.ID, c.TaskID, c.AuthorKind, c.AuthorName, c.Content, c.CreatedAt)
	return c, err
}

func (d *DB) ListTaskComments(taskID string) ([]*TaskComment, error) {
	rows, err := d.db.Query(`SELECT id,task_id,author_kind,author_name,content,created_at FROM task_comments WHERE task_id=? ORDER BY created_at ASC LIMIT 500`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*TaskComment
	for rows.Next() {
		c := &TaskComment{}
		if err := rows.Scan(&c.ID, &c.TaskID, &c.AuthorKind, &c.AuthorName, &c.Content, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ── routines (P2#4, paperclip routines.ts) ──────────────────────────

func scanRoutine(row rowScanner) (*Routine, error) {
	t := &Routine{}
	var lastRun sql.NullString
	var roomID, agentID sql.NullString
	if err := row.Scan(&t.ID, &t.Name, &t.Cron, &t.Prompt, &roomID, &agentID, &t.Enabled, &t.NextRunAt, &lastRun, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	t.RoomID = roomID.String
	t.AgentID = agentID.String
	t.LastRunAt = lastRun.String
	return t, nil
}

func (d *DB) ListRoutines() ([]*Routine, error) {
	rows, err := d.db.Query(`SELECT id,name,cron,prompt,room_id,agent_id,enabled,next_run_at,last_run_at,created_at,updated_at FROM routines ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Routine
	for rows.Next() {
		t, err := scanRoutine(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (d *DB) DueRoutines(nowISO string) ([]*Routine, error) {
	rows, err := d.db.Query(`SELECT id,name,cron,prompt,room_id,agent_id,enabled,next_run_at,last_run_at,created_at,updated_at
		FROM routines WHERE enabled=1 AND next_run_at <= ?`, nowISO)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Routine
	for rows.Next() {
		t, err := scanRoutine(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (d *DB) GetRoutine(id string) (*Routine, error) {
	row := d.db.QueryRow(`SELECT id,name,cron,prompt,room_id,agent_id,enabled,next_run_at,last_run_at,created_at,updated_at FROM routines WHERE id=?`, id)
	t, err := scanRoutine(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

func (d *DB) InsertRoutine(t *Routine) error {
	_, err := d.db.Exec(`INSERT INTO routines (id,name,cron,prompt,room_id,agent_id,enabled,next_run_at,last_run_at,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,NULL,?,?)`,
		t.ID, t.Name, t.Cron, t.Prompt, nullable(t.RoomID), nullable(t.AgentID), boolInt(t.Enabled), t.NextRunAt, t.CreatedAt, t.UpdatedAt)
	return err
}

// UpdateRoutine: Feld-scharf (name/cron/prompt/room/agent/enabled); next_run
// NICHT via Update — neu berechnet vom cron-Parser (Tick).
func (d *DB) UpdateRoutine(t *Routine) error {
	var roomID, agentID any
	if t.RoomID != "" {
		roomID = t.RoomID
	}
	if t.AgentID != "" {
		agentID = t.AgentID
	}
	_, err := d.db.Exec(`UPDATE routines SET name=?,cron=?,prompt=?,room_id=?,agent_id=?,enabled=?,updated_at=? WHERE id=?`,
		t.Name, t.Cron, t.Prompt, roomID, agentID, boolInt(t.Enabled), now(), t.ID)
	return err
}

// RoutineFired: last_run setzen + next_run vom cron weiterrechnen (Tick).
func (d *DB) RoutineFired(id, nowISO, nextRunAt string) error {
	_, err := d.db.Exec(`UPDATE routines SET last_run_at=?, next_run_at=?, updated_at=? WHERE id=?`,
		nowISO, nextRunAt, now(), id)
	return err
}

func (d *DB) DeleteRoutine(id string) error {
	_, err := d.db.Exec(`DELETE FROM routines WHERE id=?`, id)
	return err
}

// RoomRoles (P2#8): map[agentID]→role je Raum; '' = default. UI + künftige
// Agent-Gewichtung; 1 Query.
func (d *DB) RoomRoles(roomID string) (map[string]string, error) {
	rows, err := d.db.Query(`SELECT agent_id, role FROM room_members WHERE room_id=?`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var aid, role string
		if err := rows.Scan(&aid, &role); err != nil {
			return nil, err
		}
		out[aid] = role
	}
	return out, rows.Err()
}

// SetRoomRole: role setzen/leeren (UPSERT-artig über UPDATE nur).
func (d *DB) SetRoomRole(roomID, agentID, role string) error {
	if role != "" && role != "admin" {
		return fmt.Errorf("ungültige Rolle %q (nur ''|admin)", role)
	}
	res, err := d.db.Exec(`UPDATE room_members SET role=? WHERE room_id=? AND agent_id=?`, role, roomID, agentID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ── skills (P2#3, paperclip company-skills) ─────────────────────────

func skillCols(r rowScanner) (*Skill, error) {
	sk := &Skill{}
	var parent, deleted sql.NullString
	if err := r.Scan(&sk.ID, &sk.Name, &sk.Instructions, &sk.Version, &parent, &deleted, &sk.CreatedAt, &sk.UpdatedAt); err != nil {
		return nil, err
	}
	if parent.Valid {
		sk.ParentID = &parent.String
	}
	sk.DeletedAt = deleted.String
	return sk, nil
}

const skillSelect = `SELECT id,name,instructions,version,parent_id,deleted_at,created_at,updated_at FROM skills`

// ListSkills: deleted=false → nur lebende; true → alle inkl. Versionen.
func (d *DB) ListSkills(includeDeleted bool) ([]*Skill, error) {
	q := skillSelect
	if !includeDeleted {
		q += ` WHERE deleted_at IS NULL`
	}
	q += ` ORDER BY name, version DESC`
	rows, err := d.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Skill
	for rows.Next() {
		sk, err := skillCols(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	d.enrichSkillAgents(out)
	return out, nil
}

func (d *DB) GetSkill(id string) (*Skill, error) {
	row := d.db.QueryRow(skillSelect+` WHERE id=?`, id)
	sk, err := skillCols(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d.enrichSkillAgents([]*Skill{sk})
	return sk, nil
}

// InsertSkill: Version übernehmen (create=1; update schreibt NEUE row mit
// version+1 und parent_id=alt — Paperclip Version-History-Muster).
func (d *DB) InsertSkill(sk *Skill) error {
	var parent any
	if sk.ParentID != nil && *sk.ParentID != "" {
		parent = *sk.ParentID
	}
	_, err := d.db.Exec(`INSERT INTO skills (id,name,instructions,version,parent_id,deleted_at,created_at,updated_at) VALUES (?,?,?,?,?,NULL,?,?)`,
		sk.ID, sk.Name, sk.Instructions, sk.Version, parent, sk.CreatedAt, sk.UpdatedAt)
	return err
}

// UpdateSkill: neue Version-Row anlegen (alt bleibt — Restore-Kette).
func (d *DB) UpdateSkill(skill *Skill) (string, error) {
	old, err := d.GetSkill(skill.ID)
	if err != nil || old == nil {
		return "", fmt.Errorf("skill %s nicht gefunden", skill.ID)
	}
	neu := &Skill{
		ID:          NewID("skill"),
		Name:        skill.Name,
		Instructions: skill.Instructions,
		Version:     old.Version + 1,
		ParentID:    &old.ID,
		CreatedAt:   now(),
		UpdatedAt:   now(),
	}
	if err := d.InsertSkill(neu); err != nil {
		return "", err
	}
	ids, err := d.SkillAgents(old.ID)
	if err != nil {
		return neu.ID, err
	}
	for _, aid := range ids {
		_ = d.AssignSkill(neu.ID, aid)
	}
	return neu.ID, nil
}

// DeleteSkill: soft-delete (Paperclip: Versionen bleiben restorebar).
func (d *DB) DeleteSkill(id string) error {
	_, err := d.db.Exec(`UPDATE skills SET deleted_at=? WHERE id=? AND deleted_at IS NULL`, now(), id)
	return err
}

// RestoreSkill: soft-deleted row zurückholen (Version bleibt).
func (d *DB) RestoreSkill(id string) error {
	_, err := d.db.Exec(`UPDATE skills SET deleted_at=NULL WHERE id=? AND deleted_at IS NOT NULL`, id)
	return err
}

// AssignSkill / UnassignSkill: Bindungen (M:N).
func (d *DB) AssignSkill(skillID, agentID string) error {
	_, err := d.db.Exec(`INSERT OR IGNORE INTO skill_assignments (skill_id,agent_id) VALUES (?,?)`, skillID, agentID)
	return err
}

func (d *DB) UnassignSkill(skillID, agentID string) error {
	_, err := d.db.Exec(`DELETE FROM skill_assignments WHERE skill_id=? AND agent_id=?`, skillID, agentID)
	return err
}

// SkillAgents: Gebundene Agent-IDs je Skill.
func (d *DB) SkillAgents(skillID string) ([]string, error) {
	rows, err := d.db.Query(`SELECT agent_id FROM skill_assignments WHERE skill_id=?`, skillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var aid string
		if err := rows.Scan(&aid); err != nil {
			return nil, err
		}
		out = append(out, aid)
	}
	return out, rows.Err()
}

// enrichSkillAgents: Agent-Bindungen je Batch anhängen (kein N+1).
func (d *DB) enrichSkillAgents(skills []*Skill) {
	if len(skills) == 0 {
		return
	}
	ids := make([]string, len(skills))
	byID := map[string]*Skill{}
	for i, sk := range skills {
		ids[i] = sk.ID
		byID[sk.ID] = sk
	}
	q := `SELECT skill_id,agent_id FROM skill_assignments WHERE skill_id IN (` + strings.Repeat("?,", len(ids)-1) + `?)`
	args := make([]any, len(ids))
	for i, v := range ids {
		args[i] = v
	}
	rows, err := d.db.Query(q, args...)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var sid, aid string
		if err := rows.Scan(&sid, &aid); err != nil {
			continue
		}
		if sk := byID[sid]; sk != nil {
			sk.AgentIDs = append(sk.AgentIDs, aid)
		}
	}
}

// AgentsSkills: gebundene Skills je Agent (Runtime-Systemprompt-Erweiterung).
func (d *DB) AgentsSkills(agentID string) ([]*Skill, error) {
	rows, err := d.db.Query(skillSelect+` WHERE deleted_at IS NULL AND id IN
		(SELECT skill_id FROM skill_assignments WHERE agent_id=?) ORDER BY name`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Skill
	for rows.Next() {
		sk, err := skillCols(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

// ── room reads (P1#10 authoritativ) ─────────────────────────────────

func (d *DB) MarkRoomRead(roomID, reader, at string) error {
	_, err := d.db.Exec(`INSERT INTO room_reads (room_id,reader,last_read_at) VALUES (?,?,?)
		ON CONFLICT(room_id,reader) DO UPDATE SET last_read_at=excluded.last_read_at`, roomID, reader, at)
	return err
}

func (d *DB) GetRoomRead(roomID, reader string) (string, error) {
	var at string
	err := d.db.QueryRow(`SELECT last_read_at FROM room_reads WHERE room_id=? AND reader=?`, roomID, reader).Scan(&at)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return at, err
}

// UnreadBatch: map[roomID]→ungelesene Nachrichten (nach Marken-Zeit je
// Reader; 1 Query je Raum-Fenster — batched über rooms).
func (d *DB) UnreadBatch(reader string) (map[string]int, error) {
	rows, err := d.db.Query(`
		SELECT r.id, COUNT(m.id)
		FROM rooms r
		LEFT JOIN room_reads rr ON rr.room_id = r.id AND rr.reader = ?
		LEFT JOIN messages m ON m.room_id = r.id AND m.created_at > COALESCE(rr.last_read_at, '1970-01-01T00:00:00Z') AND m.sender_kind != 'system'
		GROUP BY r.id`, reader)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var rid string
		var c int
		if err := rows.Scan(&rid, &c); err != nil {
			return nil, err
		}
		out[rid] = c
	}
	return out, rows.Err()
}

func (d *DB) GetApproval(id string) (*Approval, error) {
	row := d.db.QueryRow(`SELECT id,agent_id,user_msg_id,reason,status,created_at,decided_at,consumed_at FROM approvals WHERE id=?`, id)
	a := &Approval{}
	var decided, consumed sql.NullString
	if err := row.Scan(&a.ID, &a.AgentID, &a.UserMsgID, &a.Reason, &a.Status, &a.CreatedAt, &decided, &consumed); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	a.DecidedAt = decided.String
	a.ConsumedAt = consumed.String
	return a, nil
}

func (d *DB) ListApprovals(status string) ([]*Approval, error) {
	q := `SELECT id,agent_id,user_msg_id,reason,status,created_at,decided_at,consumed_at FROM approvals`
	var rows *sql.Rows
	var err error
	if status != "" {
		rows, err = d.db.Query(q+` WHERE status=? ORDER BY created_at DESC LIMIT 200`, status)
	} else {
		rows, err = d.db.Query(q+` ORDER BY created_at DESC LIMIT 200`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Approval
	for rows.Next() {
		a := &Approval{}
		var decided, consumed sql.NullString
		if err := rows.Scan(&a.ID, &a.AgentID, &a.UserMsgID, &a.Reason, &a.Status, &a.CreatedAt, &decided, &consumed); err != nil {
			return nil, err
		}
		a.DecidedAt = decided.String
		a.ConsumedAt = consumed.String
		out = append(out, a)
	}
	return out, rows.Err()
}

// DecideApproval: grant|deny + Zeitstempel; false wenn nicht mehr pending
// (Race-Schutz: zweite Entscheidung überschreibt nicht die erste).
func (d *DB) DecideApproval(id, decision string) (bool, error) {
	if decision != "granted" && decision != "denied" {
		return false, fmt.Errorf("ungültige Entscheidung %q", decision)
	}
	res, err := d.db.Exec(`UPDATE approvals SET status=?, decided_at=? WHERE id=? AND status='pending'`,
		decision, now(), id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (d *DB) InsertMessage(m *Message) error {
	parentID := ""
	if m.ParentID != nil {
		parentID = *m.ParentID
	}
	_, err := d.db.Exec(`INSERT INTO messages (id,room_id,agent_id,sender_name,sender_kind,content,status,created_at,updated_at,parent_id)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		m.ID, m.RoomID, nullable(m.AgentID), m.SenderName, m.SenderKind, m.Content, m.Status, m.CreatedAt, m.UpdatedAt, nullable(parentID))
	return err
}

// Replies (P1#4): Direkt-Antworten je Wurzel, älteste zuerst.
func (d *DB) ListReplies(rootID string) ([]*Message, error) {
	rows, err := d.db.Query(`SELECT `+msgCols+` FROM messages WHERE parent_id=? ORDER BY created_at ASC LIMIT 500`, rootID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	d.enrichAttachments(out)
	return out, nil
}

// ReplyCountBatch (P1#2-Stil: 1 Query, kein N+1): map[rootID]→count.
func (d *DB) ReplyCountBatch(rootIDs []string) (map[string]int, error) {
	if len(rootIDs) == 0 {
		return map[string]int{}, nil
	}
	q := `SELECT parent_id, COUNT(*) FROM messages WHERE parent_id IN (` +
		strings.Repeat("?,", len(rootIDs)-1) + `?) GROUP BY parent_id`
	args := make([]any, len(rootIDs))
	for i, v := range rootIDs {
		args[i] = v
	}
	rows, err := d.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var pid string
		var c int
		if err := rows.Scan(&pid, &c); err != nil {
			return nil, err
		}
		out[pid] = c
	}
	return out, rows.Err()
}

func (d *DB) UpdateMessage(id string, content string, status string) error {
	_, err := d.db.Exec(`UPDATE messages SET content=?,status=?,updated_at=? WHERE id=?`,
		content, status, now(), id)
	return err
}

// ── tasks ───────────────────────────────────────────────────────────

func scanTask(r rowScanner) (*Task, error) {
	t := &Task{}
	var pid sql.NullString
	if err := r.Scan(&t.ID, &pid, &t.Title, &t.Notes, &t.Status, &t.Assignee, &t.Position, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	if pid.Valid {
		t.ProjectID = &pid.String
	}
	return t, nil
}

const taskCols = `id,project_id,title,notes,status,assignee,position,created_at,updated_at`

func (d *DB) ListTasks(projectID string) ([]*Task, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if projectID != "" {
		rows, err = d.db.Query(`SELECT `+taskCols+` FROM tasks WHERE project_id=? ORDER BY position,created_at`, projectID)
	} else {
		rows, err = d.db.Query(`SELECT `+taskCols+` FROM tasks ORDER BY position,created_at`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (d *DB) GetTask(id string) (*Task, error) {
	r := d.db.QueryRow(`SELECT `+taskCols+` FROM tasks WHERE id=?`, id)
	t, err := scanTask(r)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return t, err
}

func (d *DB) InsertTask(t *Task) error {
	var pid any
	if t.ProjectID != nil && *t.ProjectID != "" {
		pid = *t.ProjectID
	}
	_, err := d.db.Exec(`INSERT INTO tasks (id,project_id,title,notes,status,assignee,position,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		t.ID, pid, t.Title, t.Notes, t.Status, t.Assignee, t.Position, t.CreatedAt, t.UpdatedAt)
	return err
}

func (d *DB) UpdateTask(t *Task) error {
	var pid any
	if t.ProjectID != nil && *t.ProjectID != "" {
		pid = *t.ProjectID
	}
	_, err := d.db.Exec(`UPDATE tasks SET project_id=?,title=?,notes=?,status=?,assignee=?,position=?,updated_at=? WHERE id=?`,
		pid, t.Title, t.Notes, t.Status, t.Assignee, t.Position, now(), t.ID)
	return err
}

func (d *DB) DeleteTask(id string) error {
	_, err := d.db.Exec(`DELETE FROM tasks WHERE id=?`, id)
	return err
}