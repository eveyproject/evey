---
source_sha: 2ec48afdaf5dd44ee661048d21aa84c7ac2b22e7b6454b4270a203bd473922b4
---

# 主题安全模型

## 主题与扩展的关键区别

**主题**只活在前端。它没有后端进程，没有语言运行时，没有 IPC 套接字，也没有任何服务端权限。它由这些部分组成：

- `theme.css` - 一份注入到每个页面 `<head>` 里的样式表
- `theme.js`（可选）- 在一个隔离的上下文中执行的 JavaScript，只能用受限的 `evey.theme.*` API

**扩展**则一定带一个后端进程（必须有 `kind: plugin`、`language`、`entrypoint`）。主题若试图声明 `language`、`entrypoint` 或 `permissions`，安装时会被拒绝。

CSS 和 JS 都要在安装时先过一遍静态分析，通过了才收下这个主题。

## CSS 净化器的规则

`theme.css` 在安装时会交给 `SanitizeCSS` 处理。净化器会改写或删掉危险写法，并返回一串警告。它**不会**整份拒绝 - 干净的 CSS 原样通过。

### 被拦下的写法

**`@import`** - 整条删除。

任何 `@import` 指令都会被剥掉。理由：主题必须自成一体，引入外部样式表是一条数据外泄的通道。

**指向外部的 `url()`** - 替换成 `none`。

指向 `http://`、`https://` 或协议相对的 `//` 地址的 `url()` 取值都会被替换。`data:` URI（`data:image/*`、`data:font/*`）是允许的，原样通过。

**`url()` 里的 `javascript:`** - 替换成 `none`。

**`expression()`**（IE 遗留写法）- 替换成 `none`。

**`-moz-binding` 和 `behavior`** - 删除。

XBL/HTC 这类遗留的代码执行通道，整条删除。

**命中敏感选择器的规则** - 整个规则块删除。

只要选择器命中下列任何一项，整个规则块都会被剥掉：

- `[type="password"]`、`[type='password']` 或 `[type=password]`
- `[type="hidden"]`
- `.admin-<任意后缀>`
- `.token-<任意后缀>`
- `.secret-<任意后缀>`

理由：绝不能让主题给密码输入框或特权界面元素改样式，那可以用来做界面劫持。

**全屏浮层规则** - 整个规则块删除。

一个规则块里出现 `position: fixed` 或 `position: sticky`，同时又至少出现 `top: 0`、`left: 0`、`width: 100vw`、`height: 100vh` 中的两项，它就会被删除。这套组合正是 CSS 钓鱼浮层的标准写法。

**交互元素上的 `pointer-events: none`** - 从规则块里删掉这条声明。

当规则的选择器命中 `button`、`[role="button"]`、`a[href*=logout]` 或 `[data-critical]` 时，该规则里的 `pointer-events: none` 声明会被删掉，规则的其余部分保留。理由：让退出登录或关键操作的按钮点不动，是一种界面劫持攻击。

**大于 999 的 z-index** - 夹到 999。

任何大于 999 的 `z-index` 取值都会被改写成 999，并记下一条警告。主题不得把自己叠到应用自身的模态框和浮层之上。

### 主题在 CSS 里能做什么

- 完全定制首页布局、主视觉区、卡片网格、侧边栏
- 重塑视频播放器的控件、进度条和各种浮层
- 自定义字体（用 `data:`，或者内联 `@font-face` 配 `src: url(data:...)`）
- 动画与过渡（`@keyframes`、`transition`、`animation`）
- 自定义属性（`--my-color: #...`）、覆盖 `:root`
- 媒体查询、容器查询
- 一切不命中密码输入框以及 `.admin-*`、`.token-*`、`.secret-*` 的选择器

## 主题 JS 扫描器的规则

`theme.js` 在安装时由 `internal/pkg/themejs/scan.go` 扫描。只要命中一条，安装就会被拒绝，并给出说明性的报错。这里没有“警告”这一档 - 扫描器是有意保守的（误报可以接受，因为主题根本没有正当理由去碰这些 API）。

### 被拦下的字面字符串

源码中任何位置出现下列子串之一，安装即被拒绝：

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

### 被拦下的正则模式

| 模式 | 它拦的是什么 |
|---|---|
| `\b(fetch\|eval\|atob\|btoa\|Function\|setTimeout\|setInterval)\s*\(` | 直接调用被禁的全局函数 |
| `\bimport\s*\(` | 动态 import |
| `(?:window\|self\|globalThis\|top\|parent\|document\|navigator\|location)\s*\[` | 用方括号访问宿主全局对象（能抓到 `window["fetch"]`） |
| `\.cookie\b` | 任何对 `.cookie` 的访问，无论走属性描述符还是计算属性 |
| `\.constructor\s*\.\s*constructor` | `[].constructor.constructor` 逃逸 |
| `new\s+Function\b` | 用 `new Function(...)` 现造代码 |
| `(?:window\|self\|top\|parent\|globalThis)\s*\.\s*(?:Function\|eval\|fetch\|atob\|btoa\|XMLHttpRequest\|WebSocket\|EventSource)\b` | 读取被禁全局对象的属性 |
| `(?:window\|top\|parent\|document\|location\|globalThis\|self)\s*\.\s*[A-Za-z_$][\w$]*\s*=[^=]` | 给宿主全局对象的属性赋值 |
| `Object\s*\.\s*prototype` | 原型链污染 |
| `\.register\s*\(\s*['"]\S+\.js` | 注册 Service Worker |

### 主题 JS 的运行时隔离

在运行时，`theme.js` 执行在一个带 `sandbox="allow-scripts"` 和锁死 CSP 的 `srcdoc` iframe 里。就算安装时的扫描器被绕过去了，这个运行时 iframe 也拿不到宿主文档的同源访问权，碰不到宿主的 `window`、`localStorage` 和 Cookie，更没有对外的网络（CSP `connect-src 'none'`）。

唯一的通信渠道是宿主桥注入的 `evey.theme.*` API。可用的方法见 [`api-reference.md`](api-reference.md)。

## 在 theme.js 里你能做什么

- 调用 `evey.theme.css(rules)` 往宿主的样式层注入 CSS
- 调用 `evey.theme.log(level, msg)` 把调试信息输出到浏览器控制台
- 用 `evey.theme.registerComponent(slot, render)` 注册组件渲染函数（V2，见下文）
- 用 `evey.theme.observe(stateKey, cb)` 观察只读的播放器和应用状态（V2）

## 你不能做什么

- 从任何地址取数据（`fetch`、`XMLHttpRequest`、`WebSocket`、`navigator.sendBeacon`）
- 读取用户凭据或令牌（`document.cookie`、`localStorage`、`sessionStorage`）
- 逃出 iframe 的上下文（`window.parent`、`window.top`、`frameElement`）
- 渲染一个铺满整个视口的全屏浮层
- 用 CSS 命中密码输入框，或 `.admin-*`、`.token-*`、`.secret-*` 这类选择器
- 让退出登录或关键操作的按钮点不动（`pointer-events: none`）
- 把自己叠到应用自身的模态框之上（`z-index > 999`）
