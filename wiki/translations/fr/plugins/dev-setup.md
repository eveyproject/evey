---
source_sha: ac4eee9f506439ea9c92e082e310a4812d9bba43666dc155e80e166aa5a52cc6
reviewed: true
---
# Préparer son environnement de développement de plugins

Le parcours complet pour quelqu'un qui vient de cloner Evey et veut écrire,
tester et publier un plugin ou un thème. Choisissez l'environnement d'exécution
qui correspond au langage de votre plugin ; le reste est commun.

## 1. Prérequis

Evey fait tourner le processus hôte (Go) et reçoit les plugins sous forme de ZIP.
L'environnement d'exécution du plugin dépend de ce qu'appelle votre code.

### Toujours nécessaires

| Outil | Pourquoi | Installation |
|---|---|---|
| Go ≥ 1.27 | Compiler l'hôte Evey | `https://go.dev/dl/` |
| Docker + Docker Compose | La pile de développement par défaut (PostgreSQL, Meilisearch, serveur, web) | https://docs.docker.com/engine/install/ |
| `bubblewrap` (`bwrap`) | Sandbox des processus de plugins, nécessaire pour les modes `bubblewrap` et `strict` | `apt install bubblewrap` (Debian/Ubuntu), `dnf install bubblewrap` (Fedora), `pacman -S bubblewrap` (Arch). macOS : installez Linux dans une VM, bwrap n'existe que sous Linux. |

Le mode de sandbox `basic` fonctionne sans bwrap, mais ne devrait jamais servir
hors d'expériences locales jetables : abandonnez-le dès que possible.

### Selon le langage

Prenez ce que votre plugin utilise vraiment ; vous n'avez pas besoin de tout.

| Langage du plugin | Outils | Remarques |
|---|---|---|
| Python 3 | `python3`, `pip` | Utilisez `pip install --user` pour garder la machine propre. Les stubs du SDK sont dans `sdk/python/`. |
| Node.js | Node 20+, `npm` ou `pnpm` | Le SDK charge `plugin_api.proto` à l'exécution via `@grpc/proto-loader`. |
| Go | Go ≥ 1.21 | Pas de stubs fournis : générez-les avec `protoc-gen-go` + `protoc-gen-go-grpc` ; un binaire Go compilé demande aussi la permission `native-binary`. |
| Dart / Java / PHP / Ruby / Swift | La chaîne d'outils du langage | Les stubs sont pré-générés dans `sdk/<langage>/`. |
| Thème en CSS seul | Rien de plus | Un thème est un ZIP avec `kind: theme` et un `theme.css`. |
| JS de thème | Rien de plus (l'hôte fait tourner votre JS dans une `iframe` isolée avec `sandbox="allow-scripts"` et une CSP verrouillée) | L'analyse statique refuse les cookies, le stockage, les appels réseau et l'accès à la page hôte (`document.cookie`, `localStorage`, `fetch(`, `window.parent`...) : voir [Thèmes](../themes.md#what-the-scanner-refuses). |

### Générer des stubs pour un autre langage

Des stubs pré-générés sont fournis pour les langages ci-dessus, mais le contrat
gRPC des plugins n'est qu'un fichier proto : vous pouvez viser **n'importe quel
langage que `protoc` prend en charge** (Rust, C#, Kotlin, Elixir, …). La source
unique est :

```
proto/plugin/v1/plugin_api.proto
```

Générez avec `protoc` (ou `buf`), en y branchant le plugin gRPC de votre langage :

```bash
# forme générale : les options --*_out exactes dépendent du langage et du plugin
protoc --proto_path=proto \
       --<lang>_out=OUT_DIR --grpc_<lang>_out=OUT_DIR \
       proto/plugin/v1/plugin_api.proto
```

Les recettes des langages fournis (quel plugin gRPC chacun demande :
`grpc_php_plugin`, `protoc-gen-grpc-java`, `protoc-gen-grpc-swift`, …) se trouvent
dans la cible `plugin-sdk` du `Makefile` racine et dans chaque
`sdk/<langage>/README.md` (par exemple `sdk/php/README.md`). `make plugin-sdk`
régénère les stubs fournis.

Livrez les stubs générés dans le ZIP de votre plugin : l'hôte ne copie
automatiquement que les stubs des langages qu'il fournit (`sdk/<langage>/`).

### Facultatifs mais recommandés

| Outil | Pourquoi |
|---|---|
| `cosign` (Sigstore) | Signer le ZIP de votre plugin. `EVEY_PLUGINS_COSIGN_POLICY` vaut `warn` par défaut (pas `off`) : avec `warn` et sans clé publique configurée, la vérification ne fait rien (un avertissement est journalisé, rien n'est bloqué) ; réglez `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` pour vérifier réellement les signatures, et la politique sur `enforce` pour refuser les envois non signés (`enforce` sans clé publique arrête le serveur au démarrage plutôt que d'accepter tous les plugins sans vérification). https://docs.sigstore.dev/cosign/installation/ |
| `pre-commit` | Repérer les problèmes évidents avant de soumettre un plugin. |
| `golangci-lint` | Nécessaire si votre plugin est en Go et que vous voulez les mêmes contrôles que l'hôte. |

## 2. Lancer Evey en local

```sh
git clone git@github.com:Evey-project/evey.git
cd evey
cp .env.dev.example .env.dev   # une configuration locale raisonnable
make dev                        # docker compose up : postgres, meilisearch, serveur (Air), web (Vite)
```

Une fois le serveur en bonne santé, ouvrez https://evey.test (servi par Caddy). L'assistant
de configuration crée le compte administrateur au premier démarrage, comme
décrit dans [Première configuration](../getting-started/first-setup.md).

Si `make dev` échoue sur `bwrap` :

- vérifiez que `which bwrap` renvoie un chemin sur la machine hôte ;
- dans Docker, le conteneur de développement contient le binaire bwrap ; si vous
  avez modifié la pile Compose, montez-le, ou réglez
  `EVEY_PLUGINS_SANDBOX_MODE=basic` *pour le seul conteneur de développement*.

## 3. Créer son premier plugin

Les exemples sont dans `examples/plugins/` : des points de départ en Node.js et
Python (`nodejs-example`, `python-example`), une source de métadonnées
(`metadata-example`), un récepteur de webhooks (`webhook-receiver`), un widget
d'accueil (`weather`), la configuration d'un plugin (`config-demo`) et deux thèmes
(`theme-dark-blue`, `open-workshop`). Prenez le plus proche de votre besoin et
copiez-le dans un dossier de travail. Pour la persistance et les permissions,
voyez **`examples/plugins/data-demo`** : il utilise le stockage de données propre
au plugin et demande les permissions `public` / `prefs` et l'emplacement
`admin.nav`.

```sh
cp -r examples/plugins/nodejs-example ~/my-first-plugin
cd ~/my-first-plugin
```

Modifiez `plugin.yaml` :

- `name` : en kebab-case, unique ;
- `version` : semver, à augmenter à chaque publication ;
- `kind` : `plugin` (partie serveur requise) ou `theme` (interface seulement) ;
- `language` : `python` | `nodejs` | `go` | `php` | `ruby` | `lua` | `bash` |
  `sh` (l'identifiant du manifeste, pas le nom du binaire de l'interpréteur ;
  `python3`/`node` ne sont pas des valeurs valides de `language`, voir
  `entrypoint` ci-dessous) ;
- `entrypoint` : la ligne de commande qu'exécute l'environnement d'exécution ;
- `permissions` : déclarez chaque API de l'hôte que vous appelez. L'installeur
  refuse les appels non déclarés. La liste exhaustive est dans le
  [catalogue des permissions](security-model.md#permissions-catalog) ;
- `external_fetch` : quand vous devez parler à une API tierce. Les entrées sont
  des noms d'hôte seuls (`api.example.com`) : aucun schéma nécessaire ni utilisé
  (un préfixe `https://` est lu et seul le nom d'hôte est gardé), pas de jokers
  (`*.example.com` est refusé à l'installation :
  `external_fetch entry %q is a wildcard - use explicit hostnames`), pas
  d'adresses IP littérales (aucune, pas seulement celles de la RFC 1918). Le
  validateur est dans `internal/adapters/driven/pluginrunner/manifest.go`
  (`validateExternalFetch`) ; ses tests sont dans
  `internal/adapters/driven/pluginrunner/manifest_test.go` : lancez
  `go test ./internal/adapters/driven/pluginrunner/...` pour vérifier votre
  valeur.

Module d'interface (facultatif pour `kind: plugin`, interdit pour `kind: theme`) :

```yaml title="plugin.yaml"
frontend:
  - slot: home.section
    src: dist/index.js
```

L'emplacement doit exister dans `internal/domain/plugin/slots.go` (`home.section`,
`home.hero`, `library.sidebar`, `media.detail.sidebar`, `player.overlay`,
`sidebar.bottom`, `settings.user.appearance`, et `admin.nav`, le seul emplacement
contrôlé de la zone d'administration, une entrée de menu qui ouvre la page isolée
de votre plugin). Tout le reste (injection dans `admin.*`, `login.*`, `setup.*`,
`profile.security.*`) est refusé à l'installation.

**Stocker des données.** Votre partie serveur dispose d'un stockage clé/valeur
privé par la socket de l'hôte (`SetData`/`GetData`/`DeleteData`/`ListDataKeys`),
sans permission, effacé automatiquement à la désinstallation. Pour des préférences
par utilisateur depuis votre iframe, demandez la permission `prefs` ; pour un
accès en lecture seule à des informations non sensibles sur les médias (titre,
affiche), demandez `public`. Voir la [référence de l'API](api-reference.md#permissions).
Les portées d'administration, d'authentification, système et d'écriture
générique ne peuvent jamais être demandées.

## 4. La boucle d'installation

```sh
# zipper votre plugin
cd ~/my-first-plugin
zip -r my-first-plugin.zip . -x ".*" "node_modules/*" "*.pyc"

# l'envoyer par l'interface d'administration
open https://evey.test/admin/plugins
# ou par l'API
curl -F "plugin=@my-first-plugin.zip" \
     -H "Authorization: Bearer $EVEY_TOKEN" \
     https://evey.test/api/v1/admin/plugins/upload
# une signature va dans un second champ : -F "sig=@my-first-plugin.zip.sig"
# une mise à jour qui demande de nouvelles permissions : -F "accept_permission_changes=true"
```

L'installeur procède dans cet ordre, et chaque étape peut refuser
l'installation :

1. **Validation du manifeste** : `plugin.yaml` lu directement dans le ZIP (avant
   les contrôles sur le ZIP lui-même, plus bas), vérifié selon le schéma. Liste
   des emplacements. Liste des permissions et capacités. Analyse de
   `external_fetch` (refuse les jokers et toute adresse IP littérale ; un préfixe
   de schéma comme `http://` est accepté mais ignoré, seul le nom d'hôte est
   gardé). L'environnement d'exécution du `language` déclaré doit exister dans
   l'image.
2. **Élévation de permissions** (mise à jour seulement) : l'installeur compare le
   nouveau manifeste à la version précédente. Toute permission, capacité,
   emplacement ou hôte `external_fetch` nouvellement demandé bloque l'installation
   avec une erreur que l'appelant doit explicitement passer outre
   (`accept_permission_changes: true` depuis l'interface). Une première
   installation saute cette étape : il n'y a rien à comparer.
3. **Signature cosign** : vérifiée quand un vérificateur de signature est
   configuré ; un échec ne bloque l'installation que si
   `EVEY_PLUGINS_COSIGN_POLICY=enforce`. Voir la section 6.
4. **Extraction du ZIP** : extension, taille totale ≤ 500 Mo, taille par entrée
   ≤ 100 Mo, taux de compression ≤ 100:1, pas de liens symboliques, pas
   d'entrées qui remontent l'arborescence ; les fichiers arrivent dans un dossier
   de préparation (une mise à jour ne touche jamais la version en service tant
   que toutes les étapes suivantes ne sont pas passées).
5. **Assainisseur CSS** (thèmes seulement) : retire `@import`, bloque les `url()`
   autres que `data:`, bloque les superpositions plein écran, bloque les
   sélecteurs qui visent `[type=password]` et `.admin-*`, plafonne `z-index` à
   999.
6. **Analyse du JS de thème** (thèmes seulement) : refuse toute référence à
   `document.cookie`, `localStorage`, `fetch`, `eval`, `globalThis`, aux accès par
   crochets sur les globales de l'hôte, à `constructor.constructor`, etc. Un thème
   ne doit utiliser que l'API `evey.theme.*`.
7. **Analyse statique** : parcourt chaque fichier texte à la recherche
   d'exécution de shell, de reverse shell, d'eval, de décodage base64 suivi
   d'exécution, de motifs d'obfuscation, de commandes d'installation à
   l'exécution, d'élévation de privilèges. Une trouvaille critique refuse
   l'installation. Les trouvailles de niveau avertissement (désérialisation,
   obfuscation, code natif) deviennent un refus sauf si la permission
   correspondante est déclarée.
8. **Enregistrement** : met le dossier de préparation en place, écrit la ligne
   dans PostgreSQL, et lance éventuellement l'environnement d'exécution.

Si une étape échoue, l'interface d'administration affiche la raison exacte. Les
journaux du serveur (`docker compose logs evey`) contiennent la trace complète.

## 5. Itérer

Rechargez votre plugin à chaud pendant le développement :

```sh
# modifier le code → re-zipper → renvoyer par l'interface (bouton Mettre à jour)
# ou, plus rapide : déposer le fichier directement dans le dossier du plugin, dans le conteneur
docker compose exec evey ls /data/plugins/<plugin-id>/
```

Un envoi n'est accepté comme mise à jour que si sa `version` est supérieure à
celle installée : augmentez-la à chaque itération (une version égale ou
inférieure est refusée), ou désinstallez d'abord. Si vous ajoutez une nouvelle permission, vous passerez par la procédure
d'élévation : c'est voulu, pas un bug.

Les journaux du processus du plugin arrivent :

- dans `/admin/plugins/<id>`, les N dernières lignes dans l'interface ;
- dans `docker compose logs evey | grep <plugin-id>`, le flux complet.

## 6. Signer son plugin (facultatif en développement, recommandé pour le distribuer)

!!! info "Ce que prouve une signature"
    Une signature atteste que le ZIP installé par l'administrateur est, octet
    pour octet, celui que **vous** avez publié, et qu'il a été signé avec la clé
    à laquelle il fait confiance. Elle ne dit rien de ce que fait le plugin :
    les permissions, les hôtes et l'analyse de l'aperçu s'appliquent toujours.
    Evey la vérifie **hors ligne et dans son propre processus** (ECDSA P-256 sur
    le SHA-256 du ZIP, le format que produit `cosign sign-blob --key`) : pas de
    binaire `cosign` sur le serveur, pas de réseau, pas de journal de
    transparence.

Il vous faut [cosign](https://docs.sigstore.dev/cosign/installation/) sur
**votre** machine uniquement.

**1. Une seule fois : générer une paire de clés.**

```sh
# choisissez une vraie phrase secrète ; cosign.key reste privée, ne la commitez jamais
COSIGN_PASSWORD="your-passphrase" cosign generate-key-pair
# -> cosign.key (privée) et cosign.pub (le fichier que vous distribuez)
```

**2. À chaque version : zipper, puis signer.** Le manifeste est à la racine du
ZIP. Signez hors ligne, comme le serveur vérifie :

```sh
cd my-first-plugin && zip -r ../my-first-plugin.zip . && cd ..

COSIGN_PASSWORD="your-passphrase" cosign sign-blob \
  --key cosign.key --tlog-upload=false --yes \
  --output-signature my-first-plugin.zip.sig \
  my-first-plugin.zip
```

**3. Vérifier avant d'expédier**, avec la même vérification que le serveur :

```sh
cosign verify-blob --key cosign.pub --signature my-first-plugin.zip.sig \
  --insecure-ignore-tlog my-first-plugin.zip      # -> Verified OK
```

**4. Publier trois fichiers ensemble** : `my-first-plugin.zip`,
`my-first-plugin.zip.sig` et `cosign.pub`. Indiquez l'empreinte de la clé
publique à un endroit où vos utilisateurs peuvent la comparer (README, notes de
version), pour qu'ils sachent que `cosign.pub` n'a pas été remplacé.

**Sur l'instance qui l'installe :**

```sh
# placer la clé publique à un endroit lisible par le serveur
sudo cp cosign.pub /etc/evey/cosign.pub

export EVEY_PLUGINS_COSIGN_PUBLIC_KEY=/etc/evey/cosign.pub
export EVEY_PLUGINS_COSIGN_POLICY=warn   # ou "enforce" pour refuser ce qui ne se vérifie pas
docker compose restart evey
```

Ensuite, dans **Admin → Plugins**, choisissez le ZIP puis, dans le champ
**Fichier signature**, son `.zip.sig`. Par l'API, la signature est le champ
facultatif `sig` de l'envoi (section 4). L'aperçu affiche **Signé** ou une
signature invalide, et la carte du plugin dans le tableau de bord de sécurité
conserve le résultat.

Les pièges habituels :

- **Toute modification du ZIP casse la signature**, même un simple re-zip des
  mêmes fichiers. Signez exactement le fichier que vous distribuez, et
  resignez à chaque version.
- **Seules les clés ECDSA P-256 sont acceptées**, ce que produit `cosign
  generate-key-pair`. La signature sans clé (identités Sigstore) et les
  références KMS ne sont pas vérifiées.
- La clé est lue dans un fichier PEM au démarrage : un fichier absent ou mal
  formé arrête le démarrage au lieu de faire échouer la première installation.
  Avec `enforce` et sans clé, le serveur refuse de démarrer.
- Avec `warn` (la valeur par défaut) et sans clé publique configurée, **rien
  n'est vérifié** ; le serveur se contente de journaliser un avertissement au
  démarrage.
- Une mauvaise clé, un ZIP altéré et un `.sig` absent se lisent tous comme une
  signature en échec : avec `enforce`, l'installation ou la mise à jour est
  refusée.

## 7. Limites de ressources et sandbox

Valeurs par défaut appliquées à chaque processus de plugin :

| Variable | Défaut | Effet |
|---|---|---|
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto` = choisit le meilleur disponible (normalement `landlock`). `landlock` = liste d'autorisations Landlock + blocage inconditionnel des sorties TCP (`-restrict-net` + seccomp `-deny-inet`, sans exception pour la permission `network`) + seccomp chargé par LD_PRELOAD. `basic` = seccomp chargé par LD_PRELOAD + changement d'UID seulement (débogage). `bubblewrap` = espaces de noms complets + racine montée en lecture seule + `--unshare-net` (inconditionnel aussi, pour chaque plugin, quelle que soit la permission `network`). `strict` = bubblewrap + le seccomp chargé par LD_PRELOAD. |
| `EVEY_PLUGINS_MEMORY_MB` | 256 | Mémoire résidente maximale. Le moniteur de ressources lit `/proc` et arrête le plugin au-delà. `0` désactive. |
| `EVEY_PLUGINS_CPU_NICE` | 10 | Valeur `nice` (0-19) appliquée au lancement, pour qu'un plugin ne puisse jamais priver le serveur de processeur. |
| `EVEY_PLUGINS_PIDS_MAX` | 64 | Nombre maximal de threads et processus. Empêche les bombes de fork. `0` désactive. |
| `EVEY_PLUGINS_DISK_MAX_MB` | 100 | Taille maximale du dossier d'installation ; le moniteur la mesure toutes les 30 s environ et arrête un plugin qui la dépasse. `0` désactive. |

Un plugin qui parle au réseau a besoin à la fois de `permissions: [network]` et de
`external_fetch: [<hôte>...]`. Les sorties réseau directes sont coupées par la
sandbox pour chaque plugin, sans condition et quelles que soient ses permissions
(voir les modes de sandbox ci-dessus) : le processus du plugin ne peut jamais
ouvrir lui-même une socket vers Internet. La seule issue est l'intermédiaire
`Fetch` côté hôte : le plugin l'appelle par la socket Unix `EVEY_PLUGIN_SOCK`,
l'hôte vérifie l'hôte visé dans `external_fetch`, puis se connecte par un client
protégé contre les SSRF (adresses privées et réservées refusées à la connexion,
redirections revérifiées selon l'IP résolue et l'hôte autorisé). Voir
`internal/adapters/driving/pluginhost/fetch.go`.

## 8. Distribuer un plugin

Il n'existe pas encore de catalogue officiel : un administrateur installe un
plugin en envoyant son ZIP dans le panneau d'administration, depuis l'endroit où
vous le publiez (une release GitHub, votre propre site). Distribuer, aujourd'hui,
signifie :

- Fournir ensemble le ZIP, son `.sig` et votre `cosign.pub` (section 6), pour que
  l'administrateur qui le reçoit puisse vérifier l'envoi avec
  `EVEY_PLUGINS_COSIGN_POLICY=warn` ou `enforce`.
- Indiquer les permissions et les hôtes `external_fetch` dans votre README :
  l'administrateur les voit à l'installation et doit les approuver ; une surprise
  à cet endroit est la première raison d'un refus.
- Pour faire figurer votre plugin parmi les exemples fournis, ouvrez une PR sur
  `examples/plugins/` avec les sources et un README. Les exemples sont relus et
  compilés depuis les sources ; une PR ne vaut pas approbation et ne porte aucune
  signature du projet.

## 9. Dépannage

| Symptôme | Cause | Correction |
|---|---|---|
| `bwrap: ... permission denied` | Mode `bubblewrap` dans Docker : les profils seccomp et AppArmor par défaut bloquent les appels `unshare` et `mount` dont il a besoin. | Gardez le mode `auto` par défaut, qui utilise Landlock dans un conteneur. Sinon, appliquez le profil fourni `docker/seccomp-evey.json` (aussi présent dans l'image sous `/etc/evey/seccomp.json`), un affaiblissement assumé de la sécurité : voir [Configuration du sandbox des plugins](sandbox-deployment.md). |
| `forbidden pattern "fetch(" found in theme JS` | L'analyse du JS de thème a repéré une API interdite. | Un thème ne doit appeler que `evey.theme.*`. Voyez `internal/pkg/themejs/scan.go` pour la liste de ce qui est bloqué. |
| `manifest: external_fetch entry "*.tmdb.org" is a wildcard - use explicit hostnames` | Tous les jokers sont refusés à l'installation, sans exception. | Utilisez `api.tmdb.org` (et listez chaque sous-domaine nécessaire), pas `*.tmdb.org` ni `cdn.*.tmdb.org`. |
| `manifest: frontend slot "admin.section" not allowed` | Les interfaces de plugins sont interdites dans les zones privilégiées. | Choisissez un emplacement non critique dans la liste, ou repensez le plugin. |
| `signature does not match the plugin or the configured key` | Le `.sig` a été fait avec une autre clé, ou le ZIP a changé après la signature. | Resignez exactement ce ZIP avec la bonne clé, ou mettez à jour `EVEY_PLUGINS_COSIGN_PUBLIC_KEY`. |
| `plugin already installed at version 1.0.0, new version must be higher` | Le ZIP porte la version déjà installée. | Augmentez `version` dans `plugin.yaml`, ou désinstallez d'abord le plugin. |
| Le plugin s'arrête, le journal indique `memory quota exceeded (… MB > … MB)` | Le moniteur de ressources l'a tué au-delà de sa limite de mémoire (code 137). | Réduisez sa consommation, ou relevez sa limite de mémoire sur sa carte dans **Admin → Plugins** (`EVEY_PLUGINS_MEMORY_MB` fixe la valeur par défaut). |
| Module d'interface invisible | Plugin non activé, emplacement erroné, ou mauvais préfixe `web_root` dans `frontend.src`. | Regardez la page Plugins de l'administration (badge d'état), vérifiez que la chaîne de l'emplacement correspond à un vrai `<PluginSlot name=...>`, et cherchez les appels `/api/v1/plugins/{id}/assets/...` dans l'onglet réseau. |

## 10. Pour aller plus loin

- Le modèle de sécurité de bout en bout : [security-model.md](security-model.md)
- La surface de l'API de l'hôte : [api-reference.md](api-reference.md)
- Les emplacements disponibles : [api-reference.md](api-reference.md#slots)
- Le code du SDK par langage : `sdk/`
- Les plugins et thèmes de référence : `examples/plugins/`
