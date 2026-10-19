# Round 9 — Design-Basis + v2-Finish

## TL;DR

Design-Basis-Dokument + 2 Frontend-Lücken geschlossen (Presence-Dots an Room-Rows, @-Mention-Chips). Shot `shots/v2-finish.png` verifiziert.

## Was stand vorher / nachher Fix

| Lücke | Vorher | Fix |
|-------|--------|-----|
| design-basis.md | fehlte (Schritt 2) | `docs/design-basis.md` — 15-Zeilen-Matrix: Pattern → Quelle → Anwendung |
| Presence-Dots an Room-Rows | nur im Roster (`Team`), nicht an Raum-Zeilen | `.room-presence` grün wenn Room enabled Agent hat |
| @-Mention-Chips im Chat | nur Popover-Suggestion, Message-Render war plain esc() | `renderContent()` → `@Name` = `.mention-chip` Pill |

Board-Tab-Tiles waren bereits vorhanden (`inlineBoard` Kanban), kein Fix nötig.

## Checkliste

- [x] design-basis.md geschrieben (<600 Wörter, Tabelle)
- [x] Frontend-Lücken gebaut + CSS
- [x] commit `design(v2): ...`
- [x] server restart + shot `shots/v2-finish.png`
- [x] dieser Report + shot verifiziert

## Shot

`shots/v2-finish.png` — 1280×800, Chat-Ansicht mit Sidebar v2 + Tokens.
