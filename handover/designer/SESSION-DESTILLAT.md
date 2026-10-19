# Session-Destillat: Designer-Dashboard (9b644870) — sicherer Stand vor Neustart
# 2026-10-18 21:15 · vom Coordinator destilliert aus sessions_history

## GaRoN-Direktweisungen (außerhalb der Berichte, aus dem Chat)

1. **„Ich habe euch beide jeweils euren Sessions zugewiesen, ihr könnt das
      jetzt alles selber aushandeln"** — Designer und Coordinator arbeiten als
      gleichberechtigte Partner-Sessions; die Zuspiel-Mechanik (sessions_send
      mit Bericht-Pfaden) ist die Vereinbarung.
2. **„Ok go für beide"** — Runde-8-Programm (Muse: B1 Composer-Verankerung,
      B2 Kontrast, B3 TEAM-Liste) wurde von beiden Seiten bestätigt.
3. **„Frontend allein, kein Umweg über Coordinator"** (letzte Weisung im
      Designer-Chat): Designer baut das Frontend-SOLO — der Coordinator ist
      nicht mehr Zwischenhandler für Markup/CSS-Work.
4. **Sidebar nach „echtem OpenClaw-Control-UI-Prinzip"**:
   - **Start-Seiten etc. als einklappbare Menüs oben** (Navigation = oberste Ebene)
   - **Settings/Admin als User-Dropdown unten** (nach oben aufklappend, an der
     Identity-Card gebunden)

## Loop-Status (wo er unterbrochen ist)

- **Runde 7** (Rebuild B1-B4): gebaut (e426293), Designer-Verify 3/4,
  Chevron-Fix eingearbeitet → geschlossen.
- **Runde 8** (Muse B1-B3): gebaut (127f92d), Verify durch Coordinator-Sicht
  3/3 + Designer-Verify 2/3 „B1 teilweise" (Placeholder blass, Margin 12px,
  Send-Button opacity .85) → **B1-Feintuning ist der offene Bauauftrag.**
- **Der Designer war bereit, das komplette FRONTEND-REBUILD zu starten**
  (unter der neuen Direktweisung 3+4), als die Modellspec-Aktualisierung kam.

## Offene Bau-Aufträge (neue Session startet hier)

1. **B1-Feintuning aus round-8-verify**: composer input placeholder →
   `var(--ink-3)`, margin → 12px, send-button opacity .85 bei Leerzustand.
2. **Sidebar v2 nach Direktweisung 4**: Start/Agents/Dashboards/… oben als
   einklappbare Menüs; Settings/Admin unten im User-Dropdown (aufklappend
   nach oben) an der Identity-Card.
3. **Frontend-Solo**: der Designer baut selbst (CSS/JS direkt), Coordinator
   verwaltet nur Repo/Commits/Server/Verifikationsshots.

## Modell-Fakten (Pinning)

- designer = muse-spark-open:cloud (Fallback gemma4:31b-cloud)
- Achtung Blockade: seit Modellspec-Aktualisierung — neue Session nach
  Gateway-Restart erwartet das **modelApplied-Verhalten** zu prüfen
  (Registry ist gepinnt; das Pinning bleibt).

## Server-Fakten

- Relay läuft unabhängig vom Gateway (eigener Go-Prozess).
  Port 8099, WSL-IP-Erreichbarkeit bewiesen, Daten in /tmp/ar-fresh.
- Dev-Datenbank: /tmp/ar-fresh (Demo), Prod-Konfig: agent-relay/config.yaml.
