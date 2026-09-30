---
source_sha: 6cc762f64836b49bd3a49bd9b35c97a935b7f1fbc1bd20e9face59ed55072167
reviewed: true
---
# Installation sans Docker

Faire tourner Evey directement sur une machine (systemd, un script d'init, ou à
la main) plutôt qu'avec [l'image Docker](installation.md). L'image Docker est la
voie recommandée : elle fournit la sandbox des plugins, le durcissement et les
environnements d'exécution déjà branchés. Sans Docker, c'est **à vous** de
reproduire cette posture : les bons binaires côte à côte, un utilisateur dédié
non root, le moyen d'isolation que votre noyau prend en charge, et des dossiers
de secrets accessibles à leur seul propriétaire. Cette page en donne les étapes.

Si vous voulez simplement l'installation facile, arrêtez-vous ici et suivez
[installation.md](installation.md).

## 1. Prérequis

### Les binaires (compilés depuis les sources)

Compilez les deux binaires Go et **placez-les dans le même dossier**. Le lanceur
de plugins est cherché d'abord dans `$EVEY_PLUGIN_LAUNCHER`, puis à côté de
l'exécutable du serveur (`os.Executable()`), puis dans le `$PATH` : les garder
côte à côte est donc la disposition la plus simple et correcte.

```bash
# depuis la racine du dépôt
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags)" -o evey ./cmd/evey
CGO_ENABLED=0 go build -ldflags="-s -w"                                        -o evey-plugin-launcher ./cmd/evey-plugin-launcher

# installation côte à côte, par exemple :
sudo install -m 0755 evey evey-plugin-launcher /opt/evey/

# facultatif : complétion du shell et page de manuel, toutes deux produites par le binaire
/opt/evey/evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
/opt/evey/evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null
```

Ordre de recherche dans `launcherPath()`
(`internal/adapters/driven/pluginrunner/runner_landlock.go`) :

1. `$EVEY_PLUGIN_LAUNCHER` (doit être un fichier exécutable) : forçage explicite.
2. `evey-plugin-launcher` dans le **même dossier** que le binaire `evey` en cours
   d'exécution.
3. `evey-plugin-launcher` quelque part dans le `$PATH`.

### La cale seccomp chargée par LD_PRELOAD

Compilez la cale `libevey-sandbox.so` (un petit fichier C ; il faut un
compilateur C) :

```bash
gcc -shared -fPIC -nostartfiles -o libevey-sandbox.so sandbox/sandbox.c
sudo install -m 0644 libevey-sandbox.so /app/libevey-sandbox.so
```

Le lanceur la cherche au **chemin codé en dur `/app/libevey-sandbox.so`**
(`sandboxLibPath` dans `runner.go`). Si elle est absente, les plugins **refusent
de démarrer** (échec fermé), sauf en mode développement. Cette cale ajoute une
liste d'interdictions seccomp pour les plugins liés dynamiquement, par-dessus
Landlock ou bubblewrap.

> L'image Docker range les binaires open source, le SDK et cette cale sous `/app`
> (aucun secret à cet endroit). Sans Docker, vous pouvez reproduire ce chemin
> (`/app/libevey-sandbox.so`) ou y créer un lien symbolique.

### Environnements d'exécution

Installez ce qu'appelle l'`entrypoint:` de vos plugins. Le lanceur reconnaît ces
interpréteurs (`knownInterpreters` dans `runner.go`) : `python3`, `python`,
`node`, `php`, `ruby`, `perl`, `lua`, `bash`, `sh`. Au minimum :

- **`python3`** : la majorité des plugins de la communauté.
- **`node`**, **`php`**, **`ruby`**, **`lua`** : seulement si vos plugins s'en
  servent.

Plus les dépendances de base :

- **ffmpeg** (transcodage ; l'image embarque aussi les pilotes VAAPI et
  `chromaprint` pour la détection des génériques).
- **PostgreSQL** (le serveur et une base).
- **Meilisearch** (l'index de recherche ; ou en mode intégré via le binaire
  officiel).
- **`bwrap`** (bubblewrap) : seulement si vous comptez utiliser le moyen
  d'isolation bubblewrap (voir plus bas).
- **`libcap`** (`setcap`) : pour accorder la capacité de fichier décrite au §4.

## 2. Faire tourner Evey sous un utilisateur dédié non root

**Ne faites jamais tourner le serveur en root.** Créez deux utilisateurs sans
privilèges, avec les UID qu'utilise l'hôte des plugins (`pluginUID = 1001`,
`pluginGID = 1001` dans `pluginhost/host_server.go`) :

```bash
sudo useradd --system --uid 1000 --shell /usr/sbin/nologin evey
sudo useradd --system --uid 1001 --gid evey --shell /usr/sbin/nologin evey-plugin
```

- `evey` (UID 1000) fait tourner le serveur.
- `evey-plugin` (UID 1001) est l'UID sous lequel les plugins passent (via le
  lanceur, quand la séparation d'UID est active) : un plugin compromis se
  retrouve ainsi sous un autre utilisateur sans privilèges, qui ne peut pas lire
  les secrets du serveur à cause des simples droits du système de fichiers. Voir
  le §4.

Le serveur se déclare **non dumpable** au démarrage (`PR_SET_DUMPABLE=0`,
`cmd/evey/hardening_linux.go`), pour qu'un plugin voisin ne puisse pas lire
`/proc/<pid-du-serveur>/{mem,environ}`.

## 3. Choisir le moyen d'isolation selon votre noyau

Réglez `EVEY_PLUGINS_SANDBOX_MODE`. `auto` (par défaut) essaie
**landlock → bubblewrap → basic** et retient le plus fort que votre machine
permet (`resolveAutoMode()` dans `runner_landlock.go`). Choisissez-le
explicitement si vous voulez un échec fermé.

### Landlock (recommandé)

Une liste d'autorisations du système de fichiers par chemin (les plugins ne
voient que leur propre dossier et celui de la socket, jamais vos données ni vos
secrets), plus un filtre Landlock v4 des sorties TCP. Il ne demande **ni
privilège, ni montage, ni espace de noms** : le lanceur se l'applique à lui-même,
puis exécute le plugin par `execve`.

**Exigence noyau** : Landlock doit être à la fois compilé *et* présent dans la
liste des LSM actifs. `landlockAvailable()` (`landlock_probe_linux.go`) sonde
`landlock_create_ruleset(VERSION)` ; l'appel renvoie une erreur quand il n'existe
pas (noyau `< 5.13`) ou quand Landlock n'est pas dans la liste des LSM actifs.
Pour vérifier :

```bash
uname -r                              # il faut >= 5.13
cat /sys/kernel/security/lsm          # doit contenir "landlock"
```

Si `landlock` manque dans cette liste, activez-le sur la ligne de commande du
noyau et redémarrez (la liste exacte varie selon la distribution : gardez les
entrées existantes et ajoutez `landlock`) :

```
# par exemple dans GRUB_CMDLINE_LINUX :
lsm=lockdown,capability,yama,apparmor,bpf,landlock
```

Ensuite :

```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock    # explicite, échec fermé (refuse un plugin si Landlock ne peut pas s'appliquer)
# ou laissez la valeur par défaut :
EVEY_PLUGINS_SANDBOX_MODE=auto        # devient landlock quand il est disponible
```

Le mode `landlock` passe `-require-landlock` au lanceur, qui s'arrête en erreur
plutôt que de lancer un plugin non confiné si Landlock ne s'applique pas.

### bubblewrap

Si vous ne pouvez pas avoir Landlock, installez le binaire `bwrap`. bubblewrap
place chaque plugin dans des espaces de noms user/pid/uts/ipc/(net) avec une
liste blanche de montages. Il demande que les **espaces de noms utilisateur non
privilégiés soient activés** et que **l'appel système `mount` soit disponible** :
c'est le cas sur la plupart des Linux installés directement, mais c'est bloqué
dans un conteneur Docker non privilégié (et c'est précisément pour cela que
Landlock est la valeur par défaut en conteneur). `auto` ne choisit bubblewrap que
si Landlock est **absent** et qu'un essai de bwrap jetable réussit réellement
(`ProbeSandbox` / `sandbox_probe.go`).

```bash
EVEY_PLUGINS_SANDBOX_MODE=bubblewrap  # espaces de noms bwrap
# ou :
EVEY_PLUGINS_SANDBOX_MODE=strict      # bubblewrap + la cale seccomp chargée par LD_PRELOAD
```

Vérifiez que la machine permet vraiment la mise en place des espaces de noms et
des montages :

```bash
bwrap --unshare-user --unshare-net --ro-bind / / /bin/true && echo "bwrap OK"
```

### basic (en dernier recours)

`EVEY_PLUGINS_SANDBOX_MODE=basic` : changement d'UID et cale seccomp chargée par
LD_PRELOAD, et le lanceur pose quand même la liste d'interdictions seccomp du
noyau, et Landlock au mieux s'il est présent. Il n'y a **aucun confinement du
système de fichiers par chemin** : un plugin tourne sous l'UID 1001 avec seccomp
seulement. Ne l'utilisez que sur des noyaux anciens ou verrouillés où ni Landlock
ni bubblewrap ne sont disponibles, et appuyez-vous sur la séparation d'UID du §4
pour protéger les secrets.

> **Un plugin qui a la permission `subprocess` exige le lanceur**, même en mode
> basic. La cale LD_PRELOAD ne survit pas à l'`exec` d'un processus enfant, et ne
> peut donc pas confiner un plugin autorisé à lancer des sous-processus : seul le
> seccomp noyau du lanceur (posé juste avant l'`exec` et hérité à travers lui) le
> peut. Si `evey-plugin-launcher` manque (voir le §1), un tel plugin **refuse de
> démarrer** en production (échec fermé) plutôt que de tourner sans confinement.
> Les plugins sans cette permission se rabattent sur la cale LD_PRELOAD, comme
> décrit plus haut.

## 4. Séparation d'UID (protéger les secrets sans root)

La séparation d'UID fait tourner chaque plugin sous un utilisateur distinct
(UID 1001, `evey-plugin`), différent du serveur (UID 1000), si bien qu'un plugin
ne peut pas lire le `jwt.key` du serveur, ses sauvegardes ou ses journaux, à cause
des simples droits du système de fichiers (DAC). Elle compte surtout en mode
**`basic`** (pas de confinement Landlock par chemin) ; sous Landlock ou bubblewrap,
le système de fichiers est déjà cloisonné, et elle y est redondante.

Elle se règle avec `EVEY_PLUGINS_UID_SEPARATION` (`auto` | `on` | `off`, par
défaut `auto`) :

- `auto` : activée seulement quand l'isolation retenue est `basic`. Sur un noyau
  récent installé directement (Landlock), elle reste désactivée, à dessein.
- `on` : toujours, même sous Landlock ou bubblewrap (une couche indépendante en
  plus).
- `off` : jamais.

**C'est le lanceur qui change d'UID, pas le serveur.** Le serveur reste non root,
**sans** aucune capacité ; le petit binaire `evey-plugin-launcher` porte une seule
capacité de fichier et passe lui-même à l'UID 1001 par `setuid` juste avant
d'exécuter le plugin :

```bash
sudo setcap cap_setuid+ep /opt/evey/evey-plugin-launcher
```

Le serveur reste ainsi entièrement sans privilèges : une exécution de code à
distance sur le serveur ne peut pas faire `setuid(0)`, puisque le serveur n'a
aucune capacité ; seul le minuscule lanceur, qui ne lit aucune entrée, en a une
(et il refuse tout UID cible inférieur à 1000). Si le lanceur ne peut pas changer
d'UID (capacité absente, ou `no_new_privs` actif), un plugin en mode `basic` est
**refusé**, puisque l'UID y est sa seule cloison pour les secrets ; sous Landlock,
un avertissement est journalisé et le plugin tourne sous l'UID du serveur.

> **Incompatible avec `no_new_privs`.** Si vous lancez le serveur avec
> `no-new-privileges` (ou `NoNewPrivileges=yes` dans systemd), la capacité de
> fichier est ignorée à l'exécution : les plugins sont refusés en mode `basic`, et
> sous Landlock la séparation d'UID se dégrade en même UID (avec un
> avertissement). Il faut choisir.

Si le serveur tourne au contraire **en root** (déconseillé), il fait passer
lui-même les plugins à l'UID 1001, et la capacité du lanceur n'est pas
nécessaire.

Le guide en langage simple donne la vue par cas :
[plugins/sandbox-deployment.md](plugins/sandbox-deployment.md).

## 5. Droits des fichiers secrets

Reproduisez ce que fait l'image Docker (`Dockerfile`) : gardez les dossiers qui
contiennent des secrets **accessibles à leur seul propriétaire**, pour qu'un
plugin passé à l'UID 1001 ne puisse pas les lire avec les simples droits DAC,
même sans Landlock.

```bash
sudo chown -R evey:evey /var/lib/evey
# Le SERVEUR possède plugins/ (il y extrait les installations) ; le groupe evey,
# dont fait partie evey-plugin, ne fait que lire et exécuter les fichiers installés :
sudo chmod 750 /var/lib/evey/plugins
# propriétaire seul (ni groupe ni autres) sur les dossiers de secrets :
sudo chmod 700 /var/lib/evey/keys /var/lib/evey/logs /var/lib/evey/backups
# la clé de signature JWT elle-même :
sudo chmod 600 /var/lib/evey/jwt.key      # (générée en 0600 au premier démarrage)
```

(Adaptez les chemins à ceux vers lesquels pointent vos `EVEY_*_DIR` /
`EVEY_JWT_PRIVATE_KEY_PATH`.)

## 6. Environnement et configuration

Toute la configuration d'exécution passe par des variables d'environnement
`EVEY_*`. Réglez la base de données, la recherche, les chemins de stockage, le
chemin de la clé JWT et les variables du reverse proxy comme pour un déploiement
Docker : voir [configuration.md](configuration.md) pour le tableau complet et
[installation.md](installation.md) pour la base de données, la recherche et le
reverse proxy. Ce tableau n'est pas répété ici. Points propres à l'installation
sans Docker :

- Faites pointer `EVEY_DB_HOST` / `EVEY_MEILI_URL` vers vos PostgreSQL et
  Meilisearch locaux.
- Le serveur sert toujours du **HTTP simple** : placez un reverse proxy devant
  pour le TLS (une URL externe en `https://` et le proxy dans Proxys de
  confiance, dans **Admin → Accès distant**, ou `EVEY_SERVER_BASE_URL` et
  `EVEY_TRUSTED_PROXIES`). Voir [reverse-proxy.md](reverse-proxy.md).
- **Le serveur ouvre DEUX ports, et un seul doit être exposé.**
  `EVEY_SERVER_PORT` (par défaut `9090`) est l'écoute gRPC native ; l'API HTTP à
  laquelle parlent vos clients et votre reverse proxy est sur **ce port plus un**
  (`9091`). Dirigez votre reverse proxy et votre pare-feu vers le port HTTP
  seulement. Le port gRPC est un transport interne : le contrôle réseau de
  l'accès distant, qui décide si les clients hors de votre réseau local peuvent
  se connecter, est branché dans la chaîne HTTP et **ne couvre pas l'écoute
  gRPC**. C'est pourquoi l'écoute gRPC est liée par défaut à la **seule boucle
  locale** (`EVEY_GRPC_BIND=127.0.0.1`), pour qu'une installation sans Docker ne
  puisse pas l'exposer par accident. Si vous avez une raison de la publier (un
  client gRPC natif sur une autre machine), réglez `EVEY_GRPC_BIND=0.0.0.0` ; le
  serveur prévient au démarrage que le contrôle ne s'y applique pas, et le
  filtrage par pare-feu vous revient alors.
- Les migrations s'exécutent automatiquement au démarrage ; aucune étape de
  migration séparée.
- Les plafonds de ressources des plugins (`EVEY_PLUGINS_MEMORY_MB`, `_PIDS_MAX`,
  `_DISK_MAX_MB`, `_CPU_NICE`) et le mode de sandbox se modifient en cours
  d'exécution depuis Admin → Plugins ; les valeurs de l'environnement ne sont que
  les valeurs initiales.

## 7. Vérifier votre sandbox

Le serveur journalise le moyen d'isolation retenu. Après le démarrage du premier
plugin, regardez le journal du serveur :

- **choix automatique** (journalisé une fois) :
  `plugin sandbox auto-detection complete  selected_mode=landlock`
  (ou `bubblewrap` / `basic`).
- **par plugin** (à chaque démarrage de plugin) :
  `plugin sandbox applied  sandbox_mode=landlock plugin_id=...`
- **repli sur basic** (un avertissement que vous ne voulez *pas* voir si vous
  attendiez Landlock) : `plugin sandbox auto-detection fell back to basic mode:
  no per-path filesystem isolation available …` : ni Landlock ni un bwrap
  fonctionnel n'ont été trouvés. Revoyez le §3.

Vérifiez les éléments du noyau indépendamment :

```bash
cat /sys/kernel/security/lsm          # Landlock actif ? (doit lister "landlock")
grep Seccomp: /proc/<pid-du-plugin>/status   # 2 = filtre actif sur un plugin en cours d'exécution
```

Une ligne `Seccomp: 2` sur un processus de plugin vivant confirme que le filtre
seccomp du noyau est posé. Si vous avez imposé `EVEY_PLUGINS_SANDBOX_MODE=landlock`
et que Landlock ne s'applique pas, le plugin **refuse de démarrer** : c'est le
contrat d'échec fermé qui fonctionne comme prévu.

---

Pour tout le reste (sauvegardes, mises à jour, CLI de récupération, dépannage),
les sections orientées Docker d'[installation.md](installation.md) s'appliquent
de la même façon : les sous-commandes `evey admin` et le fonctionnement des
sauvegardes sont ceux du même binaire.
