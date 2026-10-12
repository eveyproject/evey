---
source_sha: 5f2dd5bf7c21906e243c0cece2ecff83e306d69d5b99a193541b9e574db7a4c2
reviewed: true
---
# 单点登录（OIDC）

让用户通过您已经在运行的身份提供方登录（Keycloak、Authentik、Authelia、Google，或任何其他 OpenID Connect 提供方）：这样，网页版和 Android 应用的登录页上，密码表单旁边会出现**使用 SSO 登录**。管理员要做两步：先在提供方那里登记 Evey，再在管理面板里填写一张卡片。

## 1. 在提供方那里登记 Evey

在提供方里创建一个客户端（或应用），设置为：授权码（Authorization Code）授权方式，范围 `openid email profile`，一个客户端密钥，以及**单点登录 (SSO)** 卡片上**在提供方处登记以下地址**下面显示的两个地址：

| 栏位 | 值 |
| --- | --- |
| **重定向 URI** | `https://<your address>/api/v1/auth/oidc/callback` |
| **退出后重定向 URI** | `https://<your address>/login` |

这两个地址都由服务器的公网地址（**管理 → 远程访问**）生成：如果用户通过另一个地址访问 Evey，请先设置好它。

## 2. 配置 Evey

**管理 → 服务器配置 → 单点登录 (SSO)**：

| 栏位 | 作用 |
| --- | --- |
| **启用单点登录** | 在登录界面上显示 SSO 按钮。 |
| **颁发者 (Issuer) URL** | 提供方的地址；Evey 会读取它下面的 `/.well-known/openid-configuration`。 |
| **客户端 ID**、**客户端密钥** | 上面登记的客户端。密钥保存在服务器上，之后不会再显示；保留那几个圆点就表示沿用原来的密钥。 |
| **允许的电子邮件域名** | 用逗号分隔。设置后，只接受这些域名的地址，而且提供方必须确认过这个地址。 |
| **按钮文字** | 可选，用来替换“使用 SSO 登录”。 |
| **通过已验证的电子邮件关联现有账户** | 默认开启。见[谁可以登录](#who-can-sign-in)。 |
| **为新的 SSO 用户创建账户** | 默认关闭，除非设置了允许的电子邮件域名。见[谁可以登录](#who-can-sign-in)。 |
| **新 SSO 账户的媒体库访问权限** | **所有媒体库**，或**仅限所选媒体库**。 |

**测试连接**检查颁发者是否有应答。**保存 SSO 设置**会立即生效，无需重启。**状态**这一行说明能否连接到提供方；无法连接时，会附上最后一次的错误。

[账户与登录](environment/accounts.md#single-sign-on-oidc)中的 `EVEY_OIDC_*` 变量是后备方案：如果卡片里的值来自这些变量，卡片会注明；保存卡片后，就以卡片为准。

!!! note "服务器的运行从不依赖提供方"
    即使提供方宕机，服务器也照常启动；无法连接到提供方时，SSO 按钮保持隐藏，密码登录照常可用。下次有人尝试登录时会再联系提供方，最多每 30 秒一次。

## 谁可以登录

1. 已经用这个提供方登录过的账户可以再次登录，除非它被禁用了。
2. 否则，如果开启了**通过已验证的电子邮件关联现有账户**，电子邮件相同的现有账户会与提供方关联并登录，但前提是提供方确认过这个地址。该账户保留原来的密码。管理员账户永远不会以这种方式关联。
3. 否则，如果开启了**为新的 SSO 用户创建账户**，就会创建一个新账户，带一个个人资料，媒体库访问权限按卡片里的选择。用户名取提供方的 `preferred_username`；没有的话，取电子邮件地址里 `@` 前面的部分；如果这个名字已被占用，会加上一个简短的后缀。

!!! warning "为什么默认不创建账户"
    否则，在使用 Google 这样的公共提供方时，任何在那里有账户的人都能在您的服务器上得到一个账户。只有当**允许的电子邮件域名**限制了哪些人符合条件时，才默认开启创建账户。关闭时，只有现有账户可以用 SSO 登录。

登录被拒绝时，登录页会说明原因：电子邮件域名不被允许、没有匹配的账户且注册已关闭、账户已被禁用、账户存在但无法关联，或者提供方不可用。

## 退出登录

如果提供方公布了退出端点，那么通过 SSO 登录后退出 Evey 时，也会结束在提供方那里的会话，然后回到 Evey 的登录页。卡片会说明提供方是否支持这一点。

<!-- sources
settings and precedence (panel keys oidc.* win over EVEY_OIDC_*): internal/app/user/oidc_settings.go - ResolveOIDCConfig, AllowSignup default from AllowedEmailDomains
lazy discovery, 30 s backoff, never at boot: internal/app/user/oidc_service.go - oidcRetryBackoff ; cmd/evey/wire.go
account resolution (identity, verified-email link, sign-up, suffix, default libraries, admin never linked): oidc_service.go findOrCreateUser ; tests oidc_policy_test.go
admin routes GET/PUT /api/v1/admin/sso, POST /api/v1/admin/sso/test: internal/adapters/driving/http/sso_admin_handler.go
login error reasons: internal/adapters/driving/http/auth_handler.go (oidc_error=<reason>) ; i18n auth.login.ssoError.*
logout endSessionUrl: internal/adapters/driving/http/server.go
mobile flow: internal/adapters/driving/http/oidc_app_flow.go ; frontend/apps/mobile/app/oidc-callback.tsx
UI: frontend/apps/web/src/features/admin/serverConfig/SsoSettingsCard.tsx ; i18n admin.sso.* ; auth.login.signInWithSSO
-->
