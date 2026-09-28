---
source_sha: 9fca0896aed2edad83b383a5c8dc68a4c1946e29c664cda6b017db17d93c7dc0
---
# Administration

Im Administrationsbereich (Avatar-Menü, dann **Admin**) betreibt ein
Administrator den Server im Alltag. Fast alles wird in der Datenbank
gespeichert: Es übersteht ein Container-Update, und das meiste wirkt sofort.
Der Bereich sagt, wenn eine Einstellung erst nach einem Neustart greift
(`docker compose restart`).

Diese Seite folgt dem Menü des Bereichs und hält bei den Einstellungen an, auf
die es ankommt. Was vor dem Serverstart feststehen muss (öffentliche URL,
vertrauenswürdiger Proxy, Dateibesitz), steht in der
[Konfiguration](../configuration.md).

## Dashboard und System

### Dashboard und Aktuelle Wiedergabe

Das **Dashboard** zeigt die Version, den Rechner und was gerade passiert.
**Aktuelle Wiedergabe** listet jeden laufenden Stream: wer, was, auf welchem
Gerät und wie - **Direktwiedergabe** (die Datei unverändert), **Direkter
Stream** (neu verpackt, Bild unverändert) oder **Transkodierung**
(umgewandelt) - Spur für Spur, mit der Engine und der verwendeten Grafikkarte.
Sie können eine Sitzung beenden oder dem Zuschauer eine Nachricht schicken.

### Fernzugriff

Wer sich von außerhalb Ihres Heimnetzes verbinden darf.

| Einstellung | Was sie tut |
| --- | --- |
| **Fernzugriff aktivieren** | Aus: Wer keine Adresse aus Ihrem lokalen Netz hat, bekommt auf jeder Seite einen Fehler. An: Er kann sich anmelden wie alle anderen. |
| **Externe URL** | Die öffentliche Adresse, wenn sie von der abweicht, mit der der Server gestartet wurde. |
| **LAN-Netzwerke** | Die Adressbereiche, die als „zu Hause“ gelten. Private Bereiche sind vorgegeben; tragen Sie hier den Bereich eines VPN (Tailscale, WireGuard) ein, wenn dessen Nutzer als zu Hause zählen sollen. |
| **Max. Remote-Bitrate** | Eine Bitrate-Obergrenze für Zuschauer außerhalb des Hauses, in kbit/s (0 = keine). Eine private Adresse wird nie gedeckelt. |

Fassen Sie es nicht an, entscheidet der Server beim Start: Fernzugriff ist an,
wenn Sie ihm eine öffentliche Adresse gegeben haben, sonst aus. Ist der Server
aus dem Internet erreichbar, der Fernzugriff aber aus, warnt die Seite in Rot.
Hinter einem Reverse Proxy funktioniert das nur, wenn der Proxy die echte
Client-Adresse weitergibt - siehe
[Reverse Proxy](../reverse-proxy.md#real-client-ips-and-the-remote-access-gate).

Fernzugriff ist eine Entscheidung darüber, wer den Server erreichen darf, kein
Schutz: Die Anmeldung schützt die Bibliothek.

### Serverkonfiguration

Die Seite fasst die eigenen Einstellungen des Servers in Karten zusammen:

| Karte | Die wichtigen Einstellungen |
| --- | --- |
| **Sprache & Region** | Standardsprache der Oberfläche, das Land, dessen Altersfreigaben gelten, und die Sprache von Titeln und Zusammenfassungen. |
| **Startseiten-Feeds** | Welche Reihen die Startseite zeigt, in welcher Reihenfolge und mit wie vielen Einträgen. Erweiterungen können Reihen hinzufügen. |
| **E-Mail / SMTP** | Ausgehende Mails für Einladungen und das Zurücksetzen von Passwörtern. Siehe [Benachrichtigungen](../notifications.md). |
| **Trickplay-Vorschaubilder** | Erzeugt die Vorschaubilder der Zeitleiste. Standardmäßig an. |
| **Zugeteilte Rechenleistung** | Wie viel des Prozessors die Hintergrundarbeit insgesamt nehmen darf: **25 %** (schonend, Standard) oder **50 %** (Schub, wenn niemand schaut). Eine schwere Aufgabe (Analyse neuer Medien, Vorschaubilder, Intro-Erkennung, Komplexitätseinstufung) kann einen eigenen Anteil bekommen. Gilt ab dem nächsten Element. Die Hintergrundarbeit pausiert, während jemand schaut. |
| **Speicherprofil** | **Niedrig** legt untätige Erweiterungen schlafen, um Speicher zu sparen; **Auto** wählt es auf Rechnern mit weniger als 3 GB. |
| **Hintergrundarbeiten** | Pausiert und taktet die Hintergrunddurchläufe (Keyframe-Index, Codec-Level). Wird beim Start gelesen: nach einer Änderung neu starten. |
| **Artwork-Cache** | Behält Poster und Hintergründe auf dem Server, nach WebP umgewandelt, statt sie jedes Mal aus dem Internet zu laden. |
| **Parser-Tokens** | Zusätzliche Release-Kennzeichen und Fassungsnamen, die der Dateinamen-Parser entfernen soll, wenn Ihre Dateien Kennzeichen tragen, die er nicht kennt. |
| **API-Schlüssel** | TMDB (ein geteilter Schlüssel ist eingebaut), OMDb, OpenSubtitles, TheIntroDB. |
| **Intro-Skip-Anbieter** | Nutzt Community-Datenbanken (TheIntroDB, dann IntroDB) für die Zeitmarken von Intro, Rückblick und Abspann, bevor die eigene Erkennung des Servers läuft. Nach dem Umschalten neu starten. Kann die sicheren Erkennungen des Servers auch an TheIntroDB zurückmelden. |
| **Servereinstellungen** | Servername, Anmelde- und globale Ratenbegrenzung, Log-Level, Dateiüberwachung und Scan-Intervall, **Intros automatisch überspringen** (standardmäßig aus: eingeschaltet überspringt jeder Player erkannte Intros von selbst), **Vorschau-Segmente überspringen**, wie lange Verlauf und Statistiken aufbewahrt werden, das **Budget des Transkodierungs-Caches** (standardmäßig 10 GB) und die Obergrenze für AV1 in Software. |
| **Updates** | Prüft alle 6 Stunden, ob ein neueres Image veröffentlicht ist, und sagt es Ihnen. Evey aktualisiert sich nie selbst. |

Unter den Karten listet **Konfigurationseinträge** jede rohe
Schlüssel-Wert-Einstellung, für den seltenen Fall, dass ein dokumentierter
Schlüssel keine eigene Karte hat.

### Transkodierung

Wie der Server Video umwandelt.

- **Encode-Kalibrierung** zeigt, was der Server über sich selbst gemessen hat:
  die höchste Auflösung, die er in Echtzeit umwandeln kann, in Software und mit
  der Grafikkarte, und welche Codecs die Karte beherrscht.
  **Hardware-Beschleunigung** wählt die Karte (**Auto** nimmt die beste
  erkannte, **Aus** erzwingt den Prozessor). **Neu kalibrieren**, nachdem Sie
  eine Grafikkarte eingebaut oder den Container auf einen anderen Rechner
  verlegt haben. Ebenfalls hier: **AV1-Ausgabe** (nur Hardware), Intels
  **Energiespar-Encoder** und eine **Obergrenze Videobitrate**, mit einer
  eigenen für Zuschauer außerhalb des Hauses.
- **Kapazität für laufende Wiedergabe** ist die Zahl der Videos, die gleichzeitig
  umgewandelt werden dürfen. Sie ist gemessen; Sie können sie nur senken. Ein
  Stream, der nur den Ton umwandelt, zählt nicht, und ein Zuschauer, der die
  Qualität wechselt, behält einen einzigen Platz.
- **Transkodier-Aufträge im Hintergrund** begrenzt Umwandlungen in der
  Warteschlange, getrennt von der laufenden Wiedergabe.
- **Erweiterte Grenzwerte** deckelt Sitzungen und Auflösungen auf schwacher
  Hardware. Wird beim Start gelesen.
- **Lasttest** simuliert echte Zuschauer, um zu messen, was der Server trägt.
  Nur starten, wenn niemand schaut.

Eine Grafikkarte in Docker einrichten:
[Transkodierung und Hardware](../transcoding.md).

### Transkodierungs-Cache und Systemzustand

**Transkodierungs-Cache** zeigt, was Umwandlungen auf der Platte hinterlassen
haben, und leert es. Nach einem Update, das ändert, wie Videos umgewandelt
werden, stellt das Leeren sicher, dass niemand eine alte Umwandlung bekommt.
**Systemzustand** zeigt Prozessor, Speicher, Bandbreite, Speicherplatz und die
belegte Umwandlungskapazität.

## Medien

- **Bibliotheken**: hinzufügen, bearbeiten, scannen, planen; **Nur Waisen
  abgleichen**, **Alle erneut mit TMDB abgleichen** und **Reihen reparieren**
  für die Metadaten. Siehe [Erste Schritte](first-steps.md#add-your-libraries).
- **Artwork-Cache**: die auf dem Server behaltenen Poster und Hintergründe.
- **Medienzustand**: Ein Scan nur mit Lesezugriff gibt jeder MKV- und
  WebM-Datei eines von zwei Urteilen. **Reparierbar** (fehlender Sprungindex,
  beschädigte Stelle, eine `.mkv`, die in Wahrheit ein MP4 ist) wird in einem
  Durchlauf mit **Alles reparieren** behoben, und jede reparierte Datei behält
  ihr Original als Sicherung. **Zur Information** (unvollständiger Download,
  Tonversatz...) listet, was sich dort nicht beheben lässt. Reparieren schreibt
  in Ihre Medienordner, die also mit Schreibzugriff eingehängt sein müssen. Mehr
  unter [Medienzustand](../runbooks/media-health.md).

Bei einem Titel kann ein Administrator außerdem die **Metadaten bearbeiten**,
das Poster ändern und die **Mediensegmente** (Intro, Rückblick, Abspann) von
Hand anpassen oder ihre Erkennung neu starten.

## Benutzer und Zugriff

- **Benutzer**: Konten anlegen, bearbeiten, deaktivieren oder löschen; jedes auf
  bestimmte Bibliotheken beschränken; seine Streaming-Bitrate, seine Zahl an
  Profilen und Geräten pro Profil deckeln; seinen Verlauf ansehen; sich **als**
  der Benutzer **anmelden**, um zu sehen, was er sieht (ein Banner erinnert
  daran, **Beenden** bringt Sie zurück).
- **Einladungslinks**: ein Link mit wahlweise Ablaufdatum, Zahl der Nutzungen,
  Bibliotheken, Profilgrenze und Administratorrolle.
- **Medienanfragen**: die Warteschlange der Titel, die Benutzer angefragt haben
  (annehmen, ablehnen oder vertagen, mit Notiz), und wer anfragen darf:
  standardmäßig alle, mit Ausnahmen pro Benutzer.

## Integrationen

- **Plugins**: aus einer ZIP-Datei installieren. Vor der
  Installation zeigt der Bereich die Berechtigungen, die die Erweiterung
  angibt, und was ihre statische Analyse gefunden hat. Pro Erweiterung:
  aktivieren, aktualisieren, Ressourcengrenzen (Speicher, Prozesse,
  Plattenplatz, Priorität), Logs. Siehe [Erweiterungen](../plugins.md).
- **Webhooks**: eine URL von Ihnen aufrufen, wenn etwas passiert (ein Titel
  hinzugefügt, eine Wiedergabe gestartet...), für alle Ereignisse oder einige
  ausgewählte, signiert mit einem Geheimnis, wenn Sie eines angeben. **Test**
  schickt einen Ping.
- **API-Schlüssel**: Schlüssel für Ihre eigenen Skripte und Integrationen, jeder
  auf die Bereiche beschränkt, die Sie ankreuzen (Medien lesen, Wiedergabe
  steuern, Benutzer verwalten...). Ein Schlüssel wird nur einmal gezeigt, beim
  Anlegen. **API-Dokumentation** listet, was jeder Bereich freischaltet, und
  lässt es ausprobieren.

## Überwachung und Wartung

- **Analysen**: Wiedergaben heute und diese Woche, aktive Benutzer, die
  meistgesehenen Titel der letzten 30 Tage.
- **Aktivität**: der Verlauf der Wiedergaben und der Verwaltungsaktionen, und
  die laufende Hintergrundarbeit.
- **Protokolle & Debug**: die Logs des Servers durchsuchen und live verfolgen,
  gefiltert nach Bereich (Wiedergabe, Umwandlung, Scan...). Die erste Anlaufstelle,
  wenn ein Benutzer ein Problem meldet.
- **Streaming-Qualität**: wie die Wiedergabe für echte Zuschauer am letzten Tag
  oder in der letzten Woche lief - wie oft sie auf das Video warten mussten,
  die Titel, die am meisten stocken, und die Gerätearten, die sich schwertun.
- **Sicherung & Wiederherstellung**: eine Sicherung erstellen, herunterladen,
  eine wiederherstellen oder aus einer Datei wiederherstellen. Zeitplan und
  Aufbewahrung stehen unter [Sicherungen](../backups.md).

Wenn der Bereich selbst nicht mehr reagiert, ist dieselbe Wartung über die
Kommandozeile erreichbar: [Kommandozeile](../runbooks/cli.md).
