# Architecture

Evey is a self-hosted media server. Backend is Go (hexagonal / DDD), frontend is a TypeScript/React monorepo, mobile is React Native / Expo. See [DEV.md](../DEV.md) for setup and deeper internals.

## Layering

Strict ports-and-adapters layout under `internal/`:

- `internal/domain/<context>/` — pure domain: models, value objects, **ports** (interfaces). No framework imports.
- `internal/app/<context>/` — application services. Orchestrate ports. No IO.
- `internal/adapters/driven/` — outbound adapters implementing domain ports: `postgres`, `meilisearch`, `ffmpeg`, `mkvgo`, `filesystem`, `opensubtitles`, `email`, `webpush`, `webhooksender`, `pluginrunner`, `metadata`, `search`.
- `internal/adapters/driving/` — inbound adapters: `grpcapi` (+ grpc-gateway HTTP), `http` (static, auth middleware, SSE), `pluginhost` (iframe sandbox).
- `internal/pkg/` — shared kernel reused across contexts (auth, logging, proto types, config).

Dependency rule: `adapters -> app -> domain`. Domain never imports app or adapters. Composition happens in `cmd/.../wire.go`.

## Bounded contexts

Twenty contexts live under `internal/domain/` and `internal/app/`. Most are independent; collaboration happens via domain events published through a shared bus, not direct imports.

| Context | Responsibility |
| --- | --- |
| `user` | accounts, auth, sessions, preferences |
| `library` | libraries, folders, scan jobs, media index |
| `media` (app only) | unified media read model over library |
| `metadata` | TMDB / OMDb enrichment, artwork |
| `playback` | streams, progress, remote control, now-playing |
| `transcoding` | ffmpeg jobs, HLS segments |
| `subtitle` | embedded + external + opensubtitles |
| `mkv` | MKV container ops (via `mkvgo`) |
| `music` | audio-specific playback + tagging |
| `photo` | photo libraries |
| `favorite` | per-user favorites |
| `playlist` | user playlists |
| `collection` | admin-curated collections |
| `rating` | user ratings |
| `feed` | activity feed / discover |
| `activity` | user activity timeline |
| `chromecast` | cast session lifecycle |
| `syncplay` | multi-device sync sessions |
| `plugin` | plugin registry, sandbox tokens, lifecycle |
| `notification` | in-app + webpush notifications |
| `webhook` | outbound webhooks |
| `backup` | DB + config backup/restore |
| `shared` | cross-cutting ports (transactions, clock, ids, events) |

### Relations

- `library` → emits `MediaItemAdded` / `MediaItemUpdated` / `MediaItemRemoved`; `metadata`, `feed`, `search` consume.
- `playback` → emits `PlaybackStarted/Stopped`; `activity`, `notification`, `syncplay` consume.
- `user` → emits `UserCreated`; `favorite`, `playlist`, `notification` bootstrap per-user state.
- `plugin` → consumes domain events through a scoped bus bridge (see ADR-0003).
- `shared.TransactionalUnit` wraps multi-aggregate writes (see ADR-0002).

No context imports another context's domain package directly. Cross-context reads go through the `media` app-layer read model or through exposed app-service interfaces.

## Typical data flow — "user plays a media"

1. Mobile/web client calls `POST /api/v1/playback/start` (grpc-gateway route).
2. `internal/adapters/driving/http` runs `HTTPAuthMiddleware`, extracts session.
3. `internal/adapters/driving/grpcapi` (`playback_handler.go`) validates proto, calls `app/playback.Service.StartSession`.
4. `app/playback.Service` calls domain ports: `MediaRepo.Get`, `SessionRepo.Create`, `EventBus.Publish(PlaybackStarted)`.
5. Driven adapters execute: `postgres` persists the session inside a `shared.TransactionalUnit.Do` scope; `eventbus` dispatches asynchronously.
6. Consumers (`activity`, `notification`, `syncplay`) react via their own subscribers, each in its own transaction.
7. Response streams back through grpc-gateway as JSON; progress updates flow over SSE / gRPC streams.

## Composition root

Wire graphs live in `cmd/evey/wire.go` (and friends). Tests bypass adapters by injecting fakes that implement the same domain ports — see [ADR-0001](adr/0001-hexagonal-architecture.md).

## Further reading

- [DEV.md](../DEV.md) — build, run, migrations, proto generation
- [docs/api/](api/) — generated API reference
- [docs/adr/](adr/) — architectural decision records

## C4 diagrams

These four diagrams document Evey at decreasing zoom levels following the [C4 model](https://c4model.com/). All diagrams render with Mermaid; no binary assets. Names match the actual Go packages under `internal/` and the container names in `docker-compose.yml`.

### Level 1 — System Context

Who talks to Evey, and which third parties Evey reaches out to.

```mermaid
flowchart LR
    user[User<br/>web / mobile / TV client]
    admin[Administrator<br/>web /admin UI]
    oidc[OIDC Identity Provider<br/>optional SSO]
    tmdb[TMDB API<br/>movie/series metadata]
    omdb[OMDb API<br/>fallback metadata + ratings]
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
    evey -.->|HTTPS subtitle fetch| opensubs
    evey -.->|HTTPS push delivery| webpush
```

### Level 2 — Container

The deployable units brought up by `docker compose --env-file .env.dev up`. Solid arrows are runtime traffic; dashed arrows are outbound to third parties.

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

    server -->|SQL/TCP 5432| pg
    server -->|HTTP 7700<br/>indexing + search| meili
    server -.->|SMTP 1025 dev| mail
    server -.->|HTTPS| tmdb
    server -->|read FS| fs
    server -->|read/write FS| cache
    server -->|read/write FS| data
```

> **Production differs from this dev topology.** The install compose has no
> separate `evey-web` container (nor the dev-only mailcatcher): the `evey-server`
> binary serves the built web UI from `/srv`, so the reverse proxy forwards
> *everything* — UI and API — to it on `:9091`. See
> [installation.md](installation.md) and [reverse-proxy.md](reverse-proxy.md).

### Level 3 — Component (zoom on `evey-server`)

Hexagonal layering inside the Go binary. Driving adapters call application services through their interfaces; application services depend only on domain ports; driven adapters implement those ports.

```mermaid
flowchart LR
    subgraph driving[Driving adapters - inbound]
        grpc[grpcapi<br/>internal/adapters/driving/grpcapi<br/>handlers per context]
        gw[grpc-gateway<br/>JSON/HTTP <-> gRPC<br/>proto-generated]
        http[http<br/>internal/adapters/driving/http<br/>static, SSE, HLS,<br/>HTTPAuthMiddleware]
        plug[pluginhost<br/>internal/adapters/driving/pluginhost<br/>iframe sandbox]
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
        a_misc[favorite, playlist, collection,<br/>rating, feed, activity,<br/>chromecast, syncplay,<br/>plugin, notification,<br/>webhook, backup, dlna, config]
    end

    subgraph domain[Domain core - internal/domain]
        d_models[Aggregates + Value Objects<br/>per bounded context]
        d_ports[Outbound Ports<br/>Repo, EventBus, Clock, Ids,<br/>TransactionalUnit]
    end

    subgraph driven[Driven adapters - outbound]
        pgad[postgres<br/>repos + migrations<br/>shared.TransactionalUnit]
        meiad[meilisearch<br/>search index]
        fsad[filesystem<br/>scanner, parser, watcher]
        ffad[ffmpeg<br/>transcoder, trickplay]
        mkvad[mkvgo<br/>MKV ops]
        metaad[metadata<br/>TMDB + OMDb clients]
        searchad[search<br/>read facade over meili]
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
    metaad -.-> ext2[[TMDB / OMDb]]
```

Goroutine notes:
- gRPC server (`grpc.Server.Serve`) and HTTP server (gateway + static) each run in their own goroutines off the main wire-up in `cmd/evey`.
- `app/library` scan jobs run on a worker pool (one goroutine per active library scan, see `library.ScanService`).
- `app/transcoding` ffmpeg jobs are supervised by a per-session goroutine that pipes ffmpeg stdout/stderr.
- `shared.EventBus` dispatches subscribers asynchronously: each subscriber runs in its own goroutine + transaction so a slow consumer never blocks publishers.
- `pluginrunner` spawns one subprocess per plugin; iframe plugins live inside the user browser and talk back via signed tokens issued by `app/plugin`.

### Level 3 (bonus) — Component zoom on the scan pipeline

The hot path that turns files on disk into searchable, enriched media. Solid arrows are direct calls; dashed arrows are domain events on the bus.

```mermaid
flowchart LR
    user([Admin triggers<br/>POST /v1/libraries/:id/scan])

    subgraph driving2[driving]
        h[grpcapi<br/>library_handler.go]
    end

    subgraph appLayer[internal/app/library]
        svc[ScanService<br/>worker pool,<br/>1 goroutine per library]
    end

    subgraph drivenScan[driven]
        scanner[filesystem.Scanner<br/>walk + parser]
        watcher[filesystem.Watcher<br/>fsnotify, incremental]
    end

    subgraph domainScan[domain/library]
        repo[(MediaItemRepo port)]
        bus[(EventBus port)]
    end

    subgraph persist[driven persistence]
        pgrepo[postgres.MediaItemRepo<br/>inside TransactionalUnit]
        searchidx[meilisearch.Indexer]
    end

    subgraph consumers[Event consumers - each in own goroutine + tx]
        meta[app/metadata<br/>TMDB/OMDb enrichment]
        trick[app/transcoding<br/>trickplay generator]
        feed[app/feed<br/>discover + recently added]
        notif[app/notification<br/>new media alerts]
        wh[app/webhook<br/>outbound POST]
    end

    user --> h --> svc
    svc --> scanner
    watcher --> svc
    scanner --> svc
    svc --> repo
    repo --> pgrepo
    svc --> bus
    bus --> searchidx
    bus -. MediaItemAdded .-> meta
    bus -. MediaItemAdded .-> trick
    bus -. MediaItemAdded .-> feed
    bus -. MediaItemAdded .-> notif
    bus -. MediaItemAdded .-> wh
    meta --> repo
    trick --> ffstore[(evey-cache<br/>trickplay sprites)]
```

Read this with the "Typical data flow" section above: the playback path is the symmetric read path; the scan pipeline is the write path that primes the read model.
