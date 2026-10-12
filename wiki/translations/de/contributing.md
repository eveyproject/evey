---
source_sha: a0eb85f44612fd3d234753208bf63c8a9fec91fc8d1020bfdb8109e199c59b19
reviewed: true
---
# Mitwirken

Wo Sie anfangen, wenn Sie am Code von Evey arbeiten möchten, und was ein
Pull Request bestehen muss. Die Dokumente für Mitwirkende liegen beim Code
auf GitHub, nicht auf dieser Website. Diese Seite verweist auf sie.

## Wo Sie anfangen

| Sie möchten | Lesen Sie |
| --- | --- |
| Evey aus dem Quellcode bauen und starten | [DEV.md](../DEV.md) |
| Die Regeln für einen Beitrag kennen: Branches, Commits, Tests, die Lizenzvereinbarung | [CONTRIBUTING.md](../CONTRIBUTING.md) |
| Sich in einer halben Stunde im Code zurechtfinden | [Karte der Codebasis](CODEBASE_MAP.md) |
| Die Schichten und die abgegrenzten Kontexte verstehen | [Architektur](ARCHITECTURE.md) |
| Den Konventionen folgen, die der Code schon nutzt | [Code-Konventionen](code-conventions.md) |
| An der Oberfläche arbeiten | [Designsystem des Frontends](frontend-design-system.md) |
| Wissen, warum eine grundlegende Wahl getroffen wurde | [Architekturentscheidungen](adr/README.md) |
| Eine Version veröffentlichen | [Runbook für Veröffentlichungen](runbooks/release.md) |

## Warum ein Entscheidungsprotokoll, und wo

Jede grundlegende Wahl hat ein Architektur-Entscheidungsprotokoll: die
hexagonale Struktur, wie die Bereiche von Plugin-Tokens begrenzt sind, warum
die Installation aus einer einzigen Compose-Datei besteht, wie ein einziger
Wiedergabeplan für jeden Client entscheidet. Jedes Protokoll hält den
Kontext fest, die abgewogenen Möglichkeiten, was gewählt wurde und was es
kostet. Sie liegen im Repository, neben dem Code, den sie erklären. Lesen Sie
sie zuerst, bevor Sie vorschlagen, eine dieser Entscheidungen zu ändern.

## Bevor Sie einen Pull Request öffnen

1. Führen Sie `make pre-push` aus: Das ist die Prüfschranke. Sie führt die
   Prüfungen der CI lokal aus, in dieser Reihenfolge: vet, Suche nach
   Schwachstellen, Build, Unit-Tests mit `-race`, Frontend (Typprüfung,
   Lint, Unit-Tests), Sicherheitsprüfung, Lint,
   [Architekturwächter](ARCHITECTURE.md#architecture-guards),
   Commit-Hygiene, Suche nach Geheimnissen, Prüfung auf inkompatible
   Proto-Änderungen, ein Build aus einem sauberen Checkout, die
   Integrationsprüfungen für Repository und Transkodierung und das TV-Audit.
2. Schreiben Sie Tests für neues Verhalten und für jede Fehlerbehebung.
3. Bei einem externen Beitrag akzeptieren Sie einmal die
   [Lizenzvereinbarung für Mitwirkende](../CLA.md): Evey steht unter
   AGPL-3.0-or-later und kann auch unter einer kommerziellen Lizenz
   angeboten werden. Das Urheberrecht an Ihrer Arbeit behalten Sie.

!!! warning "Die Prüfschranke baut weder das Docker-Image noch die Android-Apps"
    Wenn Sie das `Dockerfile` oder Kotlin-Code geändert haben, bauen Sie sie
    vor dem Push selbst (`docker build --target runtime`, die
    Release-Aufgabe von Gradle).

Sicherheitsprobleme werden nicht öffentlich gemeldet: Schreiben Sie an
`security@eveyproject.org` (siehe [SECURITY.md](../SECURITY.md)).

<!-- sources
gate: Makefile - pre-push, ci-fast-vet, ci-fast-vuln, ci-fast-build, ci-fast-unit, ci-fast-frontend, ci-fast-sec, ci-local-lint, ci-arch, ci-hygiene, ci-secrets, ci-proto-breaking, ci-clean-checkout, repo-itest-gate, transcode-itest-gate, tv-audit
documents: DEV.md ; CONTRIBUTING.md ; CLA.md ; SECURITY.md ; docs/CODEBASE_MAP.md ; docs/code-conventions.md ; docs/frontend-design-system.md ; docs/adr/README.md ; docs/runbooks/release.md
-->
