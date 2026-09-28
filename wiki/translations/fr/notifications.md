---
source_sha: eea86697845889523ecbd07dd3d88da0143ac37f477404249051c73e160c80c8
---

# Notifications

Evey sait prévenir vos utilisateurs quand un média vient d'être ajouté, quand une sauvegarde échoue, et à l'occasion d'autres événements.

## Canaux

### Notifications dans l'application
Toujours actives. Une icône de cloche affiche le nombre de messages non lus. Aucune configuration n'est nécessaire.

### Courriel (SMTP)
Un courriel part dès qu'une notification est créée. Il faut pour cela configurer le SMTP :

```env title=".env"
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=notifications@example.com
EVEY_SMTP_PASSWORD=your-smtp-password
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls
```

Les modes TLS :
- `starttls` - la connexion s'ouvre en clair puis bascule en TLS (port 587, le cas le plus répandu)
- `ssl` - la connexion est chiffrée d'emblée (port 465)
- `none` - aucun chiffrement (déconseillé)

### Web Push
Les notifications poussées par le navigateur. Les clés VAPID sont générées automatiquement au premier démarrage, et chaque utilisateur y souscrit depuis les réglages de son profil.

## Récupération du mot de passe

Les courriels de réinitialisation réclament un SMTP configuré. Un administrateur doit en outre activer la fonction :

1. Configurez le SMTP (voir plus haut)
2. Dans le panneau d'administration, passez `auth.password_recovery_enabled` à `true` dans la configuration du serveur
3. La page de connexion affiche alors un lien « Mot de passe oublié ? »

Si le SMTP est retiré après coup, la récupération du mot de passe se désactive toute seule.

## Niveaux de notification

Chaque utilisateur règle sa préférence type de média par type de média :
- **Toutes** - être averti de tout
- **Pertinentes** - uniquement quand les genres correspondent à l'historique de visionnage
- **Importantes uniquement** - une correspondance de genres plus stricte
- **Désactivé** - aucune notification pour ce type
