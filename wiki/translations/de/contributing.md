---
source_sha: 0d64371960f5bb56be25ad84664ab862f39caf2e438a08da552c1713a2274048
reviewed: false
---
# Mitwirken

Evey wird offen entwickelt, und die Dokumente für die Menschen, die an seinem
Code arbeiten, liegen beim Code auf GitHub statt auf dieser Website. Diese Seite
verweist auf sie.

## Wo anfangen

| Sie wollen | Lesen Sie |
| --- | --- |
| Evey aus dem Quellcode bauen und starten | [DEV.md](../DEV.md) |
| Die Regeln eines Beitrags kennen: Branches, Commits, Tests, die Lizenzvereinbarung | [CONTRIBUTING.md](../CONTRIBUTING.md) |
| Sich in einer halben Stunde im Code zurechtfinden | [Codebase-Karte](CODEBASE_MAP.md) |
| Die Schichten und die Bounded Contexts verstehen | [Architektur](ARCHITECTURE.md) |
| Den Konventionen folgen, die der Code bereits nutzt | [Code-Konventionen](code-conventions.md) |
| An der Oberfläche arbeiten | [Frontend-Designsystem](frontend-design-system.md) |
| Wissen, warum eine strukturelle Entscheidung getroffen wurde | [Architekturentscheidungen](adr/README.md) |
| Eine Version veröffentlichen | [Release-Runbook](runbooks/release.md) |

## Warum ein Entscheidungsprotokoll, und wo

Jede strukturelle Entscheidung - der hexagonale Aufbau, wie Plugin-Tokens
begrenzt werden, warum die Installation eine einzige Compose-Datei ist, wie ein
Wiedergabeplan für jeden Client entscheidet - hat ein Architecture Decision
Record: den Kontext, die abgewogenen Optionen, was gewählt wurde und was es
kostet. Sie liegen im Repository, neben dem Code, den sie erklären, und sind der
Ausgangspunkt, bevor man vorschlägt, eine dieser Entscheidungen zu ändern.

## Bevor Sie einen Pull Request öffnen

- `make pre-push` ist die Hürde: Es führt die Prüfungen der CI lokal aus (vet,
  Schwachstellensuche, Build, Unit-Tests mit `-race`, Frontend, Lint,
  Architekturwächter). Das Docker-Image und die Android-Apps baut es nicht:
  Nach Änderungen am `Dockerfile` oder an Kotlin-Code bauen Sie sie selbst.
- Tests werden für neues Verhalten und für jede Fehlerbehebung erwartet.
- Externe Beiträge brauchen eine einmalige Zustimmung zur
  [Contributor License Agreement](../CLA.md): Evey steht unter
  AGPL-3.0-or-later und wird auch unter einer kommerziellen Lizenz angeboten. Sie
  behalten das Urheberrecht an Ihrer Arbeit.

Sicherheitsprobleme werden nicht öffentlich gemeldet: Schreiben Sie an
`security@eveyproject.org` (siehe [SECURITY.md](../SECURITY.md)).
