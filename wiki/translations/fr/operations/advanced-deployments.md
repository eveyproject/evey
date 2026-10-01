---
source_sha: fffc05516a2ca96d615a64bc159a1b837a7601876648b7e40b6167296b59f1d5
reviewed: true
---
# Déploiements avancés

L'[installation en un seul conteneur](../installation.md) convient à la
plupart des serveurs. Cette page couvre les dispositions qu'un administrateur
système peut préférer : la base de données ou le moteur de recherche en
services séparés, les machines ARM, les variantes de l'image, et la sandbox
des plugins sur des machines particulières. Pour faire tourner Evey sans
Docker, voir [Sans conteneur](../deployment-bare-metal.md).

## La base de données intégrée

La base vit dans `./evey-data/postgres` (Postgres 17, créée au premier
démarrage, propriété de l'utilisateur du serveur). Elle n'écoute que sur la
boucle locale du conteneur et exige le mot de passe généré : un plugin qui
tourne sous un autre utilisateur ne peut pas l'ouvrir. `docker stop` l'arrête
proprement une fois que le serveur a fermé ses connexions.

Le dossier de données doit se trouver sur un système de fichiers local (ext4,
btrfs, xfs...), pas sur un partage SMB/CIFS ou NFS.

Pour ouvrir un shell SQL :

```bash
docker exec -it evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

### Montée de version majeure de Postgres

L'image fige la version majeure de Postgres intégrée (17). Une image livrant
une version majeure plus récente refuse d'ouvrir l'ancienne base, avec un
message qui renvoie ici, plutôt que d'exploiter un dossier de données qu'elle
ne sait pas lire. Le passage se fait par un dump avec l'ancienne image et une
restauration avec la nouvelle :

```bash
docker compose exec evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_dump -h 127.0.0.1 -U evey -Fc evey' > evey-avant-montee.dump
docker compose down
mv evey-data/postgres evey-data/postgres.old      # à garder jusqu'à ce que la restauration soit vérifiée
# mettez le nouveau tag d'image dans le fichier compose, puis :
docker compose up -d                              # crée une base vide avec la nouvelle version majeure
docker compose exec -T evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_restore -h 127.0.0.1 -U evey -d evey --no-owner' < evey-avant-montee.dump
docker compose restart evey
```

## Postgres externe

Pour gérer vous-même le cycle de vie de la base (un Postgres managé, une
version majeure figée, votre propre politique de dump), faites-la tourner en
service séparé et indiquez-la au serveur. **C'est la présence de
`EVEY_DB_HOST` qui désactive la base intégrée.**

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... comme dans le fichier en un seul conteneur, plus :
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
    EVEY_DB_PASSWORD=<mot-de-passe-robuste>   # le serveur refuse de démarrer avec la valeur par défaut 'evey'
    ```

## Meilisearch externe

Même principe pour le moteur de recherche : désactivez celui qui est intégré,
indiquez le vôtre au serveur et partagez la clé. Un `MEILI_MASTER_KEY`
explicite prime sur le fichier généré `./evey-data/keys/meili.key`.

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
    MEILI_MASTER_KEY=<à générer : openssl rand -hex 16>
    ```

L'index se reconstruit depuis la base (`evey admin reindex-search`) :
`meili-data` peut toujours être supprimé.

## Passer à la base de données intégrée

Une installation où Postgres tourne en service séparé peut passer au conteneur
unique. La base intégrée importe un dump trouvé dans
`./evey-data/postgres-import.sql.gz`, une seule fois, dans la base qu'elle
crée à son premier démarrage et avant que le serveur ne démarre ; les
migrations du serveur s'appliquent ensuite par-dessus.

```bash
# 1. exporter la base actuelle (SQL brut, compressé en gzip)
docker compose exec -T postgres pg_dump -U evey --no-owner --no-privileges evey \
  | gzip > evey-data/postgres-import.sql.gz
docker compose down

# 2. remplacer le fichier compose par celui en un seul conteneur, en gardant
#    vos dossiers de médias, vos réglages de reverse proxy et vos lignes matérielles.

# 3. premier démarrage : création de la base, import, migrations, démarrage du serveur
docker compose up -d && docker compose logs -f evey     # "evey: import done"

# 4. l'index d'un Meilisearch externe n'est pas repris : le reconstruire
docker exec evey-server evey admin reindex-search
```

`EVEY_DB_PASSWORD`, `MEILI_MASTER_KEY` et les réglages `EVEY_DB_HOST` /
`EVEY_MEILI_URL` ne servent plus. Gardez `./postgres-data` jusqu'à avoir
vérifié le serveur. Un dump laissé à côté d'une base existante est signalé et
ignoré, jamais fusionné ; un import raté supprime la nouvelle base pour que le
démarrage suivant recommence.

## Machines ARM

Les images construites depuis un tag de version (`0.2.0`, préversions comme
`0.2.0-beta.1` comprises) sont multi-architecture (`linux/amd64` et
`linux/arm64`) ; Docker choisit la bonne. Les tags de branche comme
`beta-0.0.1` sont en amd64 uniquement : figez un tag de version sur une machine
ARM.

- **Le transcodage matériel sur ARM** se limite aux SoC Rockchip (classe
  RK3588, RK3576, RK3568), via RKMPP. Montez `/dev/mpp_service`, `/dev/dri`
  et, si le noyau l'expose, `/dev/rga` ; Evey teste et mesure l'encodeur avant
  de l'utiliser. RKMPP n'a pas encore été validé par le projet sur du vrai
  matériel. Un Raspberry Pi ou un NAS ARM à base de Realtek n'a aucun
  encodeur matériel utilisable par Evey : ces serveurs s'appuient sur la
  lecture directe et le transcodage logiciel, dimensionnés par la même mesure
  que n'importe quelle autre machine.
- **Le tone mapping HDR** utilise sur ARM le même filtre SIMD `tonemapx`
  (NEON) que sur x86-64 (AVX2).

## Tags d'image

L'image est `gravityz/evey`.

- Les tags de version `vX.Y.Z` publient `X.Y.Z`, `X.Y`, `X` et `latest`.
  `latest` désigne toujours la dernière version stable : une préversion
  (`vX.Y.Z-beta.N`) ne publie que son propre tag et ne déplace jamais `latest`.
- Les builds de branche publient le nom de la branche (par exemple
  `beta-0.0.1`) et `sha-<commit>`.
- **Runtimes des plugins.** L'image par défaut embarque Python 3 pour les
  plugins. La variante `-nodejs` ajoute Node.js ; `-full` ajoute PHP, Ruby et
  Lua. N'utilisez une variante que si un plugin que vous installez en a
  besoin.
- **Familles de GPU.** Les images publiées embarquent les pilotes de toutes
  les générations Intel et AMD prises en charge. Une image construite
  soi-même peut n'en garder qu'une avec
  `docker build --build-arg GPU=intel-legacy .` (Intel Gen9 à Gen11),
  `GPU=intel` (Gen12 et plus récent), `GPU=amd` ou `GPU=none`. NVIDIA n'a pas
  besoin de variante : le NVIDIA Container Toolkit injecte le pilote de la
  machine hôte à l'exécution.

## Sandbox des plugins

L'image contient tout le nécessaire pour confiner les plugins, sans mode
privilégié. Le mode de sandbox par défaut (`auto`) choisit le mécanisme le
plus robuste que permet la machine : Landlock sur tout noyau récent. Vérifiez
le mode réellement appliqué sur la carte **Sécurité** d'**Admin → Plugins**.
Une machine qui affiche `basic` ne peut pas isoler l'accès des plugins aux
fichiers : voir
[Mise en place de la sandbox des plugins](../plugins/sandbox-deployment.md)
pour ce que cela implique et ce qu'il faut changer. Les couches de protection
elles-mêmes sont décrites dans
[Modèle de sécurité et sandbox des plugins](../SECURITY.md).
