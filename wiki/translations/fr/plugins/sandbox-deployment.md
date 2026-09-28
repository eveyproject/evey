---
source_sha: 0846a653f87edb54addf042a0876b46fd3ab9e42930e3f12e27706e59ce0bc97
---

# Isolation des extensions : quelle configuration pour votre serveur

Evey exécute les extensions tierces dans un bac à sable. **Pour la quasi-totalité
des installations, la réponse est : ne touchez à rien.** Le serveur détecte votre
machine et retient tout seul l'isolation la plus forte qu'elle permet. Ne lisez
la suite que si vous avez un **vieux NAS**, si vous faites tourner Evey **sans
Docker**, ou si vous voulez **serrer davantage**.

Le modèle technique complet est dans [SECURITY.md](../SECURITY.md). Cette page
est le guide de décision en langage clair.

---

## Ce qui vous protège, en clair

Une extension que vous installez est enfermée. Selon votre machine, cet enclos :

- **Limite les fichiers qu'elle peut toucher** : elle voit son propre dossier et
  les bibliothèques système dont elle a besoin pour tourner, rien d'autre (ni vos
  médias, ni vos secrets, ni les autres extensions).
- **Bloque les actions système dangereuses** : déboguer d'autres processus,
  charger des modules noyau, changer l'horloge, monter des disques, etc. sont
  refusés par le noyau lui-même.
- **Coupe le réseau**, sauf si l'extension l'a explicitement demandé et que vous
  avez approuvé cette permission.
- **Plafonne processeur, mémoire, disque et nombre de processus**, pour qu'une
  extension emballée ne puisse pas mettre le serveur à terre.
- **Garde vos secrets illisibles** : la clé de signature des connexions et les
  sauvegardes.

À cela s'ajoute que l'installation d'une extension est **réservée à
l'administrateur**, que le code est **analysé** à la recherche de motifs
suspects, et que vous **approuvez ses permissions** avant qu'elle ne tourne.

**Pour voir ce qui s'applique réellement :** Admin → Plugins → Security Dashboard
affiche le *bac à sable de l'hôte*, c'est-à-dire ce à quoi le mode `auto` s'est
résolu (`landlock`, `bubblewrap` ou `basic`), avec le résultat de chaque sonde.
S'il affiche **basic**, votre hôte n'offre aucune isolation du système de
fichiers par chemin (les extensions tournent avec la seule bascule d'UID et
seccomp) ; les sondes disent pourquoi, et les sections ci-dessous disent comment
y remédier.

---

## Trouvez votre cas

| Votre serveur | Ce que vous obtenez d'emblée | Quelque chose à faire ? |
|---|---|---|
| **Linux récent sous Docker** (noyau ≥ 5.13, en gros tout ce qui date de 2021 ou après) - *le cas normal* | **L'isolation la plus forte** (Landlock), automatiquement. | **Rien.** Laissez les valeurs par défaut. |
| **Vieux NAS sous Docker** (Synology ancien par exemple, noyau < 5.13) | Bonne isolation, **sans les murs par dossier**. Les secrets, le réseau et les actions système restent verrouillés. | Facultatif : activer la **séparation d'uid** (voir plus bas) comme ceinture et bretelles pour les secrets. |
| **Linux quelconque, sans Docker** | **L'isolation la plus forte**, automatiquement (Landlock sur un noyau récent, sinon les espaces de noms de bubblewrap). | Suivez le [guide sans conteneur](../deployment-bare-metal.md). |

Pour savoir dans quel cas vous êtes : ouvrez **Admin → Plugins**, la pastille du
panneau Security indique le mode de bac à sable réellement en vigueur. `landlock`
ou `bubblewrap` signifient « fort », `basic` correspond au cas du vieux NAS.

---

## Le seul réglage que vous pourriez toucher : la séparation d'uid

**Ce qu'elle fait, simplement :** elle fait tourner une extension sous un **compte
utilisateur différent** de celui d'Evey, de sorte qu'elle ne peut physiquement
pas lire les fichiers secrets d'Evey, même si rien d'autre ne l'en empêchait.

**Quand elle compte :** uniquement sur une **vieille machine dépourvue des murs
par dossier**, c'est-à-dire le cas `basic`. Sur les machines récentes, les murs
par dossier tiennent déjà les secrets hors de portée, et la séparation d'uid n'y
est qu'une couche redondante de plus.

**Le réglage**, `EVEY_PLUGINS_UID_SEPARATION` :

| Valeur | Signification | À utiliser quand |
|---|---|---|
| `auto` *(par défaut)* | S'active **seulement là où c'est réellement utile** (le cas `basic`). Reste éteinte là où elle serait redondante. | **Laissez ça** : le bon comportement est choisi machine par machine. |
| `on` | **Toujours active**, même sur une machine récente : un second mur à secrets, indépendant, par-dessus les murs par dossier. | Vous voulez le maximum de ceinture et bretelles et vous acceptez le petit coût décrit plus bas. |
| `off` | **Jamais.** | Une extension précise se comporte mal sous ce régime (c'est rare). |

**Le piège :** la séparation d'uid est **incompatible avec l'option de durcissement
Docker `no-new-privileges`**, il faut choisir. Si votre installation ne peut pas
l'accorder sur une machine ancienne (`basic`), Evey **refuse de démarrer
l'extension** plutôt que de la faire tourner à côté de vos secrets ; sur une
machine récente, il enregistre un avertissement et exécute l'extension
normalement (vous n'avez simplement pas le mur supplémentaire).
L'image officielle embarque déjà tout le nécessaire ; vous n'avez que cette
variable à poser.

**Petit coût de `on` :** l'extension tourne sous un compte distinct, donc une
extension qui écrit des fichiers de travail *dans son propre dossier* pourrait
trébucher. Si cela arrive, passez à `off`. C'est bien pour cela que la valeur par
défaut est `auto` : elle ne prend ce risque que là où il y a un bénéfice réel.

---

## Les risques, en clair

Ce qu'une extension **malveillante** pourrait faire ou non, selon les cas :

**Linux récent (avec ou sans Docker) :**
Une extension malveillante est complètement enfermée. Elle **ne peut pas** lire
vos secrets, **ne peut pas** voir ni toucher les fichiers des autres extensions,
**ne peut pas** faire d'appels réseau arbitraires et **ne peut pas** trafiquer le
système. La seule sortie réaliste serait une faille sérieuse du **noyau** Linux,
ce qui vaut pour *tout* bac à sable qui n'est pas une machine virtuelle complète.

**Vieux NAS sous Docker, sans murs par dossier (`basic`) :**
- ✅ Vos **secrets restent protégés** (clé de connexion, sauvegardes), *surtout si
  vous passez la séparation d'uid à `on`*.
- ✅ L'extension n'a **aucun réseau** sans permission.
- ✅ Les **actions système dangereuses restent bloquées** par le noyau.
- ⚠️ Mais sans murs par dossier, une extension malveillante **pourrait lire des
  fichiers ordinaires non secrets**, **lister les processus en cours**, et,
  puisque les extensions partagent un même compte, éventuellement **lire ou
  perturber les fichiers d'une autre extension**.
- **En résumé :** sur un tel hôte, n'installez que des extensions de confiance,
  ou passez à un noyau récent pour bénéficier des murs par dossier.

---

## Exemples de configuration (à copier tels quels)

Ils vont dans votre `.env` (Docker compose) ou en variables d'environnement `EVEY_*`.

**1. Serveur récent ordinaire, recommandation : ne rien faire.**
```bash
# Defaults are correct. Optionally make it explicit:
EVEY_PLUGINS_SANDBOX_MODE=auto          # picks the strongest available
# EVEY_PLUGINS_UID_SEPARATION=auto      # (default) on only where needed
```

**2. Vieux NAS, protection maximale des secrets :**
```bash
EVEY_PLUGINS_SANDBOX_MODE=auto
EVEY_PLUGINS_UID_SEPARATION=on          # force the secret wall on
# and do NOT set no-new-privileges in your compose security_opt
```

**3. Ceinture et bretelles maximales sur un serveur récent (les deux murs empilés) :**
```bash
EVEY_PLUGINS_SANDBOX_MODE=auto          # → landlock
EVEY_PLUGINS_UID_SEPARATION=on          # add the independent uid wall on top
```

**4. Épingler un mode précis (avancé) :**
```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock      # refuse to start plugins if Landlock is unavailable (fail-closed)
```

**5. Une extension casse sous la séparation d'uid :**
```bash
EVEY_PLUGINS_UID_SEPARATION=off
```

**6. Plafonds de ressources (tous modifiables ensuite depuis Admin → Plugins) :**
```bash
EVEY_PLUGINS_MEMORY_MB=256              # max RAM per plugin
EVEY_PLUGINS_PIDS_MAX=64                # max processes/threads
EVEY_PLUGINS_DISK_MAX_MB=100            # max install-dir size (0 = off)
EVEY_PLUGINS_CPU_NICE=10                # scheduling priority 0-19 (higher = nicer)
```

---

## Changer de mode de bac à sable en cours de fonctionnement

Le mode se change depuis la carte **Resource limits** de **Admin → Plugins**. Les
extensions en cours d'exécution sont **redémarrées automatiquement** pour que le
nouveau mode s'applique tout de suite, sans désactivation ni réactivation
manuelle. La pastille Security montre le mode réellement en vigueur. Si une
extension ne peut pas tourner sous le nouveau mode (par exemple `bubblewrap`
choisi sous Docker, où il lui faut des privilèges), elle apparaît **en erreur**
dans le tableau de bord : il suffit de revenir en arrière.

---

## Vérifier que cela fonctionne

```bash
# the launcher carries the capability used for uid separation:
docker exec evey-server getcap /evey-plugin-launcher        # → cap_setuid=ep

# which user a running plugin runs as (evey = shared with server, evey-plugin = separated):
docker exec evey-server ps -o user,pid,args | grep -v grep | grep -iE "node|python"

# the sandbox mode actually in force: the Security badge on the Admin → Plugins page.
```

---

## Voir aussi

- [SECURITY.md](../SECURITY.md) : le modèle de sécurité complet des extensions
  (chaque couche, le tableau par mode, les limites connues).
- [deployment-bare-metal.md](../deployment-bare-metal.md) : faire tourner Evey
  sans Docker.
- [installation.md](../installation.md) : le pas-à-pas d'auto-hébergement avec
  Docker.
