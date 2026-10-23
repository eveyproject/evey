---
source_sha: a4932bd9b417f0f18be610734a595c186387167b23522b0dbd34267c0fc17fc7
reviewed: true
---
# Utilisateurs, profils et contrôle parental { #users-profiles-and-parental-control }

Deux mots comptent sur cette page :

- Un **compte**, c'est ce qui se connecte : un nom d'utilisateur et un mot de
  passe.
- Un **profil**, c'est une personne qui regarde. Chaque profil a son propre
  historique, ses points de reprise, ses favoris, ses langues et sa page
  d'accueil.

Un compte peut contenir plusieurs profils. La plupart des foyers partagent
un seul compte, avec un profil par personne. Vous pouvez aussi donner à
chaque adulte son propre compte.

Cette page commence par les tâches courantes : ajouter quelqu'un, ajouter un
enfant. Les réglages plus fins (limites par compte, horaires d'ouverture,
demandes, page d'accueil, liens de téléchargement) viennent ensuite.

## Comptes et profils { #accounts-and-profiles }

Un compte a un nom d'utilisateur et un mot de passe, ou se connecte par
authentification unique (voir [OIDC](../oidc.md)). C'est soit un compte
administrateur, soit un compte normal. Chaque nouveau compte commence avec un
profil, qui porte son nom d'utilisateur.

## Créer un compte { #create-an-account }

<figure class="evey-shot" markdown="span">
  ![Administration → Utilisateurs sur le serveur de démo](../images/admin-users.webp){ loading=lazy }
  <figcaption><strong>Administration → Utilisateurs</strong> : chaque compte avec ses profils, son rôle et son état.</figcaption>
</figure>

Allez dans **Administration → Utilisateurs**. Il y a deux façons de faire.

**Envoyer un lien d'invitation**, pour que la personne choisisse son mot de
passe :

1. Appuyez sur **Créer un lien d'invitation**.
2. Si vous voulez, limitez-le : **Date d'expiration (optionnel)**, **Nombre
   max d'utilisations (vide = illimité)**, **Bibliothèques accessibles** (les
   bibliothèques que le nouveau compte verra), et le nombre de profils qu'il
   pourra créer. Cochez **Créer un administrateur** seulement pour quelqu'un
   qui gérera le serveur avec vous.
3. Sous **Liens d'invitation**, appuyez sur **Copier le lien** et envoyez-le.

Le lien est construit à partir de l'adresse de votre navigateur. Ouvrez le
panneau d'administration depuis l'adresse que les autres utiliseront (par
exemple `http://192.168.1.20:9091`), pas depuis `localhost`. **Révoquer**
empêche un lien de fonctionner.

**Ou créer le compte vous-même** : appuyez sur **Créer un utilisateur**,
choisissez le **Nom d'utilisateur** et le **Mot de passe**, puis donnez-les à
la personne. Cochez **Administrateur** pour un administrateur. Vous pouvez
aussi régler tout de suite les bibliothèques, le nombre de profils et le
nombre d'appareils (voir
[Ce qu'un administrateur peut restreindre par compte](#what-an-administrator-can-restrict-per-account)).

Sur la ligne d'un compte :

- **Désactiver** bloque ses connexions sans rien supprimer. **Activer** rétablit
  ses connexions.
- **Supprimer** retire le compte et ses profils.

## Profils, profils enfant et code PIN { #profiles-kids-profiles-and-the-pin }

**Pour ajouter un profil** : depuis ce compte, allez dans **Paramètres →
Profils → Créer un profil**. Un administrateur peut aussi en ajouter un à
n'importe quel compte, depuis **Modifier l'utilisateur → Nouveau profil**. Un
profil a un **Nom**, une **Langue** et une **Région**.

**Pour ajouter le profil d'un enfant** :

1. Créez le profil et cochez **Profil enfant**.
2. Choisissez un **Âge maximum**. Si vous laissez **Par défaut (12+)**, la
   limite est de 12 ans.
3. Donnez un **Code PIN (4 chiffres)** à tous les autres profils du compte.
   Le serveur l'exige dès qu'un profil enfant existe, et refuse d'enregistrer
   un profil qui n'en a pas.

Ce que l'enfant voit ensuite :

- Le profil enfant s'ouvre sans code PIN.
- Pour passer à un autre profil, il faut le code PIN de ce profil.
- Les titres au-dessus de la limite d'âge n'apparaissent jamais (voir
  [Limites d'âge](#age-limits)).

??? info "Détails sur le code PIN, et qui peut gérer les profils"

    - Pour créer un code PIN la première fois, il faut le mot de passe du
      compte. Pour le changer, il faut le code actuel. **Paramètres →
      Contrôle parental** règle le code PIN du profil en cours.
    - Les profils et le contrôle parental ne se gèrent que depuis un profil
      qui n'est pas un profil enfant.
    - Le compte d'un administrateur ne peut pas avoir de profil enfant : la
      limite cacherait une partie de la bibliothèque de l'administrateur
      lui-même.

## Limites d'âge { #age-limits }

C'est le serveur, pas l'application, qui applique la limite d'âge. Un titre
au-dessus de la limite n'est pas seulement caché à l'écran : il n'est jamais
envoyé au profil enfant. C'est vrai pour la navigation, la recherche, la
lecture et les téléchargements.

- **Quelles classifications sont utilisées.** L'âge est lu dans le système
  de classification de la **Région** du profil (**Paramètres → Profils →
  Modifier**). Avec **Automatique**, la région vient de la langue du profil,
  et le système britannique sert quand cela ne suffit pas.
- **Un titre sans classification utilisable** est caché au profil enfant
  plutôt que montré.
- **En cas de doute, l'accès est refusé.** Si le serveur ne parvient pas à lire les réglages
  d'un profil (une erreur de base de données, par exemple), il refuse le
  contenu au lieu de l'autoriser.
- **Les téléchargements aussi.** Un film téléchargé sur un téléphone garde sa
  limite sans connexion : un profil enfant ne peut pas ouvrir un
  téléchargement fait par un autre profil s'il dépasse son âge maximum.

Les administrateurs ne sont pas soumis aux limites des profils.

## Ce qu'un administrateur peut restreindre par compte { #what-an-administrator-can-restrict-per-account }

Ces réglages sont dans **Créer un utilisateur** et **Modifier
l'utilisateur**, et sur la ligne du compte dans **Administration →
Utilisateurs**.

| Réglage | Ce qu'il fait |
| --- | --- |
| **Accès aux bibliothèques** (le bouton **Bibliothèques** de la ligne) | Les bibliothèques que le compte voit. Les autres n'apparaissent nulle part pour lui : ni dans la recherche, ni sur sa page d'accueil. Un administrateur voit toujours tout. |
| **Débit maximal** (le bouton **Débit**) | Limite la qualité vidéo que reçoit le compte, un des six préréglages de 1 à 40 Mbps, ou Illimité. Un titre au-dessus est converti pour descendre à cette limite. |
| **Profils max** | Combien de profils le compte peut créer (0 = pas de limite). |
| **Appareils max par profil** | Combien de vidéos chaque profil peut lire en même temps, tous appareils confondus (0 = pas de limite). Une vidéo de plus est refusée au démarrage. |

**Se connecter en tant que** montre le serveur exactement comme cet
utilisateur le voit. Servez-vous-en pour vérifier une restriction ou pour
voir un problème par vous-même. Un bandeau reste à l'écran, et **Quitter**
vous ramène à votre propre compte.

## Horaires d'ouverture { #opening-hours }

Vous pouvez autoriser un profil enfant à regarder seulement à certaines
heures.

1. Dans **Paramètres → Profils**, appuyez sur l'icône d'horloge à côté du
   profil enfant.
2. Activez **Limiter aux horaires d'ouverture**. Evey propose du lundi au
   vendredi de 18:00 à 22:00, et le samedi et le dimanche de 9:00 à 12:00 et
   de 14:00 à 21:00.
3. Ajustez les horaires, soit avec **Semaine / week-end** (un bloc pour le
   lundi au vendredi, un pour le week-end), soit avec **Jour par jour**.

Un jour peut avoir plusieurs plages horaires. Une plage peut passer minuit
(20:00 à 0:30). Un jour sans plage est fermé.

Ce qui se passe :

- **En dehors des horaires**, le profil est fermé partout : navigation,
  films, musique, téléchargements. Chaque application affiche un écran
  verrouillé qui indique quand il redevient accessible.
- **Dix minutes avant la fermeture**, le lecteur affiche un avertissement. À
  l'heure de fermeture, la lecture s'arrête.
- **Pour laisser l'enfant entrer quand même**, saisissez le **Code PIN parent**
  sur l'écran verrouillé : le code PIN d'un profil du même compte qui n'est
  pas un profil enfant. Il ouvre le profil enfant jusqu'à sa prochaine plage
  d'ouverture. Le code PIN de l'enfant ne fonctionne pas ici. **Changer de
  profil** revient à la liste des profils.

??? info "Fuseau horaire et cas particuliers"

    - Le serveur applique les horaires dans son propre fuseau horaire :
      **Administration → Configuration du serveur → Langue et région →
      Fuseau horaire du serveur** (s'il est vide, la variable `TZ`, puis
      Europe/Paris). Changer l'heure d'une tablette ne change rien.
    - Revenir de **Jour par jour** à **Semaine / week-end** est refusé quand
      les jours d'un bloc sont différents : ainsi, rien n'est perdu par
      erreur.
    - Des horaires que le serveur ne parvient pas à lire gardent le profil
      fermé.
    - Un film téléchargé et lu sans connexion ne passe jamais par le
      serveur : les horaires ne s'y appliquent donc pas.

## Demandes { #requests }

Les utilisateurs peuvent demander un film, une série ou un artiste que vous n'avez
pas encore. Leurs demandes arrivent dans **Administration → Demandes de
média**. Pour chacune, choisissez **Accepter**, **Refuser** ou **Plus tard**,
et ajoutez une note si vous voulez. La personne est prévenue de votre
réponse.

Pour choisir qui peut demander, ouvrez l'onglet **Accès** : **Autoriser les
demandes par défaut pour tous les utilisateurs**, puis **Dérogations par
utilisateur** pour les exceptions.

## La page d'accueil { #the-home-page }

La page d'accueil est une suite de rangées. Une rangée qui n'a rien à montrer
est cachée.

| Rangée | Ce qu'elle montre |
| --- | --- |
| **Reprendre la lecture** | Ce que le profil a commencé sans le finir, le plus récent d'abord. Un titre compte comme vu à 90 %. |
| **À suivre** | Pour chaque série que le profil a commencée, le prochain épisode pas encore vu. |
| **Collections** | Les collections, avec un lien vers la liste complète. |
| **Recommandé pour vous** | Des films et séries dans les genres que le profil regarde le plus, ceux qui correspondent le mieux d'abord, puis par note. Un genre compte quand il apparaît dans au moins 3 des 200 dernières lectures du profil. |
| **Les plus populaires** | Les films et séries les plus lus sur le serveur ces 90 derniers jours. |
| Une rangée par bibliothèque | Les derniers ajouts à cette bibliothèque, les plus récents d'abord. |

**Recommandé pour vous** et **Les plus populaires** laissent de côté ce que
le profil a déjà terminé. Une série compte comme terminée une fois tous ses
épisodes vus. Les plugins peuvent ajouter leurs propres rangées.

**Qui décide des rangées :**

- Vous réglez le choix par défaut pour tous les profils dans
  **Administration → Configuration du serveur → Flux d'accueil** : quelles
  rangées apparaissent, dans quel ordre, et combien d'éléments chacune
  montre.
- Chaque profil peut ensuite arranger son propre accueil avec
  **Personnaliser** (sur la page d'accueil des applications web et
  téléphone, dans la barre du haut de l'application TV) : afficher ou cacher
  une rangée, la déplacer, ou **Réinitialiser**. C'est enregistré sur le
  serveur et s'applique au profil sur tous ses appareils.
- Un profil peut afficher une rangée que vous avez cachée, sauf la rangée
  d'un plugin : là, votre choix est définitif.

## Liens de téléchargement publics { #public-download-links }

Sur la page d'un film ou d'un épisode, un administrateur peut créer un
**Lien de téléchargement public** (applications web et Android). Toute
personne qui a le lien peut télécharger le fichier d'origine, sans compte.
Le lien est copié pour que vous puissiez le coller.

- Il cesse de fonctionner au bout de 48 heures.
- Quand l'accès distant est désactivé, il ne fonctionne que depuis le réseau
  domestique.
- Qui a le lien peut télécharger le fichier jusqu'à ce qu'il expire :
  partagez-le avec autant de soin que le fichier lui-même.

??? info "Par l'API du serveur"

    Seuls les administrateurs peuvent créer, lister et révoquer des liens.
    Les applications ne font que les créer ; la liste et la révocation
    passent par l'API (`GET /api/v1/admin/share-links`,
    `DELETE /api/v1/share-links/{id}`). Par l'API, un lien peut durer
    jusqu'à 72 heures et être limité à un nombre de téléchargements
    (`POST /api/v1/media/{id}/share`).
