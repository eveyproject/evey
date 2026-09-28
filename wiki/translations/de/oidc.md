---
source_sha: f5903cb3e9c912275de11f7f8719c97fd054491f4dfab32be7fdaa643ea1c396
---
# OIDC / SSO einrichten

Evey beherrscht Single Sign-On über OpenID Connect. Ihre Benutzer melden sich
damit über Google, Keycloak, Authentik, Authelia oder jeden anderen Anbieter an,
der OIDC unterstützt.

## Konfiguration

```env title=".env"
EVEY_OIDC_ENABLED=true
EVEY_OIDC_ISSUER=https://auth.example.com/realms/main
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=your-client-secret
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=example.com,company.org
```

## Den Anbieter einrichten

### 1. Evey als OIDC-Client registrieren

Legen Sie bei Ihrem Identitätsanbieter einen neuen Client beziehungsweise eine
neue Anwendung an:

| Feld | Wert |
|-------|-------|
| Client ID | `evey` |
| Client Secret | eines erzeugen |
| Redirect URI | `{EVEY_SERVER_BASE_URL}/api/v1/auth/oidc/callback` |
| Grant Type | Authorization Code |
| Scopes | `openid email profile` |

### 2. Evey konfigurieren

Tragen Sie die obigen Umgebungsvariablen in Ihre `.env` ein. `EVEY_OIDC_ISSUER`
muss die Basis-URL des Discovery-Endpunkts Ihres OIDC-Anbieters sein, also ohne
das angehängte `/.well-known/openid-configuration`.

### 3. Neu starten

```bash
docker compose restart evey
```

Auf der Anmeldeseite erscheint daraufhin die Schaltfläche „Mit SSO anmelden“.

## Wie das abläuft

1. Der Benutzer klickt auf der Anmeldeseite auf „Mit SSO anmelden“
2. Evey leitet ihn zum OIDC-Anbieter weiter
3. Der Benutzer meldet sich beim Anbieter an
4. Der Anbieter leitet ihn mit einem Autorisierungscode zurück zu Evey
5. Evey tauscht den Code gegen Tokens ein und legt das Benutzerkonto an oder
   aktualisiert es

## Beschränkung auf Domains

`EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` legt fest, aus welchen E-Mail-Domains sich
jemand registrieren darf. Lassen Sie den Wert leer, sind alle Domains erlaubt.

## Konten automatisch anlegen

Wer sich zum ersten Mal über OIDC anmeldet, bekommt automatisch ein Konto, und
zwar mit:
- einem Benutzernamen aus `preferred_username` oder dem Teil der E-Mail-Adresse vor dem `@`
- der E-Mail-Adresse aus dem OIDC-Token
- einer Rolle ohne Administratorrechte
- einem Standardprofil

Über OIDC angelegte Benutzer können kein lokales Passwort setzen - sie melden
sich immer über den Anbieter an.
