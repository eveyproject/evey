---
source_sha: 3bd3ddf0df0a475d6ddf4417b5aa5b3630c871184888584df522bdae5daee7d4
reviewed: true
---
# Entrega de la reproducción y descargas

Los interruptores que deciden cómo se empaquetan los flujos y qué prepara el
servidor para las descargas. Los valores por defecto son el comportamiento
recomendado: estas variables existen para desactivar una vía mientras
investigas un problema, o para activar una que está desactivada. Cómo se
elige un flujo en primer lugar:
[Cómo elige Evey el modo de reproducción](../playback/how-playback-is-decided.md).

Una variable con clave de panel también se puede definir como una fila con
esa clave en **Administración → Configuración del servidor → Entradas de
configuración**; la fila gana al entorno. El panel sugiere las claves de las
descargas; las demás se escriben tal como aparecen aquí. Todas se leen al
arrancar, así que un cambio se aplica en el siguiente reinicio, se haga como
se haga.

## Empaquetado

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_HLS_ONDEMAND` | activado | Sirve un archivo cuyas pistas se pueden copiar reempaquetándolo sobre la marcha, sin ffmpeg y sin escribir en la caché. `off`: esos archivos pasan por la vía de copia de ffmpeg. | Entradas de configuración `transcoding.hls_ondemand`, al reiniciar |
| `EVEY_REMUX_COPY` | activado | La vía de copia de ffmpeg: el vídeo se copia sin recodificar cuando el archivo lo permite. `off`: esos archivos se transcodifican. | Entradas de configuración `transcoding.remux_copy`, al reiniciar |
| `EVEY_HLS_AUDIO_RENDITION_ENABLED` | `true` | Ofrece las pistas de audio como versiones aparte, para que un reproductor cambie de idioma sin volver a descargar el vídeo. También permite reproducir un vídeo copiado con una pista de audio recodificada por separado. | ninguno |
| `EVEY_HLS_COPY_FALLBACK_RUNG` | `true` | Añade una calidad baja transcodificada junto a un flujo copiado, para que un reproductor cuya conexión se hunde pueda bajar de calidad en lugar de congelarse. No cuesta nada hasta que un reproductor la usa. | ninguno |
| `EVEY_HLS_ONDEMAND_CONCAT` | activado | Reproducción sin cortes entre episodios, cuando el reproductor web la pide. Un episodio que no se puede copiar vuelve al flujo normal. | Entradas de configuración `transcoding.hls_ondemand_concat`, al reiniciar |
| `EVEY_HLS_ONDEMAND_TRACK_TRIM` | desactivado | Pone en cada flujo solo la pista de audio seleccionada en lugar de todas. | Entradas de configuración `transcoding.hls_ondemand_track_trim`, al reiniciar |
| `EVEY_HLS_ONDEMAND_ENCRYPT` | desactivado | Cifra los flujos reempaquetados con AES-128 (solo HLS: entonces no se ofrece ningún manifiesto DASH). | Entradas de configuración `transcoding.hls_ondemand_encrypt`, al reiniciar |
| `EVEY_HLS_KEY_ROTATION_SEGMENTS` | `0` | Con el cifrado activado, cambia la clave cada N segmentos; `0` = una clave por título. | Entradas de configuración `transcoding.hls_key_rotation_segments`, al reiniciar |
| `EVEY_CAST_WASM_AUDIO_ENABLED` | `false` | Permite que un Chromecast decodifique él mismo el Dolby Digital en lugar de recibir AAC. Desactivado porque sobrecargaba los receptores modestos sin ahorrar nada. Los navegadores no se ven afectados. | ninguno |

## Descargas

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_COMPAT_DOWNLOAD` | activado | Prepara una descarga cuyo audio puede reproducir un teléfono (AAC), sin tocar el vídeo. Solo se recodifica el audio, lo que lleva unos minutos. | Entradas de configuración `transcoding.compat_download`, al reiniciar |
| `EVEY_DOWNLOAD_QUALITY_CHOICE` | desactivado | Ofrece una copia más pequeña en 720p para las descargas. Recodifica el vídeo, a más o menos tiempo real en una máquina modesta, y por eso está desactivado hasta que lo actives. | Entradas de configuración `downloads.quality_choice`, al reiniciar |
| `EVEY_DOWNLOAD_REDUCE_ABOVE_BYTES` | `4294967296` (4 GiB) | El tamaño a partir del cual la aplicación ofrece la copia más pequeña; el teléfono también la ofrece cuando la película no cabe. | Entradas de configuración `downloads.reduce_above_bytes`, al reiniciar |

## Reproductor

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_PLAYBACK_SKIP_FORWARD` | `30` | Segundos que salta el botón de avanzar. | ninguno |
| `EVEY_PLAYBACK_SKIP_BACKWARD` | `30` | Segundos que salta el botón de retroceder. | ninguno |

<!-- sources
packaging flags: internal/adapters/driving/http/transcode_vod_wire.go (serverFlagEnabled, resolveServerFlag: hls_ondemand, remux_copy, hls_ondemand_concat, hls_ondemand_track_trim, hls_ondemand_encrypt, hls_key_rotation_segments)
renditions, fallback rung, cast wasm: internal/config/config.go - AudioRenditionEnabled, CopyFallbackRungEnabled, CastWasmAudioEnabled
downloads: cmd/evey/http.go (compat_download, quality_choice, reduce_above_bytes via resolveFlag)
config entries catalogue: frontend/apps/web/src/features/admin/operatorFlags.ts ; i18n admin.serverConfig.entriesTitle, appliedOnRestart
skip: internal/config/config.go - EVEY_PLAYBACK_SKIP_*
-->
