---
source_sha: 571dd707d8c33cf978fa932960bd67dcdf04a821b8c609c1622fc5767b36c52f
reviewed: true
---
# Récupération

Ouvrez cette page quand plus personne ne peut se connecter, quand la clé qui
signe les jetons d'accès a pu fuiter, ou quand la base doit revenir à un dump
pris avant une migration. Chaque procédure tient en une commande `evey admin`
lancée dans le conteneur ; la page [Ligne de commande](cli.md) liste les
autres sous-commandes.

## Avant de commencer

Les commandes lisent la même configuration `EVEY_*` que le serveur : les
lancer dans le conteneur du serveur ne demande rien de plus.

```sh
docker exec evey-server evey admin <sous-commande>        # serveur en marche
docker compose run --rm evey admin <sous-commande>        # conteneur arrêté
```

La seconde forme démarre d'abord la base embarquée, exécute la commande et
sort. Aucune de ces commandes ne démarre le serveur.

## Donner un nouveau mot de passe à un utilisateur

À utiliser quand un utilisateur est bloqué dehors, ou juste après une
compromission supposée de son compte. Le nouveau mot de passe est affiché une
seule fois et jamais journalisé : copiez-le avant de fermer le terminal.

```sh
docker exec evey-server evey admin reset-password --user admin@example.com
```

Un bon résultat ressemble à ceci :

```
user:     admin@example.com
user_id:  usr-xxxxxxxx
password: <24 octets aléatoires en base64url>

Save this - it is the only time the new password is shown.
Every session of this user has been ended.
```

Toutes les sessions de cet utilisateur se terminent immédiatement : ses
jetons de rafraîchissement sont supprimés et ses jetons d'accès révoqués. Une
application ouverte s'en aperçoit sous une trentaine de secondes et redemande
une connexion. Si vous perdez la sortie, relancez la commande.

## Renouveler la clé qui signe les jetons d'accès

À utiliser quand le fichier de clé a pu fuiter, ou pour déconnecter tout le
monde d'un coup. Pour un seul compte, **Admin -> Utilisateurs -> Désactiver**
ou la réinitialisation de mot de passe ci-dessus suffit.

```sh
docker exec evey-server evey admin rotate-jwt-key
```

Ce qui se passe :

1. Le fichier de clé (`EVEY_JWT_PRIVATE_KEY_PATH`, `/data/jwt.key` par défaut)
   est renommé en `<chemin>.rotated-<horodatage UTC>`.
2. Tous les jetons de rafraîchissement sont supprimés et tous les jetons
   d'accès révoqués, en une seule transaction. Les applications ouvertes s'en
   aperçoivent sous une trentaine de secondes ; tout le monde se reconnecte.
3. Au démarrage suivant, le serveur génère une nouvelle clé RSA de 2048 bits.

Redémarrez ensuite le conteneur (`docker compose restart evey`) pour que la
nouvelle clé serve. Si la base n'a pas pu être jointe, la clé est quand même
mise de côté et la commande l'annonce : `key rotated, but sessions could not
be ended (restart the server, then run this again)`. Faites exactement cela.

## Restaurer un dump pris avant une migration

À utiliser quand une mise à jour doit être annulée et que les données écrites
depuis peuvent être perdues. La commande passe un dump `.sql.gz` par
`gunzip | psql` dans la base configurée.

!!! warning
    La restauration écrase les lignes actuelles. Prenez d'abord une
    sauvegarde de l'état courant (**Admin -> Sauvegarde BDD et restauration ->
    Lancer maintenant**) s'il y a la moindre chance d'en avoir besoin.

```sh
docker compose stop evey
docker compose run --rm evey admin restore-backup /data/backups/pre-migration-152-20260418T220130Z.sql.gz
docker compose up -d
```

Ce qui se passe :

1. La commande attend 5 secondes avant de toucher à quoi que ce soit :
   Ctrl+C annule.
2. `psql` applique le dump avec `--single-transaction` et `ON_ERROR_STOP=1`.
   À la moindre erreur la transaction est annulée et la base reste telle
   qu'elle était.
3. En cas de succès, la dernière ligne est `[admin] restored backup <chemin>`.

Prérequis :

- Le nom du fichier doit finir par `.sql.gz`. Les archives `.tar.gz` de
  **Admin -> Sauvegarde BDD et restauration** se restaurent depuis le
  panneau, pas avec cette commande.
- La base cible doit exister ; le dump supprime et recrée les tables, pas la
  base.
- `psql` et `gunzip` doivent être dans le PATH, ce que l'image garantit.

Les dumps de pré-migration sont écrits automatiquement avant chaque
changement de schéma, sous `EVEY_BACKUP_DIR` (`/data/backups`, donc
`./evey-data/backups` sur l'hôte), nommés
`pre-migration-<version du schéma>-<horodatage UTC>.sql.gz`. Le dump le plus
récent de chacune des 3 dernières versions du schéma est conservé, les autres
sont élagués. [Migrations de la base](migration-runbook.md) explique quand
restaurer un dump plutôt que revenir d'un cran sur le schéma.

<!-- sources
reset-password / rotate-jwt-key: cmd/evey/admin.go - adminResetPassword (DeleteByUserID + BumpTokenEpoch), adminRotateJWTKey, endAllSessions ; epoch cache TTL: internal/app/user/token_epoch.go - epochCacheTTL
restore-backup: cmd/evey/admin.go - adminRestoreBackup (5 s pause, --single-transaction, ON_ERROR_STOP=1, .sql.gz suffix)
admin commands boot the embedded database: docker/entrypoint.sh
refresh validation by stored hash: internal/app/user/auth.go - validateRefreshToken
disable signs out: internal/app/user/user_service.go - DisableUser
dump naming and retention: internal/adapters/driven/postgres/migrate_backup.go - dumpDatabase, backupFilePrefix, pruneOldBackups, defaultBackupRetention
defaults: internal/config/config.go (EVEY_JWT_PRIVATE_KEY_PATH, EVEY_BACKUP_DIR)
-->
