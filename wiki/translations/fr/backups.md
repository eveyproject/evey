---
source_sha: 3529db45dbbd12f6980bca0989017583b6ccad6c3c5a3b54597887c6b09d6285
reviewed: true
---
# Sauvegardes et mises à jour

Deux habitudes gardent votre serveur à l'abri :

- **Sauvegarder.** Activez une sauvegarde quotidienne de la base de données,
  et gardez une copie du dossier `evey-data` ailleurs.
- **Mettre à jour.** Une commande va chercher la nouvelle version. Evey
  sauvegarde d'abord la base de données, tout seul, chaque fois qu'une mise
  à jour la modifie.

Cette page montre les deux, puis ce que contient une sauvegarde, comment en
restaurer une, et comment revenir à une version plus ancienne.

## Planifier les sauvegardes

1. Allez dans **Administration → Sauvegarde BDD et restauration**, carte
   **Planification**.
2. Dans **Expression cron**, dites quand la lancer. `0 3 * * *` veut dire
   tous les jours à 3 h du matin. (Le format est : minute, heure, jour, mois,
   jour de la semaine.)
3. Dans **Rétention (jours)**, dites combien de temps garder chaque
   sauvegarde, par exemple `30`. Les sauvegardes plus anciennes sont
   supprimées après chaque sauvegarde planifiée.
4. Cochez **Activé**, puis appuyez sur **Enregistrer**.

**Lancer maintenant** fait une sauvegarde tout de suite, tout comme **Créer
une sauvegarde** en haut de la page.

<figure class="evey-shot" markdown="span">
  ![Administration → Sauvegarde BDD et restauration avec une sauvegarde quotidienne](images/admin-backup.webp){ loading=lazy }
  <figcaption>Une sauvegarde chaque jour à 3 h du matin, gardée 30 jours.</figcaption>
</figure>

Les sauvegardes sont rangées dans `./evey-data/backups`. C'est à l'intérieur
du dossier que vous devriez de toute façon copier sur un autre disque (voir
[Ce que contient une sauvegarde](#what-a-backup-contains)).

??? info "En ligne de commande"

    Un script ou une tâche cron sur la machine peut faire la même
    sauvegarde :

    ```bash
    docker exec evey-server evey admin backup
    ```

    Elle suit le réglage de télémétrie du panneau (plus bas) ;
    `--include-telemetry` ajoute la télémétrie pour cette fois seulement.

Le serveur sauvegarde aussi la base de données tout seul avant qu'une mise à
jour la modifie, quelle que soit la planification (voir
[Mettre à jour Evey](#updating-evey)).

## Ce que contient une sauvegarde

Une sauvegarde est une copie de la **base de données** : comptes, profils,
bibliothèques et leur catalogue, historique de visionnage et points de
reprise, collections, playlists et favoris, réglages, informations des
plugins, webhooks, clés API, notifications et journaux d'audit.

Elle ne contient **pas** :

- vos fichiers de médias ;
- le cache vidéo (il se remplit de nouveau tout seul) ;
- l'index de recherche (reconstruit à partir de la base de données) ;
- les clés gardées dans `./evey-data` : `jwt.key`, qui protège toutes les
  connexions, et le dossier `keys` (mot de passe de la base de données, clé
  de recherche) ;
- les fichiers des plugins installés.

**Gardez donc aussi une copie de tout le dossier `./evey-data`.** Une
sauvegarde de la base de données remet Evey en état sur la même
installation. Pour reconstruire une machine à partir de rien, il vous faut le
dossier.

??? info "La télémétrie, et le format du fichier"

    Les mesures de lecture et du serveur (derrière les graphiques de qualité
    et de charge) sont laissées de côté par défaut : ce sont elles qui
    grossissent le plus vite, et elles ne disent rien de votre catalogue.
    **Inclure la télémétrie dans les sauvegardes** (**Administration →
    Configuration du serveur**) les remet dedans, pour les sauvegardes
    manuelles, planifiées et en ligne de commande. Restaurer une sauvegarde
    vide toujours ces tables.

    Une sauvegarde est un fichier `.tar.gz` qui contient l'export de la base
    de données et une courte description avec son empreinte (SHA256) et sa
    version de schéma.

## Restaurer une sauvegarde

**Administration → Sauvegarde BDD et restauration** liste les sauvegardes.
Restaurez-en une depuis la liste, ou envoyez un fichier avec **Restaurer
depuis un fichier**. Restaurer remplace la base de données actuelle : tout
ce qui a changé depuis cette sauvegarde est perdu.

!!! warning "Seulement une sauvegarde d'une version avec la même organisation de base"
    Le serveur vérifie d'abord le fichier et l'organisation de la base de
    données (la version du schéma). Il refuse une sauvegarde faite par une
    version dont l'organisation est différente, au lieu de la charger. Pour
    revenir à une version plus ancienne avec ses données, voir
    [Revenir en arrière](#going-back).

Si le serveur ne démarre plus, la ligne de commande peut restaurer les copies
de secours faites avant une mise à jour (`pre-migration-*.sql.gz`), pas les
sauvegardes `.tar.gz` de ce panneau : voir [Restauration](runbooks/recovery.md).

## Mettre à jour Evey

Dans le dossier qui contient votre fichier compose :

```bash
docker compose pull && docker compose up -d
docker compose logs -f evey
```

C'est toute la mise à jour.

- **Vos données sont sauvegardées d'abord.** Quand la nouvelle version doit
  modifier la base de données, elle en écrit d'abord une copie dans
  `./evey-data/backups/pre-migration-*.sql.gz` (les trois plus récentes sont
  gardées), puis fait le changement au démarrage.
- **Cela peut prendre quelques minutes** sur un gros catalogue. Le conteneur
  est de nouveau signalé en bonne santé dès qu'il répond.
- **Evey vous dit quand une mise à jour existe.** Il vérifie toutes les
  heures si une image plus récente est publiée, et le signale dans le
  panneau d'administration. Il ne se met jamais à jour tout seul.
- **Quelle version tourne :** `docker exec evey-server evey version` affiche
  la version et le commit.

??? info "Fixer exactement la version que vous faites tourner"

    Pendant la bêta, il n'y a ni version numérotée ni tag `latest`. Le tag
    `beta-0.0.1` change à chaque mise à jour. Pour un serveur sur lequel vous
    comptez, mettez un tag fixe `sha-<commit>` sur la ligne `image:`, pour
    que deux `pull` à un jour d'écart fassent tourner le même code. Quand des
    versions existeront, `latest` suivra la dernière version stable, et un
    tag de version (`0.2.0`) permettra de la fixer.

### Revenir en arrière

1. Remettez le tag précédent sur la ligne `image:` de votre fichier compose.
2. Si la version plus récente a modifié la base de données, restaurez
   d'abord sa copie d'avant la mise à jour :
   `docker exec evey-server evey admin rollback` affiche les étapes exactes,
   avec le chemin du fichier.
3. Lancez `docker compose up -d`.

Les tags d'image, les autres variantes et les versions pour carte graphique
sont décrits dans
[Déploiements avancés](operations/advanced-deployments.md#image-tags-and-variants).

<!-- sources
contents: internal/app/backup/service.go - BackupTables, CreateBackup (sha256 + schema version in manifest), RestoreBackup (checksum + schema guard), CleanupOldBackups ; internal/adapters/driven/filesystem/backup_store.go
schedule: i18n admin.backup.schedule.* (cronLabel, retentionLabel, runNow) ; admin.backup.createBackup, restoreFromFile ; cmd/evey/main.go (DB cron scheduler, env fallback)
telemetry left out by default, emptied on restore: internal/adapters/driven/postgres/backup.go - DumpAll, telemetryTables ; internal/app/backup/service.go - WithIncludeTelemetry ; internal/adapters/driving/http/settings_handler.go - BackupIncludeTelemetryKey ; i18n admin.serverSettings.backupTelemetry, backupTelemetryHint
command-line backup: cmd/evey/admin_backup.go - adminBackup (--include-telemetry, else the panel setting)
keys outside the dump: internal/config/config.go (EVEY_JWT_PRIVATE_KEY_PATH /data/jwt.key, /data/keys)
pre-migration dump at boot, 3 kept: cmd/evey/database.go - RunMigrations BackupOpts Retention 3 ; internal/adapters/driven/postgres/migrate_backup.go (pre-migration-<stamp>.sql.gz, EVEY_BACKUP_DIR default /data/backups)
update check hourly, never self-updates: internal/app/updatecheck/service.go - checkInterval, firstCheck
rollback advisor: cmd/evey/admin_commands.go - adminRollback ; image tags: .github/workflows/ci.yml (beta-0.0.1 + sha-<short>, latest gated on vX.Y.Z)
schedule card controls (Enabled, Save, Run now): frontend/apps/web/src/routes/_auth/admin/backup.lazy.tsx ; i18n admin.backup.schedule.enabledLabel, save, cronHelp
-->
