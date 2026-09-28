---
source_sha: caf11b9849876eb65c290e2bc0934d093760aa2b38f6e3c27b303e9af47472f8
---
# Contribuer

Evey se développe au grand jour, et les documents écrits pour ceux qui
travaillent sur son code vivent avec le code, sur GitHub, plutôt que sur ce
site. Cette page les indique.

## Par où commencer

| Vous voulez | Lisez |
| --- | --- |
| Compiler et lancer Evey depuis les sources | [DEV.md](../DEV.md) |
| Connaître les règles d'une contribution : branches, commits, tests, accord de licence | [CONTRIBUTING.md](../CONTRIBUTING.md) |
| Vous repérer dans le code en une demi-heure | [Carte du code](CODEBASE_MAP.md) |
| Comprendre le découpage en couches et les contextes métier | [Architecture](ARCHITECTURE.md) |
| Suivre les conventions que le code utilise déjà | [Conventions de code](code-conventions.md) |
| Travailler sur l'interface | [Système de design de l'interface](frontend-design-system.md) |
| Savoir pourquoi un choix structurel a été fait | [Décisions d'architecture](adr/README.md) |

## Pourquoi un registre de décisions, et où

Chaque choix structurel - le découpage hexagonal, la portée des jetons
d'extension, pourquoi l'installation tient en un seul fichier compose, comment
un seul plan de lecture décide pour tous les clients - a son enregistrement de
décision d'architecture : le contexte, les options pesées, ce qui a été choisi
et ce que cela coûte. Ils sont conservés dans le dépôt, à côté du code qu'ils
expliquent, et c'est par eux qu'il faut commencer avant de proposer de changer
l'un de ces choix.

## Avant d'ouvrir une pull request

- `make pre-push` est le seul point de contrôle. Il reproduit la CI : un
  passage au vert ici, c'est une CI au vert.
- Des tests sont attendus pour tout nouveau comportement et pour chaque
  correction de bug.
- Les contributions externes demandent d'accepter une fois
  l'[accord de licence des contributeurs](../CLA.md) : Evey est sous
  AGPL-3.0-or-later et proposé aussi sous licence commerciale. Vous gardez le
  droit d'auteur sur votre travail.

Les problèmes de sécurité ne se signalent pas en public : écrivez à
`security@eveyproject.org` (voir [SECURITY.md](../SECURITY.md)).
