# Accounts and sign-in

Session lifetimes, the signing key, and single sign-on.

## Sessions

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_JWT_PRIVATE_KEY_PATH` | `/data/jwt.key` | The key that signs every session, generated on the first start. Losing it signs everyone out. |
| `EVEY_JWT_ACCESS_TTL` | `15m` | Lifetime of an access token. Apps renew it silently. |
| `EVEY_JWT_REFRESH_TTL` | `168h` (7 days) | Lifetime of a sign-in session: without using an app for that long, you sign in again. |
| `EVEY_JWT_REFRESH_LONG_TTL` | `4320h` (180 days) | Lifetime of a session an app asks to keep, such as a TV signed in with Quick Connect. |

## Single sign-on (OIDC)

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_OIDC_ENABLED` | `false` | Turns single sign-on on. Restart after changing it. |
| `EVEY_OIDC_ISSUER` | empty | The provider's issuer URL, for example `https://auth.example.com/realms/main`. |
| `EVEY_OIDC_CLIENT_ID` | empty | The client registered for Evey at the provider. |
| `EVEY_OIDC_CLIENT_SECRET` | empty | Its secret. |
| `EVEY_OIDC_REDIRECT_URL` | empty | The callback address sent to the provider: `https://<your address>/api/v1/auth/oidc/callback`. Empty: none is sent, and the provider uses the one registered for the client. |
| `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` | empty (all) | E-mail domains allowed to sign in, comma-separated. |

Setting up a provider: [Single sign-on (OIDC)](../oidc.md).

<!-- sources
internal/config/config.go - EVEY_JWT_*, EVEY_OIDC_*
long ttl: internal/adapters/driving/grpcapi/user_handler.go - refreshTTLOverride ; internal/adapters/driving/grpcapi/interceptors/client_info.go (x-persistent-session)
redirect: internal/app/user/oidc_service.go (oauth2 RedirectURL as given)
-->
