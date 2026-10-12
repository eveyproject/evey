---
source_sha: f7107d811a1a25b00c55dbd3789c083144308003ef8ab0ea3e52905c56b1bc7c
---
# Runbook für Veröffentlichungen

Wie eine Evey-Version von einem Branch zu etwas wird, das ein Betreiber fest
einstellen kann. Geschrieben vor der ersten getaggten Veröffentlichung, damit
die erste einem Verfahren folgt, statt eines zu erfinden.

## Was die CI mit einem Tag macht (vor dem Taggen lesen)

`.github/workflows/ci.yml` baut und veröffentlicht Images bei jedem Push und bei
jedem `v*.*.*`-Tag, in drei Varianten der Plugin-Laufzeiten (`` (lean),
`-nodejs`, `-full`), nach `docker.io/eveyproject/evey` und `ghcr.io/evey-project/evey`:

| Sie pushen | Veröffentlichte Image-Tags (jeweils auch `-nodejs` / `-full`) |
|---|---|
| einen Branch (z. B. `beta-0.0.1`) | `beta-0.0.1`, `sha-<short>` |
| Tag `v0.2.0-beta.1` (Vorabversion: enthält einen Bindestrich) | `0.2.0-beta.1`, `sha-<short>` |
| Tag `v0.2.0` (stabil) | `0.2.0`, `0.2`, `0`, `latest`, `sha-<short>` |

Drei Folgen:

- **Ein Branch-Tag ändert sich.** `beta-0.0.1-full` zeigt auf das, was zuletzt
  gepusht wurde; zwei `docker compose pull` im Abstand eines Tages können
  unterschiedlichen Code laufen lassen, und ein Rollback „auf
  `beta-0.0.1-full`“ funktioniert nicht. Betreiber müssen einen
  `sha-<short>`-Tag (oder einen Release-Tag) fest einstellen, nie einen
  Branchnamen - genau das gibt `scripts/evey-update.sh` am Ende aus, damit sie
  es in `.env` eintragen.
- **Eine Vorabversion berührt `latest` nie.** `latest` bedeutet die neueste
  stabile Version; eine Version der geschlossenen Beta ist für alle unsichtbar,
  die `latest` folgen. (Eine Vorabversion bekommt auch keine `X.Y`- / `X`-Tags:
  die gibt es nur für stabile Versionen.)
- **Multi-Arch bei jedem Build** (amd64 + arm64), sodass ein ARM-Betreiber
  jeden dieser Tags fest einstellen kann.

Der Commit wird ins Binary (`evey version`, `/healthz`) und in die Image-Labels
(`org.opencontainers.image.revision`) eingetragen, sodass sich „welcher Code ist
das?“ immer beantworten lässt, ohne einem Tag zu vertrauen.

## Eine Vorabversion erstellen (geschlossene Beta)

1. **Alle Prüfungen auf genau diesem Commit bestanden**: `make pre-push` lokal. Die CI
   führt beim Push dasselbe aus.
2. **Zuerst auf einer echten Installation geprüft.** Der Commit muss schon auf
   der Referenzinstallation als `sha-<short>`-Tag gelaufen sein, aktualisiert mit
   `scripts/evey-update.sh --tag sha-<short> --apply` und geprüft mit
   `--commit <sha>`. Ein Tag ist eine Zusage an andere Betreiber; erstellen Sie
   es erst nach dieser Prüfung, nicht davor.
3. **`CHANGELOG.md`**: die Einträge unter `[Unreleased]` mit Datum unter die neue
   Version verschieben. Keep-a-Changelog-Format, nur Formulierungen für
   Benutzer.
4. **Von diesem Commit aus taggen**, annotiert, und nur den Tag pushen:
   ```bash
   git tag -a v0.2.0-beta.1 -m "v0.2.0-beta.1"
   git push origin v0.2.0-beta.1
   ```
5. **Die CI beobachten**, wie sie `0.2.0-beta.1` (+ Varianten) veröffentlicht.
   Dann auf der Referenzinstallation `scripts/evey-update.sh --tag 0.2.0-beta.1
   --apply` - der feste Release-Tag ersetzt die `sha-`-Festlegung.
6. **Den Tag ankündigen**, mit seinem Abschnitt aus dem Changelog. Betreiber
   stellen `eveyproject/evey:0.2.0-beta.1` (oder `-full`) in ihrer `image:`-Zeile
   fest ein.

Tags für Vorabversionen lassen sich ohne Aufwand anlegen: `-beta.2`, `-beta.3` nach Bedarf.
Zurücknehmen lassen sie sich aber nicht - ein veröffentlichter Tag bleibt
veröffentlicht. Um eine fehlerhafte Version zu ersetzen, erstellen Sie
die nächste; verschieben Sie nie einen Tag.

## Eine stabile Version erstellen

Dieselben Schritte, Tag `vX.Y.Z` ohne Suffix. Eine Sache, die eine Vorabversion
nicht braucht: Die Postgres-Hauptversion, die das Image mitbringt, muss im
Changelog stehen, denn ein Betreiber mit der eingebetteten Datenbank muss den
Abschnitt zum Upgrade in [installation.md](../installation.md) befolgen, wenn
sie sich ändert.

## Eine Version zurückrollen

Die Images bleiben in der Registry. Ein Betreiber rollt zurück mit
`scripts/evey-rollback.sh --tag <previous> [--restore <pre-update dump>] --apply`
(das Update-Skript gibt genau diese Zeile aus). Auf Seiten der Veröffentlichung
muss nichts rückgängig gemacht werden; veröffentlichen Sie ein korrigiertes
`-beta.N+1`, sobald die Korrektur da ist.
