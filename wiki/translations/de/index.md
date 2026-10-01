---
source_sha: 9e2d7867cc55d0c1e56c8b94a38f02164edd7cfdafed58c39880904efaa4e00c
reviewed: false
hide:
  - navigation
  - toc
---

# Evey-Dokumentation

Evey ist ein selbst gehosteter Server für Filme, Serien, Musik und Fotos. Bei
jeder Wiedergabe vergleicht er die Datei, den Player und den Server, behält das
Original, wann immer der Player damit umgehen kann, und kodiert nur neu, was
neu kodiert werden muss. Die meisten Installationen brauchen nicht mehr als
Bibliotheken und Benutzer: Hardware, Kapazität und Wiedergabewege erkennt und
misst der Server selbst.

<div class="evey-showcase" markdown>

<figure class="evey-shot" markdown="span">
  ![Die Startseite](images/showcase-home.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>Die Startseite: Wiedergabe fortsetzen, dann jede Bibliothek.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Eine Filmseite](images/showcase-media.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>Eine Filmseite mit Bildern, Bewertungen und Besetzung.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Der Web-Player](images/showcase-player.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>Der Web-Player.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Das Admin-Dashboard während zwei Filme laufen](images/showcase-dashboard.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>Das Admin-Dashboard, während zwei Personen schauen: das Bild, das jede sieht, und wie es ausgeliefert wird.</figcaption>
</figure>

</div>

## Mehr mit weniger

<div class="evey-points" markdown>

<div markdown>

**Der günstigste Weg zuerst**

Die Datei, wie sie ist, dann ohne Encoder neu verpackt, dann nur die Spuren
neu kodiert, die es brauchen. Ein Zuschauer in Direktwiedergabe oder Remux
kostet den Server kaum mehr als das Lesen der Datei, so bedient eine
bescheidene Maschine viele gleichzeitig.
[Wie Evey die Wiedergabeart wählt](playback/how-playback-is-decided.md)

</div>

<div markdown>

**Ein Server, der sich selbst misst**

Er misst echte 4K-Kodierungen auf dem Prozessor und jeder Grafikkarte und
nutzt eine Karte nur für das, was sie bewiesen hat. Keine Kapazität zu
schätzen.
[Transkodierung und Serverkapazität](playback/transcoding.md)

</div>

<div markdown>

**Voll heißt abgelehnt, nicht langsamer**

Nicht mehr Kodierungen als gemessen; ist er voll, erfährt der Player, dass der
Server beschäftigt ist, statt dass jeder Stream schlechter wird.
Hintergrundaufgaben pausieren während der Wiedergabe.
[Hintergrundaufgaben und Serverlast](operations/background-work.md)

</div>

<div markdown>

**Ein Plugin, drei Bildschirme**

Dasselbe Plugin-Paket läuft im Web, auf Android und auf Android TV und weiß,
auf welchem Bildschirm es gelandet ist.
[Plugin-API-Referenz](plugins/api-reference.md#on-a-television)

</div>

<div markdown>

**Plugins werden eingesperrt, nicht für vertrauenswürdig gehalten**

Code vor der Installation analysiert, nur das eigene Verzeichnis, gefährliche
Systemaufrufe verweigert, kein direktes Netzwerk, ein standardmäßig
verweigertes Token.
[Sicherheitsmodell und Plugin-Sandbox](SECURITY.md)

</div>

<div markdown>

**Was Stream-Info sagt, läuft auch**

Eine Entscheidung pro Wiedergabe, und Stream-Info liest sie aus dem Plan, den
der Encoder ausführt, samt dem Grund, warum die Datei nicht so abgespielt
wurde, wie sie ist.
[Wie Evey die Wiedergabeart wählt](playback/how-playback-is-decided.md#reading-stream-info)

</div>

</div>

## Wegweiser

<div class="grid cards" markdown>

-   **[Evey installieren](installation.md)**

    Ein Container und eine Compose-Datei. Voraussetzungen und erster Start.

-   **[Ersteinrichtung](getting-started/first-setup.md)**

    Der Einrichtungsassistent, Ihre Bibliotheken, die Personen, die den Server
    nutzen werden.

-   **[Verwaltung](administration/libraries.md)**

    Bibliotheken, Benutzer und Profile, Fernzugriff, Hardwarebeschleunigung,
    Sicherungen.

-   **[Wie Evey die Wiedergabeart wählt](playback/how-playback-is-decided.md)**

    Direktwiedergabe, Remux und Transkodierung, und wie man Stream-Info liest.

-   **[Fehlerbehebung](operations/troubleshooting.md)**

    Wo man nachsieht, wenn etwas nicht funktioniert: Symptome, Logs,
    Wiederherstellung.

-   **[Plugins und API](plugins.md)**

    Plugins und Themes installieren und schreiben, die Server-API.

</div>

<p class="evey-meta">
Evey ist in der Beta und hat noch keine nummerierte Version: Diese
Dokumentation folgt dem Entwicklungszweig. ·
<a href="https://github.com/Evey-project/evey">Quellcode auf GitHub</a> ·
<a href="https://demo.eveyproject.org">Öffentliche Demo</a> (Anmeldung mit demo / demo) ·
AGPL-3.0-or-later ·
Sicherheitsmeldungen: security@eveyproject.org
</p>
