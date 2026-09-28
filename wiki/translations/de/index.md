---
source_sha: 4ee97dc5ef2cdb11b0aa01a535284eaf511ab308d1429dbb9882b9a5838ba8ca
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

**Ihre Filme, Serien, Musik und Fotos, gestreamt von Ihrem eigenen Rechner auf
jeden Bildschirm im Haus - und darüber hinaus.**

Evey ist ein selbst gehosteter Medienserver. Sie zeigen ihm die Ordner, die Sie
schon haben, er erkennt, was darin liegt, versieht es mit Postern,
Zusammenfassungen und Besetzung und spielt es im Browser, auf Android-Handys,
auf Android TV und auf einem Chromecast ab. Nichts verlässt Ihr Zuhause, solange
Sie es nicht veröffentlichen, und niemand sonst entscheidet, was Sie sehen
dürfen.

[Evey entdecken](philosophy.md){ .md-button .md-button--primary }
[Installieren](installation.md){ .md-button }
[Demo ausprobieren](https://demo.eveyproject.org){ .md-button }

Erst einmal herumklicken? Die öffentliche Demo unter
[demo.eveyproject.org](https://demo.eveyproject.org) läuft mit dem echten Server und freien Filmen:
Melden Sie sich mit `demo` / `demo` an. Sie wird regelmäßig zurückgesetzt, und
die Verwaltungsseiten sind sichtbar, aber größtenteils schreibgeschützt.

## Zwei Wege hinein

<div class="grid cards" markdown>

-   __Entdecken__

    ---

    Was Evey sein will, was es denen bringt, die zuschauen, und auf welchen
    Geräten es läuft. Kein Terminal nötig.

    [Warum Evey](philosophy.md) · [Was Sie bekommen](discover/features.md) ·
    [Auf jedem Bildschirm](discover/apps.md)

-   __Technisches Handbuch__

    ---

    Den Server installieren, die ersten zehn Minuten nach dem Start, jede
    Einstellung, die Zuschauer oder Administratoren ändern können, und was zu
    tun ist, wenn etwas schiefgeht.

    [Installation](installation.md) · [Erste Schritte](guide/first-steps.md) ·
    [Administration](guide/admin-settings.md)

</div>

## Warum man es betreibt

- **Es spielt die Datei, die Sie haben.** Kann ein Gerät eine Datei so lesen,
  wie sie ist, schickt Evey sie unverändert: volle Qualität, keine Arbeit für
  den Server. Kann es das nicht, wandelt Evey nur um, was fehlt - mit der
  Grafikkarte, wenn eine da ist.
- **Es installiert sich mit einer Datei.** Ein Container, eine Compose-Datei,
  kein Passwort zum Ausdenken. Datenbank und Suchmaschine leben darin und
  richten sich selbst ein.
- **Es ist für einen Haushalt gemacht.** Profile für alle, Kinderprofile hinter
  einer PIN, Altersgrenzen, die der Server durchsetzt statt der App, und ein
  Raum, um von verschiedenen Wohnungen aus gemeinsam zu schauen.
- **Man kann es gefahrlos ins Internet stellen.** Der Fernzugriff öffnet sich,
  wenn Sie den Server veröffentlichen, und bleibt sonst zu; die Anmeldung
  scheitert im Zweifel geschlossen, und Erweiterungen laufen in einer Sandbox
  mit nur den Berechtigungen, die sie angegeben haben.
- **Es ist freie Software.** Lizenziert unter
  [AGPL-3.0-or-later](https://www.gnu.org/licenses/agpl-3.0.html). Der Code
  liegt auf [GitHub](https://github.com/Evey-project/evey), und dort steht
  auch, wovor der Server Sie noch nicht schützen kann.

## Schnellzugriff

| Sie möchten | Lesen Sie |
| --- | --- |
| Es auf Ihrem Rechner ausprobieren | [Installation](installation.md) |
| Es mit HTTPS ins Internet stellen | [Reverse Proxy](reverse-proxy.md) |
| Ihre Grafikkarte für die Umwandlung nutzen | [Transkodierung und Hardware](transcoding.md) |
| Wissen, was jede Einstellung tut | [Einstellungen für Zuschauer](guide/viewer-settings.md) · [Administration](guide/admin-settings.md) |
| Einen Server nach einem Absturz zurückholen | [Wiederherstellung](runbooks/recovery.md) |
| Eine Erweiterung schreiben | [Erweiterungen](plugins.md) |

Sicherheitsmeldungen gehen an `security@eveyproject.org` - siehe
[Sandbox und Bedrohungsmodell für Erweiterungen](SECURITY.md).
