---
source_sha: 437e3591ffafde2e124be29a142fc1cfa7ecb154ea3e77dfbcb32e462668f452
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
  `localhost`); en otro caso el ajuste muestra **Las notificaciones push necesitan una dirección HTTPS: abre este servidor por HTTPS para activarlas.** Las aplicaciones de Android y TV no
  reciben notificaciones push.
- **Correo**: una vez configurado el correo, cada notificación se envía también
  a la dirección de la cuenta, en el idioma del perfil y con un enlace al
  servidor. Una cuenta con varios perfiles recibe un solo correo por evento. No
  hay baja por usuario.

## Configurar el correo

**Admin → Configuración del servidor → Correo / SMTP**: el servidor de correo,
su puerto, la cuenta de acceso, la dirección del remitente y el modo TLS.
Guardar se aplica al momento, sin reiniciar. **Enviar correo de prueba** envía
un mensaje breve a la dirección de tu propia cuenta con los ajustes guardados y
muestra la respuesta del servidor de correo si falla.

`starttls` pasa a TLS una conexión en claro (normalmente puerto 587), `ssl` usa
TLS desde el principio (normalmente puerto 465), `none` envía sin cifrado.

Las variables `EVEY_SMTP_*` son el respaldo, mientras la tarjeta no tenga
servidor:

```env title=".env"
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=notifications@example.com
EVEY_SMTP_PASSWORD=your-smtp-password
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls
```

En cuanto se guarda un servidor en la tarjeta, todos los campos salen de la
tarjeta y se ignoran las variables: así el servidor nunca envía las credenciales
de las variables a un servidor de correo escrito en el panel.

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
