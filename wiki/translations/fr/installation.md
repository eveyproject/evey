---
source_sha: a6c61f9f7b1e619f46c4d5eea7ca2c2b93356f93c04681e95963303bdf287723
reviewed: true
---
# Installation

Evey tient dans un seul conteneur. L'image sert l'API et l'interface web sur
un même port, et fait tourner sa propre base de données (Postgres) et son
moteur de recherche (Meilisearch), dont les mots de passe sont générés au
premier démarrage. Le fichier compose ci-dessous constitue toute
l'installation : pas de `.env`, pas de mode privilégié, aucun réglage de
mémoire ou de CPU.

!!! warning "Pas le docker-compose.yml du dépôt"

    Le `docker-compose.yml` à la racine du dépôt Git correspond à une autre
    disposition (Postgres en service séparé, Caddy intégré, `.env`
    obligatoire). Utilisez le fichier de cette page. L'autre disposition est
    décrite dans [Déploiements avancés](operations/advanced-deployments.md).

## Prérequis

- Docker avec Compose v2 (`docker compose`, et non l'ancien `docker-compose`).
- Un disque persistant pour les données d'Evey (le fichier compose ci-dessous
  les place dans `./evey-data`, à côté du fichier).
- Une machine x86-64, ou ARM64 avec un tag de version : voir
  [Machines ARM](operations/advanced-deployments.md#arm-hosts).

Evey sert du HTTP simple. Pour y accéder depuis l'extérieur en HTTPS, vous
placerez plus tard un reverse proxy devant : voir
[Accès distant et HTTPS](administration/remote-access.md).

## Le fichier compose

Enregistrez ce contenu sous `docker-compose.yml` et remplacez le chemin de vos
médias sur la dernière ligne de `volumes`.

```yaml title="docker-compose.yml"
services:
  evey:
    image: gravityz/evey:latest # (1)!
    container_name: evey-server
    ports:
      - "9091:9091" # (2)!
    volumes:
      - ./evey-data:/data # (3)!
      - evey-cache:/cache # (4)!
      - /chemin/vers/vos/medias:/media # (5)!
    stop_grace_period: 60s # (6)!
    restart: unless-stopped

volumes:
  evey-cache:
```

1. `latest` désigne la dernière version stable. Figez un tag dès que le
   serveur est utilisé pour de bon, pour que deux `pull` à un jour d'écart exécutent
   le même code : voir [Sauvegardes et mises à jour](backups.md#updating-evey).
2. L'API et l'interface web, sur un seul port, en HTTP simple.
3. Tout ce qui appartient à Evey : base de données, clé de signature, mots de
   passe générés, index de recherche, miniatures de navigation, visuels, logs,
   sauvegardes, plugins. C'est le dossier à sauvegarder.
4. Le cache de transcodage : segments, photos et pochettes redimensionnées. Il
   peut être supprimé à tout moment et se reconstitue à la demande.
5. Vos médias. Montés en lecture-écriture volontairement : les réparations de
   la [Santé des médias](runbooks/media-health.md) réécrivent les fichiers sur
   place. Ajoutez `:ro` pour une bibliothèque en lecture seule, et ces
   réparations échoueront. D'autres dossiers se montent en `/media2`,
   `/media3`...
6. Laisse à la base intégrée le temps de s'arrêter proprement lors d'un
   `docker stop`. Les 10 secondes par défaut de Docker sont courtes pour un
   gros catalogue.

## Premier démarrage

```bash
docker compose up -d
docker compose logs -f evey
```

Au premier démarrage, le serveur génère sa clé de signature et ses mots de
passe dans `./evey-data`, crée la base de données et applique ses migrations.
Rien n'est à préparer à la main.

Ouvrez ensuite `http://<adresse-du-serveur>:9091`. Un serveur sans
administrateur s'ouvre sur l'assistant de configuration : poursuivez avec la
[Première configuration](getting-started/first-setup.md).

## Où sont les données

| Emplacement | Contenu | Peut-on le supprimer ? |
| --- | --- | --- |
| `./evey-data` | Base de données, clé de signature, mots de passe générés, index de recherche, miniatures de navigation, visuels, logs, sauvegardes, plugins | Non : vous perdriez les comptes, l'historique de lecture et les réglages. Seul `meili/` peut partir : l'index de recherche se reconstruit depuis la base avec `evey admin reindex-search`. |
| Volume `evey-cache` | Segments transcodés, photos et pochettes redimensionnées | Oui. Tout se régénère à la demande. |
| Vos dossiers de médias | Vos fichiers | Evey ne les supprime pas : retirer un titre dans Evey le retire de la bibliothèque, pas du disque. Il y écrit dans trois cas : une réparation lancée depuis la Santé des médias (l'original est gardé en copie de secours), les sous-titres téléchargés depuis OpenSubtitles (un dossier `.subtitles` à côté du film), et un fichier de test de quelques secondes qui vérifie que la surveillance du dossier reçoit toujours les événements. |

## Variantes courantes

Chacune tient en quelques lignes ajoutées au même fichier.

**Un second dossier de médias.** Ajoutez `- /autre/chemin:/media2` sous
`volumes:`.

**Propriétaire des fichiers.** Par défaut, le conteneur adopte le
propriétaire de votre montage `/media`, pour que les réparations puissent
écrire. Pour le fixer vous-même, renseignez `PUID` et `PGID` sous
`environment:` (le résultat de `id -u` et `id -g`). Le conteneur démarre en
root uniquement pour appliquer ce changement ; le serveur lui-même ne tourne
jamais en root.

**Une carte graphique pour le transcodage.** Donnez le périphérique au
conteneur : voir [Accélération matérielle](administration/hardware-acceleration.md).

**Derrière un reverse proxy existant.** Voir
[Accès distant et HTTPS](administration/remote-access.md).

??? note "Sur un NAS (Synology Container Manager, QNAP Container Station)"

    - Créez le dossier `evey-data` avant le premier démarrage. Ces interfaces
      refusent un montage dont le dossier n'existe pas, contrairement à la
      ligne de commande Docker. Rien d'autre n'est à créer.
    - Renseignez `PUID` et `PGID` avec l'utilisateur du NAS propriétaire des
      médias (`id <utilisateur>` en SSH ; sur Synology, généralement autour de
      `1026`, groupe `100`). Un partage de NAS appartient souvent à `root`, que
      la détection automatique n'adopte jamais.
    - NAS Intel avec Quick Sync : ajoutez les lignes `devices:` et
      `group_add:` décrites dans
      [Accélération matérielle](administration/hardware-acceleration.md).

## Ce que vous n'avez pas à configurer

La capacité de transcodage, la carte graphique, le mode de lecture adapté à
chaque appareil et l'accès distant sont décidés par le serveur : il mesure la
machine dès qu'elle est au repos, détecte le matériel que vous lui avez
donné, et ouvre l'accès distant quand vous lui fournissez une adresse
publique. Les réglages susceptibles de changer (nom du serveur,
planifications, limites, clés d'API) se trouvent dans le panneau
d'administration, pas dans le fichier compose. Les variables
d'environnement qui existent sont listées dans la
[Référence de configuration](configuration.md).
