---
source_sha: 48560184418f09f59a0147095354999490a70abb4f0d86904e7c50b9b4b2b835
---
# Einstellungen für Zuschauer

Was jedes Konto selbst ändern kann, ohne den Administrator zu fragen. Das
meiste steht unter **Einstellungen** (das Avatar-Menü, dann Einstellungen); der
Rest im Player. Die Android- und TV-Apps bieten dieselben Einstellungen in
ihrem eigenen Einstellungsbildschirm.

Einstellungen hängen am **Profil** vor dem Bildschirm, nicht am Gerät: Wählen
Sie am Fernseher englischen Ton, nutzt das Handy ihn auch.

## Darstellung und Sprache

| Einstellung | Was sie tut |
| --- | --- |
| **Design** | Das Farbdesign, einschließlich der vom Administrator installierten. |
| **Sprache** | Die Sprache der Oberfläche. Sechs stehen zur Wahl: Englisch, Französisch, Deutsch, Spanisch, Hindi und Chinesisch. |
| **Region** | Welches Landes Altersfreigaben angezeigt und angewandt werden. **Automatisch** folgt dem Server. |
| **Bevorzugte Audiosprache** | Die Spur, die zuerst gewählt wird, wenn eine Datei mehrere Sprachen hat. |
| **Bevorzugte Untertitelsprache** | Die Untertitelspur, die angezeigt wird, wenn eine passt; **Keine Präferenz** lässt sie aus, sofern die Datei sie nicht erzwingt. |

Titel und Zusammenfassungen folgen der Metadatensprache des Servers, die der
Administrator wählt.

## Wiedergabe

| Einstellung | Standard | Was sie tut |
| --- | --- | --- |
| **Nächste Folge automatisch abspielen** | an | Startet die nächste Folge, wenn eine endet. Die Taste für die nächste Folge bleibt so oder so. |
| **Maximale Streaming-Bitrate** | Unbegrenzt | Eine Obergrenze für Ihre Streams. Senken Sie sie bei einer langsamen Leitung: Der Server wandelt passend um, statt den Player stocken zu lassen. |
| **Maximale Mobilfunk-Bitrate** (Handy) | - | Eine niedrigere Obergrenze, nur im Mobilfunknetz. |
| **Bevorzugter Video-Codec** / **Audio-Codec** | Auto | Lassen Sie Auto, außer ein Gerät hat Probleme mit einem Codec: Auto lässt den Server wählen, was das Gerät am besten dekodiert. |
| **Maximale Audio-Kanäle** | Auto | Stereo, 5.1 oder 7.1. Wählen Sie Stereo, wenn Sie mit Kopfhörern oder über die Fernsehlautsprecher schauen und Dialoge in Surround-Mischungen zu leise sind. |
| **Untertitel einbrennen** | Auto | Manche Untertitelformate (bildbasiert oder stark gestaltet) kann nicht jeder Player zeichnen; Auto brennt sie nur dann ins Bild ein. **Immer einbrennen** und **Nie einbrennen** erzwingen es, um den Preis einer Umwandlung oder fehlender Untertitel. |
| **Direktwiedergabe** / **Direkt-Stream** / **Transkodierung erlauben** | alle an | Die drei Wege, auf denen ein Film zu Ihnen kommt: unverändert, neu verpackt oder umgewandelt. Einen abzuschalten ist ein Werkzeug zur Fehlersuche, keine Alltagseinstellung. |

Der Administrator kann für Ihr Konto oder für Verbindungen von außerhalb eine
niedrigere Obergrenze setzen; es gewinnt immer die niedrigste.

## Im Player

Das Zahnrad öffnet die Einstellungen des Players:

- **Qualität**: **Auto** passt sich Ihrer Verbindung an; **Original** verlangt
  die unveränderte Datei, wenn das Gerät sie lesen kann; eine feste Höhe (1080p,
  720p...) erzwingt eine Umwandlung in dieser Größe.
- **Audio** und **Untertitel**: jederzeit die Spur wechseln, ohne von vorn zu
  beginnen. **Untertitel hochladen...** fügt diesem Titel eine Untertiteldatei
  von Ihrem Computer hinzu.
- **Audio-Synchronisation**: verschiebt den Ton gegenüber dem Bild, in
  Millisekunden (positiv = Ton später). Verfügbar, wenn der Film umgewandelt
  wird; der Stream startet an der aktuellen Stelle neu.
- **Untertitel-Synchronisation**: verschiebt die Untertitel (positiv = später),
  sofort.
- **Untertitel-Stil**: Größe, Textfarbe, Hintergrund, Schrift, Position,
  Schatten.
- **Geschwindigkeit** und **Schlaf-Timer** (eine Zahl von Minuten oder das Ende
  der Folge).

Rund um die Zeitleiste: **Kapitel**, Tasten zum **Überspringen von Intro**,
**Rückblick** und **Abspann**, wenn der Server weiß, wo sie liegen, ein
Countdown zur nächsten Folge, **Streamen**, **Abspielen auf** (an eine andere
Evey-App schicken), **Zusammen schauen** und **Herunterladen** (Handy).

**Stream-Info** sagt Ihnen, wie der Film zu Ihnen kommt - Direktwiedergabe,
Remux oder Transkodierung - und warum. Es ist das Erste, was Sie ansehen sollten,
wenn etwas schlecht läuft: „Audiocodec, den dieser Player nicht dekodieren kann“
oder „Bitrate über dem erlaubten Limit“ erklären es meist.

Am Computer zeigt `?` die Tastenkürzel: `Leertaste` oder `K` für Pause, `J`/`L`
oder die Pfeile links und rechts zum Springen, hoch und runter für die
Lautstärke, `F` für Vollbild, `M` zum Stummschalten, `C` für Untertitel, `<` und
`>` für die Geschwindigkeit, `P` für Bild-in-Bild und `0` bis `9`, um zu 0-90 %
des Films zu springen.

## Profile und Jugendschutz

**Einstellungen > Profile** legt Profile an, bearbeitet und löscht sie (das
Standardprofil lässt sich nicht löschen). Jedes Profil hat:

| Feld | Was es tut |
| --- | --- |
| **Name** und **Avatar** | Erscheinen in der Auswahl „Wer schaut zu?“. |
| **Kinderprofil** | Öffnet sich ohne PIN; wer es für ein anderes Profil verlässt, braucht die PIN des Kontos. |
| **Max. Altersfreigabe** | Titel darüber werden diesem Profil nie geschickt - nicht in Listen, nicht in der Suche, nicht an den Player. Der Server legt es fest, keine App kann es umgehen. |
| **Sprache** und **Region** | Überschreiben für dieses Profil die Sprache der Oberfläche und das Land der Freigaben. |

**Einstellungen > Kindersicherung** legt die **vierstellige PIN** des Kontos
fest. Beim ersten Mal fragt sie nach dem Kontopasswort; um sie später zu
ändern, nach der aktuellen PIN. Eine PIN ist Pflicht, sobald ein Kinderprofil
existiert.

Kann der Server die Alterseinstellungen eines Profils nicht lesen, zeigt er
nichts statt alles: Eine kaputte Einstellung öffnet einem Kind nie die
Bibliothek.

## Downloads (Handy)

**Herunterladen** bei einem Film, einer Staffel, einer Serie, einer Sammlung
oder einer Playlist behält sie auf dem Handy. Ist ein Film zu groß für den
freien Platz, bietet die App eine kleinere Kopie an, die der Server vorbereitet,
mit der erwarteten Größe jeder Wahl. **Downloads** zeigt, was gespeichert ist
und wie viel Platz es belegt.

Ein Download bleibt an die Regeln des Profils gebunden: Ein Titel über der
Altersgrenze eines Profils ist von diesem Profil aus nicht abspielbar, selbst
wenn ein anderes Profil ihn heruntergeladen hat. Ein Abzeichen zeigt an, wenn
ein Download abgelaufen ist und neu geholt werden muss.

## Benachrichtigungen

**Einstellungen > Benachrichtigungen** schaltet Push-Benachrichtigungen für
dieses Gerät ein und legt für Filme, Serien und Musik getrennt fest, wie viel
Sie hören wollen: **Alle**, **Relevant**, **Nur wichtige** oder
**Deaktiviert**. Push setzt voraus, dass der Administrator es eingerichtet hat
([Benachrichtigungen](../notifications.md)).

## Konto und Geräte

- **Konto** (Profilkarte): Benutzername, E-Mail, Passwort (mindestens acht
  Zeichen) und Avatar. Die E-Mail ist es, mit der Sie ein vergessenes Passwort
  zurücksetzen können, sofern der Server Mails verschicken kann.
- **Geräte**: die für Push-Benachrichtigungen registrierten Geräte, die Sie
  entfernen können.
- **Schnellverbindung**: Geben Sie den Code ein, den ein Fernseher oder ein
  anderes Gerät zeigt, oder scannen Sie ihn, um es mit Ihrem Konto zu
  verbinden, ohne dort Ihr Passwort einzutippen.
- **Wiedergabeverlauf** (Profilmenü): alles, was Sie gesehen haben, nach Tagen,
  mit einer Taste, um einen Eintrag zu entfernen oder alles zu löschen.
- **Plugins**: Hat der Administrator Erweiterungen mit eigenen Einstellungen
  installiert, erscheinen sie hier.

## Anfragen

Wenn der Administrator es erlaubt, können Sie unter **Medienanfragen** nach
einem Film, einer Serie oder einem Künstler suchen, der nicht in der Bibliothek
ist, und ihn anfragen. **Meine Anfragen** zeigt die Antwort: angenommen,
abgelehnt oder später, mit der Notiz des Administrators.
