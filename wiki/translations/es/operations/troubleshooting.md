---
source_sha: e679166aebb377eab1f02f6459bcb9daf6480843df9d2b438037f5909294a541
reviewed: true
---
# Solución de problemas { #troubleshooting }

¿Algo no funciona? Busca lo que ves en la tabla de abajo. La columna del
medio dice dónde mirar primero; la última lleva a la solución. Si no basta,
los registros y `evey admin doctor` (más abajo) dan más información.

## Dónde mirar, según lo que ves { #where-to-look-by-symptom }

| Lo que ves | Mira primero | Después |
| --- | --- | --- |
| No responde nada, ni siquiera en casa | ¿Está Evey funcionando? `docker compose ps`, y después las últimas líneas de su salida: `docker compose logs --tail=200 evey` | [El servidor no arranca](#the-server-does-not-start) más abajo; [El servidor no responde](../runbooks/incident-runbook.md#the-server-does-not-answer) |
| La app dice **No se puede alcanzar el servidor** o **Conexión fallida** | La dirección escrita en la app | [Conectar las apps](../getting-started/apps.md#the-server-address-to-enter); solo fuera de casa: [Acceso remoto y HTTPS](../administration/remote-access.md) |
| Se ha cerrado la sesión de todo el mundo, o nadie puede iniciar sesión | **Administración → Acceso remoto**, los proxies de confianza; después los registros | [Nadie puede iniciar sesión](#nobody-can-sign-in) más abajo |
| Una película no muestra imagen, no arranca o se detiene | **Información del flujo** en el reproductor, y después **Administración → Salud de medios** para ese título | [Problemas de reproducción](#playback-problems) más abajo |
| Va a tirones o no para de cargar | **Administración → Calidad de streaming** (qué títulos y qué dispositivos esperan) y **Administración → Estado del sistema** (si el servidor está demasiado ocupado) | [Transcodificación y capacidad del servidor](../playback/transcoding.md) |
| Póster equivocado, título equivocado, o una película no se reconoce | **Editar metadatos** en el título | [Corregir un título equivocado o que falta](../administration/libraries.md#fix-a-wrong-or-missing-title) |
| Faltan pósteres o están rotos | **Administración → Caché de artworks** | [Biblioteca y pósteres](#library-and-posters) más abajo |
| Los archivos nuevos no aparecen | El resultado del escaneo en la fila de la biblioteca, en **Administración → Bibliotecas** | [Biblioteca y pósteres](#library-and-posters) más abajo |
| Los correos no llegan | **Enviar correo de prueba** en **Administración → Configuración del servidor → Correo / SMTP** | [Los correos no llegan](#e-mails-do-not-arrive) más abajo |
| Un plugin falla | Sus **Registros**, en su tarjeta de **Administración → Plugins** | [Un plugin se ha bloqueado](../runbooks/incident-runbook.md#a-plugin-crashed) |
| Un envío a la tele (cast) se ha congelado o se ha quedado sin sonido | **Administración → Reproduciendo ahora**: ¿sigue apareciendo la reproducción? | [Un envío se ha congelado, se ha quedado sin sonido, o el teléfono no deja de cargar](../runbooks/incident-runbook.md#a-cast-froze-went-silent-or-the-phone-keeps-spinning) |
| El disco se llena | **Administración → Caché de transcodificación** | [El disco de la caché está lleno](../runbooks/incident-runbook.md#the-cache-disk-is-full) |

## Los registros { #the-logs }

El registro (log) es el archivo donde Evey anota lo que hace: cada error
se anota ahí, con la hora.

**La forma fácil: Administración → Registros y depuración.** Muestra el registro
con un buscador, una vista **En vivo** que sigue las líneas nuevas, y un
filtro **Zona** para una sola parte (reproductor y conversión, subtítulos,
detección de intros, Chromecast, inicio de sesión...). Cuando alguien te
cuenta un problema, mira aquí primero, alrededor de la hora que te diga.

??? info "Dónde están los archivos de registro"

    - Los archivos están en `./evey-data/logs/`, junto a tu archivo compose
      (`/data/logs/evey-YYYY-MM-DD.log` dentro del contenedor). Un archivo
      por día, en JSON, más un archivo con sufijo de hora
      (`evey-YYYY-MM-DD-HH.log`) cuando un archivo pasa de 50 MB
      (`EVEY_LOG_MAX_SIZE_MB`), que se guarda 90 días por defecto
      (`EVEY_LOG_RETENTION_DAYS`).
    - `docker compose logs evey` muestra otra cosa: lo que el contenedor
      escribe mientras arranca (las líneas `evey:` del script de arranque,
      la base de datos, el banner). Léelo cuando el servidor no llega a
      arrancar. Una vez funcionando, el registro está en los archivos y en
      el panel.

## `evey admin doctor` { #evey-admin-doctor }

Un solo comando revisa todo el servidor. Muestra una línea por
comprobación, `ok`, `WARN` o `FAIL`, con lo que hay que hacer:

```bash
docker exec evey-server evey admin doctor
```

??? info "Qué comprueba"

    La base de datos y su esquema, la clave de firma, el índice de búsqueda
    (y si coincide con la base de datos), las carpetas de contenido
    (incluido si Evey puede escribir en cada biblioteca), las carpetas de
    caché y de registros, `ffmpeg` y `mkvgo`, los proxies de confianza, el
    acceso remoto y el espacio libre en disco. `--repair` aplica solo las
    correcciones seguras (crear las carpetas que faltan y darles el
    propietario correcto). Termina con un código de error cuando falla una
    comprobación. Los demás comandos están en
    [Línea de comandos](../runbooks/cli.md).

## El servidor no arranca { #the-server-does-not-start }

| Lo que dice `docker compose logs evey` | Qué significa |
| --- | --- |
| Un mensaje de validación de la configuración | Con una base de datos externa, el servidor rechaza la contraseña por defecto `evey`; con un Meilisearch externo, una clave vacía o de ejemplo. Corrige el valor que indica el mensaje. |
| Líneas que empiezan por `evey:` sobre Postgres | La base de datos integrada se negó a arrancar: un clúster de otra versión principal de Postgres ([Actualización de versión principal de Postgres](advanced-deployments.md#move-to-a-newer-postgres-major)), un directorio de datos que no es un clúster de Postgres, un disco lleno, o `./evey-data/postgres` en una carpeta compartida en red. |
| `permission denied` en `/cache/...`, en un bucle de reinicios | El volumen `evey-cache` pertenece a un usuario distinto del que ejecuta el servidor. Actualiza la imagen; con una imagen anterior al 2026-09-17, ejecuta una vez `docker run --rm -v <project>_evey-cache:/c alpine chown -R <PUID>:<PGID> /c`. |

Una contraseña de administrador perdida, una clave de firma comprometida o
una base de datos dañada: [Recuperación](../runbooks/recovery.md).

## Nadie puede iniciar sesión { #nobody-can-sign-in }

Dos causas habituales:

- **La base de datos no responde.** Evey comprueba cada inicio de sesión
  con ella, y lo rechaza cuando no puede. El registro lo dice.
- **Falta un proxy inverso en los proxies de confianza** (**Administración →
  Acceso remoto**). Entonces todos los visitantes parecen venir de la
  dirección del proxy, y todos comparten un mismo límite de intentos de
  inicio de sesión.

El procedimiento paso a paso:
[Todos los inicios de sesión devuelven 401](../runbooks/incident-runbook.md#every-login-returns-401).

!!! note
    Perder o sustituir la clave de firma no cierra por sí solo la sesión de
    nadie: las apps renuevan sus tokens. Para cerrar todas las sesiones a
    propósito está `evey admin rotate-jwt-key`, ver
    [Recuperación](../runbooks/recovery.md#rotate-the-key-that-signs-access-tokens).

El servidor responde en casa pero no fuera, o las apps dicen que el acceso
remoto está desactivado: [Acceso remoto y HTTPS](../administration/remote-access.md).
**Administración → Acceso remoto** lo dice en rojo cuando el servidor es
accesible desde internet mientras el acceso remoto está desactivado.

### Falla el inicio de sesión con SSO { #sso-sign-in-fails }

La página de inicio de sesión indica el motivo:

- **dominio no permitido**: la dirección no pertenece a los dominios de
  correo permitidos de la tarjeta;
- **ninguna cuenta coincide**: el registro está desactivado; activa **Crear
  cuentas para nuevos usuarios SSO** o crea la cuenta;
- **cuenta desactivada**;
- **la cuenta existe pero no se pudo vincular**: el proveedor no confirmó el
  correo, la vinculación está desactivada, o la cuenta está vinculada a
  otra identidad;
- **no disponible**: no se puede llegar al proveedor; **Estado** y **Probar
  la conexión** en la tarjeta muestran el error.

Una página de error que muestra el propio proveedor suele significar que la
URI de redirección registrada allí no es la que muestra la tarjeta. Ver
[OIDC / SSO](../oidc.md).

## Los correos no llegan { #e-mails-do-not-arrive }

**Enviar correo de prueba** en **Administración → Configuración del servidor
→ Correo / SMTP** envía un mensaje a tu propia dirección con los ajustes
guardados, y muestra la respuesta del servidor de correo cuando falla. El
error habitual es un modo TLS que no corresponde al puerto (`starttls` con
587, `ssl` con 465). Un host guardado en la tarjeta sustituye a todas las
variables `EVEY_SMTP_*`. Los envíos de notificaciones que fallan se anotan
como `email: send notification`. Ver
[Notificaciones](../notifications.md).

## Problemas de reproducción { #playback-problems }

Empieza por **Información del flujo** en el reproductor. Dice cómo se envía
la película (tal cual, copiada en otro formato o convertida), pista por
pista, y por qué. Si un título se convierte cuando esperabas que se
reprodujera tal cual: ver
[Cómo se decide la reproducción](../playback/how-playback-is-decided.md).

- **Un título** se detiene, no para de cargar, o tarda en saltar a otro
  momento: abre **Administración → Salud de medios**. Las causas habituales
  son un archivo sin índice de posicionamiento para saltar, un tramo dañado, o
  pistas guardadas en bloques grandes. La mayoría se pueden reparar. Ver
  [Salud de los medios](../runbooks/media-health.md).
- **Varios títulos, cuando ven varias personas**: puede que el servidor esté
  al límite de lo que puede convertir. **Administración → Estado del
  sistema** muestra lo ocupado que está. Ver
  [Transcodificación y capacidad del servidor](../playback/transcoding.md).
- La reproducción transcodificada falla directamente:
  [La transcodificación falla](../runbooks/incident-runbook.md#transcoded-playback-fails).
- Se queda cargando y los registros no dicen por qué:
  [el procedimiento de diagnóstico](../runbooks/incident-runbook.md#it-buffers-or-cuts-out-and-the-log-does-not-say-why).
- No hay sonido solo en un navegador, con una pista Dolby Digital
  (AC-3/E-AC-3): un proxy inverso que pone su propia
  Content-Security-Policy puede bloquear el decodificador que usa el
  reproductor web. Ver [Audio](../playback/audio.md#dolby-digital-in-a-browser).
- El sonido va desfasado en un título: **Administración → Salud de medios**
  detecta el audio que empieza después de la imagen y las pistas que no
  coinciden entre sí.
- Los subtítulos no aparecen: [Subtítulos](../playback/subtitles.md) dice
  qué formatos dibujan los reproductores y cuáles tiene que incrustar el
  servidor en la imagen.
- **Preparando subtítulos…** se queda varios minutos: el archivo no tiene
  índice para esa pista y el servidor lo lee entero, una vez. Ver
  [La primera vez que se muestra un subtítulo](../playback/subtitles.md#the-first-time-a-subtitle-is-shown).
- Los títulos transcodificados no arrancan o se detienen mientras los títulos
  ya reproducidos siguen funcionando, o los administradores reciben **Caché
  de subtítulos llena**: el disco está por debajo de su margen de espacio
  libre. Ver
  [La caché de transcodificación](../playback/transcoding.md#the-transcoding-cache).

## Biblioteca y pósteres { #library-and-posters }

Los pósteres y las imágenes de fondo se copian al servidor durante los
escaneos, en `./evey-data/artworks`, y los que faltan se vuelven a intentar
cada hora. **Administración → Caché de artworks** muestra cuántos siguen sin
copia local; **Descargar artworks faltantes** los descarga ahora, y
**Purgar variantes corruptas** quita los pósteres redimensionados que un
redimensionado fallido dejó vacíos, que se vuelven a generar cuando hacen
falta.

Un póster o un título que pertenece a otra película es un problema de
coincidencia de metadatos, no de imágenes:
[Corregir un título equivocado o que falta](../administration/libraries.md#fix-a-wrong-or-missing-title).

Un archivo nuevo aparece normalmente en unos minutos (ver
[Cómo se detectan los archivos nuevos](../administration/libraries.md#how-new-files-are-picked-up)).
Si no aparece, pulsa **Escanear** en la fila de la biblioteca en
**Administración → Bibliotecas** y lee el resultado.

Un escaneo termina con **Escaneo parcial: no se pudieron leer estas
carpetas:** seguido de rutas: Evey no puede leer esas carpetas (permisos,
una carpeta compartida en red que se ha desconectado). El resto de la
biblioteca se escanea, y los títulos de una carpeta ilegible se conservan en
lugar de borrarse. Revisa el montaje y el propietario (`PUID`/`PGID`, ver
[Instalación](../installation.md#common-variations)), y vuelve a escanear.

La búsqueda no devuelve nada: reconstruye el índice de búsqueda a partir de
la base de datos con `docker exec evey-server evey admin reindex-search`. No
se pierde nada: el índice siempre se puede reconstruir.

## Informar de un problema { #reporting-a-problem }

Escribe a **contact@eveyproject.org**. Un informe útil dice:

- qué hiciste, qué esperabas y qué pasó en su lugar;
- cuándo pasó, al minuto: los registros se buscan por hora;
- el dispositivo y la app: **Ajustes → Acerca de → Versión de la app** en la
  app de Android, el navegador y su versión para la app web;
- la versión del servidor: `docker exec evey-server evey version`;
- para un problema de reproducción, el título y una captura de pantalla de
  **Información del flujo** (ver
  [Leer la información del flujo](../playback/how-playback-is-decided.md#reading-stream-info)):
  indica el método elegido y sus motivos, que es la mayor parte del
  diagnóstico.

Un administrador puede añadir las líneas del registro de ese momento
(**Administración → Registros y depuración**, ver [Los registros](#the-logs)) y
el informe de [`evey admin doctor`](#evey-admin-doctor). Los registros
contienen nombres de cuentas y direcciones de red: léelos antes de
enviarlos.

Un fallo de seguridad no se cuenta en un sitio público: escribe a
**security@eveyproject.org**, y deja tiempo para que se corrija antes de
hablar de él.

<!-- sources
symptom table: i18n mobile.serverConnect.unreachable / connectionFailed ; i18n media.rematchMetadata, admin.library.refreshMetadataTooltip
logs: internal/pkg/logger/logger.go - New (file handler only outside DevMode), rotatingWriter.rotate (evey-YYYY-MM-DD.log), StartCleanup (90 days) ; internal/adapters/driving/http/admin_handler_logs.go - currentLogPath ; docker-compose.yml EVEY_LOG_DIR=/data/logs, ./evey-data:/data ; i18n admin.logs.where.*, admin.logs.live
docker compose logs: docker/entrypoint.sh (evey: lines), cmd/evey/banner.go
doctor: cmd/evey/admin_commands.go - doctor ; cmd/evey/admin_ops.go - checkResult names (database, schema, jwt.key, meilisearch, search index, media <root>, trusted proxies, remote access, disk), checkTool ffmpeg/mkvgo
sso: internal/adapters/driving/http/auth_handler.go /login?oidc_error=<reason>, i18n auth.login.ssoError.* ; smtp: internal/adapters/driving/http/smtp_test_handler.go (test send), internal/adapters/driven/email/notification_emailer.go (log)
partial scan: internal/app/library/scan_service.go - scanDirectories (unreadable root = failed path, protected from pruning) ; i18n admin.libraries.scanPartial
posters: internal/app/metadata/command_service_artwork.go (hourly retry) ; i18n admin.artworks.* (/data/artworks, purge variants)
rotate-jwt-key ends sessions: cmd/evey/admin.go - endAllSessions
startup failures: docker/entrypoint.sh ; cache ownership: docker/entrypoint.sh (re-own /cache)
remote access warning: docs/adr/0026-remote-access-inferred-from-a-published-url.md
reporting: docs/privacy.md (contact@eveyproject.org) ; docs/SECURITY.md (security@eveyproject.org) ; app version: frontend/apps/mobile/app/(auth)/(tabs)/settings.tsx, i18n mobile.settings.appVersion ; server version: cmd/evey/cli.go
new files row: i18n admin.libraries.scan, scanPartial ; docs/administration/libraries.md (How new files are picked up) ; e-mail row: i18n admin.serverConfig.sendTestEmail
-->
