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
  `localhost`); elsewhere the setting says **Push notifications need an HTTPS address: open this server over HTTPS to turn them on.** The Android and TV apps do not receive push notifications.
- **E-mail**: once e-mail is set up, every notification is also sent to the
  account's address, in the profile's language, with a link to the server. An
  account with several profiles receives one e-mail per event. There is no
  per-user opt-out.

## Setting up e-mail

**Admin → Server Config → Email / SMTP**: the mail server, its port, the
account to sign in with, the sender address and the TLS mode. Saving applies at
once, without a restart. **Send test email** sends a short message to your own
account's address with the saved settings, and shows the mail server's answer
when it fails.

`starttls` upgrades a plain connection (usually port 587), `ssl` uses TLS from
the start (usually port 465), `none` sends without encryption.

The `EVEY_SMTP_*` variables are the fallback, used while the card has no host:

```env title=".env"
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=notifications@example.com
EVEY_SMTP_PASSWORD=your-smtp-password
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls
```

As soon as a host is saved in the card, every field comes from the card and the
variables are ignored, so the server never sends the variables' credentials to
a mail server typed in the panel.

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
email: internal/adapters/driven/email/notification_emailer.go (push title key, recipient locale, one per account per FanoutID, base URL link)
SMTP live from the card, env fallback, panel host = panel only: internal/app/config/smtp.go (ResolveSMTPSettings, LiveSMTP) ; test send: internal/adapters/driving/http/smtp_test_handler.go
no native push in the mobile/TV apps (no push client) ; VAPID generated: cmd/evey/wire_helpers.go
password recovery: admin.serverSettings.passwordRecovery ; gated on smtpConfigured (cmd/evey/wire.go)
-->
