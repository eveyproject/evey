---
source_sha: 771f24a342b202edfea3fa8ec419ba1e7d488e2112cdd4a535c84b7cd1b586c4
reviewed: false
---
# Datenbankmigrationen

Wie Evey sein Datenbankschema während eines Updates ändert und was zu tun ist,
wenn es schiefgeht. Die Befehle zielen auf die Standardinstallation, in der die
Datenbank im Container `evey-server` läuft.

## Wie Migrationen laufen

Die Migrationen sind in das Programm eingebettet (`go:embed`) und werden beim
Serverstart von `golang-migrate` angewendet
(`internal/adapters/driven/postgres/migrate.go`, Dateien
`internal/adapters/driven/postgres/migrations/NNNNNN_<name>.{up,down}.sql`).
Der Zustand ist die Tabelle `schema_migrations`: `{version, dirty}`.

Bevor er etwas anwendet, schreibt der Server einen vollständigen Dump nach
`/data/backups/pre-migration-<version>-<time>.sql.gz` (`EVEY_BACKUP_DIR`). Der
neueste Dump jeder der 3 jüngsten Schemaversionen wird behalten.

Dieselben Vorgänge gibt es von Hand:

```sh
docker exec evey-server evey admin migrate version   # Exit 1, wenn das Schema "dirty" ist
docker exec evey-server evey admin migrate up
docker exec evey-server evey admin migrate down      # genau ein Schritt
```

`up` und `down` erstellen ebenfalls zuerst einen Dump; `--no-backup` lässt ihn
weg.

Um SQL gegen die eingebettete Datenbank auszuführen:

```sh
docker exec evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

## Vor einem Update

1. **Erstellen Sie eine Sicherung** unter **Admin → Sicherung &
   Wiederherstellung** (**Jetzt ausführen**), oder aus einem Skript:
   ```sh
   curl -X POST -H "Authorization: Bearer <admin-token>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   Der automatische Dump vor der Migration ist eine zweite Kopie, kein Ersatz.

2. **Bei einem großen Katalog** sehen Sie sich die Größe der größten Tabellen
   an: Eine Migration, die eine davon umschreibt, dauert länger.
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

3. **Prüfen Sie die aktuelle Version**: `evey admin migrate version`. Meldet
   er ein Schema im Zustand „dirty", lesen Sie zuerst
   [Eine auf halbem Weg gestoppte Migration](#a-migration-that-stopped-halfway).

## Die Migrationen ausführen

Sie laufen, wenn das neue Image startet:

```sh
docker compose pull && docker compose up -d
docker logs -f evey-server
```

Der Container meldet sich als gesund, sobald die Migrationen angewendet sind und
der Server antwortet:

```sh
curl -sf http://localhost:9091/healthz && echo OK
docker exec evey-server evey admin migrate version
```

## Zurückgehen

`evey admin rollback` gibt die laufende Version, die Schemaversion, den
neuesten Dump vor einer Migration und die genauen Befehle für den Host aus. Er
ändert nichts: Der Container hat keinen Zugriff auf Docker.

Der übliche Weg zurück ist das vorherige Image mit dem vor dem Update erstellten
Dump: siehe [Zurückgehen](../backups.md#going-back) unter Sicherungen und
Updates. `evey admin migrate down` geht stattdessen Schritt für Schritt zurück,
wenn die `.down.sql` dieses Schritts genügt.

Bearbeiten Sie `schema_migrations` nicht von Hand, außer um einen Zustand
„dirty" aufzuheben (unten), und erstellen Sie vorher einen Dump.

Stellen Sie den Dump von vor der Migration wieder her, statt zurückzumigrieren,
wenn das Update Daten entfernt hat (eine gelöschte Spalte, ein geänderter Typ).

## Eine auf halbem Weg gestoppte Migration

Zeigt das Log `Dirty database version N. Fix and force version.`, ist eine
Migration mittendrin abgebrochen. Bringen Sie das Schema in den gewünschten
Zustand und heben Sie dann die Markierung auf:

```sql
SELECT version, dirty FROM schema_migrations;

-- Re-apply the rest of the up.sql by hand, or revert with the down.sql,
-- then record the version you actually reached:
UPDATE schema_migrations SET version = <N>, dirty = false;
```

Starten Sie den Container neu; die Migrationen laufen von dort weiter. Den Dump
von vor der Migration wiederherzustellen ist der andere Ausweg.

## Eine Migration schreiben

Für Mitwirkende. Beispiele in `internal/adapters/driven/postgres/migrations/`:

- **`000066_cascade_and_email_unique.up.sql`** - `ALTER TABLE ... DROP CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`. Hält `ACCESS EXCLUSIVE` auf beiden Tabellen für die Dauer des ADD. Billig auf kleinen Tabellen, schmerzhaft auf `media_items` / `watch_progress`. Muster für große Tabellen: mit `NOT VALID` hinzufügen, dann `VALIDATE CONSTRAINT` in einer zweiten Anweisung (die Validierung hält nur `SHARE UPDATE EXCLUSIVE`).
- **`000067_sort_title_index.up.sql`, `000068_watch_progress_composite_index.up.sql`, `000069_quick_connect_shortcode_index.up.sql`** - einfaches `CREATE INDEX`. Auf großen Tabellen sperrt das Schreibzugriffe. `CREATE INDEX CONCURRENTLY` kann nicht in einer Migrationstransaktion laufen.
- **`000072_syncplay_queue.up.sql`** - `ALTER TABLE ... ADD COLUMN ... NOT NULL DEFAULT '[]'`. Ab PG 11 betrifft das nur Metadaten und ist schnell. Hält trotzdem kurz `ACCESS EXCLUSIVE`.

Regeln:

- **Langes ALTER**, das die Tabelle umschreibt (Typänderung, nicht konstanter Standardwert auf altem PG) → bei einem großen Katalog mit Ausfallzeit rechnen.
- **DROP COLUMN** → betrifft nur Metadaten, aber die Spalte lässt sich ohne Sicherung nicht wiederherstellen. Nie in derselben Version löschen wie den Code, der aufhört, sie zu lesen - lieber in zwei Schritten: aufhören, die Spalte zu lesen, ausrollen, dann in der nächsten Version löschen.
- **Fremdschlüssel auf großen Tabellen** → `ADD CONSTRAINT ... NOT VALID`, dann `VALIDATE CONSTRAINT` in einer folgenden Migration. Ein einzelnes blockierendes `ADD CONSTRAINT ... FOREIGN KEY ...` vermeiden.
- **Massen-UPDATE / Nachbefüllen** → nie in einer einzigen Migration auf einer großen Tabelle. Eine Schema-Migration ohne Wirkung plus eine Hintergrundaufgabe ausliefern.
- **Spaltenersatz (erweitern → migrieren → zusammenziehen)** → `ADD COLUMN` + Nachbefüllen + `SET NOT NULL` + `DROP COLUMN` der alten nicht in einer einzigen Migration bündeln. Diese Form funktioniert nur, weil Evey heute als einzelne Instanz läuft, mit beim Start synchron angewendeten Migrationen: Während des Zeitfensters können alter Code, der die gelöschte Spalte liest, und neuer Code, der die neue liest, nicht nebeneinander bestehen, sodass ein schrittweises oder mehrfach repliziertes Ausrollen Fehler aus der zurückliegenden Hälfte liefern würde.
  Bekannte so geschriebene Migrationen, unverändert gelassen, weil bereits ausgeliefert: `000077` und `000098_playlists_profile_id.up.sql` (fügt `profile_id` hinzu, befüllt nach, setzt `NOT NULL`, löscht `user_id`).
  Für eine neue auf drei Versionen verteilen:
  1. **erweitern** - die neue Spalte nullable hinzufügen, in *beide* Spalten schreiben, weiter die alte lesen;
  2. **migrieren** - die neue Spalte nachbefüllen (Hintergrundaufgabe, nicht die Migration), dann die Lesezugriffe auf sie umstellen;
  3. **zusammenziehen** - sobald kein ausgerollter Code die alte Spalte mehr anfasst, `SET NOT NULL` und sie löschen.
  Jeder Schritt lässt sich unabhängig ausrollen und zurücknehmen, was die Version in einer Datei nicht kann.

## Nach einem Update

```sh
curl -sf http://localhost:9091/healthz
docker exec evey-server evey admin migrate version
docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
```

Leeren Sie danach den Transkodierungs-Cache, wenn die Version die Art geändert
hat, wie Videos kodiert werden (**Admin → Transkodierungs-Cache**).
