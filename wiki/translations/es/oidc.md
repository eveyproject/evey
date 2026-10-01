---
source_sha: 4a1aad8cb9228c4a82e7b9a0a3cbe3ee43a07f46b3a7109eb678dd7331ba9fd2
reviewed: false
---
# Configuración de OIDC / SSO

Evey admite el inicio de sesión único mediante OpenID Connect: Keycloak,
Authentik, Authelia, Google o cualquier otro proveedor OIDC. La página de inicio
de sesión de las aplicaciones web y Android muestra entonces **Iniciar sesión
con SSO** junto al formulario de contraseña.

## 1. Registrar Evey en el proveedor

Crea en el proveedor un cliente (o aplicación) con el tipo de concesión
Authorization Code, los scopes `openid email profile`, un secreto de cliente y
las dos direcciones que la tarjeta **Inicio de sesión único (SSO)** muestra en
**Registra estas direcciones en tu proveedor**:

| Campo | Valor |
|-------|-------|
| URI de redirección | `https://<tu dirección>/api/v1/auth/oidc/callback` |
| URI de redirección tras cerrar sesión | `https://<tu dirección>/login` |

Ambas se construyen a partir de la dirección pública del servidor (**Admin →
Acceso remoto**): configúrala primero si se accede a Evey por otra dirección.

## 2. Configurar Evey

**Admin → Configuración del servidor → Inicio de sesión único (SSO)**:

| Campo | Efecto |
| --- | --- |
| **Activar el inicio de sesión único** | Muestra el botón SSO en las pantallas de inicio de sesión. |
| **URL del emisor (issuer)** | La dirección del proveedor, sin `/.well-known/openid-configuration`. |
| **ID de cliente**, **Secreto de cliente** | El cliente registrado arriba. El secreto se guarda en el servidor y no vuelve a mostrarse; dejar los puntos lo conserva. |
| **Dominios de correo permitidos** | Separados por comas. Si se indican, solo se aceptan esas direcciones, y el proveedor debe confirmar la dirección. |
| **Texto del botón** | Opcional, sustituye a «Iniciar sesión con SSO». |
| **Vincular con cuentas existentes por correo verificado** | Activado por defecto. Ver más abajo. |
| **Crear cuentas para nuevos usuarios SSO** | Desactivado salvo que haya dominios de correo permitidos. Ver más abajo. |
| **Acceso a bibliotecas de las nuevas cuentas SSO** | **Todas las bibliotecas**, o **Solo las bibliotecas elegidas**. |

**Probar la conexión** comprueba que el emisor responde. **Guardar ajustes SSO**
se aplica al momento, sin reiniciar. La línea **Estado** indica si el proveedor
responde, con el último error si no.

Las variables `EVEY_OIDC_*` de [Cuentas e inicio de sesión](environment/accounts.md)
son el respaldo: la tarjeta indica cuándo sus valores vienen de ellas, y
guardarla toma el relevo.

El servidor nunca espera al proveedor: arranca aunque el proveedor esté caído,
el botón SSO sigue oculto mientras no se pueda contactar con el proveedor, y el
inicio de sesión con contraseña sigue funcionando. El proveedor se vuelve a
contactar en el siguiente intento de inicio de sesión, como mucho cada 30
segundos.

## Quién puede iniciar sesión

1. Una cuenta que ya inició sesión con este proveedor vuelve a entrar, salvo que
   esté desactivada.
2. Si no, con **Vincular con cuentas existentes por correo verificado** activado,
   una cuenta existente con el mismo correo se vincula al proveedor y entra,
   pero solo si el proveedor confirma la dirección. La cuenta conserva su
   contraseña.
3. Si no, con **Crear cuentas para nuevos usuarios SSO** activado, se crea una
   cuenta nueva, con un perfil y el acceso a bibliotecas elegido en la tarjeta.
   Su nombre de usuario es el `preferred_username` del proveedor o, si falta, la
   parte del correo anterior a `@`; un nombre ya ocupado recibe un sufijo corto.

**Crear cuentas para nuevos usuarios SSO** está desactivado por defecto cuando no
hay ningún dominio de correo permitido: con un proveedor público como Google,
cualquiera que tenga una cuenta allí podría obtener una en tu servidor. Si está
desactivado, solo las cuentas existentes pueden iniciar sesión con SSO.

Cuando se rechaza un inicio de sesión, la página indica el motivo: el dominio
del correo no está permitido, ninguna cuenta coincide y la creación está
desactivada, la cuenta está desactivada, la cuenta existe pero no se pudo
vincular, o el proveedor no está disponible.

## Cerrar sesión

Cuando el proveedor publica un punto de cierre de sesión, cerrar sesión en Evey
tras un inicio de sesión SSO también cierra la sesión en el proveedor y luego
vuelve a la página de inicio de sesión de Evey. La tarjeta indica si el
proveedor lo admite.
