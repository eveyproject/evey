# Accounts and sign-in

The variables behind sessions and single sign-on: how long a sign-in lasts,
where the signing key lives, and the OIDC provider. Single sign-on is
normally set up in the panel, as described in
[Single sign-on (OIDC)](../oidc.md); the variables here are its fallback.

## Sessions

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_JWT_PRIVATE_KEY_PATH` | `/data/jwt.key` | The key that signs access tokens, generated on the first start. If it is lost, the next start generates a new one and the apps renew their tokens: nobody is signed out. | none |
| `EVEY_JWT_ACCESS_TTL` | `15m` | Lifetime of an access token. Apps renew it silently. | none |
| `EVEY_JWT_REFRESH_TTL` | `168h` (7 days) | Lifetime of a sign-in session: without using an app for that long, you sign in again. | none |
| `EVEY_JWT_REFRESH_LONG_TTL` | `4320h` (180 days) | Lifetime of a session an app asks to keep, such as a TV signed in with Quick Connect. | none |

## Single sign-on (OIDC)

Each field saved in the **Single sign-on (SSO)** card of **Admin → Server
Config** replaces its variable, without a restart.

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_OIDC_ENABLED` | `false` | Turns single sign-on on. | Server Config → Single sign-on (SSO), live |
| `EVEY_OIDC_ISSUER` | empty | The provider's issuer URL, for example `https://auth.example.com/realms/main`. | same card, live |
| `EVEY_OIDC_CLIENT_ID` | empty | The client registered for Evey at the provider. | same card, live |
| `EVEY_OIDC_CLIENT_SECRET` | empty | Its secret. Enabled without it, the server warns at startup. | same card, live |
| `EVEY_OIDC_REDIRECT_URL` | empty | The callback address sent to the provider: `https://<your address>/api/v1/auth/oidc/callback`. Empty: built from the server's public address (**Admin → Remote Access**). | none |
| `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` | empty (all) | E-mail domains allowed to sign in, comma-separated. The provider must have verified the address. | same card, live |
| `EVEY_OIDC_ALLOW_SIGNUP` | empty | `true` or `false`: create accounts for new SSO users. Empty: on only when e-mail domains are set. | same card, live |

<!-- sources
internal/config/config.go - EVEY_JWT_*, EVEY_OIDC_* ; WarnValidate (OIDC enabled without secret)
key regeneration: cmd/evey/wire_infra.go - auth.LoadOrGenerateRSAKey ; refresh tokens in DB: internal/adapters/driven/postgres/refresh_token_repo.go
long ttl: internal/adapters/driving/grpcapi/user_handler.go - refreshTTLOverride ; internal/adapters/driving/grpcapi/interceptors/client_info.go (x-persistent-session)
redirect: internal/app/user/oidc_service.go (oauth2 RedirectURL as given) ; panel over env: internal/app/user/oidc_settings.go ResolveOIDCConfig (OIDCKey*)
panel: i18n admin.sso.title
-->
