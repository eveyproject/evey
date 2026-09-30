---
source_sha: 97b31b19a771829c011c797f4712f4cccf1903064069d080bb2eaadb59c386e2
reviewed: true
---
# Sauvegardes et mises à jour

## Ce que contient une sauvegarde

Une sauvegarde est une copie de la **base de données** : comptes, profils,
bibliothèques et leurs métadonnées, historique de lecture et points de
reprise, collections et playlists, réglages, enregistrements des plugins,
webhooks et journaux d'audit. C'est une archive `.tar.gz` qui contient le
dump SQL et un manifeste avec son empreinte SHA256.

Elle ne contient **pas** :

- vos fichiers de médias ;
- le cache de transcodage (régénéré à la demande) ;
- l'index de recherche (reconstruit depuis la base) ;
- les clés rangées dans `./evey-data` : `jwt.key`, qui signe les sessions, et
  le dossier `keys` (mot de passe de la base, clé de recherche) ;
- les fichiers des plugins installés.

Une sauvegarde de la base restaure donc l'état d'Evey sur la même
installation. Pour pouvoir reconstruire une machine, gardez aussi une copie
de tout le dossier `./evey-data` : les sauvegardes de la base s'y trouvent,
dans `./evey-data/backups`.

## Sauvegardes planifiées

**Admin → Sauvegarde BDD et restauration → Planification** : une expression
cron (par exemple `0 3 * * *`, tous les jours à 3 h) et une durée de
conservation en jours. Les sauvegardes plus anciennes sont supprimées après
chaque exécution planifiée. **Lancer maintenant** en crée une
immédiatement.

<figure class="evey-shot" markdown="span">
  ![Admin → Sauvegarde BDD et restauration avec une planification quotidienne](images/admin-backup.webp){ loading=lazy }
  <figcaption>Une sauvegarde chaque jour à 3 h, conservée 30 jours.</figcaption>
</figure>

Indépendamment de la planification, le serveur sauvegarde aussi la base avant
d'appliquer une migration lors d'une mise à jour (voir plus bas).

## Restaurer

**Admin → Sauvegarde BDD et restauration** liste les sauvegardes et en
restaure une, ou restaure un fichier que vous téléversez. La restauration
remplace la base actuelle.

Une sauvegarde ne se restaure que sur un serveur dont le schéma de base est
identique : le serveur vérifie d'abord l'empreinte et la version du schéma,
et refuse une sauvegarde prise par une version dont le schéma diffère plutôt
que de la charger. Pour revenir à une version plus ancienne avec ses données,
voir [Revenir en arrière](#going-back).

Quand le serveur ne démarre plus, la ligne de commande restaure les dumps pris
avant une migration (`pre-migration-*.sql.gz`), pas les sauvegardes `.tar.gz`
de ce panneau : voir [Restauration](runbooks/recovery.md).

## Mettre à jour Evey

```bash
docker compose pull && docker compose up -d
docker compose logs -f evey
```

C'est toute la mise à jour. Quand la nouvelle version doit modifier la base,
elle écrit d'abord un dump dans `./evey-data/backups/pre-migration-*`, puis
applique les migrations au démarrage ; sur un gros catalogue, cela peut
prendre quelques minutes, et le conteneur est de nouveau signalé en bonne
santé dès que le serveur répond. Evey vérifie toutes les 6 heures si une image plus récente
est publiée et le signale dans le panneau d'administration ; il ne se met
jamais à jour tout seul.

**Figez ce que vous exécutez.** `latest` suit la dernière version publiée.
Pour un serveur sur lequel vous comptez, mettez un tag de version (`0.2.0`)
ou un tag immuable `sha-<commit>` sur la ligne `image:`, pour que deux `pull`
à un jour d'écart exécutent le même code. Un tag de branche comme
`beta-0.0.1` bouge à chaque modification.

**Savoir quelle version tourne.** `docker exec evey-server evey version`
affiche la version et le commit du binaire en cours d'exécution.

### Revenir en arrière

Remettez le tag précédent sur la ligne `image:` et lancez
`docker compose up -d`. Si la version plus récente avait migré la base,
restaurez d'abord son dump d'avant migration :
`docker exec evey-server evey admin rollback` affiche les étapes exactes avec
le chemin du dump.

Les tags d'image, les variantes de runtime et les familles de GPU sont
décrits dans [Déploiements avancés](operations/advanced-deployments.md#image-tags).
