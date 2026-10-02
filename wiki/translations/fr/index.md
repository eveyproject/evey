---
source_sha: 519ca471ac509567eb54bbd0ba3bd684995c10632c38c0b067d3b6546b0d6b29
reviewed: true
hide:
  - navigation
  - toc
---

# Evey

Evey est un serveur auto-hébergé pour vos films, séries, musique et photos. Il
se veut un appareil plutôt qu'un système à administrer : vous lui donnez vos
médias et les personnes qui regardent, il déduit le reste de la machine sur
laquelle il tourne.

[Installer Evey](installation.md){ .md-button .md-button--primary }
[Essayer la démo](https://demo.eveyproject.org){ .md-button }

## Faire tourner un serveur multimédia, pas son infrastructure

Une installation normale, c'est un conteneur et un fichier compose, sans
`.env` à écrire. Ce dont un serveur multimédia a besoin autour de lui est
fourni à l'intérieur :

- **Base de données et moteur de recherche inclus.** Postgres et Meilisearch
  tournent dans le même conteneur, n'écoutent qu'à l'intérieur de celui-ci, et
  reçoivent au premier démarrage des mots de passe générés, rangés dans le
  dossier de données et lisibles par leur seul propriétaire.
- **Propriétaire des fichiers déduit.** Quand c'est possible, le conteneur
  adopte le propriétaire de votre dossier de médias, pour que les réparations
  puissent écrire. Le serveur lui-même ne tourne jamais en root.
- **Moins à dimensionner à la main.** La carte graphique est détectée, la
  capacité de transcodage est mesurée, et la voie de chaque lecture suit ce que
  l'appareil dit savoir lire.

Utiliser votre propre base de données ou votre propre moteur de recherche
reste possible : [Déploiements avancés](operations/advanced-deployments.md).

<div class="evey-showcase" markdown>

<figure class="evey-shot" markdown="span">
  ![L'accueil](images/showcase-home.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>L'accueil : reprendre la lecture, puis chaque bibliothèque.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![La page d'un film](images/showcase-media.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>La page d'un film, avec ses visuels, ses notes et sa distribution.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Le lecteur web](images/showcase-player.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>Le lecteur web.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Le tableau de bord d'administration pendant deux lectures](images/showcase-dashboard.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>Le tableau de bord pendant que deux personnes regardent : l'image que voit chacune, et la façon dont elle est servie.</figcaption>
</figure>

</div>

## Comment il fonctionne

<div class="evey-points" markdown>

<div markdown>

**En faire le moins possible**

D'abord le fichier tel quel ; sinon les mêmes pistes réempaquetées, sans
encodeur ; un réencodage seulement pour les pistes qui l'exigent. Le
réempaquetage se fait dans le serveur quand c'est possible, sans lancer
FFmpeg : un spectateur en lecture directe ou en remux ne coûte guère plus
qu'une lecture de fichier.
[Comment Evey choisit le mode de lecture](playback/how-playback-is-decided.md)

</div>

<div markdown>

**Savoir ce dont la machine est vraiment capable**

Un accélérateur ne compte qu'une fois qu'un vrai encodage y a réussi. À son
premier moment calme, le serveur chronomètre de vrais encodages à partir d'un
extrait 4K, jusqu'en 1440p sur le processeur et jusqu'en 2160p sur chaque
accélérateur, puis envoie chaque flux vers la voie la moins coûteuse qui a
prouvé pouvoir le servir. Quand la place manque, un nouveau flux reçoit une
qualité plus basse, ou passe d'une carte graphique pleine au processeur ; il
n'est refusé, avec un message clair, que lorsqu'il ne reste plus aucune
capacité.
[Transcodage et capacité du serveur](playback/transcoding.md)

</div>

<div markdown>

**Du code tiers confiné, pas cru sur parole**

Un plugin est analysé avant l'installation, ne reçoit que les permissions
déclarées et acceptées par l'administrateur, et tourne dans son propre
processus, sans accès direct au réseau, avec mémoire, processus et disque
plafonnés. Sous Docker avec un noyau récent, il ne voit que son propre dossier,
et une liste de refus seccomp bloque les appels système connus pour être
dangereux ; un hôte plus ancien a une sandbox plus faible, et le panneau
d'administration indique laquelle est active.
[Sandbox des plugins et modèle de menace](SECURITY.md#sandbox-modes)

</div>

<div markdown>

**Trouver les médias abîmés avant la lecture**

La **Santé des médias** lit vos fichiers MKV et WebM et explique ce qu'elle
trouve. Ce qu'elle sait réparer (un index de lecture manquant, un MP4 nommé
`.mkv`, des pistes rangées par blocs) est réparé à la demande, vérifié avant
de remplacer le fichier, avec l'original gardé par défaut et chaque
modification consignée au journal. Le reste est signalé, jamais réécrit dans
votre dos.
[Santé des médias](runbooks/media-health.md)

</div>

</div>

## Voir ce qu'Evey a décidé, et pourquoi

Le tableau de bord ci-dessus liste chaque lecture : qui, sur quel appareil, en
**Lecture directe**, **Remux** ou **Transcodage**, si la vidéo et l'audio sont
copiés ou encodés et par quoi, le débit et la progression. Dans le lecteur, les
**Infos du flux** lisent le plan même qu'exécute l'encodeur, et disent pourquoi
le fichier n'a pas été lu tel quel.

<figure class="evey-shot evey-shot--narrow" markdown="span">
  ![Les Infos du flux dans le lecteur web](images/player-stream-info.webp){ loading=lazy }
  <figcaption>Les Infos du flux d'un film en remux : vidéo et audio copiés, et la raison pour laquelle il n'est pas lu directement.</figcaption>
</figure>

[Lire les Infos du flux](playback/how-playback-is-decided.md#reading-stream-info)

## Applications et extensions

Une application web servie par le serveur lui-même, des applications Android
et Android TV (connectée depuis un téléphone ou un ordinateur grâce à la
**Connexion rapide**), et la diffusion vers un Chromecast. Plugins et thèmes
étendent l'interface, et le même plugin tourne sur le web, sur Android et sur
la TV.
[Applications et appareils](getting-started/apps.md) ·
[Plugins](plugins.md)

## Par où commencer

<div class="grid cards" markdown>

-   **[Installer Evey](installation.md)**

    Un conteneur, un fichier compose. Prérequis et premier démarrage.

-   **[Première configuration](getting-started/first-setup.md)**

    L'assistant de configuration, vos bibliothèques, les personnes qui
    utiliseront le serveur.

-   **[Administration](administration/libraries.md)**

    Bibliothèques, utilisateurs et profils, accès distant, accélération
    matérielle, sauvegardes.

-   **[Comment Evey choisit le mode de lecture](playback/how-playback-is-decided.md)**

    Lecture directe, remux ou transcodage, et comment lire les Infos du flux.

-   **[Dépannage](operations/troubleshooting.md)**

    Où regarder quand quelque chose ne marche pas : symptômes, logs,
    restauration.

-   **[Plugins et API](plugins.md)**

    Installer et écrire des plugins et des thèmes, l'API du serveur.

</div>

<p class="evey-meta">
Evey est en bêta et n'a pas encore de version numérotée : cette documentation
suit la branche de développement. ·
<a href="https://github.com/Evey-project/evey">Code source sur GitHub</a> ·
<a href="https://demo.eveyproject.org">Démo publique</a> (identifiants demo / demo) ·
AGPL-3.0-or-later ·
Signalement de failles : security@eveyproject.org
</p>
