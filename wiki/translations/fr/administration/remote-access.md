---
source_sha: b8448a636921ec7091b9ab98200600dd596b087d5b5d02b1d71230f1dbda6d3d
reviewed: true
---
# Accès distant et HTTPS

Chez vous, Evey fonctionne à une adresse comme `http://192.168.1.20:9091`.
Pour regarder depuis l'extérieur, il vous faut deux choses de plus :

- **Une adresse publique**, par exemple `https://evey.example.org` : un nom
  de domaine qui mène jusqu'à chez vous.
- **Le HTTPS**, pour que les mots de passe et les vidéos traversent Internet
  chiffrés. Evey ne fait pas le HTTPS lui-même. C'est un **reverse proxy**
  qui s'en charge : un petit programme placé devant Evey, qui gère la
  connexion sécurisée et transmet chaque demande à Evey.

Cette page montre comment publier le serveur, puis ce que fait chaque
réglage.

## Publier le serveur en trois étapes

1. **Installez un reverse proxy** qui gère le HTTPS et transmet tout au port
   `9091` d'Evey. Vous pouvez utiliser le Caddy fourni avec Evey, ou un proxy
   que vous avez déjà. Des configurations toutes prêtes :
   [Configurations de reverse proxy](../reverse-proxy.md).
2. **Indiquez-le à Evey** dans **Administration → Accès distant** :
    - **URL externe** : votre adresse publique en `https://`.
    - **Proxys de confiance (IP ou CIDR, séparés par des virgules)** :
      seulement quand le proxy tourne dans un autre conteneur. Mettez-y son
      réseau, par exemple `172.16.0.0/12` pour les réseaux Docker par
      défaut.
3. **C'est tout.** Dès qu'il a une adresse publique, le serveur ouvre l'accès
   distant de lui-même.

Pour vérifier, lancez `docker exec evey-server evey admin doctor`. Il montre
les proxys de confiance utilisés, et prévient quand un serveur publié refuse
les personnes de l'extérieur.

## L'accès distant se décide d'après l'adresse publique

Sauf si vous choisissez vous-même, Evey décide au démarrage :

- **Le serveur a une adresse publique** (**URL externe**, ou les variables
  `EVEY_SERVER_BASE_URL` ou `EVEY_REMOTE_ACCESS_EXTERNAL_URL`) : l'accès
  distant est **activé**.
- **Il n'en a pas** : l'accès distant est **désactivé**. Seuls les appareils
  du réseau domestique peuvent se connecter.

Dès que vous changez vous-même **Activer l'accès distant**, Evey garde votre
choix et arrête de décider à votre place. Un serveur publié peut donc rester
limité au réseau domestique si vous le voulez.

L'accès distant décide seulement qui peut joindre le serveur. Ce n'est pas
une protection à lui seul : chaque page demande toujours de se connecter.

**Quand l'accès distant est désactivé**, quelqu'un hors de chez vous est
refusé sur toutes les pages (une erreur `403`). Les applications affichent
**Accès distant désactivé** au lieu d'une erreur vague. Si le serveur peut
être joint depuis Internet alors que l'accès distant est désactivé, Evey vous
prévient à plusieurs endroits :

- **Administration → Accès distant** affiche **Tous les utilisateurs
  distants sont bloqués** ;
- les logs du serveur l'indiquent au démarrage, puis notent chaque refus avec
  l'adresse d'où il venait ;
- `evey admin doctor` le compte comme un échec.

## Réglages

Tout est dans **Administration → Accès distant**. Chaque réglage s'applique
sans redémarrage : les réglages réseau dès la demande suivante, le débit dès
la lecture suivante.

<figure class="evey-shot" markdown="span">
  ![Administration → Accès distant avec une URL externe en HTTPS et un proxy de confiance](../images/admin-remote-access.webp){ loading=lazy }
  <figcaption>Un serveur publié à une adresse en <code>https://</code>, derrière un proxy sur le réseau Docker.</figcaption>
</figure>

| Réglage | Ce qu'il fait |
| --- | --- |
| **Activer l'accès distant** | Activé : les personnes hors du réseau domestique peuvent se connecter comme tout le monde. Désactivé : ils sont refusés (`403`). |
| **URL externe** | Votre adresse publique. Les liens dans les e-mails, l'authentification unique et le cast l'utilisent. Une adresse en `https://` active aussi le mode HTTPS (plus bas). |
| **Proxys de confiance (IP ou CIDR, séparés par des virgules)** | Les reverse proxys auxquels Evey fait confiance quand ils indiquent qui est le vrai visiteur. La machine elle-même (boucle locale) est toujours de confiance. Vide : la liste vient de `EVEY_TRUSTED_PROXIES`. |
| **Réseaux LAN (CIDR, séparés par des virgules)** | Les plages d'adresses qu'Evey considère comme le réseau domestique. Les plages privées sont listées par défaut. Si vous utilisez un VPN (Tailscale, WireGuard) et voulez que ses utilisateurs comptent comme locaux, ajoutez sa plage ici. |
| **Débit distant maximal (Kbps, 0 = illimité)** | Une limite de qualité pour les personnes hors du réseau domestique. Une adresse privée n'est jamais limitée. Si **Administration → Configuration transcodage** fixe aussi une limite pour les spectateurs à distance, c'est la plus basse qui s'applique. Voir [Comment Evey choisit le mode de lecture](../playback/how-playback-is-decided.md#limits-on-the-session). |

## Ce qu'Evey doit savoir du proxy

Evey répond en HTTP simple sur le port 9091. Le reverse proxy gère le HTTPS.
Evey a besoin de savoir deux choses sur lui, toutes les deux dans
**Administration → Accès distant** :

- **URL externe** en `https://`. Cela dit à Evey qu'un proxy gère le HTTPS.
  Evey protège alors ses cookies de connexion (attribut `Secure`), demande
  aux navigateurs de toujours utiliser le HTTPS (HSTS), et fait confiance à l'en-tête
  `X-Forwarded-Proto` d'un proxy de confiance.
- **Les proxys de confiance**, quand le proxy n'est pas sur la même machine
  qu'Evey. Un proxy dans un autre conteneur a besoin de son réseau ici, par
  exemple `172.16.0.0/12` pour les réseaux Docker par défaut.

!!! warning "Une plage qui couvre toutes les adresses est refusée"
    `0.0.0.0/0` ou `::/0` comme proxy de confiance laisserait n'importe quel
    visiteur faire croire qu'il vient de n'importe où. Le panneau et
    `evey admin doctor` le refusent tous les deux. Indiquez plutôt l'adresse
    ou le réseau du proxy lui-même.

### Le régler plutôt dans l'environnement

Les variables `EVEY_SERVER_BASE_URL` et `EVEY_TRUSTED_PROXIES` font la même
chose depuis le fichier compose. C'est pratique pour une installation écrite
entièrement dans des fichiers, ou pour un serveur qui doit répondre en HTTPS
dès son tout premier démarrage.

Quand les deux sont réglés, le panneau l'emporte. Une **URL externe** en
`https://` active le mode HTTPS même si l'URL de base est en HTTP simple, et
une liste de proxys enregistrée remplace la variable. Videz le champ pour
revenir à la variable. Voir
[Serveur et réseau](../environment/server.md#remote-access).

## L'adresse réelle du client

Pour que l'accès distant et la limite de qualité à distance fonctionnent, Evey
doit voir la vraie adresse de chaque visiteur. Deux réglages travaillent
ensemble :

1. Les proxys de confiance déterminent à qui Evey fait confiance pour la vraie
   adresse du visiteur.
2. Les réseaux LAN décident ensuite si cette adresse est « locale » ou
   « distante ».

Si le proxy ne transmet pas la vraie adresse, ou si Evey ne lui fait pas
confiance, toutes les demandes semblent venir de l'adresse privée du proxy.
Tout le monde compte alors comme local : le réglage d'accès distant n'a
aucun effet, et aucune limite à distance ne s'applique.

!!! note "Corriger le proxy change qui compte comme à distance"
    Si l'accès distant était désactivé, les personnes de l'extérieur sont refusées
    dès que le proxy transmet leur vraie adresse. Lancez `evey admin doctor`
    après avoir changé les réglages du proxy : il montre la liste utilisée et
    d'où elle vient, prévient en cas de plage publique, et échoue sur une
    plage qui couvre toutes les adresses.

<!-- sources
inference and explicit choice: docs/adr/0026-remote-access-inferred-from-a-published-url.md ; internal/config/remote_access_resolve.go - ResolveRemoteAccess, IsPubliclyReachableURL
gate, live switch and LAN list: cmd/evey/gateway.go - networkMiddleware ; cmd/evey/network_middleware_test.go - TestNetworkMiddleware_LANListChangeAppliesWithoutRestart
remote cap live: internal/adapters/driving/grpcapi/playback_remote_bitrate_test.go
labels: i18n admin.remoteAccess.* (enableRemoteAccess, externalUrl, trustedProxies, lanNetworks, maxRemoteBitrate, lockoutRiskTitle)
proxy settings: docs/reverse-proxy.md
https mode and trusted proxies live: internal/config/network_live.go - EffectiveHTTPS, EffectiveTrustedProxies ; internal/adapters/driving/http/access_handler_live_test.go
catch-all range refused: internal/config/trusted_proxies.go - IsCatchAllProxyRange ; internal/config/network_live.go
doctor check: cmd/evey/admin_ops.go - checkTrustedProxies
exact field labels: i18n admin.remoteAccess.trustedProxies, lanNetworks, maxRemoteBitrate, lockoutRiskTitle ; apps message: i18n auth.remoteAccess.title
-->
