---
source_sha: 80123a8d7d69b50b13baa0a7c638767ebf54582aa06f7b85e176a6fee202d4ca
---

# Modèle de sécurité des extensions : ce qu'un auteur peut déclarer

> **Quelle page cherchez-vous ?** Celle-ci s'adresse aux auteurs d'extensions :
> le catalogue des permissions, les règles qu'un appel externe doit respecter,
> les emplacements où vous pouvez vous greffer, et ce qui se passe lors d'une
> mise à jour. Si vous administrez un serveur et voulez savoir comment une
> extension est confinée et ce que le bac à sable ne sait toujours pas empêcher,
> lisez plutôt [le bac à sable et le modèle de menace](../SECURITY.md).

Les extensions d'Evey sont du code tiers qui tourne dans des processus séparés, à côté du serveur. Cette page explique ce qu'une extension malveillante pourrait tenter, comment Evey se défend contre chaque vecteur, et les limites honnêtes de l'implémentation actuelle.

## Modèle de menace : ce qu'on confie à un manifeste

| Menace | Description |
|---|---|
| Exécution de code sur le serveur | L'extension s'échappe de son bac à sable et exécute des commandes arbitraires sous l'identité du serveur |
| Exfiltration de secrets | L'extension lit les clés JWT, les identifiants de base de données ou les jetons d'utilisateurs sur le disque |
| Escalade horizontale | L'extension se fait passer pour une autre sur le canal local afin d'appeler des RPC dont elle n'a pas la permission |
| Exfiltration réseau | L'extension ouvre des connexions sortantes pour envoyer des données captées vers un hôte contrôlé par l'attaquant |
| Déni de service, abus de ressources | L'extension épuise le processeur, la mémoire, les processus ou le disque |
| Hameçonnage dans l'interface | L'extension injecte une surcouche plein écran dans le navigateur pour voler des identifiants |
| Chaîne d'approvisionnement | L'auteur livre du code obfusqué ou compilé que l'analyse statique ne peut pas inspecter |

## Couches de défense

### 1. Validation du manifeste (à l'installation)

Chaque `plugin.yaml` est validé avant toute extraction. L'installation est refusée si :

- `kind` est absent ou vaut autre chose que `plugin` ou `theme` ;
- une extension ne déclare pas son langage ou son point d'entrée ;
- un thème déclare un langage, un point d'entrée ou des permissions ;
- un emplacement ne figure pas dans la liste blanche du serveur (voir [Liste blanche des emplacements](#slot-whitelist)) ;
- une entrée d'appel externe viole les règles d'hôtes (voir [Règles des appels externes](#external-fetch-rules)) ;
- une capacité ou une permission déclarée est inconnue.

Source de vérité : `internal/domain/plugin/model.go` et `internal/adapters/driven/pluginrunner/capabilities.go`.

### 2. Extraction sûre de l'archive

Avant qu'un seul fichier ne soit écrit sur le disque :

- au plus 5 000 entrées ;
- au plus 100 Mo par fichier ;
- au plus 500 Mo décompressés au total ;
- taux de compression plafonné à 100 pour 1, ce qui détecte les bombes zip ;
- refus de toute entrée dont le chemin résolu contient une traversée ;
- refus inconditionnel des liens symboliques ;
- retrait des bits setuid, setgid et collant sur tous les fichiers extraits.

Source : `internal/adapters/driven/pluginrunner/zip.go`.

### 3. Analyseur statique (à l'installation)

Chaque fichier texte de l'archive est passé au crible par un analyseur multi-langage fondé sur des expressions régulières, avant que l'extension n'entre en base. Une passe de normalisation désamorce d'abord les obfuscations courantes : résolution d'alias, repliement des concaténations, expansion des chaînes formatées.

| Catégorie | Gravité | Exemples |
|---|---|---|
| exécution de commandes | CRITIQUE | `os.system`, `subprocess.run`, `child_process.exec`, les équivalents PHP et Ruby |
| réseau | AVERTISSEMENT ou CRITIQUE | `requests.get`, `fetch(`, `net.Dial`, motifs de shell inversé |
| système de fichiers | AVERTISSEMENT ou CRITIQUE | `/etc/passwd`, `.ssh/`, `rm -rf`, manipulations de liens, `/proc/<pid>/environ` |
| obfuscation | CRITIQUE | `eval(`, décodage base64, séquences hexadécimales, chaînes de codes de caractères, `str_rot13` |
| privilèges | CRITIQUE | `setuid`, `chmod 4777`, `sudo`, signal envoyé au processus 1 |
| minage | CRITIQUE | `stratum+tcp`, `xmrig`, `cryptonight` |
| installation à l'exécution | CRITIQUE | `pip install`, `npm install`, un téléchargement redirigé vers un shell |
| import dynamique | CRITIQUE | `__import__()`, `importlib.import_module`, accès aux fonctions internes par nom |
| évasion du bac à sable | CRITIQUE | `vm.runInNewContext`, `process.binding`, `new Function(`, les détours par les sous-classes Python |
| désérialisation | CRITIQUE ou AVERTISSEMENT | `pickle.loads`, `marshal.loads`, `__reduce__` ; avertissement pour un chargement YAML sans chargeur explicite |
| code natif | CRITIQUE ou AVERTISSEMENT | `ctypes.CDLL`, `ffi.Library` ; avertissement pour les pointeurs non sûrs en Go |
| binaire compilé | AVERTISSEMENT devenant blocage | `.so`, `.dll`, `.exe`, `.wasm`, `.jar` : opaques à l'analyseur |
| archive imbriquée | AVERTISSEMENT devenant blocage | `.zip`, `.tar.gz` : peuvent contenir du code non analysé |
| type de fichier incohérent | AVERTISSEMENT traité comme critique | l'extension annonce une image ou une police, le contenu est exécutable |
| URL suspecte | AVERTISSEMENT | schéma non HTTP dans une URL littérale |
| dépendances non embarquées | AVERTISSEMENT | un fichier de dépendances déclaré sans modules embarqués |

Une trouvaille critique, ou un type de fichier incohérent, bloque l'installation.

Les binaires compilés et les archives imbriquées bloquent l'installation tant que le manifeste ne déclare pas la permission `native-binary`. L'administrateur voit l'avertissement et la permission à l'étape de prévisualisation, avant d'accepter.

Source : `internal/adapters/driven/pluginrunner/analyzer.go` et `analyzer_normalize.go`.

#### Au mieux, et non une garantie

L'analyseur travaille par motifs. Il désamorce les classes d'obfuscation connues mais **ne peut pas garantir la détection de tout contournement adverse**. Quelques classes couvertes : les appels PHP construits par concaténation, y compris en plusieurs étapes, avec résolution jusqu'à un point fixe ; la recherche de fonctions internes Python par nom ; l'évasion JavaScript par le constructeur de fonction, y compris quand la référence est stockée dans une variable ; et les concaténations fragmentées, où les fragments innocents sont suivis séparément et le verdict ne tombe que sur la chaîne finale.

Chaque classe de contournement a un test de non-régression permanent dans `tests/fixtures/plugins/malicious/`. Ajouter une classe suppose d'y committer un cas, d'étendre la règle ou la passe de normalisation, puis de basculer l'attente du test d'installation.

**Hors de portée, et c'est le bac à sable qui prend le relais :** les transformations dynamiques au niveau de l'arbre syntaxique, comme une chaîne inversée à l'exécution ou un décodage suivi d'une évaluation. L'analyseur ne peut pas les résoudre ; ce sont la liste blanche de fichiers, la liste noire d'appels système, le moniteur de ressources et l'authentification du canal local qui garantissent que l'extension ne s'échappe pas, n'exfiltre pas `/data` et ne dépasse pas ses permissions déclarées.

**Faux positifs connus :** l'analyseur voit aussi l'intérieur des commentaires. Une ligne de commentaire mentionnant une commande d'installation déclenchera la règle correspondante. C'est assumé : demander aux auteurs de nettoyer ces commentaires coûte moins cher qu'un retrait de commentaires par langage dans l'analyseur, et le blocage à l'exécution reste indépendant.

#### Confirmation empirique à l'exécution

La liste blanche de montages a été vérifiée de bout en bout contre une pile vivante, au moyen d'une extension témoin qui sonde sa propre vue du système de fichiers au démarrage :

```
ROOT_LIST count=9 dirs=bin,data,dev,etc,lib,proc,sbin,tmp,usr
STAT ok /data type=dir size=60 kids=1 sample=plugins
STAT_BLOCKED /var, /home, /root, /mnt, /media, /opt, /srv, /run (all ENOENT)
STAT_BLOCKED /etc/passwd, /etc/shadow, /data/jwt.key (all ENOENT)
```


Le répertoire `/data` apparaît non pas parce que celui de la machine est monté, ce qui n'arrive jamais, mais parce que bubblewrap synthétise les répertoires parents vides de la cible en écriture propre à l'extension : il ne contient donc que ce seul sous-répertoire. L'environnement du processus 1 est bien lisible, mais dans cet espace de noms le processus 1 est l'extension elle-même : lire son propre environnement n'est pas une fuite.

Relancez cette extension témoin après toute modification du montage pour confirmer que la disposition bloque toujours.

**La frontière d'application est le bac à sable, plus la liste noire d'appels système, le moniteur de ressources et l'authentification du canal local, pas l'analyseur.** Une extension qui ferait passer un motif critique ne peut toujours pas lire `/data`, appeler un appel système refusé, dépasser le plafond de processus, ni invoquer des méthodes pour lesquelles elle n'a aucune permission. L'analyseur est une couche parmi d'autres, utile pour faire apparaître du code manifestement malveillant à la prévisualisation, pas un périmètre.

### 4. Vérification de signature cosign

Réglable par la politique de signature :

| Politique | Comportement |
|---|---|
| `off` | La signature n'est pas vérifiée |
| `warn` (défaut) | Un échec de vérification est enregistré et montré à l'administrateur sans bloquer ; sans clé publique configurée, la vérification ne fait rien |
| `enforce` | Un échec de vérification bloque l'installation |

Source : `internal/adapters/driven/pluginrunner/cosign.go`.

### 5. Assainisseur CSS

Appliqué au `theme.css` à l'installation. L'ensemble des règles est décrit dans [le modèle de sécurité des thèmes](../themes/security-model.md). L'assainissement retire ou réécrit les constructions dangereuses plutôt que de rejeter le fichier entier, en consignant des avertissements.

### 6. Analyseur du JS de thème

Appliqué au `theme.js` à l'installation. Un motif reconnu refuse l'installation. La liste des motifs bloqués est dans [le modèle de sécurité des thèmes](../themes/security-model.md).

### 7. Bac à sable d'exécution (Landlock ou bubblewrap, seccomp, moniteur de ressources)

> **Le mode par défaut est `landlock`** : liste blanche de fichiers au niveau du
> noyau, coupure de toute sortie IP directe pour toute extension, et liste noire
> d'appels système, le tout sous les seccomp et AppArmor par défaut de Docker,
> sans aucune modification de la machine. Le tableau complet par mode est dans
> [SECURITY.md](../SECURITY.md).

Les couches ci-dessous s'appliquent selon ce que la machine permet. L'amorce de lancement, et la liste noire d'appels système qu'elle installe, ne servent qu'en modes `landlock` et `basic` : les modes `bubblewrap` et `strict` passent par un autre chemin qui ne l'invoque jamais et s'appuient sur l'isolation par espaces de noms.

- **Séparation d'uid** : quand elle est active, chaque extension bascule vers un compte distinct de celui du serveur, au moyen d'une capacité portée par l'amorce, le serveur restant non privilégié. Le mode automatique ne l'active qu'en mode `basic`, où les permissions de fichiers sont le seul mur à secrets. Active, elle rend la clé JWT et les répertoires de clés, de journaux et de sauvegardes illisibles par simple permission, même sans Landlock.
- **Liste noire d'appels système au niveau du noyau**, installée par l'amorce avant l'exécution, ce qui la rend applicable **aux binaires compilés aussi**. Elle bloque le traçage de processus, les montages, les espaces de noms, le chargement de modules, la famille setuid, les trousseaux et l'envoi de signaux au processus 1, et elle est héritée par les enfants.
- **Refus des sockets internet** pour **toute** extension, avec ou sans permission réseau. Les sockets locales restent autorisées pour le canal avec l'hôte.
- **Refus de création de processus** pour toute extension sans la permission `subprocess`. La création de fils d'exécution n'est pas touchée, si bien que les environnements Node, Python et Go fonctionnent normalement.
- **Le serveur se marque non capturable**, si bien qu'une extension partageant son identité ne peut pas lire sa mémoire ni son environnement, où vivent la clé JWT et les mots de passe.
- **Moniteur de ressources en espace utilisateur** : voir plus bas, les cgroups ne sont pas utilisés.

Ce que le **mécanisme de fichiers et de réseau** ajoute dépend du mode :

| Mode | Confinement des fichiers | Réseau | Retenu quand |
|---|---|---|---|
| `landlock` | Liste blanche du noyau : système en lecture seule, répertoire de l'extension et sa socket en écriture, `/data` et les voisines refusées | Toute sortie IP coupée | Mode imposé, ou mode automatique sur un noyau qui gère Landlock, le cas normal de l'image officielle |
| `bubblewrap` | Liste blanche de montages dans des espaces de noms non partagés | Réseau non partagé, pour toute extension | Mode imposé, ou automatique là où Landlock manque mais où la sonde bubblewrap passe |
| `strict` | Comme `bubblewrap`, plus la couche préchargée | Comme `bubblewrap` | Mode imposé |
| `basic` | Aucun ; seules les permissions de fichiers jouent | Sockets internet refusées | Mode imposé, ou automatique quand ni Landlock ni bubblewrap ne sont utilisables |

Le mode automatique se résout **landlock, puis bubblewrap, puis basic**, une fois au premier démarrage d'extension, et journalise son choix. Le mode `basic` n'est **pas** « aucune isolation » : la séparation d'uid, les deux couches d'appels système, le refus des sockets internet, un Landlock au mieux et le moniteur de ressources s'appliquent toujours. Il perd seulement le confinement des fichiers par chemin.

En mode `bubblewrap` ou `strict`, chaque processus d'extension tourne dans des espaces de noms non partagés : utilisateur, processus, nom de machine, communication inter-processus, cgroup et réseau, ce dernier l'étant **toujours**, quelle que soit la permission réseau.

Le bac à sable monte une **liste blanche** de chemins système en lecture seule, nécessaires à l'exécution - interpréteurs, bibliothèques partagées, magasin de certificats, résolution de noms - sans jamais exposer de chemin de données. Il n'y a **aucun** montage de la racine entière : `/data`, `/var`, `/home`, `/root` et les répertoires des autres extensions n'existent tout simplement pas dans l'espace de noms.

| Chemin | Accès | Raison |
|---|---|---|
| `/usr`, `/bin`, `/sbin` | Lecture seule | Interpréteurs et binaires système |
| `/lib`, `/lib64` | Lecture seule, si présents | Bibliothèques partagées |
| Certificats, résolution de noms, fichiers de version | Lecture seule, si présents | TLS et détection d'environnement. **Pas** `/etc` en entier : ni `/etc/passwd`, ni `/etc/shadow`, ni `/etc/ssh` ne sont montés. |
| Répertoire de l'extension | Lecture et écriture | Son propre répertoire d'installation |
| Répertoire de sa socket | Lecture et écriture | Le canal avec l'hôte |
| `/proc` | Montage dédié | Requis par les environnements d'exécution |
| `/dev` | Synthétique | Jeu minimal de périphériques, aucun périphérique physique |
| `/tmp` | Système de fichiers en mémoire | Espace de travail éphémère ; le `/tmp` de la machine n'est pas visible |

Source : `buildBwrapCmd` dans `internal/adapters/driven/pluginrunner/runner_bwrap.go`.

**Le moniteur de ressources**, par extension, applique les plafonds depuis l'espace utilisateur et non par cgroups, inutilisables dans un conteneur non privilégié. Il interroge `/proc` toutes les deux secondes environ et arrête l'extension au-delà d'un plafond ; la politesse d'ordonnancement est appliquée au lancement pour qu'une extension ne prive jamais le serveur de processeur. Les plafonds portent sur la mémoire résidente, le nombre de processus et de fils, la priorité processeur, et la taille du répertoire d'installation, mesurée toutes les trente secondes environ.

Voir [ADR-0013](../adr/0013-userspace-plugin-resource-monitor.md) pour le raisonnement derrière l'abandon des cgroups. À noter : l'espace de noms cgroup non partagé donne à l'extension une hiérarchie vierge mais **n'impose aucune limite** ; celles-ci viennent du moniteur.

### 8. Liste blanche DNS (retirée)

La liste blanche par fichier d'hôtes décrite dans les versions antérieures de ce document a été **retirée**. Elle a été remplacée par un mécanisme plus strict : la sortie IP directe est coupée entièrement pour toute extension, et celles qui ont la permission réseau ne joignent l'extérieur que par le relais de l'hôte, qui applique la liste des hôtes déclarés et un client protégé contre les requêtes vers l'intérieur.

### 9. Authentification du canal local par identifiants de pair

L'hôte expose une socket Unix par extension. Chaque appel entrant est authentifié en lisant l'identifiant de processus du pair auprès du noyau, valeur qu'un programme ne peut pas falsifier, puis en le comparant à celui enregistré au lancement.

Deux conséquences : une extension ne peut pas se faire passer pour une autre, les en-têtes d'identité étant ignorés au profit de la socket ; et une extension qui lance un enfant ne peut pas faire transiter ses appels par lui, l'identifiant de processus étant différent.

Source : `internal/adapters/driving/pluginhost/`.

### 10. Isolation de la partie frontale

Les modules frontaux tournent dans une iframe isolée, sans accès à la même origine, sans soumission de formulaire et sans fenêtre surgissante, avec une politique de sécurité de contenu qui n'autorise à joindre que les hôtes déclarés dans le manifeste. S'y ajoutent l'origine des messages vérifiée côté hôte, une limite de trois notifications par seconde plafonnées à 200 caractères sans HTML, une liste blanche de chemins de navigation qui refuse les zones d'administration, d'authentification, d'installation et de sécurité du profil, un redimensionnement borné entre 50 et 800 pixels, et la liste blanche des emplacements appliquée à la fois à l'installation et au rendu.

## Catalogue des permissions

| Permission | Ce qu'elle accorde |
|---|---|
| `network` | L'extension peut sortir sur le réseau. **Doit** s'accompagner d'une liste d'hôtes externes non vide, faute de quoi l'installation est refusée. |
| `subprocess` | L'extension peut lancer des processus enfants. Sans elle, deux couches s'y opposent : la couche préchargée bloque l'exécution pour les extensions interprétées, et l'amorce refuse la création de processus au niveau du noyau. Les fils d'exécution continuent de fonctionner dans les deux cas. |
| `native-binary` | L'extension peut livrer des binaires ou des archives imbriquées. L'administrateur voit ce code opaque et doit l'accepter à la prévisualisation. |
| `filesystem` | Intention déclarée d'accéder au système de fichiers au-delà de son propre répertoire. Constatée à l'audit, pas encore imposée par le bac à sable (voir les limites connues). |
| `media.read` | Lire les métadonnées des médias et accéder aux flux. |
| `media.write` | Créer ou modifier des enregistrements de médias. |
| `api.library` | Lister et rechercher des bibliothèques, déclencher des scans. |
| `api.user` | Appeler les RPC propres à un utilisateur : lire son profil et ses préférences. |
| `api.metadata` | Rechercher et écrire des données d'enrichissement. |
| `notification.sender` | Publier des événements de notification. |
| `webhook.receiver` | S'enregistrer comme destinataire de webhook. |
| `hardware.gpu` | **La permission la plus risquée, réservée à l'administration et aux extensions de première partie.** Elle donne accès au nœud de rendu graphique de la machine : une surface d'attaque de pilote noyau et une frontière de mémoire graphique PARTAGÉE avec le transcodeur, où une fuite de mémoire entre processus est possible sur certains matériels et où un blocage du processeur graphique peut arrêter le transcodage. Sans cette permission, les périphériques graphiques sont exclus de la liste blanche, quelles que soient les permissions de fichiers. L'installeur affiche un avertissement critique bien visible : ne l'accordez qu'à des extensions de confiance. |

## Règles des appels externes

Déclarées dans `external_fetch` du manifeste, et vérifiées à l'installation par `validateExternalFetch` (`internal/adapters/driven/pluginrunner/manifest.go`). C'est tout ou rien : une seule entrée invalide refuse l'installation entière.

Une entrée est un **nom d'hôte nu**. Un préfixe de schéma est toléré puis écarté : seul le nom d'hôte est conservé.

| Entrée | Valide | Raison |
|---|---|---|
| `api.tmdb.org` | Oui | Nom d'hôte exact, la forme normale |
| `https://api.tmdb.org` | Oui | Le schéma est analysé puis retiré, le nom d'hôte est gardé |
| `*.tmdb.org` | Non | Les caractères génériques sont refusés : listez chaque sous-domaine nécessaire |
| `*` | Non | Même règle |
| `192.168.1.1`, `::1` | Non | Les adresses IP littérales sont refusées, dans les deux familles |

À l'exécution, le relais de l'hôte ajoute ses propres limites, que le manifeste ne peut pas élargir : `http` et `https` seulement, `GET` et `HEAD` seulement, l'hôte visé comparé sans tenir compte de la casse à votre liste, et les adresses privées ou réservées refusées après résolution DNS, les redirections étant revérifiées contre les deux règles.

> Le comparateur du relais sait aussi lire une forme avec caractère générique,
> mais l'installeur refuse une telle entrée : elle ne peut donc jamais
> l'atteindre. Écrivez des noms d'hôtes explicites.

## Liste blanche des emplacements

Une extension ne peut injecter de l'interface que dans des emplacements explicitement autorisés. Les chemins critiques en sont volontairement absents.

| Emplacement | Remarques |
|---|---|
| `home.section` | Zone de contenu de la page d'accueil |
| `home.hero` | Bandeau principal de l'accueil |
| `library.sidebar` | Panneau latéral des vues de bibliothèque |
| `media.detail.sidebar` | Panneau latéral du détail d'un média |
| `player.overlay` | Superposition du lecteur |
| `settings.user.appearance` | Section apparence des réglages utilisateur |
| `sidebar.bottom` | Bas de la barre de navigation |
| `admin.nav` | Une entrée de menu dans le panneau d'administration, ouvrant la page isolée de l'extension ; elle n'injecte rien dans les écrans d'administration existants |

**Jamais autorisés :** les emplacements d'administration autres que `admin.nav`, ceux de connexion, d'installation et de sécurité du profil. Ces chemins sont critiques pour l'authentification et les privilèges, et y injecter du code est un vecteur d'élévation ou d'hameçonnage.

Le contenu d'`admin.nav` reste isolé comme n'importe quel autre : il s'affiche dans une iframe sans accès à la même origine et sans droit de connexion sortante. C'est du contenu tiers affiché dans la zone d'administration, pas une capacité d'administration accordée à l'extension.

Source : `internal/domain/plugin/slots.go`.

## Circuit de mise à jour

1. L'administrateur envoie une nouvelle archive ; la prévisualisation exécute l'analyseur statique et la vérification de signature, puis renvoie un différentiel des permissions, emplacements et hôtes externes par rapport à la version installée.
2. Si ce différentiel n'est pas vide, l'administrateur doit l'acquitter explicitement avant que la mise à jour ne s'applique.
3. L'empreinte de la source est prise à la prévisualisation et revérifiée à l'application : l'archive analysée est bien celle qui s'installe.
4. Si la politique de signature est `enforce`, une signature qui ne correspond pas bloque la mise à jour même si la prévisualisation était passée.

## Limites connues du modèle de permissions

| Limite | Portée | Ce qui l'atténue aujourd'hui |
|---|---|---|
| Le mode automatique peut retomber sur `basic` sur une machine sans Landlock ni bubblewrap utilisable | Pas de confinement des fichiers **par chemin** : une extension peut lire des fichiers non sensibles lisibles par tous, et la liste des processus est visible | Les secrets du serveur restent protégés par la séparation d'uid et le marquage non capturable ; les appels système et le refus des sockets internet s'appliquent toujours ; le mode retenu est journalisé au premier démarrage |
| Toutes les extensions partagent la même identité | Une extension malveillante pourrait envoyer des signaux à ses voisines | Le marquage non capturable empêche la lecture croisée de la mémoire et de l'environnement ; Landlock et bubblewrap isolent complètement les répertoires entre eux |
| Le relais contrôle *où* un appel sortant peut aller, pas *ce qui* en sort | Une fois un hôte sur la liste, le relais n'inspecte pas le contenu : l'exfiltration vers un hôte autorisé reste possible | N'autorisez que des hôtes à qui vous confieriez ces données ; le relais bloque toujours les autres destinations et les adresses privées |
| La couche préchargée ne contraint que les extensions interprétées | Un binaire statique en mode `basic`, sans l'amorce, contourne entièrement cette liste noire | L'image officielle livre toujours l'amorce, dont la liste noire couvre aussi les binaires compilés ; ne faites pas tourner d'extension non fiable sur une image de développement qui en serait dépourvue |
| L'analyseur est contournable par une obfuscation avancée | Un attaquant motivé peut dissimuler ses motifs | La prévisualisation et la revue des permissions par l'administrateur ; la frontière d'application reste le bac à sable, le moniteur de ressources et l'authentification du canal local |

## Ce que la version 2 apportera

- Un pare-feu de sortie au niveau du conteneur, comme couche indépendante derrière le relais : c'est un durcissement d'exploitation, pas un trou du bac à sable, la sortie IP directe étant déjà entièrement coupée.
- Un mandataire de sortie pour une application au niveau de chaque requête.
- Un mode seccomp en liste blanche, n'autorisant qu'un jeu minimal d'appels système.
- La prise en charge des entrées avec caractère générique, par application requête par requête.
