---
source_sha: bb7b744c4f03cee0ce9a6cdfdd4b96cd6e99f6d71eee21c50f601fc4595ae619
reviewed: true
---
# Référence de l'API des plugins

Ce document s'adresse aux développeurs externes qui écrivent un plugin Evey. Pour
les contraintes de sécurité appliquées aux plugins, voir
[`security-model.md`](security-model.md).

## Schéma du manifeste

Chaque ZIP de plugin doit contenir un `plugin.yaml` à sa racine.

```yaml
name: my-plugin              # obligatoire ; nom lisible
version: 1.2.3               # obligatoire ; semver
kind: plugin                 # obligatoire ; doit valoir "plugin" ("theme" pour un thème fait seulement de CSS/JS)
language: nodejs             # obligatoire pour un plugin ; nodejs | python | go | php | ruby | lua | bash | sh
entrypoint: node index.js    # obligatoire pour un plugin ; la ligne de commande complète que lance l'interpréteur :
                              # le premier élément est l'interpréteur, cherché dans le PATH ; les
                              # suivants sont le script et ses arguments, confinés aux chemins du ZIP.
                              # "language" ne sert qu'à vérifier que l'environnement d'exécution est
                              # disponible et à copier les stubs du SDK : il ne lance PAS l'entrypoint.

description: "..."           # facultatif
author: "..."                # facultatif
min_evey_version: "1.0.0"   # facultatif ; version minimale de l'hôte, en semver

permissions:
  - network                  # voir la section Permissions
  - media.read

external_fetch:              # obligatoire quand la permission "network" est déclarée
  - api.themoviedb.org       # voir les règles d'accès externe dans security-model.md
  - api.example.com          # nom d'hôte seul : ni schéma, ni joker, ni adresse IP littérale

capabilities:
  - metadata.provider        # voir la section Capacités

# Facultatif : un sous-dossier dont le contenu est servi comme fichiers statiques.
# Obligatoire quand des modules d'interface sont déclarés.
web_root: public/

# Facultatif : modules d'interface injectés dans l'interface de l'hôte.
frontend:
  - slot: home.section       # voir la liste des emplacements dans security-model.md
    src: public/front.js     # chemin relatif à la racine du ZIP ; doit être sous web_root

config_schema:               # facultatif ; objet JSON Schema qui décrit les clés de configuration du plugin
  api_key:
    type: string
    secret: true             # ou `type: password` : jamais renvoyé à aucun client, administrateur compris
    description: "TMDB API key"
```

Un champ marqué `secret: true` (ou de `type: password`) est stocké et transmis au
plugin comme d'habitude, mais toutes les routes de lecture (la publique comme le
panneau de configuration de l'administration) le renvoient masqué. Le panneau
d'administration garde la valeur stockée quand le masque est renvoyé tel quel :
enregistrer les autres champs n'efface donc jamais un secret.

**Règles de validation appliquées à l'installation :**

- `kind: plugin` exige `language` et `entrypoint`.
- `kind: theme` ne doit PAS déclarer `language`, `entrypoint` ni `permissions`.
- `web_root` est obligatoire quand `frontend` n'est pas vide.
- `frontend[].src` doit être sous `web_root`.
- `frontend[].slot` doit figurer dans la liste des emplacements du serveur.
- La permission `network` exige une liste `external_fetch` non vide.
- Toutes les entrées de `external_fetch` doivent passer le validateur de noms
  d'hôte.
- Toutes les capacités et permissions doivent être connues du serveur.

## Communication avec le serveur

Le processus du plugin et l'hôte Evey dialoguent en gRPC sur deux sockets de
domaine Unix, dont les chemins sont transmis dans l'environnement au démarrage :

```
EVEY_PLUGIN_ID=<plugin_id>
EVEY_PLUGIN_SOCK=/tmp/evey/plugins/<plugin_id>/plugin.sock   # votre processus y écoute et sert PluginService
EVEY_HOST_SOCK=/tmp/evey/plugins/<plugin_id>/host.sock       # votre processus y appelle HostService
```

L'hôte authentifie le processus du plugin sur sa socket par le PID que signale
le noyau (`SO_PEERCRED`) : aucun jeton n'est remis au processus lui-même.

Un JWT distinct, de courte durée (5 minutes, limité à `plugin_id` + `user_id`), est
émis à la demande par le RPC `IssuePluginToken` : c'est *l'iframe d'interface* du
plugin qui en récupère un pour authentifier ses propres appels de portée
`public`/`prefs`. Il n'a aucun rapport avec la socket ci-dessus et n'est jamais
remis au processus du plugin au démarrage.

Les définitions du service gRPC sont dans `proto/plugin/v1/plugin_api.proto`. Les
stubs de SDK propres à chaque langage sont générés à partir de ce proto et peuvent
être inclus dans le ZIP du plugin ou livrés à côté. Des stubs pré-générés sont
fournis pour plusieurs langages (`sdk/<langage>/`) ; pour en viser un qui n'est
pas fourni, voir
[dev-setup.md → Générer des stubs pour un autre langage](dev-setup.md#generating-stubs-for-another-language).

L'hôte applique les permissions au niveau des RPC : appeler un RPC que votre
manifeste n'a pas déclaré renvoie `PermissionDenied`.

### Stockage de données

Chaque plugin reçoit son propre stockage clé/valeur persistant, propre au plugin :
**aucune permission requise, un plugin possède ses propres données**. Servez-vous-en
pour des curseurs, des caches, des horodatages de dernière synchronisation, tout ce
dont le plugin doit se souvenir d'un redémarrage à l'autre. Les valeurs sont des
chaînes opaques (encodez vous-même en JSON s'il vous faut une structure).

| RPC de `HostService` | Rôle |
|---|---|
| `SetData(key, value)` | Crée ou remplace une valeur. |
| `GetData(key) → {value, found}` | Lit une valeur. |
| `DeleteData(key)` | Supprime une clé. |
| `ListDataKeys() → {keys}` | Liste les clés du plugin. |

L'identifiant du plugin est tiré de la socket authentifiée : un plugin ne peut
jamais lire ou écrire que son **propre** espace, jamais celui d'un autre. Quand le
plugin est désinstallé, tout son stockage est effacé automatiquement
(`ON DELETE CASCADE`) : aucune donnée périmée ne survit à sa suppression.

Ce stockage est global au plugin. Pour garder des données **par utilisateur** (par
exemple une préférence de l'utilisateur qui regarde), voir la permission `prefs`
plus bas : cette surface est atteinte depuis l'iframe du plugin, où un contexte
utilisateur existe.

## API de l'interface

Quand un plugin déclare un module `frontend`, le HTML du `srcdoc` de l'iframe
contient un script de pont qui expose `window.evey` au JavaScript du module.

Le module d'interface tourne dans un cadre isolé, d'origine opaque : il n'a
accès ni au DOM de la page hôte, ni à ses cookies, ni à son stockage, et ne
joint Evey que par `window.evey`. Son propre accès au réseau est limité par le
`connect-src` du cadre aux hôtes de sa liste `external_fetch`, en `https://` ;
un plugin sans cette liste ne peut ouvrir aucune connexion.

### `evey.navigate(path)`

Fait naviguer l'application hôte vers `path`. Le chemin doit commencer par
`/home`, `/library`, `/media`, `/player`, `/settings` ou `/search` ; tout le
reste (l'administration, la connexion, la configuration initiale, une adresse
absolue ou sans schéma) est ignoré sans message.

```js
evey.navigate("/library/movies");
evey.navigate("/media/abc123");
```

### `evey.toast(level, message)`

Affiche une notification éphémère dans l'interface de l'hôte.

- `level` : `"success"` | `"error"` | `"info"`
- `message` : texte brut, 200 caractères au plus, HTML retiré
- Limité à 3 notifications par seconde

```js
evey.toast("success", "Metadata enriched.");
evey.toast("error", "API key missing.");
```

### `evey.resize(height)`

Demande à l'hôte de redimensionner l'emplacement de l'iframe à `height` pixels.
Borné entre 50 et 800. `0` replie l'emplacement : un plugin qui n'a rien à montrer
ne prend aucune place sur la page.

```js
evey.resize(300);
evey.resize(0); // rien à dire : l'emplacement disparaît
```

### `evey.onInit(callback)`

Enregistre une fonction appelée une fois que l'hôte a envoyé au plugin son
contexte d'initialisation. Si l'hôte a déjà envoyé `init` avant l'appel à
`onInit`, la fonction est appelée tout de suite avec les données gardées en
mémoire.

```js
evey.onInit(function(data) {
  // data.context         - les valeurs de configuration du plugin (issues de config_schema)
  // data.locale          - la locale active, par exemple "en-US"
  // data.theme           - l'identifiant du thème actif
  // data.surface         - "web", "mobile" ou "tv" : là où le widget a atterri
  // data.suggestedHeight - home.section seulement : la hauteur (px) que l'hôte recommande à cet endroit
  // data.viewportHeight  - tv seulement : la hauteur du panneau en dp
});
```

L'hôte attend jusqu'à 5 secondes que le plugin signale `ready` (ce que fait
automatiquement le script de pont). Au-delà, l'iframe est démontée et une erreur
est signalée.

#### Hauteur

Un widget `home.section` s'ouvre à `data.suggestedHeight` sans rien faire : l'hôte
connaît la surface sur laquelle il vous a placé (une page de bureau, un téléphone,
un panneau de télévision) et recommande la hauteur qu'y a un widget d'accueil, pour
que tous les widgets d'un accueil aient la même taille. N'appelez `evey.resize()`
que si vous savez mieux ; l'hôte ne redimensionne ni ne met jamais à l'échelle
votre contenu dans votre dos.

#### Sur une télévision

L'application TV fait tourner votre widget dans une WebView, et quelques vérités
du navigateur cessent d'y être vraies :

- **Ne vous dimensionnez pas d'après votre propre fenêtre.** Dans la WebView,
  `window.innerHeight` est l'emplacement que l'hôte vient de vous accorder :
  raisonnement circulaire. Si vous vous redimensionnez, partez de
  `data.viewportHeight` (le panneau) ou de `data.suggestedHeight`, jamais de
  `innerHeight`.
- **Ni focus, ni pointeur.** Le widget ne reçoit jamais le focus de la croix
  directionnelle, et il n'y a pas de survol. Tout ce qui est interactif a sa place
  sur les surfaces web et mobile.
- **Éloignez le texte des bords.** Les téléviseurs rognent l'image (overscan) :
  laissez environ 5 % de la largeur et de la hauteur libres de chaque côté.
- **Testez le WebGL sur un vrai boîtier, en zoomant.** Un shader tourne sur le GPU
  du boîtier et son pilote, pas sur ceux de votre ordinateur. Un hachage en treillis
  qui multipliait son entrée par quelques centaines avant `fract()` dessinait des
  polygones plats au lieu de nuages sur un GPU Mali, alors qu'il s'affichait bien
  dans un navigateur ; gardez des coordonnées de bruit bornées (`mod()`) et hachez
  des valeurs bornées. `tests/plugins/weather-v2` est le widget de référence pour
  tout ce qui précède.

## Emplacements

| Emplacement | Où il apparaît |
|---|---|
| `home.section` | Section de contenu de la page d'accueil |
| `home.hero` | Zone de mise en avant de la page d'accueil |
| `library.sidebar` | Panneau latéral des vues de bibliothèque |
| `media.detail.sidebar` | Panneau latéral des pages de détail d'un média |
| `player.overlay` | Couche superposée au lecteur vidéo |
| `settings.user.appearance` | Sous-section Apparence des réglages de l'utilisateur |
| `sidebar.bottom` | Bas de la barre latérale de navigation principale |
| `admin.nav` | Une entrée de navigation dans le panneau d'administration, qui ouvre la page du plugin dans son propre cadre isolé |

`admin.nav` est le **seul** emplacement dans la zone d'administration. Il n'injecte
pas d'interface dans les écrans d'administration existants : il ajoute une seule
entrée de menu, qui ouvre la page de votre plugin dans une iframe isolée, sur une
route d'administration dédiée. Le demander apparaît dans les différences soumises
au consentement de l'administrateur à l'installation.

Tous les autres emplacements sous `admin.*`, `login.*`, `setup.*`,
`profile.security.*` sont refusés à l'installation.

## Capacités

Les capacités déclarent le rôle que le plugin remplit côté serveur. Elles
déterminent les RPC que l'hôte route vers le plugin et la façon dont il
l'enregistre dans son registre de services.

### `metadata.provider`

Le plugin peut fournir des métadonnées enrichies pour les médias. Il n'existe pas
de RPC d'enrichissement dédié : le `PluginService` qu'appelle l'hôte n'expose que
`Initialize` / `Shutdown` / `OnEvent` / `HealthCheck` / `GetCapabilities`
(`proto/plugin/v1/plugin_api.proto`). Un plugin `metadata.provider` reçoit les
événements utiles par `OnEvent` et déclare sa capacité par `GetCapabilities` ; il
n'implémente pas de méthode d'enrichissement à part.

Permission requise : `api.metadata`.

### `media.read`

Le plugin peut lire les fiches et les flux des médias. Correspond à la permission
`media.read` du manifeste.

### `media.write`

Le plugin peut créer ou modifier des fiches de médias. Correspond à la permission
`media.write`.

### `notification.sender`

Le plugin peut publier des notifications. L'hôte fait passer par le plugin les
événements de notification sortants. Correspond à la permission
`notification.sender`.

### `webhook.receiver`

Le plugin s'enregistre comme point d'arrivée de webhooks. L'hôte lui transmet en
gRPC les requêtes HTTP POST entrantes qui le concernent. Aucune permission
supplémentaire n'est requise au-delà de la déclaration de la capacité.

## Permissions

`public` et `prefs` sont des **permissions**, pas des capacités : elles ne
changent pas le rôle du plugin côté serveur, elles accordent à l'iframe du plugin
des portées de lecture et d'écriture supplémentaires. Déclarez-les sous
`permissions` dans le manifeste.

### `public`

Accorde à l'iframe du plugin un accès en lecture seule à des données non sensibles
du serveur : `GetPublicMediaItem` / `ListPublicMediaItems`, qui renvoient une
**projection sûre** (identifiant, titre, type, année, résumé, URL de l'affiche),
jamais de chemins de fichiers, de tailles, d'identifiants de fournisseurs ni de
flux. L'identité du serveur (son nom) est déjà publique et ne demande aucune
permission. Déclarez `public` dans `permissions` pour la demander.

### `prefs`

Accorde à l'iframe du plugin la lecture et l'écriture des préférences de
**l'utilisateur courant**, dans l'espace propre au plugin, via
`PluginUserDataService` (`GetPref` / `SetPref` / `DeletePref` / `ListPrefs`). Le
plugin et l'utilisateur sont liés par le jeton de l'iframe : un plugin ne peut
toucher que les données (son propre plugin, l'utilisateur courant), jamais celles
d'un autre plugin ni d'un autre utilisateur, jamais les champs d'identité ou de
sécurité. Les lignes sont supprimées en cascade avec le plugin ou l'utilisateur.
Déclarez `prefs` dans `permissions`.

> **Ce que vous ne pourrez jamais demander.** `admin.*`, `auth.*` (connexion,
> déconnexion, session), `system.*`, et les génériques `user.write` /
> `library.write` sont verrouillés : ils sont retirés de tout jeton et refusés à
> l'entrée de l'API, même si un manifeste tente de les déclarer. Un jeton de plugin
> à portée limitée est **refusé par défaut** sur l'API du serveur : il n'atteint
> que les méthodes que les permissions accordées ci-dessus débloquent
> explicitement.

## Exemples

`examples/plugins/` contient des plugins fonctionnels au format de manifeste
actuel : des points de départ en Node.js et Python (`nodejs-example`,
`python-example`), une source de métadonnées (`metadata-example`), un récepteur de
webhooks (`webhook-receiver`), un widget d'accueil (`weather`), la configuration et
les secrets d'un plugin (`config-demo`), le stockage de données avec les
permissions `public` / `prefs` et l'emplacement `admin.nav` (`data-demo`), et deux
thèmes. [dev-setup.md](dev-setup.md#3-scaffold-your-first-plugin) explique comment
partir de l'un d'eux.
