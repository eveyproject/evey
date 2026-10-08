---
source_sha: 1632ea5df6d8e43d0a8895c8efa853e83316a37433cb8b556d00d317a9ee4951
reviewed: true
---
# Serveur et réseau

Les variables qui disent où le serveur est publié, quels proxys il croit, qui
peut le joindre de l'extérieur et combien de requêtes il accepte. La plupart
se définissent une fois dans le fichier compose ; ce que **Admin → Accès
distant** enregistre ensuite l'emporte sur elles, comme l'explique la
[Référence de configuration](../configuration.md).

## Adresse et ports

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_SERVER_BASE_URL` | `http://localhost:9091` | L'adresse publique. Une adresse en `https://` active les cookies `Secure`, HSTS et la confiance dans `X-Forwarded-Proto`. Les liens des e-mails, les redirections d'authentification unique et les URL de diffusion en sont dérivés. | Accès distant → **URL externe** l'emporte quand elle est renseignée, immédiat ; une URL externe en `https://` active aussi le HTTPS |
| `EVEY_SERVER_PORT` | `9090` | Le port gRPC interne. L'API HTTP et l'interface web écoutent sur ce port plus un (`9091`). | aucun |
| `EVEY_GRPC_BIND` | `127.0.0.1` | L'adresse d'écoute du port gRPC. Le contrôle d'accès distant ne couvre pas gRPC : l'élargir (`0.0.0.0`) affiche un avertissement, et le filtrer devient votre responsabilité. | aucun |
| `EVEY_SERVER_NAME` | `Evey` | Le nom affiché sur la page de connexion, dans la liste des serveurs des applications et dans les e-mails. | Configuration du serveur → Paramètres du serveur → **Nom du serveur**, immédiat |

## Reverse proxy

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_TRUSTED_PROXIES` | boucle locale seulement | Les réseaux dont l'adresse client transmise est crue, séparés par des virgules (pour un proxy en conteneur : `172.16.0.0/12`, à resserrer une fois connu). S'ajoutent à la boucle locale, sans jamais la remplacer ; une entrée mal formée empêche le démarrage. | Accès distant → **Proxys de confiance** remplace cette liste, immédiat ; vider le champ rétablit la variable |
| `EVEY_REAL_IP_HEADER` | `X-Forwarded-For` | L'en-tête dans lequel un proxy de confiance place l'adresse du client. | aucun |
| `EVEY_FORWARDED_PROTO` | déduite | Obsolète : le HTTPS se déduit du schéma de `EVEY_SERVER_BASE_URL`. Encore acceptée pour forcer la valeur, avec un avertissement. | aucun |

Les réglages du proxy qui comptent pour la diffusion : [Configurations de reverse proxy](../reverse-proxy.md).

## Accès distant

Ces variables servent au premier démarrage ; **Admin → Accès distant**
enregistre ensuite ses propres valeurs et les applique sans redémarrage. La
façon dont le contrôle décide :
[Accès distant et HTTPS](../administration/remote-access.md).

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_REMOTE_ACCESS_ENABLED` | non définie | Non définie : déduite au démarrage, activée quand `EVEY_SERVER_BASE_URL` ou `EVEY_REMOTE_ACCESS_EXTERNAL_URL` est publique. `true` ou `false` : votre choix explicite, jamais remplacé. | Accès distant → **Activer l'accès distant**, immédiat |
| `EVEY_REMOTE_ACCESS_EXTERNAL_URL` | vide | L'adresse publique, quand elle diffère de `EVEY_SERVER_BASE_URL`. Une adresse en `https://` active le mode HTTPS, comme l'URL de base. | Accès distant → **URL externe**, immédiat |
| `EVEY_REMOTE_ACCESS_LAN_NETWORKS` | `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16` | Les réseaux considérés comme la maison. Ajoutez une plage de VPN ou de CGNAT (`100.64.0.0/10`) dont les clients doivent compter comme locaux. | Accès distant → **Réseaux LAN**, immédiat |
| `EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS` | `0` | Un plafond de débit pour les clients hors de la maison, en kbit/s ; `0` = aucun. Une adresse privée n'est jamais plafonnée. | Accès distant → **Débit distant maximal**, immédiat |

## Limites de requêtes

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_RATELIMIT_LOGIN_MAX` | `10` | Tentatives de connexion par adresse et par minute. | Configuration du serveur → Paramètres du serveur → **Limite connexion**, immédiat |
| `EVEY_RATELIMIT_GLOBAL_MAX` | `300` | Requêtes par adresse et par minute sur l'API. | Configuration du serveur → Paramètres du serveur → **Limite globale**, immédiat |

## Utilisateur du conteneur

Lues par l'entrypoint, pas par le serveur.

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `PUID`, `PGID` | propriétaire des montages `/media` | L'utilisateur et le groupe sous lesquels tourne le serveur. `1001` est refusé (réservé aux plugins). Détails dans la [Référence de configuration](../configuration.md#file-ownership-puid-and-pgid). | aucun |

## Démo publique

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_DEMO` | `false` | Fait tourner le serveur comme démo publique : écritures d'administration verrouillées, ni e-mail, ni push, ni webhooks sortants. Pas pour une vraie installation. | aucun |
