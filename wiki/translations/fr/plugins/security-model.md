---
source_sha: e10872a44bc7a4594b78350969813e3dffa0a11bd28a8acc97dcfb617e5b019b
reviewed: true
---
# Permissions et emplacements { #permissions-and-slots }

Pour les auteurs de plugins : ce qu'un manifeste peut déclarer, ce que
l'installateur vérifie, et les règles qui s'appliquent à un plugin pendant
son exécution. Le catalogue ci-dessous est la liste dans laquelle choisir ;
la façon dont le serveur confine un plugin, et ce que ce confinement ne peut
pas empêcher, est décrite dans
[Sandbox des plugins et modèle de menace](../SECURITY.md).

## Ce qui est vérifié à l'installation { #what-is-checked-at-install }

Un plugin est refusé à l'installation quand :

- `kind` manque, ou ne vaut ni `plugin` ni `theme` ;
- un `kind: plugin` n'a pas de `language` ou d'`entrypoint`, ou un
  `kind: theme` les déclare, ou déclare des `permissions` ou des
  `capabilities` ;
- une permission ou une capacité est inconnue ;
- un module d'interface vise un emplacement hors de la
  [liste des emplacements](#slot-whitelist) ;
- une entrée de `external_fetch` enfreint les
  [règles d'accès externe](#external-fetch-rules), ou `network` est déclaré
  sans aucune entrée ;
- le ZIP enfreint les [limites d'extraction](../SECURITY.md#zip-extraction) ;
- l'analyse statique trouve un motif critique, ou un avertissement que
  l'installateur traite comme tel (désérialisation non sûre, obfuscation).
  Les binaires compilés, les archives embarquées et le code natif sont
  refusés sauf si le manifeste déclare `native-binary`.

Les commentaires sont retirés avant l'analyse : une ligne mise en commentaire
n'est pas signalée. Les catégories sont listées dans
[Sandbox des plugins et modèle de menace](../SECURITY.md#static-analysis).

Avant l'installation, l'administrateur voit les permissions et les capacités
déclarées, et ce que l'analyse a trouvé ; les emplacements, les hôtes et la
signature sont montrés à la mise à jour et dans le Tableau de bord sécurité. Rien de ce qu'un plugin
déclare n'est accordé en silence.

## Dans quelles conditions le plugin s'exécute { #what-the-plugin-runs-under }

Quoi que déclare le manifeste :

- le plugin voit son propre dossier, pas les données du serveur ;
- il n'a aucun accès direct au réseau : un plugin qui a la permission
  `network` atteint les hôtes de sa liste `external_fetch` par l'appel
  `Fetch` de l'hôte, et rien d'autre ;
- sans `subprocess`, il ne peut pas lancer de processus enfants (les threads
  fonctionnent) ;
- il tourne sous des limites de ressources que l'administrateur peut
  ajuster : 256 Mio de mémoire, 64 processus et threads, 100 Mio de disque
  pour son dossier, et une priorité processeur abaissée, par défaut.

La signature est facultative pendant le développement. Comment signer un
plugin, et la politique de signature qu'un serveur peut imposer :
[Écrire un plugin](dev-setup.md#7-sign-your-plugin).

## Catalogue des permissions { #permissions-catalog }

Une permission se déclare sous `permissions` ; toute autre valeur fait refuser
l'installation. Recevoir des webhooks (`webhook.receiver`) et fournir des
métadonnées (`metadata.provider`) sont des capacités, déclarées sous
`capabilities` : voir
[Référence de l'API des plugins](api-reference.md#capabilities).

| Permission | Ce qu'elle accorde |
|---|---|
| `network` | Le backend peut appeler `Fetch`. Doit être combinée avec une liste `external_fetch` non vide ; refusée à l'installation sans elle. Le processus lui-même n'a toujours aucun socket. |
| `subprocess` | Le backend peut créer des processus enfants. Sans elle, le filtre noyau du lanceur refuse `fork`, `vfork` et `clone` sans `CLONE_THREAD`, et la couche seccomp refuse `execve` pour les plugins interprétés. Les threads fonctionnent dans les deux cas. L'administrateur voit un avertissement dédié. |
| `native-binary` | Le paquet peut livrer des `.so`, `.dll`, `.exe`, `.wasm`, `.jar`, des archives embarquées ou du code natif que l'analyse ne sait pas lire. L'administrateur voit et doit accepter du code opaque. Obligatoire pour un plugin `go`. |
| `filesystem` | Intention déclarée de toucher au système de fichiers au-delà du dossier du plugin. Enregistrée et montrée à l'administrateur ; la sandbox n'accorde aucun accès hors du dossier du plugin, quoi qu'il soit déclaré. |
| `media.read` | `GetMediaItem` et `ListMediaItems` sur l'hôte : la fiche média complète. |
| `media.write` | Réservée aux appels d'écriture de métadonnées, non servis aujourd'hui. |
| `api.library` | Les mêmes appels de lecture des médias que `media.read`. |
| `api.user` | Réservée à `GetUserPreferences`, non servi aujourd'hui. |
| `api.metadata` | Réservée à `UpdateMetadata`, non servi aujourd'hui. |
| `notification.sender` | Réservée à `SendNotification`, non servi aujourd'hui. |
| `public` | L'interface peut lire une projection sûre des médias (identifiant, titre, type, année, résumé, affiche), jamais les chemins ni les flux. Voir [`evey.media.*`](api-reference.md#eveymedia-permission-public). |
| `prefs` | L'interface peut lire et écrire les préférences de l'utilisateur courant dans l'espace propre du plugin. Voir [`evey.prefs.*`](api-reference.md#eveyprefs-permission-prefs). |
| `hardware.gpu` | Le processus reçoit le noeud de rendu DRM de l'hôte (le vrai `renderD1xx`, jamais un noeud carte). Une surface d'attaque sur le pilote noyau et une frontière de mémoire GPU partagée avec le transcodeur : un GPU bloqué peut figer le transcodage. Sans elle, les noeuds GPU sont entièrement exclus de la liste Landlock ; avec elle, le lanceur ajoute une règle `IOCTL_DEV` limitée à ce seul fichier (Landlock ABI 5 ou plus ; sur un noyau plus ancien, l'accès en lecture et écriture suffit déjà à soumettre des commandes). Le Tableau de bord sécurité affiche un avertissement critique une fois le plugin installé. Ne l'accordez qu'à un plugin que vous feriez tourner hors sandbox. |

## Règles d'accès externe { #external-fetch-rules }

Déclarées dans `external_fetch: [...]` et vérifiées à l'installation
(`validateExternalFetch`). Tout ou rien : une seule entrée invalide refuse
toute l'installation.

Une entrée est un **nom d'hôte**. Un préfixe de schéma et un chemin final sont
retirés à l'analyse ; seul le nom d'hôte est conservé. Les hôtes correspondent
exactement et un joker `*.example.com` est refusé à l'installation. Une entrée
sans port n'autorise que les ports 80 et 443 ; `host:port` n'autorise que ce port.

| Entrée | Valide | Raison |
|---|---|---|
| `api.tmdb.org` | Oui | Nom d'hôte exact, la forme normale |
| `https://api.tmdb.org` | Oui | Schéma et chemin retirés à l'analyse, nom d'hôte conservé |
| `api.example.com:8443` | Oui | Cet hôte sur le port 8443 seulement |
| `*.tmdb.org` | Non | Les jokers sont refusés à l'installation ; listez chaque hôte |
| `*` | Non | Un joker seul est refusé |
| `192.168.1.1`, `::1` | Non | Les adresses IP sont refusées, dans les deux familles |

À l'exécution, le courtier de l'hôte ajoute ses propres limites, que le
manifeste ne peut pas élargir : `http` et `https` seulement, `GET` et `HEAD`
seulement, l'hôte cible comparé à votre liste sans tenir compte de la casse,
le port cible comparé à l'entrée (80 ou 443 sans port déclaré), les adresses privées ou
réservées refusées après résolution DNS, un corps plafonné à 8 Mio et une
requête à 20 secondes. Les redirections sont revérifiées contre la liste,
la règle de port et la règle d'adresse.

La même liste d'hôtes est le `connect-src` du cadre du plugin, préfixée de
`https://` : un widget peut appeler ces hôtes directement, en `https`
seulement.

## Liste des emplacements { #slot-whitelist }

Les plugins ne peuvent injecter des éléments d'interface que dans des emplacements
explicitement autorisés. Les chemins critiques en sont volontairement
absents.

| Emplacement | Authentification requise | Notes |
|---|---|---|
| `home.section` | Oui | Zone de contenu de la page d'accueil |
| `home.hero` | Oui | Zone de bannière |
| `library.sidebar` | Oui | Barre latérale des vues de bibliothèque |
| `media.detail.sidebar` | Oui | Barre latérale d'une fiche média |
| `player.overlay` | Oui | Surcouche du lecteur |
| `settings.user.appearance` | Oui | Section apparence des paramètres utilisateur |
| `sidebar.bottom` | Oui | Bas de la barre de navigation |
| `admin.nav` | Oui | Une entrée de menu dans le panneau d'administration qui ouvre la page isolée du plugin ; n'injecte rien dans les écrans d'administration existants |

**Jamais autorisés :** `admin.*` (sauf `admin.nav`), `login.*`, `setup.*`,
`profile.security.*`. Ces chemins sont critiques pour l'authentification ou
les privilèges, et une injection à cet endroit est un vecteur d'élévation de
privilèges ou d'hameçonnage.

Le contenu de `admin.nav` est isolé comme tout autre emplacement : il
s'affiche dans un `<iframe sandbox="allow-scripts">` sans
`allow-same-origin`, dans une page dont la politique est
`default-src 'none'`. C'est du contenu tiers affiché dans la zone
d'administration, pas une capacité d'administration accordée au plugin.

Le pont limite ce qu'un cadre peut demander à la page : `evey.toast` au plus
3 messages par seconde, 200 caractères, sans HTML ; `evey.navigate` seulement
`/home`, `/library`, `/media`, `/player`, `/settings` et `/search` ;
`evey.resize` borné à 50-800 px. Le cadre ne détient aucun jeton : l'hôte
émet un jeton de cinq minutes limité à (plugin, utilisateur) pour les appels
`public` et `prefs`, et le serveur refuse par défaut toute méthode que ce
jeton ne nomme pas.

## Déroulement d'une mise à jour { #update-workflow }

1. L'administrateur envoie un nouveau ZIP. `PreviewPluginUpdate` lance
   l'analyse statique et la vérification de signature, et renvoie la
   différence de permissions, de capacités, d'emplacements et
   d'`external_fetch` par rapport à la version installée.
2. Si la différence n'est pas vide, l'administrateur doit l'accepter
   avant que la mise à jour ne soit appliquée (`requires_ack = true` ; par la
   route d'envoi, `accept_permission_changes=true`).
3. L'empreinte du paquet est relevée à l'aperçu et vérifiée à l'application :
   le ZIP analysé est le ZIP installé.
4. La clé de signature est contrôlée pour la continuité : une mise à jour
   signée par une autre clé que la version installée, ou qui n'est plus
   signée, est refusée tant que l'administrateur n'a pas accepté le nouvel
   éditeur. Sous `EVEY_PLUGINS_COSIGN_POLICY=enforce`, une signature non
   fiable ou invalide à l'application bloque la mise à jour même si l'aperçu
   a réussi.

<!-- sources
manifest validation: internal/adapters/driven/pluginrunner/manifest.go - validateManifest, validateExternalFetch ; internal/domain/plugin/model.go - Validate ; capabilities.go - KnownPermissions, KnownCapabilities
escalated warnings, native-binary: internal/app/plugin/install.go - checkEscalatedWarnings, validateStagedPlugin
comments stripped before analysis: docs/SECURITY.md (Static analysis) ; internal/adapters/driven/pluginrunner/analyzer_normalize.go
subprocess, deny-fork, deny-inet, render-dev: cmd/evey-plugin-launcher/main.go ; i18n admin.plugins.subprocessWarning
resource defaults: internal/config/config.go - Plugins
permission per RPC: internal/adapters/driving/grpcapi/plugin_interceptor.go ; served or not: internal/adapters/driving/pluginhost/host_server.go
broker: internal/adapters/driving/pluginhost/fetch.go - Fetch, hostAllowed, maxFetchBytes, fetchTimeout
frame connect-src: frontend/packages/client/src/plugins/hostProtocol.ts - buildConnectSrc ; internal/adapters/driving/http/plugin_frame_handler.go - pluginFramePolicy
slots: internal/domain/plugin/slots.go - AllowedSlots
bridge limits: frontend/packages/client/src/plugins/hostProtocol.ts ; token: internal/app/plugin/token_service.go - PluginTokenTTL ; docs/adr/0012-plugin-token-deny-by-default.md
update workflow: internal/app/plugin/update.go - ApplyUpdate ; internal/app/plugin/install.go - resolveExistingPlugin ; proto/evey/v1/admin_plugin.proto - PreviewPluginUpdate, ApplyPluginUpdate
-->
