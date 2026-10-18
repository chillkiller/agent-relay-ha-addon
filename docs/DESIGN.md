# Design — Agent Relay (human-made Vorlagen)

Stand: 2026-10-18. Basis: Reverse-Engineering (REVERSE-ENGINEERING.md) +
Design-Recherche mit Quelle-Zitaten. Design-Sprache des Relays folgt drei
human-gemachten Referenzen:

## 1) Linear (linear.app) — Canvas + Tokens für Projekt-/Board-Oberfläche

Quelle: shadcn.io Design-Directory, „Linear raw" DESIGN.md (Google-Labs-Spec,
https://www.shadcn.io/design/linear/raw, last updated 2026-05-12, Autor
Dov Azencot / @dovazencot; Linear brand guidelines: linear.app/brand).

**Wir übernehmen die Struktur-Empfehlungen (nicht Farben blind, sondern als
Token-Rahmen):**

- Canvas: `#010102` („deepest dark, fainted blue tint — nie reines #000")
- 4-stufige Surface-Leiter OHNE Schatten: `#0f1011` → `#141516` → `#18191a` → `#191a1b`,
  Hairlines `#23252a`/`#34343a`/`#3e3e44`
- **Ein** chromatischer Akzent: `#5e6ad2` (Lavender) sparsam — Focus-Ring, brand,
  primary CTA; Hover `#828fff`, Focus-Tint `#5e69d1`
- Text: `#f7f8f8` (ink), `#d0d6e0` (muted), `#8a8f98` (subtle), `#62666d` (tertiary)
- Semantic success `#27a644`; sonst keine mehrfarbige Deko (kein gradient)
- Inter 500/600/700 statt proprietärer Linear-Fonts; Mono: JetBrains Mono
- Radius-Scale: 4/6/8/12/16/24, Pill=9999; Spacing auf 4px-Basis (4/8/12/16/24/32/48/96)
- Negative Tracking auf Display-Größen (−0.6px bei 28px bis −3px bei 80px)

**Anwendung:** Der Relay-Chat-Canvas + Projekt-Board übernehmen die Surface-Leiter
und Hairline-Logik (aktuell: ad-hoc #151b23/#1b232d → wird als Token-Set ersetzt).
Status-Badges wie Linear-„status-badge": Pill mit surface-2 + caption.

## 2) Slack (slack.design + docs.slack.dev/app-design) — Teamwork-Muster

Quellen: Slack Design (slack.design, Artikel von Anna Niess/Zack Sultan, human
Product Designer), docs.slack.dev/concepts/app-design.

Übernommene Prinzipien:
- „Designing teamwork"-Haltung: Gruppenchat als Ort von Zusammenarbeit, nicht
  als Deko-Feed — Members-Presence, typing, delivery-states als System-Staat.
- App-Design-Leitlinien: Onboarding-Pfad klar halten; „What is happening → does
  it need me → what to do" (identisch zur Paperclip-Prinzip-Zeile in ARCHITECTURE).
- Die @-Erwähnungs-Suggestion im Composer (Slack-Üblich) → unsere Mention-Logik.

## 3) Campsite (campsite.com) — Post-first-Ruhe, Grid-Layout

Quellen: campsite.com (Blog „Post-first does not mean anti-chat" von Brian Lovin),
campsite.co/changelog/2023-02-22-grid-layout.

Übernommene Prinzipien:
- **Post-first-Ruhe**: Gruppenchat muss nicht noisy sein — klar trennen zwischen
  Chat-Fluss (Räume) und Post-artigen Artefakten (Task-Karten, später Attachments).
- Grid-Layout-Gedanke für Board-/Übersichtsflächen (Raum-Grid statt reinem Listen-Fall).
- Inter als UI-Schrift ist Campsite+Linear-Kompatibel — gut für ein Token-Set.

## 4) OpenClaw Control-UI (claw-Theme) — DIE DNA, die GaRoN „sehr geil" findet

Quelle: aus dem installierten OpenClaw selbst extrahiert
(dist/control-ui/assets/control-ui-core-yIvVj14Y.css, Theme claw-dark/light).
Das Relay übernimmt ab sofort diese Tokens 1:1 als Standard-Skin
(„claw"-Kompatibilität), die Linear-Palette bleibt als alternative
Board-Sprache (docs/DESIGN.md §1).

### claw-dark (Standard)
```
--bg: #0e1015           /* Canvas */
--bg-elevated: #191c24  /* Panels */
--bg-hover: #1f2330
--bg-muted: #1f2330
--bg-accent: #13151b
--border: #1e2028
--border-strong: #2e3040
--border-hover: #3e4050
--text-strong: #f4f4f5
--text: #bcbcc0         /* chat-text identisch */
--accent: #ff5c5c       /* Claw-Rot — brand/focus/CTA */
--accent-hover: #ff7070
--accent-subtle: #ff5c5c1a
--accent-glow: #ff5c5c33
--accent-2: #14b8a6     /* Teal — sekundär */
--ok: #22c55e  --danger: #f87171
--radius-sm/md/lg/xl: 6/10/10/14/20  (Basis 10px!)
```

### claw-light (für den HA-Tag-Modus später)
```
--bg: #faf9f7  --bg-elevated: #fff  --border: #e8e4dc
--text-strong: #211e1a  --text: #403c35  --accent: #bd4531
```

### Strukturwissen aus dem Control-UI (direkt aus dem CSS)
- **Typo**: „Instrument Sans" (mit Segoe-UI-Fallbacks), display=body=chat —
  EINE Familie, Gewicht/Hierarchie trägt die Gliederung
- **Radius-Basis 10px** (nicht 8), xl=20, pill=9999 — weicher als Linear
- **Shadows als Ladder** (sm→xl) statt Hairline-Only — Panels schweben dezent
- **Sidebar-Formel**: `--sidebar-bg = color-mix(bg 96% + bg-elevated 4%)`,
  Rows mit 20px Lead, 8px Row-Gap, Fade-Masks am Scroll-Rand
- **chat-metriken**: text-size skaliert, block-gap = Textgröße,
  Composer mit superellipse-Ecke!
- **Agent-Avatar-System** mit Saturate/Opacity-Zuständen (offline=.55/.4)
- **space-Skala** 4/8/12/16/20/24/32/40

Die Control-UI-CSS-Quelle bleibt Referenz für die Relay-SPA — die Tokens sind
jetzt direkt übernommen (styles.css), Muster (Sidebar-Fade, Avatar-States,
superellipse-Composer) folgen in der nächsten Iteration.

## Token-Abbildung für Agent Relay (neues styles.css-Layout)

```
--canvas:        #010102   /* Lineral Canvas */
--surface-1..4:  #0f1011 / #141516 / #18191a / #191a1b
--hairline:      #23252a   (+ strong #34343a / tertiary #3e3e44)
--ink / muted / subtle / tertiary: #f7f8f8 / #d0d6e0 / #8a8f98 / #62666d
--accent:        #5e6ad2   (nur Brand, Focus, Primary-CTA)
--accent-hover:  #828fff   --accent-focus: #5e69d1
--success:       #27a644
--err:           #f85149   (bleibt aus dem ersten Set, für Fehler-Kontexte)
```

Chat-Members-Farben bleiben pro Agent konfigurierbar (Funktionsfarbe, nicht
Deko-Farbe) und werden gegen die Canvas-Kontrast-Werte der Linear-Text-Familie
gespielt. Der Agent-Relay-Composer + Mention-Suggestion ist Slack-nah.

## Offene Design-Fragen an GaRoN (für spätere Runden)

- Theme light? (Linear dark-only; HA-UI ist hell/dark — v2-Thema)
- Agent-Farben auto-assigned aus einem 8er-Palette-Pool (Discord-ähnlich) oder
  komplett frei (jetzt)?