---
source_sha: c2adfbf17677a38b130ae3027ed7811d41b5a424dee85626ca7f0532956b91f6
reviewed: true
hide:
  - navigation
  - toc
---

# Evey

Evey est un serveur auto-hébergé pour vos films, séries, musique et photos.
Vous ajoutez vos médias et vos utilisateurs ; Evey adapte automatiquement la
lecture à votre serveur et à vos appareils.

[Installer Evey](installation.md){ .md-button .md-button--primary }
[Essayer la démo](https://demo.eveyproject.org){ .md-button }

<p class="evey-status">
<span>Bêta, pas encore de version numérotée</span>
<span>Logiciel libre, AGPL-3.0</span>
<span>Démo : <code>demo</code> / <code>demo</code></span>
</p>

<p class="evey-platforms">
<strong>Regarder</strong> dans n'importe quel navigateur web, sur un Chromecast,
sur les téléphones et tablettes Android, et sur Android TV. Les applications
Android et Android TV sont en test et ne sont pas encore sur Google Play.<br>
<strong>Serveur :</strong> un conteneur Docker sur un ordinateur ou un NAS
x86-64.
</p>

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
  <figcaption>Le tableau de bord pendant que deux personnes regardent : l'image que voit chacune, et la façon dont elle lui est envoyée.</figcaption>
</figure>

</div>

## Gérer vos médias, pas l'infrastructure

Un conteneur, un fichier compose, aucun `.env` à écrire.

- **Base de données et moteur de recherche inclus.** Ils ne sont joignables
  que depuis le conteneur, avec des mots de passe générés au premier
  démarrage.
- **Permissions des fichiers adaptées automatiquement**, quand c'est possible,
  pour que les réparations puissent écrire. Le serveur lui-même ne tourne
  jamais en root.
- **Moins de réglages à faire à la main.** La carte graphique est détectée, la
  capacité de transcodage est mesurée, et le mode de lecture dépend de ce que
  chaque appareil sait lire.

Vous pouvez toujours utiliser votre propre base de données ou votre propre
moteur de recherche : [Déploiements avancés](operations/advanced-deployments.md).

## Comment il fonctionne

<div class="evey-points" markdown>

<div markdown>

**En faire le moins possible**

La **Lecture directe** envoie le fichier tel quel. Quand l'appareil sait lire
les pistes mais pas le conteneur du fichier, **MKVGo** change le conteneur sans
réencoder les pistes audio et vidéo, et sans lancer un programme supplémentaire pour chaque lecture ; là où
MKVGo ne peut pas, FFmpeg copie les pistes. **FFmpeg** se charge des vraies
conversions.
[MKVGo dans Evey](playback/mkvgo.md)

</div>

<div markdown>

**Savoir ce dont la machine est vraiment capable**

Un accélérateur ne compte qu'une fois qu'un vrai encodage y a réussi. Dans un
moment calme, le serveur chronomètre des encodages d'essai, puis confie chaque
flux au matériel le moins coûteux qui a prouvé qu'il pouvait le transcoder.
Quand la capacité manque, un nouveau flux reçoit une qualité plus basse, ou
passe d'une carte graphique saturée au processeur ; il n'est refusé, avec un
message clair, que s'il ne reste plus aucune capacité.
[Transcodage et capacité du serveur](playback/transcoding.md)

</div>

<div markdown>

**Du code tiers confiné, pas cru sur parole**

Un plugin est analysé avant l'installation, ne reçoit que les permissions
acceptées par l'administrateur, et tourne dans son propre processus, sans
accès direct au réseau et avec des ressources plafonnées. La rigueur de la
sandbox dépend de l'hôte, et le panneau d'administration indique laquelle est
active.
[Sandbox des plugins et modèle de menace](SECURITY.md#sandbox-modes)

</div>

<div markdown>

**Repérer les médias abîmés avant la lecture**

La **Santé des médias** analyse vos fichiers MKV et WebM et explique ce
qu'elle trouve. Une réparation se lance à la demande, est vérifiée avant de
remplacer le fichier, garde l'original par défaut et est consignée au journal.
Le reste est signalé, jamais réécrit dans votre dos.
[Santé des médias](runbooks/media-health.md)

</div>

</div>

## Voir ce qu'Evey a décidé, et pourquoi

Le tableau de bord liste chaque lecture : qui regarde, sur quel appareil, en
**Lecture directe**, **Remux** ou **Transcodage**, et ce qui est copié ou
réencodé. Dans le lecteur, les **Infos du flux** montrent ce qui est
réellement envoyé, et pourquoi le fichier n'a pas été lu tel quel.

<figure class="evey-shot evey-shot--narrow" markdown="span">
  ![Les Infos du flux dans le lecteur web](images/player-stream-info.webp){ loading=lazy }
  <figcaption>Remux : le conteneur change, sans réencoder la vidéo ni l'audio. « Copié » décrit les pistes, pas le mode de lecture.</figcaption>
</figure>

[Lire les Infos du flux](playback/how-playback-is-decided.md#reading-stream-info)

## Applications et extensions

Une TV se connecte depuis un téléphone ou un ordinateur grâce à la
**Connexion rapide**, sans saisir de mot de passe. Plugins et thèmes étendent
l'interface, et un même plugin fonctionne sur le web, sur Android et sur la
TV.
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
Cette documentation suit la branche de développement. ·
<a href="https://github.com/Evey-project/evey">Code source sur GitHub</a> ·
Signalement de failles : security@eveyproject.org
</p>
