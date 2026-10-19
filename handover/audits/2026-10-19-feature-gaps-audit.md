# ACP-Zug Feature-Gaps-Audit — Agent Relay vs. buzz / paperclip

**Datum:** 2026-10-19  
**Modus:** Review-Only — kein Bau  
**Scope:** `/mnt/192.168.178.68/share/projekte/github/agent-desktop-ha-app` (Agent Relay, HEAD ~e2cc4bc+) im Vergleich mit buzz (`/tmp/re-buzz`) und paperclip (`/tmp/re-paperclip`).

---

## TL;DR

Agent Relay hat aktuell eine schlanke „Räume + Nachrichten + Agents + Projekt-Tasks"-Basis. buzz und paperclip decken darüber hinaus fast alles ab, was GaRoN unter „so viele Funktionen" vermisst: Suche, Dateianhänge, Reaktionen, Threads, Notifications, Task-Chatter, Approvals, Skills, Voice, Mobile-Apps usw.

| Priorität | Anzahl Lücken | Kategorie |
|---|---|---|
| **P1 — sofort sichtbar** | 10 | Chat-UX, Suche, Attachments, Notifications |
| **P2 — Workflow & Orga** | 12 | Tasks, Approvals, Skills, Routines, DM |
| **P3 — später / strategisch** | 9 | Voice, Mobile, Multi-Tenant, Audit, Budgets |

**Top-3 Quick-Wins für sofortigen Bau:**
1. **Emoji-Reaktionen auf Nachrichten** (eigene Tabelle + POST/DELETE + WS-Push).
2. **Chat-Suche** (SQLite FTS über `messages.content`, GET-Endpoint, Frontend-Overlay).
3. **Online/Presence-Status** (Heartbeat/ping in WS oder REST, `last_seen`-Speicher, UI-Dot).

---

## Relay-Basis (was bereits existiert)

### Backend (`internal/api`, `internal/store`)
- REST: `settings`, `agents`, `projects`, `rooms`, `room/{id}/members`, `room/{id}/messages`, `tasks`
- WebSocket: `/ws` — Broadcast-Hub für `message.created`, `agent.started/token/finished`, `typing`, `rooms.changed`, `system`
- Store: SQLite mit Tabellen `settings`, `agents`, `projects`, `rooms`, `room_members`, `messages`, `tasks`
- Auth: optionaler Bearer-Token (REST + WS `?token=`)
- Adapter: `echo`, `openai`, `openclaw` (OpenClaw Gateway-WS native)
- Runtime: Mention-Trigger, sequentielle Agent-Turns, 60 Nachrichten Historie

### Frontend (`web/public/app.js` + `index.html` + `styles.css`)
- Views: Chat, Agents, Projekte, Einstellungen
- Chat: Nachrichtenliste, Composer mit `@Agent`-Mention, Typing-Indikator, inline Board (Tasks)
- UI-Details: Sidebar-Collapse, Identity-Card, Raum-Menü, Agent-einladen, drag&drop-Raum-Reihenfolge
- Kein Search-Overlay, keine Attachments, keine Reaktionen, keine Threads, keine Notifications, kein DM, kein Presence-Dot außer eigener Verbindung.

---

## Lücken-Matrix

Legende:
- **Quelle:** Wo das Feature im Original implementiert / geplant ist.
- **Relay-API (bräuchte):** neuer Endpoint + Store, der im Relay hinzugefügt werden müsste.
- **Aufwand:** `S` = 1–2 Tage, `M` = 3–5 Tage, `L` = 1–2 Wochen (Grobschätzung für einzelnen Entwickler, inkl. API + UI).

---

### P1 — Kern-nützlich, sofort sichtbar

| # | Feature | Was ist es beim Original | Relay-API (bräuchte) | Aufwand | Quelle |
|---|---|---|---|---|---|
| 1.1 | **Chat-Suche** | buzz: Postgres-FTS über `events.search_tsv` (kinds 9/40002/40003). paperclip: Company-wide Search über Tasks, Kommentare, Dokumente, Agents. | `GET /api/rooms/{id}/search?q=…` + SQLite FTS5 auf `messages.content`; optional global `GET /api/search`. | M | buzz ARCH 8 / paperclip ROADMAP |
| 1.2 | **Dateianhänge / Uploads** | buzz: `buzz-media` mit Blossom/S3-kompatiblem Storage, Bild-/Video-Vorschau. paperclip: `file-resources` mit Limiter, Attachments an Tasks/Nachrichten. | `POST /api/attachments` (multipart), Tabelle `attachments(message_id|task_id, url, mime, size)`, `GET /api/attachments/{id}`, WS `attachment.created`. | L | buzz ARCH 8 / paperclip `file-resources.ts` |
| 1.3 | **Notifications / Inbox** | paperclip: Inbox mit Dismissals, Badges, Attention-Routing („what needs me"). buzz: Nostr `#p`-Tags als Lightweight-Mentions. | Tabelle `notifications(user_id?, type, resource_id, read, created_at)`, `GET /api/notifications`, `PATCH /api/notifications/{id}/read`, WS `notification.created`, Sidebar-Badge. | M | paperclip `inbox-*`, `attention.ts`, `sidebar-badges.ts` |
| 1.4 | **Reaktionen (Emoji)** | buzz: Nostr `kind 7` (NIP-25) + eigene `add_reaction`-Workflow-Action. | Tabelle `reactions(message_id, agent_id?, emoji, created_at)`, `POST /api/messages/{id}/reactions`, `DELETE /api/messages/{id}/reactions/{emoji}`, WS `reaction.changed`. | S | buzz ARCH 5.2 / 6 |
| 1.5 | **Threads / Reply-on-Message** | buzz: Thread-IDs über `e`-/`a`-Tags in Nostr-Events. paperclip: Task-Chat / Issue-Chat als Thread-View. | `parent_id` in `messages`, `GET /api/threads/{id}`, UI-Reply-Button, WS `message.created` mit parent. | M | buzz ARCH 5.2 / paperclip `issue-tree-control.ts` |
| 1.6 | **Nachrichten editieren / löschen** | buzz: `KIND_STREAM_MESSAGE_EDIT` + `kind 5` Deletion. | `PATCH /api/messages/{id}` (content, status='deleted'), `DELETE /api/messages/{id}`, WS `message.updated`. | S | buzz ARCH 5.2 |
| 1.7 | **Presence / Online-Status** | buzz: Redis `buzz:presence:{pubkey}` mit TTL, `/api/presence`. paperclip: `user-profiles` + `voice-sessions` Presence. | `last_seen`-Spalte in `agents` + ggf. `presence` Tabelle; WS heartbeat oder `POST /api/heartbeat`; UI-Dot neben Member-Chips. | S | buzz ARCH 5.3 / 8 / `user-profiles.ts` |
| 1.8 | **Raum-Thema / Beschreibung** | buzz: Channel topic (`set_channel_topic` Workflow-Action). | `topic`/`description` in `rooms`, `PATCH /api/rooms/{id}` erweitern. | S | buzz ARCH 6 |
| 1.9 | **Mention-Push / @all** | buzz: `#p` / `#a` Tags, paperclip: Mention-Notifications. | Notification-Generierung bei Mention in `OnUserMessage`; `@all` expandiert zu allen Raum-Members. | S | buzz ARCH 5.2 / paperclip `attention.ts` |
| 1.10 | **Ungelesen-Zähler / Raum-Badges** | paperclip: `sidebar-badges` + `inbox-dismissals`. | Zähler `unread_count` pro Raum/Member, `GET /api/rooms` liefert es, WS aktualisiert. | M | paperclip `sidebar-badges.ts` |

---

### P2 — Workflow & Zusammenarbeit

| # | Feature | Was ist es beim Original | Relay-API (bräuchte) | Aufwand | Quelle |
|---|---|---|---|---|---|
| 2.1 | **Task-Chatter / Task-Kommentare** | paperclip: Jede Task/Issue hat einen persistenten Chat-Verlauf ("TaskChat"). | `comments` Tabelle (`task_id`, `author_kind`, `content`), `GET/POST /api/tasks/{id}/comments`, WS `task.comment.created`. | M | paperclip `task-workspaces.ts`, `components/task-chat/*` |
| 2.2 | **Approval-Gates für Agent-Aktionen** | paperclip: `approvals.ts`, `decisions.ts` mit Token/Timeout. buzz: Workflow `request_approval` (aktuell teilweise stub). | `POST /api/approvals` (Anfrage), `POST /api/approvals/{id}/grant|deny`, Tabelle `approvals`, Runtime hält Agent an, WS `approval.required`. | M | paperclip `approvals.ts` / buzz ARCH 6 |
| 2.3 | **Skills / Skill-Store** | paperclip: `company-skills.ts`, Skill Studio, Version History, Restore. | Tabelle `skills(id, name, instructions, version, parent_id)`, `GET/POST /api/skills`, Skill-Auswahl im Agent-Modal. | M | paperclip `company-skills.ts` |
| 2.4 | **Workflows / Routines (recurring Tasks)** | paperclip: `routines.ts`, `pipelines.ts`. buzz: `buzz-workflow` YAML-as-Code + Cron. | Tabelle `routines(id, title, cron, prompt, enabled)`, Cron-Loop im Runtime, `GET/POST /api/routines`. | L | paperclip `routines.ts` / buzz ARCH 6 |
| 2.5 | **Direct Messages (1:1)** | buzz: `KIND_DM` / NIP-17, paperclip: AgentChat. | `kind='dm'` in `rooms` oder eigene `direct_rooms` Tabelle, `POST /api/dm` (recipient), WS gleich. | M | buzz ARCH 5.2 / paperclip `AgentChat` |
| 2.6 | **Projekt-Workspaces / Board-Views** | paperclip: `projects.ts`, `task-workspaces.ts`, Kanban-ähnliche Board-View. | Board-Layout in `app.js` (statt nur Listen-Tab), Filter/Sortierung, Swimlanes nach Status. | M | paperclip UI `Projects`, `Workspaces` |
| 2.7 | **Entscheidungs-Warteschlangen** | paperclip: `decision-queues.ts` + `decision-models.ts`. | `GET /api/decisions/queue`, Tabelle `decisions`, UI-Queue-View. | M | paperclip `decisions.ts` |
| 2.8 | **Agent-Profile / Rollen / Memberships** | paperclip: `resource-memberships.ts`, `teams-catalog.ts`, `company-access.ts`. buzz: `channel_members` mit Rollen. | `roles` Tabelle, `room_members.role`, `GET /api/rooms/{id}/memberships`, Rollen-Prüfung im Handler. | M | paperclip `resource-memberships.ts` / buzz ARCH 7 |
| 2.9 | **Status-Cards / Dashboard** | paperclip: `status-cards.ts`, `dashboard.ts` — „what is happening, does it need me". | `GET /api/dashboard`, Aggregates aus Tasks/Approvals/Unread, neue View `view-dashboard`. | M | paperclip `dashboard.ts`, `status-cards.ts` |
| 2.10 | **Pin wichtiger Nachrichten** | buzz: Pinned messages über dedizierte kind (üblich in Chat-Apps). | `pinned_message_id` in `rooms` oder Tabelle `pinned_messages`, UI-Pin-Action. | S | buzz (Nostr-Idiom) |
| 2.11 | **Entwürfe (Drafts)** | paperclip: Message-Drafts in Composer. | `drafts(room_id, content, updated_at)`, auto-save, Restore beim Raumwechsel. | S | paperclip UI |
| 2.12 | **@Agent-Command-Slashbefehle** | paperclip: `/`-Commands, buzz: Workflow-Trigger. | Parser im Composer, `POST /api/commands`, z.B. `/summarize`, `/task`. | M | paperclip UI / buzz ARCH 6 |

---

### P3 — Später / strategisch

| # | Feature | Was ist es beim Original | Relay-API (bräuchte) | Aufwand | Quelle |
|---|---|---|---|---|---|
| 3.1 | **Voice / Huddle** | buzz: WebSocket-Opus-Relay `/huddle/{channel_id}/audio`, 25 peers. paperclip: `voice-sessions.ts`. | Neuer WS-Pfad `/huddle/{room_id}/audio`, In-Memory-Rooms, Opus-Forward, Permission-Check. | L | buzz ARCH 7 / paperclip `voice-sessions.ts` |
| 3.2 | **Mobile-App / PWA** | buzz: Mobile-Client (Tauri-Shell), paperclip: `pnpm dev:mobile`. | Responsive CSS, Service-Worker, Push-Notifications (optional). | L | buzz VISION_MOBILE / paperclip README |
| 3.3 | **Desktop-App** | buzz: Tauri-Desktop-Shell. paperclip: Desktop-App auf Roadmap. | Tauri/Electron Wrapper um die SPA — außerhalb HA-Add-on Scope. | L | buzz ARCH 2 / paperclip ROADMAP |
| 3.4 | **Multi-Tenant / Communities** | buzz: Host-derived `community_id`, Multi-Company in paperclip. | `communities`/`companies` Tabelle, Tenant-Scoping in allen Queries, JWT/Bearer-Company-Mapping. | L | buzz ARCH 1 / paperclip ROADMAP |
| 3.5 | **Audit-Log / Hash-Chain** | buzz: `buzz-audit` mit Postgres, 10 Aktionen, Hash-Chain. | `audit_log` Tabelle, Event-Listener in allen mutierenden Handlern, Export-Endpoint. | L | buzz ARCH 5.5 |
| 3.6 | **Budgets / Kosten-Tracking** | paperclip: `costs.ts`, Budget-Enforcement. | `budgets`/`costs` Tabellen, Agent-Run-Cost-Tracking, Warn/Block-Logik. | L | paperclip `costs.ts`, ROADMAP |
| 3.7 | **Git-Integration (Repos, Commits, Tree)** | buzz web/src: `features/repos/*` (Repo-Browse, Commits, Blob). paperclip: GitHub-Identity bei Execution. | `/git/*` Endpoints, Git-Client, Repo-Storage — außerhalb reinen Chat-Relay. | L | buzz `web/src/features/repos` / paperclip ROADMAP |
| 3.8 | **Externe Integrations (Slack, Email, Calendar)** | paperclip: `slack-tools.ts`, `email.ts`, Chat-Connectors. | Adapter-ähnliche Integrations-Layer, Webhook-Endpoints, OAuth-Flows. | L | paperclip `slack-tools.ts`, `email.ts`, ROADMAP |
| 3.9 | **Memory / Knowledge (Mem0, Zep, LLM-Wiki)** | paperclip: Experimentelle Memory-Connections, `company-skills` als Wissen. | Knowledge-Store + Retrieval, Embeddings, Integration mit Agent-Harness. | L | paperclip ROADMAP / `memory connections` |

---

## Quick-Wins: Empfohlene Reihenfolge für den sofortigen Bau

Die folgenden drei Features liefern den größten sichtbaren Mehrwert bei minimalem Risiko und passen in die bestehende SQLite-Mono-Node-Architektur.

### 1. Emoji-Reaktionen auf Nachrichten
- **Wert:** Macht Relay sofort „chat-affiner"; GaRoN vermisst Interaktivität.
- **Backend:** Neue Tabelle `reactions(message_id TEXT, agent_id TEXT, emoji TEXT, created_at TEXT)`, Composite-PK `(message_id, agent_id, emoji)`. Endpoints:
  - `POST /api/messages/{id}/reactions` mit Body `{emoji}`
  - `DELETE /api/messages/{id}/reactions/{emoji}`
  - WS-Push `reaction.changed` mit Aggregat `{messageId, counts: {emoji: n}, userReactions: [...]}`
- **Frontend:** Reaktions-Bubble unter jeder Nachricht; Emoji-Picker-Popover.
- **Aufwand:** **S** (1–2 Tage).

### 2. Chat-Suche (lokal, pro Raum)
- **Wert:** Sobald mehrere Agenten viele Nachrichten produzieren, ist Suche essenziell.
- **Backend:** SQLite FTS5 virtual table `messages_fts` über `content`; `GET /api/rooms/{id}/messages?q=…&limit=20` erweitert `ListMessages`. Kein zusätzlicher Service nötig.
- **Frontend:** Search-Input im Chat-Header, Overlay mit Trefferliste + Sprung zum Kontext.
- **Aufwand:** **S/M** (2–3 Tage).

### 3. Presence / Online-Status der Raum-Member
- **Wert:** Operator sieht auf einen Blick, welche Agents „da" sind.
- **Backend:** Spalte `last_seen` in `agents`; WS-Ping alle 30 s aktualisiert sie; `GET /api/rooms/{id}/presence` liefert `agent_id → last_seen`.
- **Frontend:** Farbiger Dot im `memberChips` (grün < 2 min, gelb < 5 min, grau sonst).
- **Aufwand:** **S** (1–2 Tage).

---

## Handoff

**10 | 12 | 9** | **Reaktionen + Chat-Suche + Presence** | `coordinator/audits/raw/2026-10-19-feature-gaps-audit.md`
