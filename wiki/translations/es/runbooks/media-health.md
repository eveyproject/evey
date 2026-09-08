---
source_sha: 4abf3c80428cd67c34ec53516788e91cc6019eab33ecc17b138c08d22a4bde30
---

# Salud de los medios (admin → Salud de los medios)

La página Salud de los medios inspecciona todos los MKV y WebM de una
biblioteca y le da a cada archivo UN veredicto. La idea es arreglar de verdad
lo que se puede arreglar y dejar de releer los mismos avisos eternamente, no
mantener una lista de 600 hallazgos sobre los que nadie actúa.

## Los cuatro veredictos

| Veredicto | Qué significa | Qué hace el servidor |
|---|---|---|
| **Sano** | El índice, la sincronía del audio, el contenedor y los tamaños cuadran. | Nada. |
| **Reparable** | Un defecto que el servidor puede arreglar solo, sin riesgo. | **Reparar todo** los corrige todos en segundo plano (véase más abajo). |
| **Usted decide** | Un defecto que solo se arregla en origen, o una reparación con una contrapartida real. | Nada de forma automática. Márquelo como visto, o consiga otra copia del archivo. |
| **Info** | Una curiosidad sin efecto en la reproducción. | Nada. Márquela como vista para despejar la vista. |

Cada fila lleva además las etiquetas de detalle que sostienen su veredicto.
Agrupadas por veredicto:

**Reparable**

- Índice de posicionamiento *ausente*, o presente pero inservible (apoyado en
  la pista de audio, con referencias a pistas que el archivo ya no tiene,
  demasiado escaso para caer cerca del punto buscado). La reparación
  reconstruye los Cues: reescribe el archivo en una copia temporal, lo
  revalida entero (pistas, duración, índice) y lo intercambia. Cura el
  posicionamiento lento y la reanudación lenta.
- *Audio +N s*: una pista de audio empieza tarde respecto al vídeo (un defecto
  de multiplexado, los clásicos labios por delante del sonido). La reparación
  la realinea en el sitio.
- *Contenedor incorrecto*: el `.mkv` es en realidad un MP4 o un MOV. No hay
  índice Matroska que construir; la reparación **lo renombra a `.mp4`** y
  actualiza la entrada de la biblioteca. No se reescribe nada.
- *Dañado ~tamaño*: un tramo corrupto en mitad del archivo que una reescritura
  estricta rechaza. La reparación puede **resincronizar**: se salta el tramo
  dañado y vuelve a sincronizar lo que viene después. Esos bytes se pierden
  para siempre, así que el diálogo de reparación lo pregunta de forma explícita
  (*Resincronizar también N archivo(s) dañado(s)*); sin marcar esa casilla, los
  archivos dañados se dejan en paz.

**Usted decide**

- *Incompleto - falta el final ~tamaño*: el final declarado del archivo cae más
  allá de su tamaño real, y el recorrido confirmó el corte. Ninguna herramienta
  recupera esa cola: el tamaño le dice si se trata de un cabo de 9 KB
  (invisible al reproducir) o de una escena final que falta. La única
  corrección real es conseguir otra copia.
- *Audio -N s*: una pista de audio termina N segundos **antes que la imagen**
  (medido contenido contra contenido, nunca contra la duración declarada del
  contenedor, que no es más que una afirmación del multiplexor). La
  reproducción rellena el final con silencio; ese audio, sencillamente, no está
  en el archivo.
- *Posicionamiento impreciso*: el índice se reconstruyó y queda un hueco. El
  detalle dice por qué: fotogramas clave que nadie apuntó en los Cues (una
  segunda reconstrucción cierra el hueco), un tramo sin ningún fotograma clave
  (solo una recodificación lo hace posicionable) o **un tramo sin imagen
  alguna**, donde el vídeo se congela haga lo que haga el índice y el único
  remedio es la fuente.

**Info**

- *Bytes sobrantes*: bytes de más después del final declarado del Segment (un
  añadido abortado, el diario de un cliente de torrents). El medio está
  completo y se reproduce bien; cualquier reescritura futura se los lleva por
  delante.

## Marcar como visto

*Visto, dejar de avisar* (por archivo) y *Marcar todo como visto* (por grupo)
sacan un hallazgo de la vista *En espera*. La marca va unida a la identidad del
archivo **y a sus bytes**: si el archivo cambia (de tamaño o de mtime), el
siguiente análisis lo diagnostica de nuevo y el aviso vuelve. *Avisar de nuevo*
lo deshace. Una marca puesta mientras hay un análisis en marcha se conserva
cuando ese análisis termina, siempre que el archivo no haya cambiado.

Las vistas de la parte de arriba de la tabla (*En espera*, *Reparable*, *Usted
decide*, *Info*, *Vistos*, *Copias*, *Todos*) y el cuadro de búsqueda son la
forma de ir despachando una biblioteca grande: *En espera* es la lista de
tareas, *Vistos* es lo que usted ya decidió.

## Los análisis, y por qué el primero tras una actualización es largo

Un análisis lee de cada archivo solo la cabecera (unos pocos KB) y reutiliza el
veredicto anterior cuando el archivo no ha cambiado. Dos cosas obligan a
diagnosticarlo todo de cero:

- que el archivo haya cambiado (de tamaño o de mtime);
- **que hayan cambiado los detectores del servidor**: una regla nueva, una
  referencia corregida, un umbral movido. Cada uno de esos cambios sube una
  versión de diagnóstico interna, y el siguiente análisis vuelve a diagnosticar
  la biblioteca entera una vez, en lugar de enseñar un veredicto que una regla
  más reciente ya no daría. En un NAS y con una biblioteca grande, eso son
  horas; los análisis siguientes vuelven a ser rápidos.

## Copias de seguridad y restauración

**Lea esto antes de tirar de *Restaurar* en plena incidencia: que exista o no
una copia física del original depende de la estrategia de reparación, y una de
las tres no guarda ninguna.**

| Estrategia | Copia física | Cómo se restaura |
|---|---|---|
| `rewrite` | un `.mkvgobak` junto al archivo | Se repone la copia |
| `auto` | un `.mkvgobak` (recae en una reescritura) | Se repone la copia |
| `inplace` | **ninguna, nunca** | Se aplica el desplazamiento inverso anotado en el diario de reparaciones |

`inplace` es un parche quirúrgico (una escritura en el archivo, sin remux) y es
la vía que toma una realineación de audio (la etiqueta «Audio +N s»). No se
copia nada: la vuelta atrás es la entrada del diario en `audit_log`, que lleva
el desplazamiento aplicado, más el hecho de que el desplazamiento contrario lo
deshace. Si esa entrada desaparece, desaparece la restauración.
`admin_media.proto` es la referencia de las tres estrategias; `inplace` ignora
tanto *Conservar copias de seguridad* como el número de copias que se
retienen.

Para las dos estrategias que sí guardan copia: *Restaurar* repone un archivo y
*Restaurar todo* repone el ámbito seleccionado. Cuando las copias fueran a
desbordar el volumen, la página lo dice y sugiere desactivarlas; entonces solo
se conservan las últimas durante el trabajo y se eliminan al final, y cada
reparación queda verificada antes del intercambio.

Las reparaciones necesitan las carpetas de medios montadas con **lectura y
escritura** en el contenedor (sin `:ro` en el montaje). La página le dice qué
archivos están en un volumen de solo lectura.

## Equivalentes en la línea de órdenes

El mismo motor está disponible desde la CLI del operador (véase `cli.md`):

- `evey admin reindex-no-cue --root <dir> [--apply] [--resync]`: la
  reconstrucción del índice, en simulación por defecto;
- `evey admin validate-streaming --root <dir>`: el informe de solo lectura
  sobre si el streaming está listo.
