---
source_sha: 4ee97dc5ef2cdb11b0aa01a535284eaf511ab308d1429dbb9882b9a5838ba8ca
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

**Sus películas, sus series, su música y sus fotos, transmitidas desde su
propia máquina a todas las pantallas de la casa - y más allá.**

Evey es un servidor multimedia autoalojado. Usted le indica las carpetas que ya
tiene, él reconoce lo que contienen, las viste con carteles, sinopsis y reparto,
y las reproduce en el navegador, en teléfonos Android, en Android TV y en un
Chromecast. Nada sale de su casa si usted no lo publica, y nadie más decide lo
que puede ver.

[Descubrir Evey](philosophy.md){ .md-button .md-button--primary }
[Instalarlo](installation.md){ .md-button }
[Probar la demo](https://demo.eveyproject.org){ .md-button }

¿Prefiere curiosear antes? La demo pública en [demo.eveyproject.org](https://demo.eveyproject.org)
ejecuta el servidor real con películas libres: inicie sesión con `demo` /
`demo`. Se restablece con regularidad, y las pantallas de administración son
visibles pero en su mayoría de solo lectura.

## Dos puertas de entrada

<div class="grid cards" markdown>

-   __Descubrir__

    ---

    Lo que Evey pretende ser, lo que aporta a quienes miran y los dispositivos
    en los que funciona. No hace falta terminal.

    [Por qué Evey](philosophy.md) · [Lo que Evey le ofrece](discover/features.md) ·
    [En todas las pantallas](discover/apps.md)

-   __Guía técnica__

    ---

    Instalar el servidor, los diez primeros minutos tras arrancarlo, cada ajuste
    que un espectador o un administrador puede cambiar y qué hacer el día en que
    algo va mal.

    [Instalación](installation.md) · [Primeros pasos](guide/first-steps.md) ·
    [Administración](guide/admin-settings.md)

</div>

## Por qué se adopta

- **Reproduce el archivo que usted tiene.** Cuando un dispositivo sabe leer un
  archivo tal cual, Evey lo envía sin tocarlo: calidad original, ningún trabajo
  para el servidor. Cuando no sabe, Evey convierte solo lo que falta, con la
  tarjeta gráfica si la hay.
- **Se instala con un archivo.** Un contenedor, un archivo compose, ninguna
  contraseña que inventar. La base de datos y el motor de búsqueda viven dentro
  y se configuran solos.
- **Está pensado para un hogar.** Perfiles para cada uno, perfiles infantiles
  protegidos con un PIN, límites de edad que aplica el servidor y no la
  aplicación, y una sala para ver juntos desde casas distintas.
- **Se puede abrir a Internet sin miedo.** El acceso remoto se abre cuando usted
  publica el servidor y se queda cerrado en caso contrario, el inicio de sesión
  falla cerrado, y las extensiones se ejecutan en un entorno aislado con solo
  los permisos que declararon.
- **Es software libre.** Con licencia
  [AGPL-3.0-or-later](https://www.gnu.org/licenses/agpl-3.0.html). El código
  está en [GitHub](https://github.com/Evey-project/evey), igual que la lista de
  lo que el servidor aún no sabe impedir.

## Accesos rápidos

| Usted quiere | Lea |
| --- | --- |
| Probarlo en su máquina | [Instalación](installation.md) |
| Ponerlo en Internet con HTTPS | [Proxy inverso](reverse-proxy.md) |
| Usar su tarjeta gráfica para convertir | [Transcodificación y hardware](transcoding.md) |
| Saber qué hace cada ajuste | [Ajustes del espectador](guide/viewer-settings.md) · [Administración](guide/admin-settings.md) |
| Recuperar un servidor tras una caída | [Recuperación](runbooks/recovery.md) |
| Escribir una extensión | [Extensiones](plugins.md) |

Los avisos de seguridad se envían a `security@eveyproject.org` - vea el
[entorno aislado y modelo de amenazas de extensiones](SECURITY.md).
