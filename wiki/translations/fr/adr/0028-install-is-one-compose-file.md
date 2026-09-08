---
source_sha: e88d78c5e9433728dc911cb830eb13b70b669e8e74eb0eb39359e88d355fd6a6
---

# ADR-0028 : l'installation tient dans un fichier compose

- Statut : acceptée
- Date : 2026-08-29

## Contexte

Installer Evey demandait auparavant d'inventer deux secrets à la main et de
faire tourner Postgres dans un second conteneur. Un audit produit a noté
l'installation et l'exploitation 7,6 sur 10 en nommant la raison : la personne
qui veut poser un conteneur et ouvrir un navigateur n'était pas servie.

## Décision

L'image porte ce dont elle a besoin. Au premier démarrage, le point d'entrée
génère le mot de passe de la base et la clé de recherche dans `/data/keys`, et
fait tourner Postgres et Meilisearch dans le conteneur, sous l'utilisateur non
root du serveur. La base est embarquée exactement quand `EVEY_DB_HOST` est
absent, si bien que toute installation qui pointait déjà vers son propre
Postgres continue de fonctionner sans y toucher.

L'installation tient donc dans un seul `docker-compose.yml` sans `.env`, et les
variantes - un second dossier de médias, un reverse proxy déjà en place, une
base externe - font quelques lignes chacune sur ce même fichier.

Écarté : SQLite, qui aurait supprimé le second conteneur au prix d'une base
différente en développement et en production. Le Postgres embarqué donne le même
résultat pendant que le code continue de parler à un seul moteur.

## Conséquences

- Aucun secret généré n'est écrit dans le fichier compose, donc rien dans le
  démarrage rapide ne peut être recopié de travers.
- Le répertoire de données contient désormais un cluster Postgres, ce qui lie sa
  version majeure à celle de l'image. Une version majeure plus récente refuse
  d'ouvrir un ancien cluster plutôt que de prendre le risque, et la marche à
  suivre est décrite dans [installation.md](../installation.md).
- L'administrateur qui veut garder la main sur le cycle de vie de la base l'a
  toujours : poser `EVEY_DB_HOST` désactive la base embarquée.
- Le `docker-compose.yml` du dépôt reste la disposition opérateur, parce que la
  pile de développement en dépend. Ce fichier n'est pas l'installation :
  l'installation est le bloc de `installation.md`.
