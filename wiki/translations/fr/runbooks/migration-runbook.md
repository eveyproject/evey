---
source_sha: 6f985b91384f5a0566bd38dfa9962efdfce4a6b07d1697db8a21ab17c08d0469
reviewed: true
---
# Migrations de la base

Comment Evey applique ses migrations de schéma, et comment les manipuler sans
risque.

## Comment les migrations s'exécutent

Les migrations sont **embarquées** (`go:embed`) et exécutées automatiquement au
démarrage du serveur par `internal/adapters/driven/postgres/migrate.go`, avec
`golang-migrate/migrate/v4`. Source :
`internal/adapters/driven/postgres/migrations/NNNNNN_<nom>.{up,down}.sql`.

Les migrations s'appliquent automatiquement au démarrage d'`evey-server`. Il
existe **aussi** une CLI pour les piloter à la main : `evey admin migrate up |
down | version` (dans `cmd/evey/admin.go` ; `version` affiche la version
actuelle du schéma et sort en erreur 1 si la base est marquée « dirty »). Les
cibles `migrate-up` / `migrate-down` du `Makefile` l'enveloppent via
`./cmd/evey admin migrate`. Un `pg_dump` d'avant migration est pris
automatiquement quand `EVEY_BACKUP_DIR` est défini (`--no-backup` pour s'en
passer).

L'état est suivi dans la table `schema_migrations` (gérée par golang-migrate) :
`{version, dirty}`.

## Avant une migration

1. **Sauvegardez la base.** Evey a un point d'accès de sauvegarde intégré (voir
   `docs/backups.md`) :
   ```sh
   curl -X POST -H "Authorization: Bearer <admin-token>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   Ou lancez `pg_dump` directement dans le conteneur :
   ```sh
   docker exec evey-postgres pg_dump -U "$EVEY_DB_USER" "$EVEY_DB_NAME" \
     | gzip > backup-$(date -u +%Y%m%dT%H%M%SZ).sql.gz
   ```
   Vérifiez que le dump n'est pas vide et que `gunzip -t` passe.

2. **Regardez la taille des tables** pour toute migration qui touche de grosses
   tables (media_items, watch_progress, tracks, hls_sessions) :
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

3. **Regardez les sessions actives et les verrous** avant une migration lourde
   en DDL :
   ```sql
   SELECT pid, state, wait_event_type, wait_event,
          now() - query_start AS duration, left(query, 120)
     FROM pg_stat_activity
    WHERE datname = current_database()
      AND pid <> pg_backend_pid()
    ORDER BY query_start;
   ```
   Terminez les sessions longues ou « idle in transaction »
   (`SELECT pg_terminate_backend(pid)`) avant une migration qui prend des verrous
   forts.

4. **Vérifiez la version actuelle** :
   ```sql
   SELECT version, dirty FROM schema_migrations;
   ```
   Si `dirty = true`, suivez la section de récupération avant toute autre
   chose.

## Lancer une migration

Les migrations s'exécutent au démarrage du serveur. Pour appliquer celles en
attente :

```sh
docker compose pull evey       # si vous déployez une nouvelle image
docker compose up -d evey
docker logs -f evey-server     # suivre le journal de démarrage
```

Une migration réussie : aucune erreur de migration, et le serveur répond 200 sur
`/healthz`. Pour vérifier :

```sh
curl -sf http://localhost:9091/healthz && echo OK
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" \
  -c "SELECT version, dirty FROM schema_migrations;"
```

## Revenir en arrière

Revenez d'une étape avec la cible prévue :

```sh
make migrate-down     # enveloppe `evey admin migrate down`, revient EXACTEMENT d'une étape
make migrate-version  # vérifier où vous êtes arrivé
```

Répétez-la une fois par étape pour remonter plus loin. Un `pg_dump` d'avant
migration est pris automatiquement quand `EVEY_BACKUP_DIR` est défini.

Autre possibilité : **redéployez l'image précédente**, dont les migrations
embarquées s'arrêtent à la version N-1.

> Ne modifiez pas `schema_migrations` à la main. Une version antérieure de ce
> runbook affirmait qu'aucune cible `migrate-down` n'existait et demandait
> d'appliquer le `.down.sql` avec `psql`, puis de faire à la main
> `UPDATE schema_migrations SET version = ...`, en plein incident. Elle
> contredisait la section « Comment les migrations s'exécutent », neuf lignes
> plus haut. La cible a toujours existé (`Makefile:314`). Ne passez à la
> chirurgie manuelle que si la base est marquée `dirty` et que `migrate down`
> refuse de tourner, et faites d'abord un dump.

Restaurez toujours la sauvegarde d'avant migration si des changements destructeurs
de données ont été livrés (DROP COLUMN, changements de type) et que la
sauvegarde est plus récente que le moment où le trafic de production a divergé.

## Récupération : état « dirty »

Si le journal de démarrage affiche `Dirty database version N. Fix and force
version.`, la dernière migration a planté à mi-chemin. Corrigez le schéma à la
main pour atteindre l'état voulu, puis effacez le drapeau :

```sql
-- Inspecter : que dit schema_migrations actuellement ?
SELECT version, dirty FROM schema_migrations;

-- Réappliquez à la main les instructions du up.sql, ou revenez en arrière avec le down.sql.
-- Puis effacez le drapeau dirty à la version réellement atteinte :
UPDATE schema_migrations SET version = <N>, dirty = false;
```

Redémarrez `evey-server`. golang-migrate reprend à partir de là.

## Cas particuliers

Migrations récentes, pour référence (voir
`internal/adapters/driven/postgres/migrations/`) :

- **`000066_cascade_and_email_unique.up.sql`** : `ALTER TABLE ... DROP
  CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`. Prend un `ACCESS EXCLUSIVE`
  sur les deux tables pendant tout l'ADD. Anodin sur de petites tables, pénible
  sur `media_items` / `watch_progress`. Méthode pour les grosses tables : ajouter
  avec `NOT VALID`, puis `VALIDATE CONSTRAINT` dans une seconde instruction (la
  validation ne prend qu'un `SHARE UPDATE EXCLUSIVE`).
- **`000067_sort_title_index.up.sql`, `000068_watch_progress_composite_index.up.sql`,
  `000069_quick_connect_shortcode_index.up.sql`** : de simples `CREATE INDEX`.
  Sur de grosses tables, ils bloquent les écritures. Envisagez un
  `CREATE INDEX CONCURRENTLY` à la main pendant une fenêtre de maintenance
  (impossible dans une transaction de migration : appliquez-le à part, puis
  enregistrez la version).
- **`000072_syncplay_queue.up.sql`** : `ALTER TABLE ... ADD COLUMN ... NOT NULL
  DEFAULT '[]'`. Sur PostgreSQL 11 et plus, c'est une simple modification de
  métadonnées, rapide. Elle prend quand même brièvement un `ACCESS EXCLUSIVE`.

Règles :

- **ALTER long** qui réécrit la table (changement de type, ajout d'une valeur
  par défaut non constante sur un vieux PostgreSQL) : vérification des verrous
  obligatoire, prévoyez une interruption.
- **DROP COLUMN** : seulement des métadonnées, mais la colonne ne se récupère
  pas sans sauvegarde. Ne supprimez jamais une colonne dans la version même qui
  déploie le nouveau code : préférez deux temps, arrêter de lire la colonne,
  déployer, puis la supprimer à la version suivante.
- **Clé étrangère sur de grosses tables** : `ADD CONSTRAINT ... NOT VALID`, puis
  `VALIDATE CONSTRAINT` dans une migration suivante. Évitez un seul
  `ADD CONSTRAINT ... FOREIGN KEY ...` bloquant.
- **UPDATE massif ou remplissage** : jamais dans une seule migration sur une
  grosse table. Livrez une migration de schéma sans effet et une tâche de fond.
- **Remplacement de colonne (expand → migrate → contract)** : ne regroupez pas
  `ADD COLUMN` + remplissage + `SET NOT NULL` + `DROP COLUMN` de l'ancienne dans
  une seule migration. Cette forme ne fonctionne que parce qu'Evey tourne
  aujourd'hui en instance unique, avec des migrations appliquées de façon
  synchrone au démarrage : pendant la fenêtre, l'ancien code qui lit la colonne
  supprimée et le nouveau qui lit la nouvelle ne peuvent pas coexister, si bien
  qu'un déploiement progressif ou à plusieurs répliques renverrait des erreurs
  depuis la moitié en retard.
  Migrations connues écrites ainsi, laissées telles quelles parce qu'elles sont
  déjà livrées : `000077` et `000098_playlists_profile_id.up.sql` (ajoute
  `profile_id`, le remplit, pose `NOT NULL`, supprime `user_id`).
  Pour une nouvelle, répartissez sur trois versions :
  1. **expand** : ajouter la nouvelle colonne, nullable, écrire dans *les deux*
     colonnes, continuer de lire l'ancienne ;
  2. **migrate** : remplir la nouvelle colonne (tâche de fond, pas la
     migration), puis basculer les lectures dessus ;
  3. **contract** : quand plus aucun code déployé ne touche l'ancienne colonne,
     `SET NOT NULL` et la supprimer.
  Chaque étape se déploie et s'annule seule, ce que la version en un seul
  fichier ne permet pas.

## Après la migration

```sh
curl -sf http://localhost:9091/healthz
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" \
  -c "SELECT version, dirty FROM schema_migrations;"
docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
```

Testez rapidement quelques points d'accès touchés par la migration
(`/api/v1/...`).
