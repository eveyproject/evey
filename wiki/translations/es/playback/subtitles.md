---
source_sha: 960cbfbbd3741cf6a15885a1e19f04e9e96e53efbe2f9861376cc3a92778dfef
reviewed: true
---
# Subtítulos { #subtitles }

**Para activar los subtítulos**, abre el menú de ajustes del reproductor y
elige **Subtítulos**. Para tenerlos siempre en tu idioma, configura
**Ajustes → Reproducción → Idioma de subtítulos por defecto**.

La mayoría de los subtítulos aparecen al momento. Hay dos casos distintos:

- **La primera vez que alguien elige un subtítulo guardado dentro del
  archivo**, Evey tiene que sacarlo del archivo. El reproductor muestra
  **Preparando subtítulos…**, el vídeo sigue reproduciéndose y los subtítulos
  aparecen solos. En una película muy grande esto puede tardar unos minutos,
  una sola vez: después, se muestran al momento para todo el mundo. Consulta
  [La primera vez que se muestra un subtítulo](#the-first-time-a-subtitle-is-shown).
- **Los subtítulos de Blu-ray, de DVD y de emisiones de TV** son imágenes,
  no texto. El servidor los convierte en imágenes una vez y el reproductor los
  dibuja, igual que el texto. Solo el raro formato XSUB lo sigue dibujando el
  servidor dentro del vídeo, lo que le hace trabajar mucho más. Consulta
  [Incrustar subtítulos](#subtitle-burn-in).

El resto de la página explica cada formato de subtítulos y lo que cambia para
el servidor.

## Qué le hace cada formato al vídeo { #what-each-format-does-to-the-video }

| Formato | Ejemplos | Cómo se muestra | Efecto en el vídeo |
| --- | --- | --- | --- |
| Texto | SRT, ASS/SSA, WebVTT, subtítulos dentro del archivo | Se envía junto al vídeo y lo dibuja el reproductor, con tu **Estilo de subtítulos** | Ninguno |
| Formatos de imagen | PGS (Blu-ray), VobSub (DVD), DVB (emisiones de TV), dentro del archivo o como archivo `.sup` o `.idx`/`.sub` a su lado | El servidor lo convierte en imágenes una vez, las guarda en caché, y el reproductor las dibuja tal cual | Ninguno |
| XSUB | XSUB (algunos archivos DivX) | El servidor lo incrusta en la imagen | El vídeo se vuelve a codificar |

Todos los reproductores (web, Android, Android TV) dibujan ellos mismos los
subtítulos de texto y de imagen, así que elegir uno nunca cambia la forma en que se
entrega el vídeo. Solo la última fila lo cambia: incrustar subtítulos en la
imagen obliga a decodificar y volver a codificar el vídeo, así que un título
que se reproducía de forma directa pasa a transcodificarse. La Información
del flujo muestra entonces **Subtítulos incrustados en la imagen**.

Un subtítulo XSUB solo se incrusta cuando lo eliges de verdad. Un
archivo que simplemente lo lleva, o que lo marca como predeterminado, sigue
en Reproducción directa.

## Incrustar subtítulos { #subtitle-burn-in }

El ajuste de perfil **Incrustar subtítulos** (**Ajustes → Reproducción**)
decide qué subtítulos de imagen incrusta el servidor en el vídeo:

| Ajuste | Una pista XSUB elegida | Una pista PGS, VobSub o DVB elegida | Texto |
| --- | --- | --- | --- |
| **Auto** | Se incrusta y el vídeo se vuelve a codificar. | La dibuja el reproductor. | Lo dibuja el reproductor. |
| **Incrustar siempre** | Se incrusta y el vídeo se vuelve a codificar. | Se incrusta y el vídeo se vuelve a codificar. | Lo dibuja el reproductor: el texto nunca se incrusta. |
| **Nunca incrustar** | No se puede mostrar: el vídeo nunca se vuelve a codificar por un subtítulo. | La dibuja el reproductor. | Lo dibuja el reproductor. |

## La primera vez que se muestra un subtítulo { #the-first-time-a-subtitle-is-shown }

Un subtítulo que va dentro del archivo (texto o imagen) lo extrae el servidor la
primera vez que alguien lo elige, y luego lo guarda: las veces siguientes,
sea quien sea, aparece al momento. Cuando el índice del propio archivo cubre
una pista de texto o PGS, el servidor lee solo los bloques del subtítulo y tarda unos
segundos. Sin ese índice, lee el archivo entero: varios minutos para una
película grande (unos minutos para un archivo muy grande, más si el mismo servidor está emitiendo vídeo a la vez).

Mientras tanto, el reproductor muestra **Preparando subtítulos…** donde
aparecen los subtítulos, vuelve a comprobar por su cuenta y los muestra en
cuanto están listos; el vídeo sigue reproduciéndose. Elegir otra pista no
detiene el trabajo: el servidor lo termina para la próxima vez.

Una pista que el servidor no puede extraer se queda vacía. El fallo se
recuerda, así que volver a elegirla no lanza otra lectura completa; una
actualización de MKVGo, la biblioteca que la extrae, lo vuelve a intentar.
Consulta [MKVGo en Evey](mkvgo.md#what-mkvgo-does) para saber qué se lee y
cuándo.

## Elegir y ajustar { #choosing-and-adjusting }

- Los reproductores eligen el subtítulo del **Idioma de subtítulos por
  defecto** del perfil y, si no lo hay, la pista que el archivo marca como
  predeterminada (una pista forzada, por ejemplo). Una pista elegida a mano
  nunca se cambia, y con un título nuevo la elección vuelve a empezar de
  cero.
- **Sincronización de subtítulos**, en el reproductor, adelanta o retrasa los
  subtítulos del título actual (con valores positivos aparecen más tarde). Se
  aplica al momento.
- **Estilo de subtítulos** define el tamaño, el color del texto, el fondo, la
  fuente, la sombra y la posición de los subtítulos de texto. Los subtítulos
  de imagen se dibujan tal cual.
- **Subir subtítulo…** añade un archivo para el título.
- Con una clave de OpenSubtitles (consulta [Bibliotecas y metadatos](../administration/libraries.md#metadata-keys)),
  la API del servidor puede buscar y descargar un subtítulo que el archivo no
  lleva (`GET /api/v1/media/{id}/subtitles/search`, y luego
  `POST /api/v1/media/{id}/subtitles/download`). El archivo se guarda en una
  carpeta `.subtitles` junto a la película. Las apps no ofrecen esta
  búsqueda.
