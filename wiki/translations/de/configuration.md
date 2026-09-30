---
source_sha: 4346505ac20853c385d4b6641af12c128cdd868479ccef57a064e5b9e0901fce
reviewed: false
---
# Konfigurationsreferenz

Zwei Orte, eine Regel. **Was existieren muss, bevor der Server startet**
(Geheimnisse, Speicher, die öffentliche Adresse, Angaben zum Reverse Proxy),
gehört in die Compose-Datei. **Was ein Administrator danach einstellt**
(Sprachen, Transkodierungsgrenzen, Hardwarebeschleunigung, Zeitpläne,
Plugin-Grenzen), liegt im Admin-Bereich, wird in der Datenbank gespeichert,
übersteht ein Container-Update und gilt meist ohne Neustart. Bei einer
Einstellung, die es an beiden Orten gibt, belegt eine Umgebungsvariable nur den
ersten Start vor: Der im Admin-Bereich gespeicherte Wert gewinnt.

Die Variablen, nach Thema:

| Seite | Umfasst |
| --- | --- |
| [Server und Netzwerk](environment/server.md) | Öffentliche Adresse, Ports, Reverse Proxy, Fernzugriff, Anfragegrenzen, Container-Benutzer |
| [Datenbank und Suche](environment/database-search.md) | Eingebettetes oder externes Postgres und Meilisearch |
| [Konten und Anmeldung](environment/accounts.md) | Sitzungsdauern, Signaturschlüssel, Single Sign-on |
| [Bibliotheken und Metadaten](environment/libraries.md) | Scannen, Metadaten-Schlüssel, Intro-Erkennung |
| [Transkodierung und Hardware](environment/transcoding.md) | Grafikkarte, Kapazität, Qualitätsgrenzen, Cache, Kalibrierungsclip |
| [Auslieferung und Downloads](environment/playback-delivery.md) | Schalter für die Stream-Verpackung, vorbereitete Downloads, Sprungtasten des Players |
| [Hintergrundaufgaben](environment/background.md) | Hintergrunddurchläufe und ihre Taktung |
| [Plugins und Sandbox](environment/plugins.md) | Sandbox-Modus, eigener Benutzer, Ressourcengrenzen, Signaturen |
| [E-Mail und Benachrichtigungen](environment/notifications.md) | SMTP, Schlüssel für Browser-Push |
| [Sicherungen, Logs und Überwachung](environment/logs-monitoring.md) | Sicherungen, Log, Aufbewahrung des Verlaufs, Metriken und Tracing |

## Geheimnisse

Das eingebettete Postgres und Meilisearch erzeugen ihre Geheimnisse beim ersten
Start in `./evey-data/keys` (`postgres.password`, `meili.key`, Modus 0600). Der
Server liest sie über `EVEY_DB_PASSWORD_FILE` und `MEILI_MASTER_KEY_FILE`, die
im Image gesetzt sind, weshalb auch die Befehle `evey admin` kein Passwort
brauchen. Setzen Sie `EVEY_DB_PASSWORD` oder `MEILI_MASTER_KEY` nur für eine
externe Datenbank oder Suchmaschine; ein ausdrücklicher Wert hat Vorrang vor der
Datei.

Ein TMDB-Schlüssel ist eingebaut. `EVEY_TMDB_API_KEY` belegt Ihren eigenen vor;
der unter **Admin → Serverkonfiguration → API-Schlüssel** gesetzte Schlüssel
hat Vorrang.

## Variablen, die Sie nicht brauchen

Zeilen, die oft aus älteren Einrichtungen übernommen werden, und warum sie
nichts ändern:


| Zeile | Warum sie unnötig ist |
| --- | --- |
| `EVEY_LOG_DIR`, `EVEY_TRANSCODING_CACHE_DIR`, `EVEY_TRICKPLAY_CACHE_DIR` | Das Image setzt sie bereits auf `/data/logs`, `/cache/transcode`, `/data/trickplay`. Sie zu wiederholen ändert nichts. |
| `EVEY_FORWARDED_PROTO` | Veraltet - wird aus dem Schema von `EVEY_SERVER_BASE_URL` abgeleitet. Sie zu setzen protokolliert nur eine Veraltungswarnung. |
| `EVEY_KEYFRAME_BACKFILL=on` | Alles außer `off` schaltet es ein, auch das Fehlen. |
| `LIBVA_DRIVER_NAME` | libva wählt den Treiber anhand der PCI-ID der GPU. Erzwingen Sie ihn nur, wenn Ihre Karte einen Treiber braucht, den libva nicht wählt (prüfen mit `docker exec evey-server vainfo`). |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` wählt die stärkste Einschränkung, die Ihr Host unterstützt. Setzen Sie sie nur ausdrücklich, wenn Sie eine *Fail-closed*-Garantie wollen - `landlock` verweigert ein Plugin, das es nicht einschränken kann, wo `auto` mit einer Warnung zurückfällt. |
| ein `group_add` für `/dev/dri/card0` | Evey öffnet immer nur `renderD128`. Prüfen Sie mit `ls -ln /dev/dri` und fügen Sie nur diese Gruppe hinzu. |


## Dateibesitz (PUID und PGID)

**Standard: automatisch.** Sind `PUID`/`PGID` nicht gesetzt, sieht sich der
Entrypoint beim Start den Besitzer Ihrer `/media*`-Einbindungen an und führt
den Server als diesen Benutzer aus, sodass die Funktionen, die *in* Ihre
Bibliothek *schreiben* (die Reparatur des Wiedergabeindex baut Dateien an Ort
und Stelle neu), ohne Weiteres funktionieren - kein `chown`, keine
Konfiguration. Das Startlog sagt, was gewählt wurde:

```
evey: media mounts owned by 1026:100 (detected on /media) - running the
server as that user so write features (seek-index repair) work out of the box
```

Medieneinbindungen, die **verschiedenen Benutzern** gehören, werden nach bestem
Bemühen behandelt: Der Server übernimmt den Benutzer, dem die meisten
Einbindungen gehören, und tritt den Gruppen der anderen Einbindungen als
Zusatzgruppen bei, sodass Reparaturen auf diesen Einbindungen funktionieren,
wenn ihre Dateien für die Gruppe beschreibbar sind (die Startwarnung listet,
welche Einbindungen Schreibzugriffe noch verweigern könnten). Die automatische
Erkennung tritt nur ganz zurück (fällt mit einer Warnung auf `1000:1000`
zurück), wenn jede Medieneinbindung root oder der reservierten UID `1001` gehört
- übertragen Sie Ihre Medien an einen normalen Benutzer oder setzen Sie
`PUID`/`PGID` ausdrücklich:

```env title=".env"
PUID=1026   # den Server mit dieser Benutzer-ID ausführen  (Host: `id -u <media-owner>`)
PGID=100    # ...und dieser Gruppen-ID
```

Ausdrückliche Werte haben immer Vorrang vor der Erkennung.

So funktioniert es: Der Container startet nur so lange als root, wie nötig ist,
um seinen internen Benutzer neu zuzuordnen (und Eveys eigene
Zustandsverzeichnisse `/data`, `/cache` neu zu übereignen - **nie** Ihre
Medieneinbindungen), und gibt dann die Rechte ab. Über `group_add:` in der
Compose-Datei gewährte Zusatzgruppen (z. B. die render-Gruppe Ihres Hosts für
die Hardware-Transkodierung über `/dev/dri`) bleiben dabei erhalten. Der
Serverprozess selbst läuft nie als root.

Alternative ganz ohne root: Ist selbst dieser kurze root-Moment für Sie
inakzeptabel, setzen Sie stattdessen `user: "UID:GID"` am Dienst in Ihrer
Compose-Datei. Der Entrypoint erkennt den Start ohne root und überspringt die
Neuzuordnung (einschließlich der Erkennung des Medienbesitzers) - dann sorgen
Sie selbst dafür, dass `./evey-data` für diese UID beschreibbar bleibt.

Hinweise:
- `PUID=1001` wird verweigert: Es ist der interne Benutzer `evey-plugin`, der
  getrennt gehalten wird, damit Plugins nie die Identität des Servers teilen.
- Die Medien werden standardmäßig mit Schreibzugriff eingebunden, damit
  Reparaturen direkt funktionieren; hängen Sie `:ro` an das Medien-Volume an,
  wenn Sie eine schreibgeschützte Bibliothek bevorzugen (Reparaturen schlagen
  dann mit einem klaren Fehler fehl).

## Variablen, die der Server nicht liest

Manche Variablen stehen in Compose-Dateien, ohne dass der Server sie liest:

- `EVEY_HTTP_PORT`, `EVEY_DOMAIN`, `MEDIA_DIR`: von der eigenen
  `docker-compose.yml` des Repositorys genutzt (veröffentlichter Port, Domain
  des mitgelieferten Caddy, Medienpfad), nicht von Evey. `EVEY_VERSION` ist dort
  das Image-Tag; der Server zeigt es nur auf `/healthz`, wenn sein Programm keine
  Version trägt.
- `EVEY_WEB_DIR` (`/srv`, wo die Dateien der Weboberfläche liegen) und
  `EVEY_DB_PASSWORD_FILE`, `MEILI_MASTER_KEY_FILE`, `EVEY_IMAGE_REPO`,
  `EVEY_IMAGE_RUNTIMES`: passen zum Aufbau des Images. Sie zu ändern wird nicht
  unterstützt.
- `EVEY_SUPERVISED`, `EVEY_COMPANION_PIDS`, `EVEY_RESTART_EXIT_CODE`,
  `EVEY_LAUNCHER_BIN`, `EVEY_PLUGIN_LAUNCHER`: vom Entrypoint und vom
  Plugin-Launcher für den eigenen Gebrauch gesetzt.
