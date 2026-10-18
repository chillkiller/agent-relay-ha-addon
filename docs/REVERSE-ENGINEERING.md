# Reverse-Engineering-Notizen: buzz & Paperclip (2026-10-18)

Zweck: Erkenntnisse aus den Original-Repos (shallow clone, nur Analyse) als
Bausteine für Agent Relay. Wir kopieren keinen Code (Lizenzen: buzz Apache-2.0,
Paperclip MIT — beide kompatibel, aber wir bauen eigenständig nach).

## buzz (github.com/block/buzz) — Rust-Monorepo

**Aufbau (aus ARCHITECTURE.md, Cargo.toml, package.json):**

- **Relay ist Single Source of Truth.** Alle Clients (Menschen + Agents) verbinden
  sich per WebSocket EINEM Relay; auth, Signatur-Check, Persistenz, Fan-out,
  Indexierung und Automatisierung passieren im Relay. Kein P2P.
- **Alles ist ein Event** in einem Log (Nostr NIP-01; eigene `kind`-Nummern pro
  Feature). Neue Feature = neue Kind-Nummer, alte Clients brechen nicht.
  Konsequenz für uns: Nachrichten/Reaktionen/Workflow-Schritte sind Events mit
  gemeinsamem Umschlag (Typ, Zeit, Autor, Korrelations-ID) statt getrennter Tabellen.
- **Desktop ist eine Tauri-Shell um dieselbe Web-App** (`desktop/` = Tauri + die
  React-App aus `web/`). Der Server (`buzz-relay`, Axum) bedient beide.
  → Für uns: HA-Ingress + Browser übernehmen die Shell-Rolle; die SPA ist die
  „Desktop-App-Oberfläche" (wie Control UI bei OpenClaw).
- **Agents = eigene Nostr-Keypairs und eigene Kanalmitgliedschaften**, gleiche
  Oberfläche wie Menschen, eigener Audit-Trail. Agents kommen über `buzz-acp`
  (ACP-Bridge für Coding-Agents) oder CLI-Adapter.
- Multi-Node via Redis-Pub/Sub, Suche via Postgres-FTS — beides für uns
  überdimensioniert (HA-Add-on: ein Node, SQLite).

**Für Agent Relay übernommen:**
1. Relay-first: ein Binärfile, WS + HTTP, alle State in SQLite, Events an Clients.
2. SPA = Desktop-Oberfläche; kein separater Desktop-Binär needed (HA-Shell).
3. Join als „erste Klasse": Room-Membership für Agents statt globaler Rolle.

## Paperclip (github.com/paperclipai/paperclip) — Node/pnpm-Monorepo

**Aufbau (aus DESIGN.md, server/, packages/adapters, ui/):**

- **Produktrolle: operational control plane** für Agent-Organisationen (orgs,
  tasks, runs, budgets, approvals, audit). UI-Prinzipien aus DESIGN.md:
  „what is happening → does it need me → what do I do about it"; Dichte durch
  Information, nicht Deko; ein Begriff pro Konzept (immer „task" etc.);
  status-badges systemübergreifend aus einem Tokenset; Maschinenwerte monospace.
- **Adapters-Architektur:** jeder Agent-Runtime-Typ ist ein eigenes Adapter-Paket
  (`packages/adapters/openclaw-gateway`, `hermes`, `hermes-gateway`,
  `claude-local`, `codex-local`, `cursor-cloud`, `gemini-local`, …). Ein
  Adapter-Paket-Kontrakt: connect + execute + stream + UI-Ausnahmebehandlung.
  → Genau unser `internal/adapter`-Vertrag.
- **Chat-Direktmodell:** Task-Chat mit Agenten; Kommentare des Nutzers rechts,
  Agents links mit Name/Avatar; Execution-Hinweise werden kontextabhängig
  ein-/ausgeblendet; Session-Reset-Grenzen bleiben sichtbar.
- **OpenClaw-Gateway-Adapter (unsere stärkste Referenz, README + execute.ts):**
  - Transport: **natives Gateway-WS-Protokoll** (`ws://`/`wss://`)
  - Ablauf: `connect.challenge`-Event (Nonce) → `req connect` (protocol, client,
    role, scopes, auth token, optional device) → `hello-ok` → `req agent`
    (message, sessionKey, idempotencyKey=runId, agentId, timeout) →
    `event agent`-Frames (runId, seq) lesen → `req agent.wait` (Terminal-Snapshot)
  - Session-Strategien: `issue|fixed|run`; Präfixmuster
    `agent:<agentId>:<sessionKey>` (execute.ts `prefixSessionKeyForAgent`)
  - Auth: Bearer-Token aus Konfiguration; Device-Pairing optional
    (Paperclip default an, wir v1 ohne)

**Für Agent Relay übernommen:**
1. Adapter-Vertrag wie Paperclip, aber schlank (Stream + Validate).
2. `openclaw`-Adapter spricht natives Gateway-WS (nicht nur OpenAI-Compat).
3. Session-Bindung pro Raum: `sessionKey = agent:<agentId>:relay:<roomId>`
   → OpenClaw merkt sich Verlauf pro Raum selbst; wir senden die Trigger-nachricht.
4. UI-Ton: Control-Plane-Dichte (Paperclip-Prinzipien) + buzz-Room-Metapher.

## OpenClaw-Gateway (eigene Doku, docs/gateway/protocol)

- Handshake v4: `connect.challenge` mit `ts`; connect params
  `minProtocol/maxProtocol`, client (id/version/platform/mode), role `operator`,
  scopes, auth.token; Antwort `hello-ok` (features.methods liste).
- Agent-Run: `agent` (message/sessionKey/idempotencyKey/agentId) +
  `agent.wait` (Terminal-Snapshot); Events pro Run mit `runId`+`seq`.
- OpenAI-Compat (`/v1/chat/completions`) läuft als Alternative, ist aber
  „Agent-first" (model = agent target `openclaw/<agentId>`).
- Standardport 18789 (WS+HTTP multiplex).