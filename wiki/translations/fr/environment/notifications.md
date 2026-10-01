---
source_sha: a2778ea0b85b4c03545d656feab9c42ce9cc78f8aa8a4a43beb5727f9eee0951
reviewed: true
---
# E-mail et notifications

Le courrier sortant (notifications, réinitialisations de mot de passe) et les
notifications push du navigateur. Voir
[E-mail et notifications](../notifications.md).

## E-mail (SMTP)

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_SMTP_HOST` | vide | Le serveur de courrier. Vide : aucun e-mail n'est envoyé. |
| `EVEY_SMTP_PORT` | `587` | Son port. |
| `EVEY_SMTP_USERNAME` | vide | Le compte avec lequel se connecter. |
| `EVEY_SMTP_PASSWORD` | vide | Son mot de passe. |
| `EVEY_SMTP_FROM` | vide | L'adresse d'expédition, par exemple `noreply@example.com`. |
| `EVEY_SMTP_TLS` | `starttls` | `starttls`, `ssl` ou `none`. |

Elles sont lues au démarrage du serveur. La carte **E-mail / SMTP** de **Admin →
Configuration du serveur** enregistre les mêmes champs, mais le serveur ne les
lit pas.

## Notifications push

| Variable | Défaut | Effet |
| --- | --- | --- |
| `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY` | générées | La paire de clés qui signe les notifications push. En leur absence, le serveur en génère une au premier démarrage et la stocke en base ; ne les définissez que pour conserver une paire déjà utilisée. |
| `VAPID_SUBJECT` | `mailto:admin@localhost` | L'adresse de contact que voient les services de push. |
