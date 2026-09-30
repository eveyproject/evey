---
source_sha: ae7eadc681e514dab83967eab7a3ef8dfadf53af27b3d1e4c676d5dd2d1176ec
reviewed: false
---
# Auslieferung und Downloads

Wie Streams verpackt werden und was der Server für Downloads vorbereitet. Die
Standards sind das empfohlene Verhalten; diese Schalter gibt es, um bei der
Untersuchung eines Problems einen Weg abzuschalten oder einen ausgeschalteten
zu aktivieren.

Eine Variable mit Schlüssel lässt sich auch als Zeile mit diesem Schlüssel unter
**Admin → Serverkonfiguration → Konfigurationseinträge** setzen; die Zeile hat
Vorrang vor der Umgebung. Der Admin-Bereich schlägt die Download-Schlüssel vor;
die anderen werden so eingegeben, wie sie hier stehen. Alle werden beim Start
gelesen.

## Verpackung

| Variable | Schlüssel im Admin-Bereich | Standard | Wirkung |
| --- | --- | --- | --- |
| `EVEY_HLS_ONDEMAND` | `transcoding.hls_ondemand` | an | Liefert eine Datei, deren Spuren kopiert werden können, durch Neuverpackung im Flug aus, ohne ffmpeg und ohne in den Cache zu schreiben. Aus: Solche Dateien gehen stattdessen über den ffmpeg-Kopierweg. |
| `EVEY_REMUX_COPY` | `transcoding.remux_copy` | an | Der ffmpeg-Kopierweg: Das Video wird ohne Neukodierung kopiert, wenn die Datei es erlaubt. Aus: Diese Dateien werden transkodiert. |
| `EVEY_HLS_AUDIO_RENDITION_ENABLED` | | `true` | Bietet die Audiospuren als eigene Varianten an, sodass ein Player die Sprache wechselt, ohne das Video erneut zu holen. Lässt außerdem ein kopiertes Video mit einer nebenher neu kodierten Audiospur laufen. |
| `EVEY_HLS_COPY_FALLBACK_RUNG` | | `true` | Fügt neben einem kopierten Stream eine niedrige transkodierte Qualität hinzu, damit ein Player, dessen Verbindung einbricht, heruntergehen kann, statt einzufrieren. Kostet nichts, bis ein Player sie nutzt. |
| `EVEY_HLS_ONDEMAND_CONCAT` | `transcoding.hls_ondemand_concat` | an | Lückenlose Wiedergabe über Folgen hinweg, wenn der Web-Player danach fragt. Eine Folge, die nicht kopiert werden kann, fällt auf den normalen Stream zurück. |
| `EVEY_HLS_ONDEMAND_TRACK_TRIM` | `transcoding.hls_ondemand_track_trim` | aus | Legt nur die gewählte Audiospur in jeden Stream statt aller. |
| `EVEY_HLS_ONDEMAND_ENCRYPT` | `transcoding.hls_ondemand_encrypt` | aus | Verschlüsselt neu verpackte Streams mit AES-128 (nur HLS: Dann wird kein DASH-Manifest angeboten). |
| `EVEY_HLS_KEY_ROTATION_SEGMENTS` | `transcoding.hls_key_rotation_segments` | `0` | Bei eingeschalteter Verschlüsselung wechselt der Schlüssel alle N Segmente; `0` = ein Schlüssel pro Titel. |
| `EVEY_CAST_WASM_AUDIO_ENABLED` | | `false` | Lässt einen Chromecast Dolby Digital selbst dekodieren, statt AAC zu erhalten. Aus, weil es schwache Empfänger überlastete, ohne etwas einzusparen. Browser sind nicht betroffen. |

## Downloads

| Variable | Schlüssel im Admin-Bereich | Standard | Wirkung |
| --- | --- | --- | --- |
| `EVEY_COMPAT_DOWNLOAD` | `transcoding.compat_download` | an | Bereitet einen Download vor, dessen Audio ein Telefon abspielen kann (AAC), mit unverändertem Video. Nur das Audio wird neu kodiert, was Minuten dauert. |
| `EVEY_DOWNLOAD_QUALITY_CHOICE` | `downloads.quality_choice` | aus | Bietet für Downloads eine kleinere 720p-Kopie an. Sie kodiert das Video neu, auf einer schwachen Maschine etwa in Echtzeit, deshalb ist sie aus, bis Sie sie einschalten. |
| `EVEY_DOWNLOAD_REDUCE_ABOVE_BYTES` | `downloads.reduce_above_bytes` | `4294967296` (4 GiB) | Die Größe, ab der die App die kleinere Kopie anbietet; das Telefon bietet sie auch an, wenn der Film nicht passt. |

## Player

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_PLAYBACK_SKIP_FORWARD` | `30` | Sekunden, die die Vorwärtstaste überspringt. |
| `EVEY_PLAYBACK_SKIP_BACKWARD` | `30` | Sekunden, die die Rückwärtstaste überspringt. |
