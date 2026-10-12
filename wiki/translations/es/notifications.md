---
source_sha: a8b7449b152c8718e3a2aef1de3cabe29af5dbe26aa1f77133448c996ce35d3c
reviewed: true
---
# Correo y notificaciones

Evey avisa a cada perfil cuando llegan películas, series, temporadas,
episodios o álbumes nuevos. También avisa:

- **a la persona que hizo una solicitud**, cuando recibe respuesta;
- **a los administradores**, de las solicitudes nuevas, las copias de
  seguridad fallidas, los errores de plugins y la caché de subtítulos llena.

Las notificaciones aparecen en las apps sin configurar nada. Por correo,
cuando configuras un servidor de correo (más abajo).

## Dónde aparecen las notificaciones

- **En todas las apps**: la campana, con el número de notificaciones sin
  leer. Siempre activa. Una notificación se guarda 30 días. Cada persona
  solo ve títulos de las bibliotecas que su cuenta puede abrir.
- **En el navegador (push)**: en la web, cada perfil activa
  **Notificaciones push** en **Ajustes → Notificaciones**, una vez por
  navegador. Los navegadores solo lo permiten en una dirección `https://`
  (o `localhost`). En los demás casos, el ajuste dice **Las notificaciones
  push necesitan una dirección HTTPS: abre este servidor por HTTPS para
  activarlas.** Las apps de Android y de TV no reciben notificaciones push.
- **Por correo**: una vez configurado el correo, cada notificación también
  se envía a la dirección de correo de la cuenta, en el idioma del perfil,
  con un enlace al servidor. Una cuenta con varios perfiles recibe un correo
  por cada evento. Nadie puede desactivar los correos para su propia
  cuenta.

??? info "Claves push"

    El servidor crea en su primer arranque las claves que necesitan las
    notificaciones push del navegador. Para usar las tuyas, ver
    [Correo y notificaciones](environment/notifications.md#browser-push).

## Configurar el correo

Evey envía los correos a través de un servidor de correo en el que ya
tienes una cuenta: tu proveedor de correo, o un servicio de envío. Las
páginas de ayuda de tu proveedor dan los valores de abajo.

1. Ve a **Administración → Configuración del servidor → Correo / SMTP**.
2. Rellena los campos:

    | Campo | Qué escribir |
    | --- | --- |
    | **Host SMTP**, **Puerto** | El servidor de correo y su puerto. Las parejas habituales son `587` con `starttls`, o `465` con `ssl`. |
    | **Nombre de usuario**, **Contraseña** | La cuenta con la que Evey inicia sesión. |
    | **Dirección de remitente** | La dirección del remitente que aparece en cada mensaje. |
    | **Modo TLS** | `starttls` o `ssl`, según el puerto (ver arriba). `none` envía sin cifrar. |

3. Pulsa **Guardar ajustes SMTP**. Se aplica en el momento, sin reiniciar.
4. Pulsa **Enviar correo de prueba**. Evey envía un mensaje corto a la
   dirección de tu propia cuenta. Si falla, muestra la respuesta del
   servidor de correo.

El error más habitual es un modo TLS que no corresponde al puerto.

El correo se usa para las notificaciones y para restablecer contraseñas.
Las invitaciones son enlaces que envías tú.

??? info "Configurar el correo en el entorno"

    Las variables `EVEY_SMTP_*` solo se usan mientras la tarjeta no tiene
    host:

    ```env title=".env"
    EVEY_SMTP_HOST=smtp.example.com
    EVEY_SMTP_PORT=587
    EVEY_SMTP_USERNAME=notifications@example.com
    EVEY_SMTP_PASSWORD=your-smtp-password
    EVEY_SMTP_FROM=noreply@example.com
    EVEY_SMTP_TLS=starttls
    ```

    En cuanto se guarda un host en la tarjeta, todos los campos salen de la
    tarjeta y las variables se ignoran. Así, el servidor nunca envía la
    contraseña de las variables a un servidor de correo escrito en el panel.
    Vacía el host para volver a las variables.

## Elegir lo que recibe un perfil

En **Ajustes → Notificaciones**, cada perfil elige un nivel para
**Películas**, **Series** y **Música**:

| Nivel | Títulos nuevos de los que te avisa |
| --- | --- |
| **Todas** | Todos. Es el valor por defecto. |
| **Relevantes** | Solo los títulos que comparten un género con lo que el perfil suele ver. |
| **Solo importantes** | Por ahora, lo mismo que Relevantes. |
| **Desactivado** | Ninguno. |

Un género cuenta como "visto habitualmente" cuando aparece en al menos 3 de
las últimas 200 reproducciones del perfil. Un título sin géneros nunca se
notifica en los niveles Relevantes y Solo importantes.

Cuando llegan varios episodios de una misma serie en menos de 24 horas,
recibes una sola notificación que los cuenta. No se envía ninguna si la
serie o la temporada se acaba de anunciar.

## Dejar que la gente restablezca su contraseña

1. Configura el correo (más arriba).
2. Activa **Recuperación de contraseña** en **Administración →
   Configuración del servidor → Ajustes del servidor**. Se aplica en
   el momento.
3. La página de inicio de sesión muestra ahora **¿Olvidaste tu
   contraseña?**. Envía por correo un enlace para elegir una contraseña
   nueva.

Sin servidor de correo, el enlace sigue oculto, aunque la opción esté
activada.

<!-- sources
web push support: frontend/apps/web/src/lib/webpush.ts isPushSupported (serviceWorker + PushManager, secure context only) ; NotificationSettings.tsx shows pushNeedsHttps when unsupported
types: internal/domain/notification/model.go (Type*, IsAdminOnly: backup_failed, plugin_error, media_request) ; emitted: internal/app/notification/event_handlers.go, batching.go
retention 30 d: internal/app/notification/service.go defaultRetentionDays ; grouping 24 h: batching.go episodeGroupWindow + shouldSkipEpisodeNotif
list filtered by library access: internal/adapters/driving/grpcapi/notification_handler.go allowedLibraryIDs
bell on TV and mobile: frontend/apps/tv/src/components/TopHeader.tsx ; frontend/apps/mobile/app/(auth)/settings/notifications.tsx
levels: batching.go shouldNotify (relevant == highly_relevant) + fetchFavoriteGenres (GetFavoriteGenresBatch minCount 3, window 200 sessions: watch_history_analyzer.go)
defaults all: migrations/000005_notifications.up.sql (notification_settings_global) ; UI: frontend/apps/web/src/features/user/NotificationSettings.tsx ; i18n notificationSettings.level*
email: internal/adapters/driven/email/notification_emailer.go (push title key, recipient locale, one per account per FanoutID, base URL link)
SMTP live from the card, env fallback, panel host = panel only, defaults 587/starttls: internal/app/config/smtp.go (ResolveSMTPSettings, LiveSMTP) ; test send: internal/adapters/driving/http/smtp_test_handler.go
labels: i18n admin.serverConfig emailSmtp, smtpHost, port, smtpUsername, smtpPassword, fromAddress, tlsMode, saveSmtp, sendTestEmail
no native push in the mobile/TV apps (no push client) ; VAPID generated: cmd/evey/wire_helpers.go
password recovery: admin.serverSettings.passwordRecovery ; gated on smtpConfigured (cmd/evey/wire.go) ; auth.login.forgotPassword
admin-only types include the subtitle cache: internal/domain/notification/model.go - IsAdminOnly (TypeCacheFull) ; i18n notifications push.cache_full
labels: i18n admin.serverConfig.port, smtpUsername, smtpPassword, smtpEnvHint ; auth.settings pushNotifications, pushNeedsHttps ; levelAll, levelRelevant, levelImportant, levelDisabled
no invitation e-mail: internal/adapters/driven/email (no invite sender)
-->
