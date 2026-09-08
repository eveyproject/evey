---
source_sha: ac74ba14732aabf186129184b065fedaeb40d996e987374e385a8edc9d0e5c84
---

# Exploitation

Que faire quand le serveur tourne déjà : le jour où il déraille, le jour où vous
le déplacez, et le jour où il faut le remettre debout.

Les pages qui suivent supposent un terminal et un compte administrateur. Presque
tout ce qui suit s'atteint aussi depuis le panneau d'administration ; le shell
est le recours pour le jour où c'est justement le panneau qui a lâché.

| Vous voulez | Lisez |
| --- | --- |
| Lancer une commande de maintenance sur le serveur | [Ligne de commande](cli.md) |
| Comprendre pourquoi un fichier refuse la navigation, ou se lit désynchronisé | [Santé des médias](media-health.md) |
| Relever un serveur qui refuse de démarrer | [Récupération](recovery.md) |
| Préparer ou annuler une migration de base de données | [Migrations](migration-runbook.md) |
| Traiter un incident en cours | [Incidents](incident-runbook.md) |

## Avant un incident, pas pendant

Deux choses méritent une vérification pendant que rien ne brûle, parce qu'il est
trop tard pour les découvrir sous pression : que les
[sauvegardes](../backups.md) sont bel et bien écrites là où vous le croyez, et
que vous savez en restaurer une. Une sauvegarde que personne n'a jamais
restaurée n'est qu'une hypothèse.
