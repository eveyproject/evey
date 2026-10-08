# E-mail and notifications

The variables behind outgoing mail (notifications, password resets) and
browser push. Mail is normally set up in the panel, as described in
[E-mail and notifications](../notifications.md); the variables are its fallback.

## E-mail (SMTP)

The variables are read at startup and used while the **Email / SMTP** card of
**Admin → Server Config** has no host. Once a host is saved there, the card
provides every field and applies without a restart.

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `EVEY_SMTP_HOST` | empty | The mail server. Empty: no mail is sent. | Server Config → Email / SMTP → **SMTP Host**, live |
| `EVEY_SMTP_PORT` | `587` | Its port. | same card → **Port**, live |
| `EVEY_SMTP_USERNAME` | empty | The account to sign in with. | same card → **Username**, live |
| `EVEY_SMTP_PASSWORD` | empty | Its password. | same card, live |
| `EVEY_SMTP_FROM` | empty | The sender address, for example `noreply@example.com`. | same card, live |
| `EVEY_SMTP_TLS` | `starttls` | `starttls`, `ssl` or `none`. | same card, live |

## Browser push

| Variable | Default | What it does | Panel |
| --- | --- | --- | --- |
| `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY` | generated | The key pair that signs push notifications. When absent, the server generates one on the first start and stores it in the database; set them only to keep a pair you already use, and always both: one alone disables push with a warning. | none |
| `VAPID_SUBJECT` | `mailto:admin@localhost` | The contact address push services see. | none |

<!-- sources
internal/config/config.go - EVEY_SMTP_*, VAPID_* ; WarnValidate (one VAPID key alone)
SMTP card wins, env fallback: internal/app/config/smtp.go ResolveSMTPSettings ; i18n admin.serverConfig.emailSmtp, smtpHost, port, smtpUsername
vapid generation: cmd/evey/wire_helpers.go - resolveVAPIDKeys (vapid.public_key / vapid.private_key)
-->
