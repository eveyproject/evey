---
source_sha: 29131cc868c2f0ed69e387e6774c9b511cfb19294dad329df8dbfc3d2c2c75c7
---

# Référence de l'API des extensions

Ce document s'adresse aux développeurs extérieurs qui écrivent une extension Evey. Pour les contraintes de sécurité imposées aux extensions, voir [`security-model.md`](security-model.md).

## Schéma du manifeste

Toute archive ZIP d'extension doit contenir un `plugin.yaml` à sa racine.

```yaml
name: my-plugin              # required; human-readable name
version: 1.2.3               # required; semver
kind: plugin                 # required; must be "plugin" (use "theme" for CSS/JS-only themes)
language: nodejs             # required for plugins; nodejs | python | go | php | ruby | lua | bash | sh
entrypoint: node index.js    # required for plugins; the full command line the interpreter runs -
                              # the first token is the interpreter, looked up on PATH; the
                              # remaining tokens are the script/args, path-confined to the ZIP.
                              # "language" is only used to check the runtime is available and to
                              # copy SDK stubs - it does NOT launch the entrypoint.

description: "..."           # optional
author: "..."                # optional
min_evey_version: "1.0.0"   # optional; semver minimum host version

permissions:
  - network                  # see Permissions section
  - media.read

external_fetch:              # required when "network" permission is declared
  - api.themoviedb.org       # see External fetch rules in security-model.md
  - api.example.com          # bare hostname only - no scheme, no wildcard, no literal IP

capabilities:
  - metadata.provider        # see Capabilities section

# Optional: declare a sub-directory whose contents are served as static assets.
# Required when frontend modules are declared.
web_root: public/

# Optional: frontend modules injected into the host UI.
frontend:
  - slot: home.section       # see Slot whitelist in security-model.md
    src: public/front.js     # path relative to ZIP root; must be under web_root

config_schema:               # optional; JSON Schema object describing plugin config keys
  api_key:
    type: string
    description: "TMDB API key"
```

**Règles de validation appliquées à l'installation :**

- `kind: plugin` exige `language` et `entrypoint`.
- `kind: theme` ne doit déclarer ni `language`, ni `entrypoint`, ni `permissions`.
- `web_root` est obligatoire dès que `frontend` n'est pas vide.
- `frontend[].src` doit se trouver sous `web_root`.
- `frontend[].slot` doit figurer dans la liste blanche d'emplacements du serveur.
- La permission `network` exige une liste `external_fetch` non vide.
- Toutes les entrées d'`external_fetch` doivent passer le validateur de liste blanche d'hôtes.
- Toutes les capacités et permissions doivent être connues du serveur.

## Communication avec le serveur

Le processus de l'extension dialogue avec l'hôte Evey en gRPC, sur une socket de domaine Unix. Le chemin de cette socket est transmis au démarrage dans la variable d'environnement `EVEY_PLUGIN_SOCK`.

L'hôte authentifie le processus de l'extension en vérifiant son PID par `SO_PEERCRED` juste après `cmd.Start` : aucun jeton n'est remis au processus lui-même.

```
EVEY_PLUGIN_SOCK=/tmp/evey/plugins/<plugin_id>/host.sock
EVEY_PLUGIN_ID=<plugin_id>
EVEY_HOST_SOCK=<path>          # socket the plugin can dial back to reach the host
```

Un JWT distinct et de courte durée (5 minutes, limité au couple `plugin_id` + `user_id`) est délivré à la demande par le RPC `IssuePluginToken` : c'est l'*iframe frontale* de l'extension qui en réclame un pour authentifier ses propres appels de portée `public` ou `prefs`. Il n'a aucun rapport avec la socket ci-dessus et n'est jamais remis au processus de l'extension au démarrage.

Les définitions du service gRPC sont dans `proto/plugin/v1/plugin_api.proto`. Les stubs de SDK propres à chaque langage sont générés depuis ce proto et peuvent être inclus dans le ZIP de l'extension ou embarqués à côté. Des stubs pré-générés sont livrés pour plusieurs langages (`sdk/<lang>/`) ; pour viser un langage absent de la liste, voir [dev-setup.md → Generating stubs for another language](dev-setup.md#generating-stubs-for-another-language).

L'hôte applique les permissions au niveau de chaque RPC : appeler un RPC que votre manifeste n'a pas déclaré donne un `PermissionDenied`.

### Magasin de données

Chaque extension dispose de son propre magasin clé/valeur persistant, cloisonné à son nom : **aucune permission n'est requise, une extension est propriétaire de ses données**. Servez-vous-en pour des curseurs, des caches, des horodatages de dernière synchronisation, tout ce que l'extension doit retenir d'un redémarrage à l'autre. Les valeurs sont des chaînes opaques (encodez vous-même du JSON si vous avez besoin de structure).

| RPC de `HostService` | Rôle |
|---|---|
| `SetData(key, value)` | Insère ou met à jour une valeur. |
| `GetData(key) → {value, found}` | Lit une valeur. |
| `DeleteData(key)` | Supprime une clé. |
| `ListDataKeys() → {keys}` | Liste les clés de l'extension. |

L'identifiant de l'extension est tiré de la socket authentifiée : une extension ne peut jamais lire ni écrire que **son propre** espace, jamais celui d'une autre. À la désinstallation, tout son magasin est effacé automatiquement (`ON DELETE CASCADE`), donc aucune donnée résiduelle ne survit à la suppression.

Ce magasin est global à l'extension. Pour persister des données **par utilisateur** (une préférence de l'utilisateur en cours de navigation, par exemple), voir la permission `prefs` ci-dessous : cette surface s'atteint depuis l'iframe de l'extension, là où un contexte utilisateur existe.

## API frontale

Quand une extension déclare un module `frontend`, le HTML du `srcdoc` de l'iframe embarque un script de pont qui expose `window.evey` au JavaScript du module.

Le module frontal n'a aucun accès direct au DOM de la page hôte, ni à `window`, `fetch`, `localStorage` ou n'importe quelle autre variable globale du navigateur, en dehors de ce que `window.evey` fournit explicitement.

### `evey.navigate(path)`

Fait naviguer l'application hôte vers `path`. Le chemin doit faire partie des routes autorisées ; les demandes vers `/admin/*`, `/auth/*`, `/setup/*` et `/profile/security/*` sont abandonnées en silence.

```js
evey.navigate("/library/movies");
evey.navigate("/media/abc123");
```

### `evey.toast(level, message)`

Affiche une notification éphémère dans l'interface hôte.

- `level` : `"success"` | `"error"` | `"info"`
- `message` : texte brut, 200 caractères au maximum, le HTML est retiré
- Limité à 3 notifications par seconde

```js
evey.toast("success", "Metadata enriched.");
evey.toast("error", "API key missing.");
```

### `evey.resize(height)`

Demande à l'hôte de redimensionner l'emplacement de l'iframe à `height` pixels. La valeur est bornée entre 50 et 800.

```js
evey.resize(300);
```

### `evey.onInit(callback)`

Enregistre une fonction de rappel déclenchée dès que l'hôte envoie à l'extension son contexte d'initialisation. Si l'hôte a déjà envoyé `init` avant l'appel à `onInit`, le rappel est invoqué immédiatement avec les données mises en cache.

```js
evey.onInit(function(data) {
  // data.context         - valeurs de configuration (config_schema)
  // data.locale          - locale active, ex. "fr-FR"
  // data.theme           - identifiant du thème actif
  // data.surface         - "web", "mobile" ou "tv" : où le widget a atterri
  // data.suggestedHeight - home.section uniquement : la hauteur (px) que l'hôte recommande là
  // data.viewportHeight  - tv uniquement : hauteur du panneau en dp
});
```

L'hôte attend jusqu'à 5 secondes que l'extension signale `ready` (le script de pont s'en charge tout seul). Passé ce délai, l'iframe est démontée et une erreur est remontée.

#### Hauteur

Un widget `home.section` s'ouvre à `data.suggestedHeight` sans rien faire : l'hôte sait sur quelle surface il vous a posé - une page de bureau, un téléphone, un panneau de télévision - et recommande la hauteur qu'un widget d'accueil a à cet endroit, si bien que tous les widgets d'un accueil ont la même taille. N'appelez `evey.resize()` que si vous savez mieux ; l'hôte ne redimensionne ni ne met à l'échelle votre contenu dans votre dos.

#### Sur une télévision

L'application télévision fait tourner votre widget dans une WebView, et quelques évidences du navigateur n'y tiennent plus :

- **Ne vous dimensionnez pas d'après votre propre viewport.** Dans la WebView, `window.innerHeight` est la hauteur que l'hôte vient de vous accorder - c'est circulaire. Si vous vous redimensionnez, partez de `data.viewportHeight` (le panneau) ou de `data.suggestedHeight`, jamais de `innerHeight`.
- **Pas de focus, pas de pointeur.** Le widget ne reçoit jamais le focus de la télécommande, et il n'y a pas de survol. Tout ce qui est interactif appartient aux surfaces web et mobile.
- **Éloignez le texte des bords.** Les télévisions rognent l'image (overscan) : laissez environ 5 % de la largeur et de la hauteur libres de chaque côté.
- **Testez le WebGL sur une vraie box, en zoomant.** Un shader tourne sur le GPU et le pilote de la box, pas sur ceux de votre poste. Un hachage de grille qui multipliait son entrée par quelques centaines avant `fract()` rendait des polygones plats à la place des nuages sur un GPU Mali alors qu'il était impeccable dans un navigateur ; gardez les coordonnées de bruit bornées (`mod()`) et hachez des valeurs bornées. `tests/plugins/weather-v2` est le widget de référence pour tout ce qui précède.

## Emplacements d'insertion

| Emplacement | Où il apparaît |
|---|---|
| `home.section` | Section de contenu sur la page d'accueil |
| `home.hero` | Bandeau principal de la page d'accueil |
| `library.sidebar` | Panneau latéral dans les vues de bibliothèque |
| `media.detail.sidebar` | Panneau latéral sur les pages de détail d'un média |
| `player.overlay` | Couche superposée au lecteur vidéo |
| `settings.user.appearance` | Sous-section Apparence des réglages utilisateur |
| `sidebar.bottom` | Bas de la barre latérale de navigation principale |
| `admin.nav` | Une entrée de menu dans le panneau d'administration, ouvrant la page isolée de l'extension |

`admin.nav` est le **seul** emplacement de la zone d'administration. Il n'injecte aucune interface dans les écrans d'administration existants : il ajoute une entrée de menu unique qui ouvre la page propre à votre extension, dans une iframe isolée, sur une route d'administration dédiée. Le demander apparaît dans le récapitulatif de consentement à l'installation.

Tous les autres emplacements sous `admin.*`, `login.*`, `setup.*` et `profile.security.*` sont rejetés au moment de l'installation.

## Capacités

Les capacités déclarent le rôle que l'extension remplit côté serveur. Elles déterminent quels RPC l'hôte lui achemine et comment il l'inscrit dans son registre de services.

### `metadata.provider`

L'extension peut fournir des métadonnées enrichies pour des éléments multimédias. Il n'existe pas de RPC d'enrichissement dédié : le `PluginService` que l'hôte appelle n'expose que `Initialize`, `Shutdown`, `OnEvent`, `HealthCheck` et `GetCapabilities` (`proto/plugin/v1/plugin_api.proto`). Une extension `metadata.provider` reçoit les événements pertinents par `OnEvent` et annonce sa capacité par `GetCapabilities` ; elle n'implémente pas de méthode d'enrichissement séparée.

Permission requise : `api.metadata`.

### `media.read`

L'extension peut lire les enregistrements et les flux des médias. Correspond à la permission `media.read` du manifeste.

### `media.write`

L'extension peut créer ou modifier des enregistrements de médias. Correspond à la permission `media.write`.

### `notification.sender`

L'extension peut publier des notifications. L'hôte fait transiter par elle les événements de notification sortants. Correspond à la permission `notification.sender`.

### `webhook.receiver`

L'extension s'inscrit comme point d'arrivée de webhook. L'hôte lui transmet en gRPC les requêtes HTTP POST entrantes correspondantes. Aucune permission supplémentaire n'est nécessaire au-delà de la déclaration de capacité.

## Permissions

`public` et `prefs` sont des **permissions**, pas des capacités : elles ne changent pas le rôle que l'extension remplit côté serveur, elles accordent à son iframe des portées de lecture et d'écriture supplémentaires. Déclarez-les sous `permissions` dans le manifeste.

### `public`

Donne à l'iframe de l'extension un accès en lecture seule aux données serveur non sensibles : `GetPublicMediaItem` et `ListPublicMediaItems`, qui renvoient une **projection sûre** (identifiant, titre, type, année, résumé, URL d'affiche), jamais de chemins de fichiers, de tailles, d'identifiants de fournisseur ni de flux. Le nom du serveur est déjà public et ne demande aucune permission. Déclarez `public` dans `permissions` pour l'obtenir.

### `prefs`

Donne à l'iframe de l'extension la lecture et l'écriture des préférences de **l'utilisateur courant**, dans l'espace propre à l'extension, via `PluginUserDataService` (`GetPref`, `SetPref`, `DeletePref`, `ListPrefs`). L'extension et l'utilisateur sont liés par le jeton de l'iframe : une extension ne peut donc toucher que le couple (sa propre extension, l'utilisateur courant), jamais les données d'une autre extension, ni d'un autre utilisateur, ni les champs d'identité ou de sécurité. Les lignes sont supprimées en cascade avec l'extension ou l'utilisateur. Déclarez `prefs` dans `permissions`.

> **Ce que vous ne pouvez jamais demander.** `admin.*`, `auth.*` (connexion, déconnexion, session), `system.*`, ainsi que `user.write` et `library.write` génériques sont verrouillés en dur : ils sont retirés de tout jeton et rejetés à la frontière de l'API, même si un manifeste tente de les déclarer. Un jeton d'extension limité est **refusé par défaut** sur l'API du serveur : il n'atteint que les méthodes que les permissions accordées ci-dessus débloquent explicitement.

## Exemples

Le dossier `examples/plugins/` ne contient pas encore d'exemples mis à jour pour le modèle actuel à deux genres. De nouveaux exemples de référence (`tmdb-enricher`, `discord-notify`, `library-stats`) sont prévus pour la phase 8.5 de la refonte des thèmes et des extensions. En attendant, les exemples présents dans `examples/plugins/` montrent le format d'échange IPC et les stubs gRPC, mais utilisent l'ancien format de manifeste (ajoutez `kind: plugin` en les adaptant).
