---
source_sha: e3a9611908da29d559070f9745961b1f31fad6154f252e1304481c5e371f2c0c
reviewed: true
---
# Konten und Anmeldung

Die Variablen hinter den Sitzungen und dem Single Sign-on: wie lange eine
Anmeldung gilt, wo der Signaturschlüssel liegt, und der OIDC-Anbieter. Single
Sign-on wird normalerweise im Verwaltungsbereich eingerichtet, wie unter
[Single Sign-on (OIDC)](../oidc.md) beschrieben. Die Variablen hier sind
der Ersatz, wenn dort nichts eingetragen ist.

## Sitzungen

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_JWT_PRIVATE_KEY_PATH` | `/data/jwt.key` | Der Schlüssel, der die Zugriffstoken signiert, beim ersten Start erzeugt. Geht er verloren, erzeugt der nächste Start einen neuen, und die Apps erneuern ihre Token: Niemand wird abgemeldet. | keine |
| `EVEY_JWT_ACCESS_TTL` | `15m` | Gültigkeitsdauer eines Zugriffstokens. Die Apps erneuern es unbemerkt. | keine |
| `EVEY_JWT_REFRESH_TTL` | `168h` (7 Tage) | Gültigkeitsdauer einer Anmeldesitzung: Wer eine App so lange nicht benutzt, meldet sich neu an. | keine |
| `EVEY_JWT_REFRESH_LONG_TTL` | `4320h` (180 Tage) | Gültigkeitsdauer einer Sitzung, die eine App behalten möchte, etwa ein Fernseher, der mit der Schnellverbindung angemeldet wurde. | keine |

## Single Sign-on (OIDC)

Jedes Feld, das in der Karte **Single Sign-on (SSO)** unter **Verwaltung →
Serverkonfiguration** gespeichert wird, ersetzt seine Variable, ohne Neustart.

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_OIDC_ENABLED` | `false` | Schaltet Single Sign-on ein. | Serverkonfiguration → Single Sign-on (SSO), sofort |
| `EVEY_OIDC_ISSUER` | leer | Die Issuer-URL des Anbieters, zum Beispiel `https://auth.example.com/realms/main`. | dieselbe Karte, sofort |
| `EVEY_OIDC_CLIENT_ID` | leer | Der Client, der beim Anbieter für Evey registriert ist. | dieselbe Karte, sofort |
| `EVEY_OIDC_CLIENT_SECRET` | leer | Sein Geheimnis. Ist Single Sign-on ohne es eingeschaltet, warnt der Server beim Start. | dieselbe Karte, sofort |
| `EVEY_OIDC_REDIRECT_URL` | leer | Die Rückrufadresse, die an den Anbieter geschickt wird: `https://<your address>/api/v1/auth/oidc/callback`. Leer: wird aus der öffentlichen Adresse des Servers gebildet (**Verwaltung → Fernzugriff**). | keine |
| `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` | leer (alle) | E-Mail-Domains, die sich anmelden dürfen, durch Kommas getrennt. Der Anbieter muss die Adresse bestätigt haben. | dieselbe Karte, sofort |
| `EVEY_OIDC_ALLOW_SIGNUP` | leer | `true` oder `false`: Konten für neue SSO-Benutzer anlegen. Leer: nur eingeschaltet, wenn E-Mail-Domains gesetzt sind. | dieselbe Karte, sofort |

<!-- sources
internal/config/config.go - EVEY_JWT_*, EVEY_OIDC_* ; WarnValidate (OIDC enabled without secret)
key regeneration: cmd/evey/wire_infra.go - auth.LoadOrGenerateRSAKey ; refresh tokens in DB: internal/adapters/driven/postgres/refresh_token_repo.go
long ttl: internal/adapters/driving/grpcapi/user_handler.go - refreshTTLOverride ; internal/adapters/driving/grpcapi/interceptors/client_info.go (x-persistent-session)
redirect: internal/app/user/oidc_service.go (oauth2 RedirectURL as given) ; panel over env: internal/app/user/oidc_settings.go ResolveOIDCConfig (OIDCKey*)
panel: i18n admin.sso.title
-->
