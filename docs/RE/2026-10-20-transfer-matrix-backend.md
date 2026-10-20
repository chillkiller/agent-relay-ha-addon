# Transfer-Matrix: Backend-Erkenntnisse → Agent Relay

**Datum:** 2026-10-20  
**Entity-Modell FINAL in Agent Relay:**

- `projects` = Ziele mit Board (buzz-Gruppen / Paperclip-Tasks-Ebene).
- `rooms` = Streams (buzz-Channels). `kind IN ('chat','project','dm')`. `human_member` immer true, da HA der menschliche Benutzer ist.
- `dms` = 1:1 e2e mit genau einem Agenten (`kind = 'dm'`).
- `agents` = Adapter-Anschlüsse (Name, Emoji, Farbe, System-Prompt, Adapter, Modell, URL, Token).
- `room_members` = M:N zwischen `rooms` und `agents`.
- `messages` = Chat-Protokoll (menschlich/agent/system, Status `streaming`/`complete`/`error`).
- `tasks` = Aufgaben innerhalb eines Projekts (Paperclip-Task-Vorbild).
- `reactions` = Emoji-Reaktionen auf Nachrichten (buzz-Vorbild).

**Technologie-Constraints:**

- Ein statisches Go-Binary, modernc.org/sqlite (CGO-frei), eingebettete SPA.
- HA-Ingress als Shell; kein separater Desktop-Client.
- Single-Node; kein Redis/Postgres im v1-Image.

---

## Legende

| Symbol | Bedeutung |
|--------|-----------|
| ✅ 1:1 übernehmen | Passt ohne Änderung an unser SQLite/Go-Modell. |
| ⚠️ anpassen | Prinzip übernehmen, Implementierung anpassen (Datenmodell, Skalierung, Auth). |
| ❌ bewusst NICHT | Nicht passend für HA-Add-on / unser Scope / überdimensioniert. |
| ❓ ungeklärt | Blockiert durch fehlenden Zugriff auf Original-Repo; Entscheidung offen. |

---

## 1. OpenClaw Gateway

| # | Erkenntnis | Transfer | Begründung / Bezug zu Entity-Modell |
|---|------------|----------|-------------------------------------|
| 1.1 | Protokoll-Handshake `connect.challenge → req connect → hello-ok` | ✅ 1:1 übernehmen | `internal/adapter/openclaw.go` implementiert bereits den Handshake. Pfad bleibt. |
| 1.2 | WS + HTTP multiplex auf Port 18789 | ✅ 1:1 übernehmen | `gatewayWSURL()` wandelt `http://` → `ws://` auf gleichem Host:Port. Passt zu HA-Add-on, wo Gateway auf festem Port läuft. |
| 1.3 | Token-Auth (`auth.token`) | ✅ 1:1 übernehmen | Am besten für HA-Add-on geeignet; Device-Pairing würde Add-on-Komplexität erhöhen. |
| 1.4 | Device-Pairing + `deviceToken` | ❌ bewusst NICHT | Kein Geräte-Setup in v1; HA-Ingress authentifiziert implizit. |
| 1.5 | `sessionKey = agent:<agentId>:relay:<roomId>` | ✅ 1:1 übernehmen | Bereits in `internal/adapter/openclaw.go` und `ARCHITECTURE.md:§5`. Isoliert Verlauf pro Raum, erfüllt OpenClaw-Agent-Matching. |
| 1.6 | `idempotencyKey = messageId` | ✅ 1:1 übernehmen | In `internal/adapter/openclaw.go:115` als Fallback; geplant als Primärwert. Nachricht = Run; Retries gefahrlos. |
| 1.7 | `agent` → Events (`runId`+`seq`) → `agent.wait` Terminal | ⚠️ anpassen | Phase 1: aktuell `agent` + `agent.wait` als Blocking-Call. Phase 2: Event-Subscription auf gleicher WS-Verbindung; `seq`-Gap-Handling vereinfachen, da wir SQLite als Wahrheit haben und nicht nur Projektion. |
| 1.8 | OpenAI-Compat `/v1/chat/completions` | ⚠️ anpassen | Bereits als zweiter Adapter `openai` vorhanden. Modell-Name `openclaw/<agentId>` intern interpretieren. |
| 1.9 | `/v1/responses` Endpunkt | ❌ bewusst NICHT | Nicht nötig; v1 konzentriert sich auf Chat + Gateway. |
| 1.10 | Config-RPCs (`config.get/patch/apply`) | ❌ bewusst NICHT | Agent Relay hat eigene Config (Add-on-Options + SQLite); Gateway-Config soll nicht aus unserem UI editiert werden (Scope-/Komplexitäts-Gründe). |
| 1.11 | Rate-Limiting 30 req/60s für Config-Writes | ❌ bewusst NICHT | Betrifft Gateway-interne Config; unser Adapter macht nur Agent-Calls. |
| 1.12 | Pro-Agent SQLite-Store für Sessions/Transkripte | ❌ bewusst NICHT | Gateway ist extern; Agent Relay speichert eigenes `messages`-Schema. Wir duplizieren den Gateway-Store nicht. |
| 1.13 | `policy.maxPayload` / `policy.attachments` aus `hello-ok` | ⚠️ anpassen | Attachment-Limits können beim `openclaw`-Adapter geprüft werden, aber v1 hat keine Attachments. Für Phase 2 relevant. |
| 1.14 | `hello-ok.features.methods/events` | ✅ 1:1 übernehmen | Nutzbar für Capability-Discovery: fehlt `agent.wait`? → Fallback auf OpenAI-Compat. |
| 1.15 | `chat.abort { runId }` | ⚠️ anpassen | Übernehmen, sobald echtes Streaming da. Braucht Mapping `messageId → runId` in unserem Store. |
| 1.16 | `operator.read` / `operator.write` Scopes | ✅ 1:1 übernehmen | Das ist der Mindest-Scope-Set für unseren Adapter. |
| 1.17 | Admin-HTTP-RPC `/api/v1/admin/rpc` | ❌ bewusst NICHT | Nicht v1; würde Admin-Scope erfordern. |
| 1.18 | `system.presence` Heartbeat | ❓ ungeklärt | Nützlich für „Gateway erreichbar"-Status, aber Shape nicht vollständig bekannt. |

---

## 2. buzz (block/buzz)

| # | Erkenntnis | Transfer | Begründung / Bezug zu Entity-Modell |
|---|------------|----------|-------------------------------------|
| 2.1 | Relay-first: ein Binary, WS + HTTP, State im Relay | ✅ 1:1 übernehmen | Agent Relay ist genau so aufgebaut (`cmd/agentrelay/main.go` + `internal/api` + `internal/store`). |
| 2.2 | SPA = Desktop-Oberfläche; keine Tauri-Shell | ✅ 1:1 übernehmen | `web/` ist eingebettete SPA; HA-Ingress ersetzt Tauri-Shell (`ARCHITECTURE.md:§3`). |
| 2.3 | Alles ist ein Event im Log (Nostr-NIP-01) | ⚠️ anpassen | Für unsere SQLite-Struktur nutzen wir relationale Tabellen (`messages`, `reactions`, `tasks`) statt eines einheitlichen Event-Logs. Die Korrelations-Metapher (Thread-ID, `roomId`, `agentId`) übernehmen wir. |
| 2.4 | Eigene `kind`-Nummern pro Feature | ❌ bewusst NICHT | Nostr-kryptographisches Event-Modell entfällt; wir haben typisierte Tabellen. |
| 2.5 | Agenten als Raum-Mitglieder mit eigener Identität | ✅ 1:1 übernehmen | `room_members` + `agents` Tabelle. Agenten müssen explizit zu einem Raum hinzugefügt werden, bevor sie antworten. |
| 2.6 | `@mention`-Routing | ⚠️ anpassen | `internal/runtime/runtime.go` matcht Mentions; sequenzielle Agent-Aktivierung mit 900 ms Versatz. Prinzip bleibt, Implementierung Go-SQLite. |
| 2.7 | Nostr-Signaturen (`id`, `pubkey`, `sig`) | ❌ bewusst NICHT | HA-Ingress und LAN-Vertrauensmodell ersetzt kryptographische Identitäten. Keine Signatur-Validierung nötig. |
| 2.8 | Postgres + Redis + MinIO Stack | ❌ bewusst NICHT | Überdimensioniert für HA-Add-on; ersetzt durch SQLite + Go-Hub + ggf. `/share`-Dateisystem. |
| 2.9 | Multi-Node via Redis-Pub/Sub | ❌ bewusst NICHT | Single-Node-v1; HAOS + Go-HTTP/WS-Hub genügt. |
| 2.10 | buzz-acp ACP-Bridge für Coding-Agents | ❓ ungeklärt | Später relevant für Hermes-Adapter; Original-Repo nicht einsehbar. |
| 2.11 | NIP-AE Agent Memory | ❌ bewusst NICHT | Agent Relay nutzt OpenClaw-Sessions oder zukünftig SQLite-Projections für Kontext; kein separates Memory-Protokoll v1. |
| 2.12 | YAML-Workflows für Automation | ❌ bewusst NICHT | Nicht v1; HA hat eigene Automatisierungen. |
| 2.13 | E2E-Direct-Messages (kryptographisch) | ⚠️ anpassen | Unser `kind='dm'` ist konzeptionell 1:1, aber nicht kryptographisch E2E. Für LAN/HA-Ingress ausreichend. |
| 2.14 | Channel-Visibility (`open` / `closed`) | ⚠️ anpassen | Wir können `rooms` später um Visibility erweitern; v1 reicht `human_member=1` + `room_members`. |
| 2.15 | Reaction-Events (`kind=reaction`) | ✅ 1:1 übernehmen | Tabelle `reactions` existiert bereits mit UNIQUE(`message_id`, `actor`, `emoji`). |

---

## 3. Paperclip (paperclipai/paperclip)

| # | Erkenntnis | Transfer | Begründung / Bezug zu Entity-Modell |
|---|------------|----------|-------------------------------------|
| 3.1 | Adapter-Vertrag connect + execute + stream | ⚠️ anpassen | Unser Go-Interface `Adapter` ist schlanker (`Stream` liefert `<-chan Chunk`). Für Phase 2 trennen wir optional in `Connect`/`Execute`/`Stream` auf, aber v1 bleibt einfach. |
| 3.2 | `openclaw-gateway` Adapter spricht natives Gateway-WS | ✅ 1:1 übernehmen | `internal/adapter/openclaw.go` ist direkt von dieser Referenz inspiriert; Pfad bleibt. |
| 3.3 | Session-Strategien `issue | fixed | run` | ⚠️ anpassen | Wir verwenden fixierte Raum-Sessions (`agent:<agentId>:relay:<roomId>`). Projekte können später eigene Sessions bekommen; v1 Raum = Session. |
| 3.4 | `prefixSessionKeyForAgent` | ✅ 1:1 übernehmen | Implementiert als `sessionKeyFor(agentID, roomID)` in `internal/adapter/openclaw.go`. |
| 3.5 | `idempotencyKey = runId` | ⚠️ anpassen | Bei uns `idempotencyKey = messageId` (Nachricht = kleinste Run-Einheit). Begründung: kein separates Task/Run-Modell. |
| 3.6 | Control-Plane-Domäne Orgs/Tasks/Runs/Budgets/Approvals/Audit | ⚠️ anpassen | `tasks` und `projects` übernehmen wir als Board-Ziele. Budgets, Approvals, Audit, Orgs bewusst nicht v1. |
| 3.7 | Status-Badges aus einem Token-Set | ✅ 1:1 übernehmen | UI-Prinzip; `status` (`streaming`, `complete`, `error`) wird in der SPA konsistent gerendert. |
| 3.8 | Maschinenwerte in Monospace | ✅ 1:1 übernehmen | Run-IDs, Message-IDs, Zeitstempel in Monospace darstellen. |
| 3.9 | Chat-Direktmodell (Nutzer rechts, Agents links) | ✅ 1:1 übernehmen | SPA-Layout folgt diesem Muster. |
| 3.10 | Adapter-spezifische UI-Ausnahmebehandlung | ❌ bewusst NICHT | v1 nutzt generische Fehleranzeige (`status='error'`, `content` als Fehlertext). |
| 3.11 | SQLite/Postgres-Wahl | ✅ 1:1 übernehmen | Paperclip unterstützt SQLite im Wrapper (`config.yaml`); bestätigt unsere Wahl. |
| 3.12 | `deployment.mode` / `deployment.exposure` | ⚠️ anpassen | HA-Ingress erledigt Exposure; Add-on-Optionen steuern LAN/HAOS-Integration. |
| 3.13 | OpenClaw-Integration via Umgebungsvariablen `OPENCLEW_URL`/`OPENCLEW_API_KEY` | ⚠️ anpassen | Agent Relay speichert Gateway-URL/Token pro Agent in `agents.base_url`/`agents.api_key`, nicht global. Begründung: mehrere Gateways/Adapters möglich. |
| 3.14 | Hermes / Claude / Cursor / etc. Adapter | ❓ ungeklärt | Pakete existieren in Paperclip, aber keine primäre Quelle verfügbar. |

---

## 4. Cross-Cutting-Entscheidungen für Agent Relay

### 4.1 State-Management

| System | State-Modell | Agent Relay |
|--------|--------------|-------------|
| OpenClaw | Gateway = SQLite-Wahrheit für Sessions; Client = Projektion | Gateway-Wahrheit nur für externe Agent-Runs; Relay = SQLite-Wahrheit für UI |
| buzz | Relay = SQLite/Postgres-Wahrheit; Client = Projektion + Live-Events | ✅ Übernommen |
| Paperclip | Control-Plane-SQL-Wahrheit; Adapter zustandslos | ✅ Übernommen |

### 4.2 Auth

| System | Auth-Modell | Agent Relay |
|--------|-------------|-------------|
| OpenClaw | Token / Device / Trusted-Proxy | Token pro Agent-Konfiguration |
| buzz | Nostr-Pubkey + Relay-Mitgliedschaft | ❌ Entfällt; HA-Ingress |
| Paperclip | Deployment-Mode + OpenClaw-API-Key | Teils entfällt; Token pro Agent |

### 4.3 Skalierung

| System | Skalierung | Agent Relay |
|--------|------------|-------------|
| OpenClaw | Single-Node + optional Cloud-Workers | Single-Node HA-Add-on |
| buzz | Multi-Node via Redis/Postgres | Single-Node; keine Redis/Postgres |
| Paperclip | SQLite oder Postgres | SQLite |

---

## 5. Zusammenfassung der Top-5-Übernahmen

1. **OpenClaw native Gateway-WS-Protokoll** (`connect.challenge → connect → hello-ok → agent → agent.wait`) mit Token-Auth und Raum-SessionKeys. Bewährt, dokumentiert, bereits in Go implementiert.
2. **buzz-Prinzip „Relay-first + SPA als Desktop-Ersatz"**. Agent Relay ist ein einzelnes Go-Binary mit eingebetteter Web-UI; HA-Ingress übernimmt die Shell.
3. **buzz-Prinzip „Agents sind erstklassige Raum-Mitglieder"**. Tabelle `room_members` und explizite Mention-Aktivierung statt globaler Agent-Rollen.
4. **Paperclip-Adapter-Vertrag (reduziert)**. `Adapter` Interface in Go mit `Validate` + `Stream`; `openclaw`-Adapter spricht natives Gateway-WS.
5. **Paperclip-UI-Prinzipien**. Information-Dichte, ein Begriff pro Konzept, Status-Badges, maschinenlesbare Werte in Monospace; Chat-Direktmodell (Nutzer rechts, Agents links).

---

## 6. Bewusst zurückgewiesene / spätere Erweiterungen

- Device-Pairing, Nostr-Signaturen, kryptographische E2E-DMs: Auth-Overhead ohne Mehrwert im LAN/HA-Ingress-Kontext.
- Postgres/Redis/MinIO: Ressourcen-Fußabdruck zu hoch für ein leichtes HA-Add-on.
- Multi-Node / Redis-PubSub: Single-Node-v1.
- Paperclip-Budgets, Approvals, Orgs: Control-Plane-Scope überdimensioniert für v1.
- OpenClaw Config-RPCs: Agent Relay verwaltet eigene Config; Gateway-Config bleibt extern.
- buzz YAML-Workflows, NIP-AE Memory: HA hat eigene Automatisierungen; Memory wird durch externe Agent-Sessions abgedeckt.

---

## 7. Offene Lücken

- `runId`/`seq`-Event-Shape in OpenClaw (Akzeptanz-Frame + Zwischen-Event-Typen) nicht vollständig statisch auflösbar.
- buzz- und Paperclip-Original-Repos nicht klonbar; spezifische TypeScript/Rust-Implementierungen unverifiziert.
- Hermes-/ACP-Adapter für Coding-Agents: Details unbekannt, v2-Relevanz.
- Ingress-Integration von OpenClaw Control UI: ungeklärt, ob Path-Rewriting nötig.
- Attachment-Upload-Stream (`chat.send` mit Attachments): nicht v1, Shape offen.
