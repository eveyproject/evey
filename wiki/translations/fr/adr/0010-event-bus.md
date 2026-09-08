---
source_sha: ad3d92e798fa4319faa760064b48c0b809fb1dd2ab8b94f3d5bc21fbd6b7f6e3
---

# ADR-0010 : bus d'événements en mémoire avec événements de domaine typés

- Statut : acceptée
- Date : 2026-04-20

## Contexte

Les contextes doivent réagir les uns aux autres : ajouter un élément déclenche
l'enrichissement des métadonnées, la génération du trickplay, la détection du
générique, l'invalidation d'un cache de flux et des notifications. Appeler ces
services directement rendrait `library` dépendant de cinq paquets qui ne le
regardent pas, et la règle hexagonale l'interdit.

## Décision

La coordination entre contextes passe par `shared.EventBus`, et les événements
sont des structures typées, possédées par le domaine qui les émet, et non des
noms de chaînes avec une charge utile opaque. L'interface tient en deux
méthodes, `Publish` et `Subscribe`.

L'implémentation vit dans le processus, avec une goroutine par abonné, câblée
une seule fois à la racine de composition. Un consommateur lent ne bloque donc
jamais un producteur, et le bus n'ouvre aucune transaction de son côté : c'est
au gestionnaire de le faire.

Une structure plutôt qu'une map signifie qu'un abonné qui se trompe de champ ne
compile pas, et que les champs portés par un événement se découvrent à côté du
modèle qui l'émet.

## Conséquences

- Les producteurs ne connaissent pas leurs abonnés : ajouter une réaction à un
  événement existant ne touche que le paquet qui réagit.
- La livraison est au mieux et en mémoire : un abonné qui panique est récupéré
  et journalisé, et les événements ne survivent pas à un redémarrage. Ce qui ne
  doit pas être perdu s'écrit dans la même transaction que l'état qu'il décrit,
  au lieu d'être publié et espéré.
- Tout ce qui pourrait être un événement n'en est pas un. L'indexation de
  recherche est un appel direct au port depuis le service de scan, parce que
  l'index doit être cohérent avec la ligne qui vient d'être écrite, et non
  cohérent avec elle à terme.
