# Architecture — Agent Relay

Stand: 2026-10-18 (nach Reverse-Engineering von buzz + Paperclip, siehe
`docs/REVERSE-ENGINEERING.md`).

## Rolle im Ökosystem

Agent Relay ist **keine Agent-Software**. Es ist ein Relay: ein eigener
HTTP(S)/WS-Server mit einer Desktop-artigen Web-Oberfläche, der externe
Agent-Laufzeiten (OpenClaw, Hermes, …) **benutzt** — das gleiche Muster wie das
OpenClaw-Konzept Gateway + Control UI, hier mit HA-Ingress als Shell.

```
┌────────────────────────────────────────────────────────────┐
│ HA-Shell: Sidebar-Panel / Ingress / Browser                │
└────────────────────────────────────────────────────────────┘
                        │ HTML/WS
┌────────────────────────────────────────────────────────────┐
│ agentrelay (ein Go-Binary)                                 │
│  ├─ web/  — eingebettete SPA = Desktop-Oberfläche          │
│  ├─ api/  — REST + WebSocket-Hub (Live-Events)             │
│  ├─ runtime — Room-Turn-Planung (Mentions, Fan-out)        │
│  ├─ store — SQLite (source of truth, wie buzz-Relay-Log)   │
│  └─ adapter — Agent-Laufzeit-Anschlüsse (austauschbar)     │
│        ├─ echo     (Demo/Smoke, immer da)                  │
│        ├─ openai   (OpenAI-compat: OpenClaw /v1, Ollama,…) │
│        └─ openclaw (natives Gateway-WS-Protokoll)          │
└────────────────────────────────────────────────────────────┘
        │ adapter-Aufruf                     │ adapter-Aufruf
┌───────▼────────┐                 ┌─────────▼────────┐
│ OpenClaw GW    │  (ws://…:18789) │ Ollama / llama…  │
└────────────────┘                 └──────────────────┘
```

## Entscheidungen (mit Begründung)

1. **Go + modernc.org/sqlite (pure Go, CGO off):** statisches Binary ~11 MB,
   Cross-Compile amd64/arm64 ohne Emulation, kein npm/node_modules im Image.
   Node- und Postgres-Setups (buzz) sind für ein HA-Add-on zu schwer
   (Abschreckungs-Referenz: unsere lokalen Feldversuche).
2. **Relay-first:** alle Zustände in einer SQLite-DB (WAL; HAOS-/Home-FS ok,
   SMB/NFS vermieden durch Konfiguration `RELAY_DATA_DIR`). Die SPA hält nur
   Projektion + Live-Events (buzz-Prinzip: Clients = Projektion, Relay = Wahrheit).
3. **Desktop = SPA:** keine Tauri-Shell v1 (buzz-Vorbild), weil HA Ingress und
   Browser die Shell-Rolle übernehmen. Eine spätere Tauri-Shell kann dieselbe
   SPA laden.
4. **Adapter statt Agenten-Verwaltung:** Agent-Runtime-Laufzeiten (OpenClaw,
   Hermes, Claude Code …) hängen als Adapter an; Agent-Einträge in der DB sind
   „Anschlüsse" (Name, Emoji, Adapter, Modell/Agent-ID, URL, Token, Prompt).
5. **Sessions pro Raum (Paperclip sessionKeyStrategy):** `openclaw`-Adapter
   leitet `sessionKey = agent:<agentId>:relay:<roomId>` — die OpenClaw-Session
   trägt den Verlauf selbst; SQLite archiviert die Chat-Protokolle für die UI.
   `idempotencyKey = messageId` macht Retries gefahrlos.
6. **Turn-Modell:** Raum-Nachricht → Mention-Matching (`@Name`) → an getaggte
   Agents sonst alle aktivierten im Raum, sequenziell (900 ms Versatz), Token
   live via WS-Events (`agent.started/typing/token/finished`).
7. **Kein Login in v1** — HA-Ingress authentifiziert; LAN-Nutzung explizit.

## Adapter-Vertrag (Go)

```go
type Adapter interface {
    ID() string
    Validate(cfg AgentConfig) error
    Stream(cfg AgentConfig, history []Message) (<-chan Chunk, error)
}
```

Der `openclaw`-Adapter (nativ): connect.challenge → connect (auth token) →
`agent`-RPC (message/sessionKey/idempotencyKey/agentId) → `agent.wait`
Terminal-Snapshot → als ein Chunk zurückgeben. Live-`agent`-Event-Streaming
(runId/seq-Deltas) ist Phase 2, sobald wir Event-Shapes gegen ein echtes Gateway
verifiziert haben (Paperclip-Parser-Logik als Referenz: execute.ts ~L1209).

## Verlaufspersistenz

- `messages` speichert humane Nach- und Agent-Antworten (inkl. Fehlerstatus).
- `openai`-Adapter bekommt den Verlauf als OpenAI-Messages; `openclaw`-Adapter
  sendet nur die Trigger-Nachricht (OpenClaw-Session erinnert sich selbst) —
  Verlauf bleibt trotzdem hier archiviert.
- Boot-Recovery: hängende `streaming`-Nachrichten werden beim Start auf
  `error` gesetzt.

## Erweiterungspfad (Roadmap)

- Hermes-Adapter (ACP/ws wie Paperclip `hermes-gateway`), ACP-Bridge für
  Coding-Agents (buzz-acp-Vorbild).
- Live-Event-Streaming aus nativen Adapters (runId/seq-Korrektur wie
  Gateway-Clients-Doku: reconnect → history-Reload, seq je Run).
- Attachments, Markdown-Rendering, Task-Kommentare wie Paperclip Task-Chats.