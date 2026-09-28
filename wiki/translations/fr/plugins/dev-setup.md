---
source_sha: 8da868d1b798afe178e9ef7c621a9a0513b023c77f535745e4e5d2c44b5dae50
---

# Mise en place pour développer une extension

Parcours complet pour qui vient de cloner Evey et veut écrire, tester et livrer une extension ou un thème. Choisissez l'environnement d'exécution qui correspond au langage de votre extension ; le reste est commun.

## 1. Prérequis

Evey exécute le processus hôte, écrit en Go, et livre les extensions sous forme d'archives ZIP. L'environnement d'exécution d'une extension dépend de ce que votre code appelle.

### Toujours nécessaire

| Outil | Pourquoi | Installation |
|---|---|---|
| Go ≥ 1.25 | Compiler l'hôte Evey | `https://go.dev/dl/` |
| Docker et Docker Compose | La pile de développement par défaut (Postgres, Meilisearch, serveur, web) | https://docs.docker.com/engine/install/ |
| `bubblewrap` (`bwrap`) | Le bac à sable des processus d'extension, nécessaire aux modes `bubblewrap` et `strict` | `apt install bubblewrap` (Debian, Ubuntu), `dnf install bubblewrap` (Fedora), `pacman -S bubblewrap` (Arch). Sur macOS, installez Linux dans une machine virtuelle : bwrap n'existe que sous Linux. |

Le mode de bac à sable `basic` fonctionne sans bwrap, mais ne devrait jamais servir hors d'une expérimentation locale jetable : quittez-le dès que possible.

### Selon le langage

Prenez ce que votre extension utilise réellement ; tout n'est pas nécessaire.

| Langage de l'extension | Outils | Remarques |
|---|---|---|
| Python 3 | `python3`, `pip` | Utilisez `pip install --user` pour ne pas polluer la machine. Les stubs de SDK vivent dans `sdk/python/`. |
| Node.js | Node 20 ou plus, `npm` ou `pnpm` | Le SDK charge le fichier proto à l'exécution. |
| Go | Go ≥ 1.21 | Aucun stub fourni : engendrez-les avec `protoc-gen-go` et `protoc-gen-go-grpc` ; un binaire Go compilé réclame en plus la permission `native-binary`. |
| Dart, Java, PHP, Ruby, Swift | La chaîne d'outils du langage | Les stubs sont pré-engendrés dans `sdk/<langage>/`. |
| Thème CSS seul | Rien de plus | Un thème est une archive avec `kind: theme` et un `theme.css`. |
| Thème avec JS | Rien de plus (l'hôte exécute votre JS dans une `iframe` isolée, avec une politique de sécurité de contenu verrouillée) | L'analyseur statique refuse tout accès à `document`, `window`, `fetch`, `localStorage` et consorts. |

### Engendrer des stubs pour un autre langage

Des stubs sont fournis pour les langages ci-dessus, mais le contrat gRPC des extensions n'est qu'un fichier proto : vous pouvez viser **n'importe quel langage que `protoc` gère** (Rust, C#, Kotlin, Elixir, etc.). La source unique de vérité est :

```
proto/plugin/v1/plugin_api.proto
```

Engendrez-les avec `protoc` ou `buf`, en pointant le greffon gRPC de votre langage dessus :

```bash
# generic shape - exact --*_out flags depend on the language/plugin
protoc --proto_path=proto \
       --<lang>_out=OUT_DIR --grpc_<lang>_out=OUT_DIR \
       proto/plugin/v1/plugin_api.proto
```

Les recettes éprouvées pour les langages fournis, et le greffon gRPC dont chacun a besoin, vivent dans la cible `plugin-sdk` du `Makefile` racine et dans chaque `sdk/<langage>/README.md`. La commande `make plugin-sdk` régénère les stubs livrés.

Embarquez les stubs engendrés dans votre archive : l'hôte ne recopie
automatiquement que ceux des langages qu'il fournit.

### Facultatif mais recommandé

| Outil | Pourquoi |
|---|---|
| `cosign` (Sigstore) | Signer l'archive de votre extension. La politique par défaut est `warn` et non `off` : avec `warn` et aucune clé publique configurée, la vérification ne fait rien, elle se contente d'un avertissement. Renseignez la clé publique pour vérifier réellement les signatures, ou passez en `enforce` pour refuser les envois non signés. |
| `pre-commit` | Attraper l'évident avant de soumettre une extension. |
| `golangci-lint` | Nécessaire si votre extension est en Go et que vous voulez la même analyse que l'hôte. |

## 2. Lancer Evey en local

```sh
git clone git@github.com:Evey-project/evey.git
cd evey
cp .env.dev.example .env.dev   # generates a sane local config
make dev                        # docker compose up: postgres, meilisearch, server (Air), web (Vite)
```

Quand le serveur est en bonne santé, ouvrez https://evey.test, servi par Caddy. Les identifiants par défaut sont semés dans `.env.dev` : changez-les avant d'exposer l'instance.

Si `make dev` échoue sur `bwrap` : vérifiez que `which bwrap` renvoie bien un chemin sur la machine hôte ; dans Docker, le conteneur de développement embarque le binaire, et si vous avez personnalisé la pile, montez-le ou passez le mode de bac à sable à `basic` *pour le seul conteneur de développement*.

## 3. Créer votre première extension

Des exemples vivent dans `examples/plugins/` : des points de départ en Node.js et en Python (`nodejs-example`, `python-example`), une source de métadonnées (`metadata-example`), un récepteur de webhooks (`webhook-receiver`), un widget d'accueil (`weather`), la configuration d'une extension (`config-demo`) et deux thèmes (`theme-dark-blue`, `open-workshop`). Prenez le plus proche de votre besoin et copiez-le dans un répertoire de travail. Pour la persistance et les permissions, regardez **`examples/plugins/data-demo`** : il utilise le magasin de données par extension et demande les permissions `public` et `prefs` ainsi que l'emplacement `admin.nav`.

```sh
cp -r examples/plugins/nodejs-example ~/my-first-plugin
cd ~/my-first-plugin
```

Éditez `plugin.yaml` :

- `name` : en minuscules avec des tirets, unique.
- `version` : au format semver, à incrémenter à chaque publication.
- `kind` : `plugin` si une partie serveur est nécessaire, `theme` si tout est côté interface.
- `language` : `python`, `nodejs`, `go`, `php`, `ruby`, `lua`, `bash` ou `sh`. C'est un identifiant de manifeste, pas le nom du binaire de l'interpréteur : `python3` et `node` ne sont pas des valeurs valides ici.
- `entrypoint` : la ligne de commande que l'exécution lance.
- `permissions` : déclarez chaque API hôte que vous appelez. L'installeur rejette les appels non déclarés.
- `external_fetch` : quand vous devez joindre une API tierce. Les entrées sont des noms d'hôtes nus : aucun schéma n'est nécessaire ni utilisé, aucun caractère générique n'est accepté, et aucune adresse IP littérale, quelle qu'elle soit.

Le module frontal, facultatif pour une extension et interdit pour un thème :

```yaml
frontend:
  - slot: home.section
    src: dist/index.js
```

L'emplacement doit exister dans la liste blanche du serveur : accueil, bandeau d'accueil, panneau latéral de bibliothèque, panneau latéral de détail, superposition du lecteur, bas de la barre latérale, section apparence des réglages, et `admin.nav`, le seul emplacement contrôlé de la zone d'administration, une entrée de menu qui ouvre la page isolée de votre extension. Tout le reste est refusé à l'installation.

**Stocker des données.** Votre partie serveur dispose d'un magasin clé-valeur privé, accessible par la socket de l'hôte, sans aucune permission à demander, et effacé automatiquement à la désinstallation. Pour des préférences par utilisateur depuis votre iframe, demandez la permission `prefs` ; pour un accès en lecture seule aux médias non sensibles, titre et affiche, demandez `public`. Les portées d'administration, d'authentification, de système et d'écriture générique ne peuvent jamais être demandées.

## 4. La boucle d'installation

```sh
# zip your plugin
cd ~/my-first-plugin
zip -r my-first-plugin.zip . -x ".*" "node_modules/*" "*.pyc"

# upload through the admin UI
open https://evey.test/admin/plugins
# or via the API
curl -F "file=@my-first-plugin.zip" \
     -H "Authorization: Bearer $EVEY_TOKEN" \
     https://evey.test/api/v1/admin/plugins
```

L'installeur procède dans cet ordre, et chaque étape peut refuser l'installation.

1. **Validation du manifeste** : `plugin.yaml` est lu directement dans l'archive, avant toute vérification de l'archive elle-même, et confronté au schéma. Liste blanche des emplacements, des permissions et des capacités. Analyse des hôtes externes, qui rejette les caractères génériques et toute adresse IP littérale. L'environnement d'exécution déclaré doit être disponible dans l'image.
2. **Escalade de permissions**, à la mise à jour seulement : l'installeur compare le nouveau manifeste au précédent. Toute permission, capacité, emplacement ou hôte externe nouvellement demandé bloque l'installation, et l'appelant doit passer outre explicitement. Une première installation saute cette étape, faute de point de comparaison.
3. **Signature cosign**, vérifiée quand un vérificateur est configuré ; un échec ne bloque qu'en politique `enforce`.
4. **Extension de l'archive** : extension de fichier, taille totale d'au plus 500 Mo, 100 Mo par entrée, taux de compression d'au plus 100 pour 1, aucun lien symbolique, aucune traversée de chemin. Les fichiers atterrissent dans un répertoire de transit, et une mise à jour ne touche jamais la version vivante avant que toutes les étapes suivantes ne soient passées.
5. **Assainissement du CSS**, pour les thèmes seulement : les imports sont retirés, les URL non incorporées bloquées, les superpositions plein écran interdites, les sélecteurs visant les champs de mot de passe et les classes d'administration bloqués, et la profondeur d'empilement plafonnée.
6. **Analyse du JS de thème**, pour les thèmes seulement : tout accès aux cookies, au stockage local, à `fetch`, à `eval`, aux variables globales de l'hôte ou aux détours par le constructeur est refusé. Un thème ne doit utiliser que l'API `evey.theme.*`.
7. **Analyseur statique** : chaque fichier texte est passé au crible pour y chercher exécution de commandes, shell inversé, évaluation dynamique, décodage base64 suivi d'exécution, obfuscation, installation de dépendances à l'exécution et élévation de privilèges. Une trouvaille critique refuse l'installation ; un avertissement la refuse aussi tant que la permission correspondante n'est pas déclarée.
8. **Persistance** : le répertoire de transit prend la place de l'ancien, la ligne est écrite en base, et l'exécution est lancée si besoin.

Si une étape échoue, l'interface d'administration affiche la raison exacte, et les journaux du serveur portent la pile complète.

## 5. Itérer

Pour recharger votre extension pendant le développement, modifiez le code, réarchivez et renvoyez par le bouton de mise à jour de l'interface, ou plus rapidement, déposez le fichier directement dans le répertoire de l'extension à l'intérieur du conteneur :

```sh
# edit code → re-zip → re-upload via UI (Update button)
# or, faster: drop the file directly into the plugin directory inside the container
docker compose exec evey ls /var/lib/evey/plugins/<plugin-id>/
```

Quand vous incrémentez la version, l'installeur accepte l'envoi comme une mise à jour. Si vous ajoutez une permission, vous tomberez sur le circuit d'escalade : c'est la conception, pas un bug.

Les journaux du processus de l'extension arrivent dans la page d'administration de l'extension, pour les dernières lignes, et dans le flux complet des journaux du serveur.

## 6. Signer votre extension (facultatif en développement, recommandé pour la distribution)

Si vous comptez distribuer l'extension à d'autres instances, signez-la. L'administrateur qui la reçoit peut alors faire remonter les envois non signés.

```sh
# one-time: generate a keypair (keep cosign.key safe - never commit it)
cosign generate-key-pair

# sign the ZIP
cosign sign-blob \
  --key cosign.key \
  --output-signature my-first-plugin.zip.sig \
  my-first-plugin.zip

# distribute: my-first-plugin.zip + my-first-plugin.zip.sig + cosign.pub
```

Sur l'instance Evey qui installe l'extension :

```sh
# put the public key somewhere readable by the server
sudo cp cosign.pub /etc/evey/cosign.pub

# point Evey at it and switch policy
export EVEY_PLUGINS_COSIGN_PUBLIC_KEY=/etc/evey/cosign.pub
export EVEY_PLUGINS_COSIGN_POLICY=warn   # or "enforce" to refuse unsigned
docker compose restart evey
```

Evey valide la clé au démarrage : un fichier absent ou mal formé fait échouer le démarrage plutôt que la première installation. Les références à un gestionnaire de clés distant sont acceptées telles quelles.

## 7. Plafonds de ressources et bac à sable

Valeurs appliquées par défaut à tout processus d'extension :

| Variable | Défaut | Effet |
|---|---|---|
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto` retient le meilleur mécanisme disponible, normalement `landlock`. `landlock` combine la liste blanche de fichiers du noyau, le refus inconditionnel de toute sortie réseau et une couche seccomp. `basic` ne garde que seccomp et la bascule d'uid, pour le débogage. `bubblewrap` ajoute les espaces de noms et une racine en lecture seule, avec là aussi un réseau coupé pour toutes les extensions. `strict` y ajoute une liste blanche d'appels système. |
| `EVEY_PLUGINS_MEMORY_MB` | 256 | Mémoire résidente maximale. Le moniteur interroge `/proc` et arrête l'extension au-delà. `0` désactive. |
| `EVEY_PLUGINS_CPU_NICE` | 10 | La politesse d'ordonnancement appliquée au lancement, de 0 à 19, pour qu'une extension ne prive jamais le serveur de processeur. |
| `EVEY_PLUGINS_PIDS_MAX` | 64 | Nombre maximal de processus et de fils. Empêche une bombe à fourche. `0` désactive. |
| `EVEY_PLUGINS_DISK_MAX_MB` | 100 | Taille maximale du répertoire d'installation, mesurée toutes les trente secondes environ. `0` désactive. |

Une extension qui parle au réseau a besoin à la fois de la permission `network` et d'une liste `external_fetch`. La sortie réseau directe est coupée au niveau du bac à sable pour **toutes** les extensions, sans condition et quelles que soient leurs permissions : le processus ne peut jamais ouvrir de socket vers internet. La seule voie est le relais `Fetch` de l'hôte, que l'extension appelle par sa socket locale : l'hôte vérifie l'hôte visé contre la liste déclarée, puis compose avec un client protégé contre les requêtes vers l'intérieur, les adresses privées et réservées étant refusées au moment de la connexion et les redirections revérifiées.

## 8. Distribuer une extension

Il n'existe pas encore de catalogue officiel : un administrateur installe une extension en envoyant son archive dans le panneau d'administration, depuis l'endroit où vous la publiez, une version sur GitHub ou votre propre site. Ce que « distribuer » veut dire aujourd'hui :

- Livrez ensemble l'archive, sa signature et votre clé publique, pour que l'administrateur qui la reçoit puisse vérifier l'envoi.
- Annoncez dans votre README les permissions et les hôtes externes demandés : l'administrateur les voit à l'installation et doit les approuver, et une surprise à ce moment-là est la première raison d'un refus.
- Pour figurer parmi les exemples fournis, proposez une contribution sur `examples/plugins/` avec les sources et un README. Les exemples sont relus et construits depuis les sources ; une contribution acceptée n'est ni une caution ni une signature du projet.

## 9. Dépannage

| Symptôme | Cause | Remède |
|---|---|---|
| `bwrap: ... permission denied` | Le profil seccomp par défaut de Docker bloque `unshare`. | Le dépôt fournit un profil dédié ; assurez-vous que votre fichier compose l'applique. |
| `forbidden pattern "fetch(" found in theme JS` | L'analyseur de JS de thème a vu une API interdite. | Un thème ne doit appeler que l'API `evey.theme.*`. |
| `manifest: external_fetch entry "*.tmdb.org" is a wildcard` | Tout caractère générique est refusé à l'installation, sans exception. | Écrivez chaque sous-domaine explicitement. |
| `installer: slot "admin.section" not allowed` | Les parties frontales d'extension sont interdites dans les zones privilégiées. | Choisissez un emplacement de la liste blanche, ou repensez l'extension. |
| `cosign verify-blob: failed signature` | La clé publique du serveur ne correspond pas à la clé privée de signature. | Resignez avec la bonne clé, ou corrigez le chemin de la clé publique. |
| L'extension démarre puis meurt en code 137 | Tuée pour dépassement mémoire. | Réduisez la consommation ou relevez le plafond mémoire. |
| Le module frontal n'apparaît pas | Extension désactivée, emplacement qui ne correspond pas, ou préfixe de racine web erroné. | Vérifiez la pastille d'état sur la page des extensions, que la chaîne d'emplacement correspond à un point de montage réel, et les appels aux ressources dans le panneau réseau du navigateur. |

## 10. Pour aller plus loin

- Le modèle de sécurité de bout en bout : [security-model.md](security-model.md)
- La surface de l'API hôte : [api-reference.md](api-reference.md)
- Les emplacements disponibles : [api-reference.md](api-reference.md#slots)
- Le code des SDK par langage : `sdk/`
- Les extensions et thèmes de référence : `examples/plugins/`
