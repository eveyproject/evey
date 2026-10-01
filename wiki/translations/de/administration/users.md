---
source_sha: 8485c27cee750eebc4a2f82a04e647554aa7e8ce88bb1b27e705c8904d573fb0
reviewed: false
---
# Benutzer, Profile und Kindersicherung

## Konten und Profile

Ein **Konto** meldet sich an: Es hat einen Benutzernamen und ein Passwort (oder
eine Single-Sign-on-Identität, siehe [OIDC](../oidc.md)) und ist entweder
Administrator oder nicht. Ein **Profil** schaut: Jedes Konto enthält ein oder
mehrere Profile, jedes mit eigenem Verlauf, eigenen Wiedergabepositionen,
Favoriten, Sprachen und eigener Startseite. Ein Haushalt teilt sich
typischerweise ein Konto, mit einem Profil pro Person.

## Konten anlegen

**Admin → Benutzer** bietet zwei Wege:

<figure class="evey-shot" markdown="span">
  ![Admin → Benutzer auf dem Demo-Server](../images/admin-users.webp){ loading=lazy }
  <figcaption><strong>Admin → Benutzer</strong>: jedes Konto mit seinen Profilen, seiner Rolle und seinem Zustand (Oberfläche auf Englisch).</figcaption>
</figure>

- **Benutzer erstellen**: Sie wählen Benutzername und Passwort.
- **Einladungslink erstellen**: Die Person wählt ihr eigenes. Ein Link kann ein
  Ablaufdatum und eine Höchstzahl an Nutzungen haben, die Bibliotheken
  einschränken, die das neue Konto sieht, die Zahl der Profile begrenzen, die
  es anlegen darf, und einen Administrator anlegen. **Einladungslinks** listet
  sie auf; ein Link kann widerrufen werden.

## Was ein Administrator pro Konto einschränken kann

| Einstellung | Wirkung |
| --- | --- |
| **Bibliothekszugriff** | Die Bibliotheken, die das Konto sieht. Alles andere fehlt in seinen Suchen, auf seiner Startseite und in den API-Antworten. |
| **Maximale Bitrate** | Eine Obergrenze für die Bitrate, die die Player des Kontos erhalten (0 = unbegrenzt). Ein Titel darüber wird auf diese Grenze heruntertranskodiert. |
| **Max. Profile** | Wie viele Profile das Konto anlegen darf (0 = unbegrenzt). |
| **Max. Geräte pro Profil** | Wie viele Wiedergaben jedes Profil gleichzeitig laufen haben darf, über alle seine Geräte (0 = unbegrenzt). Eine weitere wird beim Start abgewiesen. |

**Anmelden als** öffnet den Server so, wie dieser Benutzer ihn sieht, um eine
Einschränkung zu prüfen oder ein Problem nachzustellen. Ein Banner bleibt
sichtbar, und **Beenden** bringt Sie zu Ihrem eigenen Konto zurück.

## Kinderprofile und die PIN

Ein als **Kinderprofil** markiertes Profil öffnet sich ohne PIN. Wer es für ein
Profil verlässt, das kein Kinderprofil ist, wird nach der **4-stelligen PIN**
gefragt, die unter **Einstellungen → Kindersicherung** festgelegt wird. Beim
ersten Festlegen der PIN wird das Kontopasswort abgefragt, und solange ein
Kinderprofil existiert, muss das Konto eine PIN haben.

Profile und Kindersicherung lassen sich nur von einem Profil aus verwalten, das
kein Kinderprofil ist.

## Altersgrenzen

Jedes Profil kann eine **Max. Altersfreigabe** haben (**Einstellungen →
Profile → Bearbeiten**). Das Bewertungssystem ist das des Landes, das unter
**Admin → Serverkonfiguration → Sprache & Region** eingestellt ist.

Die Grenze wendet der Server an, nicht die Apps: Ein Titel darüber wird in der
Oberfläche nicht versteckt, er wird nie gesendet. Das gilt für Stöbern und
Suche, für die Wiedergabe und für Downloads. Kann der Server die Einstellungen
eines Profils nicht lesen (zum Beispiel bei einem Datenbankfehler), verweigert
er den Inhalt, statt ihn zuzulassen. Administratoren unterliegen keinen
Profilgrenzen.

Ein auf ein Telefon heruntergeladener Film bleibt auch offline der Grenze
unterworfen: Ein Kinderprofil kann keinen Download öffnen, den ein anderes
Profil oberhalb seiner Altersgrenze erstellt hat.

## Anfragen

Benutzer können nach einem Film, einer Serie oder einem Künstler suchen, der in
den Bibliotheken fehlt, und ihn anfragen. Anfragen landen unter **Admin →
Medienanfragen**, wo ein Administrator sie mit einer Notiz annimmt, ablehnt
oder zurückstellt. Wer anfragen darf, wird dort festgelegt: standardmäßig
alle, mit Ausnahmen pro Benutzer.

## Öffentliche Download-Links

Auf der Seite eines Films oder einer Folge kann ein Administrator einen
**Öffentlichen Download-Link** erstellen (Web- und Android-App). Das ist eine
Adresse, die jeder ohne Konto öffnen kann, um die Originaldatei
herunterzuladen; sie wird in die Zwischenablage kopiert.

- Er läuft nach 48 Stunden ab. Über die Server-API kann er bis zu 72 Stunden
  gelten und auf eine Anzahl von Downloads begrenzt werden
  (`POST /api/v1/media/{id}/share`).
- Nur Administratoren können Links erstellen, auflisten und widerrufen. Die
  Apps erstellen sie nur: Auflisten und Widerrufen laufen über die API
  (`GET /api/v1/admin/share-links`, `DELETE /api/v1/share-links/{id}`).
- Bei ausgeschaltetem Fernzugriff funktioniert der Link nur aus den lokalen
  Netzwerken.
- Wer den Link hat, kann die Datei bis zum Ablauf herunterladen: Teilen Sie
  ihn wie die Datei selbst.

## Die Startseite

Die Startseite ist ein Stapel von Zeilen; eine Zeile ohne Inhalt wird
ausgeblendet.

| Zeile | Was sie zeigt |
| --- | --- |
| **Wiedergabe fortsetzen** | Was das Profil begonnen und nicht beendet hat, das Neueste zuerst. Ein Titel gilt bei 90 % als gesehen. |
| **Als Nächstes** | Für jede Serie, von der das Profil eine Folge gesehen hat, die nächste ungesehene Folge. |
| **Sammlungen** | Die Sammlungen, mit einem Link zur vollständigen Liste. |
| **Für Sie empfohlen** | Filme und Serien aus den üblichen Genres des Profils, die beste Übereinstimmung zuerst, dann nach Bewertung. Ein Genre zählt, wenn es in mindestens 3 der letzten 200 Wiedergaben des Profils vorkommt. |
| **Am beliebtesten** | Die Filme und Serien, die in den letzten 90 Tagen auf dem Server am häufigsten abgespielt wurden. |
| Eine Zeile pro Bibliothek | Die neuesten Zugänge dieser Bibliothek, das Neueste zuerst. |

Für Sie empfohlen und Am beliebtesten lassen weg, was das Profil schon beendet
hat; eine Serie gilt als beendet, sobald jede Folge gesehen ist. Plugins können
eigene Zeilen hinzufügen.

**Admin → Serverkonfiguration → Startseiten-Feeds** legt die Vorgabe für alle
Profile fest: welche Zeilen erscheinen, in welcher Reihenfolge und wie viele
Einträge jede zeigt. Jedes Profil ordnet seine eigene Startseite dann mit
**Anpassen** (auf der Startseite der Web- und Mobil-App, in der Kopfzeile der
TV-App): eine Zeile ein- oder ausblenden, verschieben oder **Zurücksetzen**. Die
Wahl wird auf dem Server gespeichert und folgt dem Profil auf jedes Gerät. Ein
Profil kann eine Zeile einblenden, die der Administrator ausgeblendet hat, außer
der Zeile eines Plugins: Dort ist die Wahl des Administrators endgültig.
