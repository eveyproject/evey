# OIDC / SSO Setup

Evey supports Single Sign-On via OpenID Connect: Keycloak, Authentik, Authelia,
Google or any other OIDC provider. The sign-in page of the web and Android apps
then shows **Sign in with SSO** next to the password form.

## 1. Register Evey with the provider

Create a client (or application) in the provider:

| Field | Value |
|-------|-------|
| Client ID | `evey`, or any name |
| Client secret | generate one (required) |
| Redirect URI | `https://<your address>/api/v1/auth/oidc/callback` |
| Post-logout redirect URI | `https://<your address>/login` |
| Grant type | Authorization Code |
| Scopes | `openid email profile` |

## 2. Configure Evey

```env title=".env"
EVEY_OIDC_ENABLED=true
EVEY_OIDC_ISSUER=https://auth.example.com/realms/main
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=your-client-secret
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=example.com,company.org
```

`EVEY_OIDC_ISSUER` is the provider's base URL, without
`/.well-known/openid-configuration`. The full list, including
`EVEY_OIDC_REDIRECT_URL`, is in [Accounts and sign-in](environment/accounts.md).

## 3. Restart

```bash
docker compose up -d
```

The server contacts the provider when it starts. If the issuer cannot be
reached, or the client secret is empty, **the server does not start**: the logs
name the cause.

## Who can sign in

The first SSO sign-in creates an Evey account, with no password and one
profile. Its username is the provider's `preferred_username`, otherwise the part
of the e-mail before `@`. The account is a regular user and, like any account
without restrictions, sees every library: narrow it in **Admin → Users**
afterwards (see [Users](administration/users.md)).

**Restrict sign-ups with `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS`.** Empty, anyone who
holds an account at the provider gets one on your server, which with a public
provider such as Google means anyone. With domains set, only those addresses are
accepted, and the provider must mark the address as verified.

An account is recognised by the provider and its user ID, not by e-mail. A
person who already has a local account with the same e-mail or username cannot
create an SSO account: the sign-in fails with "SSO sign-in failed". Change the
address or the username at the provider, or remove the local account first.

A disabled account is refused at sign-in as usual.

## Signing out

When the provider publishes a logout endpoint, signing out of Evey also ends
the session at the provider, then returns to the Evey sign-in page.

<!-- sources
config: internal/config/config.go OIDCConfig (EVEY_OIDC_*) ; boot: cmd/evey/wire.go "oidc init" returns error ; NewOIDCService (empty secret, discovery)
routes: internal/adapters/driving/http/auth_handler.go (/api/v1/auth/oidc/login, /callback, failure -> /login?oidc_error=1)
scopes openid profile email: internal/app/user/oidc_service.go:104
account creation: oidc_service.go findOrCreateUser (GetByOIDCSubject by issuer+sub, allowlist + email_verified, username preferred_username > email prefix > oidc-<hex>, default profile)
unique email (non-empty) + username (lower) measured in dev pg_indexes 01-10: idx_users_email_unique, idx_users_username_lower
zero library grants = every library: internal/adapters/driving/grpcapi/library_handler.go getAllowedLibraryIDs
logout: internal/adapters/driving/http/server.go POST /api/v1/auth/logout (end_session_endpoint, post_logout_redirect_uri = base/login)
UI: i18n auth.login.signInWithSSO / ssoFailed ; mobile login.tsx ; no SSO on the TV app
-->
