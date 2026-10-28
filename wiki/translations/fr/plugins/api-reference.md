---
source_sha: 158d06870a14e5afae5d1ed21a8f0969f64bf50c032410bba9bd178e72f8888a
reviewed: true
---
# Référence de l'API des plugins { #plugin-api-reference }

Le contrat que doit respecter un plugin : les champs de `plugin.yaml`, les
deux services gRPC que le backend et le serveur échangent, et le pont
`window.evey` qu'un module d'interface reçoit dans son cadre. Le parcours qui
les met en oeuvre est [Écrire un plugin](dev-setup.md) ; ce que chaque
permission autorise et ce que l'installateur refuse est dans
[Permissions et emplacements](security-model.md).

## Manifeste { #manifest }

Chaque ZIP porte un `plugin.yaml` (ou `plugin.yml`) à sa racine, de 1 Mo au
plus.

```yaml title="plugin.yaml"
name: my-plugin              # obligatoire ; unique, en kebab-case
version: 1.2.3               # obligatoire ; semver, X.Y.Z ou X.Y.Z-label
kind: plugin                 # obligatoire ; "plugin" ou "theme"
language: nodejs             # obligatoire pour un plugin ; un environnement de l'image
entrypoint: node index.js    # obligatoire pour un plugin ; la ligne de commande que le serveur lance

description: "..."           # facultatif
author: "..."                # facultatif
min_evey_version: "1.0.0"    # facultatif ; accepté et ignoré

permissions:                 # ce que le backend et l'interface peuvent appeler
  - network
  - media.read
external_fetch:              # obligatoire avec "network" ; noms d'hôte nus uniquement
  - api.themoviedb.org
capabilities:                # les rôles que le plugin dit remplir
  - metadata.provider

web_root: public/            # obligatoire quand frontend est déclaré ; servi en ressources statiques
frontend:
  - slot: home.section       # l'un des emplacements autorisés
    src: public/front.js     # doit être sous web_root

config_schema:               # les champs du panneau Config sur la carte du plugin
  api_key:
    type: password           # jamais renvoyé à un client
    label: Clé d'API         # non affiché : le panneau nomme les champs par leur clé
    required: true
  city:
    type: string
    default: Paris
    description: Résolue par géocodage
```

| Champ | Règle |
|---|---|
| `name`, `version`, `kind` | Obligatoires. `version` doit suivre `X.Y.Z` ou `X.Y.Z-label` ; une mise à jour doit porter une version supérieure à celle installée. |
| `language` | Obligatoire pour `kind: plugin` : `python`, `nodejs`, `php`, `ruby`, `lua`, `bash`, `sh` ou `go`. Il détermine la vérification de l'environnement et les stubs copiés à côté du point d'entrée ; il ne lance rien. |
| `entrypoint` | Obligatoire pour `kind: plugin`. Le premier mot est l'interpréteur (ou le binaire, avec `native-binary`), le reste ses arguments ; le processus tourne dans le dossier du plugin. |
| `permissions` | Chaque valeur doit figurer dans le [catalogue des permissions](security-model.md#permissions-catalog). `network` exige un `external_fetch` non vide. |
| `external_fetch` | Des noms d'hôte. Un schéma et un chemin final sont retirés à l'analyse ; les hôtes correspondent exactement, un joker `*.example.com` est refusé à l'installation et un suffixe de port est ignoré ; une adresse IP fait refuser l'installation ([règles](security-model.md#external-fetch-rules)). |
| `capabilities` | `metadata.provider`, `media.read`, `media.write`, `webhook.receiver` ou `notification.sender` ; toute autre valeur est refusée. Un thème ne peut en déclarer aucune. |
| `web_root` | Obligatoire quand `frontend` n'est pas vide ; relatif, sans `..`, sans barre oblique inverse. `.` sert tout le dossier du plugin et ne produit qu'un avertissement. |
| `frontend[]` | `slot` doit figurer dans la [liste des emplacements](#slots) ; `src` doit être sous `web_root`. |
| `config_schema` | Une table nom de champ vers `{type, label, description, required, default, min, max, options}` ; `label` n'est pas affiché, le panneau nomme les champs par leur clé. Types : `string`, `text`, `password`, `boolean`, `integer`, `number`, `select` (avec `options: [{value, label}]`). |

Un thème (`kind: theme`) ne doit déclarer ni `language`, ni `entrypoint`, ni
`permissions`, ni `capabilities` ; son format est dans [Thèmes](../themes.md).

**Secrets.** Un champ de `type: password`, ou avec `secret: true`, est
stocké et transmis au backend normalement, mais revient masqué de toute route
de lecture, panneau **Config** de l'administration compris. Enregistrer le
panneau avec le masque inchangé conserve la valeur stockée : modifier un autre
champ n'efface jamais un secret.

## Backend : les deux services { #backend-the-two-services }

Le processus backend et le serveur communiquent en gRPC sur deux sockets Unix dont les
chemins arrivent dans l'environnement :

```
EVEY_PLUGIN_ID=<id du plugin>
EVEY_PLUGIN_SOCK=<chemin>   # votre processus écoute ici et sert PluginService
EVEY_HOST_SOCK=<chemin>     # votre processus se connecte ici pour appeler HostService
```

Le serveur authentifie le processus sur son socket par le PID que rapporte le
noyau (`SO_PEERCRED`) ; aucun jeton n'est remis au processus. Les appels sont
limités à 600 par minute et par plugin. `proto/plugin/v1/plugin_api.proto`
est le contrat ; les stubs des langages fournis sont copiés à côté du point
d'entrée au démarrage du plugin ([prérequis](dev-setup.md#1-prerequisites)).

### PluginService : ce que le serveur appelle { #pluginservice-what-the-server-calls }

| Méthode | Quand |
|---|---|
| `Initialize(plugin_id, config, host_version)` | Une fois que le processus a ouvert son socket. `config` porte les valeurs de `config_schema` saisies par l'administrateur. Répondez `success: false` avec un `error` pour refuser de démarrer. |
| `HealthCheck()` | Périodiquement, pour distinguer un processus actif d'un processus bloqué. Répondez `healthy: true` tant que vous pouvez servir. |
| `Shutdown(reason)` | Quand le plugin est désactivé, mis à jour, désinstallé, ou que le serveur s'arrête. Quittez peu après avoir répondu. |
| `OnEvent(event_type, payload, source)` | Uniquement `metadata.request`, vers un plugin doté de la capacité `metadata.provider` ([requêtes de métadonnées](#metadata-requests) plus bas). Tout autre plugin ne reçoit aucun événement : répondez `handled: false`. |
| `GetCapabilities()` | Défini dans le contrat ; le serveur ne l'appelle pas aujourd'hui. |

### HostService : ce que le plugin peut appeler { #hostservice-what-the-plugin-can-call }

Un appel dont le manifeste ne déclare pas la permission est refusé avec
`PermissionDenied`. Les méthodes marquées *non servie* répondent
`Unimplemented` ; elles font partie du contrat pour une version ultérieure et
sont listées pour qu'un stub ne vous induise pas en erreur.

| Méthode | Permission | Notes |
|---|---|---|
| `Log(level, message, fields)` | aucune | Écrit dans le journal du plugin, affiché sous **Logs** sur sa carte. |
| `GetConfig(key)` / `SetConfig(key, value)` | aucune | La configuration du plugin : les valeurs du panneau **Config**, plus les clés que le plugin écrit lui-même. Au plus 256 clés, des clés de 256 octets, des valeurs de 16 Kio. |
| `GetData`, `SetData`, `DeleteData`, `ListDataKeys` | aucune | Le [stockage de données](#data-store) ci-dessous. |
| `EmitEvent(event_type, payload)` | aucune | Publié sur le bus d'événements du serveur sous `plugin.event.<event_type>`, où les webhooks peuvent s'y abonner. |
| `GetMediaItem(id)`, `ListMediaItems(library_id, type, parent_id, limit, offset)` | `api.library` ou `media.read` | La fiche média complète, `file_path`, `provider_ids` et `genres` compris. |
| `Fetch(url, method, headers)` | `network` | Le seul accès au réseau depuis la sandbox : `http` ou `https`, `GET` ou `HEAD`, un hôte de `external_fetch`, adresses privées refusées après résolution et à travers les redirections. Le corps est plafonné à 8 Mio (`truncated` le signale) et la requête entière à 20 secondes. Le serveur n'ajoute aucun identifiant qui lui appartienne. |
| `SearchMedia`, `GetMediaStreams`, `GetChapters`, `GetSubtitles` | `api.library` ou `media.read` | *Non servie.* |
| `UpdateMetadata(media_item_id, fields, provider_ids)` | `api.metadata` ou `media.write`, et la capacité `metadata.provider` | Fournit votre texte pour un film ou une série sans qu'on vous le demande, comme le ferait une [réponse de métadonnées](#metadata-requests). `fields` porte `locale` (obligatoire) et tout ou partie de `title`, `overview`, `tagline` ; le texte remplace ce que vous aviez fourni pour cette langue, et un `fields` qui ne porte que `locale` retire ce que vous aviez fourni pour cette langue. `provider_ids` est refusé : c'est TMDB qui identifie l'élément. Ce qu'un plugin stocke est borné : 16 langues par élément, 200 000 textes et 128 Mio en tout. Au-delà, l'appel répond `RESOURCE_EXHAUSTED` ; remplacer ou retirer un texte déjà fourni reste possible. |
| `GetUserPreferences` | `api.user` | *Non servie.* |
| `SendNotification` | `notification.sender` | *Non servie.* |

### Requêtes de métadonnées { #metadata-requests }

Chaque fois que le serveur identifie ou actualise un film ou une série depuis
TMDB, il envoie à chaque plugin `metadata.provider` activé un `OnEvent` dont
l'`event_type` est `metadata.request`, un plugin après l'autre, et attend
jusqu'à 15 secondes chacun. Le contenu :

| Champ | Contenu |
|---|---|
| `media_item_id` | L'identifiant de l'élément, pour un `UpdateMetadata` ultérieur. |
| `type` | `movie` ou `series`. |
| `title`, `original_title` | Le titre TMDB dans la langue du serveur, et le titre original. |
| `year` | Absent quand il est inconnu. |
| `provider_ids` | `tmdb`, `imdb` et les autres identifiants connus de l'élément. |
| `language` | La langue du texte TMDB, par exemple `fr`. |
| `locales` | Toutes les langues que le serveur affiche. |

Répondez `handled: true` avec un `result` de la forme :

```json
{
  "locale": "fr",
  "title": "...",
  "overview": "...",
  "tagline": "...",
  "translations": {
    "de": { "overview": "..." }
  }
}
```

Les champs de premier niveau sont dans la langue `locale`, ou dans la
`language` de la requête quand `locale` est absent ; `translations` ajoute
d'autres langues. Omettez un champ que vous ne couvrez pas. Les étiquettes de
langue sont ramenées à leur langue (`fr-FR` se lit `fr`), et un champ de plus
de 8 Kio est ignoré. Une réponse remplace tout ce que vous aviez fourni pour
l'élément ; `handled: false`, ou une réponse sans texte, signifie que vous ne
connaissez pas l'élément et efface ce que vous aviez fourni. Une erreur ou un délai dépassé conserve
ce que vous aviez fourni.

L'administrateur choisit lesquels de `title`, `overview` et `tagline` votre
texte remplace, et peut vous interroger sur toute la bibliothèque d'un coup
([Plugins de métadonnées](../plugins.md#metadata-plugins)). Les saisons et les
épisodes ne sont pas envoyés.

### Stockage de données { #data-store }

Chaque plugin possède un stockage clé/valeur persistant, sans permission à
déclarer : curseurs, caches, horodatage de la dernière synchronisation, tout
ce qu'il faut retenir entre deux redémarrages. Les valeurs sont des chaînes
opaques ; encodez le JSON vous-même. L'identifiant du plugin vient du socket
authentifié, donc un plugin ne lit et n'écrit que son propre espace, et tout
le stockage est supprimé avec le plugin.

| Méthode | Rôle |
|---|---|
| `SetData(key, value)` | Crée ou remplace une valeur. |
| `GetData(key)` | Renvoie `value` et `found`. |
| `DeleteData(key)` | Supprime une clé. |
| `ListDataKeys()` | Les clés du plugin. |

Ce stockage est global au plugin. Les données **par utilisateur** (une
préférence de la personne qui regarde le widget) sont gérées côté interface, par
la permission `prefs` ci-dessous.

## API de l'interface { #frontend-api }

Un module d'interface tourne dans un cadre à origine opaque, sous une
politique `default-src 'none'` : aucun accès à la page hôte, à ses cookies ou
à son stockage, pas de formulaire, pas de fenêtre surgissante, et un
`connect-src` limité aux hôtes de `external_fetch` en `https`. Tout le reste
passe par `window.evey`, injecté avant l'exécution du module. Le même module
tourne sans modification dans l'application web et dans les applications
mobile et télévision.

### `evey.onInit(callback)` { #eveyoninitcallback }

Appelé une fois avec le contexte qu'envoie l'hôte ; un callback enregistré
après l'arrivée du contexte est appelé immédiatement.

```js
evey.onInit(function (data) {
  // data.config          - les valeurs de configuration du plugin (config_schema)
  // data.context         - le contexte de l'hôte
  // data.locale          - la langue active, par ex. "fr-FR"
  // data.theme           - "dark" ou "light"
  // data.surface         - "web", "mobile" ou "tv"
  // data.suggestedHeight - home.section seulement : la hauteur (px) que l'hôte recommande
  // data.viewportHeight  - tv seulement : la hauteur du panneau en dp
});
```

### `evey.navigate(path)` { #eveynavigatepath }

Déplace l'application hôte vers `path`. Seuls les préfixes `/home`,
`/library`, `/media`, `/player`, `/settings` et `/search` sont suivis ; tout
le reste (la zone d'administration, la connexion, une URL absolue) est ignoré
en silence.

### `evey.toast(level, message)` { #eveytoastlevel-message }

Affiche une notification dans l'interface hôte. `level` vaut `"success"`,
`"error"` ou `"info"` ; `message` est du texte brut, coupé à 200 caractères ;
au plus 3 notifications par seconde sont affichées.

### `evey.resize(height)` { #eveyresizeheight }

Demande une hauteur d'emplacement de `height` pixels, bornée entre 50 et
800. `0` replie l'emplacement : un widget qui n'a rien à montrer ne prend
aucune place. Un widget `home.section` s'ouvre à `data.suggestedHeight` sans
rien faire : l'hôte connaît la surface où il vous a placé et recommande la
hauteur qu'un widget d'accueil y a, pour que tous les widgets aient la
même taille. Ne redimensionnez que si vous avez une raison précise de le faire.

### `evey.prefs.*` (permission `prefs`) { #eveyprefs-permission-prefs }

Les préférences de l'utilisateur courant dans l'espace propre du plugin,
liées à (ce plugin, cet utilisateur) par le jeton que l'hôte détient pour le
cadre. Chaque appel renvoie une promesse ; une permission absente la rejette.

```js
await evey.prefs.set("units", "celsius");
const { value, found } = await evey.prefs.get("units");
const all = await evey.prefs.list();        // [{ key, value }]
await evey.prefs.delete("units");
```

### `evey.media.*` (permission `public`) { #eveymedia-permission-public }

Une projection sûre des médias : identifiant, titre, type, année, résumé, URL
d'affiche. Jamais un chemin de fichier, une taille, un identifiant de
fournisseur ni un flux.

```js
const item  = await evey.media.get(id);
const page  = await evey.media.list({ libraryId });
const data  = await evey.media.poster(id);  // l'affiche en URL data:
```

`poster` renvoie une URL `data:` parce que la politique du cadre interdit de
charger une image depuis le serveur directement ; utilisez-la comme source
d'un `<img>` ou comme texture.

### `evey.assets.read(path)` { #eveyassetsreadpath }

Lit l'un des fichiers `web_root` du plugin lui-même, par chemin relatif, en
URL `data:` : un fichier écrit par le backend à l'exécution devient utilisable
dans le cadre sans question d'origine croisée. Seules les ressources du
plugin sont accessibles.

### Sur une télévision { #on-a-television }

L'application télévision exécute le module dans une WebView, et quelques
comportements habituels du navigateur n'y sont plus valables :

- **Ne dimensionnez pas le widget d'après sa propre fenêtre.** Dans la WebView,
  `window.innerHeight` est l'emplacement que l'hôte vient de vous accorder.
  Partez de `data.viewportHeight` ou de `data.suggestedHeight`.
- **Ni focus, ni pointeur.** Le widget ne reçoit jamais le focus de la
  télécommande et il n'y a pas de survol ; tout ce qui est interactif
  appartient aux surfaces web et mobile.
- **Éloignez le texte des bords.** Les télévisions rognent l'image : laissez
  environ 5 % de la largeur et de la hauteur libres de chaque côté.
- **Testez le WebGL sur un vrai boîtier.** Un shader tourne sur le GPU et le
  pilote du boîtier, pas sur ceux de votre bureau ; gardez les coordonnées de
  bruit bornées. `tests/plugins/weather-v2` est le widget de référence pour
  tout ce qui précède.

## Emplacements { #slots }

| Emplacement | Où il apparaît |
|---|---|
| `home.section` | Une section de contenu sur la page d'accueil |
| `home.hero` | La zone de bannière de la page d'accueil |
| `library.sidebar` | Un panneau latéral dans les vues de bibliothèque |
| `media.detail.sidebar` | Un panneau latéral sur une fiche média |
| `player.overlay` | Une surcouche sur le lecteur vidéo |
| `settings.user.appearance` | La section apparence des paramètres utilisateur |
| `sidebar.bottom` | Le bas de la barre de navigation principale |
| `admin.nav` | Une entrée du menu d'administration, qui ouvre la page du plugin |

`admin.nav` est le seul emplacement de la zone d'administration. Il n'injecte
rien dans les écrans d'administration existants : il ajoute une entrée de
menu qui ouvre la page du plugin dans le même cadre isolé, sur une route
réservée aux administrateurs. Le demander apparaît dans la différence
soumise au consentement. Tout autre emplacement `admin.*`, `login.*`,
`setup.*` et `profile.security.*` est refusé à l'installation.

## Capacités { #capabilities }

Une capacité déclare le rôle que le plugin dit remplir : `metadata.provider`,
`media.read`, `media.write`, `webhook.receiver` ou `notification.sender`.
L'installateur vérifie le nom, l'administrateur voit la liste dans l'aperçu
et tout ajout à la mise à jour, et le Tableau de bord sécurité l'enregistre.
Le serveur oriente les appels selon l'une d'elles : un `metadata.provider`
reçoit les [requêtes de métadonnées](#metadata-requests) et peut appeler
`UpdateMetadata`. Les autres n'orientent rien aujourd'hui ; un
`webhook.receiver` ne reçoit aucune requête entrante. Ce qu'un plugin peut
réellement faire est décidé par ses **permissions**.

## Permissions de l'interface { #permissions-of-the-interface }

`public` et `prefs` se déclarent sous `permissions` comme les autres. Elles
n'accordent rien au processus backend ; elles donnent accès à `evey.media.*` et
`evey.prefs.*` dans le cadre. Le cadre ne détient jamais de jeton : l'hôte
émet un jeton de cinq minutes limité à (plugin, utilisateur) et relaie les
appels, et le serveur refuse par défaut toute méthode que les portées du
jeton ne nomment pas
([ADR-0012](https://github.com/eveyproject/evey/blob/master/docs/adr/0012-plugin-token-deny-by-default.md)).

!!! note "Ce qu'un plugin ne peut jamais demander"
    `admin.*`, `auth.*`, `system.*`, `user.write` et `library.write` sont
    interdits : un manifeste qui les nomme est refusé, un jeton ne peut
    jamais les porter, et les méthodes d'administration refusent un jeton de
    plugin quelles que soient ses portées.

## Exemples { #examples }

`examples/plugins/` contient des plugins fonctionnels au format de manifeste
courant : des bases en Node.js et en Python (`nodejs-example`,
`python-example`), un fournisseur de métadonnées (`metadata-example`), un
récepteur de webhooks (`webhook-receiver`), un widget d'accueil avec une
étape de build (`weather`), le stockage de données avec les permissions
`public` et `prefs` et l'emplacement `admin.nav` (`data-demo`), un exemple de
chaque type de champ de `config_schema`, avec une page `admin.nav` (`config-demo`), et deux thèmes
(`theme-dark-blue`, `open-workshop`). `make plugin-zips` les empaquette
tous.

<!-- sources
manifest fields, validation: internal/domain/plugin/model.go - PluginManifest, Validate, validateWebRoot, validateSrcUnderWebRoot, IsSecretConfigField ; internal/adapters/driven/pluginrunner/manifest.go - Parse, validateManifest, validateExternalFetch ; capabilities.go - KnownCapabilities, KnownPermissions
runtimes: internal/adapters/driven/pluginrunner/runtime_availability.go ; interpreters: runner_start.go - knownInterpreters
config field types: frontend/apps/web/src/features/admin/PluginCard.tsx ; examples/plugins/config-demo/plugin.yaml
secrets masked: internal/adapters/driving/grpcapi/admin_plugin.go (SecretMask)
env vars, Initialize call: internal/adapters/driven/pluginrunner/runner_start.go ; ipc_client.go - Initialize, Shutdown, HealthCheck, OnEvent ; runner_control.go - SendEvent (no caller)
peer auth, rate limit: internal/adapters/driving/pluginhost/host_server.go - makePluginAuthInterceptor, pluginRPCRatePerMin ; peer_creds.go
host RPCs and permissions: internal/adapters/driving/grpcapi/plugin_interceptor.go ; pluginhost/host_server.go - Log, GetConfig, SetConfig (maxConfigKeys, maxConfigKeyLen, maxConfigValueLen), EmitEvent (pluginEmittedEvent.EventName), GetMediaItem, ListMediaItems, mediaItemToProto, UpdateMetadata, GetUserPreferences (Unimplemented), UnimplementedHostServiceServer ; fetch.go - Fetch, maxFetchBytes, fetchTimeout
proto: proto/plugin/v1/plugin_api.proto - PluginService, HostService, InitRequest, FetchRequest, FetchResponse, MediaItem
frame policy: internal/adapters/driving/http/plugin_frame_handler.go - pluginFramePolicy ; frontend/packages/client/src/plugins/hostProtocol.ts - buildConnectSrc, ALLOWED_NAV_PREFIXES, SLOT_MAX_HEIGHT, clampSlotHeight, TOAST_MAX, TOAST_WINDOW_MS, TOAST_MAX_LENGTH ; frontend/packages/client/src/web/PluginIframe.tsx - MIN_SLOT_HEIGHT, suggestedSlotHeight ; frontend/packages/client/src/native/PluginWebView.tsx (surface)
bridge: frontend/packages/client/src/plugins/bridgeScript.ts - EveyBridge ; pluginRpc.ts - pluginRequestFor
slots: internal/domain/plugin/slots.go - AllowedSlots
token, deny by default: internal/app/plugin/token_service.go - PluginTokenTTL ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes, forbiddenPrefix ; docs/adr/0003-plugin-scoped-tokens.md ; docs/adr/0012-plugin-token-deny-by-default.md
examples: examples/plugins/* ; Makefile - plugin-zips
-->
