# E-mail and notifications

Evey tells each profile when new films, series, seasons, episodes or albums
arrive. It also tells:

- **the person who made a request**, when it is answered;
- **administrators**, about new requests, failed backups, plugin errors and
  a full subtitle cache.

Notifications appear in the apps with nothing to set up. By e-mail, once you
set up a mail server (below).

## Where notifications appear

- **In every app**: the bell, with the number of unread notifications. Always
  on. A notification is kept 30 days. People only see titles from libraries
  their account can open.
- **In the browser (push)**: on the web, each profile turns on **Push
  Notifications** in **Settings → Notifications**, once per browser.
  Browsers only allow this on an `https://` address (or `localhost`).
  Elsewhere the setting says **Push notifications need an HTTPS address:
  open this server over HTTPS to turn them on.** The Android and TV apps do
  not receive push notifications.
- **By e-mail**: once e-mail is set up, every notification is also sent to
  the account's e-mail address, in the profile's language, with a link to
  the server. An account with several profiles receives one e-mail per event.
  People cannot turn e-mails off for themselves.

??? info "Push keys"

    The server creates the keys browser push needs on its first start. To
    bring your own, see
    [E-mail and notifications](environment/notifications.md#browser-push).

## Set up e-mail

Evey sends e-mail through a mail server you already have an account with:
your e-mail provider, or a sending service. Your provider's help pages give
the values below.

1. Go to **Admin → Server Config → Email / SMTP**.
2. Fill in the fields:

    | Field | What to type |
    | --- | --- |
    | **SMTP Host**, **Port** | The mail server and its port. The usual pairs are `587` with `starttls`, or `465` with `ssl`. |
    | **Username**, **Password** | The account Evey signs in with. |
    | **From Address** | The sender address shown on every message. |
    | **TLS Mode** | `starttls` or `ssl`, to match the port (see above). `none` sends without encryption. |

3. Press **Save SMTP Settings**. It applies at once, without a restart.
4. Press **Send test email**. Evey sends a short message to your own
   account's address. If it fails, it shows the mail server's answer.

The most common mistake is a TLS mode that does not match the port.

E-mail is used for notifications and password resets. Invitations are links
you send yourself.

??? info "Setting e-mail in the environment instead"

    The `EVEY_SMTP_*` variables, under `environment:` in the compose file,
    are used only while the card has no host:

    ```yaml title="docker-compose.yml"
    services:
      evey:
        environment:
          EVEY_SMTP_HOST: smtp.example.com
          EVEY_SMTP_PORT: "587"
          EVEY_SMTP_USERNAME: notifications@example.com
          EVEY_SMTP_PASSWORD: your-smtp-password
          EVEY_SMTP_FROM: noreply@example.com
          EVEY_SMTP_TLS: starttls
    ```

    As soon as a host is saved in the card, every field comes from the card
    and the variables are ignored. So the server never sends the variables'
    password to a mail server typed in the panel. Clear the host to go back
    to the variables.

## Choose what a profile receives

In **Settings → Notifications**, each profile picks a level for **Movies**,
**Series** and **Music**:

| Level | New titles you are told about |
| --- | --- |
| **All** | Every one. This is the default. |
| **Relevant** | Only titles that share a genre with what the profile usually watches. |
| **Important only** | For now, the same as Relevant. |
| **Disabled** | None. |

A genre counts as "usually watched" when it appears in at least 3 of the
profile's last 200 playbacks. A title without genres is never notified at
the Relevant and Important only levels.

When several episodes of one series arrive within 24 hours, you get one
notification that counts them. None is sent when the series or the season
was just announced.

## Let people reset their password

1. Set up e-mail (above).
2. Turn on **Password recovery** in **Admin → Server Config → Server
   Settings**. It applies immediately.
3. The sign-in page now shows **Forgot your password?**. It e-mails a link to
   choose a new password.

Without a mail server, the link stays hidden, even with the option on.

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
