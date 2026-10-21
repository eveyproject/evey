---
source_sha: bb59efb19b44f714d153c63f29645a30745cd729e75bb9e92bae2127c34c3296
reviewed: true
---
# ac3go en Evey

**En resumen:**

- Chrome, Firefox y Edge no pueden reproducir sonido Dolby Digital (AC-3)
  ni Dolby Digital Plus (E-AC-3). En lugar de que el servidor lo convierta,
  el reproductor web de Evey lo descodifica dentro de la propia página, con
  ac3go, un descodificador que funciona en el navegador como WebAssembly.
- El sonido te llega tal como está en el archivo, y el servidor no tiene que
  convertir nada del audio para ello.
- No tienes nada que configurar. **Información del flujo** en el reproductor
  muestra el audio como *descodificado en el cliente*, y **Administración →
  Reproduciendo ahora** muestra *ac3go en el reproductor* para esa pista.
- Si el descodificador no puede funcionar, el reproductor pasa por sí solo al
  sonido convertido por el servidor (AAC).

El resto de esta página explica cuándo se toma este camino, cómo el sonido
se mantiene sincronizado con la imagen y qué revisar cuando una película se
reproduce sin sonido. Qué pista de audio suena, y cuándo se copia o se
convierte en general, está en [Audio](audio.md).

ac3go se desarrolla como una biblioteca aparte, escrita en Go y sin
dependencias: [github.com/gravity-zero/ac3go](https://github.com/gravity-zero/ac3go).

## Cuándo el navegador descodifica el sonido por sí mismo

Tienen que cumplirse todas estas condiciones:

| Condición | Si no |
| --- | --- |
| El navegador no puede reproducir AC-3 ni E-AC-3 por sí mismo, y ejecuta WebAssembly. El reproductor pone a prueba el navegador en ambos puntos en vez de reconocerlo por su nombre. | Un navegador que descodifica Dolby Digital por sí mismo, como Safari, recibe la pista como una pista normal. |
| La pista de audio que suena es AC-3 o E-AC-3. Las demás pistas de la película no cuentan. | Otro códec sigue las reglas de [Audio](audio.md). |
| La pista tiene como mucho seis canales (5.1), y no más de los **Canales de audio máximos** del perfil. Este camino entrega la pista tal como está en el archivo, así que no puede reducir los canales. | El servidor convierte la pista a AAC, que puede reducir los canales para un perfil en estéreo. Una pista E-AC-3 en 7.1 siempre se convierte. |
| La película pasa por el flujo del servidor (Remux o transcodificación), no por la lectura directa del archivo. | La pista sigue la decisión tomada para esa reproducción. |

## Cómo se reproduce

La imagen pasa por el reproductor habitual del navegador, en silencio. El
sonido llega por un flujo de audio aparte, que el servidor indica en la lista
de reproducción: la pista tal como está en el archivo, empaquetada por
[MKVGo](mkvgo.md) cuando puede, o copiada por FFmpeg si no. Ninguno de los
dos la descodifica.

En la página, un hilo en segundo plano (un *worker*) carga ac3go y
descodifica cada segmento de audio a sonido sin comprimir, lejos del código
que dibuja la imagen. Después el reproductor reproduce ese sonido por la salida de
audio del navegador, al ritmo de la imagen: la imagen hace de reloj y el
sonido se programa sobre ella. Una pausa detiene las dos cosas a la vez. Un
salto en la película, o un hueco después de que el flujo se atascara, vuelve
a alinear el sonido con la nueva posición.

Una pista 5.1 se entrega al navegador en el orden de canales que espera, para
que pueda reducirla a estéreo para altavoces o auriculares.

Al empezar una película, el descodificador tarda un momento en cargarse.
Cuando el servidor ofrece también la pista convertida a AAC, ese sonido suena
durante ese momento, y ac3go toma el relevo en cuanto está listo. El
navegador descarga el descodificador una sola vez y lo vuelve a comprobar en
cada uso, así que una actualización del servidor nunca mezcla una copia
antigua y una nueva.

## Si ac3go no puede funcionar

Cuando el descodificador no se carga, falla en un segmento o el servidor no
le indicó ningún flujo de audio, el reproductor deja de usarlo durante el
resto del título y pide al servidor el flujo normal, con el sonido
convertido a AAC. La película conserva el sonido; **Información del flujo**
muestra entonces la conversión en lugar de *descodificado en el cliente*.

!!! warning
    Un proxy inverso que pone su propia Content-Security-Policy debe
    permitir `'wasm-unsafe-eval'` en `script-src`; si no, el descodificador
    no puede arrancar. También debe servir los archivos del descodificador
    en `/wasm/`, como el resto de la aplicación web. Consulta
    [Configuraciones de proxy inverso](../reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own).

## En un receptor de Chromecast

El receptor de Chromecast de Evey no usa ac3go. En cajas de TV con poca
memoria, los receptores que descodificaban el sonido de esta forma se
detenían tras unos minutos de reproducción. La TV recibe Dolby Digital tal
cual cuando su receptor indica que lo deja pasar, y el AAC del servidor si
no: consulta [Enviar a Chromecast](casting.md).

## La biblioteca y su licencia

ac3go se publica bajo la GNU AGPLv3, como Evey. Se compila desde sus fuentes
al construir la imagen de Evey (a WebAssembly, con las herramientas de Go), y
su licencia se sirve junto al descodificador, en `/wasm/ac3go.LICENSE.md`.
