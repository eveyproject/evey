---
source_sha: d6466c24262a137d7107741ff21ca1ce685f8aa99221f04a4c6f8b6ddfcb6a27
reviewed: true
---
# Instalación { #install }

Esta página es para la persona que va a tener Evey funcionando para toda la
casa. Necesitas un ordenador o un NAS que esté siempre encendido, con Docker
instalado. Son tres pasos:

1. Guardar un archivo, el archivo compose.
2. Arrancar Evey con un comando.
3. Abrir Evey en un navegador.

Después, el asistente de [Primera configuración](getting-started/first-setup.md)
se encarga del resto.

Evey funciona en un solo contenedor. Su base de datos y su motor de búsqueda
van dentro, así que no hay nada más que instalar, ningún archivo `.env` que
escribir y ningún ajuste de memoria o de procesador.

!!! warning "Usa el archivo de esta página, no el del repositorio Git"

    El `docker-compose.yml` que está en la raíz del repositorio Git es otra
    instalación, para desarrolladores e instalaciones avanzadas: necesita un
    archivo `.env` y arranca servicios adicionales. Se explica en
    [Despliegues avanzados](operations/advanced-deployments.md).

## Qué necesitas { #what-you-need }

- **Un ordenador o un NAS que esté siempre encendido**, con un procesador
  x86-64 (Intel o AMD). Las máquinas ARM64 necesitan una etiqueta de versión,
  que todavía no existe: consulta
  [Máquinas ARM](operations/advanced-deployments.md#arm-hosts).
- **Docker con Compose v2.** El comando es `docker compose`, con un espacio,
  y no el antiguo `docker-compose`.
- **Tu carpeta de archivos**: donde están tus películas, series, música o
  fotos.
- **Algo de espacio en disco para los datos de Evey** (base de datos,
  imágenes, índice de búsqueda). El archivo compose de abajo los guarda en
  una carpeta llamada `evey-data`, junto al archivo.

Evey responde en HTTP simple dentro de la red de tu casa. Para ver algo desde
fuera de casa hace falta HTTPS, que puedes añadir más adelante:
[Acceso remoto y HTTPS](administration/remote-access.md).

## Paso 1: guardar el archivo compose { #the-compose-file }

Crea una carpeta para Evey, por ejemplo `evey`, y guarda dentro este archivo
con el nombre `docker-compose.yml`. Cambia una sola línea: la ruta de tus
archivos (la línea marcada con el 5). Por ejemplo, si tus películas están en
`/srv/media`, la línea queda así: `- /srv/media:/media`.

```yaml title="docker-compose.yml"
services:
  evey:
    image: eveyproject/evey:latest # (1)!
    container_name: evey-server
    ports:
      - "9091:9091" # (2)!
    volumes:
      - ./evey-data:/data # (3)!
      - evey-cache:/cache # (4)!
      - /path/to/your/media:/media # (5)!
    stop_grace_period: 60s # (6)!
    restart: unless-stopped

volumes:
  evey-cache:
```

1. La versión de Evey. `latest` es la versión publicada más reciente. Para
   quedarte en una versión concreta, usa una etiqueta de versión como
   `0.1.0` o una etiqueta `sha-<commit>`: consulta
   [Copias de seguridad y actualizaciones](backups.md#updating-evey).
2. El puerto. El número de la izquierda es el que escribirás en el navegador
   y en las apps. Deja `9091`, salvo que otro programa ya lo use.
3. Los datos de Evey: base de datos, claves, contraseñas generadas, índice de
   búsqueda, imágenes, registros, copias de seguridad, plugins. **Esta es la
   carpeta de la que hay que hacer copia de seguridad.**
4. Una caché de vídeo convertido, de fotos y de portadas de música
   redimensionadas. Se puede borrar cuando quieras y se vuelve a llenar sola.
   Las variantes de los pósters no están en ella: viven en
   `evey-data/artworks`.
5. Tus archivos. Cambia `/path/to/your/media` por la carpeta real. Dentro de
   Evey, esta carpeta se llama `/media`. Evey puede escribir en ella, y es a
   propósito: así [Salud de medios](runbooks/media-health.md) puede reparar
   un archivo cuando se lo pides. Añade `:ro` al final para dejarla en solo
   lectura: entonces Evey no puede escribir en ella y las reparaciones
   fallan.
6. Le da tiempo a la base de datos para pararse bien cuando el contenedor se
   detiene. Los 10 segundos que da Docker por defecto son poco para una
   colección grande.

## Paso 2: arrancar Evey { #first-launch }

En una terminal, dentro de la carpeta donde está el archivo:

```bash
docker compose up -d
```

Evey arranca en segundo plano. Para ver lo que está haciendo, ejecuta
`docker compose logs -f evey`; pulsa `Ctrl+C` para dejar de mirar, y Evey
sigue funcionando.

En el primer arranque, Evey crea sus contraseñas y su base de datos en
`evey-data`. No tienes que preparar nada.

## Paso 3: abrir Evey en un navegador { #the-address-to-open-next }

La dirección es `http://`, luego la dirección de la máquina donde funciona
Docker, y luego `:9091`:

| Dónde está tu navegador | Dirección que hay que escribir |
| --- | --- |
| En la máquina donde funciona Docker | `http://localhost:9091` |
| En otro ordenador o en un teléfono de casa | `http://` + la dirección IP de la máquina donde funciona Docker + `:9091`, por ejemplo `http://192.168.1.20:9091` |
| Fuera de casa | Todavía no. Hace falta HTTPS y una dirección pública: consulta [Acceso remoto y HTTPS](administration/remote-access.md). |

Para saber la dirección IP de la máquina donde funciona Docker, mira la lista
de dispositivos conectados de tu router. También puedes ejecutar `ip a` en
Linux, `ipconfig` en Windows o `ifconfig` en macOS. Si cambiaste el puerto
en el paso 1, pon el número de la izquierda de la línea `ports:` en lugar de
`9091`.

La primera vez, la dirección abre el asistente de configuración. Sigue con la
[Primera configuración](getting-started/first-setup.md#1-the-setup-wizard).
Cuando termines el asistente, la misma dirección abrirá la página de inicio
de sesión.

!!! tip "Apunta esta dirección"

    Todos en casa la van a necesitar. Las apps de Android y Android TV piden
    exactamente esta dirección en su primera pantalla, con `http://` y el
    puerto: [Conectar las aplicaciones](getting-started/apps.md#the-server-address-to-enter).

## Dónde están tus datos { #where-the-data-lives }

| Ubicación | Qué contiene | ¿Se puede borrar? |
| --- | --- | --- |
| `./evey-data` | Base de datos, claves, contraseñas generadas, índice de búsqueda, miniaturas de la barra de reproducción, imágenes, registros, copias de seguridad, plugins | **No.** Si la pierdes, pierdes todas las cuentas, el historial y los ajustes. La única excepción es `meili/`, que está dentro: es el índice de búsqueda, y el comando `evey admin reindex-search` lo vuelve a crear a partir de la base de datos. |
| Volumen `evey-cache` | Vídeo convertido, fotos y portadas redimensionadas | Sí. Se vuelve a llenar solo. |
| Tus carpetas de archivos | Tus archivos | Evey nunca los borra. Quitar un título en Evey lo quita de la biblioteca, no del disco. |

Evey solo escribe en tus carpetas de archivos en tres casos:

- una reparación que lanzas desde **Salud de medios** (el original se guarda
  como copia de seguridad);
- un subtítulo descargado de OpenSubtitles (se guarda en una carpeta
  `.subtitles` junto a la película);
- un pequeño archivo de prueba, que se borra justo después, para comprobar
  que Evey sigue viendo los archivos nuevos.

## Cambios habituales { #common-variations }

Cada uno de estos cambios ocupa unas pocas líneas en el mismo archivo.

**Una segunda carpeta de archivos.** Añade `- /other/path:/media2` debajo de
`volumes:`. Una tercera sería `/media3`, y así sucesivamente.

**Un segundo Evey en la misma máquina.** Dale otro `container_name` y otros
puertos del anfitrión; si no, `docker compose up` se niega a arrancar (el
nombre ya está en uso).

**Otro puerto.** Cambia solo el número de la izquierda, por ejemplo
`"8080:9091"`. Después escribe `:8080` en el navegador y en las apps.

**De quién son los archivos.** Por defecto, Evey toma como suyo el
propietario de tu carpeta de archivos, para que las reparaciones puedan
escribir. Si prefieres elegirlo tú, añade `PUID` y `PGID` debajo de
`environment:`, con los números que dan `id -u` e `id -g`. El contenedor
arranca como root solo para aplicar esto; Evey en sí nunca funciona como
root.

**Una tarjeta gráfica para convertir vídeo.** Dale al contenedor acceso a la
tarjeta: consulta [Aceleración por hardware](administration/hardware-acceleration.md).

**Ya tienes un proxy inverso** (un programa que pone HTTPS delante de tus
servicios): consulta [Acceso remoto y HTTPS](administration/remote-access.md).

??? note "En un NAS (Synology Container Manager, QNAP Container Station)"

    - Crea la carpeta `evey-data` antes del primer arranque. Estas interfaces
      se niegan a montar una carpeta que no existe, al contrario que la línea
      de comandos de Docker. No hace falta crear nada más.
    - Pon en `PUID` y `PGID` el usuario del NAS al que pertenecen los
      archivos (`id <usuario>` por SSH; en Synology suele ser alrededor de
      `1026`, grupo `100`). Las carpetas compartidas de un NAS suelen
      pertenecer a `root`, y Evey nunca toma ese usuario por su cuenta.
    - NAS Intel con Quick Sync: añade las líneas `devices:` y `group_add:`
      que se explican en
      [Aceleración por hardware](administration/hardware-acceleration.md).

## Lo que Evey configura solo { #what-evey-sets-up-by-itself }

Esto no lo tienes que configurar tú:

- **Cuánto vídeo puede convertir el servidor a la vez.** Mide la máquina en
  su primer momento tranquilo.
- **La tarjeta gráfica.** Evey detecta la tarjeta que le diste al contenedor
  y la comprueba con una conversión real.
- **Cómo reproduce cada dispositivo un archivo.** Se decide en cada
  reproducción: consulta
  [Cómo elige Evey el modo de reproducción](playback/how-playback-is-decided.md).
- **El acceso desde fuera de casa.** Cerrado hasta que le des al servidor una
  dirección pública.

Los ajustes que quizá quieras cambiar más adelante (nombre del servidor,
horarios, límites, claves de API) están en el panel de administración, no en
el archivo compose. Las pocas variables de entorno que existen aparecen en la
[Referencia de configuración](configuration.md).
