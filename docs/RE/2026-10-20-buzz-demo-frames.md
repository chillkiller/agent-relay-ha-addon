# buzz-Demo-Video — Frame-Analyse (offiziellesbuzz-Demo)

Stand: 2026-10-20 · Quelle: `https://buzz.xyz/sites/buzz/demo.mp4` (37,25 s,
1486×1080, 60 fps, kein Ton; 3,7 MB) — Frames (1 fps, 37 Stück) als Beweis
unter `shots/buzz-demo/f_001..037.png`. Analyse-Bezug: Transfer-Matrix
Frontend (35 Patterns) — dies hier ist die visuelle Bestätigungs-Quelle +
Pattern-Delta des offiziellen buzz-Demos.

## 1) Sequenz (Frame-Nachweise)

| Frames | Surface | Gezeigte Patterns |
|---|---|---|
| 1–5 | `#engineering` Desktop | „↑ 21 new messages"-Pill oben im Transcript; DayDivider „Today"; Message-Grouping; Reactions mit Count (`🙌 2`, `🙏 1`, `👀 1`, `💬 1`, `✅ 1`); Code-Block mit Zeilennummer + Syntax-Highlight (`ts`); **Linear·issue-Rich-Card** (Icon, „Linear · issue", `BUZ-519 · Reconnect reliability`); @mention-Pill grau; Sidebar-Zonen: Inbox/Projects/Agents-Navigation, Workspace-Gruppen (The Hive / Product / Launch Swarm) mit **Unread-Dots je Kanal**, `Channels` (Welcome), `Direct messages` (3 Personen, DM-Unread-Badge `1`), Identity-Card unten („Alex Rivera · Honeycomb Studios" + Online-Punkt); „Search everything ⌘K"; Kopf-Actions (9 Mitglieder, Huddle-Icon, Filter); Composer mit @/📎/🙂/AA-Iconleiste + Send-Button mit ↑-Glyph |
| 5–11 | Scroll ins Transcript | Agent-Handoff-Fanout im Kanal: Mensch @Alex Rivera adressiert **Bumble**-Mention-Chip (Inline-Pill mit Bot-Icon), Bumble antwortet, übernimmt und **übergibt an Fizz** (Inline-Mention), Fizz an **Honey** — agent-to-agent-Kette komplett im Kanal sichtbar, jeder Schritt eine eigene Message mit Zeitstempel; **„Bumble: Working"-Statuszeile unter dem Composer** (Avatar + Agent-Name + Run-Status) |
| 11–17 | weiterer Verlauf | Statuszeile wechselt Agent (Fizz: Working → Honey: Working) = Live-Turn-Indikator je laufendem Agent; Reactions an Agent-Nachrichten (👀 1, 💬 1 — Aufmerksamkeit/Threads); Human-Agent-Konversation gleichermaßen im Stream |
| 17–25 | PR-Kette | **Buzz·PR-Rich-Card** („Source PR is open in Buzz: Port Flight Path mobile prototype to Flutter") + **GitHub·PR-Rich-Card** (`honeycomb-studios/deploy...` mit GitHub-Icon); Fizz postet Deploy-PR-Link, Honey reviewed beide PRs und @-adressiert den Menschen für Human-Review; „Fizz: Working"/„Honey: Working"-Fußzeile durchgehend |
| 25–29 | Hover-Aktionen | Cursor auf Bumble-PR-Message: **Hover-Action-Bar** (👍 ❤️ 😂 🎉 ↻ ⋯) blendet rechts über der Message ein (Matrix-33/34 visuell bestätigt) |
| 29–33 | **PR-Detail-View im Workspace** (Frame ~31) | Navigation: Projects › flight-path › PRs › „Port Flight Path mobile prototype to Flutter #announce"; Tabs Conversation/Commits/Issues/PRs/Contributors; Status-Pill „Open" grün; Author (Bumble, Avatar); Branches (Mono, „agents/flight-path-flutter-port → main"); Activity (Created/Updated je 20 Jul); Reviewers-Panel: „Review requested — no approvals yet" + **Approve-Button** (grün) + Inline-Kommentar-Box — Code-Review-Surface in buzz selbst |
| 33–37 | Desktop-Kanal final | Gesamtlayout nochmal komplett; Statuszeile „Honey: Working" läuft weiter; DM-Unread-Blase (1) an Maya Chen persistent |

## 2) Delta gegenüber Transfer-Matrix (die 35 Patterns)

### Bereits in Matrix (durch Demo visuell BESTÄTIGT)
- **Matrix-11 Follow-Scroll/Latest-Pill**: „↑ 21 new messages" oben statt
  unten — buzz zeigt den Pill-Spender am Transcript-Kopf (Anpassung: bei
  uns newBelow unten bewährt; Kopf-Variante nur bei Deep-Link-Öffnung).
- **Matrix-13 Message-Grouping + DayDivider**: „Today"-Zeile sichtbar.
- **Matrix-15 StatusBadge-Pill**: „Open" grün im PR-View; Unread als
  Punkte-Dots in Gruppen + Zähler-Badge je DM.
- **Matrix-33/34 Hover-Aktionen + Reactions-Palette**: Genau die
  Hover-Bar (👍❤️😂🎉 + ↻) über der Message; Count-Chips darunter.
- **Matrix-22 Row-Subtitle**: Sidebar zeigt keine Subtitles, dafür
  **Unread-Dots** je Kanal + Zähler je DM (Dichte-Variante).
- **Matrix-19 Shortcuts**: „⌘K" Hint direkt im Suchfeld.
- **Matrix-16 Approval-Card**: „Review requested — no approvals yet" mit
  Approve-Button = Paperclip-Muster live (Buttons nur bei offenem Status).

### NEU (nicht in Matrix — aufgenommen als N1–N7)
| # | Pattern | Beweis (Frame) | Transfer-Urteil |
|---|---|---|---|
| N1 | **Agent-Working-Fußzeile**: unter dem Composer ein permanenter „<Agent>: Working"-Chip (Avatar + Name + Status), wechselt mit dem laufenden Run | 7, 17, 25, 33 | **P2-Sprint**: wir haben `agent.started/finished` + `#typingBar` — ein per-agent „🔧 <Name> arbeitet"-Chip im `chat-head`/unter Composer ist ein kleiner Sprint (WS-Zustand vorhanden) |
| N2 | **Inline-Agent-Mention-Chips**: Mentions von Agents als Pill mit Bot-Icon („🤖 Bumble") mitten im Text; Menschen-Klammern grau ohne Icon | 11, 17 | **P2-Sprint**: Mention-Rendering in der SPA (esc + Replace gegen Agent-Namen) — visuelle Agent-Abgrenzung, hoher Effekt |
| N3 | **Rich-Cards für externe Artefakte** (Linear·issue, GitHub·PR, Buzz·PR): Icon-Quelle + Kinds-Zeile + Titelfall unter der Message | 5, 17, 23 | **v3**: braucht Link-Metadaten/Adapter-Signal — wir haben keine externe Systeme-Anbindung v1; Backlog, nicht P3-Lane-Kern |
| N4 | **Agent-Handoff-Kette im Kanal**: Agent adressiert im Text den nächsten Agenten (@Fizz/@Honey), Turn-Kette läuft sichtbar durch 3 Agents + Human-Review-Anfrage zurück | 11–23 | **Arbeitsweise, kein Feature**: nuestro `@mention`-Routing kann das heute (Mention eines anderen Agents in Agent-Antwort → Turn) — im Demo-Skript als Flow dokumentieren |
| N5 | **Unread-Dots je Kanal in Gruppen** (Punkte, nicht Zähler) + DM-Zähler-Badge: Zonen-Header ohne Zähler, Gruppen-Kanäle mit kleinstmöglichen Dots | 1, 31 | **P3-Kleinigkeit**: unsere `.room-presence.unread`-Dot existiert; DM-Zähler haben wir — Dots in den Gruppen-Headern sind CSS-only-Politur |
| N6 | **In-Workspace PR/Issue-Surface** mit Approve-Flow | 31 | **v3/P3-Lane**: unser Board/Tasks deckt Issues ab; PR-Review braucht Git-Integration (P3-„Git-Integration"-Eintrag — bestätigt als buzz-konform) |
| N7 | **Composer-Iconleiste** @ / 📎 / 🙂 / AA + „Message #kanal"-Placeholder mit Kanal-Namen | 1 | **P2-Feinschliff**: Placeholder mit Raumnamen (Text-only, trivial); @-Button = Mention-Toggle haben wir über Composer-Input; AA/Zoom ist buzz-v3 |

### Bewusste Nichts (Demo bestätigt unsere Ausschlüsse)
- Keine Token-Streaming-Pillen im Kanal: buzz zeigt Agent-Runs als
  fertig gerenderte Nachrichten + Working-Fußzeile (N1) — NICHT als
  Live-Tokens; bestätigt unsere agent.wait-Phase-1-Wahl (ARCHITECTURE §2.2).
- Keine Kanban-Tiles im Kanal; Board bleibt in der Projekt-Surface.
- Keine Snooze/Sidebar-Filter-Popover im Demo — Dichte bleibt schlicht.

## 3) Sprint-Impuls (konkret, klein)

1. **N1 Agent-Working-Chip**: `agent.started/finished` → `state.agentWorking[roomId]`
   → Chip unter Composer („🤖 <Name> arbeitet…"); 30 %-Aufwand, 70 %-Wirkung.
2. **N2 Mention-Chips**: in `renderMessages` Agent-Namen als Pill rendern.
3. **N7-Placeholder**: `"Nachricht in #<Raumname>…"`.
4. **N5 Dots in Gruppen-Headern**: CSS-only.

Diese vier sind der „buzz-Demo-Sprint" — alle aus bestehenden WS-Events
und CSS ableitbar, kein Backend-Change. N3/N6 bleiben v3-Backlog.

## 4) Einordnungen

- Das Demo zeigt **buzz-Desktop 1486×1080 hell** (kein dark im Video) —
  unsere claw-dark-DNA bleibt (DESIGN.md §4 bindend), die PATTERNS sind
  farbunabhängig übertragbar.
- Frame-Nachweise: alle Zahlen oben verifiziert gegen
  `shots/buzz-demo/f_XXX.png` (Frame = Sekunde).
- Nichts anderes gefunden: keine Passwörter/Secrets/Externe-URLs außer
  GitHub-Beispiel-Link im Demo-Content (fiktiv honeycomb-studios).