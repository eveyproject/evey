---
source_sha: 25ed5f4a742c5f5e2457ec15cbc7d5d59bfd30c52c88a5fc2548a86553d13fb5
reviewed: true
---
# Plugins und Sandbox

Die Variablen dafür, wie Plugins abgeschottet und begrenzt werden. Die
Standardwerte passen zu einem normalen Docker-Host. Welche Einrichtung zu
welchem Host passt, steht unter
[Plugin-Sandbox einrichten](../plugins/sandbox-deployment.md), und wie die
Schichten funktionieren, unter
[Plugin-Sandbox und Bedrohungsmodell](../SECURITY.md).

## Sandbox

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_PLUGINS_ENABLED` | `true` | Das Plugin-System. `false` schaltet es ganz ab. | keine |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto`, `landlock`, `bubblewrap`, `strict` oder `basic`. `auto` wählt die stärkste Stufe, die der Host unterstützt; ein ausdrücklich gesetztes `landlock` lehnt ein Plugin ab, das es nicht abschotten kann. | Plugins → Ressourcenlimits → **Sandbox-Modus**, gilt für danach gestartete Plugins |
| `EVEY_PLUGINS_UID_SEPARATION` | `auto` | `auto`, `on` oder `off`: startet Plugins unter einem eigenen Benutzer. `auto` tut das nur, wenn die Sandbox auf `basic` hinausläuft. Nicht vereinbar mit `no_new_privs` am Container. | keine |

## Ressourcenlimits

Das sind die Startwerte. Die unter **Verwaltung → Plugins →
Ressourcenlimits** gespeicherten Werte gelten für jedes Plugin und gewinnen
gegenüber ihnen.

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_PLUGINS_MEMORY_MB` | `256` | Arbeitsspeicher pro Plugin, in MiB. | Plugins → Ressourcenlimits → **Max. Arbeitsspeicher**, sofort |
| `EVEY_PLUGINS_PIDS_MAX` | `64` | Prozesse und Threads pro Plugin. | Plugins → Ressourcenlimits → **Max. Prozesse**, sofort |
| `EVEY_PLUGINS_DISK_MAX_MB` | `100` | Größe des Verzeichnisses eines Plugins, in MiB; `0` = keine Grenze. | Plugins → Ressourcenlimits → **Max. Festplatte**, sofort |
| `EVEY_PLUGINS_CPU_NICE` | `10` | CPU-Priorität, 0 bis 19; ein höherer Wert bedeutet eine niedrigere Priorität. | Plugins → Ressourcenlimits → **CPU-Nice**, gilt für danach gestartete Plugins |

## Signaturen

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_PLUGINS_COSIGN_POLICY` | `warn` | `off`, `warn` oder `enforce`: was eine fehlgeschlagene Signaturprüfung bewirkt. `enforce` ohne Schlüssel verhindert den Start. | keine |
| `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` | leer | Pfad zum öffentlichen PEM-Schlüssel, gegen den Signaturen geprüft werden. Mit `warn` und ohne Schlüssel wird nichts geprüft, und der Server warnt beim Start. | keine |

## Protokolle

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_LOG_PLUGIN_RETENTION_DAYS` | `10` | Tage, die das eigene Protokoll eines Plugins aufbewahrt wird. | keine |

<!-- sources
internal/config/config.go - EVEY_PLUGINS_*, EVEY_LOG_PLUGIN_RETENTION_DAYS ; WarnValidate (enum values, cosign enforce without key, cosign warn without key)
panel: internal/app/plugin/resource_config.go (plugins.memory_mb, pids_max, disk_max_mb, cpu_nice, sandbox_mode ; live on next sweep / next start) ; i18n admin.plugins.resourceLimits.*
docs/SECURITY.md
-->
