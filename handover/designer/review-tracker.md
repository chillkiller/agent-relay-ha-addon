# Review-Tracker — Vision-Loop (Designer × Guide)

Stand: 2026-10-18, 19:25 · Verwaltet von Guide (Amts-Memory: ../guide/memory/)

## Runden-Sequenz

| Runde | Modus | Modell | Screenshot | Bericht | Befunde | Status |
|---|---|---|---|---|---|---|
| 1 | blind (kein Bild: /tmp außerhalb Sandbox) | qwen3.5:9b | — | round-1.md | 3 (2 aus Kontext geraten) | teilweise widerlegt |
| 2 | echtes Sehen | qwen3.5:9b | shots/round-2-current.png | round-2.md | 2 belegt (Input-Fokus-Rot, Agent-Leiste), ohne Fix-Wert | geliefert |
| 3 | echtes Sehen | gemma4:31b-cloud | shots/round-2-current.png (wiederverwendet) | round-3.md | 1 pixel-belegt: **Tab-Kontrast** [X:264–1920/Y:56–112] | geliefert |
| 3-fix | Bau (Coordinator) | — | — | — | Fix: inaktiv --ink-4, aktiv --ink-2 + Hover | umgesetzt → `5a52e90` |
| 4 | VERIFY | gemma4:31b-cloud | shots/round-4-after-fix.png | round-4-verify.md | Erwartungssatz bestätigt | **bestätigt ✅** |
| 5 | MUSE-DEBÜT (echter Pixel-Befundkatalog) | muse-spark-open:cloud | shots/round-5-before.png | round-5-muse-debut.md | 3: B1 Composer-Permanent-Rahmen · B2 Karten-Kontrast 1.07:1 · B3 Marquee harter Clip | geliefert, Checkliste sauber abgehakt |
| 5-fix | Bau | — | — | — | B1: kein Autofokus beim Raumwechsel · B2: Hairline+s1 · B3: redundant mask | umgesetzt → `6c05315` |
| 5-verify | VERIFY | muse-spark-open:cloud | shots/round-5-after-fixes.png | round-5-verify.md | **1/3**: B1 bestätigt · B2 „nicht belegbar" · B3 „nicht belegbar" | geliefert |
| coordinator-view | Eigene Sicht (view_image) | (ich selbst) | shots/coordinator-verify-look.png | — | **B2 WIDERLEGT** (Hairline klar sichtbar) · B3 bestätigt (harter Clip) | dokumentiert |
| 6 | FINAL VERIFY | muse-spark-open:cloud | shots/round-6-after-b3fix.png | round-6-verify.md | **1/2**: B3 bleibt hart · B2 depriorisiert | geliefert |
| 6-fix-a | Bau | — | — | — | B3 root-cause: Messung vor Font-Load → fix: `document.fonts.ready` | umgesetzt, griff nicht → `15337f0` |
| 7 | Eigene Sicht | (ich selbst) | shots/round-7-after-b3redundanz.png | — | B3: Klasse nie im DOM (Messung/Prefix reichten nicht) | dokumentiert |
| 6-fix-b | Bau (pragmatisch) | — | — | — | **Ellipsis statt Marquee** (Control-UI-canonical, kein JS-Timing), toter Marquee-JS entfernt | umgesetzt → `70650f7` |
| 8 | Eigene Sicht (final) | (ich selbst) | shots/round-8-after-ellipsis.png | — | „Agent Relay — Review-Boa…" — **weicher Abschnitt bewiesen** | **B3 geschlossen ✅** |
| 9 | du-Drag-Bau (GaRoN-Order) | — | shots/round-9-du-drag.png | — | human_member Backend+SPA, AUSGELADEN-Gruppe | umgesetzt → `51c8807`, Live-Proof False→True |

## Alle Befunde geschlossen (Stand 19:25)

| # | Befund | Auflösung |
|---|---|---|
| B-R3 | Tab-Kontrast | `5a52e90`, Verify Runde 4 bestätigt |
| B-R5-1 | Composer-Permanent-Rahmen (Autofokus) | `6c05315`, Verify bestätigt |
| B-R5-2 | Agent-Karten ohne Absetzung | `6c05315` (Muse-„nicht belegbar" durch eigene Sicht widerlegt) |
| B-R5-3 | Marquee harter Clip | `15337f0` + `70650f7` (Ellipsis), eigene Sicht auf round-8 bewies „…" |

## Modell-Historie (Guide-Befund 1: Pinning)

| Zeitraum | Modell | Anlass |
|---|---|---|
| Runde 1–2 | qwen3.5:9b | Erstausstattung (GaRoN: „gewinnt keine Blumen") |
| Runde 3–4 | gemma4:31b-cloud | Umtausch |
| ab Runde 5 | **muse-spark-open:cloud** | GaRoN-Bestellung: neuestes Vision-Modell (Katalog-Import $1.25/$0.15/$4.25, backup + Live-Check n. Skill), gemma4 als Fallback |

## Runde 7 — Rebuild (Sidebar = Control-UI-Gesicht) [GELIEFERT + VERIFIZIERT]

| Teil | Inhalt | Status |
|---|---|---|
| A | 3 pixel-belegte Befunde (A1 Gruppen-Toggles fehlen, A2 Team ohne Facepile, A3 DnD ohne Reihenfolge) | geliefert |
| B | 4 Deltas mit File/Zeile: B1 Toggles · B2 Facepile+Presence · B3 Drop-Zonen+Reorder · B4 Identity-Footer | geliefert |
| Bau | Alle 4 umgesetzt (Backend position+ALTER+ReorderRooms+POST /api/rooms/reorder; SPA toggles/avatars/dropzones/identity) | **`e426293`** |
| Verify (Designer) | 3/4: B2/B4 pixel-treu; B1-Chevron zu subtil → Fix built; B3 code-belegt (Live-Bedienung entscheidet) | **geschlossen** |

---

| 10 | Dichte-Verify + B1-Finish + Board-Shot + 3 Befunde | muse-spark-open:cloud | shots/round-9-after-density.png + shots/round-9-board.png | 2026-10-19-round-10.md | 3: B-R10-1 Board-Tab hidden · B-R10-2 Composer-Footprint schwer · B-R10-3 Header-Ellipsis aggressiv (B1 3/3 ✅, Dichte 8px uniform visuell dicht) | geliefert |
| 11 | model-v2 Frischer Shot + 3 Befunde (typisch-KI Surface-Sprache) | muse-spark-open:cloud | shots/round-11-modelv2.png (75K, RÄUME 7+TEAM 7+🧑‍💻 Codex) + shots/round-11-modelv2-dropdown.png (81K, Identity/Einstellungen) | 2026-10-19-round-11-muse.md | 3: B-R11-1 Identity-Card generische Pillen-Ästhetik · B-R11-2 Nav-Gruppen-Sprache gleichförmig (RÄUME=TEAM) · B-R11-3 Composition-Ruhe leer (Main ohne Raster, Composer schwebt, Header-Ellipsis) — alle typisch-KI wegen fehlender Materialität/Hierarchie/Autorität + JS-Crash hotfixed | geliefert |

## Nächste Runden-Themen (Runde 11+)

- Muse-Blick auf du-Drag-Ergebnis (round-9-Shot) + AUSGELADEN-Gruppe
- Composer-Proportionen / Leerraum-Dichte (aus Runde 5 gestrichen, vom Designer für Runde 6+ vorgemerkt)
- Sidebar-Fade-Masken (claw-Muster) am Scroll-Rand
- Agent-Avatar-Zustände (offline = Opacity .55 wie Control-UI)