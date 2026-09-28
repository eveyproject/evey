---
source_sha: 5c6e8f241b8d7ece14cf4136bad0d6ef75c0ebe244984e6c99eda87eab1f6dc9
---
# Arquitectura

Evey es un servidor multimedia autoalojado. El backend está en Go (hexagonal / DDD), el frontend es un monorepo TypeScript/React y la aplicación móvil es React Native / Expo. La puesta en marcha y los detalles internos están en [DEV.md](../DEV.md).

## Capas

Organización estricta en puertos y adaptadores bajo `internal/`:

- `internal/domain/<context>/` - dominio puro: modelos, objetos de valor, **puertos** (interfaces). Sin importaciones de frameworks.
- `internal/app/<context>/` - servicios de aplicación. Orquestan los puertos. La E/S pasa normalmente por un puerto; dos contextos son excepciones deliberadas y van directamente a la red: `dlna` (SSDP / UPnP AVTransport) y `loadtest` (su propio cliente HTTP).
- `internal/adapters/driven/` - adaptadores de salida que implementan los puertos del dominio: `postgres`, `meilisearch`, `ffmpeg`, `mkvgo`, `filesystem`, `prober`, `opensubtitles`, `email`, `webpush`, `webhooksender`, `pluginrunner`, `metadata`, `introproviders`, `search`. Otros dos viven aquí pero satisfacen una interfaz de la capa de aplicación en lugar de un puerto del dominio: `itunes` y `musicbrainz`, ambos `app/music.AlbumArtFetcher`.
- `internal/adapters/driving/` - adaptadores de entrada: `grpcapi` (+ HTTP de grpc-gateway), `http` (estáticos, middleware de autenticación, SSE, HLS, recursos de los iframes de extensiones), `pluginhost` (servidor gRPC sobre un socket Unix, autenticado por credenciales del par - la API del anfitrión a la que llaman los subprocesos aislados de las extensiones).
- `internal/pkg/` - núcleo técnico compartido entre contextos (auth, logger, eventbus, i18n, metrics, tracing, ratelimit, procfork, ssrf, cache, ...). La configuración del servidor vive en `internal/config/`; el código proto generado en `gen/`.

Regla de dependencias: `adapters -> app -> domain`. El dominio nunca importa app ni adapters. La composición se hace en `cmd/evey/wire*.go` - cableado escrito a mano, no google/wire: `wire.go`, `wire_infra.go`, `wire_services.go`, `wire_adapters.go`, `wire_events.go`, `wire_helpers.go`.

## Contextos delimitados

Treinta paquetes de contexto viven bajo `internal/domain/` e `internal/app/`: 19 existen en las dos capas, 2 solo en el dominio (`ratings`, `shared`), 9 solo en la aplicación. La colaboración se hace en parte con eventos de dominio publicados en un bus compartido; la capa de aplicación también lee directamente el paquete de dominio de un contexto vecino (vea [Relaciones](#relations)).

| Contexto | Responsabilidad |
| --- | --- |
| `user` | cuentas, autenticación, sesiones, preferencias |
| `library` | bibliotecas, carpetas, trabajos de escaneo, índice de medios, personas, colecciones, favoritos y pulgares de un perfil (los pulgares no tienen servicio de aplicación: la RPC de valoración guarda y lee `UserRatingRepository` tal cual), trickplay, detección de intros/créditos |
| `media` (solo aplicación) | enlaces para compartir y tokens de flujo |
| `mediarequest` | solicitudes de medios enviadas por los usuarios |
| `metadata` | enriquecimiento TMDB / OMDb, carátulas |
| `playback` | flujos, progreso, control remoto, reproducción en curso |
| `transcoding` | trabajos ffmpeg, segmentos HLS |
| `subtitle` | incrustados + externos + opensubtitles |
| `mkv` | operaciones sobre contenedores MKV (mediante `mkvgo`) |
| `music` | reproducción específica de audio + etiquetado |
| `photo` | bibliotecas de fotos |
| `favorite` (solo aplicación) | favoritos por perfil; el modelo y `FavoriteRepository` viven en `domain/library` |
| `playlist` | listas de usuario, listas inteligentes |
| `collection` (solo aplicación) | colecciones que cuida el administrador; sus puertos viven en `domain/library` |
| `ratings` (solo dominio) | traslada las clasificaciones de cada país (CSA, FSK, MPAA, BBFC, ACB) a un único eje de edad mínima |
| `feed` | flujo de actividad / descubrir |
| `activity` | historial de actividad del usuario |
| `chromecast` | ciclo de vida de las sesiones de cast |
| `syncplay` | sesiones sincronizadas entre varios dispositivos |
| `plugin` | registro de extensiones, tokens del entorno aislado, ciclo de vida |
| `notification` | notificaciones dentro de la aplicación + web push |
| `webhook` | webhooks salientes |
| `backup` | copia y restauración de base de datos + configuración |
| `config` (solo aplicación) | ajustes persistentes del servidor, escritos por los puntos de entrada REST y gRPC |
| `dlna` (solo aplicación) | descubrimiento SSDP y control UPnP AVTransport |
| `loadtest` (solo aplicación) | simula espectadores HLS contra la propia pila HTTP del servidor |
| `maintenance` (solo aplicación) | mantenimiento de medios lanzado por el administrador: reconstrucción del índice de búsqueda, diario de reparaciones, deshacer, integridad del audio |
| `presence` (solo aplicación) | registro en memoria, no persistido, de quién está conectado ahora |
| `qoe` (solo aplicación) | ingesta y agregación de la telemetría de reproducción CMCD (CTA-5004) |
| `shared` (solo dominio) | puertos transversales (`EventBus`, `TransactionalUnit`, `ServerConfigRepository`, `ShareLinkRepository`), ids tipados, cursores, tokens de flujo |

### Relaciones

- `library` → emite `LibraryCreated` / `LibraryDeleted`, `LibraryScanStarted` / `LibraryScanCompleted`, `MediaItemAdded`, `MediaItemRemoved`, `TrickplayGenerated`. `media.item.added` lo consumen `notification`, `metadata` (enriquecimiento automático), el propio `library` (worker de trickplay + detección de intros) y `feed` (invalidación de la caché de géneros).
- La indexación de la búsqueda **no** va por eventos: `library.ScanService` llama directamente al puerto `library.SearchIndex`.
- `playback` → emite `PlaybackStarted` / `PlaybackStopped`. Ninguno tiene hoy un suscriptor dedicado; solo los recibe el comodín `"*"` de `webhook`.
- `user` → emite `UserCreated`, que no tiene suscriptor, y `user.password.reset_requested`, que consume el adaptador de salida `email`.
- `plugin` → `pluginrunner` publica `plugin.error` / `plugin.started`, y `app/plugin` los consume para su panel de seguridad. No hay ningún puente que lleve los eventos de dominio a las extensiones: las extensiones llegan a la API con JWT de ámbito limitado (vea ADR-0003). El bus en sí es ADR-0010.
- `shared.TransactionalUnit` envuelve las escrituras sobre varios agregados (vea ADR-0002).

Ningún paquete bajo `internal/domain` importa el paquete de dominio de otro contexto - una regla que impone `internal/contractguard/domain_imports_test.go` (vea [Guardas de arquitectura](#architecture-guards)). La capa de aplicación no está sujeta a ella: 14 paquetes de aplicación importan hoy el paquete de dominio de un contexto vecino.

## Flujo de datos típico - «el usuario reproduce un medio»

1. El cliente móvil/web llama a `POST /api/v1/playback/start` (ruta de grpc-gateway).
2. `internal/adapters/driving/http` ejecuta su middleware de autenticación (`auth_middleware.go`, `httpAuthMiddleware`) y extrae la sesión.
3. `internal/adapters/driving/grpcapi` (`playback_handler.go`) valida el proto y llama a `app/playback.CommandService.StartStream` (separación de órdenes y consultas, vea ADR-0009).
4. `app/playback.CommandService` llama a los puertos del dominio: `user.UserRepository`, `playback.SessionQueryRepo` (idempotencia + límite de dispositivos), `playback.SessionCommandRepo`, y luego `shared.EventBus.Publish(PlaybackStarted)`.
5. Los adaptadores de salida ejecutan: `postgres` persiste la sesión dentro de un ámbito `shared.TransactionalUnit.Do`; el bus en proceso (`internal/pkg/eventbus`) distribuye de forma asíncrona.
6. `PlaybackStarted` no tiene hoy suscriptor dedicado: solo lo recibe el comodín `"*"` de `webhook`.
7. La respuesta vuelve por grpc-gateway como JSON; las actualizaciones de progreso circulan por SSE / flujos gRPC.

## Raíz de composición

Los grafos de cableado viven en `cmd/evey/wire.go` y sus hermanos (`wire_infra.go`, `wire_services.go`, `wire_adapters.go`, `wire_events.go`, `wire_helpers.go`). Las pruebas evitan los adaptadores inyectando fakes que implementan los mismos puertos del dominio - 21 de los 22 paquetes de dominio incluyen un subpaquete `fakes/`. Vea [ADR-0001](adr/0001-hexagonal-architecture.md).

## Guardas de arquitectura

Las reglas de arriba no son solo documentación. `internal/contractguard/` contiene 31 pruebas estáticas repartidas en 20 archivos que hacen fallar la compilación en cuanto el código se aparta de ellas.

La regla hexagonal en sí es `TestDomainContextsDoNotImportEachOther` (ADR-0001), en `domain_imports_test.go`. Recorre `internal/domain` e impone exactamente esto: un contexto puede importar `internal/domain/shared` y su propio árbol de paquetes, nada más de `internal/domain`, y nunca `internal/app` ni `internal/adapters`. Su alcance termina en `internal/domain` - no dice nada de la capa de aplicación.

Las demás vigilan contratos que cruzan una frontera de lenguaje o de proceso, entre ellas:

- `TestNoClientReadOfAFieldTheServerNeverWrites` - todo campo de respuesta proto que lea un cliente debe escribirse en algún lugar del servidor.
- `TestRegisteredHandlersGetEveryPortTheyDeclare` y `TestBootInstallsItsSilentWirings` - cableado que de otro modo fallaría en silencio.
- `TestPostgresRepositoriesBoundTheirQueries` - todo repositorio Postgres limita sus consultas con un tiempo de espera.
- `TestProcfork_EveryForkGoesThroughCommand` - todo fork de subproceso pasa por `internal/pkg/procfork`.
- `TestEverySortFieldTheAPIAcceptsHasASQLColumn` y `...CanBuildACursorValue` - los campos de ordenación existen tanto en el mapa de columnas SQL como en el codificador de cursores.
- `TestEveryWebSurfaceReachingAnAdminRPCIsClassified`, más su equivalente nativo para móvil y TV - toda superficie de la interfaz que llama a una RPC reservada a administradores decide quién la ve.
- `TestEveryAllowedSlotIsMountedOnWebAndMobile` y `TestPluginBridgeParityWebMobile` - paridad de las ranuras de extensiones entre clientes.
- `TestImageSubtitleCodecsAgreeAcrossServerAndClients` - una única lista de códecs de subtítulos en imagen, para servidor y reproductores.
- `TestHandWrittenClientTypesMatchTheEndpointTheyCall` - los tipos de respuesta escritos a mano en móvil/TV corresponden al endpoint al que llaman.
- `TestRestartExitCodeAgreesWithTheEntrypoint` - el código de salida de Go y `docker/entrypoint.sh` coinciden en qué significa un reinicio.
- Guardas de i18n - las claves que emite el servidor existen en el catálogo, los marcadores coinciden entre idiomas.

## Para saber más

- [DEV.md](../DEV.md) - compilar, ejecutar, migraciones, generación de proto
- [docs/CODEBASE_MAP.md](CODEBASE_MAP.md) - qué archivos tocar para los cambios habituales
- [docs/playback-serving.md](playback-serving.md) - la ruta de lectura del servicio de vídeo HLS, en profundidad (decisión, escalera, huella, CMAF/HLS/DASH, mkvgo frente a ffmpeg)
- [docs/api/](api/) - referencia de la API generada
- [docs/adr/](adr/) - registros de decisiones de arquitectura

## Diagramas C4

Estos cuatro diagramas documentan Evey con un zoom decreciente según el [modelo C4](https://c4model.com/). Todos se dibujan con Mermaid; sin archivos binarios. Los nombres de los nodos son los nombres reales de los paquetes Go bajo `internal/`. Los nombres de los contenedores salen de `docker-compose.yml` para `evey-server`, `evey-postgres` y `evey-caddy`; `evey-web`, `evey-meilisearch`, `evey-mobile` y `evey-mailcatcher` se declaran en `docker-compose.override.yml` y solo existen en la topología de desarrollo.

### Nivel 1 - Contexto del sistema

Quién habla con Evey y a qué terceros llama Evey.

```mermaid
flowchart LR
    user[User<br/>web / mobile / TV client]
    admin[Administrator<br/>web /admin UI]
    oidc[OIDC Identity Provider<br/>optional SSO]
    tmdb[TMDB API<br/>movie/series metadata]
    omdb[OMDb API<br/>fallback metadata + ratings]
    imdb[IMDbAPI / TVmaze<br/>fallback series metadata]
    art[MusicBrainz / iTunes Search<br/>album artwork]
    introdb[IntroDB / TIDB<br/>intro + credits timestamps]
    opensubs[OpenSubtitles<br/>subtitle search]
    webpush[Web Push gateway<br/>FCM / APNs / VAPID]

    subgraph evey[Evey Media Server]
        core((Self-hosted media platform<br/>library, playback, transcoding,<br/>music, photo, plugins))
    end

    user -->|HTTPS<br/>HLS / SSE / gRPC-Web| evey
    admin -->|HTTPS<br/>/admin| evey
    evey -.->|OIDC code flow| oidc
    evey -.->|HTTPS metadata fetch| tmdb
    evey -.->|HTTPS metadata fetch| omdb
    evey -.->|HTTPS metadata fetch| imdb
    evey -.->|HTTPS artwork fetch| art
    evey -.->|HTTPS intro lookup| introdb
    evey -.->|HTTPS subtitle fetch| opensubs
    evey -.->|HTTPS push delivery| webpush
```

### Nivel 2 - Contenedores

Las unidades desplegables que levanta `docker compose --env-file .env.dev up`. Las flechas continuas son tráfico en ejecución; las discontinuas, salidas hacia terceros.

```mermaid
flowchart TB
    user([User browser / mobile])
    admin([Admin browser])

    subgraph compose[Docker Compose network]
        caddy[evey-caddy<br/>Caddy 2.9<br/>:80 / :443<br/>TLS, reverse proxy]
        web[evey-web<br/>Vite dev / nginx prod<br/>:5173 dev<br/>React SPA + RN web]
        server[evey-server<br/>Go binary<br/>:9091 HTTP+gRPC-gateway<br/>internal :9090 gRPC]
        pg[(evey-postgres<br/>PostgreSQL 16<br/>:5432)]
        meili[(evey-meilisearch<br/>Meilisearch 1.12<br/>:7700)]
        mobile[evey-mobile<br/>dev only<br/>:8081 Expo / Metro bundler]
        mail[evey-mailcatcher<br/>dev only<br/>:1025 SMTP / :1080 UI]
    end

    tmdb[[TMDB / OMDb / OpenSubtitles]]
    fs[(/media volumes<br/>read-only bind mounts)]
    cache[(evey-cache volume<br/>transcode + trickplay)]
    data[(evey-data volume<br/>keys, logs, backups,<br/>plugins, avatars)]

    user -->|HTTPS| caddy
    admin -->|HTTPS /admin| caddy
    caddy -->|HTTP /| web
    caddy -->|HTTP /v1, /api, SSE, HLS| server
    web -->|gRPC-Web via /v1| server
    user -.->|Metro dev bundle| mobile

    server -->|SQL/TCP 5432| pg
    server -->|HTTP 7700<br/>indexing + search| meili
    server -.->|SMTP 1025 dev| mail
    server -.->|HTTPS| tmdb
    server -->|read FS| fs
    server -->|read/write FS| cache
    server -->|read/write FS| data
```

> **La producción difiere de esta topología de desarrollo.** El compose de
> instalación declara solo tres servicios - `evey`, `postgres`, `caddy`. No hay
> contenedor `evey-web` aparte (ni el mailcatcher ni el bundler Metro, que solo
> existen en desarrollo): el binario `evey-server` sirve la interfaz web
> compilada desde `/srv`, así que el proxy inverso reenvía *todo* - interfaz y
> API - a él en `:9091`. Tampoco hay contenedor `evey-meilisearch` aparte: el
> binario de Meilisearch viene integrado en la imagen `evey` (`Dockerfile`, etapa
> `meili-download`, fijado en 1.12.8) y guarda su índice en `/data/meili`. Vea
> [installation.md](installation.md) y [reverse-proxy.md](reverse-proxy.md).

### Nivel 3 - Componentes (zoom sobre `evey-server`)

Capas hexagonales dentro del binario Go. Los adaptadores de entrada llaman a los servicios de aplicación a través de sus interfaces; los servicios de aplicación solo dependen de los puertos del dominio; los adaptadores de salida implementan esos puertos.

```mermaid
flowchart LR
    subgraph driving[Driving adapters - inbound]
        grpc[grpcapi<br/>internal/adapters/driving/grpcapi<br/>handlers per context]
        gw[grpc-gateway<br/>JSON/HTTP <-> gRPC<br/>proto-generated]
        http[http<br/>internal/adapters/driving/http<br/>static, SSE, HLS,<br/>auth_middleware.go]
        plug[pluginhost<br/>internal/adapters/driving/pluginhost<br/>gRPC over Unix socket<br/>peer-credential auth]
    end

    subgraph app[Application layer - internal/app]
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
        a_misc[favorite, playlist, collection,<br/>feed, activity,<br/>chromecast, syncplay,<br/>plugin, notification, webhook,<br/>backup, dlna, config, mediarequest,<br/>maintenance, presence, qoe, loadtest]
    end

    subgraph domain[Domain core - internal/domain]
        d_models[Aggregates + Value Objects<br/>per bounded context]
        d_ports[Outbound Ports<br/>Repos - command/query split,<br/>EventBus, TransactionalUnit,<br/>ServerConfigRepository]
    end

    subgraph driven[Driven adapters - outbound]
        pgad[postgres<br/>repos + migrations<br/>shared.TransactionalUnit]
        meiad[meilisearch<br/>search index]
        fsad[filesystem<br/>scanner, parser, watcher]
        ffad[ffmpeg<br/>transcoder, trickplay]
        mkvad[mkvgo<br/>MKV ops]
        metaad[metadata<br/>TMDB, OMDb, IMDbAPI,<br/>TVmaze clients]
        introad[introproviders<br/>IntroDB + TIDB chain]
        itunad[itunes<br/>album artwork]
        mbad[musicbrainz<br/>album artwork]
        probead[prober<br/>mkvgo fast path,<br/>ffprobe fallback]
        searchad[search<br/>resilient facade over meili]
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
    d_ports -.implements.- pgad
    d_ports -.implements.- meiad
    d_ports -.implements.- fsad
    d_ports -.implements.- ffad
    d_ports -.implements.- mkvad
    d_ports -.implements.- metaad
    d_ports -.implements.- introad
    d_ports -.implements.- probead
    a_music -.app-layer iface.- itunad
    a_music -.app-layer iface.- mbad
    d_ports -.implements.- searchad
    d_ports -.implements.- osad
    d_ports -.implements.- emad
    d_ports -.implements.- wpad
    d_ports -.implements.- whad
    d_ports -.implements.- prad

    pgad --> pg2[(PostgreSQL)]
    meiad --> me2[(Meilisearch)]
    fsad --> fs2[(/media bind mounts)]
    ffad --> cc2[(evey-cache + ffmpeg binary)]
    metaad -.-> ext2[[TMDB / OMDb / IMDbAPI / TVmaze]]
    introad -.-> ext3[[IntroDB / TIDB]]
    itunad -.-> ext4[[iTunes Search / MusicBrainz]]
    mbad -.-> ext4
```

Notas sobre las goroutines:
- El servidor gRPC (`grpc.Server.Serve`) y el servidor HTTP (gateway + estáticos) corren cada uno en su propia goroutine, derivada del cableado principal en `cmd/evey`.
- Los trabajos de escaneo de `app/library` corren en un pool de workers (una goroutine por escaneo de biblioteca activo, vea `library.ScanService`).
- Los trabajos ffmpeg los lanza con fork el adaptador de salida `ffmpeg` (`ffmpeg.Transcoder.Start`) a través de `internal/pkg/procfork`, con una goroutine supervisora por trabajo que vacía el stderr del proceso.
- `shared.EventBus` distribuye a los suscriptores de forma asíncrona: cada suscriptor corre en su propia goroutine para que un consumidor lento nunca bloquee a quien publica. El bus no abre ninguna transacción propia - eso le toca al handler.
- `pluginrunner` lanza un subproceso por extensión; las extensiones en iframe viven en el navegador del usuario y responden mediante tokens firmados que emite `app/plugin`.

### Nivel 3 (extra) - Zoom de componentes sobre la cadena de escaneo

La ruta caliente que convierte archivos del disco en medios enriquecidos y buscables. Las flechas continuas son llamadas directas; las discontinuas, eventos de dominio en el bus. Fíjese en que la indexación de la búsqueda es una llamada directa a un puerto desde `ScanService`, no un evento.

```mermaid
flowchart LR
    user([Admin triggers<br/>POST /api/v1/libraries/:id/scan])

    subgraph driving2[driving]
        h[grpcapi<br/>library_handler.go]
    end

    subgraph appLayer[internal/app/library]
        svc[ScanService<br/>worker pool,<br/>1 goroutine per library]
    end

    subgraph drivenScan[driven]
        scanner[filesystem.Scanner<br/>walk + parser]
        watcher[filesystem.FSWatcher<br/>fsnotify, incremental]
    end

    subgraph domainScan[domain/library]
        repo[(MediaItemCommandRepo + MediaItemQueryRepo ports)]
        bus[(EventBus port)]
    end

    subgraph persist[driven persistence]
        pgrepo[postgres.MediaItemRepository<br/>inside TransactionalUnit]
        searchidx[meilisearch.SearchAdapter<br/>implements library.SearchIndex]
    end

    subgraph consumers[Event consumers - each in its own goroutine]
        meta[app/metadata<br/>TMDB/OMDb enrichment]
        trick[app/library<br/>trickplay worker]
        intro[app/library<br/>intro detection]
        feed[app/feed<br/>genre cache invalidation]
        notif[app/notification<br/>new media alerts]
        wh[app/webhook<br/>catch-all, outbound POST]
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
    trick --> ffstore[(evey-cache<br/>trickplay sprites)]
```

Léalo junto a la sección «Flujo de datos típico» de arriba: la ruta de reproducción es la ruta de lectura simétrica; la cadena de escaneo es la ruta de escritura que llena el índice de medios del que se sirve la ruta de lectura.
