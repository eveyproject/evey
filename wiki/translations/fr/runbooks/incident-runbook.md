---
source_sha: d6c99c91037371ee077e57625c65e0ac542e80e0737a6e83c3320644603ab562
---

# Runbook d'incident

Le manuel de première réaction aux pannes courantes d'Evey. Chaque section suit le même plan : **symptôme → diagnostic → correctif**.

Les conteneurs : `evey-server`, `evey-postgres`, `evey-meilisearch`, `evey-caddy` (en production), `evey-web` et `evey-mailcatcher` (surcharge de développement). Une installation par défaut n'a **que `evey-server`** : Postgres et Meilisearch tournent dedans. Remplacez donc partout ci-dessous `docker exec evey-postgres psql -U ...` par `docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey ...'`, et sachez que leurs journaux sont les lignes `postgres:` et Meili de `docker logs evey-server`.

Port HTTP par défaut : `9091` (`EVEY_HTTP_PORT`). Point d'accès de santé : `/healthz`.

**Les journaux.** Deux copies du même flux : le fichier quotidien `evey-data/logs/evey-YYYY-MM-DD.log` (`EVEY_LOG_DIR`, `/data/logs` dans l'image) et `docker logs evey-server`. C'est le fichier qui fait foi : le pilote json-file fait tourner `docker logs`, et après une rotation la commande peut s'arrêter des heures avant le conteneur - le `stop_detail` d'un test de charge ou un verdict de calibration semble alors manquant alors qu'il n'est que passé à la rotation. Quand les deux divergent, lisez le fichier, ou en root les fichiers bruts `/var/lib/docker/containers/<id>/*-json.log*`. Les commandes en une ligne ci-dessous supposent une fenêtre récente, sans rotation.

## 1. Serveur injoignable

**Symptôme** : les clients n'arrivent pas à se connecter ; `curl http://localhost:9091/healthz` expire ou renvoie une connexion refusée.

**Diagnostic** :
```sh
docker ps --filter name=evey-server
docker logs --tail=200 evey-server
docker inspect --format='{{.State.Health.Status}}' evey-server
ss -ltnp | grep 9091                # le port écoute-t-il vraiment ?
curl -v http://localhost:9091/healthz
```

**Causes fréquentes et correctifs** :
- Le conteneur s'est arrêté → `docker logs evey-server` pour trouver la panique, corrigez, puis `docker compose up -d evey`.
- Le contrôle de santé échoue parce que la base ou Meili est à terre → voir les sections 4 et 5.
- Collision de port sur l'hôte → changez `EVEY_HTTP_PORT`, ou libérez le port.
- Le Caddy en frontal ne route pas → `docker logs evey-caddy`, vérifiez `EVEY_DOMAIN` et que le contrôle de santé d'`evey` est au vert (Caddy en dépend).

## 2. Toutes les connexions répondent 401

**Symptôme** : tous les utilisateurs - y compris les administrateurs connectés la veille - reçoivent un 401 depuis `/api/v1/auth/*`.

**Diagnostic** :
```sh
docker logs --since 10m evey-server | grep -iE 'epoch|jwt|auth|signing|private key'
docker exec evey-server ls -l /data/jwt.key
```

**Cherchez d'abord la ligne « epoch ».** Chaque requête authentifiée revérifie
en base l'époque du jeton de l'utilisateur (ADR-0019), et ce contrôle échoue
FERMÉ. Une base lente, verrouillée ou injoignable fait donc renvoyer à toutes
les requêtes du serveur le même 401 que renvoie un jeton expiré - ce qui
ressemble exactement à un problème de JWT, sans en être un. Le serveur le dit
explicitement :

```
auth rejected: the token epoch could not be read, so every authenticated
request is failing - check the database, not the JWT key
```

Si cette ligne est là, arrêtez de lire cette section et allez au §1 (base de
données). `/healthz` peut très bien rester vert ici : il ne fait que sonder la
connexion, il ne lance pas la requête d'époque. Le compteur Prometheus est
`evey_auth_failures_total{reason="epoch_lookup_failed"}`.

**Causes fréquentes et correctifs** :
- **Clé JWT absente ou changée** : `EVEY_JWT_PRIVATE_KEY_PATH` (`/data/jwt.key` par défaut) illisible, ou clé ayant fait l'objet d'une rotation. Tout jeton signé par l'ancienne clé est désormais invalide - il n'existe pas d'autre issue que restaurer l'ancienne clé depuis une sauvegarde, ou faire se reconnecter tout le monde. Vérifiez que le volume `evey-data` est bien monté sur `/data`.
- **La limite de débit se déclenche sur chaque requête** : regardez `EVEY_RATELIMIT_LOGIN_MAX` (`10` par IP et par minute, par défaut) et `EVEY_RATELIMIT_GLOBAL_MAX` (`300` par défaut). Un reverse proxy qui transmet tout le trafic sous une seule IP la fait sauter très vite - renseignez `EVEY_TRUSTED_PROXIES`, puis redémarrez. La surcharge de développement pose déjà `1000` et `10000`.
- Décalage d'horloge entre le serveur et les clients → `date -u` sur l'hôte.

## 3. Le transcodage échoue, ou les vidéos refusent de se lire transcodées

**Symptôme** : la lecture directe fonctionne, mais tout échoue dès qu'un transcodage est nécessaire. Le lecteur affiche des erreurs, les journaux du serveur montrent des codes de sortie ffmpeg.

**Diagnostic** :
```sh
docker exec evey-server which ffmpeg ffprobe
docker exec evey-server ffmpeg -version | head -1
docker logs --since 10m evey-server | grep -iE 'ffmpeg|transcode|hls'
docker exec evey-server ls -ld /cache/transcode /data/trickplay
docker exec evey-server df -h /cache
```

**Causes fréquentes et correctifs** :
- **Binaire ffmpeg absent** (image sur mesure qui ne l'embarque pas) : recompilez depuis le `Dockerfile` officiel.
- **Répertoire de cache non accessible en écriture** : `EVEY_TRANSCODING_CACHE_DIR` vaut `/data/transcode` par défaut dans le code, mais le compose le fait correspondre à `/cache/transcode` via le volume nommé `evey-cache`. `EVEY_TRICKPLAY_CACHE_DIR` vaut `/cache/trickplay` par défaut. Vérifiez qu'on peut y écrire :
  ```sh
  docker exec evey-server touch /cache/transcode/.write-test && \
    docker exec evey-server rm /cache/transcode/.write-test
  ```
- **Cache plein** → voir la section 6.
- **Accélération matérielle mal configurée** : posez `EVEY_TRANSCODING_HW_ACCEL_ENABLED=false` pour forcer le logiciel, redémarrez, et confirmez que la régression vient bien du matériel avant de le réactiver.
- **Trop de transcodages simultanés** : `EVEY_TRANSCODING_MAX_CONCURRENT` (`2` par défaut). À augmenter prudemment, si la machine a des cœurs.

### 3c. « Ça met en tampon / ça a coupé » et les journaux ne disent pas pourquoi

**Symptôme** : un spectateur signale des blocages ou des coupures sur un titre
transcodé, et les journaux du serveur montrent `hls-vod: segment timeout` avec
`err:"context canceled"` - autrement dit c'est le CLIENT qui a raccroché, pas le
serveur qui a expiré.

**Diagnostic** : le serveur consigne déjà chacune de ses décisions. Commencez
par là, c'est gratuit :

```sh
# Pourquoi une requête de segment a réemployé un encodeur ou en a lancé un, et
# ce que cela a coûté au lecteur. Consigné seulement quand ça coûte quelque
# chose : une attente au-delà d'un segment, ou un lancement.
# `over_client_patience: true` est la preuve accablante.
docker exec evey-server grep '"software: reach decision"' /data/logs/evey-$(date +%F).log | tail -40

# Les encodeurs eux-mêmes : lancements, arrêts et leur motif.
docker exec evey-server grep -E '"software: session (started|killed)"|per-hash cap' \
  /data/logs/evey-$(date +%F).log | tail -40
```

Ces lignes disent ce que le serveur a *décidé*. Quand il vous faut ce que ffmpeg
a réellement *accompli*, seconde par seconde - la position réelle du front
d'encodage au fil du temps - activez l'enregistrement de la progression par
encodeur :

```sh
# ACTIVER. S'applique au PROCHAIN encodeur ; aucun redémarrage nécessaire.
# $TOKEN est un JWT d'administrateur.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"true"}'

# Reproduisez le problème, puis lisez une session : out_time = contenu produit
# jusque-là, speed = l'avance de l'encodeur sur le temps réel.
docker exec evey-server ls -t /data/logs/ffmpeg | head
docker exec evey-server tail -20 /data/logs/ffmpeg/<file>.log

# DÉSACTIVER une fois l'investigation terminée.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"false"}'
```

**Laissez-le coupé en dehors d'une investigation.** C'est un fichier par
encodeur, quelques Mo par session, écrit pour chaque lecture sur la machine -
du vrai disque et de vraies écritures sur un NAS modeste. Le nom du fichier
porte le hachage de la décision, le palier (`-vN-`) et le segment de départ
(`-sN-`), ce qui le met en regard du journal JSON ci-dessus ; un relancement à
la même position obtient son propre fichier au lieu d'écraser la tentative
précédente, et c'est précisément ce qui rend visible le brassage d'encodeurs.

**Causes fréquentes et correctifs** :
- **Des petites navigations avant, répétées** : chacune peut lancer un encodeur
  neuf, et le plafond par hachage (`MaxSessionsPerHash`, 2 par défaut) évince le
  moins récemment employé. Attendez-vous à un `session killed reason:lru_cap`
  dans le journal. C'est normal, à dose raisonnable.
- **Le client est moins patient que le serveur** : c'est sur
  `clientSegmentPatience` (6 s) qu'est dimensionnée la fenêtre d'atteinte après
  navigation ; AndroidX Media3 abandonne un segment au bout d'environ 8 s,
  quand le `WaitBudgetDur` du serveur vaut 60 s. Une ligne `reach decision`
  portant `over_client_patience: true` signifie que le serveur s'apprêtait à
  demander une attente qu'aucun lecteur ne supporterait.

### 3b. Des 410 `stale_rendition`, le lecteur recharge son master en pleine lecture

**Symptôme** : le lecteur bloque brièvement puis repart ; les journaux montrent
`hls-vod: stale rendition URL refused (the ladder moved under a live player)`
et des requêtes répondues en `410 Gone` avec `X-Evey-Error: stale_rendition`.

**Ce que cela veut dire** : c'est une *protection qui se déclenche*, pas la
panne elle-même. Chaque URI de manifeste porte le hachage de décision sous
lequel elle a été construite (`fp=`). Un 410 signifie qu'un lecteur a présenté
une URL issue d'une session dont l'échelle ABR a depuis été remodelée : la
servir reviendrait à livrer au décodeur des octets appartenant à une autre
déclinaison (la classe d'incidents dite de l'« écran arc-en-ciel », ADR 0022
§1b). Le refus force à la place un rechargement propre du master.

**Diagnostic** : le WARN porte `sealed_fp`, `current_fp` et l'identifiant du média.
- Une **rafale juste après un déploiement, un redémarrage ou une recalibration
  d'administration** est attendue : des sessions vivantes ont survécu à un
  changement de capacité. Aucune action ; les lecteurs se resynchronisent en une
  scrutation.
- Un **filet continu en fonctionnement normal** signifie que quelque chose
  remodèle sans cesse les échelles sous des flux en cours - cherchez une
  capacité qui bat (`docker logs evey-server | grep -iE 'calibrat|capability'`),
  un modeleur d'admission qui brasse sous la charge, ou une dérogation de
  plafond CMCD qui oscille.
- Les lecteurs qui **bouclent sur des 410 sans jamais s'en remettre** détiennent
  un master en cache (ancienne version de l'application, proxy trop agressif sur
  la mise en cache des `.m3u8`). Vérifiez que le proxy ne met pas les manifestes
  en cache (`Cache-Control: no-cache` doit passer sans être altéré).

## 4. Connexion à la base refusée, ou erreurs de base

**Symptôme** : les journaux du serveur contiennent `dial tcp ... connect: connection refused`, `pq: ...`, ou les requêtes expirent.

**Diagnostic** :
```sh
docker ps --filter name=evey-postgres
docker inspect --format='{{.State.Health.Status}}' evey-postgres
docker exec evey-postgres pg_isready -U "$EVEY_DB_USER"
docker logs --tail=200 evey-postgres
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
  SELECT count(*), state
    FROM pg_stat_activity
   WHERE datname = current_database()
   GROUP BY state;"
```

**Causes fréquentes et correctifs** :
- Conteneur Postgres arrêté → `docker compose up -d postgres`, puis surveillez les journaux (corruption, OOM).
- Mauvais identifiants → l'environnement du serveur doit correspondre aux `EVEY_DB_USER`, `EVEY_DB_PASSWORD` et `EVEY_DB_NAME` qu'emploie le service `postgres`. Valeurs par défaut : `evey` / `evey` / `evey`.
- Pool épuisé (`too many connections`, ou de longues attentes) → regardez `EVEY_DB_MAX_OPEN_CONNS` (`50` par défaut). Tuez les sessions coincées :
  ```sql
  SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
   WHERE state = 'idle in transaction'
     AND now() - state_change > interval '5 minutes';
  ```
- Disque plein sur `postgres-data/` → `df -h`, libérez de la place, redémarrez.

## 5. Meilisearch est à terre

**Symptôme** : la recherche rend des résultats dégradés, les journaux montrent `meilisearch unavailable, using SQL search fallback`.

**Comportement** : Evey dispose d'un repli robuste - la recherche continue de fonctionner en SQL (`cmd/evey/wire.go` câble un adaptateur de recherche résilient, qui journalise `meilisearch unavailable, using SQL search fallback` quand Meili est injoignable). Aucune action utilisateur n'est nécessaire pour rester en ligne ; seule la qualité du classement se dégrade.

**À noter sur le mode de déploiement** : le `docker-compose.yml` par défaut fait tourner Meilisearch **embarqué dans le conteneur `evey-server`** (`EVEY_EMBEDDED_MEILI: "true"`). Un conteneur `evey-meilisearch` séparé n'existe que dans la surcharge de développement (`docker-compose.override.yml`). En mode embarqué, « Meilisearch est à terre » veut dire que le processus evey-server est en mauvaise santé : il n'y a pas de conteneur distinct à redémarrer.

**Diagnostic et correctif (embarqué, le cas par défaut)** :
```sh
docker ps --filter name=evey-server
curl -sf http://localhost:7700/health          # port redirigé depuis l'intérieur d'evey-server
docker logs --tail=200 evey-server | grep -i meili
```
- Serveur en mauvaise santé → `docker compose restart evey-server`.
- Index corrompu → arrêtez evey-server, sauvegardez `./evey-data/meili-data/`, effacez, redémarrez, puis déclenchez une réindexation complète depuis le panneau d'administration.

**Diagnostic et correctif (conteneur séparé, surcharge de développement seulement)** :
```sh
docker ps --filter name=evey-meilisearch
curl -sf http://localhost:7700/health
docker logs --tail=200 evey-meilisearch
```
- Conteneur arrêté → `docker compose up -d meilisearch`.
- `MEILI_MASTER_KEY` désaccordée → les journaux du serveur montrent des échecs d'authentification. Alignez la variable sur les deux services, puis redémarrez.
- Index corrompu → arrêtez Meili, sauvegardez `./meili-data/`, effacez, redémarrez, puis déclenchez une réindexation complète depuis le panneau d'administration.

## 6. Disque du cache de transcodage plein

**Symptôme** : les transcodages échouent en `no space left on device` ; le contrôle de santé du conteneur bat.

**Diagnostic** :
```sh
docker exec evey-server df -h /cache
docker exec evey-server du -sh /cache/transcode /cache/trickplay /data/trickplay 2>/dev/null
docker volume inspect evey_evey-cache   # le nom malmené par compose peut varier
```

**Correctif** :
- Purge sans risque, le cache étant régénérable :
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/transcode/*'
  ```
  Elle emporte aussi ce qu'il contient sous `photos/` (les variantes de photos
  redimensionnées) et `track-art/` (la pochette embarquée dans les tags d'une
  piste, extraite une fois puis redimensionnée) - les deux se reconstruisent à
  la requête suivante. Le bouton « Clear cache » du panneau d'administration
  fait exactement la même purge.
- Le trickplay est plus coûteux à régénérer ; ne le purgez qu'en cas de nécessité :
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/trickplay/* /data/trickplay/*'
  ```
- Abaissez le budget du cache de transcodage (Admin > Paramètres du serveur, `10` Go par défaut) si le volume de l'hôte est juste ; quel que soit le budget, le balayeur garde 2 Gio libres sur le volume du cache.

## 7. Une extension a planté

**Symptôme** : une extension cesse de répondre ; les journaux du serveur montrent des paniques venues du lanceur d'extensions.

**Diagnostic** :
```sh
docker logs --since 30m evey-server 2>&1 | grep -iE 'plugin|pluginrunner|panic'
# Cherchez : "panic in plugin runner", "plugin process crashed", "remove plugin sock on crash"
```

**Correctif** :
- Tentez d'abord un redémarrage chirurgical, sans interruption du conteneur : `POST /api/v1/admin/plugins/<id>/enable`. Cela fonctionne même quand l'extension est déjà `enabled` mais coincée en statut `Error` - le garde-fou de `StartPlugin` la relance dès lors que le statut n'est pas `Running`. Préférez cette voie au redémarrage du serveur entier, en particulier quand l'accès SSH à l'hôte n'est pas immédiat.
- Si le redémarrage chirurgical n'y suffit pas, désactivez plutôt l'extension fautive depuis le panneau d'administration.
- Si c'est le serveur lui-même qui est instable, redémarrez : `docker compose restart evey`.
- Inspectez les fichiers de l'extension dans `./evey-data/plugins/` (hôte) == `/data/plugins` (conteneur). Supprimez le zip ou le répertoire cassé, puis redémarrez.
- Envoyez la trace de la panique et l'identifiant de l'extension à son auteur.

## 8. Une diffusion a gelé, s'est tue, ou le téléphone tourne sans fin

Tout ce qui suit se lit depuis la page Logs de l'administration (ou par
`GET /api/v1/admin/logs/search?q=<term>&limit=200`) : aucun shell sur la machine
n'est nécessaire. Les heures sont en UTC. Chaque balise est une ligne
`player-client: playback event reported`, avec un `type=` et une chaîne
`details=`. Les identifiants n'apparaissent jamais dans ce que renvoient ces
routes (bearers, JWT et valeurs `token=`/`password=` s'affichent
`[redacted]`) : le jeton d'une URL de cast ne se retrouve donc pas par une
recherche dans les logs - cherchez l'identifiant du média, pas le jeton.

1. **Le récepteur était-il vivant, et jusqu'à quand ?** Cherchez
   `cast_receiver_alive`. Le récepteur en émet une toutes les 30 s pendant la
   lecture ; `details` porte `buf=<behind>/<ahead>/<ranges>` (les secondes en
   tampon derrière et devant la tête de lecture) et `pos_sec`. Une lecture saine
   affiche un `ahead` de plusieurs dizaines de secondes. **L'heure de la
   dernière balise est celle où le récepteur est mort ou a été tué** : plus rien
   après elle signifie que le téléviseur ne fait plus tourner le récepteur, quoi
   qu'affiche le téléphone.
2. **Pourquoi s'est-il arrêté ?** Cherchez `cast_receiver_finished` :
   `reason=INTERRUPTED` désigne un nouveau LOAD qui remplace celui en cours
   (cherchez un `StartStream` quelques secondes plus tôt), `reason=ERROR` porte
   l'erreur du lecteur. Aucun `finished` du tout après le dernier `alive` = le
   processus du récepteur a disparu (le plus souvent, un Chromecast HD tué pour
   cause de mémoire sur un master de copie à plus de 20 Mbit/s).
3. **Qu'a vu le téléphone ?** Cherchez `cast_sender_stall` : l'émetteur poste
   `state=playing frozen=<n>s pos=<p>` quand le SDK Cast dit encore PLAYING
   alors que la position n'a pas bougé depuis 20 s, et `state=idle reason=<r>`
   quand le récepteur a rapporté un motif d'inactivité. Ces lignes sont
   journalisées en WARN : le filtre « avertissements » de la page Logs les fait
   donc remonter. C'est le versant émetteur de la même minute - comparez son
   `pos` au dernier `pos_sec` du récepteur.
4. **La session a-t-elle fait boule de neige ?** Cherchez `refresh rejected` :
   un jeton de rafraîchissement « not found » déconnecte puis reconnecte
   l'application (identifiants enregistrés), ce qui réémet le flux - la ligne
   dit désormais le `jti` du jeton, son `issued` et son `age`, de quoi
   distinguer une copie périmée d'une contrefaçon. Cherchez `StartStream` pour
   voir les réémissions ; l'émetteur ne recharge plus le récepteur sur une URL
   réémise pour le même titre.
5. **Le flux était-il trop gros pour l'appareil ?** Cherchez
   `advertises a fallback rung` (le débit crête du master de copie) et
   `plan fitted to the client's line` (la mesure `maxKbps` du récepteur ou
   son secours `abr=floor`, désormais des entrées du plan - le master
   transcodé qui suit porte le motif `client_line_exceeded` ou
   `abr_floor_requested`) : une session qui démarre sur un master de copie à
   plus de 20 Mbit/s et retombe sur un palier 1080p ajusté après quelques
   secondes, c'est le sauvetage par le plancher qui fait son travail ; un
   re-LOAD qui repart sur le master de copie, c'est le motif qui tue un
   Chromecast HD.
6. **Famine côté serveur ?** Cherchez `on-demand resource refused` : une rafale
   de refus en `context canceled` sur des segments consécutifs est un
   déplacement de la tête de lecture (chaque requête annulée par la suivante),
   pas une faute du serveur ; les lignes `served` aux durées longues, si.

Si les étapes 1 à 3 montrent un récepteur vivant et un émetteur muet, le
problème est dans la session Cast du téléphone (le SDK, le réseau jusqu'au
téléviseur) ; si ce sont les balises du récepteur qui s'arrêtent d'abord, c'est
du côté du téléviseur qu'il faut chercher (mémoire, codec, débit du flux).

## Tuer une session ou un appareil compromis

Symptôme : un administrateur voit un appareil inattendu dans **Now Playing** (panneau d'administration), ou un utilisateur signale un appareil inconnu dans sa liste de sessions.

1. Identifiez le `session_id` :
   ```sh
   docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
     SELECT id, user_id, profile_id, device, ip_address, started_at
     FROM play_sessions
     WHERE ended_at IS NULL
     ORDER BY started_at DESC LIMIT 20;"
   ```
2. Tuez-la depuis l'interface d'administration : **Admin → Activity → Now Playing → ⋯ → Kill session**, ou par le gRPC `AdminSessionService/KillSession` muni du `session_id`. La session est effectivement terminée 5 s plus tard, pour que le lecteur ait le temps de recevoir l'ordre d'arrêt.
3. Si la session ne répond pas, supprimez de force la session HLS (le répertoire est `<cacheDir>/<mediaItemID>`, soit `/cache/transcode/<mediaItemID>` par défaut) :
   ```sh
   docker exec evey-server rm -rf /cache/transcode/<media_item_id>/
   ```
4. S'il faut aussi révoquer le jeton de rafraîchissement (mot de passe soupçonné compromis) :
   ```sh
   docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
     DELETE FROM refresh_tokens WHERE user_id = '<user_id>';"
   ```
   Tous les appareils de cet utilisateur devront se reconnecter. Pour un verrouillage complet, réinitialisez le mot de passe par `evey admin reset-password --user <username>`.

## Débloquer une analyse de bibliothèque coincée

Symptôme : une analyse de bibliothèque affiche « in progress » depuis des heures, sans le moindre avancement fichier dans les journaux.

1. Contrôlez indirectement la table `scan_service.scanning` - si le processus est bloqué, le processeur est au repos et les journaux récents ne montrent aucun `scan_service` :
   ```sh
   docker compose logs --tail=500 evey-server | grep scan_service
   ```
2. Le verrou d'analyse vit **en mémoire** (aucune ligne en base) : un redémarrage du serveur le libère donc :
   ```sh
   docker compose restart evey-server
   ```
3. Si un fichier précis provoque des paniques à répétition (voir les traces de pile), mettez-le en quarantaine en le sortant du chemin de la bibliothèque, puis relancez l'analyse.
4. Si les veilles inotify sont épuisées (limite du noyau), relevez-la :
   ```sh
   echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf
   sudo sysctl -p
   ```

## Aide-mémoire du diagnostic général

```sh
# État de tout
docker compose ps
curl -sf http://localhost:9091/healthz && echo healthy

# Suivre les journaux de tous les conteneurs
docker compose logs --tail=100 -f

# Vidage de la configuration du serveur
docker exec evey-server env | grep ^EVEY_ | sort

# Taille et verrous Postgres
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
  SELECT pg_size_pretty(pg_database_size(current_database()));"

# Disque de l'hôte
df -h . ./postgres-data ./meili-data ./evey-data
```
