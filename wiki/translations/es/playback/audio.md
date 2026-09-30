---
source_sha: 671aed4ea8f4b0ef01dac83a6cadd4f8e569eb5206951057955cbf29a9e7b727
reviewed: false
---
# Audio

La pista de audio se juzga por separado del vídeo. En la mayoría de las
transcodificaciones es lo único que se recodifica: el vídeo se copia y el sonido
se convierte a un formato que el reproductor lee.

## Qué pista se reproduce

Una pista elegida a mano, en la página del título o en el reproductor, siempre
gana. Si no, los reproductores siguen el **Idioma de audio por defecto** del
perfil, después la pista que el archivo marca como predeterminada y después la
primera. Una pista marcada como comentario se salta en favor de una normal cuando
el archivo tiene ambas.

Solo se juzga la pista en reproducción: una pista DTS que no estás escuchando
nunca provoca una transcodificación.

## Copiada o recodificada

La pista se copia cuando el reproductor descodifica su códec y tiene canales
suficientes para ella, dentro de los **Canales de audio máximos** del perfil
(Estéreo, 5.1 o 7.1). Si no, se recodifica a AAC:

- con tantos canales como permitan la fuente, el reproductor y el perfil, hasta
  5.1 (una fuente 7.1 se reduce a 5.1, que todos los reproductores descodifican;
  las fuentes estéreo siguen en estéreo);
- sea cual sea la frecuencia de muestreo, que por sí sola nunca es motivo para
  recodificar.

Una pista copiada dentro de un flujo transcodificado o remuxado también debe
estar en un formato que el reproductor acepte dentro de los segmentos del flujo.
AAC, AC-3 y E-AC-3 siempre lo están; los demás códecs solo cuando el reproductor
lo indica. Cuando no es así, la pista se recodifica e Información del flujo lo
dice: un reproductor puede descodificar MP3 en un archivo y aun así rechazarlo
dentro de un flujo.

## Dolby Digital en un navegador

La mayoría de los navegadores no pueden descodificar AC-3 ni E-AC-3. En lugar de
recodificar la pista en el servidor, el reproductor web la descodifica él mismo,
con un descodificador que se ejecuta en la página: la pista se envía sin cambios
e Información del flujo la muestra como *descodificado en el cliente*. Los
navegadores que descodifican Dolby Digital de forma nativa, como Safari, la
reciben como una pista normal; la elección se hace probando el navegador, no
reconociéndolo.

Un proxy inverso que pone su propia Content-Security-Policy debe permitir
`'wasm-unsafe-eval'` en `script-src`; si no, el descodificador no puede arrancar
y la película se reproduce sin sonido. Consulta
[Configuraciones de proxy inverso](../reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own).

## Sonido desincronizado

**Sincronización de audio**, en el reproductor, desplaza el sonido del título
actual. Una pista copiada no se puede desplazar, así que el servidor la
recodifica; Información del flujo muestra **Resincronización de audio
solicitada**.

Cuando el desfase está en el propio archivo (una pista de audio que empieza
después de la imagen, o pistas que no coinciden entre sí), **Admin → Salud de
medios** lo indica pero nunca lo reescribe por su cuenta: un desfase que empieza
tarde suele ser un ajuste deliberado de un doblaje. **Comprobar sincronía**, en
la fila del archivo, te permite juzgarlo a simple vista y aplicar un
desplazamiento, que su historial puede deshacer. Consulta
[Salud de medios](../runbooks/media-health.md).
