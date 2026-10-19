# FORGE-START — Agent Relay Übergabe (für Forge auf dem HA-Server)

**Von:** GaRoN (Owner) · **Übergeben:** 2026-10-19 durch Marvin (Coordinator, Music-Book)
**Zweck:** Du (Forge) übernimmst dieses Projekt selbstständig. Das Goal bleibt aktiv
(`c5cec45b`): „Frontend = OpenClaw-DNA mit buzz/paperclip-Funktionen — Loop bis GaRoN
es selbst bewertet". Du arbeitest es weiter — **nicht ab**,weiter.

## Sofort lesen (in dieser Reihenfolge)

1. /mnt/192.168.178.68/share/projekte/github/agent-desktop-ha-app/HANDOVER.md — die komplette Betriebs-übergabe (11 Abschnitte: Projekt, Architektur, Loop, Infra, Pfade, offener Plan, Fallstricke, deine Verantwortungen)
2. /mnt/.../agent-desktop-ha-app/ARCHITECTURE.md + docs/DESIGN.md (+ REVERSE-ENGINEERING.md, design-basis.md)
3. handover/designer/ — der Vision-Loop über 11 Runden: SESSION-DESTILLAT.md (GaRoNs Direktweisungen!), BIAS.md (12 Regeln), review-tracker.md (Kette), die 13 Berichte, die Persona-Dateien
4. handover/audits/ — Codex' 31-Lücken-Matrix (P1/P2/P3) + der Runde-1-Review
5. handover/shots/ — der visuelle Verlauf (vor/nach je Runde)

## Der Start

1. Server: `PORT=8099 RELAY_DATA_DIR=<pfad> ./dist/agentrelay-linux-amd64` (oder rebuild: `bin/build.sh` — GO muss installiert sein) — NIE auf SMB-Mounts (SQLite-fcntl-Regel)!
2. Seed: `node bin/seed-demo.js` (idempotent)
3. Shots: npx playwright ODER edge-headless (Muster im HANDOVER §4)
4. Loop: `sessions_spawn agentId=designer` (auf dem HA-OpenClaw — der Agent muss dort registriert sein: Registry-Setup siehe HANDOVER §3: muse-spark-open:cloud gepinnt, BIAS/SOUL aus handover/designer/)
5. P1-Bau: die 4 Features aus Codex' Matrix (Message-Edit, Attachments, Notifications, Threads) — die R11-Design-Befunde parallel (Identity-Materialität/Differenzierung/Komposition)

## Die Regeln (NICHT ändern)

- Entity-Modell FINAL: Projekte=Ziele (eigenständig mit Board), Räume=Streams (optionaler Projekt-Bezug), DMs=end-to-end mit 1 Agent (🔒, auto-named)
- Design-DNA: claw-dark Tokens + Slack-Dichte + Control-UI-Muster (11 Runden bewährt)
- SQLite bleibt (modernc, pure-Go) — NIE SMB/NFS-Mounts
- Der Loop: Sehen → Befunde (max 3, pixel) → Fix → Verify → Tracker — Guide-BIAS gilt
- GaRoN bewertet — das Goal endet nur auf sein Wort

## Kontakt

- Marvin (Coordinator) auf Music-Book: `sessions_send → agent:coordinator:dashboard:5e8012b3-fc03-4aac-bcee-711e5f96af57` — für Eskalationen/Model-Fragen/Server-Notfälle
- GaRoN: bewertet am Shot, erteilt Prioritäten
- Guide (📕, Music-Book): für BIAS-Pflege falls der Designer driftet

Alles ist beweisbar dokumentiert — 17 Commits, 11 Runden, 31-Lücken-Matrix, die
loop-Infrastruktur. Bau weiter, GaRoN bewertet beim nächsten Shot.