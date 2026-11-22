---
source_sha: bc4e8000ca3b6f13e4b5d57a81bf41b5b314d18f1578b3eb3a7b6737c5d1b091
reviewed: true
---
# Mises à jour des applications servies par l'instance { #app-updates-served-by-the-instance }

Les applications pour téléphone et pour TV chargent leur JavaScript depuis
l'instance Evey à laquelle elles sont connectées. Un correctif du JavaScript
de l'application atteint alors un appareil sans publication sur un store, dès
que son serveur est mis à jour. Les changements natifs en demandent encore
une.

Une application ne fait qu'avancer : elle prend une release plus récente que
celle qu'elle exécute, et garde son propre code face à un serveur plus ancien.

## Comment les pièces s'assemblent { #how-it-fits-together }

- **L'image embarque les bundles.** `make client-updates` construit un bundle
  signé par application dans `client-updates/`, et l'image copie ce dossier
  dans `/app/client-updates` (`EVEY_CLIENT_UPDATES_DIR`). Une image construite
  sans cette étape ne sert aucune mise à jour, et les applications gardent le
  code avec lequel elles ont été construites.
- **Le serveur ne fait que remettre des octets.**
  `GET /api/v1/client-updates/{app}/manifest` et
  `.../assets/{platform}/{runtime}/{name}` parlent le protocole Expo Updates
  (`internal/adapters/driving/http/client_update_handler.go`). L'instance ne
  détient aucune clé, ne construit aucun manifeste, et n'ouvre que des
  fichiers situés dans ce dossier.
- **Les applications vérifient.** Chaque bundle est signé par le projet et
  envoyé avec le certificat de la clé qui l'a signé ; les applications
  contrôlent ce certificat contre la racine compilée en elles,
  `frontend/client-updates/certificate.pem`. Une instance ne peut distribuer
  que du code publié ; tout le reste est refusé sur l'appareil.
- **Les applications ne reviennent jamais en arrière.** Voir « Plancher de
  release » plus bas.
- **Les applications suivent le serveur actif.** Une fois un serveur choisi,
  l'application pointe expo-updates vers lui et télécharge son bundle en
  arrière-plan (`frontend/packages/client/src/native/clientUpdates.ts`). Le
  bundle démarre au lancement suivant. Le téléphone propose un redémarrage
  quand c'est une autre version de l'application que celle qui tourne, jamais
  par-dessus le lecteur ; la TV ne le propose jamais, un rechargement à chaud
  étant trop lourd pour un petit boîtier. Sans serveur choisi, l'application n'interroge
  personne.

## Version du runtime natif { #native-runtime-version }

Un bundle ne fonctionne que sur la build native pour laquelle il a été fait.
Chaque application déclare une `runtimeVersion` dans son `app.json`
(`mobile-1`, `tv-1`), et un bundle est stocké et demandé sous ce nom.

**Montez `runtimeVersion` dans le même changement que tout ce qui est
natif** : un module natif ajouté ou mis à jour, une montée du SDK Expo ou de
React Native, un plugin de configuration, le manifeste Android, ou le code Go
lié dans `offline-audio`. L'application pour téléphone garde son projet
Android dans git, la valeur vit donc aussi dans
`frontend/apps/mobile/android/app/src/main/res/values/strings.xml`.

Une instance n'a aucun bundle pour un runtime avec lequel elle n'a pas été
construite : une build du store plus ancienne ou plus récente garde simplement
son propre code.

## Plancher de release { #release-floor }

Une signature dit qui a publié une release, pas qu'elle est récente. Sans rien
d'autre, n'importe quel serveur, ou quiconque se trouve sur le chemin d'une
connexion HTTP en clair, pourrait remettre à une application une ancienne
release signée et faire revenir un bug corrigé depuis.

- Une release est datée par le commit à partir duquel elle a été construite
  (`createdAt` dans le manifeste signé, date du committer).
- Chaque appareil garde un plancher : la plus récente entre la date du commit
  à partir duquel sa build du store a été faite (enregistrée par Gradle) et
  celle de la release servie la plus récente qui s'y est affichée. Le plancher
  ne fait que monter.
- L'application ne télécharge rien sous le plancher et démarre la release la
  plus récente qu'elle détient, à ce niveau ou au-dessus, quel que soit le
  serveur d'où elle vient. Un serveur qui ne sert rien, ou quelque chose de
  plus ancien, laisse l'application sur ce qu'elle a.
- Le code intégré à l'application a toujours le droit de tourner. C'est lui
  qui démarre quand rien de ce qui est stocké ne convient. Il est daté par son
  commit lui aussi, pas par le moment où l'application a été compilée, si bien
  que la release du même commit, et de tout commit ultérieur, est prise à sa
  place.
- Le JavaScript tient le même plancher de son côté et refuse de s'exécuter en
  dessous.

Ce qui en découle :

- Face à un serveur plus ancien, une application exécute un JavaScript plus
  récent que celui que ce serveur embarquait, comme le fait une build du
  store. L'API du serveur doit rester compatible avec des clients plus
  récents.
- Quand le garde écarte une release qui s'était déjà affichée, celle d'avant
  est en général sous le plancher : l'application exécute son code intégré
  jusqu'à ce que l'instance publie une release plus récente.
- Une build dont le plancher ne peut pas être lu n'accepte aucune mise à
  jour.
- Deux builds d'un même commit sont deux releases de date presque identique.
  Les applications stockent une release par date : si les deux dates
  coïncident, la seconde n'est pas stockée et l'application garde ce qu'elle
  exécute. Construisez une seule fois par release.

Rien de non signé n'oriente tout cela : les applications ignorent les filtres
de manifeste, les en-têtes définis par le serveur, la partie extensions et les
directives, et n'envoient aux serveurs ni identifiant d'installation ni le
texte d'un plantage précédent.

## Quand une mise à jour se comporte mal { #when-an-update-misbehaves }

- Un bundle qui plante avant son premier affichage est écarté par
  expo-updates, qui revient au précédent ou au code intégré à l'application.
- Un bundle qui s'affiche puis meurt à chaque lancement est rattrapé par le
  garde de l'application (`clientUpdateBootGuard`) : après trois lancements de
  suite terminés sans se stabiliser, le suivant demande le même retour.
- Un bundle écarté n'est pas réessayé. L'instance doit en publier un autre.
- Quand le bundle écarté ne laisse rien à démarrer (celui d'avant est sous le
  plancher), l'application redémarre d'elle-même sur son code intégré.

## Le patch d'expo-updates { #the-expo-updates-patch }

`frontend/patches/expo-updates@*.patch` porte ce dont la conception a besoin
et pour quoi expo-updates n'a aucun réglage :

- les adresses d'assets d'un manifeste peuvent être relatives, résolues contre
  l'adresse du manifeste, de sorte qu'un même manifeste signé est valable sur
  toutes les instances ;
- la demande de retour arrière du garde écarte une mise à jour même si elle a
  déjà démarré avec succès ;
- quand cela ne laisse rien à démarrer, l'application est relancée au lieu de
  planter ;
- le plancher de release, et le lancement par release plutôt que par adresse
  de serveur ;
- le code intégré à l'application est daté par son commit ;
- une release déjà stockée est adoptée, et non téléchargée de nouveau, quand
  une autre adresse la sert (la même instance depuis le réseau local et depuis
  l'extérieur) ;
- chaque asset doit porter son empreinte dans le manifeste signé ;
- les certificats sont contrôlés à la plus tardive entre l'horloge de
  l'appareil et le plancher, et une release ne peut pas être datée après
  l'expiration de son certificat de signature ;
- les entrées non signées et les en-têtes d'identification cités plus haut
  sont supprimés.

Refaites le patch quand expo-updates est mis à jour ; la dépendance est
épinglée à la version exacte patchée.

## Ce qu'une build doit avoir { #what-a-build-must-have }

`frontend/scripts/check-expo-updates-patch.mjs` vérifie, pour les deux
applications : que le patch est dans les sources qu'Android compile, que le
certificat et ses métadonnées sont configurés (sans certificat, expo-updates
ne vérifie rien), que le certificat est une racine et que les applications
vérifient à travers elle, que la build enregistre son plancher de release,
qu'aucun réglage n'élargit ce que les applications acceptent, et que la
version du runtime natif correspond à `app.json`. Il s'exécute avant chaque
build Gradle des applications, et dans le gate sur toute branche qui touche
aux dépendances, à leurs patchs, au certificat ou au côté natif d'une
application.

## Clés { #keys }

Deux clés, créées par `frontend/scripts/client-update-certificate.sh` :

- **La racine** (`root <dir>`), une seule fois. Son certificat est
  `frontend/client-updates/certificate.pem`, compilé dans chaque build du
  store ; sa clé est chiffrée par une phrase de passe et reste hors ligne,
  avec une sauvegarde. Elle ne va jamais sur une machine de build. La perdre,
  ou devoir la remplacer, impose une nouvelle build du store des deux
  applications avant qu'une mise à jour puisse de nouveau être servie.
- **Le certificat de signature** (`leaf <dir> [days]`), un an par défaut, émis
  par la racine. Sa clé est ce que détient une build de release.

Renouvelez le certificat de signature avant son expiration : l'outil de
publication en refuse un auquel il reste moins de 30 jours, et les appareils
refusent une release une fois son certificat expiré, si bien qu'une image
cesse de remettre son bundle un an après l'émission de son certificat (les
appareils gardent ce qu'ils détiennent déjà). Il n'y a pas de révocation : une
clé de signature qui a fuité reste valable jusqu'à l'expiration de son
certificat, et seulement pour des releases datées avant celle-ci. Remplacer la
racine est le seul moyen de couper plus tôt.

## Publier une release { #releasing }

1. `EVEY_CLIENT_UPDATE_KEY_FILE=<signing key> EVEY_CLIENT_UPDATE_CERT_FILE=<signing certificate> make client-updates`
   une fois, sur le commit de release avec un arbre propre, avant la
   construction de l'image. Le bytecode Hermes n'est pas reproductible : deux
   exécutions donnent deux mises à jour différentes, donc toutes les
   architectures d'une release doivent embarquer la même sortie. L'outil
   refuse de publier par-dessus une release plus récente.
2. Construisez l'image. La construction échoue sur un bundle non signé,
   construit depuis un autre commit, ou destiné à un runtime que les
   applications ne déclarent plus.

La CI le fait pour les tags de release (`v*.*.*`) seulement, en deux jobs
(`.github/workflows/ci.yml`) :

- l'un construit les bundles non signés, avec toutes les dépendances des
  applications et aucune clé ;
- l'autre les signe, avec la clé et rien d'autre qu'un checkout :
  `frontend/scripts/sign-client-update.mjs` ne charge aucune dépendance, et
  refuse un bundle qui n'est pas celui du commit tagué, pour les runtimes que
  les applications déclarent à ce commit, daté par ce commit, avec des
  fichiers conformes à ce que dit le manifeste.

La construction de l'image de chaque architecture prend ensuite cette même
sortie signée. Un tag dont les bundles ne peuvent pas être signés échoue, il
ne sort pas sans eux.

Ce que la signature ne couvre pas : le job de construction exécute les
dépendances des applications, et l'une d'elles pourrait altérer le JavaScript
avant sa signature. Le job de signature contrôle la forme, le commit et la
date d'un bundle, pas ce que fait son bytecode. La défense, c'est le lockfile
gelé et la relecture de chaque montée de dépendance.

Les images construites depuis une branche ou à la main n'embarquent aucun
bundle. Un bundle signé pour une branche serait accepté par toutes les
applications publiées, depuis n'importe quel serveur, et monterait leur
plancher au-dessus des releases.

Le job de signature s'exécute dans l'environnement GitHub
`client-update-signing`, qui détient les deux secrets
`EVEY_CLIENT_UPDATE_SIGNING_KEY` et `EVEY_CLIENT_UPDATE_SIGNING_CERT` (deux
fichiers PEM, entiers) et ne laisse entrer que les tags `v*.*.*` : une branche
qui modifie le workflow ne peut pas les lire. Remplacez les deux quand le
certificat de signature est renouvelé.

Aucune clé privée n'entre jamais dans le dépôt ni dans une image, et les clés
ne sont jamais fournies autrement que par un chemin de fichier.

<!-- sources
routes and what the server serves: internal/adapters/driving/http/client_update_handler.go ; internal/adapters/driving/http/client_update_handler_test.go ; internal/config/config.go - EVEY_CLIENT_UPDATES_DIR
following the active server, restart prompt, launch guard: frontend/packages/client/src/native/clientUpdates.ts - followClientUpdateSource, clientUpdateBootGuard, clientUpdateMarkSettled ; frontend/packages/client/src/native/clientUpdates.test.ts ; i18n common.clientUpdate.*
floor, launch by release, certificates, dropped inputs: frontend/patches - the expo-updates patch
build checks: frontend/scripts/check-expo-updates-patch.mjs ; Dockerfile
keys and publication: frontend/scripts/client-update-certificate.sh ; frontend/scripts/build-client-update.mjs ; frontend/scripts/sign-client-update.mjs ; .github/workflows/ci.yml (client-updates-build, client-updates-sign)
-->
