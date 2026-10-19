# HANDOVER — Agent Relay an Forge (HA-Server, 2026-10-19)

**Zweck:** Forge übernimmt das Agent-Relay-Projekt selbstständig. Diese Datei +
die verlinkten Docs sind die vollständige Betriebs- und Entwicklungsübergabe.
**Das Goal c5cec45b bleibt aktiv** („Frontend = OpenClaw-DNA + buzz/paperclip-Funktionen
bis GaRoN es selbst bewertet") — Forge arbeitet im Loop **weiter**, nicht ab.

---

## 1. Was das Projekt IST

**Agent Relay** = Multi-Agent-Kommunikations-Relay für die Home-Assistant-Sidebar.
Ein Go-Binary (~11 MB, statisch), SQLite (modernc, pure-Go), Embedded-SPA.
**Rolle**: Desktop-artige Oberfläche für Agent-Orchestrierung (wie OpenClaw Control UI),
die externe Agent-Laufzeiten **benutzt** (OpenClaw-Gateway, Ollama, …) statt sie zu enthalten.

**Repo**: `/mnt/192.168.178.68/share/projekte/github/agent-desktop-ha-app`
**Branch**: `development`, **HEAD**: `88a315a` (+ 3 weitere Commits bis `3391123`-Linie —
alle committed, Vet grün), **17 Commits seit 2026-10-18**.
**Live-Server**: Port 8099 (nicht persistent gestartet — siehe Server-Start unten).

**Der Entity-Kern (GaRoN-Definition, FINAL — nicht abändern!):**
- **Projekte** sind Aufgaben/Ziele (Agent Relay, HA-Integration, …) — **eigenständige
  Entities mit Task-Board**, unabhängig von Räumen. Räume KÖNNEN sie referenzieren
  (Board-Tab), müssen aber nicht.
- **Räume (Streams)** = Gruppen-Kanäle, team-/projektbezogen, mit Presence-Dots.
- **Chats (DMs)** = end-to-end-private Kommunikation mit GENAU EINEM Agent —
  nie projektgebunden, auto-benannt „🧑‍💻 <AgentName>".
- SPA-Gruppen: **STREAMS / DIREKT (🔒) / PROJEKTE / AUSGELADEN** (GaRoN-Wording:
  „end-to-end-Kommunikation während Räume streams sind").

## 2. Architektur (verlinkte Docs lesen — vollständig!)

- `ARCHITECTURE.md` — das Entity-Modell + Adapter-Konzept + Verlaufs-Persistenz
- `docs/DESIGN.md` — die Design-DNA (4 Quellen!): **§4 OpenClaw-Claw-Dark** (die
  bindende Tokens: canvas #0e1015, s2 #191c24, accent #ff5c5c, text #bcbcc0, radius 10),
  §1 Linear (alternativ), §2 Slack-Dichte, §3 Campsite-Ruhe; plus Instrument-Sans
- `docs/REVERSE-ENGINEERING.md` — buzz/paperclip/Control-UI-Analysen (buzz=pure
  Event-Log, Paperclip=Control-Plane, OpenClaw=Gateway+ControlUI-Muster)
- `docs/design-basis.md` — die 15-Pattern-Matrix (Pattern→Quelle→Relay-Anwendung)

## 3. Der Loop (so arbeitest du, Forge)

Der Vision-Loop mit dem Designer-Agent ist **bewährt** über 11 Runden:
1. **Sehen** (Screenshot via Edge-headless ODER npx playwright — Muster in
   `~/.openclaw/workspace/designer/SOUL.md`)
2. **Kritisieren** (max. 3 Befunde, pixel-belegt, Erwartungssatz je Fix)
3. **Bauen** (Coordinator: Frontend-Solo geht auch — der Designer committe selbst 4Commits)
4. **Verify** (eigener After-Shot, Erwartungssätze bestätigen/widerlegen)

**Designer-Persona/BIAS**: `~/.openclaw/workspace/designer/` (BIAS.md = 12 Regeln
von Guide-Erbe, SOUL.md = Vision-Loop, review-tracker.md = Runden 1–11 Kette,
SESSION-DESTILLAT.md = GaRoN-Direktweisungen).
**Modell-Pinning**: designer = muse-spark-open:cloud (Vision!), fallback gemma4:31b.
**WICHTIG**: glm-5.3:cloud ist NICHT vision-fähig — nur flash. muse-spark/gemma4:31b/mistral-large-4/kimi-k3 sind Vision.

**ACP-Review-Läufe** (bewährt): codex (kimi-k2.7-code:cloud) = Go/Feature-Audits,
copilot (deepseek-v4-pro) = Security/HA-Contract. **Beide ACP-Entries aktiv**,
permissionMode=approve-all, Agent-Entries unter Harness-NAMEN (codex/copilot/claude)
mit runtime.acp.agent-Mapping. Der yielded-Bug der Dashboard-Sessions: Subagenten-Läufe
umgehen ihn zuverlässig.

## 4. Server-Infrastruktur (kritische Fakten)

- **Relay-Start**: `PORT=8099 RELAY_DATA_DIR=/tmp/ar-fresh /tmp/agentrelay` —
  Background via OpenClaw exec-Session (setsid/nohup-Wrapper sterben am Turn-Ende!)
- **Datenbank**: SQLite braucht **echte fcntl-Locks** — NIE auf SMB/NFS-Mounts
  (Synology-SMB bricht). In `/tmp/ar-fresh` (Dev) bzw. `/data` (HA-Add-on).
- **Demo-Seed**: `node bin/seed-demo.js` (idempotent, prüft chat-room presence;
  legt 6 Agents, 4 Projekte, 4 Chat-Räume + 5 Projekt-Räume, Tasks, Chat-Verlauf,
  Reaktionen an). Marker-Check: chat-room „General" vorhanden = skip.
- **Dual-Binary-Falle**: Beim Parallel-Bau (Designer-Frontend-Solo + Coordinator-
  Backend) bauen beide ein Binary — der Letzte hält den Port. Klären: wer serviert
  gerade. `agentrelay-r11`-Prozesse des Designers ggf. killen.
- **Screenshots**: Windows-Edge-headless von WSL2 (WSL-IP statt localhost!):
  `"/mnt/c/Program Files (x86)/Microsoft/Edge/Application/msedge.exe" --headless=new
  --disable-gpu --user-data-dir="C:\\Temp\\edge-profile" --window-size=1440,900
  --virtual-time-budget=20000 --screenshot="C:\Temp\relay-shot.png" "http://<WSL-IP>:8099/"`
  — Budget ≥ 20 s (der Async-Boot render verzögert).
  **Oder**: `npx playwright screenshot` (Designers Tool, vollständiger/besser).

## 5. Code-Organisation (die Pfade)

```
cmd/agentrelay/        — main.go (graceful shutdown, env-wiring)
internal/store/        — SQLite-Repo (modernc, CRUD, Reactions, Search, dm-Migration)
internal/adapter/      — echo | openai | openclaw (natives WS-Protokoll, live-proven)
internal/runtime/      — Turn-Planung (Mentions, Serialisierung, dm-Routing)
internal/api/          — REST + WS-Hub (per-sub write pumps, bearer-gate optional)
web/public/            — SPA (index.html, styles.css, app.js — plain JS, kein Build!)
web/web.go             — Embed + SPA-Fallback
agent-relay/           — HA-App-Paket (config.yaml, Dockerfile, run.sh, build.yaml)
bin/build.sh           — Cross-Compile amd64/arm64 + Staging agent-relay/dist/
bin/seed-demo.js       — Demo-Seed (idempotent)
design-reviews/ + shots/ — Designer-Workspace-Chain (in ~/.openclaw/workspace/designer/)
```

## 6. Was Forge ALS NÄCHSTES baut (der offene Plan — Prioritäten)

**Das Ziel** (c5cec45b): „Frontend visuell+funktional = OpenClaw-DNA mit buzz/paperclip-Funktionen".
**Der aktueller Stand (11 Runden + 2 Audits + 3 Quick-Wins) ≈ 55 % am Ziel.**

**Codex' Funktions-Audit** (31 Lücken: 10 P1 / 12 P2 / 9 P3; Bericht:
`audits/raw/2026-10-19-feature-gaps-audit.md`) — **die P1-Lane (offene Quick-Wins)**:
- Message-Edit (S-Aufwand: ✎ an eigenen Messages, PATCH-Endpoint existiert)
- Attachments (M: Multi-Part-Upload, Vorschau, Store in /data)
- Notifications (S/M: Browser-Notification API v1 ODER HA-Bridge)
- Threads (M: Antworten-Verzweigung — braucht Thread-Modell+UI)

**Design-Baulane** (die 3 R11-Befunde des Designers — alle pixel-belegt, Report:
`~/.openclaw/workspace/designer/design-reviews/2026-10-19-round-11-muse.md`):
- Identity-Card **Materialität** (taktiler Anker: Inset-Shadow, Avatar-Ring, Dropdown
  mit Shortcut-Hints/Trenner — seine Korrekturvorschläge stehen im Bericht)
- Gruppen-**Differenzierung** (DM=Avatar-Styling, Projekt=Badge, AUSGELADEN=gedimmt
  — seine Erwartungssätze: „RÄUME/DIREKT/PROJEKTE je eigenes visuelles Register")
- Main-**Komposition** (Panels/Raster statt isolierter Blase, Breadcrumb statt
  Ellipsis-Kürzung, Tiefenschicht zwischen Sidebar/Main)

**P2-Nächste**: Threads, Task-Chatter, Approvals-UI, Routines, DMs ausgebaut
**P3-Später**: Voice, Mobile, Desktop-App, Multi-Tenant, Audit-Log, Budgets

**Backend-Fixes noch offen**: Attachments-API + Threads-Datenmodell + Notification-Bridge.

## 7. Bekannte Fallstricke (die nicht dokumentierten Lehren)

1. **yielded-Bug in Dashboard-Sessions**: Nach ACP/Config-Reload-Artefakten friert
   die Dashboard-Sessions-Queue ein („sessions_yield"-Zirkel). Fix: **Subagenten-Läufe**
   (`sessions_spawn agentId=designer` o.ä.) umgehen die Session komplett und liefern
   zuverlässig — das ist der Weg für den Loop, NICHT die Dashboard-Session.
2. **go:embed**: CSS/JS-Änderungen im SPA sind **nicht live ohne Rebuild** — immer
   `go build -o /tmp/agentrelay ./cmd/agentrelay` + Server-Restart nach SPA-Edits.
3. **SQLite+Fehler-FKs**: ALTER-RENAME repointet FK-Referenzen abhängiger Tabellen
   (messages/room_members zeigten nach rooms_old). Rebuild-Migrationen müssen die
   Dependents mitsätzlich re-createn (migrate()-Funktion hat das Pattern).
4. **Config-Reload-Supersede**: Modellspec-Änderungen (agents.entries.*.model)
   invalidieren die Runtime-Generation — ACP-Sessions hängen dann in Yield-Loops.
   Neustart + neue Session = saubere Lösung.
5. **Model-Tagging**: nur Vision-Fähige für den Designer (muse-spark/gemma4:31b/
   mistral-large-4/kimi-k3). glm-5.3:cloud ist TEXT-ONLY.

## 8. Forge-Verantwortungen (die Übergabe im Detail)

- **Forge übernimmt**: den Loop (Designer-Befunde → Fixes → Verify → Shot),
  das Repo (Commits, Server-Betrieb, Builds), die Designer/ACP-Wiring,
  das Goal-Tracking bis GaRoN es bewertet
- **Marvin (Coordinator) bleibt erreichbar**: auf Music-Book (diese Session-Infra),
  für Eskalationen, Model-Pinning-Fragen, BIAS-Revisions, Server-Notfälle
- **GaRoN bleibt der Owner**: bewertet das Ergebnis am Shot, erteilt Gos/Prios,
  ist die letzte Instanz für Design-Entscheidungen

## 9. Nächster Forge-Schritt (start hier!)

1. **Lies Diese Datei + ARCHITECTURE.md + docs/DESIGN.md** (komplett)
2. **Server starten + seed**: `PORT=8099 RELAY_DATA_DIR=/tmp/ar-fresh /tmp/agentrelay &`
   dann `node bin/seed-demo.js` (idempotent — 6 Agents, 5/4 Räume, 4 Projekte,
   Tasks, 23+6+4+4 Chat-Verlauf, Reaktionen) → der Demo-Stand ist testbar
3. **Designer-Reaktivierung**: `sessions_spawn agentId=designer` mit Verweis auf
   SESSION-DESTILLAT + review-tracker.md → er startet mit dem Loop
4. **Sichtbar machen**: Screenshots via Edge/Playwright-Muster → shots/ in den
   Designer-Workspace (der erlaubte Pfad)
5. **Der Loop läuft**: Sehen → Befund → Fix → Shot → Verify — bis GaRoN es bewertet
6. **P1-Features bauen** (Codex' Matrix + die 3 R11-Befunde): Message-Edit →
   Attachments → Notifications → Threads

## 10. Forge sollte NICHT tun

- Das **Entity-Modell abändern** (GaRoN-Definition §1 ist FINAI — Projekte/Streams/DMs)
- Die **Design-DNA wechseln** (claw-dark + Slack-Dichte + Control-UI-Muster —
  die Formelsprache ist geprüft über 11 Runden)
- Den **Designer/Guide-System abstellen** (das Vision-Loop-Muster ist der Motor)
- Das **Relay an ein anderes DB-Backend hängen** (SQLite+modernc ist eine bewährte
  Wahl — Postgres/Redis sind buzzs Überbau, nicht ours)

## 11. Kontakt + Goal-Verankerung

**Marvin (Coordinator)**: Session `agent:coordinator:dashboard:5e8012b3-fc03-4aac-bcee-711e5f96af57`
auf Music-Book — erreichbar via sessions_send für Eskalationen, Model-Pinning,
BIAS-Revisionen, Server-Notfälle. Der Designer-Workspace:
`~/.openclaw/workspace/designer/` (Loop-Kette über 11 Runden + Tracker + Shots + Destillat).

**Das aktive Goal** (systemseitig): `c5cec45b` — „Agent-Relay-Frontend visuell+funktional
= OpenClaw-DNA mit buzz/paperclip-Funktionen; iterativer Loop bis GaRoN-es so bewertet"
— **Status: advance** (läuft weiter; Blockier-Regel: 3 konsekutive Turns desselben
Blockers). Das Progress-Card der Coordinator-Session zeigt die Runden-Tabelle
(8/9 geschlossen). **GaRoN hat das letzte Wort** über Prioritäten und Ziel-Erreichung —
das Goal endet nur auf sein Urteil, nicht auf Forge-Durchsatz.

**Guide (📕)**: `agent:guide` auf Music-Book (eigener Workspace, minimax-m2.7) —
Prompt-Doktor/BIAS-Pfleger; sein Amts-Votum zum Designer (3 Befunde) ist
eingearbeitet (Modell-Pinning ✓ Report-Gate ✓ review-tracker ✓).

---

**Übergabe abgeschlossen** — Forge hat das Repo, die Docs, das Designer-System,
die Server-Infrastruktur, die P1/P2/P3-Taxonomie, die Fallstricke und den Loop.
Der Goal bleibt active — Forge arbeitet weiter, bis GaRoN es bewertet.
EOF