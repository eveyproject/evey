---
source_sha: 5df44f47a0668f747a8ddbed4f5d77234414ec1d6cfdf1c546c567546e2674af
reviewed: false
---
# 账户与登录

会话有效期、签名密钥和单点登录。

## 会话

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_JWT_PRIVATE_KEY_PATH` | `/data/jwt.key` | 签署访问令牌的密钥，在首次启动时生成。如果丢失，下次启动会生成新的密钥，应用会续期令牌：没有人会被登出。 |
| `EVEY_JWT_ACCESS_TTL` | `15m` | 访问令牌的有效期。应用会悄悄地续期它。 |
| `EVEY_JWT_REFRESH_TTL` | `168h`（7 天） | 登录会话的有效期：这么长时间不使用应用，就需要重新登录。 |
| `EVEY_JWT_REFRESH_LONG_TTL` | `4320h`（180 天） | 应用请求保持的会话的有效期，例如用快速连接登录的电视。 |

## 单点登录（OIDC）

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_OIDC_ENABLED` | `false` | 开启单点登录。 |
| `EVEY_OIDC_ISSUER` | 空 | 提供方的 issuer URL，例如 `https://auth.example.com/realms/main`。 |
| `EVEY_OIDC_CLIENT_ID` | 空 | 在提供方处为 Evey 注册的客户端。 |
| `EVEY_OIDC_CLIENT_SECRET` | 空 | 它的密钥。 |
| `EVEY_OIDC_REDIRECT_URL` | 空 | 发送给提供方的回调地址：`https://<你的地址>/api/v1/auth/oidc/callback`。为空：根据服务器的公开地址（**Admin → 远程访问**）生成。 |
| `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` | 空（全部） | 允许登录的电子邮件域名，逗号分隔。提供方必须已验证该地址。 |
| `EVEY_OIDC_ALLOW_SIGNUP` | 空 | `true` 或 `false`：为新的 SSO 用户创建账户。为空：仅在设置了电子邮件域名时开启。 |

这些是后备值：在 **Admin → 服务器配置** 的 **单点登录 (SSO)** 卡片中保存的每项设置都会取代对应变量，无需重启。设置提供方：[单点登录（OIDC）](../oidc.md)。
