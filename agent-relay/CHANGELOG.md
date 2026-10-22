# Changelog Agent Relay (Add-on)

## 0.2.0 — Feature-Block P1+P2/Lane-2 (2026-10-20, lokal)

**P1 (Codex-Matrix Kernfeatures):**
- Message-Edit: eigene Nachrichten mit ✎ bearbeiten (`PATCH /api/messages/{id}`, human-only-Guard; DOM-Refresh WS-getrieben)
- Attachments: 📎 Anhänge an Messages (multipart ≤32 MB, Traversal-Schutz, Bild-Vorschau inline, Datei-Chips mit Größe)
- Notifications: 🔔 Browser-Benachrichtigungen (Toggle im User-Menü; fremd-Raum/tab-hidden-Guard, Koaleszenz je Raum)
- Threads: ↩ Antworten mit Thread-Chip + ReplyCount-Badges (Agent-Antworten erben den Thread)

**P2/Lane 2:**
- Task-Chatter: 💬 Kommentar-Verlauf je Task (Live via WS, Cross-Window)
- Approvals-Gate: ⛔ Freigabe-Flow — Turn hält vor Stream, Banner-Entscheidung (Grant/Deny, 409-Race-Schutz, Grant-Token wird konsumiert)
- Board-DnD: Task-Karten zwischen Status-Spalten ziehen
- Mention-Invite-inline: „＋ einladen?" direkt im Mention-Pop (Nicht-Member)

**Infra:** dist-Binaries frisch aus HEAD (amd64/aarch64); SPA relative Asset-URLs (HA-Ingress-kompatibel); Mobile-Back-Buttons in allen Vollansichten; Follow-Scroll + Unread-Marker + Row-Subtitles (Control-UI-Muster).

## 0.1.1 — Ausgangsstand (Vor-Übergabe)
