---
source_sha: 72324d88dd91ef30b9ee280c903c4879dad626512f8f23deee264b8281e0538f
---

# 主题 API 参考

本文面向为 Evey 编写主题的外部开发者。主题受到的安全约束请看 [`security-model.md`](security-model.md)。

## 什么是主题

主题是一个 `kind: theme` 的包，**没有后端进程**。它可以包含：

- `theme.css` - 经过净化后，注入到每个页面的 `<head>` 里
- `theme.js` - 可选的 JavaScript，加载在一个隔离的上下文中，只能用受限的 `evey.theme.*` API

主题不能声明 `language`、`entrypoint` 或 `permissions`，一旦声明，安装会被拒绝。

## 清单结构

```yaml
name: aurora               # 必填；给人看的名字
version: 1.0.0             # 必填；语义化版本号
kind: theme                # 必填；必须是 "theme"

description: "..."         # 可选
author: "..."              # 可选
min_evey_version: "1.0.0"  # 可选

# 不要写 language、entrypoint、permissions - 对主题来说它们是非法的
```

ZIP 的根目录下必须有 `theme.css` 和/或 `theme.js`。两者都没有的包也能装上，只是这个主题什么效果都没有。

## theme.css

对外地址是 `/api/v1/themes/<id>/theme.css`。文件在安装时会过一遍 CSS 净化器（被拦下的写法见 [`security-model.md`](security-model.md)）。存下来并对外提供的是净化之后的结果 - 只要有规则被触发，你写的和最终注入的就会不一样。

样式表以 `<link rel="stylesheet">` 的形式注入 `<head>`，对宿主文档全局生效。标准 CSS 都能用：自定义属性、`@keyframes`、`@media`、`@font-face`（配 `data:` 源）、`@layer`、容器查询。

### 稳定的主题挂载点（选择器契约）

Evey 给一小批纯装饰性、不涉及敏感信息的元素打上了**稳定的类名**，你可以直接选中它们，
不必依赖那些随时会变的工具类。这些类名是一份契约：重构中不会被改名。

| 选择器 | 元素 | 可以放心改的样式 |
|---|---|---|
| `.evey-app-header` | 顶栏 | 颜色、高度、间距、模糊 - **绝不可以**隐藏或挪动其中的控件 |
| `.evey-media-grid` | 媒体卡片网格（媒体库、横排） | 列数、`gap`、对齐 |
| `.evey-media-card` | 单张媒体卡片（海报 + 信息） | 宽度、间距、悬停效果 |
| `.evey-poster` | 海报 / 封面图（详情页上也是它） | `aspect-ratio`、`border-radius`、阴影、填充方式 |
| `.evey-media-detail` | 媒体详情页的根节点 | 详情正文的布局 |

净化器禁止的东西在这里同样禁止（不许 `@import`、不许全屏浮层、控件上不许
`pointer-events:none`、不许命中 `[type=password]` / `.admin-*` / `.token-*` / `.secret-*`，
`z-index` 会被夹到上限内）。主题可以随意重塑这些挂载点的外观，但**不可以**把顶栏挪作他用，
去隐藏或冒充那些可信控件（退出登录、配置文件、真正的通知）。

### 示例：更大的海报、更密的网格、更利落的圆角

```css
/* 网格更紧凑，海报更大 */
.evey-media-card { width: 12rem; }
.evey-poster     { border-radius: 2px; }
.evey-media-grid { gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); }

.evey-media-card:hover { transform: scale(1.03); transition: transform 120ms ease; }
```

### 示例：自定义播放器主题

```css
.player-controls {
  background: linear-gradient(to top, rgba(0,0,0,.8), transparent);
  border-radius: 0 0 8px 8px;
}

.player-progress-bar {
  accent-color: var(--color-accent);
  height: 4px;
}
```

## theme.js

对外地址是 `/api/v1/themes/<id>/theme.js`（需要认证）。它加载在一个带 `sandbox="allow-scripts"` 和锁死的 Content-Security-Policy 的 iframe 里 - 脚本碰不到宿主页面，发不了网络请求，也读不了存储。

唯一可用的接口是 `window.evey.theme`，由宿主桥在 `theme.js` 运行之前注入。

### `evey.theme.css(rules)`

在运行时把一段 CSS 字符串注入宿主的样式层。适合那些取决于计算结果、或者取决于主题内部交互的动态样式。

```js
evey.theme.css(`
  .hero-banner {
    background-image: url(data:image/svg+xml,...);
  }
`);
```

### `evey.theme.log(level, msg)`

把一条日志输出到浏览器控制台，带 `[evey-theme]` 前缀。终端用户看不见。

- `level`：`"debug"` | `"info"` | `"warn"` | `"error"`
- `msg`：纯文本字符串

```js
evey.theme.log("info", "Aurora theme loaded.");
```

## V2 API（路线图）

以下方法尚在计划中，V1 还用不了：

### `evey.theme.registerComponent(slot, render)`（V2）

为某个具名插槽注册一个渲染函数。插槽挂载时，宿主会调用 `render(props, container)`。`container` 是一个限定在主题 shadow 上下文内的 DOM 节点 - 往 `container` 里写的东西，会出现在宿主页面的一个 ShadowRoot 之中。

```js
// V2 - 尚不可用
evey.theme.registerComponent("home.hero", function(props, container) {
  container.innerHTML = `<div class="my-hero">Welcome, ${props.userName}</div>`;
});
```

### `evey.theme.observe(stateKey, callback)`（V2）

订阅宿主的只读状态更新。被观察的键一有变化，回调就会被调用。

```js
// V2 - 尚不可用
evey.theme.observe("player.playing", function(isPlaying) {
  evey.theme.css(isPlaying ? ".hero { opacity: 0.3; }" : "");
});
```

### `evey.theme.transition(selector, animation)`（V2）

经由桥接给 `selector` 命中的宿主元素套上一个具名动画（不直接操作 DOM）。

## 示例

扩展示例里附带了两个主题：`examples/plugins/theme-dark-blue`（一个只覆盖颜色变量的简短 `theme.css`）和 `examples/plugins/open-workshop`（完整的界面重塑）。任选一个复制作为起点 - 两者都已经能通过净化器。
