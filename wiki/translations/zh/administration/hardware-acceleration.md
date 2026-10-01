---
source_sha: c5dbea370fd3437769623f35f8bb5f32be76bca382bacfd248fac739d7bb12e9
reviewed: false
---
# 硬件加速

一张显卡或集成 GPU 能让服务器比只用处理器同时转码更多的流，并且分辨率更高。所有情况都用同一个镜像：它不包含任何特定显卡的驱动，同一个容器可以只用软件运行，也可以配合 Intel 或 AMD GPU，或 NVIDIA 显卡运行，取决于你给它哪些设备。

两步：把设备暴露给容器，并在 **Admin → 转码 → 编码校准** 中把 **硬件加速** 保持为 **自动（最佳可用）**。

## 暴露设备

=== "Intel / AMD (VAAPI, Quick Sync)"

    ```yaml title="docker-compose.yml"
    services:
      evey:
        devices:
          - /dev/dri:/dev/dri
        group_add:
          - "104"   # 主机的 render 组：getent group render
    ```

    这个组必须是主机的 `render` 组（`getent group render`，或 `stat -c %g /dev/dri/renderD128`）。值错了不会造成破坏：只是 GPU 无法访问，服务器继续使用处理器。

=== "NVIDIA (NVENC)"

    在主机上安装 NVIDIA Container Toolkit，预留 GPU，并请求 `video` 能力，没有它编码器库就不会被注入：

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

    传入 `/dev/mpp_service`、`/dev/dri`，以及内核提供时的 `/dev/rga`。见[ARM 主机](../operations/advanced-deployments.md#arm-hosts)。

NVIDIA 预留和 `/dev/dri` 可以同时传入。

## 自动模式做了什么

在 **自动** 模式下，服务器只依据测量工作，从不依据显卡的型号名称：

1. **检测。** 每个渲染节点都用 VAAPI 测试，在 Intel 上还用 Quick Sync 测试；每个 NVIDIA GPU 和 Rockchip 编码器也一样。编码器只有在一次真实编码成功后才算数。
2. **认证。** 每个加速器都以与真实播放相同的流水线形式，对一系列分辨率（从 720p 到 2160p）进行编码，保留它能以 1.2 倍实时速度维持的最高档，并通过一次画面检查。连 720p 都维持不了的加速器不会被使用。
3. **测量路线。** 对加速器证明过的每种能力（SDR、HEVC、AV1、GPU 上的色调映射、保留 HDR），测量一次真实会话的开销。
4. **按流选择。** 每次播放都交给已证明具备该流所需能力的最便宜的加速器，或者交给处理器。见[转码与服务器容量](../playback/transcoding.md)。

测量在服务器第一个空闲时刻运行，并在硬件变化时再次运行。**重新校准** 会立即运行它：在添加显卡或把容器迁移到另一台机器后使用。

**关闭（仅软件）** 让所有转码都留在处理器上，这是驱动有问题时的紧急出口。手动选择一个 **后端**（NVENC、VAAPI、QSV、RKMPP）会强制使用它；如果它不可用，服务器会退回处理器，卡片上会显示实际使用的后端。

在有多个加速器的机器上，自动模式没有固定偏好：每个加速器都会被认证和测量，每个流都交给已证明具备所需路线的后端，取已证明的最佳质量和测得的最低开销。因此同一台服务器上的两个流可以使用不同的后端，例如在一张卡上做 HDR 色调映射，在另一张卡上做 HEVC 编码。

## 确认它在被使用

**Admin → 转码 → 编码校准** 显示检测到的加速器、正在使用的后端、为 GPU 和处理器认证的分辨率，以及任何退回处理器的情况。播放期间，**Admin → 正在播放** 会显示每个流使用哪个后端编码。

在 WSL2 上，NVENC 可以工作；VAAPI 和 Quick Sync 不行（WSL 的图形驱动在这些编码上会失败），管理面板会把它们显示为灰色。
