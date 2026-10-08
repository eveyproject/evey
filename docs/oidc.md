# Single sign-on (OIDC)

Let people sign in with the identity provider you already run (Keycloak,
Authentik, Authelia, Google or any other OpenID Connect provider): the sign-in
page of the web and Android apps then shows **Sign in with SSO** next to the
password form. Two steps for the administrator: register Evey at the provider,
then fill one card in the admin panel.

## 1. Register Evey with the provider

Create a client (or application) in the provider with the Authorization Code
grant, the scopes `openid email profile`, a client secret, and the two
addresses the **Single sign-on (SSO)** card shows under **Register these
addresses at your provider**:

| Field | Value |
| --- | --- |
| **Redirect URI** | `https://<your address>/api/v1/auth/oidc/callback` |
| **Post-logout redirect URI** | `https://<your address>/login` |

Both are built from the server's public address (**Admin → Remote Access**):
set that first if people reach Evey under another address.

## 2. Configure Evey

**Admin → Server Config → Single sign-on (SSO)**:

| Field | Effect |
| --- | --- |
| **Enable single sign-on** | Shows the SSO button on the sign-in screens. |
| **Issuer URL** | The provider's address; Evey reads `/.well-known/openid-configuration` under it. |
| **Client ID**, **Client secret** | The client registered above. The secret is kept on the server and never shown again; leaving the dots keeps it. |
| **Allowed e-mail domains** | Comma separated. When set, only these addresses are accepted, and the provider must confirm the address. |
| **Button label** | Optional, replaces "Sign in with SSO". |
| **Link to existing accounts by verified e-mail** | On by default. See [Who can sign in](#who-can-sign-in). |
| **Create accounts for new SSO users** | Off unless e-mail domains are allowed. See [Who can sign in](#who-can-sign-in). |
| **Library access for new SSO accounts** | **All libraries**, or **Only the selected libraries**. |

**Test connection** checks that the issuer answers. **Save SSO settings**
applies at once, without a restart. The **Status** line says whether the
provider is reachable, with the last error if it is not.

The `EVEY_OIDC_*` variables of
[Accounts and sign-in](environment/accounts.md#single-sign-on-oidc) are the
fallback: the card says when its values come from them, and saving the card
takes over.

!!! note "The provider is never on the server's critical path"
    The server starts even when the provider is down, the SSO button stays
    hidden while the provider cannot be reached, and password sign-in keeps
    working. The provider is contacted again on the next sign-in attempt, at
    most every 30 seconds.

## Who can sign in

1. An account that already signed in with this provider signs in again,
   unless it is disabled.
2. Otherwise, with **Link to existing accounts by verified e-mail** on, an
   existing account with the same e-mail is linked to the provider and signed
   in, but only when the provider confirms the address. The account keeps its
   password. Administrator accounts are never linked this way.
3. Otherwise, with **Create accounts for new SSO users** on, a new account is
   created, with one profile and the library access chosen in the card. Its
   username is the provider's `preferred_username`, otherwise the part of the
   e-mail before `@`; a name already taken gets a short suffix.

!!! warning "Why account creation is off by default"
    With a public provider such as Google, anyone holding an account there
    could otherwise get one on your server. Creation is on by default only
    when **Allowed e-mail domains** restricts who qualifies. When it is off,
    only existing accounts can sign in with SSO.

When a sign-in is refused, the sign-in page says why: the e-mail domain is not
allowed, no account matches and sign-up is off, the account is disabled, the
account exists but could not be linked, or the provider is unavailable.

## Signing out

When the provider publishes a logout endpoint, signing out of Evey after an SSO
sign-in also ends the session at the provider, then returns to the Evey sign-in
page. The card says whether the provider supports it.

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
