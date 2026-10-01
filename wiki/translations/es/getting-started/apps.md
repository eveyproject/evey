---
source_sha: fb9873d46e2323723ae54ede6035d085a424d9e6c047c3fb833962b8358dd857
reviewed: false
---
# Aplicaciones y dispositivos

Cada aplicación habla con tu propio servidor. Le das la dirección del servidor
una vez, inicias sesión y eliges un perfil. Los puntos de reanudación, el
historial y los favoritos pertenecen al servidor, así que te siguen de un
dispositivo a otro.

| Aplicación | De dónde sale | Inicio de sesión |
| --- | --- | --- |
| **Web** | La sirve el propio Evey, en la dirección del servidor. Nada que instalar. | Usuario y contraseña, o inicio de sesión único si está configurado. |
| **Android** (teléfono, tableta) | La aplicación de Android. | Dirección del servidor, luego usuario y contraseña. |
| **Android TV** | La aplicación de Android TV. | Dirección del servidor, luego **Conexión rápida** (abajo) o usuario y contraseña. |
| **Chromecast** | Ninguna aplicación: se transmite desde la aplicación web en Chrome o desde la de Android. | Usa la sesión del dispositivo que transmite. |

El panel de administración forma parte de la aplicación web, para las cuentas de
administrador.

## Conexión rápida: iniciar sesión en una tele sin escribir la contraseña

En la televisión, elige **Conexión rápida**. Muestra un código y un código QR. En
un teléfono o un ordenador con la sesión ya iniciada, abre **Ajustes → Conexión
rápida** y escribe el código o escanea el QR con la aplicación de Android. La
televisión queda conectada a la misma cuenta.

## Lo que aporta cada aplicación

- **Web.** Atajos de teclado en el reproductor (`?` los lista), notificaciones
  push del navegador (consulta [Notificaciones](../notifications.md)) y el panel
  de administración.
- **Android.**
  - **Descargas** para ver sin conexión. Cuando el administrador lo ha activado
    (`downloads.quality_choice`, consulta
    [Entrega de la reproducción y descargas](../environment/playback-delivery.md)),
    la aplicación ofrece una copia más pequeña preparada por el servidor para una
    película demasiado grande para el teléfono. Una descarga sigue reproducible
    sin conexión durante 30 días sin contactar con el servidor, y solo para los
    perfiles autorizados a verla: un perfil infantil no puede abrir una película
    descargada por otro perfil por encima de su límite de edad.
  - **Transmitir** a un Chromecast.
- **Android TV.** Una interfaz pensada para el mando.

**Reproducir en**, en los reproductores web y de Android, envía lo que estás
viendo a otra aplicación de Evey conectada a la misma cuenta y abierta en ese
momento, y la reproducción continúa allí.

## Qué dispositivo reproduce qué

Cada aplicación informa de lo que su dispositivo puede descodificar, y el
servidor decide para cada título si envía el archivo tal cual o lo adapta. La
misma película puede llegar intacta a una tele y recodificada a un teléfono más
antiguo. El panel **Información del flujo** del reproductor muestra la decisión y
sus motivos: consulta
[Cómo elige Evey el modo de reproducción](../playback/how-playback-is-decided.md).
Transmitir tiene sus propias reglas: consulta [Enviar a Chromecast](../playback/casting.md).
