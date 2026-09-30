---
source_sha: 1bffdb30b4e4b18a27866ddee2328b15bb2e673bbb3d62cca51a941c7084643b
reviewed: true
---
# Accès distant et HTTPS

Joindre le serveur depuis l'extérieur met en jeu deux choses distinctes : une
adresse publique en HTTPS devant Evey, et le réglage d'accès distant d'Evey,
qui décide si les clients situés hors de votre réseau sont servis.

## L'accès distant se déduit de l'adresse publique

Tant que vous n'avez rien choisi, Evey décide de l'accès distant au démarrage :

- si le serveur a reçu une adresse publique (`EVEY_SERVER_BASE_URL` ou
  `EVEY_REMOTE_ACCESS_EXTERNAL_URL`), l'accès distant est **activé** ;
- sinon il est **désactivé**, et seuls les clients de vos réseaux locaux sont
  servis.

Dès que vous modifiez ce réglage dans **Admin → Accès distant**, votre choix
est conservé et le serveur cesse de le déduire : un serveur publié peut
rester limité au réseau local si c'est ce que vous voulez.

Quand l'accès distant est désactivé, un client hors des réseaux locaux reçoit
une erreur `403` sur toutes les pages, et les applications affichent un
message indiquant que l'accès distant est désactivé plutôt qu'une erreur
générique. Si le serveur est joignable depuis Internet alors que l'accès
distant est désactivé, la page d'administration affiche **Tous les
utilisateurs distants sont bloqués**, le serveur le signale dans ses logs au
démarrage et journalise chaque refus avec l'adresse du client, et
`evey admin doctor` le compte comme un échec.

L'accès distant est un choix sur qui peut joindre le serveur, pas une
protection : chaque page exige toujours une connexion.

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
| **Débit distant maximal** | Un plafond de débit pour les clients hors des réseaux locaux, en kbit/s (0 = aucun). Une adresse privée n'est jamais plafonnée. Voir [Comment la lecture est décidée](../playback/how-playback-is-decided.md#limits-on-the-session). |

## HTTPS : un reverse proxy devant Evey

Evey sert du HTTP simple sur le port 9091, API et interface web ensemble. Le
HTTPS est le rôle d'un reverse proxy, qui transmet tout à `evey-server:9091`.
Evey a besoin de savoir deux choses à son sujet, toutes deux dans
**Admin → Accès distant** :

- **URL externe** en `https://`. C'est ce qui indique à Evey qu'un proxy
  assure le TLS : les cookies reçoivent l'attribut `Secure`, l'en-tête HSTS
  est envoyé et l'en-tête `X-Forwarded-Proto` d'un proxy de confiance est cru.
- **Proxys de confiance**, quand le proxy ne tourne pas sur la même machine
  qu'Evey. Un proxy dans un autre conteneur a besoin de son réseau ici, par
  exemple `172.16.0.0/12` pour les réseaux Docker par défaut. Une plage qui
  couvre toutes les adresses (`0.0.0.0/0`, `::/0`) est refusée : n'importe
  quel client pourrait alors choisir l'adresse sous laquelle il est vu.

Sans proxy pour l'instant, le dépôt d'Evey fournit un service Caddy qui
obtient seul un certificat Let's Encrypt. Configurations prêtes à l'emploi
pour Caddy, Nginx, Traefik et Nginx Proxy Manager, et effet des réglages par
défaut d'un proxy sur le streaming :
[Configurations de reverse proxy](../reverse-proxy.md).

### Le régler plutôt dans l'environnement

`EVEY_SERVER_BASE_URL` et `EVEY_TRUSTED_PROXIES` donnent les mêmes réponses
depuis le fichier compose. C'est utile pour une installation entièrement
décrite dans des fichiers, ou pour un serveur qui doit être joignable en HTTPS
dès son premier démarrage. Quand les deux sont renseignés, le panneau
l'emporte : une URL externe en `https://` active le mode HTTPS même si l'URL
de base est en HTTP simple, et une liste de proxys enregistrée remplace la
variable. Vider le champ rend la main à la variable. Voir
[Serveur et réseau](../environment/server.md).

## Les deux étapes réseau

Proxys de confiance et Réseaux LAN sont deux étapes successives, pas deux
façons de dire la même chose. Les proxys de confiance déterminent quelle
adresse transmise est crue, donc quelle adresse reçoit chaque client ; les
Réseaux LAN classent ensuite cette adresse comme locale ou distante.

## L'adresse réelle du client

L'accès distant et le **Débit distant maximal** dépendent tous deux de
l'adresse réelle de chaque client. Un proxy qui ne la transmet pas, ou auquel
Evey ne fait pas confiance, fait croire que toutes les requêtes viennent de
l'adresse privée du proxy : tout le monde est considéré comme local, le
réglage d'accès distant n'a aucun effet et aucun plafond distant ne
s'applique.

Corriger le proxy change donc qui est considéré comme distant. Si l'accès
distant était désactivé, les clients distants sont refusés à partir de ce
moment. Lancez `evey admin doctor` après avoir modifié la configuration du
proxy : il indique la liste en vigueur et sa provenance, avertit si une plage
est publique et échoue sur une plage qui couvre toutes les adresses.
