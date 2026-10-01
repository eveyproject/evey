---
source_sha: 3a41d86519a03aab692820f316d27cf9feb1e8da69e124f838de91eea3408a9b
reviewed: false
---
# 事故手册

针对 Evey 常见故障的第一响应手册。每一节：**症状 → 诊断 → 修复**。

命令针对标准安装：一个容器 `evey-server`，Postgres 和 Meilisearch 在其中运行；它们的日志是 `docker logs evey-server` 中的 `postgres:` 行和 Meilisearch 行。仓库的 `docker-compose.yml` 布局会额外加上 `evey-postgres` 和 `evey-caddy` 容器（[高级部署](../operations/advanced-deployments.md)）。

要对内置数据库执行 SQL：

```sh
docker exec evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

默认 HTTP 端口：`9091`（`EVEY_HTTP_PORT`）。健康检查端点：`/healthz`。

**日志。** 同一个流有两份副本：每日文件 `evey-data/logs/evey-YYYY-MM-DD.log`（`EVEY_LOG_DIR`，镜像内为 `/data/logs`）和 `docker logs evey-server`。文件是权威来源：json-file 驱动会轮转 `docker logs`，轮转之后该命令的输出可能比容器停止的时间早好几个小时结束；负载测试的 `stop_detail` 或校准结论看起来像是缺失了，其实只是被轮转掉了。两者不一致时，读文件，或者以 root 身份读原始的 `/var/lib/docker/containers/<id>/*-json.log*` 文件。下面的 `docker logs` 命令假定日志窗口是最近的、未被轮转的。

## 1. 服务器无法访问

**症状**：客户端无法连接；`curl http://localhost:9091/healthz` 超时或返回 connection refused。

**诊断**：

```sh
docker ps --filter name=evey-server
docker logs --tail=200 evey-server
docker inspect --format='{{.State.Health.Status}}' evey-server
ss -ltnp | grep 9091                # 端口真的在监听吗？
curl -v http://localhost:9091/healthz
```

**常见原因 + 修复**：
- 容器已退出 → 用 `docker logs evey-server` 查看 panic，修复，`docker compose up -d evey`。
- 健康检查失败是因为数据库或 Meili 宕机 → 见第 4 和第 5 节。
- 主机端口冲突 → 修改 `EVEY_HTTP_PORT` 或释放该端口。
- 前面的 Caddy 没有转发 → `docker logs evey-caddy`，检查 `EVEY_DOMAIN`，以及 `evey` 的健康检查是否为绿色（Caddy 依赖它）。

## 2. 所有登录都返回 401

**症状**：所有用户（包括昨天还登录过的管理员）从 `/api/v1/auth/*` 得到 401。

**诊断**：

```sh
docker logs --since 10m evey-server | grep -iE 'epoch|jwt|auth|signing|private key'
docker exec evey-server ls -l /data/jwt.key
```

**先找 epoch 那一行。** 每个经过身份验证的请求都会对照数据库重新检查用户令牌的 epoch（ADR-0019），而这项检查是失败即拒绝的。因此，一个缓慢、被锁或无法访问的数据库，会让服务器上的每个请求都返回与令牌过期相同的 401，看起来完全像是 JWT 问题，但其实不是。服务器会明确说明这一点：

```
auth rejected: the token epoch could not be read, so every authenticated
request is failing - check the database, not the JWT key
```

如果出现了这一行，就不用再读本节，直接去第 4 节（数据库）。此时 `/healthz` 可能仍是绿色：它只 ping 连接，不执行 epoch 查询。对应的 Prometheus 计数器是 `evey_auth_failures_total{reason="epoch_lookup_failed"}`。

**常见原因 + 修复**：
- **JWT 密钥丢失或被更改**：`EVEY_JWT_PRIVATE_KEY_PATH`（默认 `/data/jwt.key`）不可读，或者已被轮换。用旧密钥签名的所有令牌现在都无效了：除了从备份恢复旧密钥或强制所有用户重新登录之外，没有别的补救办法。确认 `evey-data` 卷正确挂载在 `/data`。
- **每个请求都触发速率限制**：检查 `EVEY_RATELIMIT_LOGIN_MAX`（默认每个 IP 每分钟 `10` 次）和 `EVEY_RATELIMIT_GLOBAL_MAX`（默认 `300`）。把所有流量都以同一个 IP 转发的反向代理会很快触发它：在 Admin → 远程访问 中把该代理加入 **受信任的代理**（无需重启即可生效；`EVEY_TRUSTED_PROXIES` 也可以）。开发环境的覆盖配置已经设置为 `1000` / `10000`。
- 服务器与客户端之间的时钟偏差 → 在主机上运行 `date -u`。

## 3. 转码失败 / 视频无法以转码方式播放

**症状**：直接播放正常，需要转码时失败。播放器报错，服务器日志显示 ffmpeg 退出码。

**诊断**：

```sh
docker exec evey-server which ffmpeg ffprobe
docker exec evey-server ffmpeg -version | head -1
docker logs --since 10m evey-server | grep -iE 'ffmpeg|transcode|hls'
docker exec evey-server ls -ld /cache/transcode /data/trickplay
docker exec evey-server df -h /cache
```

**常见原因 + 修复**：
- **缺少 ffmpeg 二进制文件**（不带它的自定义镜像）：用官方 `Dockerfile` 重新构建。
- **缓存目录不可写**：`EVEY_TRANSCODING_CACHE_DIR` 在代码中默认为 `/data/transcode`，但 compose 通过名为 `evey-cache` 的卷把它映射到 `/cache/transcode`。`EVEY_TRICKPLAY_CACHE_DIR` 在镜像中为 `/data/trickplay`（在镜像之外默认为 `/cache/trickplay`）。检查是否可写：
  ```sh
  docker exec evey-server touch /cache/transcode/.write-test && \
    docker exec evey-server rm /cache/transcode/.write-test
  ```
- **缓存已满** → 见第 6 节。
- **硬件加速出问题**：在 **Admin → 转码 → 编码校准** 中把 **硬件加速** 设置为 **关闭（仅软件）**（实时生效并重新测量），让所有转码都留在处理器上，并在切回 **自动** 之前确认问题确实与硬件有关。
- **"服务器正忙于转码其他流"**：实时容量是测量出来的，而不是设置的；见[转码与服务器容量](../playback/transcoding.md#sharing-the-capacity)。`EVEY_TRANSCODING_MAX_CONCURRENT`（**Admin → 转码缓存** 中的 **Max Concurrent**）只涵盖实时播放之外的转码任务。

### 3c. "一直缓冲 / 播放中断"而日志没有说明原因

**症状**：一位观众报告某个转码条目出现卡顿或中断，服务器日志显示 `hls-vod: segment timeout` 带 `err:"context canceled"`，这表示是客户端断开了，而不是服务器超时。

**诊断**：服务器已经记录了它做出的每个决定。从这里开始，因为不需要任何代价：

```sh
# 一个分段请求为什么重用或启动了编码器，以及它让播放器付出了什么代价。
# 只在有代价时记录：超过一个分段的等待，或者一次启动。
# `over_client_patience: true` 就是确凿的证据。
docker exec evey-server grep '"software: reach decision"' /data/logs/evey-$(date +%F).log | tail -40

# 编码器本身：启动、终止及其原因。
docker exec evey-server grep -E '"software: session (started|killed)"|per-hash cap' \
  /data/logs/evey-$(date +%F).log | tail -40
```

这些行说明服务器*决定*了什么。当你需要知道 ffmpeg 每一秒实际*做到*了什么（编码前沿随时间的真实位置）时，打开每个编码器的进度记录：

```sh
# 开启。对下一个编码器生效；无需重启。$TOKEN 是管理员 JWT。
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"true"}'

# 重现问题，然后读取一个会话：out_time = 到目前为止生成的内容，
# speed = 编码器领先实时多少。
docker exec evey-server ls -t /data/logs/ffmpeg | head
docker exec evey-server tail -20 /data/logs/ffmpeg/<file>.log

# 完成后再关闭。
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"false"}'
```

**调查之外请保持关闭。** 它为每个编码器写一个文件，每个会话几 MB，为机器上的每次播放写入：在性能较弱的 NAS 上，这是实实在在的磁盘占用和写入。文件名包含决定哈希、档位（`-vN-`）和起始分段（`-sN-`），所以能与上面的 JSON 日志对应；在同一位置重新启动会得到自己的文件，而不是覆盖上一次的运行，这正是让编码器反复重启变得可见的原因。

**常见原因 + 修复**：
- **反复的向前短距离跳转**：每一次都可能启动一个新的编码器，而每个哈希的上限（`MaxSessionsPerHash`，默认 2）会淘汰最久未使用的那个。日志中会出现 `session killed reason:lru_cap`。适度出现是正常的。
- **客户端比服务器更没耐心**：`clientSegmentPatience`（6 秒）是跳转可达窗口的依据；AndroidX Media3 在约 8 秒时放弃一个分段，而服务器的 `WaitBudgetDur` 是 60 秒。带有 `over_client_patience: true` 的 `reach decision` 行表示服务器正准备要求一段没有播放器会等下去的等待。

### 3b. `stale_rendition` 410 / 播放器在播放中途重新获取 master

**症状**：播放器短暂卡顿后恢复；日志显示 `hls-vod: stale rendition URL refused (the ladder moved under a live player)`，请求以 `410 Gone` 和 `X-Evey-Error: stale_rendition` 响应。

**含义**：这是*一项保护措施在起作用*，而不是故障本身。每个清单 URI 都带有构建它时的决定哈希（`fp=`）。410 表示播放器提交了一个来自某会话的 URL，而该会话的 ABR 阶梯此后已被重新调整：提供它会把另一个 rendition 的字节交给解码器（"彩虹屏"一类事故，ADR 0022 §1b）。拒绝会迫使播放器干净地重新获取 master。

**诊断**：WARN 中带有 `sealed_fp` 与 `current_fp` 以及媒体 id。
- **在部署/重启或管理员重新校准之后立即出现的一阵** 是预料之中的：实时会话经历了一次能力变化。无需处理；播放器会在一次轮询内重新同步。
- **正常运行期间持续不断地出现** 表示有东西在实时流之下不断重新调整阶梯：查找能力是否在反复变化（`docker logs evey-server | grep -iE 'calibrat|capability'`）、负载下反复调整的准入整形器，或者来回变化的 CMCD 上限覆盖。
- **在 410 上循环而无法恢复** 的播放器持有一个缓存的 master（旧版应用、对 `.m3u8` 进行激进缓存的代理）。确认代理不缓存清单（`Cache-Control: no-cache` 必须能传递过去）。

## 4. 数据库连接被拒绝 / 出错

**症状**：服务器日志包含 `dial tcp ... connect: connection refused`、`pq: ...`，或者查询超时。

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

**常见原因 + 修复**：
- `./evey-data` 下**磁盘已满**（数据库位于 `./evey-data/postgres`）→ 释放空间，然后 `docker compose restart evey`。
- **连接池耗尽**（`too many connections` 或长时间等待）→ `EVEY_DB_MAX_OPEN_CONNS`（默认 `25`）。终止卡住的会话：
  ```sql
  SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
   WHERE state = 'idle in transaction'
     AND now() - state_change > interval '5 minutes';
  ```
- **独立的 Postgres 容器**（仓库布局）：`docker ps --filter name=evey-postgres`、`docker logs --tail=200 evey-postgres`；服务器的 `EVEY_DB_USER` / `EVEY_DB_PASSWORD` / `EVEY_DB_NAME` 必须与 `postgres` 服务一致。

## 5. Meilisearch 宕机

**症状**：搜索返回降级的结果，日志显示 `meilisearch unavailable, using SQL search fallback`。

**行为**：搜索通过 SQL 继续工作（`cmd/evey/wire.go` 接入了一个有韧性的搜索适配器，在 Meilisearch 无法访问时记录这一行）。保持在线不需要用户做任何事；只是排序质量会下降。

Meilisearch 运行在 `evey-server` 内（`EVEY_EMBEDDED_MEILI`，默认开启），只在容器内监听 `127.0.0.1:7700`。

**诊断 + 修复**：
```sh
docker exec evey-server curl -sf http://127.0.0.1:7700/health
docker logs --tail=300 evey-server | grep -i meili
```
- 不健康 → `docker compose restart evey`。
- 索引损坏 → 停止容器，把 `./evey-data/meili` 移到别处，再次启动，然后从数据库重建索引：`docker exec evey-server evey admin reindex-search`。

## 6. 转码缓存磁盘已满

**症状**：转码因 `no space left on device` 失败；容器健康检查时好时坏。

**诊断**：

```sh
docker exec evey-server df -h /cache
docker exec evey-server du -sh /cache/transcode /data/trickplay 2>/dev/null
docker volume inspect evey_evey-cache   # compose 生成的名称可能不同
```

**修复**：
- 安全清理，缓存可以重新生成：
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/transcode/*'
  ```
  这也会删除其下的 `photos/`（缩放后的照片版本）和 `track-art/`（从曲目标签中提取一次并缩放的内嵌封面）：两者都会在下一次请求时重建。Admin → 转码缓存 中的 **Clear All** 会执行同样的清理。
- Trickplay 重新生成的代价更高；仅在必要时清理：
  ```sh
  docker exec evey-server sh -c 'rm -rf /data/trickplay/*'
  ```
- 如果主机卷空间紧张，降低 **转码缓存预算**（**Admin → 服务器配置 → 服务器设置**，默认 `10` GB）；无论预算多少，清理程序都会在缓存卷上保留 2 GiB 的可用空间。

## 7. 插件崩溃

**症状**：某个插件停止响应；服务器日志显示来自插件运行器的 panic。

**诊断**：

```sh
docker logs --since 30m evey-server 2>&1 | grep -iE 'plugin|pluginrunner|panic'
# 查找： "panic in plugin runner", "plugin process crashed", "remove plugin sock on crash"
```

**修复**：
- 先尝试一次精确的重启，容器无需停机：`POST /api/v1/admin/plugins/<id>/enable`。即使插件已经是 `enabled` 但卡在 `Error` 状态，这也能工作：只要状态不是 `Running`，`StartPlugin` 的保护逻辑就会重新启动它。优先使用这种方式而不是重启整个服务器，尤其是在不方便 SSH 到主机时。
- 如果精确重启无法解决，就从管理面板禁用出问题的插件。
- 如果服务器本身不稳定，重启：`docker compose restart evey`。
- 检查位于 `./evey-data/plugins/`（主机）== `/data/plugins`（容器）的插件文件。删除损坏的插件 zip 或目录，重启。
- 把 panic 堆栈和插件 id 发送给插件作者。

## 8. 投放卡住、没有声音，或者手机一直在转圈

下面的所有内容都可以从管理日志页面读取（或者 `GET /api/v1/admin/logs/search?q=<term>&limit=200`）：不需要在机器上打开 shell。时间为 UTC。每个信标都是一条带有 `type=` 和 `details=` 字符串的 `player-client: playback event
reported` 行。凭据从不出现在这些路由返回的内容中（bearer、JWT 以及 `token=`/`password=` 的值显示为 `[redacted]`），所以投放 URL 中的令牌不是日志搜索能找回的：请搜索媒体 id，而不是令牌。

1. **接收器是否还活着，活到什么时候？** 搜索 `cast_receiver_alive`。接收器在播放时每 30 秒发送一次；`details` 带有 `buf=<behind>/<ahead>/<ranges>`（播放头之后和之前缓冲的秒数）和 `pos_sec`。健康的播放显示 `ahead` 为几十秒。**最后一个信标的时间就是接收器死掉或被杀掉的时间**：此后什么都没有，就表示电视已经不再运行接收器，无论手机显示什么。
2. **为什么停止了？** 搜索 `cast_receiver_finished`：`reason=INTERRUPTED` 表示一个新的 LOAD 替换了正在运行的那个（查找几秒之前的 `StartStream`），`reason=ERROR` 带有播放器错误。最后一个 `alive` 之后完全没有 `finished` = 接收器进程消失了（最常见的是 Chromecast HD 在 20+ Mbps 直拷 master 上因内存不足被杀掉）。
3. **手机看到了什么？** 搜索 `cast_sender_stall`：当 Cast SDK 仍然显示 PLAYING 但位置 20 秒没有变化时，发送端会发送 `state=playing frozen=<n>s pos=<p>`；当接收器报告了空闲原因时，发送 `state=idle reason=<r>`。这些行以 WARN 级别记录，所以管理日志页面的警告筛选器会显示它们。这是同一分钟内发送端这一侧的情况：把它的 `pos` 与接收器最后的 `pos_sec` 进行比较。
4. **会话是否发生了连锁反应？** 搜索 `refresh rejected`：刷新令牌 "not found" 会让应用登出再登录（保存的凭据），从而重新签发流；这一行现在会给出令牌的 `jti`、`issued` 和 `age`，可以区分过期的副本和伪造的令牌。搜索 `StartStream` 查看重新签发；对于同一个条目重新签发的 URL，发送端不再重新加载接收器。
5. **流对设备来说是否太大？** 搜索 `advertises a fallback
   rung`（直拷 master 的峰值码率）和 `plan fitted to the client's
   line`（接收器的 `maxKbps` 测量值或其 `abr=floor` 救援，现在是计划的输入：随后的转码 master 带有原因 `client_line_exceeded` 或 `abr_floor_requested`）：一个从 20+ Mbps 直拷 master 开始、几秒后降到适配的 1080p 档位的会话，是保底救援在正常工作；而重新 LOAD 又从直拷 master 开始，正是会杀掉 Chromecast HD 的模式。
6. **服务器端资源不足？** 搜索 `on-demand resource refused`：连续分段上的一阵 `context canceled` 拒绝是快速拖动（每个请求都被下一个取消），而不是服务器故障；持续时间很长的 `served` 行才是。

如果第 1 至 3 步显示接收器还活着而发送端没有动静，问题在手机的 Cast 会话（SDK、到电视的网络）；如果接收器的信标先停止，就该检查电视（内存、编码、流的码率）。

## 终止被盗用的会话或设备

症状：管理员在 **正在播放**（管理面板）中看到一台意外的设备，或者用户报告其会话列表中有一台未知设备。

1. 找出 `session_id`：
   ```sh
   docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
     SELECT id, profile_id, device_name, client_ip, started_at
     FROM play_sessions
     WHERE ended_at IS NULL
     ORDER BY started_at DESC LIMIT 20;"'
   ```
2. 从管理界面终止：**Admin → 正在播放**，点击该会话的停止图标（**Stop session**），然后点击 **停止** 确认；或者通过 gRPC `AdminSessionService/KillSession` 传入 `session_id`。会话会在 5 秒后终止，以便播放器能收到停止指令。
3. 如果流仍在运行，重启容器：所有编码器都会随之停止。
   ```sh
   docker compose restart evey
   ```
4. 如果账户本身被盗用，对它执行 **Admin → 用户 → 禁用**：这会删除它的刷新令牌并立即使它的访问令牌失效，因此所有设备都会被登出。设置一个新密码（`evey admin reset-password --user <email>`，它本身不会结束任何会话），然后再 **启用** 该账户。

## 解除卡住的媒体库扫描

症状：一次媒体库扫描显示"进行中"好几个小时，但日志中没有文件进度。

1. 检查扫描是否还在记录任何内容：如果进程挂起，CPU 处于空闲，最近的日志中没有 `scan:` 行：
   ```sh
   docker logs --tail=500 evey-server | grep 'scan:'
   ```
2. 扫描锁是**在内存中**的（数据库中没有对应行），所以重启服务器就能清除它：
   ```sh
   docker compose restart evey
   ```
3. 如果某个特定文件反复导致 panic（见堆栈跟踪），把它移出媒体库路径进行隔离，然后重新扫描。
4. 如果 inotify 监视数耗尽（内核限制），把它调高：
   ```sh
   echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf
   sudo sysctl -p
   ```

## 通用诊断速查表

```sh
# 所有服务的状态
docker compose ps
curl -sf http://localhost:9091/healthz && echo healthy

# 跟踪所有容器的日志
docker compose logs --tail=100 -f

# 导出服务器配置
docker exec evey-server env | grep ^EVEY_ | sort

# 数据库大小
docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
  SELECT pg_size_pretty(pg_database_size(current_database()));"'

# 主机磁盘
df -h ./evey-data
```
