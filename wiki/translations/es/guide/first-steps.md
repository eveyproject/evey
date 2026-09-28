---
source_sha: f1ea44197021d6c8d4791aa939ef1b103613cd438a70d3c00ce1af151c0ca580
---
# Primeros pasos

El contenedor está en marcha y ha abierto `http://<su-ip>:9091`. Esta página
recorre los diez primeros minutos: el asistente de instalación, las
bibliotecas, las personas, y las tres cosas que conviene comprobar antes de dar
la dirección a nadie. Si el servidor aún no está en marcha, empiece por la
[instalación](../installation.md).

## El asistente de instalación

Un servidor sin administrador se abre con un asistente de cuatro pasos.

1. **Idioma.** El idioma por defecto de los perfiles nuevos y de los metadatos
   (títulos, sinopsis, géneros). Puede cambiarlo más tarde en
   **Admin → Configuración del servidor → Idioma y región**, junto con la región
   que decide qué clasificaciones de edad se aplican.
2. **Bienvenida.**
3. **Cuenta de administrador.** Un nombre de usuario de al menos tres caracteres
   y una contraseña de al menos ocho. El **nombre del servidor**, opcional
   («Salón»), aparece en la página de inicio de sesión, en la lista de
   servidores de las aplicaciones y en los correos.
4. **Primera biblioteca.** Un nombre y un tipo. Puede saltarse los detalles aquí
   y terminar en el panel de administración.

## Añadir sus bibliotecas

**Admin → Bibliotecas → Añadir biblioteca.** Una biblioteca tiene un nombre, un
tipo (películas, series, música o fotos) y una o varias carpetas.

**Las carpetas son rutas dentro del contenedor**, no en su máquina. Con el
archivo compose por defecto, su carpeta de medios se monta en `/media`: una
biblioteca de películas guardada en `/srv/media/films` en la máquina es
`/media/films` para Evey. Una segunda carpeta montada (`/media2`, ...) se añade
igual. El selector muestra las carpetas que ve.

Mantenga películas y series en bibliotecas separadas: el tipo le dice al
escáner qué buscar. Evey reconoce nombres como estos:

```text
/media/films/The Mirror (1999)/The Mirror (1999) 1080p.mkv
/media/films/Lantern.2010.1080p.BluRay.x264.mkv
/media/series/The Outpost/Season 01/The.Outpost.S01E05.720p.mkv
/media/series/Quiet Storm/Quiet.Storm.03x10.mkv
```

En una película, el año ayuda mucho. En un episodio, lo que cuenta es `S01E05`
o `1x05`. Las etiquetas de release (resolución, origen, códec, audio, marcas de
idioma como `VOSTFR` o `MULTI`, ediciones como «Director's Cut») se quitan antes
de la búsqueda. Si sus archivos llevan una etiqueta que el escáner no conoce,
añádala en **Admin → Configuración del servidor → Tokens del analizador**.

Para cada biblioteca, además elige:

| Opción | Qué hace |
| --- | --- |
| **Monitoreo en tiempo real** | Los archivos nuevos, renombrados o borrados se detectan en cuanto aparecen. Una comprobación periódica (cada 2 minutos por defecto) recoge lo que el sistema de archivos no avisó, por ejemplo en una carpeta de red. |
| **Escanear al iniciar** | Vuelve a escanear la biblioteca cada vez que arranca el servidor. |
| **Programar** | Un escaneo completo cada 6 o 12 horas, cada día, cada semana, o según su propia expresión cron. |

El primer escaneo empieza solo. Los carteles y los datos llegan en segundo
plano, así que una biblioteca grande se completa en unos minutos.

**Cuando un título está mal o falta.** En la lista de bibliotecas, **Vincular
solo huérfanos** reintenta los títulos que nunca se reconocieron, y **Volver a
vincular todo con TMDB** repite la búsqueda para cada título (lento, pero
corrige errores antiguos tras un gran renombrado). En un título, **Editar
metadatos** permite corregir el título o pegar el identificador correcto de
TMDB o IMDb.

## Fuentes de metadatos

Evey incluye una clave de **TMDB**, la fuente de títulos, carteles, repartos y
colecciones, así que los metadatos funcionan desde el principio. Las demás
fuentes son opcionales y sus claves se introducen en
**Admin → Configuración del servidor → Claves API**:

| Clave | Aporta |
| --- | --- |
| TMDB (la suya) | Su propia cuota en lugar de la compartida. |
| OMDb | Una segunda fuente de metadatos, para completar un título que TMDB conoce poco. |
| OpenSubtitles | Buscar y descargar subtítulos que un archivo no trae. |
| TheIntroDB | Más marcas de intro, resumen y créditos (500 consultas al día en lugar de 100). |

Una clave introducida en el panel prevalece sobre la misma clave pasada en el
entorno.

## Invitar al hogar

**Admin → Usuarios** ofrece dos puertas de entrada:

- **Crear usuario**: usted elige el nombre y la contraseña.
- **Crear enlace de invitación**: la persona elige los suyos. Un enlace puede
  caducar, limitarse a un número de usos, dar acceso solo a algunas
  bibliotecas, limitar el número de perfiles que podrá crear la nueva cuenta, e
  incluso crear un administrador.

Para cada usuario puede después restringir las **bibliotecas** que ve, limitar
su **bitrate de streaming**, sus **perfiles** y sus **dispositivos por perfil**,
consultar su historial, o **conectarse como** él para ver exactamente lo que ve.

Los perfiles, los perfiles infantiles y el PIN parental los gestiona cada cuenta
por sí misma: vea los
[ajustes del espectador](viewer-settings.md#profiles-and-parental-control).

## Dejar que el servidor se mida

La primera vez que arranca en una máquina, y de nuevo cuando cambia el
hardware, Evey mide cuántos vídeos puede convertir a la vez, con el procesador
y con la tarjeta gráfica si la hay. Tarda unos dos minutos, solo se ejecuta
cuando nadie está mirando, y a un reproductor que pida una conversión mientras
tanto se le indica que lo vuelva a intentar en un momento. El resultado está en
**Admin → Transcodificación**: ese número es un techo que puede bajar, nunca
subir. Para usar una tarjeta gráfica, vea
[Transcodificación y hardware](../transcoding.md).

## Antes de compartir la dirección

1. **Acceso remoto.** Evey solo se abre a Internet si le ha dado una dirección
   pública. Revise **Admin → Acceso remoto**: si el servidor es accesible desde
   fuera pero el acceso remoto está desactivado, la página lo avisa en rojo, y
   todos los que están fuera de su red reciben un error. Vea la
   [administración](admin-settings.md#remote-access).
2. **HTTPS.** Evey habla HTTP simple. Póngalo detrás de un proxy inverso antes
   de publicarlo: [Proxy inverso](../reverse-proxy.md).
3. **Copias de seguridad.** El servidor respalda su base de datos antes de cada
   actualización. Para copias regulares, fije una programación:
   [Copias de seguridad](../backups.md). Guarde bien `./evey-data`: contiene la
   base de datos y la clave que firma cada sesión.

## Instalar las aplicaciones

Abra la aplicación de Android o de Android TV, escriba la dirección del
servidor e inicie sesión. En una tele, elija la **conexión rápida**: la tele
muestra un código y un código QR, que usted aprueba desde
**Ajustes → Conexión rápida** en un teléfono o un ordenador en el que ya haya
iniciado sesión. Más en [En todas las pantallas](../discover/apps.md).
