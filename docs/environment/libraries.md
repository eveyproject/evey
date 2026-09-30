# Libraries and metadata

Scanning, metadata sources and intro detection. Most of these have a
counterpart in the admin panel, which wins once a value is saved there.

## Scanning

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_LIBRARY_WATCHER_ENABLED` | `true` | Realtime file watching. |
| `EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL` | `2m` | How often the folders are checked for what the watcher missed (network shares). **Admin → Server Config → Server Settings** overrides it. |
| `EVEY_LIBRARY_SCAN_WORKERS` | from the CPU share | Files analysed in parallel during a scan. Set by hand, it bypasses the **Processing power** share for scans. |
| `EVEY_SEEKINDEX_STATE_DIR` | `/data/seek-index` | Where the Media health scan keeps its results and its running job across restarts. Empty: not kept. |

## Metadata

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_METADATA_LANGUAGE` | `en` | The language of titles and summaries. The language chosen in the setup wizard or in **Admin → Server Config → Language & Region** wins. |
| `EVEY_TMDB_API_KEY` | built-in key | Your own TMDB key, for your own quota. The key set in **Admin → Server Config → API Keys** wins. |
| `EVEY_OMDB_API_KEY` | empty | An OMDb key, a second source for films. Same override. |
| `EVEY_OPENSUBTITLES_API_KEY` | empty | An OpenSubtitles key, for subtitle search through the API. Same override. |
| `EVEY_OPENSUBTITLES_USERNAME`, `EVEY_OPENSUBTITLES_PASSWORD` | empty | An OpenSubtitles account, used to sign in to its API. |

## Intro, recap and credits

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_INTRO_PROVIDERS_ENABLED` | `false` | Uses the community databases (TheIntroDB, then IntroDB) before the server's own detection. **Admin → Server Config → Intro Skip Providers** wins; restart after changing it. |
| `EVEY_INTRO_TIDB_API_KEY` | empty | A TheIntroDB key: 500 lookups a day instead of 100. |
| `EVEY_INTRO_AUTO_SKIP_ENABLED` | `false` | Every player skips detected intros on its own. Applies live from the panel. |
| `EVEY_INTRO_SKIP_PREVIEW_ENABLED` | `false` | Also offers to skip the preview of the next episode. Applies live from the panel. |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | from the CPU share | Titles analysed at once. Set by hand, it bypasses the **Processing power** share for this task. |
| `EVEY_INTRO_FFMPEG_THREADS` | `1` | Threads per ffmpeg process during detection (1 to 16). |

<!-- sources
internal/config/config.go - EVEY_LIBRARY_*, EVEY_SEEKINDEX_STATE_DIR, EVEY_METADATA_LANGUAGE, EVEY_INTRO_*
scan workers: internal/app/library/scan_service.go - scanWorkersFromEnv
api keys: cmd/evey/wire_infra.go - resolveAPIKeyWithBuiltin ; cmd/evey/wire_services.go (opensubtitles)
language: cmd/evey/wire_infra.go (metadata.language wins)
intro: cmd/evey/wire.go (EVEY_INTRO_DETECTION_MAX_PARALLEL) ; internal/adapters/driven/ffmpeg/intro_threads.go
-->
