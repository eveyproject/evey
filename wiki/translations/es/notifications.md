---
source_sha: eea86697845889523ecbd07dd3d88da0143ac37f477404249051c73e160c80c8
---

# Notificaciones

Evey puede avisar a sus usuarios cuando se añade contenido nuevo, cuando falla una copia de seguridad y ante otros sucesos.

## Canales

### Notificaciones dentro de la aplicación
Siempre activas. El usuario ve un icono de campana con el número de mensajes sin leer. No hay nada que configurar.

### Correo electrónico (SMTP)
Envía un correo cada vez que se genera una notificación. Necesita la configuración SMTP:

```env title=".env"
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=notifications@example.com
EVEY_SMTP_PASSWORD=your-smtp-password
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls
```

Modos TLS:
- `starttls` - conexión en claro que después se eleva a TLS (puerto 587, el caso más habitual)
- `ssl` - conexión directamente cifrada con TLS (puerto 465)
- `none` - sin cifrado (no recomendable)

### Web Push
Notificaciones push del navegador. Las claves VAPID se generan solas en el primer arranque. Cada usuario decide si quiere recibirlas desde los ajustes de su perfil.

## Recuperación de la contraseña

Los correos de restablecimiento de contraseña necesitan que SMTP esté configurado. Además, un administrador tiene que activar la función:

1. Configure SMTP (véase más arriba)
2. En el panel de administración, ponga `auth.password_recovery_enabled` en `true` dentro de la configuración del servidor
3. La página de inicio de sesión mostrará entonces el enlace «¿Ha olvidado su contraseña?»

Si se retira la configuración SMTP después de haber activado la función, la recuperación de contraseña se desactiva sola.

## Niveles de notificación

Cada usuario elige qué quiere recibir para cada tipo de contenido:
- **Todas** - avisar de todo
- **Relevantes** - solo si los géneros coinciden con su historial de visionado
- **Solo importantes** - coincidencia de géneros más estricta
- **Desactivadas** - ninguna notificación para ese tipo
