---
source_sha: 58ae53ccc12bc8dbe6e691e81203c38189833f7fe97b8f072c82a6641ae48c30
reviewed: true
---
# Fehlerbehebung

Etwas funktioniert nicht? Suchen Sie in der Tabelle unten, was Sie sehen. Die
mittlere Spalte sagt, wo Sie zuerst nachsehen; die letzte führt zur Lösung.
Reicht das nicht, sagen die Protokolle und `evey admin doctor` (weiter unten)
mehr.

## Wo Sie nachsehen, nach Anzeichen

| Was Sie sehen | Zuerst nachsehen | Dann |
| --- | --- | --- |
| Nichts antwortet, nicht einmal zu Hause | Läuft Evey? `docker compose ps`, dann die letzten Zeilen seiner Ausgabe: `docker compose logs --tail=200 evey` | [Der Server startet nicht](#the-server-does-not-start) unten; [Der Server antwortet nicht](../runbooks/incident-runbook.md#the-server-does-not-answer) |
| Die App zeigt **Server nicht erreichbar** oder **Verbindung fehlgeschlagen** | Die in der App eingetippte Adresse | [Apps verbinden](../getting-started/apps.md#the-server-address-to-enter); nur unterwegs: [Fernzugriff und HTTPS](../administration/remote-access.md) |
| Alle sind abgemeldet, oder niemand kann sich anmelden | **Verwaltung → Fernzugriff**, die vertrauenswürdigen Proxys; dann die Protokolle | [Niemand kann sich anmelden](#nobody-can-sign-in) unten |
| Ein Film zeigt kein Bild, startet nicht oder bricht ab | **Stream-Info** im Player, dann **Verwaltung → Medienzustand** für diesen Titel | [Probleme bei der Wiedergabe](#playback-problems) unten |
| Es ruckelt oder lädt ständig | **Verwaltung → Streaming-Qualität** (welche Titel und Geräte warten) und **Verwaltung → Systemzustand** (ist der Server zu ausgelastet) | [Transkodierung und Serverkapazität](../playback/transcoding.md) |
| Falsches Poster, falscher Titel, oder ein Film wird nicht erkannt | **Metadaten bearbeiten** beim Titel | [Einen falschen oder fehlenden Titel korrigieren](../administration/libraries.md#fix-a-wrong-or-missing-title) |
| Poster fehlen oder sind kaputt | **Verwaltung → Artwork-Cache** | [Bibliothek und Poster](#library-and-posters) unten |
| Neue Dateien erscheinen nicht | Das Scan-Ergebnis in der Zeile der Bibliothek unter **Verwaltung → Bibliotheken** | [Bibliothek und Poster](#library-and-posters) unten |
| E-Mails kommen nicht an | **Test-E-Mail senden** unter **Verwaltung → Serverkonfiguration → E-Mail / SMTP** | [E-Mails kommen nicht an](#e-mails-do-not-arrive) unten |
| Ein Plugin schlägt fehl | Seine **Protokolle**, auf seiner Karte unter **Verwaltung → Plugins** | [Ein Plugin ist abgestürzt](../runbooks/incident-runbook.md#a-plugin-crashed) |
| Ein Cast ist eingefroren oder stumm geworden | **Verwaltung → Aktuelle Wiedergabe**: Steht die Wiedergabe noch in der Liste? | [Ein Cast ist eingefroren, stumm geworden, oder das Handy lädt endlos](../runbooks/incident-runbook.md#a-cast-froze-went-silent-or-the-phone-keeps-spinning) |
| Die Festplatte läuft voll | **Verwaltung → Transkodierungs-Cache** | [Die Cache-Festplatte ist voll](../runbooks/incident-runbook.md#the-cache-disk-is-full) |

## Die Protokolle

Das Protokoll ist Eveys Tagebuch: Jeder Fehler wird dort eingetragen, mit der
Uhrzeit.

**Der einfache Weg: Verwaltung → Protokolle & Debug.** Dort sehen Sie das
Protokoll mit einem Suchfeld, einer Ansicht **Live**, die neuen Zeilen folgt,
und einem Filter **Bereich** für einen bestimmten Teil (Player und
Umwandlung, Untertitel, Intro-Erkennung, Chromecast, Anmeldung...). Meldet
jemand ein Problem, sehen Sie zuerst hier nach, um die Uhrzeit herum, die man
Ihnen nennt.

??? info "Wo die Protokolldateien liegen"

    - Die Dateien liegen in `./evey-data/logs/`, neben Ihrer Compose-Datei
      (`/data/logs/evey-YYYY-MM-DD.log` im Container). Eine Datei pro Tag, in
      JSON, standardmäßig 90 Tage aufbewahrt (`EVEY_LOG_RETENTION_DAYS`).
    - `docker compose logs evey` zeigt etwas anderes: was der Container beim
      Start ausgibt (die `evey:`-Zeilen des Startskripts, die Datenbank, das
      Banner). Lesen Sie das, wenn der Server nicht hochkommt. Sobald er
      läuft, steht das Protokoll in den Dateien und im Verwaltungsbereich.

## `evey admin doctor`

Ein Befehl prüft den ganzen Server. Er gibt pro Prüfung eine Zeile aus, `ok`,
`WARN` oder `FAIL`, mit dem, was zu tun ist:

```bash
docker exec evey-server evey admin doctor
```

??? info "Was er prüft"

    Die Datenbank und ihr Schema, den Signaturschlüssel, den Suchindex (und ob
    er zur Datenbank passt), die Medienordner (auch, ob Evey in jede
    Bibliothek schreiben kann), die Cache- und Protokollordner, `ffmpeg` und
    `mkvgo`, die vertrauenswürdigen Proxys, den Fernzugriff und den freien
    Speicherplatz. `--repair` wendet nur die ungefährlichen Korrekturen an
    (fehlende Ordner anlegen und ihnen den richtigen Besitzer geben). Schlägt
    eine Prüfung fehl, endet der Befehl mit einem Fehlercode. Die anderen
    Befehle stehen unter [Kommandozeile](../runbooks/cli.md).

## Der Server startet nicht

| Was `docker compose logs evey` sagt | Was es bedeutet |
| --- | --- |
| Eine Meldung zur Prüfung der Konfiguration | Mit einer externen Datenbank lehnt der Server das Standardpasswort `evey` ab; mit einem externen Meilisearch einen leeren Schlüssel oder einen Platzhalter. Korrigieren Sie den Wert, den die Meldung nennt. |
| Zeilen, die mit `evey:` beginnen und Postgres betreffen | Die eingebaute Datenbank wollte nicht starten: ein Cluster einer anderen Postgres-Hauptversion ([Wechsel der Postgres-Hauptversion](advanced-deployments.md#move-to-a-newer-postgres-major)), ein Datenverzeichnis, das kein Postgres-Cluster ist, eine volle Festplatte oder `./evey-data/postgres` auf einer Netzwerkfreigabe. |
| `permission denied` bei `/cache/...`, in einer Neustartschleife | Das Volume `evey-cache` gehört einem anderen Benutzer als dem, unter dem der Server läuft. Aktualisieren Sie das Image; bei einem Image von vor dem 17.09.2026 führen Sie einmal `docker run --rm -v <project>_evey-cache:/c alpine chown -R <PUID>:<PGID> /c` aus. |

Ein verlorenes Administratorpasswort, ein kompromittierter Signaturschlüssel
oder eine kaputte Datenbank: [Wiederherstellung](../runbooks/recovery.md).

## Niemand kann sich anmelden

Zwei übliche Ursachen:

- **Die Datenbank antwortet nicht.** Evey prüft jede Anmeldung gegen sie und
  lehnt ab, wenn das nicht geht. Das Protokoll sagt es.
- **Ein Reverse Proxy fehlt bei den vertrauenswürdigen Proxys** (**Verwaltung
  → Fernzugriff**). Jeder Besucher scheint dann von der Adresse des Proxys zu
  kommen, und alle teilen sich eine einzige Grenze für Anmeldeversuche.

Das Vorgehen Schritt für Schritt:
[Jede Anmeldung liefert 401](../runbooks/incident-runbook.md#every-login-returns-401).

!!! note
    Ein verlorener oder ersetzter Signaturschlüssel allein meldet niemanden
    ab: Die Apps erneuern ihre Tokens. Wer absichtlich jede Sitzung beenden
    will, nutzt `evey admin rotate-jwt-key`, siehe
    [Wiederherstellung](../runbooks/recovery.md#rotate-the-key-that-signs-access-tokens).

Der Server antwortet zu Hause, aber nicht anderswo, oder die Apps sagen, dass
der Fernzugriff deaktiviert ist:
[Fernzugriff und HTTPS](../administration/remote-access.md).
**Verwaltung → Fernzugriff** zeigt in Rot an, wenn der Server aus dem
Internet erreichbar ist, während der Fernzugriff aus ist.

### Die Anmeldung über SSO schlägt fehl

Die Anmeldeseite nennt den Grund:

- **Domain nicht zugelassen**: Die Adresse liegt außerhalb der in der Karte
  zugelassenen E-Mail-Domains;
- **kein passendes Konto**: Die Registrierung ist aus; schalten Sie **Konten
  für neue SSO-Benutzer anlegen** ein oder legen Sie das Konto an;
- **Konto deaktiviert**;
- **Konto existiert, konnte aber nicht verknüpft werden**: Der Anbieter hat
  die E-Mail nicht bestätigt, das Verknüpfen ist aus, oder das Konto ist mit
  einer anderen Identität verknüpft;
- **nicht verfügbar**: Der Anbieter ist nicht erreichbar; **Status** und
  **Verbindung testen** in der Karte zeigen den Fehler.

Eine Fehlerseite, die der Anbieter selbst anzeigt, heißt meist, dass die dort
eingetragene Weiterleitungs-URI nicht die ist, die die Karte zeigt. Siehe
[OIDC / SSO](../oidc.md).

## E-Mails kommen nicht an

**Test-E-Mail senden** unter **Verwaltung → Serverkonfiguration → E-Mail /
SMTP** schickt mit den gespeicherten Einstellungen eine Nachricht an Ihre
eigene Adresse und zeigt die Antwort des Mailservers, wenn es nicht klappt.
Ein TLS-Modus, der nicht zum Port passt (`starttls` mit 587, `ssl` mit 465),
ist der übliche Fehler. Ein in der Karte gespeicherter Host ersetzt jede
Variable `EVEY_SMTP_*`. Fehlgeschlagene Benachrichtigungs-E-Mails werden als
`email: send notification` protokolliert. Siehe
[Benachrichtigungen](../notifications.md).

## Probleme bei der Wiedergabe

Beginnen Sie mit **Stream-Info** im Player. Dort steht, wie der Film gesendet
wird (unverändert, in ein anderes Format umkopiert oder umgewandelt), Spur
für Spur, und warum. Ein Titel wird umgewandelt, obwohl Sie erwartet haben,
dass er unverändert läuft: siehe
[Wie über die Wiedergabe entschieden wird](../playback/how-playback-is-decided.md).

- **Ein Titel** bricht ab, lädt ständig oder springt nur langsam an eine
  andere Stelle: Öffnen Sie **Verwaltung → Medienzustand**. Die üblichen
  Ursachen sind eine Datei ohne Index zum Springen, ein beschädigter Abschnitt
  oder Spuren, die in großen Blöcken gespeichert sind. Das meiste davon lässt
  sich reparieren. Siehe [Medienzustand](../runbooks/media-health.md).
- **Mehrere Titel, wenn mehrere Leute schauen**: Der Server ist vielleicht an
  der Grenze dessen, was er umwandeln kann. **Verwaltung → Systemzustand**
  zeigt, wie ausgelastet er ist. Siehe
  [Transkodierung und Serverkapazität](../playback/transcoding.md).
- Die transkodierte Wiedergabe schlägt ganz fehl:
  [Die Transkodierung schlägt fehl](../runbooks/incident-runbook.md#transcoded-playback-fails).
- Es puffert, und die Protokolle sagen nicht, warum:
  [das Vorgehen zur Diagnose](../runbooks/incident-runbook.md#it-buffers-or-cuts-out-and-the-log-does-not-say-why).
- Kein Ton, nur im Browser, bei einer Dolby-Digital-Spur (AC-3/E-AC-3): Ein
  Reverse Proxy, der eine eigene Content-Security-Policy setzt, blockiert
  vielleicht den Decoder, den der Web-Player benutzt. Siehe
  [Audio](../playback/audio.md#dolby-digital-in-a-browser).
- Ton nicht synchron bei einem Titel: **Verwaltung → Medienzustand** erkennt
  Ton, der nach dem Bild beginnt, und Spuren, die nicht zueinander passen.
- Untertitel erscheinen nicht: [Untertitel](../playback/subtitles.md) sagt,
  welche Formate die Player selbst zeichnen und welche der Server ins Bild
  einbrennen muss.
- **Untertitel werden vorbereitet…** bleibt minutenlang stehen: Die Datei hat
  für diese Spur keinen Index, und der Server liest sie einmal ganz durch.
  Siehe
  [Wenn ein Untertitel zum ersten Mal angezeigt wird](../playback/subtitles.md#the-first-time-a-subtitle-is-shown).
- Transkodierte Titel wollen nicht starten oder bleiben hängen, während schon
  gespielte Titel weiter funktionieren, oder Administratoren bekommen
  **Untertitel-Cache voll**: Die Festplatte liegt unter ihrer Reserve an
  freiem Speicher. Siehe
  [Der Transkodierungs-Cache](../playback/transcoding.md#the-transcoding-cache).

## Bibliothek und Poster

Poster und Hintergrundbilder werden bei den Scans auf den Server kopiert, nach
`./evey-data/artworks`, und fehlende werden jede Stunde erneut versucht.
**Verwaltung → Artwork-Cache** zeigt, wie viele noch keine lokale Kopie haben;
**Fehlende Artworks herunterladen** holt sie sofort, und **Beschädigte
Varianten bereinigen** entfernt verkleinerte Poster, die ein misslungenes
Verkleinern leer gelassen hat. Sie werden dann bei Bedarf neu erstellt.

Ein Poster oder ein Titel, der zu einem anderen Film gehört, ist ein Fehler
bei der Zuordnung der Metadaten, kein Problem mit den Bildern:
[Einen falschen oder fehlenden Titel korrigieren](../administration/libraries.md#fix-a-wrong-or-missing-title).

Eine neue Datei erscheint normalerweise innerhalb weniger Minuten (siehe
[Wie neue Dateien gefunden werden](../administration/libraries.md#how-new-files-are-picked-up)).
Wenn nicht, drücken Sie in der Zeile der Bibliothek unter **Verwaltung →
Bibliotheken** auf **Scannen** und lesen Sie das Ergebnis.

Ein Scan endet mit **Unvollständiger Scan - diese Ordner konnten nicht gelesen
werden:**, gefolgt von Pfaden: Evey kann diese Ordner nicht lesen
(Berechtigungen, eine Netzwerkfreigabe, die weggefallen ist). Der Rest der
Bibliothek wird gescannt, und die Titel unter einem unlesbaren Ordner bleiben
erhalten, statt entfernt zu werden. Prüfen Sie die Einbindung und den
Besitzer (`PUID`/`PGID`, siehe
[Installation](../installation.md#common-variations)) und scannen Sie dann
erneut.

Die Suche findet nichts: Bauen Sie den Suchindex mit
`docker exec evey-server evey admin reindex-search` aus der Datenbank neu
auf. Dabei geht nichts verloren: Der Index lässt sich immer neu aufbauen.

## Ein Problem melden

Schreiben Sie an **contact@eveyproject.org**. Eine Meldung, mit der man etwas
anfangen kann, sagt:

- was Sie getan haben, was Sie erwartet haben und was stattdessen passiert
  ist;
- wann es passiert ist, auf die Minute genau: Die Protokolle werden nach
  Uhrzeit durchsucht;
- das Gerät und die App: **Einstellungen → Über → App-Version** in der
  Android-App, bei der Web-App der Browser und seine Version;
- die Version des Servers: `docker exec evey-server evey version`;
- bei einem Problem mit der Wiedergabe den Titel und ein Bildschirmfoto von
  **Stream-Info** (siehe
  [Stream-Info lesen](../playback/how-playback-is-decided.md#reading-stream-info)):
  Dort stehen die gewählte Methode und ihre Gründe, und das ist schon der
  größte Teil der Diagnose.

Ein Administrator kann die Protokollzeilen um diesen Zeitpunkt herum beilegen
(**Verwaltung → Protokolle & Debug**, siehe [Die Protokolle](#the-logs)) und
den Bericht von [`evey admin doctor`](#evey-admin-doctor). Protokolle nennen
Konten und Netzwerkadressen: Lesen Sie sie, bevor Sie sie verschicken.

Eine Sicherheitslücke meldet man nicht an einem öffentlichen Ort: Schreiben
Sie an **security@eveyproject.org**, und lassen Sie Zeit für die Behebung,
bevor Sie darüber sprechen.

<!-- sources
symptom table: i18n mobile.serverConnect.unreachable / connectionFailed ; i18n media.rematchMetadata, admin.library.refreshMetadataTooltip
logs: internal/pkg/logger/logger.go - New (file handler only outside DevMode), rotatingWriter.rotate (evey-YYYY-MM-DD.log), StartCleanup (90 days) ; internal/adapters/driving/http/admin_handler_logs.go - currentLogPath ; docker-compose.yml EVEY_LOG_DIR=/data/logs, ./evey-data:/data ; i18n admin.logs.where.*, admin.logs.live
docker compose logs: docker/entrypoint.sh (evey: lines), cmd/evey/banner.go
doctor: cmd/evey/admin_commands.go - doctor ; cmd/evey/admin_ops.go - checkResult names (database, schema, jwt.key, meilisearch, search index, media <root>, trusted proxies, remote access, disk), checkTool ffmpeg/mkvgo
sso: internal/adapters/driving/http/auth_handler.go /login?oidc_error=<reason>, i18n auth.login.ssoError.* ; smtp: internal/adapters/driving/http/smtp_test_handler.go (test send), internal/adapters/driven/email/notification_emailer.go (log)
partial scan: internal/app/library/scan_service.go - scanDirectories (unreadable root = failed path, protected from pruning) ; i18n admin.libraries.scanPartial
posters: internal/app/metadata/command_service_artwork.go (hourly retry) ; i18n admin.artworks.* (/data/artworks, purge variants)
rotate-jwt-key ends sessions: cmd/evey/admin.go - endAllSessions
startup failures: docker/entrypoint.sh ; cache ownership: docker/entrypoint.sh (re-own /cache)
remote access warning: docs/adr/0026-remote-access-inferred-from-a-published-url.md
reporting: docs/privacy.md (contact@eveyproject.org) ; docs/SECURITY.md (security@eveyproject.org) ; app version: frontend/apps/mobile/app/(auth)/(tabs)/settings.tsx, i18n mobile.settings.appVersion ; server version: cmd/evey/cli.go
new files row: i18n admin.libraries.scan, scanPartial ; docs/administration/libraries.md (How new files are picked up) ; e-mail row: i18n admin.serverConfig.sendTestEmail
-->
