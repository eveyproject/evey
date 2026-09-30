---
source_sha: d5cb641722482071362563f54ebfa269c77cb3d81faa2c0ec9878ad7f4ef501c
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
| Go ≥ 1.25 | Compiler l'hôte Evey | `https://go.dev/dl/` |
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
| JS de thème | Rien de plus (l'hôte fait tourner votre JS dans une `iframe` isolée avec `sandbox="allow-scripts"` et une CSP verrouillée) | L'analyse statique refuse tout accès à `document`, `window`, `fetch`, `localStorage`, etc. Voir `internal/pkg/themejs/scan.go`. |

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
| `cosign` (Sigstore) | Signer le ZIP de votre plugin. `EVEY_PLUGINS_COSIGN_POLICY` vaut `warn` par défaut (pas `off`) : avec `warn` et sans clé publique configurée, la vérification ne fait rien (un avertissement est journalisé, rien n'est bloqué) ; réglez `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` pour vérifier réellement les signatures, ou `enforce` pour refuser les envois non signés. https://docs.sigstore.dev/cosign/installation/ |
| `pre-commit` | Repérer les problèmes évidents avant de soumettre un plugin. |
| `golangci-lint` | Nécessaire si votre plugin est en Go et que vous voulez les mêmes contrôles que l'hôte. |

## 2. Lancer Evey en local

```sh
git clone git@github.com:Evey-project/evey.git
cd evey
cp .env.dev.example .env.dev   # une configuration locale raisonnable
make dev                        # docker compose up : postgres, meilisearch, serveur (Air), web (Vite)
```

Une fois le serveur en bonne santé, ouvrez https://evey.test (servi par Caddy). Des
identifiants par défaut sont posés dans `.env.dev` : changez-les avant d'exposer
l'instance.

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
curl -F "file=@my-first-plugin.zip" \
     -H "Authorization: Bearer $EVEY_TOKEN" \
     https://evey.test/api/v1/admin/plugins
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
docker compose exec evey ls /var/lib/evey/plugins/<plugin-id>/
```

Quand vous augmentez `version`, l'installeur accepte l'envoi comme une mise à
jour. Si vous ajoutez une nouvelle permission, vous passerez par la procédure
d'élévation : c'est voulu, pas un bug.

Les journaux du processus du plugin arrivent :

- dans `/admin/plugins/<id>`, les N dernières lignes dans l'interface ;
- dans `docker compose logs evey | grep <plugin-id>`, le flux complet.

## 6. Signer son plugin (facultatif en développement, recommandé pour le distribuer)

Si vous comptez distribuer le plugin à d'autres instances d'Evey, signez-le.
L'administrateur qui le reçoit peut alors régler
`EVEY_PLUGINS_COSIGN_POLICY=warn` (ou `enforce`) pour faire ressortir les envois
non signés.

```sh
# une seule fois : générer une paire de clés (gardez cosign.key en lieu sûr, ne la commitez jamais)
cosign generate-key-pair

# signer le ZIP
cosign sign-blob \
  --key cosign.key \
  --output-signature my-first-plugin.zip.sig \
  my-first-plugin.zip

# à distribuer : my-first-plugin.zip + my-first-plugin.zip.sig + cosign.pub
```

Sur l'instance d'Evey qui installe le plugin :

```sh
# placer la clé publique à un endroit lisible par le serveur
sudo cp cosign.pub /etc/evey/cosign.pub

# y faire pointer Evey et changer de politique
export EVEY_PLUGINS_COSIGN_PUBLIC_KEY=/etc/evey/cosign.pub
export EVEY_PLUGINS_COSIGN_POLICY=warn   # ou "enforce" pour refuser ce qui n'est pas signé
docker compose restart evey
```

Evey vérifie la clé au démarrage (lecture du PEM) : un fichier de clé absent ou
mal formé fait échouer le démarrage plutôt que la première installation. Les
références KMS (`gcpkms://...`, `awskms://...`, `k8s://...`) sont acceptées
telles quelles.

## 7. Limites de ressources et sandbox

Valeurs par défaut appliquées à chaque processus de plugin :

| Variable | Défaut | Effet |
|---|---|---|
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto` = choisit le meilleur disponible (normalement `landlock`). `landlock` = liste d'autorisations Landlock + blocage inconditionnel des sorties TCP (`-restrict-net` + seccomp `-deny-inet`, sans exception pour la permission `network`) + seccomp chargé par LD_PRELOAD. `basic` = seccomp chargé par LD_PRELOAD + changement d'UID seulement (débogage). `bubblewrap` = espaces de noms complets + racine montée en lecture seule + `--unshare-net` (inconditionnel aussi, pour chaque plugin, quelle que soit la permission `network`). `strict` = bubblewrap + liste d'autorisations seccomp. |
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
| `bwrap: ... permission denied` | Le profil seccomp Docker par défaut bloque `unshare`. | Le dépôt fournit un profil seccomp personnalisé (`docker/seccomp.json`). Vérifiez que votre fichier compose l'applique. |
| `forbidden pattern "fetch(" found in theme JS` | L'analyse du JS de thème a repéré une API interdite. | Un thème ne doit appeler que `evey.theme.*`. Voyez `internal/pkg/themejs/scan.go` pour la liste de ce qui est bloqué. |
| `manifest: external_fetch entry "*.tmdb.org" is a wildcard - use explicit hostnames` | Tous les jokers sont refusés à l'installation, sans exception. | Utilisez `api.tmdb.org` (et listez chaque sous-domaine nécessaire), pas `*.tmdb.org` ni `cdn.*.tmdb.org`. |
| `installer: slot "admin.section" not allowed` | Les interfaces de plugins sont interdites dans les zones privilégiées. | Choisissez un emplacement non critique dans la liste, ou repensez le plugin. |
| `cosign verify-blob: failed signature` | La clé publique du serveur ne correspond pas à la clé privée qui a signé. | Resignez avec la bonne clé, ou mettez à jour `EVEY_PLUGINS_COSIGN_PUBLIC_KEY`. |
| Le plugin démarre puis meurt avec le code 137 | Tué par le cgroup pour dépassement de mémoire. | Réduisez sa consommation ou augmentez `EVEY_PLUGINS_MEMORY_MB`. |
| Module d'interface invisible | Plugin non activé, emplacement erroné, ou mauvais préfixe `web_root` dans `frontend.src`. | Regardez la page Plugins de l'administration (badge d'état), vérifiez que la chaîne de l'emplacement correspond à un vrai `<PluginSlot name=...>`, et cherchez les appels `/api/v1/plugins/{id}/assets/...` dans l'onglet réseau. |

## 10. Pour aller plus loin

- Le modèle de sécurité de bout en bout : [security-model.md](security-model.md)
- La surface de l'API de l'hôte : [api-reference.md](api-reference.md)
- Les emplacements disponibles : [api-reference.md](api-reference.md#slots)
- Le code du SDK par langage : `sdk/`
- Les plugins et thèmes de référence : `examples/plugins/`
