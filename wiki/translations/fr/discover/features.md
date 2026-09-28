---
source_sha: 51c7a248181b2b8e94e8f646b4cf91f36a740e2bc6410ad95e7d53bb1099c0cd
---
# Ce qu'Evey vous offre

Une visite d'Evey du côté de ceux qui s'en servent tous les soirs. Chaque
section se termine par l'endroit où trouver les réglages correspondants.

## Une bibliothèque qui s'entretient seule

Déposez des fichiers dans les dossiers qu'Evey surveille et ils apparaissent
d'eux-mêmes, en général en quelques secondes : le serveur remarque les nouveaux
fichiers, reconnaît films et épisodes à leur nom, et récupère affiches, fonds,
résumés, distribution, équipe et notes dans la langue que vous avez choisie.
Les films d'une même saga sont regroupés en collections, les séries en
saisons, la musique en artistes et albums, les photos en albums.

Quand un nom est trop ambigu pour être reconnu, le titre est conservé plutôt
que perdu, et un administrateur peut l'associer à la main ou corriger ses
informations. Ce que vous modifiez à la main le reste : la prochaine
actualisation ne l'écrase pas.

Quatre types de bibliothèques : **films**, **séries**, **musique** et
**photos**.

Réglages : [Premiers pas](../guide/first-steps.md#add-your-libraries) ·
[Administration](../guide/admin-settings.md#media).

## Une lecture qui démarre, tout simplement

Vous appuyez sur Lecture ; Evey décide comment. Si l'appareil sait lire le
fichier tel quel, le fichier est envoyé intact, dans sa qualité d'origine. Si
seul le conteneur pose problème, il est réempaqueté à la volée sans toucher à
l'image. Si l'appareil ne sait vraiment pas le décoder - un codec inconnu, du
HDR sur un écran qui ne l'affiche pas, une connexion trop faible pour
l'original - Evey ne convertit que le nécessaire, avec la carte graphique quand
le serveur en a une.

Pendant la lecture :

- **reprise partout** - arrêtez sur la télé, reprenez sur le téléphone à la
  même seconde ;
- **passer le générique, le résumé et le générique de fin** d'un bouton, grâce
  aux bases communautaires ou à la détection du serveur lui-même ;
- **l'épisode suivant** démarre seul, avec un compte à rebours que vous pouvez
  annuler ;
- des **vignettes sur la barre de progression** montrent où vous allez quand
  vous la faites glisser ;
- les **pistes audio et de sous-titres** suivent vos préférences de langue, se
  changent en plein film sans repartir du début, et les sous-titres peuvent être
  restylés, décalés dans le temps ou ajoutés depuis un fichier ;
- **chapitres**, **minuterie de mise en veille**, **vitesse** de lecture, et
  raccourcis clavier sur le web.

Réglages : [Réglages utilisateur](../guide/viewer-settings.md#in-the-player) ·
[Transcodage et matériel](../transcoding.md).

## Quelque chose pour chacun à la maison

Chaque compte contient plusieurs **profils**, chacun avec son historique, ses
points de reprise, sa langue, ses favoris et sa page d'accueil. Le choix du
profil ouvre l'application, comme sur une télé familiale.

- Les **profils enfants** s'ouvrent sans friction ; les quitter pour un profil
  adulte demande un code PIN à quatre chiffres.
- Les **limites d'âge** se règlent par profil et sont appliquées par le
  serveur : un titre au-dessus de la limite n'est pas masqué par l'application,
  il n'est jamais envoyé. Si le serveur ne parvient pas à lire la limite d'un
  profil, il n'affiche rien plutôt que tout.
- **Invitations** : un administrateur envoie un lien, le destinataire choisit
  un mot de passe, et arrive avec les bibliothèques et le nombre de profils
  décidés pour lui.
- **Demandes** : un utilisateur peut chercher un film, une série ou un artiste
  absent et le demander ; l'administrateur accepte, refuse ou reporte, avec une
  note.

Réglages : [Réglages utilisateur](../guide/viewer-settings.md#profiles-and-parental-control) ·
[Administration](../guide/admin-settings.md#users-and-access).

## Regarder ensemble, depuis des domiciles différents

**Regarder ensemble** ouvre un salon autour d'un film, d'un épisode ou d'une
file de musique. Partagez le code à six caractères ou le lien ; tous ceux qui
rejoignent lisent au même rythme que l'hôte - pause, déplacement et reprise
compris. Celui qui a pris du retard peut se recaler seul sans déranger le
salon.

## Musique et photos

La musique a ses artistes, albums, listes de lecture, lecture aléatoire, file
d'attente, une lecture sans blanc pour les albums qui en ont besoin, et un
mini-lecteur qui continue pendant que vous naviguez. Les photos ont leurs
albums, une visionneuse plein écran avec les informations de prise de vue, et
un diaporama.

## Vos propres étagères

- **Favoris** et **notes** par profil.
- **Collections** : les sagas qu'Evey construit lui-même, plus les vôtres, dans
  l'ordre que vous voulez.
- **Collections intelligentes** : un filtre enregistré (genre, année,
  résolution, langue...) qui se remplit tout seul à mesure que la bibliothèque
  grandit.
- **Listes de lecture** de films, d'épisodes et de morceaux, à lire dans
  l'ordre ou au hasard.
- **Historique** et **statistiques** de ce que vous avez regardé, que vous
  pouvez effacer.
- Une **page d'accueil** que vous pouvez réorganiser : reprendre, à suivre,
  ajouts récents, recommandations, collections.

## Hors ligne, sur le téléphone

L'application Android télécharge un film, une saison, une série entière, une
collection ou une liste de lecture pour l'avion. Quand un film est trop lourd
pour le téléphone, elle propose une copie plus légère préparée par le serveur.
Les téléchargements respectent le profil qui les a faits : un profil enfant ne
peut pas ouvrir un film téléchargé par un autre profil au-dessus de sa limite
d'âge.

## Garder un œil sur le serveur

Les administrateurs voient en direct qui regarde quoi et comment - lu tel quel,
réempaqueté ou converti, et pourquoi - avec la charge du serveur, sa capacité
de conversion mesurée, la qualité de diffusion par appareil, et une page de
santé des médias qui trouve et répare les fichiers où l'on se déplace mal. La
base de données est sauvegardée avant chaque mise à jour, et selon une
planification si vous en fixez une.

Réglages : [Administration](../guide/admin-settings.md#monitoring-and-maintenance).

## L'étendre

Les extensions ajoutent des sources de métadonnées, des notifications, des
webhooks, des widgets pour la page d'accueil et des thèmes. Elles tournent dans
un bac à sable avec les seules permissions qu'elles déclarent, et un
administrateur voit ces permissions avant d'installer. Un thème est une feuille
de style : il change l'apparence de l'interface sans toucher à son code.

En savoir plus : [Extensions](../plugins.md) · [Référence de l'API des thèmes](../themes/api-reference.md).

## L'essayer

La démo publique sur [demo.eveyproject.org](https://demo.eveyproject.org) est le même serveur que celui
que vous installeriez, avec une petite bibliothèque de films sous licence libre.
Connectez-vous avec `demo` / `demo`. Parcourez, lancez un film, créez des
collections, ouvrez l'administration : les réglages sont affichés mais la
plupart sont verrouillés, et l'instance est remise à zéro régulièrement.
