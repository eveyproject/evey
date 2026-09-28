---
source_sha: f1ea44197021d6c8d4791aa939ef1b103613cd438a70d3c00ce1af151c0ca580
---
# Erste Schritte

Der Container läuft, und Sie haben `http://<ihre-ip>:9091` geöffnet. Diese Seite
führt durch die ersten zehn Minuten: den Einrichtungsassistenten, die
Bibliotheken, die Menschen, und die drei Dinge, die Sie prüfen sollten, bevor
Sie irgendwem die Adresse geben. Läuft der Server noch nicht, beginnen Sie mit
der [Installation](../installation.md).

## Der Einrichtungsassistent

Ein Server ohne Administrator öffnet sich mit einem Assistenten in vier
Schritten.

1. **Sprache.** Die Standardsprache für neue Profile und für Metadaten (Titel,
   Zusammenfassungen, Genres). Sie können sie später unter
   **Admin → Serverkonfiguration → Sprache & Region** ändern, zusammen mit der
   Region, die festlegt, welche Altersfreigaben gelten.
2. **Willkommen.**
3. **Admin-Konto.** Ein Benutzername mit mindestens drei Zeichen und ein
   Passwort mit mindestens acht. Der optionale **Servername** („Wohnzimmer“)
   erscheint auf der Anmeldeseite, in der Serverliste der Apps und in E-Mails.
4. **Erste Bibliothek.** Ein Name und ein Typ. Die Einzelheiten können Sie hier
   überspringen und im Administrationsbereich nachholen.

## Ihre Bibliotheken hinzufügen

**Admin → Bibliotheken → Bibliothek hinzufügen.** Eine Bibliothek hat einen
Namen, einen Typ (Filme, Serien, Musik oder Fotos) und einen oder mehrere
Ordner.

**Die Ordner sind Pfade innerhalb des Containers**, nicht auf Ihrem Rechner. Mit
der Standard-Compose-Datei ist Ihr Medienordner unter `/media` eingehängt: Eine
Filmbibliothek, die auf dem Rechner in `/srv/media/films` liegt, ist für Evey
`/media/films`. Ein zweiter eingehängter Ordner (`/media2`, ...) kommt genauso
hinzu. Die Auswahl zeigt die Ordner, die sie sieht.

Halten Sie Filme und Serien in getrennten Bibliotheken: Der Typ sagt dem
Scanner, wonach er suchen soll. Evey erkennt Namen wie diese:

```text
/media/films/The Mirror (1999)/The Mirror (1999) 1080p.mkv
/media/films/Lantern.2010.1080p.BluRay.x264.mkv
/media/series/The Outpost/Season 01/The.Outpost.S01E05.720p.mkv
/media/series/Quiet Storm/Quiet.Storm.03x10.mkv
```

Bei Filmen hilft das Jahr sehr. Bei Folgen zählt `S01E05` oder `1x05`.
Release-Kennzeichen (Auflösung, Quelle, Codec, Ton, Sprachkennzeichen wie
`VOSTFR` oder `MULTI`, Fassungen wie „Director's Cut“) werden vor der Suche
entfernt. Tragen Ihre Dateien ein Kennzeichen, das der Scanner nicht kennt,
fügen Sie es unter **Admin → Serverkonfiguration → Parser-Tokens** hinzu.

Pro Bibliothek wählen Sie außerdem:

| Option | Was sie tut |
| --- | --- |
| **Echtzeit-Überwachung** | Neue, umbenannte und gelöschte Dateien werden erfasst, sobald sie auftauchen. Eine regelmäßige Prüfung (standardmäßig alle 2 Minuten) fängt ab, was das Dateisystem nicht gemeldet hat, etwa auf einer Netzwerkfreigabe. |
| **Beim Start scannen** | Scannt die Bibliothek bei jedem Serverstart neu. |
| **Zeitplan** | Ein vollständiger Scan alle 6 oder 12 Stunden, täglich, wöchentlich oder nach Ihrem eigenen Cron-Ausdruck. |

Der erste Scan startet von selbst. Poster und Angaben kommen im Hintergrund,
eine große Bibliothek füllt sich also innerhalb weniger Minuten.

**Wenn ein Titel falsch ist oder fehlt.** In der Bibliotheksliste versucht
**Nur Waisen abgleichen** die Titel erneut, die nie erkannt wurden, und **Alle
erneut mit TMDB abgleichen** wiederholt die Suche für jeden Titel (langsam, aber
es korrigiert alte Fehler nach einer großen Umbenennung). Bei einem einzelnen
Titel lässt **Metadaten bearbeiten** den Titel korrigieren oder die richtige
TMDB- oder IMDb-ID einfügen.

## Metadatenquellen

Evey bringt einen Schlüssel für **TMDB** mit, die Quelle für Titel, Poster,
Besetzung und Sammlungen, sodass Metadaten sofort funktionieren. Andere Quellen
sind optional, ihre Schlüssel gehören unter
**Admin → Serverkonfiguration → API-Schlüssel**:

| Schlüssel | Bringt |
| --- | --- |
| TMDB (Ihr eigener) | Ihr eigenes Kontingent statt des geteilten. |
| OMDb | Eine zweite Metadatenquelle, um einen Titel zu ergänzen, den TMDB kaum kennt. |
| OpenSubtitles | Suche und Download von Untertiteln, die eine Datei nicht mitbringt. |
| TheIntroDB | Mehr Zeitmarken für Intro, Rückblick und Abspann (500 Abfragen am Tag statt 100). |

Ein im Bereich gesetzter Schlüssel gewinnt gegen denselben Schlüssel aus der
Umgebung.

## Den Haushalt einladen

**Admin → Benutzer** bietet zwei Wege hinein:

- **Benutzer erstellen**: Sie wählen Benutzername und Passwort.
- **Einladungslink erstellen**: Die Person wählt ihre eigenen. Ein Link kann
  ablaufen, auf eine Zahl von Nutzungen begrenzt sein, nur Zugriff auf
  bestimmte Bibliotheken geben, die Zahl der Profile begrenzen, die das neue
  Konto anlegen darf, und sogar einen Administrator anlegen.

Für jeden Benutzer können Sie später die **Bibliotheken** einschränken, die er
sieht, seine **Streaming-Bitrate** deckeln, seine **Profile** und **Geräte pro
Profil** begrenzen, seinen Verlauf ansehen oder sich **als** er **anmelden**, um
genau zu sehen, was er sieht.

Profile, Kinderprofile und die Jugendschutz-PIN verwaltet jedes Konto selbst:
siehe [Einstellungen für Zuschauer](viewer-settings.md#profiles-and-parental-control).

## Den Server sich selbst vermessen lassen

Beim ersten Start auf einem Rechner und erneut, wenn sich die Hardware ändert,
misst Evey, wie viele Videos es gleichzeitig umwandeln kann, mit dem Prozessor
und mit der Grafikkarte, falls vorhanden. Das dauert etwa zwei Minuten, läuft
nur, wenn niemand schaut, und ein Player, der in dieser Zeit eine Umwandlung
anfordert, wird gebeten, es gleich noch einmal zu versuchen. Das Ergebnis steht
unter **Admin → Transkodierung**: Diese Zahl ist eine Obergrenze, die Sie senken,
aber nie anheben können. Um eine Grafikkarte zu nutzen, siehe
[Transkodierung und Hardware](../transcoding.md).

## Bevor Sie die Adresse teilen

1. **Fernzugriff.** Evey öffnet sich dem Internet nur, wenn Sie ihm eine
   öffentliche Adresse gegeben haben. Prüfen Sie **Admin → Fernzugriff**: Ist der
   Server von außen erreichbar, der Fernzugriff aber aus, sagt die Seite das in
   Rot, und alle außerhalb Ihres Netzes bekommen einen Fehler. Siehe
   [Administration](admin-settings.md#remote-access).
2. **HTTPS.** Evey spricht einfaches HTTP. Stellen Sie es hinter einen Reverse
   Proxy, bevor Sie es veröffentlichen: [Reverse Proxy](../reverse-proxy.md).
3. **Sicherungen.** Der Server sichert seine Datenbank vor jedem Update. Für
   regelmäßige Sicherungen legen Sie einen Zeitplan fest:
   [Sicherungen](../backups.md). Bewahren Sie `./evey-data` gut auf - darin
   liegen die Datenbank und der Schlüssel, der jede Sitzung signiert.

## Die Apps installieren

Öffnen Sie die Android- oder Android-TV-App, geben Sie die Adresse des Servers
ein und melden Sie sich an. Wählen Sie auf einem Fernseher die
**Schnellverbindung**: Der Fernseher zeigt einen Code und einen QR-Code, den Sie
unter **Einstellungen → Schnellverbindung** auf einem Handy oder Computer
bestätigen, auf dem Sie schon angemeldet sind. Mehr dazu unter
[Auf jedem Bildschirm](../discover/apps.md).
