TL;DR: 1/3 bestaetigt — Composer ist ruhig; Agent-Karten und Marquee-Fade ohne pixel-Beweis im After-Screenshot.

| # | Erwartungssatz | Urteil | Pixel-Beweis (after-shot) |
|---|---|---|---|
| B1 | Composer als Hairline im Ruhezustand, nur on-focus Accent. | BESTAETIGT | Composer-Zeile [Y:900–970] zeigt `var(--s1)`-Flaeche mit feiner `--line`-Hairline; kein Accent-Rahmen `#ff5c5c` / Tint-Glow sichtbar — Ruhezustand wie gefordert. |
| B2 | Agent-Karten als eigenstaendige Karten mit Hairline + `var(--s1)` lesbar. | NICHT BEHOBEN | `.msg.agent .content` [X:340–1065, Y:200–620] erscheinen im 1440px-Screenshot ohne abgesetzte Hairline/BG-Stufe — wirken weiterhin flach auf Canvas `#0e1015`; Fix im Bild nicht belegbar (moeglw. CSS greift, aber in dieser Aufloesung kein Kontrastnachweis). |
| B3 | Marquee statisch mit 12px-Fade statt hartem Clip. | NICHT BEHOBEN | Header-Titel [X:220–1030] endet bei „Marquee-Test)" hart am Rand ohne Fade-Gradient; Sidebar-Raumname [X:8–212] zeigt nur geklipptes „Review-Boarc" ohne Mask. Affordance im Bild nicht sichtbar. |

### Nachbesserung

| # | WAS | WARUM falsch | KORREKTUR | ERWARTUNG |
|---|---|---|---|---|
| B2→ | `.msg.agent .content` [Y:200–620] | Kartenrand/BG im Screenshot nicht differenzierbar vom Canvas | `border: 1px solid var(--line)` + `background: var(--s1)` verifizieren; falls vorhanden Kontrast erhoehen (z. B. `var(--line)` auf `#262a36` oder Karte auf `#161922`) oder Shot in 2x-Aufloesung neu fotografieren | Nach dem Fix sollte jede Agent-Karte im 1440px-Screenshot mit duenner Linie klar von der Canvas-Flaeche abgesetzt sein |
| B3→ | `.marquee--overflowing` Header + Sidebar-Raumname | Statischer Clip ohne Fade — Klasse/Selektor greift nicht auf gerenderten Titel | `mask-image`/`-webkit-mask-image: linear-gradient(to right, #000 calc(100% - 12px), transparent)` auf den Titel-Wrapper (`.room-title`, `.room-item .label`) direkt anwenden; Selektor-Coverage pruefen | Nach dem Fix sollte der lange Titel rechts 12px weich ausfaden statt hart abzuschneiden |

---
**Validierungs-Checkliste:**
- [x] Screenshot mit `view_image` geoeffnet (Beweis: Composer Y:900–970, Header „Marquee-Test)" X:220–1030 zitiert)
- [x] Kein Befund ohne exakte Stellenangabe (Pixel/Element-Name)
- [x] Erwartungssaetze gegen Bild geprueft (BESTAETIGT/NICHT BEHOBEN)
- [x] Berichtslänge ≤ 250 Woerter
- [x] Anti-Patterns-Tabelle: null Treffer
- [x] Keine Commits, keine Writes ausserhalb des Berichts
