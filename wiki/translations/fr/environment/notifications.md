---
source_sha: f9e7107994608f10a03d81a80862011f9ceefaa236fe5b2f076b2dcdc7937d5a
reviewed: true
---
# E-mail et notifications

Le courrier sortant (invitations, réinitialisations de mot de passe,
notifications) et les notifications push du navigateur. Voir
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

Les mêmes réglages se trouvent dans **Admin → Configuration du serveur →
E-mail / SMTP**.

## Notifications push

| Variable | Défaut | Effet |
| --- | --- | --- |
| `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY` | générées | La paire de clés qui signe les notifications push. En leur absence, le serveur en génère une au premier démarrage et la stocke en base ; ne les définissez que pour conserver une paire déjà utilisée. |
| `VAPID_SUBJECT` | `mailto:admin@localhost` | L'adresse de contact que voient les services de push. |
