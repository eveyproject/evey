---
source_sha: f0478ec26ab0b9d817c4a2fa786f673cb92219628c9b8b18dc92835e4f9ff361
reviewed: true
---
# Ersteinrichtung { #first-setup }

Evey läuft, und Sie haben seine Adresse im Browser geöffnet, zum Beispiel
`http://192.168.1.20:9091` (welche Adresse? Siehe
[Schritt 3 der Installation](../installation.md#the-address-to-open-next)).

Es bleiben vier Schritte, bis Ihr Haushalt schauen kann:

1. [Der Einrichtungsassistent](#1-the-setup-wizard): etwa eine Minute.
2. [Ihre Bibliotheken hinzufügen](#2-add-your-libraries): Evey sagen, wo Ihre
   Medien liegen.
3. [Ihren Haushalt einladen](#3-invite-your-household): ein Konto pro
   Person, oder ein gemeinsames Konto mit einem Profil pro Person.
4. [Die Apps verbinden](#4-connect-the-apps): Handys, Tablets und Fernseher.

## 1. Der Einrichtungsassistent { #1-the-setup-wizard }

Solange der Server keinen Administrator hat, öffnet seine Adresse immer den
Assistenten. Oben steht **Schritt 1 von 4 - Sprache**, dazu vier Punkte, die
sich nach und nach füllen.

### Schritt 1 von 4 - Sprache { #step-1-of-4---language }

Die Karte zeigt **Wählen Sie die Serversprache**. Wählen Sie:

- **Sprache**: die Standardsprache neuer Profile und der Titel und
  Inhaltsangaben, die Evey abruft. Der Assistent wechselt sofort in diese
  Sprache.
- **Region**: wird anhand der Sprache ausgefüllt. Sie bestimmt, welche
  Altersfreigaben angezeigt werden. **Automatisch** steht zur Auswahl.

Beides können Sie später im Verwaltungsbereich ändern. Drücken Sie auf
**Weiter**.

### Schritt 2 von 4 - Willkommen { #step-2-of-4---welcome }

Nichts auszufüllen. Drücken Sie auf **Loslegen**.

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![Schritt 1 des Assistenten: Sprache und Region](../images/setup-wizard-1-language.webp){ loading=lazy data-gallery="setup" }
  <figcaption>Schritt 1: die Sprache des Servers und die Region für die Altersfreigaben (Oberfläche auf Englisch).</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Schritt 2 des Assistenten: Willkommen](../images/setup-wizard-2-welcome.webp){ loading=lazy data-gallery="setup" }
  <figcaption>Schritt 2: nichts auszufüllen.</figcaption>
</figure>

</div>

### Schritt 3 von 4 - Admin-Konto { #step-3-of-4---admin-account }

Die Karte zeigt **Admin-Konto erstellen**. Das ist Ihr eigenes Konto, mit dem
Sie den Server verwalten.

| Feld | Was Sie eintippen |
| --- | --- |
| **Benutzername** | Mindestens 3 Zeichen. |
| **Passwort** | Mindestens 8 Zeichen. |
| **Passwort bestätigen** | Noch einmal dasselbe Passwort. |
| **Servername** | Freiwillig. Ein Name wie „Wohnzimmer“. Man sieht ihn auf der Anmeldeseite, im Browser-Tab, in der Serverliste der Apps und in E-Mails. Sie können ihn später unter **Verwaltung → Serverkonfiguration → Servername** ändern. |

Drücken Sie auf **Weiter**. Evey legt das Konto an und meldet Sie an. Der
Assistent erscheint nicht mehr.

### Schritt 4 von 4 - Erste Bibliothek { #step-4-of-4---first-library }

Die Karte zeigt **Erste Bibliothek hinzufügen**. Eine Bibliothek ist eine Art
von Medien in einem oder mehreren Ordnern, zum Beispiel „Filme“.

- **Bibliotheksname**: zum Beispiel „Filme“.
- **Bibliothekstyp**: **Filme**, **Serien**, **Musik** oder **Fotos**.
- **Pfade**: Wählen Sie einen Ordner aus der Liste. Sie zeigt die Ordner, die
  Evey sehen kann, angefangen mit `/media`. Um einen Pfad wie `/media/movies`
  einzutippen, wählen Sie **Benutzerdefinierten Pfad eingeben...**.

Drücken Sie auf **Erstellen**: Evey beginnt, den Ordner zu lesen, und die Titel erscheinen im Lauf des Scans (ein großer Ordner braucht ein paar Minuten). Oder
drücken Sie auf **Überspringen** und fügen Sie Bibliotheken später hinzu.
Beides führt Sie zur Startseite.

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![Schritt 3 des Assistenten: das Administratorkonto](../images/setup-wizard-3-admin.webp){ loading=lazy data-gallery="setup" }
  <figcaption>Schritt 3: das Administratorkonto; der Servername ist freiwillig.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Schritt 4 des Assistenten: die erste Bibliothek](../images/setup-wizard-4-library.webp){ loading=lazy data-gallery="setup" }
  <figcaption>Schritt 4: ein Name, ein Typ und ein Ordner, so wie Evey ihn sieht.</figcaption>
</figure>

</div>

??? note "Wenn der Assistent nicht erscheint, oder zweimal"

    Sobald es einen Administrator gibt, öffnet die Adresse die Anmeldeseite,
    und `/setup` schickt Sie ebenfalls dorthin. Öffnen zwei Personen den
    Assistenten gleichzeitig, bekommt nur die erste das Administratorkonto:
    Die zweite wird zur Anmeldeseite geschickt.

<figure class="evey-shot" markdown="span">
  ![Das Admin-Dashboard des Demo-Servers](../images/admin-dashboard.webp){ loading=lazy }
  <figcaption>Der Verwaltungsbereich nach der Anmeldung: das Dashboard und alle Bereiche im Seitenmenü (Oberfläche auf Englisch).</figcaption>
</figure>

## 2. Ihre Bibliotheken hinzufügen { #2-add-your-libraries }

Gehen Sie zu **Verwaltung → Bibliotheken → Bibliothek hinzufügen**. Geben Sie
einen Namen, einen Typ (Filme, Serien, Musik oder Fotos) und einen oder
mehrere Ordner an.

Die Ordner heißen so, wie Evey sie sieht, nicht so wie auf Ihrem Computer.
Mit der Compose-Datei aus der [Installation](../installation.md#the-compose-file)
ist Ihr Medienordner `/media`. Ein Ordner `/srv/media/films` auf Ihrem
Computer ist in Evey also `/media/films`.

Evey beginnt von selbst, den Ordner zu lesen, und die Titel erscheinen im Lauf des Scans. Poster, Inhaltsangaben und
Besetzung kommen im Hintergrund dazu, eine große Bibliothek füllt sich also
im Lauf einiger Minuten. Neue Dateien, die Sie später hinzufügen, erscheinen
von selbst.

Ihre Titel tauchen nicht auf, oder sie zeigen das falsche Poster? Siehe
[Bibliotheken und Metadaten](../administration/libraries.md): wie Sie Dateien
benennen und wie Sie eine falsche Zuordnung korrigieren.

## 3. Ihren Haushalt einladen { #3-invite-your-household }

Es gibt zwei Wege, jemandem ein Konto zu geben, beide unter **Verwaltung →
Benutzer**.

**Einen Einladungslink schicken** (die Person wählt ihr Passwort selbst):

1. Öffnen Sie den Verwaltungsbereich über die Adresse, die die anderen
   benutzen werden, zum Beispiel `http://192.168.1.20:9091`, nicht
   `http://localhost:9091`. Der Link wird aus der Adresse in Ihrem Browser
   gebildet.
2. Drücken Sie auf **Einladungslink erstellen**. Sie können begrenzen, wie
   lange er gilt (**Ablaufdatum (optional)**), wie oft er benutzt werden kann
   (**Max. Verwendungen (leer = unbegrenzt)**) und welche Bibliotheken das
   neue Konto sieht (**Zugängliche Bibliotheken**).
3. Drücken Sie unter **Einladungslinks** auf **Link kopieren** und schicken
   Sie ihn per Nachricht oder E-Mail.
4. Die Person öffnet den Link, sieht **Sie wurden eingeladen! Füllen Sie Ihre
   Daten unten aus.**, wählt einen Benutzernamen und ein Passwort (mindestens
   8 Zeichen) und ist angemeldet.

**Oder das Konto selbst anlegen**: Drücken Sie auf **Benutzer erstellen**,
wählen Sie **Benutzername** und **Passwort** und geben Sie beides der Person.

Jedes Konto beginnt mit einem Profil, das nach dem Benutzernamen benannt ist.
Ein Haushalt kann sich ein Konto teilen, mit einem Profil pro Person: Legen
Sie diese unter **Einstellungen → Profile → Profil erstellen** an. Für ein
Kind haken Sie **Kinderprofil** an und wählen eine **Max. Altersfreigabe**.
Jedes andere Profil dieses Kontos braucht dann eine **PIN (4 Ziffern)**,
damit das Kind nicht dorthin wechseln kann.

Schicken Sie dann jeder Person drei Dinge: die Adresse, ihren Benutzernamen
und ihr Passwort (oder den Einladungslink) und die Seite, die für sie
geschrieben ist: [Mit Evey schauen](watching.md).

Weitere Möglichkeiten (Grenzen pro Konto, Nutzungszeiten, PINs,
Altersgrenzen):
[Benutzer, Profile und Kindersicherung](../administration/users.md).

## 4. Die Apps verbinden { #4-connect-the-apps }

- **Im Browser** gibt es nichts zu installieren: Es ist die Adresse, die Sie
  schon geöffnet haben.
- **Auf einem Android-Handy, -Tablet oder -Fernseher** fragt die App zuerst
  nach der Serveradresse. Tippen Sie **dieselbe Adresse wie im Browser** ein,
  mit `http://` und dem Port, zum Beispiel `http://192.168.1.20:9091`. Melden
  Sie sich dann an.
- **Auf einem Fernseher** können Sie sich mit einem Code anmelden, den Sie auf
  Ihrem Handy bestätigen, statt mit der Fernbedienung ein Passwort
  einzutippen.

Schritt für Schritt, und was jede Fehlermeldung bedeutet:
[Apps verbinden](apps.md#the-server-address-to-enter).

## Was der Server von selbst erledigt { #what-the-server-does-by-itself }

Nichts davon müssen Sie einstellen:

- **Wie jedes Gerät eine Datei abspielt.** Jedes Gerät meldet dem Server, was
  es abspielen kann. Für jeden Titel schickt der Server die Datei dann so, wie
  sie ist, verpackt sie neu, oder wandelt nur das um, was das Gerät nicht
  abspielen kann. Siehe
  [Wie Evey die Wiedergabeart wählt](../playback/how-playback-is-decided.md).
- **Wie viele Personen er bedienen kann.** In seinem ersten ruhigen Moment
  misst der Server, wie viel Video er gleichzeitig umwandeln kann, mit dem
  Prozessor und einer Grafikkarte, falls vorhanden, und nimmt nie mehr an.
  Das dauert ein paar Minuten. In der Zwischenzeit muss eine Wiedergabe, die
  eine Umwandlung braucht, warten. Siehe
  [Transkodierung und Serverkapazität](../playback/transcoding.md).
- **Der Zugriff von außerhalb Ihres Zuhauses.** Aus, bis Sie dem Server eine
  öffentliche Adresse geben. Siehe
  [Fernzugriff und HTTPS](../administration/remote-access.md).
- **Eine Sicherheitskopie der Datenbank** vor jedem Update, das sie verändert.
  Regelmäßige Sicherungen richten Sie mit einer einzigen Einstellung ein:
  [Sicherungen und Updates](../backups.md).

## Bevor Sie die Adresse außerhalb Ihres Zuhauses weitergeben { #before-you-share-the-address-outside-your-home }

1. Stellen Sie den Server hinter HTTPS. Evey selbst verwendet nur einfaches
   HTTP, was zu Hause in Ordnung ist, im Internet aber nicht.
   [Fernzugriff und HTTPS](../administration/remote-access.md).
2. Richten Sie einen Sicherungsplan ein und bewahren Sie eine Kopie des
   Ordners `evey-data` auf. Er enthält die Datenbank und den Schlüssel, der
   jede Anmeldung schützt.
   [Sicherungen und Updates](../backups.md).
