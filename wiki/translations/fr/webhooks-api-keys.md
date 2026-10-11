---
source_sha: 09be615f048b42a5c91430ee4558b2bc906d1e19a1f0c51eb2577ffa9d579533
reviewed: true
---
# Webhooks et clés d'API

Deux façons de relier Evey à d'autres outils sans écrire de plugin : un
webhook envoie les événements du serveur vers une URL qui vous appartient,
une clé d'API permet à un script d'appeler le serveur en votre nom. Les deux
se créent par un administrateur dans le panneau d'administration ; cette
page s'adresse à la personne qui les met en place et à celle qui écrit le
récepteur ou le script.

## Envoyer les événements vers une URL

**Admin → Webhooks → Créer un webhook** : un **Nom**, une **URL**, les
**Événements** à envoyer, et un **Secret** facultatif pour la signature.
**Tester** envoie un ping et affiche la réponse.

<figure class="evey-shot" markdown="span">
  ![Un webhook dans Admin → Webhooks](images/admin-webhooks.webp){ loading=lazy }
  <figcaption>Un webhook qui envoie le début et la fin de chaque lecture.</figcaption>
</figure>

**Événements.** Un webhook s'abonne à **Tous les événements** ou à une liste
choisie. Le formulaire propose tous les événements que publie le serveur ;
parmi eux :

| Domaine | Événements |
| --- | --- |
| Bibliothèque | `library.scan.started`, `library.scan.completed`, `media.item.added`, `media.item.updated`, `media.item.removed`, `metadata.refreshed` |
| Lecture | `playback.started`, `playback.stopped`, `media.marked.watched`, `transcode.started`, `transcode.failed` |
| Comptes | `user.created`, `user.login.succeeded`, `user.login.failed`, `user.profile.created`, `user.apikey.created` |
| Demandes | `media.request.created`, `media.request.answered` |
| Serveur | `backup.created`, `backup.failed`, `plugin.installed`, `plugin.error`, `server.config.changed`, `transcoding.derived_cache_full` (stockage des sous-titres sous sa marge d'espace libre) |

Un plugin qui appelle `EmitEvent` publie `plugin.event.<type>`, qu'un
webhook abonné à **Tous les événements** reçoit aussi.

**Envoi.** Chaque événement est un `POST` JSON, accompagné de deux en-têtes :

- `X-Evey-Delivery` : un identifiant identique pour toutes les tentatives
  d'un même envoi, qui permet à votre récepteur d'ignorer un doublon ;
- `X-Evey-Signature` : `sha256=` suivi du HMAC-SHA256 du corps, calculé avec
  le secret du webhook. Vérifiez-le avec le même secret pour savoir que la
  requête vient bien de votre serveur. Sans secret, la signature ne prouve
  rien.

Chaque tentative attend une réponse pendant 30 secondes au plus. Un envoi qui
échoue est retenté après 1, 5 et 30 secondes. Chaque tentative est
consignée ; l'historique se lit par l'API (`ListWebhookDeliveries` dans
l'[API des plugins](api/plugins/README.md)), pas dans le panneau.

!!! warning "Adresses publiques uniquement"
    L'URL d'un webhook doit être en `http` ou `https` et pointer vers une
    adresse publique. `localhost`, les adresses privées et de lien local
    (`192.168.x.x`, `10.x.x.x`, `172.16.x.x`...), et un nom qui se résout
    vers l'une d'elles, sont refusés à l'enregistrement du webhook puis de
    nouveau au moment de la connexion. Cela empêche de se servir du serveur
    pour atteindre votre réseau interne ; un récepteur situé sur votre réseau
    local a besoin d'une adresse publique ou d'un relais.

## Appeler le serveur depuis un script

Une clé d'API permet à un script d'appeler l'API du serveur sans mot de
passe. Les clés se créent dans **Admin → Clés API → Nouvelle clé API**, par
un administrateur, et lui appartiennent.

- **La clé n'est affichée qu'une fois**, à sa création. Conservez-la à ce
  moment-là : elle ne peut plus être affichée, seulement révoquée avec
  **Révoquer**.
- **Elle agit au nom de son propriétaire**, avec des droits d'administrateur,
  dans la limite de ses permissions.
- **Les permissions** restreignent ce que la clé peut faire :

| Permission | Autorise |
| --- | --- |
| `media:read` | La lecture des métadonnées des médias |
| `media:write` | La modification des médias et le lancement de scans |
| `playback:read` | La lecture de l'état et de l'historique de lecture |
| `playback:write` | Le contrôle de la lecture et de la progression |
| `users:read` | La lecture des utilisateurs |
| `users:write` | La modification des utilisateurs |
| `admin:read` | La lecture de l'activité et des statistiques d'administration |
| `admin:write` | La modification des réglages d'administration |

Une clé créée sans aucune permission (**Toutes (legacy)**) a un accès complet
aux méthodes non administratives, comme son propriétaire, mais les méthodes
d'administration exigent toujours leur permission : une clé legacy ne les
atteint pas. Sélectionnez les permissions dont le script a besoin.

La clé se place dans l'en-tête `Authorization`, avec ou sans `Bearer` :

```bash
curl -H "Authorization: Bearer evey_..." https://media.example.com/api/v1/...
```

Une clé cesse de fonctionner dès qu'elle est révoquée, ou dès que son
propriétaire est désactivé. Les points d'accès eux-mêmes sont listés dans
l'[API du serveur](api/README.md), avec les autres façons de s'authentifier.

<!-- sources
form labels: i18n admin.webhooks.form.* (name, url, secret, events, allEvents), admin.webhooks.createWebhook, common.test, admin.webhooks.testPingSent
event list: frontend/packages/client/src/lib/webhookEvents.ts - WEBHOOK_EVENT_TYPES ; EventName() in internal/domain ; plugin events: internal/adapters/driving/pluginhost/host_server.go - pluginEmittedEvent.EventName
delivery, headers, timeout, retries, signature: internal/app/webhook/service.go - handleAny, attemptDeliver, sign, defaultRetryDelays, deliveryTimeout ; internal/domain/webhook/model.go - MatchesEvent ; history: proto/evey/v1/webhook.proto - ListWebhookDeliveries
public addresses only: internal/pkg/urlvalidator/validator.go - ValidateWebhookURL, isPrivateIP ; internal/pkg/ssrf/dialer.go
api keys: internal/adapters/driving/grpcapi/interceptors/auth.go - authenticateAPIKey (Bearer optional, evey_ prefix), authorize (admin methods always enforce the scope) ; internal/pkg/auth/rbac.go (scopes) ; rbac.go - CreateAPIKey RoleAdmin ; i18n auth.apiKeys.* (newKeyTitle, scopesAll, saveWarning, revoke)
-->
