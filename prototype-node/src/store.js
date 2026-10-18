// sqlite persistence via node:sqlite (no native deps)
const { DatabaseSync } = require("node:sqlite");
const path = require("node:path");
const fs = require("node:fs");

let db;

function nowISO() {
  return new Date().toISOString();
}

function open(dbPath) {
  fs.mkdirSync(path.dirname(dbPath), { recursive: true });
  db = new DatabaseSync(dbPath);
  db.exec("PRAGMA busy_timeout = 5000;");
  // WAL is ideal but breaks on SMB/CIFS mounts (shared mmap). Try it, fall
  // back to the default rollback journal when the filesystem refuses.
  try {
    db.exec("PRAGMA journal_mode = WAL;");
  } catch (err) {
    console.warn(
      `[store] journal_mode=WAL nicht möglich (${err.errcode ?? ""}${err.errstr ? " " + err.errstr : ""}) — nutze Standard-Journal (OK auf SMB/Netzwerk-Mounts)`
    );
    db = new DatabaseSync(dbPath);
    db.exec("PRAGMA busy_timeout = 5000;");
  }
  db.exec("PRAGMA foreign_keys = ON;");
  migrate();
  return db;
}

function migrate() {
  db.exec(`
    CREATE TABLE IF NOT EXISTS settings (
      key TEXT PRIMARY KEY,
      value TEXT NOT NULL
    );

    CREATE TABLE IF NOT EXISTS agents (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      emoji TEXT NOT NULL DEFAULT '🤖',
      color TEXT NOT NULL DEFAULT '#6ea8fe',
      system_prompt TEXT NOT NULL DEFAULT '',
      adapter TEXT NOT NULL DEFAULT 'echo',
      model TEXT,
      base_url TEXT,
      api_key TEXT,
      enabled INTEGER NOT NULL DEFAULT 1,
      created_at TEXT NOT NULL,
      updated_at TEXT NOT NULL
    );

    CREATE TABLE IF NOT EXISTS projects (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      description TEXT NOT NULL DEFAULT '',
      color TEXT NOT NULL DEFAULT '#8a7ff0',
      created_at TEXT NOT NULL,
      updated_at TEXT NOT NULL
    );

    CREATE TABLE IF NOT EXISTS rooms (
      id TEXT PRIMARY KEY,
      name TEXT NOT NULL,
      project_id TEXT REFERENCES projects(id) ON DELETE SET NULL,
      created_at TEXT NOT NULL,
      updated_at TEXT NOT NULL
    );

    CREATE TABLE IF NOT EXISTS room_members (
      room_id TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
      agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
      PRIMARY KEY (room_id, agent_id)
    );

    CREATE TABLE IF NOT EXISTS messages (
      id TEXT PRIMARY KEY,
      room_id TEXT NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
      agent_id TEXT,
      sender_name TEXT NOT NULL,
      sender_kind TEXT NOT NULL CHECK (sender_kind IN ('human','agent','system')),
      content TEXT NOT NULL DEFAULT '',
      status TEXT NOT NULL DEFAULT 'complete' CHECK (status IN ('streaming','complete','error')),
      created_at TEXT NOT NULL,
      updated_at TEXT NOT NULL
    );
    CREATE INDEX IF NOT EXISTS idx_messages_room ON messages(room_id, created_at);

    CREATE TABLE IF NOT EXISTS tasks (
      id TEXT PRIMARY KEY,
      project_id TEXT REFERENCES projects(id) ON DELETE CASCADE,
      title TEXT NOT NULL,
      notes TEXT NOT NULL DEFAULT '',
      status TEXT NOT NULL DEFAULT 'todo' CHECK (status IN ('todo','doing','done')),
      assignee INTEGER,
      position INTEGER NOT NULL DEFAULT 0,
      created_at TEXT NOT NULL,
      updated_at TEXT NOT NULL
    );
  `);
}

// ── settings ────────────────────────────────────────────────────────
const DEFAULT_SETTINGS = {
  relayName: "Agent Relay",
  theme: "dark",
  openclawBaseUrl: "",
  openclawToken: "",
};

function getSettings() {
  const rows = db.prepare("SELECT key, value FROM settings").all();
  const out = { ...DEFAULT_SETTINGS };
  for (const r of rows) {
    if (r.key in out && r.value !== null) {
      try {
        out[r.key] = JSON.parse(r.value);
      } catch {
        out[r.key] = r.value;
      }
    }
  }
  return out;
}

function updateSettings(patch) {
  const allowed = Object.keys(DEFAULT_SETTINGS);
  for (const [k, v] of Object.entries(patch)) {
    if (!allowed.includes(k)) continue;
    db.prepare(
      "INSERT INTO settings (key, value) VALUES (?, ?) " +
      "ON CONFLICT(key) DO UPDATE SET value = excluded.value"
    ).run(k, JSON.stringify(v));
  }
  return getSettings();
}

// ── agents ──────────────────────────────────────────────────────────
function listAgents() {
  return db.prepare("SELECT * FROM agents ORDER BY created_at").all();
}

function getAgent(id) {
  return db.prepare("SELECT * FROM agents WHERE id = ?").get(id) || null;
}

function insertAgent(agent) {
  const row = {
    id: agent.id,
    name: agent.name,
    emoji: agent.emoji ?? "🤖",
    color: agent.color ?? "#6ea8fe",
    system_prompt: agent.systemPrompt ?? "",
    adapter: agent.adapter ?? "echo",
    model: agent.model ?? null,
    base_url: agent.baseUrl ?? null,
    api_key: agent.apiKey ?? null,
    enabled: agent.enabled ? 1 : 0,
    created_at: nowISO(),
    updated_at: nowISO(),
  };
  db.prepare(`
    INSERT INTO agents (id, name, emoji, color, system_prompt, adapter, model, base_url, api_key, enabled, created_at, updated_at)
    VALUES (@id, @name, @emoji, @color, @system_prompt, @adapter, @model, @base_url, @api_key, @enabled, @created_at, @updated_at)
  `).run(row);
  return getAgent(agent.id);
}

function updateAgent(id, patch) {
  const existing = getAgent(id);
  if (!existing) return null;
  const next = {
    name: patch.name ?? existing.name,
    emoji: patch.emoji ?? existing.emoji,
    color: patch.color ?? existing.color,
    system_prompt: patch.systemPrompt ?? existing.system_prompt,
    adapter: patch.adapter ?? existing.adapter,
    model: patch.model !== undefined ? patch.model : existing.model,
    base_url: patch.baseUrl !== undefined ? patch.baseUrl : existing.base_url,
    api_key: patch.apiKey !== undefined ? patch.apiKey : existing.api_key,
    enabled: patch.enabled !== undefined ? (patch.enabled ? 1 : 0) : existing.enabled,
    updated_at: nowISO(),
  };
  db.prepare(`
    UPDATE agents SET name=@name, emoji=@emoji, color=@color, system_prompt=@system_prompt,
      adapter=@adapter, model=@model, base_url=@base_url, api_key=@api_key, enabled=@enabled, updated_at=@updated_at
    WHERE id=@id
  `).run({ id, ...next });
  return getAgent(id);
}

function deleteAgent(id) {
  db.prepare("DELETE FROM agents WHERE id = ?").run(id);
}

// ── projects ────────────────────────────────────────────────────────
function listProjects() {
  return db.prepare("SELECT * FROM projects ORDER BY created_at").all();
}

function getProject(id) {
  return db.prepare("SELECT * FROM projects WHERE id = ?").get(id) || null;
}

function insertProject(p) {
  db.prepare(
    "INSERT INTO projects (id, name, description, color, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)"
  ).run(p.id, p.name, p.description ?? "", p.color ?? "#8a7ff0", nowISO(), nowISO());
  return getProject(p.id);
}

function updateProject(id, patch) {
  const existing = getProject(id);
  if (!existing) return null;
  db.prepare(
    "UPDATE projects SET name=?, description=?, color=?, updated_at=? WHERE id=?"
  ).run(
    patch.name ?? existing.name,
    patch.description ?? existing.description,
    patch.color ?? existing.color,
    nowISO(),
    id
  );
  return getProject(id);
}

function deleteProject(id) {
  db.prepare("DELETE FROM projects WHERE id = ?").run(id);
}

// ── rooms ───────────────────────────────────────────────────────────
function listRooms() {
  const rooms = db.prepare("SELECT * FROM rooms ORDER BY created_at").all();
  const members = db.prepare("SELECT room_id, agent_id FROM room_members").all();
  const counts = db
    .prepare("SELECT room_id, COUNT(*) AS c FROM messages GROUP BY room_id")
    .all();
  const countMap = Object.fromEntries(counts.map((r) => [r.room_id, r.c]));
  for (const room of rooms) {
    room.memberAgentIds = members.filter((m) => m.room_id === room.id).map((m) => m.agent_id);
    room.messageCount = countMap[room.id] || 0;
  }
  return rooms;
}

function getRoom(id) {
  const room = db.prepare("SELECT * FROM rooms WHERE id = ?").get(id);
  if (!room) return null;
  room.memberAgentIds = db
    .prepare("SELECT agent_id FROM room_members WHERE room_id = ?")
    .all(id)
    .map((r) => r.agent_id);
  return room;
}

function insertRoom(room) {
  db.prepare(
    "INSERT INTO rooms (id, name, project_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)"
  ).run(room.id, room.name, room.projectId ?? null, nowISO(), nowISO());
  for (const agentId of room.agentIds || []) {
    db.prepare("INSERT OR IGNORE INTO room_members (room_id, agent_id) VALUES (?, ?)").run(
      room.id,
      agentId
    );
  }
  return getRoom(room.id);
}

function updateRoom(id, patch) {
  const existing = getRoom(id);
  if (!existing) return null;
  db.prepare("UPDATE rooms SET name=?, project_id=?, updated_at=? WHERE id=?").run(
    patch.name ?? existing.name,
    patch.projectId !== undefined ? patch.projectId : existing.project_id,
    nowISO(),
    id
  );
  return getRoom(id);
}

function deleteRoom(id) {
  db.prepare("DELETE FROM messages WHERE room_id = ?").run(id);
  db.prepare("DELETE FROM room_members WHERE room_id = ?").run(id);
  db.prepare("DELETE FROM rooms WHERE id = ?").run(id);
}

function addRoomMember(roomId, agentId) {
  db.prepare("INSERT OR IGNORE INTO room_members (room_id, agent_id) VALUES (?, ?)").run(
    roomId,
    agentId
  );
}

function removeRoomMember(roomId, agentId) {
  db.prepare("DELETE FROM room_members WHERE room_id = ? AND agent_id = ?").run(
    roomId,
    agentId
  );
}

function getRoomAgents(roomId) {
  return db
    .prepare(`
      SELECT a.* FROM agents a
      JOIN room_members m ON m.agent_id = a.id
      WHERE m.room_id = ?
      ORDER BY a.created_at
    `)
    .all(roomId);
}

// ── messages ────────────────────────────────────────────────────────
function listMessages(roomId, limit = 200) {
  const rows = db
    .prepare(
      "SELECT * FROM (SELECT * FROM messages WHERE room_id = ? ORDER BY created_at DESC LIMIT ?) ORDER BY created_at"
    )
    .all(roomId, limit);
  return rows;
}

function getMessage(id) {
  return db.prepare("SELECT * FROM messages WHERE id = ?").get(id) || null;
}

function insertMessage(m) {
  db.prepare(`
    INSERT INTO messages (id, room_id, agent_id, sender_name, sender_kind, content, status, created_at, updated_at)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
  `).run(
    m.id,
    m.roomId,
    m.agentId ?? null,
    m.senderName,
    m.senderKind,
    m.content ?? "",
    m.status ?? "complete",
    nowISO(),
    nowISO()
  );
  return getMessage(m.id);
}

function updateMessage(id, { content, status }) {
  const existing = getMessage(id);
  if (!existing) return null;
  db.prepare("UPDATE messages SET content=?, status=?, updated_at=? WHERE id=?").run(
    content ?? existing.content,
    status ?? existing.status,
    nowISO(),
    id
  );
  return getMessage(id);
}

function failStaleStreaming() {
  db.prepare("UPDATE messages SET status='error', updated_at=? WHERE status='streaming'").run(
    nowISO()
  );
}

// ── tasks ───────────────────────────────────────────────────────────
function listTasks(projectId) {
  if (projectId) {
    return db
      .prepare("SELECT * FROM tasks WHERE project_id = ? ORDER BY position, created_at")
      .all(projectId);
  }
  return db.prepare("SELECT * FROM tasks ORDER BY position, created_at").all();
}

function insertTask(t) {
  db.prepare(`
    INSERT INTO tasks (id, project_id, title, notes, status, assignee, position, created_at, updated_at)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
  `).run(
    t.id,
    t.projectId ?? null,
    t.title,
    t.notes ?? "",
    t.status ?? "todo",
    t.assignee ?? null,
    t.position ?? 0,
    nowISO(),
    nowISO()
  );
  return db.prepare("SELECT * FROM tasks WHERE id = ?").get(t.id);
}

function updateTask(id, patch) {
  const existing = db.prepare("SELECT * FROM tasks WHERE id = ?").get(id);
  if (!existing) return null;
  db.prepare(`
    UPDATE tasks SET project_id=?, title=?, notes=?, status=?, assignee=?, position=?, updated_at=? WHERE id=?
  `).run(
    patch.projectId !== undefined ? patch.projectId : existing.project_id,
    patch.title ?? existing.title,
    patch.notes ?? existing.notes,
    patch.status ?? existing.status,
    patch.assignee !== undefined ? patch.assignee : existing.assignee,
    patch.position !== undefined ? patch.position : existing.position,
    nowISO(),
    id
  );
  return db.prepare("SELECT * FROM tasks WHERE id = ?").get(id);
}

function deleteTask(id) {
  db.prepare("DELETE FROM tasks WHERE id = ?").run(id);
}

module.exports = {
  open,
  getSettings,
  updateSettings,
  listAgents,
  getAgent,
  insertAgent,
  updateAgent,
  deleteAgent,
  listProjects,
  getProject,
  insertProject,
  updateProject,
  deleteProject,
  listRooms,
  getRoom,
  insertRoom,
  updateRoom,
  deleteRoom,
  addRoomMember,
  removeRoomMember,
  getRoomAgents,
  listMessages,
  getMessage,
  insertMessage,
  updateMessage,
  failStaleStreaming,
  listTasks,
  insertTask,
  updateTask,
  deleteTask,
};