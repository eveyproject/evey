# Notifications

Evey can notify users when new media is added, backups fail, and other events.

## Channels

### In-App Notifications
Always enabled. Users see a bell icon with unread count. No configuration needed.

### Email (SMTP)
Sends an email when a notification is created. Requires SMTP configuration:

```env title=".env"
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=notifications@example.com
EVEY_SMTP_PASSWORD=your-smtp-password
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls
```

TLS modes:
- `starttls` - connect plain, upgrade to TLS (port 587, most common)
- `ssl` - connect directly with TLS (port 465)
- `none` - no encryption (not recommended)

### Web Push
Browser push notifications. VAPID keys are auto-generated on first startup. Users opt in from their profile settings.

## Password Recovery

Password reset emails require SMTP to be configured. An admin must also enable the feature:

1. Configure SMTP (see above).
2. Turn on **Password recovery** in **Admin → Server Config**. It applies
   immediately.
3. The login page then shows a "Forgot your password?" link.

If SMTP is removed after enabling, password recovery is automatically disabled.

## Notification Levels

Users can set their notification preference per media type:
- **All** - notify for everything
- **Relevant** - only if genres match the user's watch history
- **Important only** - stricter genre matching
- **Disabled** - no notifications for this type
