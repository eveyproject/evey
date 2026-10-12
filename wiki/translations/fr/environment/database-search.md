---
source_sha: 6efce4118fc9a8bfa56a70e2648cc5f619c415c257832c5a9ea6da0e9e98e645
reviewed: true
---
# Base de données et recherche

Les variables de la base Postgres et de l'index Meilisearch. Par défaut,
l'image fait tourner les deux elle-même et génère leurs mots de passe : une
installation standard n'en définit aucune. Elles servent à une installation
qui apporte sa propre base ou son propre moteur de recherche, ou qui ajuste
ceux qui sont intégrés ; les procédures sont dans
[Déploiements avancés](../operations/advanced-deployments.md). Aucune n'a
d'équivalent dans le panneau.

## Base de données

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_EMBEDDED_POSTGRES` | non définie | Non définie : base intégrée si `EVEY_DB_HOST` n'est pas définie, externe sinon. `true` ou `false` pour décider explicitement. Lue par l'entrypoint. | aucun |
| `EVEY_POSTGRES_DATA_DIR` | `/data/postgres` | L'emplacement de la base intégrée. Il doit se trouver sur un système de fichiers local, pas sur un partage réseau. Lue par l'entrypoint. | aucun |
| `EVEY_POSTGRES_IMPORT` | `/data/postgres-import.sql.gz` | Un `pg_dump` compressé restauré dans la base intégrée à sa première création, puis renommé `*.imported-<date>`. Ignoré, avec un message, quand la base existe déjà. Lue par l'entrypoint. | aucun |
| `EVEY_DB_HOST` | `localhost` | L'hôte de la base. **La définir désactive la base intégrée.** | aucun |
| `EVEY_DB_PORT` | `5432` | Le port de la base. | aucun |
| `EVEY_DB_USER` | `evey` | Le compte de connexion à la base. Quand c'est un superutilisateur (base intégrée et compose fourni), le serveur ne le garde que pour la restauration et tourne sous `EVEY_DB_APP_USER`. | aucun |
| `EVEY_DB_APP_USER` | `evey_app` | Le rôle sans privilège sous lequel tournent le serveur et ses migrations quand `EVEY_DB_USER` est un superutilisateur. Créé au démarrage, il reçoit la base et ses objets, et un nouveau mot de passe aléatoire à chaque démarrage (jamais écrit nulle part). Une injection SQL ne peut alors ni lancer de programme ni lire de fichier sur l'hôte. | aucun |
| `EVEY_DB_PRIVILEGE_SPLIT` | `true` | `false` fait tout tourner sous `EVEY_DB_USER`, superutilisateur compris. Un recours si une base refuse la séparation ; sinon le démarrage s'arrête en nommant cette variable. | aucun |
| `EVEY_DB_NAME` | `evey` | Le nom de la base. | aucun |
| `EVEY_DB_PASSWORD` | générée | Le mot de passe. Avec une base externe, le serveur refuse de démarrer sur la valeur `evey`, et avertit sur `changeme`. | aucun |
| `EVEY_DB_PASSWORD_FILE` | `/data/keys/postgres.password` | Un fichier contenant le mot de passe, lu quand `EVEY_DB_PASSWORD` n'est pas définie. Définie par l'image. | aucun |
| `EVEY_DB_SSLMODE` | `disable` en local, `prefer` sinon | `disable`, `prefer`, `require` ou `verify-full`. `prefer` vers un hôte distant produit un avertissement : utilisez `require` ou `verify-full`. | aucun |
| `EVEY_DB_MAX_OPEN_CONNS` | `25` | Connexions ouvertes vers la base. Chacune coûte de la mémoire côté Postgres ; ne l'augmentez que pour un gros déploiement. | aucun |
| `EVEY_DB_MAX_IDLE_CONNS` | `10` | Connexions gardées inactives dans le pool. Plafonnée au nombre de connexions ouvertes. | aucun |

## Recherche

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_EMBEDDED_MEILI` | `true` | `false` utilise le Meilisearch de `EVEY_MEILI_URL` à la place de celui qui est intégré. Lue par l'entrypoint. | aucun |
| `EVEY_MEILI_URL` | `http://localhost:7700` | L'adresse de Meilisearch. | aucun |
| `MEILI_MASTER_KEY` | générée | La clé de Meilisearch. Avec une instance externe, une clé vide ou factice empêche le démarrage ; une clé de moins de 16 caractères produit un avertissement. | aucun |
| `MEILI_MASTER_KEY_FILE` | `/data/keys/meili.key` | Un fichier contenant la clé, lu quand `MEILI_MASTER_KEY` n'est pas définie. Définie par l'image. | aucun |
| `MEILI_DATA_DIR` | `/data/meili` | L'emplacement de l'index intégré. Lue par l'entrypoint. | aucun |
| `MEILI_MAX_INDEXING_MEMORY` | environ 20 % de la mémoire du conteneur, au moins 128 Mio | La mémoire que le Meilisearch intégré peut utiliser pour l'indexation. Calculée d'après la limite du conteneur, pas d'après la mémoire de la machine, pour qu'un scan n'épuise pas un petit conteneur. Une valeur explicite est transmise telle quelle. | aucun |
