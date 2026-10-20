# Backend-Reverse-Engineering: Paperclip (github.com/paperclipai/paperclip)

**Datum:** 2026-10-20  
**Lizenz der untersuchten Software:** MIT  
**Quellen:** HA-Add-on-Wrapper unter `/share/projekte/github/paperclip-ha-addon/paperclip` (Dockerfile, config.yaml, run.sh) plus bereits bekannte Architektur-Dokumentation aus `agent-desktop-ha-app/docs/REVERSE-ENGINEERING.md`. Ein direktes shallow-clone von `https://github.com/paperclipai/paperclip.git` wurde von der Sandbox abgelehnt (Netzwerk/Approval-Blockade); das Original-Repo konnte daher nicht statisch inspiziert werden.  
**Bezug zu Agent Relay:** Paperclip definiert den Adapter-Vertrag für externe Agent-Laufzeiten und liefert mit `packages/adapters/openclaw-gateway` die Referenz-Implementierung für den nativen OpenClaw-Gateway-Transport. Dieses Dokument fasst den bekannten Control-Plane-Aufbau und Adapter-Kontrakt zusammen.

---

## 1. Verifikation des lokalen Repo-Umfangs

```
/share/projekte/github/paperclip-ha-addon/paperclip/
  ASSETS.md
  DOCS.md
  Dockerfile
  apparmor.txt
  build.yaml
  config.yaml
  healthcheck.sh
  run.sh
  translations/
```

Das Verzeichnis enthält ausschließlich einen Home-Assistant-Add-on-Wrapper für das echte `paperclipai/paperclip`-Repo. Es liegt **kein** Paperclip-Quellcode vor:

- `DOCS.md` und `README.md` (letzteres fehlt im lokalen Wrapper) verweisen auf das Original-Repo.
- `config.yaml` definiert das Add-on `paperclip`, Port 3100, Ingress, SQLite/Postgres-Optionen, OpenClaw-Integration.
- `run.sh` konfiguriert `DATABASE_URL` (SQLite/Postgres), `OPENCLEW_URL`, `OPENCLEW_API_KEY` etc.
- `Dockerfile` baut vermutlich das upstream Paperclip-Image nach; ohne Netzwerkzugriff nicht weiter aufschlüsselbar.

**Lücke:** Primärquellen (Node/pnpm-Monorepo, `server/`, `packages/adapters/*`, `packages/adapters/openclaw-gateway/execute.ts`) fehlen. Die folgenden Abschnitte basieren auf der in unserem Projekt vorhandenen Sekundärdokumentation und dem lokalen Wrapper.

---

## 2. Architektur-Überblick (Control Plane)

Quelle: `agent-desktop-ha-app/docs/REVERSE-ENGINEERING.md`.

Paperclip ist eine **operational control plane** für Agent-Organisationen. Kernkonzepte:

- **Orgs, Tasks, Runs, Budgets, Approvals, Audit** – domain-getriebene Datenmodellierung.
- **Adapter-Architektur** – jede Agent-Runtime (OpenClaw, Hermes, Claude Code, Cursor, …) ist ein eigenes Paket unter `packages/adapters/*`.
- **Chat-Direktmodell** – Tasks haben einen Chat zwischen Nutzer und Agenten; Agent-Nachrichten links, Nutzer-Nachrichten rechts.
- **UI-Prinzipien** aus `DESIGN.md`: Dichte durch Information, ein Begriff pro Konzept, Status-Badges, maschinenlesbare Werte in Monospace.

Für Agent Relay ist der Adapter-Kontrakt das Wichtigste; die Control-Plane-Domäne (Orgs/Budgets/Approvals) ist für ein HA-Add-on überdimensioniert.

---

## 3. Datenmodell (Orgs / Tasks / Runs / Budgets / Approvals / Audit)

Quelle: `agent-desktop-ha-app/docs/REVERSE-ENGINEERING.md` + Paperclip-DOCS.md (lokal).

### 3.1 Entitäten

| Entität | Zweck | Für Agent Relay |
|---------|-------|-----------------|
| **Org** | Mandant / Team | Nicht v1; HA-Ingress trennt Benutzer implizit. |
| **Task** | Zu erledigende Arbeitseinheit; hat Chat/Comments | Konzept übertragbar auf „Ziele/Projekte" mit Board; bei uns `projects` + `tasks`. |
| **Run** | Konkrete Agent-Ausführung | Entspricht unserem `messages`-Status `streaming` / `complete` / `error`. |
| **Budget** | Kosten-/Token-Limits pro Task/Org | Nicht v1. |
| **Approval** | Menschliche Freigabe für kritische Aktionen | Nicht v1; später via Home Assistant notify/events. |
| **Audit** | Nachvollziehbarkeit aller Runs und Approvals | Bei uns implizit durch `messages` + zukünftiges `audit_log`. |

### 3.2 Status-Badges und Monospace

- Paperclip verwendet ein konsistentes Token-Set für Status-Badges über alle Adapter hinweg.
- Maschinenwerte (IDs, Hashes, Run-IDs) werden in Monospace dargestellt.
- Für Agent Relay: wir übernehmen das UI-Prinzip, implementieren es aber in der eingebetteten SPA.

---

## 4. Adapter-Kontrakt (`packages/adapters/*`)

Quelle: `agent-desktop-ha-app/docs/REVERSE-ENGINEERING.md`.

### 4.1 Paket-Kontrakt pro Adapter

Jedes Adapter-Paket implementiert:

- **connect** – Verbindung zur Laufzeit aufbauen, Credentials validieren.
- **execute** – Nachricht/Trigger an Agent übergeben, Stream empfangen.
- **stream** – Zwischenergebnisse / Token / Tool-Events zurückgeben.
- **UI-Ausnahmebehandlung** – Adapter-spezifische Fehlerdarstellung (z. B. Sandbox-Policy, Rate-Limit).

### 4.2 Vergleich mit Agent Relay

Agent Relay hat einen ähnlichen Vertrag in Go (`internal/adapter/adapter.go`):

```go
type Adapter interface {
    ID() string
    Validate(cfg AgentConfig) error
    Stream(cfg AgentConfig, history []Message) (<-chan Chunk, error)
}
```

Unterschiede zu Paperclip:

- Paperclip-Adapter sind feingranularer (connect, execute, stream, UI-Exception).
- Agent Relay reduziert auf `Stream`, da wir eine einheitliche Chunk-Semantik über WS-Hub haben.
- Paperclip hat pro Adapter eigene UI-Komponenten; Agent Relay zeigt Adapter-Fehler generisch an.

---

## 5. OpenClaw-Gateway-Adapter (`packages/adapters/openclaw-gateway`)

Quelle: `agent-desktop-ha-app/docs/REVERSE-ENGINEERING.md` (Referenz auf `execute.ts`) + `internal/adapter/openclaw.go`.

### 5.1 Transport

- Nativer Gateway-WebSocket-Transport (`ws://` / `wss://`).
- Nicht OpenAI-Compat; damit werden `runId`, `seq`, `sessionKey`, `agent.wait` genutzt.

### 5.2 Ablauf

1. `connect.challenge`-Event mit Nonce empfangen.
2. `req connect` mit `protocol`, `client`, `role`, `scopes`, `auth.token`, optional `device` senden.
3. `hello-ok` empfangen.
4. `req agent` senden mit:
   - `message`
   - `sessionKey`
   - `idempotencyKey = runId`
   - `agentId`
   - `timeout`
5. `event agent`-Frames lesen (`runId` + `seq`).
6. `req agent.wait` für Terminal-Snapshot.

### 5.3 Session-Strategien

Quelle: `agent-desktop-ha-app/docs/REVERSE-ENGINEERING.md`.

Paperclip-Adapter unterstützt Strategien:

- `issue` – pro Issue/Task eigene Session.
- `fixed` – feste Session pro Agent.
- `run` – pro Run eine Session (kein Kontextübertrag).

Zusätzlich gibt es das Präfixmuster:

```
agent:<agentId>:<sessionKey>
```

aus `execute.ts` (`prefixSessionKeyForAgent`).

### 5.4 Für Agent Relay abgeleitete Session-Bindung

Agent Relay verwendet:

```
sessionKey = agent:<agentId>:relay:<roomId>
```

Begründung:

- `agentId` muss im Key enthalten sein, da OpenClaw Policy-Matching erzwingt.
- `relay:` Namespace trennt Agent-Relay-Sessions von nativen OpenClaw-Clients.
- `roomId` isoliert pro Raum, damit unterschiedliche Diskussionen nicht ineinander laufen.
- `idempotencyKey = messageId` macht Retries gefahrlos.

Siehe `internal/adapter/openclaw.go:115-126` und `ARCHITECTURE.md:§5`.

### 5.5 idempotencyKey = runId

- Paperclip setzt `idempotencyKey` auf die `runId` des Tasks.
- Agent Relay setzt sie auf die `messageId`, weil wir kein separates Task-Run-Modell haben; eine Nachricht = ein Run.
- Bei Retry derselben `messageId` liefert OpenClaw ggf. dasselbe Ergebnis oder startet keinen neuen Run.

---

## 6. OpenClaw-Integration im HA-Add-on-Wrapper

Quelle: `paperclip-ha-addon/paperclip/config.yaml` + `run.sh` (lokal).

### 6.1 Konfiguration

```yaml
openclaw:
  enabled: true
  url: "http://your-openclaw-url:18790"
  api_key: ""
```

Diese Konfiguration wird in `run.sh` auf Umgebungsvariablen `OPENCLEW_URL` / `OPENCLEW_API_KEY` gemappt.

### 6.2 Datenbank

```yaml
database:
  type: "sqlite"   # oder postgres
  sqlite_path: "/share/paperclip/paperclip.db"
```

Paperclip unterstützt also ebenfalls SQLite für einfache Deployments – bestätigt unsere Wahl, SQLite statt Postgres zu verwenden.

### 6.3 Deployment-Modi

```yaml
deployment:
  mode: "authenticated"   # oder unauthenticated
  exposure: "private"     # oder public
```

Für Agent Relay v1 ist HA-Ingress die Auth-Schicht; LAN-Nutzung explizit, kein separater Login.

---

## 7. UI-Prinzipien für Agent Relay

Quelle: `agent-desktop-ha-app/docs/REVERSE-ENGINEERING.md` (Paperclip-DESIGN.md).

### 7.1 Übernommene Prinzipien

- **„What is happening → does it need me → what do I do about it"** – hierarchische Informationsdichte.
- **Ein Begriff pro Konzept** – bei uns: `project`, `room`, `agent`, `message`, `task`.
- **Status-Badges** aus einem Token-Set; bei uns `status` (`streaming`, `complete`, `error`).
- **Maschinenwerte in Monospace** – z. B. Run-IDs, Zeitstempel.

### 7.2 Chat-Direktmodell

- Nutzerkommentare rechts, Agent-Antworten links mit Name/Avatar.
- Execution-Hinweise kontextabhängig ein-/ausblendbar.
- Session-Reset-Grenzen sichtbar.

Für Agent Relay: SPA zeigt Rooms als Streams (buzz-Metapher) und DMs als 1:1-Chats; Agent-Antworten werden mit Name/Emoji dargestellt.

---

## 8. Offene / ungeklärte Punkte (wegen fehlendem Original-Repo)

1. **Exaktes Adapter-Interface in TypeScript.** `packages/adapters/*/src/index.ts` nicht einsehbar; Schnittstellen-Shapes abgeleitet aus REVERSE-ENGINEERING.md.
2. **`execute.ts` vollständiger Code.** `prefixSessionKeyForAgent`, Session-Strategien, Retry-Logik nur aus Sekundärdoku bekannt.
3. **Control-Plane-Schema für Orgs/Tasks/Runs/Budgets/Approvals.** Konkrete DDL/REST-Routen unbekannt.
4. **Audit-Log-Modell und Speicherung.** Ungeklärt, ob Event-Sourced oder relational.
5. **Approval-Workflow-Engine.** Nicht lokal vorhanden.
6. **Hermes-Adapter und ACP-Bridge.** `packages/adapters/hermes` / `hermes-gateway` nicht inspiziert.
7. **OpenAI-Compat-Adapter-Implementierung.** Paperclip dürfte neben nativem OpenClaw-WS auch OpenAI-Compat-Adapter haben; Details unbekannt.

---

## 9. Lizenz-Hinweis

Paperclip (github.com/paperclipai/paperclip) steht unter der MIT-Lizenz. Dieses Dokument enthält keine Code-Kopien aus dem Paperclip-Repo; es dokumentiert ausschließlich bekannte Architekturmerkmale und leitet Übertragungsempfehlungen für Agent Relay ab.
