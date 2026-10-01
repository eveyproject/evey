---
source_sha: 24e34e0b33eb8acd096fe2be879ca8bda28a01e6cbf963847e6dd4b661827adb
reviewed: false
---
# 账户与登录

会话有效期、签名密钥和单点登录。

## 会话

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_JWT_PRIVATE_KEY_PATH` | `/data/jwt.key` | 签署每个会话的密钥，在首次启动时生成。丢失它会让所有人登出。 |
| `EVEY_JWT_ACCESS_TTL` | `15m` | 访问令牌的有效期。应用会悄悄地续期它。 |
| `EVEY_JWT_REFRESH_TTL` | `168h`（7 天） | 登录会话的有效期：这么长时间不使用应用，就需要重新登录。 |
| `EVEY_JWT_REFRESH_LONG_TTL` | `4320h`（180 天） | 应用请求保持的会话的有效期，例如用快速连接登录的电视。 |

## 单点登录（OIDC）

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `EVEY_OIDC_ENABLED` | `false` | 开启单点登录。修改后需重启。 |
| `EVEY_OIDC_ISSUER` | 空 | 提供商的 issuer URL，例如 `https://auth.example.com/realms/main`。 |
| `EVEY_OIDC_CLIENT_ID` | 空 | 在提供商处为 Evey 注册的客户端。 |
| `EVEY_OIDC_CLIENT_SECRET` | 空 | 它的密钥。 |
| `EVEY_OIDC_REDIRECT_URL` | 空 | 发送给提供商的回调地址：`https://<你的地址>/api/v1/auth/oidc/callback`。为空：不发送，提供商使用为该客户端注册的地址。 |
| `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` | 空（全部） | 允许登录的电子邮件域名，逗号分隔。 |

设置提供商：[单点登录 (OIDC)](../oidc.md)。
