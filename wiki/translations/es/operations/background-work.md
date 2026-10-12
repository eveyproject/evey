---
source_sha: 935a77e1c7b524e4df14c0a67f9ad57f4571098b770e8c3245fda5189aefc0c3
reviewed: true
---
# Trabajo en segundo plano y carga del servidor

Por qué el servidor está ocupado cuando nadie ve nada, y por qué las
miniaturas de una biblioteca nueva tardan horas: además de la reproducción,
el servidor hace trabajo propio (escanear bibliotecas, descargar imágenes,
generar las miniaturas de la barra de progreso, detectar intros y créditos,
analizar archivos). Esta página explica cómo reparte ese trabajo la máquina
con las personas que están viendo algo, y dónde fijar su parte.

## La reproducción va primero

Las tareas en segundo plano usan una parte del procesador, la **Cuota
global** de **Administración → Configuración del servidor → Potencia
asignada**: **25 %** por defecto, **50 %** para acelerarlas cuando nadie usa
el servidor. En **Tareas pesadas**, una de ellas (análisis de contenido
nuevo, miniaturas trickplay, detección de intros, clasificación de
complejidad) puede tener su propia cuota, que sustituye a la global para esa
tarea; las cuotas nunca se suman. Un cambio se aplica al siguiente elemento
procesado, sin reiniciar.

Sea cual sea la cuota, una tarea en segundo plano se pausa mientras alguien
está viendo algo, y se reanuda después. Por eso, en una biblioteca nueva,
las miniaturas y las marcas de intro llegan a lo largo de horas y no de
minutos si el servidor está en uso.

## Lo que se ejecuta en segundo plano

| Trabajo | Lo que aporta | Cuándo se ejecuta |
| --- | --- | --- |
| Escaneo de bibliotecas | Archivos nuevos, modificados y eliminados | Cuando cambian los archivos, según la programación, y al arrancar si está activado. Ver [Bibliotecas y metadatos](../administration/libraries.md). |
| Descarga de imágenes | Pósteres e imágenes de fondo guardados en el servidor, en WebP | Durante el escaneo; las que faltan se vuelven a intentar cada hora. Desactivada: el navegador carga las imágenes desde internet, algo que algunos navegadores bloquean. |
| Miniaturas de la barra de progreso | Las vistas previas que aparecen al arrastrar la barra de progreso (una imagen cada 7 segundos) | Cuando se añade un título, y al arrancar para los que no las tienen. **Activar miniaturas trickplay** en **Configuración del servidor** las desactiva. |
| Detección de intros y créditos | Los botones **Saltar** | Continuamente, para los títulos que no cubren las bases de datos comunitarias |
| Índice de fotogramas clave y niveles de códec | Reproducción directa y copia sin volver a codificar, colores correctos | Continuamente, en los archivos escaneados antes de que se recogiera esa información |

## Memoria

**Perfil de memoria** (**Administración → Configuración del servidor**):
**Bajo** suspende los plugins que no están haciendo nada para ahorrar
memoria, y los reactiva cuando hacen falta. **Automático (recomendado)** elige
Bajo en las máquinas con menos de 3 GB de memoria.

## Ajustes que se leen al arrancar

Algunos ajustes se leen una sola vez, cuando arranca el servidor: al
guardarlos se guarda tu elección, y un reinicio la aplica. El panel lo
indica junto a ellos. Son la tarjeta **Trabajos en segundo plano** (pausa y
ritmo de las pasadas de fotogramas clave y de niveles de códec), el
interruptor **Proveedores de salto de intro**, y los **Límites avanzados**
de la transcodificación.

<!-- sources
processing power, heavy tasks, pause during playback: i18n admin.serverConfig.cpuBudget.* (globalLabel, tasksTitle, tasksDesc, applyHint) ; admin.activity.bgTaskHelp.*
thumbnails switch: i18n admin.serverConfig.trickplayEnabled
artwork: i18n admin.serverConfig.artworkInfo, artworkRetryDesc
thumbnails: i18n admin.activity.bgTaskHelp.trickplay_generation
memory: i18n admin.serverConfig.memoryProfileDesc
read at startup: i18n admin.serverConfig.backgroundJobs.restartWarning, introProvidersRestart ; docs/playback/transcoding.md
-->
