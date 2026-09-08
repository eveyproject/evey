---
source_sha: fdd1b9dad1372811d40d9ca8d3437d54a977a9403252fe761be62eb7407bdea8
---

# ADR-0002 : port d'unité transactionnelle

- Statut : acceptée
- Date : 2026-04-13

## Contexte

L'audit du 13-04-2026 a montré que `UserService.CreateUser` n'était pas atomique : il écrivait dans `users`, puis dans `user_preferences`, puis dans `favorites`, par trois appels de dépôt distincts. Une panne entre la deuxième et la troisième écriture laissait des lignes orphelines. Plusieurs autres services applicatifs suivaient le même schéma.

Il nous faut un moyen, pour la couche applicative, de composer des écritures portant sur plusieurs agrégats dans une seule transaction, sans laisser fuir `*sql.Tx` dans le domaine.

## Décision

Introduire un port de domaine `shared.TransactionalUnit` à méthode unique :

```go
type TransactionalUnit interface {
    Do(ctx context.Context, fn func(ctx context.Context) error) error
}
```

L'adaptateur Postgres l'implémente en ouvrant un `sql.Tx` et en le rangeant dans le `ctx` sous une clé typée. Tous les dépôts postgres appellent un utilitaire `txFromCtx(ctx, db)` qui renvoie le `*sql.Tx` ambiant s'il existe, et le pool sinon.

Les services applicatifs enveloppent les écritures en plusieurs étapes :

```go
return s.uow.Do(ctx, func(ctx context.Context) error {
    if err := s.users.Insert(ctx, u); err != nil { return err }
    if err := s.prefs.InsertDefault(ctx, u.ID); err != nil { return err }
    return s.favs.InitFor(ctx, u.ID)
})
```

## Conséquences

- Les dépôts restent légers en dépendances (ils ne dépendent toujours que de `context.Context` et de `sql.DB` ou `sql.Tx` derrière l'utilitaire).
- Les tests injectent une doublure de `TransactionalUnit` qui se contente d'appeler `fn(ctx)` : aucune vraie transaction n'est nécessaire en test unitaire.
- Les appels imbriqués à `Do` réutilisent la transaction existante (l'opération est idempotente).
- Tous les services non atomiques existants doivent migrer. `UserService.CreateUser` est la première migration.

## Références

- `internal/domain/shared/transactional.go`
- `internal/adapters/driven/postgres/txunit.go`
- [ADR-0001](0001-hexagonal-architecture.md)
