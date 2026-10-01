---
source_sha: 83ece8837b08f345398777a65fee01eeec37d776c49c29ad7d1b6f675ab96046
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

这些变量在服务器启动时读取，只要 **Admin → 服务器配置** 中的 **邮件 / SMTP** 卡片没有填写主机就会使用它们。一旦卡片中保存了主机，所有字段都取自卡片，并且无需重启即可生效。

## 浏览器推送

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `VAPID_PUBLIC_KEY`、`VAPID_PRIVATE_KEY` | 自动生成 | 签署推送通知的密钥对。如果没有设置，服务器会在首次启动时生成一对并保存在数据库中；只有为了沿用你已经在使用的密钥对时才设置它们。 |
| `VAPID_SUBJECT` | `mailto:admin@localhost` | 推送服务看到的联系地址。 |
