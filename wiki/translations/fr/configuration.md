---
source_sha: a7cdb2195f99559246f23706d11f64ad40c0cb3dcd88a52f3caf61af02c7ce8c
---

# Configuration

Deux endroits, une règle. **Ce qui doit exister avant le démarrage du
serveur** - secrets, câblage du conteneur, informations sur le reverse proxy - vit dans le
fichier compose. **Ce qu'un administrateur règle ensuite** - niveau de
journalisation, langue, limites de transcodage, accélération matérielle,
surveillance des dossiers, bac à sable et quotas des extensions - vit dans
**Admin > Server config**, est rangé dans la base, survit à une recréation du
conteneur et s'applique le plus souvent sans redémarrage.

Poser l'un de ces réglages dans l'environnement ne fait qu'amorcer le premier
démarrage. C'est pourquoi cette page ne liste pas toutes les variables `EVEY_*`
que le binaire accepte : les autres sont des amorces de premier démarrage et des
soupapes. `internal/config/config.go` est la référence exhaustive si vous en
avez besoin.

## 1. Ce que vous devez renseigner vous-même

Rien, pour l'installation par défaut : le chemin des médias et le tag de l'image
figurent dans le fichier compose lui-même, et il n'y a pas de `.env`. Le
`docker-compose.yml` du dépôt, lui, lit `MEDIA_DIR` depuis un `.env` :

```env
MEDIA_DIR=/path/to/your/media
```

Dans les deux cas, Postgres et Meilisearch tournent en embarqué dans le
conteneur et génèrent leurs propres secrets au premier démarrage, rangés sous
`./evey-data/keys` (`postgres.password`, `meili.key`, en 0600). Le serveur les
lit par `EVEY_DB_PASSWORD_FILE` et `MEILI_MASTER_KEY_FILE`, inscrits dans
l'image, ce qui explique aussi que `docker exec evey-server evey admin ...` ne
réclame aucun mot de passe en ligne de commande. Ne renseignez explicitement
`EVEY_DB_PASSWORD` et `MEILI_MASTER_KEY` que si vous apportez votre propre base
de données ou votre propre moteur de recherche : une valeur explicite l'emporte
toujours sur le fichier.

Les métadonnées fonctionnent dès l'installation : une clé TMDB est intégrée.
Une clé à vous (gratuite sur themoviedb.org) vous donne votre propre quota ;
saisissez-la dans **Admin > Configuration du serveur > Clés API**, ou amorcez-la
avec `EVEY_TMDB_API_KEY`. Une fois le serveur publié, `EVEY_SERVER_BASE_URL` est
la seule variable à ajouter - section 2.

## 2. Une instance publiée, en deux variables

Derrière un reverse proxy, avec le transcodage matériel, tout l'environnement
tient en deux variables. Le reste de ce que l'on traîne d'ordinaire dans un
fichier compose est soit déjà dans l'image, soit une valeur par défaut :

```yaml
services:
  evey:
    image: gravityz/evey:latest
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 10.0.0.5/32      # votre reverse proxy, en /32 si vous connaissez son adresse
    volumes:
      - ./evey-data:/data
      - evey-cache:/cache
      - /srv/media:/media
    devices:
      - /dev/dri:/dev/dri                     # transcodage matériel
    group_add:
      - "991"                                 # le groupe propriétaire de /dev/dri/renderD128 sur VOTRE hôte
```

Des lignes qu'il vaut mieux **ne pas** recopier depuis d'anciennes
installations :

| Ligne | Pourquoi elle est inutile |
| --- | --- |
| `EVEY_LOG_DIR`, `EVEY_TRANSCODING_CACHE_DIR`, `EVEY_TRICKPLAY_CACHE_DIR` | L'image les fixe déjà à `/data/logs`, `/cache/transcode` et `/data/trickplay`. Les répéter ne change rien. |
| `EVEY_FORWARDED_PROTO` | Dépréciée : elle se déduit du schéma d'`EVEY_SERVER_BASE_URL`. La renseigner ne fait que produire un avertissement de dépréciation dans le journal. |
| `EVEY_KEYFRAME_BACKFILL=on` | Toute valeur autre que `off` l'active, son absence comprise. |
| `LIBVA_DRIVER_NAME` | libva choisit le pilote d'après l'identifiant PCI du GPU. Ne le forcez que si votre carte réclame un pilote que libva ne retient pas (à vérifier avec `docker exec evey-server vainfo`). |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` retient le confinement le plus solide que votre hôte prend en charge. Ne la fixez explicitement que si vous voulez une garantie *fermée par défaut* : `landlock` refuse d'exécuter une extension qu'il ne peut pas confiner, là où `auto` se dégrade avec un avertissement. |
| un `group_add` pour `/dev/dri/card0` | Evey n'ouvre jamais que `renderD128`. Vérifiez avec `ls -ln /dev/dri` et n'ajoutez que ce groupe-là. |

## 3. Les deux réglages que le panneau d'administration ne remplace pas

Tous deux ont un sosie dans **Admin → Remote Access**, et aucun des deux ne
rend la variable d'environnement superflue. C'est un piège classique, autant
être explicite.

Ce qui les distingue, c'est l'endroit où chacun réside. Le bloc d'accès distant
(activation, URL externe, plages du réseau local, plafond de débit à distance)
est **persisté en base de données** : ses variables d'environnement amorcent le
premier démarrage et sont ignorées ensuite. `EVEY_SERVER_BASE_URL` et
`EVEY_TRUSTED_PROXIES` font exactement l'inverse : elles sont lues dans
l'environnement à chaque démarrage et **rien dans le panneau ne les écrase
jamais**. Une valeur qui ressemble à un doublon dans le panneau est donc, dans
ces deux cas précis, un autre réglage qui se trouve mentionner une adresse.

**« External URL » ne remplace pas `EVEY_SERVER_BASE_URL`.** La valeur du
panneau alimente les URL que le serveur *fabrique*. Mais savoir s'il fait
confiance à `X-Forwarded-Proto`, et donc s'il se croit servi en HTTPS, se
tranche **au démarrage, à partir de la seule variable d'environnement**. Sans
elle, il retombe sur `http://localhost:9091` : cookies `Secure`, redirections et
tous les liens absolus se dégradent en `http://`, et les URL de partage et de
téléchargement pointent vers localhost. La valeur du panneau est lue trop tard
pour y changer quoi que ce soit.

**« LAN networks » ne remplace pas `EVEY_TRUSTED_PROXIES`.** Ce sont deux
étapes, pas deux noms pour une même chose :

1. `EVEY_TRUSTED_PROXIES` décide **de qui l'on croit le `X-Forwarded-For`**, et
   donc quelle adresse est attribuée à un client.
2. Les LAN networks classent ensuite **cette** adresse comme locale ou distante.

La seconde travaille sur le résultat de la première. Sans la première, tous les
clients arrivent avec l'adresse du proxy et se classent à l'identique : c'est
exactement ainsi que le verrou d'accès distant est resté en sommeil pendant
quatre mois (voir `internal/config/remote_access_resolve.go`). La boucle locale
est toujours de confiance et ne peut pas être retirée ; votre entrée s'y
*ajoute*.

## 4. Propriété des fichiers (PUID/PGID)

**Par défaut, c'est automatique.** Quand `PUID` et `PGID` ne sont pas définis, le
point d'entrée regarde au démarrage à qui appartiennent vos montages `/media*`
et fait tourner le serveur sous cet utilisateur : les fonctions qui *écrivent
dans* votre bibliothèque (la réparation de l'index de navigation reconstruit les
fichiers sur place) marchent alors d'emblée, sans `chown` ni configuration. Le
journal de démarrage indique ce qui a été retenu :

```
evey: media mounts owned by 1026:100 (detected on /media) - running the
server as that user so write features (seek-index repair) work out of the box
```

Des montages appartenant à **des utilisateurs différents** sont traités au
mieux : le serveur adopte l'utilisateur qui possède le plus de montages et
rejoint les groupes des autres montages comme groupes secondaires, de sorte que
les réparations y fonctionnent quand les fichiers sont accessibles en écriture
au groupe (l'avertissement de démarrage énumère les montages qui pourraient
encore refuser l'écriture). La détection automatique ne s'efface complètement,
en se repliant sur `1000:1000` avec un avertissement, que lorsque tous les
montages de médias appartiennent à root ou à l'uid réservé `1001` : donnez alors
vos médias à un utilisateur ordinaire avec `chown`, ou fixez `PUID` et `PGID`
explicitement :

```env
PUID=1026   # faire tourner le serveur sous cet id d'utilisateur (hôte : `id -u <media-owner>`)
PGID=100    # ...et sous cet id de groupe
```

Les valeurs explicites l'emportent toujours sur la détection.

Comment cela fonctionne : le conteneur démarre en root juste le temps de
remapper son utilisateur interne (et de reprendre la propriété des répertoires
d'état propres à Evey, `/data` et `/cache`, **jamais** vos montages de médias),
puis abandonne ses privilèges. Les groupes secondaires accordés par le
`group_add:` du compose (par exemple le groupe render de votre hôte, pour le
transcodage matériel par `/dev/dri`) sont conservés au passage. Le processus
serveur lui-même ne tourne jamais en root.

Variante sans le moindre root : si même ce root passager vous paraît
inacceptable, mettez plutôt `user: "UID:GID"` sur le service dans votre fichier
compose. Le point d'entrée détecte le démarrage non root et saute le remappage,
détection du propriétaire des médias comprise ; il vous revient alors de garder
`./evey-data` accessible en écriture à cet UID.

À noter :
- `PUID=1001` est refusé : c'est l'utilisateur interne `evey-plugin`, tenu à
  l'écart pour qu'une extension ne partage jamais l'identité du serveur.
- Les médias sont montés en lecture-écriture par défaut, afin que les
  réparations fonctionnent nativement ; ajoutez `:ro` au volume des médias si
  vous préférez une bibliothèque en lecture seule, étant entendu que les
  réparations échoueront alors avec une erreur explicite.

## 5. Référence des variables

### Serveur
```env
EVEY_SERVER_PORT=9090                  # port d'écoute gRPC (9090 par défaut) ; l'API HTTP écoute sur SERVER_PORT+1 = 9091
EVEY_HTTP_PORT=9091                    # compose uniquement : port de l'hôte exposé par Docker pour la passerelle HTTP (le binaire du serveur ne le lit pas)
EVEY_SERVER_NAME=Evey                  # nom affiché
EVEY_DEMO=false                        # instance de démo publique uniquement (demo.eveyproject.org) : verrouille les écritures admin, ni mail ni push ni webhooks
EVEY_VERSION=latest                    # tag de l'image Docker (latest, v0.1.0-beta.1, etc.)
```

### Base de données
```env
# Embarquée (par défaut quand EVEY_DB_HOST n'est pas défini) : Postgres 17 dans le conteneur.
EVEY_EMBEDDED_POSTGRES=            # true | false ; non défini = embarquée si et seulement si EVEY_DB_HOST ne l'est pas
EVEY_POSTGRES_DATA_DIR=/data/postgres   # emplacement du cluster dans le conteneur (sous ./evey-data)
EVEY_DB_USER=evey
EVEY_DB_NAME=evey
EVEY_DB_PASSWORD=                  # non défini = généré au premier démarrage dans /data/keys/postgres.password
EVEY_DB_PASSWORD_FILE=/data/keys/postgres.password   # lu quand EVEY_DB_PASSWORD n'est pas défini (valeur par défaut de l'image)

# Externe : utilisez ces variables À LA PLACE du bloc ci-dessus, pas en plus.
# Définir l'hôte éteint la base embarquée.
EVEY_DB_HOST=postgres              # dans le compose du dépôt, le service voisin s'appelle `postgres`
EVEY_DB_PORT=5432
EVEY_DB_PASSWORD=changeme
EVEY_DB_SSLMODE=prefer             # ou disable | require | verify-full
```

Le Postgres embarqué n'écoute que sur la boucle locale du conteneur et exige un
mot de passe sur TCP ; celui qui a été généré est celui qu'utilisent le serveur
et `evey admin`. Son répertoire de données doit reposer sur un vrai système de
fichiers (ext4, btrfs, xfs, ...), pas sur un partage réseau. Les migrations
s'exécutent automatiquement au démarrage dans les deux dispositions.

### Authentification
```env
EVEY_JWT_ACCESS_TTL=15m
EVEY_JWT_REFRESH_TTL=168h              # 7 jours
EVEY_RATELIMIT_LOGIN_MAX=10            # par IP et par minute
```

### Bibliothèques
```env
EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL=2m # intervalle de scrutation de repli quand inotify rate des événements (2m par défaut) ; modifiable depuis Admin → Server Config
EVEY_LIBRARY_WATCHER_ENABLED=true      # surveillance des fichiers en temps réel
```

La sonde d'analyse des bibliothèques lit les métadonnées `.mkv`/`.webm` et
`.mp4`/`.m4v` dans le processus lui-même (mkvgo, en-tête seul, environ 35 fois
plus rapide lors d'une réanalyse, d'une réparation ou sur un NAS), sans aucune
configuration : elle est toujours active. ffprobe reste l'autorité et prend en
charge tout fichier que le chemin rapide ne sait pas transposer champ pour champ
(HDR et Dolby Vision, codecs non pris en charge, …). Voir
[adr/0016-mkv-fastpath-prober.md](adr/0016-mkv-fastpath-prober.md).

### Transcodage
```env
EVEY_TRANSCODING_MAX_CONCURRENT=2
EVEY_TRANSCODING_HW_ACCEL_ENABLED=false
```

Pour la mise en place matérielle, voir [transcoding.md](transcoding.md).

**Le son Dolby sur un Chromecast** (`EVEY_CAST_WASM_AUDIO_ENABLED`, `false` par
défaut). Evey sait transmettre à un récepteur de diffusion la piste AC-3/E-AC-3
d'origine et le laisser la décoder en WebAssembly sur le téléviseur, ce qui
évite un transcodage. C'est **désactivé pour les récepteurs de diffusion** parce
qu'une box d'opérateur d'entrée de gamme s'est fait tuer en plein film, deux
fois, avec ce chemin en service ; son décodeur y a d'ailleurs été mesuré à
3,13 fois le temps réel, si bien qu'il n'y gagnait rien. Désactivé, une
diffusion reçoit le master multiplexé ordinaire avec une déclinaison AAC : du
son dès la première seconde, sur tous les appareils. Les navigateurs ne sont PAS
concernés, ils conservent le décodeur wasm, qui y est validé. Passez la variable
à `true` pour la réactiver sur les diffusions.

**Préférez le panneau d'administration.** La concurrence, l'accélération
matérielle, l'AV1, le budget de cache et les limites avancées (nombre maximal de
sessions HLS simultanées, plafonds de hauteur pour le logiciel et le logiciel
HDR, encodage d'un palier à la fois, un encodeur matériel par palier) se
trouvent tous sous **Admin > Transcoding configuration**. Les valeurs
enregistrées là survivent aux redémarrages et priment sur l'environnement ; le
panneau signale celles qui ne prennent effet qu'après un redémarrage.

**Les drapeaux des tâches de fond** se posent sous forme de lignes dans
**Admin > Server configuration** : le panneau propose les clés et affiche, pour
chacune, la variable d'environnement correspondante et la valeur attendue.
Toutes sont lues au démarrage, elles ne s'appliquent donc qu'au redémarrage
suivant :

| Clé | Valeur |
| --- | --- |
| `transcoding.keyframe_backfill` | `off` pour désactiver le remplissage rétroactif de l'index des images clés (actif par défaut) |
| `transcoding.keyframe_backfill_batch` | éléments par passe, > 0 |
| `transcoding.keyframe_backfill_pause_ms` | millisecondes entre deux éléments, >= 0 |
| `transcoding.keyframe_backfill_idle_sec` | secondes entre deux passes à vide, > 0 |
| `transcoding.level_backfill` | `off` pour désactiver le remplissage rétroactif des niveaux H.264/HEVC (actif par défaut) |
| `transcoding.level_backfill_batch` | éléments par passe, > 0 |
| `transcoding.level_backfill_pause_ms` | millisecondes entre deux éléments, >= 0 |
| `transcoding.level_backfill_idle_sec` | secondes entre deux passes à vide, > 0 |
| `transcoding.pause_bg_on_playback` | `off` pour laisser le travail de fond tourner pendant la lecture (actif par défaut : il s'efface) |
| `transcoding.snapshot_fastpath` | `off` pour forcer l'extracteur ffmpeg exact au lieu du chemin rapide des images clés de mkvgo |

Chacune a aussi son équivalent en variable d'environnement
(`EVEY_KEYFRAME_BACKFILL`, `EVEY_LEVEL_BACKFILL_BATCH`, ...), mais c'est la
ligne du panneau qui l'emporte.

Le **budget CPU de fond** (Admin > Configuration du serveur > Puissance allouée)
borne le processeur que toutes les tâches de fond peuvent prendre ensemble, à
chaud (sans redémarrage) :

| Clé | Valeur |
| --- | --- |
| `background.cpu_percent` | part de la capacité CPU où puisent toutes les tâches de fond : `25` (par défaut, en douceur) ou `50` (coup de fouet) |
| `background.cpu_percent.<task>` | la part propre d'une tâche lourde, de `1` à `80`, qui remplace la part globale pour cette tâche : `library_scan`, `trickplay_generation`, `intro_detection`, `complexity_classifier` |

Les parts ne s'additionnent jamais : la somme de tout ce qui tourne en fond reste
sous la plus haute part fixée. Une part est convertie en cœurs entiers (25 % de
8 cœurs = 2), un au minimum. Chaque ffmpeg de fond réserve ses threads dans ce
budget avant de démarrer ; le scan des bibliothèques et la détection des
génériques y dimensionnent leur parallélisme, la génération des miniatures
trickplay ses threads de décodage. La lecture met toujours en pause toute
analyse de fond (`transcoding.pause_bg_on_playback`). `EVEY_LIBRARY_SCAN_WORKERS`
et `EVEY_INTRO_DETECTION_MAX_PARALLEL` dimensionnent ces deux réserves à la main
et contournent le budget pour elles. Les anciennes clés `trickplay.adaptive_cpu`
(`false` = 50 %) et `intro_detection.use_all_cores` (`true` = 80 % pour la
détection des génériques) sont toujours honorées tant que les nouvelles ne sont
pas fixées.

Le **budget du cache de transcodage** (Admin > Paramètres du serveur,
`transcoding.cache_max_gb`, `10` par défaut, appliqué au redémarrage) borne tout
le cache de transcodage sur le disque : segments de lecture, remux de démarrage
rapide et téléchargements compatibles, ensemble, sous un seul nombre. C'est une
taille absolue à dessein : le cache vit sur le volume `evey-cache` à côté de ce
que l'opérateur y fait tourner d'autre, et l'usage du reste du volume lui
appartient. Un seul balayeur l'applique, du moins récemment utilisé au plus
récent, en n'épargnant que ce qu'un spectateur est en train de lire. Quel que
soit le budget, il garde aussi 2 Gio libres sur le volume du cache, et retire un
téléchargement que personne n'a récupéré depuis 7 jours. Une session de copie
(remux) ne stocke jamais le film entier : elle garde une fenêtre de segments
autour du spectateur et relit la source pour tout ce qui en sort. Le chemin
mkvgo sans stockage, qui sert la plupart des sources MKV et MP4, ne touche
jamais au cache.

### Extensions (bac à sable et limites)
```env
EVEY_PLUGINS_SANDBOX_MODE=auto         # auto|landlock|bubblewrap|strict|basic - auto retient le plus solide que votre hôte prend en charge
EVEY_PLUGINS_UID_SEPARATION=auto       # auto|on|off - exécuter les extensions sous un autre utilisateur pour cloisonner les secrets (auto = actif seulement si l'isolation effective vaut "basic", donc ni Landlock ni bubblewrap) ; incompatible avec no-new-privileges
EVEY_PLUGINS_MEMORY_MB=256             # RAM maximale par extension (Mio) ; modifiable depuis Admin → Plugins
EVEY_PLUGINS_PIDS_MAX=64               # nombre maximal de processus et de fils par extension ; modifiable dans le panneau
EVEY_PLUGINS_DISK_MAX_MB=100           # taille maximale du répertoire d'installation par extension (Mio ; 0 = désactivé) ; modifiable dans le panneau
EVEY_PLUGINS_CPU_NICE=10               # nice d'ordonnancement 0-19 (plus haut = moins prioritaire) ; modifiable dans le panneau
EVEY_PLUGINS_ENABLED=true              # interrupteur général du sous-système d'extensions
EVEY_PLUGINS_COSIGN_POLICY=warn        # off|warn|enforce - exiger une signature cosign sur les ZIP d'extensions
EVEY_PLUGINS_COSIGN_PUBLIC_KEY=        # chemin de la clé publique PEM (requis dès que la politique n'est pas off)
```

Guide scénario par scénario : [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md). Modèle complet : [SECURITY.md](SECURITY.md).

### Courriel (SMTP)
```env
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=
EVEY_SMTP_PASSWORD=
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls                 # starttls, ssl, none
```

Voir [notifications.md](notifications.md).

### SSO (OIDC)
```env
EVEY_OIDC_ENABLED=false
EVEY_OIDC_ISSUER=https://auth.example.com
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=       # séparés par des virgules, vide = tous
```

Voir [oidc.md](oidc.md).

### Sauvegardes
```env
EVEY_BACKUP_INTERVAL_HOURS=0           # 0 = désactivé, 24 = une fois par jour
EVEY_BACKUP_RETENTION_DAYS=30
```

Voir [backups.md](backups.md).

### Reverse proxy
```env
EVEY_TRUSTED_PROXIES=172.16.0.0/12     # s'ajoute à la boucle locale, toujours de confiance ; ne la remplace pas
EVEY_REAL_IP_HEADER=X-Forwarded-For
EVEY_DOMAIN=media.example.com          # uniquement pour le Caddy fourni
```

Le comportement HTTPS (cookies `Secure`, HSTS, confiance accordée à
`X-Forwarded-Proto`) suit le schéma d'`EVEY_SERVER_BASE_URL` : une URL de base
en `https://` l'active. `EVEY_FORWARDED_PROTO=true|false` est dépréciée et n'est
conservée que comme dérogation.

Voir [reverse-proxy.md](reverse-proxy.md).

### Détection des génériques
```env
EVEY_INTRO_TIDB_API_KEY=                # jeton Bearer facultatif pour TheIntroDB (500 requêtes par jour, contre 100 en anonyme)
EVEY_INTRO_FFMPEG_THREADS=1             # fils par processus ffmpeg employé pour la détection des génériques (1-16)
EVEY_INTRO_SKIP_PREVIEW_ENABLED=false   # fournir au lecteur les segments de "preview" (la bande-annonce de l'épisode suivant) pour qu'un bouton "Skip Preview" apparaisse
EVEY_INTRO_AUTO_SKIP_ENABLED=false      # tous les lecteurs sautent automatiquement les génériques détectés (le web et le mobile lisent le drapeau dans la réponse des segments ; pour les récepteurs de diffusion, il conditionne l'envoi des segments de générique)
```

`api.tidb_key`, `intro_providers.enabled`, `intro_providers.tidb_submit_enabled`,
`intro_providers.skip_preview_enabled` et `intro_providers.auto_skip_enabled`
se configurent aussi par
`/admin/server-config` (la base d'abord, l'environnement en repli). Redémarrez
le serveur après avoir basculé `intro_providers.enabled` ; les bascules du saut
de bande-annonce et du saut automatique de générique s'appliquent à chaud, sans
redémarrage.

Voir [adr/0011-intro-detection-providers.md](adr/0011-intro-detection-providers.md).

### Journalisation
```env
EVEY_LOG_LEVEL=info                    # debug, info, warn, error
EVEY_LOG_DIR=/data/logs                # la valeur par défaut de l'image ; le fichier compose la fait correspondre à ./evey-data/logs
EVEY_LOG_RETENTION_DAYS=90
```

Le fichier quotidien `evey-YYYY-MM-DD.log`, sous `EVEY_LOG_DIR`, est le journal
qui fait foi. `docker logs evey-server` montre le même flux, mais le pilote
json-file de Docker le fait tourner, et après une rotation la commande peut
sembler s'arrêter plusieurs heures avant le conteneur. Quand les deux divergent,
croyez le fichier - ou, en root, les fichiers bruts
`/var/lib/docker/containers/<id>/*-json.log*`. Hors de l'image, dans une
installation sans conteneur, la valeur par défaut est un répertoire `logs`
relatif au répertoire de travail : faites plutôt pointer `EVEY_LOG_DIR` vers un
chemin absolu accessible en écriture.

### Accès distant (qui peut se connecter depuis l'extérieur de votre réseau)
```env
EVEY_REMOTE_ACCESS_ENABLED=                                              # non défini = auto (voir plus bas) ; true/false pour trancher vous-même
EVEY_REMOTE_ACCESS_EXTERNAL_URL=                                         # l'adresse publique, si elle diffère d'EVEY_SERVER_BASE_URL
EVEY_REMOTE_ACCESS_LAN_NETWORKS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16  # les CIDR traités comme du réseau local
EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS=0                                    # 0 = aucun plafond pour les sessions distantes (WAN)
```

> ⚠️ Le verrou protège l'écoute **HTTP** (`EVEY_SERVER_PORT` + 1). L'écoute gRPC
> native, sur `EVEY_SERVER_PORT` lui-même, est un transport interne qu'il ne
> couvre pas ; elle n'écoute donc que sur la **boucle locale**
> (`EVEY_GRPC_BIND`, `127.0.0.1` par défaut), son unique client étant la
> passerelle HTTP du même processus. Mettez `EVEY_GRPC_BIND=0.0.0.0` pour
> l'exposer délibérément : le démarrage avertit alors que le verrou d'accès
> distant ne la couvre pas, et le pare-feu devient votre affaire.

**Le verrou.** Quand l'accès distant est coupé, tout client dont l'IP ne figure
pas dans `EVEY_REMOTE_ACCESS_LAN_NETWORKS` est refusé sur toutes les routes, avec
`403 {"code":"remote_access_disabled"}`. C'est un choix de déploiement (« cette
instance est réservée à mon réseau »), pas une couche de sécurité : ce qui
protège votre bibliothèque, c'est l'authentification.

**Automatique par défaut.** Laissez `EVEY_REMOTE_ACCESS_ENABLED` vide et le
serveur tranche au démarrage : si `EVEY_REMOTE_ACCESS_EXTERNAL_URL` ou
`EVEY_SERVER_BASE_URL` désigne une adresse joignable publiquement, l'accès
distant est **activé**, car un serveur que l'on publie est fait pour être
joint ; sinon, il reste cantonné au réseau local. La décision est journalisée au
démarrage, et `evey admin doctor` la rapporte.

Renseignez la variable, ou passez par **Admin → Remote Access**, pour trancher
explicitement ; un choix explicite n'est jamais écrasé par la déduction. Si vous
gardez délibérément un serveur publié en réseau local seulement, le journal de
démarrage porte un avertissement qui le dit, car le symptôme qui vous
parviendrait autrement, c'est « ça marche sur ton téléphone, pas sur le mien ».

> Derrière un reverse proxy, cela ne fonctionne que si le proxy transmet la
> véritable IP du client et qu'`EVEY_TRUSTED_PROXIES` couvre ce proxy. Sinon,
> tous les clients prennent l'apparence de l'adresse du proxy - voir
> [reverse-proxy.md](reverse-proxy.md).

Quand `EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS` est renseignée, les sessions de
lecture dont l'IP cliente est classée WAN sont plafonnées à ce débit ; ce
plafond ne fait jamais qu'ABAISSER la limite du client ou du profil. Deux
garde-fous à connaître :

- Une IP cliente privée, au sens de la RFC 1918, n'est **jamais** plafonnée,
  même si vous avez oublié d'inscrire sa plage dans
  `EVEY_REMOTE_ACCESS_LAN_NETWORKS` : le plafond ne s'applique qu'à une adresse
  positivement publique.
- Si vous employez des plages locales inhabituelles (le CGNAT
  `100.64.0.0/10`, un sous-réseau VPN ou Tailscale) et que vous ne voulez PAS
  plafonner ces spectateurs, inscrivez-les explicitement dans
  `EVEY_REMOTE_ACCESS_LAN_NETWORKS`. Derrière un reverse proxy, assurez-vous
  qu'il transmet la véritable IP du client (`X-Forwarded-For`), faute de quoi
  toutes les sessions prendront l'apparence de l'adresse du proxy.
