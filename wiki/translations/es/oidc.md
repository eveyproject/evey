---
source_sha: f5903cb3e9c912275de11f7f8719c97fd054491f4dfab32be7fdaa643ea1c396
---

# Configuración de OIDC / SSO

Evey admite el inicio de sesión único mediante OpenID Connect. Sus usuarios pueden entrar con Google, Keycloak, Authentik, Authelia o cualquier otro proveedor compatible con OIDC.

## Configuración

```env title=".env"
EVEY_OIDC_ENABLED=true
EVEY_OIDC_ISSUER=https://auth.example.com/realms/main
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=your-client-secret
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=example.com,company.org
```

## Configuración del proveedor

### 1. Registrar Evey como cliente OIDC

En su proveedor de identidad, cree un cliente o aplicación:

| Campo | Valor |
|-------|-------|
| Client ID | `evey` |
| Client Secret | genere uno |
| Redirect URI | `{EVEY_SERVER_BASE_URL}/api/v1/auth/oidc/callback` |
| Grant Type | Authorization Code |
| Scopes | `openid email profile` |

### 2. Configurar Evey

Defina en su `.env` las variables de entorno de más arriba. `EVEY_OIDC_ISSUER` tiene que ser la URL base del extremo de descubrimiento de su proveedor OIDC, sin el `/.well-known/openid-configuration` final.

### 3. Reiniciar

```bash
docker compose restart evey
```

La página de inicio de sesión mostrará entonces el botón «Iniciar sesión con SSO».

## Cómo funciona

1. El usuario pulsa «Iniciar sesión con SSO» en la página de inicio de sesión
2. Evey lo redirige al proveedor OIDC
3. El usuario se autentica ante el proveedor
4. El proveedor lo devuelve a Evey con un código de autorización
5. Evey canjea ese código por unos tokens y crea o actualiza la cuenta de usuario

## Restricción por dominio

`EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` limita qué dominios de correo pueden registrarse. Déjela vacía para admitirlos todos.

## Creación automática de usuarios

Quien entra por OIDC por primera vez obtiene una cuenta creada sobre la marcha, con:
- Un nombre de usuario derivado de `preferred_username` o del prefijo del correo
- El correo electrónico que venga en el token OIDC
- Un rol sin privilegios de administración
- Un perfil por defecto

Los usuarios OIDC no pueden definir una contraseña local: siempre se autentican a través del proveedor.
