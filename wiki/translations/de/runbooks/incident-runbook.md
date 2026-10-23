---
source_sha: 516b8642fffb6e38095ed3717e312f92a272b9c0f020b7accc2c86330c45dbf2
reviewed: true
---
# Störungshandbuch { #incident-runbook }

Die ersten Schritte bei den häufigen Ausfällen von Evey, für den Betreiber im
Bereitschaftsdienst. Suchen Sie den Abschnitt, der zum Anzeichen passt,
führen Sie seine Prüfungen aus und wenden Sie die Lösung an, die zu Ihrem
Befund passt. Jeder Abschnitt ist gleich aufgebaut: **Symptom**,
**Diagnose**, **Lösung**.

## Bevor Sie beginnen { #before-you-start }

Die Befehle gehen von der Standardinstallation aus: ein Container,
`evey-server`, in dem Postgres und Meilisearch laufen. Deren Zeilen stehen im
selben Protokoll wie die des Servers (Präfix `postgres:` für die Datenbank).
Die `docker-compose.yml` des Repositorys fügt die Container `evey-postgres`
und `evey-caddy` hinzu; siehe
[Erweiterte Bereitstellungen](../operations/advanced-deployments.md).

**Ports.** Die HTTP-API lauscht auf `EVEY_SERVER_PORT` + 1, standardmäßig
`9091`; dort antwortet `/healthz`. Veröffentlicht wird der Port durch die
Zeile `ports:` der Compose-Datei.

**SQL-Shell** auf der eingebauten Datenbank:

```sh
docker exec -it evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

**Protokolle.** Dieselben Zeilen gibt es zweimal: in der Tagesdatei
`./evey-data/logs/evey-YYYY-MM-DD.log` (`EVEY_LOG_DIR`, `/data/logs` im
Image; eine Datei über 50 MB, `EVEY_LOG_MAX_SIZE_MB`, wird in `evey-YYYY-MM-DD-HH.log` fortgesetzt) und in `docker logs evey-server`. Maßgeblich ist die Datei: Der
json-file-Treiber von Docker rotiert `docker logs`, und nach einer Rotation
kann die Ausgabe des Befehls um Stunden hinter dem Container zurückliegen. Widersprechen sich die
beiden, lesen Sie die Datei. Die Seite **Protokolle & Debug** im
Verwaltungsbereich durchsucht dieselben Zeilen
(`GET /api/v1/admin/logs/search?q=<term>&limit=200`). Bearer-Token, JWTs und
Werte von `token=` / `password=` stehen dort als `[redacted]`. Suchen Sie
also nach einer Medien-ID, nie nach einem Token.

## Der Server antwortet nicht { #the-server-does-not-answer }

**Symptom**: Clients können sich nicht verbinden;
`curl http://localhost:9091/healthz` läuft in eine Zeitüberschreitung oder
wird abgewiesen.

**Diagnose**:

```sh
docker ps --filter name=evey-server
docker inspect --format='{{.State.Health.Status}}' evey-server
docker logs --tail=200 evey-server
ss -ltnp | grep 9091                # lauscht der Port überhaupt?
curl -v http://localhost:9091/healthz
```

**Lösung**:

- Container beendet: Die Panic steht am Ende von `docker logs evey-server`.
  Beheben Sie die Ursache, dann `docker compose up -d evey`.
- Gesundheitsprüfung schlägt fehl, weil die Datenbank oder die Suchmaschine
  ausgefallen ist: siehe [Datenbankfehler](#database-errors) und
  [Die Suche ist eingeschränkt](#search-is-degraded).
- Portkonflikt auf dem Rechner: Ändern Sie die Rechnerseite der Zeile
  `ports:` in der Compose-Datei, oder geben Sie den Port frei.
- Caddy davor leitet nicht weiter (Aufbau des Repositorys):
  `docker logs evey-caddy`, prüfen Sie `EVEY_DOMAIN` und ob die
  Gesundheitsprüfung von `evey` erfolgreich ist, denn Caddy wartet auf sie.

## Jede Anmeldung liefert 401 { #every-login-returns-401 }

**Symptom**: Jeder Benutzer, auch Administratoren, bekommt 401 von
`/api/v1/auth/*`.

**Diagnose**:

```sh
docker logs --since 10m evey-server | grep -iE 'epoch|jwt|auth|signing|private key'
docker exec evey-server ls -l /data/jwt.key
```

Suchen Sie zuerst nach der Zeile zur Epoche. Jede angemeldete Anfrage prüft
die Token-Epoche des Benutzers erneut in der Datenbank, und diese Prüfung
schlägt im Zweifel fehl: Ist die Datenbank langsam, gesperrt oder nicht
erreichbar, liefert jede Anfrage dasselbe 401 wie ein abgelaufenes Token. Das
sieht aus wie ein Problem mit dem JWT, ist aber keines. Der Server meldet das:

```
auth rejected: the token epoch could not be read, so every authenticated request is failing - check the database, not the JWT key
```

Steht diese Zeile da, gehen Sie zu [Datenbankfehler](#database-errors).
`/healthz` kann währenddessen weiter erfolgreich antworten: Es prüft die Verbindung, führt
aber die Abfrage der Epoche nicht aus. Der Prometheus-Zähler ist
`evey_auth_failures_total{reason="epoch_lookup_failed"}`.

**Lösung**:

- **Ein fehlender oder geänderter JWT-Schlüssel ist selten die Ursache.** Eine
  unlesbare Schlüsseldatei (`EVEY_JWT_PRIVATE_KEY_PATH`, `/data/jwt.key`)
  verhindert den Start des Servers; eine fehlende wird neu erzeugt.
  Zugangstoken, die mit dem alten Schlüssel signiert wurden, werden dann
  abgelehnt. Die Apps erneuern sie aber mit ihrem Refresh-Token, das in der
  Datenbank geprüft wird, nicht gegen den Schlüssel: Die Benutzer bleiben
  angemeldet. Prüfen Sie, dass das Volume `./evey-data` unter `/data`
  eingebunden ist; ohne es fehlt auch die Datenbank.
- **Die Begrenzung der Anfragen greift bei jeder Anfrage**:
  `EVEY_RATELIMIT_LOGIN_MAX` (`10` pro IP und Minute) und
  `EVEY_RATELIMIT_GLOBAL_MAX` (`300`). Ein Reverse Proxy, der alles unter
  einer einzigen Adresse weiterreicht, löst sie schnell aus: Tragen Sie den
  Proxy unter **Vertrauenswürdige Proxys (IP oder CIDR, kommagetrennt)** in
  **Verwaltung → Fernzugriff** ein (gilt ohne Neustart;
  `EVEY_TRUSTED_PROXIES` ist der Ersatz). Die
  `docker-compose.override.yml` des Repositorys hebt beide für die
  Entwicklung auf `1000` / `10000` an.
- **Abweichende Uhrzeit** zwischen Server und Clients: `date -u` auf dem
  Rechner.

## Die Transkodierung schlägt fehl { #transcoded-playback-fails }

**Symptom**: Die Direktwiedergabe funktioniert, eine Wiedergabe mit
Transkodierung schlägt fehl; der Player meldet einen Fehler, und das
Protokoll des Servers zeigt Exit-Codes von ffmpeg.

**Diagnose**:

```sh
docker exec evey-server which ffmpeg ffprobe
docker exec evey-server ffmpeg -version | head -1
docker logs --since 10m evey-server | grep -iE 'ffmpeg|transcode|hls'
docker exec evey-server ls -ld /cache/transcode /data/trickplay
docker exec evey-server df -h /cache
docker exec evey-server touch /cache/transcode/.write-test && \
  docker exec evey-server rm /cache/transcode/.write-test
```

**Lösung**:

- **ffmpeg fehlt** (ein eigenes Image): Bauen Sie aus dem offiziellen
  `Dockerfile` neu.
- **Cache-Ordner nicht beschreibbar**: Das Image setzt
  `EVEY_TRANSCODING_CACHE_DIR=/cache/transcode` (das Volume `evey-cache`) und
  `EVEY_TRICKPLAY_CACHE_DIR=/data/trickplay`; ein Eintrag unter
  `environment:` in der Compose-Datei überschreibt sie. Außerhalb des Images
  sind die Go-Standardwerte `/data/transcode` und `/cache/trickplay`.
- **Cache voll**: siehe [Die Cache-Festplatte ist voll](#the-cache-disk-is-full).
- **Hardware-Beschleunigung verursacht Probleme**: Stellen Sie
  **Hardwarebeschleunigung** unter **Verwaltung → Transkodierung →
  Encode-Kalibrierung** auf **Aus (nur Software)** (gilt sofort und
  kalibriert neu), damit jede Transkodierung auf dem Prozessor läuft.
  Stellen Sie wieder auf **Auto (bestmöglich)**, sobald die Hardware
  repariert ist.
- **„Der Server transkodiert gerade andere Streams“**: Die Kapazität für
  laufende Wiedergabe wird gemessen, nicht eingestellt; siehe
  [Die Kapazität teilen](../playback/transcoding.md#sharing-the-capacity).
  `EVEY_TRANSCODING_MAX_CONCURRENT` (**Max. gleichzeitig** unter
  **Verwaltung → Transkodierungs-Cache**) gilt nur für
  Transkodierungsaufträge außerhalb der laufenden Wiedergabe.

### Es puffert oder bricht ab, und das Protokoll sagt nicht warum { #it-buffers-or-cuts-out-and-the-log-does-not-say-why }

**Symptom**: Ein Zuschauer meldet Unterbrechungen bei einem transkodierten Titel; das
Protokoll zeigt `hls-vod: segment timeout` mit `err:"context canceled"`. Das
heißt: Der Client hat die Verbindung beendet, nicht der Server hat zu lange gebraucht.

**Diagnose**: Beginnen Sie mit dem, was der Server schon aufgezeichnet hat,
das erfordert keinen zusätzlichen Aufwand:

```sh
# Warum eine Segmentanfrage einen Encoder wiederverwendet oder einen neuen
# gestartet hat, und was das den Player gekostet hat. Nur protokolliert, wenn
# es etwas kostet. `over_client_patience: true` ist der klare Beweis.
docker exec evey-server grep '"software: reach decision"' /data/logs/evey-$(date +%F)*.log | tail -40

# Die Encoder selbst: Starts, Abbrüche und ihr Grund.
docker exec evey-server grep -E '"software: session (started|killed)"|per-hash cap' \
  /data/logs/evey-$(date +%F)*.log | tail -40
```

Diese Zeilen sagen, was der Server entschieden hat. Um Sekunde für Sekunde
zu sehen, was ffmpeg geschafft hat, schalten Sie das Fortschrittsprotokoll
pro Encoder ein (`$TOKEN` ist ein Admin-JWT):

```sh
# AN. Gilt für den nächsten Encoder, ohne Neustart.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"true"}'

# Das Problem nachstellen, dann eine Sitzung lesen: out_time = bisher
# erzeugter Inhalt, speed = wie weit der Encoder der Echtzeit voraus war.
docker exec evey-server ls -t /data/logs/ffmpeg | head
docker exec evey-server tail -20 /data/logs/ffmpeg/<file>.log

# AUS, wenn Sie fertig sind.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"false"}'
```

!!! note "Hinweis"
    Lassen Sie das Fortschrittsprotokoll außerhalb einer Untersuchung aus:
    eine Datei pro Encoder, einige MB pro Sitzung, für jede Wiedergabe auf
    dem Rechner. Der Dateiname enthält den Hash der Entscheidung, die Stufe
    (`-vN-`) und das Startsegment (`-sN-`). So passt er zum JSON-Protokoll.
    Ein Neustart an derselben Stelle bekommt eine eigene Datei; genau daran
    sieht man, wenn Encoder ständig neu starten.

**Lösung**:

- **Wiederholte kurze Sprünge nach vorn**: Jeder kann einen neuen Encoder
  starten, und die Grenze pro Hash (`MaxSessionsPerHash`, standardmäßig 2,
  im Profil für wenig Arbeitsspeicher 1) beendet den am längsten nicht
  benutzten: `session killed reason:lru_cap` im Protokoll. In Maßen
  normal.
- **Der Client wartet kürzer als der Server**: Das Fenster für das
  Erreichen eines Sprungziels ist auf `clientSegmentPatience` (6 s)
  ausgelegt, während das eigene Warte-Budget des Servers (`WaitBudgetDur`)
  60 s beträgt. Eine Zeile `reach decision` mit
  `over_client_patience: true` bedeutet: Der Server war dabei, eine Wartezeit
  zu verlangen, die kein Player abwartet.

### `stale_rendition`-410er, der Player lädt seinen Master mitten in der Wiedergabe neu { #stale_rendition-410s-the-player-refetches-its-master-mid-play }

**Symptom**: Der Player stockt kurz und läuft dann weiter; das Protokoll zeigt
`hls-vod: stale rendition URL refused (the ladder moved under a live player)`
und Anfragen, die mit `410 Gone` und `X-Evey-Error: stale_rendition`
beantwortet werden.

Das ist eine Schutzfunktion, die greift, nicht der Fehler selbst. Jede
URI im Manifest trägt den Hash der Entscheidung, unter der sie gebaut wurde
(`fp=`). Ein 410 bedeutet: Ein Player hat eine URL aus einer Sitzung
vorgelegt, deren ABR-Leiter inzwischen umgebaut wurde. Sie auszuliefern
würde dem Decoder Bytes aus einer anderen Variante geben. Die Ablehnung
erzwingt ein sauberes Neuladen des Masters.

**Diagnose**: Die WARN-Zeile enthält `sealed_fp`, `current_fp` und die
Medien-ID.

- Eine **Häufung direkt nach einem Ausrollen, einem Neustart oder einer neuen
  Kalibrierung** ist zu erwarten: Laufende Sitzungen haben eine Änderung der
  Fähigkeiten überdauert. Die Player synchronisieren sich innerhalb einer
  Abfrage neu.
- Eine **geringe, aber stetige Zahl im normalen Betrieb** heißt, dass etwas die
  Leitern unter laufenden Streams immer wieder umbaut: Suchen Sie nach
  schwankenden Fähigkeiten (`docker logs evey-server | grep -iE
  'calibrat|capability'`), einem Admission-Shaper, der unter Last ständig
  umsteuert, oder einer CMCD-Obergrenze, die hin und her springt.
- Player, die **bei 410 in einer Schleife hängen**, halten einen
  zwischengespeicherten Master (eine alte App-Version, ein Proxy, der
  `.m3u8` cacht). Der Proxy muss `Cache-Control: no-cache` durchreichen.

## Datenbankfehler { #database-errors }

**Symptom**: Das Protokoll enthält `dial tcp ... connect: connection refused`,
`pq: ...`, oder Abfragen laufen in eine Zeitüberschreitung.

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

**Lösung**:

- **Festplatte voll** unter `./evey-data` (der Cluster liegt in
  `./evey-data/postgres`): Platz freigeben, dann
  `docker compose restart evey`.
- **Verbindungspool erschöpft** (`too many connections`, lange Wartezeiten):
  `EVEY_DB_MAX_OPEN_CONNS` (Standard `25`). Hängende Sitzungen beenden:
  ```sql
  SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
   WHERE state = 'idle in transaction'
     AND now() - state_change > interval '5 minutes';
  ```
- **Eigener Postgres-Container** (Aufbau des Repositorys):
  `docker ps --filter name=evey-postgres`,
  `docker logs --tail=200 evey-postgres`; `EVEY_DB_USER` /
  `EVEY_DB_PASSWORD` / `EVEY_DB_NAME` des Servers müssen zum Dienst
  `postgres` passen.

## Die Suche ist eingeschränkt { #search-is-degraded }

**Symptom**: Die Suche liefert schlechtere Ergebnisse; das Protokoll zeigt
`meilisearch unavailable, using SQL search fallback`.

Die Suche funktioniert über SQL weiter, solange Meilisearch nicht erreichbar
ist; nur die Reihenfolge der Treffer wird schlechter, niemand ist offline.
Meilisearch läuft in `evey-server` (`EVEY_EMBEDDED_MEILI`, standardmäßig an)
auf der Loopback-Adresse des Containers, Port 7700.

**Diagnose**:

```sh
docker exec evey-server curl -sf http://127.0.0.1:7700/health
docker logs --tail=300 evey-server | grep -i meili
```

**Lösung**:

- Nicht gesund: `docker compose restart evey`.
- Index beschädigt: Container stoppen, `./evey-data/meili` an einen anderen Ort verschieben,
  wieder starten, dann den Index aus der Datenbank neu aufbauen:
  ```sh
  docker exec evey-server evey admin reindex-search
  ```
  Der Neuaufbau ist ein Befehl, keine Aktion im Verwaltungsbereich.

## Die Cache-Festplatte ist voll { #the-cache-disk-is-full }

**Symptom**: Transkodierungen schlagen mit `no space left on device` fehl;
die Gesundheitsprüfung wechselt ständig zwischen Erfolg und Fehler.

**Diagnose**:

```sh
docker exec evey-server df -h /cache
docker exec evey-server du -sh /cache/transcode /data/trickplay 2>/dev/null
docker volume inspect evey_evey-cache   # das Compose-Präfix kann abweichen
```

**Lösung**:

- Leeren Sie den Transkodierungs-Cache, er lässt sich neu erzeugen:
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/transcode/*'
  ```
  Das entfernt auch `photos/` (verkleinerte Fassungen von Fotos) und
  `track-art/` (Cover, die aus den Tags von Audiodateien extrahiert
  wurden) darunter; beide werden bei der nächsten Anfrage neu gebaut.
  **Alle leeren** unter **Verwaltung → Transkodierungs-Cache** leert
  denselben Cache aus dem Verwaltungsbereich.
- Trickplay ist aufwendiger neu zu erzeugen; leeren Sie es nur, wenn es sein muss:
  ```sh
  docker exec evey-server sh -c 'rm -rf /data/trickplay/*'
  ```
- Senken Sie **Budget des Transkodierungs-Caches (GB)** unter **Verwaltung → Transkodierung** (Standard `10`), wenn das
  Volume auf dem Rechner knapp ist. Unabhängig vom Budget hält die
  Aufräumfunktion 2 GiB auf dem Cache-Volume frei.

## Ein Plugin ist abgestürzt { #a-plugin-crashed }

**Symptom**: Ein Plugin antwortet nicht mehr; das Protokoll zeigt Panics aus
dem Plugin-Runner.

**Diagnose**:

```sh
docker logs --since 30m evey-server 2>&1 | grep -iE 'plugin|pluginrunner|panic'
# suchen nach: "panic in plugin runner", "plugin process crashed", "remove plugin sock on crash"
```

**Lösung**:

1. Starten Sie das Plugin neu, ohne den Server neu zu starten: **Aktivieren**
   Sie es erneut unter **Verwaltung → Plugins**, oder
   `POST /api/v1/admin/plugins/<id>/enable`. Das funktioniert, wenn das
   Plugin aktiviert ist, aber in `Error` hängt: Der Start lehnt nur ein
   Plugin ab, das schon `Running` oder `Initializing` ist.
2. Hilft das nicht, **Deaktivieren** Sie das Plugin im Verwaltungsbereich.
3. Ist der Server selbst instabil: `docker compose restart evey`.
4. Die Dateien der Plugins liegen auf dem Rechner unter
   `./evey-data/plugins/` (`/data/plugins` im Container). Entfernen Sie den
   Ordner des defekten Plugins und starten Sie neu.
5. Schicken Sie den Panic-Trace und die ID des Plugins an seinen Autor.

## Ein Cast ist eingefroren, stumm geworden, oder das Handy lädt endlos { #a-cast-froze-went-silent-or-the-phone-keeps-spinning }

Alles hier lässt sich unter **Verwaltung → Protokolle & Debug** lesen; Sie
brauchen keine Shell auf dem Rechner. Die Zeiten sind in UTC. Jedes Signal
ist eine Zeile `player-client: playback event reported` mit `type=` und einer
Zeichenkette `details=`.

1. **Lief der Empfänger noch, und bis wann?** Suchen Sie
   `cast_receiver_alive`. Der Empfänger sendet alle 30 s eines, solange er
   abspielt; `details` enthält `buf=<behind>/<ahead>/<ranges>` (gepufferte
   Sekunden rund um die Abspielposition) und `pos_sec`. Bei normaler
   Wiedergabe liegt `ahead` bei einigen zehn Sekunden. Die Zeit des letzten
   Signals ist der Moment, in dem der Empfänger abgestürzt ist oder beendet
   wurde: Kommt danach nichts mehr, läuft der Empfänger auf dem Fernseher
   nicht mehr, egal was das Handy zeigt.
2. **Warum hat er aufgehört?** Suchen Sie `cast_receiver_finished`:
   `reason=INTERRUPTED` ist ein neues LOAD, das das laufende ersetzt (suchen
   Sie nach einem `StartStream` einige Sekunden vorher), `reason=ERROR`
   enthält den Fehler des Players. Kein `finished` nach dem letzten `alive`
   heißt, dass der Prozess des Empfängers verschwunden ist, meist ein
   Chromecast HD, der bei einem Kopie-Master mit hoher Bitrate (so beobachtet) wegen
   Speichermangels beendet wurde.
3. **Was hat das Handy gesehen?** Suchen Sie `cast_sender_stall`: Der Sender
   meldet `state=playing frozen=<n>s pos=<p>`, wenn das Cast-SDK noch
   PLAYING meldet, die Position sich aber seit 20 s nicht bewegt hat, und
   `state=idle reason=<r>`, wenn der Empfänger einen Grund für den Leerlauf
   gemeldet hat. Das sind WARN-Zeilen, der Warnfilter der Seite zeigt sie
   also. Vergleichen Sie ihr `pos` mit dem letzten `pos_sec` des Empfängers.
4. **Hat die Sitzung Folgefehler ausgelöst?** Suchen Sie
   `refresh rejected`: Ein Refresh-Token `not found` meldet die App ab und
   wieder an (gespeicherte Zugangsdaten), was den Stream neu anfordert; die
   Zeile enthält `jti`, `issued` und `age` des Tokens. Daran unterscheiden
   Sie eine veraltete Kopie von einer Fälschung. Suchen Sie `StartStream`,
   um die neuen Anforderungen zu sehen.
5. **War der Stream zu groß für das Gerät?** Suchen Sie `advertises a
   fallback rung` (die Spitzenbitrate des Kopie-Masters) und `plan fitted to
   the client's line` (die Messung `maxKbps` des Empfängers oder seine
   Rückfallstufe `abr=floor`; der Transkodierungs-Master, der folgt, trägt den
   Grund `client_line_exceeded` oder `abr_floor_requested`). Eine Sitzung,
   die mit einem Kopie-Master mit hoher Bitrate begann und nach wenigen Sekunden
   auf eine angepasste 1080p-Stufe wechselte, ist die Rückfallstufe, die wie
   vorgesehen greift; ein erneutes LOAD, das wieder mit dem Kopie-Master beginnt, ist das
   Muster, das einen Chromecast HD zum Absturz bringt.
6. **Ist der Server überlastet?** Suchen Sie `on-demand resource
   refused`: Eine Häufung von Ablehnungen mit `context canceled` bei
   aufeinanderfolgenden Segmenten ist Spulen (jede Anfrage wird von der
   nächsten abgebrochen), kein Fehler des Servers; `served`-Zeilen mit
   langen Dauern deuten auf einen hin.

Zeigen die Schritte 1-3 einen laufenden Empfänger und einen Sender ohne Meldungen,
liegt das Problem in der Cast-Sitzung des Handys (das SDK, das Netzwerk zum
Fernseher). Hören die Signale des Empfängers zuerst auf, prüfen Sie den
Fernseher: Speicher, Codec, die Bitrate des Streams.

## Eine Sitzung oder ein kompromittiertes Gerät stoppen { #stop-a-session-or-a-compromised-device }

**Symptom**: Ein unerwartetes Gerät unter **Verwaltung → Aktuelle
Wiedergabe**, oder ein Benutzer meldet ein unbekanntes Gerät in seiner Liste
der Sitzungen.

1. Finden Sie die Sitzung:
   ```sh
   docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
     SELECT id, profile_id, device_name, client_ip, started_at
       FROM play_sessions
      WHERE ended_at IS NULL
      ORDER BY started_at DESC LIMIT 20;"'
   ```
2. Stoppen Sie sie unter **Verwaltung → Aktuelle Wiedergabe**: das
   Stopp-Symbol an der Sitzung (**Sitzung beenden**), dann zur Bestätigung
   **Stoppen**; oder `AdminSessionService/KillSession` über gRPC mit der ID
   der Sitzung. Der Player erhält zuerst einen Stopp-Befehl, und der Stream
   wird 5 Sekunden später beendet. Das beendet nur die Wiedergabe; der
   Benutzer bleibt angemeldet.
3. Läuft der Stream trotzdem weiter, starten Sie den Container neu: Jeder
   Encoder stoppt mit ihm.
   ```sh
   docker compose restart evey
   ```
4. Ist das Konto selbst kompromittiert: **Verwaltung → Benutzer →
   Deaktivieren**. Die Refresh-Token werden gelöscht und die Zugangstoken
   sofort widerrufen, jedes Gerät ist also abgemeldet. Setzen Sie ein neues
   Passwort (`evey admin reset-password --user <email>`, siehe
   [Wiederherstellung](recovery.md#give-a-user-a-new-password)) und
   **Aktivieren** Sie das Konto dann wieder.

## Ein Bibliotheksscan hängt { #a-library-scan-is-stuck }

**Symptom**: Ein Scan läuft seit Stunden, ohne dass im Protokoll Fortschritt
bei den Dateien zu sehen ist.

1. Prüfen Sie, ob der Scan noch etwas protokolliert; ein hängender Scan
   zeigt einen Prozessor ohne Last und keine neue `scan:`-Zeile:
   ```sh
   docker logs --tail=500 evey-server | grep 'scan:'
   ```
2. Die Sperre des Scans liegt im Arbeitsspeicher, ein Neustart hebt sie also auf:
   ```sh
   docker compose restart evey
   ```
3. Löst eine Datei immer wieder Panics aus (die Stack-Traces nennen sie),
   verschieben Sie sie aus dem Pfad der Bibliothek und scannen Sie neu.
4. Meldet die Dateiüberwachung `watcher: inotify failed for path, will use
   periodic scan`, ist die Grenze des Kernels für Überwachungen erschöpft:
   ```sh
   echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf
   sudo sysctl -p
   ```

## Kurzübersicht { #cheatsheet }

```sh
# Status von allem
docker compose ps
curl -sf http://localhost:9091/healthz && echo healthy

# Alle Container verfolgen
docker compose logs --tail=100 -f

# Tatsächlich wirksame Konfiguration
docker exec evey-server env | grep ^EVEY_ | sort

# Größe der Datenbank
docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
  SELECT pg_size_pretty(pg_database_size(current_database()));"'

# Festplatte des Rechners
df -h ./evey-data
```

<!-- sources
ports: internal/config/config.go (EVEY_SERVER_PORT, EVEY_GRPC_BIND) ; cmd/evey/main.go - httpPort = Port+1 ; Dockerfile - HEALTHCHECK
logs: Dockerfile ENV EVEY_LOG_DIR ; internal/adapters/driving/http/admin_handler.go (GET /api/v1/admin/logs/search)
epoch fail-closed: internal/adapters/driving/http/auth_middleware.go, internal/adapters/driving/grpcapi/interceptors/auth.go ; internal/pkg/metrics/metrics.go - AuthFailuresTotal
rate limits: internal/config/config.go (EVEY_RATELIMIT_LOGIN_MAX, EVEY_RATELIMIT_GLOBAL_MAX, EVEY_TRUSTED_PROXIES) ; docker-compose.override.yml
cache dirs: Dockerfile ENV EVEY_TRANSCODING_CACHE_DIR / EVEY_TRICKPLAY_CACHE_DIR ; docker-compose.yml ; internal/config/config.go defaults
hardware mode and calibration labels: i18n/en/admin.yaml - transcodeCalibration.hwModeLabel, hwModeOff, hwModeAuto, cardTitle
software producer: internal/app/transcoding/software_producer.go - clientSegmentPatience, WaitBudgetDur, MaxSessionsPerHash, "software: reach decision", over_client_patience ; internal/app/transcoding/software_producer_lifecycle.go - "software: session killed" ; cmd/evey/wire.go - low-memory profile, transcode.ffmpeg_progress_log, logs/ffmpeg
server-config route: proto/evey/v1/server_config.proto - SetServerConfigEntry (PUT /api/v1/admin/server-config/{key})
stale rendition: internal/adapters/driving/http/transcode_handler_vod.go - stale_rendition, sealed_fp
db pool: internal/config/config.go (EVEY_DB_MAX_OPEN_CONNS)
search fallback: cmd/evey/wire_infra.go ; cmd/evey/admin.go - adminReindexSearch ; docker/entrypoint.sh (EVEY_EMBEDDED_MEILI, MEILI_DATA_DIR)
cache budget: internal/config/config.go (EVEY_TRANSCODING_CACHE_MAX_GB) ; internal/app/transcoding/segment_cache_evict.go - DefaultMinFreeBytes ; internal/adapters/driving/http/server.go - track-art under PhotoCacheDir ; cmd/evey/http.go - PhotoCacheDir
plugins: internal/adapters/driven/pluginrunner/runner_start.go (panic/crash lines) ; internal/app/plugin/runtime_service.go - StartPlugin ; proto/evey/v1/admin_plugin.proto - EnablePlugin
cast beacons: internal/adapters/driving/http/player_event_handler.go ; internal/app/user/auth.go - "refresh rejected" ; internal/adapters/driving/http/transcode_ondemand.go ; internal/app/transcoding/resolver_line.go
sessions: internal/adapters/driven/postgres/migrations/000001_init.up.sql - play_sessions ; internal/adapters/driving/grpcapi/admin_audit.go - KillSession (5 s) ; internal/app/user/user_service.go - DisableUser
scan: internal/app/library/scan_service.go - scanning map ; internal/app/library/watcher.go - inotify warning
labels: i18n/en/admin.yaml - sidebar (logs, nowPlaying, remoteAccess, serverConfig, transcodeCache, users, plugins), nowPlaying.stopSession, transcodeCache.clearAll, transcodeCache.maxConcurrent, remoteAccess.trustedProxies, serverSettings.transcodeCacheMaxGB ; i18n/en/common.yaml - enable, disable
-->
