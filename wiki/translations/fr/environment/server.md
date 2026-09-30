---
source_sha: d3bbf5c5d256e29ed7bd2108d72166e0906938ea2e32684ab5496cdb6bb7755e
reviewed: true
---
# Serveur et réseau

L'adresse à laquelle le serveur est publié, les proxys auxquels il fait
confiance, qui peut le joindre de l'extérieur, et ses limites de requêtes.

## Adresse et ports

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_SERVER_BASE_URL` | `http://localhost:9091` | L'adresse publique. Une adresse en `https://` active les cookies `Secure`, HSTS et la confiance dans `X-Forwarded-Proto`. Les liens des e-mails, les redirections d'authentification unique et les URL de diffusion en sont dérivés, sauf si **Administration → Accès distant → URL externe** est renseignée : c'est elle qui l'emporte. Une URL externe en `https://` active aussi le HTTPS quand celle-ci est en HTTP simple. |
| `EVEY_SERVER_PORT` | `9090` | Le port gRPC interne. L'API HTTP et l'interface web écoutent sur ce port plus un (`9091`). |
| `EVEY_GRPC_BIND` | `127.0.0.1` | L'adresse d'écoute du port gRPC. Le contrôle d'accès distant ne couvre pas gRPC : l'élargir (`0.0.0.0`) affiche un avertissement, et le filtrer devient votre responsabilité. |
| `EVEY_SERVER_NAME` | `Evey` | Le nom affiché sur la page de connexion, dans la liste des serveurs des applications et dans les e-mails. **Admin → Configuration du serveur** le remplace. |

## Reverse proxy

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_TRUSTED_PROXIES` | boucle locale seulement | Les réseaux dont l'adresse client transmise est crue, séparés par des virgules (pour un proxy en conteneur : `172.16.0.0/12`, à resserrer une fois connu). S'ajoutent à la boucle locale, sans jamais la remplacer. Une liste enregistrée dans **Administration → Accès distant → Proxys de confiance** remplace celle-ci à chaud ; vider le champ la rétablit. |
| `EVEY_REAL_IP_HEADER` | `X-Forwarded-For` | L'en-tête dans lequel un proxy de confiance place l'adresse du client. |
| `EVEY_FORWARDED_PROTO` | déduite | Obsolète : le HTTPS se déduit du schéma de `EVEY_SERVER_BASE_URL`. Encore acceptée pour forcer la valeur, avec un avertissement. |

## Accès distant

Ces variables servent au premier démarrage ; **Admin → Accès distant**
enregistre ensuite ses propres valeurs et les applique sans redémarrage.

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_REMOTE_ACCESS_ENABLED` | non définie | Non définie : déduite au démarrage, activée quand `EVEY_SERVER_BASE_URL` ou `EVEY_REMOTE_ACCESS_EXTERNAL_URL` est publique. `true` ou `false` : votre choix explicite, jamais remplacé. |
| `EVEY_REMOTE_ACCESS_EXTERNAL_URL` | vide | L'adresse publique, quand elle diffère de `EVEY_SERVER_BASE_URL`. Une adresse en `https://` active le mode HTTPS, comme l'URL de base. |
| `EVEY_REMOTE_ACCESS_LAN_NETWORKS` | `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16` | Les réseaux considérés comme la maison. Ajoutez une plage de VPN ou de CGNAT (`100.64.0.0/10`) dont les clients doivent compter comme locaux. |
| `EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS` | `0` | Un plafond de débit pour les clients hors de la maison, en kbit/s ; `0` = aucun. Une adresse privée n'est jamais plafonnée. |

Le fonctionnement du contrôle d'accès : [Accès distant et HTTPS](../administration/remote-access.md).

## Limites de requêtes

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_RATELIMIT_LOGIN_MAX` | `10` | Tentatives de connexion par adresse et par minute. **Admin → Configuration du serveur → Paramètres du serveur** la remplace. |
| `EVEY_RATELIMIT_GLOBAL_MAX` | `300` | Requêtes par adresse et par minute sur l'API. Remplaçable au même endroit. |

## Utilisateur du conteneur

Lues par l'entrypoint, pas par le serveur.

| Variable | Défaut | Effet |
| --- | --- | --- |
| `PUID`, `PGID` | propriétaire des montages `/media` | L'utilisateur et le groupe sous lesquels tourne le serveur. `1001` est refusé (réservé aux plugins). Détails dans la [Référence de configuration](../configuration.md#file-ownership-puid-and-pgid). |

## Démo publique

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_DEMO` | `false` | Fait tourner le serveur comme démo publique : écritures d'administration verrouillées, ni e-mail, ni push, ni webhooks sortants. Pas pour une vraie installation. |
