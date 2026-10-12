---
source_sha: 2aa3f137eb7403f9ad58369e4f8f4eb6ea411a331f295395267794f1472f7153
reviewed: true
---
# Architecture

Comment le serveur est assemblé : les couches hexagonales sous `internal/`,
les contextes bornés, les événements qui les relient, et les gardes statiques
qui maintiennent le code sur ces rails. Le code vit dans `internal/domain`,
`internal/app`, `internal/adapters/{driving,driven}` et est câblé dans
`cmd/evey/wire*.go` ; la disposition est décidée par
[ADR-0001](adr/0001-hexagonal-architecture.md), la séparation des dépôts par
[ADR-0009](adr/0009-cqrs-repo-split.md) et le bus d'événements par
[ADR-0010](adr/0010-event-bus.md). Pour construire et lancer le serveur, lisez
[DEV.md](../DEV.md) ; pour savoir quels fichiers toucher pour une modification
donnée, lisez la [Carte du code](CODEBASE_MAP.md).

Le serveur est en Go ; les clients web, mobile et TV forment un monorepo
TypeScript/React sous `frontend/` (React Native / Expo pour le mobile et la TV).

## Couches

```mermaid
flowchart LR
    driving[Adaptateurs entrants<br/>internal/adapters/driving<br/>grpcapi, http, pluginhost]
    app[Services applicatifs<br/>internal/app/contexte]
    domain[Domaine<br/>internal/domain/contexte<br/>modèles, objets-valeurs, ports]
    driven[Adaptateurs sortants<br/>internal/adapters/driven<br/>postgres, ffmpeg, mkvgo, ...]
    driving --> app --> domain
    driven -. implémentent les ports .-> domain
```

| Dossier | Contient | Règle |
| --- | --- | --- |
| `internal/domain/<contexte>/` | les modèles, les objets-valeurs et les **ports** (interfaces) | aucun import de framework, aucun import d'un autre contexte |
| `internal/app/<contexte>/` | les services applicatifs qui orchestrent les ports | les entrées-sorties passent par un port ; `dlna` (SSDP / UPnP AVTransport) et `loadtest` (son propre client HTTP) sont les deux exceptions délibérées qui atteignent le réseau directement |
| `internal/adapters/driven/` | les adaptateurs sortants qui implémentent les ports : `postgres`, `meilisearch`, `ffmpeg`, `mkvgo`, `filesystem`, `prober`, `opensubtitles`, `email`, `webpush`, `webhooksender`, `pluginrunner`, `metadata`, `introproviders`, `search` | trois autres satisfont une interface de la couche applicative plutôt qu'un port du domaine : `itunes` et `musicbrainz` (`app/music.AlbumArtFetcher`), `ociregistry` (`app/updatecheck`) |
| `internal/adapters/driving/` | les adaptateurs entrants : `grpcapi` (+ HTTP via grpc-gateway), `http` (statique, middleware d'authentification, SSE, HLS, ressources des iframes de plugins), `pluginhost` (serveur gRPC sur une socket Unix, authentifié par les identifiants du pair, l'API hôte que rappellent les plugins en sandbox) | |
| `internal/pkg/` | le noyau technique partagé : auth, logger, eventbus, i18n, metrics, tracing, ratelimit, procfork, ssrf, cache, ... | |
| `internal/config/` | la configuration du serveur | |
| `gen/` | le code proto généré | |

Règle de dépendance : `adapters -> app -> domain`. Le domaine n'importe jamais
la couche applicative ni les adaptateurs. La composition est écrite à la main
(pas google/wire) dans `cmd/evey/wire.go` et ses voisins `wire_infra.go`,
`wire_services.go`, `wire_adapters.go`, `wire_events.go`, `wire_helpers.go`.
Les tests contournent les adaptateurs en injectant des doublures qui
implémentent les mêmes ports du domaine : 20 des 21 paquets du domaine
livrent un sous-paquet `fakes/`.

## Contextes bornés

Trente-deux paquets de contexte vivent sous `internal/domain/` et
`internal/app/` : 19 existent dans les deux couches, 2 n'existent que dans le
domaine (`ratings`, `shared`), 11 n'existent que dans la couche applicative.
Ils collaborent en partie par des événements de domaine sur un bus partagé ;
la couche applicative lit aussi directement le paquet de domaine d'un
contexte voisin (voir [Relations](#relations)).

| Contexte | Responsabilité |
| --- | --- |
| `user` | comptes, authentification, sessions, préférences |
| `library` | bibliothèques, dossiers, tâches de scan, index des médias, personnes, collections, favoris et pouces d'un profil (les pouces n'ont pas de service applicatif : le RPC de notation stocke et lit `UserRatingRepository` tel quel), trickplay, détection des intros et génériques |
| `media` (app seulement) | liens de partage et jetons de flux |
| `mediarequest` | demandes de médias soumises par les utilisateurs |
| `metadata` | enrichissement TMDB / OMDb, illustrations |
| `playback` | flux, progression, télécommande, lecture en cours |
| `transcoding` | tâches ffmpeg, segments HLS |
| `subtitle` | sous-titres intégrés + externes + opensubtitles |
| `mkv` | opérations sur le conteneur MKV (via [mkvgo](playback/mkvgo.md)) |
| `music` | lecture et étiquetage propres à l'audio |
| `photo` | bibliothèques de photos |
| `favorite` (app seulement) | favoris par profil ; le modèle et `FavoriteRepository` vivent dans `domain/library` |
| `playlist` | listes de lecture des utilisateurs, listes intelligentes |
| `collection` (app seulement) | collections constituées par l'administrateur ; ses ports vivent dans `domain/library` |
| `ratings` (domaine seulement) | projette les classifications nationales (CSA, FSK, MPAA, BBFC, ACB) sur un seul axe d'âge minimum |
| `feed` | fil d'activité / découverte |
| `activity` | chronologie d'activité des utilisateurs |
| `chromecast` | cycle de vie des sessions de cast |
| `syncplay` | sessions synchronisées entre appareils |
| `plugin` | registre des plugins, jetons de sandbox, cycle de vie |
| `notification` | notifications dans l'application + webpush |
| `webhook` | webhooks sortants |
| `backup` | sauvegarde et restauration de la base et de la configuration |
| `config` (app seulement) | réglages persistants du serveur, écrits par les points d'entrée REST et gRPC |
| `dlna` (app seulement) | découverte SSDP et pilotage UPnP AVTransport |
| `loadtest` (app seulement) | simule des spectateurs contre la propre pile HTTP du serveur : en HLS, ou sur le fichier brut quand le verdict d'un client est une lecture directe |
| `maintenance` (app seulement) | maintenance des médias déclenchée par l'administrateur : reconstruction de l'index de lecture, journal de réparation, annulation, intégrité audio |
| `presence` (app seulement) | registre en mémoire, non persisté, de qui est connecté en ce moment |
| `qoe` (app seulement) | ingestion et agrégation de la télémétrie de lecture CMCD (CTA-5004) |
| `demo` (app seulement) | l'instance de démonstration publique (`EVEY_DEMO=1`) : alimente ses comptes, ses bibliothèques et ses médias sous licence libre via les services du produit, et se réinitialise |
| `updatecheck` (app seulement) | compare le build en cours avec ce que publie le registre et prévient l'administrateur ; ne met jamais rien à jour |
| `shared` (domaine seulement) | ports transverses (`EventBus`, `TransactionalUnit`, `ServerConfigRepository`, `ShareLinkRepository`), identifiants typés, curseurs, jetons de flux |

### Relations

| Émetteur | Événements | Consommés par |
| --- | --- | --- |
| `library` | `LibraryCreated` / `LibraryDeleted`, `LibraryScanStarted` / `LibraryScanCompleted`, `MediaItemAdded`, `MediaItemRemoved`, `TrickplayGenerated` | `media.item.added` : `notification`, `metadata` (enrichissement automatique), `library` lui-même (worker trickplay + détection des intros), `feed` (invalidation du cache des genres) |
| `playback` | `PlaybackStarted` / `PlaybackStopped` | aucun abonné dédié aujourd'hui ; seul le fourre-tout `"*"` de `webhook` les reçoit |
| `user` | `UserCreated`, `user.password.reset_requested` | le premier n'a pas d'abonné ; le second est consommé par l'adaptateur sortant `email` |
| `pluginrunner` | `plugin.error` / `plugin.started` | `app/plugin` pour son tableau de bord sécurité, `notification` pour l'administrateur |
| `shared` | `ServerConfigChanged` | les services qui appliquent sans redémarrage un réglage enregistré dans le panneau d'administration (SMTP, fuseau horaire, budget CPU, ...) |

- L'indexation de recherche n'est **pas** pilotée par événement :
  `library.ScanService` appelle directement le port `library.SearchIndex`.
- Aucune passerelle ne transmet les événements de domaine aux plugins : les
  plugins atteignent l'API avec des JWT à portée limitée
  ([ADR-0003](adr/0003-plugin-scoped-tokens.md)). Le bus lui-même est
  l'[ADR-0010](adr/0010-event-bus.md).
- `shared.TransactionalUnit` enveloppe les écritures multi-agrégats
  ([ADR-0002](adr/0002-transactional-unit-port.md)).

Aucun paquet sous `internal/domain` n'importe le paquet de domaine d'un autre
contexte, règle imposée par `internal/contractguard/domain_imports_test.go`
(voir [Gardes d'architecture](#architecture-guards)). La couche applicative
n'y est pas tenue : 15 paquets applicatifs importent aujourd'hui le paquet de
domaine d'un contexte voisin.

## Flux de données typique : « un utilisateur lance un média »

1. Le client mobile ou web appelle `POST /api/v1/playback/start` (route grpc-gateway).
2. `internal/adapters/driving/http` exécute son middleware d'authentification (`auth_middleware.go`, `httpAuthMiddleware`) et extrait la session.
3. `internal/adapters/driving/grpcapi` (`playback_handler.go`) valide le proto et appelle `app/playback.CommandService.StartStream` (séparation commande/requête, voir [ADR-0009](adr/0009-cqrs-repo-split.md)).
4. `app/playback.CommandService` appelle les ports du domaine : `user.UserRepository`, `playback.SessionQueryRepo` (idempotence + plafond d'appareils), `playback.SessionCommandRepo`, puis `shared.EventBus.Publish(PlaybackStarted)`.
5. Les adaptateurs sortants s'exécutent : `postgres` persiste la session dans une portée `shared.TransactionalUnit.Do` ; le bus en mémoire (`internal/pkg/eventbus`) distribue de façon asynchrone.
6. `PlaybackStarted` n'a pas d'abonné dédié aujourd'hui : seul le fourre-tout `"*"` de `webhook` le reçoit.
7. La réponse revient en JSON via grpc-gateway ; les mises à jour de progression passent par SSE / flux gRPC.

Ce qui se passe ensuite, quand le lecteur demande le manifeste et les
segments, fait l'objet de [Pipeline de lecture](playback-serving.md).

## Gardes d'architecture

Les règles ci-dessus ne sont pas que de la documentation.
`internal/contractguard/` contient 58 tests statiques répartis sur 30
fichiers qui font échouer le build dès que le code s'en écarte.

La règle hexagonale elle-même est `TestDomainContextsDoNotImportEachOther`
(ADR-0001), dans `domain_imports_test.go`. Elle parcourt `internal/domain` et
impose exactement ceci : un contexte peut importer `internal/domain/shared` et
sa propre arborescence, rien d'autre dans `internal/domain`, et jamais
`internal/app` ni `internal/adapters`. Sa portée s'arrête à
`internal/domain` : elle ne dit rien de la couche applicative.

Les autres gardent des contrats qui franchissent une frontière de langage ou
de processus, parmi lesquels :

| Garde | Ce qu'elle refuse |
| --- | --- |
| `TestNoClientReadOfAFieldTheServerNeverWrites` | un champ de réponse proto lu par un client que le serveur n'écrit nulle part |
| `TestRegisteredHandlersGetEveryPortTheyDeclare`, `TestBootInstallsItsSilentWirings` | un câblage qui échouerait en silence |
| `TestPostgresRepositoriesBoundTheirQueries` | une requête d'un dépôt Postgres sans délai limite |
| `TestProcfork_EveryForkGoesThroughCommand` | un lancement de sous-processus qui contourne `internal/pkg/procfork` |
| `TestEverySortFieldTheAPIAcceptsHasASQLColumn`, `...CanBuildACursorValue` | un champ de tri absent de la table des colonnes SQL ou de l'encodeur de curseur |
| `TestEveryWebSurfaceReachingAnAdminRPCIsClassified`, et son pendant natif pour mobile et TV | une surface d'interface appelant un RPC réservé à l'administrateur sans décider qui la voit |
| `TestEveryAllowedSlotIsMountedOnWebAndMobile`, `TestPluginBridgeParityWebMobile` | un slot d'interface de plugin ou un message du pont présent sur un seul client |
| `TestImageSubtitleCodecsAgreeAcrossServerAndClients` | une seconde liste de codecs de sous-titres bitmap sur un lecteur |
| `TestHandWrittenClientTypesMatchTheEndpointTheyCall` | un type de réponse écrit à la main pour mobile/TV qui dérive de son point d'entrée |
| `TestOnlyThePlanDecidesPlayback`, `TestNoCallerDerivesDownstreamOfThePlaybackPlan` | une décision de lecture prise hors du résolveur de plan ([ADR-0030](adr/0030-one-plan-one-decision.md)) |
| `TestRestartExitCodeAgreesWithTheEntrypoint` | un code de sortie Go et `docker/entrypoint.sh` en désaccord sur ce que signifie un redémarrage |
| `TestServerEmittedI18nKeysExistInCatalogue`, `TestI18nPlaceholdersAgreeAcrossLocales` | une clé émise par le serveur absente du catalogue, des espaces réservés qui diffèrent entre langues |

## Diagrammes C4

Quatre diagrammes documentent Evey à des niveaux de zoom décroissants selon
le [modèle C4](https://c4model.com/). Les noms des nœuds sont les vrais noms
des paquets Go sous `internal/`. Les noms des conteneurs viennent de
`docker-compose.yml` pour `evey-server`, `evey-postgres` et `evey-caddy` ;
`evey-web`, `evey-meilisearch`, `evey-mobile` et `evey-mailcatcher` sont
déclarés dans `docker-compose.override.yml` et n'existent que dans la
topologie de développement.

### Niveau 1 : contexte du système

Qui parle à Evey, et quels tiers Evey contacte.

```mermaid
flowchart LR
    user[Utilisateur<br/>client web / mobile / TV]
    admin[Administrateur<br/>interface web /admin]
    oidc[Fournisseur d'identité OIDC<br/>SSO optionnel]
    tmdb[API TMDB<br/>métadonnées films/séries]
    omdb[API OMDb<br/>métadonnées et notes de repli]
    imdb[IMDbAPI / TVmaze<br/>métadonnées de séries de repli]
    art[MusicBrainz / iTunes Search<br/>pochettes d'albums]
    introdb[IntroDB / TIDB<br/>horodatages intro + générique]
    opensubs[OpenSubtitles<br/>recherche de sous-titres]
    webpush[Passerelle Web Push<br/>FCM / APNs / VAPID]
    registry[Registre OCI<br/>tags d'image publiés]

    subgraph evey[Serveur multimédia Evey]
        core((Plateforme multimédia auto-hébergée<br/>bibliothèque, lecture, transcodage,<br/>musique, photo, plugins))
    end

    user -->|HTTPS<br/>HLS / SSE / gRPC-Web| evey
    admin -->|HTTPS<br/>/admin| evey
    evey -.->|flux de code OIDC| oidc
    evey -.->|récupération de métadonnées HTTPS| tmdb
    evey -.->|récupération de métadonnées HTTPS| omdb
    evey -.->|récupération de métadonnées HTTPS| imdb
    evey -.->|récupération de pochettes HTTPS| art
    evey -.->|recherche d'intro HTTPS| introdb
    evey -.->|récupération de sous-titres HTTPS| opensubs
    evey -.->|envoi de push HTTPS| webpush
    evey -.->|vérification de mise à jour HTTPS| registry
```

### Niveau 2 : conteneurs

Les unités déployables lancées par `docker compose --env-file .env.dev up`.
Les flèches pleines sont le trafic à l'exécution ; les flèches pointillées
sont le trafic sortant vers des tiers.

```mermaid
flowchart TB
    user([Navigateur / mobile de l'utilisateur])
    admin([Navigateur de l'administrateur])

    subgraph compose[Réseau Docker Compose]
        caddy[evey-caddy<br/>Caddy 2.9<br/>:80 / :443<br/>TLS, reverse proxy]
        web[evey-web<br/>Vite en dev / nginx en prod<br/>:5173 en dev<br/>SPA React + RN web]
        server[evey-server<br/>binaire Go<br/>:9091 HTTP+gRPC-gateway<br/>:9090 gRPC interne]
        pg[(evey-postgres<br/>PostgreSQL 16<br/>:5432)]
        meili[(evey-meilisearch<br/>Meilisearch 1.12<br/>:7700)]
        mobile[evey-mobile<br/>dev seulement<br/>:8081 Expo / bundler Metro]
        mail[evey-mailcatcher<br/>dev seulement<br/>:1025 SMTP / :1080 UI]
    end

    tmdb[[TMDB / OMDb / OpenSubtitles]]
    fs[(volumes /media<br/>montages en lecture seule)]
    cache[(volume evey-cache<br/>transcodage + trickplay)]
    data[(volume evey-data<br/>clés, logs, sauvegardes,<br/>plugins, avatars)]

    user -->|HTTPS| caddy
    admin -->|HTTPS /admin| caddy
    caddy -->|HTTP /| web
    caddy -->|HTTP /v1, /api, SSE, HLS| server
    web -->|gRPC-Web via /v1| server
    user -.->|bundle Metro de dev| mobile

    server -->|SQL/TCP 5432| pg
    server -->|HTTP 7700<br/>indexation + recherche| meili
    server -.->|SMTP 1025 en dev| mail
    server -.->|HTTPS| tmdb
    server -->|lecture FS| fs
    server -->|lecture/écriture FS| cache
    server -->|lecture/écriture FS| data
```

!!! note "La production diffère de cette topologie de développement"
    Le compose d'installation déclare un seul service, `evey`. Il n'y a pas
    de conteneur `evey-web` séparé (ni le mailcatcher et le bundler Metro,
    réservés au dev) : le binaire sert l'interface web construite depuis
    `/srv`, donc un reverse proxy lui transmet *tout*, interface et API, sur
    `:9091`. Postgres et Meilisearch tournent dans le même conteneur,
    supervisés par `docker/entrypoint.sh` : PostgreSQL 17 avec ses données
    sous `/data/postgres`, et le binaire Meilisearch (`Dockerfile`, étape
    `meili-download`, figé en 1.12.8) avec son index sous `/data/meili`. Le
    `docker-compose.yml` du dépôt garde des services `postgres` et `caddy`
    séparés, décrits dans
    [Déploiements avancés](operations/advanced-deployments.md). Voir
    [Installation](installation.md) et [Reverse proxy](reverse-proxy.md).

### Niveau 3 : composants (zoom sur `evey-server`)

Les couches hexagonales à l'intérieur du binaire Go. Les adaptateurs entrants
appellent les services applicatifs par leurs interfaces ; les services
applicatifs ne dépendent que des ports du domaine ; les adaptateurs sortants
implémentent ces ports.

```mermaid
flowchart LR
    subgraph driving[Adaptateurs entrants]
        grpc[grpcapi<br/>internal/adapters/driving/grpcapi<br/>handlers par contexte]
        gw[grpc-gateway<br/>JSON/HTTP <-> gRPC<br/>généré depuis les proto]
        http[http<br/>internal/adapters/driving/http<br/>statique, SSE, HLS,<br/>auth_middleware.go]
        plug[pluginhost<br/>internal/adapters/driving/pluginhost<br/>gRPC sur socket Unix<br/>auth par identifiants du pair]
    end

    subgraph app[Couche applicative - internal/app]
        direction TB
        a_lib[library]
        a_med[media]
        a_meta[metadata]
        a_play[playback]
        a_trans[transcoding]
        a_sub[subtitle]
        a_mkv[mkv]
        a_music[music]
        a_photo[photo]
        a_user[user]
        a_upd[updatecheck]
        a_misc[favorite, playlist, collection,<br/>feed, activity,<br/>chromecast, syncplay,<br/>plugin, notification, webhook,<br/>backup, dlna, config, mediarequest,<br/>maintenance, presence, qoe, loadtest,<br/>demo]
    end

    subgraph domain[Cœur du domaine - internal/domain]
        d_models[Agrégats + objets-valeurs<br/>par contexte borné]
        d_ports[Ports sortants<br/>dépôts - séparation commande/requête,<br/>EventBus, TransactionalUnit,<br/>ServerConfigRepository]
    end

    subgraph driven[Adaptateurs sortants]
        pgad[postgres<br/>dépôts + migrations<br/>shared.TransactionalUnit]
        meiad[meilisearch<br/>index de recherche]
        fsad[filesystem<br/>scanner, parser, watcher]
        ffad[ffmpeg<br/>transcodeur, trickplay]
        mkvad[mkvgo<br/>opérations MKV]
        metaad[metadata<br/>clients TMDB, OMDb,<br/>IMDbAPI, TVmaze]
        introad[introproviders<br/>chaîne IntroDB + TIDB]
        itunad[itunes<br/>pochettes d'albums]
        mbad[musicbrainz<br/>pochettes d'albums]
        ociad[ociregistry<br/>tags publiés]
        probead[prober<br/>voie rapide mkvgo,<br/>repli ffprobe]
        searchad[search<br/>façade résiliente sur meili]
        osad[opensubtitles]
        emad[email]
        wpad[webpush]
        whad[webhooksender]
        prad[pluginrunner]
    end

    grpc --> app
    gw --> grpc
    http --> app
    plug --> app

    app --> d_ports
    app --> d_models
    d_ports -.implémente.- pgad
    d_ports -.implémente.- meiad
    d_ports -.implémente.- fsad
    d_ports -.implémente.- ffad
    d_ports -.implémente.- mkvad
    d_ports -.implémente.- metaad
    d_ports -.implémente.- introad
    d_ports -.implémente.- probead
    a_music -.interface applicative.- itunad
    a_music -.interface applicative.- mbad
    a_upd -.interface applicative.- ociad
    d_ports -.implémente.- searchad
    d_ports -.implémente.- osad
    d_ports -.implémente.- emad
    d_ports -.implémente.- wpad
    d_ports -.implémente.- whad
    d_ports -.implémente.- prad

    pgad --> pg2[(PostgreSQL)]
    meiad --> me2[(Meilisearch)]
    fsad --> fs2[(montages /media)]
    ffad --> cc2[(evey-cache + binaire ffmpeg)]
    metaad -.-> ext2[[TMDB / OMDb / IMDbAPI / TVmaze]]
    introad -.-> ext3[[IntroDB / TIDB]]
    itunad -.-> ext4[[iTunes Search / MusicBrainz]]
    mbad -.-> ext4
    ociad -.-> ext5[[Registre OCI]]
```

Notes sur les goroutines :

- Le serveur gRPC (`grpc.Server.Serve`) et le serveur HTTP (gateway + statique) tournent chacun dans leur propre goroutine, lancée depuis le câblage principal de `cmd/evey`.
- Les tâches de scan de `app/library` tournent sur un pool de workers (une goroutine par scan de bibliothèque actif, voir `library.ScanService`).
- Les tâches ffmpeg sont lancées via `internal/pkg/procfork` par l'adaptateur sortant `ffmpeg` (`ffmpeg.Transcoder.Start`), avec une goroutine de supervision par tâche qui vide la sortie d'erreur du processus.
- `shared.EventBus` distribue aux abonnés de façon asynchrone : chaque abonné tourne dans sa propre goroutine, pour qu'un consommateur lent ne bloque jamais les émetteurs. Le bus n'ouvre aucune transaction de son propre chef ; c'est le travail du handler.
- `pluginrunner` lance un sous-processus par plugin ; les plugins en iframe vivent dans le navigateur de l'utilisateur et répondent via des jetons signés émis par `app/plugin`.

### Niveau 3 (bonus) : zoom sur le pipeline de scan

Le chemin chaud qui transforme des fichiers sur disque en médias enrichis et
cherchables. Les flèches pleines sont des appels directs ; les flèches
pointillées sont des événements de domaine sur le bus. L'indexation de
recherche est un appel direct de port depuis `ScanService`, pas un événement.

```mermaid
flowchart LR
    user([L'administrateur déclenche<br/>POST /api/v1/libraries/:id/scan])

    subgraph driving2[entrant]
        h[grpcapi<br/>library_handler.go]
    end

    subgraph appLayer[internal/app/library]
        svc[ScanService<br/>pool de workers,<br/>1 goroutine par bibliothèque]
    end

    subgraph drivenScan[sortant]
        scanner[filesystem.Scanner<br/>parcours + parser]
        watcher[filesystem.FSWatcher<br/>fsnotify, incrémental]
    end

    subgraph domainScan[domain/library]
        repo[(ports MediaItemCommandRepo + MediaItemQueryRepo)]
        bus[(port EventBus)]
    end

    subgraph persist[persistance sortante]
        pgrepo[postgres.MediaItemRepository<br/>dans un TransactionalUnit]
        searchidx[meilisearch.SearchAdapter<br/>implémente library.SearchIndex]
    end

    subgraph consumers[Consommateurs d'événements - chacun dans sa goroutine]
        meta[app/metadata<br/>enrichissement TMDB/OMDb]
        trick[app/library<br/>worker trickplay]
        intro[app/library<br/>détection des intros]
        feed[app/feed<br/>invalidation du cache des genres]
        notif[app/notification<br/>alertes de nouveaux médias]
        wh[app/webhook<br/>fourre-tout, POST sortant]
    end

    user --> h --> svc
    svc --> scanner
    watcher --> svc
    scanner --> svc
    svc --> repo
    repo --> pgrepo
    svc --> searchidx
    svc --> bus
    bus -. MediaItemAdded .-> meta
    bus -. MediaItemAdded .-> trick
    bus -. MediaItemAdded .-> intro
    bus -. MediaItemAdded .-> feed
    bus -. MediaItemAdded .-> notif
    bus -. MediaItemAdded .-> wh
    trick -. TrickplayGenerated .-> intro
    meta --> repo
    trick --> ffstore[(evey-cache<br/>sprites de trickplay)]
```

À lire avec la section « Flux de données typique » ci-dessus : le chemin de
lecture est le chemin de lecture symétrique ; le pipeline de scan est le
chemin d'écriture qui prépare l'index des médias que le chemin de lecture
sert.

## Pour aller plus loin

- [DEV.md](../DEV.md) : construction, lancement, migrations, génération des proto
- [Carte du code](CODEBASE_MAP.md) : quels fichiers toucher pour les modifications courantes
- [Pipeline de lecture](playback-serving.md) : le chemin de lecture vidéo HLS, en détail
- [Référence de l'API](api/) : référence générée de l'API
- [Décisions d'architecture](adr/) : les ADR

<!-- sources
layering: internal/domain ; internal/app ; internal/adapters/driven ; internal/adapters/driving ; internal/pkg ; internal/config ; gen ; cmd/evey/wire.go, wire_infra.go, wire_services.go, wire_adapters.go, wire_events.go, wire_helpers.go
app-layer interfaces: internal/app/music/album_art_chain.go - AlbumArtFetcher ; internal/adapters/driven/ociregistry (app/updatecheck)
network exceptions: internal/app/dlna/service.go ; internal/app/loadtest/runner.go
events and subscribers: internal/app/notification/command_service.go ; internal/app/metadata/command_service_enrich.go ; internal/app/library/trickplay_service.go ; internal/app/library/intro_detection_service.go ; internal/app/feed/genre_cache.go ; internal/app/webhook/service.go ; internal/adapters/driven/email/password_reset_emailer.go ; internal/app/plugin/security_dashboard_service.go ; internal/app/config/smtp.go, timezone.go, cpu_budget.go
guards: internal/contractguard/*_test.go - TestDomainContextsDoNotImportEachOther, TestOnlyThePlanDecidesPlayback, TestNoCallerDerivesDownstreamOfThePlaybackPlan, TestServerEmittedI18nKeysExistInCatalogue, TestI18nPlaceholdersAgreeAcrossLocales
topology: docker-compose.yml ; docker-compose.override.yml ; Dockerfile - MEILI_VERSION, postgresql-17 ; docker/entrypoint.sh - PG_MAJOR
-->
