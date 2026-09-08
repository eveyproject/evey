---
source_sha: 8e943dfd72345f44add4c70011cd163dc77c351f5f410f69d2165ba7c88ea11d
---

# Installation sans conteneur (hors Docker)

Faire tourner Evey directement sur une machine (avec systemd, un script d'init,
ou à la main) plutôt qu'à partir de l'[image Docker](installation.md). L'image
Docker reste la voie recommandée : elle livre le bac à sable des extensions, le
durcissement et les environnements d'exécution déjà câblés. Se passer de
conteneur, c'est reproduire **soi-même** cette posture : les bons binaires côte
à côte, un utilisateur dédié non privilégié, le mécanisme d'isolation que prend
en charge votre noyau, et des répertoires de secrets lisibles par leur seul
propriétaire. Cette page en donne le pas-à-pas.

Si vous cherchez simplement l'installation facile, arrêtez-vous ici et suivez
[installation.md](installation.md).

## 1. Prérequis

### Les binaires (compilation depuis les sources)

Compilez les deux binaires Go et **placez-les dans le même répertoire**. Le
gestionnaire d'extensions cherche le lanceur d'abord dans
`$EVEY_PLUGIN_LAUNCHER`, puis à côté de son propre exécutable
(`os.Executable()`), puis dans `$PATH` : les garder voisins est donc la
disposition correcte la plus simple.

```bash
# depuis la racine du dépôt
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags)" -o evey ./cmd/evey
CGO_ENABLED=0 go build -ldflags="-s -w"                                        -o evey-plugin-launcher ./cmd/evey-plugin-launcher

# installation côte à côte, par exemple :
sudo install -m 0755 evey evey-plugin-launcher /opt/evey/
```

Ordre de recherche dans `launcherPath()` (`internal/adapters/driven/pluginrunner/runner_landlock.go`) :

1. `$EVEY_PLUGIN_LAUNCHER` (qui doit être un fichier exécutable) - la dérogation
   explicite.
2. `evey-plugin-launcher` dans le **même répertoire** que le binaire `evey` en
   cours d'exécution.
3. `evey-plugin-launcher` quelque part dans le `$PATH`.

### L'interposition seccomp LD_PRELOAD

Compilez la bibliothèque d'interposition `libevey-sandbox.so` (un petit fichier
C ; il vous faut un compilateur C) :

```bash
gcc -shared -fPIC -nostartfiles -o libevey-sandbox.so sandbox/sandbox.c
sudo install -m 0644 libevey-sandbox.so /app/libevey-sandbox.so
```

Le gestionnaire la cherche au **chemin codé en dur `/app/libevey-sandbox.so`**
(`sandboxLibPath` dans `runner.go`). Si elle manque, les extensions **refusent
de démarrer** (comportement fermé par défaut), sauf en mode développement. Cette
bibliothèque ajoute une liste noire d'appels système seccomp pour les extensions
liées dynamiquement, et vient se superposer à Landlock ou à bubblewrap.

> L'image Docker range les binaires open source, le SDK et cette bibliothèque
> sous `/app`, où ne se trouve aucun secret. Sans conteneur, vous pouvez soit
> reproduire ce chemin (`/app/libevey-sandbox.so`), soit y placer un lien
> symbolique.

### Les environnements d'exécution

Installez ce qu'invoque l'`entrypoint:` de vos extensions. Le gestionnaire
reconnaît ces interpréteurs (`knownInterpreters` dans `runner.go`) : `python3`,
`python`, `node`, `php`, `ruby`, `perl`, `lua`, `bash`, `sh`. Au minimum :

- **`python3`** - la majorité des extensions de la communauté.
- **`node`**, **`php`**, **`ruby`**, **`lua`** - seulement si vos extensions
  s'en servent.

S'y ajoutent les dépendances d'exécution du cœur :

- **ffmpeg** (le transcodage ; l'image embarque en plus les pilotes VAAPI et
  `chromaprint` pour la détection des génériques).
- **PostgreSQL** (le serveur, plus une base de données).
- **Meilisearch** (l'index de recherche ; ou son exécution en embarqué à partir
  du binaire officiel).
- **`bwrap`** (bubblewrap) - seulement si vous comptez utiliser bubblewrap comme
  mécanisme d'isolation (voir plus bas).
- **`libcap`** (`setcap`) - pour accorder la capacité de fichier décrite au §4.

## 2. Tourner sous un utilisateur dédié, non root

**Ne lancez jamais le serveur en root.** Créez deux utilisateurs non
privilégiés, correspondant aux UID qu'emploie l'hôte d'extensions
(`pluginUID = 1001`, `pluginGID = 1001` dans `pluginhost/host_server.go`) :

```bash
sudo useradd --system --uid 1000 --shell /usr/sbin/nologin evey
sudo useradd --system --uid 1001 --gid evey --shell /usr/sbin/nologin evey-plugin
```

- `evey` (UID 1000) fait tourner le serveur.
- `evey-plugin` (UID 1001) est l'UID vers lequel les extensions sont
  rétrogradées, par le lanceur, lorsque la séparation d'UID est active : une
  extension compromise se retrouve ainsi sous un autre utilisateur, non
  privilégié, incapable de lire les secrets du serveur par les simples
  permissions du système de fichiers. Voir §4.

Le serveur se marque lui-même **non dumpable** au démarrage
(`PR_SET_DUMPABLE=0`, `cmd/evey/hardening_linux.go`), afin qu'une extension
cohabitante ne puisse pas fouiller `/proc/<server-pid>/{mem,environ}`.

## 3. Choisir le mécanisme d'isolation adapté à votre noyau

Renseignez `EVEY_PLUGINS_SANDBOX_MODE`. La valeur par défaut, `auto`, parcourt
l'ordre **landlock → bubblewrap → basic** et retient le plus solide que votre
machine prend en charge (`resolveAutoMode()` dans `runner_landlock.go`). Fixez
une valeur explicite si vous voulez un comportement fermé par défaut.

### Landlock (recommandé)

Une liste blanche de chemins sur le système de fichiers (une extension ne voit
que son propre répertoire et celui de sa socket, jamais vos données ni vos
secrets), doublée d'un filtre de sortie TCP Landlock v4. Cela ne demande **ni
privilège, ni montage, ni espace de noms** : le lanceur se l'applique à
lui-même, puis fait un `execve` de l'extension.

**Exigence côté noyau** : Landlock doit être à la fois compilé dans le noyau
*et* présent dans la liste des LSM actifs. La fonction `landlockAvailable()` du
gestionnaire (`landlock_probe_linux.go`) sonde
`landlock_create_ruleset(VERSION)` ; elle renvoie un errno quand l'appel système
est absent (noyau `<5.13`) ou quand Landlock ne figure pas dans la liste des LSM
actifs. Pour vérifier :

```bash
uname -r                              # on veut >= 5.13
cat /sys/kernel/security/lsm          # doit contenir "landlock"
```

Si `landlock` manque à cette liste, activez-le sur la ligne de commande du noyau
puis redémarrez. La liste exacte varie d'une distribution à l'autre : conservez
les entrées existantes et ajoutez `landlock` à la fin.

```
# par exemple dans GRUB_CMDLINE_LINUX :
lsm=lockdown,capability,yama,apparmor,bpf,landlock
```

Ensuite :

```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock    # explicite et fermé par défaut (refuse une extension si Landlock ne peut pas être appliqué)
# ou laissez la valeur par défaut :
EVEY_PLUGINS_SANDBOX_MODE=auto        # donne landlock lorsqu'il est disponible
```

Le mode `landlock` passe `-require-landlock` au lanceur, qui sort avec un code
non nul plutôt que d'exécuter une extension sans confinement si Landlock ne
s'applique pas.

### bubblewrap

Si Landlock est hors d'atteinte, installez le binaire `bwrap`. bubblewrap place
chaque extension dans des espaces de noms user, pid, uts, ipc (et net) assortis
d'une liste blanche de montages. Il exige que les **espaces de noms utilisateur
non privilégiés soient activés** et que l'**appel système `mount` soit
disponible** : c'est le cas sur la plupart des Linux installés sur machine nue,
mais pas dans un conteneur Docker non privilégié, où il est bloqué. C'est
précisément pour cette raison que Landlock est le choix par défaut en conteneur.
`auto` ne retient bubblewrap que si Landlock est **absent** et qu'une sonde
bwrap jetable réussit réellement (`ProbeSandbox` / `sandbox_probe.go`).

```bash
EVEY_PLUGINS_SANDBOX_MODE=bubblewrap  # espaces de noms bwrap
# ou :
EVEY_PLUGINS_SANDBOX_MODE=strict      # bubblewrap + l'interposition seccomp LD_PRELOAD
```

Vérifiez que la machine autorise réellement la combinaison espaces de noms et
montage :

```bash
bwrap --unshare-user --unshare-net --ro-bind / / /bin/true && echo "bwrap OK"
```

### basic (en dernier recours)

`EVEY_PLUGINS_SANDBOX_MODE=basic` : rétrogradation d'UID et interposition
seccomp LD_PRELOAD ; le lanceur continue d'installer la liste noire seccomp du
noyau et, au mieux, Landlock s'il est présent. Il n'y a **aucun confinement du
système de fichiers par chemin** : l'extension tourne sous l'UID 1001, avec
seccomp pour seule barrière. Ne vous en servez que sur des noyaux anciens ou
verrouillés, où ni Landlock ni bubblewrap ne sont disponibles, et appuyez-vous
sur la séparation d'UID du §4 pour protéger les secrets.

> **Une extension à qui la permission `subprocess` a été accordée exige le
> lanceur**, y compris en mode basic. L'interposition LD_PRELOAD ne survit pas à
> l'`exec` d'un processus fils : elle ne peut donc pas confiner une extension
> autorisée à lancer des sous-processus. Seul le seccomp noyau du lanceur le
> peut, puisqu'il est installé juste avant l'`exec` et hérité au travers. Si
> `evey-plugin-launcher` est absent (voir §1), une telle extension **refuse de
> démarrer** en production (fermé par défaut) plutôt que de tourner sans
> confinement. Les extensions dépourvues de cette permission retombent sur
> l'interposition LD_PRELOAD, comme décrit plus haut.

## 4. La séparation d'UID (protéger les secrets sans root)

La séparation d'UID exécute chaque extension sous un utilisateur distinct
(UID 1001, `evey-plugin`), différent de celui du serveur (UID 1000) : une
extension ne peut alors lire ni le `jwt.key` du serveur, ni ses sauvegardes, ni
ses journaux, par le simple jeu des permissions du système de fichiers (DAC).
Elle compte surtout en mode **`basic`**, où le confinement par chemin de
Landlock fait défaut ; sous Landlock ou bubblewrap, le système de fichiers est
déjà cloisonné et elle y fait double emploi.

Elle se pilote par `EVEY_PLUGINS_UID_SEPARATION` (`auto` | `on` | `off`, `auto`
par défaut) :

- `auto` - activée seulement lorsque l'isolation retenue est `basic`. Sur un
  noyau moderne installé sur machine nue, donc avec Landlock, elle reste
  inactive, et c'est voulu.
- `on` - toujours activée, y compris sous Landlock ou bubblewrap, comme couche
  supplémentaire et indépendante.
- `off` - jamais activée.

**C'est le lanceur qui procède à la rétrogradation, pas le serveur.** Le serveur
reste non root et **sans aucune** capacité ; le petit binaire
`evey-plugin-launcher` porte une unique capacité de fichier et se fait un
`setuid` vers 1001 juste avant d'exécuter l'extension :

```bash
sudo setcap cap_setuid+ep /opt/evey/evey-plugin-launcher
```

Le serveur reste ainsi totalement dépourvu de privilèges : une exécution de code
à distance sur le serveur ne peut pas appeler `setuid(0)`, puisque le serveur ne
détient aucune capacité. Seul le lanceur, minuscule et sans entrée à traiter, en
possède une, et il refuse tout UID cible inférieur à 1000. Le mécanisme
fonctionne **au mieux** : si le lanceur ne parvient pas à rétrograder (capacité
manquante, ou `no_new_privs` positionné), il journalise un avertissement et
exécute l'extension sous l'UID du serveur au lieu d'échouer, de sorte qu'il
n'empêche jamais une extension de démarrer.

> **Incompatible avec `no_new_privs`.** Si vous lancez le serveur avec
> `no-new-privileges` (ou `NoNewPrivileges=yes` sous systemd), la capacité de
> fichier est ignorée à l'`exec` et la séparation d'UID se dégrade en silence
> vers un UID unique, avec un avertissement. Il faut choisir l'un ou l'autre.

Si le serveur tourne **en root** (ce qui n'est pas recommandé), il rétrograde
lui-même les extensions vers 1001 et la capacité du lanceur devient inutile.

Le guide en langage courant détaille la situation scénario par scénario :
[plugins/sandbox-deployment.md](plugins/sandbox-deployment.md).

## 5. Les permissions des fichiers de secrets

Reproduisez ce que fait l'image Docker (`Dockerfile`) : gardez les répertoires
porteurs de secrets **accessibles à leur seul propriétaire**, pour qu'une
extension rétrogradée vers l'UID 1001 ne puisse pas les lire par le simple DAC,
même sans Landlock.

```bash
sudo chown -R evey:evey /var/lib/evey
# Le SERVEUR possède plugins/ (c'est là qu'il extrait les installations) ; le
# groupe evey, auquel evey-plugin appartient, se contente de lire et d'exécuter
# les fichiers installés :
sudo chmod 750 /var/lib/evey/plugins
# propriétaire seul (ni groupe, ni autres) sur les répertoires de secrets :
sudo chmod 700 /var/lib/evey/keys /var/lib/evey/logs /var/lib/evey/backups
# la clé de signature JWT elle-même :
sudo chmod 600 /var/lib/evey/jwt.key      # (générée automatiquement en 0600 au premier démarrage)
```

(Adaptez les chemins à ce que désignent vos `EVEY_*_DIR` et
`EVEY_JWT_PRIVATE_KEY_PATH`.)

## 6. Environnement et configuration

Toute la configuration d'exécution passe par des variables d'environnement
`EVEY_*`. Renseignez la base de données, la recherche, les chemins de stockage,
le chemin de la clé JWT et les variables du reverse proxy exactement comme le
fait un déploiement Docker : [configuration.md](configuration.md) donne le
tableau complet, et [installation.md](installation.md) déroule la mise en place
de la base, de la recherche et du reverse proxy. Ce tableau n'est pas repris
ici. Quelques points propres à l'installation sans conteneur :

- Faites pointer `EVEY_DB_HOST` et `EVEY_MEILI_URL` vers vos PostgreSQL et
  Meilisearch locaux.
- Le serveur ne sert toujours que du **HTTP simple** : placez un reverse proxy
  devant lui pour le TLS (`EVEY_SERVER_BASE_URL` avec un schéma `https://`,
  `EVEY_TRUSTED_PROXIES`). Voir [reverse-proxy.md](reverse-proxy.md).
- **Le serveur ouvre DEUX ports, et un seul doit être exposé.**
  `EVEY_SERVER_PORT` (`9090` par défaut) est le port d'écoute gRPC natif ; l'API
  HTTP à laquelle s'adressent vos clients et votre reverse proxy est **ce port
  plus un** (`9091`). Ne dirigez votre reverse proxy et votre pare-feu que vers
  le port HTTP. Le port gRPC est un transport interne : le verrou réseau d'accès
  distant, celui qui décide si des clients extérieurs à votre réseau local ont
  le droit de se connecter, est branché sur la chaîne HTTP et **ne couvre pas
  l'écoute gRPC**. C'est pourquoi celle-ci n'écoute par défaut que sur la
  **boucle locale** (`EVEY_GRPC_BIND=127.0.0.1`) : une installation sans
  conteneur ne peut donc pas l'exposer par mégarde. Si vous avez une raison de
  la publier, par exemple un client gRPC natif sur une autre machine, mettez
  `EVEY_GRPC_BIND=0.0.0.0` ; le serveur avertit alors au démarrage que le verrou
  ne s'y applique pas, et le pare-feu devient votre affaire.
- Les migrations s'exécutent automatiquement au démarrage ; il n'y a pas
  d'étape de migration séparée.
- Les plafonds de ressources des extensions (`EVEY_PLUGINS_MEMORY_MB`,
  `_PIDS_MAX`, `_DISK_MAX_MB`, `_CPU_NICE`) et le mode d'isolation se modifient
  à chaud depuis Admin → Plugins ; les valeurs d'environnement ne sont que les
  valeurs de départ.

## 7. Vérifier votre isolation

Le serveur journalise le mécanisme d'isolation retenu. Une fois la première
extension démarrée, consultez le journal du serveur :

- **résolution du mode auto** (journalisée une seule fois) :
  `plugin sandbox auto-detection complete  selected_mode=landlock`
  (ou `bubblewrap`, ou `basic`).
- **par extension** (à chaque démarrage d'extension) :
  `plugin sandbox applied  sandbox_mode=landlock plugin_id=...`
- **repli sur basic** (un avertissement que vous ne voulez *pas* voir si vous
  attendiez Landlock) : `plugin sandbox auto-detection fell back to basic mode:
  no per-path filesystem isolation available …` - il signifie que ni Landlock ni
  un bwrap fonctionnel n'ont été trouvés. Reprenez le §3.

Confirmez les briques du noyau de votre côté :

```bash
cat /sys/kernel/security/lsm          # Landlock est-il actif ? (la liste doit contenir "landlock")
grep Seccomp: /proc/<plugin-pid>/status   # 2 = mode filtre actif sur une extension en cours
```

Une ligne `Seccomp: 2` sur le processus d'une extension en cours d'exécution
confirme que le filtre seccomp du noyau est bien installé. Si vous avez fixé
`EVEY_PLUGINS_SANDBOX_MODE=landlock` et que Landlock ne s'applique pas,
l'extension **refuse de démarrer** : c'est le contrat fermé par défaut qui joue
son rôle.

---

Pour tout le reste (sauvegardes, mises à jour, CLI de récupération, dépannage),
les sections orientées Docker d'[installation.md](installation.md) valent tout
autant : les sous-commandes `evey admin` et les mécanismes de sauvegarde
relèvent du même binaire.
