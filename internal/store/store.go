// Package store: SQLite persistence for Agent Relay.
// Uses modernc.org/sqlite (pure Go, no CGO) so we can cross-compile
// amd64 + aarch64 without emulation and keep a tiny distroless image.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
}

type Agent struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Emoji        string `json:"emoji"`
	Color        string `json:"color"`
	SystemPrompt string `json:"systemPrompt"`
	Adapter      string `json:"adapter"`
	Model        string `json:"model"`
	BaseURL      string `json:"baseUrl"`
	// APIKey is deliberately NOT serialised (review-2 copilot H-2): the UI
	// only sets it (write-only), never needs it back.
	APIKey    string `json:"-"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type Project struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type Room struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Kind           string   `json:"kind"` // "chat" | "project" (project rooms carry the task board)
	HumanMember    bool     `json:"humanMember"` // operator participation; false = room hidden from sidebar
	Position       int      `json:"position"` // manual sort order (drag&drop, review-7 B3)
	ProjectID      *string  `json:"projectId"` // legacy link; project-kind rooms are the room themselves
	CreatedAt      string   `json:"createdAt"`
	UpdatedAt      string   `json:"updatedAt"`
	MemberAgentIDs []string `json:"memberAgentIds"`
	MessageCount   int      `json:"messageCount"`
}

type Message struct {
	ID         string `json:"id"`
	RoomID     string `json:"roomId"`
	AgentID    string `json:"agentId"` // empty for human/system
	SenderName string `json:"senderName"`
	SenderKind string `json:"senderKind"` // human|agent|system
	Content    string `json:"content"`
	Status     string `json:"status"` // streaming|complete|error
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

type Task struct {
	ID        string  `json:"id"`
	ProjectID *string `json:"projectId"`
	Title     string  `json:"title"`
	Notes     string  `json:"notes"`
	Status    string  `json:"status"` // todo|doing|done
	Assignee  string  `json:"assignee"`
	Position  int     `json:"position"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

// RFC3339 with fixed 9-digit nanoseconds: lexicographically sortable, no
// collisions on rapid inserts (review-2 L-A).
func now() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z07:00") }

func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("createdir: %w", err)
	}
	dsn := "file:" + filepath.Join(dataDir, "relay.db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	// Note: modernc sqlite handles DSN pragmas; if the FS cannot provide
	// proper locks (network mounts), WAL creation fails and we retry
	// with the rollback journal.
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := tryWAL(db); err != nil {
		db.Close()
		dsn = "file:" + filepath.Join(dataDir, "relay.db") + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(DELETE)&_pragma=foreign_keys(1)"
		db, err = sql.Open("sqlite", dsn)
		if err != nil {
			return nil, err
		}
	}
	db.SetMaxOpenConns(1) // single writer; avoids SQLITE_BUSY in-process

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &DB{db: db}, nil
}

func tryWAL(db *sql.DB) error {
	if err := db.Ping(); err != nil {
		return err
	}
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode=WAL").Scan(&mode); err != nil {
		return err
	}
	if mode != "wal" {
		return fmt.Errorf("wal refused: %q", mode)
	}
	return nil
}

func migrate(db *sql.DB) error {
	// idempotent upgrade for pre-existing databases: on fresh DBs the CREATE
	// TABLE below already carries kind; run ALTER only when the table exists
	// without it (error "no such table" is fine for fresh DBs).
	if _, err := db.Exec(`ALTER TABLE rooms ADD COLUMN kind TEXT NOT NULL DEFAULT 'chat'`); err != nil {
		msg := err.Error()
		if !strings.Contains(msg, "duplicate column") && !strings.Contains(msg, "no such table") {
			return fmt.Errorf("rooms.kind alter: %w", err)
		}
	}
	if _, err := db.Exec(`ALTER TABLE rooms ADD COLUMN human_member INTEGER NOT NULL DEFAULT 1`); err != nil {
		msg := err.Error()
		if !strings.Contains(msg, "duplicate column") && !strings.Contains(msg, "no such table") {
			return fmt.Errorf("rooms.human_member alter: %w", err)
		}
	}
	if _, err := db.Exec(`ALTER TABLE rooms ADD COLUMN position INTEGER NOT NULL DEFAULT 0`); err != nil {
		msg := err.Error()
		if !strings.Contains(msg, "duplicate column") && !strings.Contains(msg, "no such table") {
			return fmt.Errorf("rooms.position alter: %w", err)
		}
	}
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS agents (
  id TEXT PRIMARY KEY, name TEXT NOT NULL, emoji TEXT NOT NULL DEFAULT '🤖',
  color TEXT NOT NULL DEFAULT '#6ea8fe', system_prompt TEXT NOT NULL DEFAULT '',
  adapter TEXT NOT NULL DEFAULT 'echo', model TEXT, base_url TEXT, api_key TEXT,
  enabled INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS projects (
  id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
  color TEXT NOT NULL DEFAULT '#8a7ff0', created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS rooms (
  id TEXT PRIMARY KEY, name TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT 'chat' CHECK (kind IN ('chat','project')),
  human_member INTEGER NOT NULL DEFAULT 1,
  position INTEGER NOT NULL DEFAULT 0,
  project_id TEXT REFERENCES projects(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS room_members (
  room_id TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
  agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
  PRIMARY KEY (room_id, agent_id)
);
CREATE TABLE IF NOT EXISTS messages (
  id TEXT PRIMARY KEY, room_id TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
  agent_id TEXT, sender_name TEXT NOT NULL, sender_kind TEXT NOT NULL,
  content TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'complete',
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_room ON messages(room_id, created_at);
CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY, project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
  title TEXT NOT NULL, notes TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'todo', assignee TEXT DEFAULT '',
  position INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL, updated_at TEXT NOT NULL
);
`)
	return err
}

// Mark stuck streaming messages as errored (crash recovery).
func (d *DB) FailStaleStreaming() {
	_, _ = d.db.Exec(`UPDATE messages SET status='error', updated_at=? WHERE status='streaming'`, now())
}

// ── helpers ─────────────────────────────────────────────────────────

func id(prefix string, now time.Time) string {
	uni := now.UnixNano()
	return fmt.Sprintf("%s_%016x", prefix, uni)
}

func NewID(prefix string) string { return id(prefix, time.Now()) }