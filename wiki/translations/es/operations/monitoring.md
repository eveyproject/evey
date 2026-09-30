---
source_sha: f6d6e61c9914bed7f2744ebf8dec6a79fc1769dd750e8b0f2109b4ab14d44526
reviewed: false
---
# Supervisión y registros

Dónde ver lo que hace el servidor, de lo más inmediato a lo más agregado. Para
investigar un síntoma, empieza por
[Solución de problemas](troubleshooting.md).

## Lo que se está reproduciendo ahora

**Admin → Reproduciendo ahora** lista cada flujo en curso: quién, qué, en qué
dispositivo y cómo, pista por pista (copiado, transcodificado, y el backend que
lo codifica). Un administrador puede **Detener** una sesión o **Enviar mensaje**
al espectador.

**Admin → Estado del sistema** muestra el procesador, la memoria, el ancho de
banda y el almacenamiento, y qué parte de la capacidad de transcodificación está
en uso.

<figure class="evey-shot" markdown="span">
  ![Admin → Estado del sistema en el servidor de demostración](../images/admin-system-health.webp){ loading=lazy }
  <figcaption><strong>Admin → Estado del sistema</strong>, con las tareas en segundo plano debajo (interfaz en inglés).</figcaption>
</figure>

## Registros

**Admin → Logs y depuración** busca en los registros del servidor y los sigue en
directo, filtrados por área (reproducción, transcodificación, escaneo,
subtítulos...). Es el primer sitio que mirar cuando un usuario informa de un
problema. Los mismos registros están disponibles con `docker compose logs evey`.

## Cómo fue realmente la reproducción

**Admin → Calidad de streaming** se construye a partir de lo que los
reproductores informan mientras reproducen (nivel del búfer, recargas, caudal, en
formato CMCD): con qué frecuencia esperaron los espectadores, los títulos que más
se atascan y los tipos de dispositivo que tienen dificultades, durante el último
día o la última semana.

El servidor también actúa en consecuencia. Cuando un tipo de fuente sigue
haciendo esperar a los espectadores, baja la calidad máxima para ese tipo de
fuente durante un tiempo, además del techo calibrado, y luego vuelve a levantar
el límite por sí solo. Los límites activos se listan en **Límites de calidad
automáticos**, con el motivo y la caducidad; **Restablecer** quita uno de
inmediato.

## Historial y estadísticas

- **Admin → Analítica**: reproducciones de hoy y de esta semana, usuarios
  activos, los títulos más vistos de los últimos 30 días.
- **Admin → Actividad**: el historial de reproducciones y de acciones de
  administración, y las tareas en segundo plano en curso.

Cuánto tiempo se conserva cada tipo de historial se fija en **Admin →
Configuración del servidor → Configuración del servidor** (sesiones de
reproducción, registro de auditoría, métricas del servidor, calidad de
streaming). Un cambio se aplica en la siguiente limpieza diaria.

## Línea de comandos

`evey admin doctor` comprueba todo el servidor en un solo informe; los demás
comandos de diagnóstico se listan en [Línea de comandos](../runbooks/cli.md).
