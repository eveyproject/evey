---
source_sha: aacd60e164be25536f5b7ca3fe0a88655d153900e00d2521c8d0795dd2769d24
reviewed: true
---
# 语言与无障碍

本页说明 Evey 在国际化（i18n）和无障碍（a11y）方面做到了哪一步：先说哪些内容有翻译、翻译怎样送到每个客户端，
再说为辅助技术准备了什么、还缺什么。翻译目录放在仓库根目录的 `i18n/<locale>/*.yaml`，
由 `internal/pkg/i18n` 提供，再由每个应用的 i18n 模块加载。这是一个由一个人维护的自托管项目：
尤其是无障碍方面，请把它看作尽力而为，而不是经过认证。

## 国际化（i18n）

### 什么会翻译，什么不会

| Evey 会翻译的 | Evey 不翻译的 |
| --- | --- |
| 界面本身：按钮、菜单、设置、错误消息、推送通知的文字、通知邮件（使用收件人的语言） | 媒体元数据（标题、剧情简介、类型）：它们来自元数据提供方，使用设置向导或**管理 → 服务器配置 → 语言和地区**里设置的**元数据语言**（`EVEY_METADATA_LANGUAGE` 只是默认值） |
| | 您自己的媒体库、文件名，以及您输入的任何内容，都按原样保存 |

密码重置邮件用英文发送。界面语言和元数据语言是两个独立的设置：界面可以是法语，而一部电影的简介是英语，反过来也一样。

### 支持的界面语言

六种：**英语（en）**、**法语（fr）**、**德语（de）**、**西班牙语（es）**、**中文（zh）**、**印地语（hi）**。
英语是源语言，也是后备语言：其他语言里缺少的任何文字都会显示英语，而不是留空或显示键名。

### 一段文字怎样显示到屏幕上

- **唯一的来源：** 仓库根目录下的 [`/i18n/<locale>/*.yaml`](../i18n)，按命名空间分开
  （`common`、`auth`、`media`、`player`、`admin`、`errors`、`notifications`、`mobile`、`downloads`、`requests`）。
  所有界面都用这同一份目录。
- **服务器**加载它，并通过 `GET /api/v1/i18n/{locale}` 提供出去；服务器自己翻译 API 错误消息和通知文字时，也直接使用它。
- **网页应用**从这个端点加载翻译（i18next 配合 HTTP 后端），在浏览器的本地存储里留一份副本供下次启动使用，
  并让 `<html lang>` 始终跟随当前语言，这样辅助技术就会用正确的语言朗读。**移动和电视应用**通过共享的原生 i18n 模块从同一个端点加载，
  并自带一份打包好的英文兜底（`fallback.en.json`、`offlineStrings.ts`），用于服务器回应之前显示的界面。
- 语言按这个顺序选择：先看您账户里的偏好；没有的话，用浏览器或设备的语言（解析 `Accept-Language`，去掉地区部分：`fr-FR` 变成 `fr`）；
  再没有的话，用**管理 → 服务器配置 → 语言和地区**里的**服务器语言**（它也决定新账户的语言）；最后是英语。
- 插值和复数形式使用 i18next（`{{count}}` 等）。

### 还没有做到的

- **完整程度不一。** 英语是权威版本，始终完整；其他五种语言靠手工保持同步，有一个测试
  （`internal/pkg/i18n/parity_test.go`）会在键的集合出现偏差时失败。但一个全新的功能可能先只有英语，在翻译完成之前显示英语。
- **不支持从右到左书写的语言。** 没有阿拉伯语、希伯来语或波斯语，布局也没有为从右到左（RTL）设计或测试过：
  加一种这样的语言是实打实的工作（CSS 逻辑属性、`dir` 的处理），不只是多一个翻译文件。
- **翻译没有经过专业审校。** 它们是维护者水平的质量。欢迎指正。
- **按地区格式化只做了一部分。** 日期和数字依赖平台的 `Intl` 或浏览器的语言设置，并没有在每个界面上统一接入。

### 添加或修正一种语言

1. 把 `i18n/en/` 复制为 `i18n/<your-locale>/`，翻译 YAML 里的值（键名保持不变）。
2. 把这种语言加进
   [`internal/pkg/i18n/i18n.go`](../internal/pkg/i18n/i18n.go)
   里的允许列表（`supportedLocales`）。
3. 让所有语言的键集合完全一致：否则一致性测试会失败；`TestI18nPlaceholdersAgreeAcrossLocales`
   还会检查每段文字里的 `{{placeholders}}` 是否也一致。

## 无障碍（a11y）

### 现状

Evey 使用语义化 HTML、ARIA 标签和 React Native 的无障碍属性构建，下面列出的几项是有意为之的。
但它**没有**按照 WCAG 做过正式审计，测试套件里**没有自动化的无障碍测试**，也**没有**用屏幕阅读器系统地测试过。
有些部分大概可以配合辅助技术使用，有些部分大概不行。如果您依赖辅助技术，难免会遇到不顺手的地方，请把具体情况告诉我们。

### 已经具备的

| 方面 | 做了什么 |
| --- | --- |
| 减少动态效果 | 全局遵循 `prefers-reduced-motion: reduce`（`frontend/apps/web/src/styles/main.css`）：当您的操作系统要求时，动画和过渡效果实际上会被关闭 |
| 辅助技术的语言 | `<html lang>` 跟随当前的界面语言 |
| 键盘（播放器） | 完整的键盘操作，并有屏幕上的快捷键说明层（`?`，`KeyboardShortcutsOverlay.tsx`）；您在输入框里打字时，快捷键会暂停生效 |
| 带标签的控件 | 只有图标的按钮和滑块，在网页上带有 `aria-label`，在移动端带有 `accessibilityLabel`/`accessibilityRole`（覆盖了大部分可交互控件，但不是全部） |
| 主题 | 内置五个主题（默认深色、AMOLED、深红、默认浅色、浅蓝）；主题插件可以提供更多 |
| Android TV | 处理方向键焦点，方便用遥控器操作 |

### 还没有做到的

- **没有用屏幕阅读器完整走过一遍。** NVDA/VoiceOver/TalkBack 下的使用流程未经验证；
  一些自定义组件（菜单、对话框、进度条、轨道选择器）可能无法正确读出自己的状态或角色。
- **焦点管理没有经过审计。** 焦点高亮样式，以及对话框和浮层里的焦点顺序，有些地方不一致。
- **只对内置主题测量了对比度。** 一个单元测试（`frontend/packages/theme/src/contrast.test.ts`）要求每个内置主题的强调色、
  三组文字色阶和警示色组，在背景和两种表面上都达到 WCAG AA 的 4.5:1；电视应用会再检查一遍它派生出来的令牌
  （`ThemeProvider.contrast.test.ts`）。主题插件以及插件在运行时注入的颜色不受检查：自定义主题仍可能在个别地方不达标。不以 AAA 为目标。
- 除了浏览器或操作系统自带的缩放，**应用内没有文字缩放**，也没有专门的高对比度模式。
- **移动端的字幕样式选择有限**（这是底层 `expo-video` 播放器的限制），不过字幕轨道本身可以正常播放。

### 报告问题

请告诉我们是哪个页面或界面、哪个控件、您原本期待什么，以及您使用的辅助技术和版本。
具体的报告会得到修复；只说“这不够无障碍”，我们就无从下手。无障碍方面的修复按尽力而为的方式处理。

<!-- sources
catalogue: i18n/en, de, es, fr, hi, zh ; internal/pkg/i18n/i18n.go - supportedLocales, FallbackLocale, ParseLocale ; internal/pkg/i18n/parity_test.go ; internal/contractguard - TestI18nPlaceholdersAgreeAcrossLocales, TestServerEmittedI18nKeysExistInCatalogue
endpoint: internal/adapters/driving/http/server.go - GET /api/v1/i18n/{locale}
loaders: frontend/apps/web/src/i18n/index.ts - HttpBackend, getCached, documentElement.lang ; frontend/packages/client/src/native/i18n.ts ; frontend/apps/mobile/src/i18n/fallback.en.json ; frontend/apps/tv/src/i18n/offlineStrings.ts
mails: internal/adapters/driven/email/password_reset_emailer.go ; internal/adapters/driven/email/notification_emailer.go - WithNotificationLocale
labels: i18n admin.serverConfig.languageRegion, serverLanguage, metadataLanguage
a11y: frontend/apps/web/src/styles/main.css - prefers-reduced-motion ; frontend/apps/web/src/features/playback/KeyboardShortcutsOverlay.tsx ; frontend/packages/theme/src/themeVars.ts ; frontend/packages/theme/src/contrast.test.ts ; frontend/apps/tv/src/providers/ThemeProvider.contrast.test.ts ; frontend/apps/mobile/package.json - expo-video
-->
