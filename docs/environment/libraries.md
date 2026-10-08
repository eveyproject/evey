# Libraries and metadata

The variables behind scanning, metadata sources and intro detection. Most of
them have a field in the admin panel, which wins once a value is saved
there; the panel side is described in
[Libraries and metadata](../administration/libraries.md).

## Scanning

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_LIBRARY_WATCHER_ENABLED` | `true` | Realtime file watching. | Server Config → Server Settings → **Library file watcher**, restart |
| `EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL` | `2m` | How often the folders are checked for what the watcher missed (network shares). | Server Config → Server Settings → **Periodic scan interval**, restart |
| `EVEY_LIBRARY_SCAN_WORKERS` | from the CPU share | Files analysed in parallel during a scan. Set by hand, it bypasses the **Processing power** share for scans. | none |
| `EVEY_SEEKINDEX_STATE_DIR` | `/data/seek-index` | Where the Media Health scan keeps its results and its running job across restarts. Empty: not kept. | none |

## Metadata

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_METADATA_LANGUAGE` | `en` | The language of titles and summaries. The language chosen in the setup wizard wins over it. | Server Config → Language & Region → **Metadata language**, live |
| `EVEY_TMDB_API_KEY` | built-in key | Your own TMDB key, for your own quota. | Server Config → API Keys → **TMDB API Key**, restart |
| `EVEY_OMDB_API_KEY` | empty | An OMDb key, a second source for films. | Server Config → API Keys → **OMDB API Key**, restart |
| `EVEY_OPENSUBTITLES_API_KEY` | empty | An OpenSubtitles key, for subtitle search through the API. | Server Config → API Keys → **OpenSubtitles API Key**, restart |
| `EVEY_OPENSUBTITLES_USERNAME`, `EVEY_OPENSUBTITLES_PASSWORD` | empty | An OpenSubtitles account, used to sign in to its API. | none |

## Intro, recap and credits

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_INTRO_PROVIDERS_ENABLED` | `false` | Uses the community databases (TheIntroDB, then IntroDB) before the server's own detection. | Server Config → Intro Skip Providers → **Enable external intro skip providers**, restart |
| `EVEY_INTRO_TIDB_API_KEY` | empty | A TheIntroDB key: 500 lookups a day instead of 100. | Server Config → API Keys → **TheIntroDB API Key**, restart |
| `EVEY_INTRO_AUTO_SKIP_ENABLED` | `false` | Every player skips detected intros on its own. | Server Config → Server Settings → **Auto-skip intros**, live |
| `EVEY_INTRO_SKIP_PREVIEW_ENABLED` | `false` | Also offers to skip the preview of the next episode. | Server Config → Server Settings → **Skip preview segments**, live |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | from the CPU share | Titles analysed at once. Set by hand, it bypasses the **Processing power** share for this task. | none |
| `EVEY_INTRO_FFMPEG_THREADS` | `1` | Threads per ffmpeg process during detection (1 to 16). | none |

<!-- sources
internal/config/config.go - EVEY_LIBRARY_*, EVEY_SEEKINDEX_STATE_DIR, EVEY_METADATA_LANGUAGE, EVEY_INTRO_*
watcher / scan interval: cmd/evey/wire_events.go (loadOverride library.watcher_enabled, library.periodic_scan_interval) ; internal/adapters/driving/http/settings_handler.go (requiresRestart)
scan workers: internal/app/library/scan_service.go - scanWorkersFromEnv
api keys: cmd/evey/wire_infra.go - resolveAPIKeyWithBuiltin, resolveAPIKey ; cmd/evey/wire_services.go (opensubtitles) ; i18n admin.serverConfig.apiKeys.*
language: cmd/evey/wire_infra.go (metadata.language wins) ; internal/app/config/locale_defaults.go
intro: cmd/evey/wire.go (intro_providers.enabled, api.tidb_key, EVEY_INTRO_DETECTION_MAX_PARALLEL) ; internal/adapters/driven/ffmpeg/intro_threads.go ; i18n admin.serverConfig.introProvidersRestart ; admin.serverSettings.introAutoSkip, skipPreview
-->
