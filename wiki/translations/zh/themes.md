---
source_sha: 53779971ebd8b5f41b11b97d89f9f1102e0d283c08d999b9cc028f3b92f630ca
reviewed: true
---
# 主题

主题用来改变 Evey 界面的外观：它是一个 `kind: theme` 的包，没有后端进程，由一个样式表组成，
还可以带一个脚本，这个脚本被关在一个隐藏的框架里运行。本页写给主题作者，说明这两个文件里可以写什么，
以及安装程序会改写或拒绝什么。主题和插件一样，从**管理 → 插件**安装；每位用户在**设置 → 外观 → 主题**里挑选自己的主题。

## 清单

```yaml title="plugin.yaml"
name: aurora               # 必填；给人看的名称
version: 1.0.0             # 必填；semver 版本号
kind: theme                # 必填

description: "..."         # 可选
author: "..."              # 可选

# 不要写 language、entrypoint、permissions 或 capabilities：写了安装就会被拒绝
```

ZIP 包的根目录里，在清单旁边放 `theme.css` 和/或 `theme.js`。两个都没有的包也能安装，只是看不出任何效果。
`examples/plugins/theme-dark-blue`（一个只覆盖颜色变量的简短 `theme.css`）和 `examples/plugins/open-workshop`（一套完整的外观重设计）
都能通过清理器，是很好的起点。

## theme.css

样式表以 `<link rel="stylesheet">` 的形式插入每个页面的 head 里，作用于整个文档：自定义属性、
`@keyframes`、`@media`、使用 `data:` 来源的 `@font-face`、`@layer` 和容器查询都可以用。它在
`/api/v1/themes/<id>/theme.css` 提供，不需要身份验证，所以浏览器记住的主题在登录之前就会生效。

### 从自定义属性入手

界面的各种表面、文字、强调色、边框和圆角，都取自 `:root` 上的一组自定义属性；
`examples/plugins/theme-dark-blue/theme.css` 列出了它们，每一个都有注释。覆盖这些属性，就能一次改变所有界面的外观：

```css title="theme.css"
:root {
  --bg: #0b1120;
  --surface: #162032;
  --text: #e2e8f0;
  --accent: #3b82f6;
  --accent-hover: #60a5fa;
  --radius: 8px;
}
```

### 稳定的样式挂钩

有几个纯装饰性的元素带有稳定的类名，您可以直接针对它们写样式，而不必依赖那些每个版本都会变化的工具类。
这是一项约定：这些类名不会被改名。

| 选择器 | 元素 | 可以放心修改的样式 |
|---|---|---|
| `.evey-app-header` | 顶部栏 | 颜色、高度、间距、模糊效果；绝不要隐藏或移动栏里的控件 |
| `.evey-media-grid` | 媒体卡片网格（媒体库、各行） | 列数、`gap`、对齐方式 |
| `.evey-media-card` | 一张媒体卡片（海报和信息） | 宽度、间距、悬停效果 |
| `.evey-poster` | 海报或封面图（详情页上也有） | `aspect-ratio`、`border-radius`、阴影、填充方式 |
| `.evey-media-detail` | 媒体详情页的根元素 | 详情主体的布局 |

```css title="theme.css - 更大的海报、更紧凑的网格、更锐利的边角"
.evey-media-card { width: 12rem; }
.evey-poster     { border-radius: 2px; }
.evey-media-grid { gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); }
.evey-media-card:hover { transform: scale(1.03); transition: transform 120ms ease; }
```

### 清理器会改写什么

安装时，`theme.css` 会经过一个清理器：主题不允许做的事情会被改写或删除，每处改动都记录一条警告，结果再写回文件。
实际提供出去的是清理后的文件，可能和您写的不一样。清理器从不拒绝整个包。

| 您写的 | 会发生什么 | 原因 |
|---|---|---|
| `@import ...` | 删除 | 主题必须自成一体；导入外部文件会造成网络泄露。 |
| `url(http://...)`、`url(https://...)`、`url(//...)`、`image-set()` 里的外部候选图、保存着这类 URL 的自定义属性 | 替换为 `none`，或使其失效 | 同样的原因。`data:` URI 原样保留。 |
| `url(javascript:...)`、`expression(...)`、`-moz-binding`、`behavior` | 删除 | 旧式的代码执行途径。 |
| 选择器匹配 `[type=password]`、`[type=hidden]`、`.admin-*`、`.token-*` 或 `.secret-*` 的规则 | 删除整个规则块 | 主题不能改变密码输入框或特权元素的样式。 |
| 用 `position: fixed` 或 `sticky` 盖住整个屏幕的规则：`inset: 0`、四条边都为 `0`，或者 `top`/`right`/`bottom`/`left: 0`、`width: 100vw`、`height: 100vh` 中至少两项且其中有一项是视口尺寸 | 删除整个规则块 | 这是典型的钓鱼遮罩层。用 `top: 0; left: 0` 固定、大小由内容决定的页眉会保留。同一个选择器在不同规则里的声明会合并起来计算。 |
| 在针对 `button`、`[role=button]`、`a[href*=logout]` 或 `[data-critical]` 的规则里写 `pointer-events: none` | 删除这一条声明，其余保留 | 让关键控件点不动，是一种界面伪装攻击。 |
| 大于 999 的 `z-index` | 压到 999 | 主题必须待在应用自己的对话框层之下。 |

转义字符会在这些检查之前先解码，所以用转义序列绕不过去。

## theme.js

脚本是可选的。它在 `/api/v1/themes/<id>/theme.js` 提供给已登录的用户，运行在一个隐藏的 `<iframe sandbox="allow-scripts">` 里，
这个框架的策略设置了 `connect-src 'none'`：即使脚本通过了安装扫描，它也碰不到宿主页面、宿主的存储，也上不了网。
它唯一能用的接口是 `window.evey.theme`，在脚本运行之前注入。

### `evey.theme.css(declarations)`

把自定义属性的声明发给宿主页面，由宿主页面应用到 `:root` 上。只接受这一种形式：一串 `--name: value`，
最多 200 条声明，每个值最多 500 个字符，不能含有 `url()`、`image-set`、`expression()`、`@import`、`javascript:`、`data:`、`http(s):`、
`//`、注释或花括号。其他内容一律丢弃，并在控制台里报告。一个属性只有在清理后的样式表引用了它的地方才会起作用。

```js title="theme.js"
evey.theme.css("--accent: #e8a33d; --accent-hover: #f0b95c");
```

### `evey.theme.log(level, message)`

把一行文字转发到浏览器控制台。`level` 取 `"debug"`、`"info"`、`"warn"` 或 `"error"`；消息超过 500 个字符的部分会被截掉。用户看不到任何内容。

`evey.theme.registerComponent` 和 `evey.theme.observe` 在对象上存在，但没有实现：宿主会对它们返回错误。

### 扫描器会拒绝什么

安装时，`theme.js` 会被当作文本扫描，只要匹配到一项，整个包就会被拒绝，并给出原因。
扫描器有意从严：主题没有任何正当理由提到这些名称，哪怕是在注释里。

源代码任何位置出现以下任一子字符串：

`document.cookie`、`localStorage`、`sessionStorage`、`indexedDB`、
`XMLHttpRequest`、`navigator.sendBeacon`、`navigator.serviceWorker`、
`WebSocket`、`EventSource`、`importScripts`、`document.write`、
`Element.prototype`、`Object.defineProperty`、`globalThis`、`window.parent`、
`window.top`、`window.opener`、`Reflect.get`、`Reflect.apply`、
`Reflect.construct`、`window.frames`、`frameElement`、`postMessage`。

以下任一模式：

| 模式 | 捕获的内容 |
|---|---|
| `\b(fetch\|eval\|atob\|btoa\|Function\|setTimeout\|setInterval)\s*\(` | 直接调用被禁止的全局函数 |
| `\bimport\s*\(` | 动态导入 |
| `(?:window\|self\|globalThis\|top\|parent\|document\|navigator\|location)\s*\[` | 用方括号访问宿主的全局对象，例如 `window["fetch"]` |
| `\.cookie\b` | 任何对 `.cookie` 的访问 |
| `\.constructor\s*\.\s*constructor` | `[].constructor.constructor` 逃逸手法 |
| `new\s+Function\b` | 动态构造代码 |
| `(?:window\|self\|top\|parent\|globalThis)\s*\.\s*(?:Function\|eval\|fetch\|atob\|btoa\|XMLHttpRequest\|WebSocket\|EventSource)\b` | 通过属性读取被禁止的全局对象 |
| `(?:window\|top\|parent\|document\|location\|globalThis\|self)\s*\.\s*[A-Za-z_$][\w$]*\s*=[^=]` | 给宿主的全局对象赋值 |
| `Object\s*\.\s*prototype` | 原型污染 |
| `\.register\s*\(\s*['"]\S+\.js` | 注册 service worker |

<!-- sources
manifest rules: internal/adapters/driven/pluginrunner/manifest.go - validateManifest (KindTheme)
theme selection: frontend/apps/web/src/features/user/ThemeSettings.tsx ; i18n common.appearance, common.theme ; GET /api/v1/plugins/themes
routes, auth: internal/adapters/driving/http/theme_asset_handler.go (theme.css open, theme.js authMw)
link injection, hidden iframe, connect-src none: frontend/apps/web/src/features/themes/ThemeLoader.tsx
sanitizer: internal/adapters/driven/pluginrunner/css_sanitizer.go - SanitizeCSS, reImport, reURLExternal, reURLExternalUnterminated, reImageSetCandidateExternal, reCustomPropExternalURL, reURLJavascript, reExpression, reMozBinding, reBehavior, reSensitiveSelector, overlaySignals.isOverlay, stripPointerEventsOnInteractive, reZIndex ; written back at install: internal/app/plugin/install.go - validateStagedPlugin
custom properties: examples/plugins/theme-dark-blue/theme.css
stable hooks: frontend/apps/web/src (evey-app-header, evey-media-grid, evey-media-card, evey-poster, evey-media-detail)
theme.js bridge: frontend/apps/web/src/features/themes/theme-bridge.ts - THEME_BRIDGE_SCRIPT ; policy: theme-css-policy.ts - applyThemeCSSPolicy, MAX_DECLARATIONS, MAX_VALUE_LENGTH, FORBIDDEN_IN_VALUE
scanner: internal/pkg/themejs/scan.go - blockedLiterals, blockedRegex, Scan
-->
