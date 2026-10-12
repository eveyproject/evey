---
source_sha: 95bca5df52c2505d699a3857e7d305e19067387a32df91af30da948dfa493ae0
reviewed: true
---
# Réponse aux incidents

Les premiers gestes face aux pannes courantes d'Evey, pour l'opérateur
d'astreinte. Trouvez la section qui correspond au symptôme, lancez ses
vérifications, appliquez la correction qui correspond à ce que vous avez
trouvé. Chaque section a la même forme : **Symptôme**, **Diagnostic**,
**Correction**.

## Avant de commencer

Les commandes visent l'installation standard : un seul conteneur,
`evey-server`, dans lequel tournent Postgres et Meilisearch. Leurs lignes
apparaissent dans le même journal que celles du serveur (préfixe `postgres:`
pour la base). Le `docker-compose.yml` du dépôt ajoute les conteneurs
`evey-postgres` et `evey-caddy` ; voir
[Déploiements avancés](../operations/advanced-deployments.md).

**Ports.** L'API HTTP écoute sur `EVEY_SERVER_PORT` + 1, soit `9091` par
défaut ; `/healthz` y répond. C'est la ligne `ports:` du compose qui le
publie.

**Shell SQL** sur la base embarquée :

```sh
docker exec -it evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

**Journaux.** Le même flux existe en deux exemplaires : le fichier quotidien
`./evey-data/logs/evey-AAAA-MM-JJ.log` (`EVEY_LOG_DIR`, `/data/logs` dans
l'image) et `docker logs evey-server`. Le fichier fait foi : le pilote
json-file de Docker fait tourner `docker logs`, et après une rotation la
commande peut s'arrêter des heures avant le conteneur. Quand les deux
divergent, lisez le fichier. La page **Logs & Debug** du panneau d'admin
cherche dans le même flux (`GET /api/v1/admin/logs/search?q=<terme>&limit=200`) ;
les bearers, JWT et valeurs `token=` / `password=` y apparaissent en
`[redacted]`, donc cherchez un identifiant de média, jamais un jeton.

## Le serveur ne répond pas

**Symptôme** : les clients n'arrivent pas à se connecter ;
`curl http://localhost:9091/healthz` expire ou est refusé.

**Diagnostic** :

```sh
docker ps --filter name=evey-server
docker inspect --format='{{.State.Health.Status}}' evey-server
docker logs --tail=200 evey-server
ss -ltnp | grep 9091                # le port écoute-t-il seulement ?
curl -v http://localhost:9091/healthz
```

**Correction** :

- Conteneur arrêté : le panic est à la fin de `docker logs evey-server`.
  Corrigez la cause, puis `docker compose up -d evey`.
- Healthcheck rouge parce que la base ou le moteur de recherche est arrêté :
  voir [Erreurs de base de données](#database-errors) et
  [La recherche est dégradée](#search-is-degraded).
- Conflit de port sur l'hôte : changez le côté hôte de la ligne `ports:` du
  compose, ou libérez le port.
- Caddy en frontal qui ne route pas (disposition du dépôt) :
  `docker logs evey-caddy`, vérifiez `EVEY_DOMAIN` et que le healthcheck de
  `evey` est vert, puisque Caddy l'attend.

## Toutes les connexions renvoient 401

**Symptôme** : chaque utilisateur, administrateurs compris, reçoit 401 sur
`/api/v1/auth/*`.

**Diagnostic** :

```sh
docker logs --since 10m evey-server | grep -iE 'epoch|jwt|auth|signing|private key'
docker exec evey-server ls -l /data/jwt.key
```

Cherchez d'abord la ligne « epoch ». Chaque requête authentifiée revérifie
l'époque de jeton de l'utilisateur dans la base, et cette vérification
refuse l'accès quand elle échoue : une base lente, verrouillée ou injoignable fait renvoyer à
chaque requête le même 401 qu'un jeton expiré, ce qui ressemble à un problème
de JWT et n'en est pas un. Le serveur l'indique :

```
auth rejected: the token epoch could not be read, so every authenticated request is failing - check the database, not the JWT key
```

Si cette ligne est présente, passez à
[Erreurs de base de données](#database-errors). `/healthz` peut rester vert
pendant ce temps : il teste la connexion, il n'exécute pas la requête
d'époque. Le compteur Prometheus est
`evey_auth_failures_total{reason="epoch_lookup_failed"}`.

**Correction** :

- **Une clé JWT absente ou changée est rarement la cause.** Un fichier de
  clé illisible (`EVEY_JWT_PRIVATE_KEY_PATH`, `/data/jwt.key`) empêche le
  serveur de démarrer ; un fichier absent est régénéré. Les jetons d'accès
  signés par l'ancienne clé sont alors refusés, mais les applications les
  renouvellent avec leur jeton de rafraîchissement, vérifié dans la base et
  non contre la clé : les utilisateurs restent connectés. Vérifiez que le
  volume `./evey-data` est monté sur `/data` ; sans lui la base manque aussi.
- **Le limiteur de débit se déclenche à chaque requête** :
  `EVEY_RATELIMIT_LOGIN_MAX` (`10` par IP et par minute) et
  `EVEY_RATELIMIT_GLOBAL_MAX` (`300`). Un proxy inverse qui transmet tout
  sous une seule adresse le déclenche vite : ajoutez le proxy dans **Proxys
  de confiance** dans **Administration → Accès distant** (appliqué sans redémarrage ;
  `EVEY_TRUSTED_PROXIES` est le repli). Le `docker-compose.override.yml` du
  dépôt monte les deux à `1000` / `10000` pour le développement.
- **Dérive d'horloge** entre le serveur et les clients : `date -u` sur
  l'hôte.

## La lecture transcodée échoue

**Symptôme** : la lecture directe fonctionne, la lecture qui demande un
transcodage échoue ; le lecteur affiche une erreur et le journal du serveur
montre des codes de sortie ffmpeg.

**Diagnostic** :

```sh
docker exec evey-server which ffmpeg ffprobe
docker exec evey-server ffmpeg -version | head -1
docker logs --since 10m evey-server | grep -iE 'ffmpeg|transcode|hls'
docker exec evey-server ls -ld /cache/transcode /data/trickplay
docker exec evey-server df -h /cache
docker exec evey-server touch /cache/transcode/.write-test && \
  docker exec evey-server rm /cache/transcode/.write-test
```

**Correction** :

- **ffmpeg absent** (image personnalisée) : reconstruisez depuis le
  `Dockerfile` officiel.
- **Dossier de cache non inscriptible** : l'image fixe
  `EVEY_TRANSCODING_CACHE_DIR=/cache/transcode` (le volume `evey-cache`) et
  `EVEY_TRICKPLAY_CACHE_DIR=/data/trickplay` ; une entrée `environment:` du
  compose les remplace. Hors de l'image, les valeurs par défaut du code sont
  `/data/transcode` et `/cache/trickplay`.
- **Cache plein** : voir [Le disque de cache est plein](#the-cache-disk-is-full).
- **Accélération matérielle instable** : passez **Accélération
  matérielle** sur **Désactivée (processeur uniquement)** dans **Administration →
  Configuration transcodage → Calibrage de l'encodage** (appliqué à chaud,
  avec recalibrage) pour que chaque transcodage tourne sur le processeur ;
  repassez sur **Auto (meilleur disponible)** une fois le matériel réparé.
- **« The server is busy transcoding other streams »** : la capacité de lecture
  en direct est mesurée, pas réglée ; voir
  [Partager la capacité](../playback/transcoding.md#sharing-the-capacity).
  `EVEY_TRANSCODING_MAX_CONCURRENT` (**Maximum simultané** dans **Administration →
  Cache de transcodage**) ne couvre que les tâches de transcodage hors
  lecture en direct.

### La lecture se met en mémoire tampon ou se coupe, et le journal n'indique pas pourquoi

**Symptôme** : un spectateur signale des blocages sur un titre transcodé ; le
journal montre `hls-vod: segment timeout` avec `err:"context canceled"`, ce
qui signifie que le client a fermé la connexion, pas que le délai du serveur a expiré.

**Diagnostic** : commencez par ce que le serveur a déjà enregistré, cela
ne coûte rien :

```sh
# Pourquoi une requête de segment a réutilisé un encodeur ou en a lancé un,
# et ce que cela a coûté au lecteur. Journalisé seulement quand cela coûte
# quelque chose. `over_client_patience: true` est la preuve flagrante.
docker exec evey-server grep '"software: reach decision"' /data/logs/evey-$(date +%F).log | tail -40

# Les encodeurs eux-mêmes : lancements, arrêts et leur raison.
docker exec evey-server grep -E '"software: session (started|killed)"|per-hash cap' \
  /data/logs/evey-$(date +%F).log | tail -40
```

Ces lignes indiquent ce que le serveur a décidé. Pour voir ce que ffmpeg a
réellement produit seconde par seconde, activez le journal de progression par
encodeur (`$TOKEN` est un JWT d'administrateur) :

```sh
# ON. S'applique au prochain encodeur, sans redémarrage.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"true"}'

# Reproduisez, puis lisez une session : out_time = contenu produit jusqu'ici,
# speed = l'avance de l'encodeur sur le temps réel.
docker exec evey-server ls -t /data/logs/ffmpeg | head
docker exec evey-server tail -20 /data/logs/ffmpeg/<fichier>.log

# OFF une fois terminé.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"false"}'
```

!!! note
    Laissez le journal de progression désactivé hors investigation : un fichier
    par encodeur, quelques Mo par session, pour chaque lecture sur la machine.
    Le nom du fichier porte le hachage de décision, le palier (`-vN-`) et le
    segment de départ (`-sN-`), donc il s'aligne avec le journal JSON ; un
    relancement à la même position a son propre fichier, ce qui rend visibles
    les relances successives des encodeurs.

**Correction** :

- **Sauts avant courts et répétés** : chacun peut lancer un nouvel encodeur,
  et le plafond par hachage (`MaxSessionsPerHash`, 2 par défaut, 1 sous le
  profil basse mémoire) évince le moins récemment utilisé : `session killed
  reason:lru_cap` dans le journal. Normal tant que cela reste occasionnel.
- **Le client est moins patient que le serveur** : la fenêtre d'atteinte
  après un saut est dimensionnée sur `clientSegmentPatience` (6 s) alors que
  le budget d'attente propre du serveur (`WaitBudgetDur`) est de 60 s. Une
  ligne `reach decision` avec `over_client_patience: true` signifie que le
  serveur allait demander une attente qu'aucun lecteur ne supporte.

### Des 410 `stale_rendition`, le lecteur recharge son master en pleine lecture

**Symptôme** : le lecteur se bloque brièvement puis reprend ; le journal
montre `hls-vod: stale rendition URL refused (the ladder moved under a live
player)` et des requêtes répondues `410 Gone` avec
`X-Evey-Error: stale_rendition`.

C'est une protection qui se déclenche, pas la panne elle-même. Chaque URI de
manifeste porte le hachage de décision sous lequel elle a été construite
(`fp=`). Un 410 signifie qu'un lecteur a présenté une URL d'une session dont
l'échelle ABR a été modifiée depuis ; la servir donnerait au décodeur des
octets d'un autre rendu. Le refus force un rechargement propre du master.

**Diagnostic** : le WARN porte `sealed_fp`, `current_fp` et l'identifiant du
média.

- Une **rafale juste après un déploiement, un redémarrage ou un recalibrage**
  est attendue : des sessions étaient en cours pendant un changement de
  capacité. Les lecteurs se resynchronisent en un cycle.
- Des **410 peu nombreux mais réguliers en fonctionnement normal** signifient
  que quelque chose modifie sans cesse les échelles de flux en cours :
  cherchez une capacité qui oscille (`docker logs evey-server | grep -iE
  'calibrat|capability'`), un régulateur d'admission qui change souvent d'état sous charge,
  ou un plafond CMCD qui oscille.
- Des lecteurs qui **reçoivent des 410 en boucle** gardent un master en cache (une
  vieille version d'application, un proxy qui met les `.m3u8` en cache). Le
  proxy doit laisser passer `Cache-Control: no-cache`.

## Erreurs de base de données

**Symptôme** : le journal contient `dial tcp ... connect: connection
refused`, `pq: ...`, ou des requêtes expirent.

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

**Correction** :

- **Disque plein** sous `./evey-data` (le cluster se trouve dans
  `./evey-data/postgres`) : libérez de l'espace, puis
  `docker compose restart evey`.
- **Pool épuisé** (`too many connections`, longues attentes) :
  `EVEY_DB_MAX_OPEN_CONNS` (`25` par défaut). Terminez les sessions bloquées :
  ```sql
  SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
   WHERE state = 'idle in transaction'
     AND now() - state_change > interval '5 minutes';
  ```
- **Conteneur Postgres séparé** (disposition du dépôt) : `docker ps --filter
  name=evey-postgres`, `docker logs --tail=200 evey-postgres` ; les
  `EVEY_DB_USER` / `EVEY_DB_PASSWORD` / `EVEY_DB_NAME` du serveur doivent
  correspondre au service `postgres`.

## La recherche est dégradée

**Symptôme** : la recherche renvoie des résultats moins bons ; le journal
montre `meilisearch unavailable, using SQL search fallback`.

La recherche continue de fonctionner par SQL tant que Meilisearch est
injoignable ; seule la qualité du classement baisse, personne n'est hors
ligne. Meilisearch tourne dans `evey-server` (`EVEY_EMBEDDED_MEILI`, actif
par défaut) sur la boucle locale du conteneur, port 7700.

**Diagnostic** :

```sh
docker exec evey-server curl -sf http://127.0.0.1:7700/health
docker logs --tail=300 evey-server | grep -i meili
```

**Correction** :

- Healthcheck en échec : `docker compose restart evey`.
- Index corrompu : arrêtez le conteneur, mettez `./evey-data/meili` de côté,
  redémarrez, puis reconstruisez l'index depuis la base :
  ```sh
  docker exec evey-server evey admin reindex-search
  ```
  La reconstruction est une commande, pas une action du panneau.

## Le disque de cache est plein

**Symptôme** : les transcodages échouent avec `no space left on device` ; le
healthcheck oscille.

**Diagnostic** :

```sh
docker exec evey-server df -h /cache
docker exec evey-server du -sh /cache/transcode /data/trickplay 2>/dev/null
docker volume inspect evey_evey-cache   # le préfixe du compose peut différer
```

**Correction** :

- Purgez le cache de transcodage, il se régénère :
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/transcode/*'
  ```
  Cela supprime aussi `photos/` (variantes redimensionnées des photos) et
  `track-art/` (pochettes extraites des tags audio) qu'il contient ; les deux
  sont reconstruits à la prochaine requête. **Tout vider** dans **Administration →
  Cache de transcodage** vide le même cache depuis le panneau.
- Le trickplay coûte plus cher à régénérer ; ne le purgez que si nécessaire :
  ```sh
  docker exec evey-server sh -c 'rm -rf /data/trickplay/*'
  ```
- Baissez le **Budget du cache de transcodage (Go)** dans **Administration →
  Configuration du serveur → Paramètres du serveur** (`10` par défaut) si
  le volume de l'hôte manque de place. Quel que soit le budget, le balayeur garde
  2 Gio libres sur le volume de cache.

## Un plugin a planté

**Symptôme** : un plugin ne répond plus ; le journal montre des panics du
lanceur de plugins.

**Diagnostic** :

```sh
docker logs --since 30m evey-server 2>&1 | grep -iE 'plugin|pluginrunner|panic'
# cherchez : "panic in plugin runner", "plugin process crashed", "remove plugin sock on crash"
```

**Correction** :

1. Relancez le plugin sans toucher au serveur : **Activer** à nouveau dans
   **Administration → Plugins**, ou `POST /api/v1/admin/plugins/<id>/enable`. Cela
   fonctionne quand le plugin est activé mais bloqué en `Error` : le démarrage ne
   refuse qu'un plugin déjà `Running` ou `Initializing`.
2. Si cela ne suffit pas, **Désactiver** le plugin depuis le panneau.
3. Si le serveur lui-même est instable : `docker compose restart evey`.
4. Les fichiers des plugins se trouvent dans `./evey-data/plugins/` sur l'hôte
   (`/data/plugins` dans le conteneur). Supprimez le dossier du plugin défaillant,
   redémarrez.
5. Envoyez la trace du panic et l'identifiant du plugin à son auteur.

## Une diffusion s'est figée, est devenue muette, ou le téléphone reste en chargement

Tout ce qui suit se lit depuis **Administration → Logs & Debug** ; aucun shell sur la
machine n'est nécessaire. Les heures sont en UTC. Chaque balise est une ligne
`player-client: playback event reported` avec `type=` et une chaîne
`details=`.

1. **Le récepteur était-il actif, et jusqu'à quand ?** Cherchez
   `cast_receiver_alive`. Le récepteur en envoie une toutes les 30 s pendant
   la lecture ; `details` porte `buf=<arrière>/<avant>/<plages>` (secondes en
   tampon autour de la tête de lecture) et `pos_sec`. Une lecture saine
   montre `ahead` à plusieurs dizaines de secondes. L'heure de la dernière
   balise est le moment où le récepteur s'est arrêté ou a été arrêté de force : rien après
   signifie que le téléviseur n'exécute plus le récepteur, quoi qu'affiche le
   téléphone.
2. **Pourquoi s'est-il arrêté ?** Cherchez `cast_receiver_finished` :
   `reason=INTERRUPTED` est un nouveau LOAD qui remplace celui en cours
   (cherchez un `StartStream` quelques secondes avant), `reason=ERROR` porte
   l'erreur du lecteur. Aucun `finished` après le dernier `alive` signifie
   que le processus du récepteur a disparu, le plus souvent un Chromecast HD
   arrêté faute de mémoire sur un master copie à 20 Mbps ou plus.
3. **Qu'a vu le téléphone ?** Cherchez `cast_sender_stall` : l'émetteur
   envoie `state=playing frozen=<n>s pos=<p>` quand le SDK Cast indique encore
   PLAYING mais que la position n'a pas bougé depuis 20 s, et `state=idle
   reason=<r>` quand le récepteur a signalé une raison d'inactivité. Ce sont
   des lignes WARN, que le filtre d'avertissements de la page fait remonter.
   Comparez son `pos` au dernier `pos_sec` du récepteur.
4. **La session a-t-elle provoqué des reconnexions en série ?** Cherchez `refresh rejected` : un jeton
   de rafraîchissement `not found` déconnecte puis reconnecte l'application
   (identifiants enregistrés), ce qui réémet le flux ; la ligne porte le
   `jti`, `issued` et `age` du jeton, ce qui distingue une copie périmée d'un
   faux. Cherchez `StartStream` pour voir les réémissions.
5. **Le flux était-il trop gros pour l'appareil ?** Cherchez `advertises a
   fallback rung` (le débit de pointe du master copie) et `plan fitted to the
   client's line` (la mesure `maxKbps` du récepteur ou son repli
   `abr=floor` ; le master transcodé qui suit porte la raison
   `client_line_exceeded` ou `abr_floor_requested`). Une session partie sur un
   master copie à 20 Mbps ou plus et retombée sur un palier 1080p ajusté
   après quelques secondes est le repli qui fonctionne comme prévu ; un re-LOAD
   qui repart sur le master copie est le scénario qui fait planter un Chromecast HD.
6. **Manque de ressources côté serveur ?** Cherchez `on-demand resource refused` : une
   rafale de refus `context canceled` sur des segments consécutifs est un
   défilement (chaque requête annulée par la suivante), pas une erreur du
   serveur ; des lignes `served` aux longues durées en sont une.

Si les étapes 1 à 3 montrent le récepteur actif et l'émetteur silencieux, le
problème est dans la session Cast du téléphone (le SDK, le réseau vers le
téléviseur). Si les balises du récepteur s'arrêtent en premier, regardez le
téléviseur : mémoire, codec, débit du flux.

## Arrêter une session ou un appareil compromis

**Symptôme** : un appareil inattendu dans **Administration → En cours de lecture**,
ou un utilisateur signale un appareil inconnu dans sa liste de sessions.

1. Identifiez la session :
   ```sh
   docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
     SELECT id, profile_id, device_name, client_ip, started_at
       FROM play_sessions
      WHERE ended_at IS NULL
      ORDER BY started_at DESC LIMIT 20;"'
   ```
2. Arrêtez-la depuis **Administration → En cours de lecture** : l'icône d'arrêt sur
   la session (**Arrêter la session**), puis **Arrêter** pour confirmer ; ou
   `AdminSessionService/KillSession` en gRPC avec l'identifiant de session.
   Le lecteur reçoit d'abord une commande d'arrêt et le flux est terminé
   5 secondes plus tard. Cela ne termine que la lecture ; l'utilisateur reste
   connecté.
3. Si le flux continue malgré tout, redémarrez le conteneur : chaque encodeur
   s'arrête avec lui.
   ```sh
   docker compose restart evey
   ```
4. Si le compte lui-même est compromis, **Administration → Utilisateurs →
   Désactiver** : les jetons de rafraîchissement sont supprimés et les jetons
   d'accès révoqués d'un coup, donc chaque appareil est déconnecté. Donnez un
   nouveau mot de passe (`evey admin reset-password --user <email>`, voir
   [Récupération](recovery.md#give-a-user-a-new-password)), puis **Activer**
   le compte à nouveau.

## Un scan de bibliothèque est bloqué

**Symptôme** : un scan s'affiche en cours depuis des heures sans progression
de fichier dans le journal.

1. Vérifiez si le scan journalise encore quelque chose ; un scan figé montre
   un processeur au repos et aucune ligne `scan:` récente :
   ```sh
   docker logs --tail=500 evey-server | grep 'scan:'
   ```
2. Le verrou de scan est en mémoire, donc un redémarrage l'efface :
   ```sh
   docker compose restart evey
   ```
3. Si un fichier provoque des panics répétés (les traces le nomment),
   sortez-le du chemin de la bibliothèque, puis relancez le scan.
4. Si le surveillant de fichiers journalise `watcher: inotify failed for
   path, will use periodic scan`, la limite de surveillances du noyau est
   épuisée :
   ```sh
   echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf
   sudo sysctl -p
   ```

## Aide-mémoire

```sh
# État de tout
docker compose ps
curl -sf http://localhost:9091/healthz && echo healthy

# Suivre tous les conteneurs
docker compose logs --tail=100 -f

# Configuration effective
docker exec evey-server env | grep ^EVEY_ | sort

# Taille de la base
docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
  SELECT pg_size_pretty(pg_database_size(current_database()));"'

# Disque de l'hôte
df -h ./evey-data
```

<!-- sources
ports: internal/config/config.go (EVEY_SERVER_PORT, EVEY_GRPC_BIND) ; cmd/evey/main.go - httpPort = Port+1 ; Dockerfile - HEALTHCHECK
logs: Dockerfile ENV EVEY_LOG_DIR ; internal/adapters/driving/http/admin_handler.go (GET /api/v1/admin/logs/search)
epoch fail-closed: internal/adapters/driving/http/auth_middleware.go, internal/adapters/driving/grpcapi/interceptors/auth.go ; internal/pkg/metrics/metrics.go - AuthFailuresTotal
rate limits: internal/config/config.go (EVEY_RATELIMIT_LOGIN_MAX, EVEY_RATELIMIT_GLOBAL_MAX, EVEY_TRUSTED_PROXIES) ; docker-compose.override.yml
cache dirs: Dockerfile ENV EVEY_TRANSCODING_CACHE_DIR / EVEY_TRICKPLAY_CACHE_DIR ; docker-compose.yml ; internal/config/config.go defaults
hardware mode and calibration labels: i18n/fr/admin.yaml - transcodeCalibration.hwModeLabel, hwModeOff, hwModeAuto, cardTitle
software producer: internal/app/transcoding/software_producer.go - clientSegmentPatience, WaitBudgetDur, MaxSessionsPerHash, "software: reach decision", over_client_patience ; internal/app/transcoding/software_producer_lifecycle.go - "software: session killed" ; cmd/evey/wire.go - low-memory profile, transcode.ffmpeg_progress_log, logs/ffmpeg
server-config route: proto/evey/v1/server_config.proto - SetServerConfigEntry (PUT /api/v1/admin/server-config/{key})
stale rendition: internal/adapters/driving/http/transcode_handler_vod.go - stale_rendition, sealed_fp
db pool: internal/config/config.go (EVEY_DB_MAX_OPEN_CONNS)
search fallback: cmd/evey/wire_infra.go ; cmd/evey/admin.go - adminReindexSearch ; docker/entrypoint.sh (EVEY_EMBEDDED_MEILI, MEILI_DATA_DIR)
cache budget: internal/config/config.go (EVEY_TRANSCODING_CACHE_MAX_GB) ; internal/app/transcoding/segment_cache_evict.go - DefaultMinFreeBytes ; internal/adapters/driving/http/server.go - track-art under PhotoCacheDir ; cmd/evey/http.go - PhotoCacheDir
plugins: internal/adapters/driven/pluginrunner/runner_start.go (panic/crash lines) ; internal/app/plugin/runtime_service.go - StartPlugin ; proto/evey/v1/admin_plugin.proto - EnablePlugin
cast beacons: internal/adapters/driving/http/player_event_handler.go ; internal/app/user/auth.go - "refresh rejected" ; internal/adapters/driving/http/transcode_ondemand.go ; internal/app/transcoding/resolver_line.go
sessions: internal/adapters/driven/postgres/migrations/000001_init.up.sql - play_sessions ; internal/adapters/driving/grpcapi/admin_audit.go - KillSession (5 s) ; internal/app/user/user_service.go - DisableUser
scan: internal/app/library/scan_service.go - scanning map ; internal/app/library/watcher.go - inotify warning
labels: i18n/fr/admin.yaml - sidebar (logs, nowPlaying, remoteAccess, serverConfig, transcodeCache, users, plugins), nowPlaying.stopSession, transcodeCache.clearAll, transcodeCache.maxConcurrent, remoteAccess.trustedProxies, serverSettings.transcodeCacheMaxGB ; i18n/fr/common.yaml - enable, disable
-->
