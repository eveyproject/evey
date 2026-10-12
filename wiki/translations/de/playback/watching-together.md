---
source_sha: 9fad7f52fd0f1b44bc2f65ef902487f1f2929c2f19b89072be7f363968ba8bb8
reviewed: true
---
# Zusammen schauen

Schauen Sie denselben Film im selben Moment wie andere Personen, jede auf
ihrem eigenen Bildschirm. Eine Person eröffnet einen Raum; die anderen treten
ihm mit einem Code bei. Auf dem Server gibt es nichts einzurichten.

Kurz gesagt:

1. Starten Sie den Film im Web-Player oder in der Android-App und drücken Sie
   auf **Zusammen schauen**. Sie sind der Gastgeber des Raums und sehen seinen
   Code: sechs Buchstaben und Ziffern.
2. Schicken Sie den Code an die Personen, mit denen Sie schauen wollen.
3. Sie treten mit dem Code bei: über ihr eigenes Konto (siehe Tabelle unten)
   oder, ohne Konto, mit **Gemeinsam ansehen** auf der Anmeldeseite.

Wer den Film für alle anhalten oder vorspulen kann, hängt von der App ab:
siehe [Was jedes Mitglied steuert](#what-each-member-controls).

Jede Person bekommt trotzdem die Fassung des Films, die zu ihrem eigenen
Gerät passt, wie in
[Wie Evey die Wiedergabeart wählt](how-playback-is-decided.md) erklärt.

## Einen Raum eröffnen und beitreten

| | Web | Android | Android TV |
| --- | --- | --- | --- |
| Raum eröffnen | **Zusammen schauen** im Player | **Zusammen schauen** im Player | - |
| Mit Konto beitreten | Der Link des Raums oder der Code unter **Gemeinsam hören** → **Raum beitreten** | Der Code unter **Gemeinsam hören** | Der Code unter **Einstellungen** → **Zusammen schauen** |
| Ohne Konto beitreten | **Gemeinsam ansehen** auf der Anmeldeseite, mit dem Code | Genauso | - |

Beim Eintippen des Codes ist Evey großzügig: Groß- oder Kleinschreibung,
Leerzeichen, `O` statt `0` oder `I` oder `L` statt `1` werden alle
akzeptiert.

Mit einem Konto beizutreten folgt den üblichen Regeln: Die Bibliothek muss
für dieses Konto freigegeben sein, und die Kindersicherung des Profils muss
den Titel erlauben. Ein eingeschränktes Profil kann auch keinen Raum für
einen Titel eröffnen, den es nicht schauen darf.

## Was jedes Mitglied steuert

| | Gastgeber | Andere Mitglieder |
| --- | --- | --- |
| Android | Abspielen, Pause und Spulen bewegen den ganzen Raum. | Abspielen, Pause und Spulen bleiben auf Ihrem eigenen Bildschirm. |
| Android TV | Abspielen und Pause bewegen den ganzen Raum; Spulen bleibt auf dem Fernseher. | Wie bei Android. |
| Web | Abspielen, Pause und Spulen bleiben auf Ihrem eigenen Bildschirm, auch beim Gastgeber. | Genauso. |

Im Web-Player ersetzen eigene Schaltflächen die Steuerung durch den
Gastgeber. **Alle synchronisieren** holt den ganzen Raum an Ihre Stelle und
setzt die Wiedergabe fort; jedes Mitglied kann es benutzen, nicht nur der
Gastgeber. **Aufholen** bringt Sie dorthin zurück, wo der Raum gerade ist,
ohne jemand anderen zu bewegen; es wird auf der Gastseite angeboten (mit
einem Code beigetreten, ohne Konto), nicht im Player eines angemeldeten
Kontos.

Wer später dazukommt, startet dort, wo der Raum jetzt ist, nicht dort, wo er
beim letzten Befehl war.

Verlässt der Gastgeber den Raum, wird das Mitglied Gastgeber, das am längsten
im Raum ist. Geht das letzte Mitglied, schließt der Raum, und der Gastzugang,
den sein Code geöffnet hat, wird entzogen. Ein Raum, den nie jemand betreten
hat, wird nach einer Stunde entfernt.

## Gäste ohne Konto

Ein Gast, der auf der Anmeldeseite einen Code eingibt, schaut genau diesen
einen Titel und sonst nichts:

- nur Streaming, nie Herunterladen;
- nur Video-Räume, keine Musik-Räume;
- der Zugang hält, solange er genutzt wird, verfällt nach drei Stunden ohne
  Wiedergabe und in jedem Fall nach 24 Stunden, und endet, sobald der Raum
  schließt.

!!! warning "Achtung"
    Der Code ist das Einzige, was einen Raum schützt: Geben Sie ihn nur den
    Personen, die Sie dabeihaben wollen. Falsche Codes sind pro Adresse und
    für den ganzen Server begrenzt, sodass man sie praktisch nicht erraten
    kann.

## Gemeinsam hören

Ein Musik-Raum wird über **Gemeinsam hören** eröffnet, mit einer
Warteschlange aus Alben, Titeln und Playlists. Der Gastgeber spielt ab,
pausiert und springt in der Warteschlange weiter; jedes Mitglied folgt.
Gäste ohne Konto können Musik-Räumen nicht beitreten.
