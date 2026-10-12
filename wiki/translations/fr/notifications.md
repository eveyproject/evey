---
source_sha: a8b7449b152c8718e3a2aef1de3cabe29af5dbe26aa1f77133448c996ce35d3c
reviewed: true
---
# E-mail et notifications

Evey prévient chaque profil quand de nouveaux films, séries, saisons,
épisodes ou albums arrivent. Il prévient aussi :

- **la personne qui a fait une demande**, quand elle reçoit une réponse ;
- **les administrateurs**, des nouvelles demandes, des sauvegardes qui ont
  échoué, des erreurs de plugins et d'un cache de sous-titres plein.

Les notifications apparaissent dans les applications sans rien régler. Par
e-mail, une fois que vous avez réglé un serveur de messagerie (plus bas).

## Où apparaissent les notifications

- **Dans chaque application** : la cloche, avec le nombre de notifications
  non lues. Toujours active. Une notification est gardée 30 jours. Chacun ne
  voit que les titres des bibliothèques que son compte peut ouvrir.
- **Dans le navigateur (push)** : sur le web, chaque profil active
  **Notifications push** dans **Paramètres → Notifications**, une fois par
  navigateur. Les navigateurs ne le permettent que sur une adresse en
  `https://` (ou `localhost`). Ailleurs, le réglage affiche **Les
  notifications push exigent une adresse HTTPS : ouvrez ce serveur en HTTPS
  pour les activer.** Les applications Android et TV ne reçoivent pas de
  notifications push.
- **Par e-mail** : une fois l'e-mail réglé, chaque notification est aussi
  envoyée à l'adresse e-mail du compte, dans la langue du profil, avec un
  lien vers le serveur. Un compte avec plusieurs profils reçoit un seul
  e-mail par événement. Personne ne peut couper les e-mails pour lui-même.

??? info "Clés de push"

    Le serveur crée les clés dont le push du navigateur a besoin à son
    premier démarrage. Pour fournir les vôtres, voir
    [E-mail et notifications](environment/notifications.md#browser-push).

## Régler l'e-mail

Evey envoie les e-mails par un serveur de messagerie où vous avez déjà un
compte : votre fournisseur d'e-mail, ou un service d'envoi. Les pages d'aide
de votre fournisseur donnent les valeurs ci-dessous.

1. Allez dans **Administration → Configuration du serveur → E-mail / SMTP**.
2. Remplissez les champs :

    | Champ | Ce qu'il faut taper |
    | --- | --- |
    | **Hôte SMTP**, **Port** | Le serveur de messagerie et son port. Les paires habituelles sont `587` avec `starttls`, ou `465` avec `ssl`. |
    | **Nom d'utilisateur**, **Mot de passe** | Le compte avec lequel Evey se connecte. |
    | **Adresse d'expédition** | L'adresse d'expéditeur affichée sur chaque message. |
    | **Mode TLS** | `starttls` ou `ssl`, selon le port (voir plus haut). `none` envoie sans chiffrement. |

3. Appuyez sur **Enregistrer les paramètres SMTP**. Cela s'applique tout de
   suite, sans redémarrage.
4. Appuyez sur **Envoyer un e-mail de test**. Evey envoie un court message à
   l'adresse de votre propre compte. En cas d'échec, il affiche la réponse du
   serveur de messagerie.

L'erreur la plus courante est un mode TLS qui ne correspond pas au port.

L'e-mail sert aux notifications et à la réinitialisation des mots de passe.
Les invitations sont des liens que vous envoyez vous-même.

??? info "Régler l'e-mail plutôt dans l'environnement"

    Les variables `EVEY_SMTP_*` ne servent que tant que la carte n'a pas
    d'hôte :

    ```env title=".env"
    EVEY_SMTP_HOST=smtp.example.com
    EVEY_SMTP_PORT=587
    EVEY_SMTP_USERNAME=notifications@example.com
    EVEY_SMTP_PASSWORD=your-smtp-password
    EVEY_SMTP_FROM=noreply@example.com
    EVEY_SMTP_TLS=starttls
    ```

    Dès qu'un hôte est enregistré dans la carte, tous les champs viennent de
    la carte et les variables sont ignorées. Ainsi, le serveur n'envoie
    jamais le mot de passe des variables à un serveur de messagerie tapé
    dans le panneau. Videz l'hôte pour revenir aux variables.

## Choisir ce qu'un profil reçoit

Dans **Paramètres → Notifications**, chaque profil choisit un niveau pour
**Films**, **Séries** et **Musique** :

| Niveau | Nouveaux titres dont vous êtes prévenu |
| --- | --- |
| **Toutes** | Tous. C'est le réglage par défaut. |
| **Pertinentes** | Seulement les titres qui ont un genre en commun avec ce que le profil regarde d'habitude. |
| **Importantes uniquement** | Pour l'instant, la même chose que Pertinentes. |
| **Désactivé** | Aucun. |

Un genre compte comme « regardé d'habitude » quand il apparaît dans au moins
3 des 200 dernières lectures du profil. Un titre sans genre n'est jamais
notifié aux niveaux Pertinentes et Importantes uniquement.

Quand plusieurs épisodes d'une même série arrivent en moins de 24 heures,
vous recevez une seule notification qui les compte. Aucune n'est envoyée
quand la série ou la saison vient d'être annoncée.

## Permettre de réinitialiser son mot de passe

1. Réglez l'e-mail (plus haut).
2. Activez **Récupération du mot de passe** dans **Administration →
   Configuration du serveur → Paramètres du serveur**. Cela s'applique tout
   de suite.
3. La page de connexion affiche alors **Mot de passe oublié ?**. Ce lien
   envoie par e-mail un lien pour choisir un nouveau mot de passe.

Sans serveur de messagerie, le lien reste caché, même avec l'option activée.

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
admin-only types include the subtitle cache: internal/domain/notification/model.go - IsAdminOnly (TypeCacheFull) ; i18n notifications push.cache_full
labels: i18n admin.serverConfig.port, smtpUsername, smtpPassword, smtpEnvHint ; auth.settings pushNotifications, pushNeedsHttps ; levelAll, levelRelevant, levelImportant, levelDisabled
no invitation e-mail: internal/adapters/driven/email (no invite sender)
-->
