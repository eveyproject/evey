---
source_sha: 86f34e154bd76e23fa0313aeb2bed55714515156f1084f6bffefe2812d4ad4d0
reviewed: true
---
# Réponse aux incidents

Les premiers gestes face aux pannes courantes d'Evey. Chaque section suit le même
fil : **symptôme → diagnostic → correction**.

Les commandes visent l'installation standard : un seul conteneur, `evey-server`,
dans lequel tournent PostgreSQL et Meilisearch ; leurs journaux sont les lignes
`postgres:` et Meilisearch de `docker logs evey-server`. La disposition du
`docker-compose.yml` du dépôt ajoute des conteneurs `evey-postgres` et
`evey-caddy` ([Déploiements avancés](../operations/advanced-deployments.md)).

Pour lancer du SQL sur la base intégrée :

```sh
docker exec evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

Port HTTP par défaut : `9091` (`EVEY_HTTP_PORT`). Point de santé : `/healthz`.

**Journaux.** Deux copies du même flux : le fichier quotidien
`evey-data/logs/evey-AAAA-MM-JJ.log` (`EVEY_LOG_DIR`, `/data/logs` dans l'image)
et `docker logs evey-server`. Le fichier fait foi : le pilote json-file fait
tourner `docker logs`, et après une rotation la commande peut s'arrêter des
heures avant le conteneur ; le `stop_detail` d'un test de charge ou le verdict
d'une calibration semble alors manquer alors qu'il a seulement été écarté par la
rotation. Quand les deux divergent, lisez le fichier, ou, en root, les fichiers
bruts `/var/lib/docker/containers/<id>/*-json.log*`. Les commandes
`docker logs` ci-dessous supposent une fenêtre récente, sans rotation.

## 1. Le serveur ne répond pas

**Symptôme** : les clients n'arrivent pas à se connecter ;
`curl http://localhost:9091/healthz` expire ou renvoie « connection refused ».

**Diagnostic** :
```sh
docker ps --filter name=evey-server
docker logs --tail=200 evey-server
docker inspect --format='{{.State.Health.Status}}' evey-server
ss -ltnp | grep 9091                # le port écoute-t-il vraiment ?
curl -v http://localhost:9091/healthz
```

**Causes fréquentes et correction** :

- Conteneur arrêté → `docker logs evey-server` pour la panique, corriger,
  `docker compose up -d evey`.
- Contrôle de santé en échec parce que la base ou Meili est tombé → voir les
  sections 4 et 5.
- Conflit de port sur la machine hôte → changer `EVEY_HTTP_PORT` ou libérer le
  port.
- Caddy devant ne route pas → `docker logs evey-caddy`, vérifier `EVEY_DOMAIN`
  et que le contrôle de santé d'`evey` est au vert (Caddy en dépend).

## 2. Toutes les connexions renvoient 401

**Symptôme** : chaque utilisateur, y compris les administrateurs connectés la
veille, reçoit un 401 de `/api/v1/auth/*`.

**Diagnostic** :
```sh
docker logs --since 10m evey-server | grep -iE 'epoch|jwt|auth|signing|private key'
docker exec evey-server ls -l /data/jwt.key
```

**Cherchez d'abord la ligne sur l'epoch.** Chaque requête authentifiée revérifie
l'epoch du jeton de l'utilisateur auprès de la base (ADR-0019), et ce contrôle
échoue FERMÉ. Une base lente, verrouillée ou injoignable fait donc renvoyer à
chaque requête le même 401 qu'un jeton expiré : cela ressemble exactement à un
problème de JWT, et ce n'en est pas un. Le serveur le dit explicitement :

```
auth rejected: the token epoch could not be read, so every authenticated
request is failing - check the database, not the JWT key
```

Si cette ligne est présente, arrêtez-vous là dans cette section et passez au §4
(base de données). `/healthz` peut rester au vert : il ne fait qu'un ping de la
connexion, sans lancer la requête d'epoch. Le compteur Prometheus correspondant
est `evey_auth_failures_total{reason="epoch_lookup_failed"}`.

**Causes fréquentes et correction** :

- **Clé JWT absente ou changée** : `EVEY_JWT_PRIVATE_KEY_PATH` (par défaut
  `/data/jwt.key`) illisible, ou clé renouvelée. Chaque jeton signé par
  l'ancienne clé est désormais invalide : il n'y a pas d'autre remède que de
  restaurer l'ancienne clé depuis une sauvegarde ou de faire reconnecter tout le
  monde. Vérifiez que le volume `evey-data` est bien monté sur `/data`.
- **La limite de débit se déclenche à chaque requête** : vérifiez
  `EVEY_RATELIMIT_LOGIN_MAX` (par défaut `10` par IP et par minute) et
  `EVEY_RATELIMIT_GLOBAL_MAX` (par défaut `300`). Un reverse proxy qui fait
  passer tout le trafic sous une seule IP la déclenche vite : ajoutez le proxy
  dans **Proxys de confiance**, dans Admin → Accès distant (appliqué sans
  redémarrage ; `EVEY_TRUSTED_PROXIES` fonctionne aussi). La surcouche de
  développement règle déjà `1000` / `10000`.
- Décalage d'horloge entre le serveur et les clients → `date -u` sur la machine
  hôte.

## 3. Le transcodage échoue, ou les vidéos ne se lisent pas en transcodage

**Symptôme** : la lecture fonctionne en lecture directe, échoue dès qu'un
transcodage est nécessaire. Erreurs dans le lecteur, codes de sortie ffmpeg dans
les journaux du serveur.

**Diagnostic** :
```sh
docker exec evey-server which ffmpeg ffprobe
docker exec evey-server ffmpeg -version | head -1
docker logs --since 10m evey-server | grep -iE 'ffmpeg|transcode|hls'
docker exec evey-server ls -ld /cache/transcode /cache/trickplay
docker exec evey-server df -h /cache
```

**Causes fréquentes et correction** :

- **Binaire ffmpeg absent** (image personnalisée sans lui) : reconstruire depuis
  le `Dockerfile` officiel.
- **Dossier de cache non inscriptible** : `EVEY_TRANSCODING_CACHE_DIR` vaut
  `/data/transcode` par défaut dans le code, mais le compose le place sur
  `/cache/transcode` via le volume nommé `evey-cache`.
  `EVEY_TRICKPLAY_CACHE_DIR` vaut `/cache/trickplay` par défaut. Vérifiez qu'on
  peut y écrire :
  ```sh
  docker exec evey-server touch /cache/transcode/.write-test && \
    docker exec evey-server rm /cache/transcode/.write-test
  ```
- **Cache plein** → voir la section 6.
- **L'accélération matérielle se comporte mal** : réglez **Accélération
  matérielle** sur **Désactivée (processeur uniquement)** dans **Admin →
  Configuration transcodage → Calibrage de l'encodage** (appliqué à chaud, avec
  une nouvelle mesure) pour garder tous les transcodages sur le processeur, et
  confirmez que le problème vient du matériel avant de revenir à **Auto**.
- **« Le serveur est occupé par d'autres transcodages »** : la capacité en
  direct est mesurée, pas réglée ; voir [Transcodage et capacité du serveur](../playback/transcoding.md#sharing-the-capacity).
  `EVEY_TRANSCODING_MAX_CONCURRENT` (**Maximum simultané** dans **Admin → Cache
  de transcodage**) ne couvre que les tâches de transcodage hors lecture en
  direct.

### 3c. « Ça charge en boucle » ou « ça a coupé », et les journaux ne disent pas pourquoi

**Symptôme** : un spectateur signale des blocages ou des coupures sur un titre
transcodé, et les journaux du serveur montrent `hls-vod: segment timeout` avec
`err:"context canceled"`, ce qui signifie que c'est le CLIENT qui a raccroché,
pas le serveur qui a expiré.

**Diagnostic** : le serveur enregistre déjà chaque décision qu'il a prise.
Commencez par là, cela ne coûte rien :

```sh
# Pourquoi une requête de segment a réutilisé un encodeur ou en a lancé un, et ce
# que cela a coûté au lecteur. Journalisé seulement quand cela coûte quelque chose :
# une attente de plus d'un segment, ou un lancement. `over_client_patience: true`
# est la preuve formelle.
docker exec evey-server grep '"software: reach decision"' /data/logs/evey-$(date +%F).log | tail -40

# Les encodeurs eux-mêmes : lancements, arrêts et leur raison.
docker exec evey-server grep -E '"software: session (started|killed)"|per-hash cap' \
  /data/logs/evey-$(date +%F).log | tail -40
```

Ces lignes disent ce que le serveur a *décidé*. Quand il vous faut ce que ffmpeg
a réellement *accompli* seconde après seconde (la position réelle de la
frontière d'encodage au fil du temps), activez l'enregistrement de progression
par encodeur :

```sh
# ACTIVER. S'applique au PROCHAIN encodeur ; pas de redémarrage. $TOKEN est un JWT d'administrateur.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"true"}'

# Reproduisez le problème, puis lisez une session : out_time = contenu produit
# jusqu'ici, speed = avance de l'encodeur sur le temps réel.
docker exec evey-server ls -t /data/logs/ffmpeg | head
docker exec evey-server tail -20 /data/logs/ffmpeg/<file>.log

# DÉSACTIVER une fois terminé.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"false"}'
```

**Laissez-le désactivé hors enquête.** C'est un fichier par encodeur, quelques
Mo par session, écrit pour chaque lecture sur la machine : du vrai disque et de
vraies écritures sur un NAS modeste. Le nom du fichier porte le hachage de la
décision, le barreau (`-vN-`) et le segment de départ (`-sN-`), ce qui permet de
le rapprocher du journal JSON ci-dessus ; un nouveau lancement à la même position
a son propre fichier au lieu d'écraser le précédent, et c'est ce qui rend visible
le renouvellement incessant des encodeurs.

**Causes fréquentes et correction** :

- **Des déplacements courts et répétés vers l'avant** : chacun peut lancer un
  nouvel encodeur, et le plafond par hachage (`MaxSessionsPerHash`, 2 par
  défaut) évince le moins récemment utilisé. Attendez-vous à
  `session killed reason:lru_cap` dans le journal. Normal avec modération.
- **Le client est moins patient que le serveur** : `clientSegmentPatience`
  (6 s) sert à dimensionner la fenêtre d'atteinte après un déplacement ;
  AndroidX Media3 abandonne un segment vers 8 s alors que le `WaitBudgetDur` du
  serveur est de 60 s. Une ligne `reach decision` avec
  `over_client_patience: true` signifie que le serveur s'apprêtait à demander
  une attente qu'aucun lecteur ne supporterait.

### 3b. Des 410 `stale_rendition`, ou un lecteur qui recharge son master en pleine lecture

**Symptôme** : le lecteur se bloque brièvement puis repart ; les journaux
montrent `hls-vod: stale rendition URL refused (the ladder moved under a live player)`
et des requêtes répondues `410 Gone` avec `X-Evey-Error: stale_rendition`.

**Ce que cela signifie** : c'est une *protection qui se déclenche*, pas la panne
elle-même. Chaque URI de manifeste porte le hachage de la décision sous laquelle
il a été construit (`fp=`). Un 410 signifie qu'un lecteur a présenté l'URL d'une
session dont l'échelle ABR a été remaniée depuis : la servir donnerait au
décodeur des octets d'une autre variante (la classe d'incidents de « l'écran
arc-en-ciel », ADR 0022 §1b). Le refus force à la place un rechargement propre
du master.

**Diagnostic** : l'avertissement porte `sealed_fp` et `current_fp`, ainsi que
l'identifiant du média.

- Une **rafale juste après un déploiement, un redémarrage ou une recalibration
  par l'administrateur** est attendue : des sessions en cours ont survécu à un
  changement de capacité. Rien à faire ; les lecteurs se resynchronisent en une
  interrogation.
- Un **goutte-à-goutte régulier en fonctionnement normal** signifie que quelque
  chose remanie sans cesse les échelles sous des flux en cours : cherchez une
  capacité qui oscille (`docker logs evey-server | grep -iE 'calibrat|capability'`),
  un régulateur d'admission qui s'agite sous la charge, ou un plafond CMCD qui
  oscille.
- Des lecteurs qui **tournent en boucle sur des 410 sans se rétablir** gardent
  un master en cache (ancienne version de l'application, proxy qui met les
  `.m3u8` en cache de façon agressive). Vérifiez que le proxy ne met pas les
  manifestes en cache (`Cache-Control: no-cache` doit passer).

## 4. Connexion à la base refusée, ou erreurs

**Symptôme** : les journaux du serveur contiennent `dial tcp ... connect: connection refused`, `pq: ...`, ou des requêtes qui expirent.

**Diagnostic** :
```sh
docker logs --tail=300 evey-server | grep -iE 'postgres|database'
docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
  SELECT count(*), state
    FROM pg_stat_activity
   WHERE datname = current_database()
   GROUP BY state;"'
df -h ./evey-data
```

**Causes courantes et correctifs** :
- **Disque plein** sous `./evey-data` (la base vit dans `./evey-data/postgres`) → libérez de la place, puis `docker compose restart evey`.
- **Pool de connexions épuisé** (`too many connections` ou longues attentes) → `EVEY_DB_MAX_OPEN_CONNS` (`25` par défaut). Coupez les sessions bloquées :
  ```sql
  SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
   WHERE state = 'idle in transaction'
     AND now() - state_change > interval '5 minutes';
  ```
- **Conteneur PostgreSQL séparé** (disposition du dépôt) : `docker ps --filter name=evey-postgres`, `docker logs --tail=200 evey-postgres` ; les `EVEY_DB_USER` / `EVEY_DB_PASSWORD` / `EVEY_DB_NAME` du serveur doivent correspondre au service `postgres`.

## 5. Meilisearch en panne

**Symptôme** : la recherche renvoie des résultats dégradés, les journaux affichent `meilisearch unavailable, using SQL search fallback`.

**Comportement** : la recherche continue de fonctionner par SQL (`cmd/evey/wire.go` câble un adaptateur de recherche résilient qui journalise cette ligne quand Meilisearch est injoignable). Aucune action n'est nécessaire pour rester en ligne ; seule la qualité du classement baisse.

Meilisearch tourne dans `evey-server` (`EVEY_EMBEDDED_MEILI`, actif par défaut) et n'écoute que sur `127.0.0.1:7700`, à l'intérieur du conteneur.

**Diagnostic et correctif** :
```sh
docker exec evey-server curl -sf http://127.0.0.1:7700/health
docker logs --tail=300 evey-server | grep -i meili
```
- En mauvaise santé → `docker compose restart evey`.
- Index corrompu → arrêtez le conteneur, mettez `./evey-data/meili` de côté, redémarrez, puis reconstruisez l'index depuis la base : `docker exec evey-server evey admin reindex-search`.

## 6. Disque du cache de transcodage plein

**Symptôme** : les transcodages échouent avec `no space left on device` ; le
contrôle de santé du conteneur clignote.

**Diagnostic** :
```sh
docker exec evey-server df -h /cache
docker exec evey-server du -sh /cache/transcode /cache/trickplay 2>/dev/null
docker volume inspect evey_evey-cache   # le nom donné par compose peut varier
```

**Correction** :

- Purge sans risque, le cache se régénère :
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/transcode/*'
  ```
  Cela supprime aussi `photos/` (les variantes redimensionnées des photos) et
  `track-art/` (la pochette intégrée aux tags d'un titre, extraite une fois puis
  redimensionnée) qui s'y trouvent : les deux sont reconstruits à la requête
  suivante. **Tout vider**, dans Admin → Cache de transcodage, fait la même
  purge.
- Les miniatures de navigation coûtent plus cher à régénérer ; ne les purgez
  qu'en cas de besoin :
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/trickplay/*'
  ```
- Baissez le **Budget du cache de transcodage** (**Admin → Configuration du
  serveur → Paramètres du serveur**, 10 Go par défaut) si le volume de la machine est juste ; quel que soit le budget, le
  nettoyeur garde 2 Gio libres sur le volume du cache.

## 7. Un plugin a planté

**Symptôme** : un plugin ne répond plus ; les journaux du serveur montrent des
paniques venant du lanceur de plugins.

**Diagnostic** :
```sh
docker logs --since 30m evey-server 2>&1 | grep -iE 'plugin|pluginrunner|panic'
# Cherchez : "panic in plugin runner", "plugin process crashed", "remove plugin sock on crash"
```

**Correction** :

- Essayez d'abord un redémarrage ciblé, sans arrêter le conteneur :
  `POST /api/v1/admin/plugins/<id>/enable`. Cela fonctionne même quand le plugin
  est déjà `enabled` mais bloqué en état `Error` : la garde de `StartPlugin` le
  relance dès que son état n'est pas `Running`. Préférez-le à un redémarrage de
  tout le serveur, surtout quand l'accès SSH à la machine n'est pas sous la main.
- Si ce redémarrage ciblé ne suffit pas, désactivez le plugin fautif depuis le
  panneau d'administration.
- Si le serveur lui-même est instable, redémarrez : `docker compose restart evey`.
- Inspectez les fichiers des plugins dans `./evey-data/plugins/` (machine hôte)
  == `/data/plugins` (conteneur). Retirez le zip ou le dossier du plugin cassé,
  redémarrez.
- Envoyez la trace de la panique et l'identifiant du plugin à son auteur.

## 8. Une diffusion s'est figée, est devenue muette, ou le téléphone tourne en boucle

Tout ce qui suit se lit depuis la page **Logs & Debug** de l'administration (ou
`GET /api/v1/admin/logs/search?q=<terme>&limit=200`) : aucun shell sur la
machine n'est nécessaire. Les heures sont en UTC. Chaque balise est une ligne
`player-client: playback event reported` avec un `type=` et une chaîne
`details=`. Les identifiants n'apparaissent jamais dans ce que renvoient ces
routes (jetons bearer, JWT et valeurs `token=`/`password=` s'affichent
`[redacted]`) : le jeton d'une URL de diffusion ne se retrouve donc pas par une
recherche dans les journaux. Cherchez l'identifiant du média, pas le jeton.

1. **Le récepteur était-il vivant, et jusqu'à quand ?** Cherchez
   `cast_receiver_alive`. Le récepteur en envoie un toutes les 30 s pendant la
   lecture ; `details` porte `buf=<derrière>/<devant>/<plages>` (secondes en
   mémoire derrière et devant la tête de lecture) et `pos_sec`. Une lecture saine
   montre un `devant` de plusieurs dizaines de secondes. **L'heure de la dernière
   balise est celle où le récepteur est mort ou a été tué** : rien après elle
   signifie que le téléviseur ne fait plus tourner le récepteur, quoi qu'affiche
   le téléphone.
2. **Pourquoi s'est-il arrêté ?** Cherchez `cast_receiver_finished` :
   `reason=INTERRUPTED` est un nouveau LOAD qui remplace celui en cours (cherchez
   un `StartStream` quelques secondes avant), `reason=ERROR` porte l'erreur du
   lecteur. Aucun `finished` après le dernier `alive` = le processus du récepteur
   a disparu (le plus souvent un Chromecast HD tué faute de mémoire sur un master
   en copie à plus de 20 Mb/s).
3. **Qu'a vu le téléphone ?** Cherchez `cast_sender_stall` : l'émetteur envoie
   `state=playing frozen=<n>s pos=<p>` quand le SDK Cast dit encore PLAYING mais
   que la position n'a pas bougé depuis 20 s, et `state=idle reason=<r>` quand le
   récepteur a signalé un motif d'inactivité. Ces lignes sont journalisées en
   WARN : le filtre des avertissements de la page Logs & Debug les fait ressortir.
   C'est la vue de l'émetteur sur la même minute : comparez son `pos` au dernier
   `pos_sec` du récepteur.
4. **La session s'est-elle emballée ?** Cherchez `refresh rejected` : un jeton de
   rafraîchissement « not found » déconnecte puis reconnecte l'application
   (identifiants enregistrés), ce qui réémet le flux ; la ligne indique désormais
   le `jti`, la date d'émission (`issued`) et l'âge (`age`) du jeton, ce qui
   distingue une copie périmée d'un faux. Cherchez `StartStream` pour voir les
   réémissions ; l'émetteur ne recharge plus le récepteur sur une URL réémise
   pour le même titre.
5. **Le flux était-il trop lourd pour l'appareil ?** Cherchez
   `advertises a fallback rung` (le débit de crête du master en copie) et
   `plan fitted to the client's line` (la mesure `maxKbps` du récepteur ou son
   secours `abr=floor`, devenus une entrée du plan : le master transcodé qui suit
   porte la raison `client_line_exceeded` ou `abr_floor_requested`) : une session
   partie sur un master en copie à plus de 20 Mb/s et retombée sur un barreau
   1080p ajusté au bout de quelques secondes, c'est le secours qui fait son
   travail ; un nouveau LOAD qui repart sur le master en copie est le schéma qui
   tue un Chromecast HD.
6. **Famine côté serveur ?** Cherchez `on-demand resource refused` : une rafale de
   refus `context canceled` sur des segments consécutifs est un balayage de la
   barre de progression (chaque requête annulée par la suivante), pas une faute du
   serveur ; des lignes `served` aux durées longues, si.

Si les étapes 1 à 3 montrent un récepteur vivant et un émetteur muet, le problème
est dans la session Cast du téléphone (le SDK, le réseau vers la télévision) ; si
les balises du récepteur s'arrêtent d'abord, c'est du côté du téléviseur qu'il
faut chercher (mémoire, codec, débit du flux).

## Couper une session ou un appareil compromis

Symptôme : l'administrateur voit un appareil inattendu dans **En cours de
lecture** (panneau d'administration), ou un utilisateur signale un appareil
inconnu dans sa liste de sessions.

1. Identifiez le `session_id` :
   ```sh
   docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
     SELECT id, profile_id, device_name, client_ip, started_at
     FROM play_sessions
     WHERE ended_at IS NULL
     ORDER BY started_at DESC LIMIT 20;"'
   ```
2. Coupez-la depuis l'interface d'administration : **Admin → En cours de
   lecture**, l'icône d'arrêt de la session (**Arrêter la session**), puis
   **Arrêter** pour confirmer ; ou via gRPC
   `AdminSessionService/KillSession` avec le `session_id`. La session prend fin
   5 s plus tard, pour que le lecteur reçoive l'ordre d'arrêt.
3. Si la session ne répond pas, supprimez de force la session HLS (le dossier est
   `<cacheDir>/<mediaItemID>`, par défaut `/cache/transcode/<mediaItemID>`) :
   ```sh
   docker exec evey-server rm -rf /cache/transcode/<media_item_id>/
   ```
4. Si le compte lui-même est compromis, **Admin → Utilisateurs → Désactiver**
   sur ce compte : cela supprime ses jetons de rafraîchissement et invalide
   aussitôt ses jetons d'accès, si bien que chaque appareil est déconnecté.
   Posez un nouveau mot de passe (`evey admin reset-password --user <e-mail>`,
   qui à lui seul ne met fin à aucune session), puis **Activer** de nouveau le
   compte.

## Débloquer un scan de bibliothèque figé

Symptôme : un scan de bibliothèque est « en cours » depuis des heures, sans
progression des fichiers dans les journaux.

1. Vérifiez si le scan journalise encore quelque chose : si le processus est
   bloqué, le processeur est au repos et les journaux récents ne montrent aucune
   ligne `scan:` :
   ```sh
   docker logs --tail=500 evey-server | grep 'scan:'
   ```
2. Le verrou d'un scan est **en mémoire** (aucune ligne en base) : un
   redémarrage du serveur l'efface :
   ```sh
   docker compose restart evey
   ```
3. Si un fichier précis provoque des paniques répétées (voir les traces),
   mettez-le en quarantaine en le sortant du chemin de la bibliothèque, puis
   relancez le scan.
4. Si les surveillances inotify sont épuisées (limite du noyau), augmentez-la :
   ```sh
   echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf
   sudo sysctl -p
   ```

## Aide-mémoire de diagnostic

```sh
# État de tout
docker compose ps
curl -sf http://localhost:9091/healthz && echo healthy

# Suivre tous les conteneurs
docker compose logs --tail=100 -f

# Configuration du serveur
docker exec evey-server env | grep ^EVEY_ | sort

# Taille de la base
docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
  SELECT pg_size_pretty(pg_database_size(current_database()));"'

# Disque de la machine hôte
df -h ./evey-data
```
