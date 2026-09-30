---
source_sha: ca793f290f46505f8ca71b8dd23267c3488a2611b736e01fe78edbb4254c840d
reviewed: true
---
# Plugins et sandbox

Le confinement et les limites des plugins. Le fonctionnement des couches de
protection : [Modèle de sécurité et sandbox des plugins](../SECURITY.md).

## Sandbox

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_PLUGINS_ENABLED` | `true` | Le sous-système des plugins. `false` le désactive entièrement. |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto`, `landlock`, `bubblewrap`, `strict` ou `basic`. `auto` choisit le plus robuste que permet la machine ; un `landlock` explicite refuse un plugin qu'il ne peut pas confiner. Modifiable dans **Admin → Plugins**. |
| `EVEY_PLUGINS_UID_SEPARATION` | `auto` | `auto`, `on` ou `off` : fait tourner les plugins sous un utilisateur séparé. `auto` ne le fait que lorsque la sandbox se résout en `basic`. Incompatible avec `no_new_privs` sur le conteneur. |

Quelle configuration pour quelle machine : [Mise en place de la sandbox des plugins](../plugins/sandbox-deployment.md).

## Limites de ressources

Valeurs de départ ; les limites de chaque plugin se modifient ensuite dans
**Admin → Plugins**.

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_PLUGINS_MEMORY_MB` | `256` | Mémoire par plugin, en Mio. |
| `EVEY_PLUGINS_PIDS_MAX` | `64` | Processus et threads par plugin. |
| `EVEY_PLUGINS_DISK_MAX_MB` | `100` | Taille du dossier d'un plugin, en Mio ; `0` = sans limite. |
| `EVEY_PLUGINS_CPU_NICE` | `10` | Priorité CPU, de 0 à 19 ; plus la valeur est haute, plus la priorité est basse. |

## Signatures

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_PLUGINS_COSIGN_POLICY` | `warn` | `off`, `warn` ou `enforce` : l'effet d'une vérification de signature qui échoue. |
| `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` | vide | Chemin de la clé publique PEM avec laquelle les signatures sont vérifiées. Avec une politique autre que `off` et sans clé, rien n'est vérifié et le serveur l'indique au démarrage. |

## Logs

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_LOG_PLUGIN_RETENTION_DAYS` | `10` | Nombre de jours pendant lesquels le log propre d'un plugin est conservé. |
