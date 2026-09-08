---
source_sha: da25e9360b1f17ecd4c14dd2057f9da09777b72c230c0965ea83e40c23912ee5
---

# ADR 0023 : fenêtre de grâce à la rotation des jetons de rafraîchissement (sessions sœurs pour les perdants de course)

Date : 2026-07-18
Statut : acceptée
Remplace : la position « rotation stricte à usage unique » d'avril 2026

## Contexte

Les jetons de rafraîchissement tournent à chaque usage : `/auth/refresh` consomme
le jeton présenté et en délivre une nouvelle paire. Les navigateurs partagent UN
seul cookie HttpOnly de rafraîchissement entre tous les onglets. Quand plusieurs
onglets se rafraîchissent en même temps (leurs jetons d'accès expirant tous
ensemble après une période d'inactivité), une seule rotation peut l'emporter ; les
perdants présentent un jeton consommé quelques millisecondes plus tôt.

Le pipeline avait déjà relâché un cran de sévérité : la détection de rejeu purgeait
à l'origine *tout* le jeu de sessions de l'utilisateur, et chaque purge observée en
préproduction s'est révélée être un faux positif (un perdant de course ou une
rotation interrompue), si bien que la purge a été retirée. Mais le perdant, lui,
était toujours rejeté par un 401, ce qui tue l'onglet net : tous les appels d'API
échouent (profil, notifications, images) jusqu'à un rechargement manuel. Observé en
conditions réelles le 18-07-2026, cassant la page des demandes de médias. Une
décision antérieure (avril 2026) avait accepté cela comme une gêne de
développement ; il s'est avéré que cela touchait de vrais usages.

## Décision

Un jeton de rafraîchissement ayant tourné reste échangeable pendant **30 secondes**
après sa rotation (`refreshRotationGrace`). Dans cette fenêtre, celui qui le
présente est traité comme un perdant de course multi-onglets et reçoit une
**session sœur** : sa propre paire fraîche d'accès et de rafraîchissement. Les
jetons du gagnant ne sont pas touchés ; les deux sessions vivent ensuite
indépendamment.

Mécanique :

- La rotation horodate `refresh_tokens.used_at` (migration 000135) et la ligne
  réclamée est **conservée** au lieu d'être supprimée : il faut qu'elle reste
  trouvable pour la vérification de la grâce. `DeleteExpired` balaie les lignes
  utilisées une fois la grâce largement passée.
- `validateRefreshToken` n'accorde la grâce que si `used_at` tombe dans la
  fenêtre. Une ligne utilisée **sans** horodatage (ligne héritée, réclamée avant
  l'existence de la colonne) échoue et refuse. Passé la fenêtre, le jeton est un
  rejeu et il est rejeté, sans purger les autres sessions, conformément à la
  décision antérieure.
- Un concurrent qui perd la réclamation atomique *en vol* (il avait validé la
  ligne avant que le gagnant ne valide sa transaction) retombe sur la même
  délivrance de session sœur : il a perdu de quelques millisecondes, très
  largement dans n'importe quelle grâce.

## Conséquences

- N onglets qui se rafraîchissent en même temps réussissent tous, avec N sessions
  distinctes (`TestRefreshToken_ConcurrentMultiTabRefresh_AllGetDistinctSessions`
  fige ce comportement ; l'ancien test de TOCTOU affirmait un gagnant unique et a
  été réancré).
- **Compromis assumé** : un jeton de rafraîchissement volé ET rejoué dans les 30
  secondes suivant une rotation légitime donne une session. En dehors de cette
  fenêtre, rien ne change. C'est le compromis de tolérance au rejeu standard dans
  l'industrie, et il est strictement borné, contrairement au 401 tueur d'onglet
  dont la gêne, elle, n'était bornée par rien.
- Les lignes de session augmentent d'une unité par perdant de course, ce que
  bornent le balayage et l'expiration des jetons.
