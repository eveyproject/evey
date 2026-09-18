---
source_sha: baf759b03f8e14d6048f850d6927ea2371fa5533ad8148f6f86faf5364ba5f66
---
# Runbook: Störungen

Erste Reaktion auf die gängigen Ausfälle von Evey. Jeder Abschnitt ist gleich aufgebaut: **Symptom → Diagnose → Behebung**.

Container: `evey-server`, `evey-postgres`, `evey-meilisearch`, `evey-caddy` (Produktivbetrieb), `evey-web` und `evey-mailcatcher` (Override für die Entwicklung). Eine Standardinstallation hat **nur `evey-server`**: Postgres und Meilisearch laufen darin - ersetzen Sie unten `docker exec evey-postgres psql -U ...` durch `docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey ...'`, und ihre Protokolle sind die `postgres:`- und Meili-Zeilen in `docker logs evey-server`.

Standard-HTTP-Port: `9091` (`EVEY_HTTP_PORT`). Endpunkt für die Gesundheitsprüfung: `/healthz`.

**Protokolle.** Zwei Kopien desselben Stroms: die Tagesdatei `evey-data/logs/evey-YYYY-MM-DD.log` (`EVEY_LOG_DIR`, im Image `/data/logs`) und `docker logs evey-server`. Maßgeblich ist die Datei: Der json-file-Treiber rotiert `docker logs`, und nach einer Rotation kann der Befehl Stunden vor dem Container aufhören - das `stop_detail` eines Lasttests oder das Urteil einer Kalibrierung sieht dann aus, als fehle es, obwohl es nur wegrotiert wurde. Widersprechen sich beide, lesen Sie die Datei, oder als root die rohen Dateien `/var/lib/docker/containers/<id>/*-json.log*`. Die Einzeiler mit `docker logs` weiter unten setzen ein frisches, nicht rotiertes Fenster voraus.

## 1. Server nicht erreichbar

**Symptom**: Clients kommen nicht durch; `curl http://localhost:9091/healthz` läuft in ein Zeitlimit oder bekommt connection refused.

**Diagnose**:
```sh
docker ps --filter name=evey-server
docker logs --tail=200 evey-server
docker inspect --format='{{.State.Health.Status}}' evey-server
ss -ltnp | grep 9091                # lauscht der Port überhaupt?
curl -v http://localhost:9091/healthz
```

**Häufige Ursachen und Behebung**:
- Container beendet → `docker logs evey-server` zeigt den Panic, beheben, dann `docker compose up -d evey`.
- Gesundheitsprüfung scheitert, weil die Datenbank oder Meili weg ist → siehe Abschnitt 4 und 5.
- Portkonflikt auf dem Host → `EVEY_HTTP_PORT` ändern oder den Port freimachen.
- Das vorgelagerte Caddy leitet nicht weiter → `docker logs evey-caddy`, dann `EVEY_DOMAIN` prüfen und dass die Gesundheitsprüfung von `evey` grün ist (Caddy hängt davon ab).

## 2. Alle Anmeldungen enden mit 401

**Symptom**: Jeder Benutzer - auch Administratoren, die sich gestern noch angemeldet haben - bekommt von `/api/v1/auth/*` ein 401.

**Diagnose**:
```sh
docker logs --since 10m evey-server | grep -iE 'epoch|jwt|auth|signing|private key'
docker exec evey-server ls -l /data/jwt.key
```

**Suchen Sie zuerst die epoch-Zeile.** Jede authentifizierte Anfrage gleicht die
Token-Epoche des Benutzers erneut mit der Datenbank ab (ADR-0019), und dieser
Abgleich ist fail-closed. Eine Datenbank, die langsam, gesperrt oder
unerreichbar ist, lässt daher jede Anfrage im Server mit demselben 401 enden,
das auch ein abgelaufenes Token liefert - was sich genau wie ein JWT-Problem
liest und keines ist. Der Server sagt es ausdrücklich:

```
auth rejected: the token epoch could not be read, so every authenticated
request is failing - check the database, not the JWT key
```

Steht diese Zeile da, hören Sie hier auf zu lesen und gehen Sie zu §1
(Datenbank). `/healthz` kann dabei durchaus grün sein: Es klopft nur an die
Verbindung, die Abfrage der Epoche führt es nicht aus. Der Zähler in Prometheus
heißt `evey_auth_failures_total{reason="epoch_lookup_failed"}`.

**Häufige Ursachen und Behebung**:
- **JWT-Schlüssel fehlt oder hat sich geändert**: `EVEY_JWT_PRIVATE_KEY_PATH` (Standard `/data/jwt.key`) ist nicht lesbar oder wurde gewechselt. Jedes mit dem alten Schlüssel signierte Token ist damit ungültig - dagegen hilft nur, den alten Schlüssel aus einer Sicherung zurückzuspielen oder alle Benutzer zur erneuten Anmeldung zu zwingen. Prüfen Sie, dass das Volume `evey-data` korrekt unter `/data` eingehängt ist.
- **Die Ratenbegrenzung greift bei jeder Anfrage**: Sehen Sie sich `EVEY_RATELIMIT_LOGIN_MAX` (Standard `10` je IP und Minute) und `EVEY_RATELIMIT_GLOBAL_MAX` (Standard `300`) an. Ein Reverse Proxy, der allen Verkehr unter einer einzigen IP weiterreicht, reißt das schnell - setzen Sie `EVEY_TRUSTED_PROXIES` und starten Sie neu. Der Override für die Entwicklung setzt bereits `1000` und `10000`.
- Zeitversatz zwischen Server und Clients → `date -u` auf dem Host.

## 3. Die Transkodierung scheitert / Videos laufen transkodiert nicht

**Symptom**: Die Direktwiedergabe funktioniert, sobald transkodiert werden muss, scheitert es. Der Player zeigt Fehler, im Serverprotokoll stehen Exit-Codes von ffmpeg.

**Diagnose**:
```sh
docker exec evey-server which ffmpeg ffprobe
docker exec evey-server ffmpeg -version | head -1
docker logs --since 10m evey-server | grep -iE 'ffmpeg|transcode|hls'
docker exec evey-server ls -ld /cache/transcode /data/trickplay
docker exec evey-server df -h /cache
```

**Häufige Ursachen und Behebung**:
- **Die ffmpeg-Binärdatei fehlt** (ein eigenes Image ohne sie): aus dem offiziellen `Dockerfile` neu bauen.
- **Das Cache-Verzeichnis ist nicht beschreibbar**: `EVEY_TRANSCODING_CACHE_DIR` steht im Code standardmäßig auf `/data/transcode`, die compose-Datei bildet es aber über das benannte Volume `evey-cache` auf `/cache/transcode` ab. `EVEY_TRICKPLAY_CACHE_DIR` steht standardmäßig auf `/cache/trickplay`. So prüfen Sie den Schreibzugriff:
  ```sh
  docker exec evey-server touch /cache/transcode/.write-test && \
    docker exec evey-server rm /cache/transcode/.write-test
  ```
- **Der Cache ist voll** → siehe Abschnitt 6.
- **Die Hardware-Beschleunigung ist falsch eingerichtet**: Setzen Sie `EVEY_TRANSCODING_HW_ACCEL_ENABLED=false`, um Software zu erzwingen, starten Sie neu und stellen Sie fest, ob der Rückschritt tatsächlich an der Hardware liegt, bevor Sie sie wieder einschalten.
- **Zu viele Transkodierungen gleichzeitig**: `EVEY_TRANSCODING_MAX_CONCURRENT` (Standard `2`). Erhöhen Sie den Wert vorsichtig, wenn die Maschine Kerne übrig hat.

### 3c. „Es lädt nach / es ist ausgefallen“, und das Protokoll sagt nicht, warum

**Symptom**: Ein Zuschauer meldet Stocken oder Aussetzer bei einem transkodierten Titel, und im Serverprotokoll steht `hls-vod: segment timeout` mit `err:"context canceled"` - das heißt, der CLIENT hat aufgelegt, und nicht, dass der Server in ein Zeitlimit gelaufen wäre.

**Diagnose**: Der Server hat jede seiner Entscheidungen bereits festgehalten. Fangen Sie dort an, das kostet nichts:

```sh
# Warum eine Segmentanfrage einen Encoder weiterverwendet oder einen neuen gestartet
# hat, und was das den Player gekostet hat. Protokolliert wird nur, was etwas kostet:
# eine Wartezeit über ein Segment hinaus oder ein Start. `over_client_patience: true`
# ist der eindeutige Beleg.
docker exec evey-server grep '"software: reach decision"' /data/logs/evey-$(date +%F).log | tail -40

# Die Encoder selbst: Starts, Abbrüche und deren Grund.
docker exec evey-server grep -E '"software: session (started|killed)"|per-hash cap' \
  /data/logs/evey-$(date +%F).log | tail -40
```

Diese Zeilen sagen, was der Server *entschieden* hat. Wenn Sie wissen müssen,
was ffmpeg Sekunde für Sekunde tatsächlich *geschafft* hat - wo die Front im
Zeitverlauf wirklich stand -, schalten Sie die Aufzeichnung des Fortschritts je
Encoder ein:

```sh
# EIN. Gilt für den NÄCHSTEN Encoder; ein Neustart ist nicht nötig. $TOKEN ist ein Admin-JWT.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"true"}'

# Das Problem nachstellen, dann eine Sitzung lesen: out_time = bislang erzeugter Inhalt,
# speed = wie weit der Encoder der Echtzeit vorausgelaufen ist.
docker exec evey-server ls -t /data/logs/ffmpeg | head
docker exec evey-server tail -20 /data/logs/ffmpeg/<file>.log

# Danach wieder AUS.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"false"}'
```

**Lassen Sie das außerhalb einer Untersuchung aus.** Es ist eine Datei je
Encoder, ein paar MB je Sitzung, geschrieben bei jeder Wiedergabe auf der
Maschine - echter Plattenplatz und echte Schreibzugriffe auf einem schwachen
NAS. Der Dateiname trägt den Hash der Entscheidung, die Stufe (`-vN-`) und das
Startsegment (`-sN-`), passt also zum JSON-Protokoll von oben; ein Neustart an
derselben Stelle bekommt eine eigene Datei, statt den vorigen Lauf zu
überschreiben - genau das macht das ständige Neustarten von Encodern sichtbar.

**Häufige Ursachen und Behebung**:
- **Wiederholte kurze Sprünge nach vorn**: Jeder davon kann einen frischen
  Encoder starten, und die Obergrenze je Hash (`MaxSessionsPerHash`, Standard 2)
  wirft den am längsten unbenutzten hinaus. Erwarten Sie
  `session killed reason:lru_cap` im Protokoll. In Maßen ist das normal.
- **Der Client ist ungeduldiger als der Server**: An `clientSegmentPatience`
  (6 s) bemisst sich das Fenster, in dem ein Sprung sein Ziel erreichen soll;
  AndroidX Media3 gibt ein Segment nach rund 8 s auf, während das
  `WaitBudgetDur` des Servers bei 60 s liegt. Eine Zeile `reach decision` mit
  `over_client_patience: true` heißt: Der Server wollte gerade eine Wartezeit
  verlangen, die kein Player aussitzt.

### 3b. 410er wegen `stale_rendition` / der Player holt sein Master mitten in der Wiedergabe neu

**Symptom**: Der Player stockt kurz und fängt sich wieder; im Protokoll steht
`hls-vod: stale rendition URL refused (the ladder moved under a live player)`,
und Anfragen werden mit `410 Gone` und `X-Evey-Error: stale_rendition`
beantwortet.

**Was das bedeutet**: Hier greift ein *Schutz*, das ist nicht der Fehler selbst.
Jede URI in einem Manifest trägt den Hash der Entscheidung, unter der sie
entstanden ist (`fp=`). Ein 410 heißt, dass ein Player eine URL aus einer
Sitzung vorgelegt hat, deren ABR-Leiter inzwischen umgeformt wurde - sie
auszuliefern hieße, dem Decoder Bytes aus einer anderen Rendition zu geben (die
Klasse von Vorfällen rund um den „Regenbogenbildschirm“, ADR 0022 §1b). Die
Abweisung erzwingt stattdessen ein sauberes Neuholen des Masters.

**Diagnose**: Die WARN-Zeile trägt `sealed_fp` gegen `current_fp` und die
Medien-ID.
- Ein **Schwall unmittelbar nach einem Deployment, einem Neustart oder einer
  Neukalibrierung durch den Administrator** ist zu erwarten: Laufende Sitzungen
  haben eine Änderung der Leistungsfähigkeit überlebt. Nichts zu tun, die Player
  synchronisieren sich binnen einer Abfrage wieder.
- Ein **stetiges Tröpfeln im Normalbetrieb** heißt, dass irgendetwas
  fortwährend Leitern unter laufenden Streams umformt - suchen Sie nach einer
  flatternden Leistungsfähigkeit
  (`docker logs evey-server | grep -iE 'calibrat|capability'`), nach einem
  Admission-Shaper, der unter Last hin und her springt, oder nach einer
  schwankenden Übersteuerung der CMCD-Obergrenze.
- Player, die **endlos auf 410 laufen, ohne sich zu fangen**, halten ein
  zwischengespeichertes Master fest (ein alter App-Build, ein Proxy, der
  `.m3u8` beherzt cacht). Vergewissern Sie sich, dass der Proxy keine Manifeste
  zwischenspeichert (`Cache-Control: no-cache` muss durchkommen).

## 4. Datenbank: connection refused oder Fehler

**Symptom**: Im Serverprotokoll steht `dial tcp ... connect: connection refused`, `pq: ...`, oder Abfragen laufen in ein Zeitlimit.

**Diagnose**:
```sh
docker ps --filter name=evey-postgres
docker inspect --format='{{.State.Health.Status}}' evey-postgres
docker exec evey-postgres pg_isready -U "$EVEY_DB_USER"
docker logs --tail=200 evey-postgres
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
  SELECT count(*), state
    FROM pg_stat_activity
   WHERE datname = current_database()
   GROUP BY state;"
```

**Häufige Ursachen und Behebung**:
- Der Postgres-Container läuft nicht → `docker compose up -d postgres`, dann die Protokolle auf Korruption oder OOM ansehen.
- Falsche Zugangsdaten → die Umgebung des Servers muss zu `EVEY_DB_USER`, `EVEY_DB_PASSWORD` und `EVEY_DB_NAME` des Dienstes `postgres` passen. Standardwerte: `evey` / `evey` / `evey`.
- Der Pool ist erschöpft (`too many connections` oder lange Wartezeiten) → sehen Sie sich `EVEY_DB_MAX_OPEN_CONNS` an (Standard `50`). Festhängende Sitzungen beenden:
  ```sql
  SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
   WHERE state = 'idle in transaction'
     AND now() - state_change > interval '5 minutes';
  ```
- Die Platte unter `postgres-data/` ist voll → `df -h`, Platz schaffen, neu starten.

## 5. Meilisearch ist weg

**Symptom**: Die Suche liefert schlechtere Ergebnisse, im Protokoll steht `meilisearch unavailable, using SQL search fallback`.

**Verhalten**: Evey hat einen belastbaren Rückfall - die Suche läuft über SQL weiter (`cmd/evey/wire.go` verdrahtet einen belastbaren Suchadapter, der `meilisearch unavailable, using SQL search fallback` protokolliert, sobald Meili unerreichbar ist). Damit alles online bleibt, müssen Sie nichts tun; schlechter wird allein die Qualität der Rangfolge.

**Hinweis zur Betriebsart**: Die voreingestellte `docker-compose.yml` betreibt Meilisearch **eingebettet im Container `evey-server`** (`EVEY_EMBEDDED_MEILI: "true"`). Einen eigenständigen Container `evey-meilisearch` gibt es nur im Override für die Entwicklung (`docker-compose.override.yml`). Eingebettet heißt „Meilisearch ist weg“ nichts anderes, als dass der Prozess evey-server krank ist - einen eigenen Container zum Neustarten gibt es nicht.

**Diagnose und Behebung (eingebettet, der Standardfall)**:
```sh
docker ps --filter name=evey-server
curl -sf http://localhost:7700/health          # aus evey-server heraus weitergereichter Port
docker logs --tail=200 evey-server | grep -i meili
```
- Server krank → `docker compose restart evey-server`.
- Index beschädigt → evey-server stoppen, `./evey-data/meili-data/` sichern, löschen, neu starten und im Administrationsbereich eine vollständige Neuindizierung anstoßen.

**Diagnose und Behebung (eigenständig, nur im Override für die Entwicklung)**:
```sh
docker ps --filter name=evey-meilisearch
curl -sf http://localhost:7700/health
docker logs --tail=200 evey-meilisearch
```
- Container läuft nicht → `docker compose up -d meilisearch`.
- `MEILI_MASTER_KEY` passt nicht zusammen → das Serverprotokoll zeigt fehlgeschlagene Anmeldungen. Gleichen Sie die Umgebung beider Dienste an und starten Sie neu.
- Index beschädigt → Meili stoppen, `./meili-data/` sichern, löschen, neu starten und im Administrationsbereich eine vollständige Neuindizierung anstoßen.

## 6. Die Platte des Transkodierungs-Cache ist voll

**Symptom**: Transkodierungen scheitern mit `no space left on device`; die Gesundheitsprüfung des Containers flattert.

**Diagnose**:
```sh
docker exec evey-server df -h /cache
docker exec evey-server du -sh /cache/transcode /cache/trickplay /data/trickplay 2>/dev/null
docker volume inspect evey_evey-cache   # der von compose verbogene Name kann abweichen
```

**Behebung**:
- Gefahrlos leeren - der Cache lässt sich neu erzeugen:
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/transcode/*'
  ```
  Damit fallen auch `photos/` (verkleinerte Fotovarianten) und `track-art/`
  (das im Tag eines Titels eingebettete Cover, einmal herausgelöst und
  verkleinert) darunter weg - beide entstehen bei der nächsten Anfrage neu. Die
  Schaltfläche „Cache leeren“ im Administrationsbereich tut genau dasselbe.
- Trickplay ist teurer neu zu erzeugen; leeren Sie es nur, wenn es sein muss:
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/trickplay/* /data/trickplay/*'
  ```
- Senken Sie `EVEY_TRANSCODING_CACHE_MAX_GB` (Standard `50`), wenn das Volume des Hosts kleiner ist als dieser Wert.

## 7. Eine Erweiterung ist abgestürzt

**Symptom**: Eine Erweiterung antwortet nicht mehr; im Serverprotokoll stehen Panics aus dem Runner für Erweiterungen.

**Diagnose**:
```sh
docker logs --since 30m evey-server 2>&1 | grep -iE 'plugin|pluginrunner|panic'
# Achten Sie auf: "panic in plugin runner", "plugin process crashed", "remove plugin sock on crash"
```

**Behebung**:
- Versuchen Sie zuerst den gezielten Neustart, ganz ohne Ausfallzeit des Containers: `POST /api/v1/admin/plugins/<id>/enable`. Das funktioniert auch dann, wenn die Erweiterung bereits `enabled` ist, aber im Status `Error` feststeckt - die Sperre in `StartPlugin` startet sie neu, sobald der Status nicht `Running` ist. Nehmen Sie das dem Neustart des ganzen Servers vor, besonders wenn Sie nicht ohne Weiteres per SSH auf den Host kommen.
- Räumt der gezielte Neustart es nicht aus dem Weg, schalten Sie die betreffende Erweiterung im Administrationsbereich ab.
- Ist der Server selbst instabil, starten Sie ihn neu: `docker compose restart evey`.
- Sehen Sie sich die Dateien der Erweiterung unter `./evey-data/plugins/` (Host) == `/data/plugins` (Container) an. Entfernen Sie das kaputte ZIP beziehungsweise Verzeichnis und starten Sie neu.
- Schicken Sie den Panic-Trace samt der ID der Erweiterung an deren Autor.

## 8. Ein Cast ist eingefroren, verstummt, oder das Handy dreht sich endlos

Alles Folgende lesen Sie auf der Protokollseite im Administrationsbereich (oder
über `GET /api/v1/admin/logs/search?q=<term>&limit=200`): Eine Shell auf der
Maschine brauchen Sie dafür nicht. Zeiten sind UTC. Jedes Signal ist eine Zeile
`player-client: playback event reported` mit `type=` und einer Zeichenkette
`details=`.

1. **Lebte der Empfänger, und bis wann?** Suchen Sie nach
   `cast_receiver_alive`. Der Empfänger schickt eines alle 30 s, solange er
   abspielt; `details` trägt `buf=<behind>/<ahead>/<ranges>` (Sekunden gepuffert
   vor und hinter dem Abspielkopf) sowie `pos_sec`. Bei gesunder Wiedergabe
   liegt `ahead` bei einigen zehn Sekunden. **Der
   Zeitpunkt des letzten Signals ist der Moment, in dem der Empfänger gestorben
   oder abgeschossen worden ist**: Kommt danach nichts mehr, läuft auf dem
   Fernseher kein Empfänger mehr, was das Handy auch anzeigen mag.
2. **Warum hat er aufgehört?** Suchen Sie nach `cast_receiver_finished`:
   `reason=INTERRUPTED` heißt, ein neues LOAD hat das laufende ersetzt (halten
   Sie nach einem `StartStream` ein paar Sekunden davor Ausschau),
   `reason=ERROR` trägt den Fehler des Players. Gar kein `finished` nach dem
   letzten `alive` heißt, dass der Prozess des Empfängers verschwunden ist (am
   häufigsten ein Chromecast HD, den der Speicher an einem copy-Master mit über
   20 Mbit/s umgebracht hat).
3. **Was hat das Handy gesehen?** Suchen Sie nach `cast_sender_stall`: Der
   Sender schickt `state=playing frozen=<n>s pos=<p>`, wenn das Cast-SDK noch
   PLAYING meldet, die Position sich aber 20 s lang nicht bewegt hat, und
   `state=idle reason=<r>`, wenn der Empfänger einen Grund für den Leerlauf
   gemeldet hat. Diese Zeilen werden als WARN protokolliert, der Warnfilter der
   Protokollseite fördert sie also zutage. Das ist die Senderseite derselben
   Minute - vergleichen Sie ihr `pos` mit dem letzten `pos_sec` des Empfängers.
4. **Hat die Sitzung eine Kettenreaktion ausgelöst?** Suchen Sie nach
   `refresh rejected`: Ein Refresh-Token, das „not found“ meldet, meldet die App
   ab und mit den gespeicherten Zugangsdaten wieder an, wodurch der Stream neu
   ausgegeben wird - die Zeile nennt inzwischen `jti`, `issued` und `age` des
   Tokens, woran sich eine veraltete Kopie von einer gefälschten unterscheiden
   lässt. Suchen Sie nach `StartStream`, um die Neuausgaben zu sehen; der Sender
   lädt den Empfänger bei einer neu ausgegebenen URL für denselben Titel nicht
   mehr neu.
5. **War der Stream zu groß für das Gerät?** Suchen Sie nach
   `advertises a fallback rung` (die Spitzenbitrate des copy-Masters) und nach
   `plan fitted to the client's line` (die `maxKbps`-Messung des Receivers
   oder seine `abr=floor`-Rettung, jetzt Eingaben des Plans - der folgende
   Transcode-Master trägt den Grund `client_line_exceeded` oder
   `abr_floor_requested`): Eine Sitzung, die auf einem copy-Master mit über
   20 Mbit/s begann und nach ein paar Sekunden auf eine angepasste 1080p-Stufe
   fiel, ist die Rettung nach unten bei der Arbeit; ein erneutes LOAD, das
   wieder auf dem copy-Master beginnt, ist das Muster, das einen Chromecast HD
   umbringt.
6. **Hungert es auf der Serverseite?** Suchen Sie nach
   `on-demand resource refused`: Ein Schwall von Abweisungen mit
   `context canceled` auf aufeinanderfolgenden Segmenten ist ein Spulen (jede
   Anfrage von der nächsten abgebrochen), kein Fehler des Servers;
   `served`-Zeilen mit langen Dauern sind einer.

Zeigen die Schritte 1 bis 3 einen lebenden Empfänger und einen stummen Sender,
liegt das Problem in der Cast-Sitzung des Handys (dem SDK, dem Netz zum
Fernseher); brechen zuerst die Signale des Empfängers ab, ist der Fernseher der
Ort, an dem Sie nachsehen (Speicher, Codec, die Bitrate des Streams).

## Eine kompromittierte Sitzung oder ein Gerät abschießen

Symptom: Der Administrator sieht in **Aktuelle Wiedergabe** (Administrationsbereich) ein unerwartetes Gerät, oder ein Benutzer meldet ein unbekanntes Gerät in seiner Sitzungsliste.

1. Die `session_id` herausfinden:
   ```sh
   docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
     SELECT id, user_id, profile_id, device, ip_address, started_at
     FROM play_sessions
     WHERE ended_at IS NULL
     ORDER BY started_at DESC LIMIT 20;"
   ```
2. Über die Oberfläche beenden: **Admin → Aktivität → Aktuelle Wiedergabe → ⋯ → Sitzung beenden**, oder per gRPC über `AdminSessionService/KillSession` mit der `session_id`. Die Sitzung endet 5 s später, damit der Player den Stoppbefehl noch entgegennehmen kann.
3. Reagiert die Sitzung nicht, löschen Sie die HLS-Sitzung mit Gewalt (das Verzeichnis ist `<cacheDir>/<mediaItemID>`, standardmäßig `/cache/transcode/<mediaItemID>`):
   ```sh
   docker exec evey-server rm -rf /cache/transcode/<media_item_id>/
   ```
4. Soll auch das Refresh-Token entzogen werden (Verdacht auf ein kompromittiertes Passwort):
   ```sh
   docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
     DELETE FROM refresh_tokens WHERE user_id = '<user_id>';"
   ```
   Sämtliche Geräte dieses Benutzers müssen sich danach neu anmelden. Für die vollständige Abriegelung setzen Sie das Passwort über `evey admin reset-password --user <username>` zurück.

## Einen feststeckenden Bibliotheksscan lösen

Symptom: Ein Bibliotheksscan steht stundenlang auf „läuft“, ohne dass im Protokoll Fortschritt bei den Dateien auftaucht.

1. Sehen Sie indirekt nach der Map `scan_service.scanning` - hängt der Prozess, ist die CPU im Leerlauf und in den jüngsten Protokollen taucht kein `scan_service` auf:
   ```sh
   docker compose logs --tail=500 evey-server | grep scan_service
   ```
2. Die Sperre eines Scans liegt **im Arbeitsspeicher** (es gibt keine Zeile in der Datenbank), ein Neustart des Servers räumt sie also weg:
   ```sh
   docker compose restart evey-server
   ```
3. Löst eine bestimmte Datei wiederholt Panics aus (siehe die Stack-Traces), stellen Sie sie unter Quarantäne, indem Sie sie aus dem Pfad der Bibliothek herausnehmen, und scannen Sie erneut.
4. Sind die inotify-Watches erschöpft (Grenze des Kernels), erhöhen Sie sie:
   ```sh
   echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf
   sudo sysctl -p
   ```

## Spickzettel für die allgemeine Diagnose

```sh
# Zustand von allem
docker compose ps
curl -sf http://localhost:9091/healthz && echo healthy

# Alle Container mitlesen
docker compose logs --tail=100 -f

# Die Konfiguration des Servers ausgeben
docker exec evey-server env | grep ^EVEY_ | sort

# Größe und Sperren in Postgres
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
  SELECT pg_size_pretty(pg_database_size(current_database()));"

# Platte des Hosts
df -h . ./postgres-data ./meili-data ./evey-data
```
