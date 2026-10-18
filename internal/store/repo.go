package store

import "database/sql"

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
		&model, &baseURL, &apiKey, &a.Enabled, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}
	a.Model, a.BaseURL, a.APIKey = model.String, baseURL.String, apiKey.String
	return a, nil
}

func (d *DB) ListAgents() ([]*Agent, error) {
	rows, err := d.db.Query(`SELECT id,name,emoji,color,system_prompt,adapter,model,base_url,api_key,enabled,created_at,updated_at FROM agents ORDER BY created_at`)
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
	r := d.db.QueryRow(`SELECT id,name,emoji,color,system_prompt,adapter,model,base_url,api_key,enabled,created_at,updated_at FROM agents WHERE id=?`, id)
	a, err := scanAgent(r)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

func agentInsertValues(a *Agent) []any {
	return []any{a.ID, a.Name, a.Emoji, a.Color, a.SystemPrompt, a.Adapter,
		nullable(a.Model), nullable(a.BaseURL), nullable(a.APIKey), boolInt(a.Enabled), a.CreatedAt, a.UpdatedAt}
}

func (d *DB) InsertAgent(a *Agent) error {
	_, err := d.db.Exec(`INSERT INTO agents
	(id,name,emoji,color,system_prompt,adapter,model,base_url,api_key,enabled,created_at,updated_at)
	VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, agentInsertValues(a)...)
	return err
}

func (d *DB) UpdateAgent(a *Agent) error {
	_, err := d.db.Exec(`UPDATE agents SET name=?,emoji=?,color=?,system_prompt=?,adapter=?,model=?,base_url=?,api_key=?,enabled=?,updated_at=? WHERE id=?`,
		a.Name, a.Emoji, a.Color, a.SystemPrompt, a.Adapter, nullable(a.Model), nullable(a.BaseURL), nullable(a.APIKey), boolInt(a.Enabled), now(), a.ID)
	return err
}

func (d *DB) DeleteAgent(id string) error {
	_, err := d.db.Exec(`DELETE FROM agents WHERE id=?`, id)
	return err
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
	rows, err := d.db.Query(`SELECT id,name,kind,project_id,created_at,updated_at FROM rooms ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Room
	byID := map[string]*Room{}
	for rows.Next() {
		r := &Room{}
		var pid sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &r.Kind, &pid, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
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
	var pid sql.NullString
	err := d.db.QueryRow(`SELECT id,name,kind,project_id,created_at,updated_at FROM rooms WHERE id=?`, id).
		Scan(&r.ID, &r.Name, &r.Kind, &pid, &r.CreatedAt, &r.UpdatedAt)
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
	var pid any
	if r.ProjectID != nil && *r.ProjectID != "" {
		pid = *r.ProjectID
	}
	kind := r.Kind
	if kind == "" {
		kind = "chat"
	}
	if _, err := tx.Exec(`INSERT INTO rooms (id,name,kind,project_id,created_at,updated_at) VALUES (?,?,?,?,?,?)`,
		r.ID, r.Name, kind, pid, r.CreatedAt, r.UpdatedAt); err != nil {
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
	if _, err := tx.Exec(`UPDATE rooms SET name=?, project_id=?, updated_at=? WHERE id=?`,
		r.Name, pid, now(), r.ID); err != nil {
		tx.Rollback()
		return err
	}
	// full member sync (review H-4): patchRoom sends the complete member list
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
		SELECT a.id,a.name,a.emoji,a.color,a.system_prompt,a.adapter,a.model,a.base_url,a.api_key,a.enabled,a.created_at,a.updated_at
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
	var agentID sql.NullString
	if err := r.Scan(&m.ID, &m.RoomID, &agentID, &m.SenderName, &m.SenderKind, &m.Content, &m.Status, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return nil, err
	}
	m.AgentID = agentID.String
	return m, nil
}

const msgCols = `id,room_id,agent_id,sender_name,sender_kind,content,status,created_at,updated_at`

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
	return out, rows.Err()
}

func (d *DB) GetMessage(id string) (*Message, error) {
	r := d.db.QueryRow(`SELECT `+msgCols+` FROM messages WHERE id=?`, id)
	m, err := scanMessage(r)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

func (d *DB) InsertMessage(m *Message) error {
	_, err := d.db.Exec(`INSERT INTO messages (id,room_id,agent_id,sender_name,sender_kind,content,status,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		m.ID, m.RoomID, nullable(m.AgentID), m.SenderName, m.SenderKind, m.Content, m.Status, m.CreatedAt, m.UpdatedAt)
	return err
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