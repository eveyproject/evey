---
source_sha: dd11b7fe28f14e3c25cf6db836b2a6f0752722f82e49a8db9ce447b959d38822
reviewed: false
---
# Primera configuración

El servidor está en marcha y has abierto `http://<server-address>:9091`. Quedan
cuatro pasos antes de que se pueda ver algo: el asistente de configuración, las
bibliotecas, los usuarios y las aplicaciones.

## 1. El asistente de configuración

Un servidor sin administrador abre un asistente de cuatro pasos:

1. **Idioma**: el idioma por defecto de los perfiles nuevos y de los metadatos
   (títulos, sinopsis, géneros).
2. **Bienvenida**.
3. **Cuenta de administrador**: un nombre de usuario de al menos 3 caracteres y
   una contraseña de al menos 8. El nombre opcional del servidor aparece en la
   página de inicio de sesión y en la lista de servidores de las aplicaciones.
4. **Primera biblioteca**: un nombre y un tipo. Puedes saltarlo y añadir
   bibliotecas después.

<figure class="evey-shot" markdown="span">
  ![El panel de administración del servidor de demostración](../images/admin-dashboard.webp){ loading=lazy }
  <figcaption>La administración tras iniciar sesión: el panel y cada sección en el menú lateral (interfaz en inglés).</figcaption>
</figure>

## 2. Añade tus bibliotecas

**Admin → Bibliotecas → Añadir biblioteca**: un nombre, un tipo (Películas,
Series, Música o Fotos) y una o varias carpetas.

Las carpetas son rutas **dentro del contenedor**. Con el archivo compose de la
instalación, tu carpeta de medios es `/media`: una carpeta de películas en
`/srv/media/films` en el equipo es `/media/films` para Evey.

El primer escaneo empieza solo. Los archivos nuevos se detectan en cuanto
aparecen, y los pósteres, sinopsis y reparto llegan en segundo plano, así que
una biblioteca grande se completa en unos minutos.

Cómo nombrar los archivos, cómo funcionan los escaneos y cómo corregir una
coincidencia errónea: [Bibliotecas y metadatos](../administration/libraries.md).

## 3. Añade a las personas

**Admin → Usuarios** crea una cuenta con una contraseña que eliges tú, o un
enlace de invitación con el que la persona elige la suya. Cada cuenta crea luego
sus perfiles: uno por persona del hogar, incluidos los perfiles infantiles.

Lo que un administrador puede restringir por cuenta y cómo funcionan los
perfiles y el control parental:
[Usuarios, perfiles y control parental](../administration/users.md).

## 4. Conecta las aplicaciones

La aplicación web es la dirección que ya abriste. Las aplicaciones de Android y
Android TV piden la dirección del servidor y después un inicio de sesión; una
televisión puede iniciar sesión con un código aprobado desde un teléfono.
Consulta [Aplicaciones y dispositivos](apps.md).

## Lo que el servidor hace por sí mismo

Nada de esto necesita un ajuste:

- **Reproducción.** Cada dispositivo informa de lo que puede decodificar, y el
  servidor elige, título a título, entre enviar el archivo tal cual,
  reempaquetarlo o recodificar solo lo que el dispositivo no puede reproducir.
  Consulta [Cómo elige Evey el modo de reproducción](../playback/how-playback-is-decided.md).
- **Capacidad.** En su primer momento tranquilo el servidor mide cuántos vídeos
  puede transcodificar a la vez, con el procesador y con cualquier tarjeta
  gráfica pasada al contenedor, y nunca admite más. La medición dura unos
  minutos; a una reproducción que necesite transcodificar mientras tanto se le
  pide que espere. Consulta
  [Transcodificación y capacidad del servidor](../playback/transcoding.md).
- **Acceso remoto.** Se abre cuando al servidor se le dio una dirección pública,
  y permanece cerrado en caso contrario. Consulta
  [Acceso remoto y HTTPS](../administration/remote-access.md).
- **Copias de seguridad.** La base de datos se vuelca antes de cada
  actualización que la modifica. Una programación regular está a un ajuste de
  distancia: [Copias de seguridad y actualizaciones](../backups.md).

## Antes de compartir la dirección fuera de casa

1. Pon el servidor detrás de HTTPS: Evey sirve HTTP sin cifrar.
   [Acceso remoto y HTTPS](../administration/remote-access.md).
2. Define una programación de copias de seguridad y guarda una copia de
   `./evey-data`: contiene la base de datos y la clave que firma cada sesión.
   [Copias de seguridad y actualizaciones](../backups.md).
