---
source_sha: 970d79357da787e635d08003e577bd5a9a2aa1600f1dd6722628b4291f0dea6f
reviewed: true
---

# Preguntas frecuentes

Respuestas cortas a las preguntas que recibe la persona que mantiene Evey en
casa. Cada una dice primero qué hacer y luego enlaza a la página que explica
por qué, para el día en que quieras entenderlo.

## ¿Por qué la película tarda en empezar?

Casi siempre porque el servidor la está adaptando. Tu televisor o tu teléfono
no puede reproducir ese archivo tal como está guardado, así que el servidor lo
convierte mientras miras, y los primeros segundos se van en prepararlo. Una
película que arranca casi al instante se envía tal cual.

Abre **Información del flujo** en el reproductor (el botón (i)): te dice cuál
de los dos casos ocurre, pista por pista, y por qué. Si un solo título es
lento cada vez mientras los demás van bien, la causa probable es el propio
archivo, y la persona que mantiene Evey puede comprobarlo en **Administración
→ Salud de medios**.

Más: [Cómo se decide la reproducción](../playback/how-playback-is-decided.md#reading-stream-info)
y, para quien administra el servidor, [Salud de medios](../runbooks/media-health.md).

## ¿Por qué la imagen se ve menos nítida en mi teléfono cuando estoy fuera?

Porque a los espectadores fuera de casa se les aplica un techo. La persona que
mantiene Evey puede fijar dos límites en **Administración →
Transcodificación**: uno para todos y otro más bajo para los espectadores
fuera de la red local, para que una película siga reproduciéndose con una
conexión móvil. Cada cuenta puede tener además su propio **Bitrate máximo**.
Un título por encima del límite se convierte para ajustarse a él, y con eso se
pierde algo de detalle.

No tienes que elegir nada: Evey nunca te pide una calidad. **Información del
flujo** muestra qué límite se aplicó.

Más: [Qué se codifica](../playback/transcoding.md#what-is-encoded) y
[Qué puede restringir un administrador por cuenta](../administration/users.md#what-an-administrator-can-restrict-per-account).

## ¿Por qué el sonido está en el idioma equivocado?

Una pista que eliges a mano, en el menú de ajustes del reproductor, siempre
gana para ese título. Si no, los reproductores toman el idioma fijado en
**Ajustes → Reproducción → Idioma de audio por defecto**, luego la pista que el
archivo marca como predeterminada, y luego la primera. Fija tu idioma ahí una
vez y todos los títulos lo seguirán. Lo mismo vale para los subtítulos, con
**Idioma de subtítulos por defecto**.

Más: [Qué pista se reproduce](../playback/audio.md#which-track-plays).

## ¿Funciona en mi televisor?

Sí, de una de estas tres formas:

- **Un Android TV o Google TV** ejecuta la aplicación Evey, hecha para el mando
  a distancia. En la pantalla de inicio de sesión, **Conexión rápida** muestra
  un código que apruebas desde tu teléfono u ordenador, así nunca escribes una
  contraseña con el mando.
- **Un televisor con navegador web** abre el mismo tipo de pantalla en la
  dirección del servidor seguida de `/tv`.
- **Un Chromecast**, o un televisor con Chromecast integrado, recibe la imagen
  desde el botón Cast del reproductor web o Android.

Más: [Conectar las aplicaciones](apps.md#quick-connect-signing-in-a-tv-without-typing-a-password)
y [Enviar a Chromecast](../playback/casting.md).

## ¿Puedo ver algo en el tren, sin conexión?

Con la aplicación Android, sí. En una película o un episodio, la aplicación
puede guardar una copia en el teléfono (**Descargas**), que sigue
reproduciéndose durante 30 días sin contactar con el servidor. Cuando la
persona que mantiene Evey lo activa, la aplicación ofrece además una copia más
pequeña para una película demasiado grande para el teléfono. Un perfil
infantil no puede abrir una descarga por encima de su límite de edad, aunque
la haya hecho otro perfil.

Más: [Qué añade cada aplicación](apps.md#what-each-app-adds).

## ¿Puedo ver una película con un amigo que vive en otro sitio?

Sí. Inicia la película en el reproductor web o en la aplicación Android y pulsa
**Ver juntos**: obtendrás un código de seis caracteres. Tu amigo lo introduce,
desde su propia cuenta o, sin cuenta, con **Ver juntos** en la página de inicio
de sesión. Cada uno ve el mismo instante, en su propia pantalla, en la versión
que conviene a su propio dispositivo.

Más: [Ver juntos](../playback/watching-together.md).

## ¿Cómo doy acceso a un amigo sin abrirle todo el catálogo?

En **Administración → Usuarios**, crea un enlace de invitación y limítalo a las
bibliotecas que esa persona puede ver: las demás no le aparecerán en ningún
sitio, ni siquiera en la búsqueda. También puedes limitar la calidad que recibe
(**Bitrate máximo**) y cuántas pantallas puede reproducir a la vez. Para una
sola película, un **Enlace de descarga público** en su página permite a
cualquiera que tenga el enlace descargar el archivo durante 48 horas, sin
cuenta.

Más: [Crear una cuenta](../administration/users.md#create-an-account) y
[Enlaces de descarga públicos](../administration/users.md#public-download-links).

## ¿Por qué una película no tiene póster, o tiene el equivocado?

Porque el nombre del archivo no apuntaba al título correcto. En una película,
lo que más cuenta es el año (`The Mirror (1999)`); en un episodio, el número de
temporada y de episodio (`S01E05`). Para corregir un título, ábrelo, elige
**Editar metadatos**, luego **Identificar en TMDB** y escoge la coincidencia
correcta. Un archivo nuevo, renombrado o borrado aparece solo en pocos minutos;
**Escanear** en la fila de la biblioteca lo fuerza.

Más: [Nombrar los archivos para que se reconozcan](../administration/libraries.md#name-files-so-they-are-recognised)
y [Corregir un título equivocado o ausente](../administration/libraries.md#fix-a-wrong-or-missing-title).

## Los subtítulos dicen que se están preparando. ¿Pasa algo?

No. La primera vez que alguien elige unos subtítulos, el servidor los extrae
del archivo; el vídeo sigue y aparecen por sí solos cuando están listos. Ocurre
una vez por pista. Si el mensaje se queda varios minutos, el archivo no tiene
índice para esa pista y el servidor lo lee entero, una vez.

Más: [La primera vez que se muestra un subtítulo](../playback/subtitles.md#the-first-time-a-subtitle-is-shown).

## Todo va lento para todos a la vez. ¿Por qué?

El servidor convierte un número limitado de vídeos a la vez, un número que
midió en su propio hardware. Cuando falta sitio, primero baja la calidad de un
flujo nuevo, luego lo pasa al procesador, y solo después muestra **El servidor
está ocupado transcodificando otros flujos**. Las películas enviadas tal cual
no cuentan, y por eso un catálogo adaptado a los dispositivos de casa rara vez
llega al límite. La persona que mantiene Evey ve la carga en **Administración
→ Estado del sistema**.

Más: [Compartir la capacidad](../playback/transcoding.md#sharing-the-capacity).

## ¿Mis películas o mi historial salen de casa?

No. Las aplicaciones hablan con tu servidor Evey y con nada que opere el
proyecto. Lo que exploras, reproduces y solicitas se queda en ese servidor, y
quién puede leerlo depende de la persona que lo administra. Las excepciones son
las esperables: un póster descargado de su fuente pública, un Chromecast que
carga la página del receptor, un enlace que tú mismo abres.

Más: [Política de privacidad](../privacy.md).

## ¿Cómo actualizo sin romper nada?

Para quien administra el servidor, en la carpeta que contiene el archivo
compose:

```bash
docker compose pull && docker compose up -d
```

Eso es toda la actualización. Cuando la nueva versión necesita cambiar la base
de datos, primero guarda una copia en `./evey-data/backups/`. Evey avisa en el
panel de administración cuando existe una versión más reciente; nunca se
actualiza solo.

Más: [Actualizar Evey](../backups.md#updating-evey).

## He olvidado mi contraseña

Si la página de inicio de sesión muestra **¿Olvidaste tu contraseña?**, úsalo:
se te envía por correo un enlace para restablecerla. Ese enlace solo está ahí
cuando la persona que mantiene Evey ha activado la **Recuperación de
contraseña** y configurado el correo. Si no, pídeselo: en **Administración →
Usuarios**, **Editar usuario** tiene un campo **Nueva contraseña**. Como último
recurso, cuando la que se ha perdido es la contraseña del propio
administrador, `evey admin reset-password` imprime una nueva en el servidor.

Más: [Correo y notificaciones](../notifications.md) y
[Dar una nueva contraseña a un usuario](../runbooks/recovery.md#give-a-user-a-new-password).
