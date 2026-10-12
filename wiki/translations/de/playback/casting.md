---
source_sha: 8c3a42e5732d38c6b13e080b8869dff85cef64c692ff15ed7ef067e6d0e686e9
reviewed: true
---
# Streamen auf Chromecast

Mit Streamen schicken Sie einen Film von Ihrem Handy oder Computer auf den
Fernseher.

1. Starten Sie den Film im Web-Player in Chrome oder in der Android-App.
2. Drücken Sie im Player auf **Streamen**.
3. Wählen Sie Ihren Chromecast oder Ihren Fernseher mit eingebautem
   Chromecast.

Der Film läuft dann auf dem Fernseher. Die Android-TV-App hat keine
Schaltfläche **Streamen**: Sie ist ja schon der Fernseher. Auch Firefox zeigt
keine Schaltfläche **Streamen**. Mit Safari oder mit Chrome auf einem
Android-Handy funktioniert die Schaltfläche anders: siehe
[Browser ohne eigene Cast-Sitzung](#browsers-without-a-cast-session-of-their-own).

Das Bild auf dem Fernseher kann anders aussehen als auf Ihrem Handy, und das
ist normal. Der Fernseher bekommt nicht das, was Ihr Handy abgespielt hat: Er
holt den Film selbst vom Server, und der Server bereitet eine Fassung vor, die
zum Fernseher passt. Der Rest dieser Seite erklärt, wie, für Neugierige und
für die Person, die Evey betreibt.

## Was der Fernseher abspielt

Der Stream wird vom Fernseher dekodiert. Die Entscheidung über die Wiedergabe
richtet sich also nach den Fähigkeiten des Empfängers, aus der
zuverlässigsten verfügbaren Quelle:

1. **Was der Empfänger an sich selbst festgestellt hat.** Eveys Empfänger
   testet sein eigenes Gerät beim Start (welche Codecs und Auflösungen es
   anzeigen kann, ob es HDR anzeigt, ob es Dolby-Ton unverändert weitergibt,
   wie viele Tonkanäle sein Ausgang trägt) und meldet das. Kommt diese
   Meldung an, wird sie verwendet.
2. **Sonst das Modell.** Der Name, den das Cast SDK meldet (Chromecast mit
   Google TV, Chromecast Ultra, Google TV Streamer...), entspricht den
   dokumentierten Fähigkeiten dieser Gerätefamilie.
3. **Sonst ein vorsichtiges Profil.** Ein unbekanntes Gerät, ein Empfänger,
   der nicht rechtzeitig geantwortet hat, oder ein Gerät ohne Modellnamen
   bekommt die Fähigkeiten des ältesten Chromecast: H.264 bis 1080p, SDR,
   kein Dolby.

Die Reihenfolge folgt dem Preis eines Fehlers: Wer dem Gerät eine Fähigkeit
zutraut, die es nicht hat, bekommt einen schwarzen Bildschirm; wer zu wenig
annimmt, verliert nur eine Transkodierung. In der Praxis: HDR kommt nur dann
beim Fernseher an, wenn sein Empfänger einen HDR-Bildschirm gemeldet hat;
Dolby Digital (AC-3, E-AC-3) wird nur an einen Empfänger unverändert
geschickt, der gemeldet hat, dass er es direkt weitergibt, sonst wird es in
AAC umgewandelt. Ein Empfänger, der Surround angegeben hat und den Stream dann
nicht laden konnte, lädt ihn einmal neu, mit einer Obergrenze von Stereo.

## Wenn die Verbindung des Fernsehers langsam ist

Bevor er einen Stream anfordert, misst der Empfänger seinen eigenen Durchsatz
und schickt ihn mit der Anfrage. Der Server behält das Originalvideo, wenn
dieser Durchsatz es mit 20 % Reserve trägt. Sonst wählt er die höchste
Qualität, die die Verbindung trägt. Stockt die Wiedergabe trotzdem, lädt der
Empfänger einmal mit der niedrigsten Qualität neu, sofern das die Bitrate
halbiert. Die Stream-Info zeigt dann **Die vom Client gemessene Leitung trägt
die Kopie nicht** oder **Vom Player angeforderte Notfallstufe**.

Ältere Cast-Geräte wechseln während der Wiedergabe nicht selbst die
Qualität; deshalb trifft der Server diese Wahl für sie.

## Browser ohne eigene Cast-Sitzung

Im Web-Player erscheint die Schaltfläche **Streamen**, wenn der Browser
streamen kann: Chrome auf einem Computer, das seine Google-Cast-Auswahl
öffnet, und Browser, die Wiedergabe auf anderen Geräten unterstützen, wie
Safari oder Chrome auf Android. Firefox zeigt keine.

Wo der Browser selbst keine Cast-Sitzung öffnen kann (Safari, Chrome auf
Android), läuft ein Tippen über den Server. Er probiert in dieser Reihenfolge
und nimmt bei jedem Schritt das erste Gerät, das er findet:

1. einen Chromecast im Netz des Geräts, das Sie in der Hand halten, abgespielt
   über Eveys Empfänger wie oben;
2. einen DLNA-Fernseher im Netz des Servers. Er bekommt den Stream, den der
   Browser abgespielt hat (die Originaldatei bei Direktwiedergabe, sonst den
   transkodierten Stream): Für den Fernseher selbst wird nichts entschieden;
3. Ihre eigenen Evey-Apps, die geöffnet und mit demselben Konto angemeldet
   sind: Ist es eine einzige, übernimmt sie den Film, wie mit **Abspielen
   auf**; sind es mehrere, verweist der Player auf **Abspielen auf**, damit
   Sie wählen;
4. die eigene Geräteauswahl des Browsers.

!!! note "Hinweis"
    Die Schritte 1 und 2 nehmen das erste gefundene Gerät, nicht eines, das
    Sie wählen. Haben Sie zu Hause mehrere Chromecasts, streamen Sie aus
    Chrome auf einem Computer oder aus der Android-App: Beide zeigen eine
    Auswahl.

## Wenn das Streamen nicht funktioniert

[A cast froze, went silent, or the phone keeps spinning](../runbooks/incident-runbook.md#a-cast-froze-went-silent-or-the-phone-keeps-spinning).
