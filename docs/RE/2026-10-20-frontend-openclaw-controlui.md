# Reverse-Engineering Frontend — OpenClaw Control UI (statisch)

Stand: 2026-10-20. Quellen (alles lokal, KEIN Live-Zugriff auf 127.0.0.1:18790 —
strikt statisch):

- Assets: `/usr/lib/node_modules/openclaw/dist/control-ui/` — `index.html`,
  `asset-manifest.json`, `assets/` (1.392 Dateien)
- Server-Bundles z. T. `dist/control-ui-*.mjs`
- Docs-Mirror: `/usr/lib/node_modules/openclaw/docs/web/control-ui/*.md`
- Build: `data-openclaw-control-ui-build-id="2026.10.4-release-383c124…"`
  (`dist/control-ui/index.html:2-3`)

## 1) Asset-Inventory / Chunk-Abbildung

- Vite-Build mit **asset-manifest.json** (version 1, sha256 pro Asset +
  `.br`/`.gz`-Vorverdichtung, z. B. `assets/about-page-B5fnhNH8.css`),
  `asset-manifest.json`. Serve-Contract: `dist/control-ui-assets-*.mjs` +
  `dist/control-ui-plugin-assets-*.mjs` (Manifest-basierte Assets, inkl.
  Plugin-Assets).
- Chunk-Namen = Feature-Karte (Auswahl, direkte Chunk-Dateien in
  `dist/control-ui/assets/`): `activity-page`, `agents-page`,
  `agents-home-page`, `approvals-page` (+`approval`), `apps-page`, `board-view`,
  `board-document`, `board-layout`, `browser*`, `capacity-meter`,
  `channels-page`, `chat-*`-Familie (`chat-session-rail`,
  `chat-sidebar-region`, `chat-transcript-styles`, `chat-detail-panel`,
  `chat-question-panel`, `chat-diff-highlight`, `chat-outbox-receipts`,
  `chat-html-preview-element`), `command-palette`, `composer-draft-retirement`,
  `config-page`, `dashboards-page`, `debug-overlay`, `dock-panel-styles`,
  `exec-approval`, `gateway-vitals`, `keyboard-shortcuts-dialog`,
  `link-reader-hovercard/panel`, `logbook-view`, `logs-page`,
  `markdown-runtime`, `markdown-mermaid`, `meetings-page`, `memory-import-*`,
  `model-providers-page`, `pending-session-create`, `plugins-page`,
  `portals-page`, `provider-usage`, `session-progress(+hovercard)`,
  `sessions-page`, `session-discussion-panel`, `sidebar-agent-roster`,
  `sidebar-people`, `simple-mode`, `skills-page`, `sparkline-tile`,
  `spreadsheet`, `stream-auto-follow-controller`, `terminal-page`,
  `theme-*` (artwork/catalog/palette — 11 Themes × color/light-dark!),
  `tool-catalog`, `usage(-page)`, `view-status`, `worktrees-page`,
  `youtube-video-card` u. a. — dazu Syntax-Highlight-Chips für ~60
  Sprachen (CLike/Clojure/Cypher/…, von `rolldown`-Chunks).
- Shell: `index.html` bootet via `boot-chat-*.js`/`boot-new-*.js`-Skripte,
  Service-Worker-„rendered"-Event + Recovery-Dialog (Fallback-UI mit Retry +
  „wait"-Button, `index.html` Footer-Script-Block). Theme-Farbe vor Erst-
  Paint mit `theme-color`-Meta „#faf9f7 (light) / #0e1015 (dark)" als Mirror
  der claw-theme `--bg`-Tokens (Kommentar im head, `index.html:8-12`).

## 2) App-Shell-/Routen-Bild

Routen-Namen ergeben sich aus Chunk-Gruppen und Docs (kein Route-Table im
Bundle lesbar): `/sessions` (+Worktrees-Tab), `/channels`, `/logs`, `/usage`,
`/systems`, `/new` (New-session-Draft), `/terminal`, `/settings/*`,
`/memory-import`, plus Plugin-seitige Seiten. Belege: docs
`feature-reference.md` (Systems: „visit `/systems`"; Chat via WS) und
`docs/web/control-ui/` + Chunk `*page*`-Namen. — Mobile/desktop-Varianten mit
`wa-page[view=desktop|mobile]` CSS-Umschaltung (Web Awesome; 
`control-ui-core-yIvVj14Y.css`, @layer wa-native-Block).

## 3) Theme/Visual-DNA — die claw-Token (aus dem Core-CSS extrahiert)

Beleg-Datei: `dist/control-ui/assets/control-ui-core-yIvVj14Y.css`
(312.900 B, minified, 1 Zeile — Werte via `grep -o` extrahiert):

- **claw-dark / claw-light als Vollsatz** (je Paare im CSS:
  dunkel zuerst hell): `--bg:#0e1015 | #faf9f7`, `--bg-elevated:#191c24 |
  #fff`, `--bg-hover / --bg-muted:#1f2330 | #efebe4`, `--bg-accent:#13151b |
  #f4f1ec`, `--bg-content #f4f1ec (light)`, `--border:#1e2028 | #e8e4dc`,
  `--border-strong:#2e3040 | #d6d0c5`, `--border-hover:#3e4050 | #b4ac9e`
  — Deckungsgleich mit docs/DESIGN.md §4-Zitaten.
- `--accent:#ff5c5c | #bd4531`, `--accent-hover:#ff7070 | #a83c29`,
  `--accent-subtle/glow` (alpha-Muster #ff5c5c1a/#ff5c5c33),
  `--accent-2:#14b8a6 | #0d9488` (Teal), `--accent-foreground #fff`.
- Semantik: `--ok:#22c55e | #166534`, `--danger:#f87171 | #b91c1c` (+ je
  muted/subtle-Aliase), `--chat-text:#bcbcc0` (chat-Text = --text),
  `--presence-idle:#f59e0b` (amber — Idle ist amber, nicht grau!).
- **Radius**: `--radius-sm:6px --radius-md:10px --radius:10px
  --radius-lg:14px --radius-xl:20px --radius-full:9999px`.
- **Space-Skala**: `--space-1..8: 4/8/12/16/20/24/32/40px`.
- **Shadow-Ladder**: `--shadow-sm:0 1px 2px #00000040` →
  `--shadow-md:0 4px 16px #0000004d` → `--shadow-lg:0 12px 32px #0006` →
  `--shadow-xl:0 24px 48px #00000080` (hell: warme Alternativen #3c2a18xx).
- **Sidebar-Formel**: `--sidebar-bg: color-mix(in srgb, var(--bg) 96%,
  var(--bg-elevated) 4%)` (dunkel) bzw. `color-mix(in srgb, var(--panel)
  98%, white 2%)` (hell) — exakt docs/DESIGN.md §3-Floskel.
- **Sidebar-Fade-Masken**: `.sidebar-shell__body--scroll-middle` wählt
  `--sidebar-body-mask: linear-gradient(to bottom, var(--sidebar-fade-in),
  var(--sidebar-fade-out))` (Kombination je Scroll-Zustand) + fixe
  Größen-Tokens `--sidebar-agent-avatar-size: 28px|32px`,
  `--sidebar-child-session-toggle-width: max(34px, calc(36px *
  var(--control-ui-text-scale)))` — es gibt einen UI-Text-Scale-Token!
- **Font**: `Instrument Sans` (Fallbacks laut DESIGN.md); UI-Scale via
  `--control-ui-text-scale` (z. B. in `--sidebar-child-session-toggle-width`
  oben).
- **Agent-Avatar-States**: `filter: saturate(.4)` / `saturate(.8)` +
  `--agent-avatar-opacity` (z. B. hover .85) in
  `.sidebar-agent-menu__agent*`-Selektoren — gedimmte nicht-aktive Agents;
  Focus-Ring doppelt (2px bg-elevated + 4px text-strong-mix).
- Blur-Avatar-Hintergrund: `filter: blur(12px) saturate(1.6)`/`blur(18px)
  saturate(1.15)` (Ambient-Reize hinter Panels).
- Web-Awesome-Basis-Palette (`--wa-color-red-95..05`, …) als Neutral-Fundament
  im selben Bundle (11 Stufen pro Farbe — nur Referenz, nicht
  übernommenswert).

## 4) UX aus Dokumenten (das eigentliche Gold)

Docs-Mirror = Design-Doku in Prosa. Kern-Patterns mit Beleg (Datei:Zeile):

### Session-Dashboard (sessions-and-sidebar.md)
- **Zonen-Sidebar**: Chip-Mode teilt die Sidebar in „Pages" (Home + pinned
  Ziele) + Session-Zonen **Other / Groups / Coding** / Pinned / Custom
  Groups; Coding startet eingeklappt und merkt den Zustand (L187-198).
- **Team-Mode**: „Show all agents" — jeder Agent als einklappbare Gruppe;
  48px-Header mit 36px-Avatar, 32px-Zeilen, 16px Indentierungen
  (L193-198); Owner-Avatare nur zeigen, wenn >1 Mensch (L195).
- **Row-Slots**: genau 3 Slots rechts: Kinder-Unread, Unread-Dot/Count, Aktivität/
  Attention; Priorität: „Requests for input and errors take priority over
  activity" (L257-258) — gleich wie unsere Attention-Sortierung.
- **Activity-Ring** statt nur Dot: laufende Runs zeigen Ring um das Row-Icon;
  Spinnen bei Startup, Pause bei „Queued", Reduced-motion → still (L187;
  Team-Mode L193). Ungeklärt: CSS-Klassename des Rings (in Chunks).
- **Attention-Vorschau bei Hover**: Attention-Icon zeigt älteste Frage/Aproval
  + „+N" als Tooltip/Preview, Tap zweits löst aus; Fragenschau schließt beim
  Öffnen der Session (L199).
- Zustand-Subtitles: Session-Observer schreibt kompakt Statusdigest als
  Row-Subtitle (`chat.md:132-134` — „The headline owns that run's sidebar
  subtitle"); done/failed-Digest bleibt sichtbar während der Row ungelesen ist.
- Row-Gruppen-Limits: max. 300 Sessions im Fenster, pinned zuerst (L211);
  Auto-Fetch der Roster-Events in randomizierten 4-5 s-Zeitslots mit
  3×-Cooldowns (L213-216) → Anti-Crash-Rate-Limitierung.
- Session-Menu-Aktionen (L295-314): Pin/Rename/Unread/Archive/Snooze- Presets/
  „Icon & color" (Farb-Streifen in Row + Dot im Chat-Title, 8 Claude-Code-
  Farben, L353-355), „Move to group", Fork, Copy-as-Markdown.
- Snooze mit Preset-Wake-Zeiten (1h/3h/evening/tomorrow/next Monday,
  L242-255) + Undo-Bestätigung.
- Filter&Sort-Popover mit Tabs `Filters` / `Display` (Group by, Sort by,
  Hide empty groups: when-filtering/always/never, L133-151, 259-269).
- New-Session-Naming: Utility-Model benennt im Vorschlag nach 1 s Pause ≥12
  Zeichen; Crustacean-Fallback-Namen (L17-43).
- „Search field updates immediately; command filtering waits 200 ms pause"
  (L226) — Such-Delays exakt.

### Chat (chat.md)
- **Follow-Scroll-Vertrag**: „While you are at or near the end, new messages
  and replies keep the transcript pinned … Scrolling up pauses following …
  Scroll back to the end, select **Latest**, or send a message to resume"
  (`chat.md:723-732`); eigene Prompts gleiten einmalig, ohne fade-out (L730).
- **„Worked for…"-Disclosure**: Commentary + Reasoning + Tool-Aktivität
  einer Turn-Einheit in EINEN Disclosure überm Answer; Live-Turns bleiben
  offen (L757-766); „Replying to"-Author-Klick scrollt zum Original und
  öffnet ggf. den Disclosure (L767-769).
- **Tool-Aktivität als kind-aware Rows**: Shell-Befehle mit Terminal-Output,
  Bounded-Inline-Diffs mit +/−-Stats; Bündelung zu „13 commands · 6 reads ·
  9 edits" (`feature-reference.md:30-32`); neuester laufender Call benennt
  die Gruppen-Header; Expander für raw args/output.
- **Sources-Strip** nach Replies wenn `web_search/web_fetch` des Runs
  matchen; Preview aus geliefertem Content, kein Refetch (L744-755).
- **Collaborator drafts**: Fremder Draft bleibt als Bubble sichtbar (max. 2
  Preview-Bubbles + „N weitere" Overflow mit „Several people are typing…"; 300 ms
  Fade-out nach 30 s Expiry; `chat.md:52-54`).
- **Reactions**: Quick-Palette mit 6 Emoji, Hover-Chip-Zoom zeigt wer; Toggle-
  Chips unter Messages (chat.md:56-75).
- Emoji shortcodes `:smi` mit „sizes to matches" und Enter/Tab (L197-208).
- **JSON in chat**: Tree-View + Raw-View für komplette JSON-Fences
  (L212-222).
- **Breite Layout**: „centered readable frame aligned with the composer",
  eigene Messages rechts INSIDE frame, andere identitätsfarben links; Avatar
  + Name + stable per-identity color für andere; „Replying to"-Linie mit
  Kurven-Konnektor aus Avatar (L782). Mobile ohne Avatare (L782 hinten).
- Composer: Attachment-Slots mit video-Frame-Preview; +Menu enthält Skills/
  Connectors/Web-search (chat.md 176-195, 189).
- Scroll: Composer-Wheel-Übergabe an Transcript („wheel continues into the
  conversation", L696-703); Task-Progress-Card-Collapsing-Heuristiken
  (L713-721).

### Panels / Docks (panels.md)
- **Ask OpenClaw-Dock**: „Surfaces that dock beside the current page instead
  of replacing it" (panels.md:11-16); merkt Platz rechts/unten + Größe im
  Browser-Profil; versteckt sich, wenn Full-Page offen ist.
- „Context attached"-Chip unter der gesendeten Message; Snapshot-Freeze.
- Unified side panel: Chat + Terminal/Browser/Files/Review/Sidechat-Tabs im
  geteilten Panel; Layout-Reset pro Session (L80-85).
- Browser-Panel: Live-Screencast, Annotationen → „Send to chat" Card
  (L133-167).
- GitHub side panel: „browser-style tab beside the conversation", max 10
  Tabs, Hover-Cards (L170-184).

## 5) Session/Chat-Render-Mechanik aus Bundles (Stichprobe)

- WS/RPC-Vertrag von der Docs-Seite: `chat.history`, `chat.send`,
  `chat.abort`, `chat.inject` (feature-reference.md:17); Up to 80 letzte
  Messages bei Open, bis 1.000/page beim Hochscrollen (L20); `sessions.list/
  patch/create`, `config.get/set/patch/apply/schema`, `cron.*`, `usage.status`,
  `logs.tail`, `device.pair.*`, `node.*`, `system-presence` (42-126) — RPC-
  Namens-Konvention: `<area>.<verb>`; wir haben das gleiche Konzept als
  REST+WS-Events, brauchen kein RPC.
- `stream-auto-follow-controller` Chunk = Follow-Scroll-Implementierung (§4).
- „composer-draft-retirement" Chunk = Draft-Recycling (Docs L537 — 20 Draft-
  Scopes pro Gateway 7 Tage 25 MiB-Attachments).
- „agent-run-control", „exec-approval", „approval-page" — die UI-Seiten für
  Run-Steuerung + Exec-Zulassungen; für uns Approvals-Muster von Paperclip
  einfacher.
- Ungeklärt: exakte Minified-Route-Tabelle; nicht nötig — Docs +
  Chunk-Namen decken die Karte.

## 6) Was für unsere kleine plain-JS Sidebar-SPA zählt

Übernehmbar (statisch beschrieben, kein Code):
1. claw-Token-Set (bereits in styles.css übernommen) + **`--presence-idle:
   amber`** — unsere Presence-Dots bekommen den 3. Zustand.
2. Sidebar-Zonenschema (Pages / Other / Groups / Coding → unsere
   STREAMS / DIREKT / PROJEKTE) 1:1-Abbildung.
3. 3-Slots-Rechts-Metrik je Zeile (Kinder/Unread/Activity) + Priorität
   error > attention > activity.
4. Follow-Scroll-Vertrag + „Latest"-Button („stream-auto-follow"-Muster).
5. „Worked for…"-Disclosure (Agent-Spanning + Tool-Group-Bündel)
   = unsere Agent-Turn-Gruppen.
6. Row-Subtitle = Session-Observer-Digest → bei uns: letzter Agent-Text
   (oder finished-Digest) als Subtitle unterm Raumnamen.
7. 4-5 s randomisierte Refresh-Anti-Storms + 3×-Cooldown — bei uns im
   loadRooms-Scheduler.
8. „Context attached"-Chip + „Starting/Queued/Snooze"-Presets — später.
Nicht anwendbar: Service-Worker-Boot (HA-Ingress ok ohne), 11-Theme-System,
Panels/Docks/PTYS/Capacity-Meter (Server-Scope), Web-Awesome-Components,
300-Session-Roster (skaliert bei uns nicht).