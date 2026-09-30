---
source_sha: dd11b7fe28f14e3c25cf6db836b2a6f0752722f82e49a8db9ce447b959d38822
reviewed: false
---
# Ersteinrichtung

Der Server läuft und Sie haben `http://<server-address>:9091` geöffnet. Vier
Schritte bleiben, bevor geschaut werden kann: der Einrichtungsassistent, die
Bibliotheken, die Benutzer und die Apps.

## 1. Der Einrichtungsassistent

Ein Server ohne Administrator öffnet einen Assistenten mit vier Schritten:

1. **Sprache**: die Standardsprache neuer Profile und der Metadaten (Titel,
   Zusammenfassungen, Genres).
2. **Willkommen**.
3. **Admin-Konto**: ein Benutzername mit mindestens 3 Zeichen und ein Passwort
   mit mindestens 8. Der optionale Servername erscheint auf der Anmeldeseite
   und in der Serverliste der Apps.
4. **Erste Bibliothek**: ein Name und ein Typ. Sie können diesen Schritt
   überspringen und Bibliotheken später hinzufügen.

<figure class="evey-shot" markdown="span">
  ![Das Admin-Dashboard des Demo-Servers](../images/admin-dashboard.webp){ loading=lazy }
  <figcaption>Die Verwaltung nach der Anmeldung: das Dashboard und jeder Bereich im Seitenmenü (Oberfläche auf Englisch).</figcaption>
</figure>

## 2. Bibliotheken hinzufügen

**Admin → Bibliotheken → Bibliothek hinzufügen**: ein Name, ein Typ (Filme,
Serien, Musik oder Fotos) und ein oder mehrere Ordner.

Die Ordner sind Pfade **im Container**. Mit der Compose-Datei aus der
Installation ist Ihr Medienordner `/media`: Ein Filmordner unter
`/srv/media/films` auf dem Host ist für Evey `/media/films`.

Der erste Scan startet von selbst. Neue Dateien werden erkannt, sobald sie
erscheinen, und Poster, Zusammenfassungen und Besetzung kommen im Hintergrund
dazu, sodass sich eine große Bibliothek in einigen Minuten füllt.

Wie Dateien benannt werden sollten, wie Scans funktionieren und wie man eine
falsche Zuordnung korrigiert: [Bibliotheken und Metadaten](../administration/libraries.md).

## 3. Personen hinzufügen

**Admin → Benutzer** legt ein Konto mit einem von Ihnen gewählten Passwort an,
oder einen Einladungslink, mit dem die Person ihr eigenes festlegt. Jedes Konto
legt dann seine Profile an: eines pro Person im Haushalt, Kinderprofile
eingeschlossen.

Was ein Administrator pro Konto einschränken kann und wie Profile und
Kindersicherung funktionieren: [Benutzer, Profile und Kindersicherung](../administration/users.md).

## 4. Die Apps verbinden

Die Web-App ist die Adresse, die Sie bereits geöffnet haben. Die Apps für
Android und Android TV fragen nach der Serveradresse und dann nach einer
Anmeldung; ein Fernseher kann mit einem Code angemeldet werden, der auf einem
Telefon bestätigt wird. Siehe [Apps und Geräte](apps.md).

## Was der Server von selbst erledigt

Nichts davon braucht eine Einstellung:

- **Wiedergabe.** Jedes Gerät meldet, was es dekodieren kann, und der Server
  wählt Titel für Titel, ob er die Datei so sendet, wie sie ist, sie neu
  verpackt oder nur das neu kodiert, was das Gerät nicht abspielen kann. Siehe
  [Wie Evey die Wiedergabeart wählt](../playback/how-playback-is-decided.md).
- **Kapazität.** Im ersten ruhigen Moment misst der Server, wie viele Videos er
  gleichzeitig transkodieren kann, mit dem Prozessor und mit jeder in den
  Container durchgereichten Grafikkarte, und lässt nie mehr zu. Die Messung
  dauert einige Minuten; eine Wiedergabe, die in dieser Zeit eine
  Transkodierung braucht, wird gebeten zu warten. Siehe
  [Transkodierung und Serverkapazität](../playback/transcoding.md).
- **Fernzugriff.** Er öffnet sich, wenn dem Server eine öffentliche Adresse
  gegeben wurde, und bleibt sonst geschlossen. Siehe
  [Fernzugriff und HTTPS](../administration/remote-access.md).
- **Sicherungen.** Die Datenbank wird vor jedem Update, das sie verändert,
  gesichert. Ein regelmäßiger Zeitplan ist nur eine Einstellung entfernt:
  [Sicherungen und Updates](../backups.md).

## Bevor Sie die Adresse außerhalb Ihres Zuhauses teilen

1. Stellen Sie den Server hinter HTTPS: Evey liefert einfaches HTTP aus.
   [Fernzugriff und HTTPS](../administration/remote-access.md).
2. Legen Sie einen Sicherungsplan fest und bewahren Sie eine Kopie von
   `./evey-data` auf: Darin liegen die Datenbank und der Schlüssel, der jede
   Sitzung signiert. [Sicherungen und Updates](../backups.md).
