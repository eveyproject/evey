---
source_sha: 220a459856f5f8a191fe5d802245a536ff4008655f388f910c0cbda05b4d0bc9
reviewed: true
---
# Accès distant et HTTPS

Publiez le serveur à une adresse en `https://` et les membres du foyer
l'utilisent de n'importe où. Cette page s'adresse à l'administrateur : elle
couvre l'adresse publique, l'interrupteur d'accès distant que le serveur règle
de lui-même, et ce qu'Evey doit savoir du reverse proxy placé devant lui.

## Publier le serveur en trois étapes

1. Placez devant Evey un reverse proxy qui termine le HTTPS et transmet tout
   au port `9091` : le Caddy fourni, ou un proxy que vous avez déjà. Les
   configurations prêtes à l'emploi sont dans
   [Configurations de reverse proxy](../reverse-proxy.md).
2. Dans **Admin → Accès distant**, renseignez l'**URL externe** en `https://`
   et, quand le proxy tourne dans un autre conteneur, son réseau dans
   **Proxys de confiance** (`172.16.0.0/12` pour les réseaux Docker par
   défaut).
3. Rien d'autre : un serveur qui a reçu une adresse publique ouvre l'accès
   distant tout seul. `evey admin doctor` confirme la liste de proxys en
   vigueur et avertit quand un serveur publié refuse les clients distants.

## L'accès distant se déduit de l'adresse publique

Tant que vous n'avez rien choisi, Evey décide de l'accès distant au démarrage :

- si le serveur a reçu une adresse publique (**URL externe**,
  `EVEY_SERVER_BASE_URL` ou `EVEY_REMOTE_ACCESS_EXTERNAL_URL`), l'accès
  distant est **activé** ;
- sinon il est **désactivé**, et seuls les clients de vos réseaux locaux sont
  servis.

Dès que vous modifiez vous-même **Activer l'accès distant**, votre choix est
conservé et le serveur cesse de le déduire : un serveur publié peut rester
limité au réseau local si c'est ce que vous voulez. L'accès distant est un
choix sur qui peut joindre le serveur, pas une protection : chaque page exige
toujours une connexion.

Quand l'accès distant est désactivé, un client hors des réseaux locaux reçoit
une erreur `403` sur toutes les pages, et les applications affichent un
message indiquant que l'accès distant est désactivé plutôt qu'une erreur
générique. Si le serveur est joignable depuis Internet alors que l'accès
distant est désactivé, la page d'administration affiche **Tous les
utilisateurs distants sont bloqués**, le serveur le signale dans ses logs au
démarrage et journalise chaque refus avec l'adresse du client, et
`evey admin doctor` le compte comme un échec.

## Réglages

**Admin → Accès distant**. Tous les réglages s'appliquent sans redémarrage :
ceux du réseau à la requête suivante, le débit à la lecture suivante.

<figure class="evey-shot" markdown="span">
  ![Admin → Accès distant avec une URL externe en HTTPS et un proxy de confiance](../images/admin-remote-access.webp){ loading=lazy }
  <figcaption>Un serveur publié à une adresse en <code>https://</code>, derrière un proxy sur le réseau Docker.</figcaption>
</figure>

| Réglage | Effet |
| --- | --- |
| **Activer l'accès distant** | Activé : les clients hors des réseaux locaux sont servis comme les autres. Désactivé : ils reçoivent une erreur `403`. |
| **URL externe** | L'adresse publique. Les liens des e-mails, les redirections d'authentification unique et les URL de diffusion en sont dérivés. Une adresse en `https://` active aussi le mode HTTPS (voir plus bas). |
| **Proxys de confiance** | Les reverse proxys dont Evey croit l'adresse client transmise. La boucle locale est toujours de confiance. Champ vide : la liste vient de `EVEY_TRUSTED_PROXIES`. |
| **Réseaux LAN** | Les plages d'adresses considérées comme la maison. Les plages privées sont listées par défaut. Ajoutez ici la plage d'un VPN (Tailscale, WireGuard) si ses clients doivent compter comme locaux. |
| **Débit distant maximal** | Un plafond de débit pour les clients hors des réseaux locaux, en kbit/s (0 = aucun). Une adresse privée n'est jamais plafonnée. Quand **Admin → Transcodage** fixe aussi un plafond pour les spectateurs distants, c'est le plus bas des deux qui s'applique. Voir [Comment la lecture est décidée](../playback/how-playback-is-decided.md#limits-on-the-session). |

## Ce qu'Evey doit savoir du proxy

Evey sert du HTTP simple sur le port 9091, API et interface web ensemble. Le
HTTPS est le rôle du reverse proxy, et Evey a besoin de deux choses à son
sujet, toutes deux dans **Admin → Accès distant** :

- l'**URL externe** en `https://`. C'est ce qui indique à Evey qu'un proxy
  assure le TLS : les cookies reçoivent l'attribut `Secure`, l'en-tête HSTS
  est envoyé et l'en-tête `X-Forwarded-Proto` d'un proxy de confiance est cru ;
- les **Proxys de confiance**, quand le proxy ne tourne pas sur la même
  machine qu'Evey. Un proxy dans un autre conteneur a besoin de son réseau
  ici, par exemple `172.16.0.0/12` pour les réseaux Docker par défaut.

!!! warning "Une plage qui couvre toutes les adresses est refusée"
    `0.0.0.0/0` ou `::/0` dans **Proxys de confiance** laisserait n'importe
    quel client choisir l'adresse sous laquelle il est vu. Le panneau comme
    `evey admin doctor` le refusent : indiquez l'adresse ou le réseau du proxy
    lui-même.

### Le régler plutôt dans l'environnement

`EVEY_SERVER_BASE_URL` et `EVEY_TRUSTED_PROXIES` donnent les mêmes réponses
depuis le fichier compose. C'est utile pour une installation entièrement
décrite dans des fichiers, ou pour un serveur qui doit être joignable en HTTPS
dès son premier démarrage. Quand les deux sont renseignés, le panneau
l'emporte : une URL externe en `https://` active le mode HTTPS même si l'URL
de base est en HTTP simple, et une liste de proxys enregistrée remplace la
variable. Vider le champ rend la main à la variable. Voir
[Serveur et réseau](../environment/server.md#remote-access).

## L'adresse réelle du client

L'accès distant et le **Débit distant maximal** dépendent tous deux de
l'adresse réelle de chaque client, établie en deux étapes successives : les
proxys de confiance déterminent quelle adresse transmise est crue, donc quelle
adresse reçoit chaque client ; les réseaux LAN classent ensuite cette adresse
comme locale ou distante.

Un proxy qui ne transmet pas l'adresse, ou auquel Evey ne fait pas confiance,
fait croire que toutes les requêtes viennent de l'adresse privée du proxy :
tout le monde est considéré comme local, le réglage d'accès distant n'a aucun
effet et aucun plafond distant ne s'applique.

!!! note "Corriger le proxy change qui est considéré comme distant"
    Si l'accès distant était désactivé, les clients distants sont refusés dès
    que le proxy transmet leur adresse réelle. Lancez `evey admin doctor`
    après avoir modifié la configuration du proxy : il indique la liste en
    vigueur et sa provenance, avertit si une plage est publique et échoue sur
    une plage qui couvre toutes les adresses.

<!-- sources
inference and explicit choice: docs/adr/0026-remote-access-inferred-from-a-published-url.md ; internal/config/remote_access_resolve.go - ResolveRemoteAccess, IsPubliclyReachableURL
gate, live switch and LAN list: cmd/evey/gateway.go - networkMiddleware ; cmd/evey/network_middleware_test.go - TestNetworkMiddleware_LANListChangeAppliesWithoutRestart
remote cap live: internal/adapters/driving/grpcapi/playback_remote_bitrate_test.go
labels: i18n admin.remoteAccess.* (enableRemoteAccess, externalUrl, trustedProxies, lanNetworks, maxRemoteBitrate, lockoutRiskTitle)
proxy settings: docs/reverse-proxy.md
https mode and trusted proxies live: internal/config/network_live.go - EffectiveHTTPS, EffectiveTrustedProxies ; internal/adapters/driving/http/access_handler_live_test.go
catch-all range refused: internal/config/trusted_proxies.go - IsCatchAllProxyRange ; internal/config/network_live.go
doctor check: cmd/evey/admin_ops.go - checkTrustedProxies
-->
