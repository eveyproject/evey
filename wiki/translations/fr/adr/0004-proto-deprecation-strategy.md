---
source_sha: 46a468af315b3277edfdbd040e6a8ff59e3617606ebc253afa23c52aafbc4574
---

# ADR-0004 : stratégie de dépréciation des protos

- Statut : acceptée
- Date : 2026-04-16

## Contexte

`proto/evey/v1/` est consommé par les clients web, mobile (React Native) et TV. Ces clients sont livrés indépendamment du serveur : casser un contrat de transport casse donc en silence des installations anciennes déjà déployées. Nous n'avions aucune convention écrite pour retirer un champ ou un RPC, et `deprecated = true` n'avait jamais été utilisé.

## Décision

Utiliser l'option native `deprecated = true` de protobuf comme mécanisme unique pour signaler « ceci va disparaître », selon la politique suivante :

1. **Ne jamais retirer un champ ou un RPC sur place.** On le marque `deprecated = true` et on ajoute un remplaçant. Les anciens clients continuent de fonctionner, les nouveaux adoptent le remplaçant.
2. **Ne jamais réutiliser un numéro de champ.** Une fois un tag attribué, il est brûlé pour toujours. Si un champ est retiré, on réserve son tag : `reserved 7; reserved "old_name";`.
3. **Déprécier pendant au moins deux versions serveur** avant de retirer physiquement. Cela laisse aux clients une vraie fenêtre pour migrer.
4. **Pas de paquet `v2/`** tant qu'il n'y a pas de refonte de toute l'API. La dépréciation champ par champ ou RPC par RPC est préférable : un changement de version majeure est un événement en soi.
5. Documenter le remplaçant dans le commentaire proto au-dessus du membre déprécié :
   ```proto
   // Deprecated: use `max_bitrate_cellular` (added in v0.42). Will be removed
   // after v0.45.
   int32 legacy_max_bitrate = 7 [deprecated = true];
   ```
6. Le code Go et TypeScript généré émet un avertissement de dépréciation à partir de l'option `deprecated = true`, si bien que les IDE et les linters font remonter les usages automatiquement. Aucun outillage supplémentaire n'est nécessaire.

## Conséquences

- Les changements cassants deviennent visibles en revue de PR, à la ligne `[deprecated = true]` du diff.
- Les PR de retrait sont des diffs triviaux, il suffit de supprimer le champ et d'ajouter l'entrée `reserved`, puisque la fenêtre de dépréciation a déjà couru.
- Le fichier proto grossit avec le temps du fait des entrées réservées ; c'est acceptable, car il est le journal des modifications qui fait autorité sur le contrat de transport.
- Les clients qui ignorent les avertissements de dépréciation continuent de fonctionner quand le membre déprécié est retiré, tant qu'ils ne lisent que des champs connus. Protobuf ignore les champs inconnus par conception.

## Références

- [Dépréciation d'un champ protobuf](https://protobuf.dev/programming-guides/proto3/#options)
- [Guide de versionnement des API Google](https://cloud.google.com/apis/design/versioning)
- Le générateur Go émet des commentaires `// Deprecated: ...` que la vérification `deprecation` de `go vet` prend en compte.
