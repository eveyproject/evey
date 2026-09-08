---
source_sha: de18778826e893fcc307b12b63d7319ff9c95d628db3f0df088772e9de9a77bd
---

# ADR-0005 : autorisation à deux couches (RBAC dans l'intercepteur, vérification de la ressource dans le gestionnaire)

- Statut : acceptée
- Amendée par : [ADR-0012](0012-plugin-token-deny-by-default.md) - le filtrage des portées à la frappe s'est révélé insuffisant ; l'application se fait désormais par refus par défaut dans l'intercepteur
- Date : 2026-04-20

## Contexte

L'API gRPC d'Evey expose à la fois des opérations d'administration valables pour
tout le serveur (gestion des bibliothèques, des utilisateurs, de la
configuration) et des ressources propres à chaque utilisateur (playlists,
favoris, historique, médias limités à une bibliothèque). Mélanger les deux sur
une seule couche d'autorisation produit deux pannes récurrentes :

- **Le garde d'administration oublié.** Un nouveau RPC d'administration part
  sans vérification et n'importe quel compte authentifié peut supprimer une
  bibliothèque. Facile à rater en revue, parce que le gestionnaire ressemble aux
  RPC utilisateur qui l'entourent.
- **La vérification de ressource oubliée.** Un RPC utilisateur lit un
  identifiant de média dans la requête et renvoie la ligne sans vérifier que
  l'appelant a accès à la bibliothèque qui la contient. L'IDOR classique.

S'y ajoutent nos audits de sécurité périodiques : chaque passage a produit des
faux positifs d'IDOR sur des points d'accès réservés à l'administration, l'outil
voyant l'absence de vérification dans le gestionnaire sans voir que
l'intercepteur, une couche au-dessus, a déjà rejeté tout non-administrateur.

## Décision

Séparer l'autorisation en deux couches aux responsabilités orthogonales.

**Couche 1, RBAC dans l'intercepteur : « avez-vous le droit d'atteindre ce
point d'accès ».** Un unique intercepteur consulte la politique avant tout
gestionnaire et range chaque méthode gRPC dans l'une de trois cases : méthodes
publiques sans authentification (connexion, rafraîchissement, inscription sur
invitation, réinitialisation de mot de passe, assistant d'installation) ;
méthodes réservées à l'administration, c'est-à-dire toute opération destructrice,
globale ou touchant un autre utilisateur, refusées par `PermissionDenied` ; et
par défaut, toute méthode absente des deux listes, ouverte à un appelant
authentifié.

L'intercepteur est le seul code d'autorisation que voient les gestionnaires
d'administration. Ils font confiance à la couche.

**Couche 2, vérification de la ressource dans le gestionnaire : « avez-vous le
droit de toucher CETTE ligne ».** Pour une ressource propre à un utilisateur, le
gestionnaire vérifie la propriété ou l'accès à la bibliothèque contre l'identité
de l'appelant. Cette couche ne peut pas remonter dans l'intercepteur, parce que
l'identifiant de la ressource se trouve dans le corps de la requête et non dans
le nom de la méthode : l'intercepteur n'a aucun moyen de résoudre un identifiant
de média vers une bibliothèque sans décoder chaque message.

Les administrateurs sautent la couche 2 par conception : on leur fait confiance
sur toutes les bibliothèques.

## Conséquences

- **Ajouter un RPC d'administration tient en une ligne** de politique. L'oublier
  fait du bruit : la méthode retombe au rôle utilisateur et le test de table
  échoue tant qu'elle n'est pas déclarée.
- **Ajouter un RPC utilisateur qui touche une bibliothèque, une playlist ou une
  session exige une vérification dans le gestionnaire.** Aucune couche
  d'intercepteur ne peut la faire à votre place.
- **Les outils d'audit qui ignorent le RBAC se trompent.** Nous en acceptons le
  coût ; la règle pour les prochains audits est de vérifier la politique avant
  de signaler un IDOR sur un RPC de mutation.
- **La défense en profondeur est volontairement absente des RPC
  d'administration.** Nous n'ajoutons pas de vérification redondante « au cas où
  le RBAC serait contourné » : si l'intercepteur est contourné, tout le serveur
  l'est, et une seconde vérification sur un gestionnaire n'y changerait rien.

## Références

- `internal/adapters/driving/grpcapi/interceptors/rbac.go` - la politique
- `internal/adapters/driving/grpcapi/interceptors/auth.go` - l'application
- `internal/adapters/driving/grpcapi/interceptors/rbac_test.go` - le test de
  table qui force toute méthode connue à être classée
- [ADR-0003](0003-plugin-scoped-tokens.md) - pourquoi les jetons d'extension
  filtrent leurs portées à la frappe au lieu de s'en remettre au RBAC
