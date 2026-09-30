---
source_sha: e59bbf6aa9d7e1a2cb3513ef0337a093ae293e253430eaa6e6d5dc087423cb60
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
| **Música** | Artistas, álbumes, pistas | Las etiquetas de los archivos, MusicBrainz; portadas de Cover Art Archive, luego iTunes |
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
