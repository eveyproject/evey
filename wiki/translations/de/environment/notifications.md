---
source_sha: f9e7107994608f10a03d81a80862011f9ceefaa236fe5b2f076b2dcdc7937d5a
reviewed: false
---
# E-Mail und Benachrichtigungen

Ausgehende E-Mails (Einladungen, Passwort-Zurücksetzungen, Benachrichtigungen)
und Browser-Push. Siehe [E-Mail und Benachrichtigungen](../notifications.md).

## E-Mail (SMTP)

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_SMTP_HOST` | leer | Der Mailserver. Leer: Es werden keine E-Mails gesendet. |
| `EVEY_SMTP_PORT` | `587` | Sein Port. |
| `EVEY_SMTP_USERNAME` | leer | Das Konto für die Anmeldung. |
| `EVEY_SMTP_PASSWORD` | leer | Sein Passwort. |
| `EVEY_SMTP_FROM` | leer | Die Absenderadresse, zum Beispiel `noreply@example.com`. |
| `EVEY_SMTP_TLS` | `starttls` | `starttls`, `ssl` oder `none`. |

Dieselben Einstellungen stehen unter **Admin → Serverkonfiguration → E-Mail / SMTP**.

## Browser-Push

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY` | erzeugt | Das Schlüsselpaar, das Push-Benachrichtigungen signiert. Fehlt es, erzeugt der Server beim ersten Start eines und speichert es in der Datenbank; setzen Sie sie nur, um ein bereits genutztes Paar zu behalten. |
| `VAPID_SUBJECT` | `mailto:admin@localhost` | Die Kontaktadresse, die Push-Dienste sehen. |
