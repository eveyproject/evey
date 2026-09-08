---
source_sha: 274ccd513e4dba5c6623e93ecb5dd21c596c63169f454f787836023a6ba714d8
---

# ADR 0026 : accès distant déduit d'une URL publiée, sauf choix explicite de l'opérateur

Date : 2026-07-30
Statut : acceptée

## Contexte

Evey possède un verrou réseau : quand l'accès distant est coupé, tout client
dont l'adresse ne figure pas dans les réseaux déclarés locaux est refusé sur
toutes les routes. C'est un choix de déploiement (« cette instance est pour mon
réseau »), pas une couche de sécurité : c'est l'authentification qui protège la
médiathèque.

Le verrou était coupé par défaut, et cette valeur portait deux sens que rien
dans le système ne distinguait : *l'opérateur veut une instance limitée au
réseau local*, et *personne n'a jamais touché ce réglage*. Sur un serveur publié
derrière un reverse proxy, c'est presque toujours la seconde lecture qui est la
bonne.

Cette ambiguïté a privé un foyer d'accès pendant deux jours le 29-07-2026. Le
verrou dormait depuis mars parce que le proxy ne transmettait pas la vraie
adresse du client : toutes les requêtes semblaient privées et passaient. Le jour
où la configuration du proxy a rendu les adresses véridiques, tous les clients
distants ont pris un 403 sur chaque route, sans qu'aucun réglage applicatif
n'ait changé. La panne a produit : aucun journal côté serveur, donc rien à
chercher pour l'opérateur ; une application qui affichait des clés de traduction
brutes, parce que le catalogue était verrouillé lui aussi, ce qui se lit comme
« l'application est cassée » et non « le serveur me refuse » ; et un symptôme
(« ça marche sur ton téléphone, pas sur le mien ») qui désigne le client.

## Décision

1. **Déduire la valeur par défaut du fait que le serveur soit publié.** Au
   démarrage, l'accès distant est activé quand l'URL externe ou l'URL de base
   désigne une adresse publiquement joignable. Un serveur que vous avez publié
   est fait pour être joint.
2. **Ne jamais passer outre un choix explicite.** Un indicateur retient qu'un
   humain a décidé, que ce soit par variable d'environnement ou depuis le
   panneau d'administration. La déduction ne remplit qu'un blanc. Sans cet
   indicateur, l'ambiguïté se déplacerait au lieu de disparaître.
3. **Le verrou ne doit jamais être silencieux.** La décision est journalisée au
   démarrage, avec un avertissement qui nomme le remède quand un serveur publié
   refuse tout le monde ; les refus sont journalisés avec l'adresse, le chemin
   et l'agent, limités en débit par adresse ; et le diagnostic en ligne de
   commande signale « publié mais refusant » comme un échec.
4. **Un refus doit rester explicable au client.** Le point de santé, l'état de
   l'installation et le catalogue de traductions échappent au verrou : aucun ne
   sert de contenu ni n'accepte d'identifiants, et verrouiller les traductions
   détruit le seul canal qui reste pour expliquer le refus. La réponse porte un
   code stable et un en-tête dédié, que les clients web, mobile et TV
   transforment en message nommé.

## Conséquences

- Une instance publiée dont l'opérateur n'a jamais touché le réglage sert
  désormais les clients distants par défaut. C'est un **changement de valeur par
  défaut vers l'ouverture**, et il est délibéré : le verrou est une préférence
  de déploiement, et toute route derrière lui exige de toute façon une
  authentification. Qui veut un serveur publié mais limité au réseau local le
  dit une fois, et la déduction n'y touche plus jamais.
- L'indicateur de choix explicite doit être persisté et posé par les deux
  chemins d'écriture. Le supprimer en « simplifiant » restaurerait en silence
  l'ambiguïté qui a causé la panne : des tests le figent.
- La liste des exemptions est une surface de sécurité. Y ajouter un chemin doit
  se justifier sur les mêmes critères qu'ici : il ne sert aucun contenu,
  n'accepte aucun identifiant, et le verrouiller casserait la capacité à
  expliquer un refus.
- Diagnostiquer la moitié « reverse proxy » réclame toujours l'opérateur : des
  adresses clientes correctes supposent que le proxy les transmette et qu'il
  soit déclaré de confiance. La page sur le reverse proxy explique que rendre
  les vraies adresses visibles est précisément ce qui réveille ce verrou.
