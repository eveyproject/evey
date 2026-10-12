---
source_sha: 8b5109c0af499483f4c9038e9c785a28ed41a147962487df2de2848bd91fe88b
reviewed: true
hide:
- navigation
- toc
---

# Evey { #evey }

Evey es un servidor multimedia que instalas en casa. Guarda tus películas,
series, música y fotos, y cada persona de la casa las ve en sus propios
dispositivos. Nunca eliges una calidad: pulsas reproducir y Evey se encarga
del resto.

[Instalar Evey](installation.md){ .md-button .md-button--primary }
[Probar la demo](https://demo.eveyproject.org){ .md-button }

<p class="evey-status">
<span>Beta, todavía sin versión numerada</span>
<span>Software libre, AGPL-3.0</span>
<span>Demo: la sesión se inicia sola (<code>demo</code> / <code>demo</code>)</span>
</p>

<p class="evey-platforms">
<strong>Qué necesitas:</strong> un ordenador o un NAS x86-64 con Docker, y
una carpeta con tus archivos. La instalación es un archivo y un comando.<br>
<strong>Dónde ver:</strong> en un navegador web, en teléfonos y tablets Android, en
Android TV, en el navegador web de un televisor y en un Chromecast. Las apps de Android y Android TV están en
pruebas y todavía no están en Google Play.
</p>

<div class="grid cards" markdown>

-   **Vas a instalar el servidor**

    Instala Evey en tu ordenador o tu NAS, añade tus archivos e invita a las
    personas de tu casa.
    [Empezar por la instalación](installation.md)

-   **Alguien te ha dado una dirección**

    Alguien de tu casa ya tiene Evey funcionando y tú solo quieres ver algo.
    [Leer Ver con Evey](getting-started/watching.md)

</div>

<div class="evey-showcase" markdown>

<figure class="evey-shot" markdown="span">
  ![La página de inicio](images/showcase-home.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>Inicio: los títulos para continuar y, debajo, tus bibliotecas fila a fila.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![La página de una película](images/showcase-media.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>La página de una película: carátula, edad recomendada, valoraciones, sinopsis y reparto.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![El reproductor web](images/showcase-player.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>El reproductor web. En la barra: Ver juntos, Transmitir y la (i) que abre la Información del flujo.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![El panel de administración mientras se reproducen dos películas](images/showcase-dashboard.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>El panel mientras dos personas ven algo: la imagen, el dispositivo, Reproducción directa o Remux.</figcaption>
</figure>

</div>

## Para toda la casa { #for-everyone-at-home }

- **Un perfil por persona.** Cada perfil guarda su historial, por dónde va en
  cada título, sus favoritos y sus idiomas. Normalmente una casa comparte una
  sola cuenta, con un perfil por persona.
- **Perfiles infantiles.** Un perfil infantil se abre sin código. Para salir
  de él hacia un perfil de adulto hace falta un PIN de cuatro cifras. Un
  perfil infantil tiene una edad máxima, y el servidor nunca envía un título
  por encima de ella, ni siquiera como descarga.
- **Sigue donde lo dejaste**, en cualquier dispositivo. El punto donde te
  quedaste se guarda en el servidor, no en el teléfono.
- **Fuera de casa.** La app de Android puede descargar un título para verlo
  sin conexión, durante 30 días.
- **En el televisor.** Inicias sesión en una Android TV con la **Conexión
  rápida**: el televisor muestra un código y tú lo apruebas desde tu teléfono o tu
  ordenador. No hay que escribir ninguna contraseña con el mando.
- **Juntos.** Varias personas pueden ver el mismo título al mismo tiempo,
  cada una en su dispositivo.

La música y las fotos funcionan igual que las películas y las series, en la
web, en Android y en el televisor.
[Conectar las aplicaciones](getting-started/apps.md) ·
[Usuarios, perfiles y control parental](administration/users.md)

## Nada que elegir antes de pulsar reproducir { #nothing-to-choose-before-pressing-play }

Algunos archivos se reproducen en cualquier sitio. Otros pesan demasiado para
un teléfono antiguo, o llevan un formato de sonido que un televisor no puede leer. Cada vez
que pulsas reproducir, el servidor mira el archivo, el dispositivo y lo
ocupado que está, y elige la forma más ligera que funciona:

- **Reproducción directa**: el archivo se envía tal cual.
- **Remux**: la imagen y el sonido se copian, sin cambios, en un formato de
  archivo que el dispositivo acepta. Al servidor casi no le cuesta nada. Evey
  tiene su propia herramienta para esto, **MKVGo**, y usa FFmpeg cuando MKVGo
  no puede.
- **Transcodificación**: el servidor convierte solo lo que el dispositivo no
  puede reproducir, a menudo solo el sonido. Es el único caso que hace
  trabajar al procesador.

Así, la misma película puede llegar intacta a un televisor y convertida a un
teléfono antiguo. En el reproductor, la **Información del flujo** muestra
cuál de los tres se ha usado, y por qué. **Administración → Panel** muestra
lo mismo para todas las personas que están viendo algo.

<figure class="evey-shot evey-shot--narrow" markdown="span">
  ![La Información del flujo en el reproductor web](images/player-stream-info.webp){ loading=lazy }
  <figcaption>La primera reproducción del panel de arriba, vista desde el reproductor. Remux: cambia el formato de archivo, la imagen y el sonido se copian, y la última línea dice por qué el archivo no se pudo enviar tal cual (interfaz en inglés).</figcaption>
</figure>

[Cómo elige Evey el modo de reproducción](playback/how-playback-is-decided.md) ·
[MKVGo en Evey](playback/mkvgo.md)

## Un solo contenedor, nada que ajustar { #one-container-nothing-to-tune }

Toda la instalación es un archivo compose. No hay ningún `.env` que escribir
ni ajustes de memoria o de procesador.

- **Todo va dentro.** La base de datos y el motor de búsqueda funcionan
  dentro del contenedor. Sus contraseñas se crean en el primer arranque.
- **El servidor nunca funciona como root.** Funciona como el propietario de
  tu carpeta de archivos cuando consigue detectarlo, para que las
  reparaciones puedan escribir.
- **El servidor se mide a sí mismo.** En un momento tranquilo, cronometra
  conversiones reales en el procesador y en la tarjeta gráfica que le hayas
  dado, si hay una. Así sabe a cuántas personas puede atender a la vez.
  Cuando le falta capacidad, una nueva reproducción recibe una calidad más
  baja, o pasa de una tarjeta gráfica ocupada al procesador. Si una
  reproducción no puede empezar, el mensaje dice por qué: el servidor está
  lleno, todavía se está midiendo, o el archivo no está listo.
- **El acceso desde fuera de casa está cerrado** hasta que le des al servidor
  una dirección pública.

También puedes usar tu propia base de datos o tu propio motor de búsqueda:
[Despliegues avanzados](operations/advanced-deployments.md).
[Transcodificación y capacidad del servidor](playback/transcoding.md)

## Qué hace Evey con tus archivos y tus datos { #what-evey-does-with-your-files-and-your-data }

**Tus archivos.** Quitar un título o una biblioteca lo quita de Evey, nunca
del disco. Evey solo escribe en tus carpetas de archivos en tres casos: una
reparación que tú has pedido, un subtítulo descargado a través de la API, y
un pequeño archivo de prueba que comprueba que Evey sigue viendo llegar
archivos nuevos. Monta tus archivos en solo lectura y no pasa nada de eso.
**Salud de medios** revisa tus archivos MKV y WebM y lista los problemas que
encuentra. Solo repara un archivo si se lo pides. Trabaja sobre una copia,
que se comprueba entera antes de sustituir el archivo, guarda el original
por defecto y anota cada cambio. Lo que no se puede reparar se avisa y se
deja como está.
[Salud de medios](runbooks/media-health.md)

**Tus datos.** El proyecto no tiene ningún servicio que los reciba: ni
cuenta con nosotros, ni estadísticas de uso, ni informes de errores. Las apps
se comunican con tu servidor y con nada más que gestionemos nosotros. Lo que sale
de tu red: el servidor busca imágenes y descripciones en servicios externos
(The Movie Database, y fuentes de portadas para la música). De vez en cuando
también pregunta si hay una versión más nueva, algo que puedes desactivar. La
base de datos se guarda antes de cada actualización que la modifica.
[Política de privacidad](privacy.md) · [Copias de seguridad y actualizaciones](backups.md)

## Los plugins, aislados y bajo control { #plugins-run-isolated-and-controlled }

Los plugins añaden fuentes de información sobre los títulos, canales de
notificación, secciones en la página de inicio y elementos encima del
reproductor. Los temas cambian el aspecto. Antes de instalar un plugin, el
administrador ve lo que el análisis del plugin ha encontrado y los permisos
que pide. Un permiso nuevo hay que aceptarlo de forma explícita. Un plugin
funciona en su propio proceso, con un usuario aparte, sin acceso directo a
la red. Se detiene si supera su límite de memoria, de procesos o de disco.
El nivel de restricción del acceso a sus archivos depende de la máquina, y el **Panel de
seguridad** muestra el modo que se usa. El widget de un plugin aparece en la
web, en Android y en el televisor. En el televisor puedes verlo, pero no usarlo.
[Plugins](plugins.md) ·
[Modelo de seguridad y sandbox de plugins](SECURITY.md#sandbox-modes)

## Por dónde empezar { #find-your-way }

<div class="grid cards" markdown>

-   **[Instalar Evey](installation.md)**

    Qué necesitas, el archivo compose, el primer arranque, la dirección que
    hay que abrir.

-   **[Primera configuración](getting-started/first-setup.md)**

    El asistente de configuración, tus bibliotecas, las personas que van a
    ver.

-   **[Conectar las aplicaciones](getting-started/apps.md)**

    La dirección que hay que escribir en un teléfono o un televisor, el inicio
    de sesión y qué significa cada error.

-   **[Ver con Evey](getting-started/watching.md)**

    Para quienes ven: los perfiles, encontrar un título, los subtítulos,
    pedir una película.

-   **[Administración](administration/libraries.md)**

    Bibliotecas, usuarios y control parental, acceso desde fuera, tarjeta
    gráfica, copias de seguridad.

-   **[Solución de problemas](operations/troubleshooting.md)**

    Una tabla por problema: dónde mirar, qué comprobar, cómo arreglarlo.

-   **[Cómo elige Evey el modo de reproducción](playback/how-playback-is-decided.md)**

    Reproducción directa, Remux, Transcodificación, y cómo leer la
    Información del flujo.

-   **[Plugins y API](plugins.md)**

    Instalar y escribir plugins y temas, la API del servidor.

</div>

<p class="evey-meta">
Esta documentación sigue la rama de desarrollo. ·
<a href="https://github.com/eveyproject/evey">Código fuente en GitHub</a> ·
Avisos de seguridad: security@eveyproject.org ·
<a href="privacy/">Política de privacidad</a>
</p>
