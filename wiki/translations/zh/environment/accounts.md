---
source_sha: e3a9611908da29d559070f9745961b1f31fad6154f252e1304481c5e371f2c0c
reviewed: true
---
# 账户与登录

这些是会话和单点登录背后的变量：一次登录能保持多久，签名密钥放在哪里，以及 OIDC 提供商。
单点登录通常在面板里设置，见[单点登录（OIDC）](../oidc.md)；这里的变量是它的后备设置。

## 会话

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_JWT_PRIVATE_KEY_PATH` | `/data/jwt.key` | 给访问令牌签名的密钥，第一次启动时生成。如果它丢失了，下次启动会生成一个新的，应用会更新自己的令牌：不会有任何人被登出。 | 无 |
| `EVEY_JWT_ACCESS_TTL` | `15m` | 访问令牌的有效期。应用会在后台自动更新它。 | 无 |
| `EVEY_JWT_REFRESH_TTL` | `168h`（7 天） | 一次登录会话的有效期：这么长时间没有使用应用，就要重新登录。 | 无 |
| `EVEY_JWT_REFRESH_LONG_TTL` | `4320h`（180 天） | 应用要求保持的会话的有效期，例如用快速连接登录的电视。 | 无 |

## 单点登录（OIDC）

在**管理 → 服务器配置**的**单点登录 (SSO)**卡片里保存的每个字段，都会取代对应的变量，不需要重启。

| 变量 | 默认值 | 作用 | 面板 |
| --- | --- | --- | --- |
| `EVEY_OIDC_ENABLED` | `false` | 开启单点登录。 | 服务器配置 → 单点登录 (SSO)，实时生效 |
| `EVEY_OIDC_ISSUER` | 空 | 提供商的签发者（issuer）地址，例如 `https://auth.example.com/realms/main`。 | 同一张卡片，实时生效 |
| `EVEY_OIDC_CLIENT_ID` | 空 | 在提供商那里为 Evey 注册的客户端。 | 同一张卡片，实时生效 |
| `EVEY_OIDC_CLIENT_SECRET` | 空 | 这个客户端的密钥。开启了单点登录却没有设置它，服务器启动时会给出警告。 | 同一张卡片，实时生效 |
| `EVEY_OIDC_REDIRECT_URL` | 空 | 发给提供商的回调地址：`https://<your address>/api/v1/auth/oidc/callback`。为空时：根据服务器的公网地址（**管理 → 远程访问**）生成。 | 无 |
| `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` | 空（全部） | 允许登录的邮箱域名，用逗号分隔。提供商必须已经验证过这个邮箱地址。 | 同一张卡片，实时生效 |
| `EVEY_OIDC_ALLOW_SIGNUP` | 空 | `true` 或 `false`：是否为新的 SSO 用户创建账户。为空时：只有设置了邮箱域名才开启。 | 同一张卡片，实时生效 |

<!-- sources
internal/config/config.go - EVEY_JWT_*, EVEY_OIDC_* ; WarnValidate (OIDC enabled without secret)
key regeneration: cmd/evey/wire_infra.go - auth.LoadOrGenerateRSAKey ; refresh tokens in DB: internal/adapters/driven/postgres/refresh_token_repo.go
long ttl: internal/adapters/driving/grpcapi/user_handler.go - refreshTTLOverride ; internal/adapters/driving/grpcapi/interceptors/client_info.go (x-persistent-session)
redirect: internal/app/user/oidc_service.go (oauth2 RedirectURL as given) ; panel over env: internal/app/user/oidc_settings.go ResolveOIDCConfig (OIDCKey*)
panel: i18n admin.sso.title
-->
