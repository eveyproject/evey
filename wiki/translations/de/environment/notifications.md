---
source_sha: a2778ea0b85b4c03545d656feab9c42ce9cc78f8aa8a4a43beb5727f9eee0951
reviewed: false
---
# E-Mail und Benachrichtigungen

Ausgehende E-Mails (Benachrichtigungen, Passwort-Zurücksetzungen) und
Browser-Push. Siehe [E-Mail und Benachrichtigungen](../notifications.md).

## E-Mail (SMTP)

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_SMTP_HOST` | leer | Der Mailserver. Leer: Es werden keine E-Mails gesendet. |
| `EVEY_SMTP_PORT` | `587` | Sein Port. |
| `EVEY_SMTP_USERNAME` | leer | Das Konto für die Anmeldung. |
| `EVEY_SMTP_PASSWORD` | leer | Sein Passwort. |
| `EVEY_SMTP_FROM` | leer | Die Absenderadresse, zum Beispiel `noreply@example.com`. |
| `EVEY_SMTP_TLS` | `starttls` | `starttls`, `ssl` oder `none`. |

Sie werden beim Start des Servers gelesen. Die Karte **E-Mail / SMTP** unter
**Admin → Serverkonfiguration** speichert dieselben Felder, aber der Server liest
sie nicht.

## Browser-Push

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY` | erzeugt | Das Schlüsselpaar, das Push-Benachrichtigungen signiert. Fehlt es, erzeugt der Server beim ersten Start eines und speichert es in der Datenbank; setzen Sie sie nur, um ein bereits genutztes Paar zu behalten. |
| `VAPID_SUBJECT` | `mailto:admin@localhost` | Die Kontaktadresse, die Push-Dienste sehen. |
