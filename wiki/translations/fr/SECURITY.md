---
source_sha: 79499ae596e00f8ccc2965d01e707a91ed16c8fef73d14c4f293e71464e05ab0
reviewed: true
---
# Sandbox des plugins et modèle de menace

Cette page décrit ce que fait le serveur pour contenir un plugin : les
vérifications à l'installation, les couches de protection à l'exécution, les
modes de sandbox, et ce que chacun ne peut toujours pas empêcher. Si vous
écrivez un plugin et voulez savoir ce que vous pouvez déclarer et ce qui sera
refusé, lisez [Permissions et slots](plugins/security-model.md).

## Modèle de menace

Les plugins tournent dans des processus séparés qui dialoguent avec le serveur
en gRPC sur des sockets Unix. Par défaut, le serveur ne fait confiance à aucun
code de plugin. Les risques contre lesquels il se protège, du plus grave au
moins grave :

1. **Usurpation** : un plugin se fait passer pour un autre afin d'utiliser ses
   permissions.
2. **Élévation de privilèges** : un plugin sort de sa sandbox et exécute des
   commandes avec les droits du serveur.
3. **Accès aux fichiers** : un plugin lit ou écrit hors de son propre dossier,
   en particulier les secrets du serveur.
4. **Exfiltration réseau** : un plugin envoie des données vers un hôte de son
   choix.
5. **Abus de ressources** : minage, fork bomb, épuisement de la mémoire ou du
   disque.
6. **Chaîne d'approvisionnement** : du code obscurci ou compilé que l'analyse
   ne sait pas lire.

## À l'installation

### Extraction du ZIP

Avant qu'aucun fichier ne soit écrit :

- au plus 5 000 entrées, 100 Mo par fichier, 500 Mo décompressés au total, et
  un taux de compression de 100:1 au-delà de 1 Mo décompressé (bombes de décompression) ;
- les entrées dont le chemin sort de l'archive (`..`) sont refusées ;
- les liens symboliques sont refusés ;
- les bits setuid, setgid et sticky sont retirés.

Source : `internal/adapters/driven/pluginrunner/zip.go`.

### Manifeste et permissions

Le manifeste déclare les permissions que demande le plugin. Une permission
inconnue fait refuser l'installation. Elles sont ensuite appliquées :

- **`subprocess`** autorise les processus enfants. Sans elle, le noyau refuse
  la création de processus (`-deny-fork`), et le filtre `LD_PRELOAD` refuse en
  plus `execve` aux plugins interprétés.
- **`native-binary`** autorise la livraison de code compilé ou d'archives, que
  l'analyse ne sait pas lire.
- **Les permissions d'API** (`api.library`, `api.user`, `api.metadata`,
  `media.read`, `media.write`, `notification.sender`...) sont vérifiées à
  chaque appel vers le serveur par
  [`PluginPermissionInterceptor`](../internal/adapters/driving/grpcapi/plugin_interceptor.go).

La liste complète des permissions se trouve dans
[Permissions et slots](plugins/security-model.md#permissions-catalog).

### Analyse statique

Chaque fichier du paquet est analysé par
[`pluginrunner.AnalyzePlugin`](../internal/adapters/driven/pluginrunner/analyzer.go),
après une passe de normalisation qui défait les obscurcissements courants
(alias, concaténations de chaînes, f-strings) :

| Catégorie | Gravité | Exemples |
|---|---|---|
| `shell_exec` | CRITIQUE | `os.system`, `subprocess.run`, `child_process.exec`, PHP `system()`/`shell_exec()`/`proc_open()`, `preg_replace /e`, backticks Ruby, `open('\|cmd')` |
| `network` | AVERTISSEMENT / CRITIQUE | `requests.get`, `fetch`, reverse shells et bind shells |
| `filesystem` | AVERTISSEMENT / CRITIQUE | `/etc/passwd`, `.ssh/`, `rm -rf`, opérations sur des liens symboliques |
| `obfuscation` | CRITIQUE | `eval`, décodage base64, séquences hexadécimales, `__builtins__`, `base64 -d \| bash` |
| `privilege` | CRITIQUE | `setuid`, `chmod 4777`, `sudo`, signal envoyé au PID 1 |
| `cryptomining` | CRITIQUE | `stratum+tcp`, `xmrig`, `cryptonight` |
| `runtime_install` | CRITIQUE | `pip install`, `npm install`, `curl ... \| sh` |
| `dynamic_import` | CRITIQUE | `__import__()`, `importlib.import_module` |
| `sandbox_escape` | CRITIQUE | `vm.runIn*`, `process.binding`, `new Function`, `__subclasses__()`, `__mro__` |
| `deserialization` | CRITIQUE / AVERTISSEMENT | `pickle.loads`, `marshal.loads`, `__reduce__` ; AVERTISSEMENT : `yaml.load()` sans Loader |
| `native_code` | CRITIQUE / AVERTISSEMENT | `ctypes.CDLL`, `ffi.Library` ; AVERTISSEMENT : `unsafe` en Go |
| `compiled_binary` | AVERTISSEMENT, puis BLOCAGE | `.so`, `.dll`, `.exe`, `.wasm`, `.jar` |
| `embedded_archive` | AVERTISSEMENT, puis BLOCAGE | `.zip`, `.tar.gz` |
| `mime_mismatch` | traité comme CRITIQUE | une extension d'image pour un autre contenu |
| `suspicious_url` | AVERTISSEMENT | un schéma autre que HTTP dans une URL |
| `unvendored_deps` | AVERTISSEMENT | un `requirements.txt` ou `package.json` sans modules embarqués |

Un résultat CRITIQUE ou un `mime_mismatch` fait refuser l'installation.
`compiled_binary` et `embedded_archive` la font refuser, sauf si le manifeste
déclare `native-binary`.

**Ce que l'analyse ne peut pas garantir.** Elle fonctionne par motifs. Elle
déjoue des classes d'obscurcissement connues (concaténation en plusieurs étapes
vers une fonction appelable, recherche d'une fonction native par son nom,
évasion par `Function.constructor`), chacune couverte par un cas de test de
régression dans `tests/fixtures/plugins/malicious/`, mais elle ne peut pas
suivre toutes les transformations faites à l'exécution (`strrev("metsys")`,
décodage d'une charge lue dans un fichier). Elle lit aussi les commentaires :
un commentaire `# pip install foo` déclenche `runtime_install`. **C'est la
sandbox d'exécution qui fait office de barrière, pas l'analyse** : un plugin
qui passerait un motif entre les mailles ne peut toujours ni lire les données
du serveur, ni faire un appel système interdit, ni dépasser ses limites de
ressources, ni appeler une méthode du serveur pour laquelle il n'a pas de
permission.

### Signature

Un paquet peut être signé avec cosign. `EVEY_PLUGINS_COSIGN_POLICY` détermine
l'effet d'une vérification qui échoue :

| Politique | Effet |
|---|---|
| `off` | La signature n'est pas vérifiée. |
| `warn` (par défaut) | Un échec est consigné et affiché à l'administrateur, sans bloquer. Sans clé publique configurée, rien n'est vérifié. |
| `enforce` | Un échec fait refuser l'installation ou la mise à jour. |

La clé publique est fournie par `EVEY_PLUGINS_COSIGN_PUBLIC_KEY`. Source :
`internal/adapters/driven/pluginrunner/cosign.go`.

### Aperçu par l'administrateur

`POST /api/v1/admin/plugins/preview` exécute l'analyse sans installer.
L'administrateur voit les permissions demandées, chaque résultat avec son
fichier, sa ligne et sa catégorie, les URL trouvées dans le code, et
l'empreinte SHA256 du paquet ; cette même empreinte est vérifiée au moment de
l'installation, si bien que le paquet analysé est bien celui qui est installé.
Une mise à jour affiche la différence de permissions, de slots et d'hôtes, et
toute nouvelle permission doit être explicitement acceptée.

L'administrateur est le point de décision pour les permissions élevées : Evey
expose la surface, l'administrateur accepte ou refuse.

## À l'exécution

### Fichiers

Un plugin voit son propre dossier et son dossier de socket, pas les données du
serveur. Le moyen dépend du [mode de sandbox](#sandbox-modes) : une liste
d'autorisations Landlock, ou une liste blanche de montages bubblewrap. En mode
`basic` sur un noyau sans Landlock, il n'y a pas de confinement par chemin : ce
sont alors l'utilisateur séparé et les permissions de fichiers qui protègent
les secrets du serveur (voir plus bas).

### Réseau

Un plugin n'a **aucun accès direct au réseau**, quoi qu'il déclare : toute
socket IP est refusée (`-deny-inet` en `landlock` et `basic`, un namespace
réseau en `bubblewrap` et `strict`). Un plugin doté de la permission `network`
appelle la méthode `Fetch` du serveur par sa socket Unix. Le serveur, dans
`internal/adapters/driving/pluginhost/fetch.go` :

- vérifie la cible dans la liste `external_fetch` du plugin (hôtes exacts,
  sans distinction de casse) ;
- n'autorise que `http` et `https`, et seulement `GET` et `HEAD` ;
- se connecte via un client protégé contre le SSRF (`internal/pkg/ssrf`), qui
  refuse les adresses privées, réservées et link-local après la résolution DNS,
  de sorte que le DNS rebinding et les redirections sont revérifiés ;
- n'ajoute aucun identifiant du serveur, et plafonne la taille de la réponse.

Cela ferme les voies que laissait l'accès direct (adresses IP littérales comme
`169.254.169.254`, résolveur propre au plugin). Reste une limite : le relais
contrôle *où* part un appel, pas *ce qu'il* transporte. Voir les
[Limites connues](#known-limits).

**Recommandé aux administrateurs système.** Le relais déplace la surface SSRF
vers le serveur. Une règle de sortie sur le conteneur `evey-server`,
indépendante du code, ajoute un second rempart : bloquer `169.254.0.0/16` et
restreindre le réseau interne aux points d'accès de la base et du moteur de
recherche.

### Appels système

Deux filtres seccomp refusent les mêmes appels système dangereux :

1. **Le filtre noyau**, installé par
   [`evey-plugin-launcher`](../cmd/evey-plugin-launcher) juste avant de lancer
   le plugin ([`seccomp_linux.go`](../cmd/evey-plugin-launcher/seccomp_linux.go)).
   Le plugin et ses enfants en héritent, et il **couvre aussi les binaires
   compilés statiques**. Un appel système d'une architecture différente tue le
   processus. Appliqué en `landlock` et `basic`.
2. **Le filtre `LD_PRELOAD`**, [`sandbox/sandbox.c`](../sandbox/sandbox.c),
   chargé dans les plugins interprétés qui ne déclarent pas `subprocess`. Il
   refuse en plus `execve`/`execveat` et interdit le vidage mémoire du plugin.
   Appliqué en `landlock`, `basic` et `strict`. Si la bibliothèque manque, un
   tel plugin n'est pas démarré (`plugin.error`).

Appels système refusés :

| Catégorie | Appels système |
|---|---|
| Débogage | `ptrace`, `process_vm_readv`, `process_vm_writev` |
| Signaux | `tgkill`, `rt_sigqueueinfo`, `rt_tgsigqueueinfo` vers tout processus autre que le plugin lui-même (les deux filtres) ; `kill` de même dans le filtre noyau, et vers toute cible dans le filtre `LD_PRELOAD` ; `tkill` dans le filtre noyau (le filtre `LD_PRELOAD` l'autorise vers le plugin lui-même) ; `pidfd_send_signal`, `pidfd_getfd` (vole un descripteur de fichier). Un runtime qui se renvoie un signal ou préempte ses propres threads continue de fonctionner. Avec Landlock ABI 6 ou plus, un plugin ne peut pas non plus envoyer de signal à un processus hors de sa sandbox |
| Montages | `mount`, `umount2`, `pivot_root`, `chroot` |
| Namespaces | `unshare`, `setns` |
| Modules noyau | `init_module`, `finit_module`, `delete_module` |
| BPF, perf | `bpf`, `perf_event_open` |
| Entrées-sorties matérielles | `ioperm`, `iopl` (x86-64 seulement, où ils existent) |
| Swap | `swapon`, `swapoff` |
| Contrôle du système | `syslog`, `reboot`, `settimeofday`, `clock_settime`, `clock_adjtime`, `adjtimex` |
| Kexec | `kexec_load`, `kexec_file_load` |
| Primitives d'exploitation du noyau | `io_uring_setup`, `io_uring_enter`, `io_uring_register`, `userfaultfd` |
| Identité | `setuid`, `setgid`, `setreuid`, `setregid`, `setresuid`, `setresgid`, `capset` |
| Trousseau de clés | `add_key`, `request_key`, `keyctl` |

Pour un plugin sans `subprocess`, le noyau refuse aussi la création de
processus (`fork`, `vfork`, `clone` sans `CLONE_THREAD`) ; les threads
fonctionnent toujours.

Il s'agit d'une **liste de refus** : `open`, `read`, `write`, `mmap`, `socket`
et `connect` en Unix restent autorisés. Les chemins sont confinés par la couche
fichiers, pas par seccomp. En `bubblewrap`, le lanceur n'est pas utilisé : un
binaire statique n'y a aucun filtre seccomp, et il est contenu par les
namespaces et la liste blanche de montages.

### Un utilisateur séparé

Un plugin peut tourner sous l'utilisateur 1001 (`evey-plugin`), distinct du
serveur (1000), de sorte que les permissions de fichiers suffisent à lui
rendre illisibles les secrets du serveur. `EVEY_PLUGINS_UID_SEPARATION` le
règle :

- `auto` (par défaut) : seulement quand la sandbox se résout en `basic`, seul
  cas où les permissions de fichiers sont l'unique rempart autour des
  secrets ;
- `on` : toujours, comme couche supplémentaire ;
- `off` : jamais.

Le changement d'utilisateur est fait par le lanceur, qui détient
`cap_setuid` ; le serveur, lui, tourne sans privilège ni capacité. Le lanceur
refuse toute cible inférieure à 1000. S'il ne peut pas changer d'utilisateur,
un plugin en `basic` est refusé (l'utilisateur y est l'unique rempart), alors
qu'en `landlock` il tourne sous l'utilisateur du serveur, avec un
avertissement. En `bubblewrap` et `strict`, c'est le namespace utilisateur qui
joue ce rôle.

Quand la séparation est active, la clé de signature (`0600`) et les
sauvegardes (dossier `0700`, fichiers `0600`) sont réservées à leur
propriétaire à l'exécution, ce qui tient même sur un `/data` monté depuis
l'hôte. Tous les plugins partagent l'utilisateur 1001 : ils sont isolés les
uns des autres par la couche fichiers, pas par l'utilisateur.

### Authentification de la socket

Chaque plugin a sa propre socket, `/tmp/evey/plugins/<plugin_id>/host.sock`,
servie par son propre serveur gRPC. À chaque appel, le serveur lit le PID de
l'appelant auprès du noyau (`SO_PEERCRED`, impossible à falsifier depuis
l'espace utilisateur) et le compare au PID enregistré au démarrage du plugin.
L'identité du plugin vient de la socket, jamais d'un en-tête. Un plugin ne
peut donc ni se faire passer pour un autre, ni appeler la socket d'un autre
plugin, ni faire passer ses appels par un processus enfant.

### Limites de ressources

Un moniteur en espace utilisateur (les cgroups ne sont pas disponibles dans
un conteneur non privilégié) lit `/proc/<pid>` environ toutes les 2 secondes
et arrête un plugin qui dépasse une limite :

- mémoire (RSS), 256 Mio par défaut ;
- processus et threads, 64 par défaut ;
- disque occupé par son dossier, 100 Mio par défaut, vérifié toutes les 30
  secondes ;
- priorité CPU, `nice` 10 par défaut, appliquée au démarrage.

Les limites se règlent par plugin dans **Admin → Plugins**. Voir
l'[ADR-0013](adr/0013-userspace-plugin-resource-monitor.md).

### Isolation de l'interface

L'interface d'un plugin s'exécute dans une `<iframe sandbox="allow-scripts">`,
sans accès à la même origine, sans formulaires ni fenêtres surgissantes, sous
une Content-Security-Policy qui n'autorise les connexions que vers ses hôtes
`external_fetch`. Le pont limite ce qu'elle peut demander à la page : `toast`
au plus 3 fois par seconde, 200 caractères, sans HTML ; `navigate` n'accepte
que `/home`, `/library`, `/media`, `/player`, `/settings` et `/search` ; `resize` entre 50
et 800 px. Les slots sont vérifiés à l'installation, puis de nouveau à
l'affichage.

### Durcissement du serveur

Au démarrage, le serveur interdit son propre vidage mémoire
(`PR_SET_DUMPABLE=0`, [`hardening_linux.go`](../cmd/evey/hardening_linux.go)) :
un processus tournant sous le même utilisateur ne peut pas lire sa mémoire ni
son environnement via `/proc`, ce qui protège la clé de signature en mémoire
ainsi que les mots de passe de la base et du moteur de recherche, même sur un
noyau sans Landlock.

### Notification des plantages

Le runner publie un événement `plugin.error` quand la bibliothèque de sandbox
manque pour un plugin qui en a besoin, ou quand un plugin se termine en
erreur. Le service de notification le transmet à l'administrateur par les
canaux configurés. Un PID qui ne correspond pas sur une socket est consigné
comme avertissement.

## Modes de sandbox

Le mode détermine la façon dont les fichiers et le réseau sont confinés.
`EVEY_PLUGINS_SANDBOX_MODE` fixe la valeur initiale (`auto` par défaut) ; elle
se modifie ensuite dans **Admin → Plugins**. Le runner l'applique dans
[`applySandbox`](../internal/adapters/driven/pluginrunner/runner_sandbox.go). Dans
tous les modes : aucun accès IP direct, le moniteur de ressources, et
l'authentification de la socket.

### `auto` (par défaut)

Se résout, une fois, au premier démarrage d'un plugin, vers le mode le plus
robuste que la machine permet : **landlock**, puis **bubblewrap**, puis
**basic**. Landlock passe en premier parce qu'il ne demande aucun privilège et
fonctionne sous le profil de sécurité par défaut de Docker. bubblewrap n'est
choisi que si Landlock est absent et qu'une sandbox de test se construit
réellement
([`ProbeSandbox`](../internal/adapters/driven/pluginrunner/sandbox_probe.go)),
ce qui revient en pratique à une installation sans conteneur ou à un
conteneur privilégié. Le mode retenu est consigné dans les logs et affiché sur
la ligne **Sandbox de l'hôte** du **Tableau de bord sécurité**, dans
**Admin → Plugins**.

### `landlock`

- **Fichiers :** une liste d'autorisations Landlock appliquée par le lanceur.
  Dossiers système (`/usr`, `/lib`, `/lib64`, `/bin`, `/sbin`, `/etc`, `/app`)
  en lecture seule ; dans `/dev`, seulement `null`, `zero`, `full`, `random` et
  `urandom`, pour que les nœuds des cartes graphiques et le terminal du serveur
  restent hors d'atteinte ; dossier du plugin et dossier de sa socket en
  lecture-écriture ; tout le reste, y compris `/data` et les autres plugins,
  refusé. Un plugin qui a reçu `hardware.gpu` obtient son nœud de rendu, seule
  exception.
- **Réseau :** `-restrict-net` et `-deny-inet` ; sortie uniquement via
  `Fetch`.
- **Appels système :** le filtre noyau et le filtre `LD_PRELOAD`.
- **Prérequis :** un noyau 5.13 ou plus récent avec Landlock dans la liste
  des LSM actifs, et le lanceur (c'est le cas avec l'image officielle sur une
  machine récente). Un mode `landlock` explicite refuse de démarrer un plugin
  qu'il ne peut pas confiner.

### `bubblewrap`

- **Fichiers et processus :** six namespaces isolés (`user`, `pid`, `uts`,
  `ipc`, `cgroup`, `net`) et une liste blanche de montages ; rien d'autre
  n'existe à l'intérieur.

  | Chemin | Accès |
  |---|---|
  | `/usr`, `/bin`, `/sbin`, `/lib`, `/lib64` | lecture seule |
  | `/etc/ssl`, `/etc/ca-certificates`, `/etc/resolv.conf`, `/etc/nsswitch.conf`, `/etc/{alpine,os}-release` | lecture seule (jamais `/etc` en entier : ni `passwd`, ni `shadow`, ni `ssh`) |
  | le dossier du plugin, le dossier de sa socket | lecture-écriture |
  | `/proc` | son propre montage |
  | `/dev` | un jeu synthétique minimal (null, zero, random, urandom, tty, pts) |
  | `/tmp` | un tmpfs vide |

  `/data`, `/var`, `/home`, `/root`, `/run` et les dossiers des autres
  plugins n'existent pas à l'intérieur. Un plugin témoin
  (`tests/fixtures/plugins/canary-sandbox-runtime/`) vérifie cette disposition
  sur une installation en fonctionnement. Source : `buildBwrapCmd` dans
  `internal/adapters/driven/pluginrunner/runner_bwrap.go`.
- **Réseau :** le namespace réseau, pour tous les plugins.
- **Appels système :** aucun filtre seccomp ; ce sont les namespaces qui
  isolent.
- **Prérequis :** le binaire `bwrap` et les appels système `mount`/`unshare`.
  Dans Docker, cela exige un profil de sécurité assoupli : c'est donc, en
  pratique, un mode pour les installations sans conteneur.

### `strict`

`bubblewrap`, plus le filtre `LD_PRELOAD` (plugins interprétés uniquement).

### `basic`

Le dernier recours d'`auto` : l'utilisateur séparé, le filtre noyau, le filtre
`LD_PRELOAD`, `-deny-inet`, et Landlock si le noyau le propose. Sur un noyau
sans Landlock, il n'y a pas de confinement par chemin : un plugin peut lire les
fichiers lisibles par tous qui ne sont pas sensibles, et voir la liste des
processus. Les secrets du serveur restent protégés par l'utilisateur séparé et
`PR_SET_DUMPABLE`, le réseau est coupé, et les appels système dangereux sont
refusés, binaires compilés compris.

### Récapitulatif

| Couche | landlock | basic | bubblewrap | strict |
|---|---|---|---|---|
| Fichiers | liste d'autorisations Landlock | aucune sans Landlock (permissions de fichiers seules) | liste blanche de montages | liste blanche de montages |
| Réseau | `-restrict-net` + `-deny-inet` | `-deny-inet` | namespace réseau | namespace réseau |
| Seccomp noyau (binaires statiques compris) | oui | oui | non | non |
| Seccomp `LD_PRELOAD` (plugins interprétés sans `subprocess`) | oui | oui | non | oui |
| Utilisateur séparé | avec `on` | oui (`auto`) | namespace utilisateur | namespace utilisateur |
| Limites de ressources | oui | oui | oui | oui |

| Déploiement | Ce que vous obtenez |
|---|---|
| Docker sur un noyau récent (`auto` donne `landlock`) | Confinement par chemin, aucun accès réseau direct, appels système dangereux refusés, binaires statiques compris. C'est le cas normal. |
| Docker sur un ancien noyau sans Landlock, par exemple un vieux NAS (`auto` donne `basic`) | Secrets protégés et réseau coupé, mais fichiers lisibles par tous visibles. Voir [Mise en place de la sandbox des plugins](plugins/sandbox-deployment.md). |
| Sans conteneur (`auto` donne `bubblewrap`) | L'isolation des fichiers et des processus la plus forte, sans filtre seccomp pour les binaires statiques. |

## Configuration

| Variable | Défaut | Effet |
|---|---|---|
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto`, `landlock`, `bubblewrap`, `strict` ou `basic`. Modifiable aussi dans **Admin → Plugins**. |
| `EVEY_PLUGINS_UID_SEPARATION` | `auto` | `auto`, `on` ou `off`. Incompatible avec `no_new_privs` sur le conteneur. |
| `EVEY_PLUGINS_COSIGN_POLICY` | `warn` | `off`, `warn` ou `enforce`. |
| `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` | aucune | La clé avec laquelle les signatures sont vérifiées. |

L'image officielle embarque `evey-plugin-launcher` (Landlock et le filtre
noyau) et `bubblewrap`. bubblewrap nécessite aussi les namespaces utilisateur
non privilégiés sur la machine hôte. Quelle configuration pour quelle machine,
en termes simples :
[Mise en place de la sandbox des plugins](plugins/sandbox-deployment.md).

## Limites connues

| Limite | Conséquence | Ce qui la restreint aujourd'hui |
|---|---|---|
| Pas de confinement par chemin en `basic` sur un noyau sans Landlock | Un plugin peut lire les fichiers non sensibles lisibles par tous, et les dossiers des autres plugins | Secrets protégés par l'utilisateur séparé et `PR_SET_DUMPABLE` ; `auto` préfère Landlock ou bubblewrap |
| Tous les plugins partagent l'utilisateur 1001 | Un plugin peut envoyer des signaux à ses voisins (pas au serveur) | `PR_SET_DUMPABLE` empêche de lire les autres processus via `/proc` ; Landlock et bubblewrap isolent les dossiers des plugins |
| Seccomp est une liste de refus | Les appels sur les fichiers et les sockets Unix restent permis | Les chemins sont confinés par Landlock ou bubblewrap ; les sockets IP sont refusées |
| Le relais déplace la surface SSRF vers le serveur | Une faille dans sa validation pourrait atteindre une adresse interne | Liste d'autorisations et client anti-SSRF en Go ; une règle de sortie sur le conteneur comme couche indépendante |
| Le relais contrôle où part un appel, pas ce qu'il transporte | Des données peuvent partir vers un hôte de la liste du plugin | N'autoriser que des hôtes auxquels vous confiez ces données |
| L'analyse fonctionne par motifs | Un obscurcissement délibéré peut cacher du code | L'aperçu par l'administrateur ; la barrière est la sandbox, pas l'analyse |
| `LD_PRELOAD` ne couvre que les plugins interprétés dynamiques | Un binaire statique en `basic` sans le lanceur y échappe | L'image officielle embarque toujours le lanceur, dont le filtre noyau couvre les binaires statiques |

## Exposition réseau (hors modèle de menace des plugins)

Deux règles déterminent qui peut atteindre le serveur :

- **Seul le port HTTP est destiné à être publié.** Le port gRPC
  (`EVEY_SERVER_PORT`, 9090 par défaut) est un transport interne dont le seul
  client est la passerelle HTTP du même processus ; l'API HTTP est sur ce port
  plus un. gRPC n'écoute par défaut que sur la boucle locale
  (`EVEY_GRPC_BIND=127.0.0.1`), car le contrôle d'accès distant est un
  middleware HTTP qui ne le couvre pas. Élargir cette écoute affiche un
  avertissement ; la filtrer devient alors la responsabilité de
  l'administrateur système.
- **Le contrôle d'accès distant se fonde sur la seule adresse client validée à
  la frontière des proxys de confiance**, jamais sur un en-tête de la requête.
  Un client ne peut pas se faire passer pour `127.0.0.1` via
  `X-Forwarded-For`, et une requête qui n'a pas traversé cette frontière est
  considérée comme distante. C'est un contrôle de déploiement, pas une
  authentification.

Voir [Accès distant et HTTPS](administration/remote-access.md) et la
[Référence de configuration](environment/server.md#remote-access).

## Ce que les logs peuvent contenir

Le log du serveur alimente **Logs & Debug**, les rapports de bug et les
sauvegardes : les identifiants y sont donc masqués au moment de l'écriture
(`internal/pkg/logger/redact.go`). Un attribut dont la clé désigne un secret
(`authorization`, `token`, `api_key`, `password`, `pin`, `secret`...) est
remplacé en entier, et toute chaîne contenant un jeton bearer, un JWT ou une
paire `secret=valeur` est masquée. Les routes de consultation des logs
appliquent la même règle aux lignes plus anciennes. Côté code : ne jamais
journaliser une valeur non secrète sous un nom de clé secret (elle serait
masquée), et ajouter tout nouveau type d'identifiant à cette liste avant de le
journaliser.

## Signaler une vulnérabilité

Signalez-la en privé, jamais dans une issue publique :
`security@eveyproject.org`, l'adresse de la
[politique de sécurité](../SECURITY.md) du dépôt. Elle couvre
l'authentification des plugins, les permissions et les évasions de sandbox
comme tout le reste. Ne testez pas sur des instances d'Evey qui ne sont pas
les vôtres.
