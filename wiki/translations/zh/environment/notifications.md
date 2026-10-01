---
source_sha: a2778ea0b85b4c03545d656feab9c42ce9cc78f8aa8a4a43beb5727f9eee0951
reviewed: false
---
# 电子邮件与通知

外发邮件（通知、密码重置）和浏览器推送。见[电子邮件与通知](../notifications.md)。

## 电子邮件（SMTP）

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_SMTP_HOST` | 空 | 邮件服务器。为空：不发送邮件。 |
| `EVEY_SMTP_PORT` | `587` | 它的端口。 |
| `EVEY_SMTP_USERNAME` | 空 | 用于登录的账户。 |
| `EVEY_SMTP_PASSWORD` | 空 | 它的密码。 |
| `EVEY_SMTP_FROM` | 空 | 发件人地址，例如 `noreply@example.com`。 |
| `EVEY_SMTP_TLS` | `starttls` | `starttls`、`ssl` 或 `none`。 |

这些变量在服务器启动时读取。**Admin → 服务器配置** 中的 **邮件 / SMTP** 卡片会保存相同的字段，但服务器不会读取它们。

## 浏览器推送

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `VAPID_PUBLIC_KEY`、`VAPID_PRIVATE_KEY` | 自动生成 | 签署推送通知的密钥对。如果没有设置，服务器会在首次启动时生成一对并保存在数据库中；只有为了沿用你已经在使用的密钥对时才设置它们。 |
| `VAPID_SUBJECT` | `mailto:admin@localhost` | 推送服务看到的联系地址。 |
