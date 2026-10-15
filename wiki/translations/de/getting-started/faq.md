---
source_sha: 970d79357da787e635d08003e577bd5a9a2aa1600f1dd6722628b4291f0dea6f
reviewed: true
---

# Häufige Fragen

Kurze Antworten auf die Fragen, die der Person gestellt werden, die Evey zu
Hause betreibt. Jede sagt zuerst, was zu tun ist, und verweist dann auf die
Seite, die erklärt, warum, für den Tag, an dem Sie es verstehen wollen.

## Warum dauert es, bis der Film startet?

Meist, weil der Server ihn anpasst. Ihr Fernseher oder Ihr Telefon kann die
Datei nicht genau so abspielen, wie sie gespeichert ist, also wandelt der
Server sie um, während Sie schauen, und die ersten Sekunden gehen in diese
Vorbereitung. Ein Film, der fast sofort startet, wird so gesendet, wie er ist.

Öffnen Sie **Stream-Info** im Player (die Taste (i)): Sie zeigt, welcher der
beiden Fälle vorliegt, Spur für Spur, und warum. Ist ein einzelner Titel
jedes Mal langsam, während die anderen normal laufen, liegt es wahrscheinlich an der
Datei selbst, und die Person, die Evey betreibt, kann das unter **Verwaltung →
Medienzustand** prüfen.

Mehr: [Wie die Wiedergabe entschieden wird](../playback/how-playback-is-decided.md#reading-stream-info)
und, für die Person, die den Server betreibt, [Medienzustand](../runbooks/media-health.md).

## Warum ist das Bild auf meinem Telefon unterwegs weniger scharf?

Weil für Zuschauer außerhalb des Hauses eine Obergrenze gilt. Die Person, die
Evey betreibt, kann unter **Verwaltung → Transkodierung** zwei Grenzen setzen:
eine für alle und eine niedrigere für Zuschauer außerhalb des lokalen Netzes,
damit ein Film über eine Mobilfunkverbindung weiterläuft. Jedes Konto kann
außerdem eine eigene **Maximale Bitrate** tragen. Ein Titel über der Grenze
wird darauf herunterkonvertiert, und etwas Detail geht dabei verloren.

Auf Ihrer Seite gibt es nichts zu wählen: Evey fragt Sie nie nach einer
Qualität. **Stream-Info** zeigt, welche Grenze gegriffen hat.

Mehr: [Was kodiert wird](../playback/transcoding.md#what-is-encoded) und
[Was ein Administrator pro Konto einschränken kann](../administration/users.md#what-an-administrator-can-restrict-per-account).

## Warum ist der Ton in der falschen Sprache?

Eine Spur, die Sie von Hand im Einstellungsmenü des Players wählen, gewinnt
für diesen Titel immer. Sonst nehmen die Player die Sprache aus
**Einstellungen → Wiedergabe → Standard-Audiosprache**, dann die Spur, die
die Datei als Standard markiert, dann die erste. Stellen Sie Ihre Sprache dort
einmal ein, und jeder Titel folgt ihr. Für Untertitel gilt dasselbe mit
**Standard-Untertitelsprache**.

Mehr: [Welche Spur gespielt wird](../playback/audio.md#which-track-plays).

## Läuft es auf meinem Fernseher?

Ja, auf eine von drei Arten:

- **Ein Android TV oder Google TV** führt die Evey-App aus, die für die
  Fernbedienung gemacht ist. Auf dem Anmeldebildschirm zeigt
  **Schnellverbindung** einen Code, den Sie von Ihrem Telefon oder Computer
  aus bestätigen, sodass Sie nie ein Passwort mit der Fernbedienung tippen.
- **Ein Fernseher mit Webbrowser** öffnet dieselbe Art von Bildschirm unter
  der Serveradresse, gefolgt von `/tv`.
- **Ein Chromecast** oder ein Fernseher mit eingebautem Chromecast empfängt
  das Bild über die Cast-Taste im Web- oder Android-Player.

Mehr: [Apps verbinden](apps.md#quick-connect-signing-in-a-tv-without-typing-a-password)
und [Streamen](../playback/casting.md).

## Kann ich im Zug schauen, ohne Verbindung?

Mit der Android-App ja. Bei einem Film oder einer Folge kann die App eine
Kopie auf dem Telefon behalten (**Downloads**), die 30 Tage lang abspielbar
bleibt, ohne den Server zu kontaktieren. Wenn die Person, die Evey betreibt,
es einschaltet, bietet die App auch eine kleinere Kopie für einen Film an, der
zu groß für das Telefon ist. Ein Kinderprofil kann einen Download über seiner
Altersgrenze nicht öffnen, auch wenn ein anderes Profil ihn angelegt hat.

Mehr: [Was jede App hinzufügt](apps.md#what-each-app-adds).

## Kann ich einen Film mit einem Freund schauen, der woanders wohnt?

Ja. Starten Sie den Film im Web-Player oder in der Android-App und drücken Sie
**Zusammen schauen**: Sie erhalten einen Code aus sechs Zeichen. Ihr Freund
gibt ihn ein, aus seinem eigenen Konto oder, ohne Konto, mit **Zusammen
schauen** auf der Anmeldeseite. Jeder sieht denselben Moment, auf dem eigenen
Bildschirm, in der Fassung, die zum eigenen Gerät passt.

Mehr: [Zusammen schauen](../playback/watching-together.md).

## Wie gebe ich einem Freund Zugang, ohne den ganzen Katalog zu öffnen?

Erstellen Sie unter **Verwaltung → Benutzer** einen Einladungslink und
beschränken Sie ihn auf die Bibliotheken, die diese Person sehen darf: Die
anderen erscheinen für sie nirgends, nicht einmal in der Suche. Sie können
auch die Qualität begrenzen, die sie erhält (**Maximale Bitrate**), und die
Zahl der Bildschirme, auf denen sie gleichzeitig abspielen darf. Für einen
einzelnen Film lässt ein **Öffentlicher Download-Link** auf dessen Seite
jeden, der den Link hat, die Datei 48 Stunden lang ohne Konto herunterladen.

Mehr: [Ein Konto anlegen](../administration/users.md#create-an-account) und
[Öffentliche Download-Links](../administration/users.md#public-download-links).

## Warum hat ein Film kein Poster, oder das falsche?

Weil der Dateiname nicht auf den richtigen Titel zeigte. Bei einem Film zählt
das Jahr am meisten (`The Mirror (1999)`), bei einer Folge die Staffel- und
Episodennummer (`S01E05`). Um einen Titel zu korrigieren, öffnen Sie ihn,
wählen **Metadaten bearbeiten**, dann **Auf TMDB identifizieren**, und nehmen
den richtigen Treffer. Eine neue, umbenannte oder gelöschte Datei erscheint
innerhalb weniger Minuten von selbst; **Scannen** in der Zeile der Bibliothek
erzwingt es.

Mehr: [Dateien so benennen, dass sie erkannt werden](../administration/libraries.md#name-files-so-they-are-recognised)
und [Einen falschen oder fehlenden Titel korrigieren](../administration/libraries.md#fix-a-wrong-or-missing-title).

## Die Untertitel sagen, sie werden vorbereitet. Stimmt etwas nicht?

Nein. Wenn jemand Untertitel zum ersten Mal auswählt, zieht der Server sie aus
der Datei; das Video läuft weiter, und sie erscheinen von selbst, sobald sie
fertig sind. Das passiert einmal pro Spur. Bleibt die Meldung mehrere Minuten
stehen, hat die Datei keinen Index für diese Spur, und der Server liest sie
einmal ganz durch.

Mehr: [Wenn ein Untertitel zum ersten Mal gezeigt wird](../playback/subtitles.md#the-first-time-a-subtitle-is-shown).

## Alles ist für alle gleichzeitig langsam. Warum?

Der Server wandelt eine begrenzte Zahl von Videos gleichzeitig um, eine Zahl,
die er auf seiner eigenen Hardware gemessen hat. Wird der Platz knapp, senkt
er zuerst die Qualität eines neuen Streams, verschiebt ihn dann auf den
Prozessor und zeigt erst danach **Der Server transkodiert gerade
andere Streams**. Filme, die so gesendet werden,
wie sie sind, zählen nicht; deshalb stößt ein Katalog, der zu den Geräten im
Haus passt, selten an die Grenze. Die Person, die Evey betreibt, sieht die
Auslastung unter **Verwaltung → Systemzustand**.

Mehr: [Die Kapazität teilen](../playback/transcoding.md#sharing-the-capacity).

## Verlassen meine Filme oder mein Verlauf das Haus?

Nein. Die Apps sprechen mit Ihrem Evey-Server und mit nichts, was das Projekt
betreibt. Was Sie durchstöbern, abspielen und anfragen, bleibt auf diesem
Server, und wer es lesen kann, entscheidet die Person, die ihn betreibt. Die
Ausnahmen sind die erwartbaren: ein Poster, das von seiner öffentlichen Quelle
geladen wird, ein Chromecast, der die Empfängerseite lädt, ein Link, den Sie
selbst öffnen.

Mehr: [Datenschutzerklärung](../privacy.md).

## Wie aktualisiere ich, ohne etwas kaputt zu machen?

Für die Person, die den Server betreibt, im Ordner mit der Compose-Datei:

```bash
docker compose pull && docker compose up -d
```

Das ist die ganze Aktualisierung. Muss die neue Version die Datenbank ändern,
speichert sie zuerst eine Kopie unter `./evey-data/backups/`. Evey meldet im
Verwaltungsbereich, wenn eine neuere Version existiert; es aktualisiert sich
nie von selbst.

Mehr: [Evey aktualisieren](../backups.md#updating-evey).

## Ich habe mein Passwort vergessen

Zeigt die Anmeldeseite **Passwort vergessen?**, nutzen Sie es: Ein Link zum
Zurücksetzen wird Ihnen per E-Mail geschickt. Dieser Link ist nur da, wenn die
Person, die Evey betreibt, die **Passwort-Wiederherstellung** eingeschaltet
und E-Mail eingerichtet hat. Sonst fragen Sie sie: Unter **Verwaltung →
Benutzer** hat **Benutzer bearbeiten** ein Feld **Neues Passwort**. Als
letzter Ausweg, wenn das Passwort des Administrators selbst verloren ist,
gibt `evey admin reset-password` auf dem Server ein neues aus.

Mehr: [E-Mail und Benachrichtigungen](../notifications.md) und
[Einem Benutzer ein neues Passwort geben](../runbooks/recovery.md#give-a-user-a-new-password).
