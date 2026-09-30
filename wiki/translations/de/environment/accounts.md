---
source_sha: 24e34e0b33eb8acd096fe2be879ca8bda28a01e6cbf963847e6dd4b661827adb
reviewed: false
---
# Konten und Anmeldung

Sitzungsdauern, der Signaturschlüssel und Single Sign-on.

## Sitzungen

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_JWT_PRIVATE_KEY_PATH` | `/data/jwt.key` | Der Schlüssel, der jede Sitzung signiert, beim ersten Start erzeugt. Geht er verloren, sind alle abgemeldet. |
| `EVEY_JWT_ACCESS_TTL` | `15m` | Lebensdauer eines Zugriffstokens. Die Apps erneuern es unbemerkt. |
| `EVEY_JWT_REFRESH_TTL` | `168h` (7 Tage) | Lebensdauer einer Anmeldesitzung: Nutzt man eine App so lange nicht, meldet man sich erneut an. |
| `EVEY_JWT_REFRESH_LONG_TTL` | `4320h` (180 Tage) | Lebensdauer einer Sitzung, die eine App behalten möchte, etwa ein Fernseher, der per Schnellverbindung angemeldet wurde. |

## Single Sign-on (OIDC)

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_OIDC_ENABLED` | `false` | Schaltet Single Sign-on ein. Nach einer Änderung neu starten. |
| `EVEY_OIDC_ISSUER` | leer | Die Issuer-URL des Anbieters, zum Beispiel `https://auth.example.com/realms/main`. |
| `EVEY_OIDC_CLIENT_ID` | leer | Der beim Anbieter für Evey registrierte Client. |
| `EVEY_OIDC_CLIENT_SECRET` | leer | Sein Geheimnis. |
| `EVEY_OIDC_REDIRECT_URL` | leer | Die an den Anbieter gesendete Rückrufadresse: `https://<your address>/api/v1/auth/oidc/callback`. Leer: Es wird keine gesendet, und der Anbieter nutzt die für den Client registrierte. |
| `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` | leer (alle) | E-Mail-Domains, die sich anmelden dürfen, kommagetrennt. |

Einen Anbieter einrichten: [Single Sign-on (OIDC)](../oidc.md).
