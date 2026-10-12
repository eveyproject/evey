---
source_sha: 1a4e824d130bb84b068353b3024d71ea59b8db2ff04ee70f219e637e02212468
reviewed: true
---
# 硬件加速

**简单来说：**

- 有了显卡，或者很多处理器里内置的图形部分（例如 Intel Quick Sync），服务器能同时转换更多视频，而且能转换更高的分辨率，比只用处理器强。
- 只有视频需要转换（转码）时它才有用。原样发送或重新封装的电影用不到它。
- 您要做的：在 compose 文件里加几行，让 Evey 能使用显卡，然后把设置保持在**自动（最佳可用）**。
- Evey 在把任何播放交给显卡之前，会先用一次真实的转换来测试它。没通过测试的显卡不会被使用。

下面先讲操作步骤，再讲服务器如何证明显卡能做什么。

## 开启硬件加速

1. 打开**管理 → 转码 → 编码校准**。**硬件加速**默认设为**自动（最佳可用）**，**已检测**这一行列出主机报告的加速器。
2. 如果显示**未检测到 GPU 加速器**，请把设备交给容器（见下一节），然后重新创建容器。
3. 点击**重新校准**，或者等一等：服务器会在第一个空闲的时候自己测量。之后，这张卡片会显示正在使用的后端，以及 GPU 和处理器各自通过认证的分辨率。

所有情况都用同一个镜像：镜像里没有为某一款显卡内置驱动。同一个容器可以只用软件运行，也可以配合 Intel 或 AMD 的 GPU，或者配合 NVIDIA 显卡，取决于您交给它哪些设备。

## 把设备交给容器

=== "Intel / AMD (VAAPI, Quick Sync)"

    ```yaml title="docker-compose.yml"
    services:
      evey:
        devices:
          - /dev/dri:/dev/dri
        group_add:
          - "104"   # 主机的 render 组：getent group render
    ```

    这个组必须是主机的 `render` 组（`getent group render`，或 `stat -c %g /dev/dri/renderD128`）。填错也不会弄坏任何东西：只是 GPU 无法访问，服务器继续使用处理器。

=== "NVIDIA (NVENC)"

    在主机上安装 NVIDIA Container Toolkit，然后预留 GPU，并申请 `video` 能力；没有这项能力，编码器库就不会被注入：

    ```yaml title="docker-compose.yml"
    services:
      evey:
        environment:
          NVIDIA_DRIVER_CAPABILITIES: all   # 必须包含 "video"
        deploy:
          resources:
            reservations:
              devices:
                - driver: nvidia
                  count: all
                  capabilities: [gpu]
    ```

=== "Rockchip (RKMPP)"

    映射 `/dev/mpp_service`、`/dev/dri`，如果内核提供的话，再映射 `/dev/rga`。见 [ARM 主机](../operations/advanced-deployments.md#arm-hosts)。

NVIDIA 预留和 `/dev/dri` 可以同时映射。

## “用真实编码来证明”是什么意思

选择**自动**时，服务器只依据测量结果，从不依据显卡的名字：

1. **检测。** 对每个渲染节点探测 VAAPI，Intel 上还会探测 Quick Sync；每块 NVIDIA GPU 和 Rockchip 编码器也同样探测。只有通过它成功完成一次简短的真实编码，编码器才算数。
2. **认证。** 每个加速器按与真实播放相同的处理流程，依次编码一组分辨率（720p 到 2160p），并记下它能以 1.2 倍实时速度持续处理的最高一档。每次探测都会把几帧解码回来，与原片比较，因为有缺陷的驱动可能正常退出，却产生绿屏或花屏的画面。连 720p 都无法持续处理的加速器不会被使用。
3. **测量各条路线。** 对加速器证明过的每项能力（SDR、HEVC、AV1、在 GPU 上做色调映射、保留 HDR），测量一次真实播放的开销。
4. **为每个流单独选择。** 每次播放都交给已证明能满足该流需要、而且开销最低的加速器，或者交给处理器。见[转码与服务器容量](../playback/transcoding.md)。

测量会等到空闲的时候：没有正在播放的流，并且主机的 CPU 占用连续五次采样（每次间隔四秒）都低于 20%，这样繁忙的启动过程或媒体库扫描就不会把上限定得太低。硬件发生变化时，它会重新测量。**重新校准**会立即运行测量：添加显卡，或者把容器搬到另一台机器之后，请使用它。

!!! note "冷启动不算定论"
    如果 GPU 在容器启动后的头几秒里初始化失败，服务器会在短时间内多次重新探测，然后才记录结果。只有持续的失败，或者确定性的失败（缺少编码器、不支持的格式），才会把加速器标记为不可用。

## 强制指定后端，或者关闭硬件加速

**关闭（仅软件）**让所有转码都在处理器上进行，这是驱动有缺陷时的退路。手动选择一个**后端**（NVENC、VAAPI、QSV、RKMPP）会强制使用它；如果它不可用，服务器会退回到处理器，卡片上会显示实际使用的后端。

在有多个加速器的机器上，自动模式没有固定的偏好：每个加速器都经过认证和测量，每个流都交给已经证明能走它所需路线的后端，选择已证明的最佳质量和测得的最低开销。因此，同一台服务器上的两个流可以使用不同的后端，例如一块卡做 HDR 色调映射，另一块卡做 HEVC 编码。

## 检查它是否在使用

**管理 → 转码 → 编码校准**显示检测到的加速器、正在使用的后端、GPU 和处理器各自通过认证的分辨率；在**硬件回退**下，还会列出最近退回到处理器的编码及其原因。播放时，**管理 → 正在播放**会写明每个流用的是哪个后端。

在 WSL2 下，NVENC 可以使用；VAAPI 和 Quick Sync 不行（WSL 的图形驱动在这些编码上会失败），面板会把它们标记为 **WSL 不支持**。

<!-- sources
auto, certify, routes, recalibrate: docs/adr/0015-hardware-acceleration.md ; docs/adr/0029-transcoding-routes-are-measured-not-assumed.md
detection by real encode: internal/adapters/driven/ffmpeg/hwdetect.go - probeHWEncode
ladder, 1.2x realtime margin, 720p floor: internal/adapters/driven/ffmpeg/benchmark.go - hwBenchLadder, Margin
visual check: internal/adapters/driven/ffmpeg/hw_output_validation.go
cold-boot init retry: internal/adapters/driven/ffmpeg/hw_readiness.go - runHWReadinessProbe, hwReadinessRetryDelays
quiet moment: internal/app/transcoding/calibration_service.go - idleProbes, idleProbeEvery, idleCPUPercent
render group: docker-compose.gpu.yml ; nvidia: docker-compose.gpu-nvidia.yml
panel: frontend/apps/web/src/features/admin/EncodeCalibrationCard.tsx (hw_mode, detected_backends, is_wsl)
labels: i18n admin.calibration.cardTitle, hwModeLabel, hwModeAuto, hwModeOff, hwBackendLabel, detected, detectedNone, recalibrate, hwFallbacks, wslUnsupported ; admin.nav.transcodingConfig, nowPlaying
plain opening: rules below ; i18n admin.calibration.hwModeAuto
-->
