# E-mail and notifications

Outgoing mail (notifications, password resets) and browser push.
See [E-mail and notifications](../notifications.md).

## E-mail (SMTP)

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_SMTP_HOST` | empty | The mail server. Empty: no mail is sent. |
| `EVEY_SMTP_PORT` | `587` | Its port. |
| `EVEY_SMTP_USERNAME` | empty | The account to sign in with. |
| `EVEY_SMTP_PASSWORD` | empty | Its password. |
| `EVEY_SMTP_FROM` | empty | The sender address, for example `noreply@example.com`. |
| `EVEY_SMTP_TLS` | `starttls` | `starttls`, `ssl` or `none`. |

They are read when the server starts and used while the **Email / SMTP** card
of **Admin → Server Config** has no host. Once a host is saved in the card, the
card provides every field and applies without a restart.

## Browser push

| Variable | Default | Effect |
| --- | --- | --- |
| `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY` | generated | The key pair that signs push notifications. When absent, the server generates one on the first start and stores it in the database; set them only to keep a pair you already use. |
| `VAPID_SUBJECT` | `mailto:admin@localhost` | The contact address push services see. |

<!-- sources
internal/config/config.go - EVEY_SMTP_*, VAPID_*
SMTP card wins, env fallback: internal/app/config/smtp.go ResolveSMTPSettings
vapid generation: cmd/evey/wire_helpers.go (GenerateVAPIDKeys, stored as vapid.public_key / vapid.private_key)
-->
