---
source_sha: 66951bf73de4e59bd21b6ea9468df82ac79d31950e742e6c87203c05a77dda76
reviewed: false
---
# Streamen auf Chromecast

Die Schaltfläche **Streamen** im Web-Player in Chrome und in der Android-App
sendet den Film an einen Chromecast oder einen Fernseher mit eingebautem
Chromecast. Das Telefon oder der Browser startet nur die Sitzung: Der Fernseher
spielt den Stream direkt vom Server ab, über Eveys eigene Empfängeranwendung,
und der Server entscheidet, was er für den Fernseher sendet, nicht für das
Gerät, das gestreamt hat.

## Was der Server über den Empfänger weiß

Der Stream wird vom Fernseher dekodiert, deshalb nutzt die
Wiedergabeentscheidung die Fähigkeiten des Empfängers, aus der
zuverlässigsten verfügbaren Quelle:

1. **Was der Empfänger an sich selbst beobachtet hat.** Eveys Empfänger testet
   beim Start sein eigenes Gerät (welche Codecs und Auflösungen es anzeigen
   kann, ob es HDR darstellt, ob es Dolby-Audio durchreicht, wie viele
   Audiokanäle sein Ausgang trägt) und meldet das. Kommt diese Meldung an, wird
   sie verwendet.
2. **Sonst das Modell.** Der Name, den das Cast-SDK meldet (Chromecast mit
   Google TV, Chromecast Ultra, Google TV Streamer...), wird den dokumentierten
   Fähigkeiten dieser Familie zugeordnet.
3. **Sonst ein vorsichtiges Profil.** Ein unbekanntes Gerät, ein Empfänger, der
   nicht rechtzeitig geantwortet hat, oder ein Gerät ohne Modellnamen erhält die
   Fähigkeiten des ältesten Chromecasts.

Die Reihenfolge folgt den Kosten eines Irrtums: Eine Fähigkeit anzunehmen, die
das Gerät nicht hat, ergibt einen schwarzen Bildschirm, während zu wenig
anzunehmen nur eine Transkodierung kostet.

Dolby Digital (AC-3, E-AC-3) wird nur an einen Empfänger unverändert gesendet,
der gemeldet hat, dass er es nativ durchreicht; sonst wird es nach AAC neu
kodiert.

## Die Verbindung des Fernsehers

Bevor er einen Stream anfordert, misst der Empfänger seinen eigenen Durchsatz
und sendet ihn mit der Anfrage. Der Server behält das Originalvideo, wenn
dieser Durchsatz es mit 20 % Reserve trägt. Sonst wählt er die höchste
Qualität, die die Verbindung trägt. Stockt die Wiedergabe trotzdem, lädt der
Empfänger einmal mit der niedrigsten Qualität neu, sofern das die Bitrate
halbiert. Stream-Info zeigt dann **Die vom Client gemessene Leitung trägt die
Kopie nicht** oder **Vom Player angeforderte Notfallstufe**.

Ältere Cast-Geräte wechseln die Qualität während der Wiedergabe nicht von
selbst; deshalb trifft der Server diese Wahl für sie.

## Wenn das Streamen nicht funktioniert

[Ein Cast ist eingefroren, stumm geworden, oder das Telefon dreht sich endlos](../runbooks/incident-runbook.md#8-a-cast-froze-went-silent-or-the-phone-keeps-spinning).
