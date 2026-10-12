---
source_sha: be9fbbf14af7dccd420515b33353e89b5ac1154b0fec5dc22468319328e97a6c
reviewed: true
---
# Principes de conception

Sept contraintes que le code respecte, chacune avec les pages et les
décisions d'architecture où elle apparaît. Lisez cette page pour comprendre
pourquoi Evey se comporte comme il le fait, avant de lire comment il est
construit ([Architecture](ARCHITECTURE.md)) ou pourquoi un choix structurel a
été fait ([décisions d'architecture](adr/README.md)). Quand deux principes
entrent en conflit, le premier de la liste l'emporte.

| Principe | Où il apparaît | Décidé par |
| --- | --- | --- |
| 1. Un accès qui ne peut pas être vérifié est refusé | contrôle parental, installation des plugins, accès distant | [ADR-0026](adr/0026-remote-access-inferred-from-a-published-url.md) |
| 2. Le code tiers est confiné, pas considéré comme fiable | sandbox des plugins | [ADR-0012](adr/0012-plugin-token-deny-by-default.md), [ADR-0014](adr/0014-landlock-plugin-sandbox.md) |
| 3. L'original est servi chaque fois que c'est possible | décision de lecture | [ADR-0030](adr/0030-one-plan-one-decision.md) |
| 4. Les décisions reposent sur ce qui a été prouvé, pas sur des noms | cast, accélération matérielle | [ADR-0029](adr/0029-transcoding-routes-are-measured-not-assumed.md) |
| 5. Une lecture, une décision | plan de lecture, Infos du flux | [ADR-0030](adr/0030-one-plan-one-decision.md), [ADR-0024](adr/0024-playback-plan-resolver.md) |
| 6. Le serveur se dimensionne à la machine | calibration, travail de fond | - |
| 7. Peu de réglages, chacun à un seul endroit | installation, panneau d'administration | [ADR-0028](adr/0028-install-is-one-compose-file.md) |

## 1. Un accès qui ne peut pas être vérifié est refusé

Quand une décision ouvre un accès, une erreur aboutit à la réponse la plus
restrictive. Un profil dont les réglages parentaux ne peuvent pas être lus ne
reçoit aucun contenu ; un plugin dont le code ne peut pas être analysé n'est
pas installé ; l'accès distant suit un choix explicite une fois qu'il a été
fait, et l'indique clairement quand il refuse tout le monde. Accès distant et
authentification sont deux choses distinctes : le premier est un choix de
déploiement, la seconde est ce qui protège la bibliothèque.

Voir [Utilisateurs, profils et contrôle parental](administration/users.md),
[Accès distant et HTTPS](administration/remote-access.md),
[ADR-0026](adr/0026-remote-access-inferred-from-a-published-url.md).

## 2. Le code tiers est confiné, pas considéré comme fiable

Un plugin obtient les permissions que son manifeste déclare et rien d'autre,
est analysé avant l'installation, s'exécute dans une sandbox sans accès
réseau direct, et détient un jeton refusé par défaut. Ce que la sandbox ne
peut toujours pas empêcher est écrit explicitement plutôt que sous-entendu.

Voir [Sandbox des plugins et modèle de menace](SECURITY.md),
[ADR-0012](adr/0012-plugin-token-deny-by-default.md),
[ADR-0014](adr/0014-landlock-plugin-sandbox.md).

## 3. L'original est servi chaque fois que c'est possible

Le résultat préféré est le fichier tel quel, puis les mêmes pistes dans un
autre conteneur, puis un transcodage limité aux pistes qui en ont besoin. La
qualité n'est pas quelque chose que le spectateur doit choisir avant de
lancer la lecture : le serveur conserve la qualité de la source sauf si une
contrainte réelle (l'appareil, la connexion, une limite) l'en empêche.

Voir [Comment la lecture est décidée](playback/how-playback-is-decided.md).

## 4. Les décisions reposent sur ce qui a été prouvé, pas sur des noms

Les capacités d'un lecteur viennent de ce que son appareil annonce, pas d'une
liste d'appareils connus. Une carte graphique est utilisée pour ce qu'elle a
réussi pendant la mesure, pas pour ce que son modèle devrait savoir faire.
Quand une capacité ne peut pas être prouvée, le serveur prend la réponse
prudente : un récepteur cast inconnu reçoit les capacités du plus ancien, et
les formats de pixels que les lecteurs n'annoncent pas encore suivent une
règle de compatibilité fixe. Supposer trop donne un écran noir ou des
couleurs fausses ; supposer trop peu coûte un transcodage.

Voir [Cast](playback/casting.md), [Accélération matérielle](administration/hardware-acceleration.md),
[ADR-0029](adr/0029-transcoding-routes-are-measured-not-assumed.md).

## 5. Une lecture, une décision

Une lecture est décidée une fois, à son démarrage, et chaque manifeste et
chaque segment de la session sont servis à partir de cette décision. Le
lecteur ne reçoit jamais des segments de deux encodages, et ce que rapportent
les Infos du flux dérive du même plan de diffusion que celui qu'utilise le
pipeline de lecture, vérifié par des tests dans les deux sens.

Voir [Pipeline de lecture](playback-serving.md),
[ADR-0030](adr/0030-one-plan-one-decision.md),
[ADR-0024](adr/0024-playback-plan-resolver.md).

## 6. Le serveur se dimensionne à la machine

Le serveur mesure combien il peut transcoder et n'admet pas davantage : une
petite machine lit moins de flux transcodés, plutôt que de tous les lire
mal. La lecture passe en premier ; le travail de fond se met en pause pendant
que quelqu'un regarde. La même image tourne sur un NAS et sur un serveur à
plusieurs cartes graphiques.

Voir [Transcodage et capacité du serveur](playback/transcoding.md),
[Travail de fond et charge du serveur](operations/background-work.md).

## 7. Peu de réglages, chacun à un seul endroit

Ce qui doit exister avant le démarrage du serveur (le stockage, les secrets,
l'adresse publique) est dans le fichier compose ; tout ce qui peut changer
ensuite est dans le panneau d'administration, stocké en base, et s'applique
sans redémarrage partout où rien ne l'exige. La détection et la mesure
remplacent les réglages partout où c'est possible : une installation, c'est
un fichier compose, des bibliothèques et des utilisateurs.

Voir [Installation](installation.md), [Référence de configuration](configuration.md),
[ADR-0028](adr/0028-install-is-one-compose-file.md).

## Décisions d'architecture

Les choix structurels sont consignés dans des décisions d'architecture
(ADR), avec le contexte, les options pesées et les conséquences. Elles sont
conservées avec le code : [docs/adr](adr/README.md).

<!-- sources
fail closed: internal/adapters/driving/http/parental.go - BlockedByParental ; internal/app/plugin/install.go
contained plugins: docs/SECURITY.md ; docs/adr/0012-plugin-token-deny-by-default.md ; docs/adr/0014-landlock-plugin-sandbox.md
original first, one decision: internal/app/transcoding/decision.go ; internal/app/transcoding/route_tonemap_report_test.go ; docs/adr/0030-one-plan-one-decision.md
proved capabilities: frontend/packages/media-caps/src/castUrl.ts - castProfileFor ; docs/adr/0029-transcoding-routes-are-measured-not-assumed.md
sizing: internal/app/transcoding/calibration_service.go ; i18n admin.serverConfig.cpuBudget.globalDesc
-->
