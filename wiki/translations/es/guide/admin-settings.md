---
source_sha: 9fca0896aed2edad83b383a5c8dc68a4c1946e29c664cda6b017db17d93c7dc0
---
# Administración

El panel de administración (menú del avatar y luego **Admin**) es desde donde
un administrador lleva el servidor en el día a día. Casi todo se guarda en la
base de datos: sobrevive a una actualización del contenedor, y la mayoría de
los ajustes se aplican al momento. El panel indica cuándo un ajuste solo surte
efecto tras un reinicio (`docker compose restart`).

Esta página sigue el menú del panel y se detiene en los ajustes que importan.
Para lo que hay que decidir antes de arrancar el servidor (URL pública, proxy de
confianza, propietario de los archivos), vea la
[configuración](../configuration.md).

## Panel y sistema

### Panel y Reproduciendo ahora

El **Panel** muestra la versión, la máquina y lo que está pasando.
**Reproduciendo ahora** lista cada flujo en curso: quién, qué, en qué
dispositivo y cómo - **Reproducción directa** (el archivo tal cual), **Stream
directo** (reempaquetado, imagen intacta) o **Transcodificación** (convertido) -
pista por pista, con el motor y la tarjeta gráfica usados. Puede detener una
sesión o enviar un mensaje al espectador.

### Acceso remoto

Quién puede conectarse desde fuera de la red de su casa.

| Ajuste | Qué hace |
| --- | --- |
| **Activar acceso remoto** | Desactivado: quien no tenga una dirección de su red local recibe un error en cada página. Activado: puede iniciar sesión como cualquiera. |
| **URL externa** | La dirección pública, cuando difiere de aquella con la que arrancó el servidor. |
| **Redes LAN** | Los rangos de direcciones que cuentan como «casa». Los rangos privados vienen por defecto; añada aquí el rango de una VPN (Tailscale, WireGuard) si sus usuarios deben contar como casa. |
| **Bitrate remoto máximo** | Un techo de bitrate para los espectadores fuera de casa, en kbit/s (0 = ninguno). Una dirección privada nunca se limita. |

Si no lo toca, el servidor decide al arrancar: el acceso remoto está activado si
le dio una dirección pública, desactivado en caso contrario. Si el servidor es
accesible desde Internet pero el acceso remoto está desactivado, la página le
avisa en rojo. Detrás de un proxy inverso, esto solo funciona si el proxy
transmite la dirección real del cliente - vea el
[proxy inverso](../reverse-proxy.md#real-client-ips-and-the-remote-access-gate).

El acceso remoto es una elección sobre quién puede llegar al servidor, no una
protección: lo que protege la biblioteca es el inicio de sesión.

### Configuración del servidor

La página agrupa los ajustes propios del servidor en tarjetas:

| Tarjeta | Los ajustes que conviene conocer |
| --- | --- |
| **Idioma y región** | Idioma por defecto de la interfaz, el país cuyas clasificaciones de edad se aplican y el idioma de los títulos y las sinopsis. |
| **Feeds de inicio** | Qué filas muestra la página de inicio, en qué orden y cuántos elementos cada una. Las extensiones pueden añadir filas. |
| **Correo / SMTP** | El correo saliente para invitaciones y restablecimiento de contraseñas. Vea [Notificaciones](../notifications.md). |
| **Miniaturas Trickplay** | Genera las imágenes de vista previa de la línea de tiempo. Activado por defecto. |
| **Potencia asignada** | Cuánto procesador puede tomar en total el trabajo en segundo plano: **25 %** (suave, por defecto) o **50 %** (impulso, para cuando nadie mira). Una tarea pesada (análisis de medios nuevos, miniaturas, detección de intros, clasificación de complejidad) puede recibir su propia parte. Se aplica al siguiente elemento. El trabajo en segundo plano se pausa mientras alguien mira. |
| **Perfil de memoria** | **Bajo** suspende las extensiones inactivas para ahorrar memoria; **Auto** lo elige en máquinas de menos de 3 GB. |
| **Trabajos en segundo plano** | Pausa y marca el ritmo de las pasadas de fondo (índice de fotogramas clave, niveles de códec). Se lee al arrancar: reinicie tras un cambio. |
| **Descarga de carátulas** | Guarda carteles y fondos en el servidor, convertidos a WebP, en lugar de cargarlos de Internet cada vez. |
| **Tokens del analizador** | Etiquetas de release y nombres de edición adicionales que el analizador de nombres de archivo debe quitar, cuando sus archivos llevan etiquetas que no conoce. |
| **Claves API** | TMDB (una clave compartida viene incluida), OMDb, OpenSubtitles, TheIntroDB. |
| **Proveedores de salto de intro** | Usa bases comunitarias (TheIntroDB, luego IntroDB) para las marcas de intro, resumen y créditos antes de lanzar la detección del propio servidor. Reinicie tras activarlo o desactivarlo. También puede enviar a TheIntroDB las detecciones seguras del servidor. |
| **Configuración del servidor** (tarjeta) | Nombre del servidor, límites de inicio de sesión y globales, nivel de log, vigilancia de archivos e intervalo de escaneo, **Saltar intros automáticamente** (desactivado por defecto: activado, cada reproductor salta solo las intros detectadas), **Saltar segmentos de avance**, cuánto tiempo se guardan el historial y las estadísticas, el **presupuesto de la caché de transcodificación** (10 GB por defecto) y el techo del AV1 por software. |
| **Actualizaciones** | Comprueba cada 6 horas si se ha publicado una imagen más reciente y se lo dice. Evey nunca se actualiza solo. |

Debajo de las tarjetas, **Entradas de configuración** lista cada ajuste bruto
clave-valor, para el raro caso en que una clave documentada no tenga tarjeta
propia.

### Transcodificación

Cómo convierte vídeo el servidor.

- **Calibración de codificación** muestra lo que el servidor ha medido de sí
  mismo: la resolución más alta que sabe convertir en tiempo real, por software
  y con la tarjeta gráfica, y qué códecs maneja la tarjeta. **Aceleración por
  hardware** elige la tarjeta (**Auto** toma la mejor detectada,
  **Desactivada** fuerza el procesador). **Recalibrar** tras añadir una tarjeta
  gráfica o mover el contenedor a otra máquina. También aquí: **Salida AV1**
  (solo hardware), el **codificador de bajo consumo** de Intel y un **tope de
  bitrate de vídeo**, con otro aparte para los espectadores fuera de casa.
- **Capacidad de reproducción en directo** es el número de vídeos que se pueden
  convertir a la vez. Está medida; solo puede bajarla. Un flujo que solo
  convierte el sonido no cuenta, y un espectador que cambia de calidad conserva
  una sola plaza.
- **Tareas de transcodificación en segundo plano** limita las conversiones en
  cola, aparte de la reproducción en directo.
- **Límites avanzados** limita sesiones y resoluciones en hardware débil. Se leen
  al arrancar.
- **Prueba de carga** simula espectadores reales para medir lo que aguanta el
  servidor. Lánzela solo cuando nadie esté mirando.

Configurar una tarjeta gráfica en Docker:
[Transcodificación y hardware](../transcoding.md).

### Caché de transcodificación y Estado del sistema

**Caché de transcodificación** muestra lo que las conversiones dejaron en el
disco y permite vaciarlo. Tras una actualización que cambia cómo se convierten
los vídeos, vaciarla garantiza que nadie reciba una conversión antigua.
**Estado del sistema** muestra procesador, memoria, ancho de banda,
almacenamiento y la capacidad de conversión en uso.

## Medios

- **Bibliotecas**: añadir, editar, escanear, programar; **Vincular solo
  huérfanos**, **Volver a vincular todo con TMDB** y **Reparar sagas** para los
  metadatos. Vea los [primeros pasos](first-steps.md#add-your-libraries).
- **Cache de artworks**: los carteles y fondos guardados en el servidor.
- **Salud de los medios**: un escaneo de solo lectura de cada archivo MKV y WebM
  emite uno de dos veredictos. **Reparable** (índice de búsqueda ausente, tramo
  dañado, un `.mkv` que en realidad es un MP4) se corrige de una vez con
  **Reparar todo**, y cada archivo reparado conserva su original como copia.
  **Para información** (descarga incompleta, desfase de audio...) lista lo que
  no se puede corregir ahí. Reparar escribe en sus carpetas de medios, que por
  tanto deben estar montadas en lectura-escritura. Más en
  [Salud de los medios](../runbooks/media-health.md).

En un título, un administrador también puede **editar metadatos**, cambiar el
cartel y ajustar los **segmentos de medios** (intro, resumen, créditos) a mano o
relanzar su detección.

## Usuarios y acceso

- **Usuarios**: crear, editar, desactivar o borrar cuentas; restringir cada una
  a ciertas bibliotecas; limitar su bitrate de streaming, su número de perfiles y
  de dispositivos por perfil; ver su historial; **Conectarse como** el usuario
  para ver lo que ve (un aviso se lo recuerda, **Salir** le devuelve).
- **Enlaces de invitación**: un enlace con, a elegir, caducidad, número de usos,
  bibliotecas, límite de perfiles y rol de administrador.
- **Solicitudes de contenido**: la cola de títulos que han pedido los usuarios
  (aceptar, rechazar o aplazar, con una nota), y quién puede pedir: todo el
  mundo por defecto, con excepciones por usuario.

## Integraciones

- **Plugins**: instalar desde un archivo ZIP. Antes de instalar, el
  panel muestra los permisos que declara la extensión y lo que encontró su
  análisis estático. Por extensión: activar, actualizar, límites de recursos
  (memoria, procesos, disco, prioridad), logs. Vea las
  [extensiones](../plugins.md).
- **Webhooks**: llamar a una URL suya cuando pasa algo (un título añadido, una
  reproducción iniciada...), para todos los eventos o algunos elegidos, firmado
  con un secreto si lo proporciona. **Probar** envía un ping.
- **Claves API**: claves para sus propios scripts e integraciones, cada una
  limitada a los ámbitos que marque (leer medios, controlar la reproducción,
  gestionar usuarios...). Una clave solo se muestra una vez, al crearla.
  **Documentación de la API** lista lo que abre cada ámbito y permite probarlo.

## Supervisión y mantenimiento

- **Analítica**: reproducciones de hoy y de esta semana, usuarios activos, los
  títulos más vistos de los últimos 30 días.
- **Actividad**: el historial de reproducciones y de acciones de
  administración, y el trabajo en segundo plano en curso.
- **Logs y depuración**: buscar y seguir en directo los logs del servidor,
  filtrados por ámbito (reproducción, conversión, escaneo...). La primera parada
  cuando un usuario informa de un problema.
- **Calidad de streaming**: cómo fue la reproducción para espectadores reales en
  el último día o la última semana - cuántas veces esperaron al vídeo, los
  títulos que más se atascan y los tipos de dispositivo que más sufren.
- **Copia de seguridad y restauración**: crear una copia, descargarla,
  restaurar una o restaurar desde un archivo. La programación y la conservación
  se tratan en [Copias de seguridad](../backups.md).

Cuando lo que deja de responder es el propio panel, el mismo mantenimiento está
disponible por línea de comandos: [Línea de comandos](../runbooks/cli.md).
