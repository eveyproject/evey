# OIDC / SSO Setup

Evey supports Single Sign-On via OpenID Connect. Users can log in with Google, Keycloak, Authentik, Authelia, or any OIDC-compliant provider.

## Configuration

```env title=".env"
EVEY_OIDC_ENABLED=true
EVEY_OIDC_ISSUER=https://auth.example.com/realms/main
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=your-client-secret
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=example.com,company.org
```

## Provider Setup

### 1. Register Evey as an OIDC client

In your identity provider, create a new client/application:

| Field | Value |
|-------|-------|
| Client ID | `evey` |
| Client Secret | generate one |
| Redirect URI | `https://<your address>/api/v1/auth/oidc/callback` |
| Grant Type | Authorization Code |
| Scopes | `openid email profile` |

### 2. Configure Evey

Set the environment variables above in your `.env`. The `EVEY_OIDC_ISSUER` must be the base URL of your OIDC provider's discovery endpoint (without `/.well-known/openid-configuration`).

### 3. Restart

```bash
docker compose restart evey
```

The login page will show a "Sign in with SSO" button.

## How It Works

1. User clicks "Sign in with SSO" on the login page
2. Evey redirects to the OIDC provider
3. User authenticates with the provider
4. Provider redirects back to Evey with an authorization code
5. Evey exchanges the code for tokens and creates/updates the user account

## Domain Restriction

`EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` limits which email domains can register. Leave empty to allow all domains.

## Auto User Creation

Users who log in via OIDC for the first time are automatically created with:
- Username derived from `preferred_username` or email prefix
- Email from the OIDC token
- Non-admin role
- A default profile

OIDC users cannot set a local password - they always authenticate through the provider.
