---
source_sha: 49645a52c7506bc78947f31b763d5b78de15eaf577542b073330d83230b45467
reviewed: false
---
# Konten und Anmeldung

Sitzungsdauern, der Signaturschlüssel und Single Sign-on.

## Sitzungen

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_JWT_PRIVATE_KEY_PATH` | `/data/jwt.key` | Der Schlüssel, der die Zugriffstoken signiert, beim ersten Start erzeugt. Geht er verloren, erzeugt der nächste Start einen neuen und die Apps erneuern ihre Token: Niemand wird abgemeldet. |
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
| `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` | leer (alle) | E-Mail-Domains, die bei der ersten Anmeldung ein Konto anlegen dürfen, kommagetrennt. Der Anbieter muss die Adresse verifiziert haben. |

Einen Anbieter einrichten: [Single Sign-on (OIDC)](../oidc.md).
