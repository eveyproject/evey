---
source_sha: 7cc0fb43b6f9e14980a2332dd94e4c79bea3397ef66018840bb68f8b90a78ca2
reviewed: true
---
# 邮件和通知

这些是外发邮件（通知、重置密码）和浏览器推送背后的变量。邮件通常在面板里设置，
见[邮件和通知](../notifications.md)；这些变量是它的备用方式。

## 邮件（SMTP）

这些变量在启动时读取，在**管理 → 服务器配置**的**邮件 / SMTP**卡片里没有填写主机时使用。
一旦在那里保存了主机，所有字段都由这张卡片提供，并且不用重启就能生效。

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_SMTP_HOST` | 空 | 邮件服务器。为空时：不发送任何邮件。 | 服务器配置 → 邮件 / SMTP → **SMTP 主机**，实时生效 |
| `EVEY_SMTP_PORT` | `587` | 它的端口。 | 同一张卡片 → **端口**，实时生效 |
| `EVEY_SMTP_USERNAME` | 空 | 用来登录的账户。 | 同一张卡片 → **用户名**，实时生效 |
| `EVEY_SMTP_PASSWORD` | 空 | 它的密码。 | 同一张卡片，实时生效 |
| `EVEY_SMTP_FROM` | 空 | 发件人地址，例如 `noreply@example.com`。 | 同一张卡片，实时生效 |
| `EVEY_SMTP_TLS` | `starttls` | `starttls`、`ssl` 或 `none`。 | 同一张卡片，实时生效 |

## 浏览器推送

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY` | 自动生成 | 给推送通知签名的密钥对。没有设置时，服务器第一次启动会生成一对，保存在数据库里；只有想继续使用已有的一对密钥时才需要设置，而且两个都要设置：只设一个，推送会被关闭，并给出警告。 | 无 |
| `VAPID_SUBJECT` | `mailto:admin@localhost` | 推送服务看到的联系地址。 | 无 |

<!-- sources
internal/config/config.go - EVEY_SMTP_*, VAPID_* ; WarnValidate (one VAPID key alone)
SMTP card wins, env fallback: internal/app/config/smtp.go ResolveSMTPSettings ; i18n admin.serverConfig.emailSmtp, smtpHost, port, smtpUsername
vapid generation: cmd/evey/wire_helpers.go - resolveVAPIDKeys (vapid.public_key / vapid.private_key)
-->
