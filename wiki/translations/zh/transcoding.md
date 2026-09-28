---
source_sha: 0de496a664ed813f5ad423017ddfe0563349e3baaee95acf2ee582c2e970dd84
---

# 转码

客户端不支持源文件格式时，Evey 会实时转码。

**简而言之。** 大多数安装在这里什么都不用做：服务器会测量这台机器能转码多少，并按这个数量同时
接纳转码。要使用显卡，把它暴露给容器（见下文），然后在**管理 → 转码**里把**硬件加速**留在
**自动**即可。本页其余部分解释的是这个选择背后发生了什么。

## 工作原理

Evey 会自动挑出最好的播放方式：
1. **直接播放** - 格式本来就支持，不做任何处理
2. **重新封装** - 只换容器（MKV → MP4），不重新编码
3. **转码** - 重新编码成带自适应码率的 HLS

## 配置

下面两个值只作为首次启动的初始值：一旦在管理面板（**管理 → 转码**）里保存了值，就以面板为准。

```env
EVEY_TRANSCODING_MAX_CONCURRENT=2
# auto | off | nvenc | vaapi | qsv | rkmpp - 见下文的“硬件加速”一节
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

硬件加速在**管理 → 转码 → 编码校准**里控制：

- 一个总的**硬件加速**开关。关 = 只用软件（GPU 或驱动不稳时的应急出口），开 = 使用 GPU。
- 开启之后会出现**后端**选择：`Auto`（由校准挑出检测到的最佳后端），
  或者强制指定 `NVENC` / `VAAPI` / `QSV` / `RKMPP`。强制指定的后端如果实际用不了，
  会回落到软件，卡片上则显示当前真正生效的后端。

**`Auto` 做了什么。** 你把硬件暴露给容器（`/dev/dri`、NVIDIA 设备预留、Rockchip 的 MPP 节点），
剩下的交给 Evey，而且只凭实测 - 绝不看 CPU 或 GPU 的型号名：

1. **检测**：每个 DRM 渲染节点都探测 VAAPI，Intel 节点再探测 QSV；每块 NVIDIA GPU 按 CUDA
   索引探测；还有 Rockchip 的 MPP 节点。每个编码器都必须先通过一次真实编码才算数。
2. **认证**：每个检测到的加速器按运行时实际使用的流水线形态走一遍编码阶梯（720p → 2160p），
   保留它能以 1.2 倍实时速度撑住的最高档位，再通过一道画面正确性检查。720p 都达不到实时 =
   拒绝，只用软件。每个认证通过的后端都保留**自己的**上限、解码矩阵、功能和已验证的路径 -
   只能到 1080p 的加速器旁边哪怕有一个能到 2160p 的，它也只服务 1080p 阶梯，绝不越界。
3. **测量路径**：对每个后端验证过的每条路径（SDR、HEVC、AV1、GPU 色调映射、CPU 桥接、
   HDR 直通、解耦解码）以及软件路径，都在参考档位和该路径的上限处实测一次热身后的生产会话 -
   这样只在 720p 通过认证的后端、或是 720p 的软件保底，也都有自己的成本数据。
4. **按负载选择**：解析器针对每个请求，挑选已验证该路径且支持该编解码器的最便宜的认证后端
   （同一台机器上，HDR→SDR 可以落到 VAAPI 的 OpenCL 色调映射，而 HEVC 走更快的 QSV）。
   准入按该路径的实测成本，分别对照 CPU 池和加速器池为会话计价；池满时，GPU 流会溢出到 CPU，
   或者被拒绝。

软件阶梯的保底是 **720p SDR / 480p HDR**：即使 CPU 在留有余量的前提下一档都过不了，也仍会
提供这些档位，只是一次只接纳一个会话。直接播放和拷贝/remux 从不占用编码名额，完全不受这些影响。

这个选择会被保存下来，并当场重新跑一遍基准测试（不用重启）。无人值守的部署也可以从环境变量播下初始值：

```env
# 要么用老式的开关标志……
EVEY_TRANSCODING_HW_ACCEL_ENABLED=true
# ……要么用统一的模式（auto|off|nvenc|vaapi|qsv|rkmpp）；管理面板里的覆盖值优先。
EVEY_TRANSCODING_HW_MODE=auto
# 卡死的 GPU 超过这个时长就快速判负，直接转用软件（默认 5s）。
EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT=5s
# 在硬件路径上为每个 ABR 档位各起一个 ffmpeg（每档一个编码器），而不是用一个多路输出的
# 进程 - 这样单档出问题不会牵连其他档，也避开了会让脆弱的 VA 驱动崩溃的多路编码滤镜图。
# 默认关闭；软件路径永远走单进程滤镜图。见 docs/adr/0015。
EVEY_TRANSCODING_HW_PER_VARIANT=false
# 只编码客户端此刻真正请求的那一档 ABR（按需，每档一个编码器），而不是用一个 ffmpeg
# 产出整条阶梯。默认开启（=1）。多档模式虽然各档共用一次解码，但也会编码观众根本不看的
# 每一档 - 生产环境实测，三路白白浪费的编码把第四路饿死了。校准测量的正是运行时的同一种
# 形态（单档模式下每次探测一档，多档模式下是累积的并行阶梯），软件和硬件阶梯都如此，所以
# 上限与一个会话的真实开销一致。只有在没有硬件解码、而且源文件又很重的主机上才设成 =0，
# 以换回共享解码。两种取值都与缓存兼容。
EVEY_TRANSCODING_SINGLE_VARIANT=1
```

> **有五项设置如今在管理面板里**（管理 > 转码 > 高级限制）：`HW_PER_VARIANT`、
> `SINGLE_VARIANT`、`MAX_SESSIONS`、`SOFTWARE_MAX_HEIGHT` 和 `SOFTWARE_HDR_MAX_HEIGHT`。
> 在那里保存的值会持久化，并且优先于环境变量，所以改一个播放上限不必再去编辑 compose
> 文件、重建容器。这五项都在启动时读取，面板上也这么写着：保存只是存下你的选择，
> 重启才会让它生效。

### 编码校准用的样片

基准测试会拿一段 4K 片段量出这台主机的实时编码上限，SDR 和 HDR 分开测
（`software_max_height` / `software_hdr_max_height`）。**启动时从不运行它**：服务器凭手头已有的
（已保存的校准结果，或首次启动时的 720p 软件保底）在几毫秒内启动，并在第一个*安静时刻*运行
基准测试 - 连续五次探测、间隔 4 秒、没有任何播放且主机 CPU 低于 20 %。这样，2 到 5 分钟的
基准测试既不拖慢启动，也不会碰上正忙于迁移或扫描媒体库的主机（负载下测出的结果会标记为
*临时*，等空闲时重测）。测试进行期间，新的播放会被告知等待约 2 分钟（`calibrating`）；
**管理 → 转码 → 编码校准**会显示校准是待进行、进行中还是已完成，“重新校准”
（`POST /api/v1/admin/transcode/calibration/recalibrate`）则会立即运行它。上限是从这段片段的
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

### HDR 直通与 HDR10+

在探测过支持 Main10 编码的 GPU 上（`hw_hdr10_encode`），支持 HDR 且能解码 HEVC 的客户端播放
HDR 源时，会被重新编码为保留 HDR 的 HEVC Main10：静态 HDR10 信令（bt2020 / PQ 或 HLG、
母版显示 SEI）会被一并带过去。HDR10+ 的动态元数据**不会**被重新写出，因此 HDR10+ 影片在这条
路径上按普通 HDR10 播放。Dolby Vision profile 7 同样回落到它的 HDR10 基础层；profile 5 则
通过色调映射转为 SDR。

音频采样率永远不是重新编码的理由：客户端编解码器列表接受的 96 kHz 音轨会原样拷贝。解码器只到
48 kHz 的客户端必须在自己的编解码器声明里说明这一点，服务器不会替它假设。

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

打开**管理 → 转码 → 编码校准**：那里会显示检测到的加速器、当前生效的后端、
实测出来的软件和硬件上限，以及所有自动发生的硬件→软件回落。换过硬件或驱动之后，
点“重新校准”按钮就能重跑一次基准测试。
