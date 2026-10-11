---
source_sha: bc6d759cd6c3e604c5638d77b66eb6e90417021b881f7a301c6c3ea992d41c82
reviewed: true
---
# Sauvegardes et mises à jour

Une sauvegarde planifiée de la base de données, conservée aussi longtemps que
vous le décidez, et une mise à jour qui tient en un `docker compose pull`.
Cette page s'adresse à l'administrateur : ce qu'une sauvegarde contient et ce
qu'elle ne contient pas, comment en restaurer une, et comment mettre à jour ou
revenir en arrière sans perdre de données.

## Ce que contient une sauvegarde

Une sauvegarde est une copie de la **base de données** : comptes, profils,
bibliothèques et leur catalogue, historique de lecture et points de reprise,
collections, playlists et favoris, réglages, enregistrements des plugins,
webhooks, clés API, notifications et journaux d'audit. C'est une archive
`.tar.gz` qui contient le dump SQL et un manifeste avec l'empreinte SHA256 du
dump et la version du schéma.

Elle ne contient **pas** :

- vos fichiers de médias ;
- le cache de transcodage (régénéré à la demande) ;
- l'index de recherche (reconstruit depuis la base) ;
- les clés rangées dans `./evey-data` : `jwt.key`, qui signe les sessions, et
  le dossier `keys` (mot de passe de la base, clé de recherche) ;
- les fichiers des plugins installés.

Les métriques de lecture et de serveur (la télémétrie derrière les graphiques
de qualité et de charge) sont laissées de côté par défaut : ce sont elles qui
grossissent le plus vite, et elles ne disent rien de votre catalogue.
**Admin → Configuration du serveur → Inclure la télémétrie dans les
sauvegardes** les réintègre ; ce réglage vaut pour les sauvegardes manuelles,
planifiées et en ligne de commande. Restaurer une sauvegarde vide toujours ces
tables.

Une sauvegarde de la base restaure donc l'état d'Evey sur la même
installation. Pour pouvoir reconstruire une machine, gardez aussi une copie de
tout le dossier `./evey-data` : les sauvegardes de la base s'y trouvent, dans
`./evey-data/backups`.

## Planifier les sauvegardes

**Admin → Sauvegarde BDD et restauration → Planification** : une **Expression
cron** (par exemple `0 3 * * *`, tous les jours à 3 h) et une **Rétention
(jours)**. Les sauvegardes plus anciennes que la rétention sont supprimées
après chaque exécution planifiée. **Lancer maintenant** en crée une
immédiatement, tout comme **Créer une sauvegarde** en haut de la page.
Depuis un script ou une tâche cron de l'hôte, la ligne de commande fait la
même sauvegarde :

```bash
docker exec evey-server evey admin backup
```

Elle suit le réglage de télémétrie du panneau ; `--include-telemetry` inclut
la télémétrie pour cette exécution seulement.

<figure class="evey-shot" markdown="span">
  ![Admin → Sauvegarde BDD et restauration avec une planification quotidienne](images/admin-backup.webp){ loading=lazy }
  <figcaption>Une sauvegarde chaque jour à 3 h, conservée 30 jours.</figcaption>
</figure>

Indépendamment de la planification, le serveur sauvegarde aussi la base avant
d'appliquer une migration lors d'une mise à jour (voir
[Mettre à jour Evey](#updating-evey)).

## Restaurer une sauvegarde

**Admin → Sauvegarde BDD et restauration** liste les sauvegardes et en
restaure une, ou restaure un fichier que vous téléversez (**Restaurer depuis
un fichier**). La restauration remplace la base actuelle.

!!! warning "Même schéma uniquement"
    Le serveur vérifie d'abord l'empreinte et la version du schéma, et refuse
    une sauvegarde prise par une version dont le schéma diffère plutôt que de
    la charger. Pour revenir à une version plus ancienne avec ses données,
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
elle écrit d'abord un dump dans `./evey-data/backups/pre-migration-*.sql.gz`
(les trois plus récents sont conservés), puis applique les migrations au
démarrage ; sur un gros catalogue, cela peut prendre quelques minutes, et le
conteneur est de nouveau signalé en bonne santé dès que le serveur répond.
Evey demande toutes les heures au registre si une image plus récente est
publiée et le signale dans le panneau d'administration ; il ne se met jamais à
jour tout seul.

**Figez ce que vous exécutez.** Pendant la bêta, il n'y a ni version
numérotée ni tag `latest` ; le tag de branche `beta-0.0.1` bouge à chaque
modification. Pour un serveur sur lequel vous comptez, mettez un tag immuable
`sha-<commit>` sur la ligne `image:`, pour que deux `pull` à un jour d'écart
exécutent le même code. Quand des versions existeront, `latest` suivra la
dernière version stable, et un tag de version (`0.2.0`) permettra de la figer.

**Savoir quelle version tourne.** `docker exec evey-server evey version`
affiche la version et le commit du binaire en cours d'exécution.

### Revenir en arrière

Remettez le tag précédent sur la ligne `image:` et lancez
`docker compose up -d`. Si la version plus récente avait migré la base,
restaurez d'abord son dump d'avant migration :
`docker exec evey-server evey admin rollback` affiche les étapes exactes avec
le chemin du dump.

Les tags d'image, les variantes de runtime et les familles de GPU sont
décrits dans [Déploiements avancés](operations/advanced-deployments.md#image-tags-and-variants).

<!-- sources
contents: internal/app/backup/service.go - BackupTables, CreateBackup (sha256 + schema version in manifest), RestoreBackup (checksum + schema guard), CleanupOldBackups ; internal/adapters/driven/filesystem/backup_store.go
schedule: i18n admin.backup.schedule.* (cronLabel, retentionLabel, runNow) ; admin.backup.createBackup, restoreFromFile ; cmd/evey/main.go (DB cron scheduler, env fallback)
telemetry left out by default, emptied on restore: internal/adapters/driven/postgres/backup.go - DumpAll, telemetryTables ; internal/app/backup/service.go - WithIncludeTelemetry ; internal/adapters/driving/http/settings_handler.go - BackupIncludeTelemetryKey ; i18n admin.serverSettings.backupTelemetry, backupTelemetryHint
command-line backup: cmd/evey/admin_backup.go - adminBackup (--include-telemetry, else the panel setting)
keys outside the dump: internal/config/config.go (EVEY_JWT_PRIVATE_KEY_PATH /data/jwt.key, /data/keys)
pre-migration dump at boot, 3 kept: cmd/evey/database.go - RunMigrations BackupOpts Retention 3 ; internal/adapters/driven/postgres/migrate_backup.go (pre-migration-<stamp>.sql.gz, EVEY_BACKUP_DIR default /data/backups)
update check hourly, never self-updates: internal/app/updatecheck/service.go - checkInterval, firstCheck
rollback advisor: cmd/evey/admin_commands.go - adminRollback ; image tags: .github/workflows/ci.yml (beta-0.0.1 + sha-<short>, latest gated on vX.Y.Z)
-->
