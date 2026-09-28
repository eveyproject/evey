---
source_sha: 65686e994e8b0eed0c73e145729f00f455caa301b21644be8c6b23c67d7964d3
---
# Warum Evey

Die Filme besitzen Sie schon. Sie haben die Discs gerippt, die Dateien
behalten, die Ordner sortiert. Was fehlt, ist das, was sie angenehm anzusehen
macht: eine Wand aus Postern, die Folge, bei der Sie aufgehört haben, die
richtige Tonspur, dieselbe Bibliothek auf dem Fernseher, dem Handy und dem
Laptop, und ein Weg, sie mit den Menschen zu teilen, mit denen Sie leben, ohne
sie einem Unternehmen zu überlassen.

Evey ist dieser Teil. Es läuft auf Ihrem Rechner - einem NAS, einem Mini-PC,
einem alten Laptop - und liefert Ihre eigenen Dateien an Ihre eigenen
Bildschirme. Fünf Zusagen prägen jede Entscheidung darin, und wenn zwei davon
kollidieren, ist dies die Reihenfolge, in der sie gewinnen.

## 1. Sicher genug fürs Internet

Ein Medienserver ist nur nützlich, wenn man ihn vom Sofa, aus dem Zug und bei
den Eltern erreicht. Deshalb ist Evey dafür gebaut, erreichbar zu sein, nicht
versteckt, und seine Voreinstellungen gehen von einem feindlichen Netz aus:

- der Fernzugriff öffnet sich, wenn Sie den Server unter einer öffentlichen
  Adresse veröffentlichen, und bleibt sonst geschlossen - Sie können es jederzeit
  ausdrücklich entscheiden;
- kann der Server nicht prüfen, wer Sie sind (eine Datenbank, die nicht
  antwortet, eine Jugendschutz-Einstellung, die er nicht lesen kann), lehnt er ab,
  statt die Anfrage durchzulassen;
- Erweiterungen bekommen nur die Berechtigungen, die ihr Autor angegeben hat,
  und der Kernel sperrt sie ein;
- der Server läuft nie als Systemadministrator.

## 2. Mit wenigen Zeilen installiert

Eine Compose-Datei, ein Container, kein Passwort zum Ausdenken. Datenbank und
Suchmaschine leben im Container und erzeugen ihre Geheimnisse beim ersten
Start selbst. Alles, was Sie später ändern wollen - Sprachen,
Umwandlungsgrenzen, Grafikkarte, Zeitpläne - steht im Administrationsbereich,
nicht in einer Konfigurationsdatei, die Sie bearbeiten und dann neu starten
müssen.

## 3. Erweiterbar in der Sprache, die Sie schreiben

Evey lässt sich mit Erweiterungen ausbauen: Metadatenquellen, Widgets für die
Startseite, Themes. Eine Erweiterung ist ein Programm, das der Server startet
und mit dem er über eine lokale Verbindung spricht; sie lässt sich also in der
Sprache schreiben, die Sie schon können - fertige Bausätze gibt es für sieben
davon. Weil Erweiterungen in einer Sandbox laufen, statt dass man ihnen
vertraut, ist es vernünftig, auch die eines anderen zu installieren.

## 4. Schnell, und ehrlich darüber

Der schnellste Weg, eine Datei abzuspielen, ist, sie so zu schicken, wie sie
ist. Evey tut das, sobald das Gerät sie lesen kann, wandelt nur den Teil um, den
das Gerät nicht beherrscht (oft nur den Ton), und entscheidet einmal pro
Wiedergabe, statt es sich mitten im Film anders zu überlegen. Die Zahlen hinter
diesen Entscheidungen werden an echten Dateien und echten Rechnern gemessen,
nicht angenommen - eine Optimierung, die clever aussah, blieb erst, nachdem
bewiesen war, dass sie dieselbe Antwort liefert wie das Werkzeug, das sie
ersetzt.

## 5. Für viele Zuschauer zugleich, auf bescheidener Hardware

Das Ziel ist ein Haushalt mit seinen Freunden auf einem NAS, kein
Rechenzentrum. Deshalb misst Evey beim ersten Start, was Ihr Rechner kann, lässt
nur so viele gleichzeitige Umwandlungen zu, wie er tragen kann, pausiert seine
Hintergrundarbeit, während jemand schaut, und richtet sich nach der Hardware,
die er vorfindet. Ein langsamer Rechner spielt weniger Streams; er spielt nicht
alle schlecht.

---

Zwei Dinge ist Evey bewusst nicht. Es ist kein Katalog aller Medienarten, jede
halbwegs erledigt - Wiedergabe ist sein Metier. Und es ist kein Produkt, das
Sie bittet, ihm zu vertrauen: Was der Server tut und was er noch nicht
verhindern kann, steht im [Sicherheitsmodell](SECURITY.md), statt nur
angedeutet zu werden.

**Weiter:** [was Sie bekommen](discover/features.md) ·
[die Geräte, auf denen es läuft](discover/apps.md) ·
[installieren](installation.md)
