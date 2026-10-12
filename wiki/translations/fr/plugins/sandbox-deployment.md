---
source_sha: 38effb73470754ed93d55d3b3cbf2f54bb84816288347233487a89e9c04593af
reviewed: true
---
# Sandbox des plugins

Quelle configuration de sandbox convient à votre serveur, en langage courant.
Pour presque tout le monde la réponse est : gardez les valeurs par défaut.
Le serveur détecte la machine et choisit l'isolation la plus forte qu'il
peut. Continuez seulement si vous avez un vieux NAS, si vous faites tourner
Evey sans Docker, ou si vous voulez renforcer davantage l'isolation. Le modèle technique
complet est dans [Modèle de sécurité et sandbox des plugins](../SECURITY.md).

## Ce qui vous protège

Un plugin que vous installez tourne isolé. Selon la machine, cette isolation :

- **limite les fichiers qu'il peut toucher** : son propre dossier et les
  bibliothèques système dont il a besoin, rien d'autre (ni vos médias, ni vos
  secrets, ni les autres plugins) ;
- **bloque les actions système dangereuses** au niveau du noyau : déboguer
  d'autres processus, charger des modules, changer l'horloge, monter des
  disques ;
- **coupe l'accès réseau direct**, toujours : un plugin qui a demandé le
  réseau, et que vous avez approuvé, n'atteint que les hôtes qu'il a
  déclarés, à travers le serveur ;
- **plafonne le processeur, la mémoire, le disque et le nombre de
  processus**, pour qu'un plugin qui consomme trop ne bloque pas le serveur ;
- **garde vos secrets illisibles** : la clé de signature des connexions et
  les sauvegardes.

Par-dessus, seuls les administrateurs installent des plugins, ils sont
analysés à la recherche de code suspect, et vous approuvez leurs permissions
avant qu'ils ne tournent.

**Vérifiez ce qui est en vigueur :** **Administration → Plugins → Tableau de bord
sécurité** montre la sandbox de l'hôte, le mode que `auto` a résolu
(`landlock`, `bubblewrap` ou `basic`) avec le résultat des sondes (**Noyau
Landlock**, **Lanceur de la sandbox**, **Bubblewrap**). `basic` signifie que
l'hôte n'offre aucune isolation des fichiers par chemin : les plugins
tournent avec un abaissement d'uid et seccomp seulement, les sondes indiquent
pourquoi, et les sections ci-dessous indiquent quoi changer.

## Trouver votre situation

| Votre serveur | Ce que vous avez d'office | Quelque chose à faire ? |
|---|---|---|
| **Linux moderne sous Docker** (noyau 5.13 ou plus récent, à peu près tout ce qui date de 2021 ou après), le cas normal | l'isolation la plus forte (Landlock), automatiquement | rien |
| **Vieux NAS sous Docker** (noyau antérieur à 5.13) | une bonne isolation, sans la restriction d'accès aux fichiers par dossier ; secrets, réseau et actions système restent verrouillés | rien : la séparation d'uid (ci-dessous) s'active d'elle-même |
| **Linux quelconque, sans Docker** | l'isolation la plus forte disponible : Landlock sur un noyau moderne, sinon les espaces de noms bubblewrap | suivez [Sans conteneur](../deployment-bare-metal.md) |

## Le seul réglage que vous pourriez modifier : la séparation d'uid

La séparation d'uid fait tourner un plugin sous un autre compte utilisateur
que le serveur, pour qu'il ne puisse en aucun cas lire les fichiers
secrets du serveur même si rien d'autre ne l'arrêtait. Elle ne compte que
sur une machine sans la restriction d'accès aux fichiers par dossier (le cas
`basic`) ; sur une machine moderne cette restriction rend déjà les secrets
inaccessibles, et la séparation n'est qu'une couche redondante.

`EVEY_PLUGINS_UID_SEPARATION` :

| Valeur | Signification | À utiliser quand |
|---|---|---|
| `auto` (défaut) | active seulement quand c'est nécessaire (le cas `basic`), inactive là où c'est redondant | toujours, sauf si l'un des deux cas ci-dessous s'applique |
| `on` | toujours active, une seconde protection indépendante en plus de la restriction d'accès aux fichiers | vous voulez une double protection et acceptez le petit coût ci-dessous |
| `off` | jamais | un plugin précis se comporte mal sous elle (rare) |

!!! warning
    La séparation d'uid est incompatible avec l'option Docker
    `no-new-privileges` : choisissez l'une ou l'autre. Quand l'abaissement
    ne peut pas être accordé sur une vieille machine (`basic`), le serveur
    refuse de démarrer le plugin plutôt que de le faire tourner à côté de vos
    secrets ; sur une machine moderne il journalise un avertissement et lance
    le plugin sans la protection supplémentaire. L'image livre tout le
    nécessaire, vous ne réglez que la variable.

Le petit coût de `on` : le plugin tourne sous un compte séparé, donc un
plugin qui écrit des fichiers temporaires dans son propre dossier peut
échouer. Si c'est le cas, passez à `off`. C'est pourquoi le défaut est
`auto` : il ne prend ce risque que là où il y a un vrai bénéfice.

## Ce qu'un plugin malveillant pourrait faire

**Linux moderne (Docker ou sans conteneur) :** un plugin malveillant est
entièrement confiné. Il ne peut pas lire vos secrets, ne peut pas voir
ni toucher les fichiers des autres plugins, ne peut pas faire d'appels réseau
arbitraires et ne peut pas altérer le système. La seule issue réaliste est un
bug sérieux du noyau, ce qui vaut pour toute sandbox qui n'est pas une
machine virtuelle complète.

**Vieux NAS sous Docker, sans restriction d'accès par dossier (`basic`) :**

- vos secrets restent protégés (clé de connexion, sauvegardes) par le compte
  utilisateur séparé, que `auto` active ici ;
- le plugin n'a pas de réseau direct ; un plugin autorisé n'atteint que les
  hôtes qu'il a déclarés ;
- les actions système dangereuses restent bloquées au niveau du noyau ;
- mais sans restriction d'accès par dossier, un plugin malveillant pourrait lire des
  fichiers ordinaires non secrets, lister les processus en cours et, parce
  que les plugins partagent un compte, lire ou perturber les fichiers d'un
  autre plugin.

N'installez que des plugins de confiance sur un tel hôte, ou passez à un
noyau moderne pour avoir la restriction d'accès complète.

## Exemples de configuration

À mettre dans votre `.env` (compose) ou comme variables d'environnement
`EVEY_*`.

```bash title="1. Serveur moderne, recommandé (rien à faire)"
EVEY_PLUGINS_SANDBOX_MODE=auto          # choisit le plus fort disponible (défaut)
# EVEY_PLUGINS_UID_SEPARATION=auto      # (défaut) active seulement là où c'est nécessaire
```

```bash title="2. Vieux NAS (rien à régler, montré pour la clarté)"
# gardez no-new-privileges hors du security_opt du compose, il bloquerait l'abaissement
EVEY_PLUGINS_SANDBOX_MODE=auto
EVEY_PLUGINS_UID_SEPARATION=auto        # (défaut) active dans le cas basic
```

```bash title="3. Double protection sur un serveur moderne"
EVEY_PLUGINS_SANDBOX_MODE=auto          # se résout en landlock
EVEY_PLUGINS_UID_SEPARATION=on          # ajoute le rempart d'uid indépendant par-dessus
```

```bash title="4. Figer un mode (avancé)"
EVEY_PLUGINS_SANDBOX_MODE=landlock      # refuse de démarrer les plugins si Landlock est indisponible
```

```bash title="5. Un plugin ne fonctionne pas avec la séparation d'uid"
EVEY_PLUGINS_UID_SEPARATION=off
```

```bash title="6. Plafonds de ressources (modifiables ensuite depuis Admin -> Plugins -> Limites de ressources)"
EVEY_PLUGINS_MEMORY_MB=256              # RAM max par plugin
EVEY_PLUGINS_PIDS_MAX=64                # processus et threads max
EVEY_PLUGINS_DISK_MAX_MB=100            # taille max du dossier d'installation (0 = désactivé)
EVEY_PLUGINS_CPU_NICE=10                # priorité d'ordonnancement 0-19 (plus haut = plus poli)
```

## Changer le mode de sandbox à chaud

Changez **Mode sandbox** dans la carte **Limites de ressources** de **Administration
→ Plugins**. Les plugins en cours sont redémarrés automatiquement pour que
le nouveau mode prenne effet aussitôt, et le **Tableau de bord sécurité**
affiche le mode en vigueur. Un plugin qui ne peut pas tourner sous le nouveau
mode (par exemple `bubblewrap` dans un conteneur non privilégié) apparaît en
erreur : rétablissez le mode précédent.

## Vérifier le fonctionnement

```bash
# le launcher porte la capability utilisée pour la séparation d'uid
docker exec evey-server getcap /evey-plugin-launcher        # cap_setuid=ep

# sous quel utilisateur tourne un plugin (evey = partagé avec le serveur,
# evey-plugin = séparé) ; l'image n'a pas de ps, demandez à Docker
docker top evey-server -o user,pid,args | grep -iE "node|python"
```

Le mode de sandbox en vigueur est le badge de **Administration → Plugins → Tableau
de bord sécurité**.

<!-- sources
auto resolution and probes: internal/adapters/driven/pluginrunner/runner_landlock.go - resolveAutoMode ; runner_sandbox.go - effectiveMode, SandboxStatus, uidSepApplies, WithUIDSeparation, applyBasicSandbox
defaults: internal/config/config.go (EVEY_PLUGINS_SANDBOX_MODE, EVEY_PLUGINS_UID_SEPARATION, EVEY_PLUGINS_MEMORY_MB, _PIDS_MAX, _DISK_MAX_MB, _CPU_NICE)
restart on mode change: internal/app/plugin/runtime_service.go - RestartRunningPlugins
image: Dockerfile (setcap cap_setuid+ep /evey-plugin-launcher, no procps) ; docker-compose.yml (no security_opt)
labels: i18n/fr/admin.yaml - plugins.security.title, probeLandlock, probeLauncher, probeBubblewrap, plugins.resourceLimits.title, resourceLimits.sandboxMode
-->
