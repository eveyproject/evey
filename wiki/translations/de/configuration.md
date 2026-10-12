---
source_sha: fc6dc6008146f2970b260b3c75a326e8cda72d352a8582783f9ee1b68923c74a
reviewed: true
---
# Konfigurationsreferenz

Wo jede Einstellung eines Evey-Servers liegt, welcher Wert gilt, wenn es eine
Einstellung sowohl im Verwaltungsbereich als auch in der Umgebung gibt, und
wofür ein Neustart nötig ist. Die Variablen selbst stehen, nach Themen
geordnet, auf den zehn unten verlinkten Seiten. Diese Seite richtet sich an
Administratoren, die die Compose-Datei bearbeiten oder sich fragen, warum eine
Variable scheinbar nicht wirkt.

## Zwei Orte, eine Regel

**Was vor dem Serverstart da sein muss** - Geheimnisse, Speicherpfade, die
öffentliche Adresse, Angaben zum Reverse Proxy - gehört als
Umgebungsvariable in die Compose-Datei. **Was ein Administrator danach
einstellt** - Sprachen, Grenzen der Transkodierung, Hardwarebeschleunigung,
Zeitpläne, Plugin-Grenzen - liegt im Verwaltungsbereich. Es wird in der
Datenbank gespeichert, übersteht ein Update des Containers und gilt meist
ohne Neustart.

Gibt es eine Einstellung an beiden Orten, gilt immer dieselbe Reihenfolge:

1. **Der im Verwaltungsbereich gespeicherte Wert gewinnt.** Bei jedem Start
   liest der Server zuerst die Umgebung. Dann ersetzt er jedes Feld durch den
   Wert aus der Datenbank, falls dort einer gespeichert ist. Eine Variable
   überschreibt nie einen gespeicherten Wert.
2. **Die Variable gilt, solange nichts gespeichert ist.** Der
   Verwaltungsbereich zeigt sie als aktuellen Standard. **Auf Standard
   zurücksetzen** löscht den gespeicherten Wert, und ab dem nächsten Start
   gilt wieder die Variable.
3. **Der eingebaute Standard** gilt, wenn keines von beiden gesetzt ist.

Geheimnisse sind die Ausnahme: Der Verwaltungsbereich zeigt nie ein Passwort
oder einen Schlüssel an, den er gespeichert hat. Eine Variable wie
`EVEY_TMDB_API_KEY` wird bei der ersten Verwendung in die Datenbank kopiert,
damit der Verwaltungsbereich anzeigen kann, dass ein eigener Schlüssel
verwendet wird.

## Was einen Neustart braucht

- Eine Änderung an einer **Umgebungsvariable** verlangt immer, dass der
  Container neu erstellt wird (`docker compose up -d`): Der Prozess liest
  seine Umgebung nur einmal.
- Eine Änderung im **Verwaltungsbereich** gilt bei den meisten Einstellungen
  sofort (Servername, Protokollstufe, Anfragegrenzen, Hardwarebeschleunigung,
  SMTP, Single Sign-on, Fernzugriff, Plugin-Grenzen, Aufbewahrungsfristen
  beim nächsten täglichen Durchlauf).
- Der Verwaltungsbereich kennzeichnet die Einstellungen, die **beim Start
  gelesen werden**, mit *(wird beim Neustart angewendet)*: die
  Dateiüberwachung der Bibliothek und das Scanintervall, das Budget des
  Transkodierungs-Caches, die Karte **Erweiterte Grenzwerte** unter
  Transkodierung, die Intro-Anbieter und jede Zeile unter
  **Konfigurationseinträge**. Speichern merkt sich die Wahl, ein Neustart
  wendet sie an.

Jede Tabelle auf den Themenseiten hat eine Spalte **Verwaltungsbereich**: den
Weg zum Feld (beginnend bei **Verwaltung**) und ob es sofort oder beim
Neustart gilt, oder `keine`, wenn es für die Variable kein Gegenstück im
Verwaltungsbereich gibt.

| Seite | Inhalt |
| --- | --- |
| [Server und Netzwerk](environment/server.md) | Öffentliche Adresse, Ports, Reverse Proxy, Fernzugriff, Anfragegrenzen, Benutzer des Containers |
| [Datenbank und Suche](environment/database-search.md) | Eingebettetes oder externes Postgres und Meilisearch |
| [Konten und Anmeldung](environment/accounts.md) | Gültigkeitsdauer der Sitzungen, Signaturschlüssel, Single Sign-on |
| [Bibliotheken und Metadaten](environment/libraries.md) | Scannen, Metadaten-Schlüssel, Intro-Erkennung |
| [Transkodierung und Hardware](environment/transcoding.md) | Grafikkarte, Kapazität, Qualitätsgrenzen, Cache, Kalibrierungsclip |
| [Auslieferung und Downloads](environment/playback-delivery.md) | Schalter für die Verpackung der Streams, vorbereitete Downloads, Überspringen-Schaltflächen im Player |
| [Hintergrundaufgaben](environment/background.md) | Durchläufe im Hintergrund und ihr Tempo |
| [Plugins und Sandbox](environment/plugins.md) | Sandbox-Modus, eigener Benutzer, Ressourcengrenzen, Signaturen |
| [E-Mail und Benachrichtigungen](environment/notifications.md) | SMTP, Schlüssel für Browser-Push |
| [Sicherungen, Protokolle und Überwachung](environment/logs-monitoring.md) | Sicherungen, Protokoll, Aufbewahrung des Verlaufs, Metriken und Tracing |

## Wie Werte gelesen werden

- Wahrheitswerte akzeptieren `1`, `t`, `true`, `0`, `f`, `false`. Zeitdauern
  brauchen eine Einheit: `30s`, `5m`, `2h` (eine bloße `30` wird ignoriert).
  Listen werden durch Kommas getrennt.
- Ein Wert, den der Server nicht lesen kann, wird **mit einer Warnung im Protokoll
  ignoriert** (`environment variable ignored, value could not be parsed`),
  und der Standard gilt. Prüfen Sie die ersten Zeilen des Protokolls, nachdem Sie
  die Compose-Datei bearbeitet haben.
- `EVEY_DB_PASSWORD` und `MEILI_MASTER_KEY` können auch aus einer Datei
  kommen, deren Pfad in derselben Variable mit der Endung `_FILE` steht. Ein
  ausdrücklich gesetzter Wert gewinnt gegenüber der Datei.

## Geheimnisse

Das eingebettete Postgres und Meilisearch erzeugen ihre Geheimnisse beim
ersten Start in `./evey-data/keys` (`postgres.password`, `meili.key`, Modus
0600). Der Server liest sie über `EVEY_DB_PASSWORD_FILE` und
`MEILI_MASTER_KEY_FILE`, die im Image gesetzt sind. Deshalb brauchen auch die
Befehle `evey admin` kein Passwort. Setzen Sie `EVEY_DB_PASSWORD` oder
`MEILI_MASTER_KEY` nur für eine externe Datenbank oder Suchmaschine.

Ein TMDB-Schlüssel ist eingebaut. `EVEY_TMDB_API_KEY` hinterlegt Ihren
eigenen. Der Schlüssel unter **Verwaltung → Serverkonfiguration →
API-Schlüssel** gewinnt gegenüber dieser Variable.

## Variablen, die Sie nicht brauchen

Zeilen, die oft aus älteren Installationen übernommen werden, und warum sie
nichts ändern:

| Zeile | Warum sie unnötig ist |
| --- | --- |
| `EVEY_LOG_DIR`, `EVEY_TRANSCODING_CACHE_DIR`, `EVEY_TRICKPLAY_CACHE_DIR` | Das Image setzt sie schon auf `/data/logs`, `/cache/transcode`, `/data/trickplay`. Sie zu wiederholen ändert nichts. |
| `EVEY_FORWARDED_PROTO` | Veraltet - wird aus dem Schema von `EVEY_SERVER_BASE_URL` abgeleitet. Wer sie setzt, bekommt nur eine Warnung im Protokoll, dass sie veraltet ist. |
| `EVEY_KEYFRAME_BACKFILL=on` | Alles außer `off` schaltet die Funktion ein, auch wenn die Variable fehlt. |
| `LIBVA_DRIVER_NAME` | libva wählt den Treiber anhand der PCI-ID der GPU. Erzwingen Sie ihn nur, wenn Ihre Karte einen Treiber braucht, den libva nicht wählt (prüfen Sie es mit `docker exec evey-server vainfo`). |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` wählt die stärkste Abschottung, die Ihr Host unterstützt. Setzen Sie die Variable nur ausdrücklich, wenn Sie eine *fail-closed*-Garantie wollen: `landlock` weigert sich, ein Plugin zu starten, das es nicht abschotten kann, während `auto` mit einer Warnung auf eine schwächere Stufe zurückfällt. |
| ein `group_add` für `/dev/dri/card0` | Evey öffnet immer nur `renderD128`. Prüfen Sie es mit `ls -ln /dev/dri` und fügen Sie nur diese Gruppe hinzu. |

## Dateibesitz (PUID und PGID)

**Standard: automatisch.** Wenn `PUID`/`PGID` nicht gesetzt sind, schaut das
Startskript (Entrypoint) beim Start nach, wem Ihre `/media*`-Mounts gehören,
und startet den Server als dieser Benutzer. So funktionieren die Funktionen,
die *in Ihre Bibliothek schreiben* (die Reparatur des Seek-Index baut Dateien
an Ort und Stelle neu auf), ohne weiteres Zutun - kein `chown`, keine
Konfiguration. Das Protokoll beim Start sagt Ihnen, was gewählt wurde:

```
evey: media mounts owned by 1026:100 (detected on /media) - running the
server as that user so write features (seek-index repair) work out of the box
```

Gehören die Medien-Mounts **verschiedenen Benutzern**, macht der Server das
Beste daraus: Er übernimmt den Benutzer, dem die meisten Mounts gehören, und
tritt den Gruppen der anderen Mounts als Zusatzgruppen bei. Reparaturen auf
diesen Mounts funktionieren dann, wenn ihre Dateien für die Gruppe
beschreibbar sind (die Warnung beim Start nennt die Mounts, die
Schreibzugriffe womöglich noch ablehnen). Die automatische Erkennung tritt nur
ganz zurück (sie fällt mit einer Warnung auf `1000:1000` zurück), wenn jeder
Medien-Mount root oder der reservierten UID `1001` gehört. Übertragen Sie Ihre
Medien dann mit chown an einen normalen Benutzer, oder setzen Sie
`PUID`/`PGID` ausdrücklich:

```env title=".env"
PUID=1026   # den Server unter dieser Benutzer-ID starten  (Host: `id -u <media-owner>`)
PGID=100    # ...und mit dieser Gruppen-ID
```

Ausdrücklich gesetzte Werte gewinnen immer gegenüber der Erkennung.

So funktioniert es: Der Container startet als root, gerade lange genug, um
seinen internen Benutzer umzustellen (und Eveys eigene Zustandsverzeichnisse
`/data`, `/cache` neu zuzuweisen - **nie** Ihre Medien-Mounts). Dann gibt er
die Rechte ab. Zusatzgruppen, die über `group_add:` in der Compose-Datei
vergeben werden (zum Beispiel die Render-Gruppe Ihres Hosts für die
Hardware-Transkodierung über `/dev/dri`), bleiben dabei erhalten. Der
Serverprozess selbst läuft nie als root.

Ganz ohne root: Wenn Ihnen selbst dieser kurze Moment als root zu viel ist,
setzen Sie stattdessen `user: "UID:GID"` am Dienst in Ihrer Compose-Datei.
Das Startskript erkennt den Start ohne root und überspringt die Umstellung
(auch die Erkennung des Medienbesitzers). Dann sorgen Sie selbst dafür, dass
`./evey-data` für diese UID beschreibbar bleibt.

!!! note
    `PUID=1001` wird abgelehnt: Das ist der interne Benutzer `evey-plugin`.
    Er bleibt getrennt, damit Plugins nie die Identität des Servers teilen.
    Medien werden standardmäßig mit Lese- und Schreibzugriff eingebunden,
    damit Reparaturen direkt funktionieren. Hängen Sie `:ro` an das
    Medien-Volume an, wenn Sie eine schreibgeschützte Bibliothek bevorzugen
    (Reparaturen scheitern dann mit einer klaren Fehlermeldung).

## Variablen, die der Server nicht liest

Manche Variablen stehen in Compose-Dateien, ohne dass der Server sie liest:

- `EVEY_HTTP_PORT`, `EVEY_DOMAIN`, `MEDIA_DIR`: werden von der
  `docker-compose.yml` des Repositorys verwendet (veröffentlichter Port, die
  Domain des mitgelieferten Caddy, der Medienpfad), nicht von Evey.
  `EVEY_VERSION` ist dort das Image-Tag. Der Server zeigt sie nur auf
  `/healthz` an, wenn sein Programm keine Version mitbringt.
- `EVEY_WEB_DIR` (`/srv`, wo die Dateien der Weboberfläche liegen) sowie
  `EVEY_DB_PASSWORD_FILE`, `MEILI_MASTER_KEY_FILE`, `EVEY_IMAGE_REPO`,
  `EVEY_IMAGE_RUNTIMES`: passen zum Aufbau des Images. Sie zu ändern wird
  nicht unterstützt.
- `EVEY_SUPERVISED`, `EVEY_COMPANION_PIDS`, `EVEY_RESTART_EXIT_CODE`,
  `EVEY_LAUNCHER_BIN`, `EVEY_PLUGIN_LAUNCHER`: werden vom Startskript und vom
  Plugin-Launcher für den eigenen Gebrauch gesetzt.
- `EVEY_BANNER=off` blendet nur das Startbanner im Protokoll aus. `NO_COLOR`
  entfernt seine Farben.

<!-- sources
precedence: cmd/evey/main.go - loadOverride (DB row wins when non-empty) ; cmd/evey/wire_helpers.go - resolveFlag, resolveAPIKeyWithBuiltin (env seeded into server_config)
reset to default: internal/adapters/driving/http/settings_handler.go - reset / resetSideEffect / requiresRestart
parsing: internal/config/config.go - envBool, envDuration, envStrSlice, warnBadEnv, envSecret (_FILE)
restart: internal/adapters/driving/http/settings_handler.go (requiresRestart) ; i18n admin.transcoding.advancedRestartHint ; admin.serverConfig.appliedOnRestart ; admin.serverConfig.introProvidersRestart
variables: internal/config/config.go ; docker/entrypoint.sh ; Dockerfile ; docker-compose.yml ; cmd/evey/banner.go
-->
