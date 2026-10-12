---
source_sha: 7cc0fb43b6f9e14980a2332dd94e4c79bea3397ef66018840bb68f8b90a78ca2
reviewed: true
---
# E-Mail und Benachrichtigungen

Die Variablen hinter ausgehenden E-Mails (Benachrichtigungen,
Passwort-Zurücksetzungen) und Browser-Push. E-Mail wird normalerweise im
Verwaltungsbereich eingerichtet, wie unter
[E-Mail und Benachrichtigungen](../notifications.md) beschrieben. Die
Variablen sind der Ersatz, wenn dort nichts eingetragen ist.

## E-Mail (SMTP)

Die Variablen werden beim Start gelesen und gelten, solange die Karte **E-Mail
/ SMTP** unter **Verwaltung → Serverkonfiguration** keinen Host hat. Sobald
dort ein Host gespeichert ist, liefert die Karte jedes Feld und gilt ohne
Neustart.

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_SMTP_HOST` | leer | Der Mailserver. Leer: Es wird keine E-Mail verschickt. | Serverkonfiguration → E-Mail / SMTP → **SMTP-Host**, sofort |
| `EVEY_SMTP_PORT` | `587` | Sein Port. | dieselbe Karte → **Port**, sofort |
| `EVEY_SMTP_USERNAME` | leer | Das Konto für die Anmeldung. | dieselbe Karte → **Benutzername**, sofort |
| `EVEY_SMTP_PASSWORD` | leer | Sein Passwort. | dieselbe Karte, sofort |
| `EVEY_SMTP_FROM` | leer | Die Absenderadresse, zum Beispiel `noreply@example.com`. | dieselbe Karte, sofort |
| `EVEY_SMTP_TLS` | `starttls` | `starttls`, `ssl` oder `none`. | dieselbe Karte, sofort |

## Browser-Push

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY` | erzeugt | Das Schlüsselpaar, das Push-Benachrichtigungen signiert. Fehlen sie, erzeugt der Server beim ersten Start ein Paar und speichert es in der Datenbank. Setzen Sie sie nur, um ein Paar zu behalten, das Sie schon verwenden, und immer beide: Nur einer allein schaltet Push mit einer Warnung ab. | keine |
| `VAPID_SUBJECT` | `mailto:admin@localhost` | Die Kontaktadresse, die Push-Dienste sehen. | keine |

<!-- sources
internal/config/config.go - EVEY_SMTP_*, VAPID_* ; WarnValidate (one VAPID key alone)
SMTP card wins, env fallback: internal/app/config/smtp.go ResolveSMTPSettings ; i18n admin.serverConfig.emailSmtp, smtpHost, port, smtpUsername
vapid generation: cmd/evey/wire_helpers.go - resolveVAPIDKeys (vapid.public_key / vapid.private_key)
-->
