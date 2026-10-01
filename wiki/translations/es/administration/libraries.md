---
source_sha: 9ec526fd6900c24c0897b5487d0abdf140e3fd5aee6f9a754b78d5e5514fc8d4
reviewed: false
---
# Bibliotecas y metadatos

Una biblioteca es un tipo y un conjunto de carpetas. Evey escanea las carpetas,
reconoce cada archivo por su nombre y obtiene sus metadatos: títulos, sinopsis,
pósteres, reparto, valoraciones. Las bibliotecas se gestionan en
**Admin → Bibliotecas**.

<figure class="evey-shot" markdown="span">
  ![Admin → Bibliotecas en el servidor de demostración](../images/admin-libraries.webp){ loading=lazy }
  <figcaption><strong>Admin → Bibliotecas</strong>: una fila por biblioteca, con sus carpetas, su número de elementos y su último escaneo (interfaz en inglés).</figcaption>
</figure>

## Tipos

| Tipo | Reconocido como | Metadatos de |
| --- | --- | --- |
| **Películas** | Una película por archivo (o por carpeta) | TMDB, completado por OMDb cuando hay una clave |
| **Series** | Series, temporadas, episodios | TMDB |
| **Música** | Artistas, álbumes, pistas | Las etiquetas de los archivos; portadas de los archivos, la carpeta del álbum, Cover Art Archive (MusicBrainz), luego iTunes |
| **Fotos** | Álbumes, imágenes con sus datos de captura | Los archivos |

Mantén películas y series en bibliotecas separadas: el tipo le dice al escáner
qué buscar.

## Las carpetas son rutas dentro del contenedor

El selector de carpetas lista lo que ve el contenedor. Con el archivo compose de
la instalación, tu carpeta de medios es `/media`, así que una carpeta
`/srv/media/films` en el equipo es `/media/films` para Evey. Una segunda carpeta
montada (`/media2`) se añade igual.

## Nombrar los archivos

El escáner reconoce nombres como estos:

```text
/media/films/The Mirror (1999)/The Mirror (1999) 1080p.mkv
/media/films/Lantern.2010.1080p.BluRay.x264.mkv
/media/series/The Outpost/Season 01/The.Outpost.S01E05.720p.mkv
/media/series/Quiet Storm/Quiet.Storm.03x10.mkv
```

Para una película, lo que más cuenta es el año. Para un episodio, `S01E05` o
`1x05`. Las etiquetas de la versión (resolución, fuente, códec, audio, etiquetas
de idioma como `VOSTFR` o `MULTI`, ediciones como «Director's Cut») se eliminan
antes de la búsqueda. Si tus archivos llevan una etiqueta que el analizador no
conoce, añádela en **Admin → Configuración del servidor → Tokens del
analizador**; se aplica a partir del siguiente escaneo.

## Bibliotecas de música

El escáner lee archivos MP3, FLAC, M4A, OGG, Opus, WAV y AAC, y toma cada pista
de sus etiquetas: título (el nombre del archivo si falta), artista, artista del
álbum, álbum, año, números de pista y de disco, género. Los artistas se agrupan
por artista del álbum, o por artista cuando esa etiqueta está vacía.

**Un álbum es una carpeta.** Las pistas se agrupan por la carpeta en la que
están, no por su etiqueta de álbum: dos carpetas con la misma etiqueta de álbum
son dos álbumes, y un álbum repartido en subcarpetas `CD1` y `CD2` aparece como
dos. Mantén cada álbum en una sola carpeta; sus pistas se ordenan por número de
disco y luego por número de pista, y el álbum toma el nombre que lleva la
mayoría de sus pistas.

La portada es la imagen incrustada en los archivos, si no una imagen de la
carpeta del álbum (`cover.jpg`, `folder.jpg` o `front.jpg`), si no Cover Art
Archive (encontrada mediante MusicBrainz), si no iTunes.

## Bibliotecas de fotos

El escáner lee archivos JPEG, PNG, HEIC, WebP, GIF, TIFF y BMP. El álbum de una
foto es su carpeta, relativa a la carpeta de la biblioteca; una foto en el nivel
superior no tiene, y mover un archivo a otra carpeta lo pasa a ese álbum en el
siguiente escaneo. La fecha de captura y el modelo de cámara vienen de los datos
EXIF de los archivos JPEG; los demás formatos se listan sin ellos. Las
miniaturas se redimensionan en el servidor y se guardan en la caché de
transcodificación.

## Cómo se detectan los archivos nuevos

Cada biblioteca tiene tres opciones:

| Opción | Efecto |
| --- | --- |
| **Monitoreo en tiempo real** | Los archivos nuevos, renombrados y borrados se detectan en cuanto aparecen. Una comprobación periódica (cada 2 minutos por defecto) atrapa lo que el sistema de archivos no notificó, algo que ocurre en las carpetas de red. |
| **Escanear al iniciar** | Un escaneo cada vez que arranca el servidor. |
| **Programar** | Un escaneo completo cada 6 o 12 horas, diario, semanal o con una expresión cron. |

El servidor comprueba además que su vigilante de carpetas sigue recibiendo
eventos, escribiendo durante unos segundos un archivo de prueba en la carpeta de
la biblioteca. Una biblioteca en la que dejan de llegar eventos se recorre más a
menudo hasta que el vigilante se recupera.

## Cuando un título es incorrecto o falta

- **Vincular solo huérfanos** (por biblioteca) vuelve a intentar los archivos
  que nunca se reconocieron. Es rápido.
- **Volver a vincular todo con TMDB** rehace la búsqueda de cada título de la
  biblioteca. Es lento y corrige errores antiguos tras un gran cambio de nombres.
- **Editar metadatos** en un título cambia sus datos a mano o pega el
  identificador correcto de TMDB o IMDb. Un valor que editas se conserva: las
  actualizaciones posteriores no lo sobrescriben. **Re-emparejar metadatos** en
  el título descarta tus cambios y vuelve a buscar a partir del nombre del
  archivo.
- **Reparar sagas** vuelve a unir a su saga las películas que no pertenecen a
  ninguna colección, cuando las otras películas de la saga ya están en la
  biblioteca.

## Colecciones

**Colecciones** agrupa títulos bajo un mismo nombre, para todas las cuentas. Hay
tres tipos:

- **Colecciones TMDB**: las sagas, que se llenan solas cuando TMDB sitúa una
  película en una de ellas. **Reparar sagas** (más arriba) recupera las
  películas que se quedaron fuera de la suya.
- **Nueva colección**: una lista que un administrador llena a mano, con
  **Agregar a colección** en un título o en una selección, y ordena.
- **Nueva colección inteligente**: reglas sobre **Género**, **Año**, **Tipo**,
  **Valoración ≥** y **Biblioteca**, que un título debe cumplir todas. Su
  contenido se calcula cada vez que se abre, así que los títulos nuevos
  aparecen sin hacer nada.

Solo los administradores crean y editan colecciones. Cualquiera puede
explorarlas, y **Descargar colección** en la web obtiene los archivos originales
en un solo ZIP (hasta 1000 títulos), sin lo que ocultan el acceso a bibliotecas
o el límite de edad de la cuenta.

## Claves de metadatos

Evey incluye una clave de TMDB, así que los metadatos funcionan sin configurar
nada. Las demás fuentes son opcionales; sus claves van en **Admin →
Configuración del servidor → Claves API**, y una clave definida ahí prevalece
sobre la misma clave dada en el entorno.

| Clave | Aporta |
| --- | --- |
| TMDB (la tuya) | Tu propia cuota de peticiones en lugar de la compartida. |
| OMDb | Una segunda fuente para películas de las que TMDB sabe poco. |
| OpenSubtitles | Buscar y descargar subtítulos que un archivo no trae, a través de la API del servidor (las aplicaciones aún no lo ofrecen). Consulta [Subtítulos](../playback/subtitles.md). |
| TheIntroDB | Tiempos de intro, resumen y créditos de una base de datos comunitaria: 500 consultas al día con clave en lugar de 100 sin ella. |

## Intro, resumen y créditos

Evey marca la intro, el resumen y los créditos de episodios y películas para que
los reproductores ofrezcan saltarlos. Los tiempos vienen de bases de datos
comunitarias (TheIntroDB y luego IntroDB) cuando conocen el título, y si no, del
propio análisis de los archivos por parte del servidor. Un administrador puede
ajustar a mano los segmentos de un título o volver a lanzar la detección.

Los reproductores muestran un botón **Omitir** por defecto. **Saltar intros
automaticamente** (**Admin → Configuración del servidor → Configuracion del
servidor**, desactivado por defecto) hace que cada reproductor las salte por sí
mismo.
