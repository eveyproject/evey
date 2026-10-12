---
source_sha: fad8d9ff7aa0e8e4c14508e1eb2fe716e13c05c66eca9acb763084c56aec898f
reviewed: true
---
# Datenbankmigrationen

Wie Evey bei einem Update den Aufbau seiner Datenbank (das Schema) ändert, was
Sie vorher und nachher prüfen, und wie Sie herauskommen, wenn eine Migration
auf halbem Weg stehen bleibt. Für Betreiber der Standardinstallation, in der
die Datenbank im Container `evey-server` läuft. Wer selbst eine Migration
schreibt, findet die Regeln am Ende.

## Wie Migrationen laufen

Die Migrationen sind in das Programm eingebaut und werden von
`golang-migrate` angewendet, wenn der Server startet. Der Stand steht in der
Tabelle `schema_migrations`, eine Zeile `{version, dirty}`. Bevor der Server
etwas anwendet, schreibt er einen vollständigen Dump nach `EVEY_BACKUP_DIR`
(`/data/backups`, also `./evey-data/backups` auf dem Rechner) mit dem Namen
`pre-migration-<version>-<UTC timestamp>.sql.gz`. Er behält den neuesten
Dump jeder der 3 jüngsten Schemaversionen.

Dieselben Schritte gibt es auch von Hand:

```sh
docker exec evey-server evey admin migrate version   # Code 1, wenn das Schema „dirty“ ist
docker exec evey-server evey admin migrate up        # alle ausstehenden Migrationen anwenden
docker exec evey-server evey admin migrate down      # genau einen Schritt zurückrollen
```

`up` und `down` legen zuerst einen Dump an; `--no-backup` lässt ihn weg. So
öffnen Sie eine SQL-Shell auf der eingebauten Datenbank:

```sh
docker exec -it evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

## Sicher aktualisieren

1. **Legen Sie eine Sicherung an** unter **Verwaltung -> Sicherung &
   Wiederherstellung** (**Jetzt ausführen**), oder aus einem Skript:
   ```sh
   curl -X POST -H "Authorization: Bearer <admin-token>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   Der automatische Dump vor der Migration ist eine zweite Kopie, kein
   Ersatz.

2. **Prüfen Sie die aktuelle Version**: `docker exec evey-server evey admin
   migrate version` gibt `schema version: N` aus. Steht dort `DIRTY`, lesen
   Sie vor dem Update
   [Eine Migration, die auf halbem Weg stehen blieb](#a-migration-that-stopped-halfway).

3. **Bei einem großen Katalog** schauen Sie sich die größten Tabellen an:
   Eine Migration, die eine davon neu schreibt, dauert länger. Die
   Gesundheitsprüfung des Containers lässt dafür 300 Sekunden Startzeit.
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

4. **Starten Sie das neue Image** und verfolgen Sie das Protokoll, bis die
   Migrationen angewendet sind und der Server antwortet:
   ```sh
   docker compose pull && docker compose up -d
   docker logs -f evey-server
   ```
   Ein gutes Ergebnis: eine Zeile `migrations: pre-migration backup written`,
   wenn es etwas anzuwenden gab, dann das Banner des Servers, dann antwortet
   `curl -sf http://localhost:9091/healthz`.

5. **Danach** bestätigen Sie die Version und suchen nach Fehlern:
   ```sh
   docker exec evey-server evey admin migrate version
   docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
   ```
   Leeren Sie den Transkodierungs-Cache (**Verwaltung ->
   Transkodierungs-Cache -> Alle leeren**), wenn die neue Version ändert,
   wie Videos kodiert werden.

## Zur vorherigen Version zurückkehren

`docker exec evey-server evey admin rollback` gibt die laufende Version, die
Schemaversion, den neuesten Dump von vor einer Migration und die genauen
Befehle aus, die Sie auf dem Rechner ausführen. Er ändert nichts: Der
Container hat keinen Zugriff auf Docker.

Der übliche Weg zurück ist das vorherige Image plus der Dump, der vor dem
Update angelegt wurde: siehe [Zurückkehren](../backups.md#going-back).
Spielen Sie den Dump ein, statt nach unten zu migrieren, wenn das Update
Daten entfernt hat (eine gelöschte Spalte, ein geänderter Typ).
`evey admin migrate down` geht dagegen eine Migration nach der anderen
zurück, wenn die `.down.sql` dieses Schritts ausreicht.

!!! warning "Achtung"
    Bearbeiten Sie `schema_migrations` nie von Hand, außer um einen
    „dirty“-Zustand zu beheben (unten), und legen Sie vorher einen Dump an.

## Eine Migration, die auf halbem Weg stehen blieb

Zeigt das Protokoll `Dirty database version N. Fix and force version.`, ist
eine Migration mittendrin abgestürzt, und `golang-migrate` macht nicht weiter.
Bringen Sie das Schema in den gewünschten Zustand und löschen Sie dann die
Markierung:

```sql
SELECT version, dirty FROM schema_migrations;

-- Den Rest der up.sql von Hand anwenden oder mit der down.sql zurücknehmen,
-- dann die Version eintragen, die Sie tatsächlich erreicht haben:
UPDATE schema_migrations SET version = <N>, dirty = false;
```

Starten Sie den Container neu; die Migrationen machen von dort aus weiter.
Den Dump von vor der Migration wiederherzustellen
([Wiederherstellung](recovery.md#restore-a-pre-migration-dump)) ist der
andere Ausweg.

## Eine Migration schreiben

Für Mitwirkende. Die Dateien liegen unter
`internal/adapters/driven/postgres/migrations/NNNNNN_<name>.{up,down}.sql`.
Beispiele, die schon ausgeliefert wurden:

- **`000066_cascade_and_email_unique.up.sql`** - `ALTER TABLE ... DROP
  CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`. Setzt für die Dauer des ADD
  `ACCESS EXCLUSIVE` auf beide Tabellen. Günstig bei kleinen Tabellen,
  schmerzhaft bei `media_items` / `watch_progress`.
- **`000067_sort_title_index.up.sql`**,
  **`000068_watch_progress_composite_index.up.sql`**,
  **`000069_quick_connect_shortcode_index.up.sql`** - einfaches
  `CREATE INDEX`, das bei großen Tabellen das Schreiben sperrt.
  `CREATE INDEX CONCURRENTLY` kann nicht in einer Transaktion von migrate
  laufen.
- **`000072_syncplay_queue.up.sql`** - `ALTER TABLE ... ADD COLUMN ... NOT
  NULL DEFAULT '[]'`. Ab PG 11 ändert das nur Metadaten, hält aber trotzdem
  kurz `ACCESS EXCLUSIVE`.

Richtlinien:

- **Ein langes ALTER**, das die Tabelle neu schreibt (Typänderung, ein nicht
  konstanter Standardwert auf einem alten PG), bedeutet bei einem großen
  Katalog eine Ausfallzeit.
- **DROP COLUMN** ändert nur Metadaten, aber ohne Sicherung kommt die Spalte
  nicht zurück. Löschen Sie sie nie in der Version, die aufhört, sie zu
  lesen: erst nicht mehr lesen, ausrollen, dann in der nächsten Version
  löschen.
- **Fremdschlüssel auf großen Tabellen**: `ADD CONSTRAINT ... NOT VALID`,
  dann `VALIDATE CONSTRAINT` in einer späteren Migration (validate nimmt nur
  `SHARE UPDATE EXCLUSIVE`).
- **UPDATE in Stapeln / Nachfüllen von Daten**: nie in einer einzigen
  Migration auf einer großen Tabelle. Liefern Sie eine Schemamigration aus,
  die nichts tut, plus eine Aufgabe im Hintergrund.
- **Eine Spalte ersetzen (erweitern, migrieren, zusammenziehen)**: Packen Sie
  `ADD COLUMN` + Nachfüllen + `SET NOT NULL` + `DROP COLUMN` nicht in eine
  Datei. Diese Form funktioniert nur, weil Evey als einzelne Instanz läuft
  und die Migrationen beim Start der Reihe nach angewendet werden; bei einem
  schrittweisen Ausrollen würde die Hälfte, die zurückliegt, Fehler liefern.
  `000077` und `000098_playlists_profile_id.up.sql` wurden so geschrieben und
  bleiben so, weil sie schon ausgeliefert sind. Verteilen Sie eine neue auf
  drei Versionen:
  1. **erweitern** - die neue Spalte als nullable hinzufügen, beide Spalten
     schreiben, weiter die alte lesen;
  2. **migrieren** - die neue Spalte nachfüllen (Aufgabe im Hintergrund,
     nicht die Migration), dann das Lesen auf sie umstellen;
  3. **zusammenziehen** - sobald kein ausgerollter Code die alte Spalte mehr
     anfasst, `SET NOT NULL` und die alte Spalte löschen.

<!-- sources
migrations: internal/adapters/driven/postgres/migrate.go - RunMigrations, RollbackLastMigration, CurrentMigrationVersion ; internal/adapters/driven/postgres/migrate_backup.go - dumpDatabase, backupFilePrefix, pruneOldBackups, defaultBackupRetention
cli: cmd/evey/admin.go - adminMigrate, adminMigrateUsage ; cmd/evey/admin_ops.go - adminRollback
embedded database and healthcheck start period: docker/entrypoint.sh (postgres.password, EVEY_DB_PASSWORD_FILE) ; Dockerfile - HEALTHCHECK --start-period
backup route: internal/adapters/driving/http/backup_handler.go (POST /api/v1/admin/backups)
labels: i18n/en/admin.yaml - sidebar.backupRestore, backup.schedule.runNow, transcodeCache.clearAll
-->
