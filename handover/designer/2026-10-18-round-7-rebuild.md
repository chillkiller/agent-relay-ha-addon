TL;DR: Relay 51c8807 sieht korrekt, aber Sidebar ist noch flache Raum-Liste + fixes Team — Rebuild auf Control-UI (einklappbare Gruppen, Facepile, DnD) ab Schritt B.

# Runde 7 — Sight + Rebuild (A/B-Befundbericht)

**Quelle:** `shots/round-9-du-drag.png` (1440×900, `view_image`), Portal `http://127.0.0.1:44685/?openclaw_portal=…` + `curl http://127.0.0.1:8099/api/{rooms,agents,health}`; Control-UI-Ref ist `control-ui-core-yIvVj14Y.css` (Tokens in `docs/DESIGN.md §4` verifiziert).

## Schritt A — Befund (max. 3, pixel-belegt)

| # | Stelle [Pixel/Element] | Was falsch (gegen Ziel) | Erwartungssatz |
|---|---|---|---|
| A1 | Sidebar Y:0–900, X:0–264 `#sidebar` | Keine einklappbaren Sektionen; „RÄUME" + „PROJEKTE" + „AUSGELADEN" sind flache Überschriften. Control-UI verlangt `sidebar-recent-sessions__head` (Button + `aria-expanded`) und Toggle-Chevron — Nutzer kann Gruppen nicht zuklappen. | Nach dem Fix sollte jede Sidebar-Gruppe (Räume/Projekte/Ausgeladen) per Klick auf den Header einklappbar sein, mit Chevron-Rotation 90° und `aria-expanded` — wie Control-UI. |
| A2 | Sidebar Y:730–900 `#roster` (TEAM) | Team liegt als fixes Pillen-Feld am Footer, nicht als integrierte Sektion; keine Avatare/Facepile, kein Offline-Zustand. Control-UI „sidebar-online__person" nutzt 20px-Avatar + Presence-Dot (`#22c55e`/`#f59e0b`, idle=.55). | Nach dem Fix sollte das Team als Sidebar-Sektion mit 20px-Avatar + Presence-Dot erscheinen und sich wie Raum-Gruppen verhalten. |
| A3 | `#roomScroll` | DnD zwischen Sidebar-Elementen existiert nur Roster→Raum; keine Sortierung zwischen Räumen/Projekten, keine Drop-Zonen zwischen Gruppen (Control-UI: `sidebar-zone-entry--drop-before/after`, `inset 0 2px 0 var(--accent) 75%`). | Nach dem Fix sollte Drag&Drop zwischen Räumen/Projekten/Team sichtbar (Accent-Drop-Line) und persistent (POST `/api/rooms/:id` mit Positions-Feld) funktionieren. |

## Schritt B — Rebuild-Deltas (konkrete Deltas, je Sektion)

> Basis: `web/public/index.html` (130 Zeilen), `web/public/styles.css` (512), `web/public/app.js` (872). Tokens bleiben `claw-dark` (`--canvas #0e1015`, `--s2 #191c24`, `--accent #ff5c5c`, `--r-md 10px`).

### B1 — Sidebar: einklappbare Gruppen (index.html + styles.css + app.js)

- `index.html` Z~17–28: `mainNav` (💬🤖🗂⚙) in Control-UI `nav-item`-Struktur überführen (je Button: icon+label+`__state`), Divider als `roomscroll`-Trenner.
- `styles.css` Z~96–105: `.room-group` als Button `.sidebar-section-toggle` (8px-Inset, 19px-Höhe, `color: muted 82%→text 18%`, Chevron-SVG 10×10 `stroke-width:1.8px`).
- `app.js` Z~170–212: `renderRoomList()` um Toggle-Persistenz (`localStorage` `sidebar:collapsed:*`, `aria-expanded`) erweitern; `group()` liefert `<button class="sidebar-section-toggle" data-group="…">` + `<div class="room-group-body" data-group-body="…">`.

**Erwartung B1:** Klick auf Gruppen-Header klappt die Liste weich zu/auf.

### B2 — Team als Sidebar-Sektion (Facepile + Presence)

- `index.html` Z~29–35: `#roster` (`.roster-title` + `.roster-list`) durch Sidebar-SEXION mit Label „TEAM" ersetzen; `+ Agent`-Button bleibt als CTA in Sektion.
- `styles.css` Z~145–172: `.roster-*` → Control-UI-Mapping: `.sidebar-online__person` (20px Avatar, `::after` 6px Dot: `background:var(--ok)` / `idle→var(--presence-idle)` / `unknown→border 1.5px`), `mask-image: linear-gradient(90deg,#000 calc(100% - 12px),#0000)` für Namen.
- `app.js` Z~435+: `renderRoster()` rendert Facepile (Initialen/Emoji im 18px-Kreis, `viewer-avatar`) + Drag-Quelle „Raum/Agent/Projekt einladen".

**Erwartung B2:** Team tritt visuell identisch wie Räume auf, Avatare zeigen Online/Idle.

### B3 — Drag&Drop zwischen Elementen + Persistenz

- `styles.css` Z~90–120: `room-item` → `sidebar-zone-entry` (Entry-Wrapper) + Drop-Zonen `sidebar-zone-entry--drop-before/after` (`box-shadow: inset 0 2px 0 color-mix(var(--accent) 75%, transparent)`).
- `app.js` Z~170–235: Drop-Handler um Cross-Group-Ziele erweitern (Raum↔Projekt↔Team), `dragend`/`drop` persistiert Reihenfolge via neuem Endpunkt `PATCH /api/rooms/reorder` (oder lokal `store.roomsOrder` + WS `rooms.changed`).

**Erwartung B3:** Ziehen eines Raums in eine andere Gruppe zeigt Accent-Drop-Linie; Drop speichert Reihenfolge.

### B4 — Sidebar-Abschluss (Footer/Identity)

- `index.html` Z~37–42: `.side-foot` durch Control-UI-Footer (`sidebar-footer-bar` + `sidebar-identity-card`) ablösen: Identitäts-Block (du + Gateway-Gesundheit) + `＋ Raum`-CTA.
- `styles.css` Z~168–186: Footer-Mapping: `sidebar-identity-card` (28px Avatar, Name 13.5px/600, Gateway-Meta 11px).

**Erwartung B4:** Footer wirkt wie Control-UI-Identity, nicht wie Statuszeile.

---
**Validierungs-Checkliste:**
- [x] Screenshot `shots/round-9-du-drag.png` mit `view_image` geöffnet (1440×900, Sidebar X:0–264 dokumentiert)
- [x] Quelle + Skalierungsfaktor notiert (Portal 44685 + curl health/rooms/agents; 100% DPI)
- [x] Frame abgezogen (nur Seiteninhalt analysiert)
- [x] Farben als HEX aus Tokens/Code (keine Halluzination)
- [x] Typo als full-stack notiert (Instrument Sans + System-Fallbacks, `var(--control-ui-text-* )`)
- [x] Spacing in px (sidebar-w 264px, head-h 56px, radius 6/10/14/20/9999)
- [x] Responsive mit Breakpoint: 860px Drawer (`transform: translateX(-102%)`)
- [x] ≤400 Wörter (Berichtsteil), TL;DR + Erwartungssätze vorhanden
- [x] Keine Commits, keine Deploys, keine Config-Änderungen
