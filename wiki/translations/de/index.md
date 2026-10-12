---
source_sha: 7054417ac65e819ee642ffbf7da5dbd0563a120f80044a01e2f2a3ed12770dc0
reviewed: true
hide:
  - navigation
  - toc
---

# Evey

Evey ist ein Medienserver, den Sie zu Hause betreiben. Er bewahrt Ihre Filme,
Serien, Musik und Fotos auf, und alle im Haushalt schauen sie auf ihren
eigenen Geräten. Eine Qualität wählen Sie nie: Sie drücken auf Abspielen, den
Rest erledigt Evey.

[Evey installieren](installation.md){ .md-button .md-button--primary }
[Demo ausprobieren](https://demo.eveyproject.org){ .md-button }

<p class="evey-status">
<span>Beta, noch keine nummerierte Version</span>
<span>Freie Software, AGPL-3.0</span>
<span>Demo: meldet Sie selbst an (<code>demo</code> / <code>demo</code>)</span>
</p>

<p class="evey-platforms">
<strong>Was Sie brauchen:</strong> einen x86-64-Computer oder ein NAS mit
Docker, und einen Ordner mit Ihren Medien. Die Installation besteht aus einer
Datei und einem Befehl.<br>
<strong>Schauen</strong> Sie im Webbrowser, auf Android-Handys und -Tablets,
auf Android TV und auf einem Chromecast. Die Apps für Android und Android TV
sind in der Testphase und noch nicht bei Google Play.
</p>

<div class="grid cards" markdown>

-   **Sie betreiben den Server**

    Installieren Sie Evey auf Ihrem Computer oder NAS, fügen Sie Ihre Medien
    hinzu, laden Sie Ihren Haushalt ein.
    [Mit der Installation beginnen](installation.md)

-   **Jemand hat Ihnen eine Adresse gegeben**

    Jemand bei Ihnen zu Hause betreibt Evey schon, und Sie möchten einfach
    schauen.
    [Weiter zu „Mit Evey schauen“](getting-started/watching.md)

</div>

<div class="evey-showcase" markdown>

<figure class="evey-shot" markdown="span">
  ![Die Startseite](images/showcase-home.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>Die Startseite: die Titel zum Fortsetzen, dann Ihre Bibliotheken Reihe für Reihe.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Die Seite eines Films](images/showcase-media.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>Die Seite eines Films: Bilder, Altersfreigabe, Bewertungen, Inhalt und Besetzung.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Der Web-Player](images/showcase-player.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>Der Web-Player. In der Leiste: Zusammen schauen, Streamen und das (i), das die Stream-Info öffnet.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Das Admin-Dashboard, während zwei Filme laufen](images/showcase-dashboard.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>Das Dashboard, während zwei Personen schauen: das Bild, das Gerät, Direktwiedergabe oder Remux.</figcaption>
</figure>

</div>

## Für alle im Haushalt

- **Ein Profil pro Person.** Jedes Profil hat seinen eigenen Verlauf, seine
  Fortsetzungspunkte, Favoriten und Sprachen. Meist teilt sich ein Haushalt
  ein Konto, mit einem Profil pro Person.
- **Kinderprofile.** Ein Kinderprofil öffnet sich ohne Code. Wer von dort zu
  einem Erwachsenenprofil wechseln will, braucht eine vierstellige PIN. Ein
  Kinderprofil hat eine Altersgrenze, und der Server schickt nie einen Titel
  darüber, auch nicht als Download.
- **Weiterschauen, wo Sie aufgehört haben**, auf jedem Gerät. Die
  Fortsetzungspunkte liegen auf dem Server, nicht auf dem Handy.
- **Unterwegs.** Die Android-App kann einen Titel herunterladen, um ihn
  30 Tage lang ohne Internet zu schauen.
- **Auf dem Fernseher.** Sie melden einen Android-TV mit der
  **Schnellverbindung** an: Der Fernseher zeigt einen Code, und Sie bestätigen
  ihn auf Ihrem Handy oder Computer. Kein Passwort, das Sie mit der
  Fernbedienung eintippen müssen.
- **Gemeinsam.** Mehrere Personen können denselben Titel im selben Moment
  schauen, jede auf ihrem eigenen Gerät.

Musik und Fotos funktionieren wie Filme und Serien, im Web, auf Android und
auf dem Fernseher.
[Apps verbinden](getting-started/apps.md) ·
[Benutzer, Profile und Kindersicherung](administration/users.md)

## Nichts zu wählen, bevor Sie auf Abspielen drücken

Manche Dateien laufen überall. Andere sind zu schwer für ein altes Handy,
oder sie haben ein Tonformat, das ein Fernseher nicht lesen kann. Jedes Mal,
wenn Sie auf Abspielen drücken, schaut sich der Server die Datei, das Gerät
und seine eigene Auslastung an. Dann wählt er den leichtesten Weg, der
funktioniert:

- **Direktwiedergabe**: Die Datei wird so geschickt, wie sie ist.
- **Remux**: Bild und Ton werden unverändert in ein Dateiformat kopiert, das
  das Gerät annimmt. Das kostet den Server fast nichts. Evey hat dafür ein
  eigenes Werkzeug, **MKVGo**, und nimmt FFmpeg, wenn MKVGo es nicht kann.
- **Transkodierung**: Der Server wandelt nur um, was das Gerät nicht
  abspielen kann, oft nur den Ton. Nur in diesem Fall hat der Prozessor
  richtig zu tun.

Derselbe Film kann also unverändert an einen Fernseher gehen und für ein
älteres Handy umgewandelt werden. Im Player zeigt die **Stream-Info**, welchen
der drei Wege Sie bekommen haben, und warum. **Verwaltung → Dashboard** zeigt
dasselbe für alle, die gerade schauen.

<figure class="evey-shot evey-shot--narrow" markdown="span">
  ![Die Stream-Info im Web-Player](images/player-stream-info.webp){ loading=lazy }
  <figcaption>Die erste Wiedergabe aus dem Dashboard oben, vom Player aus gesehen. Remux: Das Dateiformat ändert sich, Bild und Ton werden kopiert, und die letzte Zeile sagt, warum die Datei nicht so geschickt werden konnte, wie sie ist (Oberfläche auf Englisch).</figcaption>
</figure>

[Wie Evey die Wiedergabeart wählt](playback/how-playback-is-decided.md) ·
[MKVGo in Evey](playback/mkvgo.md)

## Ein Container, nichts einzustellen

Die ganze Installation ist eine Compose-Datei. Es gibt keine `.env` zu
schreiben und keine Einstellung für Arbeitsspeicher oder Prozessor.

- **Alles ist drin.** Die Datenbank und die Suchmaschine laufen im Container.
  Ihre Passwörter werden beim ersten Start erzeugt.
- **Der Server läuft nie als root.** Er läuft als Besitzer Ihres
  Medienordners, wenn er ihn erkennen kann, damit Reparaturen schreiben
  können.
- **Der Server misst sich selbst.** In einem ruhigen Moment stoppt er echte
  Umwandlungen auf dem Prozessor und auf einer Grafikkarte, falls Sie ihm eine
  gegeben haben. Danach weiß er, wie viele Personen er gleichzeitig bedienen
  kann. Wird es knapp, bekommt eine neue Wiedergabe eine niedrigere Qualität,
  oder sie wandert von einer ausgelasteten Grafikkarte auf den Prozessor. Kann
  eine Wiedergabe nicht starten, sagt die Meldung, warum: Der Server ist voll,
  er misst sich noch, oder die Datei ist noch nicht bereit.
- **Der Zugriff von außerhalb Ihres Zuhauses ist aus**, bis Sie dem Server
  eine öffentliche Adresse geben.

Sie können auch Ihre eigene Datenbank oder Suchmaschine verwenden:
[Erweiterte Bereitstellungen](operations/advanced-deployments.md).
[Transkodierung und Serverkapazität](playback/transcoding.md)

## Was Evey mit Ihren Dateien und Ihren Daten macht

**Ihre Dateien.** Wenn Sie einen Titel oder eine Bibliothek entfernen,
verschwindet er aus Evey, nie von der Festplatte. Evey schreibt nur in drei
Fällen in Ihre Medienordner: bei einer Reparatur, um die Sie gebeten haben,
bei einem Untertitel, der über die API geholt wurde, und bei einer kleinen
Testdatei, die prüft, ob Evey neue Dateien noch bemerkt. Binden Sie Ihre
Medien schreibgeschützt ein, dann passiert nichts davon. **Medienzustand**
prüft Ihre MKV- und WebM-Dateien und listet die gefundenen Probleme auf. Er
repariert eine Datei nur, wenn Sie es wollen. Er arbeitet an einer Kopie, die
vollständig geprüft wird, bevor sie die Datei ersetzt, behält standardmäßig
das Original und protokolliert jede Änderung. Was sich nicht reparieren lässt,
wird gemeldet und in Ruhe gelassen.
[Medienzustand](runbooks/media-health.md)

**Ihre Daten.** Das Projekt betreibt keinen Dienst, der sie empfängt: kein
Konto bei uns, keine Nutzungsstatistik, keine Absturzberichte. Die Apps
sprechen mit Ihrem Server und mit nichts anderem, das wir betreiben. Was Ihr
Netzwerk verlässt: Der Server holt Bilder und Beschreibungen von externen
Diensten (The Movie Database, und Quellen für Musik-Cover). Ab und zu fragt er
außerdem nach, ob es eine neuere Version gibt, was Sie abschalten können. Die
Datenbank wird vor jedem Update gesichert, das sie verändert.
[Datenschutzerklärung](privacy.md) · [Sicherungen und Updates](backups.md)

## Plugins laufen isoliert und kontrolliert

Plugins fügen Quellen für Titelinformationen, Benachrichtigungskanäle,
Bereiche auf der Startseite und Einblendungen im Player hinzu. Themes ändern
das Aussehen. Bevor ein Plugin installiert wird, sieht der Administrator, was
die Prüfung des Plugins gefunden hat und welche Berechtigungen es verlangt.
Eine neue Berechtigung muss ausdrücklich angenommen werden. Ein Plugin läuft
als eigener Prozess, unter einem eigenen Benutzer, ohne direkten
Netzwerkzugang. Es wird gestoppt, wenn es seine Grenze für Arbeitsspeicher,
Prozesse oder Speicherplatz überschreitet. Wie streng seine Dateien
abgeschottet sind, hängt vom Rechner ab, und das **Sicherheits-Dashboard**
zeigt den verwendeten Modus. Das Widget eines Plugins erscheint im Web, auf
Android und auf dem Fernseher. Auf dem Fernseher können Sie es sehen, aber
nicht bedienen.
[Plugins](plugins.md) ·
[Sicherheitsmodell und Plugin-Sandbox](SECURITY.md#sandbox-modes)

## Hier geht es weiter

<div class="grid cards" markdown>

-   **[Evey installieren](installation.md)**

    Was Sie brauchen, die Compose-Datei, der erste Start, die Adresse zum
    Öffnen.

-   **[Ersteinrichtung](getting-started/first-setup.md)**

    Der Einrichtungsassistent, Ihre Bibliotheken, die Personen, die schauen
    werden.

-   **[Apps verbinden](getting-started/apps.md)**

    Die Adresse, die Sie auf einem Handy oder Fernseher eintippen, die
    Anmeldung, und was jede Fehlermeldung bedeutet.

-   **[Mit Evey schauen](getting-started/watching.md)**

    Für alle, die schauen: Profile, einen Titel finden, Untertitel, einen
    Film anfragen.

-   **[Verwaltung](administration/libraries.md)**

    Bibliotheken, Benutzer und Kindersicherung, Zugriff von außen,
    Grafikkarte, Sicherungen.

-   **[Fehlerbehebung](operations/troubleshooting.md)**

    Eine Tabelle pro Problem: wo Sie nachsehen, was Sie prüfen, wie Sie es
    beheben.

-   **[Wie Evey die Wiedergabeart wählt](playback/how-playback-is-decided.md)**

    Direktwiedergabe, Remux, Transkodierung, und wie Sie die Stream-Info
    lesen.

-   **[Plugins und API](plugins.md)**

    Plugins und Themes installieren und schreiben, die Server-API.

</div>

<p class="evey-meta">
Diese Dokumentation folgt dem Entwicklungszweig. ·
<a href="https://github.com/Evey-project/evey">Quellcode auf GitHub</a> ·
Sicherheitsmeldungen: security@eveyproject.org ·
<a href="privacy/">Datenschutzerklärung</a>
</p>
