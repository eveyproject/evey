---
source_sha: cb3a20b8cc9545de2abfb60adb9e7c2272d0ad6fbc8666679ce2cb180603bae4
reviewed: false
---
# Störungshandbuch

Erste Maßnahmen bei häufigen Ausfällen von Evey. Jeder Abschnitt: **Symptom → Diagnose → Behebung**.

Die Befehle zielen auf die Standardinstallation: ein einziger Container, `evey-server`, in dem Postgres und Meilisearch laufen; ihre Logs sind die Zeilen `postgres:` und Meilisearch von `docker logs evey-server`. Der Aufbau der `docker-compose.yml` des Repositorys fügt Container `evey-postgres` und `evey-caddy` hinzu ([Erweiterte Bereitstellungen](../operations/advanced-deployments.md)).

Um SQL gegen die eingebettete Datenbank auszuführen:

```sh
docker exec evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

Standard-HTTP-Port: `9091` (`EVEY_HTTP_PORT`). Health-Endpunkt: `/healthz`.

**Logs.** Zwei Kopien desselben Stroms: die tägliche Datei `evey-data/logs/evey-YYYY-MM-DD.log` (`EVEY_LOG_DIR`, `/data/logs` im Image) und `docker logs evey-server`. Die Datei ist maßgeblich: Der json-file-Treiber rotiert `docker logs`, und nach einer Rotation kann der Befehl Stunden vor dem Container aufhören - das `stop_detail` eines Lasttests oder ein Kalibrierungsurteil sieht dann verschwunden aus, obwohl es nur wegrotiert wurde. Stimmen beide nicht überein, lesen Sie die Datei, oder als root die rohen Dateien `/var/lib/docker/containers/<id>/*-json.log*`. Die `docker logs`-Einzeiler unten setzen ein aktuelles, nicht rotiertes Zeitfenster voraus.

## 1. Server nicht erreichbar

**Symptom**: Clients können sich nicht verbinden; `curl http://localhost:9091/healthz` läuft in ein Timeout oder meldet connection refused.

**Diagnose**:
```sh
docker ps --filter name=evey-server
docker logs --tail=200 evey-server
docker inspect --format='{{.State.Health.Status}}' evey-server
ss -ltnp | grep 9091                # lauscht der Port wirklich?
curl -v http://localhost:9091/healthz
```

**Häufige Ursachen und Behebung**:
- Container beendet → `docker logs evey-server` für den Panic, beheben, `docker compose up -d evey`.
- Healthcheck schlägt fehl, weil Datenbank oder Meili ausgefallen sind → siehe Abschnitte 4 und 5.
- Portkonflikt auf dem Host → `EVEY_HTTP_PORT` ändern oder den Port freigeben.
- Vorgeschalteter Caddy leitet nicht weiter → `docker logs evey-caddy`, `EVEY_DOMAIN` prüfen und ob der Healthcheck von `evey` grün ist (Caddy hängt davon ab).

## 2. Alle Anmeldungen liefern 401

**Symptom**: Jeder Benutzer - auch Administratoren, die gestern angemeldet waren - erhält 401 von `/api/v1/auth/*`.

**Diagnose**:
```sh
docker logs --since 10m evey-server | grep -iE 'epoch|jwt|auth|signing|private key'
docker exec evey-server ls -l /data/jwt.key
```

**Suchen Sie zuerst die Epoch-Zeile.** Jede authentifizierte Anfrage prüft die
Token-Epoche des Benutzers erneut gegen die Datenbank (ADR-0019), und diese
Prüfung schlägt GESCHLOSSEN fehl. Eine langsame, gesperrte oder unerreichbare
Datenbank lässt also jede Anfrage des Servers dasselbe 401 liefern wie ein
abgelaufenes Token - was genau wie ein JWT-Problem aussieht und keines ist. Der
Server sagt es ausdrücklich:

```
auth rejected: the token epoch could not be read, so every authenticated
request is failing - check the database, not the JWT key
```

Ist diese Zeile vorhanden, lesen Sie diesen Abschnitt nicht weiter und gehen
Sie zu §4 (Datenbank). `/healthz` kann hier grün bleiben: Es pingt nur die
Verbindung, es führt die Epoch-Abfrage nicht aus. Der Prometheus-Zähler ist
`evey_auth_failures_total{reason="epoch_lookup_failed"}`.

**Häufige Ursachen und Behebung**:
- **JWT-Schlüssel fehlt oder wurde geändert** ist selten die Ursache. Ein nicht lesbarer oder beschädigter `EVEY_JWT_PRIVATE_KEY_PATH` (Standard `/data/jwt.key`) verhindert den Start des Servers; ein fehlender wird neu erzeugt. Mit dem alten Schlüssel signierte Zugriffstoken werden dann abgewiesen, aber die Apps erneuern sie mit ihrem Refresh-Token, das in der Datenbank geprüft wird, nicht gegen den Schlüssel: Die Benutzer bleiben angemeldet. Prüfen Sie, dass das Volume `evey-data` unter `/data` eingebunden ist: Ohne es fehlt auch die Datenbank.
- **Ratenbegrenzung greift bei jeder Anfrage**: Prüfen Sie `EVEY_RATELIMIT_LOGIN_MAX` (Standard `10` pro IP und Minute) und `EVEY_RATELIMIT_GLOBAL_MAX` (Standard `300`). Ein Reverse Proxy, der den ganzen Verkehr als eine einzige IP weiterleitet, löst sie schnell aus - tragen Sie den Proxy unter **Vertrauenswürdige Proxys** in Admin → Fernzugriff ein (gilt ohne Neustart; `EVEY_TRUSTED_PROXIES` funktioniert auch). Die Entwicklungskonfiguration setzt bereits `1000` / `10000`.
- Uhrzeitabweichung zwischen Server und Clients → `date -u` auf dem Host.

## 3. Die Transkodierung schlägt fehl / Videos laufen nicht transkodiert

**Symptom**: Die Wiedergabe funktioniert bei Direktwiedergabe und scheitert, sobald eine Transkodierung nötig ist. Fehler im Player, die Server-Logs zeigen Exit-Codes von ffmpeg.

**Diagnose**:
```sh
docker exec evey-server which ffmpeg ffprobe
docker exec evey-server ffmpeg -version | head -1
docker logs --since 10m evey-server | grep -iE 'ffmpeg|transcode|hls'
docker exec evey-server ls -ld /cache/transcode /data/trickplay
docker exec evey-server df -h /cache
```

**Häufige Ursachen und Behebung**:
- **ffmpeg fehlt** (eigenes Image ohne ffmpeg): aus dem offiziellen `Dockerfile` neu bauen.
- **Cache-Verzeichnis nicht beschreibbar**: `EVEY_TRANSCODING_CACHE_DIR` ist im Code standardmäßig `/data/transcode`, aber die Compose-Datei legt es über das benannte Volume `evey-cache` auf `/cache/transcode`. `EVEY_TRICKPLAY_CACHE_DIR` ist im Image `/data/trickplay` (`/cache/trickplay` ist der Standard außerhalb davon). Beschreibbarkeit prüfen:
  ```sh
  docker exec evey-server touch /cache/transcode/.write-test && \
    docker exec evey-server rm /cache/transcode/.write-test
  ```
- **Cache voll** → siehe Abschnitt 6.
- **Die Hardwarebeschleunigung verhält sich falsch**: Stellen Sie **Hardware-Beschleunigung** unter **Admin → Transkodierung → Encode-Kalibrierung** auf **Aus (nur Software)** (gilt sofort, mit neuer Messung), um jede Transkodierung auf dem Prozessor zu halten, und prüfen Sie, dass das Problem von der Hardware kommt, bevor Sie zu **Auto** zurückkehren.
- **„Der Server transkodiert gerade andere Streams"**: Die Kapazität für laufende Wiedergabe wird gemessen, nicht eingestellt; siehe [Transkodierung und Serverkapazität](../playback/transcoding.md#sharing-the-capacity). `EVEY_TRANSCODING_MAX_CONCURRENT` (**Max. gleichzeitig** unter **Admin → Transkodierungs-Cache**) betrifft nur Transkodierungsaufträge außerhalb der laufenden Wiedergabe.

### 3c. „Es puffert / es hat abgebrochen", und die Logs sagen nicht warum

**Symptom**: Ein Zuschauer meldet Aussetzer oder Abbrüche bei einem transkodierten
Titel, und die Server-Logs zeigen `hls-vod: segment timeout` mit
`err:"context canceled"` - was bedeutet, dass der CLIENT aufgelegt hat, nicht
dass der Server in ein Timeout lief.

**Diagnose**: Der Server protokolliert bereits jede Entscheidung, die er
getroffen hat. Beginnen Sie dort, das kostet nichts:

```sh
# Warum eine Segmentanfrage einen Encoder wiederverwendet oder gestartet hat, und was es
# den Player gekostet hat. Nur protokolliert, wenn es etwas kostet: eine Wartezeit über ein
# Segment hinaus, oder ein Start. `over_client_patience: true` ist der rauchende Colt.
docker exec evey-server grep '"software: reach decision"' /data/logs/evey-$(date +%F).log | tail -40

# Die Encoder selbst: Starts, Abbrüche und ihr Grund.
docker exec evey-server grep -E '"software: session (started|killed)"|per-hash cap' \
  /data/logs/evey-$(date +%F).log | tail -40
```

Diese Zeilen sagen, was der Server *entschieden* hat. Wenn Sie brauchen, was
ffmpeg tatsächlich Sekunde für Sekunde *erreicht* hat - die echte Position der
Front über die Zeit -, schalten Sie die Fortschrittsaufzeichnung pro Encoder ein:

```sh
# AN. Gilt für den NÄCHSTEN Encoder; kein Neustart nötig. $TOKEN ist ein Admin-JWT.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"true"}'

# Das Problem nachstellen, dann eine Sitzung lesen: out_time = bisher erzeugter Inhalt,
# speed = wie weit der Encoder der Echtzeit voraus war.
docker exec evey-server ls -t /data/logs/ffmpeg | head
docker exec evey-server tail -20 /data/logs/ffmpeg/<file>.log

# Danach wieder AUS.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"false"}'
```

**Lassen Sie sie außerhalb einer Untersuchung aus.** Sie ist eine Datei pro
Encoder, einige MB pro Sitzung, für jede Wiedergabe auf der Maschine geschrieben -
echter Speicherplatz und echte Schreibvorgänge auf einem schwachen NAS. Der
Dateiname trägt den Entscheidungs-Hash, die Stufe (`-vN-`) und das Startsegment
(`-sN-`), passt also zum JSON-Log oben; ein Neustart an derselben Position erhält
seine eigene Datei, statt den vorherigen Lauf zu überschreiben, was das Hin und
Her der Encoder sichtbar macht.

**Häufige Ursachen und Behebung**:
- **Wiederholte kurze Sprünge nach vorn**: Jeder kann einen neuen Encoder
  starten, und die Obergrenze pro Hash (`MaxSessionsPerHash`, Standard 2)
  verdrängt den am längsten ungenutzten. Erwarten Sie `session killed reason:lru_cap`
  im Log. In Maßen normal.
- **Der Client ist ungeduldiger als der Server**: `clientSegmentPatience` (6 s)
  ist das Maß, an dem das Fenster zum Erreichen eines Sprungziels ausgerichtet
  ist; AndroidX Media3 gibt ein Segment nach ~8 s auf, während das
  `WaitBudgetDur` des Servers 60 s beträgt. Eine Zeile `reach decision` mit
  `over_client_patience: true` bedeutet, dass der Server gerade eine Wartezeit
  verlangen wollte, die kein Player abwarten würde.

### 3b. `stale_rendition`-410er / der Player lädt seinen Master mitten in der Wiedergabe neu

**Symptom**: Der Player stockt kurz und erholt sich dann; die Logs zeigen
`hls-vod: stale rendition URL refused (the ladder moved under a live player)`
und Anfragen, die mit `410 Gone` und `X-Evey-Error: stale_rendition` beantwortet
werden.

**Was es bedeutet**: Hier greift ein *Schutz*, nicht der Fehler selbst. Jede
Manifest-URI trägt den Entscheidungs-Hash, unter dem sie gebaut wurde (`fp=`).
Ein 410 bedeutet, dass ein Player eine URL aus einer Sitzung vorgelegt hat,
deren ABR-Leiter inzwischen umgeformt wurde - sie auszuliefern würde dem Decoder
Bytes aus einer anderen Variante geben (die Vorfallklasse „Regenbogenbild",
ADR 0022 §1b). Die Ablehnung erzwingt stattdessen ein sauberes Neuladen des
Masters.

**Diagnose**: Die WARN-Zeile trägt `sealed_fp` gegenüber `current_fp` und die
Medien-ID.
- Eine **Häufung direkt nach einem Deploy/Neustart oder einer Neukalibrierung**
  ist zu erwarten: Laufende Sitzungen haben eine Änderung der Fähigkeiten überdauert.
  Keine Maßnahme nötig; die Player synchronisieren sich binnen einer Abfrage.
- Ein **stetiges Tröpfeln im normalen Betrieb** bedeutet, dass etwas Leitern
  unter laufenden Streams immer wieder umformt - suchen Sie nach schwankenden
  Fähigkeiten (`docker logs evey-server | grep -iE 'calibrat|capability'`), einem
  unter Last hin- und herschaltenden Admission-Shaper oder einer pendelnden
  CMCD-Obergrenze.
- Player, die **in einer 410-Schleife hängen, ohne sich zu erholen**, halten
  einen zwischengespeicherten Master (alte App-Version, aggressives Caching von
  `.m3u8` im Proxy). Prüfen Sie, dass der Proxy keine Manifeste cacht
  (`Cache-Control: no-cache` muss durchgereicht werden).

## 4. Datenbankverbindung verweigert / Fehler

**Symptom**: Die Server-Logs enthalten `dial tcp ... connect: connection refused`, `pq: ...`, oder Abfragen laufen in ein Timeout.

**Diagnose**:
```sh
docker logs --tail=300 evey-server | grep -iE 'postgres|database'
docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
  SELECT count(*), state
    FROM pg_stat_activity
   WHERE datname = current_database()
   GROUP BY state;"'
df -h ./evey-data
```

**Häufige Ursachen und Behebung**:
- **Festplatte voll** unter `./evey-data` (die Datenbank liegt in `./evey-data/postgres`) → Platz schaffen, dann `docker compose restart evey`.
- **Verbindungspool erschöpft** (`too many connections` oder lange Wartezeiten) → `EVEY_DB_MAX_OPEN_CONNS` (Standard `25`). Hängende Sitzungen beenden:
  ```sql
  SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
   WHERE state = 'idle in transaction'
     AND now() - state_change > interval '5 minutes';
  ```
- **Separater Postgres-Container** (Aufbau des Repositorys): `docker ps --filter name=evey-postgres`, `docker logs --tail=200 evey-postgres`; `EVEY_DB_USER` / `EVEY_DB_PASSWORD` / `EVEY_DB_NAME` des Servers müssen zum Dienst `postgres` passen.

## 5. Meilisearch ausgefallen

**Symptom**: Die Suche liefert schlechtere Ergebnisse, die Logs zeigen `meilisearch unavailable, using SQL search fallback`.

**Verhalten**: Die Suche funktioniert über SQL weiter (`cmd/evey/wire.go` verdrahtet einen robusten Suchadapter, der diese Zeile protokolliert, wenn Meilisearch nicht erreichbar ist). Um online zu bleiben, ist nichts zu tun; nur die Qualität der Rangfolge sinkt.

Meilisearch läuft in `evey-server` (`EVEY_EMBEDDED_MEILI`, standardmäßig an) und lauscht nur im Container auf `127.0.0.1:7700`.

**Diagnose und Behebung**:
```sh
docker exec evey-server curl -sf http://127.0.0.1:7700/health
docker logs --tail=300 evey-server | grep -i meili
```
- Nicht gesund → `docker compose restart evey`.
- Index beschädigt → den Container stoppen, `./evey-data/meili` beiseitelegen, neu starten, dann den Index aus der Datenbank neu aufbauen: `docker exec evey-server evey admin reindex-search`.

## 6. Festplatte des Transkodierungs-Caches voll

**Symptom**: Transkodierungen scheitern mit `no space left on device`; der Healthcheck des Containers schwankt.

**Diagnose**:
```sh
docker exec evey-server df -h /cache
docker exec evey-server du -sh /cache/transcode /data/trickplay 2>/dev/null
docker volume inspect evey_evey-cache   # der von Compose erzeugte Name kann abweichen
```

**Behebung**:
- Sicheres Leeren - der Cache lässt sich neu erzeugen:
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/transcode/*'
  ```
  Das entfernt darunter auch `photos/` (verkleinerte Fotovarianten) und
  `track-art/` (das in den Tags eines Titels eingebettete Cover, einmal
  extrahiert und verkleinert) - beide werden bei der nächsten Anfrage neu
  erzeugt. **Alle leeren** unter Admin → Transkodierungs-Cache leert genauso.
- Trickplay ist teurer neu zu erzeugen; nur bei Bedarf leeren:
  ```sh
  docker exec evey-server sh -c 'rm -rf /data/trickplay/*'
  ```
- Senken Sie das **Budget des Transcode-Caches** (**Admin → Serverkonfiguration → Servereinstellungen**, Standard `10` GB), wenn das Volume des Hosts knapp ist; unabhängig vom Budget hält die Bereinigung 2 GiB auf dem Cache-Volume frei.

## 7. Ein Plugin ist abgestürzt

**Symptom**: Ein Plugin antwortet nicht mehr; die Server-Logs zeigen Panics aus dem Plugin-Runner.

**Diagnose**:
```sh
docker logs --since 30m evey-server 2>&1 | grep -iE 'plugin|pluginrunner|panic'
# Suchen nach: "panic in plugin runner", "plugin process crashed", "remove plugin sock on crash"
```

**Behebung**:
- Versuchen Sie zuerst einen gezielten Neustart ohne Ausfall des Containers: `POST /api/v1/admin/plugins/<id>/enable`. Das funktioniert auch, wenn das Plugin bereits `enabled` ist, aber im Status `Error` hängt - der Schutz in `StartPlugin` startet es neu, sobald der Status nicht `Running` ist. Ziehen Sie das einem Neustart des ganzen Servers vor, besonders wenn kein SSH-Zugang zum Host griffbereit ist.
- Hilft der gezielte Neustart nicht, deaktivieren Sie das betroffene Plugin stattdessen im Admin-Bereich.
- Ist der Server selbst instabil, neu starten: `docker compose restart evey`.
- Die Plugin-Dateien liegen unter `./evey-data/plugins/` (Host) == `/data/plugins` (Container). Das defekte Plugin-ZIP oder -Verzeichnis entfernen, neu starten.
- Den Panic-Trace und die Plugin-ID an den Autor des Plugins senden.

## 8. Ein Cast ist eingefroren, stumm geworden, oder das Telefon dreht sich endlos

Alles Folgende ist auf der Log-Seite des Admin-Bereichs lesbar (oder mit
`GET /api/v1/admin/logs/search?q=<term>&limit=200`): Es braucht keine Shell auf
der Maschine. Die Zeiten sind in UTC. Jedes Signal ist eine Zeile
`player-client: playback event reported` mit `type=` und einer Zeichenkette
`details=`. Zugangsdaten erscheinen nie in dem, was diese Routen zurückgeben
(Bearer, JWTs und Werte `token=`/`password=` lauten `[redacted]`), das Token einer
Cast-URL lässt sich also nicht über eine Log-Suche finden - suchen Sie nach der
Medien-ID, nicht nach dem Token.

1. **War der Empfänger aktiv, und bis wann?** Suchen Sie `cast_receiver_alive`.
   Der Empfänger sendet eines alle 30 s, während er abspielt; `details` enthält
   `buf=<behind>/<ahead>/<ranges>` (gepufferte Sekunden hinter und vor der
   Abspielposition) und `pos_sec`. Eine gesunde Wiedergabe zeigt `ahead` im
   Bereich einiger zehn Sekunden. **Die Zeit des letzten Signals ist der
   Moment, in dem der Empfänger gestorben ist oder beendet wurde**: Nichts danach
   bedeutet, dass der Fernseher den Empfänger nicht mehr ausführt, egal was das
   Telefon anzeigt.
2. **Warum hat er aufgehört?** Suchen Sie `cast_receiver_finished`:
   `reason=INTERRUPTED` ist ein neues LOAD, das das laufende ersetzt (suchen Sie
   ein `StartStream` einige Sekunden davor), `reason=ERROR` trägt den
   Player-Fehler. Gar kein `finished` nach dem letzten `alive` = der
   Empfängerprozess ist verschwunden (meist ein Chromecast HD, der wegen
   Speichermangels bei einem Kopie-Master von 20+ Mbps beendet wurde).
3. **Was hat das Telefon gesehen?** Suchen Sie `cast_sender_stall`: Der Sender
   meldet `state=playing frozen=<n>s pos=<p>`, wenn das Cast-SDK noch PLAYING
   sagt, die Position sich aber 20 s lang nicht bewegt hat, und
   `state=idle reason=<r>`, wenn der Empfänger einen Leerlaufgrund gemeldet hat.
   Diese Zeilen werden als WARN protokolliert, der Warnungsfilter der Log-Seite
   zeigt sie also an. Das ist die Seite des Senders in derselben Minute -
   vergleichen Sie sein `pos` mit dem letzten `pos_sec` des Empfängers.
4. **Hat die Sitzung eine Kettenreaktion ausgelöst?** Suchen Sie `refresh rejected`: Ein
   Refresh-Token „not found" meldet die App ab und wieder an (gespeicherte
   Zugangsdaten), was den Stream neu ausstellt - die Zeile nennt jetzt `jti`,
   `issued` und `age` des Tokens, was eine veraltete Kopie von einer Fälschung
   unterscheidet. Suchen Sie `StartStream`, um Neuausstellungen zu sehen; der
   Sender lädt den Empfänger bei einer neu ausgestellten URL für denselben Titel
   nicht mehr neu.
5. **War der Stream zu groß für das Gerät?** Suchen Sie `advertises a fallback
   rung` (die Spitzenbitrate des Kopie-Masters) und `plan fitted to the client's
   line` (die Messung `maxKbps` des Empfängers oder seine Rettung `abr=floor`,
   jetzt eine Eingabe des Plans - der folgende Transkodierungs-Master trägt den
   Grund `client_line_exceeded` oder `abr_floor_requested`): Eine Sitzung, die auf
   einem Kopie-Master mit 20+ Mbps begann und nach einigen Sekunden auf eine
   angepasste 1080p-Stufe fiel, ist die Notfallrettung bei der Arbeit; ein
   erneutes LOAD, das wieder auf dem Kopie-Master beginnt, ist das Muster, das
   einen Chromecast HD umbringt.
6. **Aushungern auf Serverseite?** Suchen Sie `on-demand resource refused`: Eine
   Häufung von Ablehnungen `context canceled` bei aufeinanderfolgenden Segmenten
   ist ein Vorspulen (jede Anfrage wird von der nächsten abgebrochen), kein
   Serverfehler; `served`-Zeilen mit langen Dauern schon.

Zeigen die Schritte 1-3 den Empfänger aktiv und den Sender stumm, liegt das
Problem in der Cast-Sitzung des Telefons (das SDK, das Netzwerk zum Fernseher);
hören die Signale des Empfängers zuerst auf, ist der Fernseher der Ort zum
Suchen (Speicher, Codec, Bitrate des Streams).

## Eine kompromittierte Sitzung oder ein Gerät abschalten

Symptom: Der Administrator sieht ein unerwartetes Gerät unter **Aktuelle Wiedergabe** (Admin-Bereich), oder ein Benutzer meldet ein unbekanntes Gerät in seiner Sitzungsliste.

1. Die `session_id` ermitteln:
   ```sh
   docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
     SELECT id, profile_id, device_name, client_ip, started_at
     FROM play_sessions
     WHERE ended_at IS NULL
     ORDER BY started_at DESC LIMIT 20;"'
   ```
2. Im Admin-Bereich beenden: **Admin → Aktuelle Wiedergabe**, das Stopp-Symbol der Sitzung (**Sitzung beenden**), dann **Stoppen** zur Bestätigung; oder per gRPC `AdminSessionService/KillSession` mit der `session_id`. Die Sitzung endet 5 s später, damit der Player den Stoppbefehl erhält.
3. Läuft der Stream trotzdem weiter, starten Sie den Container neu: Jeder Encoder stoppt mit ihm.
   ```sh
   docker compose restart evey
   ```
4. Ist das Konto selbst kompromittiert, **Admin → Benutzer → Deaktivieren** bei diesem Konto: Das löscht seine Refresh-Tokens und macht seine Zugriffstokens sofort ungültig, sodass jedes Gerät abgemeldet wird. Setzen Sie ein neues Passwort (`evey admin reset-password --user <email>`, was allein keine Sitzung beendet), dann **Aktivieren** Sie das Konto wieder.

## Einen hängenden Bibliotheksscan lösen

Symptom: Ein Bibliotheksscan steht seit Stunden auf „läuft", ohne Fortschritt der Dateien in den Logs.

1. Prüfen Sie, ob der Scan noch etwas protokolliert - hängt der Prozess, ist die CPU untätig und die jüngsten Logs zeigen keine Zeile `scan:`:
   ```sh
   docker logs --tail=500 evey-server | grep 'scan:'
   ```
2. Eine Scan-Sperre liegt **im Speicher** (keine Zeile in der Datenbank), ein Neustart des Servers hebt sie also auf:
   ```sh
   docker compose restart evey
   ```
3. Verursacht eine bestimmte Datei wiederholt Panics (siehe Stacktraces), stellen Sie sie unter Quarantäne, indem Sie sie aus dem Bibliothekspfad verschieben, dann erneut scannen.
4. Sind die inotify-Überwachungen erschöpft (Kernelgrenze), erhöhen Sie sie:
   ```sh
   echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf
   sudo sysctl -p
   ```

## Diagnose-Spickzettel

```sh
# Status von allem
docker compose ps
curl -sf http://localhost:9091/healthz && echo healthy

# Alle Container verfolgen
docker compose logs --tail=100 -f

# Serverkonfiguration ausgeben
docker exec evey-server env | grep ^EVEY_ | sort

# Datenbankgröße
docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
  SELECT pg_size_pretty(pg_database_size(current_database()));"'

# Festplatte des Hosts
df -h ./evey-data
```
