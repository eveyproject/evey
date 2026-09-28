---
source_sha: caf11b9849876eb65c290e2bc0934d093760aa2b38f6e3c27b303e9af47472f8
---
# Mitarbeiten

Evey wird offen entwickelt, und die Dokumente für alle, die an seinem Code
arbeiten, liegen beim Code auf GitHub statt auf dieser Seite. Diese Seite
verweist auf sie.

## Wo anfangen

| Sie möchten | Lesen Sie |
| --- | --- |
| Evey aus dem Quellcode bauen und starten | [DEV.md](../DEV.md) |
| Die Regeln eines Beitrags kennen: Branches, Commits, Tests, Lizenzvereinbarung | [CONTRIBUTING.md](../CONTRIBUTING.md) |
| Sich in einer halben Stunde im Code zurechtfinden | [Karte des Codes](CODEBASE_MAP.md) |
| Die Schichten und die fachlichen Kontexte verstehen | [Architektur](ARCHITECTURE.md) |
| Den Konventionen folgen, die der Code schon nutzt | [Code-Konventionen](code-conventions.md) |
| An der Oberfläche arbeiten | [Design-System der Oberfläche](frontend-design-system.md) |
| Wissen, warum eine strukturelle Entscheidung getroffen wurde | [Architekturentscheidungen](adr/README.md) |

## Warum Entscheidungsprotokolle, und wo

Jede strukturelle Entscheidung - die hexagonale Aufteilung, wie Tokens von
Erweiterungen begrenzt werden, warum die Installation eine einzige
Compose-Datei ist, wie ein einziger Wiedergabeplan für alle Clients
entscheidet - hat ein Architekturentscheidungsprotokoll: der Zusammenhang, die
abgewogenen Möglichkeiten, was gewählt wurde und was es kostet. Sie liegen im Repository,
neben dem Code, den sie erklären, und sind der Ausgangspunkt, bevor man
vorschlägt, eine dieser Entscheidungen zu ändern.

## Bevor Sie einen Pull Request öffnen

- `make pre-push` ist die einzige Prüfstelle. Es spiegelt die CI: Ein grüner
  Lauf dort bedeutet eine grüne Pipeline.
- Tests werden für neues Verhalten und für jede Fehlerbehebung erwartet.
- Externe Beiträge brauchen eine einmalige Zustimmung zur
  [Lizenzvereinbarung für Mitwirkende](../CLA.md): Evey steht unter
  AGPL-3.0-or-later und wird auch unter einer kommerziellen Lizenz angeboten.
  Sie behalten das Urheberrecht an Ihrer Arbeit.

Sicherheitsprobleme werden nicht öffentlich gemeldet: Schreiben Sie an
`security@eveyproject.org` (siehe [SECURITY.md](../SECURITY.md)).
