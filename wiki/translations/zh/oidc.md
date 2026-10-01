---
source_sha: 2edc135c644943052453572d9223c7834031131c057cd92f3e44e3a9ae72cc8c
reviewed: false
---
# OIDC / SSO 设置

Evey 支持通过 OpenID Connect 进行单点登录：Keycloak、Authentik、Authelia、Google 或任何其他 OIDC 提供方。网页和 Android 应用的登录页随后会在密码表单旁显示 **使用 SSO 登录**。

## 1. 在提供方注册 Evey

在提供方创建一个客户端（或应用）：

| 字段 | 值 |
|-------|-------|
| Client ID | `evey` 或任意名称 |
| 客户端密钥 | 生成一个（必填） |
| 重定向 URI | `https://<你的地址>/api/v1/auth/oidc/callback` |
| 登出后重定向 URI | `https://<你的地址>/login` |
| 授权类型 | Authorization Code |
| Scopes | `openid email profile` |

## 2. 配置 Evey

```env title=".env"
EVEY_OIDC_ENABLED=true
EVEY_OIDC_ISSUER=https://auth.example.com/realms/main
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=your-client-secret
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=example.com,company.org
```

`EVEY_OIDC_ISSUER` 是提供方的基础 URL，不含 `/.well-known/openid-configuration`。完整列表（包括 `EVEY_OIDC_REDIRECT_URL`）见[账户与登录](environment/accounts.md)。

## 3. 重启

```bash
docker compose up -d
```

服务器在启动时联系提供方。如果无法访问签发方，或客户端密钥为空，**服务器将无法启动**：日志会说明原因。

## 谁可以登录

第一次 SSO 登录会创建一个 Evey 账户，没有密码，带一个个人资料。用户名取提供方的 `preferred_username`，否则取电子邮件中 `@` 之前的部分。该账户是普通用户，和任何没有限制的账户一样可以看到所有媒体库：之后请在 **Admin → 用户** 中加以限制（见[用户](administration/users.md)）。

**用 `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` 限制注册。** 留空时，任何在提供方拥有账户的人都会在你的服务器上获得一个账户；对于 Google 这样的公共提供方，这意味着任何人。设置域名后，只接受这些地址，并且提供方必须将地址标记为已验证。

账户通过提供方及其用户 ID 识别，而不是通过电子邮件。已经拥有相同电子邮件或相同用户名的本地账户的人无法创建 SSO 账户：登录会失败并显示“SSO 登录失败”。请在提供方使用其他地址，或先删除本地账户。

已禁用的账户照常会在登录时被拒绝。

## 登出

当提供方公布了登出端点时，从 Evey 登出也会结束提供方的会话，然后返回 Evey 的登录页。
