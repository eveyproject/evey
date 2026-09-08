---
source_sha: aedd9ba6244ba2dbc7e348c929241546a233301b93c88350c78c524424a8728f
hide:
- navigation
---

<pre class="evey-wordmark" aria-hidden="true"><span>███████╗</span><span class="v">██╗   ██╗</span><span>███████╗██╗   ██╗</span>
<span>██╔════╝</span><span class="v">██║   ██║</span><span>██╔════╝╚██╗ ██╔╝</span>
<span>█████╗  </span><span class="v">██║   ██║</span><span>█████╗   ╚████╔╝ </span>
<span>██╔══╝  </span><span class="v">╚██╗ ██╔╝</span><span>██╔══╝    ╚██╔╝  </span>
<span>███████╗</span><span class="v"> ╚████╔╝ </span><span>███████╗   ██║   </span>
<span>╚══════╝</span><span class="v">  ╚═══╝  </span><span>╚══════╝   ╚═╝   </span></pre>

# Evey

Selbst gehosteter Medienserver für Filme, Serien, Musik und Fotos. Ein Container,
Ihre eigene Hardware, Ihre eigenen Dateien.

[Evey installieren](installation.md){ .md-button .md-button--primary }
[Quellcode ansehen](https://github.com/Evey-project/evey){ .md-button }

## Hier anfangen

<div class="grid cards" markdown>

-   __Installieren__

    ---

    Eine einzige Compose-Datei, kein Geheimnis zum Ausdenken. Postgres und der
    Suchindex laufen im Container und erzeugen ihre Zugangsdaten beim ersten
    Start selbst.

    [Installationsanleitung](installation.md)

-   __Konfigurieren__

    ---

    Alles, was sich einstellen lässt, und wo es liegt. Fast alles davon steht im
    Administrationsbereich statt in Umgebungsvariablen.

    [Konfigurationsreferenz](configuration.md)

-   __Hinter einen Proxy stellen__

    ---

    Caddy, nginx, Traefik und Apache, mit den Headern, die Streaming und
    WebSocket-Wiedergabe tatsächlich brauchen.

    [Reverse-Proxy-Anleitung](reverse-proxy.md)

-   __Daten absichern__

    ---

    Was der Server von sich aus sichert, was nicht, und wie Sie eine Datenbank
    wiederherstellen, die nicht mehr startet.

    [Sicherungen](backups.md) · [Wiederherstellung](runbooks/recovery.md)

</div>

## Was er kann

- **Er spielt, was Ihnen gehört.** Direkte Wiedergabe, sobald der Client die
  Datei unverändert öffnen kann, Transkodierung in Hardware oder Software, wenn
  nicht, und ein HLS-Stream, der Tonspur und Qualität mitten im Film wechselt.
- **Er folgt Ihnen von Gerät zu Gerät.** Web, Android, Android TV und
  Chromecast, mit Wiedergabepositionen, Gesehen-Status und Downloads für die
  Wiedergabe ohne Netz.
- **Er ist für einen Haushalt gemacht.** Profile mit eigenem Design und eigener
  Sprache, serverseitig durchgesetzte Kindersicherung und gemeinsames Schauen
  mit SyncPlay.
- **Er lässt sich ohne Neubau erweitern.** Erweiterungen laufen abgeschottet,
  mit eng gefassten Tokens und standardmäßig verweigerten Rechten, und Themes
  sind Daten statt Code.

## Mitarbeiten

Die Seite [Architektur](ARCHITECTURE.md) erklärt den Aufbau des Servers, und die
[Architekturentscheidungen](adr/README.md) halten fest, warum jede strukturelle
Entscheidung so getroffen wurde. Wer Erweiterungen schreibt, beginnt bei der
[Einrichtung für Erweiterungen](plugins/dev-setup.md).

Evey steht unter der [AGPL-3.0-or-later](https://www.gnu.org/licenses/agpl-3.0.html).
Sicherheitsmeldungen gehen an `security@eveyproject.org` - siehe
[Bedrohungsmodell](SECURITY.md).
