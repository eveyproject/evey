---
source_sha: 38a4a8e5d613744950c6957edecea4a23ce659feef7f194e3db03f3a2d3fc530
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
