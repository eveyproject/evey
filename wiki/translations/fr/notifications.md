---
source_sha: f905b0c655ef0e8732ff61ae6298621532868128cd52c20af3e88b2f5592f0de
reviewed: true
---
# Notifications

Evey peut prévenir les utilisateurs quand un média est ajouté, quand une
sauvegarde échoue, et lors d'autres événements.

## Canaux

### Dans l'application
Toujours actives. Une cloche affiche le nombre de notifications non lues. Rien à
configurer.

### E-mail (SMTP)
Envoie un e-mail à chaque notification créée. Demande une configuration SMTP :

```env title=".env"
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=notifications@example.com
EVEY_SMTP_PASSWORD=your-smtp-password
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls
```

Modes TLS :

- `starttls` : connexion en clair, puis passage en TLS (port 587, le plus
  courant) ;
- `ssl` : connexion directement en TLS (port 465) ;
- `none` : aucun chiffrement (déconseillé).

### Notifications push du navigateur
Les clés VAPID sont générées automatiquement au premier démarrage. Chaque
utilisateur les active depuis les réglages de son profil.

## Récupération du mot de passe

L'e-mail de réinitialisation du mot de passe demande que le SMTP soit configuré.
Un administrateur doit aussi activer la fonction :

1. Configurez le SMTP (voir plus haut).
2. Enregistrez le réglage `auth.password_recovery_enabled` = `true`. Le panneau
   n'a pas d'interrupteur pour cela : un administrateur l'envoie par l'API,
   `PUT /api/v1/admin/config` avec
   `{"values": {"auth.password_recovery_enabled": "true"}}`.
3. La page de connexion affiche alors un lien **Mot de passe oublié ?**.

Si le SMTP est retiré après coup, la récupération du mot de passe se désactive
d'elle-même.

## Niveaux de notification

Chaque utilisateur choisit son niveau de notification par type de média :

- **Toutes** : tout est notifié ;
- **Pertinentes** : seulement si les genres correspondent à son historique de
  visionnage ;
- **Importantes uniquement** : une correspondance de genres plus stricte ;
- **Désactivé** : aucune notification pour ce type.
