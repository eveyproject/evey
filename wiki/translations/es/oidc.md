---
source_sha: 65b23365db26b7098f71c51fcbae29e81a9f718d1b2be22250829cabd85987c1
reviewed: false
---
# Configuración de OIDC / SSO

Evey admite el inicio de sesión único mediante OpenID Connect: Keycloak,
Authentik, Authelia, Google o cualquier otro proveedor OIDC. La página de inicio
de sesión de las aplicaciones web y Android muestra entonces **Iniciar sesión
con SSO** junto al formulario de contraseña.

## 1. Registrar Evey en el proveedor

Crea un cliente (o aplicación) en el proveedor:

| Campo | Valor |
|-------|-------|
| Client ID | `evey`, o cualquier nombre |
| Secreto del cliente | genera uno (obligatorio) |
| URI de redirección | `https://<tu dirección>/api/v1/auth/oidc/callback` |
| URI de redirección tras cerrar sesión | `https://<tu dirección>/login` |
| Tipo de concesión | Authorization Code |
| Scopes | `openid email profile` |

## 2. Configurar Evey

```env title=".env"
EVEY_OIDC_ENABLED=true
EVEY_OIDC_ISSUER=https://auth.example.com/realms/main
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=your-client-secret
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=example.com,company.org
```

`EVEY_OIDC_ISSUER` es la URL base del proveedor, sin
`/.well-known/openid-configuration`. La lista completa, incluida
`EVEY_OIDC_REDIRECT_URL`, está en [Cuentas e inicio de sesión](environment/accounts.md).

## 3. Reiniciar

```bash
docker compose up -d
```

El servidor contacta con el proveedor al arrancar. Si el emisor no responde, o
el secreto del cliente está vacío, **el servidor no arranca**: los registros
indican la causa.

## Quién puede iniciar sesión

El primer inicio de sesión SSO crea una cuenta de Evey, sin contraseña y con un
perfil. Su nombre de usuario es el `preferred_username` del proveedor o, si
falta, la parte del correo anterior a `@`. La cuenta es un usuario normal y,
como toda cuenta sin restricciones, ve todas las bibliotecas: restríngela
después en **Admin → Usuarios** (ver [Usuarios](administration/users.md)).

**Limita los registros con `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS`.** Si está vacía,
cualquiera que tenga una cuenta en el proveedor obtiene una en tu servidor, lo
que con un proveedor público como Google significa cualquiera. Con dominios,
solo se aceptan las direcciones de esos dominios, y el proveedor debe marcar la dirección como
verificada.

Una cuenta se reconoce por el proveedor y su identificador de usuario, no por el
correo. Una persona que ya tiene una cuenta local con el mismo correo o el mismo
nombre de usuario no puede crear una cuenta SSO: el inicio de sesión falla con
«Error de inicio de sesión con SSO». Cambia la dirección o el nombre de usuario
en el proveedor, o elimina antes la cuenta local.

Una cuenta desactivada se rechaza al iniciar sesión, como siempre.

## Cerrar sesión

Cuando el proveedor publica un punto de cierre de sesión, cerrar sesión en Evey
también cierra la sesión en el proveedor y luego vuelve a la página de inicio de
sesión de Evey.
