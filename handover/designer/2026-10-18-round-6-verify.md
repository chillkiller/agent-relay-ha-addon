TL;DR: B3-Fix 15337f0 im After-Shot nicht sichtbar — harter Clip statt 12px-Fade; B2 im Chat-Shot nicht pruefbar, gilt per Briefing als umgesetzt.

| # | Erwartungssatz | Urteil | Pixel-Beweis (S1 `round-6-after-b3fix.png`) |
|---|---|---|---|
| B3-Sidebar | Langer Raumname faedet 12px weich | NICHT BEHOBEN | Sidebar [X:8–212, Y:310] „Agent Relay — Review-Boar" hart geklippt, kein Gradient/Mask sichtbar. |
| B3-Header | Titel faedet 12px (affordance) | NICHT BEHOBEN | Header [X:220–1030] endet „…Marquee-Test)" scharf; keine Aufhellung/Transparenz am rechten Rand. |
| B2 | Agent-Karten Hairline + s1 | NICHT PRUEFBAR | After-Shot zeigt Chat-Tab (Marvin/Codex/Copilot); keine Board-Agent-Karten im Viewport → kein Beleg moeglich. Per Briefing als umgesetzt gewertet. |

**Fazit:** B3 bleibt offen (fonts.ready-Fix greift nicht oder falscher Selektor/Shot). B2 formal offen, faktisch per V2 depriorisiert.

---
**Validierungs-Checkliste:**
- [x] Screenshot `shots/round-6-after-b3fix.png` mit view_image geoeffnet
- [x] Erwartungssaetze gegen Bild geprueft (BESTAETIGT/NICHT BEHOBEN je Zeile)
- [x] Pixel-Beweis mit Koordinaten je Urteil
- [x] Bericht < 200 Woerter
- [x] Keine Commits/Deploys
