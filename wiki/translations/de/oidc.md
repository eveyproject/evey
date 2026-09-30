---
source_sha: bba2d9d0a70621281091516a01462cd4178130e57b6654f467dfe62130069342
reviewed: false
---
# OIDC / SSO einrichten

Evey unterstützt Single Sign-on über OpenID Connect. Benutzer können sich mit Google, Keycloak, Authentik, Authelia oder jedem OIDC-konformen Anbieter anmelden.

## Konfiguration

```env title=".env"
EVEY_OIDC_ENABLED=true
EVEY_OIDC_ISSUER=https://auth.example.com/realms/main
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=your-client-secret
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=example.com,company.org
```

## Beim Anbieter einrichten

### 1. Evey als OIDC-Client registrieren

Legen Sie bei Ihrem Identitätsanbieter einen neuen Client bzw. eine neue Anwendung an:

| Feld | Wert |
|-------|-------|
| Client ID | `evey` |
| Client Secret | eines erzeugen |
| Redirect URI | `https://<your address>/api/v1/auth/oidc/callback` |
| Grant Type | Authorization Code |
| Scopes | `openid email profile` |

### 2. Evey konfigurieren

Setzen Sie die Umgebungsvariablen oben in Ihrer `.env`. `EVEY_OIDC_ISSUER` muss die Basis-URL des Discovery-Endpunkts Ihres OIDC-Anbieters sein (ohne `/.well-known/openid-configuration`).

### 3. Neu starten

```bash
docker compose restart evey
```

Die Anmeldeseite zeigt dann eine Schaltfläche „Mit SSO anmelden".

## So funktioniert es

1. Der Benutzer klickt auf der Anmeldeseite auf „Mit SSO anmelden"
2. Evey leitet zum OIDC-Anbieter weiter
3. Der Benutzer authentifiziert sich beim Anbieter
4. Der Anbieter leitet mit einem Autorisierungscode zu Evey zurück
5. Evey tauscht den Code gegen Tokens und legt das Benutzerkonto an oder aktualisiert es

## Domain-Einschränkung

`EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` begrenzt, welche E-Mail-Domains sich registrieren können. Leer lassen, um alle Domains zuzulassen.

## Automatische Benutzeranlage

Benutzer, die sich zum ersten Mal über OIDC anmelden, werden automatisch angelegt mit:
- einem Benutzernamen aus `preferred_username` oder dem Präfix der E-Mail-Adresse
- der E-Mail-Adresse aus dem OIDC-Token
- der Rolle Nicht-Administrator
- einem Standardprofil

OIDC-Benutzer können kein lokales Passwort festlegen - sie authentifizieren sich immer über den Anbieter.
