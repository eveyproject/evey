---
source_sha: aedd9ba6244ba2dbc7e348c929241546a233301b93c88350c78c524424a8728f
hide:
- navigation
---

<pre class="evey-wordmark" aria-hidden="true"><span>███████╗</span><span class="v">██╗   ██╗</span><span>███████╗██╗   ██╗</span>
<span>██╔════╝</span><span class="v">██║   ██║</span><span>██╔════╝╚██╗ ██╔╝</span>
<span>█████╗  </span><span class="v">██║   ██║</span><span>█████╗   ╚████╔╝ </span>
<span>██╔══╝  </span><span class="v">╚██╗ ██╔╝</span><span>██╔══╝    ╚██╔╝  </span>
<span>███████╗</span><span class="v"> ╚████╔╝ </span><span>███████╗   ██║   </span>
<span>╚══════╝</span><span class="v">  ╚═══╝  </span><span>╚══════╝   ╚═╝   </span></pre>

# Evey

Servidor multimedia autoalojado para películas, series, música y fotos. Un
contenedor, tu propio hardware, tus propios archivos.

[Instalar Evey](installation.md){ .md-button .md-button--primary }
[Ver el código fuente](https://github.com/Evey-project/evey){ .md-button }

## Por dónde empezar

<div class="grid cards" markdown>

-   __Instalar__

    ---

    Un solo archivo compose, ningún secreto que inventar. Postgres y el índice
    de búsqueda se ejecutan dentro del contenedor y generan sus propias
    credenciales en el primer arranque.

    [Guía de instalación](installation.md)

-   __Configurar__

    ---

    Todo lo que un administrador puede cambiar, y dónde se cambia. Casi todo
    está en el panel de administración y no en variables de entorno.

    [Referencia de configuración](configuration.md)

-   __Ponerlo tras un proxy__

    ---

    Caddy, nginx, Traefik y Apache, con las cabeceras que la difusión y la
    reproducción por WebSocket necesitan de verdad.

    [Guía de proxy inverso](reverse-proxy.md)

-   __Proteger tus datos__

    ---

    Lo que el servidor respalda por su cuenta, lo que no, y cómo restaurar una
    base de datos que ya no arranca.

    [Copias de seguridad](backups.md) · [Recuperación](runbooks/recovery.md)

</div>

## Qué hace

- **Reproduce lo que es tuyo.** Reproducción directa siempre que el cliente
  pueda abrir el archivo tal cual, transcodificación por hardware o software
  cuando no puede, y un flujo HLS que cambia de pista de audio y de calidad sin
  cortar.
- **Te acompaña entre dispositivos.** Web, Android, Android TV y Chromecast, con
  puntos de reanudación, estado de visionado y descargas para ver sin conexión.
- **Está pensado para una casa.** Perfiles con su propio tema e idioma, control
  parental aplicado en el servidor y visionado compartido con SyncPlay.
- **Se amplía sin recompilar.** Las extensiones se ejecutan aisladas, con tokens
  acotados y permisos denegados por defecto, y los temas son datos, no código.

## Contribuir

La página de [arquitectura](ARCHITECTURE.md) explica cómo está organizado el
servidor, y las [decisiones de arquitectura](adr/README.md) recogen por qué se
tomó cada decisión estructural. Quien escriba extensiones empieza por la
[preparación del entorno para extensiones](plugins/dev-setup.md).

Evey se distribuye bajo [AGPL-3.0-or-later](https://www.gnu.org/licenses/agpl-3.0.html).
Los informes de seguridad van a `security@eveyproject.org` - consulta el
[modelo de amenazas](SECURITY.md).
