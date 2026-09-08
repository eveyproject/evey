---
source_sha: 6317b9ff7aea2812480b54059afb928ddcbc3bcb28de18e7235c881915c7866d
---

# Décisions d'architecture

Les décisions structurantes d'Evey, numérotées à la suite.

**On amende un fait, on remplace une décision.** Un tiers des entrées portent une
section datée `## Amendment` ou `## Update`, et c'est la pratique voulue : quand
le texte s'est révélé décrire le code de travers, la correction est ajoutée avec
sa date et le corps reste tel qu'il a été écrit, si bien qu'un lecteur voit à la
fois ce qu'on croyait et ce qu'on a constaté. Quand c'est la *décision* qui
change, l'entrée n'est pas modifiée du tout : une nouvelle décision la remplace
et le dit. La règle porte sur la décision, pas sur la prose : corriger une
affirmation fausse est de l'honnêteté, réécrire l'histoire non.

**Ce qui mérite une décision d'architecture.** Un choix coûteux à défaire, et dont
la raison ne se retrouve ni dans le code ni dans un test. Tout le reste va
ailleurs, à moindre frais : une règle d'écriture dans
[code-conventions.md](../code-conventions.md), une intention non implémentée dans
[docs/design/](../design/), et une justification locale dans un commentaire à côté
du code qu'elle explique. Une décision qui consigne ce qu'un lecteur pouvait
déduire ajoute une page à maintenir et rien à comprendre.

**Les numéros ne sont jamais réutilisés.** Une entrée retirée laisse un trou
plutôt que de céder son numéro à une future décision, car d'anciens commits, des
commentaires et des revues y renvoient encore.

| # | Titre | Date | Statut |
| --- | --- | --- | --- |
| [0001](0001-hexagonal-architecture.md) | Architecture hexagonale (ports et adaptateurs) | 2026-01-15 | Acceptée |
| [0002](0002-transactional-unit-port.md) | Port d'unité transactionnelle | 2026-04-13 | Acceptée |
| [0003](0003-plugin-scoped-tokens.md) | Jetons JWT limités à une extension | 2026-02-20 (amendée le 2026-05-23) | Acceptée |
| [0004](0004-proto-deprecation-strategy.md) | Stratégie de dépréciation des protos | 2026-04-16 | Acceptée |
| [0005](0005-two-layer-authorization.md) | Autorisation à deux couches (RBAC dans l'intercepteur, vérification de la ressource dans le gestionnaire) | 2026-04-20 | Acceptée |
| [0006](0006-scanner-change-detection.md) | Contrat de détection de changement du scanner | 2026-04-20 (amendée le 2026-05-23) | Acceptée |
| [0009](0009-cqrs-repo-split.md) | Séparation CQRS des dépôts | 2026-04-20 (amendée le 2026-05-23) | Acceptée |
| [0010](0010-event-bus.md) | Bus d'événements en mémoire avec événements de domaine typés | 2026-04-20 (amendée le 2026-05-23) | Acceptée |
| [0011](0011-intro-detection-providers.md) | Fournisseurs externes de générique avant le pipeline heuristique | 2026-05-04 (amendée le 2026-05-23) | Acceptée |
| [0012](0012-plugin-token-deny-by-default.md) | Autorisation refusée par défaut pour les jetons d'extension | 2026-05-23 | Acceptée |
| [0013](0013-userspace-plugin-resource-monitor.md) | Moniteur de ressources des extensions en espace utilisateur (abandon des cgroups) | 2026-05-23 | Acceptée |
| [0014](0014-landlock-plugin-sandbox.md) | Bac à sable Landlock, appliqué par une amorce de lancement | 2026-05-24 | Acceptée |
| [0015](0015-hardware-acceleration.md) | Une seule image pour tous les GPU, pilotes injectés à l'exécution | 2026-06-01 | Acceptée |
| [0016](0016-mkv-fastpath-prober.md) | Chemin rapide mkvgo en mémoire pour la sonde de scan MKV (ffprobe reste l'autorité) | 2026-06-03 | Acceptée |
| [0017](0017-software-hdr-tonemap-delivery-resolution.md) | Tonemap logiciel HDR→SDR à la résolution de diffusion, pas à celle de la source | 2026-06-04 | Acceptée |
| [0018](0018-cmcd-qoe-pipeline.md) | Télémétrie de qualité fondée sur CMCD, avec un collecteur non bloquant qui jette quand il est plein | 2026-06-14 | Acceptée |
| [0019](0019-token-epoch-revocation.md) | Révocation par époque de jeton pour des JWT sans état | 2026-06-14 | Acceptée |
| [0020](0020-hls-native-audio-rendition.md) | Piste audio native HLS (EXT-X-MEDIA) pour le changement de langue | 2026-06-14 | Acceptée |
| [0021](0021-plugins-on-tv.md) | Extensions sur TV : portage WebView maintenant, rendu natif SDUI plus tard | 2026-07-09 | Acceptée |
| [0022](0022-hls-decision-pinning-and-av-resync.md) | Épinglage de la décision HLS (vd= et ad=) et resynchronisation audio-vidéo (annulation du retard source et décalage utilisateur) | 2026-07-09 (amendée le 2026-07-19) | Acceptée |
| [0023](0023-refresh-rotation-grace.md) | Fenêtre de grâce à la rotation des jetons de rafraîchissement (sessions sœurs pour les perdants de course multi-onglets) | 2026-07-18 | Acceptée |
| [0024](0024-playback-plan-resolver.md) | Résolveur de plan de lecture dans la couche applicative (une résolution par requête) | 2026-07-18 | Acceptée |
| [0025](0025-server-driven-cast.md) | Cast piloté par le serveur avec un récepteur web dédié (identifiant d'application D36A449B) | 2026-07-25 | Acceptée |
| [0026](0026-remote-access-inferred-from-a-published-url.md) | Accès distant déduit d'une URL publiée, sauf choix explicite de l'opérateur | 2026-07-30 | Acceptée |
| [0028](0028-install-is-one-compose-file.md) | L'installation tient dans un fichier compose | 2026-08-29 | Acceptée |

## Retirées

| # | C'était | Où c'est parti | Pourquoi |
| --- | --- | --- | --- |
| 0007 | Cycle de vie du contexte des goroutines détachées | [code-conventions.md](../code-conventions.md#fire-and-forget-goroutines) | Une règle d'écriture, réversible fichier par fichier, tenue par la revue et par les gardes. |
| 0008 | Motif de constructeur des agrégats de domaine | [code-conventions.md](../code-conventions.md#domain-aggregate-constructors) | Pareil : un motif à suivre, pas une décision à défendre. |
| 0027 | Mode démonstration par drapeau de démarrage | [design/demo-mode.md](../design/demo-mode.md) | Proposée, jamais implémentée. Elle consignait une intention, pas une décision que le code incarne. |

Voir aussi [docs/ARCHITECTURE.md](../ARCHITECTURE.md) pour la vue d'ensemble.
