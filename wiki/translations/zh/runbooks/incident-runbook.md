---
source_sha: 95bca5df52c2505d699a3857e7d305e19067387a32df91af30da948dfa493ae0
reviewed: true
---
# 事故手册

本页写给值班的运维人员，说明 Evey 常见故障的初步处理。找到和症状相符的小节，按其中的方法检查，再根据检查结果采取对应的修复办法。每个小节的结构都一样：**症状**、**诊断**、**修复**。

## 开始之前

这些命令针对标准安装：只有一个容器 `evey-server`，Postgres 和 Meilisearch 都在里面运行。它们的日志行和服务器的日志出现在同一个日志里（数据库的日志行以 `postgres:` 开头）。代码仓库里的 `docker-compose.yml` 另外加了 `evey-postgres` 和 `evey-caddy` 两个容器；见[高级部署](../operations/advanced-deployments.md)。

**端口。** HTTP API 监听 `EVEY_SERVER_PORT` + 1，默认是 `9091`；`/healthz` 也在这个端口上应答。compose 里的 `ports:` 那一行负责把它对外公开。

在内置数据库上打开 **SQL 命令行**：

```sh
docker exec -it evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

**日志。** 同样的日志流有两份：每天一个的文件 `./evey-data/logs/evey-YYYY-MM-DD.log`（`EVEY_LOG_DIR`，镜像内是 `/data/logs`），以及 `docker logs evey-server`。以文件为准：Docker 的 json-file 驱动会轮转 `docker logs` 的内容，轮转之后，这条命令显示的日志可能比容器实际停止的时间早好几个小时就断了。两者不一致时，请看文件。管理面板的**日志与调试**页面搜索的也是同一个日志流（`GET /api/v1/admin/logs/search?q=<term>&limit=200`）；在那里，bearer 令牌、JWT 以及 `token=` / `password=` 的值都会显示为 `[redacted]`，所以请搜索媒体 ID，不要搜索令牌。

## 服务器没有应答

**症状**：客户端无法连接；`curl http://localhost:9091/healthz` 超时或被拒绝。

**诊断**：

```sh
docker ps --filter name=evey-server
docker inspect --format='{{.State.Health.Status}}' evey-server
docker logs --tail=200 evey-server
ss -ltnp | grep 9091                # 端口到底有没有在监听？
curl -v http://localhost:9091/healthz
```

**修复**：

- 容器已经退出：panic 信息在 `docker logs evey-server` 的末尾。解决原因后，运行 `docker compose up -d evey`。
- 健康检查变红，是因为数据库或搜索引擎停止运行：见[数据库错误](#database-errors)和[搜索功能降级](#search-is-degraded)。
- 主机上端口冲突：修改 compose 里 `ports:` 那一行的主机端口，或者释放这个端口。
- 前面的 Caddy 不转发请求（代码仓库的部署方式）：查看 `docker logs evey-caddy`，检查 `EVEY_DOMAIN`，并确认 `evey` 的健康检查是绿色的，因为 Caddy 会等待它就绪。

## 每次登录都返回 401

**症状**：所有用户，包括管理员，从 `/api/v1/auth/*` 得到的都是 401。

**诊断**：

```sh
docker logs --since 10m evey-server | grep -iE 'epoch|jwt|auth|signing|private key'
docker exec evey-server ls -l /data/jwt.key
```

先找关于 epoch 的那一行。每个经过身份验证的请求，都会到数据库里重新检查用户的令牌 epoch，而这项检查在出问题时一律拒绝：只要数据库很慢、被锁住或者无法连接，每个请求都会返回和令牌过期时完全相同的 401，看起来像是 JWT 的问题，其实不是。服务器会在日志中明确说明：

```
auth rejected: the token epoch could not be read, so every authenticated request is failing - check the database, not the JWT key
```

如果有这一行，请去看[数据库错误](#database-errors)。这期间 `/healthz` 可能仍然是绿色的：它只测试连接，不执行 epoch 查询。对应的 Prometheus 计数器是 `evey_auth_failures_total{reason="epoch_lookup_failed"}`。

**修复**：

- **JWT 密钥丢失或改变，很少是原因。** 密钥文件（`EVEY_JWT_PRIVATE_KEY_PATH`，`/data/jwt.key`）无法读取时，服务器根本无法启动；文件不存在时，会重新生成一个。这时旧密钥签发的访问令牌会被拒绝，但应用会用刷新令牌来续签，而刷新令牌是在数据库里检查的，和密钥无关：用户仍然保持登录。请确认 `./evey-data` 卷挂载在 `/data`；没有它，数据库也不存在。
- **每个请求都触发了限流**：`EVEY_RATELIMIT_LOGIN_MAX`（每个 IP 每分钟 `10` 次）和 `EVEY_RATELIMIT_GLOBAL_MAX`（`300`）。如果反向代理把所有请求都以同一个地址转发过来，很快就会触发限流：在**管理 → 远程访问**里把这个代理加到**受信任的代理**中（无需重启就会生效；`EVEY_TRUSTED_PROXIES` 是后备方式）。代码仓库里的 `docker-compose.override.yml` 在开发时把两者分别提高到 `1000` / `10000`。
- **服务器和客户端之间的时钟不一致**：在主机上运行 `date -u` 查看。

## 转码播放失败

**症状**：直接播放正常，需要转码的播放失败；播放器报错，服务器日志里有 ffmpeg 的退出码。

**诊断**：

```sh
docker exec evey-server which ffmpeg ffprobe
docker exec evey-server ffmpeg -version | head -1
docker logs --since 10m evey-server | grep -iE 'ffmpeg|transcode|hls'
docker exec evey-server ls -ld /cache/transcode /data/trickplay
docker exec evey-server df -h /cache
docker exec evey-server touch /cache/transcode/.write-test && \
  docker exec evey-server rm /cache/transcode/.write-test
```

**修复**：

- **没有 ffmpeg**（自己定制的镜像）：用官方的 `Dockerfile` 重新构建。
- **缓存目录不可写**：镜像设置了 `EVEY_TRANSCODING_CACHE_DIR=/cache/transcode`（`evey-cache` 卷）和 `EVEY_TRICKPLAY_CACHE_DIR=/data/trickplay`；compose 里 `environment:` 下的设置会覆盖它们。在镜像之外，Go 代码里的默认值是 `/data/transcode` 和 `/cache/trickplay`。
- **缓存已满**：见[缓存磁盘已满](#the-cache-disk-is-full)。
- **硬件加速工作不正常**：在**管理 → 转码 → 编码校准**里把**硬件加速**设为**关闭（仅软件）**（立即生效，并重新校准），让所有转码都在处理器上进行；硬件修好后，再改回**自动（最佳可用）**。
- **“服务器正忙于转码其他流”**：实时容量是测量出来的，不是设置出来的；见[分配这份能力](../playback/transcoding.md#sharing-the-capacity)。`EVEY_TRANSCODING_MAX_CONCURRENT`（**管理 → 转码缓存**里的**最大并发**）只适用于实时播放以外的转码任务。

### 播放缓冲或中断，日志却没有说明原因

**症状**：一位观众反映，某部转码播放的影片会卡住；日志里有 `hls-vod: segment timeout` 和 `err:"context canceled"`，意思是客户端挂断了连接，而不是服务器超时。

**诊断**：先看服务器已经记录下来的内容，这不需要额外开销：

```sh
# 某个片段请求为什么沿用了一个编码器、或者新启动了一个，以及这让播放器付出了什么代价。
# 只有产生代价时才会记录。`over_client_patience: true` 就是确凿的证据。
docker exec evey-server grep '"software: reach decision"' /data/logs/evey-$(date +%F).log | tail -40

# 编码器本身：启动、终止，以及终止的原因。
docker exec evey-server grep -E '"software: session (started|killed)"|per-hash cap' \
  /data/logs/evey-$(date +%F).log | tail -40
```

这些日志行说明的是服务器做了什么决定。要想逐秒看到 ffmpeg 实际做到了什么，请打开每个编码器的进度日志（`$TOKEN` 是一个管理员 JWT）：

```sh
# 打开。对下一个编码器生效，无需重启。
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"true"}'

# 重现问题，然后读取一个会话：out_time = 到目前为止产出的内容，
# speed = 编码器比实时快了多少。
docker exec evey-server ls -t /data/logs/ffmpeg | head
docker exec evey-server tail -20 /data/logs/ffmpeg/<file>.log

# 用完后关闭。
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"false"}'
```

!!! note
    不在排查问题时，请让进度日志保持关闭：每个编码器一个文件，每个会话几 MB，这台机器上的每次播放都会产生。文件名里带有决策哈希、档位（`-vN-`）和起始片段（`-sN-`），所以能和 JSON 日志对应起来；在同一位置重新启动的编码器会有自己单独的文件，正是这一点让编码器的反复启动清晰可见。

**修复**：

- **反复短距离向前跳转**：每次跳转都可能启动一个新的编码器，而每个哈希的上限（`MaxSessionsPerHash`，默认 2，低内存配置下为 1）会终止最久未使用的那个：日志里显示 `session killed reason:lru_cap`。适度出现属于正常。
- **客户端的等待时间比服务器短**：跳转可达窗口是按 `clientSegmentPatience`（6 秒）来定的，而服务器自己的等待预算（`WaitBudgetDur`）是 60 秒。如果一行 `reach decision` 带有 `over_client_patience: true`，说明服务器正准备让播放器等待超过任何播放器所能等待的时间。

### `stale_rendition` 410，播放器在播放中途重新获取主播放列表

**症状**：播放器短暂卡住，然后恢复；日志里有 `hls-vod: stale rendition URL refused (the ladder moved under a live player)`，请求得到的回复是 `410 Gone`，带有 `X-Evey-Error: stale_rendition`。

这是一项保护机制在起作用，本身并不是故障。每个清单 URI 都带有生成它时的决策哈希（`fp=`）。410 表示播放器出示的 URL 来自一个会话，而这个会话的 ABR 档位阶梯后来已经改变了；如果照样提供，就会把另一个版本的字节交给解码器。拒绝请求会迫使播放器完整地重新获取主播放列表。

**诊断**：这条 WARN 带有 `sealed_fp`、`current_fp` 和媒体 ID。

- **在部署、重启或重新校准之后立即出现一阵**是正常的：正在进行的会话经历了一次能力变化。播放器在一次轮询之内就会重新同步。
- **在正常运行期间持续零星出现**，说明有某种机制在不断改变正在播放的视频流的档位阶梯：检查能力是否反复变化（`docker logs evey-server | grep -iE 'calibrat|capability'`）、准入调节器是否在负载下频繁变动，或者 CMCD 上限覆盖是否在来回摆动。
- **在 410 上反复循环**的播放器，持有的是缓存下来的主播放列表（旧版应用，或者缓存了 `.m3u8` 的代理）。代理必须把 `Cache-Control: no-cache` 原样传过去。

## 数据库错误

**症状**：日志里有 `dial tcp ... connect: connection refused`、`pq: ...`，或者查询超时。

**诊断**：

```sh
docker logs --tail=300 evey-server | grep -iE 'postgres|database'
docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
  SELECT count(*), state
    FROM pg_stat_activity
   WHERE datname = current_database()
   GROUP BY state;"'
df -h ./evey-data
```

**修复**：

- `./evey-data` 下**磁盘已满**（数据库集群位于 `./evey-data/postgres`）：腾出空间，然后 `docker compose restart evey`。
- **连接池耗尽**（`too many connections`，等待时间很长）：`EVEY_DB_MAX_OPEN_CONNS`（默认 `25`）。终止卡住的会话：
  ```sql
  SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
   WHERE state = 'idle in transaction'
     AND now() - state_change > interval '5 minutes';
  ```
- **独立的 Postgres 容器**（代码仓库的部署方式）：`docker ps --filter name=evey-postgres`、`docker logs --tail=200 evey-postgres`；服务器的 `EVEY_DB_USER` / `EVEY_DB_PASSWORD` / `EVEY_DB_NAME` 必须和 `postgres` 服务一致。

## 搜索功能降级

**症状**：搜索结果变差了；日志里有 `meilisearch unavailable, using SQL search fallback`。

无法连接 Meilisearch 时，搜索会通过 SQL 继续工作；只是排序质量下降，不会有人因此无法使用。Meilisearch 运行在 `evey-server` 里面（`EVEY_EMBEDDED_MEILI`，默认开启），使用容器的回环地址，端口 7700。

**诊断**：

```sh
docker exec evey-server curl -sf http://127.0.0.1:7700/health
docker logs --tail=300 evey-server | grep -i meili
```

**修复**：

- 状态不健康：`docker compose restart evey`。
- 索引损坏：停止容器，把 `./evey-data/meili` 移到别处，再启动，然后从数据库重建索引：
  ```sh
  docker exec evey-server evey admin reindex-search
  ```
  重建是一条命令，不是面板里的操作。

## 缓存磁盘已满

**症状**：转码失败，提示 `no space left on device`；健康检查时好时坏。

**诊断**：

```sh
docker exec evey-server df -h /cache
docker exec evey-server du -sh /cache/transcode /data/trickplay 2>/dev/null
docker volume inspect evey_evey-cache   # compose 加的前缀可能不同
```

**修复**：

- 清空转码缓存，它可以重新生成：
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/transcode/*'
  ```
  这也会删除其中的 `photos/`（调整过大小的照片版本）和 `track-art/`（从音频标签中提取的封面）；两者都会在下次请求时重建。**管理 → 转码缓存**里的**清除全部**可以在面板里清空同一个缓存。
- trickplay 重新生成的代价更高；只在必要时才清空它：
  ```sh
  docker exec evey-server sh -c 'rm -rf /data/trickplay/*'
  ```
- 如果主机上的卷空间紧张，在**管理 → 服务器配置 → 服务器设置**里调低**转码缓存预算（GB）**（默认 `10`）。不管预算是多少，清理程序都会在缓存卷上保留 2 GiB 的空闲空间。

## 插件崩溃了

**症状**：某个插件不再响应；日志里有来自插件运行器的 panic。

**诊断**：

```sh
docker logs --since 30m evey-server 2>&1 | grep -iE 'plugin|pluginrunner|panic'
# 要找的是："panic in plugin runner"、"plugin process crashed"、"remove plugin sock on crash"
```

**修复**：

1. 无需改动服务器，重新启动插件：在**管理 → 插件**里再次**启用**它，或者 `POST /api/v1/admin/plugins/<id>/enable`。当插件处于启用状态、却卡在 `Error` 时，这样做有效：启动流程只会拒绝已经处于 `Running` 或 `Initializing` 状态的插件。
2. 如果这样还不行，在面板里**禁用**这个插件。
3. 如果服务器本身不稳定：`docker compose restart evey`。
4. 插件文件位于主机上的 `./evey-data/plugins/`（容器内是 `/data/plugins`）。删除出问题的插件的目录，然后重启。
5. 把 panic 堆栈和插件 ID 发给插件作者。

## 投屏卡住、没有声音，或者手机一直显示加载中

这里的所有信息都可以在**管理 → 日志与调试**里看到；不需要登录到机器上的 shell。时间都是 UTC。每个信标都是一行 `player-client: playback event reported`，带有 `type=` 和一个 `details=` 字符串。

1. **接收端当时是否在运行？运行到什么时候？** 搜索 `cast_receiver_alive`。接收端在播放时每 30 秒发送一次；`details` 里带有 `buf=<behind>/<ahead>/<ranges>`（播放位置前后缓冲了多少秒）和 `pos_sec`。播放正常时，`ahead` 是几十秒。最后一个信标的时间，就是接收端停止运行或被终止的时间：在那之后什么都没有，就说明电视已经不再运行接收端了，不管手机上显示的是什么。
2. **它为什么停了？** 搜索 `cast_receiver_finished`：`reason=INTERRUPTED` 表示一个新的 LOAD 替换了正在播放的内容（查找几秒钟前是否有 `StartStream`），`reason=ERROR` 带有播放器的错误。如果最后一个 `alive` 之后没有 `finished`，说明接收端进程消失了，最常见的情况是 Chromecast HD 在播放 20 Mbps 以上的直接复制主播放列表时，因内存不足被终止。
3. **手机看到了什么？** 搜索 `cast_sender_stall`：当 Cast SDK 仍然显示 PLAYING、但播放位置 20 秒都没有动时，发送端会发出 `state=playing frozen=<n>s pos=<p>`；当接收端报告了空闲原因时，会发出 `state=idle reason=<r>`。这些是 WARN 级别的日志行，所以页面的警告筛选会把它们显示出来。把它的 `pos` 和接收端最后一个 `pos_sec` 比较一下。
4. **会话有没有引发连锁反应？** 搜索 `refresh rejected`：刷新令牌 `not found` 会让应用退出登录再重新登录（用保存的凭据），从而重新发起视频流；这一行带有令牌的 `jti`、`issued` 和 `age`，可以据此分辨是一份过时的副本还是伪造的令牌。搜索 `StartStream` 可以看到重新发起的情况。
5. **视频流对设备来说太大了吗？** 搜索 `advertises a fallback rung`（直接复制主播放列表的峰值码率）和 `plan fitted to the client's line`（接收端测得的 `maxKbps`，或者它的 `abr=floor` 补救；随后的转码主播放列表会带有原因 `client_line_exceeded` 或 `abr_floor_requested`）。如果一个会话从 20 Mbps 以上的直接复制主播放列表开始，几秒钟后降到一个适配过的 1080p 档位，那是补救机制在正常工作；如果重新 LOAD 后又从直接复制主播放列表开始，这正是会让 Chromecast HD 崩溃的模式。
6. **是服务器无法及时提供数据吗？** 搜索 `on-demand resource refused`：连续几个片段出现一连串 `context canceled` 拒绝，是在拖动进度条（每个请求都被下一个请求取消了），不是服务器的问题；持续时间很长的 `served` 日志行才是。

如果第 1-3 步显示接收端仍在运行、发送端却没有任何记录，问题出在手机的投屏会话上（SDK，或者到电视的网络）。如果接收端的信标先停了，就去看电视：内存、编码、视频流的码率。

## 停止一个会话或一台被盗用的设备

**症状**：**管理 → 正在播放**里出现了一台意料之外的设备，或者有用户反映自己的会话列表里有一台不认识的设备。

1. 找出这个会话：
   ```sh
   docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
     SELECT id, profile_id, device_name, client_ip, started_at
       FROM play_sessions
      WHERE ended_at IS NULL
      ORDER BY started_at DESC LIMIT 20;"'
   ```
2. 在**管理 → 正在播放**里停止它：点击会话上的停止图标（**停止会话**），然后点击**停止**确认；或者通过 gRPC 用会话 ID 调用 `AdminSessionService/KillSession`。播放器会先收到一条停止命令，5 秒后视频流结束。这只会结束这次播放；用户仍然保持登录。
3. 如果视频流还是在继续，就重启容器：所有编码器都会随之停止。
   ```sh
   docker compose restart evey
   ```
4. 如果账户本身已经被盗用，使用**管理 → 用户 → 禁用**：刷新令牌会被删除，访问令牌会立即被吊销，所以所有设备都会退出登录。设置一个新密码（`evey admin reset-password --user <email>`，见[恢复](recovery.md#give-a-user-a-new-password)），然后再次**启用**这个账户。

## 媒体库扫描卡住了

**症状**：扫描显示进行中已经好几个小时，日志里却没有任何文件进度。

1. 检查扫描是否还在写日志；卡住的扫描，CPU 是空闲的，最近也没有 `scan:` 日志行：
   ```sh
   docker logs --tail=500 evey-server | grep 'scan:'
   ```
2. 扫描锁保存在内存里，所以重启就能清除它：
   ```sh
   docker compose restart evey
   ```
3. 如果某一个文件反复引起 panic（堆栈信息里会出现它的名字），把它移出媒体库路径，然后重新扫描。
4. 如果文件监视器记录了 `watcher: inotify failed for path, will use periodic scan`，说明内核的监视数量上限已经用完了：
   ```sh
   echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf
   sudo sysctl -p
   ```

## 速查表

```sh
# 所有东西的状态
docker compose ps
curl -sf http://localhost:9091/healthz && echo healthy

# 跟踪所有容器的日志
docker compose logs --tail=100 -f

# 实际生效的配置
docker exec evey-server env | grep ^EVEY_ | sort

# 数据库大小
docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
  SELECT pg_size_pretty(pg_database_size(current_database()));"'

# 主机磁盘
df -h ./evey-data
```

<!-- sources
ports: internal/config/config.go (EVEY_SERVER_PORT, EVEY_GRPC_BIND) ; cmd/evey/main.go - httpPort = Port+1 ; Dockerfile - HEALTHCHECK
logs: Dockerfile ENV EVEY_LOG_DIR ; internal/adapters/driving/http/admin_handler.go (GET /api/v1/admin/logs/search)
epoch fail-closed: internal/adapters/driving/http/auth_middleware.go, internal/adapters/driving/grpcapi/interceptors/auth.go ; internal/pkg/metrics/metrics.go - AuthFailuresTotal
rate limits: internal/config/config.go (EVEY_RATELIMIT_LOGIN_MAX, EVEY_RATELIMIT_GLOBAL_MAX, EVEY_TRUSTED_PROXIES) ; docker-compose.override.yml
cache dirs: Dockerfile ENV EVEY_TRANSCODING_CACHE_DIR / EVEY_TRICKPLAY_CACHE_DIR ; docker-compose.yml ; internal/config/config.go defaults
hardware mode and calibration labels: i18n/en/admin.yaml - transcodeCalibration.hwModeLabel, hwModeOff, hwModeAuto, cardTitle
software producer: internal/app/transcoding/software_producer.go - clientSegmentPatience, WaitBudgetDur, MaxSessionsPerHash, "software: reach decision", over_client_patience ; internal/app/transcoding/software_producer_lifecycle.go - "software: session killed" ; cmd/evey/wire.go - low-memory profile, transcode.ffmpeg_progress_log, logs/ffmpeg
server-config route: proto/evey/v1/server_config.proto - SetServerConfigEntry (PUT /api/v1/admin/server-config/{key})
stale rendition: internal/adapters/driving/http/transcode_handler_vod.go - stale_rendition, sealed_fp
db pool: internal/config/config.go (EVEY_DB_MAX_OPEN_CONNS)
search fallback: cmd/evey/wire_infra.go ; cmd/evey/admin.go - adminReindexSearch ; docker/entrypoint.sh (EVEY_EMBEDDED_MEILI, MEILI_DATA_DIR)
cache budget: internal/config/config.go (EVEY_TRANSCODING_CACHE_MAX_GB) ; internal/app/transcoding/segment_cache_evict.go - DefaultMinFreeBytes ; internal/adapters/driving/http/server.go - track-art under PhotoCacheDir ; cmd/evey/http.go - PhotoCacheDir
plugins: internal/adapters/driven/pluginrunner/runner_start.go (panic/crash lines) ; internal/app/plugin/runtime_service.go - StartPlugin ; proto/evey/v1/admin_plugin.proto - EnablePlugin
cast beacons: internal/adapters/driving/http/player_event_handler.go ; internal/app/user/auth.go - "refresh rejected" ; internal/adapters/driving/http/transcode_ondemand.go ; internal/app/transcoding/resolver_line.go
sessions: internal/adapters/driven/postgres/migrations/000001_init.up.sql - play_sessions ; internal/adapters/driving/grpcapi/admin_audit.go - KillSession (5 s) ; internal/app/user/user_service.go - DisableUser
scan: internal/app/library/scan_service.go - scanning map ; internal/app/library/watcher.go - inotify warning
labels: i18n/en/admin.yaml - sidebar (logs, nowPlaying, remoteAccess, serverConfig, transcodeCache, users, plugins), nowPlaying.stopSession, transcodeCache.clearAll, transcodeCache.maxConcurrent, remoteAccess.trustedProxies, serverSettings.transcodeCacheMaxGB ; i18n/en/common.yaml - enable, disable
-->
