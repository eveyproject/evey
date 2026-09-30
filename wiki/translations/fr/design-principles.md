---
source_sha: dffa9823329d7253d7991f9037012a372bdeeb2765f9c4c68f0d73f04efc4dbc
reviewed: true
---
# Principes de conception

Les principes ci-dessous expliquent pourquoi Evey se comporte comme il le
fait. Chacun est une contrainte que le code respecte, avec les pages et les
décisions d'architecture où elle apparaît. Quand deux principes entrent en
conflit, le premier de la liste l'emporte.

## 1. Un accès qui ne peut pas être vérifié est refusé

Quand une décision ouvre un accès, une erreur aboutit à la réponse la plus
restrictive. Un profil dont les réglages parentaux sont illisibles ne reçoit
aucun contenu ; un plugin dont le code ne peut pas être analysé n'est pas
installé ; l'accès distant suit un choix explicite dès qu'il a été fait, et
signale clairement quand il refuse tout le monde. Accès distant et
authentification sont distincts : le premier est un choix de déploiement, la
seconde protège la bibliothèque.

Voir [Utilisateurs, profils et contrôle parental](administration/users.md),
[Accès distant et HTTPS](administration/remote-access.md),
[ADR-0026](adr/0026-remote-access-inferred-from-a-published-url.md).

## 2. Le code tiers est confiné, pas cru sur parole

Un plugin reçoit les permissions que déclare son manifeste et rien d'autre ; il
est analysé avant l'installation, tourne dans une sandbox sans accès direct au
réseau, et détient un jeton refusé par défaut. Ce que la sandbox ne peut pas
empêcher est écrit noir sur blanc plutôt que sous-entendu.

Voir [Modèle de sécurité et sandbox des plugins](SECURITY.md),
[ADR-0012](adr/0012-plugin-token-deny-by-default.md),
[ADR-0014](adr/0014-landlock-plugin-sandbox.md).

## 3. L'original est livré chaque fois que c'est possible

Le résultat privilégié est le fichier tel quel, puis les mêmes pistes dans un
autre conteneur, puis un transcodage limité aux pistes qui l'exigent. Le
spectateur n'a pas à choisir une qualité avant de lancer la lecture : le
serveur conserve la qualité de la source, sauf si une contrainte réelle
(l'appareil, la connexion, une limite) l'en empêche.

Voir [Comment Evey choisit le mode de lecture](playback/how-playback-is-decided.md).

## 4. Les décisions reposent sur ce qui est prouvé, pas sur des noms

Les capacités d'un lecteur viennent de ce que déclare son appareil, pas d'une
liste d'appareils connus. Une carte graphique est utilisée pour ce qu'elle a
réussi lors de la mesure, pas pour ce que son modèle devrait savoir faire.
Quand une capacité ne peut pas être prouvée, le serveur retient la réponse
prudente : un récepteur de diffusion inconnu reçoit les capacités du plus
ancien, et les formats de pixel que les lecteurs ne déclarent pas encore
suivent une règle de compatibilité fixe. Présumer trop donne un écran noir ou
de fausses couleurs ; présumer trop peu coûte un transcodage.

Voir [Diffusion (Chromecast)](playback/casting.md),
[Accélération matérielle](administration/hardware-acceleration.md),
[ADR-0029](adr/0029-transcoding-routes-are-measured-not-assumed.md).

## 5. Une lecture, une décision

Une lecture est décidée une seule fois, au lancement, et tous les manifestes
et segments de la session sont servis à partir de cette décision. Le lecteur
ne reçoit jamais de segments issus de deux encodages, et ce qu'affichent les
Infos du flux provient du même plan que celui qu'exécute l'encodeur, ce que des
tests vérifient dans les deux sens.

Voir [ADR-0030](adr/0030-one-plan-one-decision.md),
[ADR-0024](adr/0024-playback-plan-resolver.md).

## 6. Le serveur se dimensionne à la machine

Le serveur mesure ce qu'il peut transcoder et n'en accepte pas davantage : une
petite machine lit moins de flux transcodés plutôt que de tous les lire mal.
La lecture passe en premier ; les tâches de fond s'arrêtent pendant qu'une
personne regarde. La même image tourne sur un NAS comme sur un serveur doté de
plusieurs cartes graphiques.

Voir [Transcodage et capacité du serveur](playback/transcoding.md),
[Tâches de fond et charge du serveur](operations/background-work.md).

## 7. Peu de réglages, chacun à un seul endroit

Ce qui doit exister avant le démarrage du serveur (stockage, secrets, adresse
publique) se trouve dans le fichier compose ; tout ce qui peut changer ensuite
est dans le panneau d'administration, stocké en base, et s'applique sans
redémarrage partout où rien ne l'impose. La détection et la mesure remplacent
les réglages chaque fois qu'elles le peuvent : une installation, c'est un
fichier compose, des bibliothèques et des utilisateurs.

Voir [Installation](installation.md), [Référence de configuration](configuration.md),
[ADR-0028](adr/0028-install-is-one-compose-file.md).

## Décisions d'architecture

Les choix structurants sont consignés dans des décisions d'architecture (ADR),
avec le contexte, les options examinées et les conséquences. Elles sont
conservées avec le code : [docs/adr](adr/README.md).
