---
source_sha: 2bab08ea37b694ae604bd4a44524da605fe82d30c8a8999099eafb428e15f45e
reviewed: true
---
# Installation { #install }

Diese Seite ist für die Person, die Evey für den Haushalt betreiben wird. Sie
brauchen einen Computer oder ein NAS, das eingeschaltet bleibt, mit Docker.
Es sind drei Schritte:

1. Eine Datei speichern, die Compose-Datei.
2. Evey mit einem Befehl starten.
3. Evey im Browser öffnen.

Danach übernimmt der Assistent der [Ersteinrichtung](getting-started/first-setup.md).

Evey läuft in einem einzigen Container. Die Datenbank und die Suchmaschine
sind darin enthalten. Sie müssen also nichts anderes installieren, keine
`.env`-Datei schreiben und nichts an Arbeitsspeicher oder Prozessor
einstellen.

!!! warning "Nehmen Sie die Datei von dieser Seite, nicht die aus dem Git-Repository"

    Die `docker-compose.yml` im Hauptordner des Git-Repositorys ist ein
    anderer Aufbau, für Entwickler und fortgeschrittene Installationen: Sie
    braucht eine `.env`-Datei und startet zusätzliche Dienste. Sie ist unter
    [Erweiterte Bereitstellungen](operations/advanced-deployments.md)
    beschrieben.

## Was Sie brauchen { #what-you-need }

- **Einen Computer oder ein NAS, das eingeschaltet bleibt**, mit einem
  x86-64-Prozessor (Intel oder AMD). ARM64-Geräte brauchen ein
  Versions-Tag, das es noch nicht gibt: siehe
  [ARM-Rechner](operations/advanced-deployments.md#arm-hosts).
- **Docker mit Compose v2.** Der Befehl heißt `docker compose`, mit
  Leerzeichen, nicht das ältere `docker-compose`.
- **Ihren Medienordner**: dort, wo Ihre Filme, Serien, Musik oder Fotos
  liegen.
- **Etwas Speicherplatz für Eveys eigene Daten** (Datenbank, Bilder,
  Suchindex). Die Compose-Datei unten legt sie in einem Ordner namens
  `evey-data` neben der Datei ab.

Evey antwortet in Ihrem Heimnetz über einfaches HTTP. Um von unterwegs zu
schauen, braucht es HTTPS, das Sie später hinzufügen können:
[Fernzugriff und HTTPS](administration/remote-access.md).

## Schritt 1: Die Compose-Datei speichern { #the-compose-file }

Legen Sie einen Ordner für Evey an, zum Beispiel `evey`, und speichern Sie
darin diese Datei als `docker-compose.yml`. Ändern Sie eine einzige Zeile:
den Pfad zu Ihren Medien (die mit 5 markierte Zeile). Liegen Ihre Filme zum
Beispiel in `/srv/media`, wird die Zeile zu `- /srv/media:/media`.

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

1. Die Version von Evey. `latest` ist die neueste Version. Um bei einer
   bestimmten Version zu bleiben, nehmen Sie ein Versions-Tag wie `0.1.0`
   oder ein `sha-<commit>`-Tag: siehe
   [Sicherungen und Updates](backups.md#updating-evey).
2. Der Port. Die Zahl links ist die, die Sie im Browser und in den Apps
   eintippen. Behalten Sie `9091`, außer ein anderes Programm nutzt ihn
   schon.
3. Eveys eigene Daten: Datenbank, Schlüssel, erzeugte Passwörter, Suchindex,
   Bilder, Logs, Sicherungen, Plugins. **Diesen Ordner müssen Sie sichern.**
4. Ein Zwischenspeicher für umgewandelte Videos, verkleinerte Fotos und
   Cover. Sie können ihn jederzeit löschen, er füllt sich von selbst wieder.
5. Ihre Medien. Ersetzen Sie `/path/to/your/media` durch den echten Ordner.
   In Evey heißt dieser Ordner `/media`. Evey darf absichtlich hineinschreiben,
   damit der [Medienzustand](runbooks/media-health.md) eine Datei reparieren
   kann, wenn Sie es wollen. Hängen Sie `:ro` an, um ihn schreibgeschützt zu
   machen: Dann kann Evey dort nicht schreiben, und Reparaturen schlagen
   fehl.
6. Gibt der Datenbank Zeit, sauber anzuhalten, wenn der Container stoppt. Die
   10 Sekunden, die Docker sonst lässt, sind für eine große Sammlung knapp.

## Schritt 2: Evey starten { #first-launch }

In einem Terminal, im Ordner mit der Datei:

```bash
docker compose up -d
```

Evey startet im Hintergrund. Um zu sehen, was es tut, führen Sie
`docker compose logs -f evey` aus; mit `Ctrl+C` beenden Sie die Anzeige,
Evey läuft weiter.

Beim ersten Start legt Evey seine Passwörter und seine Datenbank in
`evey-data` an. Sie müssen nichts vorbereiten.

## Schritt 3: Evey im Browser öffnen { #the-address-to-open-next }

Die Adresse ist `http://`, dann die Adresse des Rechners, auf dem Docker
läuft, dann `:9091`:

| Wo Ihr Browser ist | Adresse zum Eintippen |
| --- | --- |
| Auf dem Rechner, auf dem Docker läuft | `http://localhost:9091` |
| Auf einem anderen Computer oder einem Handy zu Hause | `http://` + die IP-Adresse des Rechners, auf dem Docker läuft, + `:9091`, zum Beispiel `http://192.168.1.20:9091` |
| Außerhalb Ihres Zuhauses | Noch nicht. Dafür braucht es HTTPS und eine öffentliche Adresse: siehe [Fernzugriff und HTTPS](administration/remote-access.md). |

Die IP-Adresse des Rechners, auf dem Docker läuft, finden Sie in der Liste
der verbundenen Geräte Ihres Routers. Oder Sie führen `ip a` unter Linux,
`ipconfig` unter Windows, `ifconfig` unter macOS aus. Haben Sie den Port in
Schritt 1 geändert, nehmen Sie statt `9091` die linke Zahl aus der Zeile
`ports:`.

Beim ersten Mal öffnet die Adresse den Einrichtungsassistenten. Weiter geht es
mit der [Ersteinrichtung](getting-started/first-setup.md#1-the-setup-wizard).
Ist der Assistent fertig, öffnet dieselbe Adresse die Anmeldeseite.

!!! tip "Schreiben Sie sich diese Adresse auf"

    Alle im Haushalt werden sie brauchen. Die Apps für Android und Android TV
    fragen auf ihrem ersten Bildschirm genau nach dieser Adresse, mit
    `http://` und dem Port:
    [Apps verbinden](getting-started/apps.md#the-server-address-to-enter).

## Wo Ihre Daten liegen { #where-the-data-lives }

| Ort | Was er enthält | Dürfen Sie ihn löschen? |
| --- | --- | --- |
| `./evey-data` | Datenbank, Schlüssel, erzeugte Passwörter, Suchindex, Vorschaubilder für die Zeitleiste, Bilder, Logs, Sicherungen, Plugins | **Nein.** Wer ihn verliert, verliert alle Konten, den Wiedergabeverlauf und alle Einstellungen. Einzige Ausnahme ist `meili/` darin, der Suchindex: Der Befehl `evey admin reindex-search` baut ihn aus der Datenbank neu auf. |
| Volume `evey-cache` | Umgewandelte Videos, verkleinerte Fotos und Cover | Ja. Er füllt sich von selbst wieder. |
| Ihre Medienordner | Ihre Dateien | Evey löscht sie nie. Wenn Sie einen Titel in Evey entfernen, verschwindet er aus der Bibliothek, nicht von der Festplatte. |

Evey schreibt nur in drei Fällen in Ihre Medienordner:

- bei einer Reparatur, die Sie im **Medienzustand** starten (das Original wird
  als Sicherung behalten);
- bei einem Untertitel, der von OpenSubtitles geladen wurde (gespeichert in
  einem Ordner `.subtitles` neben dem Film);
- bei einer kleinen Testdatei, die gleich wieder gelöscht wird und prüft, ob
  Evey neue Dateien noch bemerkt.

## Häufige Änderungen { #common-variations }

Jede davon braucht nur ein paar Zeilen in derselben Datei.

**Ein zweiter Medienordner.** Fügen Sie `- /other/path:/media2` unter
`volumes:` hinzu. Ein dritter wird `/media3`, und so weiter.

**Ein zweites Evey auf demselben Rechner.** Geben Sie ihm einen anderen
`container_name` und andere Host-Ports, sonst weigert sich `docker compose up`
zu starten (der Name ist schon vergeben).

**Ein anderer Port.** Ändern Sie nur die linke Zahl, zum Beispiel
`"8080:9091"`. Tippen Sie dann `:8080` im Browser und in den Apps.

**Wem die Dateien gehören.** Standardmäßig übernimmt Evey den Besitzer Ihres
Medienordners, damit Reparaturen schreiben können. Wenn Sie das selbst
festlegen wollen, fügen Sie `PUID` und `PGID` unter `environment:` hinzu, mit
den Zahlen, die `id -u` und `id -g` ausgeben. Der Container startet nur als
root, um das anzuwenden; Evey selbst läuft nie als root.

**Eine Grafikkarte zum Umwandeln von Videos.** Geben Sie dem Container Zugriff
auf die Karte: siehe [Hardwarebeschleunigung](administration/hardware-acceleration.md).

**Sie betreiben schon einen Reverse Proxy** (ein Programm, das HTTPS vor Ihre
Dienste setzt): siehe [Fernzugriff und HTTPS](administration/remote-access.md).

??? note "Auf einem NAS (Synology Container Manager, QNAP Container Station)"

    - Legen Sie den Ordner `evey-data` vor dem ersten Start an. Diese
      Oberflächen weigern sich, einen Ordner einzubinden, der nicht
      existiert, anders als die Docker-Kommandozeile. Sonst müssen Sie nichts
      anlegen.
    - Setzen Sie `PUID` und `PGID` auf den NAS-Benutzer, dem die Medien
      gehören (`id <benutzer>` per SSH; bei Synology meist um `1026`, Gruppe
      `100`). Freigaben auf einem NAS gehören oft `root`, und den übernimmt
      Evey nie von selbst.
    - Intel-NAS mit Quick Sync: Fügen Sie die Zeilen `devices:` und
      `group_add:` hinzu, die unter
      [Hardwarebeschleunigung](administration/hardware-acceleration.md)
      beschrieben sind.

## Was Evey selbst einrichtet { #what-evey-sets-up-by-itself }

Das müssen Sie nicht einstellen:

- **Wie viel Video der Server gleichzeitig umwandeln kann.** Er misst den
  Rechner in seinem ersten ruhigen Moment.
- **Die Grafikkarte.** Evey erkennt die Karte, die Sie dem Container gegeben
  haben, und prüft sie mit einer echten Umwandlung.
- **Wie jedes Gerät eine Datei abspielt.** Das wird bei jeder Wiedergabe neu
  entschieden: siehe
  [Wie Evey die Wiedergabeart wählt](playback/how-playback-is-decided.md).
- **Der Zugriff von außerhalb Ihres Zuhauses.** Aus, bis Sie dem Server eine
  öffentliche Adresse geben.

Einstellungen, die Sie später vielleicht ändern (Servername, Zeitpläne,
Grenzen, API-Schlüssel), liegen im Verwaltungsbereich, nicht in der
Compose-Datei. Die wenigen Umgebungsvariablen, die es gibt, stehen in der
[Konfigurationsreferenz](configuration.md).
