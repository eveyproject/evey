---
source_sha: a10d4c679e7aed09a2fa56ed708a6beb299e8c731fcea4016e7cbf7b9f1e30e5
reviewed: false
---
# Tareas en segundo plano y carga del servidor

Además de la reproducción, el servidor ejecuta trabajo propio: escanear
bibliotecas, descargar imágenes, generar las miniaturas de la barra de
progreso, detectar intros y créditos, analizar archivos (fotogramas clave,
niveles de códec, salud de medios). Esta página explica cómo reparte ese trabajo
la máquina con las personas que están viendo algo.

## La reproducción va primero

Las tareas en segundo plano usan una parte del procesador, un **25 %** por
defecto (**Admin → Configuración del servidor → Potencia asignada**). Un **50 %**
las acelera, para cuando nadie usa el servidor; a una tarea pesada (análisis de
medios, miniaturas, detección de intros, clasificación) se le puede dar una parte
propia. Un cambio se aplica al siguiente elemento procesado.

Sea cual sea la parte, una tarea en segundo plano se pausa mientras alguien ve
algo y se reanuda después. En una biblioteca nueva, las miniaturas y los
marcadores de intro llegan por tanto en horas y no en minutos si el servidor
está en uso.

## Qué se ejecuta en segundo plano

| Trabajo | Lo que aporta | Cuándo se ejecuta |
| --- | --- | --- |
| Escaneo de bibliotecas | Archivos nuevos, modificados y eliminados | Cuando cambian los archivos, según la programación, al arrancar si está activado. Consulta [Bibliotecas y metadatos](../administration/libraries.md). |
| Descarga de imágenes | Pósteres y fondos guardados en el servidor, en WebP | Durante el escaneo; los que faltan se reintentan cada hora. Desactivada: el navegador carga las imágenes desde internet, algo que algunos navegadores bloquean. |
| Miniaturas de la barra de progreso | Las vistas previas que se muestran al arrastrar la barra de progreso (una imagen cada 7 segundos) | Cuando se añade un título, y al arrancar para los que no las tienen. Se puede desactivar en **Configuración del servidor**. |
| Detección de intros y créditos | Los botones **Saltar intro** y **Saltar créditos** | De forma continua, para los títulos que no cubren las bases de datos comunitarias |
| Índice de fotogramas clave y niveles de códec | Reproducción directa y copia sin recodificar, colores correctos | De forma continua, en los archivos escaneados antes de que se recogiera la información |

## Memoria

**Perfil de memoria** (**Admin → Configuración del servidor**): **Bajo** suspende
los plugins inactivos para ahorrar memoria y los despierta cuando se necesitan.
**Auto** elige Bajo en máquinas con menos de 3 GB de memoria.

## Ajustes leídos al arrancar

Algunos ajustes se leen una sola vez, al arrancar el servidor: guardarlos
almacena tu elección y un reinicio la aplica. El panel lo indica junto a ellos.
Son la tarjeta **Trabajos en segundo plano** (pausa y ritmo de las pasadas de
fotogramas clave y niveles de códec), el interruptor **Proveedores de salto de
intro** y los **Límites avanzados** de la transcodificación.
