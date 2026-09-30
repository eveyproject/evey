---
source_sha: f8b9121424daaf442c366f75a6973b21de77d280d45f2cb7e7dadab38b7328be
reviewed: true
---
# Webhooks et clés d'API

Deux façons de relier Evey à d'autres outils sans écrire de plugin : les
webhooks envoient les événements d'Evey vers une URL qui vous appartient, les
clés d'API permettent à vos scripts d'appeler Evey.

## Webhooks

**Admin → Webhooks → Créer un webhook** : un nom, une URL, les événements à
envoyer, et un secret facultatif. **Tester** envoie un ping et affiche la
réponse.

<figure class="evey-shot" markdown="span">
  ![Un webhook dans Admin → Webhooks](images/admin-webhooks.webp){ loading=lazy }
  <figcaption>Un webhook qui envoie le début et la fin de chaque lecture.</figcaption>
</figure>

**Événements.** Un webhook s'abonne à **Tous les événements** ou à une liste
choisie. Ce sont les événements du serveur lui-même, par exemple :

| Domaine | Événements |
| --- | --- |
| Bibliothèque | `library.scan.started`, `library.scan.completed`, `media.item.added`, `media.item.updated`, `media.item.removed` |
| Lecture | `playback.started`, `playback.stopped`, `media.marked.watched`, `transcode.started`, `transcode.failed` |
| Comptes | `user.created`, `user.login.succeeded`, `user.login.failed`, `user.profile.created` |
| Demandes | `media.request.created`, `media.request.answered` |
| Serveur | `backup.created`, `backup.failed`, `plugin.installed`, `plugin.error`, `server.config.changed` |

**Envoi.** Chaque événement est un `POST` JSON, accompagné de deux en-têtes :

- `X-Evey-Delivery` : un identifiant identique pour toutes les tentatives d'un
  même envoi, qui permet à votre récepteur d'ignorer un doublon ;
- `X-Evey-Signature` : `sha256=` suivi du HMAC-SHA256 du corps, calculé avec
  le secret du webhook. Vérifiez-le avec le même secret pour savoir que la
  requête vient bien de votre serveur. Sans secret, la signature ne prouve
  rien.

Un envoi qui échoue est retenté après 1, 5 et 30 secondes. Chaque tentative
est consignée dans le journal d'envoi du webhook.

**Adresses publiques uniquement.** L'URL d'un webhook doit être en `http` ou
`https` et pointer vers une adresse publique. Les adresses privées et locales
(`192.168.x.x`, `10.x.x.x`, `localhost`, un nom de conteneur qui se résout vers
l'une d'elles) sont refusées, à l'enregistrement du webhook puis de nouveau au
moment de la connexion. Cela empêche de se servir du serveur pour atteindre
votre réseau interne ; un récepteur situé sur votre réseau local a besoin
d'une adresse publique ou d'un relais.

## Clés d'API

Une clé d'API permet à un script d'appeler l'API du serveur sans mot de passe.
Les clés se créent dans **Admin → Clés API**, par un administrateur, et lui
appartiennent.

- **La clé n'est affichée qu'une fois**, à sa création. Conservez-la à ce
  moment-là : elle ne peut plus être affichée, seulement révoquée.
- **Elle agit au nom de son propriétaire**, avec des droits
  d'administrateur, dans la limite de ses portées.
- **Les portées** restreignent ce que la clé peut faire :

| Portée | Autorise |
| --- | --- |
| `media:read` | La lecture des métadonnées des médias |
| `media:write` | La modification des médias et le lancement de scans |
| `playback:read` | La lecture de l'état et de l'historique de lecture |
| `playback:write` | Le contrôle de la lecture et de la progression |
| `users:read` | La lecture des utilisateurs |
| `users:write` | La modification des utilisateurs |
| `admin:read` | La lecture de l'activité et des statistiques d'administration |
| `admin:write` | La modification des réglages d'administration |

**Une clé créée sans aucune portée a un accès complet**, comme son
propriétaire : c'est l'ancien comportement, conservé pour les clés existantes.
Sélectionnez les portées dont le script a besoin.

La clé se place dans l'en-tête `Authorization`, avec ou sans `Bearer` :

```bash
curl -H "Authorization: Bearer evey_..." https://media.example.com/api/v1/...
```

Une clé cesse de fonctionner dès qu'elle est révoquée, ou dès que son
propriétaire est désactivé. Les points d'accès eux-mêmes sont listés dans
l'[API du serveur](api/README.md).
