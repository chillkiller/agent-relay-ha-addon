TL;DR: Composer trägt permanent den Accent-Rahmen (#ff5c5c) und wirkt wie ein Fehlerzustand — er stiehlt den Fokus des gesamten 1440×900-Canvas.

| # | Stelle [Pixel/Element] | Was falsch (gegen DNA) | Korrektur (exakte Regel) | Erwartungssatz |
|---|---|---|---|---|
| 1 | Composer-Textarea [X:355–1095, Y:853–885, `textarea` in `.composer`] | Rand durchgehend `1px solid var(--accent) #ff5c5c` + Tint-Glow, auch ohne Fokus. DNA §4 claw-dark: Accent nur für Focus-Ring/CTA — unfokussiert gilt `border: 1px solid var(--line) #1e2028`. Wirkt wie Validierungsfehler, zieht Blick vom Chatinhalt ab. | `styles.css .composer textarea { border-color: var(--line); }` — nur `:focus { border-color: var(--accent); box-shadow: 0 0 0 1px var(--accent-tint); }` belassen. | Nach dem Fix sollte der Composer im Ruhezustand als ruhige Hairline erscheinen und erst beim Fokussieren (Klick/Tap) in Accent-Rot aufleuchten. |
| 2 | Agent-Message-Cards [X:355–1065, Y:180–545, `.msg.agent .content` ×3] | Karten-BG `var(--s2) #191c24` auf Canvas `#0e1015` — Kontrast ~1.07:1, kaum von Fläche unterscheidbar. Textfarbe `var(--ink-2) #bcbcc0` darin wirkt ausgewaschen, Slack-Dichte-Prinzip (klare Informationshierarchie) verletzt. | `.msg .content { border: 1px solid var(--line); }` und optional `background: var(--s1) #13151b` für einen Schritt mehr Tiefe; Agent-Chips im Header (Marvin/Codex/Copilot) behalten Swatch-Farbe. | Nach dem Fix sollte jede Agent-Nachricht als eigenständige Karte mit Hairline lesbar sein, ohne dass der Chatfluss in eine dunkle Fläche verschwimmt. |
| 3 | Sidebar-Raumname [X:8–212, Y:325–355, `.room-item.active .marquee`] | Statisch geklippt zu „Agent Relay — Review-Boarc" — letztes Zeichen abgeschnitten bei ~X:208, kein Ellipsis/Fade-Affordance. Marquee-Fade (`--marquee-fade: 12px`) greift nur on-hover, im Ruhezustand fehlt Lesbarkeits-Signal. DNA Sidebar-Formel verlangt Fade-Masks am Scroll-Rand auch statisch. | `.marquee--overflowing { mask-image: linear-gradient(to right, black calc(100% - 12px), transparent); }` bereits im Ruhezustand anwenden; Hover behält Scroll-Mechanik (`transform: translateX(var(--marquee-shift))`). | Nach dem Fix sollte der lange Raumname im Ruhezustand mit 12px-Fade auslaufen statt hart abzuschneiden und beim Hovern die Marquee-Scroll-Animation auslösen. |

*Hinweis:* Großer Leerraum [Y:545–835, ~290px] zwischen letztem Agent-Msg und Composer — bei nur 4 Nachrichten wirkt Canvas leer; aus Budget gestrichen (Top-3-Priorität), aber für Runde 6 zu prüfen (z. B. Empty-State-Hinweis oder dichtere `gap`-Staffelung).

---
**Validierungs-Checkliste:**
- [x] Screenshot mit `view_image` geöffnet (Beweis: Composer bei Y:853–885, Raumname „Review-Boarc" bei X:8–212 zitiert)
- [x] Kein Befund ohne exakte Stellenangabe (Pixel/Element-Name)
- [x] Alle 3 Fixes tragen Erwartungssatz („Nach dem Fix sollte…")
- [x] Berichtslänge ≤ 400 Wörter
- [x] Anti-Patterns-Tabelle: null Treffer (kein „wirkt unruhig" ohne Pixelmaß)
- [x] Keine Commits, keine Writes außerhalb des Berichts
