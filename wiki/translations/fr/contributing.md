---
source_sha: 6da699a191571156f6c459828ca57f752309874f1694e191971ce9d2ba7d6c95
reviewed: true
---
# Contribuer

Par où commencer quand vous voulez travailler sur le code d'Evey, et ce
qu'une pull request doit passer. Les documents écrits pour les contributeurs
vivent avec le code, sur GitHub, plutôt que sur ce site ; cette page y
renvoie. Ils sont en anglais.

## Par où commencer

| Vous voulez | Lisez |
| --- | --- |
| Construire et lancer Evey depuis les sources | [DEV.md](../DEV.md) |
| Connaître les règles d'une contribution : branches, commits, tests, l'accord de licence | [CONTRIBUTING.md](../CONTRIBUTING.md) |
| Vous repérer dans le code en une demi-heure | [Carte du code](CODEBASE_MAP.md) |
| Comprendre les couches et les contextes bornés | [Architecture](ARCHITECTURE.md) |
| Suivre les conventions que le code utilise déjà | [Conventions de code](code-conventions.md) |
| Travailler sur l'interface | [Système de design du frontend](frontend-design-system.md) |
| Savoir pourquoi un choix structurel a été fait | [Décisions d'architecture](adr/README.md) |
| Publier une version | [Procédure de release](runbooks/release.md) |

## Pourquoi une décision d'architecture, et où

Chaque choix structurel - la disposition hexagonale, la portée des jetons
de plugin, pourquoi l'installation tient en un seul fichier compose,
comment un seul plan de lecture décide pour tous les clients - a sa
décision d'architecture (ADR) : le contexte, les options pesées, ce qui a
été choisi et ce que cela coûte. Elles sont conservées dans le dépôt, à côté
du code qu'elles expliquent, et sont le point de départ avant de proposer de
changer l'un de ces choix.

## Avant d'ouvrir une pull request

1. Lancez `make pre-push` : c'est la barrière, et elle exécute en local les
   vérifications de la CI dans cet ordre : vet, recherche de vulnérabilités,
   build, tests unitaires avec `-race`, frontend (typage, lint, tests
   unitaires), analyse de sécurité, lint,
   [gardes d'architecture](ARCHITECTURE.md#architecture-guards), hygiène des
   commits, recherche de secrets, détection de rupture des proto, build
   depuis un checkout propre, les barrières d'intégration du dépôt et du
   transcodage, et l'audit TV.
2. Ajoutez des tests pour tout nouveau comportement et pour chaque
   correction de bug.
3. Pour une contribution externe, acceptez une fois
   l'[accord de licence de contribution](../CLA.md) : Evey est sous
   AGPL-3.0-or-later et est aussi proposé sous licence commerciale. Vous
   gardez le droit d'auteur sur votre travail.

!!! warning "La barrière ne construit ni l'image Docker ni les applications Android"
    Après avoir touché le `Dockerfile` ou du code Kotlin, construisez-les
    vous-même (`docker build --target runtime`, la tâche Gradle de release)
    avant de pousser.

Les problèmes de sécurité ne se signalent pas en public : écrivez à
`security@eveyproject.org` (voir [SECURITY.md](../SECURITY.md)).

<!-- sources
gate: Makefile - pre-push, ci-fast-vet, ci-fast-vuln, ci-fast-build, ci-fast-unit, ci-fast-frontend, ci-fast-sec, ci-local-lint, ci-arch, ci-hygiene, ci-secrets, ci-proto-breaking, ci-clean-checkout, repo-itest-gate, transcode-itest-gate, tv-audit
documents: DEV.md ; CONTRIBUTING.md ; CLA.md ; SECURITY.md ; docs/CODEBASE_MAP.md ; docs/code-conventions.md ; docs/frontend-design-system.md ; docs/adr/README.md ; docs/runbooks/release.md
-->
