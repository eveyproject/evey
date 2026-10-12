---
source_sha: 181fd5fb4e9492dc7feeae765f9c469162e43e7a56a39a6399be584d8e660049
reviewed: true
---
# Salud de medios

Un título tarda en saltar, se queda parado en un punto, o su sonido se
desfasa: **Administración → Salud de medios** lee todos los archivos MKV/WebM
de tus bibliotecas y da a cada uno una sola respuesta. La idea es una página
que se va vaciando: lo que Evey puede arreglar, lo arregla (**Reparar
todo**); lo demás se menciona una vez y se puede marcar como visto.

<figure class="evey-shot" markdown="span">
  ![Administración → Salud de medios después de un escaneo: un archivo por reparar, una observación](../images/admin-media-health.webp){ loading=lazy }
  <figcaption>Un escaneo con un archivo que Evey puede reparar (le falta el índice de posicionamiento) y una observación (bytes sobrantes después del final del archivo, sin consecuencias) (interfaz en inglés).</figcaption>
</figure>

## Dos veredictos

Un archivo que no está **Sano** (el índice, el contenedor, los tamaños y el
tiempo del audio están bien) recibe una de dos respuestas, y a propósito solo
dos:

| Veredicto | Qué significa | Qué hace Evey |
| --- | --- | --- |
| **Reparable** | Un defecto que Evey repara en este servidor. | **Reparar todo** se encarga de ellos en segundo plano; su número es exactamente el número de estos archivos. |
| **Info** | Algo que se ha medido en el archivo y que Evey no reescribe por su cuenta. | Nada. Márcalo como visto. |

Ninguno de los dos te pide trabajo manual. Una medida sobre la que Evey no
puede actuar es solo una observación, y una medida puede equivocarse: si el
archivo se reproduce correctamente, no hay nada que hacer.

### Reparable

- **Sin índice de posicionamiento**, o con uno que no se puede usar. El
  síntoma es un salto lento o una reanudación lenta. El índice se reconstruye
  en una copia temporal, la copia se comprueba entera (pistas, duración,
  índice) y después sustituye al archivo.
- **Contenedor incorrecto**: el `.mkv` es en realidad un archivo MP4 o MOV.
  Se renombra a `.mp4` y la biblioteca lo sigue; no se reescribe nada.
- **Tramo dañado**: una zona corrupta en mitad del archivo. La reparación se
  la salta y vuelve a sincronizar lo que viene después, así que esos bytes se
  pierden para siempre. La ventana de reparación lo pide de forma explícita
  (**Resincronizar también N archivos dañados**); si no lo marcas, los
  archivos dañados no se tocan. Lo mismo vale para un archivo cuya corrupción
  solo aparece mientras se reescribe (**Corrupción hallada durante la
  reparación**): el escaneo solo lee las cabeceras y no podía verla.
- **Pistas guardadas por bloques**: minutos de vídeo, luego minutos de audio,
  en lugar de ir juntos. Ningún reproductor pasa del primer bloque. **Volver
  a intercalar las pistas** las reescribe juntas sin volver a codificar, y
  comprueba que no falta ningún paquete antes de sustituir el archivo.

### Info

- **Descarga incompleta**: el final del archivo nunca llegó. Se reproduce
  hasta el corte. Ninguna herramienta recupera la parte que falta; solo la
  fuente.
- **Búsqueda imprecisa**: el índice reconstruido todavía tiene un hueco, casi
  siempre en los créditos finales. Un salto dentro del hueco cae en su borde;
  en el resto del archivo, los saltos son exactos.
- **Audio a +N s**: una pista de audio empieza después de la imagen. Así se
  ajusta un doblaje a un montaje, y todos los reproductores lo respetan. Evey
  nunca lo reescribe.
- **Pistas de audio a N s de diferencia**: las pistas de audio del propio
  archivo no coinciden entre sí, así que una de ellas está mal colocada.
  Ninguna medida puede decir cuál.
- **Sonido a N s de la imagen**: una pista de audio está lejos de donde los
  subtítulos, sincronizados con la imagen, sitúan los diálogos.
- **Pista de audio llevada a 0**: la marca que deja una reparación anterior
  cuando retrasó una pista para quitarle su retardo declarado. Si el doblaje
  suena ahora demasiado pronto, la cantidad quitada ya no está en el archivo.
- **Audio -N s**: una pista de audio termina antes que la imagen. La
  reproducción rellena el hueco con silencio.
- **Bytes sobrantes**: bytes de más después del final del contenido del
  archivo. Se reproduce bien.
- **Ilegible**: el escaneo no ha podido leer el archivo (falta, no se puede
  leer, o es un contenedor que el analizador ha rechazado). Revisa el punto
  de montaje o la fuente.

## Comprobar la sincronía a ojo

Para las tres observaciones de audio, **Comprobar sincronía**, en la fila del
archivo, prepara un clip corto en el servidor: eliges un momento, una pista y
un desfase que probar, y luego lo ves, a cámara lenta si hace falta. Comparar
pistas dice si coinciden entre sí, nunca cuál sigue a la imagen; solo verlo
lo resuelve.

**Aplicar N s a esta pista** desplaza esa pista el desfase elegido. El cambio
queda registrado como tu criterio, no como una medida, y **Historial** puede
deshacerlo.

## Marcar como visto

**Visto, dejar de avisar** (un archivo) y **Marcar todo como visto** (un
grupo) sacan una observación Info de la vista **En espera**. Solo se pueden
marcar las observaciones: un defecto reparable se repara. La marca vale para
el archivo tal como es: si cambian su tamaño o su fecha, el siguiente escaneo
lo vuelve a diagnosticar. **Avisar de nuevo** quita la marca.

Las vistas encima de la tabla (**En espera**, **Reparable**, **Info**,
**Vistos**, **Copias**, **Todos**) y el cuadro de búsqueda sirven para
recorrer una biblioteca grande: **En espera** es la lista de tareas.

## Escaneos

Un escaneo lee el principio de cada archivo (unos pocos kilobytes) y
reutiliza la respuesta anterior para un archivo que no ha cambiado. **Uso de
los discos** marca el ritmo:

- **Discreto**: un archivo cada vez, y después una pausa tan larga como la
  lectura. Los discos quedan libres más o menos la mitad del tiempo, así que
  alguien puede seguir viendo algo.
- **Rápido**: varios archivos a la vez, sin pausa. Unas tres veces más rápido
  en una biblioteca repartida en varios discos, pero mientras tanto la
  reproducción puede ir a tirones.

Dos cosas hacen que un escaneo vuelva a diagnosticarlo todo: un archivo que
ha cambiado, y una nueva versión de Evey cuyas comprobaciones han cambiado.
Por eso, el primer escaneo después de una actualización así es largo en una
biblioteca grande; los siguientes vuelven a ser rápidos.

## Copias de seguridad, historial y deshacer

**Conservar copias de seguridad** está activado por defecto: cada archivo
reescrito guarda su original como un `.mkvgobak` a su lado. **Restaurar**
recupera un archivo, **Restaurar todo** el conjunto seleccionado, y
**Eliminar copias** las borra para siempre. Si está desactivado, solo existen
las últimas copias durante una pasada y se borran al terminar; cada
reparación se comprueba entonces más a fondo antes de sustituir el archivo.
Cuando guardar una copia de cada archivo llenaría el volumen, la página lo
avisa antes de la pasada.

El **Diario de reparaciones** recoge cada cambio que una reparación ha hecho
en los bytes de un archivo, en toda la biblioteca; **Historial** lo muestra
para un solo archivo. Algunos cambios se pueden deshacer desde ahí sin
ninguna copia:

| Cambio | Deshacer |
| --- | --- |
| Pista de audio desplazada | Aplica exactamente el desplazamiento contrario. |
| Renombrado a `.mp4` | Le devuelve su nombre. |
| Índice reconstruido | Nada que deshacer: la imagen y el sonido están intactos. |
| Tramo dañado omitido | Solo la copia de seguridad recupera los bytes perdidos; por eso esta reparación siempre guarda una. |

<figure class="evey-shot" markdown="span">
  ![Administración → Salud de medios después de Reparar todo, con el diario de reparaciones abierto](../images/admin-media-health-repaired.webp){ loading=lazy }
  <figcaption>Después de <strong>Reparar todo</strong>: el índice se ha reconstruido, el diario lo registra y el archivo sale de la cola; la observación se queda. Este índice se ha corregido directamente dentro del archivo, sin guardar copia: solo un archivo que hay que reescribir guarda una copia de seguridad (interfaz en inglés).</figcaption>
</figure>

Las reparaciones escriben en tus carpetas de medios. Tienen que estar
montadas en lectura y escritura en el contenedor (sin `:ro` en el volumen);
después de una pasada que se ha encontrado un volumen de solo lectura, la
página lo indica. El escaneo funciona en los dos casos.

## Desde la línea de comandos

El mismo motor está disponible desde la CLI del operador, consulta
[Línea de comandos](cli.md#media-maintenance):

- `evey admin reindex-no-cue --root <dir> [--apply] [--resync]` reconstruye
  los índices que faltan, como simulación salvo que se indique `--apply`;
- `evey admin validate-streaming --root <dir>` muestra el informe de solo
  lectura que dice si los archivos están listos para el streaming.

<!-- sources
verdicts (two non-healthy answers, the dropped third one): internal/app/maintenance/seek_index.go - Verdict, SeekStatus.Verdict, Ackable ; labels: i18n admin.seekIndex.verdictHealthy/verdictRepairable/verdictInformational
labels and hints: i18n admin.seekIndex.*, admin.repairJournal.*, admin.repairHistory.*, admin.syncPreview.*
what can be undone without a copy: internal/app/maintenance/repair_journal.go - RepairRecord.Reversible
apply an audio offset: internal/adapters/driving/grpcapi/admin_media.go - ApplyAudioShift
keep backups on by default: frontend/apps/web/src/features/admin/seekIndex/hooks.ts
-->
