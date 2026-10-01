---
source_sha: 83ece8837b08f345398777a65fee01eeec37d776c49c29ad7d1b6f675ab96046
reviewed: false
---
# Correo y notificaciones

Correo saliente (notificaciones, restablecimiento de contraseñas) y push del
navegador. Consulta [Correo y notificaciones](../notifications.md).

## Correo (SMTP)

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_SMTP_HOST` | vacía | El servidor de correo. Vacía: no se envía correo. |
| `EVEY_SMTP_PORT` | `587` | Su puerto. |
| `EVEY_SMTP_USERNAME` | vacía | La cuenta con la que iniciar sesión. |
| `EVEY_SMTP_PASSWORD` | vacía | Su contraseña. |
| `EVEY_SMTP_FROM` | vacía | La dirección del remitente, por ejemplo `noreply@example.com`. |
| `EVEY_SMTP_TLS` | `starttls` | `starttls`, `ssl` o `none`. |

Se leen al arrancar el servidor y se usan mientras la tarjeta **Correo / SMTP**
de **Admin → Configuración del servidor** no tenga servidor. En cuanto se guarda
uno en ella, la tarjeta aporta todos los campos y se aplica sin reiniciar.

## Push del navegador

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY` | generadas | El par de claves que firma las notificaciones push. Si no están, el servidor genera uno en el primer arranque y lo guarda en la base de datos; defínelas solo para conservar un par que ya usas. |
| `VAPID_SUBJECT` | `mailto:admin@localhost` | La dirección de contacto que ven los servicios de push. |
