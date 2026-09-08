---
source_sha: ac74ba14732aabf186129184b065fedaeb40d996e987374e385a8edc9d0e5c84
---

# Explotación

Qué hacer con un servidor que ya está en marcha: el día que se porta mal, el día
que lo traslada y el día que necesita recuperarlo.

Estas páginas dan por hecho que tiene una terminal y una cuenta de
administrador. Casi todo lo que sigue está también en el panel de
administración; el intérprete de órdenes es el recurso para cuando lo que ha
dejado de funcionar es el panel.

| Qué necesita | Lea |
| --- | --- |
| Ejecutar una orden de mantenimiento en el servidor | [Línea de órdenes](cli.md) |
| Averiguar por qué un archivo no se deja posicionar, o suena desincronizado | [Salud de los archivos](media-health.md) |
| Levantar un servidor que se niega a arrancar | [Recuperación](recovery.md) |
| Planificar o deshacer una migración de la base de datos | [Migraciones](migration-runbook.md) |
| Salir de una incidencia en curso | [Incidencias](incident-runbook.md) |

## Antes de la incidencia, no durante

Dos cosas conviene comprobarlas mientras no hay nada ardiendo, porque
descubrirlas bajo presión no sirve de nada: que las
[copias de seguridad](../backups.md) se están escribiendo de verdad donde usted
cree, y que sabe restaurar una. Una copia que nadie ha restaurado es una
hipótesis.
