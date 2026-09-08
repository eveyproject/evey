---
source_sha: b5ebe9aa528d6b195b210870c763195df806015fe80273fefda50e5e3927aad7
---

# OIDC / SSO 接入

Evey 支持通过 OpenID Connect 实现单点登录。用户可以用 Google、Keycloak、Authentik、Authelia 或任何符合 OIDC 规范的身份提供方登录。

## 配置

```env
EVEY_OIDC_ENABLED=true
EVEY_OIDC_ISSUER=https://auth.example.com/realms/main
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=your-client-secret
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=example.com,company.org
```

## 在身份提供方一侧的设置

### 1. 把 Evey 注册为一个 OIDC 客户端

在你的身份提供方里新建一个客户端 / 应用：

| 字段 | 取值 |
|-------|-------|
| Client ID | `evey` |
| Client Secret | 自行生成一个 |
| Redirect URI | `{EVEY_SERVER_BASE_URL}/api/v1/auth/oidc/callback` |
| Grant Type | 授权码（Authorization Code） |
| Scopes | `openid email profile` |

### 2. 配置 Evey

把上面那几个环境变量写进 `.env`。`EVEY_OIDC_ISSUER` 必须是身份提供方发现端点的基础地址（不带 `/.well-known/openid-configuration`）。

### 3. 重启

```bash
docker compose restart evey
```

登录页面随即出现“使用 SSO 登录”按钮。

## 工作流程

1. 用户在登录页面点击“使用 SSO 登录”
2. Evey 把浏览器重定向到身份提供方
3. 用户在身份提供方那里完成认证
4. 身份提供方带着一个授权码重定向回 Evey
5. Evey 用该授权码换取令牌，随后创建或更新用户账号

## 限制邮箱域名

`EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` 规定哪些邮箱域名可以注册。留空则不作限制。

## 自动创建用户

首次通过 OIDC 登录的用户会被自动创建，其属性为：
- 用户名取自 `preferred_username`，没有就取邮箱地址的前缀
- 邮箱取自 OIDC 令牌
- 非管理员角色
- 一个默认配置文件

OIDC 用户无法设置本地密码，他们始终通过身份提供方认证。
