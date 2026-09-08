---
source_sha: 62943851293888a0d427390abe522ed9511828af03ddc0ed6109fbea958fe8df
---

# ADR-0009 : séparation CQRS des dépôts

- Statut : acceptée
- Date : 2026-04-20

## Contexte

Les dépôts de domaine d'origine étaient des interfaces fourre-tout mêlant
lectures et écritures :

```go
type LibraryRepository interface {
    Create(ctx, lib) error
    Update(ctx, lib) error
    Delete(ctx, id) error
    GetByID(ctx, id) (*Library, error)
    List(ctx, filter) ([]*Library, error)
}
```

Cela couplait tout appelant en lecture seule - l'écran de liste, le surveillant
de dossiers, l'indexeur de recherche - à la surface d'écriture. Deux problèmes
concrets en découlaient. Les doublures de test enflaient : un test en lecture
seule devait tout de même implémenter la création, la mise à jour et la
suppression, et une erreur dans ces bouchons masquait de vrais bugs. Et la
frontière hexagonale fuyait : un service annoncé « en lecture seule » pouvait
malgré tout atteindre la création à travers le port, et une écriture égarée dans
un service de requête compilait sans broncher.

## Décision

Séparer chaque port de dépôt en deux interfaces le long de l'axe CQRS, l'une
pour les écritures, l'autre pour les lectures, et migrer les appelants
progressivement.

**Les trois interfaces pendant la migration.** Le dépôt de commande porte la
création, la mise à jour, la suppression et toute autre mutation. Le dépôt de
requête porte les lectures et les comptages. L'interface historique est
conservée à titre transitoire, définie comme l'assemblage des deux et marquée
dépréciée. Cet assemblage suit automatiquement l'ajout d'une méthode à l'une ou
l'autre moitié, sans duplication écrite à la main ; l'adaptateur Postgres et les
doublures satisfont les trois.

**L'ordre de migration, dépôt par dépôt.** On introduit les deux nouvelles
interfaces à côté de l'ancienne, on migre d'abord les appelants faciles - les
services de lecture prennent le dépôt de requête, les services de commande
prennent celui d'écriture - chacun dans son propre commit, puis on supprime
l'interface historique quand plus personne ne la nomme.

**Pas de migration d'un bloc.** Réécrire tous les appelants en un seul commit a
produit, lors de tentatives précédentes, des conflits de fusion à travers
plusieurs paquets. Migrer appelant par appelant est plus lent sur le papier mais
livre plus vite, parce que chaque commit se relit indépendamment.

## Portée appliquée

Le dépôt de bibliothèques est séparé, le surveillant de dossiers ayant été migré
vers le port de lecture. Celui des éléments multimédias n'est séparé qu'en
partie : sa migration complète reste au backlog, son rayon d'impact étant large.

## Conséquences

- **Les nouveaux appelants choisissent explicitement.** Il n'y a plus de « gros
  port » par défaut : un service veut lire ou écrire, et prend ce qui
  correspond.
- **Les doublures peuvent être étroites.** Une doublure de lecture n'implémente
  que des lectures, et les tests des services de lecture n'ont plus de bouchons
  d'écriture.
- **L'interface historique se retire progressivement.** Un dépôt est « migré »
  quand plus aucun fichier ne la nomme, et on ne bloque pas le travail en cours
  en attendant que tous les dépôts y soient.
- **Le typage coûte un fichier Go de plus par domaine.** C'est acceptable.

## Références

- `internal/domain/library/queries.go` et `commands.go` - les deux ports
- `internal/app/library/watcher.go` - le premier appelant à port étroit

## Amendement (2026-05-23)

Pour les bibliothèques, la migration est **terminée** : l'interface de
transition a été retirée, l'adaptateur Postgres et la doublure satisfaisant
directement les deux ports. La portée décrite plus haut décrit l'état
intermédiaire ; l'assemblage n'existe plus. Le dépôt des éléments multimédias
reste une interface fourre-tout, et cette partie tient toujours.
