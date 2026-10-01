---
source_sha: ae7eadc681e514dab83967eab7a3ef8dfadf53af27b3d1e4c676d5dd2d1176ec
reviewed: false
---
# Entrega de la reproducción y descargas

Cómo se empaquetan los flujos y qué prepara el servidor para las descargas. Los
valores por defecto son el comportamiento recomendado; estos interruptores
existen para desactivar una vía mientras se investiga un problema, o para activar
una que está desactivada.

Una variable con clave también se puede definir como una fila con esa clave en
**Admin → Configuración del servidor → Entradas de configuración**; la fila
prevalece sobre el entorno. El panel sugiere las claves de descarga; las demás se
escriben tal como aparecen aquí. Todas se leen al arrancar.

## Empaquetado

| Variable | Clave del panel | Por defecto | Efecto |
| --- | --- | --- | --- |
| `EVEY_HLS_ONDEMAND` | `transcoding.hls_ondemand` | activado | Sirve un archivo cuyas pistas se pueden copiar reempaquetándolo sobre la marcha, sin ffmpeg y sin escribir en la caché. Desactivado: esos archivos pasan en su lugar por la vía de copia de ffmpeg. |
| `EVEY_REMUX_COPY` | `transcoding.remux_copy` | activado | La vía de copia de ffmpeg: el vídeo se copia sin recodificar cuando el archivo lo permite. Desactivado: esos archivos se transcodifican. |
| `EVEY_HLS_AUDIO_RENDITION_ENABLED` | | `true` | Ofrece las pistas de audio como renditions aparte, para que un reproductor cambie de idioma sin volver a descargar el vídeo. También permite que un vídeo copiado se reproduzca con una pista de audio recodificada aparte. |
| `EVEY_HLS_COPY_FALLBACK_RUNG` | | `true` | Añade una calidad baja transcodificada junto a un flujo copiado, para que un reproductor cuya conexión se hunde pueda bajar en lugar de congelarse. No cuesta nada hasta que un reproductor la usa. |
| `EVEY_HLS_ONDEMAND_CONCAT` | `transcoding.hls_ondemand_concat` | activado | Reproducción sin cortes entre episodios, cuando el reproductor web la pide. Un episodio que no se puede copiar vuelve al flujo normal. |
| `EVEY_HLS_ONDEMAND_TRACK_TRIM` | `transcoding.hls_ondemand_track_trim` | desactivado | Pone en cada flujo solo la pista de audio seleccionada en lugar de todas. |
| `EVEY_HLS_ONDEMAND_ENCRYPT` | `transcoding.hls_ondemand_encrypt` | desactivado | Cifra los flujos reempaquetados con AES-128 (solo HLS: entonces no se ofrece manifiesto DASH). |
| `EVEY_HLS_KEY_ROTATION_SEGMENTS` | `transcoding.hls_key_rotation_segments` | `0` | Con el cifrado activado, cambia la clave cada N segmentos; `0` = una clave por título. |
| `EVEY_CAST_WASM_AUDIO_ENABLED` | | `false` | Permite que un Chromecast descodifique él mismo Dolby Digital en lugar de recibir AAC. Desactivado porque sobrecargaba los receptores de gama baja sin ahorrar nada. Los navegadores no se ven afectados. |

## Descargas

| Variable | Clave del panel | Por defecto | Efecto |
| --- | --- | --- | --- |
| `EVEY_COMPAT_DOWNLOAD` | `transcoding.compat_download` | activado | Prepara una descarga cuyo audio puede reproducir un teléfono (AAC), con el vídeo intacto. Solo se recodifica el audio, lo que lleva minutos. |
| `EVEY_DOWNLOAD_QUALITY_CHOICE` | `downloads.quality_choice` | desactivado | Ofrece una copia 720p más pequeña para las descargas. Recodifica el vídeo, a más o menos tiempo real en una máquina modesta, y por eso está desactivado hasta que lo actives. |
| `EVEY_DOWNLOAD_REDUCE_ABOVE_BYTES` | `downloads.reduce_above_bytes` | `4294967296` (4 GiB) | El tamaño a partir del cual la aplicación ofrece la copia más pequeña; el teléfono también la ofrece cuando la película no cabe. |

## Reproductor

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_PLAYBACK_SKIP_FORWARD` | `30` | Segundos que salta el botón de avance. |
| `EVEY_PLAYBACK_SKIP_BACKWARD` | `30` | Segundos que salta el botón de retroceso. |
