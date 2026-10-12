---
source_sha: be2352babc32903dbb0a839c8c6754b1f9c0751d295a958a65e2ab12031c037e
reviewed: true
---
# Déploiements avancés

L'[installation en un conteneur](../installation.md) convient à la plupart
des serveurs. Ouvrez cette page quand vous voulez la base ou le moteur de
recherche en services séparés, quand l'hôte est en ARM, quand il vous faut
une variante de l'image, ou quand la sandbox des plugins signale autre chose
que `landlock`. Pour faire tourner Evey sans Docker du tout, voir
[Sans conteneur](../deployment-bare-metal.md).

## La base embarquée

Le cluster se trouve dans `./evey-data/postgres` (Postgres 17, créé au premier
démarrage, propriété de l'utilisateur du serveur). Il n'écoute que sur la
boucle locale du conteneur et exige le mot de passe généré, donc un plugin
tournant sous un autre utilisateur ne peut pas l'ouvrir. `docker stop`
l'arrête proprement une fois que le serveur a fermé ses connexions.

Le dossier de données doit être sur un système de fichiers local (ext4,
btrfs, xfs...), pas sur un partage SMB/CIFS ou NFS. Pour ouvrir un shell
SQL :

```bash
docker exec -it evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

### Passer à une version majeure plus récente de Postgres

L'image fige la version majeure embarquée. Une image livrant une majeure plus
récente refuse d'ouvrir l'ancien cluster, avec un message qui renvoie ici,
plutôt que de lancer un dossier de données qu'elle ne sait pas lire. Le
passage se fait par un dump avec l'ancienne image et une restauration avec la
nouvelle :

```bash
docker compose exec evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_dump -h 127.0.0.1 -U evey -Fc evey' > evey-before-upgrade.dump
docker compose down
mv evey-data/postgres evey-data/postgres.old      # à garder jusqu'à ce que la restauration soit vérifiée
# mettez le nouveau tag d'image dans le fichier compose, puis :
docker compose up -d                              # crée un cluster vide sur la nouvelle majeure
docker compose exec -T evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_restore -h 127.0.0.1 -U evey -d evey --no-owner' < evey-before-upgrade.dump
docker compose restart evey
```

## Faire tourner Postgres en service séparé

Pour gérer vous-même le cycle de vie de la base (un Postgres managé, une
majeure figée, votre propre politique de dumps), lancez-la comme service à
part et configurez le serveur pour l'utiliser. C'est le fait de définir
`EVEY_DB_HOST` qui désactive la base embarquée. Le serveur refuse de démarrer avec le mot de
passe par défaut `evey`.

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... comme dans le fichier à un conteneur, plus :
        environment:
          EVEY_DB_HOST: postgres
          EVEY_DB_SSLMODE: disable          # réseau interne, pas de TLS
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
    EVEY_DB_PASSWORD=<mot-de-passe-solide>
    ```

## Faire tourner Meilisearch en service séparé

Même principe pour le moteur de recherche : désactivez celui qui est embarqué,
configurez le serveur pour utiliser le vôtre et partagez la clé. Un `MEILI_MASTER_KEY`
explicite l'emporte sur le `./evey-data/keys/meili.key` généré.

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
    MEILI_MASTER_KEY=<lancez : openssl rand -hex 16>
    ```

L'index se reconstruit depuis la base avec `docker exec evey-server evey
admin reindex-search`, donc `meili-data` peut toujours être supprimé.

## Revenir à la base embarquée

Une installation qui fait tourner Postgres en service séparé peut passer au
conteneur unique. La base embarquée importe une fois un dump trouvé dans
`./evey-data/postgres-import.sql.gz`, dans le cluster qu'elle crée à son
premier démarrage, avant que le serveur ne tourne ; les migrations du serveur
s'appliquent ensuite par-dessus.

```bash
# 1. dumpez la base actuelle (SQL brut, gzippé)
docker compose exec -T postgres pg_dump -U evey --no-owner --no-privileges evey \
  | gzip > evey-data/postgres-import.sql.gz
docker compose down

# 2. remplacez le fichier compose par celui à un conteneur, en gardant vos
#    dossiers de médias, vos réglages de proxy inverse et vos lignes matériel.

# 3. premier démarrage : création du cluster, import, migration, lancement du serveur
docker compose up -d && docker compose logs -f evey     # "evey: import done"

# 4. un index Meilisearch externe n'est pas repris : reconstruisez-le
docker exec evey-server evey admin reindex-search
```

`EVEY_DB_PASSWORD`, `MEILI_MASTER_KEY` et les réglages `EVEY_DB_HOST` /
`EVEY_MEILI_URL` ne sont plus nécessaires. Gardez `./postgres-data` jusqu'à
avoir vérifié le serveur. Un dump laissé à côté d'un cluster existant est
signalé et ignoré, jamais fusionné ; un import échoué supprime le nouveau cluster
pour que le démarrage suivant réessaie depuis le dump.

## Hôtes ARM

Les images construites depuis un tag de version (`0.2.0`, préversions comme
`0.2.0-beta.1` comprises) sont multi-architectures (`linux/amd64` et
`linux/arm64`) ; Docker choisit la bonne. Les tags de branche comme
`beta-0.0.1` sont amd64 uniquement : sur un hôte ARM, utilisez un tag de
version.

- **Le transcodage matériel sur ARM** se limite aux SoC Rockchip (classe
  RK3588, RK3576, RK3568) via RKMPP. Mappez `/dev/mpp_service`, `/dev/dri`
  et, quand le noyau l'expose, `/dev/rga` ; le serveur sonde et mesure
  l'encodeur avant de s'en servir. RKMPP n'a pas encore été validé par le
  projet sur du matériel réel. Un Raspberry Pi ou un NAS ARM à base de
  Realtek n'a aucun encodeur matériel utilisable par le serveur : ces hôtes
  s'appuient sur la lecture directe et le transcodage logiciel, dimensionné
  par le même calibrage que n'importe quel autre hôte.
- **Le tone mapping HDR** utilise le même filtre SIMD `tonemapx` sur ARM
  (NEON) que sur x86-64 (AVX2).

## Tags et variantes de l'image

L'image est `eveyproject/evey`.

| Tag | Publié par | Notes |
|---|---|---|
| `X.Y.Z`, `X.Y`, `X`, `latest` | un tag de version `vX.Y.Z` | `latest` est toujours la version stable la plus récente |
| `X.Y.Z-beta.N` | un tag de préversion | ne publie que son propre tag, ne déplace jamais `latest` |
| `beta-0.0.1`, `sha-<commit>` | une construction de branche | amd64 uniquement |

Chaque tag existe en trois variantes : celle par défaut embarque Python 3
pour les plugins, `-nodejs` ajoute Node.js, `-full` ajoute PHP, Ruby et Lua
(par exemple `latest-full`). N'utilisez une variante que si un plugin en a
besoin : l'installateur refuse un plugin dont le langage déclaré manque à
l'image, avec un message qui nomme la bonne variante.

Les images publiées embarquent les pilotes de chaque génération Intel et AMD
prise en charge. Une image construite soi-même peut ne garder qu'une famille
avec `docker build --build-arg GPU=intel-legacy .` (Intel Gen9 à Gen11),
`GPU=intel` (Gen12 et plus récent), `GPU=amd` ou `GPU=none`. NVIDIA n'a pas
besoin de variante : le NVIDIA Container Toolkit injecte le pilote de l'hôte
à l'exécution.

## La sandbox des plugins sur un hôte inhabituel

L'image contient tout ce qu'il faut pour confiner les plugins, sans mode
privilégié. Le mode de sandbox par défaut (`auto`) choisit le moteur le plus
fort que l'hôte autorise : Landlock sur tout noyau moderne. Vérifiez le mode
en vigueur dans le **Tableau de bord sécurité** de **Administration → Plugins**. Un
hôte qui affiche `basic` ne sait pas isoler l'accès des plugins aux
fichiers : [Sandbox des plugins](../plugins/sandbox-deployment.md) explique
ce que cela signifie et quoi changer. Les couches elles-mêmes sont décrites dans
[Modèle de sécurité et sandbox des plugins](../SECURITY.md).

<!-- sources
embedded db, import, major upgrade, embedded meili: docker/entrypoint.sh (PG_MAJOR, PG_IMPORT, "evey: import done", EVEY_EMBEDDED_MEILI, MEILI_MASTER_KEY_FILE)
external db switch and default password refusal: internal/config/config.go (EVEY_DB_HOST, hardFail on "evey")
reindex: cmd/evey/admin.go - adminReindexSearch
arm, variants, GPU families: Dockerfile (TARGETARCH, RUNTIMES, GPU) ; .github/workflows/ci.yml (tags, flavor suffix, platforms) ; internal/adapters/driven/ffmpeg/hwdetect.go - detectRKMPP
sandbox auto: internal/adapters/driven/pluginrunner/runner_landlock.go - resolveAutoMode ; i18n/fr/admin.yaml - plugins.security.title
-->
