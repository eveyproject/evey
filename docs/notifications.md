# Notifications

Evey tells each profile when new films, series, seasons, episodes or albums
arrive. Administrators also hear about failed backups, plugin errors and new
media requests, and whoever made a request hears back when it is answered.

## Where notifications appear

- **In every app**: the bell, with an unread count. Always on, nothing to set
  up. A notification is kept 30 days. The list only shows titles from libraries
  the account can open.
- **Browser push**: on the web, each profile turns on **Push Notifications** in
  **Settings → Notifications**, once per browser. The server creates its push
  keys on first start; see [E-mail and notifications](environment/notifications.md#browser-push)
  to bring your own. Browsers allow push only on an HTTPS address (or
  `localhost`); elsewhere the setting says **Push notifications not configured
  on this server.** The Android and TV apps do not receive push notifications.
- **E-mail**: once SMTP is set up, every notification is also sent to the
  account's address. There is no per-user opt-out, and an account with several
  profiles receives one e-mail per profile.

## Setting up e-mail

SMTP is read from these variables when the server starts; restart it after a
change:

```env title=".env"
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=notifications@example.com
EVEY_SMTP_PASSWORD=your-smtp-password
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls
```

`EVEY_SMTP_TLS` is `starttls` (upgrade a plain connection, usually port 587),
`ssl` (TLS from the start, usually port 465) or `none` (no encryption). The
**Email / SMTP** card in **Admin → Server Config** stores the same fields, but
the server does not read them: use the variables.

E-mail is used for notifications and password resets; invitations are links you
share yourself.

## What each profile receives

In **Settings → Notifications**, each profile picks a level for **Movies**,
**Series** and **Music**:

| Level | New titles notified |
| --- | --- |
| **All** | Every one. The default. |
| **Relevant** | Only those sharing a genre with the profile's usual genres. |
| **Important only** | Currently the same as Relevant. |
| **Disabled** | None. |

A genre counts as usual when it appears in at least 3 of the profile's last 200
playbacks. A title without genres is never notified at the Relevant and Important only
levels.

Several episodes of one series arriving within 24 hours make a single
notification that counts them, and none is sent when the series or the season
was just announced.

## Password recovery

1. Set up e-mail (see above).
2. Turn on **Password recovery** in **Admin → Server Config → Server Settings**.
   It applies immediately.
3. The sign-in page then shows **Forgot your password?**, which e-mails a reset
   link.

Without SMTP the link stays hidden, even with the option on.

<!-- sources
web push support: frontend/apps/web/src/lib/webpush.ts isPushSupported (serviceWorker + PushManager, secure context only) ; NotificationSettings.tsx shows pushNotConfigured when unsupported
types: internal/domain/notification/model.go (Type*, IsAdminOnly) ; emitted: internal/app/notification/event_handlers.go, batching.go
retention 30 d: internal/app/notification/service.go defaultRetentionDays ; grouping 24 h: batching.go episodeGroupWindow + shouldSkipEpisodeNotif
list filtered by library access: internal/adapters/driving/grpcapi/notification_handler.go allowedLibraryIDs
levels: batching.go shouldNotify (relevant == highly_relevant) + fetchFavoriteGenres (GetFavoriteGenresBatch minCount 3, window 200 sessions: watch_history_analyzer.go)
defaults all: migrations/000005_notifications.up.sql (notification_settings_global) ; UI: frontend/apps/web/src/features/user/NotificationSettings.tsx
email: internal/adapters/driven/email/notification_emailer.go (every notification.created, to the profile's account) ; wired only with EVEY_SMTP_HOST: cmd/evey/wire.go
SMTP from env only: internal/config/config.go SMTPConfig ; the card writes smtp.* keys (SmtpSettingsCard.tsx) that no Go code reads
no native push in the mobile/TV apps (no push client) ; VAPID generated: cmd/evey/wire_helpers.go
password recovery: admin.serverSettings.passwordRecovery ; gated on smtpConfigured (cmd/evey/wire.go)
-->
