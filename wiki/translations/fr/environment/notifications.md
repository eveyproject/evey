---
source_sha: 7cc0fb43b6f9e14980a2332dd94e4c79bea3397ef66018840bb68f8b90a78ca2
reviewed: true
---
# E-mail et notifications

Les variables qui règlent le courrier sortant (notifications, réinitialisations
de mot de passe) et les notifications push du navigateur. L'e-mail se
configure normalement dans le panneau, comme décrit dans
[E-mail et notifications](../notifications.md) ; les variables en sont le repli.

## E-mail (SMTP)

Les variables sont lues au démarrage et utilisées tant que la carte
**E-mail / SMTP** de **Administration → Configuration du serveur** n'a pas d'hôte. Dès
qu'un hôte y est enregistré, la carte fournit tous les champs et s'applique
sans redémarrage.

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_SMTP_HOST` | vide | Le serveur de courrier. Vide : aucun e-mail n'est envoyé. | Configuration du serveur → E-mail / SMTP → **Hôte SMTP**, immédiat |
| `EVEY_SMTP_PORT` | `587` | Son port. | même carte → **Port**, immédiat |
| `EVEY_SMTP_USERNAME` | vide | Le compte avec lequel se connecter. | même carte → **Nom d'utilisateur**, immédiat |
| `EVEY_SMTP_PASSWORD` | vide | Son mot de passe. | même carte, immédiat |
| `EVEY_SMTP_FROM` | vide | L'adresse d'expédition, par exemple `noreply@example.com`. | même carte, immédiat |
| `EVEY_SMTP_TLS` | `starttls` | `starttls`, `ssl` ou `none`. | même carte, immédiat |

## Notifications push

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY` | générées | La paire de clés qui signe les notifications push. En leur absence, le serveur en génère une au premier démarrage et la stocke en base ; ne les définissez que pour conserver une paire déjà utilisée, et toujours les deux : une seule désactive le push avec un avertissement. | aucun |
| `VAPID_SUBJECT` | `mailto:admin@localhost` | L'adresse de contact que voient les services de push. | aucun |
