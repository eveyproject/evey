---
source_sha: 87e08f341b329e7354b9d42ef5972fac5cb564784a1488786bff878e7d74ba8a
reviewed: true
---
# Audio

**Kurz gesagt:**

- Sie hören die Sprache Ihres Profils, wenn der Film sie hat: Stellen Sie sie
  unter **Einstellungen → Wiedergabe → Standard-Audiosprache** ein. Eine Spur,
  die Sie im Player von Hand wählen, gewinnt immer.
- Der Ton wird so gesendet, wie er ist, wenn Ihr Gerät ihn abspielen kann.
  Sonst wandelt der Server ihn in ein gängiges Format um (AAC). Oft ist der
  Ton das Einzige, was umgewandelt wird, und das Bild bleibt unverändert.
- Über den Lautsprecher eines Handys, über Kopfhörer oder Bluetooth spielt die
  Android-App Stereo. Mit der App für Android oder Android TV über HDMI
  bekommen Sie so viele Kanäle, wie der Fernseher oder der Verstärker annimmt
  (zum Beispiel 5.1).
- Ton und Bild laufen nicht synchron? Siehe
  [Ton nicht synchron](#sound-out-of-sync).

Der Rest dieser Seite erklärt, welche Spur läuft, wann sie kopiert oder
umgewandelt wird und mit wie vielen Kanälen. Die Profileinstellungen, die
hier genannt werden, finden Sie unter **Einstellungen → Wiedergabe**.

## Was Sie hören, je nach Gerät

| Ihr Gerät | Was ankommt |
| --- | --- |
| Ein Browser | Die Spur, wie sie ist, wenn der Browser sie dekodiert; AC-3 und E-AC-3 werden in der Seite dekodiert (siehe unten); sonst AAC. Eine AAC-Spur mit Raumklang mischt der Browser auf Ihre Ausgabe herunter. |
| Ein Android-Handy über seinen Lautsprecher, Kopfhörer oder Bluetooth | Stereo. Die App meldet eine Stereo-Ausgabe. Eine Spur mit Raumklang kodiert der Server daher in Stereo neu, mit einer ausgeglichenen Abmischung. Dolby Digital, das die App selbst dekodiert, wird gesendet, wie es ist, und von der App heruntergemischt. |
| Ein Android-Handy oder Android TV über HDMI | So viele Kanäle, wie das HDMI-Gerät nach eigener Meldung annimmt: Eine 5.1-Spur bleibt auf einem 5.1-Verstärker 5.1. |
| Ein Cast-Empfänger | Was der Empfänger über seine Ausgabe gemeldet hat: siehe [Streamen auf Chromecast](casting.md). |

Die Android-Apps prüfen die Ausgabe bei jedem Start einer Wiedergabe:
Schließen Sie Kopfhörer an, wirkt das beim nächsten Titel, beim nächsten
Spurwechsel oder bei der nächsten Änderung der Qualität, nicht mitten in
einer Szene.

## Welche Spur läuft

Eine Spur, die Sie von Hand wählen, auf der Titelseite oder im Player,
gewinnt immer. Sonst folgen die Player der **Standard-Audiosprache** des
Profils, dann der Spur, die die Datei als Standard markiert, dann der ersten.
Hat die Datei eine Kommentarspur und eine normale Spur, wird die
Kommentarspur übersprungen.

Beurteilt wird nur die Spur, die gerade läuft: Eine DTS-Spur, die Sie nicht
hören, löst nie eine Transkodierung aus.

## Kopiert oder neu kodiert

Die Spur wird kopiert, wenn der Player ihren Codec dekodiert und seine
Ausgabe genug Kanäle dafür hat, im Rahmen von **Maximale Audio-Kanäle** im
Profil (**Stereo (2.0)**, **Surround (5.1)** oder **Surround (7.1)**). Sonst
wird sie in AAC neu kodiert:

- mit so vielen Kanälen, wie Quelle, Gerät und Profil erlauben, höchstens 5.1
  (eine 7.1-Quelle wird auf 5.1 heruntergemischt, das jeder Player dekodiert;
  Stereo-Quellen bleiben Stereo);
- unabhängig von der Abtastrate. Sie allein ist nie ein Grund für eine
  Neukodierung.

Eine kopierte Spur in einem transkodierten oder umverpackten Stream muss
außerdem ein Format haben, das der Player in den Segmenten des Streams
annimmt. AAC, AC-3 und E-AC-3 gehen immer; andere Codecs nur, wenn der Player
es meldet. Sonst wird die Spur neu kodiert, und Stream-Info zeigt
**Audiospur kann im Stream nicht unverändert übertragen werden**: Ein Player
kann MP3 in einer Datei dekodieren und es in einem Stream trotzdem ablehnen.

## Dolby Digital im Browser

Die meisten Browser können AC-3 oder E-AC-3 nicht dekodieren. Statt die Spur
auf dem Server neu zu kodieren, dekodiert der Web-Player sie selbst, mit einem
Decoder, der in der Seite läuft: Die Spur wird unverändert gesendet, und
Stream-Info zeigt sie als *clientseitig dekodiert*. Browser, die Dolby
Digital selbst dekodieren, etwa Safari, erhalten sie als gewöhnliche Spur. Ob
ein Browser das kann, wird getestet, nicht an seinem Namen erkannt. Eine Spur
mit Raumklang wird nur dann in der Seite dekodiert, wenn **Maximale
Audio-Kanäle** im Profil es erlaubt: Ein Profil, das auf Stereo steht,
bekommt stattdessen die Neukodierung in AAC, die die Kanäle heruntermischen
kann.

!!! warning "Achtung"
    Ein Reverse Proxy, der seine eigene Content-Security-Policy setzt, muss
    `'wasm-unsafe-eval'` in `script-src` erlauben. Sonst kann der Decoder
    nicht starten, und der Film läuft ohne Ton. Siehe
    [Reverse-Proxy-Konfigurationen](../reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own).

## Ton nicht synchron

**Audio-Synchronisation** im Player verschiebt den Ton für den aktuellen
Titel (positive Werte spielen ihn später ab). Sie wird angeboten, wenn der
Stream vom Server als Remux oder Transkodierung kommt; bei Direktwiedergabe
zeigt das Bedienelement **Nur bei transkodierter Wiedergabe verfügbar**. Eine
Verschiebung startet den Stream an der aktuellen Stelle neu. Eine kopierte
Spur lässt sich nicht verschieben. Der Server kodiert sie daher neu, und
Stream-Info zeigt **Audio-Resynchronisation angefordert**.

Liegt der Versatz in der Datei selbst (eine Tonspur, die nach dem Bild
beginnt, oder Spuren, die nicht zueinander passen), meldet ihn **Verwaltung →
Medienzustand**, schreibt ihn aber nie von selbst um: Ein Ton, der später
beginnt, ist meist eine gewollte Ausrichtung einer Synchronfassung. Mit
**Synchronität prüfen** in der Zeile der Datei beurteilen Sie ihn mit eigenen
Augen und wenden eine Verschiebung an, die sich über den Verlauf der Datei
rückgängig machen lässt. Siehe
[Medienzustand](../runbooks/media-health.md#checking-the-sync-by-eye).

<!-- sources
track choice: internal/app/transcoding/decision.go - selectedAudioSpec ; frontend/packages/client/src/hooks/usePlayerTrackSelection.ts
copy vs aac, channels: internal/app/transcoding/decision.go - evaluateAudio, applyAudioChannelTarget (aacMaxTargetChannels)
device channel ceiling: frontend/packages/client/src/native/outputChannels.ts - resolveMaxOutputChannels (STEREO_ONLY_TYPES, hdmi.maxChannelCount) ; frontend/packages/client/src/native/playSession.ts - playerDecodesCopiedAudio ; internal/adapters/driving/http/client_capabilities.go - maxAudioChannelsQuery
passthrough in segments: internal/app/transcoding/decision.go - applyPackagedAudioPassthrough ; internal/app/transcoding/source_facts.go - AudioCopyCodecAllowedFor ; internal/domain/transcoding/playback_decision.go - ClientProfile.FMP4AudioPassthrough
wasm: internal/app/transcoding/wasm_audio.go - WasmAudioCodecSupported, WasmAudioChannelsOK ; internal/app/transcoding/decision.go - wasmAudioApplies ; frontend/packages/client/src/web/codec.ts
sample rate: internal/app/transcoding/decision.go - evaluateAudio (no sample-rate check)
resync: internal/app/transcoding/playback_plan_resolver.go - Decide (audio_resync_requested) ; frontend/apps/web/src/features/playback/HLSPlayer.tsx (audioOffsetAvailable=transcodeMode) ; frontend/apps/mobile/app/(auth)/player/[id].tsx (audioOffsetAvailable)
media health: i18n admin.syncPreview.btn ; docs/runbooks/media-health.md
labels: i18n auth.settings.playback.maxAudioChannels, channelsStereo, channels51, channels71, audioLanguage ; player.settings.audioSync, audioSyncHint, audioSyncUnavailable ; player.streamInfo.decisionWasm, reason.audio_passthrough_unsupported, reason.audio_resync_requested
plain opening: frontend/packages/client/src/hooks/usePlayerTrackSelection.ts ; i18n auth.settings.playback.audioLanguage
-->
