---
source_sha: bba2d9d0a70621281091516a01462cd4178130e57b6654f467dfe62130069342
reviewed: false
---
# OIDC / SSO 设置

Evey 支持通过 OpenID Connect 进行单点登录。用户可以使用 Google、Keycloak、Authentik、Authelia 或任何兼容 OIDC 的提供商登录。

## 配置

```env title=".env"
EVEY_OIDC_ENABLED=true
EVEY_OIDC_ISSUER=https://auth.example.com/realms/main
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=your-client-secret
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=example.com,company.org
```

## 设置提供商

### 1. 把 Evey 注册为 OIDC 客户端

在你的身份提供商中，创建一个新的客户端或应用：

| 字段 | 值 |
|-------|-------|
| Client ID | `evey` |
| Client Secret | 生成一个 |
| Redirect URI | `https://<你的地址>/api/v1/auth/oidc/callback` |
| Grant Type | Authorization Code |
| Scopes | `openid email profile` |

### 2. 配置 Evey

在你的 `.env` 中设置上面的环境变量。`EVEY_OIDC_ISSUER` 必须是你的 OIDC 提供商发现端点的基础 URL（不带 `/.well-known/openid-configuration`）。

### 3. 重启

```bash
docker compose restart evey
```

登录页面会显示一个"使用 SSO 登录"按钮。

## 工作原理

1. 用户在登录页面点击"使用 SSO 登录"
2. Evey 重定向到 OIDC 提供商
3. 用户在提供商处进行身份验证
4. 提供商带着授权码重定向回 Evey
5. Evey 用授权码换取令牌，并创建或更新用户账户

## 域名限制

`EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` 限制哪些电子邮件域名可以注册。留空则允许所有域名。

## 自动创建用户

首次通过 OIDC 登录的用户会被自动创建，并带有：
- 由 `preferred_username` 或电子邮件前缀生成的用户名
- 来自 OIDC 令牌的电子邮件
- 非管理员角色
- 一个默认个人资料

OIDC 用户不能设置本地密码：他们始终通过提供商进行身份验证。
