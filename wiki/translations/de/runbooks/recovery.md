---
source_sha: 75f950590ae90c9a9a77cfcea37f93cbbe8eb70a0dbea1df56d154a205d2fc12
---
# Runbook: Wiederherstellung

Vorgehen auf der Kommandozeile für die Fälle, in denen Sie sich nicht mehr
anmelden können, der JWT-Schlüssel womöglich kompromittiert ist oder die
Datenbank auf eine Sicherung von vor der Migration zurückgesetzt werden muss.

Alle Befehle laufen als `evey admin <subcommand>` und lesen ihre Konfiguration
aus denselben `EVEY_*`-Umgebungsvariablen wie der Server. Sie starten den Server
**nicht** - die meisten dürfen Sie gefahrlos bei gestopptem Server ausführen.

## reset-password

Erzeugt ein neues Passwort für einen bestehenden Benutzer. Das neue Passwort
erscheint genau einmal auf stdout - greifen Sie es sofort ab.

```sh
evey admin reset-password --user admin@example.com
```

Ausgabe:
```
user:     admin@example.com
user_id:  usr-xxxxxxxx
password: <random 24-byte base64url>
```

Anmerkungen:
- Am Benutzerdatensatz wird `updated_at` hochgesetzt, sonst ändert sich nichts.
- Bestehende Sitzungen bleiben gültig, bis sie von selbst ablaufen. Wollen Sie
  den Benutzer zusätzlich überall abmelden, führen Sie danach
  `rotate-jwt-key` aus.
- Das Passwort im Klartext wird nie protokolliert. Verlieren Sie die Ausgabe,
  müssen Sie den Befehl erneut ausführen.

## rotate-jwt-key

Macht jede aktive Sitzung ungültig, indem der RSA-Signaturschlüssel ersetzt
wird.

```sh
evey admin rotate-jwt-key
```

Was dabei geschieht:
- Die aktuelle Schlüsseldatei (Pfad = `EVEY_JWT_PRIVATE_KEY_PATH`, Standard
  `/data/jwt.key`) wird in `<path>.rotated-<UTC-timestamp>` umbenannt.
- Beim nächsten Serverstart erzeugt `LoadOrGenerateRSAKey` einen frischen
  RSA-Schlüssel mit 2048 Bit.
- Sämtliche zuvor ausgegebenen Access-Tokens fallen bei der Signaturprüfung
  durch → jeder Benutzer ist abgemeldet.
- Refresh-Tokens sind JWTs mit demselben Schlüssel und damit ebenfalls tot -
  aufräumen müssen Sie da nichts.

Wann Sie ihn einsetzen:
- Verdacht auf einen abgeflossenen Schlüssel.
- Alle Sitzungen müssen auf einen Schlag beendet werden (Reaktion auf einen
  Vorfall, Abgang eines externen Mitarbeiters).

## restore-backup

Spielt ein `pg_dump.gz` in die konfigurierte Datenbank ein, das aus der
automatischen Sicherung vor einer Migration stammt (oder aus einer beliebigen
anderen Quelle). **Vorhandene Daten werden überschrieben.**

```sh
evey admin restore-backup /data/backups/pre-migration-15-20260418T220130Z.sql.gz
```

Ablauf:
1. Der Befehl wartet 5 Sekunden, bevor er irgendetwas tut - mit Strg+C brechen
   Sie ab.
2. Die Datei läuft über `gunzip | psql` in die konfigurierte Datenbank, mit
   `--single-transaction` und `ON_ERROR_STOP=1`.
3. Bei jedem Fehler rollt psql die Transaktion zurück - die Datenbank bleibt in
   ihrem vorherigen Zustand.

Voraussetzungen:
- `psql` und `gunzip` müssen im PATH liegen (beide sind im Produktivimage
  enthalten, neben `pg_dump`).
- Die Zieldatenbank muss existieren; der Dump enthält nur DROP und CREATE für
  Tabellen, nicht die Datenbank selbst.
- Der konfigurierte `EVEY_DB_USER` muss der Eigentümer der Zieldatenbank sein.

Sicherungen vor einer Migration entstehen automatisch vor jedem `migrate.Up()`
und liegen unter `EVEY_BACKUP_DIR` (Standard `/data/backups`). Die letzten drei
bleiben erhalten, ältere werden weggeräumt.
