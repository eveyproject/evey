---
source_sha: ca793f290f46505f8ca71b8dd23267c3488a2611b736e01fe78edbb4254c840d
reviewed: false
---
# Plugins und Sandbox

Wie Plugins eingesperrt und begrenzt werden. Wie die Schichten funktionieren:
[Sicherheitsmodell und Plugin-Sandbox](../SECURITY.md).

## Sandbox

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_PLUGINS_ENABLED` | `true` | Das Plugin-Subsystem. `false` schaltet es ganz ab. |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto`, `landlock`, `bubblewrap`, `strict` oder `basic`. `auto` wählt das stärkste, das der Host unterstützt; ein ausdrückliches `landlock` verweigert ein Plugin, das es nicht einschränken kann. Unter **Admin → Plugins** änderbar. |
| `EVEY_PLUGINS_UID_SEPARATION` | `auto` | `auto`, `on` oder `off`: führt Plugins unter einem eigenen Benutzer aus. `auto` tut es nur, wenn die Sandbox zu `basic` aufgelöst wird. Nicht vereinbar mit `no_new_privs` am Container. |

Welche Einrichtung zu welchem Host passt: [Plugin-Sandbox einrichten](../plugins/sandbox-deployment.md).

## Ressourcengrenzen

Startwerte; die Grenzen jedes Plugins werden danach unter **Admin → Plugins**
bearbeitet.

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_PLUGINS_MEMORY_MB` | `256` | Speicher pro Plugin, in MiB. |
| `EVEY_PLUGINS_PIDS_MAX` | `64` | Prozesse und Threads pro Plugin. |
| `EVEY_PLUGINS_DISK_MAX_MB` | `100` | Größe des Verzeichnisses eines Plugins, in MiB; `0` = keine Grenze. |
| `EVEY_PLUGINS_CPU_NICE` | `10` | CPU-Priorität, 0 bis 19; höher heißt niedrigere Priorität. |

## Signaturen

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_PLUGINS_COSIGN_POLICY` | `warn` | `off`, `warn` oder `enforce`: was eine fehlgeschlagene Signaturprüfung bewirkt. |
| `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` | leer | Pfad zum öffentlichen PEM-Schlüssel, gegen den Signaturen geprüft werden. Mit einer anderen Richtlinie als `off` und ohne Schlüssel wird nichts geprüft, und der Server warnt beim Start. |

## Logs

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_LOG_PLUGIN_RETENTION_DAYS` | `10` | Tage, die das eigene Log eines Plugins aufbewahrt wird. |
