---
source_sha: 8b11c96da5123c6f0e5e8cde1d8f7783a38cf429f2a42a58f1c2faaa270cbbe3
---

# ADR-0001 : architecture hexagonale (ports et adaptateurs)

- Statut : acceptée
- Date : 2026-01-15

## Contexte

Evey vise un produit auto-hébergé de longue durée, qui doit évoluer tout en restant testable. Il nous faut pouvoir remplacer les implémentations d'entrées-sorties (Postgres, Meilisearch, ffmpeg, TMDB, OMDb, mkvgo, SMTP, push web) sans réécrire la logique métier, et disposer de tests unitaires qui s'exécutent en quelques millisecondes sans lancer Docker.

## Décision

Adopter une architecture hexagonale avec un découpage strict en couches :

- `internal/domain/<contexte>/` contient les modèles purs et les **ports** (interfaces Go). Aucun framework, pas de `database/sql`, pas de HTTP.
- `internal/app/<contexte>/` contient les services applicatifs. Ils ne dépendent que des ports du domaine.
- `internal/adapters/driven/` implémente les ports sortants (postgres, meilisearch, ffmpeg, mkvgo, opensubtitles, filesystem, email, webpush, webhooksender, pluginrunner).
- `internal/adapters/driving/` implémente les ports entrants (grpcapi et grpc-gateway, http, pluginhost).
- La composition se fait dans `cmd/*/wire.go`. Rien d'autre n'instancie un adaptateur concret.

## Conséquences

- Chaque port du domaine est livré avec une doublure utilisée dans les tests unitaires ; les tests au niveau des adaptateurs couvrent la vraie infrastructure séparément.
- Les nouveaux contextes délimités suivent le même gabarit : `domain/<ctx>` plus `app/<ctx>` plus les ajouts d'adaptateurs.
- La collaboration entre contextes passe par les interfaces des services applicatifs ou par le bus d'événements partagé, jamais par un import direct de domaine.
- Le coût est un peu de code répétitif (interface, doublure, câblage), accepté en échange de la testabilité et de la capacité à évoluer.

## Références

- [DEV.md](../../DEV.md)
- [docs/ARCHITECTURE.md](../ARCHITECTURE.md)
