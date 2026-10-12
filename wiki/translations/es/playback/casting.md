---
source_sha: 8c3a42e5732d38c6b13e080b8869dff85cef64c692ff15ed7ef067e6d0e686e9
reviewed: true
---
# Enviar a Chromecast

Con el envío a Chromecast, una película pasa de tu teléfono o tu ordenador
al televisor.

1. Pon la película en el reproductor web en Chrome, o en la app de Android.
2. Pulsa **Transmitir** en el reproductor.
3. Elige tu Chromecast, o tu televisor con Chromecast integrado.

La película se reproduce entonces en el televisor. La app de Android TV no
tiene botón **Transmitir**: ya es el televisor. Firefox tampoco muestra el
botón **Transmitir**. Con Safari, o con Chrome en un teléfono Android, el
botón funciona de otra manera: consulta
[Navegadores sin sesión de Cast propia](#browsers-without-a-cast-session-of-their-own).

Es normal que la imagen del televisor no sea igual que la de tu teléfono. El
televisor no recibe lo que reproducía tu teléfono: pide la película al
servidor por su cuenta, y el servidor prepara una versión que le va bien al
televisor. El resto de la página explica cómo, para los curiosos y para la
persona que gestiona Evey.

## Qué reproduce el televisor

Quien decodifica el flujo es el televisor, así que la decisión de
reproducción usa las capacidades del receptor, tomadas de la fuente más
fiable que haya:

1. **Lo que el receptor ha comprobado en sí mismo.** El receptor de Evey
   prueba su propio dispositivo al arrancar (qué códecs y resoluciones puede
   mostrar, si muestra HDR, si deja pasar el audio Dolby, cuántos canales de
   audio lleva su salida) y lo comunica. Cuando llega este informe, es el que
   se usa.
2. **Si no, el modelo.** El nombre que da el SDK de Cast (Chromecast with
   Google TV, Chromecast Ultra, Google TV Streamer...) se corresponde con las
   capacidades documentadas de esa familia.
3. **Si no, un perfil prudente.** Un dispositivo desconocido, un receptor que
   no respondió a tiempo, o un dispositivo encontrado sin nombre de modelo
   recibe las capacidades del Chromecast más antiguo: H.264 hasta 1080p, SDR,
   sin Dolby.

El orden sigue el precio de equivocarse: suponer una capacidad que el
dispositivo no tiene da una pantalla negra, mientras que suponer de menos
solo cuesta una transcodificación. En la práctica: el HDR solo llega al
televisor si su receptor ha dicho que tiene una pantalla HDR; el Dolby
Digital (AC-3, E-AC-3) solo se envía tal cual a un receptor que ha dicho que
lo deja pasar de forma nativa, y si no, se vuelve a codificar en AAC. Un
receptor que dijo tener sonido envolvente y luego no consiguió cargar el
flujo vuelve a cargarlo una vez, limitado a estéreo.

## Cuando la conexión del televisor es lenta

Antes de pedir un flujo, el receptor mide su propia velocidad de conexión y
la envía con la petición. El servidor mantiene el vídeo original cuando esa
velocidad lo soporta con un margen del 20 %. Si no, elige la calidad más alta
que soporta la conexión. Si aun así la reproducción se detiene, el receptor
vuelve a cargar una vez en la calidad más baja, siempre que eso reduzca el
caudal a la mitad. La Información del flujo muestra entonces **La línea
medida por el cliente no puede llevar la copia** o **Escalón de supervivencia
pedido por el reproductor**.

Los dispositivos de Cast más antiguos no cambian de calidad por sí solos
durante la reproducción; por eso el servidor hace esa elección por ellos.

## Navegadores sin sesión de Cast propia

En el reproductor web, el botón **Transmitir** aparece cuando el navegador
puede enviar a otro dispositivo: Chrome en un ordenador, que abre su selector
de Google Cast, y los navegadores que admiten la reproducción remota, como
Safari o Chrome en Android. Firefox no muestra ninguno.

Cuando el navegador no puede abrir por sí mismo una sesión de Cast (Safari,
Chrome en Android), al tocar el botón la petición pasa por el servidor, que
prueba en este orden y se queda con el primer dispositivo que encuentra en
cada paso:

1. un Chromecast en la red del dispositivo que tienes en la mano,
   reproducido a través del receptor de Evey como se explica arriba;
2. un televisor DLNA en la red del servidor. Recibe el flujo que estaba
   reproduciendo el navegador (el archivo original en Reproducción directa, el
   flujo transcodificado en los demás casos): no se toma ninguna decisión
   para el televisor en sí;
3. tus propias apps de Evey abiertas y con la sesión iniciada en la misma
   cuenta: si hay una sola, se queda con la película, como con **Reproducir
   en**; si hay varias, el reproductor te indica **Reproducir en** para
   elegir;
4. el selector de dispositivos del propio navegador.

!!! note
    Los pasos 1 y 2 eligen el primer dispositivo que encuentran, no uno que
    elijas tú. Si tienes varios Chromecast en casa, envía desde Chrome en un
    ordenador o desde la app de Android, que muestran los dos un selector.

## Cuando el envío no funciona

[El envío se congeló, se quedó sin sonido, o el teléfono no deja de cargar](../runbooks/incident-runbook.md#a-cast-froze-went-silent-or-the-phone-keeps-spinning).
