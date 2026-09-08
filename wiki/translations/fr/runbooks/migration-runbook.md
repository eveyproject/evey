---
source_sha: 6f985b91384f5a0566bd38dfa9962efdfce4a6b07d1697db8a21ab17c08d0469
---

# Runbook des migrations

Comment Evey exécute les migrations de schéma, et comment intervenir dessus sans casse.

## Comment les migrations s'exécutent

Les migrations sont **embarquées** (`go:embed`) et exécutées automatiquement au démarrage du serveur depuis `internal/adapters/driven/postgres/migrate.go`, via `golang-migrate/migrate/v4`. Source : `internal/adapters/driven/postgres/migrations/NNNNNN_<name>.{up,down}.sql`.

Les migrations s'appliquent toutes seules au démarrage d'`evey-server`. Il existe **aussi** une CLI pour reprendre la main : `evey admin migrate up | down | version` (implémentée dans `cmd/evey/admin.go` ; `version` affiche la version de schéma courante et sort en 1 si la base est marquée sale). Les cibles `migrate-up` et `migrate-down` du `Makefile` l'enveloppent par `./cmd/evey admin migrate`. Un `pg_dump` d'avant migration est pris automatiquement dès qu'`EVEY_BACKUP_DIR` est renseignée (passez `--no-backup` pour l'éviter).

L'état est suivi dans la table `schema_migrations`, gérée par golang-migrate : `{version, dirty}`.

## Liste de contrôle avant migration

1. **Sauvegardez la base.** Evey embarque un point d'accès de sauvegarde (voir `docs/backups.md`) :
   ```sh
   curl -X POST -H "Authorization: Bearer <admin-token>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   Ou lancez `pg_dump` directement contre le conteneur :
   ```sh
   docker exec evey-postgres pg_dump -U "$EVEY_DB_USER" "$EVEY_DB_NAME" \
     | gzip > backup-$(date -u +%Y%m%dT%H%M%SZ).sql.gz
   ```
   Vérifiez que la taille du dump n'est pas nulle et que `gunzip -t` passe.

2. **Regardez la taille des tables** pour toute migration qui touche à de grosses tables (media_items, watch_progress, tracks, hls_sessions) :
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

3. **Regardez les sessions actives et les verrous** avant une migration lourde en DDL :
   ```sql
   SELECT pid, state, wait_event_type, wait_event,
          now() - query_start AS duration, left(query, 120)
     FROM pg_stat_activity
    WHERE datname = current_database()
      AND pid <> pg_backend_pid()
    ORDER BY query_start;
   ```
   Tuez les sessions longues ou restées inactives dans une transaction (`SELECT pg_terminate_backend(pid)`) avant de lancer une migration qui prend des verrous forts.

4. **Regardez la version de migration courante** :
   ```sql
   SELECT version, dirty FROM schema_migrations;
   ```
   Si `dirty = true`, suivez la section Récupération avant toute autre chose.

## Lancer une migration

Les migrations s'exécutent au démarrage du serveur. Pour appliquer celles qui sont en attente :

```sh
docker compose pull evey       # si vous déployez une nouvelle image
docker compose up -d evey
docker logs -f evey-server     # surveillez le journal de démarrage
```

Exécution réussie : aucune erreur de migrate, et le serveur répond 200 sur `/healthz`. Vérifiez :

```sh
curl -sf http://localhost:9091/healthz && echo OK
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" \
  -c "SELECT version, dirty FROM schema_migrations;"
```

## Retour arrière

Reculez d'un cran avec la cible prévue pour cela :

```sh
make migrate-down     # enveloppe `evey admin migrate down`, recule d'EXACTEMENT un cran
make migrate-version  # confirmez où vous êtes retombé
```

Répétez la commande une fois par cran pour reculer davantage. Un `pg_dump`
d'avant migration est pris automatiquement dès qu'`EVEY_BACKUP_DIR` est
renseignée.

L'autre voie : **redéployez l'image précédente**, dont les migrations embarquées
s'arrêtent à la version N-1.

> N'éditez jamais `schema_migrations` à la main. Une version antérieure de ce
> runbook affirmait qu'aucune cible `migrate-down` n'existait et enjoignait aux
> exploitants d'appliquer le `.down.sql` avec `psql` puis de faire un
> `UPDATE schema_migrations SET version = ...` à la main - en plein incident.
> Elle contredisait la section « Comment les migrations s'exécutent », neuf
> lignes plus haut. La cible a toujours existé (`Makefile:314`). Ne sortez le
> scalpel que si la base est marquée `dirty` et que `migrate down` refuse de
> tourner, et prenez un dump d'abord.

Restaurez toujours la sauvegarde d'avant migration si des changements destructeurs de données sont passés (DROP COLUMN, changement de type) et que cette sauvegarde est plus fraîche que la divergence du trafic de production.

## Récupération - l'état « dirty »

Si le journal de démarrage affiche `Dirty database version N. Fix and force version.`, c'est que la dernière migration s'est écrasée en cours de route. Remettez le schéma à la main dans l'état voulu, puis effacez le drapeau :

```sql
-- Inspection : que rapporte schema_migrations en ce moment ?
SELECT version, dirty FROM schema_migrations;

-- Rejouez à la main les instructions du up.sql, ou revenez en arrière par le down.sql.
-- Puis effacez le bit dirty, à la version que vous avez réellement atteinte :
UPDATE schema_migrations SET version = <N>, dirty = false;
```

Redémarrez `evey-server` : golang-migrate reprendra à partir de là.

## Cas particuliers

Quelques migrations récentes, à titre de référence (voir `internal/adapters/driven/postgres/migrations/`) :

- **`000066_cascade_and_email_unique.up.sql`** - `ALTER TABLE ... DROP CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`. Prend un `ACCESS EXCLUSIVE` sur les deux tables pendant toute la durée de l'ADD. Sans conséquence sur de petites tables, douloureux sur `media_items` ou `watch_progress`. Le motif à employer sur une grosse table : ajouter en `NOT VALID`, puis `VALIDATE CONSTRAINT` dans une seconde instruction (la validation ne prend qu'un `SHARE UPDATE EXCLUSIVE`).
- **`000067_sort_title_index.up.sql`, `000068_watch_progress_composite_index.up.sql`, `000069_quick_connect_shortcode_index.up.sql`** - de simples `CREATE INDEX`. Sur une grosse table, cela bloque les écritures. Envisagez un `CREATE INDEX CONCURRENTLY` à la main, pendant une fenêtre de maintenance (impossible dans la transaction d'une migration : appliquez-le hors bande, puis enregistrez la version).
- **`000072_syncplay_queue.up.sql`** - `ALTER TABLE ... ADD COLUMN ... NOT NULL DEFAULT '[]'`. À partir de PG 11, l'opération ne touche que les métadonnées et va vite. Elle tient tout de même brièvement un `ACCESS EXCLUSIVE`.

Les règles à suivre :

- **ALTER long** qui réécrit la table (changement de type, ajout d'une valeur par défaut non constante sur un vieux PG) → contrôle des verrous obligatoire au préalable, et attendez-vous à une interruption de service.
- **DROP COLUMN** → ne touche que les métadonnées, mais la colonne ne se restaure que depuis une sauvegarde. Ne supprimez jamais une colonne dans la version qui déploie le nouveau code : préférez deux temps, cessez de lire la colonne, déployez, puis supprimez-la à la version suivante.
- **Clé étrangère sur une grosse table** → `ADD CONSTRAINT ... NOT VALID`, puis `VALIDATE CONSTRAINT` dans une migration ultérieure. Évitez un unique `ADD CONSTRAINT ... FOREIGN KEY ...` bloquant.
- **UPDATE en lot ou remplissage rétroactif** → jamais dans une seule migration sur une grosse table. Livrez une migration de schéma sans effet, plus une tâche de fond.
- **Remplacement de colonne (étendre → migrer → resserrer)** → ne repliez pas `ADD COLUMN`, remplissage, `SET NOT NULL` et `DROP COLUMN` de l'ancienne dans une même migration. Cette forme ne tient que parce qu'Evey tourne aujourd'hui en instance unique, migrations appliquées de façon synchrone au démarrage : pendant la fenêtre, l'ancien code qui lit la colonne supprimée et le nouveau qui lit la nouvelle ne peuvent pas coexister, si bien qu'un déploiement progressif ou à plusieurs répliques servirait des erreurs depuis celle des deux moitiés qui est en retard.
  Les migrations connues écrites ainsi sont conservées telles quelles, puisque déjà livrées : `000077` et `000098_playlists_profile_id.up.sql` (qui ajoute `profile_id`, le remplit, le passe en `NOT NULL`, puis supprime `user_id`).
  Pour une nouvelle, découpez sur trois versions :
  1. **étendre** - ajoutez la nouvelle colonne, nullable, écrivez dans les *deux* colonnes, continuez de lire l'ancienne ;
  2. **migrer** - remplissez la nouvelle colonne (par une tâche de fond, pas par la migration), puis basculez les lectures dessus ;
  3. **resserrer** - une fois qu'aucun code déployé ne touche plus l'ancienne colonne, passez la nouvelle en `SET NOT NULL` et supprimez l'ancienne.
  Chaque étape se déploie et se défait indépendamment, ce que la version en un seul fichier ne permet pas.

## Contrôles après migration

```sh
curl -sf http://localhost:9091/healthz
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" \
  -c "SELECT version, dirty FROM schema_migrations;"
docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
```

Passez un test de fumée sur quelques points d'accès que la migration touche (`/api/v1/...`).
