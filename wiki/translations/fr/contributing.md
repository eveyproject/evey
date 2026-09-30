---
source_sha: 053efa5319e4b48ff12a944ce76f5755b01c4022ff1105d87cf82c5135ac1b4d
reviewed: true
---
# Contribuer

Evey se développe au grand jour, et les documents écrits pour celles et ceux qui
travaillent sur son code vivent avec le code, sur GitHub, plutôt que sur ce
site. Cette page y renvoie. Ils sont en anglais.

## Par où commencer

| Vous voulez | Lisez |
| --- | --- |
| Compiler et lancer Evey depuis les sources | [DEV.md](../DEV.md) |
| Connaître les règles d'une contribution : branches, commits, tests, accord de licence | [CONTRIBUTING.md](../CONTRIBUTING.md) |
| Vous repérer dans le code en une demi-heure | [Carte du code](CODEBASE_MAP.md) |
| Comprendre les couches et les contextes métier | [Architecture](ARCHITECTURE.md) |
| Suivre les conventions que le code applique déjà | [Conventions de code](code-conventions.md) |
| Travailler sur l'interface | [Système de design du frontend](frontend-design-system.md) |
| Savoir pourquoi un choix structurel a été fait | [Décisions d'architecture](adr/README.md) |
| Publier une version | [Runbook de publication](runbooks/release.md) |

## Pourquoi des fiches de décision, et où

Chaque choix structurel (l'architecture hexagonale, la portée des jetons des
plugins, une installation en un seul fichier compose, un plan de lecture unique
qui décide pour tous les clients) a sa fiche de décision d'architecture : le
contexte, les options pesées, ce qui a été retenu et ce que cela coûte. Elles
sont rangées dans le dépôt, à côté du code qu'elles expliquent, et c'est par
elles qu'il faut commencer avant de proposer de revenir sur l'un de ces choix.

## Avant d'ouvrir une pull request

- `make pre-push` est le seul point de contrôle. Il reproduit la CI : un
  passage au vert en local, c'est un pipeline au vert.
- Des tests sont attendus pour tout nouveau comportement et pour chaque
  correction de bug.
- Une contribution externe demande d'accepter une fois
  [l'accord de licence des contributeurs](../CLA.md) : Evey est sous
  AGPL-3.0-or-later et est aussi proposé sous licence commerciale. Vous gardez
  le droit d'auteur sur votre travail.

Les problèmes de sécurité ne se signalent pas en public : écrivez à
`security@eveyproject.org` (voir [SECURITY.md](../SECURITY.md)).
