---
source_sha: 3bd3ddf0df0a475d6ddf4417b5aa5b3630c871184888584df522bdae5daee7d4
reviewed: true
---
# Auslieferung und Downloads

Die Schalter dafür, wie Streams verpackt werden und was der Server für
Downloads vorbereitet. Die Standardwerte sind das empfohlene Verhalten: Diese
Schalter dienen dazu, einen Weg abzuschalten, während Sie einem Problem
nachgehen, oder einen ausgeschalteten Weg bewusst einzuschalten. Wie ein
Stream überhaupt gewählt wird:
[Wie Evey die Wiedergabeart wählt](../playback/how-playback-is-decided.md).

Eine Variable mit einem Schlüssel im Verwaltungsbereich lässt sich auch als
Zeile mit diesem Schlüssel unter **Verwaltung → Serverkonfiguration →
Konfigurationseinträge** setzen. Die Zeile hat Vorrang vor der Umgebung.
Der Verwaltungsbereich schlägt die Download-Schlüssel vor; die anderen tippen
Sie so ein, wie sie hier stehen. Alle werden beim Start gelesen: Eine Änderung
gilt also ab dem nächsten Neustart, egal auf welchem Weg sie gemacht wurde.

## Verpackung

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_HLS_ONDEMAND` | an | Liefert eine Datei, deren Spuren kopiert werden können, indem sie im laufenden Betrieb neu verpackt wird, ohne ffmpeg und ohne in den Cache zu schreiben. `off`: Solche Dateien gehen stattdessen über den Kopierweg von ffmpeg. | Konfigurationseinträge `transcoding.hls_ondemand`, Neustart |
| `EVEY_REMUX_COPY` | an | Der Kopierweg von ffmpeg: Das Video wird ohne neue Kodierung kopiert, wenn die Datei es erlaubt. `off`: Diese Dateien werden transkodiert. | Konfigurationseinträge `transcoding.remux_copy`, Neustart |
| `EVEY_HLS_AUDIO_RENDITION_ENABLED` | `true` | Bietet die Tonspuren als eigene Renditionen an, damit ein Player die Sprache wechseln kann, ohne das Video neu zu laden. Erlaubt auch, ein kopiertes Video mit einer nebenher neu kodierten Tonspur abzuspielen. | keine |
| `EVEY_HLS_COPY_FALLBACK_RUNG` | `true` | Fügt neben einem kopierten Stream eine niedrige transkodierte Qualität hinzu. So kann ein Player, dessen Verbindung einbricht, eine Stufe heruntergehen, statt anzuhalten. Kostet nichts, solange kein Player sie nutzt. | keine |
| `EVEY_HLS_ONDEMAND_CONCAT` | an | Lückenlose Wiedergabe über Folgen hinweg, wenn der Web-Player sie anfordert. Eine Folge, die nicht kopiert werden kann, fällt auf den normalen Stream zurück. | Konfigurationseinträge `transcoding.hls_ondemand_concat`, Neustart |
| `EVEY_HLS_ONDEMAND_TRACK_TRIM` | aus | Packt nur die gewählte Tonspur in jeden Stream statt aller. | Konfigurationseinträge `transcoding.hls_ondemand_track_trim`, Neustart |
| `EVEY_HLS_ONDEMAND_ENCRYPT` | aus | Verschlüsselt neu verpackte Streams mit AES-128 (nur HLS: Dann wird kein DASH-Manifest angeboten). | Konfigurationseinträge `transcoding.hls_ondemand_encrypt`, Neustart |
| `EVEY_HLS_KEY_ROTATION_SEGMENTS` | `0` | Wechselt bei eingeschalteter Verschlüsselung den Schlüssel alle N Segmente; `0` = ein Schlüssel pro Titel. | Konfigurationseinträge `transcoding.hls_key_rotation_segments`, Neustart |
| `EVEY_CAST_WASM_AUDIO_ENABLED` | `false` | Lässt einen Chromecast Dolby Digital selbst dekodieren, statt AAC zu empfangen. Aus, weil es schwache Empfänger überlastet hat, ohne etwas einzusparen. Browser sind nicht betroffen. | keine |

## Downloads

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_COMPAT_DOWNLOAD` | an | Bereitet einen Download vor, dessen Ton ein Handy abspielen kann (AAC), das Video bleibt unverändert. Nur der Ton wird neu kodiert, was einige Minuten dauert. | Konfigurationseinträge `transcoding.compat_download`, Neustart |
| `EVEY_DOWNLOAD_QUALITY_CHOICE` | aus | Bietet für Downloads eine kleinere Kopie in 720p an. Dafür wird das Video neu kodiert, auf einer schwachen Maschine etwa in Echtzeit. Deshalb ist es aus, bis Sie es einschalten. | Konfigurationseinträge `downloads.quality_choice`, Neustart |
| `EVEY_DOWNLOAD_REDUCE_ABOVE_BYTES` | `4294967296` (4 GiB) | Die Größe, ab der die App die kleinere Kopie anbietet; das Handy bietet sie auch an, wenn der Film nicht hineinpasst. | Konfigurationseinträge `downloads.reduce_above_bytes`, Neustart |

## Player

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_PLAYBACK_SKIP_FORWARD` | `30` | Sekunden, die die Vorwärts-Taste überspringt. | keine |
| `EVEY_PLAYBACK_SKIP_BACKWARD` | `30` | Sekunden, die die Zurück-Taste überspringt. | keine |

<!-- sources
packaging flags: internal/adapters/driving/http/transcode_vod_wire.go (serverFlagEnabled, resolveServerFlag: hls_ondemand, remux_copy, hls_ondemand_concat, hls_ondemand_track_trim, hls_ondemand_encrypt, hls_key_rotation_segments)
renditions, fallback rung, cast wasm: internal/config/config.go - AudioRenditionEnabled, CopyFallbackRungEnabled, CastWasmAudioEnabled
downloads: cmd/evey/http.go (compat_download, quality_choice, reduce_above_bytes via resolveFlag)
config entries catalogue: frontend/apps/web/src/features/admin/operatorFlags.ts ; i18n admin.serverConfig.entriesTitle, appliedOnRestart
skip: internal/config/config.go - EVEY_PLAYBACK_SKIP_*
-->
