# ⌁ Agent Relay — Multi-Agent-Chats für die Home-Assistant-Sidebar

**agent-desktop-ha-app** ist ein Relay: ein einzelner Go-Server mit einer
eingebetteten Web-Oberfläche, der externe Agent-Laufzeiten (OpenClaw, Ollama,
Hermes, …) **benutzt**, anstatt sie zu enthalten. Konzipiert als Home-Assistant
App (Add-on) für das Sidebar-Panel — „Desktop-App-Gefühl" in der Companion-App.

Vorbilder (reverse engineered, siehe [docs/REVERSE-ENGINEERING.md](docs/REVERSE-ENGINEERING.md)):
[Paperclip](https://github.com/paperclipai/paperclip) (Agents org-aufgebaut,
Task-Chats, Approvals) und [buzz](https://github.com/block/buzz) (Relay + Raum
als erste-Klasse-Ort für Menschen *und* Agents).

> Architektur-Kern: **Relay-first, ein Binary.** SQLite als Quelle der Wahrheit,
> Live-Events per WebSocket, Desktop-Oberfläche = eingebettete SPA (wie Control
> UI bei OpenClaw). Agent-Runtimes hängen als austauschbare Adapter an.

## Features (v0.1)

- **Multi-Agent-Räume** wie buzz: du + mehrere Agents im selben Chat; Agents
  antworten sequenziell, `@Name` adressiert gezielt einen Agent.
- **Agents-Pflege** in der UI: Name, Emoji, Farbe, System-Prompt, Adapter.
- **Projekte + Task-Board** (todo/doing/done) mit Agent-Zuordnung
  (Paperclip-Stil: Control-Plane, „Was passiert – braucht es mich – was mache ich").
- **Einstellungen**: Relay-Name, Gateway-Basis-URL, Token.
- **Adapter**:
  - `echo` — Demo-Agent (immer verfügbar)
  - `openai` — OpenAI-kompatible Chat-Completions (OpenClaw `/v1`,
    Ollama, llama.cpp, vLLM …)
  - `openclaw` — **natives OpenClaw-Gateway-Protokoll** über WebSocket
    (`connect.challenge → connect → agent → agent.wait`, Session-Bindung
    `relay:<agentId>`), nach dem Vorbild des Paperclip-Adapters
- **Live-Streaming**: Token-Fenster per WebSocket (`agent.started / typing /
  agent.token / agent.finished`).
- Persistente SQLite-DB (WAL), Crash-Recovery (hängende Streaming-Nachrichten
  werden beim Mark auf error gesetzt), SPA ohne Build-Toolchain.

## Lokale Entwicklung

```bash
./bin/build.sh                       # Cross-Compile amd64 + arm64 (11 MB)
./dist/agentrelay-amd64              # startet auf :8099
# Browser: http://localhost:8099
```

Datenbank-Ort: `$RELAY_DATA_DIR` bzw. `~/.agent-relay` (SQLite braucht echte
fcntl-Locks — niemals auf SMB/NFS-Mounts betreiben).

## Home-Assistant-Installation (Add-on)

1. Repo-Ordner `agent-relay/` enthält die App (`config.yaml`, `Dockerfile`,
   `run.sh`); zugehöriges Repository über **Einstellungen → Add-ons →
   Add-on-Shop → ⋮ → Repository hinzufügen** einbinden, oder Ordner nach
   `/config/addons/agent_relay` legen.
2. Binary vor den Container-Bau erzeugen: `./bin/build.sh` (liefert
   `dist/agentrelay-amd64` und `dist/agentrelay-arm64`).
3. App starten → Sidebar-Panel **„Agent Relay"** öffnen.

### Add-on-Optionen

| Option | Standard | Zweck |
| --- | --- | --- |
| `relay_user_name` | `gaRON` | Anzeigename für deine Nachrichten |
| `openclaw_base_url` | `http://localhost:18789/v1` | Gateway-/LLM-Basis-URL |
| `openclaw_token` | – | Bearer (OpenClaw-Gateway-Token, Ollama braucht nichts) |
| `log_level` | `info` | Loglevel |

Für OpenClaw nativ (statt `/v1/...`): Agent in der UI anlegen, Adapter
`openclaw`, Basis-URL `ws://<gateway-host>:18789`, Token = Gateway-Token,
Modell-Feld = Ziel-Agent-ID (leer/`openclaw` = Default-Agent).

## API-Überblick

| Methode | Pfad | Zweck |
| --- | --- | --- |
| GET | `/api/health` | Status + Adapterliste |
| GET/POST/PATCH/DELETE | `/api/agents[/:id]` | Agents |
| GET/POST/PATCH/DELETE | `/api/projects[/:id]` | Projekte |
| GET/POST/PATCH/DELETE | `/api/rooms[/:id]` | Räume (+ `POST/DELETE …/members`) |
| GET/POST | `/api/rooms/:id/messages` | Verlauf / Nachricht senden |
| GET/POST/PATCH/DELETE | `/api/tasks[/:id]` | Task-Board |
| GET/PATCH | `/api/settings` | Einstellungen |
| WS | `/ws` | `hello, message.created, typing, agent.started, agent.token, agent.finished, rooms.changed, system` |

## Sicherheit

- LAN-lokal; Auth über HA-Ingress. Kein Cloud, kein Tracking.
- Tokens nur als Env über die Add-on-Optionen (`openclaw_token`), nicht im Repo.
- Adapter senden den Verlauf nur an die konfigurierte Agent-Basis-URL.

## Lizenz

MIT — [LICENSE](LICENSE).