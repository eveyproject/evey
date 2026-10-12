---
source_sha: 25ed5f4a742c5f5e2457ec15cbc7d5d59bfd30c52c88a5fc2548a86553d13fb5
reviewed: true
---
# Plugins y sandbox

Las variables que deciden cómo se confinan y se limitan los plugins. Los
valores por defecto sirven para un equipo Docker normal; qué configuración
conviene a cada equipo está en
[Configurar el sandbox de plugins](../plugins/sandbox-deployment.md), y cómo
funcionan las capas, en
[Sandbox de plugins y modelo de amenazas](../SECURITY.md).

## Sandbox

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_PLUGINS_ENABLED` | `true` | El sistema de plugins. `false` lo desactiva por completo. | ninguno |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto`, `landlock`, `bubblewrap`, `strict` o `basic`. `auto` elige el más fuerte que admite el equipo; un `landlock` explícito rechaza un plugin que no puede confinar. | Plugins → Límites de recursos → **Modo de sandbox**, se aplica a los plugins que arranquen después |
| `EVEY_PLUGINS_UID_SEPARATION` | `auto` | `auto`, `on` u `off`: ejecuta los plugins con un usuario aparte. `auto` lo hace solo cuando el sandbox queda en `basic`. Incompatible con `no_new_privs` en el contenedor. | ninguno |

## Límites de recursos

Estos son los valores iniciales; los que se guardan en **Administración →
Plugins → Límites de recursos** se aplican a todos los plugins y tienen
prioridad sobre ellos.

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_PLUGINS_MEMORY_MB` | `256` | Memoria por plugin, en MiB. | Plugins → Límites de recursos → **Memoria máx.**, inmediato |
| `EVEY_PLUGINS_PIDS_MAX` | `64` | Procesos e hilos por plugin. | Plugins → Límites de recursos → **Procesos máx.**, inmediato |
| `EVEY_PLUGINS_DISK_MAX_MB` | `100` | Tamaño de la carpeta de un plugin, en MiB; `0` = sin límite. | Plugins → Límites de recursos → **Disco máx.**, inmediato |
| `EVEY_PLUGINS_CPU_NICE` | `10` | Prioridad de CPU, de 0 a 19; cuanto más alto, menos prioridad. | Plugins → Límites de recursos → **Nice de CPU**, se aplica a los plugins que arranquen después |

## Firmas

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_PLUGINS_COSIGN_POLICY` | `warn` | `off`, `warn` o `enforce`: qué pasa cuando falla la comprobación de una firma. `enforce` sin clave impide el arranque. | ninguno |
| `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` | vacío | Ruta de la clave pública PEM con la que se comprueban las firmas. Con `warn` y sin clave, no se verifica nada y el servidor avisa al arrancar. | ninguno |

## Registros

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_LOG_PLUGIN_RETENTION_DAYS` | `10` | Días que se conserva el registro propio de un plugin. | ninguno |

<!-- sources
internal/config/config.go - EVEY_PLUGINS_*, EVEY_LOG_PLUGIN_RETENTION_DAYS ; WarnValidate (enum values, cosign enforce without key, cosign warn without key)
panel: internal/app/plugin/resource_config.go (plugins.memory_mb, pids_max, disk_max_mb, cpu_nice, sandbox_mode ; live on next sweep / next start) ; i18n admin.plugins.resourceLimits.*
docs/SECURITY.md
-->
