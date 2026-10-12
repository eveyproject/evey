---
source_sha: ed64eaf04c09d740775060639c8ba122fb90d8ee17950d66a387a572c3fea16d
reviewed: true
---
# 媒体库与元数据

**媒体库**就是放在一个或多个文件夹里的同一类媒体（电影、电视剧、音乐或照片）。Evey 会读取这些文件夹，根据文件名认出每个文件，再从网上获取它的详细信息：片名、简介、海报、演员、评分。这些信息叫作**元数据**。电影和电视剧的元数据来自 The Movie Database（TMDB），一个免费的在线影视数据库。

本页先讲您最先要做的三件事：添加媒体库、给文件起名、纠正认错的片名。之后再讲扫描如何进行、音乐和照片、合集，以及元数据的来源。

<figure class="evey-shot" markdown="span">
  ![演示服务器上的“管理 → 媒体库”](../images/admin-libraries.webp){ loading=lazy }
  <figcaption><strong>管理 → 媒体库</strong>：每个媒体库占一行，显示它的文件夹、条目数量和上次扫描（界面为英文）。</figcaption>
</figure>

## 添加媒体库

1. 打开**管理 → 媒体库 → 添加媒体库**。
2. 填写**名称**，选择**类型**。电影和电视剧请放在不同的媒体库里：类型告诉扫描程序要找什么。
3. 在**路径**下，从列表里选一个文件夹；也可以选择**输入自定义路径...**，自己输入。**+ 添加路径**可以给同一个媒体库再加一个文件夹。
4. 点击**创建媒体库**。Evey 会在几分钟内读取这些文件夹；想马上开始，请点击该媒体库那一行的**扫描**。海报和简介在后台陆续到达，所以大的媒体库要几分钟才会填满。

| 类型 | 识别为 | 元数据来源 |
| --- | --- | --- |
| **电影** | 每个文件（或每个文件夹）一部电影 | TMDB；设置了 OMDb 密钥时，再由 OMDb 补充 |
| **电视剧** | 剧集、季、集 | TMDB |
| **音乐** | 艺人、专辑、曲目 | 文件里的标签；封面依次取自文件、专辑文件夹、Cover Art Archive（MusicBrainz），最后是 iTunes |
| **照片** | 相册、照片及其拍摄信息 | 文件本身 |

!!! note "文件夹按 Evey 看到的样子命名"
    文件夹列表显示的是 Evey 在自己的容器里看到的文件夹。如果您用的是[安装](../installation.md)一页里的 compose 文件，您的媒体文件夹就是 `/media`。所以，您电脑上的文件夹 `/srv/media/films` 在 Evey 里就是 `/media/films`。第二个媒体文件夹（`/media2`）也是同样的道理。

## 给文件起名，让 Evey 认得出来

扫描程序能认出这样的名字：

```text
/media/films/The Mirror (1999)/The Mirror (1999) 1080p.mkv
/media/films/Lantern.2010.1080p.BluRay.x264.mkv
/media/series/The Outpost/Season 01/The.Outpost.S01E05.720p.mkv
/media/series/Quiet Storm/Quiet.Storm.03x10.mkv
```

- **电影**：年份最重要，例如 `The Mirror (1999)`。
- **剧集**：季号和集号，例如 `S01E05` 或 `1x05`。
- 名字里多余的词会被忽略：分辨率、片源、编码、音频、`VOSTFR` 或 `MULTI` 这样的语言标记，以及“Director's Cut”这样的版本名。

如果您的文件里有某个词 Evey 还不会忽略，请在**管理 → 服务器配置 → 解析器标记**里添加：单个词（如 `HDRIP`）填在**额外的质量标记**里，词组填在**额外的版本后缀**里。从下一次扫描起生效。

## 纠正错误或缺失的片名

一部电影显示的是另一部同名电影的海报？最快的办法：

1. 打开这个片名，选择**编辑元数据**。
2. 点击**在 TMDB 上识别**，搜索正确的片名。
3. 点击**使用此项**。

**编辑元数据**里还有其他选项：

- **手动修改某一栏。** 您改过的值会被保留，以后刷新时不会被覆盖。修改 **TMDB ID** 或 **IMDB ID** 后，保存时会获取这个 ID 对应的信息。
- **刷新元数据**：按当前匹配重新获取信息。
- **重置元数据**和**重新匹配元数据**：从文件名重新开始，并丢弃您的修改。重置还会删除图片和演员，对电视剧还会删除季和集。

对整个媒体库，**管理 → 媒体库**里该媒体库的那一行提供：

- **仅匹配孤立项**：重新尝试那些从未被认出的文件。速度快。
- **从 TMDB 重新匹配全部**：为每个片名重新搜索。速度慢。适合在重命名大量文件之后使用。
- **修复系列**：如果某个系列的其他电影已经在媒体库里，就把不属于任何合集的电影重新归入这个系列。

## 新文件是怎样被发现的

您什么都不用做：新增、改名或删除的文件，几分钟内就会在 Evey 里体现出来。

想立刻执行，或者想定期完整扫描，请使用该媒体库那一行：

| 操作 | 作用 |
| --- | --- |
| **扫描** | 立即读取文件夹。这一行先显示**扫描中...**，然后显示结果。无法读取的文件夹会被点名。 |
| **计划** | 按**每 6 小时**、**每 12 小时**、**每天（凌晨 3 点）**、**每周（周日凌晨 3 点）**，或按 **Cron 表达式**进行完整扫描。 |

??? info "服务器如何检查文件夹"

    - 服务器每次启动时都会读取所有媒体库，之后每 2 分钟读一次（**管理 → 服务器配置 → 服务器设置 → 定期扫描间隔**，重启后生效）。有内容正在播放时，它每隔 15 分钟才读一次，以免拖慢播放。
    - 对于开启了实时监控的媒体库，系统也可以在文件变化时通知服务器（**管理 → 服务器配置 → 服务器设置 → 媒体库文件监视**）。为了确认这项功能仍然有效，服务器会往文件夹里写入一个小测试文件，几秒后删除。已经证明这项功能有效的媒体库，改为每小时读一次，而不是每 2 分钟。
    - 在管理面板里创建的媒体库不使用文件监视：每 2 分钟一次的读取会发现它们的文件。

!!! note "网络共享"
    在 NFS 或 SMB 共享上，由另一台机器写入的文件不会给服务器发送任何信号。定期读取会发现它。

## 音乐媒体库

扫描程序读取 MP3、FLAC、M4A、OGG、Opus、WAV 和 AAC 文件，并从标签中获取每首曲目的信息：标题（没有时用文件名）、艺人、专辑艺人、专辑、年份、曲目号和碟号、流派。艺人按专辑艺人分组；这个标签为空时，按艺人分组。

**一个专辑就是一个文件夹。** 请把每张专辑放在一个文件夹里。Evey 按曲目所在的文件夹来分组，而不是按专辑标签。所以，两个专辑标签相同的文件夹会变成两张专辑；一张专辑如果分成 `CD1` 和 `CD2` 两个子文件夹，也会显示成两张。在一张专辑里，曲目先按碟号、再按曲目号排序，专辑名称取大多数曲目上的那个名字。

封面首先取文件里内嵌的图片；没有的话，取专辑文件夹里的图片（`cover.jpg`、`folder.jpg` 或 `front.jpg`）；再没有，就取 Cover Art Archive（通过 MusicBrainz 查找）；最后是 iTunes。

## 照片媒体库

扫描程序读取 JPEG、PNG、HEIC、WebP、GIF、TIFF 和 BMP 文件。照片的相册就是它所在的文件夹（相对于媒体库文件夹而言）；直接放在最上层的照片不属于任何相册。把文件移到另一个文件夹，下次扫描时它就会移到那个相册。拍摄日期和相机型号来自 JPEG 文件的 EXIF 数据；其他格式的照片不显示这些信息。缩略图在服务器上缩放，保存在转码缓存里。

## 合集

**合集**把一些片名归到一个名字下，所有账户都能看到。合集分三种：

- **TMDB 合集**：即电影系列。TMDB 把一部电影归入某个系列时，它会自动填入。**修复系列**（见上文）可以找回漏掉的电影。
- **新建合集**：由管理员手动填充的列表。在一个片名或一组选中的条目上使用**添加到合集**，还可以调整顺序。
- **新建智能合集**：按**类型**、**年份**、**媒体类型**、**评分 ≥** 和**媒体库**设定规则，片名必须全部符合。每次打开时都会重新计算内容，所以新片名会自动出现。

只有管理员可以创建和编辑合集。所有人都可以浏览合集。在网页上，**下载合集**会把原始文件打包成一个 ZIP（最多 1000 个片名），并排除该账户的媒体库权限或年龄限制所隐藏的内容。

## 元数据密钥

Evey 自带一个 TMDB 密钥，所以不用任何设置就能获取元数据。其他来源是可选的。要使用它们，请从该服务获取一个密钥（类似密码的东西），粘贴到**管理 → 服务器配置 → API 密钥**里。在这里设置的密钥优先于环境变量中给出的同一密钥（见[环境变量：媒体库与元数据](../environment/libraries.md)）。

| 密钥 | 带来什么 |
| --- | --- |
| TMDB（您自己的） | 使用您自己的请求配额，而不是共用的配额。 |
| OMDb | 对于 TMDB 了解不多的电影，多一个信息来源。 |
| OpenSubtitles | 通过服务器 API 搜索和下载文件本身没有的字幕（应用暂时还不提供这个功能）。见[字幕](../playback/subtitles.md)。 |
| TheIntroDB | 来自社区数据库的片头、前情提要和片尾时间点：有密钥每天可查询 500 次，没有密钥 100 次。 |

## 片头、前情提要和片尾

Evey 会找出剧集和电影的片头、前情提要和片尾，让播放器可以提供跳过。如果社区数据库（先查 TheIntroDB，再查 IntroDB）认识这个片名，时间点就来自那里。否则，服务器会分析文件，自己算出来。管理员可以为某个片名手动调整时间点，或重新运行检测。

播放器默认显示**跳过片头**、**跳过前情提要**和**跳过片尾**按钮。开启**自动跳过片头**（**管理 → 服务器配置 → 服务器设置**，默认关闭）后，所有播放器都会自动跳过。

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
labels: i18n admin.libraries.scan, createLibrary, addPath ; media.detail refreshMetadata, resetMetadata, rematchMetadata ; media identify openButton, useThis
-->
