# Libraries and metadata

A library is a type and a set of folders: Evey walks the folders, recognises
each file from its name and fetches its details (titles, summaries, posters,
cast, ratings). This page is for the administrator who adds one: the three
things you will do first (add a library, name the files, fix a wrong match)
come first; how scans run, music and photo specifics, collections and the
metadata sources follow.

<figure class="evey-shot" markdown="span">
  ![Admin → Libraries on the demo server](../images/admin-libraries.webp){ loading=lazy }
  <figcaption><strong>Admin → Libraries</strong>: one row per library, with its folders, its item count and its last scan.</figcaption>
</figure>

## Add a library

1. **Admin → Libraries → Add Library**.
2. Give it a **Name** and a **Type**. Keep films and series in separate
   libraries: the type tells the scanner what to look for.
3. Under **Paths**, pick a folder from the list, or **Enter custom path...**
   to type one; **+ Add Path** adds a second folder to the same library.
4. **Create Library**. The first walk of the folders follows within a couple
   of minutes; **Scan** on the row starts it right away. Posters and summaries
   arrive in the background, so a large library fills in over a few minutes.

| Type | Recognised as | Metadata from |
| --- | --- | --- |
| **Movies** | One film per file (or per folder) | TMDB, completed by OMDb when a key is set |
| **TV Shows** | Series, seasons, episodes | TMDB |
| **Music** | Artists, albums, tracks | The files' tags; covers from the files, the album folder, Cover Art Archive (MusicBrainz), then iTunes |
| **Photos** | Albums, pictures with their shooting details | The files |

!!! note "Paths are the container's"
    The folder picker lists what the container sees. With the installation's
    compose file, your media folder is `/media`, so a folder `/srv/media/films`
    on the host is `/media/films` for Evey. A second mounted folder (`/media2`)
    is added the same way.

## Name files so they are recognised

The scanner recognises names like these:

```text
/media/films/The Mirror (1999)/The Mirror (1999) 1080p.mkv
/media/films/Lantern.2010.1080p.BluRay.x264.mkv
/media/series/The Outpost/Season 01/The.Outpost.S01E05.720p.mkv
/media/series/Quiet Storm/Quiet.Storm.03x10.mkv
```

For a film, the year matters most. For an episode, `S01E05` or `1x05`. Release
tags (resolution, source, codec, audio, language tags such as `VOSTFR` or
`MULTI`, editions such as "Director's Cut") are removed before the search.

If your files carry a tag the parser does not know, add it in **Admin → Server
Config → Parser tokens**: **Extra quality tokens** for single words (`HDRIP`),
**Extra edition suffixes** for phrases. It applies from the next scan.

## Fix a wrong or missing title

For one title, open it and choose **Edit Metadata**:

- **Identify on TMDB** searches TMDB by title; **Use this match** applies the
  result. This is the quickest fix for a film matched to a namesake.
- The fields change the details by hand. A value you edit is kept: later
  refreshes do not overwrite it. Changing **TMDB ID** or **IMDB ID** fetches
  the details of that id when you save.
- **Refresh metadata** fetches the details again for the current match.
  **Reset metadata** and **Re-match metadata** start over from the file name,
  dropping your edits; Reset also wipes artwork and cast, and the seasons and
  episodes of a series.

For a whole library, the row in **Admin → Libraries** offers:

- **Match orphans only** retries the files that were never recognised. It is
  fast.
- **Re-match all from TMDB** redoes the search for every title. It is slow,
  and corrects old mistakes after a large renaming.
- **Repair sagas** joins films that belong to no collection back to their
  saga, when the other films of the saga are already in the library.

## How new files are picked up

The server walks every library at each start, then every 2 minutes
(**Admin → Server Config → Server Settings → Periodic scan interval**, applied
on restart). While something is playing, the walks are spaced out to 15
minutes so they do not compete with playback for the disk. A new, renamed or
deleted file is therefore seen within minutes without any setting.

Two more ways to scan, on the library's row:

| Action | Effect |
| --- | --- |
| **Scan** | A walk now. The row shows **Scanning...** then the result; a folder that could not be read is named. |
| **Schedule** | A full scan **Every 6 hours**, **Every 12 hours**, **Daily (3 AM)**, **Weekly (Sunday 3 AM)**, or on a **Cron expression**. |

The server also watches folders for file system events (**Admin → Server
Config → Server Settings → Library file watcher**), on the libraries whose
realtime monitoring is on. It checks that the watch still answers by writing a
probe file into the folder for a few seconds; a library whose watch is proven
alive is walked once an hour instead of every 2 minutes, the others keep the
periodic walk. Libraries created from the panel are not watched: the periodic
walk is what picks their files up.

!!! note "Network shares"
    On an NFS or SMB share, a file written by another machine raises no event
    on the server. The periodic walk is what finds it.

## Music libraries

The scanner reads MP3, FLAC, M4A, OGG, Opus, WAV and AAC files, and takes each
track from its tags: title (the file name when it is missing), artist, album
artist, album, year, track and disc numbers, genre. Artists are grouped by
album artist, or by artist when that tag is empty.

**An album is a folder.** Tracks are grouped by the folder they sit in, not by
their album tag: two folders sharing one album tag are two albums, and an album
split into `CD1` and `CD2` subfolders shows as two. Keep each album in one
folder; its tracks are ordered by disc number, then track number, and the album
takes the name most of its tracks carry.

The cover is the picture embedded in the files, otherwise an image in the album
folder (`cover.jpg`, `folder.jpg` or `front.jpg`), otherwise Cover Art Archive
(found through MusicBrainz), otherwise iTunes.

## Photo libraries

The scanner reads JPEG, PNG, HEIC, WebP, GIF, TIFF and BMP files. A photo's
album is its folder, relative to the library folder; a photo at the top level
has none, and moving a file to another folder moves it to that album at the
next scan. The date taken and the camera model come from the EXIF data of JPEG
files; the other formats are listed without them. Thumbnails are resized on the
server and kept in the transcoding cache.

## Collections

**Collections** gathers titles under one name, for every account. There are
three kinds:

- **TMDB Collections**: the sagas, filled in on their own when TMDB places a
  film in one. **Repair sagas** (above) catches films that missed theirs.
- **New collection**: a list an administrator fills by hand, with **Add to
  collection** on a title or on a selection, and puts in order.
- **New smart collection**: rules on **Genre**, **Year**, **Type**, **Rating ≥**
  and **Library**, which a title must all meet. Its content is worked out each
  time it is opened, so new titles appear without doing anything.

Only administrators create and edit collections. Anyone can browse them, and
**Download collection** on the web gets the original files as one ZIP (up to
1000 titles), leaving out what the account's library access or age limit hides.

## Metadata keys

Evey ships with a TMDB key, so metadata works without configuration. Other
sources are optional; their keys go in **Admin → Server Config → API Keys**,
and a key set there wins over the same key given in the environment (see
[Environment: libraries and metadata](../environment/libraries.md)).

| Key | Adds |
| --- | --- |
| TMDB (your own) | Your own request quota instead of the shared one. |
| OMDb | A second source for films TMDB knows little about. |
| OpenSubtitles | Searching and downloading subtitles a file does not carry, through the server API (the apps do not offer it yet). See [Subtitles](../playback/subtitles.md). |
| TheIntroDB | Intro, recap and credits timings from a community database: 500 lookups a day with a key instead of 100 without. |

## Intro, recap and credits

Evey marks the intro, the recap and the credits of episodes and films so the
players can offer to skip them. The timings come from community databases
(TheIntroDB, then IntroDB) when they know the title, and from the server's own
analysis of the files otherwise. An administrator can adjust the segments of a
title by hand, or run the detection again.

Players show **Skip Intro**, **Skip Recap** and **Skip Credits** buttons by
default. **Auto-skip intros** (**Admin → Server Config → Server Settings**, off
by default) makes every player skip them on its own.

<!-- sources
add library: frontend/apps/web/src/features/admin/LibraryForm.tsx (name, type, paths only - no realtime/scan-on-startup toggles) ; i18n admin.libraries.addLibrary, tvShows, paths, enterCustomPath, addPath, createLibrary ; internal/domain/library/model.go NewLibrary (RealtimeMonitoring false by default) ; grpcapi/library_handler.go CreateLibrary (no scan trigger ; only watcher + feed subscribe library.created)
types: i18n admin.libraries.types.* ; internal/adapters/driven/metadata/musicbrainz_provider.go ; internal/app/music/album_art_chain.go
parser: internal/adapters/driven/filesystem/parser.go (reEditionSuffix, knownQualities, SetExtraParserTokens) ; internal/app/library/parser_config.go ApplyParserConfig ; i18n admin.serverConfig.parserTokens*
fix a title: frontend/apps/web/src/features/library/MediaDetailEdit.tsx (save, refresh, reset, rematch, identify) ; IdentifyMetadataModal.tsx ; http/admin_handler_media.go (identify search) ; internal/app/metadata/command_service_overrides.go SetOverride, ResetItemMetadata (overrides, artwork, persons, Catalog=nil, children) ; command_service_refresh.go RefreshItemMetadata ; i18n media.detail.editMetadata, identify.*, tmdbId, imdbId, idChangedWarning, refreshMetadata, resetMetadata, rematchMetadata, rematchConfirm
per library: i18n admin.libraries.matchOrphans, refreshMetadata, relinkCollections (+Tooltip) ; internal/app/metadata/relink_collections.go
scans: internal/app/library/command_service.go StartPeriodicScan (startup pass, ticker, runScanPass) ; scan_service.go playbackScanBackoff 15m, watchedScanInterval 1h, shouldScanNow ; cmd/evey/wire_events.go (library.periodic_scan_interval, all libraries) ; internal/config/config.go EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL 2m ; i18n admin.serverSettings.periodicScanInterval, restartHint, libraryWatcher ; admin.libraries.scan, scanning, scanPartial, schedule, presets.* ; routes/_auth/admin/libraries.lazy.tsx PRESET_SCHEDULES ; internal/app/library/scheduler.go
watcher: internal/app/library/watcher.go Start (RealtimeMonitoring && Enabled) ; watcher_probe.go probeOnce, UnhealthyLibraries (network share blind spot) ; cmd/evey/wire_helpers.go (WatcherEnabled) ; command_service.go WithWatchHealth
music: internal/adapters/driven/filesystem/music_scanner.go (audioExts) ; internal/app/music/command_service.go - parseTrack (AlbumFolder = parent dir, title fallback) ; postgres/music_track_repo.go - GetAlbums GROUP BY album_folder, mode() display_album, ORDER BY disk_number, track_number ; GetArtists COALESCE(album_artist, artist)
album art order: internal/app/music/album_art_folder.go (folderCoverNames, embedded wins) ; cmd/evey/wire_services.go (musicbrainz then itunes)
photos: internal/adapters/driven/filesystem/photo_scanner.go (imageExts) ; internal/app/photo/command_service.go - albumOf, processOnePhoto (EXIF JPEG only) ; http/photo_handler.go - serveTranscoded (cache photos/)
collections: internal/domain/library/model.go Collection (TmdbID, Rules), MaxCollectionItems 1000 ; internal/adapters/driven/postgres/collection_repo.go (genre, type, year, library_id, rating_gte, AND) ; internal/app/collection/query_service.go GetItems ; interceptors/rbac.go CollectionService ; http/collection_handler.go (library access + parental filter) ; i18n media.collections.*, media.library.addToCollection, media.playlist.field*
keys: i18n admin.serverConfig.apiKeys.* ; admin.serverConfig.introProvidersTidbAuthed ; cmd/evey/wire_infra.go resolveAPIKeyWithBuiltin
opensubtitles: internal/app/subtitle/service.go
intro providers: docs/adr/0011-intro-detection-providers.md ; i18n admin.serverSettings.introAutoSkip
-->
