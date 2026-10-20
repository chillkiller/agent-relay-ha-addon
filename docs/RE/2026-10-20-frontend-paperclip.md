# Reverse-Engineering Frontend — Paperclip (paperclipai/paperclip)

Stand: 2026-10-20. Quelle: upstream-Klon github.com/paperclipai/paperclip @
Tag `v2026.428.0` (Commit `3494e84a`, 2026-04-28, Release-Changelog #4665)
unter `/tmp/re-work/paperclip-src` (MIT, `LICENSE`). Vorher lokal verifiziert,
dass `/share/projekte/github/paperclip-ha-addon/paperclip` nur ein HA-Wrapper
ist; der Dockerfile baut `pnpm --filter @paperclipai/ui build`
(`paperclip/Dockerfile:61-69`) — d. h. das UI-Paket heißt verifiziert
`@paperclipai/ui` und liegt in `ui/`.

## 1) Design-System & Tokens

- **shadcn/ui-Stack** (radix-ui + class-variance-authority; Belege
  `ui/components.json`, `components/ui/badge.tsx:1-10` — cva-Varianten
  default/secondary/destructive/outline/ghost/link, „rounded-full … px-2
  py-0.5 text-xs").
- Tokens in `ui/src/index.css` (911 LoC): shadcn-Standard-Semantik in
  **oklch** — `.dark { --background: oklch(0.145 0 0) … }`, plus `--chart-1..5`,
  `--sidebar*`, `--ring` (`.dark`-Block in `ui/src/index.css`). Kein
  claw-/Linear-Skin; Paperclip ist neutral-grau (zwei grau-Töne-Sets,
  Akzenten nur über Status-Tailwind-Klassen).
- **Kanonische Status-Farben-Registry** — der wichtigste Transfer-Baustein:
  `lib/status-colors.ts` (Header: „Every component that renders a status
  indicator … should import from here so colors stay consistent",
  `status-colors.ts:1-4`) exportiert Sets:
  - `issueStatusIcon` (backlog=mut-gray, todo=blau, in_progress=gelb,
    in_review=violett, done=grün, cancelled=neutral, blocked=rot;
    `:13-21`), `issueStatusText` (`:26-34`),
  - `statusBadge` (Pill-Klassen pro Status, inkl. Agent-Status active=cyan?
    — running=cyan, active=grün, scheduled_retry=sky, paused=orange,
    idle=gelb, archived=muted; `:42-80`, Default
    `statusBadgeDefault="bg-muted text-muted-foreground"` `:80`),
  - `agentStatusDot` (dot-BG-Klassen: `pending_approval=bg-amber-400`,
    `error=bg-red-400`, `:86-96`),
  - `priorityColor` (critical=rot, high=orange, medium=gelb, low=blau;
    `:102-107`).
- **StatusBadge** = Pill aus Registry, Status-String mit `_` → Leerzeichen
  (`components/StatusBadge.tsx` — „rounded-full px-2.5 py-0.5 text-xs …
  statusBadge[status] ?? statusBadgeDefault"). Ein Badge-Renderer für alle
  Entity-Typen (Issues, Runs, Agents).
- **Storybook als design-language-Quelle**: `ui/storybook/stories/
  status-language.stories.tsx` importiert `AGENT_STATUSES, ISSUE_PRIORITIES,
  ISSUE_STATUSES` aus `@paperclipai/shared` und zeigt Badge/Icon/Blocked-
  Attention-Arten; `control-plane-surfaces.stories.tsx` hält die
  Dichte-Devise: „Paperclip's common board surfaces are deliberately dense:
  task rows, approvals, budget policy cards, and audit rows all need to scan
  quickly while preserving enough state to make autonomous work governable."
  (Kontextabsatz, control-plane-surfaces.stories.tsx ~L40-45).

### Design-Prinzip „what is happening → does it need me" — Status
Gesucht im v2026.428.0-Klon: die wörtliche Formulierung existiert hier NICHT;
nächster Beleg ist die Akzeptanz-Zeile „Board can tell what is happening
without reading shell commands. Raw logs are still accessible, but not the
default surface." (`doc/plans/2026-03-13-features.md:205-207`). Die Formulierung
„what is happening → does it need me → what do I do" stammt in unseren
Docs (docs/DESIGN.md §2, docs/design-basis.md) aus Slack-Leitlinien + der
Transfer-Idee auf Paperclip — im Paperclip-Repo selbst nur als Geist vorhanden.
(Markiert: nicht wörtlich belegbar in v2026.428.0.)

## 2) Task/Run/Issue-Views

- **„Board" ≠ Kanban**: Issues ist eine Inbox-/Listen-Oberfläche
  (`pages/Issues.tsx`, `MyIssues.tsx`, `Inbox.tsx`); Spalten sind
  *Meta-Spalten* (assignee/project/workspace/parent/labels/updated) mit
  Umschalter (`IssueColumns.tsx`: `issueTrailingColumns`,
  `issueColumnLabels`, Beschreibungen wie „Issue state chip on the left
  edge.", „Ticket identifier like PAP-1009."). Kanban-Assoziation aus
  design-basis.md stammt von OpenClaw CSS-Chunks `board-view`/
  `board-document` (siehe OpenClaw-Dokument) bzw. von unseren eigenen
  Board-Tiles — Paperclip selbst rendert **Dichte-Listen**, keine Tiles.
- **EntityRow** = die universelle Dichte-Reihe: `leading / identifier /
  title / subtitle / trailing / selected / to` (Props `EntityRow.tsx:10-20`),
  Klassen „flex items-center gap-3 px-4 py-2 text-sm border-b border-border
  … hover:bg-accent/50, selected && bg-accent/30" (`:27-35`); der Identifier
  (PAP-1009-artig) in `text-xs font-mono` (`:41-44`) — **mono für
  Maschinenwerte**.
- **Runs**: `LiveRunWidget.tsx` pollt Active/Live-Runs alle 3000 ms
  (`useQuery({ refetchInterval: 3000 })`, `LiveRunWidget.tsx:24-38`),
  dedupliziert Run-Map, Cancel-Buttons pro Run (`isRunActive`/`cancellingRunIds`,
  `:15-38`); Transkript via `useLiveRunTranscripts` (components/transcript/).
- **Run-Ledger mit Liveness-Kopie**: `IssueRunLedger.tsx` definiert pro Run-
  Zustand Label+Tone+Beschreibung — `completed` („Completed", grün), `advanced`
  („Advanced", cyan), `plan_only` („Plan only", amber: „Run described future
  work without concrete action evidence"), `empty_response` … als Map
  `LIVENESS_COPY: Record<RunLivenessState, LivenessCopy>` (`:43-60`).
  → Muster: Run-Status bekommt semantische Unterscheidung, nicht nur done/failed.
- Watchdog: Run-Ledger kann Watchdog-Entscheidungen (keep-alive/stop)
  aufzeichnen (`IssueRunLedgerProps.pendingWatchdogDecision`,
  `IssueRunLedger.tsx:31-40` — Langlebigkeits-Regelung für hängende Runs).

## 3) Approvals / Approval-Flow

- Typ-System im Payload: `ApprovalPayload.tsx` exportiert `typeIcon`,
  `defaultTypeIcon`, `typeLabel`, `approvalSubject`, `ApprovalPayloadRenderer`
  (`ApprovalCard.tsx:1-15`) — jede Approval-art (d. h. tool/budget/…) hat
  Icon+Label+Renderer für die Nutzlast.
- **Status-Icons mit Semantik**: approved=grün Check, rejected=rot X,
  revision_requested=amber Clock, pending=yellow Clock
  (`ApprovalCard.tsx:11-16`).
- Card-Layout (`ApprovalCard.tsx:31-60`): Pill-Icon-Container →
  Kind-Badge (outline, „uppercase tracking-[0.08em] text-[11px]") →
  „Requested by <Identity>" → Subject-H3 → Payload-Renderer → Footer mit
  Auflösungs-Buttons **nur wenn pending oder revision_requested**
  (`showResolutionButtons`, `:28-30`; budget_override_required ist
  nie auflösbar direkt).
- Pages: `pages/Approvals.tsx` nutzt Mutations `approvalsApi.approve/reject`;
  Erfolg → `navigate(/approvals/{id}?resolved=approved)` + Filter-Reset,
  Fehler → setActionError (`Approvals.tsx:44-63`); Filter-Zweig
  „statusFilter === 'all' || pending || revision_requested" (`:69-74`); Cards
  bekommen `isPending`/`pendingAction` für disabled-State (`:122-125`).
- Dediziertes `ApprovalDetail.tsx` für Einzel-/Payload-View + Audit-Kontext.

## 4) Chat-Direktmodell (Nutzer rechts, Agents links) — VERIFIZIERT

- In `IssueChatThread.tsx`: menschliche Messages nutzen
  `resolveIssueChatHumanAuthor` → `isCurrentUser`; Layout:
  `items-end`+`justify-end` für den eigenen, `justify-start` für andere
  Menschen (`IssueChatThread.tsx:1188-1205`, Bubble
  „rounded-2xl px-4 py-2.5 bg-muted", max-w-[85%], `:1204,1214-1220`).
- Agent-Antwort rendert nicht als Bubble gegenüber, sondern als
  **Execution-Abschnitte**: „IssueChatChainOfThought" — Reasoning-Gruppen
  (`group.parts`) zwischen den Messages (`IssueChatThread.tsx:1150-1156`).
- Queue-States als Inline-Status in der eigenen Bubble: Badge
  `queueBadgeLabel` = „Queued"/„⏸ Deferred wake", Buttons **Interrupt**
  (roter Outline) und **Cancel** (amber Outline) in der Bubble Kopfzeile
  (`:1183,1222-1248`); „Sending..."-Zustandszeile statt Aktionen bei pending
  (`:1255-1258`); Hover-Reveal der Aktionszeile
  („opacity-0 … group-hover:opacity-100", `:1261-1264`).
- Identitäten: Avatar-fallback Initialen (`initialsForName`),
  Autor-Name „text-sm font-medium" über der Bubble (`:1197-1206`).
- Run-Kontext im Chat: `RunChatSurface.tsx` (Live-Run-Chat-Fläche) und
  `agentName: activeRun.agentName` im Thread-Kontext (`IssueChatThread.tsx:3120`).

## 5) Density-Prinzip (konkret)

- EntityRow: `py-2 text-sm` + Hairline border-b (oben); Identifier mono
  klein; trailing rechtsbündig klein.
- Chat: Message-Max-Breite 85 % (`max-w-[85%]`, `:1204`), Bubble-Roundness
  2xl vs. Badge 10px-Tracking — Klarheit über Deko.
- Storybook-Framing „deliberately dense … scan quickly" (siehe §1).
- EmptyState als Standard-Baustein: Icon-Kasten (bg-muted/50) + Message +
  primäre Action (`EmptyState.tsx:9-27`).
- Shortcuts minimal und konsequent (`hooks/useKeyboardShortcuts.ts:39-76`):
  `/` Fokus-Suche, `?` Shortcuts-Cheatsheet, `c` create, `[`/`]` Back/Forward.

## 6) State / Realtime

- TanStack Query + **LiveUpdatesProvider** (WebSocket, eigener
  `LiveUpdatesSocketLike`-Adapter `context/LiveUpdatesProvider.tsx:16-22`).
- Event-Switch über **typisierte Event-Strings**:
  `heartbeat.run.log`, `heartbeat.run.queued`, `heartbeat.run.status`,
  `heartbeat.run.event`, `agent.status`, `activity.logged`
  (`LiveUpdatesProvider.tsx:791-830`). Jeder Typ patcht gezielt Query-Keys
  (`queryKeys.issues.*`), inkl. Optimistic-Helfer
  `upsertIssueCommentInPages`, `clearIssueExecutionRun`, `removeLiveRunById`
  (imports `:3-5`).
- Toasts mit Anti-Flut: `TOAST_COOLDOWN_WINDOW_MS = 10_000`,
  `TOAST_COOLDOWN_MAX = 3`, `RECONNECT_SUPPRESS_MS = 2000`
  (`LiveUpdatesProvider.tsx:20-22`) — Toast-Koaleszenz nach Reconnect.
- Query-Refresh-Hygiene: `keepPreviousDataForSameQueryTail` („query-placeholder-
  data", imports `IssueRunLedger.tsx:8`) — „älteres Listing schlägt nicht
  neuere Namen" (unser Muster: beim WS-Patch Felder behalten, wenn Event alt).
- Breadcumbs als Context-Provider über der App: `Breadcrumb[] = {label,href?}`
  + `document.title` aus umgekehrter Breadcrumb-Kette („Parts · Paperclip",
  `context/BreadcrumbContext.tsx:8-31`) + `mobileToolbar`-Slot.
- Pages/Routen (Auszug aus `pages/App.tsx` → Route-Liste `App.tsx:58-99`):
  dashboard(+/live), agents(all/active/paused/error/new/:id/runds), projects(/
  :id/…), approvals(/:id), issues, goals, routines, plugins, settings/*,
  org, invites/join — Struktur: Liste → Detail mit Tabs.

## 7) Nutzen für Agent Relay

1. **status-colors.ts kopiert als unsere Token-Wahrheit** — wir bauen
   eine einzige Badge/Status-Farben-Registry (JS-Objekt) für Raum-Status,
   Agent-Zustand, Run-Zustand, Projekt-Ampel.
2. Approval-Card als UI-Pattern für unsere Stream-Approval/Tool-Use-Gates
   (sobald Adapter sie melden): Pill-Icon+Kind-Badge+„Requested by"+Payload
   +Auflösungs-Buttons nur bei offenem Status.
3. Chat-Kontrakt: eigener Chip rechts (`bg-muted`, 85 %-Max-Breite), Agents
   links mit Context-of-Thought-Abschnitt, Hover-Aktionen.
4. Liveness-Tone-Mapping fürs Run-Archiv (advanced/plan_only/empty).
5. EntityRow als universal Listen-Zeile (wir: Room-Rows, Agent-Cards,
   Projekt-Tiles).

## Offene Punkte (ungeklärt)

- Kein kanonisches DESIGN.md im Repo (design-basis.md verweist fälschlich
  auf „Paperclip DESIGN.md") — UI-Sprache lebt im Storybook + stories-
  Fixtures, nicht in einem Design-Doc.
- „Chat-Direktmodell (Nutzer rechts, Agents links)" ist für IssueChatThread
  belegt; ob es für ALLE Surfaces (Run-Detail) genauso gilt, unklar —
  Run-Detail nutzt RunChatSurface mit eigenem Layout (ungeklärt bis nicht
  im Detail gelesen).
- Status-Sätze sind als Story-„Fixtures" definiert (AGENT_STATUSES etc.),
  vollständige Liste der Agent-Statuses nicht extrahiert.