# Codebase map - where things live and how to change them

The 30-minute orientation for a new contributor. [DEV.md](../DEV.md) gets your
environment running; [docs/ARCHITECTURE.md](ARCHITECTURE.md) explains the
layering philosophy; THIS document tells you which files to touch for the
five most common kinds of change, in order, with the gotchas that actually
bite.

## The one diagram that matters

```
proto/evey/v1/*.proto            the API contract (gRPC + HTTP gateway annotations)
        │  make proto-gen (buf) → gen/ (gitignored)
        ▼
internal/adapters/driving/       "the world calls us"
  grpcapi/                       gRPC handlers (one file per service)
  http/                          raw-HTTP handlers (streaming, HLS, assets, subtitles)
        ▼
internal/app/<domain>/           use cases (command/query services, workers)
        ▼
internal/domain/<name>/          pure models + PORTS (interfaces) + typed events
        ▲ implemented by
internal/adapters/driven/        "we call the world"
  postgres/                      repositories (+ migrations/)
  ffmpeg/  mkvgo/  prober/       media engines
  ...
cmd/evey/wire.go                 composition root: EVERYTHING is wired here
```

Frontend mirror: `frontend/apps/web` (React 19 + TanStack Router + Tailwind
tokens), `apps/mobile` (Expo/React Native), `apps/tv` (react-native-tvos),
`packages/api` (orval-generated client from the gateway OpenAPI - regenerate
with `pnpm generate` in that package, run inside the `evey-web` container).

## Golden path 1 - add an API endpoint (gRPC + HTTP)

1. `proto/evey/v1/<service>.proto` - add the rpc + messages, with a
   `google.api.http` annotation (that is what generates the REST route and
   the orval client).
2. `make proto-gen` (or `buf generate` inside the `evey-server` container),
   then **restart the dev server** - Air does not watch `gen/`.
3. Handler in `internal/adapters/driving/grpcapi/<service>.go`. Guards:
   `requireClaims(ctx)`, and `checkLibraryAccess`/`requireMediaAccess` on
   anything taking a library/media id (IDOR pattern - reviewers look for it).
4. Use case in `internal/app/<domain>/` - handlers stay thin.
5. Wiring: `cmd/evey/wire.go` (constructor) + `GRPCServerDeps` if the handler
   needs a new dependency.
6. Frontend: regenerate the client (`pnpm generate` in `packages/api`), then
   consume the generated `use<Service><Rpc>` hook.
7. Tests: handler test beside the handler (fakes live in
   `internal/domain/<name>/fakes/`), service test beside the service.

## Golden path 2 - add a database change

1. `internal/adapters/driven/postgres/migrations/NNNNNN_name.up.sql` + `.down.sql`
   (next number; migrations run at server boot).
2. **IDs are TEXT slugs (`usr-…`, `lib-…`, `mi-…`), never UUID columns.** Check
   the real column type with `\d <table>` before writing a foreign key.
3. Repository in `internal/adapters/driven/postgres/` - and remember every
   SELECT list in that repo that must now carry your new column (GetByID AND
   the list queries; a partial read means a later full-row UPDATE silently
   erases the field).
4. Domain struct field + port doc in `internal/domain/<name>/`.
5. Air does not re-run migrations on live containers reliably - `docker
   restart evey-server` after adding one.

## Golden path 3 - add a web UI page

1. Route file in `frontend/apps/web/src/routes/` (TanStack file-based:
   `foo.tsx` = route config, `foo.lazy.tsx` = component).
2. Data via the generated hooks from `@evey/api`; never raw `fetch` - the
   shared `eveyFetch` carries auth/refresh/CSRF.
3. Strings: NEVER hardcoded. Keys live server-side in `i18n/<lang>/*.yaml`
   (all SIX languages: en, fr, de, es, hi, zh) and must also be added to the
   test fixture `frontend/apps/web/src/test/en-translations.json`.
4. Styling: design tokens only (`var(--accent)`, `var(--surface)`…); no
   hardcoded colors, no em-dashes anywhere.
5. Tests: vitest + testing-library beside the page. Run inside the container:
   `docker exec evey-web sh -c 'cd /app/apps/web && pnpm vitest run <file>'`,
   same for `pnpm tsc --noEmit`.

## Golden path 4 - touch the transcoding pipeline (read this twice)

> Full flow (mental model, reading order, patterns, cost split, where to
> improve): [docs/playback-serving.md](playback-serving.md).

The player is VITAL and the plan↔ffmpeg contract is subtle: three audible
desyncs shipped in one day once, invisible to every unit suite.

- Decision layer: `internal/app/transcoding/playback_plan_resolver.go`
  (`PlanResolver.Resolve` - decision, ladder, fingerprint resolved ONCE per
  request; the http adapter only marshals a `PlaybackRequest`, see
  `http/transcode_vod_plan.go`). RULE: every input of the decision must be
  baked into the child URLs (`querySuffix` in
  `internal/app/transcoding/manifest_generator.go`) or a mid-play playlist
  poll can flip the decision under the player (that class of bug rendered
  green frames on Chromecast once). ADRs 0022 + 0024.
- Cache identity: `DecisionFingerprint` in `internal/app/transcoding/segment_cache.go`
  - different bytes MUST mean different `Hash()`. New dimensions are hashed
  conditionally (only when non-default) so existing caches are not orphaned.
- ffmpeg args: `internal/adapters/driven/ffmpeg/hls_transcoder.go` -
  `BuildFFmpegArgs` is PURE (probes happen in `Start`), five builders (copy ×3
  branches, single/multi software, single/multi hardware).
- **`make transcode-itest` is MANDATORY before declaring any transcode change
  ready.** It runs the real producer against the real bundled ffmpeg with an
  adversarial MKV and correlates the served AUDIO CONTENT. Wired into
  `make pre-push` as an auto-gate on transcode paths.

## Golden path 5 - build a plugin

Reference implementation: `tests/plugins/weather-v2` (zero-runtime-dep WebGL
+ DDD; the packaging script `build-zip.sh` knows the analyzer's traps - e.g.
the literal string "npm install" anywhere in the zip is a CRITICAL reject).
Frontends run in a sandboxed iframe (web) / WebView (mobile) with the
`window.evey` bridge (resize, prefs per profile, assets.read, media APIs via
a scoped token the plugin never sees). Backend `main.js` may only reach the
hosts declared under `external_fetch` and may only write inside its own
folder. Full docs: [docs/plugins.md](plugins.md).

## Quality gates (the order that saves you time)

```
go vet <package>                after every Go edit
pnpm tsc --noEmit               after every frontend edit (per app)
go test <package>               before every commit (NEVER go test ./...)
make transcode-itest            if you touched transcode paths
make pre-push                   before declaring anything ready (vet + vuln +
                                build + gosec + lint + arch guards + itest gate)
make doctor                     when your environment misbehaves
```

## Vocabulary decoder

| Term | Meaning |
|---|---|
| copy rung | HLS quality that REMUXES the source video (`-c:v copy`) instead of re-encoding |
| on-demand / od_ | mkvgo packager serving HLS straight from the container, no ffmpeg |
| rafale | a seek-spawned ffmpeg session producing segments from a mid-file keyframe |
| decision / fingerprint | what to serve for (media × client caps × prefs), and its cache key |
| keyframe backfill | background worker filling `keyframes_ms` so the copy rung can cut segments |
| seek-index repair / media health | triaging every MKV/WebM into one verdict (repairable here / needs you / for information), repairing the first group, acknowledging the rest (admin > Media health) |
| SDUI | sanitized server-driven UI schema (`web/src/features/sdui`) |
| tripwire | a runtime assertion that logs ERROR + metric instead of corrupting playback |
