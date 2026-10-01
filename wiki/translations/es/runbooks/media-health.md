---
source_sha: 20c15f25fe69f68ab885ee03c012f737103bebcc94ce6da6d3efd20c3bbaff60
reviewed: false
---
# Salud de medios

**Admin → Salud de medios** lee cada archivo MKV/WebM de tus bibliotecas y da a
cada uno una sola respuesta. El objetivo es una página que se vacía: lo que Evey
puede arreglar, lo arregla; el resto se menciona una vez y se puede marcar como
visto.

<figure class="evey-shot" markdown="span">
  ![Admin → Salud de medios tras un escaneo de las bibliotecas de demostración](../images/admin-media-health.webp){ loading=lazy }
  <figcaption>Un escaneo sin nada pendiente: todos los archivos están sanos (interfaz en inglés).</figcaption>
</figure>

## Dos veredictos

| Veredicto | Lo que significa | Lo que hace Evey |
| --- | --- | --- |
| **Sano** | Índice, contenedor, tamaños y tiempos del audio son correctos. | Nada. |
| **Reparable aquí** | Un defecto que Evey repara en este servidor. | **Reparar todo** se ocupa de ellos en segundo plano. |
| **Información** | Algo medido en el archivo que Evey no reescribe por su cuenta. | Nada. Márcalo como visto. |

No hay un tercer veredicto que te deje una tarea. Una medición sobre la que Evey
no puede actuar es una observación, y una medición puede equivocarse: si el
archivo se reproduce bien, está bien.

### Reparable aquí

- **Sin índice de búsqueda** (el índice de posicionamiento del archivo), o con
  uno que no se puede usar. El síntoma es un salto o una reanudación lentos. El
  índice se reconstruye en una copia temporal, la copia se comprueba por
  completo (pistas, duración, índice) y después sustituye al archivo.
- **Contenedor incorrecto**: el `.mkv` es en realidad un archivo MP4 o MOV. Se
  renombra a `.mp4` y la biblioteca lo sigue; no se reescribe nada.
- **Tramo dañado**: una región corrupta en mitad del archivo. La reparación la
  salta y resincroniza lo que sigue, así que esos bytes se pierden para siempre.
  El diálogo de reparación lo pide de forma explícita (**Resincronizar también N
  archivos dañados**); si la casilla queda sin marcar, los archivos dañados no se
  tocan. Lo mismo vale para un archivo cuya corrupción solo aparece al
  reescribirlo (**Corrupción hallada durante la reparación**): el escaneo solo
  lee las cabeceras y no podía verla.
- **Pistas guardadas por bloques**: minutos de vídeo y luego minutos de audio, en
  lugar de estar uno junto al otro. Ningún reproductor pasa del primer bloque.
  **Volver a intercalar las pistas** las reescribe una junto a otra sin
  recodificar y comprueba que ha llegado cada paquete antes de sustituir el
  archivo.

### Información

- **Descarga incompleta**: el final del archivo nunca llegó. Se reproduce hasta
  el corte. Ninguna herramienta recupera la parte que falta; solo la fuente
  puede.
- **Búsqueda imprecisa**: el índice reconstruido todavía tiene un hueco, casi
  siempre en los créditos finales. Un salto dentro del hueco cae en su borde; en
  el resto, los saltos son exactos.
- **Audio a +N s**: una pista de audio empieza después de la imagen. Así se
  alinea un doblaje con un montaje, y todos los reproductores lo siguen. Evey
  nunca lo reescribe.
- **Pistas de audio a N s de diferencia**: las propias pistas de audio del
  archivo no coinciden, así que una de ellas está en el sitio equivocado. Ninguna
  medición puede decir cuál.
- **Sonido a N s de la imagen**: una pista de audio está lejos de donde los
  subtítulos, sincronizados con la imagen, sitúan el diálogo.
- **Pista de audio llevada a 0**: la marca que deja una reparación anterior
  cuando adelantó una pista en su retardo declarado. Si el doblaje ahora va
  adelantado, la cantidad eliminada ya no está en el archivo.
- **Audio -N s**: una pista de audio termina antes que la imagen. La
  reproducción rellena el hueco con silencio.
- **Bytes sobrantes**: bytes de más después del final del contenido del archivo.
  Se reproduce bien.
- **Ilegible**: el escaneo no pudo leer el archivo (no existe, no se puede leer,
  o un contenedor que el analizador rechazó). Comprueba el montaje o la fuente.

## Comprobar la sincronía a simple vista

Para las tres observaciones de audio, **Comprobar sincronía**, en la fila del
archivo, genera un clip corto en el servidor: elige un momento, una pista y un
desfase que probar, y míralo, ralentizado si hace falta. Comparar pistas dice si
coinciden, nunca cuál sigue a la imagen; solo mirarlo lo decide.

**Aplicar N s a esta pista** desplaza esa pista en el desfase elegido. El cambio se guarda como tu
juicio, no como una medición, y el **Historial** puede deshacerlo.

## Marcar como visto

**Visto, dejar de avisar** (un archivo) y **Marcar todo como visto** (un grupo)
sacan una observación de Información de la vista **En espera**. Solo se pueden
marcar observaciones: un defecto reparable se repara. La marca pertenece al
archivo tal como es: si cambian su tamaño o su fecha, el siguiente escaneo lo
diagnostica de nuevo. **Avisar de nuevo** quita la marca.

Las vistas encima de la tabla (**En espera**, **Reparable**, **Info**,
**Vistos**, **Copias**, **Todos**) y el cuadro de búsqueda son la forma de
recorrer una biblioteca grande: **En espera** es la lista de tareas.

## Escaneos

Un escaneo lee el principio de cada archivo (unos pocos kilobytes) y reutiliza la
respuesta anterior para un archivo que no ha cambiado. **Uso de los discos** fija
el ritmo:

- **Discreto**: un archivo cada vez y luego una pausa tan larga como duró la
  lectura. Los discos quedan libres más o menos la mitad del tiempo, para que
  alguien pueda seguir viendo algo.
- **Rápido**: varios archivos a la vez, sin pausa. Unas tres veces más rápido en
  una biblioteca repartida en varios discos, pero la reproducción puede
  entrecortarse mientras tanto.

Dos cosas hacen que un escaneo lo diagnostique todo de nuevo: un archivo que ha
cambiado y una nueva versión de Evey cuyas comprobaciones han cambiado. El primer
escaneo tras una actualización así es por tanto largo en una biblioteca grande;
los siguientes vuelven a ser rápidos.

## Copias, historial y deshacer

**Conservar copias de seguridad** está activado por defecto: cada archivo
reescrito conserva su original como `.mkvgobak` a su lado. **Restaurar** devuelve
un archivo, **Restaurar todo** el ámbito seleccionado, y **Eliminar copias** las
borra para siempre. Desactivado, solo existen las últimas copias durante una
pasada y se eliminan al final; cada reparación se verifica entonces más a fondo
antes de sustituir el archivo. Cuando conservar una copia de cada archivo
llenaría el volumen, la página lo dice antes de la pasada.

El **Diario de reparaciones** lista cada cambio que una reparación hizo en los
bytes de un archivo, en toda la biblioteca; el **Historial** lo muestra para un
archivo. Algunos cambios se pueden deshacer desde ahí sin ninguna copia:

| Cambio | Deshacer |
| --- | --- |
| Pista de audio desplazada | Aplica exactamente el desplazamiento contrario. |
| Renombrado a `.mp4` | Le devuelve su nombre. |
| Índice reconstruido | Nada que deshacer: la imagen y el sonido no se tocan. |
| Tramo dañado omitido | Solo la copia devuelve los bytes perdidos, por eso esta reparación siempre conserva una. |

Las reparaciones escriben en tus carpetas de medios. Deben estar montadas con
lectura y escritura en el contenedor (sin `:ro` en el volumen); tras una pasada
que se topó con un volumen de solo lectura, la página lo dice. El escaneo
funciona en ambos casos.

## Desde la línea de comandos

El mismo motor está disponible en la CLI de operación (consulta `cli.md`):

- `evey admin reindex-no-cue --root <dir> [--apply] [--resync]` reconstruye los
  índices que faltan, como simulación salvo que se indique `--apply`;
- `evey admin validate-streaming --root <dir>` muestra el informe de solo lectura
  sobre la preparación para el streaming.
