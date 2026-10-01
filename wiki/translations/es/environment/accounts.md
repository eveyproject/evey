---
source_sha: 49645a52c7506bc78947f31b763d5b78de15eaf577542b073330d83230b45467
reviewed: false
---
# Cuentas e inicio de sesión

Duración de las sesiones, la clave de firma y el inicio de sesión único.

## Sesiones

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_JWT_PRIVATE_KEY_PATH` | `/data/jwt.key` | La clave que firma los tokens de acceso, generada en el primer arranque. Si se pierde, el siguiente arranque genera una nueva y las aplicaciones renuevan sus tokens: nadie pierde la sesión. |
| `EVEY_JWT_ACCESS_TTL` | `15m` | Duración de un token de acceso. Las aplicaciones lo renuevan sin que se note. |
| `EVEY_JWT_REFRESH_TTL` | `168h` (7 días) | Duración de una sesión iniciada: si no usas una aplicación durante ese tiempo, vuelves a iniciar sesión. |
| `EVEY_JWT_REFRESH_LONG_TTL` | `4320h` (180 días) | Duración de una sesión que una aplicación pide conservar, como un televisor conectado con Conexión rápida. |

## Inicio de sesión único (OIDC)

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_OIDC_ENABLED` | `false` | Activa el inicio de sesión único. Reinicia tras cambiarla. |
| `EVEY_OIDC_ISSUER` | vacía | La URL del emisor del proveedor, por ejemplo `https://auth.example.com/realms/main`. |
| `EVEY_OIDC_CLIENT_ID` | vacía | El cliente registrado para Evey en el proveedor. |
| `EVEY_OIDC_CLIENT_SECRET` | vacía | Su secreto. |
| `EVEY_OIDC_REDIRECT_URL` | vacía | La dirección de retorno enviada al proveedor: `https://<tu dirección>/api/v1/auth/oidc/callback`. Vacía: no se envía ninguna, y el proveedor usa la registrada para el cliente. |
| `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` | vacía (todos) | Dominios de correo autorizados a crear una cuenta en su primer inicio de sesión, separados por comas. El proveedor debe haber verificado la dirección. |

Configurar un proveedor: [Inicio de sesión único (OIDC)](../oidc.md).
