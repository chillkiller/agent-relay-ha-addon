# Backend-Reverse-Engineering: OpenClaw Gateway

**Datum:** 2026-10-20  
**Lizenz der untersuchten Software:** MIT (`/usr/lib/node_modules/openclaw/package.json`)  
**Quellen:** statische Analyse des OpenClaw-Gateway-Bundles unter `/usr/lib/node_modules/openclaw` (dist-Bundles + lokaler docs-Mirror). Kein Live-Zugriff auf 127.0.0.1:18790 erforderlich / durchgeführt.  
**Bezug zu Agent Relay:** Agent Relay soll die OpenClaw-Gateway-Laufzeit als Adapter (native WS statt nur OpenAI-Compat) ansprechen. Dieses Dokument dokumentiert den Serverteil, den wir konsumieren.

---

## 1. Überblick

Das OpenClaw-Gateway ist ein Node.js-Server, der WebSocket- und HTTP-Traffic auf demselben Port multiplext (Standard 18789, vgl. `docs/gateway/index.md:237`). Es ist kein reiner Chat-Proxy, sondern ein Zustands-server für Sessions, Agent-Runs, Config-Hot-Reload, Auth/Pairing und Control-UI-Auslieferung. Für Agent Relay ist die Gateway-Rolle rein extern: unser Binary verbindet sich als Client über das Gateway-Protokoll.

Wichtige Eigenschaften für unser Design:

- Eigener SQLite-Store pro Agent für Session-Transkripte (`reference/session-management-compaction/schema.md`).
- `sessionKey` als Isolations-Namespace - wir können damit pro Raum eine eigene Session führen.
- Agent-Runs sind asynchron und adressierbar über `runId` + sequenzielle `seq`-Events; Terminal-Ergebnis liefert `agent.wait`.
- Auth kann Token, Passwort, Device-Pairing oder Trusted-Proxy sein; v1 von Agent Relay nutzt Token-only.
- OpenAI-Compat ist vorhanden (`/v1/chat/completions`, `/v1/responses`), aber das native Protokoll gibt uns Run-IDs und korrekte Session-Bindung.

---

## 2. Protokoll-Handshake

Quellen:

- `docs/gateway/protocol/handshake.md`
- `docs/gateway/protocol/auth.md`
- `dist/agent-BeqU0ris.mjs` (Handler-Implementierung `agent` / `agent.wait`)

### 2.1 Ablauf

1. **Server sendet Challenge** (sofort nach WS-Verbindung):
   ```json
   {
     "type": "event",
     "event": "connect.challenge",
     "payload": { "nonce": "…", "ts": 1737264000000 }
   }
   ```
   `ts` ist der Anker für Device-Auth-Signaturen. Für Token-Clients ist er nicht kritisch, muss aber gültig sein (`ts` non-negative integer, `handshake.md:14`).

2. **Client antwortet mit `connect` (type: req):**
   ```json
   {
     "type": "req",
     "id": "…",
     "method": "connect",
     "params": {
       "minProtocol": 4,
       "maxProtocol": 4,
       "client": { "id": "…", "version": "…", "platform": "…", "mode": "operator" },
       "role": "operator",
       "scopes": ["operator.read", "operator.write"],
       "auth": { "token": "…" },
       "locale": "en-US",
       "userAgent": "agentrelay/0.1.0"
     }
   }
   ```
   `role` bestimmt die effektiven Rechte. Für Agent Relay ist `operator` passend, weil wir Agent-Runs auf dem Gateway starten müssen. `scopes` kann bei Token-Auth weggelassen / verhandelt werden; Gateway gibt in `hello-ok.auth.scopes` die effektiven Scopes zurück (`auth.md:23`).

3. **Gateway antwortet `hello-ok` (type: res):**
   ```json
   {
     "type": "res",
     "id": "…",
     "ok": true,
     "payload": {
       "type": "hello-ok",
       "protocol": 4,
       "server": { "version": "…", "connId": "…" },
       "features": { "methods": ["…"], "events": ["…"] },
       "snapshot": { "…": "…" },
       "auth": { "role": "operator", "scopes": ["…"] },
       "policy": {
         "maxPayload": 26214400,
         "maxBufferedBytes": 52428800,
         "tickIntervalMs": 15000,
         "attachments": { "maxBytes": 20971520, "maxImageBytes": 6291456 }
       }
     }
   }
   ```
   `server.version` hilft uns bei Diagnose. `features.methods`/`features.events` zeigen, welche RPCs auf diesem Gateway verfügbar sind. `policy` ist bindend: `maxPayload` 25 MiB, Attachments 20 MiB / Bilder 6 MiB decodiert (`handshake.md:60`).

### 2.2 Auth-Modi

Quelle: `docs/gateway/protocol/auth.md`

| Modus | Wire-Feld | Bemerkung |
|-------|-----------|-----------|
| `token` | `connect.params.auth.token` | Standard für Agent Relay v1. |
| `password` | `connect.params.auth.password` | Alternative Shared-Secret. |
| `trusted-proxy` | Header-basiert | Nur bei Reverse-Proxy-Setup. |
| `none` | kein Auth | Nur Loopback/vertrauenswürdig; nicht für HA-Ingress. |
| Device-Pairing | `device.{id,publicKey,signature,signedAt,nonce}` | Liefert `hello-ok.auth.deviceToken`. Agent Relay v1 bewusst nicht implementiert (kein Geräte-Setup im Add-on). |

Wichtig: wenn beide `auth.token` und `auth.password` gesendet werden, wählt das Gateway je nach `gateway.auth.mode` (`auth.md:19`). Ein explicit `deviceToken` in `auth.token` überschreibt Shared-Secret.

---

## 3. WebSocket-Methoden und -Events

Quellen:

- `docs/gateway/protocol/rpc-methods.md`
- `docs/gateway/protocol/rpc-talk-config-and-agents.md`
- `docs/gateway/protocol/rpc-session-control.md`
- `docs/gateway/protocol/operator-methods.md`
- `dist/agent-BeqU0ris.mjs` (`agentRunHandler`, `agentWaitHandler`)

### 3.1 Für Agent Relay relevante RPCs

| Methode | Scope | Zweck |
|---------|-------|-------|
| `connect` | auth | Handshake (siehe §2). |
| `agent` | `operator.write` | Startet einen Agent-Run. Parameter: `message`, `sessionKey`, `idempotencyKey`, `agentId`, `timeout`, optional `attachments`, `tools`, `model`, etc. |
| `agent.wait` | `operator.read` / run visibility | Liefert Terminal-Snapshot eines laufenden/abgeschlossenen Runs. |
| `chat.send` | `operator.write` | Nachricht an Session senden (Control-UI-Pfad). |
| `sessions.create` / `sessions.send` | `operator.write` | Session-Verwaltung. |
| `config.get` / `config.patch` / `config.apply` | `operator.admin` / `operator.write` | Config-Hot-Reload (siehe §7). |
| `health` | unterschiedlich | Laufzeit-Status / RPC-Probe. |
| `system.presence` | `operator.read` | Anwesenheit/Heartbeat-Information. |

### 3.2 Agent-Run: Request → Events → Terminal

Quellen:

- `docs/gateway/protocol/rpc-talk-config-and-agents.md`
- `dist/agent-BeqU0ris.mjs:61-122`

**Request `agent` (req):**
```json
{
  "type": "req",
  "id": "relay-…",
  "method": "agent",
  "params": {
    "message": "…",
    "sessionKey": "agent:coordinator:relay:room-…",
    "idempotencyKey": "msg-…",
    "agentId": "coordinator",
    "timeout": 480000
  }
}
```

- `sessionKey` muss zum Zielagenten passen. Gateway prüft Policy: „agent 'coordinator' does not match session key agent '…'" (`internal/adapter/openclaw.go:131`). Daher leiten wir `sessionKey` aus dem Ziel-Agenten ab: `agent:<agentId>:relay:<roomId>`.
- `idempotencyKey` macht Retries gefahrlos. Agent Relay setzt sie auf die `messageId` (`ARCHITECTURE.md` §5; `internal/adapter/openclaw.go:115` nutzt `runID = firstNonEmpty(a.IdempotencyKey, …)`).
- `timeout` in Millisekunden. OpenClaw-internes Wait-Fenster liegt typisch bei 8 Minuten (480000 ms), danach liefert `agent.wait` den Snapshot.

**Antwort/Akzeptanz:**
Gateway sendet zunächst eine Acceptance-Antwort (`io.emitAcceptance`), danach asynchrone `event agent`-Frames während der Laufzeit.

**Event-Frames während des Runs** (Belege in `rpc-talk-config-and-agents.md` und `dist/agent-activity-events-CYqKNEY.mjs`):

- `agent`-Event mit `runId` und `seq` pro Delta.
- Inhaltstypen: `text`, `tool_call`, `tool_result`, `thinking`, `error`, `status`, `usage`.
- `seq` ist monoton pro `runId`; auf Gap → kompletter Reload (`docs/gateway/index.md:400`: „Events are not replayed. On sequence gaps, refresh state").
- `runId` identifiziert den Run für `agent.wait` und `chat.abort`.

**Terminal-Snapshot via `agent.wait`:**
```json
{
  "type": "req",
  "id": "…",
  "method": "agent.wait",
  "params": { "runId": "…" }
}
```

Implementierung in `dist/agent-BeqU0ris.mjs:76` (`agentWaitHandler`):
- prüft Run-Visibility (`authorizeWait`)
- wartet auf Fertigstellung (`prepared.wait()`)
- liefert `{ result }`

Agent Relay v1 nutzt diesen Pfad: `runAgentOnce` schickt `agent`, wartet dann auf `agent.wait`, liefert den gesamten Text als einen Chunk zurück (`internal/adapter/openclaw.go`). Phase 2 wird echtes Streaming der `agent`-Events implementieren.

---

## 4. HTTP-Surface

Quellen:

- `dist/gateway-http-route-contracts-jBtizxKU.mjs`
- `dist/audit.nondeep.runtime-CX4OlTB5.mjs:460`
- `docs/gateway/index.md`
- `docs/web/urls.md`

### 4.1 Endpunkte

| Pfad | Zweck | Bemerkung |
|------|-------|-----------|
| `/health` | Liveness | `gateway-http-route-contracts-jBtizxKU.mjs:3` |
| `/healthz` | Liveness (Alias) | `gateway-http-route-contracts-jBtizxKU.mjs:4` |
| `/v1/chat/completions` | OpenAI-Compat Chat | optional, abhängig von `chatCompletionsEnabled` (`audit.nondeep.runtime-CX4OlTB5.mjs:483`) |
| `/v1/responses` | OpenAI-Compat Responses-API | optional (`audit.nondeep.runtime-CX4OlTB5.mjs:484`) |
| `/api/v1/admin/rpc` | Admin-HTTP-RPC | optional, admin-scoped (`audit.nondeep.runtime-CX4OlTB5.mjs:485`) |
| `/` / `/chat?session=…` | Control UI (SPA) | `docs/web/urls.md:157`, `docs/web/urls.md:483` |
| `/api/v1/…` | ClawHub / Plugin-Marketplace | nicht für Relay relevant |

### 4.2 Multiplex-Port

WS und HTTP teilen sich denselben Port (`docs/gateway/index.md`). Das erklärt, warum `gatewayWSURL` in `internal/adapter/openclaw.go:62` einen reinen `ws://` auf denselben Host:Port abbildet, ohne `/v1`-Suffix.

### 4.3 OpenAI-Compat-Surface

Quelle: `docs/providers/openai/runtimes.md` (implizit), `dist/audit.nondeep.runtime-CX4OlTB5.mjs`.

- `/v1/chat/completions` nimmt OpenAI-Schema entgegen.
- Modell-Name ist intern ein Agent-Target (`openclaw/<agentId>`) - bei uns wird der `model`-Wert der Agent-Konfiguration zum `agentId` im nativen Adapter (`internal/adapter/openclaw.go:99-104`).
- Nachteil gegenüber nativem Protokoll: fehlende `runId`/`seq`, keine explizite `sessionKey`-Kontrolle, kein `agent.wait`-Snapshot. Daher priorisiert Agent Relay den nativen Adapter.

---

## 5. Auth / Device-Flow / Scopes

Quelle: `docs/gateway/protocol/auth.md`.

### 5.1 Token-Flow

1. Client sendet `connect.params.auth.token` mit konfiguriertem Secret.
2. Gateway antwortet `hello-ok.auth.scopes` mit effektiven Scopes.
3. Client speichert ggf. `deviceToken` für Reconnects.

### 5.2 Device-Flow

- Client generiert Keypair, signiert Challenge (`ts` + `nonce`) mit privatem Key.
- `connect.params.device` enthält `id`, `publicKey`, `signature`, `signedAt`, `nonce`.
- Gateway prüft Signatur und gibt `hello-ok.auth.deviceToken` aus.
- Token ist auf Rolle + Gerät + approved grant gebunden.
- Rotation/Revokation über `device.token.rotate` / `device.token.revoke` (benötigt `operator.pairing` bzw. `operator.admin`).

### 5.3 Für Agent Relay relevante Scopes

- `operator.read` - Status, Sessions, `agent.wait`.
- `operator.write` - Agent-Runs starten.
- `operator.admin` - Config schreiben (optional, nicht v1).
- `operator.talk.secrets` - Nur für native Talk-Konfiguration.

Empfehlung: Agent Relay erwartet für den OpenClaw-Adapter einen Token mit `operator.read` + `operator.write`. Admin-Scopes werden nicht benötigt.

---

## 6. sessionKey-Semantik

Quellen:

- `docs/reference/session-management-compaction/schema.md`
- `docs/concepts/session.md`
- `dist/session-store-B3kIJ9zf.mjs`

### 6.1 SessionKey-Muster

| Muster | Beispiel | Bedeutung |
|--------|----------|-----------|
| Main/Direct | `agent:<agentId>:main` | Standard-Chat pro Agent |
| Group | `agent:<agentId>:<channel>:group:<id>` | Gruppen-/Projekt-Chat |
| Room/Channel | `agent:<agentId>:<channel>:channel:<id>` oder `…:room:<id>` | Raum-gebunden |
| Cron | `cron:<job.id>` | Automation |
| Webhook | `hook:<uuid>` | Eingehende Hooks |

### 6.2 sessionId-Lifecycle

- Jeder `sessionKey` zeigt auf eine aktuelle `sessionId` (SQLite-Transkript-Identität).
- Gateway reset (`/new`, `/reset`) behält `sessionId` bei, fügt Reset-Boundary ein.
- Täglicher / Idle-Reset erzeugt neue `sessionId` je nach Konfiguration (`session.reset.mode`).
- Für Agent Relay bedeutet das: wir verwenden `agent:<agentId>:relay:<roomId>` als stabiles Isolationskonstrukt; der Gateway führt den Verlauf selbst, wir archivieren nur unsere UI-Nachrichten in SQLite.

### 6.3 Agent-Matching-Policy

Das Gateway erzwingt, dass der im `sessionKey` kodierte Agent mit dem tatsächlich aufgerufenen `agentId` übereinstimmt. Daher darf Agent Relay `agentId` nicht leer lassen, wenn ein `sessionKey` mit Agent-Präfix gesendet wird. Unsere `sessionKeyFor`-Funktion muss konsistent mit dem Ziel-Agenten sein.

---

## 7. Config / Hot-Reload

Quellen:

- `docs/gateway/configuration/config-rpc.md`
- `docs/gateway/configuration/hot-reload.md`

### 7.1 Config-RPCs

- `config.get` - liefert aktive Config + Hash.
- `config.patch` - partieller Patch mit `baseHash` (optimistische Konflikterkennung).
- `config.apply` - komplette Config-Ersetzung.
- `config.set` - persistiert, ohne anzuwenden.

### 7.2 Hot-Reload-Verhalten

- Config-Datei wird von File-Watcher überwacht; RPCs können dieselbe Write erkennen.
- `config.patch` wartet, bis die aktive Gateway-Instanz die exakte Write applied hat.
- Channel-/Plugin-Reloads werden ggf. deferred, wenn aktive Runs laufen.
- Control-plane writes sind rate-limited: 30 Requests/60s pro Methode pro `deviceId+clientIp` (`config-rpc.md:42`).

### 7.3 Relevanz für Agent Relay

- Agent Relay selbst hat kein Config-Hot-Reload (Go-Binary + SQLite + eingebettete SPA).
- Wir können aber die Gateway-Config über `config.get` abfragen, um z. B. verfügbare Agents zu enumerieren (sofern Scope vorhanden). Für v1 nicht implementiert.

---

## 8. Agent-Run Streaming: runId + seq

Quellen:

- `docs/gateway/protocol/rpc-talk-config-and-agents.md`
- `docs/concepts/queue.md`
- `dist/agent-BeqU0ris.mjs`
- `dist/agent-activity-events-CYq4KNEY.mjs`

### 8.1 Run-Identität

- Ein `agent`-Run wird durch `runId` identifiziert.
- `idempotencyKey` beim `agent`-Request verhindert doppelte Runs bei Retry.
- `runId` ist vermutlich (ungeklärt: exakte Generierung nicht im untersuchten Bundle sichtbar) eine serverseitige UUID, die im Acceptance-Frame zurückgegeben wird.

### 8.2 Event-Sequenz

- `agent`-Events enthalten `runId` + `seq`.
- `seq` ist lückenlos pro Run; bei Gap muss der Client den kompletten Status neu laden.
- Terminal-Status wird per `agent.wait` abgefragt.

### 8.3 Abort

- `chat.abort { runId }` bricht den spezifischen Run ab (`docs/concepts/queue.md:135`).
- Ohne `runId` bricht `chat.abort` authorisierte queued Turns ab.

### 8.4 Übertragung auf Agent Relay

- Phase 1: `openclaw`-Adapter sendet `agent`, wartet `agent.wait`, liefert finalen Text (wie aktuell in `internal/adapter/openclaw.go`).
- Phase 2: native Event-Streams über denselben WS: Handler subscribed `agent`-Events mit `runId` und emittiert Zwischen-Token / Tool-Events an unsere API-Hub, die sie per WS an die SPA verteilt.

---

## 9. SQLite-Store-Layout (soweit aus Bundle/Docs erkennbar)

Quellen:

- `docs/reference/session-management-compaction/schema.md`
- `docs/reference/session-management-compaction/store.md`
- `docs/reference/session-management-compaction/maintenance.md`
- `dist/session-store-B3kIJ9zf.mjs`
- `dist/session-accessor.sqlite-entry-Dn_MsDKD.mjs`
- `dist/session-accessor.sqlite-transcript-write-tYk0mJFG.mjs`
- `node-sqlite.mjs` (CLI-Hilfsbundle)

### 9.1 Architektur

- Pro **Agent** (genauer: pro Agent-Store-Pfad) existiert eine SQLite-DB.
- Speicherort: konfigurierbar, typisch unter `~/.openclaw/agents/<agentId>/sessions.sqlite` (ungeklärt: exakter Pfad aus Bundle nicht direkt ersichtlich, aber `storePath` in `session-store.mjs` deutet auf Agenten-Verzeichnis).
- WAL-Mode wird genutzt (`maintenance.md` erwähnt WAL-Kompaktierung).

### 9.2 SessionEntry (laufende Metadaten)

Tabelle/JSON-Store: `SessionEntry` (`src/config/sessions.ts`, erwähnt in `schema.md`).

Wichtige Felder (dokumentiert in `schema.md`):

- `sessionId`
- `sessionStartedAt`
- `lastInteractionAt`
- `lastActivityAt`
- `model`, `modelProvider`
- `agentHarnessId`
- `contextTokens`, `contextTokensSource`, `contextBudgetStatus`
- `inputTokens`, `outputTokens`, `cacheRead`, `cacheWrite`
- `estimatedCostUsd`
- `totalTokens`, `totalTokensFresh`, `totalTokensVersion`
- `abortedLastRun`
- `systemPromptReport`

### 9.3 Transkript-Rows

- Jede Session hat ein Transkript in SQLite.
- Einträge enthalten `runId`, `sequence`, `kind`, `actorType`, `timestamp`, Inhalt.
- Reset-Boundaries werden als Einträge markiert.
- Kompaktierung (`compaction`) entfernt alte Einträge oberhalb eines Retention-Fensters, behält aber Head/State-Version.

### 9.4 Maintenance

- Periodische Checkpointing.
- Pruning-Watermark pro Session; `historyGap: true` signalisiert, dass eine angefragte Version außerhalb des Retained-History liegt (`docs/concepts/session-state.md:111`).
- Für Agent Relay irrelevant: wir speichern eigenes einfaches `messages`-Schema; Gateway-Session-Store bleibt extern.

---

## 10. Offene / ungeklärte Punkte

1. **Exakte `runId`-Generierung und ob sie identisch mit `idempotencyKey` ist.** `internal/adapter/openclaw.go` setzt `idempotencyKey` auf eine eigene ID; Acceptance-Frame könnte eine separate `runId` zurückgeben. Ungeklärt: shape des Acceptance-Frames.
2. **Vollständige Event-Liste für `agent`-Events.** Sichtbare Shapes in `agent-activity-events` deuten auf `text`, `tool_call`, `tool_result`, `thinking`, `error`, `usage`; vollständiges Schema liegt nicht offen im untersuchten Bundle.
3. **Exakte SQLite-Schema-DDL.** Bundle zeigt nur JavaScript-Metadaten-Objekte; CREATE TABLE Statements sind nicht direkt lesbar. Layout ist aus `schema.md`/`store.md` abgeleitet.
4. **HTTP-Admin-RPC-Methodenliste.** `audit.nondeep.runtime-CX4OlTB5.mjs:485` zeigt nur den Endpunkt, nicht die zulässigen Methoden.
5. **Control-UI-Weiterleitung / Ingress-Integration.** Gateway liefert `controlUiUrl`; wie sich diese URL im HA-Ingress-Kontext verhält, ist ungeklärt.

---

## 11. Lizenz-Hinweis

OpenClaw steht unter der MIT-Lizenz (`/usr/lib/node_modules/openclaw/package.json`). Dieses Dokument enthält keine Code-Kopien, sondern protokolliert beobachtete Schnittstellen und Datenflüsse.
