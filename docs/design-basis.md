# Design-Basis — Agent Relay v2

Stand: 2026-10-19. Quelle-Analyse der drei Referenzen für das Frontend.

## Methode

Jede Zeile = ein übernommenes UI-Pattern mit Quelle und konkreter Anwendung im Relay.

| PATTERN | QUELLE | ANWENDUNG IM RELAY |
|---------|--------|---------------------|
| **Canvas + Surface-Leiter ohne Shadow** | OpenClaw UI `base.css` + `app-sidebar.css` — `--bg`/`--bg-elevated`/`--bg-hover`/`--border` | `--canvas`/`--s1`–`--s4` + `--line` Tokens in `styles.css` (1:1 aus `control-ui-core-yIvVj14Y.css`) |
| **Radius 10px Basis, Pill 9999** | OpenClaw `base.css` `--radius-md:10px` | `--r-xs/sm/md/lg/xl/pill` — Composer, Cards, Chips |
| **Sidebar-Gruppen einklappbar + Fade-Masks** | OpenClaw `app-sidebar.css` — `.nav-group[aria-expanded]` + Mask-Gradients | `.nav-group` mit Toggle + Scroll-Fade am Sidebar-Rand |
| **Avatar-States (saturate/opacity offline)** | OpenClaw `agent-*.css` — `.idle` Filter | `.roster-chip.idle` / `.presence.idle` gedimmt |
| **Composer als Superellipse-Card** | OpenClaw `chat/*.css` — Composer mit `border-radius` XL + Shadow | `#composer` mit `--r-lg` + `box-shadow` + `mention-pop` |
| **Chat-Dichte: kompakte Zeilen, block-gap = Textgröße** | OpenClaw `chat` + `components.css` — `.message` Spacing | `.messages` mit 12px Gap, Grouped-Header (gleicher Autor = kein Re-Render) |
| **Ein chromatischer Akzent sparsam** | OpenClaw claw-theme `--accent #ff5c5c` | `--accent` nur für Brand, Focus-Ring, Primary-CTA, Dot |
| **Multi-Agent-Räume als Default** | buzz `ARCHITECTURE.md` — Room-Membership, Agents = eigene Keypairs + Members | `room.members[]` + Roster-Chips + Drag&Drop Invite |
| **Member-Inline-Aktionen (✕ am Chip)** | buzz `web/` — Member-Row mit Remove am Chip | `.member-chips .chip-x` (hover-reveal) bei Raum-Members |
| **@-Mention im Composer mit Suggestion-Pop** | buzz `PLANS/` — @-Autocomplete; auch Slack-Prinzip | `#mentionPop` + `mention-pop` — Filter über `agents`/`members` |
| **Presence-Dots an Room-Rows** | buzz — Online-State pro Member; Campsite Grid-Gedanke | `.nav-group-body .room-row .presence-dot` (grün/grau) |
| **Projekt-Gruppen (Räume ↔ Projekte)** | Paperclip `ui/src/features/connections` — Projekt als Container für Tasks/Runs | `.nav-groups[data-group=projects]` — Projekt-Räume bündeln Boards |
| **Board-Tiles (Kanban-Spalten + Task-Chips)** | Paperclip `board.css` / `board-document.css` — Columns + Status-Badges | `#inlineBoard .board-wrap .column .task` mit Status-Move/Delete |
| **Control-Plane-Sprache (what/need/do)** | Paperclip `DESIGN.md` — Header-Prinzip | `chat-head` + `room-tabs` (Chat/Board) — klare Orts-Anzeige |
| **Status-Badges als Pill (surface-2 + caption)** | Linear via `docs/DESIGN.md` §1 — `status-badge` | `.task.done` + `.presence` Pills — einheitliches Badge-System |

## Nicht übernommen

- Linear Canvas `#010102` — durch claw-dark `#0e1015` ersetzt (GaRoN-Entscheidung).
- buzz Nostr-Events/Redis — Relay bleibt SQLite/WS (HA-Add-on Scope).
- Paperclip Adapter-Pakete — Adapter bleibt `internal/adapter` (ein Vertrag).

## Konsequenz für v2-Finish

Offene Lücken gegen diese Basis: Board-Tiles, @-Mention-Chips im Composer, Presence-Dots. Siehe Round-9-Report.
