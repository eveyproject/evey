---
source_sha: a7cdb2195f99559246f23706d11f64ad40c0cb3dcd88a52f3caf61af02c7ce8c
---
# Konfiguration

Zwei Orte, eine Regel. **Was vor dem Serverstart existieren muss** - Geheimnisse,
Verdrahtung des Containers, Angaben zum Reverse Proxy - steht in der
Compose-Datei. **Was ein Betreiber danach einstellt** - Log-Level, Sprache,
Grenzen der Transkodierung, Hardware-Beschleunigung, Dateiüberwachung, Sandbox
und Kontingente der Erweiterungen - steht unter **Admin > Serverkonfiguration**,
wird in der Datenbank gespeichert, übersteht ein Neuerzeugen des Containers und
wirkt meist ohne Neustart.

Setzt man eine dieser Einstellungen in der Umgebung, belegt das nur den ersten
Start vor. Deshalb listet diese Seite nicht jede `EVEY_*`-Variable auf, die das
Binary annimmt: Die übrigen sind Vorbelegungen für den ersten Start und
Notausgänge. `internal/config/config.go` ist die vollständige Referenz, wenn Sie
eine brauchen.

## 1. Was Sie selbst setzen müssen

Nichts, bei der Standardinstallation: Medienpfad und Image-Tag stehen in der
Compose-Datei selbst, und es gibt keine `.env`. Die `docker-compose.yml` des
Repositorys liest `MEDIA_DIR` dagegen aus `.env`:

```env
MEDIA_DIR=/path/to/your/media
```

So oder so laufen Postgres und Meilisearch eingebettet im Container und erzeugen
beim ersten Start ihre eigenen Geheimnisse, abgelegt unter `./evey-data/keys`
(`postgres.password`, `meili.key`, 0600). Der Server liest sie über
`EVEY_DB_PASSWORD_FILE` / `MEILI_MASTER_KEY_FILE` (im Image fest eingestellt),
weshalb `docker exec evey-server evey admin ...` auch kein Passwort auf der
Kommandozeile braucht. Setzen Sie `EVEY_DB_PASSWORD` / `MEILI_MASTER_KEY` nur
dann ausdrücklich, wenn Sie Ihre eigene Datenbank oder Suchmaschine mitbringen -
ein ausdrücklicher Wert gewinnt immer gegen die Datei.

Metadaten funktionieren sofort: Ein TMDB-Schlüssel ist eingebaut. Ein eigener
Schlüssel (kostenlos auf themoviedb.org) gibt Ihnen Ihr eigenes Kontingent;
setzen Sie ihn unter **Admin > Serverkonfiguration > API-Schlüssel** oder
belegen Sie ihn mit `EVEY_TMDB_API_KEY` vor. Ist der Server veröffentlicht, ist
`EVEY_SERVER_BASE_URL` die eine Variable, die hinzukommt - Abschnitt 2.

## 2. Eine veröffentlichte Instanz, mit zwei Variablen

Hinter einem Reverse Proxy und mit Hardware-Transkodierung besteht die ganze
Umgebung aus zwei Variablen. Alles andere, was man gern in eine Compose-Datei
schreibt, steckt entweder schon im Image oder ist ein Standardwert:

```yaml
services:
  evey:
    image: gravityz/evey:latest
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 10.0.0.5/32      # your reverse proxy, /32 if you know its address
    volumes:
      - ./evey-data:/data
      - evey-cache:/cache
      - /srv/media:/media
    devices:
      - /dev/dri:/dev/dri                     # hardware transcoding
    group_add:
      - "991"                                 # the group owning /dev/dri/renderD128 on YOUR host
```

Zeilen, die Sie aus älteren Setups **nicht** übernehmen sollten:

| Zeile | Warum sie unnötig ist |
| --- | --- |
| `EVEY_LOG_DIR`, `EVEY_TRANSCODING_CACHE_DIR`, `EVEY_TRICKPLAY_CACHE_DIR` | Das Image setzt sie schon auf `/data/logs`, `/cache/transcode`, `/data/trickplay`. Sie zu wiederholen ändert nichts. |
| `EVEY_FORWARDED_PROTO` | Veraltet - wird aus dem Schema von `EVEY_SERVER_BASE_URL` abgeleitet. Sie zu setzen loggt nur eine Veraltet-Warnung. |
| `EVEY_KEYFRAME_BACKFILL=on` | Alles außer `off` schaltet sie ein, auch das Fehlen. |
| `LIBVA_DRIVER_NAME` | libva wählt den Treiber anhand der PCI-ID der GPU. Erzwingen Sie ihn nur, wenn Ihre Karte einen Treiber braucht, den libva nicht wählt (prüfen mit `docker exec evey-server vainfo`). |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` wählt den stärksten Einschluss, den Ihr Host unterstützt. Setzen Sie ihn nur ausdrücklich, wenn Sie eine Garantie des *geschlossenen Scheiterns* wollen - `landlock` weigert sich, eine Erweiterung zu starten, die es nicht einschließen kann, wo `auto` mit einer Warnung zurückfällt. |
| ein `group_add` für `/dev/dri/card0` | Evey öffnet immer nur `renderD128`. Prüfen Sie mit `ls -ln /dev/dri` und fügen Sie nur diese Gruppe hinzu. |

## 3. Die zwei Einstellungen, die der Administrationsbereich nicht ersetzt

Beide haben ein Gegenstück unter **Admin → Fernzugriff**, und keines davon macht
die Umgebungsvariable überflüssig. Das bringt Leute durcheinander, deshalb lohnt
es sich, es ausdrücklich zu sagen.

Der Unterschied liegt darin, wo jede lebt. Der Block zum Fernzugriff
(ein/aus, externe URL, LAN-Bereiche, Bitrate-Obergrenze für entfernte Zugriffe)
wird **in der Datenbank gespeichert** - seine Umgebungsvariablen belegen den
ersten Start vor und werden danach ignoriert. `EVEY_SERVER_BASE_URL` und
`EVEY_TRUSTED_PROXIES` sind das Gegenteil: Sie werden bei jedem Start aus der
Umgebung gelesen, und **nichts im Bereich überschreibt sie je**. Ein Wert, der im
Bereich wie ein Duplikat aussieht, ist in diesen zwei Fällen also eine andere
Einstellung, die zufällig eine Adresse erwähnt.

**`EVEY_SERVER_BASE_URL` wird nicht durch „Externe URL“ ersetzt.** Der Wert des
Bereichs speist die URLs, die der Server *erzeugt*. Ob der Server aber
`X-Forwarded-Proto` vertraut - und damit, ob er glaubt, über HTTPS ausgeliefert
zu werden -, wird **beim Start allein aus der Umgebungsvariable** entschieden.
Fehlt die Variable, fällt er auf `http://localhost:9091` zurück:
`Secure`-Cookies, Weiterleitungen und jeder absolute Link fallen auf `http://`
zurück, und Freigabe-/Download-URLs zeigen auf localhost. Der Wert des Bereichs
wird zu spät gelesen, um zu helfen.

**`EVEY_TRUSTED_PROXIES` wird nicht durch „LAN-Netzwerke“ ersetzt.** Das sind
zwei Stufen, nicht zwei Namen für dieselbe Sache:

1. `EVEY_TRUSTED_PROXIES` entscheidet, **wessen `X-Forwarded-For` geglaubt
   wird**, und damit, welche Adresse ein Client bekommt.
2. Die LAN-Netzwerke ordnen dann **diese** Adresse als lokal oder entfernt ein.

Die zweite arbeitet auf dem Ergebnis der ersten. Ohne die erste kommt jeder
Client mit der Adresse des Proxys an, und alle werden gleich eingeordnet - genau
so blieb die Sperre für den Fernzugriff vier Monate lang wirkungslos (siehe
`internal/config/remote_access_resolve.go`). Loopback ist immer
vertrauenswürdig und lässt sich nicht entfernen; Ihr Eintrag kommt *hinzu*.

## 4. Dateibesitz (PUID/PGID)

**Standard: automatisch.** Sind `PUID`/`PGID` nicht gesetzt, sieht sich der
Einstiegspunkt beim Start den Besitzer Ihrer `/media*`-Mounts an und startet den
Server als dieser Benutzer, sodass die Funktionen, die *in* Ihre Bibliothek
schreiben (die Reparatur des Sprungindex baut Dateien an Ort und Stelle neu),
sofort funktionieren - kein `chown`, keine Konfiguration. Das Startlog sagt, was
gewählt wurde:

```
evey: media mounts owned by 1026:100 (detected on /media) - running the
server as that user so write features (seek-index repair) work out of the box
```

Medien-Mounts, die **verschiedenen Benutzern** gehören, werden nach bestem
Bemühen behandelt: Der Server übernimmt den Benutzer, dem die meisten Mounts
gehören, und tritt den Gruppen der anderen Mounts als Zusatzgruppen bei, sodass
Reparaturen auf diesen Mounts funktionieren, wenn ihre Dateien für die Gruppe
beschreibbar sind (die Startwarnung listet, welche Mounts Schreibzugriffe
vielleicht noch verweigern). Die automatische Erkennung tritt nur ganz zurück
(fällt mit einer Warnung auf `1000:1000` zurück), wenn jeder Medien-Mount root
oder der reservierten UID `1001` gehört - übertragen Sie Ihre Medien per chown
an einen normalen Benutzer, oder setzen Sie `PUID`/`PGID` ausdrücklich:

```env
PUID=1026   # run the server as this user id  (host: `id -u <media-owner>`)
PGID=100    # ...and this group id
```

Ausdrückliche Werte gewinnen immer gegen die Erkennung.

So funktioniert es: Der Container startet nur so lange als root, wie er braucht,
um seinen internen Benutzer neu zuzuordnen (und Eveys eigene
Zustandsverzeichnisse `/data`, `/cache` neu zu besitzen - **nie** Ihre
Medien-Mounts), und gibt dann seine Privilegien ab. Zusatzgruppen, die über das
Compose-`group_add:` gewährt werden (z. B. die Render-Gruppe Ihres Hosts für die
Hardware-Transkodierung über `/dev/dri`), bleiben über die Abgabe hinweg
erhalten. Der Serverprozess selbst läuft nie als root.

Alternative ganz ohne root: Ist Ihnen selbst dieses kurzzeitige root zu viel,
setzen Sie stattdessen `user: "UID:GID"` für den Dienst in Ihrer Compose-Datei.
Der Einstiegspunkt erkennt den Start ohne root und überspringt die Neuzuordnung
(einschließlich der Erkennung des Medienbesitzers) - dann sind Sie selbst dafür
verantwortlich, dass `./evey-data` für diese UID beschreibbar bleibt.

Hinweise:
- `PUID=1001` wird abgelehnt: Das ist der interne Benutzer `evey-plugin`, der
  getrennt gehalten wird, damit Erweiterungen nie die Identität des Servers
  teilen können.
- Medien werden standardmäßig mit Schreibzugriff eingehängt, damit Reparaturen
  direkt funktionieren; hängen Sie `:ro` an das Medienvolume an, wenn Sie eine
  nur lesbare Bibliothek bevorzugen (Reparaturen scheitern dann mit einem klaren
  Fehler).

## 5. Referenz der Variablen

### Server
```env
EVEY_SERVER_PORT=9090                  # gRPC listen port (default 9090); HTTP API binds at SERVER_PORT+1 = 9091
EVEY_HTTP_PORT=9091                    # compose-only: host port exposed by Docker for the HTTP gateway (not read by the server binary)
EVEY_SERVER_NAME=Evey                  # display name
EVEY_DEMO=false                        # public demo instance only (demo.eveyproject.org): locks admin writes, no mail/push/webhooks
EVEY_VERSION=latest                    # Docker image tag (latest, v0.1.0-beta.1, etc.)
```

### Datenbank
```env
# Embedded (default when EVEY_DB_HOST is unset): Postgres 17 inside the container.
EVEY_EMBEDDED_POSTGRES=            # true | false; unset = embedded iff EVEY_DB_HOST is unset
EVEY_POSTGRES_DATA_DIR=/data/postgres   # cluster location inside the container (under ./evey-data)
EVEY_DB_USER=evey
EVEY_DB_NAME=evey
EVEY_DB_PASSWORD=                  # unset = generated on first boot into /data/keys/postgres.password
EVEY_DB_PASSWORD_FILE=/data/keys/postgres.password   # read when EVEY_DB_PASSWORD is unset (image default)

# External: use these INSTEAD of the block above, not alongside it.
# Setting the host turns the embedded database off.
EVEY_DB_HOST=postgres              # the repository compose's sibling service is `postgres`
EVEY_DB_PORT=5432
EVEY_DB_PASSWORD=changeme
EVEY_DB_SSLMODE=prefer             # or disable | require | verify-full
```

Das eingebettete Postgres lauscht nur auf dem Loopback des Containers und
verlangt über TCP ein Passwort; das erzeugte ist das, was der Server (und
`evey admin`) benutzen. Sein Datenverzeichnis muss auf einem echten Dateisystem
liegen (ext4, btrfs, xfs, ...), nicht auf einer Netzwerkfreigabe. Migrationen
laufen in beiden Layouts beim Start automatisch.

### Authentifizierung
```env
EVEY_JWT_ACCESS_TTL=15m
EVEY_JWT_REFRESH_TTL=168h              # 7 days
EVEY_RATELIMIT_LOGIN_MAX=10            # per IP per minute
```

### Bibliotheken
```env
EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL=2m # fallback polling interval when inotify misses events (default 2m); admin-editable via Admin → Server Config
EVEY_LIBRARY_WATCHER_ENABLED=true      # realtime file watching
```

Die Scan-Probe der Bibliothek liest die Metadaten von `.mkv`/`.webm` und
`.mp4`/`.m4v` prozessintern (mkvgo, nur Dateikopf - ~35-mal schneller beim
erneuten Scan/Reparieren/auf dem NAS) ohne jede Konfiguration: Sie ist immer an.
ffprobe bleibt maßgeblich und behandelt jede Datei, die der Schnellpfad nicht
feldgleich abbilden kann (HDR/Dolby Vision, nicht abgebildete Codecs, …). Siehe
[adr/0016-mkv-fastpath-prober.md](adr/0016-mkv-fastpath-prober.md).

### Transkodierung
```env
EVEY_TRANSCODING_MAX_CONCURRENT=2
EVEY_TRANSCODING_HW_ACCEL_ENABLED=false
```

Die Einrichtung der Hardware steht in [transcoding.md](transcoding.md).

**Dolby-Ton auf einem Chromecast** (`EVEY_CAST_WASM_AUDIO_ENABLED`, Standard
`false`). Evey kann einem Cast-Empfänger die originale AC-3/E-AC-3-Spur geben
und sie auf dem Fernseher in WebAssembly dekodieren lassen, was eine
Transkodierung vermeidet. Das ist **für Cast-Empfänger aus**, weil ein
schwaches Gerät eines Betreibers zweimal mitten im Film abgeschossen wurde, als
dieser Weg im Spiel war - und sein Decoder dort mit dem 3,13-Fachen der
Echtzeit gemessen wurde, also ohnehin nichts gewann. Ist es aus, bekommt ein
Cast den gewöhnlichen gemuxten Master mit einer AAC-Spur: Ton ab der ersten
Sekunde, auf jedem Gerät. Browser sind NICHT betroffen - sie behalten den
wasm-Decoder, der dort geprüft ist. Setzen Sie es auf `true`, um es für Casts
wieder einzuschalten.

**Bevorzugen Sie den Administrationsbereich.** Gleichzeitigkeit,
Hardware-Beschleunigung, AV1, das Budget des Caches und die erweiterten Grenzen
(maximale gleichzeitige HLS-Sitzungen, Höhenobergrenzen für Software und
Software-HDR, eine Stufe nach der anderen kodieren, ein Hardware-Encoder je
Stufe) liegen alle unter **Admin > Transkodierung**. Dort gespeicherte Werte
überstehen Neustarts und haben Vorrang vor der Umgebung; der Bereich sagt, welche
erst nach einem Neustart wirken.

**Schalter der Hintergrund-Worker** werden als Zeilen unter **Admin >
Serverkonfiguration** gesetzt (der Bereich schlägt die Schlüssel vor und zeigt
für jeden die passende Umgebungsvariable und den erwarteten Wert). Alle werden
beim Start gelesen und wirken daher nach einem Neustart:

| Schlüssel | Wert |
| --- | --- |
| `transcoding.keyframe_backfill` | `off`, um das Nachfüllen des Keyframe-Index abzuschalten (standardmäßig an) |
| `transcoding.keyframe_backfill_batch` | Einträge pro Durchlauf, > 0 |
| `transcoding.keyframe_backfill_pause_ms` | Millisekunden zwischen Einträgen, >= 0 |
| `transcoding.keyframe_backfill_idle_sec` | Sekunden zwischen Durchläufen im Leerlauf, > 0 |
| `transcoding.level_backfill` | `off`, um das Nachfüllen der H.264/HEVC-Level abzuschalten (standardmäßig an) |
| `transcoding.level_backfill_batch` | Einträge pro Durchlauf, > 0 |
| `transcoding.level_backfill_pause_ms` | Millisekunden zwischen Einträgen, >= 0 |
| `transcoding.level_backfill_idle_sec` | Sekunden zwischen Durchläufen im Leerlauf, > 0 |
| `transcoding.pause_bg_on_playback` | `off`, um die Hintergrundarbeit während der Wiedergabe weiterlaufen zu lassen (standardmäßig an: sie tritt zurück) |
| `transcoding.snapshot_fastpath` | `off`, um statt des mkvgo-Schnellpfads für Keyframes den exakten ffmpeg-Extraktor zu erzwingen |

Jeder hat auch eine Entsprechung in der Umgebung (`EVEY_KEYFRAME_BACKFILL`,
`EVEY_LEVEL_BACKFILL_BATCH`, ...), aber die Zeile im Bereich gewinnt.

**CPU-Budget für den Hintergrund** (Admin > Serverkonfiguration > Zugeteilte
Rechenleistung) begrenzt den Prozessor, den alle Hintergrundaufgaben zusammen
nehmen dürfen, live (ohne Neustart):

| Schlüssel | Wert |
| --- | --- |
| `background.cpu_percent` | Anteil der CPU-Kapazität, aus dem jede Hintergrundaufgabe schöpft: `25` (Standard, schonend) oder `50` (Schub) |
| `background.cpu_percent.<task>` | der eigene Anteil einer schweren Aufgabe, `1`..`80`, ersetzt für diese Aufgabe den globalen: `library_scan`, `trickplay_generation`, `intro_detection`, `complexity_classifier` |

Anteile addieren sich nie: Die Summe von allem, was im Hintergrund läuft, bleibt
unter dem höchsten gesetzten Anteil. Ein Anteil wird in ganze Kerne umgerechnet
(25 % von 8 Kernen = 2), mindestens einer. Jedes ffmpeg im Hintergrund reserviert
seine Threads in diesem Budget, bevor es startet; der Bibliotheksscan und die
Intro-Erkennung bemessen ihre Parallelität daran, die Erzeugung der
Trickplay-Bilder ihre Decoder-Threads. Die Wiedergabe pausiert weiterhin jede
Hintergrundanalyse (`transcoding.pause_bg_on_playback`).
`EVEY_LIBRARY_SCAN_WORKERS` und `EVEY_INTRO_DETECTION_MAX_PARALLEL` bemessen
diese beiden Pools von Hand und umgehen für sie das Budget. Die früheren
Schlüssel `trickplay.adaptive_cpu` (`false` = 50 %) und
`intro_detection.use_all_cores` (`true` = 80 % für die Intro-Erkennung) werden
weiter beachtet, bis die neuen gesetzt sind.

**Budget des Transkodierungs-Caches** (Admin > Servereinstellungen,
`transcoding.cache_max_gb`, Standard `10`, beim Neustart angewandt) begrenzt den
ganzen Transkodierungs-Cache auf der Platte: Wiedergabesegmente,
Schnellstart-Remuxe und kompatible Downloads zusammen, unter einer einzigen
Zahl. Es ist absichtlich eine absolute Größe: Der Cache liegt auf dem Volume
`evey-cache` neben allem anderen, was der Betreiber dort laufen lässt, und wofür
der Rest des Volumes da ist, entscheidet er. Ein einziger Aufräumer setzt es
durch, am längsten Unbenutztes zuerst, und verschont nur, was ein Zuschauer
gerade liest. Unabhängig vom Budget hält er außerdem 2 GiB auf dem Cache-Volume
frei und entfernt einen Download, den 7 Tage lang niemand abgeholt hat. Eine
copy-Sitzung (Remux) speichert nie den ganzen Film: Sie hält ein Fenster von
Segmenten um den Zuschauer und springt für alles außerhalb neu in die Quelle. Der
speicherlose mkvgo-Weg, der die meisten MKV- und MP4-Quellen bedient, berührt
den Cache überhaupt nicht.

### Erweiterungen (Sandbox & Grenzen)
```env
EVEY_PLUGINS_SANDBOX_MODE=auto         # auto|landlock|bubblewrap|strict|basic - auto picks the strongest your host supports
EVEY_PLUGINS_UID_SEPARATION=auto       # auto|on|off - run plugins as a separate user to wall off secrets (auto = on only when the effective sandbox resolves to "basic", i.e. neither Landlock nor bubblewrap); incompatible with no-new-privileges
EVEY_PLUGINS_MEMORY_MB=256             # max RAM per plugin (MiB); admin-editable via Admin → Plugins
EVEY_PLUGINS_PIDS_MAX=64               # max processes/threads per plugin; admin-editable
EVEY_PLUGINS_DISK_MAX_MB=100           # max install-dir size per plugin (MiB; 0 = off); admin-editable
EVEY_PLUGINS_CPU_NICE=10               # scheduling nice 0-19 (higher = lower priority); admin-editable
EVEY_PLUGINS_ENABLED=true              # master switch for the plugin subsystem
EVEY_PLUGINS_COSIGN_POLICY=warn        # off|warn|enforce - require a cosign signature on plugin ZIPs
EVEY_PLUGINS_COSIGN_PUBLIC_KEY=        # PEM public-key path (required when policy != off)
```

Anleitung je Szenario: [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md). Vollständiges Modell: [SECURITY.md](SECURITY.md).

### E-Mail (SMTP)
```env
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=
EVEY_SMTP_PASSWORD=
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls                 # starttls, ssl, none
```

Siehe [notifications.md](notifications.md).

### SSO (OIDC)
```env
EVEY_OIDC_ENABLED=false
EVEY_OIDC_ISSUER=https://auth.example.com
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=       # comma-separated, empty = all
```

Siehe [oidc.md](oidc.md).

### Sicherungen
```env
EVEY_BACKUP_INTERVAL_HOURS=0           # 0 = disabled, 24 = daily
EVEY_BACKUP_RETENTION_DAYS=30
```

Siehe [backups.md](backups.md).

### Reverse Proxy
```env
EVEY_TRUSTED_PROXIES=172.16.0.0/12     # added to the always-trusted loopback, not a replacement
EVEY_REAL_IP_HEADER=X-Forwarded-For
EVEY_DOMAIN=media.example.com          # only for bundled Caddy
```

Das HTTPS-Verhalten (Secure-Cookies, HSTS, Vertrauen in `X-Forwarded-Proto`)
folgt dem Schema von `EVEY_SERVER_BASE_URL`: Eine `https://`-Basis-URL schaltet es
ein. `EVEY_FORWARDED_PROTO=true|false` ist veraltet und bleibt nur als
Überschreibung erhalten.

Siehe [reverse-proxy.md](reverse-proxy.md).

### Intro-Erkennung
```env
EVEY_INTRO_TIDB_API_KEY=                # optional Bearer token for TheIntroDB (500 lookups/day vs 100 anonymous)
EVEY_INTRO_FFMPEG_THREADS=1             # threads per ffmpeg process used by intro detection (1-16)
EVEY_INTRO_SKIP_PREVIEW_ENABLED=false   # serve "preview" (next-episode reel) segments to the player so a "Skip Preview" button appears
EVEY_INTRO_AUTO_SKIP_ENABLED=false      # every player auto-skips detected intros (web/mobile read the flag in the segments response; cast receivers get intro segments gated by it)
```

`api.tidb_key`, `intro_providers.enabled`, `intro_providers.tidb_submit_enabled`,
`intro_providers.skip_preview_enabled` und `intro_providers.auto_skip_enabled`
lassen sich auch über `/admin/server-config` einstellen (erst die Datenbank, dann
die Umgebung als Rückfall). Starten Sie den Server nach dem Umschalten von
`intro_providers.enabled` neu; die Schalter für das Überspringen der Vorschau und
das automatische Überspringen von Intros wirken live (ohne Neustart).

Siehe [adr/0011-intro-detection-providers.md](adr/0011-intro-detection-providers.md).

### Protokollierung
```env
EVEY_LOG_LEVEL=info                    # debug, info, warn, error
EVEY_LOG_DIR=/data/logs                # the image's default; the compose file maps it to ./evey-data/logs
EVEY_LOG_RETENTION_DAYS=90
```

Die tägliche Datei `evey-YYYY-MM-DD.log` unter `EVEY_LOG_DIR` ist das maßgebliche
Log. `docker logs evey-server` zeigt denselben Strom, aber der json-file-Treiber
von Docker rotiert ihn, und nach einer Rotation kann der Befehl Stunden vor dem
Container scheinbar aufhören. Sind sich beide uneinig, glauben Sie der Datei -
oder als root den rohen Dateien `/var/lib/docker/containers/<id>/*-json.log*`.
Außerhalb des Images (eine Installation ohne Container) ist der Standard ein
Verzeichnis `logs` relativ zum Arbeitsverzeichnis; richten Sie `EVEY_LOG_DIR`
also auf einen beschreibbaren absoluten Pfad.

### Fernzugriff (wer sich von außerhalb Ihres Netzes verbinden darf)
```env
EVEY_REMOTE_ACCESS_ENABLED=                                              # unset = auto (see below); true/false to decide yourself
EVEY_REMOTE_ACCESS_EXTERNAL_URL=                                         # the public address, if it differs from EVEY_SERVER_BASE_URL
EVEY_REMOTE_ACCESS_LAN_NETWORKS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16  # CIDRs treated as LAN
EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS=0                                    # 0 = no cap for remote (WAN) sessions
```

> ⚠️ Die Sperre schützt den **HTTP**-Listener (`EVEY_SERVER_PORT` + 1). Der native
> gRPC-Listener auf `EVEY_SERVER_PORT` selbst ist ein interner Transport und wird
> nicht gesperrt, deshalb bindet er **nur an Loopback** (`EVEY_GRPC_BIND`,
> Standard `127.0.0.1`): Sein einziger Client ist das HTTP-Gateway im selben
> Prozess. Setzen Sie `EVEY_GRPC_BIND=0.0.0.0`, um ihn bewusst zu öffnen - der
> Start warnt dann, dass die Sperre für den Fernzugriff ihn nicht abdeckt, und
> das Abschirmen per Firewall wird Ihre Aufgabe.

**Die Sperre.** Ist der Fernzugriff aus, wird jeder Client, dessen IP nicht in
`EVEY_REMOTE_ACCESS_LAN_NETWORKS` liegt, auf jeder Route mit
`403 {"code":"remote_access_disabled"}` abgewiesen. Das ist eine Entscheidung zur
Bereitstellung („diese Instanz ist für mein eigenes Netz“), keine
Sicherheitsschicht - Ihre Bibliothek schützt die Authentifizierung.

**Standardmäßig automatisch.** Lassen Sie `EVEY_REMOTE_ACCESS_ENABLED` ungesetzt,
entscheidet der Server beim Start: Ist `EVEY_REMOTE_ACCESS_EXTERNAL_URL` oder
`EVEY_SERVER_BASE_URL` eine öffentlich erreichbare Adresse, ist der Fernzugriff
**eingeschaltet** (ein Server, den Sie veröffentlicht haben, soll erreichbar
sein); sonst bleibt er auf das LAN beschränkt. Die Entscheidung wird beim Start
geloggt, und `evey admin doctor` meldet sie.

Setzen Sie die Variable - oder nutzen Sie **Admin → Fernzugriff** -, um
ausdrücklich zu entscheiden; eine ausdrückliche Wahl wird von der Ableitung nie
überschrieben. Halten Sie einen veröffentlichten Server bewusst im LAN, trägt das
Startlog eine Warnung, denn das Symptom, das Sie sonst erreicht, lautet „auf
deinem Handy geht es, auf meinem nicht“.

> Hinter einem Reverse Proxy funktioniert das nur, wenn der Proxy die echte
> Client-IP weitergibt und `EVEY_TRUSTED_PROXIES` den Proxy abdeckt. Sonst sieht
> jeder Client wie die eigene Adresse des Proxys aus - siehe
> [reverse-proxy.md](reverse-proxy.md).

Ist `EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS` gesetzt, werden Wiedergabesitzungen,
deren Client-IP als WAN eingeordnet ist, auf diese Bitrate gedeckelt; die
Obergrenze SENKT die Obergrenze von Client/Profil immer nur. Zwei
Sicherheitsregeln sollten Sie kennen:

- Eine private IP nach RFC1918 wird **nie** gedeckelt, selbst wenn Sie vergessen
  haben, ihren Bereich in `EVEY_REMOTE_ACCESS_LAN_NETWORKS` einzutragen - die
  Obergrenze trifft nur eine eindeutig öffentliche Adresse.
- Nutzen Sie ungewöhnliche lokale Bereiche (CGNAT `100.64.0.0/10`, ein
  VPN-/Tailscale-Subnetz) und wollen Sie diese Zuschauer NICHT deckeln, tragen Sie
  sie ausdrücklich in `EVEY_REMOTE_ACCESS_LAN_NETWORKS` ein. Hinter einem Reverse
  Proxy achten Sie darauf, dass er die echte Client-IP weitergibt
  (`X-Forwarded-For`), sonst sieht jede Sitzung wie die Adresse des Proxys aus.
