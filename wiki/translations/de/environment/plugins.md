---
source_sha: c99cc1b66de151e0e27b786bbc03a5bbf4ecf4e6c64681fc28cf588a10e0cdfa
reviewed: true
---
# Plugins und Sandbox

Die Variablen dafür, wie Plugins isoliert und begrenzt werden. Die
Standardwerte passen zu einem normalen Docker-Host. Welche Einrichtung zu
welchem Host passt, steht unter
[Plugin-Sandbox einrichten](../plugins/sandbox-deployment.md), und wie die
Schichten funktionieren, unter
[Plugin-Sandbox und Bedrohungsmodell](../SECURITY.md).

## Sandbox

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_PLUGINS_ENABLED` | `true` | Das Plugin-System. `false` schaltet es ganz ab. | keine |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto`, `landlock`, `bubblewrap`, `strict` oder `basic`. `auto` wählt die stärkste Stufe, die der Host unterstützt; ein ausdrücklich gesetztes `landlock` lehnt ein Plugin ab, das es nicht isolieren kann. | Plugins → Ressourcenlimits → **Sandbox-Modus**, gilt für danach gestartete Plugins |
| `EVEY_PLUGINS_UID_SEPARATION` | `auto` | `auto`, `on` oder `off`: startet Plugins unter einem eigenen Benutzer. `auto` tut das nur, wenn die Sandbox zu `basic` aufgelöst wird. Nicht vereinbar mit `no_new_privs` am Container. | keine |

## Ressourcenlimits

Das sind die Startwerte. Die unter **Verwaltung → Plugins →
Ressourcenlimits** gespeicherten Werte gelten für jedes Plugin und haben
Vorrang vor ihnen.

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_PLUGINS_MEMORY_MB` | `256` | Arbeitsspeicher pro Plugin, in MiB. | Plugins → Ressourcenlimits → **Max. Arbeitsspeicher**, sofort |
| `EVEY_PLUGINS_PIDS_MAX` | `64` | Prozesse und Threads pro Plugin. | Plugins → Ressourcenlimits → **Max. Prozesse**, sofort |
| `EVEY_PLUGINS_DISK_MAX_MB` | `100` | Größe des Verzeichnisses eines Plugins, in MiB; `0` = keine Grenze. | Plugins → Ressourcenlimits → **Max. Festplatte**, sofort |
| `EVEY_PLUGINS_CPU_NICE` | `10` | CPU-Priorität, 0 bis 19; ein höherer Wert bedeutet eine niedrigere Priorität. | Plugins → Ressourcenlimits → **CPU-Nice**, gilt für danach gestartete Plugins |

## Signaturen

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_PLUGINS_COSIGN_POLICY` | `warn` | `off`, `warn` oder `enforce`: was eine fehlgeschlagene Signaturprüfung bewirkt. Signaturen werden gegen den Schlüssel geprüft, den jedes Paket mitbringt, und gegen die Liste vertrauenswürdiger Herausgeber; ein Schlüssel ist also nicht nötig. | keine |
| `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` | leer | Optionaler Pfad zum öffentlichen PEM-Schlüssel Ihres eigenen Herausgebers, der bei jedem Start zu den vertrauenswürdigen Herausgebern hinzugefügt wird. | keine |

## Protokolle

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_LOG_PLUGIN_RETENTION_DAYS` | `10` | Tage, die das eigene Protokoll eines Plugins aufbewahrt wird. | keine |

<!-- sources
internal/config/config.go - EVEY_PLUGINS_*, EVEY_LOG_PLUGIN_RETENTION_DAYS ; WarnValidate (enum values, cosign enforce without key, cosign warn without key)
panel: internal/app/plugin/resource_config.go (plugins.memory_mb, pids_max, disk_max_mb, cpu_nice, sandbox_mode ; live on next sweep / next start) ; i18n admin.plugins.resourceLimits.*
docs/SECURITY.md
-->
