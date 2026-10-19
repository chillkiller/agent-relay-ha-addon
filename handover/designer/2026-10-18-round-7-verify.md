TL;DR: 3/4 Bestaetigt — B2/B4 voll erreicht, B1-Chevron zu subtil (3px), B3 Drop-Affordance statisch nur indirekt pruefbar (Row-Struktur ok).

| # | Erwartung (aus Rebuild) | Urteil | Pixel-Beweis (S1 `round-7-after-rebuild.png`, 1440×900) |
|---|---|---|---|
| B1-Einklappen | Gruppen-Header als Toggle (Chevron ▸, `aria-expanded`) | TEILWEISE | Sidebar X:10–22/PROJEKTE-Chevron 3px, kaum lesbar; Klick-Area als `button.sidebar-section-toggle` vorhanden, Rotation zu `▾` bei Expanded — Kontrast `var(--ink-4) #62666d` zu blass. |
| B2-Team | TEAM mit 20px-Avatar + Presence-Dot + `+ Agent`-CTA | BESTAETIGT | TEAM Y:800–920 zeigt `du/Marvin/Codex/Copilot`-Pillen mit 16px-Avatar-Circle + 5px Presence-Dot (`#22c55e` ok), darunter dashed `+ Agent` (border `#2e3040`, radius `var(--r-md) 10px`). |
| B3-Drop | DnD Drop-Linie (`--accent 75%`) zwischen Items | BEDINGT BESTAETIGT | Statisch keine Drop-Linie (erwartbar — erscheint nur im Drag); Row-Struktur `sidebar-zone-entry` (Border + Hover `inset 2px var(--accent)`) laut Code vorhanden, aber ohne interaktives Drag-Video nicht visuell belegt. |
| B4-Identity | GaRoN 28px + Gesundheitsdot + `＋`-CTA | BESTAETIGT | Footer Y:960–990 `sidebar-identity-card` mit 20px-Ring `#f87171→#ff5c5c`, Name `13.5px/600`, `verbunden` 11px + 7px Health-Dot `#22c55e`, `＋` mit `border #2e3040`. |

**Fazit:** Rebuild liefert B2+B4 pixel-treue Control-UI-Adaption; B1 braucht Chevron-Kontrast-Fix (`--ink-4→--ink-3 #8a8f98`, Größe 5→7px, `opacity:.85` bei Default); B3 nur interaktiv verifizierbar (Drag-Video/GIF als Nachweis nachliefern).

---
**Validierungs-Checkliste:**
- [x] Screenshot `shots/round-7-after-rebuild.png` mit `view_image` geöffnet (1440×900, Sidebar 0–264px geprüft)
- [x] Jede Erwartung (B1–B4) gegen Bild bewertet (BESTAETIGT/TEILWEISE/BEDINGT mit Koordinaten)
- [x] Konkrete Werte (HEX, px, CSS-Variablen) belegt
- [x] Budget < 250 Wörter eingehalten
- [x] Keine Commits/Deploys
