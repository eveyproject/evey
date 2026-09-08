---
source_sha: 6f985b91384f5a0566bd38dfa9962efdfce4a6b07d1697db8a21ab17c08d0469
---
# Runbook: Migrationen

Wie Evey Schema-Migrationen ausführt und wie Sie gefahrlos damit umgehen.

## Wie Migrationen ablaufen

Die Migrationen sind **eingebettet** (`go:embed`) und laufen beim Serverstart automatisch aus `internal/adapters/driven/postgres/migrate.go` heraus, über `golang-migrate/migrate/v4`. Quelle: `internal/adapters/driven/postgres/migrations/NNNNNN_<name>.{up,down}.sql`.

Migrationen werden beim Start von `evey-server` automatisch angewandt. Für die Steuerung von Hand gibt es **zusätzlich** eine Kommandozeile: `evey admin migrate up | down | version` (umgesetzt in `cmd/evey/admin.go`; `version` gibt die aktuelle Schemaversion aus und endet mit 1, wenn die Datenbank `dirty` ist). Die Ziele `migrate-up` und `migrate-down` im `Makefile` rufen sie über `./cmd/evey admin migrate` auf. Ist `EVEY_BACKUP_DIR` gesetzt, entsteht vor der Migration automatisch ein `pg_dump` (mit `--no-backup` überspringen Sie das).

Den Zustand hält die Tabelle `schema_migrations` fest (von golang-migrate verwaltet): `{version, dirty}`.

## Checkliste vor der Migration

1. **Sichern Sie die Datenbank.** Evey bringt dafür einen Endpunkt mit (siehe `docs/backups.md`):
   ```sh
   curl -X POST -H "Authorization: Bearer <admin-token>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   Oder lassen Sie `pg_dump` unmittelbar gegen den Container laufen:
   ```sh
   docker exec evey-postgres pg_dump -U "$EVEY_DB_USER" "$EVEY_DB_NAME" \
     | gzip > backup-$(date -u +%Y%m%dT%H%M%SZ).sql.gz
   ```
   Prüfen Sie, dass der Dump nicht leer ist und `gunzip -t` durchläuft.

2. **Sehen Sie sich die Tabellengrößen an**, sobald eine Migration große Tabellen anfasst (media_items, watch_progress, tracks, hls_sessions):
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

3. **Prüfen Sie aktive Sitzungen und Sperren** vor einer Migration mit viel DDL:
   ```sql
   SELECT pid, state, wait_event_type, wait_event,
          now() - query_start AS duration, left(query, 120)
     FROM pg_stat_activity
    WHERE datname = current_database()
      AND pid <> pg_backend_pid()
    ORDER BY query_start;
   ```
   Beenden Sie lang laufende Sitzungen und solche im Zustand idle-in-transaction (`SELECT pg_terminate_backend(pid)`), bevor Sie eine Migration starten, die starke Sperren nimmt.

4. **Prüfen Sie die aktuelle Migrationsversion**:
   ```sql
   SELECT version, dirty FROM schema_migrations;
   ```
   Steht `dirty = true`, arbeiten Sie zuerst den Abschnitt zur Wiederherstellung ab, bevor Sie irgendetwas anderes tun.

## Eine Migration ausführen

Migrationen laufen beim Serverstart. So wenden Sie ausstehende Migrationen an:

```sh
docker compose pull evey       # wenn Sie ein neues Image ausrollen
docker compose up -d evey
docker logs -f evey-server     # das Startprotokoll mitlesen
```

Erfolgreich gelaufen heißt: keine Fehler von migrate, und der Server antwortet auf `/healthz` mit 200. So prüfen Sie das nach:

```sh
curl -sf http://localhost:9091/healthz && echo OK
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" \
  -c "SELECT version, dirty FROM schema_migrations;"
```

## Zurückrollen

Einen einzelnen Schritt rollen Sie über das fertig verdrahtete Ziel zurück:

```sh
make migrate-down     # ruft `evey admin migrate down` auf, rollt GENAU einen Schritt zurück
make migrate-version  # nachsehen, wo Sie gelandet sind
```

Wiederholen Sie es einmal je Schritt, um weiter zurückzugehen. Ist
`EVEY_BACKUP_DIR` gesetzt, entsteht vor der Migration automatisch ein
`pg_dump`.

Alternativ **rollen Sie das vorige Image erneut aus**, dessen eingebettete
Migrationen bei Version N-1 aufhören.

> Bearbeiten Sie `schema_migrations` niemals von Hand. Eine frühere Fassung
> dieses Runbooks behauptete, es gebe kein Ziel `migrate-down`, und wies
> Betreiber an, das `.down.sql` mit `psql` einzuspielen und danach
> `UPDATE schema_migrations SET version = ...` von Hand abzusetzen - mitten in
> einer Störung. Damit widersprach sie dem Abschnitt „Wie Migrationen ablaufen“
> neun Zeilen weiter oben. Das Ziel gab es immer schon (`Makefile:314`).
> Greifen Sie nur dann zum Eingriff von Hand, wenn die Datenbank als `dirty`
> markiert ist und `migrate down` den Dienst verweigert - und ziehen Sie vorher
> einen Dump.

Spielen Sie immer die Sicherung von vor der Migration zurück, wenn
datenzerstörende Änderungen ausgeliefert wurden (DROP COLUMN, Typänderungen)
und die Sicherung jünger ist als der Punkt, ab dem der Produktivbetrieb
auseinanderläuft.

## Wiederherstellung - der Zustand `dirty`

Steht im Startprotokoll `Dirty database version N. Fix and force version.`, ist die letzte Migration auf halbem Weg abgestürzt. Bringen Sie das Schema von Hand in den Zustand, den Sie haben wollen, und löschen Sie danach das Kennzeichen:

```sql
-- Nachsehen: was meldet schema_migrations gerade?
SELECT version, dirty FROM schema_migrations;

-- Entweder die Anweisungen aus up.sql von Hand nachziehen oder über down.sql zurücknehmen.
-- Danach das dirty-Bit auf der Version löschen, die Sie tatsächlich erreicht haben:
UPDATE schema_migrations SET version = <N>, dirty = false;
```

Starten Sie `evey-server` neu. golang-migrate macht von dort aus weiter.

## Sonderfälle

Jüngere Migrationen als Anschauungsmaterial (siehe `internal/adapters/driven/postgres/migrations/`):

- **`000066_cascade_and_email_unique.up.sql`** - `ALTER TABLE ... DROP CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`. Nimmt für die Dauer des ADD auf beiden Tabellen `ACCESS EXCLUSIVE`. Auf kleinen Tabellen billig, auf `media_items` und `watch_progress` schmerzhaft. Das Muster für große Tabellen: mit `NOT VALID` anlegen und in einer zweiten Anweisung `VALIDATE CONSTRAINT` hinterherschicken (das Validieren nimmt nur `SHARE UPDATE EXCLUSIVE`).
- **`000067_sort_title_index.up.sql`, `000068_watch_progress_composite_index.up.sql`, `000069_quick_connect_shortcode_index.up.sql`** - schlichtes `CREATE INDEX`. Auf großen Tabellen sperrt das die Schreibzugriffe. Erwägen Sie `CREATE INDEX CONCURRENTLY` von Hand in einem Wartungsfenster (das läuft nicht innerhalb einer migrate-Transaktion - also außerhalb anwenden und die Version danach eintragen).
- **`000072_syncplay_queue.up.sql`** - `ALTER TABLE ... ADD COLUMN ... NOT NULL DEFAULT '[]'`. Ab PG 11 betrifft das nur die Metadaten und geht schnell. `ACCESS EXCLUSIVE` wird trotzdem kurz gehalten.

Leitlinien:

- **Ein langes ALTER**, das die Tabelle neu schreibt (Typänderung, ein nicht konstanter Standardwert auf altem PG) → die Prüfung der Sperren vorher ist Pflicht, rechnen Sie mit einer Auszeit.
- **DROP COLUMN** → nur Metadaten, aber ohne Sicherung bekommen Sie die Spalte nicht zurück. Werfen Sie eine Spalte nie in derselben Veröffentlichung weg, in der neuer Code ausgerollt wird - besser in zwei Schritten: erst aufhören, die Spalte zu lesen, ausrollen, und sie in der nächsten Veröffentlichung entfernen.
- **Fremdschlüssel auf großen Tabellen** → `ADD CONSTRAINT ... NOT VALID` und in einer Folgemigration `VALIDATE CONSTRAINT`. Vermeiden Sie ein einzelnes, blockierendes `ADD CONSTRAINT ... FOREIGN KEY ...`.
- **Ein UPDATE in Stapeln oder ein Nachtragen von Daten** → niemals in einer einzigen Migration auf einer großen Tabelle. Liefern Sie eine Schema-Migration ohne Wirkung plus einen Hintergrundauftrag aus.
- **Eine Spalte ersetzen (expand → migrate → contract)** → packen Sie `ADD COLUMN`, das Nachtragen, `SET NOT NULL` und das `DROP COLUMN` der alten Spalte nicht in eine einzige Migration. Diese Form geht nur gut, weil Evey heute in einer einzigen Instanz läuft und die Migrationen beim Start synchron angewandt werden: In diesem Zeitfenster können alter Code, der die entfernte Spalte liest, und neuer Code, der die neue liest, nicht nebeneinander bestehen - ein rollierendes Deployment oder eines mit mehreren Repliken würde also aus der jeweils zurückliegenden Hälfte heraus Fehler ausliefern.
  So geschriebene Migrationen sind bekannt und bleiben, wie sie sind, weil sie bereits ausgeliefert wurden: `000077` und `000098_playlists_profile_id.up.sql` (legt `profile_id` an, trägt Daten nach, setzt `NOT NULL`, entfernt `user_id`).
  Verteilen Sie eine neue auf drei Veröffentlichungen:
  1. **expand** - die neue Spalte nullable anlegen, in *beide* Spalten schreiben, weiterhin die alte lesen;
  2. **migrate** - die neue Spalte nachtragen (als Hintergrundauftrag, nicht in der Migration) und danach die Lesezugriffe darauf umstellen;
  3. **contract** - sobald kein ausgerollter Code die alte Spalte mehr anfasst, `SET NOT NULL` setzen und sie entfernen.
  Jeder Schritt lässt sich für sich ausrollen und zurücknehmen, die Fassung in einer einzigen Datei nicht.

## Prüfungen nach der Migration

```sh
curl -sf http://localhost:9091/healthz
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" \
  -c "SELECT version, dirty FROM schema_migrations;"
docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
```

Prüfen Sie stichprobenartig ein paar Endpunkte, die die Migration betrifft (`/api/v1/...`).
