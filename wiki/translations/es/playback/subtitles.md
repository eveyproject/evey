---
source_sha: 3fced19f436765baa59845968676caa83737f9a681cd41288d6e854fcde880ae
reviewed: false
---
# Subtítulos

Lo que le pasa a un subtítulo depende de su formato: texto o imágenes.

| Formato | Ejemplos | Cómo se muestra | Efecto en el vídeo |
| --- | --- | --- | --- |
| Texto | SRT, ASS/SSA, WebVTT, subtítulos dentro del archivo | Se envía junto al vídeo y lo dibuja el reproductor | Ninguno |
| PGS (Blu-ray) | `hdmv_pgs_subtitle` | El servidor lo descodifica en imágenes una vez, lo guarda en caché y lo dibuja el reproductor | Ninguno |
| Otros formatos de imagen | VobSub (DVD), DVB, XSUB | El servidor lo incrusta en la imagen | El vídeo se recodifica |

Todos los reproductores (web, Android, Android TV) dibujan ellos mismos los
subtítulos de texto y PGS, así que elegir uno nunca cambia cómo se entrega el
vídeo. Solo la última fila lo hace: incrustar subtítulos en la imagen exige
descodificar y recodificar el vídeo, así que un título que se reproducía de forma
directa pasa a transcodificarse. Información del flujo indica entonces
**Subtítulos incrustados en la imagen**.

Un subtítulo de imagen solo se incrusta cuando lo seleccionas de verdad. Un
archivo que simplemente lo lleva, o lo marca como predeterminado, conserva su
Reproducción directa.

## Incrustar subtítulos

El ajuste de perfil **Incrustar subtítulos** (**Ajustes → Reproducción**) rige los
formatos de la última fila:

- **Auto** e **Incrustar siempre**: una pista VobSub, DVB o XSUB seleccionada se
  incrusta. El texto nunca se incrusta, sea cual sea el ajuste.
- **Nunca incrustar**: el vídeo nunca se recodifica por un subtítulo. Una pista
  VobSub, DVB o XSUB no puede mostrarse entonces.

## Elegir y ajustar

- Los reproductores seleccionan el subtítulo del **Idioma de subtítulos por
  defecto** del perfil y, después, la pista que el archivo marca como
  predeterminada (una pista forzada, por ejemplo).
- **Sincronización de subtítulos**, en el reproductor, desplaza los subtítulos
  del título actual (los valores positivos los muestran más tarde). Se aplica al
  instante.
- **Estilo de subtítulos** fija el tamaño, el color, el fondo, la fuente y la
  posición de los subtítulos de texto.
- **Subir subtítulo…** añade un archivo al título.
- Con una clave de OpenSubtitles (consulta [Bibliotecas y metadatos](../administration/libraries.md#metadata-keys)),
  la API del servidor puede buscar y descargar un subtítulo que el archivo no
  lleva (`GET /api/v1/media/{id}/subtitles/search`, y después
  `POST /api/v1/media/{id}/subtitles/download`). El archivo se guarda en una
  carpeta `.subtitles` junto a la película.
  Las aplicaciones todavía no ofrecen esta búsqueda.
