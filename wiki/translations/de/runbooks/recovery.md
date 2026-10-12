---
source_sha: 15f4243e2aa41d1a370efb30c5607eb485799cf11a0be191ea0b7489684f4485
reviewed: true
---
# Wiederherstellung

Öffnen Sie diese Seite, wenn sich niemand mehr anmelden kann, wenn der
Schlüssel, der die Zugangstoken signiert, nach außen gelangt sein könnte,
oder wenn die Datenbank auf einen Dump von vor einer Migration zurückgesetzt
werden muss.
Jeder Ablauf ist ein einziger `evey admin`-Befehl, der im Container läuft; die
Seite [Kommandozeile](cli.md) listet die anderen Unterbefehle auf.

## Bevor Sie beginnen

Die Befehle lesen dieselbe `EVEY_*`-Konfiguration wie der Server. Im
Container des Servers brauchen sie daher nichts weiter:

```sh
docker exec evey-server evey admin <subcommand>        # Server läuft
docker compose run --rm evey admin <subcommand>        # Container gestoppt
```

Die zweite Form startet zuerst die eingebaute Datenbank, führt dann den Befehl
aus und beendet sich. Keiner dieser Befehle startet den Server.

## Einem Benutzer ein neues Passwort geben

Nutzen Sie das, wenn ein Benutzer sich nicht mehr anmelden kann, oder sofort, wenn Sie
vermuten, dass ein Konto übernommen wurde. Das neue Passwort wird einmal
angezeigt und nie protokolliert: Kopieren Sie es, bevor Sie das Terminal
schließen.

```sh
docker exec evey-server evey admin reset-password --user admin@example.com
```

Eine erfolgreiche Ausgabe sieht so aus:

```
user:     admin@example.com
user_id:  usr-xxxxxxxx
password: <random 24-byte base64url>

Save this - it is the only time the new password is shown.
Every session of this user has been ended.
```

Alle Sitzungen dieses Benutzers enden sofort: Die Refresh-Token werden
gelöscht und die Zugangstoken widerrufen. Eine offene App merkt das innerhalb
von etwa 30 Sekunden und bittet um eine neue Anmeldung. Haben Sie die Ausgabe
verloren, führen Sie den Befehl einfach noch einmal aus.

## Den Schlüssel wechseln, der die Zugangstoken signiert

Nutzen Sie das, wenn die Schlüsseldatei nach außen gelangt sein könnte, oder
um alle Benutzer auf einmal abzumelden. Für ein einzelnes Konto genügt
**Verwaltung → Benutzer → Deaktivieren** oder das Zurücksetzen des Passworts
oben.

```sh
docker exec evey-server evey admin rotate-jwt-key
```

Was passiert:

1. Die Schlüsseldatei (`EVEY_JWT_PRIVATE_KEY_PATH`, standardmäßig
   `/data/jwt.key`) wird in `<path>.rotated-<UTC timestamp>` umbenannt.
2. Jedes Refresh-Token wird gelöscht und jedes Zugangstoken widerrufen, in
   einer einzigen Transaktion. Offene Apps merken das innerhalb von etwa 30
   Sekunden; alle melden sich neu an.
3. Beim nächsten Start erzeugt der Server einen neuen RSA-Schlüssel mit 2048
   Bit.

Starten Sie den Container danach neu (`docker compose restart evey`), damit
der neue Schlüssel benutzt wird. War die Datenbank nicht erreichbar, wird der
Schlüssel trotzdem umbenannt, und der Befehl meldet `key rotated, but
sessions could not be ended (restart the server, then run this again)`: Tun
Sie genau das.

## Einen Dump von vor einer Migration wiederherstellen

Nutzen Sie das, wenn ein Update rückgängig gemacht werden muss und die Daten,
die seitdem geschrieben wurden, verloren gehen dürfen. Der Befehl spielt einen
`.sql.gz`-Dump über `gunzip | psql` in die konfigurierte Datenbank ein.

!!! warning "Achtung"
    Die Wiederherstellung überschreibt die aktuellen Zeilen. Legen Sie zuerst
    eine Sicherung des aktuellen Stands an (**Verwaltung → Sicherung &
    Wiederherstellung → Jetzt ausführen**), wenn Sie ihn vielleicht noch
    einmal brauchen.

```sh
docker compose stop evey
docker compose run --rm evey admin restore-backup /data/backups/pre-migration-152-20260418T220130Z.sql.gz
docker compose up -d
```

Was passiert:

1. Der Befehl liest zuerst die ganze Datei: Sie muss sich fehlerfrei entpacken
   lassen und mit der Abschlusszeile von `pg_dump` enden. Ein abgeschnittener
   oder beschädigter Dump wird abgelehnt (`refusing to restore: ...`), bevor
   irgendetwas gelöscht wird.
2. Dann wartet er 5 Sekunden, bevor er etwas ändert: Strg+C bricht ab.
3. `psql` spielt den Dump mit `--single-transaction` und `ON_ERROR_STOP=1`
   ein. Bei jedem Fehler wird die Transaktion zurückgerollt, und die
   Datenbank bleibt, wie sie war.
4. Bei Erfolg lautet die letzte Zeile `[admin] restored backup <path>`.

Voraussetzungen:

- Der Dateiname muss auf `.sql.gz` enden. Die `.tar.gz`-Archive aus
  **Verwaltung → Sicherung & Wiederherstellung** werden im Verwaltungsbereich
  wiederhergestellt, nicht mit diesem Befehl.
- Die Zieldatenbank muss existieren; der Dump löscht Tabellen und legt sie neu
  an, nicht die Datenbank.
- `psql` und `gunzip` müssen im PATH liegen. Das Image stellt das sicher.

Dumps von vor einer Migration werden vor jeder Änderung des Schemas
automatisch geschrieben, unter `EVEY_BACKUP_DIR` (`/data/backups`, also
`./evey-data/backups` auf dem Rechner), mit dem Namen
`pre-migration-<schema version>-<UTC timestamp>.sql.gz`. Aufbewahrt wird der
neueste Dump jeder der 3 jüngsten Schemaversionen, die anderen werden
gelöscht. [Datenbankmigrationen](migration-runbook.md) erklärt, wann Sie
besser einen Dump wiederherstellen, statt das Schema einen Schritt
zurückzusetzen.

<!-- sources
reset-password / rotate-jwt-key: cmd/evey/admin.go - adminResetPassword (DeleteByUserID + BumpTokenEpoch), adminRotateJWTKey, endAllSessions ; epoch cache TTL: internal/app/user/token_epoch.go - epochCacheTTL
restore-backup: cmd/evey/admin.go - adminRestoreBackup, verifyDumpComplete (whole gzip + completion marker before anything runs), 5 s pause, --single-transaction, ON_ERROR_STOP=1, .sql.gz suffix)
admin commands boot the embedded database: docker/entrypoint.sh
refresh validation by stored hash: internal/app/user/auth.go - validateRefreshToken
disable signs out: internal/app/user/user_service.go - DisableUser
dump naming and retention: internal/adapters/driven/postgres/migrate_backup.go - dumpDatabase, backupFilePrefix, pruneOldBackups, defaultBackupRetention
defaults: internal/config/config.go (EVEY_JWT_PRIVATE_KEY_PATH, EVEY_BACKUP_DIR)
-->
