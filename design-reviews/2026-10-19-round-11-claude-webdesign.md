# Round 11 — Web-Design-Bau (Composer mittig, User-Menu, Feel-Zug 1)

**Zug:** Claude-Webdesign (Frontend solo) · **Basis:** HEAD e2cc4bc (development) · **Datum:** 2026-10-19
**Verifikation:** Headless-Edge-Shots 1440×900 (localhost:8099, S1), Pixelrand-Messung der Spaltenkanten

## TL;DR

Alle drei Bau-Ziele umgesetzt und im Shot verifiziert: Composer zentriert passgenau unter der Nachrichten-Spalte, Identity-Card als A11y-Anker mit nach oben aufklappendem User-Menü, und die OpenClaw-DNA-Feel-Details (Typing-Dots, Hover-Lift, Tabular-Nums, Disabled-Dim, sanfte Transitions) sind drin. **Bonus: Boot-Blocker der Zugs-Kollegen entdeckt und entschärft** (`#searchInput`-Null-Crash legte die komplette App lahm) plus ein echtes UX-Bugfix (`switchView('agents')` renderte nie die Cards). Build: `go build` aus Repo-Stand, 8099 neu bestückt.

## Was stand ( Ist e2cc4bc / 04:54-GaRoN-Befunde)

| # | Befund | Ist-Zustand |
|---|--------|-------------|
| 1 | Composer rechtsbündig (R8-Artifact) | `margin: auto var(--sp-3) … auto`, `width: calc(100% - var(--sp-6) - 12px)`; `.messages` max-width 980px — Spaltenkante ≠ Composerkante |
| 2 | User-Dropdown fehlt als echter Anker | identityCard/button + `#userMenu` teilweise vorbereitet, aber nur Einstellungen+newAgent (data-view gemischt), Outside-Close via `click`, New-Room löste Modal-Doppelöffnung aus |
| 3 | „Bot-mäßig": blinder Cursor `▍`, kein Lift, springende Zahlen, keine Disabled-Dim, Transitions nur auf border-color | — |

## Was steht (nachher)

**1. Composer mittig** — `styles.css`
- `.composer`: `margin: 0 auto var(--sp-3)`, `width: calc(100% - var(--sp-6))` (R8-Artifact entfernt)
- `.messages`: `max-width: 948px; width: calc(100% - var(--sp-6))`, horizontale Ausrückung über width (padding 0) → Kanten fluchten exakt mit der Composerkarte
- Mobil: `.messages` width 100% (nur padding rückt aus — keine Doppel-Ausrückung)
- **Messung:** Composer-Zentrumsachse 852px = Main-Spaltenzentrum 852px; Human-Bubble-Kante 1321 ↔ Composerbox 1326 (Flucht)

**2. User-Menu unten-links verankert** — `index.html` + `app.js` + `styles.css`
- Menü: Einstellungen / Agents / Projekte (jetzt einheitlich `data-user-act`), Sep, Neuer Raum / Agent anlegen
- JS: Toggle bei Card-Click, `data-user-act`-Dispatcher (Views via `switchView`, newRoom/newAgent), Close bei Escape + **outside-mousedown** (war click), Modal-Doppelöffnung gefixt (`openNewRoom()` direkt)
- A11y: `role=menu`/`menuitem`, `aria-haspopup="menu"`, `aria-expanded`-Umschaltung, Enter/Space nativ (Button)
- CSS: `position:absolute; left:var(--sp-2); bottom:calc(100% + 6px)` an `.side-foot` (relative) — öffnet nach oben, Chevron dreht sich (aria-States)
- **Shot:** Menü offen über Roster, klar geschichtet

**3. Feel-Zug 1** — Muster aus `/tmp/oc-src/ui/src/styles/` (Usage/Control-UI), kein Framework-Klon
- Typing-Indikator: drei Punkte (`•••`) statt Cursor-Blind — erscheinen **sequenziell** (Keyframe-Halte statt `steps()`, damit Punkt 3 im Loop wirklich sichtbar ist — erstes `steps(3,end)`-Muster verschluckte ihn), 1.2s Loop, accent-hover
- Hover-LIFT auf `.card`/`.proj-tile`: `translateY(-1px)` + `--shadow-sm` (Pattern: usage.css Zeile 1182)
- `font-variant-numeric: tabular-nums` auf `.msg .time` + `.tab-badge` (Zahlen springen nicht)
- Letter-Spacing -0.2px auf `.card .name`, `.tile-name`, `.p-head h3` (h2 waren schon da)
- Disabled-Dim auf Agent-Cards: `opacity .55` für Emoji+Name via `.card.is-disabled` (Control-UI `--agent-avatar-opacity`-Muster)
- Konsolidierte sanfte `.12s ease`-Transitions auf allen interaktiven Flächen (btn, icon-btn, nav-btn, identity, chip, menu/popup/tabs/actions)

## Bonus-Befunde (Koordination in der Loop-Crew)

- **Boot-Blocker entschärft:** Der Feel/React-Zug schrieb `wireSearch()` mit `#searchInput` schneller ins app.js als die zugehörige index.html — `addEventListener` auf `null` killte die **ganze Boot-Kette** (kein Raum, kein Roster mehr). Null-Guard gesetzt (`if (!inp) return;` mit Verweis auf den HTML-Zug) — App bootet für alle Züge wieder.
- **UX-Bugfix:** `switchView("agents")` renderte die Cards **nie** (`renderAgents` feuerte nur, falls `loadAgents()` zufällig im aktiven View lief). Fix in `switchView` — Agents-View zeigt jetzt Cards (Shot belegt).
- **Ride-alongs im Commit** (Zugs-Kollegen, web/public): Task-Edit-Modal, Tile-Preview (offene Tasks auf Projekt-Tiles), Message-Reaktionen (rx-chips, +) samt `wireSearch`-Popover. Web-JS referenziert `POST /api/reactions/batch` — der zugehörige Go-Code steckt noch **unkommitted** in `internal/api`+`internal/store` (läuft in meinem Build, 200). → Reihenfolge-Kennzeichen für den Coordinator: Der React-Zug muss `internal/*` committen.
- **Server-Reload-Gotcha:** `go:embed` — Assets sitzen im Binary; CSS-Edit ohne Rebuild wird ignoriert. Kanonisches Binary ist `dist/agentrelay-amd64` (Rebuild + 8099-Neustart dokumentiert im Build-Log-Tail).

## Checkliste Round 11

- [x] 1 Composer mittig (`margin 0 auto`, beide Container 948px, width-Formeln angleichen)
- [x] 1a `.messages`-Ausrichtung angleichen, Kanten-Flucht gemessen
- [x] 1b Mobile-Doppel-Ausrückung vermieden
- [x] 2 Identity-Card: click-Toggle, aria-expanded, Enter/Space nativ
- [x] 2a userMenu opens NACH OBEN an `.side-foot` (absolute, left sp-2, bottom 100%+6px)
- [x] 2b Settings/Agents/Projects als data-user-act (+ newRoom/newAgent)
- [x] 2c Close-on-outside-**mousedown** + Escape
- [x] 2d Modal-Doppelöffnung behoben
- [x] 3 Typing-Dots (3 Punkte, sequenziell) — im Shot sichtbar
- [x] 3a Hover-Lift card/proj-tile — CSS, Mechanik wie usage.css
- [x] 3b tabular-nums auf .time/.tab-badge
- [x] 3c Letter-Spacing -0.2px auf Karten-/Tile-Namen + Board-h3
- [x] 3d Disabled-Avatar .55 — Demo im Shot Marvin-Card
- [x] 3e Sanfte .12s-transitions konsolidiert
- [x] `node --check app.js` OK · `go build` OK (Repo + Worktree-Isolat)
- [x] S1-Shots: siehe unten

## Shot-Referenz (designer-workspace `designer/shots/`)

| Shot | Aussage |
|------|---------|
| `round-11-before-composer.png` | Ist: Composer rechtsbündig (05:00, alter Bestand) |
| `round-11-after-composer.png` | Ziel 1: Composer zentriert, voller Chat |
| `round-11-user-menu.png` | Ziel 2: Menü öffnet nach oben, Settings/Agents/Projects |
| `round-11-stream-dots.png` | Ziel 3: drei rote Punkte am letzten Agent-Msg |
| `round-11-agent-dim.png` | Ziel 3: Marvin-Card gedimmt (.55) — Demo-Regel im Shots-Binary, echte Trigger-Klasse im Code geprüft |

Kopie (2 Kern-Shots) auch unter repo `shots/round-11-after-composer.png`, `shots/round-11-user-menu.png`.

## Offen / Nächster Zug

- Hover-Lift ist nur per CSS-Code belegt (Lift-Interaktionen in Static-Headless-Shots nicht fotografierbar) — im Reviewer-Loop einmal live ansurfen.
- Der React-Zug committet `internal/api`+`internal/store` (Reactions-API), sonst 404 im HEAD-Stand des Web-JS.
- `#searchInput` kommt mit dem HTML-Zug; der Guard kann dann bleiben oder fallen.