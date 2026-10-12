---
source_sha: 7e38ef34503d1582f3a4a5fc3e6171c1d2f137aa52dae78d9840504057637151
---

# Runbook für Veröffentlichungen

Wie eine Version von Evey von einem Branch zu einem Image wird, das ein
Betreiber fest einstellen kann. Geschrieben vor der ersten getaggten Version,
damit die erste einem festgelegten Ablauf folgt, statt ihn zu erfinden.

## Was die CI mit einem Tag macht (vor dem Taggen lesen)

`.github/workflows/ci.yml` baut und veröffentlicht Images bei jedem Push und bei
jedem `v*.*.*`-Tag, in drei Varianten der Plugin-Laufzeiten (`` (schlank),
`-nodejs`, `-full`), nach `docker.io/eveyproject/evey` und
`ghcr.io/eveyproject/evey`:

| Sie pushen | Veröffentlichte Image-Tags (jeweils auch `-nodejs` / `-full`) |
|---|---|
| einen Branch (z. B. `beta-0.0.1`) | `beta-0.0.1`, `sha-<short>` |
| das Tag `v0.2.0-beta.1` (Vorabversion: enthält einen Bindestrich) | `0.2.0-beta.1`, `sha-<short>` |
| das Tag `v0.2.0` (stabil) | `0.2.0`, `0.2`, `0`, `latest`, `sha-<short>` |

Drei Folgen:

- **Ein Branch-Tag ändert sich.** `beta-0.0.1-full` zeigt auf das, was zuletzt
  gepusht wurde; zwei `docker compose pull` im Abstand eines Tages können
  unterschiedlichen Code ausführen, und ein Zurückrollen „auf
  `beta-0.0.1-full`“ bezeichnet keine bestimmte Version. Betreiber müssen ein
  `sha-<short>`-Tag (oder ein Versions-Tag) festlegen, nie einen Branch-Namen:
  genau das gibt `scripts/evey-update.sh` am Ende aus, zum Eintragen in `.env`.
- **Eine Vorabversion ändert `latest` nie.** `latest` bezeichnet die neueste
  stabile Version; eine Vorabversion der geschlossenen Beta bleibt für alle
  unsichtbar, die `latest` folgen. (Eine Vorabversion erhält auch keine
  `X.Y`- / `X`-Tags: Diese gibt es nur für stabile Versionen.)
- **Multi-Arch nur bei Versions-Tags.** Ein `v*.*.*`-Tag (stabil oder
  Vorabversion) baut amd64 + arm64; der Push eines Branches baut nur amd64,
  außer die Commit-Nachricht enthält `[arm64]` oder der Workflow wird von Hand
  gestartet. Ein ARM-Betreiber legt ein Versions-Tag fest.

Der Commit wird ins Binary (`evey version`, `/healthz`) und in die Image-Labels
(`org.opencontainers.image.revision`) geschrieben. So lässt sich immer
feststellen, welcher Code läuft, ohne sich auf ein Tag zu verlassen.

## Eine Vorabversion erstellen (geschlossene Beta)

1. **Prüfungen bestanden auf genau diesem Commit**: `make pre-push` lokal. Die
   CI führt beim Push dieselben Prüfungen aus.
2. **Zuerst auf einer echten Installation geprüft.** Der Commit muss bereits
   als `sha-<short>`-Tag auf dem Referenz-Deployment gelaufen sein, aktualisiert
   mit `scripts/evey-update.sh --tag sha-<short> --apply` und geprüft mit
   `--commit <sha>`. Ein Tag ist eine Zusage an andere Betreiber; erstellen Sie
   es erst nach dieser Prüfung.
3. **`CHANGELOG.md`**: Verschieben Sie die Einträge unter `[Unreleased]` mit
   Datum unter die neue Version. Format Keep a Changelog, nur für Benutzer
   verständliche Formulierungen.
4. **Taggen Sie diesen Commit** mit einem annotierten Tag und pushen Sie nur
   das Tag:
   ```bash
   git tag -a v0.2.0-beta.1 -m "v0.2.0-beta.1"
   git push origin v0.2.0-beta.1
   ```
5. **Beobachten Sie, wie die CI** `0.2.0-beta.1` (+ Varianten) veröffentlicht.
   Danach auf dem Referenz-Deployment
   `scripts/evey-update.sh --tag 0.2.0-beta.1 --apply`: Das Versions-Tag
   ersetzt die `sha-`-Festlegung.
6. **Kündigen Sie das Tag an**, mit seinem Abschnitt aus dem Changelog.
   Betreiber tragen `eveyproject/evey:0.2.0-beta.1` (oder `-full`) in ihre
   `image:`-Zeile ein.

Tags für Vorabversionen kosten nichts: `-beta.2`, `-beta.3` nach Bedarf. Sie
lassen sich aber nicht zurückziehen: Ein veröffentlichtes Tag bleibt
veröffentlicht. Um eine fehlerhafte Vorabversion zu korrigieren, veröffentlichen
Sie die nächste; verschieben Sie nie ein Tag.

## Eine stabile Version erstellen

Dieselben Schritte, mit einem Tag `vX.Y.Z` ohne Suffix. Ein Punkt, den eine
Vorabversion nicht braucht: Die Postgres-Hauptversion, die das Image enthält,
muss im Changelog stehen, denn ein Betreiber mit der eingebetteten Datenbank
muss bei einem Wechsel dem Upgrade-Abschnitt von
[installation.md](../installation.md) folgen.

## Eine Version zurückrollen

Die Images bleiben in der Registry. Ein Betreiber rollt zurück mit
`scripts/evey-rollback.sh --tag <previous> [--restore <pre-update dump>] --apply`
(das Update-Skript gibt genau diese Zeile aus). Auf der Veröffentlichungsseite
ist nichts rückgängig zu machen; veröffentlichen Sie eine korrigierte
`-beta.N+1`, sobald die Korrektur fertig ist.
