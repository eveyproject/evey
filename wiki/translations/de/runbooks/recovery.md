---
source_sha: ee8527dd07b1c7262f2f353e0f33f6004d9ae87c801d3274d9c9a31c109637be
reviewed: false
---
# Wiederherstellung

Kommandozeilenverfahren für Fälle, in denen Sie sich nicht mehr anmelden können,
der JWT-Schlüssel vermutlich kompromittiert ist oder die Datenbank auf eine
Sicherung von vor einer Migration zurückgesetzt werden muss.

Alle Befehle laufen als `evey admin <subcommand>` und lesen ihre Konfiguration
aus denselben `EVEY_*`-Umgebungsvariablen wie der Server. Sie starten den
Server **nicht** - die meisten lassen sich gefahrlos bei gestopptem Server
ausführen.

## reset-password

Erzeugt ein neues Passwort für einen bestehenden Benutzer. Das neue Passwort
wird einmal auf stdout ausgegeben - halten Sie es sofort fest.

```sh
evey admin reset-password --user admin@example.com
```

Ausgabe:
```
user:     admin@example.com
user_id:  usr-xxxxxxxx
password: <random 24-byte base64url>
```

Hinweise:
- Das Feld `updated_at` des Benutzers wird aktualisiert, sonst ändert sich
  nichts.
- Bestehende Sitzungen bleiben bis zu ihrem natürlichen Ablauf gültig. Um den
  Benutzer überall abzumelden, deaktivieren Sie das Konto unter
  **Admin → Benutzer** (**Deaktivieren**) und aktivieren es danach wieder: Das
  Deaktivieren löscht seine Refresh-Tokens und macht seine Zugriffstokens
  sofort ungültig.
- Das Klartextpasswort wird nie protokolliert. Geht die Ausgabe verloren, muss
  der Befehl erneut ausgeführt werden.

## rotate-jwt-key

Ersetzt den RSA-Schlüssel, der die Zugriffstokens signiert.

```sh
evey admin rotate-jwt-key
```

Verhalten:
- Die aktuelle Schlüsseldatei (Pfad = `EVEY_JWT_PRIVATE_KEY_PATH`, Standard
  `/data/jwt.key`) wird in `<path>.rotated-<UTC-timestamp>` umbenannt.
- Beim nächsten Serverstart erzeugt `LoadOrGenerateRSAKey` einen neuen
  2048-Bit-RSA-Schlüssel.
- Alle zuvor ausgestellten Zugriffstokens scheitern an der
  Signaturprüfung.
- Refresh-Tokens werden gegen den in der Datenbank gespeicherten Hash
  geprüft, nicht gegen den Schlüssel, und bleiben daher gültig: Eine App, deren
  Zugriffstoken abgelehnt wird, erneuert es und macht weiter. Um außerdem jede
  Sitzung zu beenden, leeren Sie die Tabelle:
  ```sql
  DELETE FROM refresh_tokens;
  ```

Wann einsetzen:
- Vermutetes Schlüsselleck: Niemand kann mit dem alten Schlüssel noch gültige
  Zugriffstokens erzeugen.
- Alle Sitzungen beenden: zusammen mit dem `DELETE` oben. Für ein einzelnes
  Konto genügt es, es unter **Admin → Benutzer** zu deaktivieren.

## restore-backup

Spielt einen `pg_dump.gz` aus der Sicherung vor einer Migration (oder aus einer
anderen Quelle) in die konfigurierte Datenbank ein. **Überschreibt vorhandene
Daten.** Die Datei muss auf `.sql.gz` enden: Die `.tar.gz`-Sicherungen von
**Admin → Sicherung & Wiederherstellung** werden im Admin-Bereich
wiederhergestellt, nicht mit diesem Befehl.

```sh
evey admin restore-backup /data/backups/pre-migration-15-20260418T220130Z.sql.gz
```

Ablauf:
1. Der Befehl wartet 5 Sekunden, bevor er etwas tut - Strg+C bricht ab.
2. Die Datei wird mit `--single-transaction` und `ON_ERROR_STOP=1` durch
   `gunzip | psql` in die konfigurierte Datenbank gestreamt.
3. Bei jedem Fehler rollt psql die Transaktion zurück - die Datenbank bleibt in
   ihrem vorherigen Zustand.

Voraussetzungen:
- `psql` und `gunzip` müssen im PATH liegen (beide sind im Produktions-Image
  neben `pg_dump` enthalten).
- Die Zieldatenbank muss existieren; der Dump enthält nur DROP/CREATE für
  Tabellen, nicht für die Datenbank selbst.
- Der konfigurierte `EVEY_DB_USER` muss Eigentümer der Zieldatenbank sein.

Sicherungen vor einer Migration entstehen automatisch vor jedem
`migrate.Up()` und liegen unter `EVEY_BACKUP_DIR` (Standard `/data/backups`).
Der neueste Dump jeder der 3 jüngsten Schemaversionen wird behalten; die
anderen werden entfernt.
