---
source_sha: 2afa372fc288cb82c9e90ad0f1bb879056bb0e14682a438cab6ce64d779bf48d
---

# Bac à sable des extensions et modèle de menace

> **Quelle page cherchez-vous ?** Celle-ci décrit ce que le *serveur* fait pour
> confiner une extension : les couches, les modes de bac à sable, ce que chacun
> ne sait toujours pas empêcher, et comment le configurer. Si vous écrivez une
> extension et voulez savoir ce que vous pouvez déclarer et ce qui sera refusé,
> lisez plutôt [le modèle de sécurité des extensions](plugins/security-model.md).
> Les deux pages traitent le même sujet par les deux bouts et évitent
> délibérément de se répéter.

Cette page décrit les couches de défense qu'Evey applique aux extensions tierces, les limites de ces défenses, et la feuille de route vers une isolation plus forte.

## Modèle de menace : ce qu'une extension hostile tenterait

Les extensions tournent dans des processus séparés qui échangent en gRPC avec l'hôte sur des sockets Unix. L'hôte ne fait par défaut confiance à aucun code d'extension. Les risques contre lesquels nous nous défendons, par gravité décroissante :

1. **Usurpation d'identité sur le canal hôte** : une extension malveillante se fait passer pour une autre afin d'obtenir ses permissions.
2. **Élévation de privilèges** : une extension s'échappe de son bac à sable et exécute des commandes arbitraires sous l'identité du serveur.
3. **Fuite ou écriture de fichiers** : une extension lit ou écrit hors de son répertoire d'installation.
4. **Exfiltration réseau** : une extension ouvre des connexions sortantes pour renvoyer des données.
5. **Abus de ressources** : minage, bombe à fourche, épuisement des descripteurs.
6. **Chaîne d'approvisionnement** : un auteur livre du code obfusqué ou compilé que l'analyseur statique ne peut pas inspecter.

## Couches de défense actuelles

### 1. Analyse statique à l'installation

Chaque archive est décompressée dans un répertoire temporaire et parcourue par l'analyseur. Les catégories examinées, avec leur gravité :

| Catégorie | Gravité | Exemples |
|---|---|---|
| exécution de commandes | CRITIQUE | `os.system`, `subprocess.run`, `exec()`, `child_process.exec`, les équivalents PHP et Ruby |
| réseau | AVERTISSEMENT ou CRITIQUE | `requests.get`, `fetch`, shells inversés |
| système de fichiers | AVERTISSEMENT ou CRITIQUE | `/etc/passwd`, `.ssh/`, `rm -rf`, manipulations de liens |
| obfuscation | CRITIQUE | `eval`, `exec`, décodage base64, séquences hexadécimales, chaînes redirigées vers un shell |
| privilèges | CRITIQUE | `setuid`, `chmod 4777`, `sudo`, signal envoyé au processus 1 |
| minage | CRITIQUE | `stratum+tcp`, `xmrig`, `cryptonight` |
| installation à l'exécution | CRITIQUE | `pip install`, `npm install`, un téléchargement redirigé vers un shell |
| import dynamique | CRITIQUE | `__import__()`, `importlib.import_module` |
| évasion du bac à sable | CRITIQUE | `vm.runIn*`, `process.binding`, `new Function`, la traversée de hiérarchie de classes Python |
| désérialisation | CRITIQUE ou AVERTISSEMENT | `pickle.loads`, `marshal.loads`, `__reduce__` ; avertissement pour un chargement YAML sans chargeur explicite |
| code natif | CRITIQUE ou AVERTISSEMENT | `ctypes.CDLL`, `ffi.Library` ; avertissement pour les pointeurs non sûrs en Go |
| binaire compilé | AVERTISSEMENT devenant blocage | `.so`, `.dll`, `.exe`, `.wasm`, `.jar`, opaques à l'analyseur |
| archive imbriquée | AVERTISSEMENT devenant blocage | `.zip`, `.tar.gz`, code extractible |
| type de fichier incohérent | AVERTISSEMENT traité comme critique | l'extension annonce une image, le contenu est autre chose |
| URL suspecte | AVERTISSEMENT | schéma non HTTP dans une URL littérale |
| dépendances non embarquées | AVERTISSEMENT | un fichier de dépendances présent sans modules embarqués |

Une trouvaille critique, ou un type de fichier incohérent, bloque l'installation.

Les binaires compilés et les archives imbriquées bloquent l'installation **par défaut**, tant que le manifeste ne déclare pas la permission `native-binary`. L'administrateur voit à la fois l'avertissement et la permission demandée à l'étape de prévisualisation, avant d'accepter.

### 2. Barrière des permissions du manifeste

Le `plugin.yaml` déclare les permissions demandées, et l'exécution les applique à deux niveaux.

- **`subprocess`** autorise le lancement de processus enfants. Sans elle, la couche préchargée refuse l'exécution pour les extensions interprétées. La liste noire du noyau, elle, ne refuse pas l'exécution puisque l'amorce doit lancer l'extension, mais les enfants héritent du filtre et du domaine Landlock.
- **`native-binary`** est nécessaire pour livrer des binaires ou des archives. L'administrateur doit accepter ce code opaque à l'installation.
- **Les permissions d'API** sont vérifiées à chaque appel par l'intercepteur dédié.

### 3. Séparation d'uid

Un processus d'extension peut basculer vers un compte distinct de celui du serveur, ce qui rend les secrets de ce dernier illisibles par simple permission de fichiers. Le réglage accepte trois valeurs :

- `auto` bascule **seulement quand le bac à sable retombe en mode `basic`**, le seul cas où les permissions de fichiers sont ce qui protège les secrets. Sous Landlock ou bubblewrap, c'est redondant et donc ignoré.
- `on` bascule toujours, comme couche indépendante supplémentaire.
- `off` ne bascule jamais.

**C'est l'amorce qui effectue la bascule, pas le serveur.** Le serveur reste non root et sans capacité ; l'amorce porte la capacité nécessaire et change d'identité juste avant de lancer l'extension. Elle refuse toute identité cible inférieure à 1000. Si elle ne peut pas basculer (faute de capacité, parce que l'option qui bloque l'acquisition de privilèges est posée, ou parce que le binaire de l'amorce lui-même manque), l'issue dépend du mode : en `basic`, où l'identité est le seul rempart autour des secrets, l'extension est **refusée** (fermeture par défaut) ; sous `landlock`, où le système de fichiers est déjà cloisonné, elle avertit et lance l'extension sous l'identité du serveur.

Ce que cela apporte, **même sur un noyau sans Landlock** :

- Les secrets du serveur sont protégés par les permissions de fichiers : la clé JWT et les sauvegardes sont rendues accessibles à leur seul propriétaire **à l'exécution**, si bien que la protection tient même sur un répertoire de données monté depuis la machine hôte.
- Combiné au marquage non capturable du serveur, la clé JWT en mémoire et les secrets d'environnement sont illisibles par les fichiers de processus.

Toutes les extensions partagent la même identité, si bien qu'elles ne sont pas isolées **les unes des autres** par les seules permissions : c'est le confinement des fichiers, Landlock ou bubblewrap, qui s'en charge. En mode `basic` avec séparation d'uid, une extension malveillante pourrait lire ou perturber les fichiers d'une voisine ; les isoler entre elles sans Landlock demanderait une identité *par extension*.

### 4. Confinement des appels système

Deux filtres appliquent presque la même liste noire, et ne diffèrent que sur les appels de signalisation. Tous deux commencent par interdire l'acquisition de nouveaux privilèges, puis installent un programme qui refuse les appels de la liste et laisse passer le reste.

1. **Au niveau du noyau, la couche principale.** L'amorce est placée en tête de la ligne de commande et installe le filtre **juste avant l'exécution**, pour l'architecture de compilation. Le filtre est hérité au travers de l'exécution, dans l'extension puis dans tout enfant qu'elle lancerait. Parce qu'il est installé par l'amorce et non par préchargement de bibliothèque, il **contraint aussi les binaires compilés**, qui ne chargeaient jamais la couche préchargée et la contournaient donc entièrement.
2. **Par préchargement, en ceinture et bretelles pour les extensions dynamiques.** Un petit constructeur est préchargé dans les extensions interprétées : même liste noire, et il marque en plus l'extension elle-même comme non capturable.

**Lequel des deux s'applique dépend du mode** : le filtre noyau de l'amorce en modes `landlock` et `basic`, la couche préchargée en modes `strict` et `basic`. Les modes `bubblewrap` et `strict` n'exécutent **pas** l'amorce : un binaire compilé sous `bubblewrap` simple n'a donc aucune liste noire d'appels système, son confinement venant des espaces de noms et de l'isolation des fichiers.

Les appels bloqués, par catégorie :

| Catégorie | Appels bloqués |
|---|---|
| Contrôle et débogage de processus | `ptrace`, lecture et écriture de la mémoire d'un autre processus |
| Signaux | `kill` dans les deux filtres ; l'amorce refuse en plus la signalisation par identifiant de fil, tandis que la couche préchargée ne la refuse que si elle vise le processus 1 |
| Montage | `mount`, `umount2`, `pivot_root`, `chroot` |
| Espaces de noms | `unshare`, `setns` |
| Modules noyau | chargement et déchargement de modules |
| BPF et compteurs de performance | `bpf`, `perf_event_open` |
| Entrées-sorties matérielles | `ioperm`, `iopl`, couche préchargée seulement |
| Mémoire d'échange | `swapon`, `swapoff` |
| Contrôle du système | `syslog`, `reboot`, et le réglage de l'horloge |
| Redémarrage par noyau chargé | `kexec_load` dans les deux filtres |
| Famille setuid | tous les changements d'identité |
| Capacités | `capset` |
| Trousseau de clés | `add_key`, `request_key`, `keyctl` |

L'exécution d'un nouveau programme est **refusée par la couche préchargée**, pour qu'une extension interprétée sans la permission `subprocess` ne puisse pas lancer d'enfant, mais **pas par le filtre noyau de l'amorce**, qui doit lui-même lancer l'extension. Les enfants héritent de toute façon du filtre et du domaine Landlock, et restent donc confinés.

**Aucune sortie IP pour AUCUNE extension.** Toute extension, avec ou sans permission réseau, se voit refuser l'ouverture de sockets internet au niveau du noyau, ou placer dans un espace réseau isolé selon le mode. Une extension qui a besoin du réseau ne se connecte pas directement : elle appelle le relais de l'hôte par sa socket locale, laquelle reste autorisée, et c'est l'hôte qui applique la liste des hôtes déclarés et un client protégé contre les requêtes vers l'intérieur. Cela ferme le trou des adresses IP littérales, celles des métadonnées d'infrastructure par exemple, qu'une liste blanche de noms et un filtre par port ne pouvaient pas fermer. Voir [ADR-0014](adr/0014-landlock-plugin-sandbox.md).

Les limites, à lire attentivement :

- **C'est une liste noire, pas une liste blanche.** Lire, écrire, ouvrir, projeter en mémoire et créer des fils restent permis. Une véritable liste blanche d'appels système reste au programme.
- **Le confinement des fichiers vient d'une autre couche.** Les appels système ne bornent pas les chemins : c'est le rôle de Landlock ou de bubblewrap. Sur un noyau qui n'a ni l'un ni l'autre, l'extension peut lire tout fichier lisible par tous, mais pas les secrets du serveur, protégés par les permissions.
- **Fermeture par défaut pour les extensions interprétées.** Si la couche préchargée est introuvable et que l'extension n'a pas la permission `subprocess`, le lanceur refuse de la démarrer et émet un événement d'erreur. Le filtre noyau, lui, travaille au mieux : un noyau sans ce mécanisme dégrade vers les autres couches.

### 5. Une socket hôte par extension, authentifiée par identifiants de pair

L'hôte expose une socket Unix par extension. Chaque appel entrant est authentifié en trois temps : lecture de l'identifiant de processus du pair auprès du **noyau**, valeur qu'un programme ne peut pas falsifier ; comparaison avec celui enregistré juste après le lancement ; et estampillage de l'identité de l'extension dans le contexte, cette identité venant de la socket et jamais du réseau.

Conséquences : une extension ne peut pas se faire passer pour une autre au moyen d'un en-tête, celui-ci étant ignoré ; elle ne peut pas atteindre la socket d'une autre, la vérification d'identifiant rejetant la connexion ; et elle ne peut pas faire transiter ses appels par un processus enfant, dont l'identifiant diffère. Ce dernier point est voulu et cohérent avec la permission `subprocess`.

### 6. Barrière de prévisualisation par l'administrateur

Un point d'accès dédié exécute l'analyseur statique et renvoie le manifeste et les trouvailles sans rien installer. L'administrateur voit les permissions demandées, chaque trouvaille avec sa gravité, son fichier et sa ligne, les URL extraites regroupées par type, et l'empreinte de la source, qui correspondra à celle de l'installation.

C'est l'administrateur qui constitue la frontière de confiance pour les permissions élevées : Evey montre la surface, l'administrateur accepte ou refuse.

### 7. Notification de panne d'une extension

Le lanceur publie un événement d'erreur quand la bibliothèque de bac à sable manque alors que l'extension n'a pas la permission `subprocess`, quand le processus se termine anormalement, et quand l'intercepteur d'authentification consigne une tentative avec un identifiant de processus qui ne correspond pas. Le service de notifications s'y abonne et prévient l'administrateur par le canal configuré.

### 8. Durcissement du processus serveur

Au démarrage, le serveur se marque non capturable. Les fichiers de son propre processus - mémoire, environnement, table des descripteurs - basculent alors sous une propriété que ses voisins de même identité ne peuvent pas lire. Cela compte surtout dans le repli sans Landlock, par exemple sur un vieux noyau de NAS, où une extension pourrait autrement récupérer la clé JWT en mémoire et les secrets d'environnement. Sur un noyau avec Landlock, l'accès est déjà refusé ; c'est la ceinture et les bretelles, valables sur tous les noyaux. Cela n'empêche pas le serveur de lire ses propres fichiers, et un échec est journalisé sans être fatal.

## Modes de bac à sable

Le mode se configure par fichier ou par variable d'environnement, et vaut `auto` par défaut. Il choisit le **mécanisme** de confinement, et les appels système comme la sortie réseau dépendent ensuite de ce mécanisme : les modes `landlock` et `basic` passent l'extension par l'amorce, qui installe la liste noire d'appels système et le refus des sockets internet ; les modes `bubblewrap` et `strict` isolent plutôt le réseau par un espace de noms et n'ont pas de liste noire venue de l'amorce, `strict` n'ajoutant que la couche préchargée. Ce qui s'applique dans **tous** les modes : la séparation d'uid, le moniteur de ressources, et le fait que **la sortie IP directe est coupée**, une extension réseau ne joignant l'extérieur que par le relais de l'hôte.

### `auto` (par défaut)

Résout le meilleur mécanisme que la machine gère réellement, dans l'ordre **landlock, bubblewrap, basic**.

- Il **préfère landlock** : aucun privilège nécessaire, fonctionnement sous les seccomp et AppArmor par défaut de Docker sans rien changer sur la machine, et impossibilité de le relâcher une fois appliqué. C'est le bon défaut pour le déploiement en conteneur.
- Il ne retombe sur **bubblewrap** que là où Landlock est absent *et* où un bac à sable jetable se construit réellement, c'est-à-dire hors conteneur ou dans un conteneur privilégié.
- Sinon **basic**. Le mode retenu est journalisé au premier démarrage d'extension.

### `landlock`

- **Ce qu'il confine :** les fichiers, par une liste blanche appliquée par l'amorce - répertoires système en lecture et exécution, répertoire de l'extension et sa socket en écriture, tout le reste refusé, notamment le répertoire de données et les répertoires des autres extensions. Côté réseau, **aucune sortie IP directe pour aucune extension** ; celles qui ont la permission passent par le relais de l'hôte. S'y ajoutent les deux listes noires d'appels système.
- **Ce qu'il exige :** un noyau 5.13 ou plus avec Landlock actif, et le binaire d'amorce. Ni montage, ni espace de noms, ni privilège : il tourne proprement sous le profil de sécurité par défaut de Docker.
- **Quand il est retenu :** quand le mode est imposé, auquel cas une extension est refusée si Landlock ne peut pas être appliqué, ou quand le mode automatique s'y résout, le cas normal de l'image officielle.
- **Ses limites :** la liste par hôte vit désormais côté hôte, dans le relais, et non dans le noyau. Le confinement des fichiers, lui, tient dans tous les cas.

### `bubblewrap`

- **Ce qu'il confine :** les fichiers, le réseau et la vue des processus, par six espaces de noms non partagés, le réseau l'étant inconditionnellement pour **toute** extension, et par une liste blanche de montages : répertoires système en lecture seule, répertoire de l'extension et sa socket en écriture, un `/proc` synthétique, un `/dev` minimal et un `/tmp` en mémoire. Le répertoire de données, `/var`, `/home`, `/root` et les répertoires voisins ne sont tout simplement pas montés. Ce mode n'invoque jamais l'amorce : sa liste noire d'appels système ne s'applique donc **pas** ici, et la couche préchargée non plus, réservée à `strict`. Le confinement vient des seuls espaces de noms.
- **Ce qu'il exige :** le binaire bubblewrap ainsi que les appels de montage et d'espaces de noms ; dans Docker cela suppose une sécurité relâchée, ce qui le réserve en pratique à une machine sans conteneur ou à un conteneur privilégié.
- **Ses limites :** l'espace de noms cgroup non partagé est un espace de noms, pas une limite de ressources, celles-ci venant du moniteur. Aucune liste noire d'appels système ne s'applique dans ce mode.

### `strict`

Identique à `bubblewrap`, **plus** la couche préchargée par-dessus. Mêmes exigences et mêmes limites. Une véritable liste blanche d'appels système reste au programme.

### `basic`

- **Ce qu'il confine :** la bascule d'identité, la liste noire d'appels système du noyau, le refus des sockets internet et un Landlock au mieux, le tout apporté en enveloppant l'extension dans l'amorce, présente dans l'image officielle. Sans elle, sur certaines images de développement, il dégrade vers la seule couche préchargée et la bascule d'identité.
- **Quand il est retenu :** quand le mode est imposé, ou quand le mode automatique n'a ni Landlock ni bubblewrap utilisable.
- **Ses limites :** c'est le mode le plus faible pour le confinement des *fichiers*. Sans Landlock, il n'y a **aucun confinement par chemin** : une extension peut lire les fichiers non sensibles lisibles par tous, et la liste des processus est visible. Ce n'est **pas** « aucune isolation » : les secrets du serveur restent protégés par les permissions et le marquage non capturable, l'extension n'a aucune sortie IP, les appels système dangereux sont bloqués au niveau du noyau, y compris pour les binaires compilés, et les ressources sont plafonnées par le moniteur.

### Résumé des postures

| Déploiement | Fichiers | Réseau | Appels dangereux | Secrets | Verdict |
|---|---|---|---|---|---|
| Docker avec Landlock | liste blanche par chemin | aucune sortie directe, tout passe par le relais | bloqués, binaires compilés compris | protégés | **forte** |
| Docker sans Landlock, vieux NAS par exemple | aucun, les fichiers lisibles par tous le sont | aucune sortie IP | bloqués, binaires compilés compris | protégés | correcte, sans équivaloir à Landlock |
| Hors conteneur, en bubblewrap | espaces de noms et liste blanche de montages | espace réseau isolé | isolation par espaces de noms ; couche préchargée en `strict`, le filtre noyau de l'amorce ne s'appliquant pas ici | protégés | **la plus forte** |

## Limites connues du bac à sable

| Limite | Portée | Ce qui l'atténue aujourd'hui | Feuille de route |
|---|---|---|---|
| Aucun confinement des fichiers par chemin en mode `basic` | une extension peut lire les fichiers non sensibles lisibles par tous, et les répertoires des extensions voisines sont visibles | les secrets du serveur restent protégés par les permissions et le marquage non capturable ; l'analyseur signale les motifs d'accès aux fichiers sensibles ; le mode automatique préfère Landlock ou bubblewrap | aucune : Landlock ou bubblewrap sont la réponse, et tous deux sont déjà livrés |
| Toutes les extensions partagent la même identité | une extension malveillante peut envoyer des signaux à ses voisines, la signalisation du serveur étant bloquée | le marquage non capturable ferme la lecture croisée des fichiers de processus ; Landlock et bubblewrap isolent complètement les répertoires | un espace de noms utilisateur par extension, via bubblewrap |
| La liste d'appels système est une liste noire | lire, écrire, ouvrir et communiquer localement restent libres ; le traçage, les montages, les espaces de noms, le chargement de modules, la famille setuid, le trousseau et les sockets internet sont bloqués au niveau du noyau, binaires compilés compris | la liste noire installée par l'amorce avant l'exécution, plus la barrière de la permission `subprocess` | un vrai mode en liste blanche pour `strict` |
| Le relais déplace la surface d'attaque vers l'hôte | un défaut dans sa validation d'URL ou d'adresse laisserait une extension joindre une adresse interne par l'hôte | le relais applique la liste des hôtes déclarés et un client qui refuse les adresses privées et réservées au moment de la connexion ; les administrateurs devraient ajouter un pare-feu de sortie sur le conteneur, comme couche indépendante | des règles de filtrage par espace de noms, ou un relais de sortie dédié |
| L'analyseur est contournable par obfuscation | un attaquant motivé peut dissimuler ses motifs | la prévisualisation et la revue des permissions par l'administrateur ; la frontière d'application reste le bac à sable, pas l'analyseur | une notation de risque côté serveur et une vérification de réputation de l'auteur |

## Réglage du mode

Le mode se configure par fichier ou par variable d'environnement ; la section précédente détaille chaque mode.

```yaml
sandbox_mode: auto   # default - resolves landlock → bubblewrap → basic for this host
# sandbox_mode: landlock    # Landlock LSM allowlist + no-IP-egress (-deny-inet) + seccomp (fails closed if Landlock absent)
# sandbox_mode: basic       # UID drop + kernel/LD_PRELOAD seccomp + -deny-inet + best-effort Landlock
# sandbox_mode: bubblewrap  # bwrap namespace isolation (bare-metal / privileged only)
# sandbox_mode: strict      # bubblewrap + LD_PRELOAD seccomp lib
```

La séparation d'uid est un réglage **distinct**, qui décide si les extensions tournent sous une identité différente. En automatique, elle ne s'active qu'en mode `basic` ; forcée, elle s'applique partout comme couche indépendante supplémentaire. Elle est portée par l'amorce, fermée par défaut en `basic` (au mieux seulement sous `landlock`), et incompatible avec l'option qui bloque l'acquisition de privilèges. Un guide par situation se trouve dans [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md).

Ce que chaque couche donne selon le mode, les valeurs reflétant toujours ce que la machine permet :

| Couche | auto | landlock | basic | bubblewrap | strict |
|---|---|---|---|---|---|
| Séparation d'uid | active en `basic` seulement | inactive, à forcer si voulu | active | par espace de noms utilisateur | par espace de noms utilisateur |
| Liste noire du noyau, avant exécution, binaires compilés compris | oui si résolu en `landlock` ou `basic`, non si résolu en `bubblewrap` | oui | oui | non, ce mode n'invoque pas l'amorce | non, `strict` n'ajoute que la couche préchargée |
| Couche préchargée, extensions interprétées | oui | oui | oui | non | oui |
| Refus des sockets internet | oui | oui | oui | sans objet, espace réseau isolé | sans objet, espace réseau isolé |
| Confinement des fichiers | le meilleur disponible | liste blanche Landlock | aucun, permissions seules | liste blanche de montages | liste blanche de montages |
| Réseau, aucune sortie directe dans tous les cas | sockets refusées | restriction et refus | sockets refusées | espace réseau isolé | espace réseau isolé |
| Espaces de noms | sans objet | sans objet | sans objet | non partagés | non partagés |
| Plafonds de ressources | moniteur et politesse d'ordonnancement | idem | idem | idem | idem |

En modes `bubblewrap` et `strict`, l'amorce n'est pas utilisée : c'est l'espace de noms utilisateur qui apporte l'isolation d'identité à la place de la bascule.

Dépendances d'implémentation : l'amorce, écrite en Go pur, livrée à côté du binaire du serveur et utilisée par les modes `landlock`, `auto` et `basic` ; le binaire bubblewrap, présent dans l'image d'exécution et utilisé par les modes `bubblewrap` et `strict` ; et, pour ce dernier, un noyau autorisant les espaces de noms utilisateur non privilégiés. Dans Docker, la mise en place des espaces de noms demande en général une sécurité relâchée, ce qui explique que le mode automatique préfère Landlock pour le déploiement en conteneur.

## Sortie réseau des extensions, par relais

Les extensions n'ont **aucun accès réseau direct** : le bac à sable refuse toute socket internet à chaque extension, quelle que soit sa permission. Celle qui a besoin du réseau appelle le relais de l'hôte par sa socket locale, appel conditionné à la permission `network`. L'hôte, en Go :

- vérifie que l'hôte visé figure sur la liste déclarée par l'extension, ce qui est désormais un vrai contrat par hôte et non une indication de résolution de noms ;
- restreint le schéma à `http` et `https`, et la méthode à `GET` et `HEAD` ;
- compose par un client protégé, qui refuse les adresses privées, réservées et de lien local **après** résolution DNS au moment de la connexion, de sorte que les redirections et les réattributions de noms sont revérifiées ;
- n'attache aucun identifiant du serveur, et plafonne la taille de la réponse.

**Durcissement recommandé côté administration.** Le relais déplace la surface d'attaque vers l'hôte : ajoutez donc une couche de sortie indépendante sur le conteneur du serveur, qui ne fait pas confiance au code. Bloquez la plage de lien local, qui porte les métadonnées d'infrastructure, et restreignez le sous-réseau interne aux seuls points de la base de données et de la recherche. Un défaut dans la validation du relais se heurte alors encore à un mur réseau, et les deux couches échouent de façons indépendantes.

**Ce que cette première version ne couvre pas** tenait à l'ancien mécanisme et n'a plus lieu d'être depuis que la sortie directe est coupée : les adresses IP littérales, les résolveurs de noms embarqués et les détours par le nom d'hôte présenté ne sont plus atteignables depuis le processus de l'extension. Ce qui demeure est le point ci-dessus : le relais contrôle la destination, pas le contenu.

**Conseils d'exploitation.** Traitez la permission réseau comme un audit manuel : vérifiez que l'extension ne s'adresse pas à des adresses en dur et n'embarque pas son propre résolveur. Et surveillez le trafic sortant de la machine qui héberge les extensions.

## Exposition réseau, hors du modèle de menace des extensions

Tout ce qui précède concerne les extensions. L'autre moitié de la surface d'attaque est de savoir qui peut joindre le serveur, et deux invariants méritent d'être énoncés ici, parce qu'un lecteur qui cherche comment exposer Evey sans risque commence par cette page.

- **Un seul point d'écoute est fait pour être publié, et ce n'est pas celui du gRPC.** Le port gRPC natif est un transport interne dont le seul client est la passerelle HTTP du même processus ; l'API HTTP est ce port plus un. Le point d'écoute gRPC se lie à la boucle locale par défaut, précisément parce que le verrou d'accès distant décrit ci-dessous est un intergiciel HTTP qui ne le couvre pas. L'élargir est possible et journalise un avertissement au démarrage ; le protéger par un pare-feu devient alors le travail de l'administrateur.
- **Le verrou d'accès distant décide « local ou distant » à partir de LA seule adresse cliente validée à la frontière des proxys de confiance, jamais d'un en-tête.** Un client ne peut pas se déclarer local par un en-tête pour être traité comme tel, et une requête qui n'a pas franchi cette frontière est classée distante, donc refusée, plutôt que crue sur parole. C'est un contrôle de *déploiement*, au sens « cette instance est pour mon réseau », et non une couche d'authentification : c'est l'authentification qui protège les données.

## Ce que les logs peuvent contenir

Le fichier de log du serveur est ce que servent les routes de logs de
l'administration (suivi, recherche, flux en direct), ce qui finit dans les
rapports de bug et ce qu'emporte une sauvegarde : les identifiants y sont donc
effacés **à l'écriture** (`internal/pkg/logger/redact.go`). Un attribut dont la
clé désigne un secret (`authorization`, `token`, `access_token`,
`refresh_token`, `api_key`, `password`, `pin`, `secret`, ...) est remplacé en
entier, et toute chaîne - le message compris - qui porte un bearer, un JWT ou
une paire `secret=valeur` est nettoyée. Les routes de logs de l'administration
appliquent la même règle à la sortie pour les lignes écrites avant que ce
nettoyage n'existe. Deux conséquences pour le code : une valeur non secrète ne
doit pas être journalisée sous un nom de clé secret (elle serait masquée -
`token_info` et `device_pin` sont les noms employés aujourd'hui), et un nouveau
type d'identifiant doit être ajouté à cette liste avant d'être journalisé une
seule fois.

## Signaler une vulnérabilité

Signalez en privé, jamais dans un ticket public : `security@eveyproject.org`. C'est l'adresse que donne la [politique de sécurité](../SECURITY.md) du dépôt, et elle couvre l'authentification, les permissions et les évasions de bac à sable des extensions comme le reste. Ne testez pas contre des instances Evey partagées ou publiques qui ne sont pas les vôtres.
