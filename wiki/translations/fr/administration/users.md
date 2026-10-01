---
source_sha: 36492c52b264a6f9e288ee05ec9cae5b29f8e340329b30fe064fcf164c5045de
reviewed: true
---
# Utilisateurs, profils et contrôle parental

## Comptes et profils

Un **compte** sert à se connecter : il a un nom d'utilisateur et un mot de
passe (ou une identité d'authentification unique, voir [OIDC](../oidc.md)),
et il est administrateur ou non. Un **profil** sert à regarder : chaque
compte contient un ou plusieurs profils, chacun avec son historique, ses
points de reprise, ses favoris, ses langues et sa page d'accueil. Un foyer
partage en général un compte, avec un profil par personne.

## Créer des comptes

**Admin → Utilisateurs** propose deux façons de faire :

<figure class="evey-shot" markdown="span">
  ![Admin → Utilisateurs sur le serveur de démo](../images/admin-users.webp){ loading=lazy }
  <figcaption><strong>Admin → Utilisateurs</strong> : chaque compte avec ses profils, son rôle et son état.</figcaption>
</figure>

- **Créer un utilisateur** : vous choisissez le nom d'utilisateur et le mot
  de passe.
- **Créer un lien d'invitation** : la personne choisit les siens. Un lien peut
  avoir une date d'expiration et un nombre maximal d'utilisations, limiter les
  bibliothèques visibles par le nouveau compte, plafonner le nombre de profils
  qu'il pourra créer, et créer un administrateur. **Liens d'invitation** les
  liste ; un lien peut être révoqué.

## Ce qu'un administrateur peut restreindre par compte

| Réglage | Effet |
| --- | --- |
| **Accès aux bibliothèques** | Les bibliothèques visibles par le compte. Le reste n'apparaît ni dans ses recherches, ni sur sa page d'accueil, ni dans les réponses de l'API. |
| **Débit maximal** | Un plafond sur le débit reçu par les lecteurs du compte (0 = illimité). Un titre au-dessus est transcodé pour s'y conformer. |
| **Profils max** | Le nombre de profils que le compte peut créer (0 = illimité). |
| **Appareils max par profil** | Le nombre d'appareils que chaque profil peut enregistrer (0 = illimité). |

**Se connecter en tant que** ouvre le serveur tel que cet utilisateur le
voit, pour vérifier une restriction ou reproduire un problème. Un bandeau
reste visible, et **Quitter** vous ramène à votre propre compte.

## Profils enfant et code PIN

Un profil marqué **Profil enfant** s'ouvre sans code. Quitter ce profil pour
un profil qui n'est pas un profil enfant demande le **code PIN à 4
chiffres**, défini dans **Paramètres → Contrôle parental**. La première
définition du code demande le mot de passe du compte, et tant qu'un profil
enfant existe, le compte doit avoir un code PIN.

La gestion des profils et du contrôle parental n'est possible que depuis un
profil qui n'est pas un profil enfant.

## Limites d'âge

Chaque profil peut avoir un **Âge maximum** (**Paramètres → Profils →
Modifier**). Le système de classification est celui du pays défini dans
**Admin → Configuration du serveur → Langue et région**.

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

## Demandes

Les utilisateurs peuvent chercher un film, une série ou un artiste absent des
bibliothèques et en faire la demande. Les demandes arrivent dans **Admin →
Demandes de média**, où un administrateur les accepte, les refuse ou les
reporte, avec une note. C'est là aussi qu'on règle qui peut faire une
demande : tout le monde par défaut, avec des exceptions par utilisateur.

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
