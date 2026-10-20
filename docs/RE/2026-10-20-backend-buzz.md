# Backend-Reverse-Engineering: buzz (github.com/block/buzz)

**Datum:** 2026-10-20  
**Lizenz der untersuchten Software:** Apache-2.0  
**Quellen:** HA-Add-on-Wrapper unter `/share/projekte/github/buzz-ha-addon/buzz_relay` plus bereits bekannte Architektur-Dokumentation aus `agent-desktop-ha-app/docs/REVERSE-ENGINEERING.md`. Ein direktes shallow-clone von `https://github.com/block/buzz.git` wurde von der Sandbox abgelehnt (Netzwerk/Approval-Blockade); das Original-Repo konnte daher nicht statisch inspiziert werden.  
**Bezug zu Agent Relay:** buzz ist strukturell das stärkste Vorbild für „Relay-first, SPA als Desktop-Ersatz, Agenten als erstklassige Mitglieder". Dieses Dokument fasst den bekannten Backend-Aufbau zusammen und markiert alles, das nicht aus primärer Quelle stammt, als ungeklärt.

---

## 1. Verifikation des lokalen Repo-Umfangs

```
/share/projekte/github/buzz-ha-addon/buzz_relay/
  Dockerfile
  README.md
  config.yaml
  landing.html
  run.sh
```

Das Verzeichnis enthält ausschließlich einen Home-Assistant-Add-on-Wrapper für das echte `block/buzz`-Repo. Es liegt **kein** buzz-Quellcode vor:

- `README.md` verweist auf `https://github.com/block/buzz`.
- `config.yaml` definiert das Add-on `buzz_relay` mit Ingress-Port 3000, Postgres/Redis/MinIO und experimentellem Status.
- `run.sh` orchestriert den Container-Start; es wurde nicht vollständig gelesen, da es sich um Shell-Orchestrierung handelt.
- `Dockerfile` baut vermutlich das upstream buzz-Image nach; ohne Netzwerkzugriff nicht weiter aufschlüsselbar.

**Lücke:** Primärquellen (Rust-Quellcode, Cargo.toml, ARCHITECTURE.md des Original-Repos) fehlen. Die folgenden Abschnitte basieren auf der in unserem Projekt vorhandenen Sekundärdokumentation und allgemein verfügbaren buzz-Dokumentation (Nostr-NIP-01, Hermes-Integration).

---

## 2. Architektur-Überblick (aus REVERSE-ENGINEERING.md)

buzz ist ein **Relay-first** System: alle Clients (Menschen und Agents) verbinden sich mit *einem* zentralen Relay. Das Relay ist Single Source of Truth für Auth, Signatur-Prüfung, Persistenz, Fan-out, Indexierung und Automatisierung. Es gibt kein P2P.

Zentrale Konzepte für Agent Relay:

- Ein Binärfile (`buzz-relay`, Rust/Axum) bedient WebSocket + HTTP + Web-UI.
- Web-UI läuft auf Port 3000 und wird vom HA-Add-on als Ingress exponiert.
- Desktop-Version ist eine Tauri-Shell um dieselbe Web-App; für HA übernehmen Browser/Ingress diese Rolle.
- Agenten besitzen eigene Nostr-Keypairs und Kanal-Mitgliedschaften, gleiche Oberfläche wie Menschen, eigener Audit-Trail.
- Multi-Node via Redis-Pub/Sub, Volltextsuche via Postgres-FTS.

---

## 3. Nostr-NIP-01-Event-Modell

Quelle: `agent-desktop-ha-app/docs/REVERSE-ENGINEERING.md` + öffentliche Nostr-NIP-01-Spezifikation.

### 3.1 Basis-Event-Umschlag

buzz verwendet das Nostr-NIP-01-Event-Format:

```json
{
  "id": "<32-byte sha256 hex>",
  "pubkey": "<32-byte hex public key des Autors>",
  "created_at": 1234567890,
  "kind": 1,
  "tags": [
    ["e", "<reply-to event id>", "<relay url>"],
    ["p", "<pubkey>", "<relay url>"]
  ],
  "content": "…",
  "sig": "<64-byte schnorr signatur>"
}
```

- `id` ist SHA256 über serialisierte Felder.
- `pubkey` identifiziert den Autor (Mensch oder Agent).
- `kind` ist der Feature-Selektor: buzz definiert eigene `kind`-Nummern für Channels, DMs, Workflows, Reactions, Memory etc.
- `tags` tragen Korrelations-IDs (`e`-Tag für Thread, `p`-Tag für Mention/Empfänger).
- `sig` ist eine Schnorr-Signatur über `id`.

### 3.2 Eigene kinds

| Kind (ungeklärt: konkrete Zahlen aus primärer Quelle nicht verifiziert) | Semantik |
|------------------------------------------------------------------------|----------|
| Text-Note / Channel-Message | Standard-Nachricht in einem Stream |
| Direct-Message | E2E-verschlüsselte 1:1-Kommunikation |
| Channel-Metadata | Raum-Name, Beschreibung, Policies |
| Membership | Beitritt/Austritt/Aktualisierung von Kanalmitgliedern |
| Reaction | Emoji-Reaktion auf Event |
| Workflow / YAML-Automation | Automations-Definition |
| Agent Memory (NIP-AE) | Persistenter agenten-spezifischer Kontext |

**Wichtige Konsequenz für Agent Relay:** Das Konzept „alles ist ein Event im gleichen Umschlag" ist übertragbar, aber wir setzen SQLite statt Postgres/Redis ein und brauchen keine kryptographischen Event-IDs pro Nachricht.

---

## 4. Event-Envelope und WebSocket-Fan-out

### 4.1 Client <-> Relay-Kommunikation

Nostr-Clients senden über WebSocket:

- `EVENT` – veröffentlicht ein signiertes Event.
- `REQ` – abonniert Events nach Filter (kinds, Autoren, Tags, Zeitfenster).
- `CLOSE` – beendet ein Abo.

Das Relay antwortet mit:

- `EVENT` – liefert ein passendes Event.
- `EOSE` – Ende des initialen Backfills.
- `OK` / `NOTICE` – Bestätigung/Fehler.
- `AUTH` – optional für NIP-42-Authentifizierung.

### 4.2 Fan-out

- Das Relay validiert Signatur und Auth.
- Gültige Events werden persistiert und an alle passenden `REQ`-Abonnements verteilt.
- Für Multi-Node-Setups (HA-Add-on laut Wrapper nutzt Postgres + Redis) publiziert das Relay das Event über Redis-Pub/Sub an andere Nodes, die ihrerseits an lokale Subscriber weiterleiten.
- Ungeklärt: exaktes Redis-Topic-Schema, Filter-Index-Schema in Postgres.

### 4.3 Mention-Matching

- `@name`-Mentions werden im Content erkannt und als `p`-Tags kodiert.
- Agents werden über ihre Pubkey-Adresse getaggt.
- Für Agent Relay übernehmen wir ein ähnliches Mention-Matching, aber auf Datenbankebene (siehe `internal/runtime/runtime.go`).

---

## 5. Auth / Signaturen

### 5.1 Nostr-Signaturen

- Jeder Akteur (Mensch, Agent) besitzt ein Nostr-Keypair (32-byte privater Key, 32-byte pubkey).
- Jedes Event wird mit dem privaten Key des Autors signiert.
- Das Relay prüft `id`-Bildung und `sig` vor Persistenz/Fan-out.

### 5.2 Relay-Mitgliedschaft

Quelle: `buzz-ha-addon/buzz_relay/README.md`.

- `require_relay_membership: true` → nur Mitglieds-Pubkeys dürfen posten.
- `relay_owner_pubkey` definiert den Besitzer.
- `allow_nip_oa_auth: true` erlaubt NIP-OA Owner Attestation.
- Agenten werden mit `buzz-admin add-member --pubkey <agent_pubkey>` hinzugefügt.

### 5.3 Channel-Mitgliedschaft

- Kanäle haben Visibility (`open` / `closed`) und explizite Mitgliedslisten.
- Agenten müssen einem Kanal beitreten, bevor sie darin posten/lesen können.
- Für Agent Relay: das Konzept „Room-Mitgliedschaft für Agents" übernehmen wir (Tabelle `room_members`), aber ohne kryptographische Signaturen.

---

## 6. Persistenz (Postgres / Redis / S3)

Quelle: `buzz-ha-addon/buzz_relay/README.md` / `config.yaml`.

### 6.1 Komponenten

| Komponente | Rolle | Für Agent Relay |
|------------|-------|-----------------|
| **Postgres 15** | Primär-Events, Channels, Memberships, Volltextsuche | Ersetzt durch SQLite (single-node HA-Add-on). |
| **Redis 7** | Presence, Pub/Sub, evtl. Rate-Limiting | Nicht benötigt; Go-Hub ersetzt Pub/Sub. |
| **MinIO / S3** | Medien-Attachments | Nicht v1; ggf. später HA `/share` oder Base64-Inline. |

### 6.2 HA-Add-on-Wrapper-Architektur

```
HA Companion App (Ingress)
    └── Buzz Web-UI (:3000)

Buzz Relay Add-on
    ├── buzz-relay  (Rust, Web-UI on :3000)
    ├── Postgres 15
    ├── Redis 7
    └── MinIO
```

Aus `buzz-ha-addon/buzz_relay/README.md`.

### 6.3 Für Agent Relay abgeleitete Entscheidung

buzz zeigt, dass ein Relay-Backend für HAOS mit Ingress funktioniert, aber der Ressourcen-Fußabdruck (Postgres + Redis + MinIO, ~1 GB RAM) für ein leichtes Add-on überdimensioniert ist. Daher Agent Relay: Go + SQLite + embedded SPA, kein separater Datenbank-/Cache-Stack.

---

## 7. Agents-Integration (buzz-acp / Hermes)

Quelle: `buzz-ha-addon/buzz_relay/README.md` + allgemeine Hermes-Dokumentation.

### 7.1 buzz-acp

- `buzz-acp` ist eine ACP-Bridge für Coding-Agents.
- Agenten verbinden sich als eigene Nostr-Identität mit dem Relay.
- Empfangene `@mentions` werden als ACP-Anfragen an die Agent-Laufzeit weitergegeben.

### 7.2 Hermes Buzz Gateway Adapter

- Hermes-Profile verbinden sich als separate Nostr-Keypairs.
- Profil-Pubkey wird als Relay-Mitglied registriert.
- Agent nimmt an Kanälen teil und reagiert auf Mentions.

### 7.3 Für Agent Relay

- Wir übernehmen das Prinzip „Agents sind Raum-Mitglieder mit eigener Identität", aber nicht das Nostr-Keypair-Modell.
- Jeder Agent in Agent Relay ist ein Konfigurationseintrag in `agents` (Name, Emoji, Adapter, Modell/Agent-ID, URL, Token, Prompt) und wird über `room_members` mit Räumen verbunden.

---

## 8. Server / Tauri-Desktop-Bindung

### 8.1 Server (`buzz-relay`)

- Rust/Axum-Server.
- Bedient WebSocket-Nostr-API, HTTP-REST-API und Web-UI.
- Single Binary plus optional Web-UI-Assets.

### 8.2 Desktop (Tauri)

- `desktop/` im Original-Repo soll eine Tauri-Shell um die React-Web-App aus `web/` sein (Quelle: `agent-desktop-ha-app/docs/REVERSE-ENGINEERING.md`).
- Für HA wird die Web-UI direkt via Ingress geladen; Browser übernimmt die Shell-Rolle.

### 8.3 Für Agent Relay

- Agent Relay ist ebenfalls ein einzelnes Go-Binary.
- Web-UI ist eingebettete SPA (`web/`), ausgeliefert über HTTP-Router.
- Keine Tauri-Shell v1; später kann eine Tauri-Shell dieselbe SPA laden.

---

## 9. Offene / ungeklärte Punkte (wegen fehlendem Original-Repo)

1. **Exaktes Rust-Modell / Crate-Layout.** Kein Zugriff auf `Cargo.toml`, `src/` etc. Aus REVERSE-ENGINEERING.md bekannt: Monorepo mit `buzz-relay`, `buzz-db`, `buzz-auth`, `buzz_pubsub`, `tower_http`.
2. **Konkrete `kind`-Nummern und Tag-Schemas.** Nur abgeleitet aus NIP-01 + buzz-Doku, nicht primär verifiziert.
3. **Filter- und Index-Implementierung.** Postgres-Schema für Events, Channels, Search unbekannt.
4. **YAML-Workflow-Engine.** Erwähnt in README („YAML workflows for automation"); interne Semantik ungeklärt.
5. **NIP-AE Memory-Implementierung.** Konkretes Schema / API unbekannt.
6. **Tauri-Desktop-Code und Web-UI-Bundle-Mechanismus.** Nicht lokal vorhanden.
7. **Redis-Pub/Sub-Topic-Struktur.** Vermutet, aber nicht belegt.

---

## 10. Lizenz-Hinweis

buzz (github.com/block/buzz) steht unter der Apache-2.0-Lizenz. Dieses Dokument enthält keine Code-Kopien aus dem buzz-Repo; es dokumentiert ausschließlich bekannte Architekturmerkmale und leitet Übertragungsempfehlungen für Agent Relay ab.
