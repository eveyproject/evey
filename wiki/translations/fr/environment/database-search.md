---
source_sha: c19f05aea8bece6fa3bf10a82d5590583384092dde0102fcdd2192bff8c0c547
reviewed: true
---
# Base de données et recherche

Par défaut, l'image fait tourner son propre Postgres et son propre
Meilisearch, et génère leurs mots de passe : aucune de ces variables n'est
nécessaire. Elles servent à une installation qui apporte sa propre base ou son
propre moteur de recherche, ou qui ajuste ceux qui sont intégrés. Voir
[Déploiements avancés](../operations/advanced-deployments.md).

## Base de données

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_EMBEDDED_POSTGRES` | non définie | Non définie : base intégrée si `EVEY_DB_HOST` n'est pas définie, externe sinon. `true` ou `false` pour décider explicitement. Lue par l'entrypoint. |
| `EVEY_POSTGRES_DATA_DIR` | `/data/postgres` | L'emplacement de la base intégrée. Il doit se trouver sur un système de fichiers local, pas sur un partage réseau. |
| `EVEY_DB_HOST` | `localhost` | L'hôte de la base. **La définir désactive la base intégrée.** |
| `EVEY_DB_PORT` | `5432` | Le port de la base. |
| `EVEY_DB_USER` | `evey` | L'utilisateur de la base. |
| `EVEY_DB_NAME` | `evey` | Le nom de la base. |
| `EVEY_DB_PASSWORD` | générée | Le mot de passe. Avec une base externe, le serveur refuse de démarrer sur la valeur `evey`. |
| `EVEY_DB_PASSWORD_FILE` | `/data/keys/postgres.password` | Un fichier contenant le mot de passe, lu quand `EVEY_DB_PASSWORD` n'est pas définie. Définie par l'image. |
| `EVEY_DB_SSLMODE` | `disable` en local, `prefer` sinon | `disable`, `prefer`, `require` ou `verify-full`. |
| `EVEY_DB_MAX_OPEN_CONNS` | `25` | Connexions ouvertes vers la base. Chacune coûte de la mémoire côté Postgres ; ne l'augmentez que pour un gros déploiement. |
| `EVEY_DB_MAX_IDLE_CONNS` | `10` | Connexions gardées inactives dans le pool. |

## Recherche

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_EMBEDDED_MEILI` | `true` | `false` utilise le Meilisearch de `EVEY_MEILI_URL` à la place de celui qui est intégré. Lue par l'entrypoint. |
| `EVEY_MEILI_URL` | `http://localhost:7700` | L'adresse de Meilisearch. |
| `MEILI_MASTER_KEY` | générée | La clé de Meilisearch. Avec une instance externe, une clé vide ou factice empêche le démarrage. |
| `MEILI_MASTER_KEY_FILE` | `/data/keys/meili.key` | Un fichier contenant la clé, lu quand `MEILI_MASTER_KEY` n'est pas définie. Définie par l'image. |
| `MEILI_MAX_INDEXING_MEMORY` | environ 20 % de la mémoire du conteneur, au moins 128 Mio | La mémoire que le Meilisearch intégré peut utiliser pour l'indexation. Calculée d'après la limite du conteneur, pas d'après la mémoire de la machine, pour qu'un scan n'épuise pas un petit conteneur. Une valeur explicite est transmise telle quelle. |
