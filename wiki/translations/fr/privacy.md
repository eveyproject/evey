---
source_sha: fb180e7c354b7b1181919d491a0e86ea3cfefb7aa152bd9ae1c1411c152d9c92
reviewed: true
---
# Règles de confidentialité

Dernière mise à jour : 2026-10-04.

Ces règles couvrent les applications Evey pour Android et Android TV. Le projet
Evey n'exploite aucun service qui reçoit vos données : pas de compte chez nous,
pas de mesure d'audience, pas de publicité, pas de rapports de plantage envoyés
chez nous. Les applications parlent au serveur Evey que vous, ou la personne
qui vous a invité, avez installé et faites tourner.

## Ce que les applications envoient, et à qui

**À votre serveur Evey, et à rien d'autre que nous exploiterions.** Les
applications envoient ce dont un lecteur multimédia a besoin pour fonctionner :

- votre nom d'utilisateur et votre mot de passe à la connexion (ou le code de
  **Connexion rapide**), puis un jeton de session ;
- ce que vous parcourez, cherchez, lisez, notez, mettez en favori ou demandez,
  et votre position de lecture, pour reprendre sur un autre appareil ;
- le nom de l'appareil (tel qu'Android le donne), pour distinguer vos
  appareils dans **Lire sur** et dans la liste des sessions du serveur ;
- des rapports techniques de lecture : les formats audio et vidéo que
  l'appareil sait décoder, et les erreurs rencontrées pendant la lecture, pour
  que le serveur choisisse un flux que l'appareil peut lire ;
- une image, seulement quand vous en choisissez une comme avatar de profil, et
  les fichiers que vous choisissez vous-même (un sous-titre externe, un plugin,
  une sauvegarde) en tant qu'administrateur.

Qui peut lire ces données dépend de la personne qui exploite ce serveur. Le
projet Evey n'y a pas accès.

**À des tiers, seulement dans ces cas :**

- **Affiches et pochettes.** Certaines images affichées dans les demandes de
  médias et dans la musique viennent directement de leur source publique (The
  Movie Database, le Cover Art Archive, IMDb). L'appareil les récupère comme le
  ferait un navigateur, ce qui révèle son adresse IP à ce site.
- **Cast.** Caster vers un Chromecast passe par Google Cast, inclus dans les
  services Google Play de votre téléphone. Le Chromecast charge la page de
  réception Evey depuis `cast.eveyproject.org`, hébergée par GitHub Pages. Le
  média lui-même arrive de votre serveur.
- **Recherche vocale sur Android TV.** Le bouton micro passe la main à la
  reconnaissance vocale du téléviseur (le plus souvent celle de Google).
  L'application reçoit le texte reconnu, jamais l'enregistrement.
- **Liens que vous ouvrez.** Une bande-annonce, une page The Movie Database ou
  IMDb, ou la page Play Store d'une mise à jour s'ouvre dans l'application ou
  le navigateur correspondant, selon les règles de ce service.
- **Plugins.** Les plugins sont installés par l'administrateur du serveur. Un
  plugin peut afficher du contenu venant d'un service extérieur (un plugin
  météo, par exemple).

## Ce qui reste sur l'appareil

L'adresse du serveur, votre jeton de session (dans le stockage chiffré
d'Android), vos préférences, et les films et la musique téléchargés pour une
lecture hors ligne. Désinstaller l'application les supprime. Le déverrouillage
biométrique (empreinte ou visage), quand vous l'activez, est géré par Android :
l'application apprend seulement s'il a réussi.

## Autorisations

| Autorisation | Pourquoi |
| --- | --- |
| Internet, état du Wi-Fi | Joindre votre serveur, distinguer le réseau de la maison de l'extérieur. |
| Appareil photo | Scanner le QR code de la **Connexion rapide**. Rien n'est enregistré ni envoyé. |
| Notifications, service au premier plan | Afficher la progression de la lecture et des téléchargements, continuer la lecture écran éteint. |
| Biométrie | Verrouillage facultatif de l'application. |

L'application pour téléphone n'a pas l'autorisation d'utiliser le micro.

## Ce site

`eveyproject.org` est hébergé par GitHub Pages, qui voit l'adresse IP du
visiteur. Le site sert lui-même ses polices et ses scripts : le navigateur ne
contacte aucun autre service. Il ne dépose aucun cookie qui lui soit propre et
ne mesure pas l'audience.

## Enfants

Les applications ne s'adressent pas aux enfants. Les profils enfants sont créés
par la personne qui exploite le serveur, et c'est elle qui détient ces données.

## Contact

Questions sur ces règles : [contact@eveyproject.org](mailto:contact@eveyproject.org).
Pour les données conservées sur un serveur Evey, adressez-vous à la personne
qui l'exploite.

<!-- sources
no tracker: frontend/apps/mobile/package.json, frontend/apps/tv/package.json
permissions: frontend/apps/mobile/android/app/src/main/AndroidManifest.xml
device name: frontend/apps/mobile/src/hooks/useDeviceRegistration.ts
playback reports: frontend/packages/client/src/native/useAudioCapsBeacon.ts
outside posters: internal/app/mediarequest/service.go - trustedPosterHosts
cover art: frontend/packages/client/src/lib/artwork.ts - coverArtUri
cast receiver: docs/adr/0025-server-driven-cast.md
voice search: frontend/packages/voice-search/android/src/main/java/com/evey/voicesearch/EveyVoiceSearchModule.kt
store page: frontend/apps/tv/src/lib/appUpdate.ts - openStorePage
-->
