---
source_sha: 5f2dd5bf7c21906e243c0cece2ecff83e306d69d5b99a193541b9e574db7a4c2
reviewed: true
---
# Inicio de sesión único (OIDC)

Permite que la gente inicie sesión con el proveedor de identidad que ya
usas (Keycloak, Authentik, Authelia, Google o cualquier otro proveedor
OpenID Connect): la página de inicio de sesión de la web y de las apps de
Android muestra entonces **Iniciar sesión con SSO** junto al formulario de
contraseña. El administrador tiene dos pasos: registrar Evey en el
proveedor y después rellenar una tarjeta en el panel de administración.

## 1. Registrar Evey en el proveedor

Crea un cliente (o aplicación) en el proveedor con el flujo Authorization
Code, los scopes `openid email profile`, un secreto de cliente y las dos
direcciones que muestra la tarjeta **Inicio de sesión único (SSO)** en
**Registra estas direcciones en tu proveedor**:

| Campo | Valor |
| --- | --- |
| **URI de redirección** | `https://<your address>/api/v1/auth/oidc/callback` |
| **URI de redirección tras cerrar sesión** | `https://<your address>/login` |

Las dos se construyen con la dirección pública del servidor
(**Administración → Acceso remoto**): ponla primero si la gente llega a Evey
por otra dirección.

## 2. Configurar Evey

**Administración → Configuración del servidor → Inicio de sesión único
(SSO)**:

| Campo | Efecto |
| --- | --- |
| **Activar el inicio de sesión único** | Muestra el botón SSO en las pantallas de inicio de sesión. |
| **URL del emisor (issuer)** | La dirección del proveedor; Evey lee `/.well-known/openid-configuration` bajo ella. |
| **ID de cliente**, **Secreto de cliente** | El cliente registrado más arriba. El secreto se guarda en el servidor y no se vuelve a mostrar nunca; si dejas los puntos, se conserva. |
| **Dominios de correo permitidos** | Separados por comas. Si se rellena, solo se aceptan esas direcciones, y el proveedor tiene que confirmar la dirección. |
| **Texto del botón** | Opcional, sustituye a "Iniciar sesión con SSO". |
| **Vincular con cuentas existentes por correo verificado** | Activado por defecto. Ver [Quién puede iniciar sesión](#who-can-sign-in). |
| **Crear cuentas para nuevos usuarios SSO** | Desactivado salvo que haya dominios de correo permitidos. Ver [Quién puede iniciar sesión](#who-can-sign-in). |
| **Acceso a bibliotecas de las nuevas cuentas SSO** | **Todas las bibliotecas**, o **Solo las bibliotecas elegidas**. |

**Probar la conexión** comprueba que el emisor responde. **Guardar ajustes
SSO** se aplica en el momento, sin reiniciar. La línea **Estado** dice si se
puede llegar al proveedor y, si no, muestra el último error.

Las variables `EVEY_OIDC_*` de
[Cuentas e inicio de sesión](environment/accounts.md#single-sign-on-oidc)
son la opción de reserva: la tarjeta indica cuándo sus valores vienen de
ellas, y al guardar la tarjeta pasa a mandar ella.

!!! note "El servidor nunca depende del proveedor para funcionar"
    El servidor arranca aunque el proveedor no esté disponible, el botón SSO
    se queda oculto mientras no se puede llegar al proveedor, y el inicio de
    sesión con contraseña sigue funcionando. Se vuelve a contactar con el
    proveedor en el siguiente intento de inicio de sesión, como mucho cada
    30 segundos.

## Quién puede iniciar sesión

1. Una cuenta que ya inició sesión con este proveedor vuelve a iniciarla,
   salvo que esté desactivada.
2. Si no, con **Vincular con cuentas existentes por correo verificado**
   activado, una cuenta existente con el mismo correo se vincula al
   proveedor e inicia sesión, pero solo cuando el proveedor confirma la
   dirección. La cuenta conserva su contraseña. Las cuentas de
   administrador nunca se vinculan de esta forma.
3. Si no, con **Crear cuentas para nuevos usuarios SSO** activado, se crea
   una cuenta nueva, con un perfil y el acceso a bibliotecas elegido en la
   tarjeta. Su nombre de usuario es el `preferred_username` del proveedor,
   o si no, la parte del correo antes de `@`; si el nombre ya está cogido,
   se le añade un sufijo corto.

!!! warning "Por qué la creación de cuentas está desactivada por defecto"
    Con un proveedor público como Google, cualquiera que tenga una cuenta
    allí podría conseguir una en tu servidor. La creación solo está activada
    por defecto cuando **Dominios de correo permitidos** limita quién puede
    entrar. Cuando está desactivada, solo las cuentas existentes pueden
    iniciar sesión con SSO.

Cuando se rechaza un inicio de sesión, la página de inicio de sesión dice
por qué: el dominio del correo no está permitido, ninguna cuenta coincide y
el registro está desactivado, la cuenta está desactivada, la cuenta existe
pero no se pudo vincular, o el proveedor no está disponible.

## Cerrar sesión

Cuando el proveedor publica una dirección de cierre de sesión, cerrar sesión
en Evey después de haber entrado por SSO también cierra la sesión en el
proveedor, y después vuelve a la página de inicio de sesión de Evey. La
tarjeta indica si el proveedor lo admite.

<!-- sources
settings and precedence (panel keys oidc.* win over EVEY_OIDC_*): internal/app/user/oidc_settings.go - ResolveOIDCConfig, AllowSignup default from AllowedEmailDomains
lazy discovery, 30 s backoff, never at boot: internal/app/user/oidc_service.go - oidcRetryBackoff ; cmd/evey/wire.go
account resolution (identity, verified-email link, sign-up, suffix, default libraries, admin never linked): oidc_service.go findOrCreateUser ; tests oidc_policy_test.go
admin routes GET/PUT /api/v1/admin/sso, POST /api/v1/admin/sso/test: internal/adapters/driving/http/sso_admin_handler.go
login error reasons: internal/adapters/driving/http/auth_handler.go (oidc_error=<reason>) ; i18n auth.login.ssoError.*
logout endSessionUrl: internal/adapters/driving/http/server.go
mobile flow: internal/adapters/driving/http/oidc_app_flow.go ; frontend/apps/mobile/app/oidc-callback.tsx
UI: frontend/apps/web/src/features/admin/serverConfig/SsoSettingsCard.tsx ; i18n admin.sso.* ; auth.login.signInWithSSO
-->
