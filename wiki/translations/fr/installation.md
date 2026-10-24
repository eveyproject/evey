---
source_sha: 24e90e68dbbd60f7111af4c4e2e50bf061e8c287d2c22f38fa970231c0d48959
reviewed: true
---
# Installation { #install }

Cette page s'adresse à la personne qui va faire tourner Evey pour son foyer.
Il vous faut un ordinateur ou un NAS qui reste allumé, avec Docker installé.
Cela se fait en trois étapes :

1. Enregistrer un fichier, le fichier compose.
2. Démarrer Evey avec une commande.
3. Ouvrir Evey dans un navigateur.

Ensuite, l'assistant de [Première configuration](getting-started/first-setup.md)
prend le relais.

Evey tourne dans un seul conteneur. Sa base de données et son moteur de
recherche sont dedans : il n'y a rien d'autre à installer, pas de fichier
`.env` à écrire, et aucun réglage de mémoire ou de processeur.

!!! warning "Prenez le fichier de cette page, pas celui du dépôt Git"

    Le `docker-compose.yml` à la racine du dépôt Git est une autre
    installation, pour les développeurs et les installations avancées : il
    demande un fichier `.env` et lance des services en plus. Il est décrit
    dans [Déploiements avancés](operations/advanced-deployments.md).

## Ce qu'il vous faut { #what-you-need }

- **Un ordinateur ou un NAS qui reste allumé**, avec un processeur x86-64
  (Intel ou AMD). Les machines ARM64 ont besoin d'un tag de version, qui
  n'existe pas encore : voir [Machines ARM](operations/advanced-deployments.md#arm-hosts).
- **Docker avec Compose v2.** La commande est `docker compose`, avec une
  espace, et non l'ancienne `docker-compose`.
- **Votre dossier de médias** : là où sont vos films, séries, musique ou
  photos.
- **Un peu de place sur le disque pour les données d'Evey** (base de données,
  images, index de recherche). Le fichier compose ci-dessous les range dans
  un dossier nommé `evey-data`, à côté du fichier.

Evey répond en HTTP simple sur le réseau domestique. Pour regarder depuis
l'extérieur, il faut du HTTPS, que vous pourrez ajouter plus tard :
[Accès distant et HTTPS](administration/remote-access.md).

## Étape 1 : enregistrer le fichier compose { #the-compose-file }

Créez un dossier pour Evey, par exemple `evey`, et enregistrez-y ce fichier
sous le nom `docker-compose.yml`. Changez une seule ligne : le chemin de vos
médias (la ligne marquée 5). Par exemple, si vos films sont dans
`/srv/media`, la ligne devient `- /srv/media:/media`.

```yaml title="docker-compose.yml"
services:
  evey:
    image: eveyproject/evey:latest # (1)!
    container_name: evey-server
    ports:
      - "9091:9091" # (2)!
    volumes:
      - ./evey-data:/data # (3)!
      - evey-cache:/cache # (4)!
      - /path/to/your/media:/media # (5)!
    stop_grace_period: 60s # (6)!
    restart: unless-stopped

volumes:
  evey-cache:
```

1. La version d'Evey. `latest` est la dernière version publiée. Pour rester
   sur une version donnée, utilisez un tag de version comme `0.1.0`, ou un tag
   `sha-<commit>` : voir
   [Sauvegardes et mises à jour](backups.md#updating-evey).
2. Le port. Le nombre de gauche est celui que vous saisirez dans le navigateur
   et dans les applications. Gardez `9091`, sauf si un autre programme
   l'utilise déjà.
3. Les données d'Evey : base de données, clés, mots de passe créés, index de
   recherche, images, journaux, sauvegardes, plugins. **C'est le dossier à
   sauvegarder.**
4. Un cache de vidéo convertie, de photos et de pochettes de musique
   redimensionnées. Vous pouvez le supprimer quand vous voulez : il se
   reconstitue automatiquement. Les variantes d'affiches n'y sont pas : elles
   sont dans `evey-data/artworks`.
5. Vos médias. Remplacez `/path/to/your/media` par le vrai dossier. Dans
   Evey, ce dossier s'appelle `/media`. Evey peut y écrire, et c'est voulu :
   ainsi, la [Santé des médias](runbooks/media-health.md) peut réparer un
   fichier quand vous le demandez. Ajoutez `:ro` à la fin pour le mettre en
   lecture seule : Evey ne peut alors plus y écrire, et les réparations
   échouent.
6. Laisse à la base de données le temps de s'arrêter proprement quand le
   conteneur s'arrête. Les 10 secondes prévues par Docker sont courtes pour
   une grosse collection.

## Étape 2 : démarrer Evey { #first-launch }

Dans un terminal, dans le dossier qui contient le fichier :

```bash
docker compose up -d
```

Evey démarre en arrière-plan. Pour voir ce qu'il fait, lancez
`docker compose logs -f evey` ; appuyez sur `Ctrl+C` pour arrêter de
regarder, Evey continue de tourner.

Au premier démarrage, Evey crée ses mots de passe et sa base de données dans
`evey-data`. Vous n'avez rien à préparer.

## Étape 3 : ouvrir Evey dans un navigateur { #the-address-to-open-next }

L'adresse, c'est `http://`, puis l'adresse de la machine qui fait tourner
Docker, puis `:9091` :

| Où est votre navigateur | Adresse à saisir |
| --- | --- |
| Sur la machine qui fait tourner Docker | `http://localhost:9091` |
| Sur un autre ordinateur ou un téléphone du réseau domestique | `http://` + l'adresse IP de la machine qui fait tourner Docker + `:9091`, par exemple `http://192.168.1.20:9091` |
| Hors de chez vous | Pas encore. Il faut du HTTPS et une adresse publique : voir [Accès distant et HTTPS](administration/remote-access.md). |

Pour trouver l'adresse IP de la machine qui fait tourner Docker, regardez la
liste des appareils connectés dans votre box ou votre routeur. Vous pouvez
aussi lancer `ip a` sous Linux, `ipconfig` sous Windows, `ifconfig` sous
macOS. Si vous avez changé le port à l'étape 1, mettez le nombre de gauche de
la ligne `ports:` à la place de `9091`.

La première fois, l'adresse ouvre l'assistant de configuration. Continuez avec
la [Première configuration](getting-started/first-setup.md#1-the-setup-wizard).
Une fois l'assistant terminé, la même adresse ouvre la page de connexion.

!!! tip "Notez cette adresse"

    Chaque personne de votre foyer en aura besoin. Les applications Android et
    Android TV demandent exactement cette adresse sur leur premier écran,
    avec `http://` et le port :
    [Connecter les applications](getting-started/apps.md#the-server-address-to-enter).

## Où sont vos données { #where-the-data-lives }

| Emplacement | Ce qu'il contient | Pouvez-vous le supprimer ? |
| --- | --- | --- |
| `./evey-data` | Base de données, clés, mots de passe créés, index de recherche, miniatures de la barre de lecture, images, journaux, sauvegardes, plugins | **Non.** Le perdre, c'est perdre tous les comptes, l'historique de visionnage et les réglages. Seule exception : `meili/` à l'intérieur, l'index de recherche. La commande `evey admin reindex-search` le reconstruit à partir de la base de données. |
| Volume `evey-cache` | Vidéo convertie, photos et pochettes de musique redimensionnées (les variantes d'affiches sont dans `evey-data/artworks`) | Oui. Il se reconstitue automatiquement. |
| Vos dossiers de médias | Vos fichiers | Evey ne les supprime jamais. Retirer un titre dans Evey le retire de la bibliothèque, pas du disque. |

Evey écrit dans vos dossiers de médias dans trois cas seulement :

- une réparation que vous lancez depuis **Santé des médias** (l'original est
  gardé en copie de secours) ;
- un sous-titre téléchargé depuis OpenSubtitles (rangé dans un dossier
  `.subtitles` à côté du film) ;
- un petit fichier de test, supprimé tout de suite, qui vérifie qu'Evey
  remarque toujours les nouveaux fichiers.

## Changements courants { #common-variations }

Chacun tient en quelques lignes dans le même fichier.

**Un deuxième dossier de médias.** Ajoutez `- /other/path:/media2` sous
`volumes:`. Un troisième devient `/media3`, et ainsi de suite.

**Une deuxième instance d'Evey sur la même machine.** Donnez-lui un autre
`container_name` et d'autres ports côté hôte, sinon `docker compose up`
refuse de démarrer (le nom est déjà utilisé).

**Un autre port.** Changez seulement le nombre de gauche, par exemple
`"8080:9091"`. Saisissez ensuite `:8080` dans le navigateur et dans les
applications.

**À qui appartiennent les fichiers.** Par défaut, Evey prend le propriétaire
de votre dossier de médias, pour que les réparations puissent écrire. Pour
choisir vous-même, ajoutez `PUID` et `PGID` sous `environment:`, avec les
nombres donnés par `id -u` et `id -g`. Le conteneur démarre en root
seulement pour appliquer ce choix ; Evey lui-même ne tourne jamais en root.

**Une carte graphique pour convertir la vidéo.** Donnez au conteneur l'accès
à la carte : voir [Accélération matérielle](administration/hardware-acceleration.md).

**La version que vous faites tourner.** La ligne `image:` en décide :

| `image:` | Ce que vous obtenez |
|---|---|
| `eveyproject/evey:latest` | la dernière version publiée ; chaque `docker compose pull` vous fait passer à la suivante |
| `eveyproject/evey:0.1` | le dernier correctif de cette ligne mineure (0.1.0, 0.1.1, ...), jamais une nouvelle version mineure |
| `eveyproject/evey:0.1.0` | exactement cette version, tant que vous ne changez pas la ligne |
| `eveyproject/evey:sha-<commit>` | un build précis, le seul tag qui ne bouge jamais |
| `eveyproject/evey:master` | la branche de développement, reconstruite à chaque push : pour tester, pas pour un serveur sur lequel vous comptez |

Les numéros de version suivent le [versionnage sémantique](https://semver.org/) et commencent à
`0.1.0` ; la ligne 0.x est la bêta publique. Pour changer de version, modifiez la
ligne, puis `docker compose pull && docker compose up -d` ; vos données restent
dans `./evey-data`. La version en cours s'affiche dans **Administration → Tableau de bord**, sur
`/healthz`, et avec `docker exec evey-server evey version`. Un tag sans
suffixe est l'image complète, avec tous les environnements d'exécution de plugins
(Python, Node.js, PHP, Ruby et Lua) ; `-nodejs` et `-lean` sont des builds plus
légers qui en contiennent moins
([détails](operations/advanced-deployments.md#image-tags-and-variants)).

**Vous avez déjà un reverse proxy** (un programme qui ajoute le HTTPS devant
vos services) : voir [Accès distant et HTTPS](administration/remote-access.md).

??? note "Sur un NAS (Synology Container Manager, QNAP Container Station)"

    - Créez le dossier `evey-data` avant le premier démarrage. Ces interfaces
      refusent de monter un dossier qui n'existe pas, contrairement à la
      ligne de commande Docker. Rien d'autre n'est à créer.
    - Mettez dans `PUID` et `PGID` l'utilisateur du NAS à qui appartiennent
      les médias (`id <utilisateur>` en SSH ; sur Synology, en général autour
      de `1026`, groupe `100`). Les partages d'un NAS appartiennent souvent à
      `root`, qu'Evey ne prend jamais de lui-même.
    - NAS Intel avec Quick Sync : ajoutez les lignes `devices:` et
      `group_add:` décrites dans
      [Accélération matérielle](administration/hardware-acceleration.md).

## Ce qu'Evey règle de lui-même { #what-evey-sets-up-by-itself }

Vous n'avez pas à configurer :

- **Combien de vidéos le serveur peut convertir en même temps.** Il mesure la
  machine dès son premier moment calme.
- **La carte graphique.** Evey détecte la carte que vous avez donnée au
  conteneur et la teste avec une vraie conversion.
- **Comment chaque appareil lit un fichier.** C'est décidé à chaque lecture :
  voir [Comment Evey choisit le mode de lecture](playback/how-playback-is-decided.md).
- **L'accès depuis l'extérieur de chez vous.** Fermé tant que vous ne donnez
  pas d'adresse publique au serveur.

Les réglages que vous voudrez peut-être changer plus tard (nom du serveur,
horaires des tâches, limites, clés d'API) sont dans le panneau
d'administration, pas dans le fichier compose. Les quelques variables
d'environnement qui existent sont listées dans la
[Référence de configuration](configuration.md).
