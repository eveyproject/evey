---
source_sha: 5cde466e14206b2a29398343ce7b115ae0b096216b8e6d056f834cf4b7763057
reviewed: false
hide:
  - navigation
  - toc
---

# Documentación de Evey

Evey es un servidor autoalojado para películas, series, música y fotos. En cada
reproducción compara el archivo, el reproductor y el servidor, conserva el
original siempre que el reproductor pueda usarlo y solo recodifica lo que es
necesario. La mayoría de las instalaciones no necesitan más que bibliotecas y
usuarios: el hardware, la capacidad y las vías de reproducción las detecta y
mide el propio servidor.

<div class="evey-showcase" markdown>

<figure class="evey-shot" markdown="span">
  ![La página de inicio](images/showcase-home.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>La página de inicio: reanudar la reproducción y, después, cada biblioteca.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![La página de una película](images/showcase-media.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>La página de una película, con sus imágenes, valoraciones y reparto.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![El reproductor web](images/showcase-player.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>El reproductor web.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![El panel de administración mientras se reproducen dos películas](images/showcase-dashboard.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>El panel de administración mientras dos personas ven algo: la imagen que ve cada una y cómo se entrega.</figcaption>
</figure>

</div>

<div class="evey-points" markdown>

<div markdown>

**La vía más barata primero**

El archivo tal cual, luego reempaquetado sin codificador y, por último, solo se
recodifican las pistas que lo necesitan. Un espectador en reproducción directa
o en remux le cuesta al servidor poco más que leer el archivo, así que una
máquina modesta sirve a muchos a la vez.
[Cómo elige Evey el modo de reproducción](playback/how-playback-is-decided.md)

</div>

<div markdown>

**Un servidor que se mide a sí mismo**

Cronometra codificaciones 4K reales en el procesador y en cada tarjeta gráfica,
y solo usa una tarjeta para lo que ha demostrado. No hay capacidad que estimar.
[Transcodificación y capacidad del servidor](playback/transcoding.md)

</div>

<div markdown>

**Lleno significa rechazado, no más lento**

No más codificaciones de las medidas; cuando está lleno, el reproductor recibe
el aviso de que el servidor está ocupado en lugar de degradar todos los flujos.
Las tareas en segundo plano se pausan durante la reproducción.
[Tareas en segundo plano y carga del servidor](operations/background-work.md)

</div>

<div markdown>

**Un plugin, tres pantallas**

El mismo paquete del plugin funciona en la web, en Android y en Android TV, y
sabe en qué pantalla se muestra.
[Referencia de la API de plugins](plugins/api-reference.md#on-a-television)

</div>

<div markdown>

**Plugins confinados, no de confianza**

Código analizado antes de instalar, solo su propio directorio, llamadas al
sistema peligrosas rechazadas, sin red directa y un token denegado por defecto.
[Modelo de seguridad y sandbox de plugins](SECURITY.md)

</div>

<div markdown>

**Lo que dice la Información del flujo es lo que se ejecuta**

Una decisión por reproducción, y la Información del flujo la lee del plan que
ejecuta el codificador, con el motivo por el que el archivo no se reprodujo tal
cual.
[Cómo elige Evey el modo de reproducción](playback/how-playback-is-decided.md#reading-stream-info)

</div>

</div>

## Por dónde empezar

<div class="grid cards" markdown>

-   **[Instalar Evey](installation.md)**

    Un contenedor y un archivo compose. Requisitos y primer arranque.

-   **[Primera configuración](getting-started/first-setup.md)**

    El asistente de configuración, tus bibliotecas, las personas que usarán el
    servidor.

-   **[Administración](administration/libraries.md)**

    Bibliotecas, usuarios y perfiles, acceso remoto, aceleración por hardware,
    copias de seguridad.

-   **[Cómo elige Evey el modo de reproducción](playback/how-playback-is-decided.md)**

    Reproducción directa, remux y transcodificación, y cómo leer la Información
    del flujo.

-   **[Solución de problemas](operations/troubleshooting.md)**

    Dónde mirar cuando algo no funciona: síntomas, registros, recuperación.

-   **[Plugins y API](plugins.md)**

    Instalar y escribir plugins y temas, la API del servidor.

</div>

<p class="evey-meta">
Evey está en beta y aún no tiene una versión numerada: esta documentación sigue
la rama de desarrollo. ·
<a href="https://github.com/Evey-project/evey">Código fuente en GitHub</a> ·
<a href="https://demo.eveyproject.org">Demo pública</a> (inicia sesión con demo / demo) ·
AGPL-3.0-or-later ·
Informes de seguridad: security@eveyproject.org
</p>
