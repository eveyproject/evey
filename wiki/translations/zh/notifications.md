---
source_sha: 8d9681876fde4e364272e6256089c42f068178b9ea4848f17733c827d82a157f
reviewed: true
---
# 邮件和通知 { #e-mail-and-notifications }

有新的电影、电视剧、季、剧集或专辑加入时，Evey 会通知每个个人资料。它还会通知：

- **发起请求的人**：请求得到答复时；
- **管理员**：有新请求、备份失败、插件出错，以及字幕缓存已满时。

通知会出现在应用里，不需要任何设置。要通过邮件接收，需要先设置邮件服务器（见下文）。

## 通知出现在哪里 { #where-notifications-appear }

- **在每个应用里**：铃铛图标，上面显示未读通知的数量。始终开启。通知保留 30 天。每个人只会看到自己账户能打开的媒体库里的内容。
- **在浏览器里（推送）**：在网页版上，每个个人资料在**设置 → 通知**里开启**推送通知**，每个浏览器开启一次。浏览器只允许在 `https://` 地址（或 `localhost`）上这样做。在其他地址上，这项设置会显示**推送通知需要 HTTPS 地址：请通过 HTTPS 打开此服务器以启用推送通知。** Android 应用和电视应用不接收推送通知。
- **通过邮件**：设置好邮件后，每条通知也会发送到账户的电子邮件地址，使用个人资料的语言，并附上服务器的链接。有多个个人资料的账户，每个事件会收到一封邮件。用户不能自己关闭邮件通知。

??? info "推送密钥"

    服务器在第一次启动时会创建浏览器推送所需的密钥。如果要使用您自己的密钥，见[邮件和通知](environment/notifications.md#browser-push)。

## 设置邮件 { #set-up-e-mail }

Evey 通过一个您已有账户的邮件服务器发送邮件：您的邮件服务商，或者一个发信服务。下面这些值可以在服务商的帮助页面上找到。

1. 打开**管理 → 服务器配置 → 邮件 / SMTP**。
2. 填写这些栏位：

    | 栏位 | 填什么 |
    | --- | --- |
    | **SMTP 主机**、**端口** | 邮件服务器及其端口。常见的组合是 `587` 配 `starttls`，或 `465` 配 `ssl`。 |
    | **用户名**、**密码** | Evey 用来登录的账户。 |
    | **发件地址** | 每封邮件上显示的发件人地址。 |
    | **TLS 模式** | `starttls` 或 `ssl`，要与端口对应（见上）。`none` 表示不加密发送。 |

3. 点击**保存 SMTP 设置**。它会立即生效，无需重启。
4. 点击**发送测试邮件**。Evey 会给您自己账户的地址发一封短邮件。如果失败，它会显示邮件服务器的回复。

最常见的错误是 TLS 模式与端口不匹配。

邮件用于通知和重置密码。邀请是您自己发送的链接。

??? info "改用环境变量设置邮件"

    只有当卡片里没有填主机时，才会使用 compose 文件中 `environment:` 下的 `EVEY_SMTP_*` 变量：

    ```yaml title="docker-compose.yml"
    services:
      evey:
        environment:
          EVEY_SMTP_HOST: smtp.example.com
          EVEY_SMTP_PORT: "587"
          EVEY_SMTP_USERNAME: notifications@example.com
          EVEY_SMTP_PASSWORD: your-smtp-password
          EVEY_SMTP_FROM: noreply@example.com
          EVEY_SMTP_TLS: starttls
    ```

    一旦在卡片里保存了主机，所有栏位都取自卡片，变量会被忽略。这样，服务器永远不会把变量里的密码发给在面板里输入的邮件服务器。清空主机，就会改回使用变量。

## 选择个人资料接收哪些通知 { #choose-what-a-profile-receives }

在**设置 → 通知**里，每个个人资料为**电影**、**剧集**和**音乐**各选一个级别：

| 级别 | 会通知您哪些新内容 |
| --- | --- |
| **全部** | 每一个。这是默认设置。 |
| **相关** | 只通知与这个个人资料平常观看的内容有相同流派的影片。 |
| **仅重要** | 目前与“相关”相同。 |
| **已禁用** | 一个都不通知。 |

某个流派在这个个人资料最近 200 次播放中出现至少 3 次，才算“平常观看”。没有流派的影片，在“相关”和“仅重要”级别下永远不会通知。

同一部电视剧在 24 小时内新增多集时，您只会收到一条通知，里面写明集数。如果这部电视剧或这一季刚刚通知过，就不再发送。

## 让用户自行重置密码 { #let-people-reset-their-password }

1. 设置好邮件（见上文）。
2. 在**管理 → 服务器配置 → 服务器设置**里开启**密码找回**。它会立即生效。
3. 登录页上现在会显示**忘记密码？**。它会用邮件发送一个链接，用来设置新密码。

没有邮件服务器时，即使开启了这个选项，链接也保持隐藏。

<!-- sources
web push support: frontend/apps/web/src/lib/webpush.ts isPushSupported (serviceWorker + PushManager, secure context only) ; NotificationSettings.tsx shows pushNeedsHttps when unsupported
types: internal/domain/notification/model.go (Type*, IsAdminOnly: backup_failed, plugin_error, media_request) ; emitted: internal/app/notification/event_handlers.go, batching.go
retention 30 d: internal/app/notification/service.go defaultRetentionDays ; grouping 24 h: batching.go episodeGroupWindow + shouldSkipEpisodeNotif
list filtered by library access: internal/adapters/driving/grpcapi/notification_handler.go allowedLibraryIDs
bell on TV and mobile: frontend/apps/tv/src/components/TopHeader.tsx ; frontend/apps/mobile/app/(auth)/settings/notifications.tsx
levels: batching.go shouldNotify (relevant == highly_relevant) + fetchFavoriteGenres (GetFavoriteGenresBatch minCount 3, window 200 sessions: watch_history_analyzer.go)
defaults all: migrations/000005_notifications.up.sql (notification_settings_global) ; UI: frontend/apps/web/src/features/user/NotificationSettings.tsx ; i18n notificationSettings.level*
email: internal/adapters/driven/email/notification_emailer.go (push title key, recipient locale, one per account per FanoutID, base URL link)
SMTP live from the card, env fallback, panel host = panel only, defaults 587/starttls: internal/app/config/smtp.go (ResolveSMTPSettings, LiveSMTP) ; test send: internal/adapters/driving/http/smtp_test_handler.go
labels: i18n admin.serverConfig emailSmtp, smtpHost, port, smtpUsername, smtpPassword, fromAddress, tlsMode, saveSmtp, sendTestEmail
no native push in the mobile/TV apps (no push client) ; VAPID generated: cmd/evey/wire_helpers.go
password recovery: admin.serverSettings.passwordRecovery ; gated on smtpConfigured (cmd/evey/wire.go) ; auth.login.forgotPassword
admin-only types include the subtitle cache: internal/domain/notification/model.go - IsAdminOnly (TypeCacheFull) ; i18n notifications push.cache_full
labels: i18n admin.serverConfig.port, smtpUsername, smtpPassword, smtpEnvHint ; auth.settings pushNotifications, pushNeedsHttps ; levelAll, levelRelevant, levelImportant, levelDisabled
no invitation e-mail: internal/adapters/driven/email (no invite sender)
-->
