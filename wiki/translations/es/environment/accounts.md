---
source_sha: e3a9611908da29d559070f9745961b1f31fad6154f252e1304481c5e371f2c0c
reviewed: true
---
# Cuentas e inicio de sesión

Las variables que hay detrás de las sesiones y del inicio de sesión único:
cuánto dura un inicio de sesión, dónde está la clave de firma y cuál es el
proveedor OIDC. El inicio de sesión único se configura normalmente en el
panel, como se describe en [Inicio de sesión único (OIDC)](../oidc.md); las
variables de aquí son su valor de respaldo.

## Sesiones

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_JWT_PRIVATE_KEY_PATH` | `/data/jwt.key` | La clave que firma los tokens de acceso, generada en el primer arranque. Si se pierde, el siguiente arranque genera una nueva y las aplicaciones renuevan sus tokens: nadie pierde la sesión. | ninguno |
| `EVEY_JWT_ACCESS_TTL` | `15m` | Duración de un token de acceso. Las aplicaciones lo renuevan sin que lo notes. | ninguno |
| `EVEY_JWT_REFRESH_TTL` | `168h` (7 días) | Duración de una sesión: si no usas una aplicación durante ese tiempo, vuelves a iniciar sesión. | ninguno |
| `EVEY_JWT_REFRESH_LONG_TTL` | `4320h` (180 días) | Duración de una sesión que una aplicación pide conservar, como una TV conectada con Quick Connect. | ninguno |

## Inicio de sesión único (OIDC)

Cada campo guardado en la tarjeta **Inicio de sesión único (SSO)** de
**Administración → Configuración del servidor** sustituye a su variable, sin
reiniciar.

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_OIDC_ENABLED` | `false` | Activa el inicio de sesión único. | Configuración del servidor → Inicio de sesión único (SSO), inmediato |
| `EVEY_OIDC_ISSUER` | vacío | La URL del emisor del proveedor, por ejemplo `https://auth.example.com/realms/main`. | misma tarjeta, inmediato |
| `EVEY_OIDC_CLIENT_ID` | vacío | El cliente registrado para Evey en el proveedor. | misma tarjeta, inmediato |
| `EVEY_OIDC_CLIENT_SECRET` | vacío | Su secreto. Si se activa sin él, el servidor avisa al arrancar. | misma tarjeta, inmediato |
| `EVEY_OIDC_REDIRECT_URL` | vacío | La dirección de retorno que se envía al proveedor: `https://<your address>/api/v1/auth/oidc/callback`. Vacía: se construye a partir de la dirección pública del servidor (**Administración → Acceso remoto**). | ninguno |
| `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` | vacío (todos) | Los dominios de correo que pueden iniciar sesión, separados por comas. El proveedor tiene que haber verificado la dirección. | misma tarjeta, inmediato |
| `EVEY_OIDC_ALLOW_SIGNUP` | vacío | `true` o `false`: crear cuentas para los nuevos usuarios de SSO. Vacío: activo solo cuando hay dominios de correo definidos. | misma tarjeta, inmediato |

<!-- sources
internal/config/config.go - EVEY_JWT_*, EVEY_OIDC_* ; WarnValidate (OIDC enabled without secret)
key regeneration: cmd/evey/wire_infra.go - auth.LoadOrGenerateRSAKey ; refresh tokens in DB: internal/adapters/driven/postgres/refresh_token_repo.go
long ttl: internal/adapters/driving/grpcapi/user_handler.go - refreshTTLOverride ; internal/adapters/driving/grpcapi/interceptors/client_info.go (x-persistent-session)
redirect: internal/app/user/oidc_service.go (oauth2 RedirectURL as given) ; panel over env: internal/app/user/oidc_settings.go ResolveOIDCConfig (OIDCKey*)
panel: i18n admin.sso.title
-->
