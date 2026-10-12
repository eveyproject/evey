---
source_sha: 8b5109c0af499483f4c9038e9c785a28ed41a147962487df2de2848bd91fe88b
reviewed: true
hide:
- navigation
- toc
---

# Evey { #evey }

Evey est un serveur de médias que vous installez chez vous. Il garde vos
films, séries, musique et photos, et chaque personne de votre foyer les regarde
sur ses propres appareils. Vous ne choisissez jamais une qualité : vous
appuyez sur lecture, Evey s'occupe du reste.

[Installer Evey](installation.md){ .md-button .md-button--primary }
[Essayer la démo](https://demo.eveyproject.org){ .md-button }

<p class="evey-status">
<span>Bêta, pas encore de version numérotée</span>
<span>Logiciel libre, AGPL-3.0</span>
<span>Démo : connexion automatique (<code>demo</code> / <code>demo</code>)</span>
</p>

<p class="evey-platforms">
<strong>Ce qu'il vous faut :</strong> un ordinateur ou un NAS x86-64 avec
Docker, et un dossier avec vos médias. L'installation tient en un fichier et
une commande.<br>
<strong>Regardez</strong> dans un navigateur web, sur les téléphones et
tablettes Android, sur Android TV, dans le navigateur web d'une télévision et sur un
Chromecast. Les applications
Android et Android TV sont en test et pas encore sur Google Play.
</p>

<div class="grid cards" markdown>

-   **Vous allez installer le serveur**

    Installez Evey sur votre ordinateur ou votre NAS, ajoutez vos médias,
    invitez votre famille.
    [Commencer par l'installation](installation.md)

-   **On vous a donné une adresse**

    Quelqu'un chez vous fait déjà tourner Evey, et vous voulez simplement regarder.
    [Lire Regarder avec Evey](getting-started/watching.md)

</div>

<div class="evey-showcase" markdown>

<figure class="evey-shot" markdown="span">
  ![La page d'accueil](images/showcase-home.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>L'accueil : les titres à reprendre, puis vos bibliothèques ligne par ligne.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![La fiche d'un film](images/showcase-media.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>La fiche d'un film : affiche, âge conseillé, notes, résumé et distribution.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Le lecteur web](images/showcase-player.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>Le lecteur web. Dans la barre : Regarder ensemble, Caster, et le (i) qui ouvre les Infos du flux.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Le tableau de bord d'administration pendant la lecture de deux films](images/showcase-dashboard.webp){ loading=lazy data-gallery="showcase" }
  <figcaption>Le tableau de bord pendant que deux personnes regardent : l'image, l'appareil, Lecture directe ou Remux.</figcaption>
</figure>

</div>

## Pour tout le foyer { #for-everyone-at-home }

- **Un profil par personne.** Chaque profil garde son historique, ses points
  de reprise, ses favoris et ses langues. En général, un foyer partage un
  seul compte, avec un profil par personne.
- **Des profils enfant.** Un profil enfant s'ouvre sans code. Pour en sortir
  vers un profil adulte, il faut un code PIN à quatre chiffres. Un profil
  enfant a un âge maximum, et le serveur n'envoie jamais un titre au-dessus,
  même en téléchargement.
- **Reprenez là où vous en étiez**, sur n'importe quel appareil. Les points de
  reprise sont gardés sur le serveur, pas sur le téléphone.
- **En déplacement.** L'application Android peut télécharger un titre pour le
  regarder sans connexion, pendant 30 jours.
- **Sur la TV.** Vous connectez une Android TV avec la **Connexion rapide** :
  la TV affiche un code, et vous l'acceptez depuis votre téléphone ou votre
  ordinateur. Aucun mot de passe à saisir avec la télécommande.
- **Ensemble.** Plusieurs personnes peuvent regarder le même titre au même
  moment, chacune sur son appareil.

La musique et les photos fonctionnent comme les films et les séries, sur le web,
sur Android et sur la TV.
[Connecter les applications](getting-started/apps.md) ·
[Utilisateurs, profils et contrôle parental](administration/users.md)

## Rien à choisir avant d'appuyer sur lecture { #nothing-to-choose-before-pressing-play }

Certains fichiers se lisent partout. D'autres sont trop lourds pour un vieux
téléphone, ou ont un son qu'une TV ne sait pas lire. À chaque lecture, le
serveur regarde le fichier, l'appareil et sa propre charge, puis choisit la
façon la plus légère qui fonctionne :

- **Lecture directe** : le fichier est envoyé tel quel.
- **Remux** : l'image et le son sont recopiés, sans changement, dans un
  format de fichier que l'appareil accepte. Cela ne coûte presque rien au
  serveur. Evey a son propre outil pour cela, **MKVGo**, et passe par FFmpeg
  quand MKVGo ne peut pas.
- **Transcodage** : le serveur convertit seulement ce que l'appareil ne sait
  pas lire, souvent juste le son. C'est le seul cas qui fait travailler le
  processeur.

Le même film peut donc partir intact vers une TV et être converti pour un
vieux téléphone. Dans le lecteur, les **Infos du flux** montrent lequel des
trois vous avez eu, et pourquoi. **Administration → Tableau de bord** montre
la même chose pour toutes les personnes qui regardent.

<figure class="evey-shot evey-shot--narrow" markdown="span">
  ![Les Infos du flux dans le lecteur web](images/player-stream-info.webp){ loading=lazy }
  <figcaption>La première lecture du tableau de bord ci-dessus, vue depuis le lecteur. Remux : le format de fichier change, l'image et le son sont recopiés, et la dernière ligne indique pourquoi le fichier n'a pas pu être envoyé tel quel (interface en anglais).</figcaption>
</figure>

[Comment Evey choisit le mode de lecture](playback/how-playback-is-decided.md) ·
[MKVGo dans Evey](playback/mkvgo.md)

## Un seul conteneur, rien à régler { #one-container-nothing-to-tune }

Toute l'installation tient dans un fichier compose. Pas de `.env` à écrire,
pas de réglage de mémoire ou de processeur.

- **Tout est dedans.** La base de données et le moteur de recherche tournent
  dans le conteneur. Leurs mots de passe sont créés au premier démarrage.
- **Le serveur ne tourne jamais en root.** Il tourne sous le propriétaire de
  votre dossier de médias quand il parvient à le déterminer, pour que les
  réparations puissent écrire.
- **Le serveur se mesure lui-même.** Dans un moment calme, il chronomètre de
  vraies conversions sur le processeur et sur la carte graphique que vous lui
  avez donnée, s'il y en a une. Il sait alors combien de personnes il peut
  servir en même temps. Quand la place manque, une nouvelle lecture reçoit une
  qualité plus basse, ou passe d'une carte graphique occupée au processeur. Si
  une lecture ne peut pas démarrer, le message indique pourquoi : le serveur est
  saturé, il est encore en train de se mesurer, ou le fichier n'est pas prêt.
- **L'accès depuis l'extérieur de chez vous est fermé** tant que vous ne
  donnez pas d'adresse publique au serveur.

Vous pouvez aussi utiliser votre propre base de données ou votre propre
moteur de recherche :
[Déploiements avancés](operations/advanced-deployments.md).
[Transcodage et capacité du serveur](playback/transcoding.md)

## Ce qu'Evey fait de vos fichiers et de vos données { #what-evey-does-with-your-files-and-your-data }

**Vos fichiers.** Retirer un titre ou une bibliothèque le retire d'Evey,
jamais du disque. Evey n'écrit dans vos dossiers de médias que dans trois
cas : une réparation que vous avez demandée, un sous-titre récupéré par l'API,
et un petit fichier de test qui vérifie qu'Evey détecte toujours les
nouveaux fichiers. Montez vos médias en lecture seule et rien de tout cela
n'arrive. **Santé des médias** vérifie vos fichiers MKV et WebM et liste les
problèmes trouvés. Elle ne répare un fichier que si vous le demandez. Elle
travaille sur une copie, vérifiée en entier avant de remplacer le fichier,
garde l'original par défaut, et note chaque changement. Ce qui ne peut pas
être réparé est signalé et laissé tel quel.
[Santé des médias](runbooks/media-health.md)

**Vos données.** Le projet ne fait tourner aucun service qui les reçoit : pas
de compte chez nous, pas de statistiques d'usage, pas de rapports de
plantage. Les applications communiquent avec votre serveur, et avec aucun
service que nous gérons. Ce qui sort de votre réseau : le serveur va chercher les images
et les descriptions sur des services extérieurs (The Movie Database, et des
sources de pochettes pour la musique). De temps en temps, il demande aussi
s'il existe une version plus récente, ce que vous pouvez désactiver. La base de
données est sauvegardée avant chaque mise à jour qui la modifie.
[Règles de confidentialité](privacy.md) · [Sauvegardes et mises à jour](backups.md)

## Les plugins sont isolés et encadrés { #plugins-run-isolated-and-controlled }

Les plugins ajoutent des sources d'informations sur les titres, des canaux de
notification, des sections sur l'accueil et des éléments par-dessus le
lecteur. Les thèmes changent l'apparence. Avant d'installer un plugin,
l'administrateur voit ce que l'analyse du plugin a trouvé et les permissions
qu'il demande. Une nouvelle permission doit être acceptée clairement. Un plugin
tourne dans son propre processus, sous un utilisateur à part, sans accès
direct au réseau. Il est arrêté s'il dépasse sa limite de mémoire, de
processus ou de disque. Le niveau de restriction d'accès à ses fichiers
dépend de la machine, et le **Tableau de bord sécurité** montre le mode utilisé. Le
widget d'un plugin s'affiche sur le web, sur Android et sur la TV. Sur la
TV, vous pouvez le voir mais pas vous en servir.
[Plugins](plugins.md) ·
[Modèle de sécurité et sandbox des plugins](SECURITY.md#sandbox-modes)

## Par où commencer { #find-your-way }

<div class="grid cards" markdown>

-   **[Installer Evey](installation.md)**

    Ce qu'il vous faut, le fichier compose, le premier démarrage, l'adresse à
    ouvrir.

-   **[Première configuration](getting-started/first-setup.md)**

    L'assistant de configuration, vos bibliothèques, les personnes qui
    regarderont.

-   **[Connecter les applications](getting-started/apps.md)**

    L'adresse à saisir sur un téléphone ou une TV, la connexion, et ce que
    signifie chaque erreur.

-   **[Regarder avec Evey](getting-started/watching.md)**

    Pour celles et ceux qui regardent : les profils, trouver un titre, les
    sous-titres, demander un film.

-   **[Administration](administration/libraries.md)**

    Bibliothèques, utilisateurs et contrôle parental, accès depuis
    l'extérieur, carte graphique, sauvegardes.

-   **[Dépannage](operations/troubleshooting.md)**

    Un tableau par problème : où regarder, quoi vérifier, comment le régler.

-   **[Comment Evey choisit le mode de lecture](playback/how-playback-is-decided.md)**

    Lecture directe, Remux, Transcodage, et comment lire les Infos du flux.

-   **[Plugins et API](plugins.md)**

    Installer et écrire des plugins et des thèmes, l'API du serveur.

</div>

<p class="evey-meta">
Cette documentation suit la branche de développement. ·
<a href="https://github.com/eveyproject/evey">Code source sur GitHub</a> ·
Signalements de sécurité : security@eveyproject.org ·
<a href="privacy/">Règles de confidentialité</a>
</p>
