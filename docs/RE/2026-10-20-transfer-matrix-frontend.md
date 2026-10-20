# Transfer-Matrix Frontend — buzz × Paperclip × OpenClaw Control-UI → Agent Relay SPA

Stand: 2026-10-20. Basis: `2026-10-20-frontend-buzz.md`,
`2026-10-20-frontend-paperclip.md`, `2026-10-20-frontend-openclaw-controlui.md`.
Ziel: unsere plain-JS-SPA (`web/public/{index.html,app.js,styles.css}` —
State = ein `state`-Objekt (`app.js:8-22`), WS-Events
`message.created/agent.started/typing/agent.token/agent.finished/rooms.changed/system`
(`app.js:89-133`), Rest-API mit Bearer, kein Build-Schritt.

Entity-Modell unseres Relays: `rooms[]` (`kind: dm | …`), `agents[]` (id,
name, emoji, color, enabled, adapter), `projects[]` (Projekt-Räume), 
`messages{}` je Raumpuffer, `reactions{}`, Roster = Chips. Sidebar-Gruppen
jetzt: **STREAMS / DIREKT / PROJEKTE / AUSGELADEN + Team** (`app.js:254,265`);
Render-Funktionen: `renderRoomList` → `#navGroups` mit `data-group-toggle`
(`app.js:190-276`), Chat-Fläche `#messages` + `#composer` + `#mentionPop`
(`index.html:80-89`), Roster `.roster-chip`+`.presence` drags (`app.js:237-238`).

Legende: **1:1** = übernehmen wie beschrieben · **Angepasst** = Idee + kleine
Relay-Übersetzung · **Nicht** = bewusst weglassen (Begründung). Je Zeile
Ursprung (System + Beleg-Pfad aus den drei RE-Docs).

| # | Pattern | Quelle (Beleg) | Relay-Umsetzung | Haltung |
|---|---|---|---|---|
| 1 | **Sidebar-Zonen** Pages/Other/Groups/Coding → unsere STREAMS/DIREKT/PROJEKTE | OpenClaw `sessions-and-sidebar.md:187-198` | Unsere Gruppenstruktur existiert schon (`app.js:254`); Anpassung: „Other"-Ausblendregel übernehmen (Header weglassen, wenn einzige Zone) + Zähler im Kopf der eingeklappten Zone („collapsed header keeps the true count") | 1:1 (klein) |
| 2 | **3 feste Slots rechts je Row** (Kinder-Count / Unread-Dot / Aktivität), Priorität Fehler > Attention > Activity | OpenClaw `sessions-and-sidebar.md:257-258` | `.room-row`: rechte `.row-badges` mit Slot-Spans statt freiem HTML-Gemix; Slot-Regel als kleine Funktion `rowSlot(room)` → `unread \| activity \| error` | Angepasst (wir haben keine Kinder-Bäume v2; Slot 1 = Projekt-Chip) |
| 3 | **Unread-Dot + „opening marks read"** | buzz `useLiveChannelUpdates.ts:34-40` (observed unread), OpenClaw L272 | Bereits partiell (`.room-presence`-Dot `app.js:200`). Ausbau: Read-Marker in `localStorage` pro `room.<roomId>.lastRead` beim Öffnen gesetzt; WS `message.created` vergleicht → Dot. Escape = „Raum als gelesen" (buzz `keyboard-shortcuts.ts:107-113`) | 1:1 |
| 4 | **Presence: Live-Sub primär, Poll-Backstop, Idle-Stufe** | buzz `presence/hooks.ts:17-34` | Unsere WS-Verbindung ist schon live; Presence-Dots an `agents[].enabled` + neuer `last_seen`-Event-Adapterstatus: 3 Zustände `online / idle(amber #f59e0b) / offline` — amber-Ausnahme aus OpenClaw (`--presence-idle`) übernehmen; 60 s-Backstop-Poll nur beim `ws.onerror`-Wiederhol | 1:1 |
| 5 | **Avatar-Saturate/Opacity-Zustand** | OpenClaw `control-ui-core-yIvVj14Y.css` (`saturate(.4)/saturate(.8)`, `--agent-avatar-opacity`) | `.roster-chip.idle .avatar` bekommt `filter: saturate(.6); opacity:.85`, DisabledAgents `saturate(.4); opacity:.55` — CSS-only, ein Regelblock | 1:1 |
| 6 | **Composer = Feature-Hook-Stack** | buzz `MessageComposer.tsx:1-66` | In plain JS statt Hooks: `composer/*`-Module (mention.js, drafts.js, upload.js, shortcodes.js) + ein zentrales `composerInit()`; Stufen-Feature-Matrix wie buzz | Angepasst (IIFE-Funktionen statt Hooks; gleiche Modularität) |
| 7 | **@-Mention-Suggestion (Pop überm Composer)** + Nicht-Member → Invite-Dialog | buzz `MentionAutocomplete`, `NonMemberMentionDialog` | `#mentionPop` existiert (`index.html:86`); Ausbau: Mention eines Nicht-Room-Agent → Inline-Zeile im Pop „@x einladen?" statt Dialog (HA-Sidebar ist eng) | Angepasst |
| 8 | **Mention nicht mitten im Code-Span** | buzz `isMentionCodeContext` | Beim `@`-Trigger im `composerInput`: nur triggern, wenn kein ``` -Block im Draft offen ist — 5-Zeilen-Abfrage | 1:1 |
| 9 | **Draft-Persistenz pro Raum** | buzz Draft-Keys (`draftSubmitKey.ts`), OpenClaw Draft-Retirement | `localStorage["relay.draft.<roomId>"]` write-on-input (debounced 300 ms), restore beim Raumwechsel; Senden clear. Offen (OpenClaw „composer-draft-retirement") zeigt: Retirement-Limits brauchen wir nicht (HA-Browser-Storage ok) | 1:1 |
| 10 | **Typing-Kette**: eigener Broadcast suppressed bei disconnect; fremde Drafts als Bubble mit Lifecycle | buzz `useTypingBroadcast` + `relayClientSession.ts:311`; OpenClaw `chat.md:52-54` | Wir haben schon `#typingBar` + `typing`-Event (`app.js:102`). Ausbau: Bubble im Raum für fremd-Agent „denkt…" mit 30 s-Expiry + Fade (CSS transition), nicht nur Bar — nur wenn Agent-Count im Raum >1 | Angepasst |
| 11 | **Follow-Scroll-Vertrag + Latest-Button** | OpenClaw `chat.md:723-732` (+ Chunk `stream-auto-follow-controller`) | `#messages`: `isNearBottom()` (<= 48 px) beim scroll-Event cachen; bei `agent.token`/`message.created` auto-scroll NUR wenn near-bottom; sonst Pill „N neu · ↓" überm Composer; Pill-Klick = scroll to bottom; Eigen-Send resynced immer | 1:1 (Kern!) |
| 12 | **Turn-Bündelung „Worked for…"/Chain-of-Thought** | OpenClaw `chat.md:757-766`; Paperclip `IssueChatThread.tsx:1150-1156` | Agent-`agent.started → finished`-Span in `#messages` als eine Gruppe: Kopf „🤖 Name · schreibt seit 12 s", Tokens inline; nach `agent.finished` Kopf einklappbar (<details>-Element!, plain JS ok) mit Zusammenfassung „3 Sätze · 2,1 s" | Angepasst (details/summary statt Disclosure-Component) |
| 13 | **Message-Grouping (gleicher Autor)** | buzz `msg-grouping` — haben wir schon | `state.lastMetaKey` (`app.js:17,508`); zusätzlich `DayDivider` (buzz) als `<div class="day">` bei Tageswechsel | 1:1 erweitert um DayDivider |
| 14 | **Status-Registry** als EINE Quelle | Paperclip `lib/status-colors.ts:1-4,42-96` | `statusColors = { badge: {...}, dot: {...}, text: {...} }` in styles.css (CSS-Variablen) + `statusBadge(status)`-Funktion — Status aus WS: `rooms.changed` + Agent-`enabled` + Run-Zustände; alle Badges (Board-Tiles, Agent-Cards, Conn-Dot) ziehen daraus | 1:1 |
| 15 | **StatusBadge-Pill** (surface-2 + caption, uppercase-Tracking) | Paperclip `StatusBadge.tsx`, Badge `text-[10px] uppercase tracking-[0.14em]` (`IssueChatThread.tsx:1208`) | `.badge-pill`-Klasse: `background:var(--s2); color:var(--ink-3); text-transform:uppercase; letter-spacing:.08em; font-size:10px` — einheitlich für Board-Tile-Status + Agent-Idle + Approval | 1:1 (Linear/Paperclip-Konsens — wie design-basis.md Zeile „Status-Badges als Pill") |
| 16 | **Approval-Card** (Kind-Badge + „Requested by" + Payload + Auflösungs-Buttons nur bei offenem Status) | Paperclip `ApprovalCard.tsx:11-60` | v2-Backlog: sobald Adapter `approval.required`-Event melden, Modal `#modal` mit Card-Layout (Icon-Pill, Kind-Badge, Beschr-Text, Buttons Genehmigen/Ablehnen); bis dahin: **Nicht** (kein Schema, keine Datenquelle) | Angepasst / später |
| 17 | **Run-Liveness-Kopie** (advanced/plan_only/empty_response mit eigenem Ton) | Paperclip `IssueRunLedger.tsx:43-60` | Fertigungs-Tüte fürs Turn-Close: bei `agent.finished` unterscheide „Ergebnis" vs. „nur gesprochen" (Tokens enthielten Code? → Badge „Patch" vs. „Antwort") | Angepasst — erst v3, braucht Adapter-Signal |
| 18 | **LiveRun-Widget mit 3 s-Poll + Cancel** | Paperclip `LiveRunWidget.tsx:24-38` | Bei uns läuft der Run live über WS (`agent.token`) — Poll **nicht** nötig; Stop-Button im `chat-head` („⏹ Run anhalten") während `agent.started..finished`-Fenster | Nicht (Poll) / 1:1 (Stop-Button) |
| 19 | **Shortcut-Katalog als Daten + Hilfsdialog** | buzz `keyboard-shortcuts.ts:24-250`; Paperclip `?` | `shortcuts[]`-Array in app.js (id/label/keys) + `?`-Dialog über `#modal`; wir übernehmen **Escape=mark read, ⇧Escape=alle gelesen, ⌘/Ctrl-K Suche, Enter/⇧Enter, @-Mention** | 1:1 |
| 20 | **Quick-Search/Command-Palette** | OpenClaw `command-palette`-Chunk + `sessions-and-sidebar.md:389-447`; Paperclip `/`-Fokus | Ctrl/⌘K = Suche über `rooms`/`agents`, Enter springt; (Kein Prompt-Start-aus-Palette — das braucht Sessions-RPC, den wir anders haben) | Angepasst (einfacher) |
| 21 | **Breadcrumb-Context + document.title** | Paperclip `BreadcrumbContext.tsx:16-31` | Beim Raumwechsel: `document.title = raumname · Agent Relay` (HA-Sidebar-Tab nutzt das), + „← zurück"-Zeile im `chat-head` wenn man aus einem Projekt kam | 1:1 (billig, hoher Effekt) |
| 22 | **Row-Subtitle = Digest** | OpenClaw `chat.md:132-134` | Unter Raumnamen in der Sidebar: letzter Message-Text (esc, 1 Zeile, `--ink-4`) — „what is happening" im Sidebar-Scan, unser stärkster Dichte-Gewinn | 1:1 |
| 23 | **Snooze/Archive mit Presets + Undo-Toast** | OpenClaw `sessions-and-sidebar.md:242-255` | Für Räume v2: „Stummschalten" (kein WS-Notify) mit Presets; Undo-Chip im Toast v2. — Nicht in v1 (keine Notification-Komponente im Scope) | Nicht jetzt |
| 24 | **Board-Tiles / Inline-Board** | OpenClaw CSS-Chunks `board-view/board-document`; unsere design-basis-Zeile | `#inlineBoard` existiert + `.room-tabs` Chat/Board (`index.html:66-83`). Übernahme: Paperclip-Konter — kanban-Tiles mit `.badge-pill`-Status; Spalten = Status; Drag&Drop wie buzz `SidebarDnd.tsx` (HTML5 draggable, haben wir im Roster schon) | 1:1 weiterbauen |
| 25 | **Diffe-Nachricht-Darstellung im Chat** | buzz `DiffMessage.tsx` | Wenn Agent-Antwort ```diff-Fence enthält → `<details>` mit +/−-Hervorhebung, plain JS (kein Highlighter, nur CSS-Farben) | Angepasst |
| 26 | **„Context attached"-Chip** | OpenClaw `panels.md:38-41` | Nicht — wir haben nur einen Kontext (der Raum), unklar warum Chip nötig (ungeklärt bis Multi-Kontext v3) | Nicht |
| 27 | **EmptyState** (Icon + Message + Prime-Action) | Paperclip `EmptyState.tsx:9-27`, buzz `ChannelIntroBlock.tsx` | `#view-projects` leer → einheitlicher Empty-Block statt Textzeile; für neue Räume: Intro-Zeile (Raum-Zweck) | 1:1 |
| 28 | **Toast-Koaleszenz** (Cooldown + Reconnect-Suppress) | Paperclip `LiveUpdatesProvider.tsx:20-22` | Bei uns: Statuszeile `#connDot/#connText` + künftige Toasts; Regeln als Konstanten (10 s Fenster, max 3, 2 s nach Reconnect) | 1:1 (wenn Toasts kommen) |
| 29 | **Service-Worker-Boot + Recovery-Dialog** | OpenClaw `index.html` Head/Fallback | Nicht — HA-Ingress served alles frisch; ein „rendered"-Event ist unnötig | Nicht |
| 30 | **Query-Refresh-Hygiene (older ≤ newer)** | Paperclip `keepPreviousDataForSameQueryTail` | Bei WS-Late-Daten: `applyMessage(m)` überspringt, wenn `m.id` schon länger/nie — wir müssen nur Append-only-Garantien einhalten | 1:1 prüfen |
| 31 | **Randomisierte Refresh-Sturm-Vermeidung** (4-5 s, 3×-Cooldown) | OpenClaw `sessions-and-sidebar.md:213-216` | Unsere `rooms.changed` → `loadRooms()` sofort; bei Häufung (burst) coalesce auf 1 Aufruf per 4 s-Fenster | Angepasst |
| 32 | **Zoom/Text-Scale** (`--control-ui-text-scale`, buzz `--buzz-type-rem`) | s. o. | `html { font-size: calc(16px * var(--ui-scale)) }` mit Einstellungs-Slider später; v2: nur Token vorbereiten | Angepasst (token-only) |
| 33 | **Hover-Aktionen statt immer-sichtbarer Chips** | Paperclip `IssueChatThread.tsx:1261-1264` (opacity-0 group-hover) | Message-Row: Aktions-Icons (Kopieren/Reply) mit `opacity:0; .messages .row:hover & {opacity:1}`, Touch: immer sichtbar | 1:1 |
| 34 | **Reactions-Palette (6 Emoji + Count-Chips)** | OpenClaw `chat.md:56-75` | Wir haben `state.reactions`; Ausbau: Hover-Bar auf Message-Row mit 4 Emoji (👍 ❤️ 🎉 👀), Klick toggelt; Chip-Zeile unterm Text zeigt Count + wer (Tooltip) | 1:1 |
| 35 | **Max-W-85 %-Bubble + rounded-2xl vs. Bubble-Roundness** | Paperclip `IssueChatThread.tsx:1204-1220` | `.messages .mine` → `margin-left:auto; max-width:85%; border-radius:10px; background:var(--s3)` — Konsistenz mit claw-Radius 10 | 1:1 |

## Begründete Nichts (bündelt Zeilen mit „Nicht")

- **Polls für Live-Daten** (Paperclip 3 s-Poll, OpenClaw-Catalog-30s): Unser
  WS-Hub ist die Wahrheit, Polls sind nur Backstop — Buzz-Presence-Backstop
  übernehmen, Rest nicht.
- **Panes/Docks/PTYS/11-Themes/**: HA-Sidebar ist eine kleine Fläche; kein
  Second-Panel-Konzept in v2.
- **TanStack Virtual / TipTap / Service-Worker**: Overhead > Nutzen bei
  unseren Nachrichtenraten (kleines HA-Deployment).

## Reihenfolge-Empfehlung (Sprint-Abfolge, aus der Matrix)

1. Zeilen 11, 3, 22 (Follow-Scroll + Read-Marker + Row-Subtitle) = Dichte/UX-Gewinn sofort.
2. Zeilen 14, 15 (Status-Registry + Badge-Pill) = Konsistenz-Basis.
3. Zeilen 9, 12, 33 (Drafts + Turn-Fold + Hover-Aktionen).
4. Zeilen 4, 5 (Presence-Drei-Zustand + gedimmte Avatare).
5. Zeilen 19, 21, 34 (Shortcuts + Title + Reactions).
6. Zeilen 24 (Board-Tile-Ausbau) + Zeile 7 (Mention-Invite-Inline).