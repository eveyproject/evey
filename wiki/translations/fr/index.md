---
source_sha: 7ece62a6b2bf102990118650fc96a4fc1876d380d696b0eb241572216e67b542
reviewed: true
hide:
  - navigation
  - toc
---
# Evey

Evey est un serveur que vous faites tourner chez vous pour vos films, séries,
musique et photos. Vous ajoutez vos médias et les personnes qui les
regarderont. À chaque lecture, le fichier, le lecteur et le serveur négocient
la façon dont l'image est livrée, sans rien à choisir avant d'appuyer sur
lecture.

[Installer Evey](installation.md){ .md-button .md-button--primary }
[Essayer la démo](https://demo.eveyproject.org){ .md-button }

<p class="evey-status">
<span>Bêta, pas encore de version numérotée</span>
<span>Logiciel libre, AGPL-3.0</span>
<span>Démo : connexion automatique (<code>demo</code> / <code>demo</code>)</span>
</p>

<p class="evey-platforms">
<strong>Ce qu'il vous faut :</strong> un ordinateur ou un NAS x86-64 avec
Docker, un dossier contenant vos médias, et un port libre. Un conteneur, un
fichier compose, une commande.<br>
<strong>Regardez</strong> dans n'importe quel navigateur, sur un Chromecast,
sur les téléphones et tablettes Android, et sur Android TV. Les applications
Android et Android TV sont en phase de test et pas encore sur Google Play.
</p>

<div class="evey-showcase" markdown>

<figure class="evey-shot" markdown="span">
  ![La page d'accueil](images/showcase-home.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>L'accueil : les titres à reprendre, puis vos bibliothèques ligne par ligne.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Une fiche de film](images/showcase-media.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>Une fiche de film : visuels, classification d'âge, notes, synopsis et distribution.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Le lecteur web](images/showcase-player.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>Le lecteur web. Dans la barre : Regarder ensemble, Caster, et le (i) qui ouvre les Infos du flux.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Le tableau de bord d'administration pendant deux lectures](images/showcase-dashboard.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>Le tableau de bord pendant que deux personnes regardent : l'image, l'appareil, Lecture directe ou Remux.</figcaption>
</figure>

</div>

## Pour toute la maison

- **Un profil par personne.** Chaque profil garde son historique, ses points
  de reprise, ses favoris et ses langues ; un foyer partage en général un seul
  compte.
- **Des profils enfant.** Un profil enfant s'ouvre sans code ; en sortir vers
  un profil adulte demande un code PIN à quatre chiffres. Un profil peut avoir
  un âge maximum, appliqué par le serveur : un titre au-dessus n'est jamais
  envoyé, même en téléchargement.
- **Reprendre là où vous en étiez**, sur n'importe quel appareil : les points
  de reprise sont sur le serveur, pas sur le téléphone.
- **En déplacement.** L'application Android télécharge un titre pour le
  regarder hors connexion, pendant 30 jours.
- **Sur la TV.** Android TV se connecte avec la **Connexion rapide** : un code
  affiché à l'écran, tapé ou scanné depuis un téléphone ou un ordinateur déjà
  connecté. Aucun mot de passe à taper à la télécommande.
- **Ensemble.** Plusieurs personnes regardent le même titre au même instant,
  chacune sur son appareil.

Les bibliothèques de musique et de photos sont traitées comme les films et
les séries, sur le web, sur Android et sur la TV.
[Applications et appareils](getting-started/apps.md) ·
[Utilisateurs, profils et contrôle parental](administration/users.md)

## Comment un fichier arrive à l'écran

Evey ne vous demande jamais de choisir une qualité. À chaque lancement d'un
titre, le serveur tranche à partir de trois choses : ce que contient le
fichier, ce que ce lecteur sait décoder, et ce que ce serveur peut encoder à
cet instant. Il préfère, dans cet ordre :

- **Lecture directe** : le fichier tel quel.
- **Remux** : la même image et le même son dans un autre format de fichier,
  rien n'est réencodé. **MKVGo** le fait à l'intérieur du serveur quand il le
  peut, sans lancer un programme de plus à chaque lecture ; sinon FFmpeg copie
  les pistes.
- **Transcodage** : seules les pistes que l'appareil ne sait pas lire sont
  réencodées par FFmpeg, souvent juste le son.

Le même film peut partir intact vers une TV et être réencodé pour un vieux
téléphone. Dans le lecteur, les **Infos du flux** montrent le plan réellement
servi et pourquoi le fichier n'a pas été envoyé tel quel. **Admin → Tableau
de bord** montre la même chose pour chaque spectateur.

<figure class="evey-shot evey-shot--narrow" markdown="span">
  ![Infos du flux dans le lecteur web](images/player-stream-info.webp){ loading=lazy }
  <figcaption>La première lecture du tableau de bord ci-dessus, vue depuis le lecteur. Remux : le format de fichier change, image et son sont copiés, et la dernière ligne dit pourquoi le fichier ne pouvait pas être envoyé tel quel (interface en anglais).</figcaption>
</figure>

[Comment Evey choisit le mode de lecture](playback/how-playback-is-decided.md) ·
[MKVGo dans Evey](playback/mkvgo.md)

## Un seul conteneur, et le serveur se mesure lui-même

Un fichier compose est toute l'installation : aucun `.env` à écrire, aucun
réglage de mémoire ou de processeur.

- **La base de données et le moteur de recherche tournent dans le
  conteneur**, joignables uniquement depuis lui, avec des mots de passe
  générés au premier démarrage.
- **Le serveur ne tourne jamais en root.** Il tourne sous le propriétaire de
  votre dossier de médias quand il peut le détecter, pour que les réparations
  puissent écrire.
- **La capacité de transcodage est mesurée, pas devinée.** Dans un moment
  calme, le serveur chronomètre de vrais encodages sur le processeur et sur
  chaque carte graphique confiée au conteneur ; une carte ne compte que pour
  ce qui a réussi. Quand la place manque, une nouvelle lecture reçoit une
  qualité plus basse, ou passe d'une carte occupée au processeur. Un démarrage
  qui ne peut pas être servi est refusé avec un message qui nomme la raison :
  le serveur est plein, il est encore en train de se mesurer, ou le fichier
  n'est pas prêt.
- **L'accès distant s'ouvre quand vous donnez une adresse publique au
  serveur**, et reste fermé sinon.

Utiliser votre propre base de données ou moteur de recherche reste possible :
[Déploiements avancés](operations/advanced-deployments.md).
[Transcodage et capacité du serveur](playback/transcoding.md)

## Ce qu'Evey fait de vos fichiers

**Vos fichiers.** Retirer un titre ou une bibliothèque le retire d'Evey, pas
du disque. Evey n'écrit dans vos dossiers de médias que pour une réparation
que vous avez demandée, pour un sous-titre récupéré par l'API, et pour un
petit fichier de sonde qui vérifie que la surveillance du dossier fonctionne
encore ; montez une bibliothèque en lecture seule et rien de tout cela ne se
produit. **Santé des médias** lit vos fichiers MKV et WebM et liste ce qu'il
trouve ; une réparation ne réécrit un fichier que sur demande, dans une copie
vérifiée en entier avant de remplacer le fichier, garde l'original par défaut,
et chaque changement est consigné. Ce qui ne peut pas être réparé est signalé
et laissé tel quel.
[Santé des médias](runbooks/media-health.md)

**Vos données.** Le projet ne fait tourner aucun service qui les reçoit :
aucun compte chez nous, aucune mesure d'audience, aucun rapport de plantage.
Les applications parlent à votre serveur et à rien d'autre que nous
exploiterions. Ce qui sort de votre réseau : le serveur demande à des services
extérieurs les visuels et les descriptions (The Movie Database, et des sources
de pochettes pour la musique), et de temps en temps demande au registre
d'images s'il existe une version plus récente, ce que vous pouvez désactiver.
La base de données est sauvegardée avant chaque mise à jour qui la modifie.
[Règles de confidentialité](privacy.md) · [Sauvegardes et mises à jour](backups.md)

## Les plugins sont confinés, pas crus sur parole

Un plugin est analysé avant l'installation ; l'administrateur voit ce que
l'analyse a trouvé et les permissions demandées, et une nouvelle permission
exige un accord explicite. Il tourne comme un processus à part, sous un
utilisateur distinct, sans accès direct au réseau, et il est arrêté dès qu'il
dépasse sa limite de mémoire, de processus ou de disque. La rigueur du
confinement du système de fichiers dépend de la machine, et le **Tableau de
bord sécurité** indique le mode de sandbox réellement utilisé. Les plugins
ajoutent des sources de métadonnées, des canaux de notification, des sections
d'accueil et des surcouches du lecteur ; les thèmes changent l'apparence. Le
widget d'un plugin s'affiche sur le web, sur Android et sur la TV ; sur la
TV, il est affiché, pas manipulé.
[Plugins](plugins.md) ·
[Sandbox des plugins et modèle de menace](SECURITY.md#sandbox-modes)

## Par où commencer

<div class="grid cards" markdown>

-   **[Installer Evey](installation.md)**

    Prérequis, le fichier compose, le premier lancement, l'adresse à ouvrir.

-   **[Première configuration](getting-started/first-setup.md)**

    L'assistant, vos bibliothèques, les personnes qui regarderont, les
    applications.

-   **[Administration](administration/libraries.md)**

    Bibliothèques, utilisateurs, profils et contrôle parental, accès distant,
    carte graphique, sauvegardes.

-   **[Comment Evey choisit le mode de lecture](playback/how-playback-is-decided.md)**

    Lecture directe, Remux, Transcodage, et comment lire les Infos du flux.

-   **[Dépannage](operations/troubleshooting.md)**

    Un tableau par symptôme : où regarder, quoi vérifier, comment récupérer.

-   **[Plugins et API](plugins.md)**

    Installer et écrire des plugins et des thèmes, l'API du serveur.

</div>

<p class="evey-meta">
Cette documentation suit la branche de développement. ·
<a href="https://github.com/Evey-project/evey">Code source sur GitHub</a> ·
Signalements de sécurité : security@eveyproject.org ·
<a href="privacy/">Règles de confidentialité</a>
</p>
