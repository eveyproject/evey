---
source_sha: 4aeebbe2feb50999e8470ea3f56b3e47af196ca12cf0100e22527f66ffe278e3
reviewed: true
---
# Sans conteneur

Comment faire tourner Evey directement sur un hôte (systemd, un script
d'init, ou à la main) au lieu de l'[image Docker](installation.md). L'image
est la voie recommandée : elle livre la sandbox des plugins, le durcissement
et les runtimes des plugins déjà câblés. Se passer du conteneur signifie
reproduire cette posture vous-même : les bons binaires côte à côte, un
utilisateur dédié non root, le moteur de sandbox que votre noyau prend en
charge, et des dossiers de secrets réservés à leur propriétaire. Cette page
déroule tout cela dans l'ordre.

## Construire et installer les binaires

Construisez les deux binaires Go et installez-les dans le même dossier : le
lanceur de plugins cherche le launcher d'abord dans `$EVEY_PLUGIN_LAUNCHER`,
puis à côté de son propre exécutable, puis dans `$PATH`.

```bash
# depuis la racine du dépôt
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags --always) -X main.commit=$(git rev-parse HEAD)" -o evey ./cmd/evey
CGO_ENABLED=0 go build -ldflags="-s -w" -o evey-plugin-launcher ./cmd/evey-plugin-launcher
sudo install -m 0755 evey evey-plugin-launcher /opt/evey/

# facultatif : la complétion shell et la page de manuel, toutes deux imprimées par le binaire
/opt/evey/evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
/opt/evey/evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null
```

`evey version` affiche la version, le commit et la date de construction sans
aucune configuration.

### La cale seccomp

Construisez `libevey-sandbox.so` (un seul fichier C, il faut un compilateur
C) et installez-la au chemin que le lanceur attend, `/app/libevey-sandbox.so` :

```bash
gcc -shared -fPIC -nostartfiles -o libevey-sandbox.so sandbox/sandbox.c
sudo install -D -m 0644 libevey-sandbox.so /app/libevey-sandbox.so
```

Le chemin est fixé dans le lanceur. Sans ce fichier, les plugins refusent de
démarrer (échec fermé) sauf si le serveur tourne en mode développement. La
cale ajoute une liste de refus seccomp pour les plugins liés dynamiquement,
par-dessus Landlock ou bubblewrap. L'image la garde sous `/app` avec le SDK ;
un lien symbolique convient aussi.

### Runtimes et services

Installez ce que le `entrypoint:` de vos plugins invoque. Le lanceur
reconnaît `python3`, `python`, `node`, `php`, `ruby`, `perl`, `lua`, `bash`
et `sh` ; `python3` couvre la plupart des plugins communautaires. Puis les
services auxquels le serveur parle :

| Dépendance | Pourquoi |
|---|---|
| **ffmpeg** et **ffprobe** | transcodage, analyse ; l'image embarque aussi les pilotes VAAPI et `fpcalc` (chromaprint) pour la détection des génériques |
| **PostgreSQL** (serveur plus une base) et **`pg_dump`** | la base ; `pg_dump` écrit le dump automatique de pré-migration |
| **Meilisearch** | l'index de recherche ; sans lui la recherche se replie sur SQL |
| **`bwrap`** (bubblewrap) | uniquement pour le moteur de sandbox bubblewrap |
| **`setcap`** (libcap) | pour accorder au launcher sa capability de fichier |

La base et le moteur de recherche embarqués sont une fonction du point
d'entrée de l'image, pas du binaire : sans conteneur, vous faites tourner les
deux vous-même et pointez `EVEY_DB_HOST` et `EVEY_MEILI_URL` dessus.

## Tourner sous un utilisateur dédié non root

Ne lancez jamais le serveur en root. Créez un groupe et deux utilisateurs
sans privilège, comme le fait l'image :

```bash
sudo groupadd --gid 1000 evey
sudo useradd --system --uid 1000 --gid evey --shell /usr/sbin/nologin evey
sudo useradd --system --uid 1001 --gid evey --shell /usr/sbin/nologin evey-plugin
```

- `evey` (UID 1000) fait tourner le serveur.
- `evey-plugin` (UID 1001) est l'utilisateur vers lequel les plugins sont
  abaissés quand la séparation d'uid est active, pour qu'un plugin compromis
  se retrouve sous un autre utilisateur sans privilège, incapable de lire les
  secrets du serveur par simples permissions de fichiers.

!!! warning
    L'identifiant du groupe compte : le launcher abaisse un plugin vers l'UID
    1001 et vers le groupe du serveur lui-même, et refuse tout uid ou gid
    inférieur à 1000. Un groupe créé par `useradd --system` seul reçoit un
    identifiant système inférieur à 1000, et les plugins en mode `basic` sont
    alors refusés.

Le serveur se marque non « dumpable » au démarrage (`PR_SET_DUMPABLE=0`),
donc un plugin voisin ne peut pas lire `/proc/<pid-du-serveur>/{mem,environ}`.

## Choisir le moteur de sandbox pour votre noyau

Définissez `EVEY_PLUGINS_SANDBOX_MODE`. `auto` (la valeur par défaut) résout
dans l'ordre landlock, bubblewrap, basic et retient le plus fort que l'hôte
prend en charge. Fixez un mode explicitement quand vous voulez un échec
fermé.

### Landlock (recommandé)

Une liste blanche de chemins (un plugin voit son propre dossier et le dossier
de socket, jamais vos données ni vos secrets) plus un filtre de sortie TCP
Landlock v4. Il ne demande ni privilège, ni montage, ni espace de noms : le
launcher se l'applique à lui-même, puis exécute le plugin.

Landlock doit être compilé dans le noyau et présent dans la liste des LSM
actifs. Le lanceur sonde `landlock_create_ruleset` ; la sonde échoue quand
l'appel système est absent (noyau antérieur à 5.13) ou quand Landlock n'est
pas dans la liste active. Vérifiez :

```bash
uname -r                              # il faut 5.13 ou plus récent
cat /sys/kernel/security/lsm          # doit contenir "landlock"
```

Si `landlock` manque dans cette liste, ajoutez-le sur la ligne de commande du
noyau et redémarrez (gardez les entrées existantes, la liste varie selon la
distribution) :

```
# par ex. dans GRUB_CMDLINE_LINUX :
lsm=lockdown,capability,yama,apparmor,bpf,landlock
```

Puis gardez `auto` ou figez le mode :

```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock    # échec fermé : un plugin est refusé quand Landlock ne peut pas être imposé
```

`landlock` passe `-require-landlock` au launcher, qui sort avec un code non
nul plutôt que de lancer un plugin sans confinement.

### bubblewrap

Quand Landlock n'est pas disponible, installez `bwrap`. Il place chaque
plugin dans des espaces de noms utilisateur, pid, uts, ipc et réseau avec
une liste blanche de montages. Il lui faut les espaces de noms utilisateur
non privilégiés et l'appel système `mount`, vrai sur la plupart des Linux
sans conteneur mais bloqué dans un conteneur non privilégié, ce qui explique
que Landlock soit le défaut du conteneur. `auto` ne retient bubblewrap que si
Landlock est absent et qu'une sonde `bwrap` jetable réussit.

```bash
EVEY_PLUGINS_SANDBOX_MODE=bubblewrap  # espaces de noms
EVEY_PLUGINS_SANDBOX_MODE=strict      # espaces de noms plus la cale seccomp
```

Vérifiez que l'hôte autorise la mise en place des espaces de noms et du
montage :

```bash
bwrap --unshare-user --unshare-net --ro-bind / / /bin/true && echo "bwrap OK"
```

### basic (dernier recours)

`EVEY_PLUGINS_SANDBOX_MODE=basic` est un abaissement d'uid plus la cale
seccomp ; le launcher, quand il est présent, installe quand même sa liste de
refus seccomp noyau et un Landlock au mieux. Il n'y a aucun confinement de
fichiers par chemin : un plugin tourne sous l'UID 1001 avec seccomp seul.
Réservez-le aux noyaux anciens ou verrouillés où ni Landlock ni bubblewrap ne
fonctionnent, et comptez sur la séparation d'uid ci-dessous pour protéger les
secrets.

Un plugin doté de la permission `subprocess` exige le launcher même en mode
`basic` : la cale ne survit pas à un `exec` d'un processus enfant, seul le
seccomp noyau du launcher y survit. Sans le launcher, un tel plugin refuse de
démarrer plutôt que de tourner sans confinement.

## Garder les secrets hors de portée d'un plugin

La séparation d'uid fait tourner chaque plugin sous `evey-plugin` (UID
1001), distinct du serveur (UID 1000), pour qu'un plugin ne puisse pas lire
`jwt.key`, les sauvegardes ou les journaux par simples permissions de
fichiers. Elle compte en mode `basic` ; sous Landlock ou bubblewrap le
système de fichiers est déjà cloisonné, elle y est redondante.

`EVEY_PLUGINS_UID_SEPARATION` :

| Valeur | Comportement |
|---|---|
| `auto` (défaut) | active seulement quand la sandbox se résout en `basic` |
| `on` | toujours, comme couche indépendante supplémentaire |
| `off` | jamais |

L'abaissement est fait par le launcher, pas par le serveur. Le serveur reste
non root et sans aucune capability ; le launcher porte une seule capability
de fichier et bascule vers l'UID 1001 juste avant d'exécuter le plugin :

```bash
sudo setcap cap_setuid+ep /opt/evey/evey-plugin-launcher
```

Le launcher refuse tout uid ou gid cible inférieur à 1000. Quand il ne peut
pas abaisser (capability absente, `no_new_privs` positionné), un plugin en
mode `basic` est refusé, puisque l'uid est son seul rempart là ; sous
Landlock il journalise un avertissement et tourne sous l'uid du serveur.

!!! warning
    La séparation d'uid est incompatible avec `no_new_privs`. Si le serveur
    tourne sous systemd avec `NoNewPrivileges=yes`, la capability de fichier
    est ignorée à l'exec : les plugins sont refusés en mode `basic`, et sous
    Landlock la séparation d'uid se dégrade en même uid. Choisissez l'un ou
    l'autre.

Si le serveur tourne en root à la place (déconseillé), il abaisse lui-même
les plugins vers 1001 et la capability n'est pas nécessaire. Le tableau en
langage courant, scénario par scénario, est dans
[Sandbox des plugins](plugins/sandbox-deployment.md).

## Permissions des fichiers secrets

Reproduisez l'image : les dossiers porteurs de secrets sont réservés à leur
propriétaire, pour qu'un plugin abaissé vers l'UID 1001 ne puisse pas les
lire même sans Landlock.

```bash
sudo chown -R evey:evey /var/lib/evey
# le serveur possède plugins/ (il y extrait les installations) ; le groupe evey,
# dont evey-plugin fait partie, ne fait que lire et exécuter les fichiers installés :
sudo chmod 750 /var/lib/evey/plugins
# propriétaire seul sur les dossiers de secrets :
sudo chmod 700 /var/lib/evey/keys /var/lib/evey/logs /var/lib/evey/backups
# la clé de signature JWT elle-même (générée en 0600 au premier démarrage) :
sudo chmod 600 /var/lib/evey/jwt.key
```

Adaptez les chemins à l'endroit où pointent les variables `EVEY_*_DIR` et
`EVEY_JWT_PRIVATE_KEY_PATH`.

## Configurer l'environnement

Toute la configuration passe par des variables d'environnement `EVEY_*` ;
[Configuration](configuration.md) a le tableau complet. Points propres à une
installation sans conteneur :

- Pointez `EVEY_DB_HOST` et `EVEY_MEILI_URL` sur vos PostgreSQL et
  Meilisearch. `EVEY_DB_PASSWORD` ne doit pas être la valeur par défaut
  `evey`.
- Les chemins par défaut du code ont été choisis pour le conteneur :
  réglez `EVEY_LOG_DIR` (par défaut le chemin relatif `logs`),
  `EVEY_BACKUP_DIR` (`/data/backups`), `EVEY_TRANSCODING_CACHE_DIR`
  (`/data/transcode`), `EVEY_TRICKPLAY_CACHE_DIR` (`/cache/trickplay`) et
  `EVEY_JWT_PRIVATE_KEY_PATH` (`/data/jwt.key`) vers vos propres dossiers.
- Le serveur sert du HTTP en clair : mettez un proxy inverse devant pour le
  TLS, avec une **URL externe** en `https://` et le proxy dans **Proxys de
  confiance** (**Admin -> Accès distant** ; `EVEY_SERVER_BASE_URL` et
  `EVEY_TRUSTED_PROXIES` sont les replis). Voir
  [Proxy inverse](reverse-proxy.md).
- **Le serveur ouvre deux ports, et un seul peut être exposé.**
  `EVEY_SERVER_PORT` (`9090` par défaut) est l'écoute gRPC native ; l'API
  HTTP à laquelle parlent vos clients et votre proxy inverse est ce port plus
  un (`9091`). La barrière d'accès distant, qui décide si des clients hors de
  votre LAN peuvent se connecter, est câblée dans la chaîne HTTP et ne couvre
  pas l'écoute gRPC, qui se lie donc par défaut à la boucle locale seule
  (`EVEY_GRPC_BIND=127.0.0.1`). Si vous avez une raison de la publier,
  définissez `EVEY_GRPC_BIND=0.0.0.0` ; le serveur avertit au démarrage que
  la barrière ne s'y applique pas, et le pare-feu est alors à votre charge.
- Les migrations tournent au démarrage ; il n'y a pas d'étape de migration
  séparée.
- Les plafonds de ressources des plugins (`EVEY_PLUGINS_MEMORY_MB`,
  `_PIDS_MAX`, `_DISK_MAX_MB`, `_CPU_NICE`) et le mode de sandbox se
  modifient à chaud depuis **Admin -> Plugins -> Limites de ressources** ;
  les valeurs d'environnement sont les défauts initiaux.
- Le redémarrage du serveur depuis le panneau d'admin s'appuie sur le
  superviseur de l'image et se déclare indisponible hors conteneur :
  redémarrez le service vous-même.

## Vérifier la sandbox

Le serveur journalise le moteur qu'il a résolu. Après le premier démarrage de
plugin, cherchez :

- une fois, pour `auto` : `plugin sandbox auto-detection complete
  selected_mode=landlock` (ou `bubblewrap`, `basic`) ;
- à chaque démarrage de plugin : `plugin sandbox applied
  sandbox_mode=landlock plugin_id=...` ;
- l'avertissement que vous ne voulez pas voir quand vous attendiez
  Landlock : `plugin sandbox auto-detection fell back to basic mode: no
  per-path filesystem isolation available ...`, qui signifie que ni Landlock
  ni un `bwrap` fonctionnel n'ont été trouvés. Revérifiez la section sur les
  moteurs ci-dessus.

Confirmez le côté noyau indépendamment :

```bash
cat /sys/kernel/security/lsm              # liste "landlock"
grep Seccomp: /proc/<pid-du-plugin>/status   # 2 = un filtre seccomp est actif sur le plugin
```

Une ligne `Seccomp: 2` sur un processus de plugin en vie confirme que le
filtre noyau est installé. Avec `EVEY_PLUGINS_SANDBOX_MODE=landlock` et
Landlock non imposé, le plugin refuse de démarrer : c'est le contrat d'échec
fermé qui fonctionne.

Pour les sauvegardes, les mises à jour et les commandes de récupération, les
pages du conteneur s'appliquent telles quelles :
[Sauvegardes et mises à jour](backups.md), [Récupération](runbooks/recovery.md)
et la [Ligne de commande](runbooks/cli.md) décrivent le même binaire
`evey admin`.

<!-- sources
launcher lookup: internal/adapters/driven/pluginrunner/runner_landlock.go - launcherPath, launcherBinaryName, resolveAutoMode
shim path and modes: internal/adapters/driven/pluginrunner/runner.go - sandboxLibPath, sandboxModeStrict, pluginUID ; sandbox/sandbox.c ; Dockerfile (gcc line, /app/libevey-sandbox.so)
interpreters: internal/adapters/driven/pluginrunner/runner_start.go - knownInterpreters
basic sandbox, subprocess refusal, uid separation policy: internal/adapters/driven/pluginrunner/runner_sandbox.go - applyBasicSandbox, pluginDropArgs, uidSepApplies, WithUIDSeparation, effectiveMode ("auto-detection complete")
landlock probe: internal/adapters/driven/pluginrunner/landlock_probe_linux.go - landlockAvailable ; bwrap probe: sandbox_probe.go - ProbeSandbox
launcher: cmd/evey-plugin-launcher/main.go (-require-landlock, -uid, -uid-besteffort) ; cmd/evey-plugin-launcher/droppriv_linux.go (uid/gid >= 1000)
hardening: cmd/evey/hardening_linux.go - PR_SET_DUMPABLE
cli: cmd/evey/cli.go (version, completion bash, man)
ports and defaults: internal/config/config.go (EVEY_SERVER_PORT, EVEY_GRPC_BIND, EVEY_LOG_DIR, EVEY_BACKUP_DIR, EVEY_TRANSCODING_CACHE_DIR, EVEY_TRICKPLAY_CACHE_DIR, EVEY_JWT_PRIVATE_KEY_PATH, EVEY_PLUGINS_*, hardFail on password "evey") ; cmd/evey/main.go - httpPort = Port+1
pre-migration dump needs pg_dump: internal/adapters/driven/postgres/migrate_backup.go - dumpDatabase
restart RPC needs the supervisor: docker/entrypoint.sh (EVEY_SUPERVISED)
image permissions: Dockerfile (chmod 750 /data/plugins, chmod 700 keys/logs/backups, setcap)
labels: i18n/fr/admin.yaml - remoteAccess.externalUrl, remoteAccess.trustedProxies, plugins.resourceLimits.title
-->
