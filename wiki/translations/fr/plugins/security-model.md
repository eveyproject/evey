---
source_sha: 2f19c650c31baaff6c7e79f4cb38f11050f9c3959419e781d95e02a21affb7f8
reviewed: true
---
# Permissions et emplacements

Cette page s'adresse aux auteurs de plugins : ce qu'un manifeste peut déclarer,
ce qui est vérifié à l'installation d'un plugin, et les règles qui s'appliquent
à lui pendant son exécution. La façon dont le serveur confine un plugin, et ce
que ce confinement ne peut pas empêcher, est décrite dans
[Sandbox des plugins et modèle de menace](../SECURITY.md).

## Ce qui est vérifié à l'installation

Un plugin est refusé à l'installation quand :

- `kind` manque, ou ne vaut ni `plugin` ni `theme` ;
- un `kind: plugin` n'a pas de `language` ou d'`entrypoint`, ou un `kind: theme`
  les déclare, ou déclare des `permissions` ;
- une permission ou une capacité est inconnue ;
- un module d'interface vise un emplacement hors de la
  [liste des emplacements](#slot-whitelist) ;
- une entrée de `external_fetch` enfreint les
  [règles d'accès externe](#external-fetch-rules) ;
- le ZIP enfreint les limites d'extraction : plus de 5 000 fichiers, un fichier
  de plus de 100 Mo, plus de 500 Mo une fois décompressé, un taux de compression
  supérieur à 100:1, un chemin qui sort de l'archive, ou un lien symbolique ;
- l'analyse statique trouve un motif critique (exécution de shell, obfuscation,
  import dynamique, évasion de sandbox, changement de privilèges, minage...). Les
  binaires compilés et les archives embarquées sont refusés, sauf si le manifeste
  déclare `native-binary`.

L'analyse lit aussi les commentaires : une ligne `# pip install foo` dans un
commentaire est signalée comme du vrai code. Retirez ce genre de ligne avant de
publier. La liste complète des catégories est dans
[Sandbox des plugins et modèle de menace](../SECURITY.md#static-analysis).

Avant l'installation, l'administrateur voit les permissions, les emplacements et
les hôtes déclarés, et ce que l'analyse a trouvé.

## Dans quelles conditions le plugin s'exécute

Quoi que déclare le manifeste :

- le plugin voit son propre dossier, pas les données du serveur ;
- il n'a aucun accès direct au réseau : un plugin qui a la permission `network`
  atteint les hôtes de sa liste `external_fetch` par l'appel `Fetch` de l'hôte ;
- sans `subprocess`, il ne peut pas lancer de processus enfants (les threads
  fonctionnent) ;
- il tourne sous des limites de ressources que l'administrateur peut ajuster :
  256 Mio de mémoire, 64 processus et threads, 100 Mio de disque pour son dossier,
  et une priorité processeur abaissée, par défaut.

La signature est facultative pendant le développement. Comment signer un plugin,
et la politique de signature qu'un serveur peut imposer :
[Écrire un plugin](dev-setup.md).

## Catalogue des permissions

| Permission | Ce qu'elle accorde |
|---|---|
| `network` | Le plugin peut ouvrir des connexions réseau sortantes. Elle **doit** aller avec une liste `external_fetch` non vide : sans elle, l'installation est refusée. |
| `subprocess` | Le plugin peut lancer des processus enfants. Sans elle, deux couches s'appliquent : la cale seccomp chargée par LD_PRELOAD bloque `execve`/`execveat` pour les plugins interprétés, et l'option `-deny-fork` du lanceur bloque la création même de processus (`fork`/`vfork`/`clone` sans `CLONE_THREAD`) dans le noyau. Les threads fonctionnent dans les deux cas. (Le seccomp noyau du lanceur n'interdit pas `execve`, puisqu'il doit exécuter le plugin, mais tout enfant hérite du filtre noyau et du domaine Landlock.) |
| `native-binary` | Le plugin peut livrer des `.so`/`.dll`/`.exe`/`.wasm`/`.jar` ou des archives embarquées. L'administrateur voit ce code opaque dans l'aperçu d'installation et doit l'accepter. |
| `filesystem` | Déclare l'intention d'accéder au système de fichiers au-delà du dossier du plugin. Enregistrée et montrée à l'administrateur ; la sandbox n'accorde aucun accès aux fichiers hors du dossier du plugin, quelle que soit la déclaration. |
| `media.read` | Le plugin peut appeler les RPC `MediaRead` : lire les métadonnées des fichiers et accéder aux flux. |
| `media.write` | Le plugin peut appeler les RPC `MediaWrite` : créer et modifier des fiches de médias. |
| `api.library` | Le plugin peut appeler les RPC de gestion des bibliothèques : lister et chercher des bibliothèques, lancer des scans. |
| `api.user` | Le plugin peut appeler les RPC propres à l'utilisateur (lire son profil, ses préférences). |
| `api.metadata` | Le plugin peut appeler les RPC de métadonnées : chercher et écrire des données d'enrichissement. |
| `notification.sender` | Le plugin peut publier des événements de notification (push web, e-mail, webhook). |
| `webhook.receiver` | Le plugin peut s'enregistrer comme cible de webhook (il reçoit des HTTP POST de services externes). |
| `hardware.gpu` | **La permission la plus risquée : réservée à l'administrateur, pour les plugins officiels du projet.** Accorde le nœud de rendu DRM de l'hôte (`-render-dev`, le vrai `renderD1xx`, jamais un nœud de carte, jamais `/dev/dxg`/`/dev/nvidia*`/`/dev/kfd`) : une surface d'attaque sur le pilote noyau, et une frontière de mémoire GPU PARTAGÉE avec le transcodeur (fuite de mémoire entre processus possible sur les GPU AMD, Apple et Qualcomm ; un GPU bloqué peut figer le transcodage). Sans cette permission, les nœuds de périphérique GPU sont entièrement exclus de la liste de périphériques autorisés par Landlock (refusés quels que soient les droits DAC). Avec elle, le lanceur ajoute une règle Landlock `IOCTL_DEV` explicite, limitée à ce seul fichier (ABI v5 et plus) ; sur les ABI Landlock plus anciennes, les ioctl ne sont pas du tout gouvernés par Landlock, si bien que l'accès en lecture et écriture suffit déjà à soumettre des commandes au GPU. L'installeur affiche un avertissement CRITIQUE bien visible dans l'aperçu ; ne l'accordez qu'à des plugins officiels, de confiance. |

## Règles d'accès externe

Déclarées dans `external_fetch: [...]` dans le manifeste et vérifiées à
l'installation par `validateExternalFetch`
(`internal/adapters/driven/pluginrunner/manifest.go`). Tout ou rien : une seule
entrée invalide refuse toute l'installation.

Une entrée est un **nom d'hôte seul**. Un préfixe de schéma est toléré puis
retiré : seul le nom d'hôte est gardé.

| Entrée | Valide | Raison |
|---|---|---|
| `api.tmdb.org` | Oui | Nom d'hôte exact, la forme normale |
| `https://api.tmdb.org` | Oui | Schéma lu puis retiré, nom d'hôte conservé |
| `*.tmdb.org` | Non | Les jokers sont refusés : listez chaque sous-domaine nécessaire |
| `*` | Non | Même règle |
| `192.168.1.1`, `::1` | Non | Les adresses IP littérales sont refusées, dans les deux familles |

À l'exécution, l'intermédiaire de l'hôte ajoute ses propres limites, que le
manifeste ne peut pas élargir : `http` et `https` seulement, `GET` et `HEAD`
seulement, l'hôte visé comparé sans tenir compte de la casse à votre liste, et
les adresses privées ou réservées refusées après résolution DNS ; les
redirections sont revérifiées selon ces deux règles.

> L'intermédiaire sait aussi comprendre une forme `*.example.com`, mais
> l'installeur refuse une telle entrée, qui ne peut donc jamais lui parvenir.
> Écrivez des noms d'hôte explicites.

## Liste des emplacements

Un plugin ne peut injecter de l'interface que dans des emplacements explicitement
autorisés. Les parcours critiques en sont volontairement absents.

| Emplacement | Authentification requise | Remarques |
|---|---|---|
| `home.section` | Oui | Zone de contenu de la page d'accueil |
| `home.hero` | Oui | Zone de mise en avant |
| `library.sidebar` | Oui | Barre latérale de la vue d'une bibliothèque |
| `media.detail.sidebar` | Oui | Barre latérale de la page d'un média |
| `player.overlay` | Oui | Interface superposée au lecteur |
| `settings.user.appearance` | Oui | Section Apparence des réglages de l'utilisateur |
| `sidebar.bottom` | Oui | Bas du panneau de navigation |
| `admin.nav` | Oui | Une entrée de navigation dans le panneau d'administration, qui ouvre la page du plugin dans son propre cadre isolé ; n'injecte rien dans les écrans d'administration existants |

**Jamais autorisés :** `admin.*` (sauf `admin.nav`), `login.*`, `setup.*`,
`profile.security.*` : ces parcours sont critiques pour l'authentification ou les
privilèges, et y injecter un plugin serait un moyen d'élévation de privilèges ou
d'hameçonnage.

Le contenu de `admin.nav` reste isolé comme celui de tout autre emplacement : il
s'affiche dans une `<iframe sandbox="allow-scripts">` (sans `allow-same-origin`)
avec la CSP `default-src 'none'`. C'est donc un contenu tiers affiché dans la
zone d'administration, pas un pouvoir d'administration accordé au plugin.

Source : `internal/domain/plugin/slots.go`.

Les modules d'interface tournent dans une `<iframe>` isolée, sans accès à la page
hôte. Le pont limite ce qu'ils peuvent faire : `evey.toast`, au plus 3 messages par
seconde, 200 caractères, sans HTML ; `evey.navigate` refuse `/admin/*`,
`/auth/*`, `/setup/*` et `/profile/security/*` ; `evey.resize` est borné entre 50
et 800 px.

## Mise à jour d'un plugin

1. L'administrateur envoie un nouveau ZIP → `PreviewPluginUpdate` lance l'analyse
   statique et la vérification cosign, et renvoie les différences de
   permissions, d'emplacements et d'`external_fetch` par rapport à la version
   installée.
2. S'il y a des différences (nouvelles permissions, nouveaux emplacements, hôtes
   externes modifiés), l'administrateur doit les accepter explicitement avant que
   la mise à jour ne s'applique (`requires_ack = true`).
3. L'empreinte de la source est relevée à l'aperçu et vérifiée à l'application
   (protection contre la course entre vérification et utilisation : le ZIP
   analysé est bien celui qui est installé).
4. Si la politique cosign est `enforce`, une signature qui ne correspond pas à
   l'étape d'application bloque la mise à jour, même si l'aperçu était passé.
