---
source_sha: a6c61f9f7b1e619f46c4d5eea7ca2c2b93356f93c04681e95963303bdf287723
reviewed: false
---
# Installation

Evey läuft als ein einziger Container. Das Image liefert die API und die
Weboberfläche über einen Port aus und betreibt darin seine eigene Datenbank
(Postgres) und Suchmaschine (Meilisearch), deren Passwörter beim ersten Start
erzeugt werden. Die Compose-Datei unten ist die gesamte Installation: keine
`.env`, kein privilegierter Modus, keine Speicher- oder CPU-Einstellung.

!!! warning "Nicht die docker-compose.yml des Repositorys"

    Die `docker-compose.yml` im Wurzelverzeichnis des Git-Repositorys ist ein
    anderer Aufbau (Postgres als eigener Dienst, ein mitgelieferter Caddy, eine
    erforderliche `.env`). Verwenden Sie die Datei auf dieser Seite. Der andere
    Aufbau ist unter [Erweiterte Bereitstellungen](operations/advanced-deployments.md)
    beschrieben.

## Voraussetzungen

- Docker mit Compose v2 (`docker compose`, nicht das ältere `docker-compose`).
- Ein dauerhafter Datenträger für Eveys Daten (die Compose-Datei unten legt sie
  in `./evey-data` neben der Datei ab).
- Ein x86-64-Host, oder ARM64 mit einem Release-Tag: siehe
  [ARM-Hosts](operations/advanced-deployments.md#arm-hosts).

Evey liefert einfaches HTTP aus. Um ihn von außerhalb Ihres Zuhauses über HTTPS
zu erreichen, stellen Sie später einen Reverse Proxy davor: siehe
[Fernzugriff und HTTPS](administration/remote-access.md).

## Die Compose-Datei

Speichern Sie dies als `docker-compose.yml` und ersetzen Sie den Medienpfad in
der letzten Volume-Zeile.

```yaml title="docker-compose.yml"
services:
  evey:
    image: eveyproject/evey:latest # (1)!
    container_name: evey-server
    ports:
      - "9091:9091" # (2)!
    volumes:
      - ./evey-data:/data # (3)!
      - evey-cache:/cache # (4)!
      - /path/to/your/media:/media # (5)!
    stop_grace_period: 60s # (6)!
    restart: unless-stopped

volumes:
  evey-cache:
```

1. `latest` ist die neueste stabile Version. Legen Sie ein Tag fest, sobald Sie Evey
   ernsthaft betreiben, damit zwei Pulls im Abstand eines Tages denselben Code
   ausführen: siehe [Sicherungen und Updates](backups.md#updating-evey).
2. Die API und die Weboberfläche, auf einem Port, in einfachem HTTP.
3. Alles, was Evey gehört: Datenbank, Signaturschlüssel, erzeugte Passwörter,
   Suchindex, Vorschaubilder der Zeitleiste, Bilder, Logs, Sicherungen, Plugins. Dieser Ordner muss gesichert werden.
4. Transkodierungs-Cache: Segmente, verkleinerte Fotos und Cover. Er kann
   jederzeit gelöscht werden und füllt sich bei Bedarf neu.
5. Ihre Medien. Absichtlich mit Schreibzugriff eingebunden: Die Reparaturen
   unter [Medienzustand](runbooks/media-health.md) schreiben Dateien an Ort
   und Stelle neu. Hängen Sie `:ro` an, um eine schreibgeschützte Bibliothek zu
   erhalten; diese Reparaturen schlagen dann fehl. Weitere Ordner werden als
   `/media2`, `/media3`... eingebunden.
6. Gibt der eingebetteten Datenbank Zeit, bei `docker stop` sauber
   herunterzufahren. Dockers Standard von 10 Sekunden ist für einen großen
   Katalog knapp.

## Erster Start

```bash
docker compose up -d
docker compose logs -f evey
```

Beim ersten Start erzeugt der Server seinen Signaturschlüssel und die
Passwörter in `./evey-data`, legt die Datenbank an und wendet seine Migrationen
an. Nichts muss von Hand vorbereitet werden.

Öffnen Sie dann `http://<server-address>:9091`. Ein Server ohne Administrator
öffnet den Einrichtungsassistenten: weiter mit
[Ersteinrichtung](getting-started/first-setup.md).

## Wo die Daten liegen

| Ort | Enthält | Löschbar? |
| --- | --- | --- |
| `./evey-data` | Datenbank, Signaturschlüssel, erzeugte Passwörter, Suchindex, Vorschaubilder der Zeitleiste, Bilder, Logs, Sicherungen, Plugins | Nein. Geht er verloren, gehen alle Konten, der Wiedergabeverlauf und alle Einstellungen verloren. Nur `meili/` darin darf weg: Der Suchindex wird mit `evey admin reindex-search` aus der Datenbank neu aufgebaut. |
| Volume `evey-cache` | Transkodierte Segmente, verkleinerte Fotos und Cover | Ja. Wird bei Bedarf neu erzeugt. |
| Ihre Medienordner | Ihre Dateien | Evey löscht sie nicht; einen Titel in Evey zu entfernen, entfernt ihn aus der Bibliothek, nicht von der Festplatte. Evey schreibt dort in drei Fällen: bei einer Reparatur aus dem Medienzustand (das Original bleibt als Sicherung erhalten), bei von OpenSubtitles heruntergeladenen Untertiteln (ein Ordner `.subtitles` neben dem Film) und für eine Prüfdatei von wenigen Sekunden, die kontrolliert, dass die Ordnerüberwachung noch Ereignisse empfängt. |

## Häufige Varianten

Jede davon besteht aus ein paar zusätzlichen Zeilen in derselben Datei.

**Ein zweiter Medienordner.** Fügen Sie `- /other/path:/media2` unter
`volumes:` hinzu.

**Dateibesitz.** Standardmäßig übernimmt der Container den Besitzer Ihrer
`/media`-Einbindung, damit Reparaturen schreiben können. Um ihn selbst zu
wählen, setzen Sie `PUID` und `PGID` unter `environment:` (die Ausgabe von
`id -u` und `id -g`). Der Container startet nur als root, um diese Zuordnung
anzuwenden; der Server selbst läuft nie als root.

**Eine Grafikkarte für die Transkodierung.** Reichen Sie das Gerät an den
Container durch: siehe [Hardwarebeschleunigung](administration/hardware-acceleration.md).

**Hinter einem bereits vorhandenen Reverse Proxy.** Siehe
[Fernzugriff und HTTPS](administration/remote-access.md).

??? note "Auf einem NAS (Synology Container Manager, QNAP Container Station)"

    - Legen Sie den Ordner `evey-data` vor dem ersten Start an. Diese
      Oberflächen verweigern eine Bind-Einbindung, deren Ordner nicht existiert,
      anders als die Docker-Kommandozeile. Sonst muss nichts angelegt werden.
    - Setzen Sie `PUID` und `PGID` auf den NAS-Benutzer, dem die Medien gehören
      (`id <user>` per SSH; auf Synology typischerweise um `1026`, Gruppe
      `100`). Eine NAS-Freigabe gehört oft `root`, den die automatische
      Erkennung nie übernimmt.
    - Intel-NAS mit Quick Sync: Fügen Sie die Zeilen `devices:` und
      `group_add:` hinzu, die unter [Hardwarebeschleunigung](administration/hardware-acceleration.md)
      beschrieben sind.

## Was Sie nicht konfigurieren müssen

Transkodierungskapazität, Grafikkarte, Wiedergabeart für jedes Gerät und
Fernzugriff entscheidet der Server: Er vermisst die Maschine im ersten ruhigen
Moment, erkennt die durchgereichte Hardware und öffnet den Fernzugriff, sobald
Sie ihm eine öffentliche Adresse geben. Einstellungen, die sich später ändern
können (Servername, Zeitpläne, Grenzen, API-Schlüssel), liegen im Admin-Bereich,
nicht in der Compose-Datei. Die vorhandenen Umgebungsvariablen stehen in der
[Konfigurationsreferenz](configuration.md).
