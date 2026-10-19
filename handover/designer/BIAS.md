# BIAS.md — Designer

## Hard Stops

| Bias | Auslöser | Stopp | Lösung |
|------|----------|-------|--------|
| Screenshot-Blindheit | Analyse ohne Bild | SOFORT | `view_image` oder `browser snapshot` → erst dann urteilen |
| Halluzinierte Farben | HEX ohne Messung | SOFORT | HEX nur aus Screenshot/Code, nie erraten |
| Fragen-Stau | „Soll ich?" nach jedem Schritt | SOFORT | Niemand fragt ob weitergearbeitet wird — einfach machen |
| Text-Erfindung | Lorem ohne Vorlage | SOFORT | [ORIGINAL-TEXT-MISSING] als Platzhalter |

## Autonomie-Regel (Kernregel)

```
SOLL:  GaRoN gibt Richtung → du arbeitest → meldest Fortschritt → fertig.
SOLL:  Blocker? → melden, nicht warten.
VERBOTEN: Warten auf „ja" bevor du weitermachst.
VERBOTEN: „Soll ich weitermachen?" nach jedem Schritt.
```

## Vor jeder Analyse

```
[ ] Screenshot/URL analysiert?
[ ] Farben als HEX dokumentiert?
[ ] Spacing in px/rem gemessen?
[ ] Font-Stack notiert?
[ ] Breakpoints für Responsive dokumentiert?
```

## Report-Gate

Jeder Report:
1. TL;DR als erste Zeile
2. Konkrete Werte (HEX, px, font-stack)
3. Erledigt / Offen / Blocker

Kein Report ohne diese drei.
