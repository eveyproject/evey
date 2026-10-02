---
source_sha: 159ec0fe1ebc74e3d0f2cf81603fc3b4476f15903cfdfbfb246a851b112343df
reviewed: true
---
# Reverse proxy

Evey sert **l'API et l'interface web** sur un seul port, le **9091**, en HTTP
simple. Un reverse proxy n'a qu'à terminer le TLS et tout renvoyer vers
`evey:9091` : il n'y a **aucun frontend séparé à compiler ni à servir**.

Quel que soit le proxy retenu, indiquez à Evey son adresse publique et le proxy
auquel faire confiance. Le plus simple est **Admin → Accès distant** : une
**URL externe** en `https://` et, quand le proxy est un conteneur, son réseau
dans **Proxys de confiance**. Les deux s'appliquent sans redémarrage. Les mêmes
réponses peuvent venir de l'environnement, pour une installation entièrement
décrite dans des fichiers (le panneau l'emporte quand les deux sont renseignés) :

```env title=".env"
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=172.16.0.0/12       # seulement si le proxy est un conteneur ; la boucle locale est toujours de confiance
```

C'est une adresse en `https://` qui indique à Evey qu'un proxy assure le TLS
devant lui : les cookies reçoivent l'attribut `Secure`, l'en-tête HSTS est
envoyé, et l'en-tête `X-Forwarded-Proto` d'un proxy de confiance est cru. Aucun
autre interrupteur n'est nécessaire (l'ancienne variable `EVEY_FORWARDED_PROTO`
reste acceptée pour forcer la valeur, mais elle est obsolète).

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
  nginx et Caddy ne le font pas par défaut) la couperait. Le scan continue de
  toute façon côté serveur.

Les chemins concernés :

```
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

Le Caddy fourni et les exemples Nginx et Traefik ci-dessous règlent les deux
(pas de tampon, pas de délai) sur ces chemins. Avec un proxy piloté par une
interface comme Nginx Proxy Manager, les valeurs par défaut conviennent et le
réglage fin est un extrait facultatif.

## En-têtes de sécurité et CSP : à lire si votre proxy pose les siens

Evey émet une Content-Security-Policy complète. Si votre proxy la laisse passer,
il n'y a rien d'autre à faire.

Beaucoup d'installations la retirent au contraire (`header_down
-Content-Security-Policy` dans Caddy) et posent la leur. C'est possible, mais
**votre** politique doit alors garder ces éléments, sans quoi des fonctions
cessent de marcher sans le moindre message :

| Élément | Directive | Ce qui casse sans lui |
| --- | --- | --- |
| `'self'` | `frame-src` | L'interface de chaque plugin et le script de chaque thème (iframe vide). Ils tournent dans `/plugin-frame.html`, une page d'Evey intégrée par l'application (voir plus bas). |
| `'wasm-unsafe-eval'` | `script-src` | La lecture AC-3/E-AC-3 dans le navigateur (le décodeur WebAssembly ac3go ne peut pas être compilé, le film est lu sans le son). Cet élément n'autorise que la compilation Wasm, PAS `eval()` en général. |
| `https://www.gstatic.com` | `script-src` (+ `frame-src`) | Le SDK d'envoi Google Cast ne se charge jamais : plus de bouton de diffusion pour personne. C'est le seul moyen pour un navigateur de découvrir les Chromecast du réseau local de l'utilisateur. |
| `blob:` | `media-src` | La lecture MSE (hls.js rattache le flux par une URL blob). |
| `blob:` | `worker-src` | hls.js se rabat sans le dire sur un démultiplexage dans le thread principal (plus saccadé sur mobile). |

Une politique éprouvée pour un proxy qui pose la sienne :

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

`script-src` ne contient pas `'unsafe-inline'` : un script injecté dans la page
ne s'exécute pas. Ne le rajoutez pas, rien dans l'application n'en a besoin.

Deux pages portent leur propre politique, envoyée par l'application ; laissez
l'en-tête de l'application sur ces deux chemins :

- `/plugin-frame.html`, la page dans laquelle tournent les interfaces des
  plugins et les scripts des thèmes. Sa politique l'isole (même ouverte hors
  d'une iframe), limite son réseau aux hôtes déclarés dans le manifeste du
  plugin et ne laisse que l'application l'intégrer ; elle envoie donc aussi
  `X-Frame-Options: SAMEORIGIN` : ne le remplacez pas par `DENY` sur ce chemin,
  sinon toutes les interfaces de plugins restent vides.
- `/cast-receiver.html`, décrite ci-dessous.

Une page a besoin d'une politique plus large : `/cast-receiver.html`, la page
que charge un Chromecast quand il lit depuis Evey. Le lecteur Cast y injecte
des styles, des polices et des images venus de `www.gstatic.com`, et
l'application envoie pour cette raison une politique propre à cette page.
Laissez passer l'en-tête de l'application sur ce chemin, ou appliquez cette
politique à ce seul chemin :

```
default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com; style-src 'self' 'unsafe-inline' https://www.gstatic.com; img-src 'self' data: https: blob:; media-src 'self' blob: https:; worker-src 'self' blob:; font-src 'self' data: https://www.gstatic.com; connect-src 'self' https: wss:; frame-src https://www.gstatic.com; base-uri 'self'; object-src 'none'
```

Quelques règles simples :

- **Un seul en-tête CSP doit arriver au navigateur.** Avec deux (celui de
  l'application ET celui du proxy, ou celui de Caddy ET l'extrait « Advanced »
  de Nginx Proxy Manager), le navigateur applique leur *intersection* : la plus
  stricte l'emporte, et l'échec est silencieux. Vérifiez avec
  `curl -sI https://votre.domaine | grep -ci content-security-policy`, qui doit
  afficher `1`.
- **Gardez la politique sur UNE seule ligne.** Une CSP coupée sur plusieurs
  lignes dans la configuration d'un proxy est une corruption silencieuse
  classique : la jonction avale les caractères de part et d'autre
  (`...gstatic.com; connect-src` est ainsi devenu `...gstaticonnect-src`, ce qui
  a désactivé le SDK Cast et fusionné deux directives).
- **Vérifiez que la page des plugins garde sa propre politique :**
  `curl -sI https://votre.domaine/plugin-frame.html | grep -i -e content-security -e x-frame`
  doit afficher `sandbox allow-scripts` et `SAMEORIGIN`.

Chaque exemple fourni lit le domaine à **un seul endroit** : `EVEY_DOMAIN` dans
votre `.env`. Renseignez-le une fois ; Caddy, Nginx et Traefik le reprennent
tous.

```env title=".env"
EVEY_DOMAIN=media.example.com
```

## Caddy (fourni)

Le dépôt fournit [`docker/Caddyfile`](../docker/Caddyfile) (construit en image
par [`docker/Dockerfile.caddy`](../docker/Dockerfile.caddy)) : il obtient
automatiquement le HTTPS pour `EVEY_DOMAIN` et renvoie tout vers `evey:9091`,
avec les exceptions de streaming déjà en place. Il pose aussi les en-têtes de
sécurité et la CSP de référence de la section précédente (et retire la CSP de
l'application, pour qu'une seule arrive au navigateur). Ajoutez un service
`caddy` à votre compose (voir le service `caddy` du
[`docker-compose.yml`](../docker-compose.yml) du dépôt).

### Derrière un proxy que vous avez déjà (TLS terminé en amont)

Sur un NAS qui a déjà un reverse proxy (celui du fabricant, Nginx Proxy Manager,
un Traefik sur un réseau Docker partagé), **Caddy est inutile** : Evey émet ses
propres en-têtes de sécurité et une CSP complète, et les règles de streaming ne
peuvent de toute façon être appliquées que par le proxy qui fait face au client.
Un second proxy derrière lui n'ajoute rien. Placez le service evey sur le réseau
de ce proxy et visez `evey-server:9091` (le nom du conteneur ; le nom du
service, `evey`, fonctionne aussi) :

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
WebSockets, et appliquez les règles de streaming ci-dessus (pas de tampon, pas
de délai de lecture ni d'écriture sur les chemins de streaming). Les sections
Nginx, Traefik et Nginx Proxy Manager ci-dessous donnent les lignes exactes.

## Nginx

Utilisez [`examples/nginx/evey.conf.template`](../examples/nginx/evey.conf.template)
(un compose prêt à l'emploi se trouve dans
[`examples/nginx/docker-compose.nginx.yml`](../examples/nginx/docker-compose.nginx.yml)).
L'image nginx le génère au démarrage en y substituant `EVEY_DOMAIN` ; les
certificats doivent exister sous les noms de fichier correspondants. Il renvoie
vers `evey:9091` sans tampon pour le streaming, sans étape de compilation du
frontend.

## Traefik

Voir [`examples/traefik/docker-compose.traefik.yml`](../examples/traefik/docker-compose.traefik.yml) :

```bash
docker compose -f docker-compose.yml -f examples/traefik/docker-compose.traefik.yml up -d
```

## Nginx Proxy Manager

Le conteneur evey doit être sur le réseau qu'utilise NPM (voir « Derrière un
proxy que vous avez déjà » plus haut). Ensuite, **Edit Proxy Host** :

1. **Details** : Scheme `http`, Forward Hostname `evey-server`, Forward Port
   `9091`. **Websockets Support : on** (SyncPlay). **Cache Assets : off** : il
   mettrait en cache les segments et les playlists avec une expiration fixe.
   Block Common Exploits peut rester actif (désactivez-le si une recherche ou
   l'envoi d'un plugin répond un jour 403 sans raison).
2. **SSL** : votre certificat, Force SSL, HTTP/2. HSTS est facultatif ici :
   l'application l'envoie déjà.
C'est tout : NPM transmet de lui-même `X-Forwarded-For` / `X-Forwarded-Proto` et
l'en-tête Host, et ses délais par défaut ne se déclenchent jamais sur les flux
d'Evey (voir la section sur le streaming).

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

## Adresse réelle des clients et contrôle de l'accès distant

Dès que votre proxy transmet l'adresse réelle du client, Evey sait distinguer un
spectateur à la maison d'un spectateur sur Internet. C'est sur cette distinction
que s'appuient le plafond de débit distant et le contrôle de l'accès distant :
les deux réglages ne se comportent donc correctement qu'ensemble.

- Les proxys de confiance (dans le panneau ou `EVEY_TRUSTED_PROXIES`) doivent
  couvrir le proxy (`172.16.0.0/12` quand c'est un conteneur sur le réseau
  Docker ; rien à ajouter quand il tourne sur la machine hôte, la boucle locale
  étant toujours de confiance). Ce que vous indiquez S'AJOUTE à la boucle
  locale sans jamais la remplacer : une valeur Docker ne peut donc pas empêcher
  en silence le contrôle de voir les vraies adresses des clients.
- Des proxys en chaîne doivent chacun conserver `X-Forwarded-For`. Dans Caddy,
  c'est `servers { trusted_proxies static private_ranges }` ; nginx utilise
  `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`.

**L'activer change qui est considéré comme distant.** Avant, toutes les requêtes
semblaient venir de l'adresse privée du proxy, et tout le monde était classé en
local. Après, les vrais clients distants sont correctement identifiés, et si
l'accès distant est désactivé, ils sont refusés sur toutes les routes
(`403 {"code":"remote_access_disabled"}`), sans qu'aucun réglage de
l'application ait changé.

Evey s'en protège : l'accès distant est activé par défaut dès qu'une adresse
publique est configurée et qu'aucun choix explicite n'a été fait, son état est journalisé à chaque démarrage, les
refus sont journalisés avec l'adresse du client, `evey admin doctor` échoue
clairement sur « publié mais refuse tout », et le panneau d'administration
affiche un avertissement. Après tout changement de la confiance accordée au
proxy, lancez `evey admin doctor`.
