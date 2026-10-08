---
source_sha: 6e4dfec863689cacafb646acf12485ba6886feb6906c692504a7a25560c2b5de
reviewed: true
---
# Utilisateurs, profils et contrôle parental

Un **compte** se connecte ; un **profil** regarde. Cette page s'adresse à
l'administrateur qui installe le foyer : créer les comptes, laisser chacun
créer ses profils, marquer ceux des enfants comme profils enfant et leur
donner une limite d'âge. Ce qu'un compte peut se voir restreindre, les
horaires d'ouverture, les demandes, les liens de téléchargement publics et la
page d'accueil viennent ensuite.

## Comptes et profils

Un compte a un nom d'utilisateur et un mot de passe (ou une identité
d'authentification unique, voir [OIDC](../oidc.md)), et il est administrateur
ou non. Chaque compte contient un ou plusieurs profils, chacun avec son
historique, ses points de reprise, ses favoris, ses langues et sa page
d'accueil. Un foyer partage en général un compte, avec un profil par
personne.

## Créer un compte

<figure class="evey-shot" markdown="span">
  ![Admin → Utilisateurs sur le serveur de démo](../images/admin-users.webp){ loading=lazy }
  <figcaption><strong>Admin → Utilisateurs</strong> : chaque compte avec ses profils, son rôle et son état.</figcaption>
</figure>

**Admin → Utilisateurs** propose deux façons de faire :

- **Créer un utilisateur** : vous choisissez le **Nom d'utilisateur** et le
  **Mot de passe**, cochez **Administrateur** pour un administrateur, et
  pouvez fixer dès ce moment les bibliothèques, le nombre de profils et le
  nombre d'appareils (voir plus bas).
- **Créer un lien d'invitation** : la personne choisit les siens. Un lien
  peut avoir une **Date d'expiration** et un **Nombre max d'utilisations**,
  limiter les bibliothèques visibles par le nouveau compte (**Bibliothèques
  accessibles**), plafonner le nombre de profils qu'il pourra créer, et
  **Créer un administrateur**. **Liens d'invitation** les liste ; **Copier le
  lien** en copie un, **Révoquer** y met fin.

**Désactiver**, sur la ligne d'un compte, refuse ses connexions sans rien
supprimer ; **Activer** les rétablit. **Supprimer** retire le compte et ses
profils.

## Profils, profils enfant et code PIN

Chaque compte crée ses profils dans **Paramètres → Profils → Créer un
profil** ; un administrateur peut aussi en ajouter un depuis **Modifier
l'utilisateur → Nouveau profil**. Un profil a un **Nom**, une **Langue** et
une **Région**, et deux réglages qui comptent pour les enfants :

- **Profil enfant** marque le profil comme celui d'un enfant. Il s'ouvre sans
  code, et le quitter pour un autre profil demande le **Code PIN (4
  chiffres)** de ce profil. Tant qu'un profil enfant existe, tous les autres
  profils du compte doivent avoir un code PIN : le serveur refuse
  d'enregistrer un profil qui n'en a pas. La première définition d'un code
  demande le mot de passe du compte ; le changer demande le code actuel.
  **Paramètres → Contrôle parental** définit le code du profil en cours.
- **Âge maximum** n'apparaît que sur un profil enfant : les titres classés
  au-dessus ne lui sont jamais envoyés. Laissé sur **Par défaut (12+)**, la
  limite est 12 ans.

La gestion des profils et du contrôle parental n'est possible que depuis un
profil qui n'est pas un profil enfant. Le compte d'un administrateur ne peut
pas avoir de profil enfant : la limite verrouillerait la propre bibliothèque
de l'administrateur.

## Limites d'âge

L'âge est lu dans le système de classification de la **Région** du profil
(**Paramètres → Profils → Modifier** ; **Automatique** la déduit de la langue
du profil, et se rabat sur le système britannique). Un titre dont ce système
ne sait pas situer la classification est masqué au profil enfant plutôt que
montré.

La limite est appliquée par le serveur, pas par les applications : un titre
au-dessus n'est pas masqué dans l'interface, il n'est jamais envoyé. Cela vaut
pour la navigation et la recherche, pour la lecture et pour les
téléchargements. Si le serveur ne parvient pas à lire les réglages d'un
profil (une erreur de base de données, par exemple), il refuse le contenu
plutôt que de l'autoriser. Les administrateurs ne sont pas soumis aux limites
des profils.

Un film téléchargé sur un téléphone reste soumis à la limite hors connexion :
un profil enfant ne peut pas ouvrir un téléchargement fait par un autre profil
au-dessus de son âge maximum.

## Ce qu'un administrateur peut restreindre par compte

Ces réglages se trouvent dans **Créer un utilisateur** et **Modifier
l'utilisateur**, et sur la ligne du compte dans **Admin → Utilisateurs**.

| Réglage | Effet |
| --- | --- |
| **Accès aux bibliothèques** (bouton **Bibliothèques** de la ligne) | Les bibliothèques visibles par le compte. Le reste n'apparaît ni dans ses recherches, ni sur sa page d'accueil, ni dans les réponses de l'API. Un administrateur voit toujours tout. |
| **Débit maximal** (bouton **Débit**) | Un plafond, de 1 à 40 Mbps, sur le débit reçu par les lecteurs du compte (0 = illimité). Un titre au-dessus est transcodé pour s'y conformer. |
| **Profils max** | Le nombre de profils que le compte peut créer (0 = illimité). |
| **Appareils max par profil** | Le nombre de lectures que chaque profil peut avoir en cours en même temps, tous appareils confondus (0 = illimité). Une lecture de plus est refusée à son démarrage. |

**Se connecter en tant que** ouvre le serveur tel que cet utilisateur le
voit, pour vérifier une restriction ou reproduire un problème. Un bandeau
reste visible, et **Quitter** vous ramène à votre propre compte.

## Horaires d'ouverture

Un profil enfant peut être limité à des **horaires d'ouverture** : l'icône
d'horloge à côté de lui dans **Paramètres → Profils** ouvre l'éditeur. Rien
n'est actif tant que vous n'activez pas **Limiter aux horaires
d'ouverture** ; l'éditeur propose alors du lundi au vendredi de 18:00 à
22:00, et le samedi et le dimanche de 9:00 à 12:00 et de 14:00 à 21:00.

Les horaires sont une liste de plages, modifiables de deux façons : **Semaine /
week-end** (un bloc pour le lundi au vendredi, un pour le week-end) ou **Jour
par jour**. Un jour peut avoir plusieurs plages, une plage peut passer minuit
(20:00 à 0:30), et un jour sans plage est fermé. Le retour à la vue semaine
est refusé quand les jours d'un bloc diffèrent : rien n'est perdu sans le dire.

Le serveur applique les horaires, dans son propre fuseau (**Admin →
Configuration du serveur → Langue et région → Fuseau horaire du serveur** ;
à défaut, la variable `TZ`, puis Europe/Paris). Changer l'heure d'une tablette
ne change rien. En dehors des horaires, le profil est refusé partout -
navigation, chaque segment d'un film en cours, musique, téléchargements - et
chaque application affiche un écran de verrouillage qui indique quand il
rouvre. Dix minutes avant la fermeture, le lecteur affiche un avertissement ;
à l'heure, la lecture s'arrête.

Sur l'écran de verrouillage, le **Code PIN parent** (le code d'un profil du
même compte qui n'est pas un profil enfant) ouvre le profil enfant jusqu'à sa
prochaine plage ; le code de l'enfant, s'il en a un, ne suffit pas. **Changer
de profil** mène au choix des profils. Des horaires que le serveur ne parvient
pas à lire gardent le profil fermé plutôt qu'ouvert.

Un film téléchargé sur un téléphone et lu sans connexion ne passe pas par le
serveur : les horaires ne s'y appliquent pas.

## Demandes

Les utilisateurs peuvent chercher un film, une série ou un artiste absent des
bibliothèques et en faire la demande. Les demandes arrivent dans **Admin →
Demandes de média**, où un administrateur choisit **Accepter**, **Refuser** ou
**Plus tard**, avec une note. L'onglet **Accès** règle qui peut faire une
demande : **Autoriser les demandes par défaut pour tous les utilisateurs**,
avec des **Dérogations par utilisateur**.

## Liens de téléchargement publics

Sur la page d'un film ou d'un épisode, un administrateur peut créer un **Lien
de téléchargement public** (applications web et Android). C'est une adresse que
n'importe qui peut ouvrir, sans compte, pour télécharger le fichier d'origine ;
elle est copiée dans le presse-papiers.

- Il expire au bout de 48 heures. Par l'API du serveur, il peut durer jusqu'à
  72 heures et être limité à un nombre de téléchargements
  (`POST /api/v1/media/{id}/share`).
- Seuls les administrateurs peuvent créer, lister et révoquer des liens. Les
  applications ne font que les créer : la liste et la révocation passent par
  l'API (`GET /api/v1/admin/share-links`, `DELETE /api/v1/share-links/{id}`).
- Accès distant coupé, le lien ne fonctionne que depuis les réseaux locaux.
- Qui détient le lien peut télécharger le fichier jusqu'à son expiration :
  partagez-le comme le fichier lui-même.

## La page d'accueil

La page d'accueil est une pile de rangées ; une rangée sans rien à montrer est
masquée.

| Rangée | Ce qu'elle montre |
| --- | --- |
| **Reprendre la lecture** | Ce que le profil a commencé sans le finir, le plus récent d'abord. Un titre compte comme vu à 90 %. |
| **À suivre** | Pour chaque série dont le profil a vu un épisode, le prochain épisode non vu. |
| **Collections** | Les collections, avec un lien vers la liste complète. |
| **Recommandé pour vous** | Les films et séries des genres habituels du profil, la meilleure correspondance d'abord, puis par note. Un genre compte quand il apparaît dans au moins 3 des 200 dernières lectures du profil. |
| **Les plus populaires** | Les films et séries les plus regardés sur le serveur ces 90 derniers jours. |
| Une rangée par bibliothèque | Les derniers ajouts à cette bibliothèque, du plus récent au plus ancien. |

Recommandé pour vous et Les plus populaires écartent ce que le profil a déjà
terminé ; une série compte comme terminée une fois chaque épisode vu. Les
plugins peuvent ajouter leurs propres rangées.

**Admin → Configuration du serveur → Flux d'accueil** fixe le réglage par défaut
de tous les profils : quelles rangées apparaissent, dans quel ordre, et combien
d'éléments chacune montre. Chaque profil arrange ensuite son propre accueil avec
**Personnaliser** (sur la page d'accueil des applications web et mobile, dans
l'en-tête de l'application TV) : afficher ou masquer une rangée, la déplacer, ou
**Réinitialiser**. Le choix est enregistré sur le serveur et suit le profil sur
chaque appareil. Un profil peut afficher une rangée que l'administrateur a
masquée, sauf la rangée d'un plugin : là, le choix de l'administrateur est
définitif.
