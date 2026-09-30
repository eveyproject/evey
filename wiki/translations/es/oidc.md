---
source_sha: bba2d9d0a70621281091516a01462cd4178130e57b6654f467dfe62130069342
reviewed: false
---
# Configuración de OIDC / SSO

Evey admite el inicio de sesión único mediante OpenID Connect. Los usuarios pueden iniciar sesión con Google, Keycloak, Authentik, Authelia o cualquier proveedor compatible con OIDC.

## Configuración

```env title=".env"
EVEY_OIDC_ENABLED=true
EVEY_OIDC_ISSUER=https://auth.example.com/realms/main
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=your-client-secret
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=example.com,company.org
```

## Configurar el proveedor

### 1. Registrar Evey como cliente OIDC

En tu proveedor de identidad, crea un nuevo cliente o aplicación:

| Campo | Valor |
|-------|-------|
| Client ID | `evey` |
| Client Secret | genera uno |
| Redirect URI | `https://<tu dirección>/api/v1/auth/oidc/callback` |
| Grant Type | Authorization Code |
| Scopes | `openid email profile` |

### 2. Configurar Evey

Define las variables de entorno de arriba en tu `.env`. `EVEY_OIDC_ISSUER` debe ser la URL base del endpoint de descubrimiento de tu proveedor OIDC (sin `/.well-known/openid-configuration`).

### 3. Reiniciar

```bash
docker compose restart evey
```

La página de inicio de sesión mostrará un botón "Iniciar sesión con SSO".

## Cómo funciona

1. El usuario pulsa "Iniciar sesión con SSO" en la página de inicio de sesión
2. Evey redirige al proveedor OIDC
3. El usuario se autentica en el proveedor
4. El proveedor redirige de vuelta a Evey con un código de autorización
5. Evey intercambia el código por tokens y crea o actualiza la cuenta del usuario

## Restricción por dominio

`EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` limita qué dominios de correo pueden registrarse. Déjalo vacío para permitir todos los dominios.

## Creación automática de usuarios

Los usuarios que inician sesión por OIDC por primera vez se crean automáticamente con:
- un nombre de usuario derivado de `preferred_username` o del prefijo del correo
- el correo del token OIDC
- un rol no administrador
- un perfil por defecto

Los usuarios OIDC no pueden definir una contraseña local: siempre se autentican a través del proveedor.
