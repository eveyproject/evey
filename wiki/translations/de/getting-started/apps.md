---
source_sha: 7f9f3de42904ab3727caa3bb692a72dede00155bd906f98e3f478738e1669309
reviewed: true
---
# Apps verbinden { #connect-the-apps }

Ihr Handy oder Ihr Fernseher fragt nach der Adresse Ihres Evey-Servers? Diese
Seite sagt Ihnen, was Sie eintippen, und was Sie tun, wenn es nicht funktioniert.

Jede App verbindet sich mit Ihrem eigenen Evey-Server bei Ihnen zu Hause. Sie
tippen seine Adresse einmal ein, melden sich an und wählen Ihr Profil. Wo Sie
aufgehört haben, was Sie geschaut haben und Ihre Favoriten liegen auf dem
Server. Sie stehen Ihnen also auf jedem Ihrer Geräte zur Verfügung.

- **Webbrowser**: nichts zu installieren. Öffnen Sie die Adresse des Servers.
  Der Verwaltungsbereich ist auch dort, für Administratoren.
- **Android** (Handy, Tablet) und **Android TV**: die Evey-Apps, bei
  [Google Play](https://play.google.com/store/apps/details?id=com.evey.app) veröffentlicht.
- **Der Webbrowser eines Fernsehers** (LG, Samsung und andere): nichts zu
  installieren. Öffnen Sie die Adresse des Servers, gefolgt von `/tv`: siehe
  [Im Webbrowser eines Fernsehers](#on-a-televisions-web-browser).
- **Chromecast**: keine App. Sie streamen aus dem Web-Player in Chrome oder
  aus der Android-App.

## Die Serveradresse eingeben { #the-server-address-to-enter }

**Fragen Sie die Person, die Evey betreibt, nach der Adresse.** Sie sieht so
aus:

```text
http://192.168.1.20:9091
```

Dann, in der App für Android oder Android TV:

1. Der erste Bildschirm zeigt **Mit Server verbinden**. Tippen Sie die
   Adresse in das Feld **Server-URL**, genau so, wie Sie sie bekommen haben:
   mit `http://` am Anfang und `:9091` (oder einer anderen Zahl) am Ende.
2. Drücken Sie auf **Verbinden**.
3. Melden Sie sich mit Benutzername und Passwort an. Auf einem Fernseher
   können Sie stattdessen die
   [Schnellverbindung](#quick-connect-signing-in-a-tv-without-typing-a-password)
   nutzen.
4. Wählen Sie Ihr Profil.

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![Die Android-App fragt nach der Serveradresse](../images/app-android-connect.webp){ loading=lazy data-gallery="connect" }
  <figcaption>Die Android-App: die Adresse des Servers zu Hause, mit http:// und dem Port (Oberfläche auf Englisch).</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Die Android-TV-App fragt nach der Serveradresse](../images/app-tv-connect.webp){ loading=lazy data-gallery="connect" }
  <figcaption>Die Android-TV-App: dieselbe Adresse, mit der Fernbedienung eingetippt oder vom Scan gefunden.</figcaption>
</figure>

</div>

Die Adresse hängt davon ab, wo Sie sind:

| Wo Sie sind | Was Sie eintippen |
| --- | --- |
| Zu Hause, im selben WLAN wie der Server | Die Adresse zu Hause, zum Beispiel `http://192.168.1.20:9091`. Ein Name geht auch, wenn Ihr Router ihn kennt, zum Beispiel `http://nas:9091`. |
| Unterwegs | Die öffentliche Adresse, die mit `https://` beginnt und meist keinen Port hat, zum Beispiel `https://evey.example.org`. Es gibt sie nur, wenn die Person, die Evey betreibt, sie eingerichtet hat. |

Tippen Sie nach dem Port nichts mehr ein: kein `/api`, keinen anderen Pfad.
Ein `/` am Ende wird für Sie entfernt.

??? info "Für die Person, die den Server betreibt: woher die Adresse kommt"

    - **Die IP** ist die des Computers oder NAS, auf dem Evey läuft, in Ihrem
      Heimnetz. Sie finden sie in der Geräteliste Ihres Routers oder auf dem
      Rechner selbst (`ip a` unter Linux, `ipconfig` unter Windows,
      `ifconfig` unter macOS). Es ist nie die IP des Handys.
    - **Der Port** ist die linke Zahl in der Zeile `ports:` Ihrer
      Compose-Datei: `9091` mit der Datei aus der
      [Installation](../installation.md#the-compose-file).
    - **`http://`** ist zu Hause normal. `https://` gilt nur für die
      öffentliche Adresse, eingestellt unter **Verwaltung → Fernzugriff →
      Externe URL**: siehe
      [Fernzugriff und HTTPS](../administration/remote-access.md).

Solange kein Server gespeichert ist, erklärt der Verbindungsbildschirm auch,
was gefragt ist (**Evey läuft auf einem Server, den jemand für Sie
eingerichtet hat. Geben Sie dessen Webadresse ein, dieselbe, die Sie im
Browser öffnen.**), und bietet **Wo finde ich die Adresse meines Servers?**
an, das zu dieser Seite führt. Das Handy öffnet sie im Browser; der Fernseher
zeigt die Adresse der Seite an, damit Sie sie auf einem anderen Gerät öffnen.

### Lokales Netzwerk scannen: was es findet und was nicht { #scan-local-network-what-it-finds-and-what-it-does-not }

**Lokales Netzwerk scannen**, unter dem Feld, sucht für Sie nach einem
Evey-Server zu Hause. Das kann bis zu etwa zwei Minuten dauern. Jeder
gefundene Server erscheint mit Name, Adresse und Version. Tippen Sie auf
einen, um das Feld auszufüllen, und drücken Sie dann auf **Verbinden**. Wird
nichts gefunden, zeigt der Bildschirm **Kein Server in diesem Netzwerk
gefunden - Adresse unten eingeben**: Tippen Sie die Adresse selbst ein.

Der Scan findet den Server nicht, wenn das Handy mobile Daten nutzt, in einem
Gäste-WLAN ist oder in einem anderen Netz als der Server.

??? info "Was der Scan genau ausprobiert"

    Er probiert `http://` auf jeder Adresse von vier üblichen Heimnetzen aus
    (`192.168.1.x`, `192.168.0.x`, `10.0.0.x` und `172.16.0.x`, Hosts 1 bis
    254), auf den Ports `9091`, `8096` und `8920`, fünfzig auf einmal, mit
    je 2 Sekunden Wartezeit. Er findet also auch keinen Server in einem
    anderen Adressbereich (`192.168.2.x`, `10.1.x.x`...), auf einem anderen
    Port oder einen, der nur über HTTPS erreichbar ist. Die Apps nutzen weder
    mDNS noch Bonjour und lesen den Adressbereich des Handys nicht aus: Diese
    vier Bereiche sind fest.

### Wenn die Verbindung fehlschlägt { #when-the-connection-fails }

Jeder Fehler öffnet ein Fenster mit dem Titel **Verbindung fehlgeschlagen**.
Suchen Sie die Meldung in dieser Tabelle (einige Meldungen der Android-App
erscheinen noch auf Englisch):

| Meldung (Android) | Was sie bedeutet | Was Sie tun |
| --- | --- | --- |
| **Beginnen Sie die Adresse mit http:// oder https://.** | Der Adresse fehlt `http://`. | Tippen Sie `http://` vor die Adresse. |
| **Diese Verbindung ist nicht verschlüsselt** | Sie haben `http://` für eine Adresse außerhalb Ihres Zuhauses eingetippt. Ihr Passwort und das Video würden ungeschützt durchs Internet gehen. | Drücken Sie auf **Abbrechen** und nehmen Sie die öffentliche Adresse mit `https://`. **Trotzdem verbinden** ist da, wenn Sie wissen, was Sie tun. |
| **No response from ... within 8s.** | Unter dieser Adresse antwortet nichts. | Prüfen Sie, ob der Server läuft, ob Adresse und Port stimmen und ob das Handy im selben WLAN ist. |
| **Couldn't reach ...** | Falsche Adresse oder falscher Port, `http` und `https` verwechselt, oder ein anderes Netz. | Dieselben Prüfungen. Zu Hause versuchen Sie `http://` statt `https://`; bei einer öffentlichen Adresse umgekehrt. |
| **TLS/certificate problem reaching ...** | Das HTTPS-Zertifikat des Servers wird nicht akzeptiert. | Zu Hause nehmen Sie `http://`. Für eine öffentliche Adresse braucht der Server ein richtiges Zertifikat. |
| **... answered "HTTP 404 ..." instead of a healthy Evey server.** | Etwas antwortet, aber nicht Evey: Hinter dem Port steht ein Pfad, oder ein anderes Programm nutzt diesen Port. | Entfernen Sie alles nach dem Port und prüfen Sie die Portnummer. |
| **... hat geantwortet, ist aber kein Evey-Server.** | Eine andere Seite hat geantwortet: eine WLAN-Anmeldeseite, die Seite Ihres Internetanbieters oder ein anderes Programm. | Prüfen Sie die Adresse. In einem öffentlichen WLAN melden Sie sich zuerst auf dessen Anmeldeseite an. |
| **Fernzugriff deaktiviert** | Sie sind unterwegs, und der Server nimmt keine Verbindungen von außen an. | Fragen Sie die Person, die Evey betreibt. Sie schaltet es unter **Verwaltung → Fernzugriff** ein: [Fernzugriff und HTTPS](../administration/remote-access.md). |

Auf Android TV sind die Meldungen kürzer: **Server returned 404**, wenn etwas
anderes als Evey antwortet, und die unveränderte Fehlermeldung des Netzwerks,
wenn nichts antwortet. Ursachen und Prüfungen sind dieselben. Auch der Fernseher braucht
das `http://` (ein bloßes `192.168.1.20:9091` schlägt fehl), er warnt aber
nicht vor `http://` bei einer öffentlichen Adresse.

Antwortet der Server nicht, wenn Sie die App später öffnen, kehrt die App zu
diesem Bildschirm zurück und zeigt den gespeicherten Server an. Auf dem
Fernseher steht zusätzlich **Ihr Server hat nicht geantwortet. Vielleicht
schläft er oder ist nicht im Netzwerk.** Sobald der Server wieder läuft,
wählen Sie ihn erneut.

### Später den Server wechseln oder einen hinzufügen { #changing-or-adding-a-server-later }

Beide Apps können sich mehrere Server merken. Der Verbindungsbildschirm
listet sie unter **Ihre Server** auf, der aktuell genutzte ist mit
**Aktuell** (Android) oder **Aktiv** (Android TV) markiert, und darunter
steht **Weiteren Server hinzufügen**. Tippen Sie auf einen gespeicherten
Server, um sich wieder zu verbinden, ohne die Adresse neu einzutippen. Jeder
Server behält seine eigene Anmeldung.

- **Android.** Auf dem Verbindungsbildschirm hat jeder gespeicherte Server
  eine Schaltfläche zum Löschen (Sie bestätigen, und seine Anmeldung wird
  gelöscht). Der Anmeldebildschirm hat **Server wechseln**. Nach der
  Anmeldung öffnet **Einstellungen → Server** den Bereich **Server
  verwalten**: Tippen Sie auf einen Server, um zu ihm zu wechseln, oder
  nutzen Sie **Umbenennen** oder **Löschen**. Den einzigen gespeicherten
  Server können Sie nicht löschen.
- **Android TV.** **Einstellungen → Server wechseln** meldet den Fernseher ab
  und kehrt zum Verbindungsbildschirm zurück; der Anmeldebildschirm hat
  dieselbe Schaltfläche. Auf dem Verbindungsbildschirm benennt **Server
  verwalten** einen gespeicherten Server um oder löscht ihn.

## Im Webbrowser eines Fernsehers { #on-a-televisions-web-browser }

Ein Fernseher ohne Android TV kann Evey trotzdem in seinem eigenen
Webbrowser abspielen. Der Server bietet dafür eine eigene Seite, die mit der
Fernbedienung bedient wird: Öffnen Sie die Adresse des Servers, gefolgt von
`/tv`, zum Beispiel:

```text
http://192.168.1.20:9091/tv
```

- **Keine Serveradresse einzugeben.** Die Seite gehört zu dem Server, von
  dem aus sie geöffnet wurde. Melden Sie sich mit der
  [Schnellverbindung](#quick-connect-signing-in-a-tv-without-typing-a-password)
  oder Ihrem Passwort an und wählen Sie dann Ihr Profil.
- **Fernsehoberfläche.** Wenn die Seite diese Frage stellt, wählen Sie
  **Hier bleiben**. **Web-Version öffnen** ist für einen Computer gedacht,
  und der Browser merkt sich die Antwort. Wurde sie versehentlich gewählt,
  öffnen Sie die Adresse mit `?client=tv` am Ende (`http://192.168.1.20:9091/tv?client=tv`)
  und speichern Sie diese als Lesezeichen.
- **Vollbild.** Wenn der Browser des Fernsehers anbietet, die Seite zum
  Startbildschirm hinzuzufügen, öffnet sie sich danach im Vollbild, wie eine
  App.
- **Die Fernbedienung.** Die Pfeiltasten bewegen die Auswahl, **OK** wählt
  aus, **Zurück** geht zurück. Im Player zeigt **OK** die Leiste an und
  pausiert, links und rechts springen zurück und vor (jeder Druck verlängert
  den Sprung, Gedrückthalten spult mit Vorschaubild), unten öffnet die
  Einstellungen (Audio, Untertitel, Qualität, Geschwindigkeit, Kapitel), und
  **Zurück** verlässt den Player. Die Tasten für Wiedergabe, Pause,
  Zurückspulen und Vorspulen funktionieren, wenn der Fernseher sie an den
  Browser weitergibt.
- **Text zu klein oder zu groß.** **Einstellungen → Größe der Oberfläche**.

## Schnellverbindung: einen Fernseher ohne Passwort anmelden { #quick-connect-signing-in-a-tv-without-typing-a-password }

Ein Passwort mit der Fernbedienung einzutippen ist mühsam. Mit der
Schnellverbindung bestätigen Sie den Fernseher stattdessen auf Ihrem Handy
oder Computer.

1. Wählen Sie auf dem Anmeldebildschirm des Fernsehers **Schnellverbindung**
   (markiert mit **Empfohlen für TV**). Der Fernseher zeigt einen Code aus
   6 Zeichen, einen QR-Code und wie lange der Code gilt.
2. Bestätigen Sie den Code auf einem Gerät, auf dem Sie schon angemeldet
   sind, auf eine dieser Arten:
    - **Im Browser**: **Einstellungen → Schnellverbindung**, den Code in
      **Gerätecode** eintippen, auf **Gerät autorisieren** drücken.
    - **In der Android-App**: **Einstellungen → Schnellverbindung**, dann
      **QR-Code scannen** mit der Kamera, oder den Code eintippen und auf
      **Gerät autorisieren** drücken.
    - **Mit der Kamera des Handys**: den QR-Code scannen. Er öffnet die Seite
      Schnellverbindung des Servers im Browser. Sind Sie dort angemeldet, ist
      der Code schon eingetragen; sonst melden Sie sich zuerst an und tippen
      dann den Code ein.
3. Nach wenigen Sekunden ist der Fernseher mit demselben Konto angemeldet.

Auch der Anmeldebildschirm der Android-App bietet die Schnellverbindung an,
für ein Handy, auf dem Sie Ihr Passwort lieber nicht eintippen.

## Was jede App mitbringt { #what-each-app-adds }

- **Web.** Tastenkürzel im Player (drücken Sie `?`, um sie anzuzeigen),
  Benachrichtigungen im Browser (siehe
  [Benachrichtigungen](../notifications.md)) und der Verwaltungsbereich.
- **Android.**
    - **Downloads**, um ohne Internet zu schauen. Ein heruntergeladener Titel
      bleibt 30 Tage lang abspielbar, ohne den Server zu kontaktieren. Er
      öffnet sich nur für Profile, die ihn sehen dürfen: Ein Kinderprofil
      kann einen Film, den ein anderes Profil heruntergeladen hat, nicht
      öffnen, wenn er über seiner Altersgrenze liegt. Wenn der Administrator
      es einschaltet, bietet die App für einen Film, der zu groß für das
      Handy ist, auch eine kleinere Kopie an, die der Server vorbereitet
      (`downloads.quality_choice`, siehe
      [Auslieferung und Downloads](../environment/playback-delivery.md)).
    - **Streamen** auf einen Chromecast.
- **Android TV.** Ein Bildschirm, der für die Fernbedienung gemacht ist.
- **Browser des Fernsehers (`/tv`).** Dieselbe Art Bildschirm für die
  Fernbedienung, im Browser des Fernsehers, ohne Schaltfläche zum Streamen
  auf einen Chromecast.

**Abspielen auf**, im Web- und im Android-Player, schickt das, was Sie gerade
schauen, an eine andere Evey-App, die geöffnet und mit demselben Konto
angemeldet ist. Die Wiedergabe läuft dort weiter.

## Die Apps aktuell halten { #keeping-the-apps-up-to-date }

Ist Ihre App für Android oder Android TV älter als die Version, die zu Ihrem
Server passt, sagt sie es Ihnen:

- Die Android-App zeigt oben auf dem Bildschirm **Update verfügbar:** und die
  Version (nie über dem Video);
- der Fernseher zeigt es in seiner oberen Leiste.

Tippen Sie darauf, um die Seite der App im Play Store zu öffnen. Der Play
Store aktualisiert Apps nicht immer von selbst (nur wenn das Gerät nicht
benutzt wird und im WLAN ist, und das lässt sich abschalten). Deshalb gibt es
diesen Hinweis. Ist die App aktuell, oder ist der Server älter als diese
Funktion, wird nichts angezeigt. Die Web-App und die Seite `/tv` brauchen nie
ein Update: Der Server liefert sie.

## Welches Gerät was abspielt { #which-device-plays-what }

Jede App meldet dem Server, was ihr Gerät abspielen kann. Für jeden Titel
entscheidet der Server dann, ob er die Datei so schickt, wie sie ist, oder
sie anpasst. Derselbe Film kann also unverändert an einen Fernseher gehen und
für ein älteres Handy umgewandelt werden. Das Feld **Stream-Info** im Player
zeigt, was entschieden wurde und warum: siehe
[Wie Evey die Wiedergabeart wählt](../playback/how-playback-is-decided.md).
Für das Streamen gelten eigene Regeln: siehe
[Streamen auf Chromecast](../playback/casting.md).
