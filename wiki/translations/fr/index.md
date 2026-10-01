---
source_sha: 5cde466e14206b2a29398343ce7b115ae0b096216b8e6d056f834cf4b7763057
reviewed: true
hide:
  - navigation
  - toc
---

# Documentation d'Evey

Evey est un serveur auto-hébergé pour vos films, séries, musique et photos. À
chaque lecture, il confronte le fichier, le lecteur et le serveur, garde
l'original dès que le lecteur sait l'exploiter, et ne réencode que ce qui doit
l'être. La plupart des installations n'ont besoin que de bibliothèques et
d'utilisateurs : le matériel, la capacité et les routes de lecture sont
détectés et mesurés par le serveur lui-même.

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

<div class="evey-points" markdown>

<div markdown>

**La voie la moins coûteuse d'abord**

Le fichier tel quel, sinon réempaqueté sans encodeur, sinon seules les pistes
qui l'exigent sont réencodées. Un spectateur en lecture directe ou en remux ne
coûte guère plus qu'une lecture de fichier : une machine modeste sert beaucoup
de monde à la fois.
[Comment Evey choisit le mode de lecture](playback/how-playback-is-decided.md)

</div>

<div markdown>

**Un serveur qui se mesure lui-même**

Il chronomètre de vrais encodages 4K sur le processeur et chaque carte
graphique, et n'utilise une carte que pour ce qu'elle a prouvé. Aucune
capacité à estimer.
[Transcodage et capacité du serveur](playback/transcoding.md)

</div>

<div markdown>

**Plein veut dire refusé, pas ralenti**

Pas plus d'encodages que mesuré ; une fois plein, le lecteur est prévenu que le
serveur est occupé, au lieu de dégrader tous les flux. Les tâches de fond se
mettent en pause pendant la lecture.
[Tâches de fond et charge du serveur](operations/background-work.md)

</div>

<div markdown>

**Un plugin, trois écrans**

Le même paquet tourne sur le web, sur Android et sur Android TV, et sait sur
quel écran il s'affiche.
[Référence de l'API des plugins](plugins/api-reference.md#on-a-television)

</div>

<div markdown>

**Des plugins confinés, pas crus sur parole**

Code analysé avant l'installation, son seul dossier visible, appels système
dangereux refusés, aucun accès direct au réseau, jeton refusé par défaut.
[Sandbox des plugins et modèle de menace](SECURITY.md)

</div>

<div markdown>

**Les Infos du flux disent ce qui tourne vraiment**

Une seule décision par lecture, et les Infos du flux la lisent dans le plan
qu'exécute l'encodeur, avec la raison pour laquelle le fichier n'est pas lu tel quel.
[Comment Evey choisit le mode de lecture](playback/how-playback-is-decided.md#reading-stream-info)

</div>

</div>

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
