---
source_sha: f8234da15cc8b216c892956a508b78f0582b1e6a863d770d646c3bd0bf08a175
---

# ADR-0011 : fournisseurs externes de générique avant le pipeline heuristique

- Statut : acceptée
- Date : 2026-05-04

## Contexte

Trouver un générique localement suppose de décoder l'audio et la vidéo pour en
calculer des empreintes perceptuelles. C'est l'opération la plus coûteuse du
scanner, et pour les contenus populaires la réponse est déjà connue : des bases
communautaires publient les horodatages de générique de début et de fin.

## Décision

Un port du domaine, `IntroMetadataProvider`, est interrogé **en premier**, avant
la passe sur les chapitres MKV et avant toute détection locale. Quand il répond,
tout le reste du pipeline est sauté : c'est là tout l'intérêt.

Derrière le port se trouvent deux sources communautaires chaînées, la première
réponse non vide l'emportant, un cache adossé à Postgres, et un limiteur de
débit qui lit les en-têtes de quota restant et se met en pause avant d'essuyer
un refus plutôt qu'après.

La durée de vie du cache dépend du résultat. Une réponse trouvée est conservée
indéfiniment ; une réponse négative expire au bout de trente jours, pour qu'un
contenu documenté plus tard par la communauté ressorte au lieu d'être marqué
absent pour toujours.

Reverser ses propres détections est facultatif, et n'envoie que les résultats
dont le détecteur local est très sûr. Un flot de contributions faibles
dégraderait la base que tout le monde lit.

## Conséquences

- Sur un contenu couvert, le scanner ne décode ni audio ni vidéo, ce qui est
  l'économie dominante sur une machine modeste.
- Ajouter une source revient à implémenter le port et à rejoindre la chaîne ; le
  pipeline ne change pas.
- Le prédicat de saut doit savoir quelles sources font autorité, sans quoi un
  nouveau scan retraite des éléments déjà couverts. Les résultats heuristiques
  restent remplaçables, ceux de la communauté et des chapitres non.
- Le serveur dépend d'un tiers pour une fonctionnalité qui doit se dégrader sans
  bruit : quand la source est injoignable, le détecteur local tourne comme avant.
