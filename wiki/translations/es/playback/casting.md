---
source_sha: 66951bf73de4e59bd21b6ea9468df82ac79d31950e742e6c87203c05a77dda76
reviewed: false
---
# Enviar a Chromecast

El botón **Transmitir**, en el reproductor web de Chrome y en la aplicación
Android, envía la película a un Chromecast o a un televisor con Chromecast
integrado. El teléfono o el navegador solo inician la sesión: el televisor
reproduce el flujo directamente desde el servidor, a través de la aplicación
receptora propia de Evey, y el servidor decide qué enviar para el televisor, no
para el dispositivo que inició el envío.

## Lo que el servidor sabe del receptor

El flujo lo descodifica el televisor, así que la decisión de reproducción usa las
capacidades del receptor, de la fuente más fiable disponible:

1. **Lo que el receptor observó de sí mismo.** El receptor de Evey prueba su
   propio dispositivo al arrancar (qué códecs y resoluciones puede mostrar, si
   muestra HDR, si deja pasar el audio Dolby, cuántos canales de audio lleva su
   salida) y lo comunica. Cuando llega este informe, es el que se usa.
2. **Si no, el modelo.** El nombre que indica el SDK de Cast (Chromecast con
   Google TV, Chromecast Ultra, Google TV Streamer...) se asocia a las
   capacidades documentadas de esa familia.
3. **Si no, un perfil prudente.** Un dispositivo desconocido, un receptor que no
   respondió a tiempo o un dispositivo encontrado sin nombre de modelo recibe las
   capacidades del Chromecast más antiguo.

El orden sigue el coste de un error: suponer una capacidad que el dispositivo no
tiene da una pantalla negra, mientras que suponer demasiado poco solo cuesta una
transcodificación.

Dolby Digital (AC-3, E-AC-3) solo se envía tal cual a un receptor que indicó que
lo deja pasar de forma nativa; si no, se recodifica a AAC.

## La conexión del televisor

Antes de pedir un flujo, el receptor mide su propio caudal y lo envía con la
petición. El servidor conserva el vídeo original cuando ese caudal lo transporta
con un margen del 20 %. Si no, elige la calidad más alta que transporta la
conexión. Si la reproducción se atasca de todos modos, el receptor recarga una
vez en la calidad más baja, siempre que eso reduzca el bitrate a la mitad.
Información del flujo muestra entonces **La línea medida por el cliente no puede
llevar la copia** o **Escalón de supervivencia pedido por el reproductor**.

Los dispositivos de envío más antiguos no cambian de calidad por sí solos durante
la reproducción; por eso el servidor toma esa decisión por ellos.

## Cuando un envío no funciona

[Un envío se congeló, se quedó sin sonido o el teléfono no para de cargar](../runbooks/incident-runbook.md#8-a-cast-froze-went-silent-or-the-phone-keeps-spinning).
