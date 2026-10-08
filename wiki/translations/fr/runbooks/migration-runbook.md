---
source_sha: fad8d9ff7aa0e8e4c14508e1eb2fe716e13c05c66eca9acb763084c56aec898f
reviewed: true
---
# Migrations de la base

Comment Evey fait évoluer le schéma de sa base lors d'une mise à jour, quoi
vérifier avant et après, et la sortie de secours quand une migration s'arrête
à mi-chemin. Pour les opérateurs de l'installation standard, où la base tourne
dans le conteneur `evey-server` ; les contributeurs qui écrivent une migration
trouveront les règles en fin de page.

## Comment les migrations s'exécutent

Les migrations sont embarquées dans le binaire et appliquées par
`golang-migrate` au démarrage du serveur. L'état tient dans la table
`schema_migrations`, une ligne `{version, dirty}`. Avant d'appliquer quoi que
ce soit, le serveur écrit un dump complet dans `EVEY_BACKUP_DIR`
(`/data/backups`, donc `./evey-data/backups` sur l'hôte), nommé
`pre-migration-<version>-<horodatage UTC>.sql.gz`, et conserve le dump le plus
récent de chacune des 3 dernières versions du schéma.

Les mêmes opérations existent à la main :

```sh
docker exec evey-server evey admin migrate version   # sort en 1 quand le schéma est « dirty »
docker exec evey-server evey admin migrate up        # applique toutes les migrations en attente
docker exec evey-server evey admin migrate down      # revient exactement d'un cran
```

`up` et `down` prennent d'abord un dump ; `--no-backup` le saute. Pour
ouvrir un shell SQL sur la base embarquée :

```sh
docker exec -it evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

## Mettre à jour sans risque

1. **Prenez une sauvegarde** dans **Admin -> Sauvegarde BDD et restauration**
   (**Lancer maintenant**), ou depuis un script :
   ```sh
   curl -X POST -H "Authorization: Bearer <jeton-admin>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   Le dump automatique de pré-migration est une seconde copie, pas un
   remplacement.

2. **Vérifiez la version courante** : `docker exec evey-server evey admin
   migrate version` affiche `schema version: N`. S'il affiche `DIRTY`, passez
   par [Une migration arrêtée à mi-chemin](#a-migration-that-stopped-halfway)
   avant de mettre à jour.

3. **Sur un gros catalogue**, regardez les plus grosses tables : une migration
   qui en réécrit une prend plus de temps, et le healthcheck du conteneur
   accorde 300 secondes de démarrage pour cela.
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

4. **Démarrez la nouvelle image** et suivez le journal jusqu'à ce que les
   migrations soient appliquées et que le serveur réponde :
   ```sh
   docker compose pull && docker compose up -d
   docker logs -f evey-server
   ```
   Un bon résultat : une ligne `migrations: pre-migration backup written`
   quand il y avait quelque chose à appliquer, puis la bannière du serveur,
   puis `curl -sf http://localhost:9091/healthz` répond.

5. **Ensuite**, confirmez la version et cherchez les erreurs :
   ```sh
   docker exec evey-server evey admin migrate version
   docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
   ```
   Videz le cache de transcodage (**Admin -> Cache de transcodage -> Tout
   vider**) si la version change la façon dont les vidéos sont encodées.

## Revenir à la version précédente

`docker exec evey-server evey admin rollback` affiche la version en cours, la
version du schéma, le dump de pré-migration le plus récent et les commandes
exactes à lancer sur l'hôte. Il ne change rien : le conteneur n'a pas accès à
Docker.

Le retour habituel est l'image précédente plus le dump pris avant la mise à
jour : voir [Revenir en arrière](../backups.md#going-back). Restaurez le dump
plutôt que de migrer vers le bas quand la mise à jour a supprimé des données
(une colonne retirée, un changement de type). `evey admin migrate down`
revient d'une migration à la fois, quand le `.down.sql` de ce cran suffit.

!!! warning
    Ne modifiez pas `schema_migrations` à la main, sauf pour effacer un état
    « dirty » (ci-dessous), et prenez un dump avant.

## Une migration arrêtée à mi-chemin

Quand le journal montre `Dirty database version N. Fix and force version.`,
une migration a planté en cours de route et `golang-migrate` refuse de
continuer. Amenez le schéma dans l'état voulu, puis effacez le drapeau :

```sql
SELECT version, dirty FROM schema_migrations;

-- Réappliquez à la main le reste du up.sql, ou annulez avec le down.sql,
-- puis enregistrez la version réellement atteinte :
UPDATE schema_migrations SET version = <N>, dirty = false;
```

Redémarrez le conteneur ; les migrations reprennent de là. Restaurer le dump
de pré-migration ([Récupération](recovery.md#restore-a-pre-migration-dump))
est l'autre issue.

## Écrire une migration

Pour les contributeurs. Les fichiers vivent dans
`internal/adapters/driven/postgres/migrations/NNNNNN_<nom>.{up,down}.sql`.
Exemples déjà livrés :

- **`000066_cascade_and_email_unique.up.sql`** - `ALTER TABLE ... DROP
  CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`. Prend `ACCESS EXCLUSIVE` sur
  les deux tables pendant tout le ADD. Bon marché sur de petites tables,
  pénible sur `media_items` / `watch_progress`.
- **`000067_sort_title_index.up.sql`**,
  **`000068_watch_progress_composite_index.up.sql`**,
  **`000069_quick_connect_shortcode_index.up.sql`** - de simples `CREATE
  INDEX`, qui bloquent les écritures sur les grosses tables. `CREATE INDEX
  CONCURRENTLY` ne peut pas tourner dans une transaction de migration.
- **`000072_syncplay_queue.up.sql`** - `ALTER TABLE ... ADD COLUMN ... NOT
  NULL DEFAULT '[]'`. Métadonnées seules sur PG 11+, mais tient quand même
  `ACCESS EXCLUSIVE` brièvement.

Règles :

- **Un long ALTER** qui réécrit la table (changement de type, valeur par
  défaut non constante sur un vieux PG) signifie une interruption sur un gros
  catalogue.
- **DROP COLUMN** ne touche que les métadonnées, mais la colonne ne revient
  pas sans sauvegarde. Ne la supprimez jamais dans la version qui cesse de la
  lire : cesser de lire, déployer, supprimer dans la version suivante.
- **Clé étrangère sur une grosse table** : `ADD CONSTRAINT ... NOT VALID`,
  puis `VALIDATE CONSTRAINT` dans une migration de suivi (la validation ne
  prend que `SHARE UPDATE EXCLUSIVE`).
- **UPDATE par lots / remplissage** : jamais dans une seule migration sur une
  grosse table. Livrez une migration de schéma vide plus une tâche de fond.
- **Remplacement de colonne (étendre, migrer, contracter)** : ne repliez pas
  `ADD COLUMN` + remplissage + `SET NOT NULL` + `DROP COLUMN` dans un seul
  fichier. Cette forme ne marche que parce qu'Evey tourne en instance unique,
  avec des migrations appliquées de façon synchrone au démarrage ; un
  déploiement progressif servirait des erreurs depuis la moitié en retard.
  `000077` et `000098_playlists_profile_id.up.sql` ont été écrites ainsi et
  restent telles quelles parce qu'elles sont livrées. Pour une nouvelle,
  découpez sur trois versions :
  1. **étendre** - ajouter la nouvelle colonne nullable, écrire les deux
     colonnes, continuer à lire l'ancienne ;
  2. **migrer** - remplir la nouvelle colonne (tâche de fond, pas la
     migration), puis basculer les lectures dessus ;
  3. **contracter** - quand plus aucun code déployé ne touche l'ancienne
     colonne, `SET NOT NULL` et la supprimer.

<!-- sources
migrations: internal/adapters/driven/postgres/migrate.go - RunMigrations, RollbackLastMigration, CurrentMigrationVersion ; internal/adapters/driven/postgres/migrate_backup.go - dumpDatabase, backupFilePrefix, pruneOldBackups, defaultBackupRetention
cli: cmd/evey/admin.go - adminMigrate, adminMigrateUsage ; cmd/evey/admin_ops.go - adminRollback
embedded database and healthcheck start period: docker/entrypoint.sh (postgres.password, EVEY_DB_PASSWORD_FILE) ; Dockerfile - HEALTHCHECK --start-period
backup route: internal/adapters/driving/http/backup_handler.go (POST /api/v1/admin/backups)
labels: i18n/fr/admin.yaml - sidebar.backupRestore, backup.schedule.runNow, transcodeCache.clearAll
-->
