---
source_sha: ffbf9fdc31bed5852bae361afeb788edd4c45c60a46c9b02b5cefe3e5c7d810a
---

# 故障处理手册

Evey 常见故障的第一响应手册。每一节的结构都一样：**症状 → 排查 → 处理**。

容器：`evey-server`、`evey-postgres`、`evey-meilisearch`、`evey-caddy`（生产环境），以及 `evey-web` + `evey-mailcatcher`（dev override）。默认安装**只有 `evey-server`**：Postgres 和 Meilisearch 都在它内部运行。把下文的 `docker exec evey-postgres psql -U ...` 换成 `docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey ...'`，它们的日志就是 `docker logs evey-server` 里以 `postgres:` 开头的行和 Meili 的行。

默认 HTTP 端口：`9091`（`EVEY_HTTP_PORT`）。健康检查端点：`/healthz`。

**日志。** 同一份日志有两个副本：按天生成的文件 `evey-data/logs/evey-YYYY-MM-DD.log`（`EVEY_LOG_DIR`，镜像内为 `/data/logs`），以及 `docker logs evey-server`。**以文件为准**：json-file 驱动会轮转 `docker logs`，轮转之后这条命令可能比容器提前几个小时就没有内容了。这时压测的 `stop_detail` 或某次校准结论看上去像是丢了，其实只是被轮转掉了。两者不一致就读文件，或者以 root 身份读原始的 `/var/lib/docker/containers/<id>/*-json.log*`。下文的 `docker logs` 单行命令都假设时间窗口较新且尚未轮转。

## 1. 服务器无法访问

**症状**：客户端连不上；`curl http://localhost:9091/healthz` 超时或返回 connection refused。

**排查**：
```sh
docker ps --filter name=evey-server
docker logs --tail=200 evey-server
docker inspect --format='{{.State.Health.Status}}' evey-server
ss -ltnp | grep 9091                # port actually listening?
curl -v http://localhost:9091/healthz
```

**常见原因与处理**：
- 容器已退出 → 用 `docker logs evey-server` 找到 panic，修好后执行 `docker compose up -d evey`。
- 健康检查失败是因为数据库或 Meili 挂了 → 见第 4 节和第 5 节。
- 宿主机端口冲突 → 改 `EVEY_HTTP_PORT`，或释放该端口。
- 前面的 Caddy 没有转发 → 查看 `docker logs evey-caddy`，确认 `EVEY_DOMAIN`，并确认 `evey` 的健康检查是绿的（Caddy 依赖它）。

## 2. 所有登录都返回 401

**症状**：每个用户都从 `/api/v1/auth/*` 收到 401，包括昨天还能登录的管理员。

**排查**：
```sh
docker logs --since 10m evey-server | grep -iE 'epoch|jwt|auth|signing|private key'
docker exec evey-server ls -l /data/jwt.key
```

**先找 epoch 那一行。** 每个已认证请求都会拿用户的 token epoch 与数据库再核对一次（ADR-0019），而这个核对在失败时是拒绝放行的。所以数据库变慢、被锁住或不可达时，服务器的每个请求都会返回与过期令牌一模一样的 401，读起来完全像是 JWT 出了问题，其实不是。服务器把话说得很明白：

```
auth rejected: the token epoch could not be read, so every authenticated
request is failing - check the database, not the JWT key
```

看到这一行就别再读本节，直接去 §1（数据库）。此时 `/healthz` 仍可能是绿的：它只 ping 连接，不执行 epoch 查询。对应的 Prometheus 计数器是 `evey_auth_failures_total{reason="epoch_lookup_failed"}`。

**常见原因与处理**：
- **JWT 密钥丢失或被更换**：`EVEY_JWT_PRIVATE_KEY_PATH`（默认 `/data/jwt.key`）读不到，或者已被轮换。用旧密钥签发的令牌全部作废，除了从备份恢复旧密钥、或让所有用户重新登录之外没有别的办法。请确认 `evey-data` 卷正确挂载在 `/data`。
- **限流把每个请求都拦下了**：检查 `EVEY_RATELIMIT_LOGIN_MAX`（默认每个 IP 每分钟 `10`）和 `EVEY_RATELIMIT_GLOBAL_MAX`（默认 `300`）。如果反向代理把所有流量都当成同一个 IP 转发过来，很快就会触发限流，此时设置 `EVEY_TRUSTED_PROXIES` 并重启。dev override 已经把它们设为 `1000` / `10000`。
- 服务器与客户端时钟不同步 → 在宿主机上执行 `date -u`。

## 3. 转码失败 / 需要转码的视频放不了

**症状**：直接播放正常，一旦需要转码就失败。播放器报错，服务器日志里出现 ffmpeg 的退出码。

**排查**：
```sh
docker exec evey-server which ffmpeg ffprobe
docker exec evey-server ffmpeg -version | head -1
docker logs --since 10m evey-server | grep -iE 'ffmpeg|transcode|hls'
docker exec evey-server ls -ld /cache/transcode /data/trickplay
docker exec evey-server df -h /cache
```

**常见原因与处理**：
- **缺少 ffmpeg 可执行文件**（自定义镜像里没打进去）：用官方 `Dockerfile` 重新构建。
- **缓存目录不可写**：代码里 `EVEY_TRANSCODING_CACHE_DIR` 的默认值是 `/data/transcode`，但 compose 通过名为 `evey-cache` 的卷把它映射到 `/cache/transcode`。`EVEY_TRICKPLAY_CACHE_DIR` 默认是 `/cache/trickplay`。验证可写性：
  ```sh
  docker exec evey-server touch /cache/transcode/.write-test && \
    docker exec evey-server rm /cache/transcode/.write-test
  ```
- **缓存满了** → 见第 6 节。
- **硬件加速配置有误**：设 `EVEY_TRANSCODING_HW_ACCEL_ENABLED=false` 强制走软件，重启，确认问题确实与硬件有关之后再重新启用。
- **并发转码过多**：`EVEY_TRANSCODING_MAX_CONCURRENT`（默认 `2`）。机器核心够的话可以谨慎调高。

### 3c. “一直在缓冲 / 中途断了”，而日志没说原因

**症状**：观众反映某个转码标题会卡住或断流，服务器日志里出现 `hls-vod: segment timeout` 并带着 `err:"context canceled"`，这表示是**客户端**挂断了，而不是服务器超时。

**排查**：服务器已经记录了它做过的每一个决定。先从这里开始，因为它不花任何代价：

```sh
# Why a segment request reused an encoder or spawned one, and what it cost the
# player. Only logged when it costs something: a wait beyond one segment, or a
# spawn. `over_client_patience: true` is the smoking gun.
docker exec evey-server grep '"software: reach decision"' /data/logs/evey-$(date +%F).log | tail -40

# The encoders themselves: spawns, kills and their reason.
docker exec evey-server grep -E '"software: session (started|killed)"|per-hash cap' \
  /data/logs/evey-$(date +%F).log | tail -40
```

这些行说明服务器*决定*了什么。当你需要知道 ffmpeg 逐秒*实际做到了*什么，也就是前沿位置随时间的真实变化时，再打开逐编码器的进度记录：

```sh
# ON. Applies to the NEXT encoder; no restart needed. $TOKEN is an admin JWT.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"true"}'

# Reproduce the problem, then read one session: out_time = content produced so
# far, speed = how far ahead of real time the encoder ran.
docker exec evey-server ls -t /data/logs/ffmpeg | head
docker exec evey-server tail -20 /data/logs/ffmpeg/<file>.log

# OFF again when you are done.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"false"}'
```

**排查之外请保持关闭。** 它为每个编码器生成一个文件，每次会话几 MB，而且这台机器上的每一次播放都会写，在性能较弱的 NAS 上这就是实打实的磁盘占用和写入。文件名里带着决策哈希、档位（`-vN-`）和起始分片（`-sN-`），因此能和上面的 JSON 日志对上；在同一位置重新拉起的编码器会得到自己的文件，而不是覆盖上一次的运行，正是这一点让编码器的反复重启变得可见。

**常见原因与处理**：
- **反复地小幅向前拖动**：每一次都可能拉起一个新的编码器，而按哈希计的上限（`MaxSessionsPerHash`，默认 2）会淘汰最久未使用的那个。日志里出现 `session killed reason:lru_cap` 是意料之中的。次数不多时属于正常现象。
- **客户端比服务器更没耐心**：拖动可达窗口是按 `clientSegmentPatience`（6 s）设计的；AndroidX Media3 大约 8 s 就会放弃一个分片，而服务器的 `WaitBudgetDur` 是 60 s。带 `over_client_patience: true` 的 `reach decision` 行意味着服务器正打算要求一个没有播放器愿意等的等待时长。

### 3b. `stale_rendition` 的 410 / 播放器在播放途中重新拉取 master

**症状**：播放器短暂卡顿后自行恢复；日志中出现 `hls-vod: stale rendition URL refused (the ladder moved under a live player)`，相关请求以 `410 Gone` 和 `X-Evey-Error: stale_rendition` 作答。

**这意味着什么**：这是*保护机制在生效*，而不是故障本身。每个 manifest URI 都带着它生成时所依据的决策哈希（`fp=`）。410 表示播放器提交了某个会话的 URL，而该会话的 ABR 档位阶梯此后已被重排，继续提供就等于把另一个 rendition 的字节交给解码器（也就是“彩虹屏”那一类事故，ADR 0022 §1b）。拒绝之后，播放器会干净地重新拉取 master。

**排查**：这条 WARN 带有 `sealed_fp`、`current_fp` 和媒体 id。
- **紧接在部署、重启或管理员重新校准之后的一波**属于预期：正在播放的会话比一次能力变更活得更久。无需处理；播放器会在一个轮询周期内重新同步。
- **正常运行期间持续零星出现**，说明有什么东西在不断重排正在播放的流的阶梯。可以查能力抖动（`docker logs evey-server | grep -iE 'calibrat|capability'`）、负载下反复变化的准入整形，或者来回振荡的 CMCD 上限覆盖。
- **一直在 410 上循环、始终恢复不了**的播放器，是抓着缓存的 master 不放（旧的应用构建，或对 `.m3u8` 缓存过于激进的代理）。请确认代理不缓存 manifest（`Cache-Control: no-cache` 必须能透传）。

## 4. 数据库拒绝连接 / 报错

**症状**：服务器日志里出现 `dial tcp ... connect: connection refused`、`pq: ...`，或者查询超时。

**排查**：
```sh
docker ps --filter name=evey-postgres
docker inspect --format='{{.State.Health.Status}}' evey-postgres
docker exec evey-postgres pg_isready -U "$EVEY_DB_USER"
docker logs --tail=200 evey-postgres
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
  SELECT count(*), state
    FROM pg_stat_activity
   WHERE datname = current_database()
   GROUP BY state;"
```

**常见原因与处理**：
- Postgres 容器已停止 → `docker compose up -d postgres`，并留意日志里是否有数据损坏或 oom。
- 凭据不对 → 服务器的环境变量必须与 `postgres` 服务使用的 `EVEY_DB_USER` / `EVEY_DB_PASSWORD` / `EVEY_DB_NAME` 一致。默认值：`evey` / `evey` / `evey`。
- 连接池耗尽（`too many connections` 或长时间等待）→ 检查 `EVEY_DB_MAX_OPEN_CONNS`（默认 `50`）。清掉卡住的会话：
  ```sql
  SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
   WHERE state = 'idle in transaction'
     AND now() - state_change > interval '5 minutes';
  ```
- `postgres-data/` 所在磁盘满了 → `df -h`，腾出空间，重启。

## 5. Meilisearch 挂了

**症状**：搜索结果质量下降，日志里出现 `meilisearch unavailable, using SQL search fallback`。

**行为**：Evey 有容错回退，搜索会改走 SQL 继续工作（`cmd/evey/wire.go` 接入了一个带回退的搜索适配器，在 Meili 不可达时记录 `meilisearch unavailable, using SQL search fallback`）。用户无需做任何事即可继续使用；只是排序质量会下降。

**关于部署方式的说明**：默认的 `docker-compose.yml` 把 Meilisearch **内嵌在 `evey-server` 容器里**运行（`EVEY_EMBEDDED_MEILI: "true"`）。独立的 `evey-meilisearch` 容器只存在于 dev override 中（`docker-compose.override.yml`）。在内嵌模式下，“Meilisearch 挂了”意味着 evey-server 进程本身不健康，并没有一个单独的容器可以重启。

**排查与处理（内嵌，默认）**：
```sh
docker ps --filter name=evey-server
curl -sf http://localhost:7700/health          # port forwarded from inside evey-server
docker logs --tail=200 evey-server | grep -i meili
```
- 服务器不健康 → `docker compose restart evey-server`。
- 索引损坏 → 停掉 evey-server，备份 `./evey-data/meili-data/`，清空，重启，然后在管理面板触发一次全量重建索引。

**排查与处理（仅限 dev override 的独立部署）**：
```sh
docker ps --filter name=evey-meilisearch
curl -sf http://localhost:7700/health
docker logs --tail=200 evey-meilisearch
```
- 容器停止 → `docker compose up -d meilisearch`。
- `MEILI_MASTER_KEY` 不一致 → 服务器日志会出现鉴权失败。把两个服务的环境变量改成一致并重启。
- 索引损坏 → 停掉 Meili，备份 `./meili-data/`，清空，重启，然后在管理面板触发一次全量重建索引。

## 6. 转码缓存磁盘满了

**症状**：转码以 `no space left on device` 失败；容器健康检查反复抖动。

**排查**：
```sh
docker exec evey-server df -h /cache
docker exec evey-server du -sh /cache/transcode /cache/trickplay /data/trickplay 2>/dev/null
docker volume inspect evey_evey-cache   # compose-mangled name may vary
```

**处理**：
- 安全清理，缓存本来就可以重建：
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/transcode/*'
  ```
  这同时会删掉它下面的 `photos/`（照片的缩放版本）和 `track-art/`（从音轨标签里提取一次并缩放的封面），两者都会在下次请求时重新生成。管理面板的“Clear cache”按钮做的是同一件事。
- Trickplay 重新生成的代价更高，非必要不要清：
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/trickplay/* /data/trickplay/*'
  ```
- 如果宿主机的卷空间紧张，就调低转码缓存预算（Admin → 服务器设置，默认 `10` GB）；无论预算多少，清理器都会在缓存卷上保留 2 GiB 空闲空间。

## 7. 扩展崩溃了

**症状**：某个扩展停止响应；服务器日志里出现来自 plugin runner 的 panic。

**排查**：
```sh
docker logs --since 30m evey-server 2>&1 | grep -iE 'plugin|pluginrunner|panic'
# Look for: "panic in plugin runner", "plugin process crashed", "remove plugin sock on crash"
```

**处理**：
- 先试精准重启，不需要停容器：`POST /api/v1/admin/plugins/<id>/enable`。即使扩展已经是 `enabled` 却卡在 `Error` 状态，这一招同样有效：只要状态不是 `Running`，`StartPlugin` 的守卫就会把它重新拉起。优先用这个，而不是重启整个服务器，尤其是在不方便 SSH 登录宿主机的时候。
- 精准重启解决不了，就在管理面板里停用出问题的那个扩展。
- 如果服务器本身不稳定，就重启：`docker compose restart evey`。
- 扩展文件在 `./evey-data/plugins/`（宿主机）== `/data/plugins`（容器）。删掉损坏的 zip 或目录，然后重启。
- 把 panic 的堆栈和 plugin id 发给扩展作者。

## 8. 投屏卡住、没声音，或者手机一直转圈

下面的一切都能从管理面板的 Logs 页面读到（或者用 `GET /api/v1/admin/logs/search?q=<term>&limit=200`），不需要登录机器。时间均为 UTC。每一条信标都是一行 `player-client: playback event reported`，带有 `type=` 和一个 `details=` 字符串。这些路由返回的内容里绝不会出现凭据（bearer、JWT 以及 `token=`/`password=` 的值都显示为 `[redacted]`），所以无法通过搜索日志找回投屏 URL 里的令牌 - 请搜索媒体 id，而不是令牌。

1. **接收端还活着吗，活到什么时候？** 搜索 `cast_receiver_alive`。播放期间接收端每 30 s 发一条；`details` 里带着 `buf=<behind>/<ahead>/<ranges>`（播放头之后和之前已缓冲的秒数）以及 `pos_sec`。播放健康时 `ahead` 在几十秒量级。**最后一条信标的时间，就是接收端死掉或被杀掉的时刻**：在那之后什么都没有，就说明电视已经不再运行接收端了，不管手机上显示什么。
2. **它为什么停了？** 搜索 `cast_receiver_finished`：`reason=INTERRUPTED` 表示有新的 LOAD 顶掉了正在运行的那个（往前几秒找 `StartStream`），`reason=ERROR` 则带着播放器的错误。最后一条 `alive` 之后完全没有 `finished`，说明接收端进程消失了，最常见的是 Chromecast HD 在 20+ Mbps 的 copy master 上因内存被杀。
3. **手机看到了什么？** 搜索 `cast_sender_stall`：当 Cast SDK 仍然报告 PLAYING 但位置 20 s 没有推进时，发送端会发 `state=playing frozen=<n>s pos=<p>`；接收端报告 idle 原因时则发 `state=idle reason=<r>`。这些行以 WARN 级别记录，所以 Logs 页面的警告过滤器能把它们筛出来。这是同一分钟里发送端的视角，把它的 `pos` 和接收端最后的 `pos_sec` 对比一下。
4. **会话是不是连锁失效了？** 搜索 `refresh rejected`：一个“not found”的 refresh token 会让应用登出再登录（用保存的凭据），从而重新签发流。这一行现在带着令牌的 `jti`、`issued` 和 `age`，据此可以分辨是过期的旧副本还是伪造的。搜 `StartStream` 可以看到重新签发；对于同一个标题，发送端不会再因为 URL 被重新签发而重新加载接收端。
5. **码流对这台设备是不是太大了？** 搜索 `advertises a fallback rung`（copy master 的峰值码率）和 `plan fitted to the client's line`（接收端的 `maxKbps` 测量或其 `abr=floor` 救援，现在是 plan 的输入 - 随后的转码 master 带有原因 `client_line_exceeded` 或 `abr_floor_requested`）：一个从 20+ Mbps 的 copy master 起播、几秒后掉到受限 1080p 档位的会话，说明兜底救援起作用了；而重新 LOAD 后又从 copy master 起播，才是能弄死 Chromecast HD 的模式。
6. **服务端资源不足？** 搜索 `on-demand resource refused`：连续分片上成片出现的 `context canceled` 拒绝是拖动造成的（每个请求都被下一个取消），不是服务器的问题；真正有问题的是耗时很长的 `served` 行。

如果第 1 至 3 步显示接收端还活着而发送端沉默，问题就在手机的 Cast 会话里（SDK，或到电视的网络）；如果先停的是接收端的信标，那要查的就是电视（内存、编解码器、码流的码率）。

## 终止被入侵的会话或设备

症状：管理员在 **Now Playing**（管理面板）里看到一台意料之外的设备，或者用户反映自己的会话列表里有不认识的设备。

1. 找出 `session_id`：
   ```sh
   docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
     SELECT id, user_id, profile_id, device, ip_address, started_at
     FROM play_sessions
     WHERE ended_at IS NULL
     ORDER BY started_at DESC LIMIT 20;"
   ```
2. 从管理界面终止：**Admin → Activity → Now Playing → ⋯ → Kill session**，或者带上 `session_id` 调用 gRPC 的 `AdminSessionService/KillSession`。会话会在 5s 后结束，好让播放器收到停止指令。
3. 如果会话没有响应，就强制删掉 HLS 会话（目录是 `<cacheDir>/<mediaItemID>`，默认 `/cache/transcode/<mediaItemID>`）：
   ```sh
   docker exec evey-server rm -rf /cache/transcode/<media_item_id>/
   ```
4. 如果连 refresh token 也需要吊销（怀疑密码泄露）：
   ```sh
   docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
     DELETE FROM refresh_tokens WHERE user_id = '<user_id>';"
   ```
   该用户的所有设备都需要重新登录。要彻底封锁，就用 `evey admin reset-password --user <username>` 重置密码。

## 解开卡住的媒体库扫描

症状：某次媒体库扫描显示“in progress”好几个小时，但日志里看不到任何文件进度。

1. 间接查看 `scan_service.scanning` 这张表：进程卡住时 CPU 是空闲的，最近的日志里也不会出现 `scan_service`：
   ```sh
   docker compose logs --tail=500 evey-server | grep scan_service
   ```
2. 扫描锁是**放在内存里的**（没有数据库记录），所以重启服务器就会清掉它：
   ```sh
   docker compose restart evey-server
   ```
3. 如果是某个文件反复引发 panic（看堆栈），把它移出媒体库路径隔离起来，然后重新扫描。
4. 如果 inotify 的监视数用尽了（内核限制），就调高：
   ```sh
   echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf
   sudo sysctl -p
   ```

## 通用排查速查表

```sh
# Status of everything
docker compose ps
curl -sf http://localhost:9091/healthz && echo healthy

# Tail every container
docker compose logs --tail=100 -f

# Server config dump
docker exec evey-server env | grep ^EVEY_ | sort

# Postgres size / locks
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
  SELECT pg_size_pretty(pg_database_size(current_database()));"

# Host disk
df -h . ./postgres-data ./meili-data ./evey-data
```
