---
source_sha: eea86697845889523ecbd07dd3d88da0143ac37f477404249051c73e160c80c8
---

# 通知

新增媒体、备份失败以及其他事件发生时，Evey 可以通知用户。

## 通知渠道

### 站内通知
始终开启。用户会看到一个带未读数量的铃铛图标。无需任何配置。

### 邮件（SMTP）
每产生一条通知就发出一封邮件。前提是配置好 SMTP：

```env title=".env"
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=notifications@example.com
EVEY_SMTP_PASSWORD=your-smtp-password
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls
```

TLS 模式：
- `starttls` - 先用明文连接，再升级到 TLS（587 端口，最常见）
- `ssl` - 直接用 TLS 连接（465 端口）
- `none` - 不加密（不推荐）

### 浏览器推送（Web Push）
由浏览器弹出的推送通知。VAPID 密钥在首次启动时自动生成。用户自行在配置文件的设置里开启。

## 找回密码

发送密码重置邮件需要先配置 SMTP，管理员还必须启用这项功能：

1. 配置 SMTP（见上文）
2. 在管理面板的服务器配置里，把 `auth.password_recovery_enabled` 设为 `true`
3. 登录页面随即出现“忘记密码？”链接

启用之后又撤掉 SMTP 配置的话，找回密码会自动关闭。

## 通知级别

用户可以按媒体类型分别设置通知偏好：
- **全部** - 任何事都通知
- **相关** - 仅当类型与该用户的观看历史吻合时才通知
- **仅重要** - 类型匹配的要求更严格
- **已禁用** - 这类媒体不发任何通知
