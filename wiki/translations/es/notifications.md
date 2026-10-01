---
source_sha: 0f3c964e32d232266d53c9ebfe192d6a3f21996e2437d7ec2cc38945c049774c
reviewed: false
---
# Notificaciones

Evey avisa a cada perfil cuando llegan películas, series, temporadas, episodios
o álbumes nuevos. Los administradores también se enteran de las copias de
seguridad fallidas, los errores de plugins y las nuevas solicitudes de
contenido, y quien hizo una solicitud recibe aviso cuando se responde.

## Dónde aparecen las notificaciones

- **En todas las aplicaciones**: la campana, con el número de no leídas.
  Siempre activa, nada que configurar. Una notificación se conserva 30 días. La
  lista solo muestra títulos de las bibliotecas a las que la cuenta tiene
  acceso.
- **Push del navegador**: en la web, cada perfil activa **Notificaciones push**
  en **Ajustes → Notificaciones**, una vez por navegador. El servidor crea sus
  claves de push en el primer arranque; consulta
  [Correo y notificaciones](environment/notifications.md#browser-push) para
  usar las tuyas. Los navegadores solo permiten push en una dirección HTTPS (o
  `localhost`); en otro caso el ajuste muestra **Las notificaciones push no
  están configuradas en este servidor.** Las aplicaciones de Android y TV no
  reciben notificaciones push.
- **Correo**: una vez configurado el SMTP, cada notificación se envía también a
  la dirección de la cuenta. No hay baja por usuario, y una cuenta con varios
  perfiles recibe un correo por perfil.

## Configurar el correo

El SMTP se lee de estas variables al arrancar el servidor; reinícialo tras un
cambio:

```env title=".env"
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=notifications@example.com
EVEY_SMTP_PASSWORD=your-smtp-password
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls
```

`EVEY_SMTP_TLS` vale `starttls` (una conexión en claro que pasa a TLS,
normalmente puerto 587), `ssl` (TLS desde el principio, normalmente puerto 465)
o `none` (sin cifrado). La tarjeta **Correo / SMTP** de **Admin →
Configuración del servidor** guarda los mismos campos, pero el servidor no los
lee: usa las variables.

El correo sirve para las notificaciones y el restablecimiento de contraseñas;
las invitaciones son enlaces que compartes tú mismo.

## Qué recibe cada perfil

En **Ajustes → Notificaciones**, cada perfil elige un nivel para
**Películas**, **Series** y **Música**:

| Nivel | Títulos nuevos notificados |
| --- | --- |
| **Todas** | Todos. El valor por defecto. |
| **Relevantes** | Solo los que comparten un género con los géneros habituales del perfil. |
| **Solo importantes** | Por ahora, lo mismo que Relevantes. |
| **Desactivado** | Ninguno. |

Un género cuenta como habitual cuando aparece en al menos 3 de las últimas 200
reproducciones del perfil. Un título sin géneros nunca se notifica en los
niveles Relevantes y Solo importantes.

Varios episodios de una misma serie que llegan en menos de 24 horas generan una
sola notificación que los cuenta, y no se envía ninguna cuando la serie o la
temporada se acaban de anunciar.

## Recuperación de contraseña

1. Configura el correo (ver arriba).
2. Activa **Recuperación de contraseña** en **Admin → Configuración del
   servidor → Configuración del servidor**. Se aplica de inmediato.
3. La página de inicio de sesión muestra entonces **¿Olvidaste tu
   contraseña?**, que envía por correo un enlace de restablecimiento.

Sin SMTP el enlace sigue oculto, aunque la opción esté activada.
