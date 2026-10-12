---
source_sha: 25ed5f4a742c5f5e2457ec15cbc7d5d59bfd30c52c88a5fc2548a86553d13fb5
reviewed: true
---
# Plugins et sandbox

Les variables qui règlent le confinement et les limites des plugins. Les
valeurs par défaut conviennent à une machine Docker standard ; quelle
configuration pour quelle machine est expliqué dans
[Sandbox des plugins : quel réglage pour votre serveur](../plugins/sandbox-deployment.md),
et le fonctionnement des couches de protection dans
[Sandbox des plugins et modèle de menace](../SECURITY.md).

## Sandbox

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_PLUGINS_ENABLED` | `true` | Le sous-système des plugins. `false` le désactive entièrement. | aucun |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto`, `landlock`, `bubblewrap`, `strict` ou `basic`. `auto` choisit le plus robuste que permet la machine ; un `landlock` explicite refuse un plugin qu'il ne peut pas confiner. | Plugins → Limites de ressources → **Mode sandbox**, pour les plugins lancés ensuite |
| `EVEY_PLUGINS_UID_SEPARATION` | `auto` | `auto`, `on` ou `off` : fait tourner les plugins sous un utilisateur séparé. `auto` ne le fait que lorsque la sandbox se résout en `basic`. Incompatible avec `no_new_privs` sur le conteneur. | aucun |

## Limites de ressources

Ce sont les valeurs de départ ; celles enregistrées dans **Administration → Plugins →
Limites de ressources** s'appliquent à tous les plugins et l'emportent.

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_PLUGINS_MEMORY_MB` | `256` | Mémoire par plugin, en Mio. | Plugins → Limites de ressources → **Mémoire max**, immédiat |
| `EVEY_PLUGINS_PIDS_MAX` | `64` | Processus et threads par plugin. | Plugins → Limites de ressources → **Processus max**, immédiat |
| `EVEY_PLUGINS_DISK_MAX_MB` | `100` | Taille du dossier d'un plugin, en Mio ; `0` = sans limite. | Plugins → Limites de ressources → **Disque max**, immédiat |
| `EVEY_PLUGINS_CPU_NICE` | `10` | Priorité CPU, de 0 à 19 ; plus la valeur est haute, plus la priorité est basse. | Plugins → Limites de ressources → **Nice CPU**, pour les plugins lancés ensuite |

## Signatures

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_PLUGINS_COSIGN_POLICY` | `warn` | `off`, `warn` ou `enforce` : l'effet d'une vérification de signature qui échoue. `enforce` sans clé refuse de démarrer. | aucun |
| `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` | vide | Chemin de la clé publique PEM avec laquelle les signatures sont vérifiées. Avec `warn` et sans clé, rien n'est vérifié et le serveur l'indique au démarrage. | aucun |

## Logs

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_LOG_PLUGIN_RETENTION_DAYS` | `10` | Nombre de jours pendant lesquels le log propre d'un plugin est conservé. | aucun |
