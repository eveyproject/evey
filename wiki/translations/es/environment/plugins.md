---
source_sha: ca793f290f46505f8ca71b8dd23267c3488a2611b736e01fe78edbb4254c840d
reviewed: false
---
# Plugins y sandbox

Cómo se confinan y limitan los plugins. Cómo funcionan las capas:
[Modelo de seguridad y sandbox de plugins](../SECURITY.md).

## Sandbox

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_PLUGINS_ENABLED` | `true` | El subsistema de plugins. `false` lo desactiva por completo. |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto`, `landlock`, `bubblewrap`, `strict` o `basic`. `auto` elige el más fuerte que admite el equipo; un `landlock` explícito rechaza un plugin que no puede confinar. Editable en **Admin → Plugins**. |
| `EVEY_PLUGINS_UID_SEPARATION` | `auto` | `auto`, `on` u `off`: ejecuta los plugins con un usuario aparte. `auto` lo hace solo cuando el sandbox resuelve a `basic`. Incompatible con `no_new_privs` en el contenedor. |

Qué configuración conviene a cada equipo: [Configurar el sandbox de plugins](../plugins/sandbox-deployment.md).

## Límites de recursos

Valores iniciales; los límites de cada plugin se editan después en **Admin → Plugins**.

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_PLUGINS_MEMORY_MB` | `256` | Memoria por plugin, en MiB. |
| `EVEY_PLUGINS_PIDS_MAX` | `64` | Procesos e hilos por plugin. |
| `EVEY_PLUGINS_DISK_MAX_MB` | `100` | Tamaño del directorio de un plugin, en MiB; `0` = sin límite. |
| `EVEY_PLUGINS_CPU_NICE` | `10` | Prioridad de CPU, de 0 a 19; cuanto más alto, menos prioridad. |

## Firmas

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_PLUGINS_COSIGN_POLICY` | `warn` | `off`, `warn` o `enforce`: qué hace una comprobación de firma fallida. |
| `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` | vacía | Ruta de la clave pública PEM con la que se comprueban las firmas. Con una política distinta de `off` y sin clave, no se verifica nada y el servidor avisa al arrancar. |

## Registros

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_LOG_PLUGIN_RETENTION_DAYS` | `10` | Días que se conserva el registro propio de un plugin. |
