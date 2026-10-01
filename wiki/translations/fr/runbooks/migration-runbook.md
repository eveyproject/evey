---
source_sha: 771f24a342b202edfea3fa8ec419ba1e7d488e2112cdd4a535c84b7cd1b586c4
reviewed: true
---
# Migrations de la base

Comment Evey fait évoluer le schéma de sa base pendant une mise à jour, et que
faire quand ça se passe mal. Les commandes visent l'installation standard, où
la base tourne dans le conteneur `evey-server`.

## Comment les migrations s'exécutent

Les migrations sont intégrées au binaire (`go:embed`) et appliquées par
`golang-migrate` au démarrage du serveur
(`internal/adapters/driven/postgres/migrate.go`, fichiers
`internal/adapters/driven/postgres/migrations/NNNNNN_<nom>.{up,down}.sql`).
L'état est la table `schema_migrations` : `{version, dirty}`.

Avant d'appliquer quoi que ce soit, le serveur écrit un dump complet dans
`/data/backups/pre-migration-<version>-<heure>.sql.gz` (`EVEY_BACKUP_DIR`). Le
dump le plus récent de chacune des 3 dernières versions du schéma est conservé.

Les mêmes opérations existent à la main :

```sh
docker exec evey-server evey admin migrate version   # code 1 si le schéma est « dirty »
docker exec evey-server evey admin migrate up
docker exec evey-server evey admin migrate down      # exactement une étape
```

`up` et `down` prennent aussi un dump d'abord ; `--no-backup` s'en passe.

Pour lancer du SQL sur la base intégrée :

```sh
docker exec evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

## Avant une mise à jour

1. **Faites une sauvegarde** dans **Admin → Sauvegarde BDD et restauration**
   (**Lancer maintenant**), ou depuis un script :
   ```sh
   curl -X POST -H "Authorization: Bearer <admin-token>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   Le dump automatique d'avant migration est une seconde copie, pas un
   remplacement.

2. **Sur un gros catalogue**, regardez la taille des plus grosses tables : une
   migration qui réécrit l'une d'elles prend plus de temps.
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

3. **Vérifiez la version actuelle** : `evey admin migrate version`. S'il
   signale un schéma « dirty », voyez d'abord
   [Une migration arrêtée en cours de route](#a-migration-that-stopped-halfway).

## Lancer les migrations

Elles s'exécutent au démarrage de la nouvelle image :

```sh
docker compose pull && docker compose up -d
docker logs -f evey-server
```

Le conteneur se déclare en bonne santé une fois les migrations appliquées et le
serveur prêt à répondre :

```sh
curl -sf http://localhost:9091/healthz && echo OK
docker exec evey-server evey admin migrate version
```

## Revenir en arrière

`evey admin rollback` affiche la version en service, la version du schéma, le
dump d'avant migration le plus récent et les commandes exactes à lancer sur la
machine hôte. Il ne change rien : le conteneur n'a pas accès à Docker.

La voie habituelle est l'image précédente avec le dump pris avant la mise à
jour : voir [Revenir en arrière](../backups.md#going-back) dans Sauvegardes et
mises à jour. `evey admin migrate down` revient plutôt une étape à la fois,
quand le `.down.sql` de cette étape suffit.

Ne modifiez pas `schema_migrations` à la main, sauf pour lever un état
« dirty » (plus bas), et faites un dump d'abord.

Restaurez le dump d'avant migration plutôt que de redescendre quand la mise à
jour a supprimé des données (une colonne retirée, un changement de type).

## Une migration arrêtée en cours de route

Si le journal affiche `Dirty database version N. Fix and force version.`, une
migration s'est arrêtée au milieu. Amenez le schéma dans l'état voulu, puis
levez le drapeau :

```sql
SELECT version, dirty FROM schema_migrations;

-- Réappliquez à la main le reste du up.sql, ou revenez avec le down.sql,
-- puis enregistrez la version réellement atteinte :
UPDATE schema_migrations SET version = <N>, dirty = false;
```

Redémarrez le conteneur ; les migrations reprennent à partir de là. Restaurer
le dump d'avant migration est l'autre issue.

## Écrire une migration

Pour les contributeurs. Exemples dans `internal/adapters/driven/postgres/migrations/` :

- **`000066_cascade_and_email_unique.up.sql`** : `ALTER TABLE ... DROP CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`. Prend `ACCESS EXCLUSIVE` sur les deux tables pendant tout l'ADD. Peu coûteux sur de petites tables, pénible sur `media_items` / `watch_progress`. Pour les grosses tables : ajouter avec `NOT VALID`, puis `VALIDATE CONSTRAINT` dans une seconde instruction (la validation ne prend que `SHARE UPDATE EXCLUSIVE`).
- **`000067_sort_title_index.up.sql`, `000068_watch_progress_composite_index.up.sql`, `000069_quick_connect_shortcode_index.up.sql`** : `CREATE INDEX` simples. Sur de grosses tables, cela bloque les écritures. `CREATE INDEX CONCURRENTLY` ne peut pas tourner dans une transaction de migration.
- **`000072_syncplay_queue.up.sql`** : `ALTER TABLE ... ADD COLUMN ... NOT NULL DEFAULT '[]'`. Depuis PG 11, ne touche que les métadonnées et reste rapide. Prend quand même `ACCESS EXCLUSIVE` un court instant.

Règles :

- **ALTER long** qui réécrit la table (changement de type, valeur par défaut non constante sur un vieux PG) → prévoir une interruption sur un gros catalogue.
- **DROP COLUMN** → ne touche que les métadonnées, mais la colonne ne se récupère pas sans sauvegarde. Ne jamais la supprimer dans la même version que le code qui cesse de la lire : en deux temps, cesser de la lire, déployer, puis la supprimer à la version suivante.
- **Clé étrangère sur une grosse table** → `ADD CONSTRAINT ... NOT VALID` puis `VALIDATE CONSTRAINT` dans une migration suivante. Éviter un `ADD CONSTRAINT ... FOREIGN KEY ...` bloquant d'un seul tenant.
- **UPDATE en masse / remplissage** → jamais dans une seule migration sur une grosse table. Livrer une migration de schéma sans effet et une tâche de fond.
- **Remplacement de colonne (étendre → migrer → contracter)** → ne pas regrouper `ADD COLUMN` + remplissage + `SET NOT NULL` + `DROP COLUMN` de l'ancienne dans une seule migration. Cette forme ne fonctionne que parce qu'Evey tourne aujourd'hui en instance unique, avec des migrations appliquées de façon synchrone au démarrage : pendant la fenêtre, l'ancien code qui lit la colonne supprimée et le nouveau qui lit la nouvelle ne peuvent pas coexister, si bien qu'un déploiement progressif ou à plusieurs réplicas servirait des erreurs depuis la moitié en retard.
  Migrations connues écrites ainsi, conservées telles quelles parce qu'elles sont déjà livrées : `000077` et `000098_playlists_profile_id.up.sql` (ajoute `profile_id`, remplit, pose `NOT NULL`, supprime `user_id`).
  Pour une nouvelle, répartir sur trois versions :
  1. **étendre** : ajouter la nouvelle colonne, nullable, écrire dans *les deux* colonnes, continuer à lire l'ancienne ;
  2. **migrer** : remplir la nouvelle colonne (tâche de fond, pas la migration), puis basculer les lectures dessus ;
  3. **contracter** : une fois qu'aucun code déployé ne touche plus l'ancienne colonne, `SET NOT NULL` et la supprimer.
  Chaque étape se déploie et se défait indépendamment, ce que la version en un seul fichier ne permet pas.

## Après une mise à jour

```sh
curl -sf http://localhost:9091/healthz
docker exec evey-server evey admin migrate version
docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
```

Videz ensuite le cache de transcodage si la version a changé la façon dont les
vidéos sont encodées (**Admin → Cache de transcodage**).
