---
source_sha: aa2eb3901d0f475fc80f6f02196efd56ffd3beb911d1b5b63e4d270fd3d28c20
---

# 转码

客户端不支持源文件格式时，Evey 会实时转码。

## 工作原理

Evey 会自动挑出最好的播放方式：
1. **直接播放** - 格式本来就支持，不做任何处理
2. **重新封装** - 只换容器（MKV → MP4），不重新编码
3. **转码** - 重新编码成带自适应码率的 HLS

## 配置

```env
EVEY_TRANSCODING_MAX_CONCURRENT=2
# auto | off | nvenc | vaapi | qsv - 见下文的“硬件加速”一节
EVEY_TRANSCODING_HW_MODE=off
```

## 硬件加速

Evey 只发布**一个镜像**，其中的 ffmpeg 把 NVENC、VAAPI 和 QSV 全都编译了进去。
镜像里没有烘焙任何 GPU 驱动 - 驱动由你在运行时映射设备注入进来，不需要特权模式。
同一个镜像究竟是纯软件、NVENC 还是 VAAPI/QSV，完全取决于你映射了什么。
背后的取舍见 [ADR-0015](adr/0015-hardware-acceleration.md)。

### NVIDIA（NVENC）

先装好 [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html)，然后既要预留 GPU，**也要**申请 `video` 这项驱动能力（少了它，工具包只会注入 `nvidia-smi` 和 CUDA，不会注入 `libnvidia-encode.so`，于是 NVENC 加载失败）：

```yaml
services:
  evey:
    environment:
      NVIDIA_DRIVER_CAPABILITIES: all   # must include `video` for NVENC
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
```

### Intel / AMD（VAAPI、QSV）

把 DRM render 节点暴露出来即可；libva 会根据 GPU 的 PCI id 自动挑对驱动
（iHD / i965 / radeonsi）：

```yaml
services:
  evey:
    devices:
      - /dev/dri:/dev/dri
```

两者可以同时映射（NVIDIA 预留 + `/dev/dri`）- 反正就一个镜像，你之后选哪个后端它就用哪个。

### 如何启用

硬件加速在**管理 → 服务器配置 → 编码校准**里控制：

- 一个总的**硬件加速**开关。关 = 只用软件（GPU 或驱动不稳时的应急出口），开 = 使用 GPU。
- 开启之后会出现**后端**选择：`Auto`（由校准挑出检测到的最佳后端），
  或者强制指定 `NVENC` / `VAAPI` / `QSV`。强制指定的后端如果实际用不了，
  会回落到软件，卡片上则显示当前真正生效的后端。

这个选择会被保存下来，并当场重新跑一遍基准测试（不用重启）。无人值守的部署也可以从环境变量播下初始值：

```env
# 要么用老式的开关标志……
EVEY_TRANSCODING_HW_ACCEL_ENABLED=true
# ……要么用统一的模式（auto|off|nvenc|vaapi|qsv）；管理面板里的覆盖值优先。
EVEY_TRANSCODING_HW_MODE=auto
# 卡死的 GPU 超过这个时长就快速判负，直接转用软件（默认 5s）。
EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT=5s
# 在硬件路径上为每个 ABR 档位各起一个 ffmpeg（每档一个编码器），而不是用一个多路输出的
# 进程 - 这样单档出问题不会牵连其他档，也避开了会让脆弱的 VA 驱动崩溃的多路编码滤镜图。
# 默认关闭；软件路径永远走单进程滤镜图。见 docs/adr/0015。
EVEY_TRANSCODING_HW_PER_VARIANT=false
# 只编码客户端此刻真正请求的那一档 ABR（按需，每档一个编码器），而不是用一个 ffmpeg
# 产出整条阶梯。默认关闭（=0，即多档阶梯）- 对配置弱的 NAS 来说这才是正确的设置：
# 多档模式下所有档位共用对源文件的同一次解码。单档模式（=1）会为每一档各解码一遍源文件，
# 于是播放器做 ABR 探测或跨档位跳转时，会同时起好几路（比如 4K）解码互相抢资源 -
# 这正是解码受限的主机上那种经典的“掉到 360p 还一直缓冲”的来回抖动。只有主机强到
# 养得起按档解码时才设成 =1。两种取值都与缓存兼容。
EVEY_TRANSCODING_SINGLE_VARIANT=0
```

> **有五项设置如今在管理面板里**（管理 > 转码配置 > 高级上限）：`HW_PER_VARIANT`、
> `SINGLE_VARIANT`、`MAX_SESSIONS`、`SOFTWARE_MAX_HEIGHT` 和 `SOFTWARE_HDR_MAX_HEIGHT`。
> 在那里保存的值会持久化，并且优先于环境变量，所以改一个播放上限不必再去编辑 compose
> 文件、重建容器。这五项都在启动时读取，面板上也这么写着：保存只是存下你的选择，
> 重启才会让它生效。

### 编码校准用的样片

启动时（以及调用 `POST /api/v1/admin/transcode/calibration/recalibrate` 时），
基准测试会拿一段 4K 片段量出这台主机的实时编码上限，SDR 和 HDR 分开测
（`software_max_height` / `software_hdr_max_height`）。上限是从这段片段的
解码 + 色调映射 + 编码开销推出来的，所以**用真实内容的片段测出来，比用合成片段诚实得多** -
合成片段会高估开销，把上限压得很低（典型症状：一部机器其实能以 1080p 播放的 4K HDR 影片，
被限到了 480p 甚至 360p）。

基准测试按下面的优先级找它的输入：

1. **运维自备的文件** - 你自己放进去的真实片段。原样使用。
2. **固定的下载** - 一段版权干净的参考片段，只下载一次（校验 SHA256），随后缓存。
3. **合成片段** - 当场生成（`testsrc2` 加一点噪点，约 5 Mbit/s）。永远可用，是断网环境下的兜底。

```bash
# 让基准测试用一个真实文件（你自己的，或者下载来的参考片段）：
EVEY_BENCH_HDR_SAMPLE=/data/bench/my-4k-hdr.hevc
EVEY_BENCH_SDR_SAMPLE=/data/bench/my-4k-sdr.hevc
# ……或者用固定的下载（URL 必须是 https；设了 URL 就必须给出 SHA256）：
EVEY_BENCH_HDR_SAMPLE_URL=https://example.org/evey-bench-hdr10.hevc
EVEY_BENCH_HDR_SAMPLE_SHA256=<sha256>
EVEY_BENCH_SDR_SAMPLE_URL=https://example.org/evey-bench-sdr.hevc
EVEY_BENCH_SDR_SAMPLE_SHA256=<sha256>
```

参考片段请用许可宽松的素材制作（Blender 开源电影，CC-BY - **不要**用 Netflix
“Sol Levante”那类 CC-BY-NC-ND 的内容），命令是
`scripts/make-bench-sample.sh <source-4k>`；它会打印出用于固定的 SHA256。

### WSL2 的注意事项

在 WSL2 上，GPU 是通过 Mesa 那个实验性的 **d3d12** 驱动访问的。**NVENC 能用**
（真正的 CUDA 透传）；**VAAPI/QSV 不行** - d3d12 的 VA 驱动一遇到 Evey 的多档编码就崩。
Evey 检测到 WSL 后会在管理面板里把 VAAPI/QSV 置灰。VAAPI 和 QSV 请在原生 Linux 主机上验证。

## 浏览器里的 AC-3 / E-AC-3 音频（WebAssembly 解码器）

大多数浏览器无法原生解码 AC-3/E-AC-3（授权原因），按老办法这就逼着服务器重新编码音频。
Evey 的做法不同：给浏览器送去一个 WebAssembly 解码器 - 母版播放列表只放视频，
AC-3 音轨作为一条独立的音轨对外提供（直接拷贝，不重新编码），由客户端解码成 PCM
并与视频对齐时钟。只要源文件能原样打包，服务器在音频上就一点 CPU 都不花。

- 那些确实能原生解码 AC-3 的浏览器（Safari 一类）会被排除在外 - 靠的是能力检测，
  不是按名字点名 - 它们走普通的直通。
- **对反向代理的要求**：到达浏览器的 CSP 必须在 `script-src` 里带上 `'wasm-unsafe-eval'`，
  否则解码器无法编译，影片放起来是静音的。见
  [reverse-proxy.md](reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own)。

## 如何确认

打开**管理 → 服务器配置 → 编码校准**：那里会显示检测到的加速器、当前生效的后端、
实测出来的软件和硬件上限，以及所有自动发生的硬件→软件回落。换过硬件或驱动之后，
点“重新校准”按钮就能重跑一次基准测试。
