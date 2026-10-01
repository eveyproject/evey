---
source_sha: f9e7107994608f10a03d81a80862011f9ceefaa236fe5b2f076b2dcdc7937d5a
reviewed: false
---
# Correo y notificaciones

Correo saliente (invitaciones, restablecimiento de contraseñas, notificaciones) y
push del navegador. Consulta [Correo y notificaciones](../notifications.md).

## Correo (SMTP)

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_SMTP_HOST` | vacía | El servidor de correo. Vacía: no se envía correo. |
| `EVEY_SMTP_PORT` | `587` | Su puerto. |
| `EVEY_SMTP_USERNAME` | vacía | La cuenta con la que iniciar sesión. |
| `EVEY_SMTP_PASSWORD` | vacía | Su contraseña. |
| `EVEY_SMTP_FROM` | vacía | La dirección del remitente, por ejemplo `noreply@example.com`. |
| `EVEY_SMTP_TLS` | `starttls` | `starttls`, `ssl` o `none`. |

Los mismos ajustes están en **Admin → Configuración del servidor → Correo / SMTP**.

## Push del navegador

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY` | generadas | El par de claves que firma las notificaciones push. Si no están, el servidor genera uno en el primer arranque y lo guarda en la base de datos; defínelas solo para conservar un par que ya usas. |
| `VAPID_SUBJECT` | `mailto:admin@localhost` | La dirección de contacto que ven los servicios de push. |
