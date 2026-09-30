---
source_sha: 5dca3b7fe64acb5cf79df57c6da42ed90ae968596cba80b381b921667c8e36e9
reviewed: false
---
# Notificaciones

Evey puede avisar a los usuarios cuando se añade contenido nuevo, cuando falla una copia de seguridad y ante otros eventos.

## Canales

### Notificaciones en la aplicación
Siempre activadas. Los usuarios ven una campana con el número de no leídas. No hace falta configurar nada.

### Correo (SMTP)
Envía un correo cuando se crea una notificación. Requiere configurar SMTP:

```env title=".env"
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=notifications@example.com
EVEY_SMTP_PASSWORD=your-smtp-password
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls
```

Modos TLS:
- `starttls`: conexión sin cifrar que pasa a TLS (puerto 587, el más habitual)
- `ssl`: conexión directa con TLS (puerto 465)
- `none`: sin cifrado (no recomendado)

### Web Push
Notificaciones push del navegador. Las claves VAPID se generan automáticamente en el primer arranque. Cada usuario las activa desde los ajustes de su perfil.

## Recuperación de contraseña

Los correos de restablecimiento de contraseña requieren SMTP configurado. Además, un administrador debe activar la función:

1. Configura SMTP (ver arriba).
2. Activa **Recuperación de contraseña** en **Admin → Configuración del servidor**.
   Se aplica de inmediato.
3. La página de inicio de sesión muestra entonces un enlace "¿Olvidaste tu contraseña?".

Si se quita SMTP después de activarla, la recuperación de contraseña se desactiva automáticamente.

## Niveles de notificación

Cada usuario puede elegir su preferencia de notificación por tipo de contenido:
- **Todas**: avisar de todo
- **Relevantes**: solo si los géneros coinciden con el historial del usuario
- **Solo importantes**: coincidencia de géneros más estricta
- **Desactivado**: ninguna notificación para este tipo
