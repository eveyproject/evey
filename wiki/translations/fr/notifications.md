---
source_sha: d27830800cb25ac8debd6abf1433f88730cdd94c9fabf03f504b714f41617c70
reviewed: true
---
# E-mail et notifications

Evey prévient chaque profil quand arrivent de nouveaux films, séries, saisons,
épisodes ou albums, dans les applications et, une fois l'e-mail configuré,
par e-mail. Cette page s'adresse à l'administrateur qui configure le serveur
de messagerie et à quiconque choisit ce que son profil reçoit. Les
administrateurs sont aussi avertis des sauvegardes en échec, des erreurs de
plugins et des nouvelles demandes de médias, et l'auteur d'une demande est
prévenu quand elle reçoit une réponse.

## Où apparaissent les notifications

- **Dans chaque application** : la cloche, avec le nombre de notifications non
  lues. Toujours là, rien à configurer. Une notification est conservée
  30 jours. La liste ne montre que les titres des bibliothèques auxquelles le
  compte a accès.
- **Push du navigateur** : sur le web, chaque profil active **Notifications
  push** dans **Paramètres → Notifications**, une fois par navigateur. Le
  serveur crée ses clés de push au premier démarrage ; voir
  [E-mail et notifications](environment/notifications.md#browser-push) pour
  fournir les vôtres. Les navigateurs n'autorisent le push que sur une adresse
  HTTPS (ou `localhost`) ; ailleurs, le réglage affiche **Les notifications
  push exigent une adresse HTTPS : ouvrez ce serveur en HTTPS pour les
  activer.** Les applications Android et TV ne reçoivent pas de notifications
  push.
- **E-mail** : une fois l'e-mail configuré, chaque notification est aussi
  envoyée à l'adresse du compte, dans la langue du profil, avec un lien vers le
  serveur. Un compte à plusieurs profils reçoit un seul e-mail par événement. Il
  n'y a pas de désinscription par utilisateur.

## Configurer l'e-mail

**Admin → Configuration du serveur → E-mail / SMTP** :

| Champ | Valeur |
| --- | --- |
| **Hôte SMTP**, **Port** | Le serveur de messagerie. `587` avec `starttls` ou `465` avec `ssl` sont les paires habituelles. |
| **Nom d'utilisateur**, **Mot de passe** | Le compte de connexion. |
| **Adresse d'expédition** | L'adresse d'expéditeur de chaque message. |
| **Mode TLS** | `starttls` passe une connexion en clair en TLS, `ssl` utilise TLS dès le départ, `none` envoie sans chiffrement. |

**Enregistrer les paramètres SMTP** s'applique aussitôt, sans redémarrage.
**Envoyer un e-mail de test** envoie un court message à l'adresse de votre
propre compte avec les réglages enregistrés, et affiche la réponse du serveur
de messagerie en cas d'échec.

Les variables `EVEY_SMTP_*` servent de repli, tant que la carte n'a pas d'hôte :

```env title=".env"
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=notifications@example.com
EVEY_SMTP_PASSWORD=your-smtp-password
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls
```

!!! note "Une seule source à la fois"
    Dès qu'un hôte est enregistré dans la carte, tous les champs viennent de
    la carte et les variables sont ignorées : le serveur n'envoie donc jamais
    les identifiants des variables à un serveur de messagerie saisi dans le
    panneau. Videz l'hôte pour revenir aux variables.

L'e-mail sert aux notifications et aux réinitialisations de mot de passe ; les
invitations sont des liens que vous partagez vous-même.

## Choisir ce qu'un profil reçoit

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

## Permettre la réinitialisation du mot de passe

1. Configurez l'e-mail (ci-dessus).
2. Activez **Récupération du mot de passe** dans **Admin → Configuration du
   serveur → Paramètres du serveur**. Le réglage s'applique immédiatement.
3. La page de connexion affiche alors **Mot de passe oublié ?**, qui envoie un
   lien de réinitialisation par e-mail.

Sans serveur de messagerie, le lien reste masqué, même avec l'option activée.

<!-- sources
web push support: frontend/apps/web/src/lib/webpush.ts isPushSupported (serviceWorker + PushManager, secure context only) ; NotificationSettings.tsx shows pushNeedsHttps when unsupported
types: internal/domain/notification/model.go (Type*, IsAdminOnly: backup_failed, plugin_error, media_request) ; emitted: internal/app/notification/event_handlers.go, batching.go
retention 30 d: internal/app/notification/service.go defaultRetentionDays ; grouping 24 h: batching.go episodeGroupWindow + shouldSkipEpisodeNotif
list filtered by library access: internal/adapters/driving/grpcapi/notification_handler.go allowedLibraryIDs
bell on TV and mobile: frontend/apps/tv/src/components/TopHeader.tsx ; frontend/apps/mobile/app/(auth)/settings/notifications.tsx
levels: batching.go shouldNotify (relevant == highly_relevant) + fetchFavoriteGenres (GetFavoriteGenresBatch minCount 3, window 200 sessions: watch_history_analyzer.go)
defaults all: migrations/000005_notifications.up.sql (notification_settings_global) ; UI: frontend/apps/web/src/features/user/NotificationSettings.tsx ; i18n notificationSettings.level*
email: internal/adapters/driven/email/notification_emailer.go (push title key, recipient locale, one per account per FanoutID, base URL link)
SMTP live from the card, env fallback, panel host = panel only, defaults 587/starttls: internal/app/config/smtp.go (ResolveSMTPSettings, LiveSMTP) ; test send: internal/adapters/driving/http/smtp_test_handler.go
labels: i18n admin.serverConfig emailSmtp, smtpHost, port, smtpUsername, smtpPassword, fromAddress, tlsMode, saveSmtp, sendTestEmail
no native push in the mobile/TV apps (no push client) ; VAPID generated: cmd/evey/wire_helpers.go
password recovery: admin.serverSettings.passwordRecovery ; gated on smtpConfigured (cmd/evey/wire.go) ; auth.login.forgotPassword
-->
