---
source_sha: 305f13e3ff6f705b51384ec1d553349c48887a6ee0bd367b61e102a000be8911
reviewed: false
---
# 主题

主题用于重新设计 Evey 界面的样式。它是一个 `kind: theme` 的包，**没有后端进程**，包含：

- `theme.css`，一个经过清理后注入每个页面的样式表；
- 可选的 `theme.js`，一个在隔离框架中运行、带有小型 `evey.theme` API 的脚本。

主题不能声明 `language`、`entrypoint` 或 `permissions`：如果声明了，安装会被拒绝。两个文件都会在安装时检查。主题和插件一样，从 **Admin → 插件** 安装。

## 清单架构

```yaml
name: aurora               # 必填；可读的名称
version: 1.0.0             # 必填；semver
kind: theme                # 必填；必须为 "theme"

description: "..."         # 可选
author: "..."              # 可选
min_evey_version: "1.0.0"  # 可选

# 不要 language、entrypoint、permissions：它们对主题无效
```

ZIP 的根目录必须包含 `theme.css` 和/或 `theme.js`。两者都没有的包会被接受，但主题没有任何可见效果。

## theme.css

在 `/api/v1/themes/<id>/theme.css` 提供。文件在安装时经过 CSS 清理器处理。保存并提供的是清理后的输出：如果触发了某些规则，你写的内容和被注入的内容可能不同。

样式表以 `<head>` 中的 `<link rel="stylesheet">` 注入。它全局作用于宿主文档。适用标准 CSS：自定义属性、`@keyframes`、`@media`、`@font-face`（使用 `data:` 来源）、`@layer`、容器查询。

清理器会改写或删除主题不允许做的事情，并为每次修改记录一条警告；它不会拒绝文件。保存并提供的是清理后的输出，所以它可能与你写的不同。

### 清理器删除的内容

**`@import`**：完全删除。

任何 `@import` 指令都会被去掉。理由：主题必须是自包含的；导入外部样式表是一种网络泄露途径。

**外部 `url()`**：替换为 `none`。

指向 `http://`、`https://` 或协议相对 `//` URL 的 `url()` 值会被替换。`data:` URI（`data:image/*`、`data:font/*`）是允许的，会原样通过。

**`url()` 中的 `javascript:`**：替换为 `none`。

**`expression()`**（IE 遗留）：替换为 `none`。

**`-moz-binding` 和 `behavior`**：删除。

遗留的 XBL/HTC 执行途径，完全删除。

**敏感选择器规则**：删除整个规则块。

选择器匹配以下任何一项的规则块会被完全去掉：

- `[type="password"]`、`[type='password']` 或 `[type=password]`
- `[type="hidden"]`
- `.admin-<anything>`
- `.token-<anything>`
- `.secret-<anything>`

理由：主题不能以可用于界面伪装的方式为密码输入框或特权界面元素设置样式。

**全屏叠加规则**：删除整个规则块。

当一个规则块把 `position: fixed` 或 `position: sticky` 与一个覆盖屏幕的盒子结合时，它会被删除：`inset: 0`、四边都为 `0`，或者 `top: 0`、`right: 0`、`bottom: 0`、`left: 0`、`width: 100vw`、`height: 100vh` 中至少两项且包含两个视口尺寸之一。用 `top: 0; left: 0` 固定、按内容确定大小的页眉会被保留。同一个选择器的声明会跨规则累加，所以把这个模式拆到多个块中也无法绕过。这种模式就是典型的 CSS 钓鱼叠加层。

**交互元素上的 `pointer-events: none`**：从规则块中删除该声明。

当规则的选择器指向 `button`、`[role="button"]`、`a[href*=logout]` 或 `[data-critical]` 时，该规则中的任何 `pointer-events: none` 声明都会被删除。规则的其余部分保留。理由：隐藏登出或关键操作按钮的可点击性是一种界面伪装攻击。

**高于 999 的 z-index**：限制为 999。

任何大于 999 的 `z-index` 值都会被改写为 999。会记录一条警告。主题不能把自己叠放在应用自身的模态框/叠加层之上。

### 主题能用 CSS 做什么

- 自定义整个首页布局、主视觉区域、卡片网格、侧边栏
- 重新设计视频播放器控件、进度条、叠加层的样式
- 自定义字体（通过 `data:` 或使用 `src: url(data:...)` 的内联 `@font-face`）
- 动画和过渡（`@keyframes`、`transition`、`animation`）
- 自定义属性（`--my-color: #...`）、`:root` 覆盖
- 媒体查询、容器查询
- 任何不指向密码字段、`.admin-*`、`.token-*`、`.secret-*` 的选择器

### 稳定的样式挂钩（选择器约定）

Evey 用**稳定的类名**标记了一小组装饰性、非敏感的元素，你可以直接指向它们，而不必依赖（易变的）工具类。这是一项约定：它们在重构中不会被重命名。

| 选择器 | 元素 | 可以安全修改的样式 |
|---|---|---|
| `.evey-app-header` | 顶部栏 | 颜色、高度、间距、模糊；**永远不要**隐藏或移动其中的控件 |
| `.evey-media-grid` | 媒体卡片网格（媒体库、行） | 列、`gap`、对齐 |
| `.evey-media-card` | 一张媒体卡片（海报 + 信息） | 宽度、间距、悬停 |
| `.evey-poster` | 海报/图片视觉元素（详情页上也有） | `aspect-ratio`、`border-radius`、阴影、适应方式 |
| `.evey-media-detail` | 媒体详情页的根元素 | 详情主体的布局 |

清理器禁止的一切仍然适用（不能 `@import`、不能全屏叠加、控件上不能 `pointer-events:none`、不能 `[type=password]` / `.admin-*` / `.token-*` / `.secret-*`、`z-index` 受限）。主题可以自由地重新设计这些挂钩的样式；但**不能**改变页眉的用途来隐藏或冒充受信任的控件（登出、个人资料、真实的通知）。

### 示例：更大的海报、更密的网格、更锐利的边角

```css
/* 更紧凑的网格上更大的海报 */
.evey-media-card { width: 12rem; }
.evey-poster     { border-radius: 2px; }
.evey-media-grid { gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); }

.evey-media-card:hover { transform: scale(1.03); transition: transform 120ms ease; }
```

### 示例：不同的强调色

界面的按钮、链接、激活状态和播放器的进度条都取自几个自定义属性；`examples/plugins/theme-dark-blue/theme.css` 列出了它们。覆盖它们会一次性改变所有界面的样式：

```css
:root {
  --accent: #e8a33d;
  --accent-hover: #f0b95c;
}
```

## theme.js

在 `/api/v1/themes/<id>/theme.js` 提供（需要身份验证）。在一个带有 `sandbox="allow-scripts"` 和锁定的 Content Security Policy 的 iframe 中加载：脚本无法访问宿主页面，无法发起网络请求，也无法读取存储。

唯一可用的接口是 `window.evey.theme`，由宿主桥接在 `theme.js` 运行之前注入。

即使是通过了安装扫描的脚本，也无法访问宿主页面、它的存储或网络：框架没有同源访问权限，其策略设置了 `connect-src 'none'`。

### `evey.theme.css(rules)`

在运行时向宿主的样式层注入一段 CSS 字符串。用于依赖计算值或主题内用户交互的动态样式。

```js
evey.theme.css(`
  .hero-banner {
    background-image: url(data:image/svg+xml,...);
  }
`);
```

### `evey.theme.log(level, msg)`

向浏览器控制台输出一条日志消息，前缀为 `[evey-theme]`。不会显示给最终用户。

- `level`：`"debug"` | `"info"` | `"warn"` | `"error"`
- `msg`：纯文本字符串

```js
evey.theme.log("info", "Aurora theme loaded.");
```

### 扫描器拒绝的内容

只要匹配一项就会拒绝安装，并给出原因。扫描器有意保守：主题没有任何正当理由使用这些 API。

#### 字面字符串

如果源代码中任何位置出现以下任何子字符串，安装都会被拒绝：

- `document.cookie`
- `localStorage`
- `sessionStorage`
- `indexedDB`
- `XMLHttpRequest`
- `navigator.sendBeacon`
- `navigator.serviceWorker`
- `WebSocket`
- `EventSource`
- `importScripts`
- `document.write`
- `Element.prototype`
- `Object.defineProperty`
- `globalThis`
- `window.parent`
- `window.top`
- `window.opener`
- `Reflect.get`
- `Reflect.apply`
- `Reflect.construct`
- `window.frames`
- `frameElement`
- `postMessage`

#### 模式

| 模式 | 捕获的内容 |
|---|---|
| `\b(fetch\|eval\|atob\|btoa\|Function\|setTimeout\|setInterval)\s*\(` | 直接调用被禁止的全局对象 |
| `\bimport\s*\(` | 动态导入 |
| `(?:window\|self\|globalThis\|top\|parent\|document\|navigator\|location)\s*\[` | 对宿主全局对象的方括号访问（可捕获 `window["fetch"]`） |
| `\.cookie\b` | 任何 `.cookie` 访问，无论通过属性描述符还是计算属性 |
| `\.constructor\s*\.\s*constructor` | `[].constructor.constructor` 逃逸 |
| `new\s+Function\b` | 用 `new Function(...)` 构造代码 |
| `(?:window\|self\|top\|parent\|globalThis)\s*\.\s*(?:Function\|eval\|fetch\|atob\|btoa\|XMLHttpRequest\|WebSocket\|EventSource)\b` | 读取被禁止的全局对象的属性 |
| `(?:window\|top\|parent\|document\|location\|globalThis\|self)\s*\.\s*[A-Za-z_$][\w$]*\s*=[^=]` | 给宿主全局对象的属性赋值 |
| `Object\s*\.\s*prototype` | 原型污染 |
| `\.register\s*\(\s*['"]\S+\.js` | 注册 service worker |

## 示例

插件示例中附带了两个主题：`examples/plugins/theme-dark-blue`（一个只覆盖颜色变量的简短 `theme.css`）和 `examples/plugins/open-workshop`（完整的样式重设计）。复制其中任何一个作为起点：两者都已经能通过清理器。
