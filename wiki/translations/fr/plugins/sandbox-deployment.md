---
source_sha: a6f8003c3c1502fd56c9686aaf41877fd1dc643a86c7f3eb3cb6910f60615fd6
reviewed: true
---
# Sandbox des plugins : quel réglage pour votre serveur

Evey exécute les plugins tiers dans une sandbox. **Pour presque tout le monde, la
réponse est : gardez les réglages par défaut.** Le serveur détecte votre machine
et choisit tout seul l'isolation la plus forte possible. Ne lisez la suite que si
vous avez un **vieux NAS**, si vous faites tourner Evey **sans Docker**, ou si
vous voulez **resserrer encore la vis**.

Le modèle technique complet est dans [SECURITY.md](../SECURITY.md). Cette page
est le guide de décision en langage simple.

---

## Ce qui vous protège, en clair

Un plugin que vous installez est enfermé dans une boîte. Selon votre machine,
cette boîte :

- **limite les fichiers qu'il peut toucher** : il voit son propre dossier et les
  bibliothèques système dont il a besoin pour tourner, rien d'autre (ni vos
  médias, ni vos secrets, ni les autres plugins) ;
- **bloque les actions système dangereuses** : déboguer d'autres processus,
  charger des modules noyau, changer l'heure, monter des disques, etc. sont
  refusés par le noyau ;
- **coupe l'accès direct au réseau**, toujours. Un plugin qui a demandé le
  réseau (et que vous avez approuvé) ne joint que les hôtes qu'il a déclarés,
  par l'intermédiaire du serveur ;
- **plafonne le processeur, la mémoire, le disque et le nombre de processus**,
  pour qu'un plugin emballé ne fasse pas tomber le serveur ;
- **garde vos secrets illisibles** : la clé qui signe les connexions, et les
  sauvegardes.

En plus de cela : seul un administrateur peut **installer** un plugin, les plugins
sont **analysés** à la recherche de code suspect, et vous **approuvez leurs
permissions** avant qu'ils ne tournent.

**Vérifiez ce qui est réellement en place :** Admin → Plugins → Tableau de bord
sécurité montre la *Sandbox de l'hôte*, c'est-à-dire le mode que `auto` a retenu
(`landlock`, `bubblewrap` ou `basic`), avec le résultat de chaque sonde. S'il
affiche **basic**, votre machine n'offre pas d'isolation du système de fichiers
par chemin (les plugins tournent seulement avec un changement d'UID et seccomp) ;
les sondes vous disent pourquoi, et les sections ci-dessous comment y remédier.

---

## Choisissez votre cas

| Votre serveur | Ce que vous obtenez d'office | Quelque chose à faire ? |
|---|---|---|
| **Linux récent dans Docker** (noyau ≥ 5.13, en gros tout ce qui date de 2021 environ ou après) : *le cas normal* | **L'isolation la plus forte** (Landlock), automatiquement. | **Rien.** Gardez les réglages par défaut. |
| **Vieux NAS dans Docker** (par exemple un ancien Synology, noyau < 5.13) | Une bonne isolation, **sans les cloisons de fichiers par dossier**. Les secrets, le réseau et les actions système restent verrouillés. | **Rien.** La séparation d'UID (plus bas) s'active d'elle-même dans ce cas. |
| **Tout Linux, sans Docker** (installation directe) | **L'isolation la plus forte**, automatiquement (Landlock sur un noyau récent, sinon les espaces de noms de bubblewrap). | Suivez le [guide d'installation sans Docker](../deployment-bare-metal.md). |

Pour savoir dans quel cas vous êtes : ouvrez **Admin → Plugins** ; le badge du
panneau de sécurité indique le mode de sandbox réellement en place : `landlock`
ou `bubblewrap` signifie une isolation forte, `basic` correspond au cas du « vieux
NAS ».

---

## Le seul réglage que vous pourriez toucher : la séparation d'UID

**Ce qu'elle fait, simplement :** elle fait tourner un plugin sous un **autre
compte utilisateur** qu'Evey lui-même, si bien qu'il ne peut matériellement pas
lire les fichiers secrets d'Evey, même si rien d'autre ne l'en empêchait.

**Quand elle compte :** seulement sur une **vieille machine sans cloisons de
fichiers par dossier** (le cas `basic`). Sur une machine récente, les cloisons
tiennent déjà les secrets hors de portée : la séparation d'UID n'y est qu'une
couche supplémentaire redondante.

**Le réglage** : `EVEY_PLUGINS_UID_SEPARATION` :

| Valeur | Signification | Quand l'utiliser |
|---|---|---|
| `auto` *(par défaut)* | S'active **seulement quand c'est vraiment utile** (le cas `basic`). Désactivée là où elle serait redondante. | **Gardez-la** : elle fait ce qu'il faut selon la machine. |
| `on` | **Toujours active**, même sur une machine récente : une seconde cloison, indépendante, pour les secrets, en plus des cloisons de fichiers. | Vous voulez le maximum de précautions et acceptez le petit coût décrit plus bas. |
| `off` | **Jamais.** | Un plugin précis se comporte mal sous elle (rare). |

**Le piège :** la séparation d'UID est **incompatible avec l'option de
durcissement Docker `no-new-privileges`** : il faut choisir. Si votre
installation ne peut pas l'accorder sur une vieille machine (`basic`), Evey
**refuse de démarrer le plugin** plutôt que de le faire tourner à côté de vos
secrets ; sur une machine récente, il journalise un avertissement et lance le
plugin normalement (vous n'avez simplement pas la cloison supplémentaire).
L'image officielle contient déjà tout le nécessaire ; vous n'avez qu'à régler
cette variable.

**Le petit coût de `on` :** le plugin tourne sous un compte distinct, si bien
qu'un plugin qui écrit des fichiers temporaires *dans son propre dossier* peut
trébucher. Si cela arrive, mettez `off`. (C'est pour cela que la valeur par
défaut est `auto` : elle ne prend ce risque que là où il apporte un vrai
bénéfice.)

---

## Les risques, en clair

Ce qu'un plugin **malveillant** pourrait faire ou non, selon le cas :

**Linux récent (Docker ou installation directe) :**
Un plugin malveillant est entièrement enfermé. Il **ne peut pas** lire vos
secrets, **ne peut pas** voir ni toucher les fichiers des autres plugins, **ne
peut pas** faire d'appels réseau arbitraires et **ne peut pas** trafiquer le
système. La seule issue réaliste serait une faille sérieuse du **noyau** Linux,
ce qui vaut pour *toute* sandbox qui n'est pas une machine virtuelle complète.

**Vieux NAS dans Docker, sans cloisons par dossier (`basic`) :**

- ✅ Vos **secrets restent protégés** (clé de connexion, sauvegardes) par le
  compte séparé, que `auto` active ici.
- ✅ Le plugin n'a **aucun accès direct au réseau** ; s'il y est autorisé, il ne
  joint que les hôtes qu'il a déclarés.
- ✅ Les **actions système dangereuses restent bloquées** par le noyau.
- ⚠️ Mais sans cloisons par dossier, un plugin malveillant **pourrait lire des
  fichiers ordinaires, non secrets**, **lister les processus en cours** et, comme
  les plugins partagent un même compte, peut-être **lire ou perturber les
  fichiers d'un autre plugin**.
- **En résumé :** sur une telle machine, n'installez que des plugins de
  confiance, ou passez à un noyau récent pour avoir les cloisons de fichiers
  complètes.

---

## Exemples de configuration (à copier)

Ils vont dans votre `.env` (Docker compose) ou en variables d'environnement
`EVEY_*`.

**1. Serveur récent normal, recommandé (ne rien faire) :**
```bash
# Les valeurs par défaut sont les bonnes. Pour les rendre explicites :
EVEY_PLUGINS_SANDBOX_MODE=auto          # choisit la plus forte disponible
# EVEY_PLUGINS_UID_SEPARATION=auto      # (par défaut) active seulement là où c'est utile
```

**2. Vieux NAS :** rien à régler, `auto` y fait déjà tourner les plugins sous
le compte séparé. Laissez `no-new-privileges` hors du `security_opt` de votre
compose, qui l'en empêcherait :
```bash
EVEY_PLUGINS_SANDBOX_MODE=auto
EVEY_PLUGINS_UID_SEPARATION=auto        # (par défaut) actif dans le cas basic
```

**3. Précautions maximales sur un serveur récent (les deux cloisons) :**
```bash
EVEY_PLUGINS_SANDBOX_MODE=auto          # → landlock
EVEY_PLUGINS_UID_SEPARATION=on          # ajoute la cloison d'UID indépendante
```

**4. Imposer un mode précis (avancé) :**
```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock      # refuse de démarrer les plugins si Landlock est indisponible (échec fermé)
```

**5. Un plugin ne fonctionne pas sous la séparation d'UID :**
```bash
EVEY_PLUGINS_UID_SEPARATION=off
```

**6. Plafonds de ressources (tous modifiables ensuite dans Admin → Plugins) :**
```bash
EVEY_PLUGINS_MEMORY_MB=256              # mémoire maximale par plugin
EVEY_PLUGINS_PIDS_MAX=64                # nombre maximal de processus et threads
EVEY_PLUGINS_DISK_MAX_MB=100            # taille maximale du dossier d'installation (0 = sans limite)
EVEY_PLUGINS_CPU_NICE=10                # priorité d'ordonnancement 0-19 (plus haut = plus discret)
```

---

## Changer le mode de sandbox en cours de route

Vous pouvez changer le mode depuis la carte **Limites de ressources** de
**Admin → Plugins**. Les plugins en cours d'exécution sont **redémarrés
automatiquement**, pour que le nouveau mode s'applique tout de suite (sans
désactiver puis réactiver à la main). Le badge de sécurité indique le mode
réellement en place. Si un plugin ne peut pas tourner sous le nouveau mode (par
exemple `bubblewrap` choisi sous Docker, où il demande des privilèges), il
apparaît **en erreur** dans le tableau de bord : revenez simplement en arrière.

---

## Vérifier que ça fonctionne

```bash
# le lanceur porte la capacité utilisée pour la séparation d'UID :
docker exec evey-server getcap /evey-plugin-launcher        # → cap_setuid=ep

# sous quel utilisateur tourne un plugin (evey = partagé avec le serveur, evey-plugin = séparé) :
docker exec evey-server ps -o user,pid,args | grep -v grep | grep -iE "node|python"

# le mode de sandbox réellement en place : le badge de sécurité de la page Admin → Plugins.
```

---

## Voir aussi

- [SECURITY.md](../SECURITY.md) : le modèle de sécurité complet des plugins
  (chaque couche, le tableau par mode, les limites connues).
- [deployment-bare-metal.md](../deployment-bare-metal.md) : faire tourner Evey sans
  Docker.
- [installation.md](../installation.md) : l'installation auto-hébergée avec Docker.
