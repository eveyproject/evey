---
source_sha: 0f3c964e32d232266d53c9ebfe192d6a3f21996e2437d7ec2cc38945c049774c
reviewed: true
---
# Notifications

Evey prévient chaque profil quand arrivent de nouveaux films, séries, saisons,
épisodes ou albums. Les administrateurs sont aussi avertis des sauvegardes en
échec, des erreurs de plugins et des nouvelles demandes de médias, et l'auteur
d'une demande est prévenu quand elle reçoit une réponse.

## Où apparaissent les notifications

- **Dans chaque application** : la cloche, avec le nombre de notifications non
  lues. Toujours là, rien à configurer. Une notification est conservée 30 jours. La liste
  ne montre que les titres des bibliothèques auxquelles le compte a accès.
- **Push du navigateur** : sur le web, chaque profil active **Notifications
  push** dans **Paramètres → Notifications**, une fois par navigateur. Le
  serveur crée ses clés de push au premier démarrage ; voir
  [E-mail et notifications](environment/notifications.md#browser-push) pour
  fournir les vôtres. Les navigateurs n'autorisent le push que sur une adresse
  HTTPS (ou `localhost`) ; ailleurs, le réglage affiche **Notifications push non
  configurées sur ce serveur.** Les applications Android et TV ne reçoivent pas
  de notifications push.
- **E-mail** : une fois le SMTP configuré, chaque notification est aussi
  envoyée à l'adresse du compte. Il n'y a pas de désinscription par
  utilisateur, et un compte à plusieurs profils reçoit un e-mail par profil.

## Configurer l'e-mail

Le SMTP est lu dans ces variables au démarrage du serveur ; redémarrez-le après
une modification :

```env title=".env"
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=notifications@example.com
EVEY_SMTP_PASSWORD=your-smtp-password
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls
```

`EVEY_SMTP_TLS` vaut `starttls` (une connexion en clair passée en TLS,
généralement port 587), `ssl` (TLS dès le départ, généralement port 465) ou
`none` (sans chiffrement). La carte **E-mail / SMTP** de **Admin →
Configuration du serveur** enregistre les mêmes champs, mais le serveur ne les
lit pas : utilisez les variables.

L'e-mail sert aux notifications et aux réinitialisations de mot de passe ; les
invitations sont des liens que vous partagez vous-même.

## Ce que reçoit chaque profil

Dans **Paramètres → Notifications**, chaque profil choisit un niveau pour
**Films**, **Séries** et **Musique** :

| Niveau | Nouveaux titres notifiés |
| --- | --- |
| **Toutes** | Tous. Le réglage par défaut. |
| **Pertinentes** | Seulement ceux qui partagent un genre avec les genres habituels du profil. |
| **Importantes uniquement** | Pour l'instant identique à Pertinentes. |
| **Désactivé** | Aucun. |

Un genre compte comme habituel quand il apparaît dans au moins 3 des 200
dernières lectures du profil. Un titre sans genre n'est jamais notifié aux
niveaux Pertinentes et Importantes uniquement.

Plusieurs épisodes d'une même série arrivés en moins de 24 heures donnent une
seule notification qui les compte, et aucune n'est envoyée quand la série ou la
saison vient d'être annoncée.

## Récupération du mot de passe

1. Configurez l'e-mail (voir plus haut).
2. Activez **Récupération du mot de passe** dans **Admin → Configuration du
   serveur → Paramètres du serveur**. Le réglage s'applique immédiatement.
3. La page de connexion affiche alors **Mot de passe oublié ?**, qui envoie un
   lien de réinitialisation par e-mail.

Sans SMTP, le lien reste masqué, même avec l'option activée.
