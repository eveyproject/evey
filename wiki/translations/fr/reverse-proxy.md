---
source_sha: 6206dabdc6ff848ca510df9c4b2f78a112fdd636b130b05b32f18fd574e98f9d
---

# Reverse proxy

Evey sert **l'API et l'interface web** sur un seul port, le **9091**, en HTTP
simple. Un reverse proxy n'a qu'à terminer le TLS et tout renvoyer vers
`evey:9091` : il n'y a **aucun frontend séparé à compiler ni à servir**.

Quel que soit le proxy retenu, ces deux variables permettent à Evey de connaître
son URL publique et de faire confiance aux en-têtes que le proxy lui transmet :

```env title=".env"
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=172.16.0.0/12       # utile seulement si le proxy est un conteneur ; la boucle locale est toujours de confiance
```

C'est une URL de base en `https://` qui indique à Evey qu'un proxy termine le
TLS devant lui : les cookies reçoivent l'attribut `Secure`, HSTS est émis, et le
`X-Forwarded-Proto` d'un proxy de confiance est pris en compte. Aucun autre
interrupteur n'est nécessaire (l'ancienne variable `EVEY_FORWARDED_PROTO` reste
acceptée comme dérogation, mais elle est dépréciée).

## Diffusion - ce qu'en font les réglages par défaut d'un proxy

Evey fonctionne derrière un reverse proxy laissé sur ses réglages d'origine :
toute réponse de longue durée envoie soit des octets en continu (segments HLS,
lecture directe, sortie de transcodage), soit un signal au client toutes les
30 s (les flux d'événements), si bien qu'un délai de lecture par défaut ne se
déclenche jamais. Deux points méritent tout de même d'être connus :

- **La mise en tampon.** Un proxy qui met les réponses en tampon (nginx le fait
  par défaut) charge le fichier lu en lecture directe dans ses propres tampons
  et fichiers temporaires avant que le client ne le reçoive. Cela fonctionne,
  mais un film de plusieurs gigaoctets transite alors par le disque de la
  machine qui héberge le proxy. `proxy_buffering off` sur les chemins de
  diffusion évite ce détour.
- **Les analyses de bibliothèque.** `/api/v1/libraries/*/scan` diffuse sa
  progression aussi longtemps que l'analyse dure ; un proxy qui plafonne la
  durée totale d'une réponse (c'est rare, ni nginx ni Caddy ne le font par
  défaut) la couperait en route. L'analyse, elle, se poursuit côté serveur quoi
  qu'il arrive.

Les chemins concernés :

```
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

Le Caddy fourni ainsi que les exemples Nginx et Traefik ci-dessous appliquent
les deux réglages (ni mise en tampon, ni délai d'expiration) sur ces chemins.
Avec un proxy piloté par une interface graphique comme Nginx Proxy Manager, les
valeurs par défaut conviennent et le réglage fin se réduit à un extrait de
configuration facultatif.

## En-têtes de sécurité et CSP - à lire si votre proxy pose les siens

Evey émet une Content-Security-Policy complète. Si votre proxy la laisse passer,
il n'y a rien de plus à faire.

> Jusqu'au 2026-07-23, cette phrase était fausse : la politique de
> l'application oubliait `media-src` et `worker-src`, `blob:` retombait donc sur
> `default-src 'self'` et la lecture MSE cassait chez tous ceux qui la
> laissaient passer. Personne ne s'en est aperçu parce que la configuration
> Caddy fournie retire l'en-tête de l'application et compose le sien. Si vous
> faites tourner une version antérieure derrière Nginx Proxy Manager ou Traefik
> et que votre lecteur refuse de démarrer, c'est l'explication : composez au
> niveau du proxy la politique donnée plus bas, ou mettez à jour.

Beaucoup d'installations préfèrent la retirer (`header_down
-Content-Security-Policy` chez Caddy) pour composer la leur. C'est tout à fait
viable, mais **votre** politique doit alors conserver les jetons suivants, sous
peine de casser des fonctionnalités en silence :

| Jeton | Directive | Ce qui casse sans lui |
| --- | --- | --- |
| `'unsafe-inline'` | `script-src` | Toutes les interfaces d'extension (iframe vide). Le bundle d'une extension est récupéré avec le jeton de l'utilisateur puis injecté dans une iframe `srcdoc`, et un document srcdoc hérite de la politique de la page qui l'intègre. Ce qui borne l'extension entretemps, c'est le `sandbox="allow-scripts"` de l'iframe : origine opaque, aucun accès au DOM ni aux cookies du parent. |
| `'wasm-unsafe-eval'` | `script-src` | La lecture AC-3/E-AC-3 dans le navigateur (le décodeur WebAssembly ac3go ne peut plus être compilé → le film est lu sans le son). C'est le jeton étroit qui autorise la seule compilation Wasm, PAS l'`eval()` en général. |
| `https://www.gstatic.com` | `script-src` (+ `frame-src`) | Le SDK émetteur Google Cast ne se charge jamais → plus de bouton de diffusion pour personne. C'est le seul moyen dont dispose un navigateur pour découvrir les Chromecast du réseau local de l'utilisateur. |
| `blob:` | `media-src` | La lecture MSE (hls.js rattache le flux par une URL blob). |
| `blob:` | `worker-src` | hls.js se rabat en silence sur un démultiplexage dans le fil principal, plus saccadé sur mobile. |

Une politique éprouvée, pour un proxy qui compose la sienne :

```
default-src 'self'; base-uri 'self'; object-src 'none';
img-src 'self' https: data: blob:; media-src 'self' blob:;
style-src 'self' 'unsafe-inline';
script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com;
connect-src 'self' https: wss:; worker-src 'self' blob:; font-src 'self' data:;
frame-src https://www.gstatic.com
```

`base-uri` et `object-src` ne se replient pas sur `default-src` : une politique
qui les omet laisse l'injection de `<base href>` et l'intégration
d'`<object>`/`<embed>` sans aucune restriction. Conservez-les si vous composez
votre propre politique.

Quelques règles simples :

- **Un seul en-tête CSP doit atteindre le navigateur.** S'il y en a deux (celui
  de l'application ET celui de votre proxy, ou celui de Caddy ET l'extrait
  « Advanced » de Nginx Proxy Manager), les navigateurs appliquent leur
  *intersection* : le plus strict l'emporte et l'échec est silencieux. Vérifiez
  avec `curl -sI https://your.domain | grep -ci content-security-policy` → la
  commande doit afficher `1`.
- **Gardez la politique sur UNE seule ligne.** Une CSP répartie sur plusieurs
  lignes dans une configuration de proxy est un grand classique de la corruption
  silencieuse : le recollement avale les caractères de la jointure
  (`...gstatic.com; connect-src` est devenu une fois `...gstaticonnect-src`, ce
  qui a désactivé le SDK Cast et fusionné deux directives).
- `'unsafe-inline'` dans `script-src` a un vrai coût, et il est pour l'instant
  porteur (les interfaces d'extension, voir le tableau). On ne peut le retirer
  sans risque que sur une installation qui n'exécute aucun frontend d'extension.

Tous les exemples fournis lisent le domaine à **un seul endroit** :
`EVEY_DOMAIN`, dans votre `.env`. Renseignez-le une fois ; Caddy, Nginx et
Traefik le reprennent tous.

```env title=".env"
EVEY_DOMAIN=media.example.com
```

## Caddy (fourni)

Le dépôt livre [`docker/Caddyfile`](../docker/Caddyfile) (transformé en image
par [`docker/Dockerfile.caddy`](../docker/Dockerfile.caddy)) : il obtient le
HTTPS automatique pour `EVEY_DOMAIN` et renvoie tout vers `evey:9091`, avec les
exceptions de diffusion déjà en place. Il compose aussi les en-têtes de sécurité
et la CSP canoniques de la section précédente, et retire la CSP de l'application
pour qu'une seule atteigne le navigateur. Ajoutez un service `caddy` à votre
compose (voir le service `caddy` du [`docker-compose.yml`](../docker-compose.yml)
du dépôt).

### Derrière un proxy que vous exploitez déjà (TLS terminé en amont)

Sur un NAS qui dispose déjà d'un reverse proxy (celui du constructeur, Nginx
Proxy Manager, un Traefik sur un réseau Docker partagé), **Caddy est inutile** :
Evey émet ses propres en-têtes de sécurité et une CSP complète, et de toute
façon seul le proxy exposé au client peut faire respecter les règles de
diffusion. Un second proxy derrière lui n'apporte rien. Placez le service evey
sur le réseau de ce proxy et visez `evey-server:9091` :

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

Ensuite, dans ce proxy, sur la route vers `evey-server:9091` : transmettez
`X-Forwarded-For` et `X-Forwarded-Proto`, activez la prise en charge des
WebSocket, et appliquez les règles de diffusion ci-dessus (ni mise en tampon, ni
délai de lecture ou d'écriture sur les chemins de diffusion). Les sections
Nginx, Traefik et Nginx Proxy Manager qui suivent en donnent les lignes exactes.

## Nginx

Servez-vous de
[`examples/nginx/evey.conf.template`](../examples/nginx/evey.conf.template) (un
compose prêt à l'emploi se trouve dans
[`examples/nginx/docker-compose.nginx.yml`](../examples/nginx/docker-compose.nginx.yml)).
L'image nginx le génère au démarrage en y substituant `EVEY_DOMAIN` ; les
certificats doivent exister sous les noms de fichiers correspondants. Elle
relaie vers `evey:9091` avec la mise en tampon désactivée pour la diffusion, et
sans aucune étape de compilation du frontend.

## Traefik

Voir [`examples/traefik/docker-compose.traefik.yml`](../examples/traefik/docker-compose.traefik.yml) :

```bash
docker compose -f docker-compose.yml -f examples/traefik/docker-compose.traefik.yml up -d
```

## Nginx Proxy Manager

Le conteneur evey doit se trouver sur le réseau qu'utilise NPM (voir « Derrière
un proxy que vous exploitez déjà » plus haut). Ensuite, dans **Edit Proxy
Host** :

1. **Details** : Scheme `http`, Forward Hostname `evey-server`, Forward Port
   `9091`. **Websockets Support : on** (pour SyncPlay). **Cache Assets : off**,
   faute de quoi les segments et les playlists seraient mis en cache avec une
   expiration fixe. Block Common Exploits peut rester actif ; désactivez-le si
   une recherche ou l'envoi d'une extension répond un jour 403 sans raison.
2. **SSL** : votre certificat, Force SSL, HTTP/2. HSTS est facultatif ici,
   l'application l'émet déjà.
C'est tout : NPM transmet de lui-même `X-Forwarded-For`, `X-Forwarded-Proto` et
l'en-tête Host, et ses délais d'expiration par défaut ne se déclenchent jamais
sur les flux d'Evey (voir la section sur la diffusion plus haut).

**Facultatif** : empêcher NPM de faire transiter par son propre disque les
fichiers lus en lecture directe. Allez dans **Advanced** → *Custom Nginx
Configuration* (l'onglet `Custom Locations` ne sait pas exprimer l'expression
régulière) :

```nginx
# Envoi d'extensions, restauration de sauvegardes.
client_max_body_size 100M;

# Diffusion - ni mise en tampon, ni délai d'expiration.
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

# Les analyses de bibliothèque diffusent leur progression tant qu'elles durent.
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
SyncPlay) passent par le bloc proxy de NPM lui-même, celui que configure
l'onglet Details. Sans cet extrait, rien ne casse : il ne change que l'endroit
où les gros fichiers sont mis en tampon.

## Les vraies adresses IP des clients et le verrou d'accès distant

Dès lors que votre proxy transmet l'adresse réelle du client, Evey sait
distinguer un spectateur à la maison d'un spectateur sur internet. C'est sur
cette distinction que reposent le plafond de débit WAN et le verrou d'accès
distant, et c'est pourquoi les deux réglages ne se comportent correctement
qu'ensemble :

- `EVEY_TRUSTED_PROXIES` doit couvrir le proxy (`172.16.0.0/12` quand c'est un
  conteneur du réseau Docker ; rien à ajouter quand il tourne sur l'hôte, la
  boucle locale est toujours de confiance). Ce que vous y mettez S'AJOUTE à la
  boucle locale de base, il ne la remplace jamais : une valeur Docker ne peut
  donc pas empêcher discrètement le verrou de voir les vraies adresses des
  clients.
- Des proxys chaînés doivent chacun préserver `X-Forwarded-For`. Chez Caddy,
  cela s'écrit `servers { trusted_proxies static private_ranges }` ; nginx
  utilise `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`.

**Activer cela change qui compte comme distant.** Avant, chaque requête semblait
venir de l'adresse privée du proxy et tout le monde était classé LAN. Après, les
vrais clients distants sont correctement identifiés, et si l'accès distant est
désactivé ils sont refusés sur toutes les routes
(`403 {"code":"remote_access_disabled"}`). C'est exactement cet enchaînement qui
a mis tout un foyer à la porte d'un serveur en parfait état de marche pendant
deux jours : la configuration du proxy avait changé, aucun réglage applicatif
n'avait bougé, et le seul symptôme visible était des clients en échec, sans la
moindre erreur côté serveur.

Evey s'en prémunit désormais : l'accès distant est activé par défaut dès qu'une
URL publique est configurée, son état est journalisé à chaque démarrage, les
refus sont journalisés avec l'adresse IP du client, `evey admin doctor` échoue
bruyamment sur un serveur « publié mais qui refuse », et le panneau
d'administration affiche un avertissement. Après avoir modifié les réglages de
confiance du proxy, repassez un `evey admin doctor`.
