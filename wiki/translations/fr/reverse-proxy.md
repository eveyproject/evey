---
source_sha: d47608ac11df56bc518674752f94a79d5498896103d51978e420575983e71cf4
reviewed: true
---
# Configurations de reverse proxy

Evey sert l'API et l'interface web ensemble sur le port **9091**, en HTTP
simple ; un reverse proxy n'a qu'à terminer le TLS et tout renvoyer vers
`evey:9091`. Choisissez votre proxy ci-dessous (Caddy fourni, Nginx, Traefik,
Nginx Proxy Manager, ou celui que vous avez déjà), puis lisez les deux
sections de la fin si vous ajustez le streaming ou posez vos propres en-têtes
de sécurité.

## Indiquer le proxy à Evey

Quel que soit le proxy, Evey a besoin de son adresse publique et du proxy
auquel faire confiance. L'endroit pour les deux est **Admin → Accès
distant** : une **URL externe** en `https://` et, quand le proxy est un
conteneur, son réseau dans **Proxys de confiance**. Les deux s'appliquent
sans redémarrage. C'est une adresse en `https://` qui indique à Evey qu'un
proxy assure le TLS devant lui : les cookies reçoivent l'attribut `Secure`,
l'en-tête HSTS est envoyé et l'en-tête `X-Forwarded-Proto` d'un proxy de
confiance est cru. L'interrupteur d'accès distant et l'adresse réelle du
client sont expliqués dans
[Accès distant et HTTPS](administration/remote-access.md).

Les mêmes réponses peuvent venir de l'environnement, pour une installation
entièrement décrite dans des fichiers (le panneau l'emporte quand les deux
sont renseignés) :

```env title=".env"
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=172.16.0.0/12       # seulement si le proxy est un conteneur ; la boucle locale est toujours de confiance
```

L'ancienne variable `EVEY_FORWARDED_PROTO` reste acceptée pour forcer la
valeur, mais elle est obsolète : le schéma de l'adresse suffit.

Chaque exemple fourni lit le domaine à un seul endroit, `EVEY_DOMAIN` dans
votre `.env`. Renseignez-le une fois ; Caddy, Nginx et Traefik le reprennent
tous.

```env title=".env"
EVEY_DOMAIN=media.example.com
```

## Caddy (fourni)

### Fichiers

[`docker/Caddyfile`](../docker/Caddyfile), construit en image par
[`docker/Dockerfile.caddy`](../docker/Dockerfile.caddy) ; le service `caddy`
du [`docker-compose.yml`](../docker-compose.yml) du dépôt.

### Mise en place

1. Renseignez `EVEY_DOMAIN` dans `.env` et assurez-vous que les ports 80 et
   443 de la machine arrivent au conteneur Caddy.
2. Ajoutez le service `caddy` du fichier compose du dépôt au vôtre et lancez
   `docker compose up -d`. Caddy obtient seul un certificat Let's Encrypt pour
   `EVEY_DOMAIN`.
3. Dans **Admin → Accès distant**, renseignez l'**URL externe** en `https://`
   et `172.16.0.0/12` dans **Proxys de confiance**.

### Ce qu'il gère déjà

Il renvoie tout vers `evey:9091`, les chemins de streaming étant servis sans
tampon ni délai. Il pose les en-têtes de sécurité et la CSP de référence de la
[section sécurité](#security-headers--csp---read-this-if-your-proxy-sets-its-own)
et retire la CSP de l'application pour qu'une seule arrive au navigateur ; les
deux pages qui portent leur propre politique la conservent.

## Nginx

### Fichiers

[`examples/nginx/evey.conf.template`](../examples/nginx/evey.conf.template)
et un compose complémentaire prêt à l'emploi,
[`examples/nginx/docker-compose.nginx.yml`](../examples/nginx/docker-compose.nginx.yml).

### Mise en place

1. Renseignez `EVEY_DOMAIN` dans `.env`. L'image nginx génère le modèle au
   démarrage en y substituant `EVEY_DOMAIN`.
2. Placez le certificat et la clé là où le modèle les attend, sous les noms
   de fichier correspondants (`/etc/ssl/certs/<domaine>.pem`,
   `/etc/ssl/private/<domaine>.key` dans le volume `nginx-certs`) : le compose
   complémentaire n'obtient pas de certificat à votre place.
3. `docker compose -f docker-compose.yml -f examples/nginx/docker-compose.nginx.yml up -d`.
4. Dans **Admin → Accès distant**, renseignez l'**URL externe** en `https://`
   et `172.16.0.0/12` dans **Proxys de confiance**.

### Ce qu'il gère déjà

Il renvoie vers `evey:9091` sans tampon ni délai sur les chemins de streaming,
avec le WebSocket de SyncPlay pris en charge, `X-Forwarded-For` et
`X-Forwarded-Proto` transmis, et `client_max_body_size 2G` pour les envois de
plugins et les restaurations de sauvegarde. Il laisse passer les en-têtes de
sécurité et la CSP de l'application.

## Traefik

### Fichiers

[`examples/traefik/docker-compose.traefik.yml`](../examples/traefik/docker-compose.traefik.yml).

### Mise en place

1. Faites tourner Traefik avec un point d'entrée `websecure`, un résolveur de
   certificats `letsencrypt` et un réseau Docker externe nommé `proxy` ; les
   étiquettes du compose complémentaire supposent ces noms.
2. Renseignez `EVEY_DOMAIN` dans `.env`, puis :

    ```bash
    docker compose -f docker-compose.yml -f examples/traefik/docker-compose.traefik.yml up -d
    ```

3. Dans **Admin → Accès distant**, renseignez l'**URL externe** en `https://`
   et la plage du réseau `proxy` dans **Proxys de confiance** (`172.16.0.0/12`
   couvre les réseaux Docker par défaut).

### Ce qu'il gère déjà

Le compose complémentaire ajoute seulement les étiquettes du routeur et
rejoint le réseau `proxy`. Traefik sur ses réglages par défaut ne met pas les
réponses en tampon et ne plafonne pas leur durée, aucun réglage de streaming
n'est donc nécessaire ; il transmet de lui-même `X-Forwarded-For` et
`X-Forwarded-Proto` et laisse passer les en-têtes de sécurité de
l'application.

## Nginx Proxy Manager

### Fichiers

Aucun : tout se règle dans l'interface de NPM. Le conteneur evey doit être
sur le réseau qu'utilise NPM, voir
[Un proxy que vous avez déjà](#a-proxy-you-already-run-tls-terminated-upstream).

### Mise en place

Dans **Edit Proxy Host** :

1. **Details** : Scheme `http`, Forward Hostname `evey-server`, Forward Port
   `9091`. **Websockets Support : on** (SyncPlay). **Cache Assets : off** : il
   mettrait en cache les segments et les playlists avec une expiration fixe.
   Block Common Exploits peut rester actif (désactivez-le si une recherche ou
   l'envoi d'un plugin répond un jour 403 sans raison).
2. **SSL** : votre certificat, Force SSL, HTTP/2. HSTS est facultatif ici :
   l'application l'envoie déjà.
3. Dans **Admin → Accès distant**, renseignez l'**URL externe** en `https://`
   et la plage du réseau de NPM dans **Proxys de confiance**.

### Ce qu'il gère déjà

NPM transmet de lui-même `X-Forwarded-For`, `X-Forwarded-Proto` et l'en-tête
Host, et ses délais par défaut ne se déclenchent jamais sur les flux d'Evey.
Rien d'autre n'est requis.

**Facultatif** : empêcher NPM de mettre en tampon les fichiers en lecture
directe sur son propre disque. **Advanced** → *Custom Nginx Configuration*
(l'onglet `Custom Locations` ne sait pas exprimer l'expression régulière) :

```nginx
# Envois de plugins (jusqu'à 100 Mo) et restaurations de sauvegarde (jusqu'à 2 Go).
client_max_body_size 2G;

# Streaming : pas de tampon, pas de délai.
location ~ ^/api/v1/media/.+/(hls|stream) {
    proxy_pass http://evey-server:9091;
    proxy_buffering off;
    proxy_request_buffering off;
    proxy_read_timeout 0;
    proxy_send_timeout 0;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}

# Les scans de bibliothèque diffusent leur progression tant qu'ils tournent.
location ~ ^/api/v1/libraries/.+/scan {
    proxy_pass http://evey-server:9091;
    proxy_buffering off;
    proxy_read_timeout 0;
    proxy_send_timeout 0;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

Tous les autres chemins (l'interface web, le reste de l'API, le WebSocket de
SyncPlay) passent par le bloc de proxy de NPM, que configure l'onglet Details.
Sans cet extrait, rien ne casse : il change seulement l'endroit où les gros
fichiers sont mis en tampon.

## Un proxy que vous avez déjà (TLS terminé en amont)

Sur un NAS qui a déjà un reverse proxy (celui du fabricant, Nginx Proxy
Manager, un Traefik sur un réseau Docker partagé), Caddy est inutile : Evey
émet ses propres en-têtes de sécurité et une CSP complète, et les règles de
streaming ne peuvent de toute façon être appliquées que par le proxy qui fait
face au client. Un second proxy derrière lui n'ajoute rien.

### Mise en place

Placez le service evey sur le réseau de ce proxy et visez `evey-server:9091`
(le nom du conteneur ; le nom du service, `evey`, fonctionne aussi) :

```yaml title="docker-compose.yml"
services:
  evey:
    # pas de ports : le proxy est la seule porte d'entrée
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # à restreindre au sous-réseau du proxy une fois connu
    # ...
networks:
  default:
    name: proxy-net
    external: true
```

Ensuite, dans ce proxy, pour la route vers `evey-server:9091` : transmettez
`X-Forwarded-For` et `X-Forwarded-Proto`, activez la prise en charge des
WebSockets, et appliquez les règles de streaming ci-dessous sur les chemins de
streaming. Les sections Nginx et Nginx Proxy Manager ci-dessus donnent les
lignes exactes.

### Ce qu'il gère déjà

Cela dépend du proxy : vérifiez qu'un seul en-tête CSP arrive au navigateur
(sections suivantes) et lancez `evey admin doctor` une fois les proxys de
confiance renseignés.

## Le streaming et les réglages par défaut d'un proxy

Evey fonctionne derrière un reverse proxy laissé sur ses réglages par défaut :
chaque réponse longue envoie des octets en continu (segments HLS, lecture
directe, sortie du transcodage) ou relance le client toutes les 30 s (les flux
d'événements), si bien qu'un délai de lecture par défaut ne se déclenche jamais.
Deux points restent utiles à connaître :

- **La mise en tampon.** Un proxy qui met les réponses en tampon (nginx le fait
  par défaut) lit un fichier en lecture directe dans ses propres tampons et
  fichiers temporaires avant que le client ne le reçoive. Cela fonctionne, mais
  un film de plusieurs gigaoctets transite alors par le disque de la machine du
  proxy. `proxy_buffering off` sur les chemins de streaming l'évite.
- **Les scans.** `/api/v1/libraries/*/scan` diffuse sa progression tant que le
  scan tourne ; un proxy qui plafonne la durée totale d'une réponse (rare :
  nginx, Caddy et Traefik ne le font pas par défaut) la couperait. Le scan
  continue de toute façon côté serveur.

Les chemins concernés :

```
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

Le Caddy fourni et l'exemple Nginx règlent les deux (pas de tampon, pas de
délai) sur ces chemins. L'exemple Traefik et un proxy piloté par une interface
comme Nginx Proxy Manager gardent leurs réglages par défaut, qui conviennent ;
l'extrait NPM ci-dessus est facultatif.

## En-têtes de sécurité et CSP : à lire si votre proxy pose les siens

Evey émet une Content-Security-Policy complète. Si votre proxy la laisse
passer, il n'y a rien d'autre à faire.

Beaucoup d'installations la retirent au contraire (`header_down
-Content-Security-Policy` dans Caddy) et posent la leur. C'est possible, mais
**votre** politique doit alors garder ces éléments, sans quoi des fonctions
cessent de marcher sans le moindre message :

| Élément | Directive | Ce qui casse sans lui |
| --- | --- | --- |
| `'self'` | `frame-src` | L'interface de chaque plugin et le script de chaque thème (iframe vide). Ils tournent dans `/plugin-frame.html`, une page d'Evey intégrée par l'application (voir plus bas). |
| `'wasm-unsafe-eval'` | `script-src` | La lecture AC-3/E-AC-3 dans le navigateur (le décodeur WebAssembly ac3go ne peut pas être compilé, le film est lu sans le son). Cet élément n'autorise que la compilation Wasm, pas `eval()` en général. |
| `https://www.gstatic.com` | `script-src` (+ `frame-src`) | Le SDK d'envoi Google Cast ne se charge jamais, plus personne n'a de bouton de diffusion. C'est le seul moyen pour un navigateur de découvrir les Chromecast du réseau local de l'utilisateur. |
| `blob:` | `media-src` | La lecture MSE (hls.js rattache le flux par une URL blob). |
| `blob:` | `worker-src` | hls.js se rabat sans le dire sur un démultiplexage dans le thread principal (plus saccadé sur mobile). |

Une politique éprouvée pour un proxy qui pose la sienne, celle qu'envoie le
Caddy fourni :

```
default-src 'self'; base-uri 'self'; object-src 'none';
img-src 'self' https: data: blob:; media-src 'self' blob:;
style-src 'self' 'unsafe-inline';
script-src 'self' 'wasm-unsafe-eval' https://www.gstatic.com;
connect-src 'self' https: wss:; worker-src 'self' blob:; font-src 'self' data:;
frame-src 'self' https://www.gstatic.com
```

`base-uri` et `object-src` ne retombent pas sur `default-src` : une politique
qui les omet laisse libres l'injection de `<base href>` et l'intégration par
`<object>`/`<embed>`. Gardez-les si vous posez votre propre politique.
`script-src` ne contient pas `'unsafe-inline'` : un script injecté dans la
page ne s'exécute pas. Ne le rajoutez pas, rien dans l'application n'en a
besoin.

Deux pages portent leur propre politique, envoyée par l'application ; laissez
l'en-tête de l'application sur ces deux chemins :

- `/plugin-frame.html`, la page dans laquelle tournent les interfaces des
  plugins et les scripts des thèmes. Sa politique l'isole (même ouverte hors
  d'une iframe), limite son réseau aux hôtes déclarés dans le manifeste du
  plugin et ne laisse que l'application l'intégrer ; elle envoie donc aussi
  `X-Frame-Options: SAMEORIGIN` : ne le remplacez pas par `DENY` sur ce chemin,
  sinon toutes les interfaces de plugins restent vides.
- `/cast-receiver.html`, la page que charge un Chromecast quand il lit depuis
  Evey. Le lecteur Cast y injecte des styles, des polices et des images venus
  de `www.gstatic.com`. Laissez passer l'en-tête de l'application sur ce
  chemin, ou appliquez cette politique à ce seul chemin :

```
default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com; style-src 'self' 'unsafe-inline' https://www.gstatic.com; img-src 'self' data: https: blob:; media-src 'self' blob: https:; worker-src 'self' blob:; font-src 'self' data: https://www.gstatic.com; connect-src 'self' https: wss:; frame-src https://www.gstatic.com; base-uri 'self'; object-src 'none'
```

!!! warning "Un seul en-tête CSP doit arriver au navigateur"
    Avec deux (celui de l'application et celui du proxy, ou celui de Caddy et
    l'extrait « Advanced » de Nginx Proxy Manager), le navigateur applique
    leur *intersection* : la plus stricte l'emporte, et l'échec est
    silencieux. Vérifiez avec
    `curl -sI https://votre.domaine | grep -ci content-security-policy`, qui
    doit afficher `1`.

Deux vérifications de plus après tout changement :

- **Gardez la politique sur une seule ligne.** Une CSP coupée sur plusieurs
  lignes dans la configuration d'un proxy est une corruption silencieuse
  classique : la jonction avale les caractères de part et d'autre
  (`...gstatic.com; connect-src` est ainsi devenu `...gstaticonnect-src`, ce
  qui a désactivé le SDK Cast et fusionné deux directives).
- **La page des plugins garde sa propre politique :**
  `curl -sI https://votre.domaine/plugin-frame.html | grep -i -e content-security -e x-frame`
  doit afficher `sandbox allow-scripts` et `SAMEORIGIN`.

## Adresse réelle des clients et contrôle de l'accès distant

Dès que le proxy transmet l'adresse réelle du client, Evey sait distinguer un
spectateur à la maison d'un spectateur sur Internet : c'est sur cette
distinction que s'appuient le **Débit distant maximal** et l'interrupteur
d'accès distant. Deux règles :

- Les proxys de confiance (dans le panneau ou `EVEY_TRUSTED_PROXIES`) doivent
  couvrir le proxy : `172.16.0.0/12` quand c'est un conteneur sur le réseau
  Docker, rien à ajouter quand il tourne sur la machine hôte, la boucle locale
  étant toujours de confiance et ce que vous indiquez s'y ajoutant.
- Des proxys en chaîne doivent chacun conserver `X-Forwarded-For`. Dans Caddy,
  c'est `servers { trusted_proxies static private_ranges }` ; nginx utilise
  `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`.

L'activer change qui est considéré comme distant : un serveur dont l'accès
distant était désactivé se met à refuser les vrais clients distants
(`403 {"code":"remote_access_disabled"}`) sans qu'aucun réglage de
l'application ait changé. Evey ouvre l'accès distant de lui-même quand une
adresse publique est renseignée et qu'aucun choix n'a été fait, journalise son
état à chaque démarrage et chaque refus avec l'adresse du client, et
`evey admin doctor` échoue sur « publié mais refuse tout ». Le mécanisme
complet est décrit dans
[Accès distant et HTTPS](administration/remote-access.md#the-real-client-address).

<!-- sources
one port for API and UI, https mode from the scheme: internal/config/config.go (EVEY_FORWARDED_PROTO deprecated) ; internal/config/network_live.go - EffectiveHTTPS
bundled Caddy: docker/Caddyfile (streaming matcher, @ownPolicy, header_down CSP) ; docker/Dockerfile.caddy ; docker-compose.yml caddy service
nginx: examples/nginx/evey.conf.template ; examples/nginx/docker-compose.nginx.yml (nginx-certs volume, no ACME)
traefik: examples/traefik/docker-compose.traefik.yml (labels + proxy network only)
streaming paths: cmd/evey/gateway.go ; event streams ping 30 s ; scan progress stream
csp and own-policy pages: internal/adapters/driving/http/helpers.go - securityHeadersMiddleware ; /plugin-frame.html ; /cast-receiver.html
trusted proxies, catch-all refused, doctor: internal/config/trusted_proxies.go ; cmd/evey/admin_ops.go - checkTrustedProxies
remote access inferred: docs/adr/0026-remote-access-inferred-from-a-published-url.md
labels: i18n admin.remoteAccess.*
-->
