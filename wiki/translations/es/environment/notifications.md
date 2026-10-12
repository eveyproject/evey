---
source_sha: 7cc0fb43b6f9e14980a2332dd94e4c79bea3397ef66018840bb68f8b90a78ca2
reviewed: true
---
# Correo y notificaciones

Las variables que hay detrás del correo saliente (notificaciones,
restablecimiento de contraseña) y de las notificaciones push del navegador.
El correo se configura normalmente en el panel, como se describe en
[Correo y notificaciones](../notifications.md); las variables son su valor de
respaldo.

## Correo (SMTP)

Las variables se leen al arrancar y se usan mientras la tarjeta **Correo /
SMTP** de **Administración → Configuración del servidor** no tenga servidor
de correo. En cuanto se guarda uno ahí, la tarjeta aporta todos los campos y
se aplica sin reiniciar.

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_SMTP_HOST` | vacío | El servidor de correo. Vacío: no se envía ningún correo. | Configuración del servidor → Correo / SMTP → **Host SMTP**, inmediato |
| `EVEY_SMTP_PORT` | `587` | Su puerto. | misma tarjeta → **Puerto**, inmediato |
| `EVEY_SMTP_USERNAME` | vacío | La cuenta con la que se inicia sesión. | misma tarjeta → **Nombre de usuario**, inmediato |
| `EVEY_SMTP_PASSWORD` | vacío | Su contraseña. | misma tarjeta, inmediato |
| `EVEY_SMTP_FROM` | vacío | La dirección del remitente, por ejemplo `noreply@example.com`. | misma tarjeta, inmediato |
| `EVEY_SMTP_TLS` | `starttls` | `starttls`, `ssl` o `none`. | misma tarjeta, inmediato |

## Notificaciones push del navegador

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY` | generadas | El par de claves que firma las notificaciones push. Si no están, el servidor genera un par en el primer arranque y lo guarda en la base de datos; defínelas solo para conservar un par que ya usas, y siempre las dos: una sola desactiva las notificaciones push con un aviso. | ninguno |
| `VAPID_SUBJECT` | `mailto:admin@localhost` | La dirección de contacto que ven los servicios de notificaciones push. | ninguno |

<!-- sources
internal/config/config.go - EVEY_SMTP_*, VAPID_* ; WarnValidate (one VAPID key alone)
SMTP card wins, env fallback: internal/app/config/smtp.go ResolveSMTPSettings ; i18n admin.serverConfig.emailSmtp, smtpHost, port, smtpUsername
vapid generation: cmd/evey/wire_helpers.go - resolveVAPIDKeys (vapid.public_key / vapid.private_key)
-->
