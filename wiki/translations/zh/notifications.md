---
source_sha: 5dca3b7fe64acb5cf79df57c6da42ed90ae968596cba80b381b921667c8e36e9
reviewed: false
---
# 通知

Evey 可以在添加新媒体、备份失败以及发生其他事件时通知用户。

## 渠道

### 应用内通知
始终开启。用户会看到一个带未读数量的铃铛图标。无需配置。

### 电子邮件（SMTP）
在创建通知时发送一封电子邮件。需要配置 SMTP：

```env title=".env"
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=notifications@example.com
EVEY_SMTP_PASSWORD=your-smtp-password
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls
```

TLS 模式：
- `starttls`：先以明文连接，再升级到 TLS（587 端口，最常见）
- `ssl`：直接以 TLS 连接（465 端口）
- `none`：不加密（不推荐）

### Web Push
浏览器推送通知。VAPID 密钥在首次启动时自动生成。用户在自己的个人资料设置中选择开启。

## 密码找回

密码重置邮件需要配置 SMTP。管理员还必须开启该功能：

1. 配置 SMTP（见上文）。
2. 在 **Admin → 服务器配置** 中开启 **密码找回**。立即生效。
3. 登录页面随后会显示一个"忘记密码？"链接。

如果开启后又移除了 SMTP，密码找回会自动被禁用。

## 通知级别

用户可以按媒体类型设置自己的通知偏好：
- **全部**：所有内容都通知
- **相关**：只有类型与用户的观看记录相符时才通知
- **仅重要**：更严格的类型匹配
- **已禁用**：此类型不发送通知
