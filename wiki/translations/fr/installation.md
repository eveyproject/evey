---
source_sha: ec58362c8a7e791e617d5615c213693bfbd23e8c8494c6d1e5b6bce5ce563cb2
---

# Installation

Evey tourne dans **un seul conteneur** : l'image sert l'API et l'interface web
sur un même port, et fait tourner Postgres et Meilisearch en son sein, sous
l'utilisateur non root du serveur, avec leurs secrets générés au premier
démarrage. Le fichier compose de la section 1 est toute l'installation. Il ne
demande ni `.env`, ni `--privileged`, ni réglage de mémoire ou de processeur.

!!! warning "Pas le docker-compose.yml du dépôt"

    Le `docker-compose.yml` à la racine du dépôt Git n'est **pas** ce fichier.
    C'est la disposition opérateur (section 7) : Postgres en service voisin, un
    Caddy fourni, et un `.env` sans lequel il refuse de démarrer. Copiez le bloc
    ci-dessous, pas le fichier du dépôt.

Pour faire tourner Evey hors de Docker, voir [deployment-bare-metal.md](deployment-bare-metal.md).

## Prérequis

- Docker et Docker Compose v2 (`docker compose`, pas l'ancien `docker-compose`).
- Un emplacement de stockage sur un disque persistant (le compose ci-dessous
  utilise des montages liés dans le répertoire de travail).
- **Recommandé :** un reverse proxy pour le HTTPS, Evey ne servant que du HTTP
  simple. Voir [reverse-proxy.md](reverse-proxy.md).

## 1. Le fichier compose

Enregistrez ceci sous le nom `docker-compose.yml`, avec le chemin de vos médias
sur la dernière ligne de volume. Les secrets sont créés dans `./evey-data/keys`
au premier démarrage ; rien à inventer.

```yaml title="docker-compose.yml"
services:
  evey:
    image: gravityz/evey:latest # (1)!
    container_name: evey-server
    ports:
      - "9091:9091" # (2)!
    # environment:
    #   EVEY_SERVER_BASE_URL: http://<your-ip>:9091
    volumes:
      - ./evey-data:/data # (3)!
      - evey-cache:/cache # (4)!
      - /path/to/your/media:/media # (5)!
    stop_grace_period: 60s # (6)!
    restart: unless-stopped

volumes:
  evey-cache:
```

1. L'image. Épinglez un tag ici (`0.2.0-beta.1`, ou un `sha-1a2b3c4` immuable) pour que deux pulls à un jour d'écart fassent tourner le même code : voir [Mettre à jour Evey](#4-updating-evey).
2. L'API et l'interface web, sur un seul port, en HTTP simple. Pour le HTTPS, placez un [reverse proxy](reverse-proxy.md) devant et décommentez `EVEY_SERVER_BASE_URL` juste en dessous avec l'adresse publique : les liens des e-mails, les redirections OIDC et les URL de cast en dépendent.
3. Tout ce qui appartient à Evey : base de données, clé JWT, secrets générés, index de recherche, journaux, sauvegardes, extensions. C'est le dossier à sauvegarder.
4. Cache de transcodage et de trickplay. Peut être vidé sans risque : il se remplit à la demande.
5. Votre bibliothèque, à gauche des deux-points. En lecture-écriture exprès : la réparation de l'index de recherche temporelle reconstruit les fichiers sur place. Ajoutez `:ro` pour une bibliothèque en lecture seule, en acceptant que ces réparations échouent. D'autres dossiers s'ajoutent en `/media2`, `/media3`...
6. À un `docker stop`, la base intégrée est arrêtée proprement après le serveur ; un gros catalogue demande plus que les 10 s par défaut de Docker.

!!! tip "Un environnement minimal"

    Seuls les secrets que vous apportez, la connexion à la base quand elle est
    externe, les chemins de stockage et ce qui concerne le reverse proxy ont leur
    place dans le compose (ou dans un `.env` pour les secrets). Presque tout le
    reste - nom du serveur, transcodages simultanés, limites de débit, niveau de
    journalisation, intervalles de scan et de surveillance, planning des
    sauvegardes, clés d'API - **se modifie à chaud depuis Admin → Configuration du
    serveur** et reste hors du compose.

!!! info "À qui appartiennent les fichiers : PUID et PGID"

    Non définis par défaut : l'entrypoint adopte le propriétaire de vos montages
    `/media`, pour que les réparations sur place puissent écrire. Pour choisir,
    définissez `PUID` et `PGID` sous `environment:` (vos `id -u` et `id -g`). Le
    conteneur démarre en root uniquement pour appliquer cette correspondance, puis
    abandonne ses privilèges : le serveur lui-même ne tourne jamais en root.

???+ note "Sur un NAS (Synology Container Manager, QNAP Container Station) "

    - Créez le dossier de données **avant** le premier démarrage (`mkdir evey-data`
      à côté du compose, ou depuis File Station) : ces interfaces refusent un
      montage dont le dossier n'existe pas, contrairement à la ligne de commande
      Docker. Rien d'autre à créer d'avance - le point d'entrée crée lui-même
      `plugins/`, `keys/` et `postgres/`.
    - Fixez `PUID`/`PGID` sur l'utilisateur DSM propriétaire des médias
      (`id <utilisateur>` en SSH ; sur Synology, c'est typiquement autour de `1026`
      avec le groupe `100` = `users`) plutôt que de compter sur la détection
      automatique : un partage DSM appartient souvent à `root`, que le point
      d'entrée n'adopte jamais.
    - NAS Intel avec Quick Sync : ajoutez les lignes `devices:` et `group_add:` de
      `docker-compose.gpu.yml` (le GID du groupe render s'obtient avec
      `getent group render` ou `stat -c %g /dev/dri/renderD128`).
    - Aucun `.env` n'est nécessaire - le `docker-compose.yml` à la racine du dépôt,
      qui exige un `.env`, est la disposition opérateur, pas celle-ci.

## 2. Premier lancement

```bash
docker compose up -d
docker compose logs -f evey
```

Au tout premier démarrage, le serveur :

1. **génère automatiquement la clé de signature JWT** à l'emplacement
   `EVEY_JWT_PRIVATE_KEY_PATH` (`/data/jwt.key`, dans `./evey-data`). Vous n'avez
   pas à la créer à la main. Mettez `evey-data` à l'abri : perdre cette clé
   déconnecte tout le monde.
2. **exécute automatiquement les migrations de la base** (intégrées, appliquées
   au démarrage, sans étape de migration séparée). Voir
   [runbooks/migration-runbook.md](runbooks/migration-runbook.md).

Ouvrez ensuite `http://<your-ip>:9091`. Tant qu'aucun administrateur n'existe,
vous arrivez sur l'**assistant d'installation** (`/setup`) : créez le premier
compte administrateur. Connectez-vous ensuite et **ajoutez une bibliothèque**
(Admin → Bibliothèques) en la faisant pointer vers un chemin sous `/media`
(le dossier que vous y avez monté). La première analyse
démarre toute seule. L'assistant, les bibliothèques, l'invitation des proches et
ce qu'il faut vérifier avant de partager l'adresse sont détaillés dans les
[premiers pas](guide/first-steps.md).

## 3. Variantes courantes

**Des variantes, chacune à quelques lignes près sur ce fichier :**

- *Un second dossier de médias* : ajoutez `- /other/path:/media2` sous
  `volumes:` (et de même pour `/media3`, etc.). Montez-les en lecture-écriture,
  comme le premier.
- *Derrière un reverse proxy que vous exploitez déjà* (Nginx Proxy Manager,
  Traefik, etc., sur un réseau Docker) : retirez `ports:`, rejoignez le réseau
  du proxy et indiquez au serveur son URL publique ainsi que le pair à qui faire
  confiance :
  ```yaml
  services:
    evey:
      environment:
        EVEY_SERVER_BASE_URL: https://media.example.com
        EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # à restreindre au sous-réseau du proxy une fois connu
  networks:
    default:
      name: proxy-net        # le réseau sur lequel se trouve votre proxy
      external: true
  ```
  Le proxy vise ensuite `evey-server:9091`. Les réglages proxy par proxy :
  [reverse-proxy.md](reverse-proxy.md).
- *Du HTTPS sans proxy pour l'instant* : ajoutez le service Caddy fourni,
  décrit dans [reverse-proxy.md](reverse-proxy.md) (certificat Let's Encrypt
  automatique).
- *Postgres ou Meilisearch en services distincts* : la section 7.

**HTTPS et reverse proxy.** Evey sert du HTTP simple sur le port 9091. Si vous n'avez pas encore de proxy,
le service Caddy fourni dans [reverse-proxy.md](reverse-proxy.md) obtient pour
vous un certificat Let's Encrypt (les ports 80 et 443 doivent atteindre la
machine). Avec votre propre proxy (Nginx, Traefik, Nginx Proxy Manager) : donnez
à `EVEY_SERVER_BASE_URL` votre URL publique en `https://`, car c'est ce schéma
qui déclenche les cookies Secure et HSTS, puis déclarez le proxy de confiance
avec `EVEY_TRUSTED_PROXIES`.
Les points d'accès de diffusion réclament la mise en tampon désactivée et aucun
délai d'expiration ; les configurations toutes faites s'en chargent. Mises en
place complètes : [reverse-proxy.md](reverse-proxy.md).

## 4. Mettre Evey à jour

```bash
docker compose pull && docker compose up -d
docker compose logs -f evey      # jusqu'à "healthy" (sur un gros catalogue, une migration prend quelques minutes)
```

Rien d'autre n'est nécessaire. La nouvelle image applique les migrations en
attente à son démarrage, après avoir d'abord fait un dump de la base
(`./evey-data/backups/pre-migration-*`) ; le contrôle de santé du conteneur
passe au vert dès qu'il sert, et `up -d` ne redémarre que le service `evey`,
sans jamais toucher à une base ou à un proxy voisins.

**Revenir en arrière.** Remettez le tag précédent sur la ligne `image:` (ou dans
`EVEY_VERSION` si votre compose l'interpole) et relancez `docker compose up -d`.
Si la version plus récente a migré le schéma, restaurez d'abord son dump
d'avant migration : `docker exec evey-server evey admin rollback` affiche la
marche à suivre exacte, chemin du dump compris.

**Vérifier ce qui tourne.** `docker exec evey-server evey version` affiche la
version et le commit du binaire en cours d'exécution (`/healthz` rapporte la
même chose) : la question « quel code tourne ici ? » ne dépend donc jamais d'un
tag.

> Les administrateurs qui travaillent depuis une copie du dépôt disposent de la
> même procédure sous forme de script : `scripts/evey-update.sh` (à blanc par
> défaut : état, dump préalable, pull, attente de la santé, vérification du
> commit, ligne de retour en arrière) et `scripts/evey-rollback.sh`. Ce sont des
> commodités pour les déploiements du dépôt lui-même, pas des outils dont une
> installation aurait besoin.

**Épinglez ce que vous exécutez.** Mettez sur la ligne `image:` un tag de
version (`0.2.0-beta.1`) ou un tag immuable `sha-<short>` ; un tag de branche
comme `beta-0.0.1` bouge à chaque push, si bien que deux pulls espacés d'un jour
peuvent faire tourner du code différent et que « revenir à `beta-0.0.1` » ne
mène nulle part. Le script affiche le tag qu'il a lancé, pour que vous puissiez
l'épingler.

**Les tags d'image** (`gravityz/evey`) :

- Les tags de version `vX.Y.Z` publient `X.Y.Z`, `X.Y`, `X` et `latest`.
  `latest` désigne toujours la **version** semver la plus récente.
- Les compilations de branche publient le nom de la branche (par exemple
  `beta-0.0.1`) et un `sha-<short>`.
- **Des variantes d'exécution** - la même application, mais des environnements
  d'exécution d'extensions différents :
  - sans suffixe (la variante par défaut, dite « lean ») - Python 3 seulement.
  - `-nodejs` - ajoute Node pour les extensions en JS.
  - `-full` - ajoute en plus PHP, Ruby et Lua.
  Ne prenez `-nodejs` ou `-full` que si vos extensions réclament ces
  environnements ; sinon, la variante par défaut est la plus légère. Écrivez par
  exemple `EVEY_VERSION=latest-nodejs`.
- **Des variantes GPU** - les étiquettes publiées embarquent toutes les familles
  d'accélérateurs (Intel Gen9 à aujourd'hui, AMD), soit environ 900 Mo rien que
  pour les compilateurs OpenCL d'Intel. Une image construite soi-même peut ne
  garder que la famille de sa machine avec
  `docker build --build-arg GPU=intel-legacy .` (Intel Gen9 à Gen11, les
  UHD 6xx), `GPU=intel` (Gen12 et plus récent), `GPU=amd`, ou `GPU=none`.
  L'étiquette d'image `org.evey.gpu` garde la trace du choix. NVIDIA n'a besoin
  d'aucune variante : aucune bibliothèque NVIDIA n'est intégrée à une image, le
  NVIDIA Container Toolkit injecte le pilote de l'hôte à l'exécution
  (`--gpus all` ou une réservation de périphérique dans le compose), donc NVENC
  fonctionne avec toutes les variantes, `none` compris. Sur une machine qui a à
  la fois un iGPU Intel et une carte NVIDIA, le mode `auto` du calibrage choisit
  VAAPI en premier ; fixez le mode matériel sur `nvenc` dans le panneau de
  transcodage pour utiliser la carte.

## 5. Sauvegardes et récupération

Activez les sauvegardes planifiées de la base (`EVEY_BACKUP_INTERVAL_HOURS`,
rangées dans `EVEY_BACKUP_DIR` → `/data/backups`, à l'intérieur d'`evey-data`),
ou déclenchez-en une depuis le panneau d'administration. Les sauvegardes
couvrent l'ensemble des données utilisateur, pas vos fichiers multimédias.
Détails : [backups.md](backups.md).

En cas de verrouillage, de perte du compte administrateur ou de restauration,
l'image embarque une CLI `evey admin` (`reset-password`, `rotate-jwt-key`,
`restore-backup`) : [runbooks/recovery.md](runbooks/recovery.md). Sûreté des
migrations : [runbooks/migration-runbook.md](runbooks/migration-runbook.md).

## 6. Dépannage

Les premières vérifications quand quelque chose cloche :

```bash
docker compose ps                       # tous les services sont-ils Up et healthy ?
docker compose logs --tail=200 evey     # journaux du serveur (erreurs de migration, validation de la configuration)
```

- **evey refuse de démarrer, erreur de configuration.** Avec une base externe,
  le serveur refuse de démarrer tant que `EVEY_DB_PASSWORD` vaut encore la
  valeur par défaut `evey` ; avec un Meilisearch externe, tant que
  `MEILI_MASTER_KEY` est vide ou reste un texte de remplissage (une clé courte
  mais différente de la valeur par défaut ne déclenche qu'un avertissement).
  Lisez le message de validation dans le journal et corrigez le `.env`.
- **La base embarquée refuse de démarrer.** Les lignes `evey:` en tête de
  journal en donnent la raison : un cluster issu d'une autre version majeure
  (voir la section sur la montée de version plus haut), un répertoire de données
  qui n'est pas un cluster Postgres, ou une erreur préfixée `postgres:` venue de
  Postgres lui-même (disque plein, permissions sur `./evey-data/postgres`,
  système de fichiers en partage réseau).
- **Redémarrages en boucle avec `permission denied` sur `/cache/...` dans le
  journal.** Le volume `evey-cache` appartient à un autre uid que celui sous
  lequel tourne le serveur (`PUID`, ou le propriétaire des médias adopté). Les
  images antérieures au 2026-09-17 ne le ré-attribuaient que si `./evey-data`
  changeait aussi de propriétaire : un dossier de données créé d'avance au nom
  de `PUID` (le cas NAS ci-dessus) laissait le cache de côté. Mettez l'image à
  jour ; sur une ancienne, lancez une fois `docker run --rm -v evey_evey-cache:/c
  alpine chown -R <PUID>:<PGID> /c` (le volume s'appelle
  `<projet>_evey-cache`).
- **La base externe est injoignable.** Vérifiez que Postgres est en bonne santé
  et que le mot de passe correspond :
  ```bash
  docker exec evey-postgres pg_isready -U "$EVEY_DB_USER"
  ```
- **La recherche est cassée ou vide.** Contrôlez la santé de Meilisearch, puis
  déclenchez une réindexation, depuis les réglages de recherche du panneau
  d'administration ou par `docker exec evey-server evey admin reindex-search`.
  L'index est reconstruit depuis Postgres : effacer `meili-data` ne présente
  donc aucun risque.
- **Point d'accès de santé :** `curl -sf http://localhost:9091/healthz && echo OK`.

### Quels volumes peuvent être supprimés sans risque

| Volume | Contenu | Suppression sans risque ? |
|--------|----------|-----------------|
| `evey-data` | Base de données (`postgres/`), clé JWT et secrets générés (`keys/`), journaux, sauvegardes, extensions, index de recherche (`meili/`) | Non - données, clé JWT et extensions perdues. `meili/` seul : oui, il se reconstruit avec `evey admin reindex-search` |
| `postgres-data` | Base de données (variante externe uniquement) | Non - toutes les données sont perdues |
| `evey-cache` | Cache de transcodage et de trickplay | Oui - régénéré à la demande |

## 7. Disposition opérateur : Postgres et Meilisearch externes

**Comment se comporte la base embarquée.** Le cluster réside dans
`./evey-data/postgres` (Postgres 17, `initdb` au premier démarrage, propriété de
l'utilisateur du serveur). Il n'écoute que sur la boucle locale du conteneur et
exige sur TCP le mot de passe généré : une extension rétrogradée vers un autre
uid ne peut donc pas l'ouvrir. Un `docker stop` lui parvient sous la forme d'un
arrêt rapide et ordonné, une fois que le serveur a fermé ses connexions. Le
répertoire de données doit reposer sur un vrai système de fichiers (ext4, btrfs,
xfs, ...), pas sur un partage CIFS ou NFS. Les sauvegardes (`evey admin`, le
panneau d'administration, le `pg_dump` planifié) fonctionnent exactement comme
avec une base externe. Pour ouvrir un shell SQL :

```bash
docker exec -it evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

### Postgres externe

Les administrateurs qui veulent garder le cycle de vie de la base entre leurs
mains (un Postgres infogéré, une version majeure épinglée, un volume à part, des
dumps et des restaurations à leur main) le font tourner en service voisin :
ajoutez le service et faites-y pointer le serveur. **C'est le fait de définir
`EVEY_DB_HOST` qui éteint la base embarquée**, ce qui explique que toutes les installations antérieures à la base embarquée continuent
de fonctionner sans y toucher :

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... comme ci-dessus, plus :
        environment:
          EVEY_DB_HOST: postgres
          EVEY_DB_SSLMODE: disable          # réseau interne, sans TLS
        depends_on:
          postgres: { condition: service_healthy }

      postgres:
        image: postgres:16-alpine
        container_name: evey-postgres
        environment:
          POSTGRES_DB: ${EVEY_DB_NAME:-evey}
          POSTGRES_USER: ${EVEY_DB_USER:-evey}
          POSTGRES_PASSWORD: ${EVEY_DB_PASSWORD:?EVEY_DB_PASSWORD must be set in .env}
        volumes:
          - ./postgres-data:/var/lib/postgresql/data
        healthcheck:
          test: ["CMD-SHELL", "pg_isready -U ${EVEY_DB_USER:-evey}"]
          interval: 5s
          timeout: 3s
          retries: 5
        restart: unless-stopped
    ```

=== ".env"

    ```env
    EVEY_DB_PASSWORD=<strong-password>   # NE doit PAS être la valeur par défaut 'evey', sinon le serveur refuse de démarrer
    ```

**Un Meilisearch externe, même principe.** Un conteneur de recherche séparé
conserve son propre cycle de vie et son propre volume, et permet de ne
réinitialiser que l'index par un `docker compose restart meilisearch`. Éteignez
celui qui est embarqué, faites-y pointer le serveur et partagez la clé : un
`MEILI_MASTER_KEY` explicite l'emporte toujours sur le
`./evey-data/keys/meili.key` généré :

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... plus :
        environment:
          EVEY_EMBEDDED_MEILI: "false"
          EVEY_MEILI_URL: http://meilisearch:7700
        depends_on:
          meilisearch: { condition: service_healthy }

      meilisearch:
        image: getmeili/meilisearch:v1.12
        container_name: evey-meilisearch
        environment:
          MEILI_MASTER_KEY: ${MEILI_MASTER_KEY:?MEILI_MASTER_KEY must be set in .env}
          MEILI_NO_ANALYTICS: "true"
        volumes:
          - ./meili-data:/meili_data
        healthcheck:
          test: ["CMD", "curl", "-sf", "http://localhost:7700/health"]
          interval: 5s
          retries: 5
        restart: unless-stopped
    ```

=== ".env"

    ```env
    MEILI_MASTER_KEY=<run: openssl rand -hex 16>
    ```

Donnez si possible à ce conteneur un `user:` non root et un `mem_limit` :
l'image officielle tourne en root, et son empreinte résidente (l'index projeté
en mémoire) paraît énorme alors qu'il s'agit pour l'essentiel de cache de pages
récupérable. L'index se reconstruit depuis Postgres
(`evey admin reindex-search`) : `meili-data` est de toute façon jetable.

Seules ces variables d'environnement relèvent de l'infrastructure ou du secret
et **ont leur place dans le compose ou le `.env`** ; elles ne sont jamais
modifiables depuis le panneau d'administration : `EVEY_DB_PASSWORD`,
`MEILI_MASTER_KEY`, `EVEY_DB_HOST`, `EVEY_DB_USER`, `EVEY_DB_NAME`,
`EVEY_DB_SSLMODE`, `EVEY_EMBEDDED_POSTGRES`, `EVEY_EMBEDDED_MEILI`,
`EVEY_MEILI_URL`, les chemins `*_DIR`, `EVEY_JWT_PRIVATE_KEY_PATH`,
`EVEY_SERVER_BASE_URL`, `EVEY_TRUSTED_PROXIES`, et l'interrupteur général des
extensions `EVEY_PLUGINS_ENABLED`. (`EVEY_PLUGINS_SANDBOX_MODE`,
`EVEY_PLUGINS_UID_SEPARATION` et les plafonds de ressources peuvent y figurer
comme valeurs de départ, mais restent modifiables à chaud depuis Admin →
Plugins.) Référence complète : [configuration.md](configuration.md).

### Faire basculer une installation existante sur la base embarquée

Une installation qui fait tourner Postgres en service voisin (la disposition
qu'employaient toutes les versions antérieures à la base embarquée) peut passer
au conteneur unique : les données ne sont qu'à un dump de distance, et tout le
reste de `./evey-data` (clé JWT, extensions, sauvegardes, trickplay) est repris
tel quel. La base embarquée importe **une seule fois** un dump trouvé en
`./evey-data/postgres-import.sql.gz`, **dans le cluster qu'elle crée à son
premier démarrage**, avant que le serveur ne tourne ; les migrations du serveur
s'appliquent ensuite par-dessus, comme après n'importe quelle mise à jour.

```bash
# 1. dump de la base actuelle (SQL brut, gzippé - ce qu'attend l'import)
docker compose exec -T postgres pg_dump -U evey --no-owner --no-privileges evey \
  | gzip > evey-data/postgres-import.sql.gz
docker compose down

# 2. rédigez le nouveau compose (gardez une copie de l'ancien) : le fichier à
#    service unique de la section 1, avec vos variantes (dossiers de médias
#    supplémentaires, le bloc du proxy) et tout ce que contenait votre
#    `environment:` (transcodage matériel : `devices: /dev/dri`, `group_add`,
#    LIBVA_DRIVER_NAME).

# 3. premier démarrage : initdb, import, migrations, puis le serveur
docker compose up -d && docker compose logs -f evey     # "evey: import done"

# 4. index de recherche : celui d'un Meilisearch externe n'est pas repris, reconstruisez-le
docker exec evey-server evey admin reindex-search
```

Ce qui n'est PLUS nécessaire : `EVEY_DB_PASSWORD`, `MEILI_MASTER_KEY` et le
câblage `EVEY_DB_HOST` / `EVEY_MEILI_URL` (les secrets sont générés), les
services `postgres` et `meilisearch`, ainsi que les clés d'API amorcées depuis
le `.env` (elles se trouvent déjà dans le `server_config` du dump). Conservez
`./postgres-data` et `./meili-data` le temps de vérifier le serveur ; l'ancien
compose sait encore les faire tourner s'il faut revenir en arrière. Un dump
laissé à côté d'un cluster déjà initialisé est signalé et ignoré, jamais
fusionné ; un import qui échoue jette le nouveau cluster, de sorte que le
démarrage suivant retente depuis le dump.

### Montée de version majeure de Postgres (base embarquée)

L'image épingle la version majeure du Postgres embarqué (17 aujourd'hui). Une
image plus récente livrée avec une majeure plus récente **refuse d'ouvrir**
l'ancien cluster, avec un message qui renvoie à cette section : elle n'exploite
jamais en silence un répertoire de données qu'elle ne sait pas lire. Le passage
se fait par un dump avec l'ancienne image, puis une restauration avec la
nouvelle :

```bash
docker compose exec evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_dump -h 127.0.0.1 -U evey -Fc evey' > evey-before-upgrade.dump
docker compose down
mv evey-data/postgres evey-data/postgres.old      # à conserver jusqu'à ce que la restauration soit vérifiée
# renseignez le nouveau EVEY_VERSION dans .env, puis :
docker compose up -d                              # initialise un cluster vide sur la nouvelle majeure
docker compose exec -T evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_restore -h 127.0.0.1 -U evey -d evey --no-owner' < evey-before-upgrade.dump
docker compose restart evey
```

Les administrateurs qui préfèrent `pg_upgrade`, ou une majeure épinglée pour des
années, se tournent vers la
[variante opérateur avec un Postgres externe](#7-operator-layout-external-postgres-and-meilisearch).

## 8. Machines ARM (x86-64 et ARM64)

Les versions stables (les tags `vX.Y.Z`) publient une image
**multi-architecture**, pour `linux/amd64` comme pour `linux/arm64` ;
`docker pull` et `compose up` retiennent la bonne automatiquement, sans le
moindre drapeau. Les tags de préversion (`-alpha`, `-beta`) et les compilations
par branche sont **réservés à amd64** : sur une machine ARM (un Raspberry Pi, un
NAS ARM, un VPS Ampere), épinglez un `vX.Y.Z` stable dans `EVEY_VERSION`.

Deux choses à savoir sur ARM :

- **Transcodage matériel sur ARM : Rockchip seulement, et seulement ce que le
  calibrage prouve.** Evey câble quatre moteurs d'accélération : Intel QSV,
  VAAPI, NVENC, et **RKMPP** de Rockchip (SoC de la classe RK3588 / RK3576 /
  RK3568, via les encodeurs `h264_rkmpp` / `hevc_rkmpp` du jellyfin-ffmpeg arm64,
  le décodage `drm_prime`, la mise à l'échelle RGA et un tone mapping OpenCL sur
  l'ICD Mali quand il est présent). QSV et les pilotes VAAPI d'Intel sont
  propres au x86 et ne sont volontairement pas installés dans l'image arm64 ;
  NVENC est du NVIDIA. Pour utiliser RKMPP, exposez au conteneur le nœud du
  service MPP et le nœud de rendu DRM (`/dev/mpp_service`, `/dev/dri`, et
  `/dev/rga` si le noyau l'expose) ; Evey le détecte, lance un vrai test
  d'encodage, puis mesure et éprouve chaque format sur votre carte avant de le
  proposer - rien n'est déduit du nom du SoC, et un format qui échoue reste sur
  le processeur. RKMPP n'a pas encore été validé par le projet sur du vrai
  matériel : considérez-le comme expérimental et regardez dans **Admin →
  Transcodage → Calibrage de l'encodage** ce qui a été certifié. L'encodeur
  **V4L2 M2M** du Raspberry Pi n'est **pas** câblé : sur un Pi ou un NAS ARM de
  type Synology (Realtek), il n'y a aucun chemin d'encodage matériel, donc ARM
  signifie **lecture directe** (le client lit le fichier tel quel, pour un coût
  quasi nul) plus **transcodage logiciel** sur le processeur. La capacité est
  calibrée, pas devinée : un cœur modeste garde le plancher logiciel de 720p et
  n'admet qu'un encodage à la fois ; vous pouvez encore abaisser
  `transcoding.max_video_sessions` (Admin → Transcodage) et vous appuyer sur la
  lecture directe.
- **Le tone mapping HDR emprunte lui aussi le chemin rapide sur ARM.** Le
  jellyfin-ffmpeg arm64 fourni embarque le filtre SIMD `tonemapx` (en NEON), tout
  comme la compilation amd64 embarque sa variante AVX2 : la conversion HDR→SDR
  passe donc par du SIMD matériel, et non par la lente chaîne zscale. (Evey sonde
  `tonemapx` à l'exécution et ne se replie sur zscale que si une compilation
  venait à en être dépourvue.)

Si vous compilez vous-même l'image pour une architecture non publiée,
`docker buildx build --platform linux/arm64` fonctionne tel quel : le Dockerfile
protège les parties propres au x86 derrière `TARGETARCH`.

## 9. Bac à sable des extensions

Les extensions sont du code tiers qui tourne dans des processus séparés. L'image
officielle contient tout ce qu'il faut pour les confiner, sans `--privileged` ni
option de sécurité Docker supplémentaire, et `EVEY_PLUGINS_SANDBOX_MODE=auto`
(la valeur par défaut) retient le mécanisme le plus fort que la machine permet,
Landlock sur tout noyau récent. Le serveur lui-même ne tourne jamais en root. Le
détail couche par couche (seccomp, séparation d'uid, plafonds de ressources) est
dans [SECURITY.md](SECURITY.md) ; cette page ne dit que ce qu'il faut vérifier.

Après le premier démarrage, vérifiez le bac à sable réellement en vigueur dans
**Admin → Plugins → Security Dashboard** (la carte du bac à sable de l'hôte) :
sur un noyau moderne, `auto` doit afficher `landlock` ; `basic` signifie que
l'hôte ne sait pas isoler les accès des extensions au système de fichiers,
auquel cas reportez-vous au guide de déploiement ci-dessous.

Quelle configuration pour votre machine (vieux NAS, installation sans conteneur, ce qu'il faut activer ou couper, les risques en langage courant) : [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md). Modèle de menace complet et détail couche par couche : [plugins/security-model.md](plugins/security-model.md).
Vous pouvez conserver le `auto` par défaut (recommandé) ou fixer
`EVEY_PLUGINS_SANDBOX_MODE: landlock` dans le compose pour être explicite et
fermé par défaut (`landlock` refuse de démarrer une extension si Landlock ne
peut pas être appliqué). Sur un noyau moderne, les deux donnent la même posture.
