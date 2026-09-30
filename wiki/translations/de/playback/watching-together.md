---
source_sha: a6742845a66194948de961b0490bc70821d9e1a06b1c6ff4aaed96f02c9910ba
reviewed: false
---
# Zusammen schauen

**Zusammen schauen** hält mehrere Personen am selben Titel im selben Moment,
jede auf ihrem eigenen Gerät: Ein Raum hält die Position, und jeder Player
folgt ihr. Der Stream jeder Person wird trotzdem für ihr eigenes Gerät
entschieden, wie unter [Wie Evey die Wiedergabeart wählt](how-playback-is-decided.md):
Eine Person kann in Direktwiedergabe schauen, während eine andere transkodiert
wird.

## Einen Raum erstellen und beitreten

| | Web | Android | Android TV |
| --- | --- | --- | --- |
| Einen Raum erstellen | **Zusammen schauen** im Player | **Zusammen schauen** im Player | - |
| Mit einem Konto beitreten | Der Link des Raums, oder der Code unter **Gemeinsam hören** → **Raum beitreten** | Der Code unter **Gemeinsam hören** | Der Code unter **Einstellungen** → **Zusammen schauen** |
| Ohne Konto beitreten | **Gemeinsam ansehen** auf der Anmeldeseite, mit dem Code | Ebenso | - |

Wer den Raum erstellt, ist sein Gastgeber und sieht seinen Code: sechs Zeichen,
Buchstaben und Ziffern. Der Code wird eingegeben, ohne auf Groß- und
Kleinschreibung, Leerzeichen oder die Verwechslung von `O` mit `0` und von `I`
oder `L` mit `1` achten zu müssen.

Der Beitritt mit einem Konto folgt den üblichen Regeln: Die Bibliothek muss für
dieses Konto freigegeben sein, und die Kindersicherung des Profils muss den
Titel erlauben. Ein eingeschränktes Profil kann auch keinen Raum für einen
Titel erstellen, den es nicht sehen darf.

## Gäste ohne Konto

Ein Gast, der auf der Anmeldeseite einen Code eingibt, sieht diesen einen Titel
und nichts anderes:

- nur als Stream, nie als Download;
- nur in Videoräumen, nicht in Musikräumen;
- der Zugang besteht, solange er genutzt wird, erlischt nach drei Stunden ohne
  Wiedergabe und in jedem Fall nach 24 Stunden, und endet, sobald der Raum
  geschlossen wird.

Der Code ist das Einzige, was einen Raum schützt: Geben Sie ihn nur den
Personen, die Sie darin haben wollen. Falsche Codes werden pro Adresse und über
den ganzen Server begrenzt, was das Erraten außer Reichweite hält.

## Was synchron bleibt

- **Auf Android und Android TV** bewegen Wiedergabe, Pause und Sprünge des
  Gastgebers den ganzen Raum. Die Bedienung der anderen Mitglieder betrifft
  niemanden sonst.
- **Im Web-Player** bleiben Pause und Sprünge auf Ihrem eigenen Bildschirm.
  **Aufholen** bringt Sie dorthin zurück, wo der Raum gerade ist, ohne jemand
  anderen zu bewegen. **Alle synchronisieren** bringt den ganzen Raum zu Ihrer
  Position; jedes Mitglied kann es nutzen, nicht nur der Gastgeber.

Wer später dazukommt, startet dort, wo der Raum jetzt ist, nicht dort, wo er
beim letzten Befehl war.

Verlässt der Gastgeber den Raum, wird das Mitglied Gastgeber, das am längsten
im Raum ist. Geht das letzte Mitglied, wird der Raum geschlossen, und der
Gastzugang, den sein Code geöffnet hat, wird entzogen. Ein Raum, den nie jemand
betreten hat, wird nach einer Stunde entfernt.

## Gemeinsam hören

Ein Musikraum wird unter **Gemeinsam hören** mit einer Warteschlange aus Alben,
Titeln und Playlists erstellt. Der Gastgeber spielt ab, pausiert und bewegt sich
durch die Warteschlange; jedes Mitglied folgt. Gäste ohne Konto können
Musikräumen nicht beitreten.
