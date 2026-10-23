---
source_sha: e527433cfc41c2cba1cfcb773095634c496390297f79ac7e7b368fa36d27d047
reviewed: true
---
# Règles de confidentialité { #privacy-policy }

Dernière mise à jour : 2026-10-04.

Ces règles concernent les applications Evey pour Android et Android TV.

En bref : le projet Evey ne fait tourner aucun service qui reçoit vos données.
Pas de compte chez nous, pas de statistiques d'usage, pas de publicité, pas de
rapports de plantage envoyés chez nous. Les applications communiquent avec le serveur
Evey que vous, ou la personne qui vous a invité, avez installé et faites
tourner.

## Ce que les applications envoient, et à qui { #what-the-apps-send-and-to-whom }

**À votre serveur Evey, et à rien d'autre que nous gérons.** Les applications
envoient ce dont un lecteur de médias a besoin pour fonctionner :

- votre nom d'utilisateur et votre mot de passe quand vous vous connectez (ou
  le code de Connexion rapide), puis un jeton de session, une clé qui
  maintient votre connexion ;
- ce que vous parcourez, cherchez, lisez, notez, mettez en favori ou
  demandez, et l'endroit où vous en êtes dans la lecture, pour reprendre sur
  un autre appareil ;
- le nom de l'appareil (tel qu'Android le donne), pour que vous puissiez
  reconnaître vos appareils dans **Lire sur** et dans la liste des sessions
  du serveur ;
- des rapports techniques de lecture : les formats audio et vidéo que
  l'appareil sait lire, et les erreurs rencontrées pendant la lecture, pour
  que le serveur choisisse un flux que l'appareil peut lire ;
- une image, seulement quand vous en choisissez une comme photo de profil, et
  les fichiers que vous choisissez vous-même (un sous-titre externe, un
  plugin, une sauvegarde) en tant qu'administrateur.

C'est la personne qui gère ce serveur qui décide qui peut lire ces données. Le
projet Evey n'y a pas accès.

**À d'autres services, seulement dans ces cas :**

- **Affiches et pochettes d'album.** Certaines images affichées dans les
  demandes de titres et dans la musique viennent directement de leur source
  publique (The Movie Database, le Cover Art Archive, IMDb). L'appareil les
  récupère comme le ferait un navigateur web, ce qui montre son adresse IP à
  ce site.
- **Cast.** Caster vers un Chromecast passe par Google Cast, qui fait partie
  des services Google Play de votre téléphone. Le Chromecast charge la page de
  réception Evey depuis `cast.eveyproject.org`, hébergée par GitHub Pages. Le
  film ou la musique, lui, arrive directement de votre serveur.
- **Recherche vocale sur Android TV.** Le bouton micro fait appel à la
  reconnaissance vocale de la TV (en général celle de Google). L'application
  reçoit le texte reconnu, jamais l'enregistrement.
- **Liens que vous ouvrez.** Une bande-annonce, une page The Movie Database
  ou IMDb, ou la page Play Store d'une mise à jour s'ouvre dans l'application
  ou le navigateur qui va avec, selon les règles de ce service.
- **Plugins.** Les plugins sont installés par l'administrateur du serveur. Un
  plugin peut afficher du contenu qui vient d'un service extérieur (un plugin
  météo, par exemple).

## Ce qui reste sur l'appareil { #what-stays-on-the-device }

L'adresse du serveur, votre jeton de session et, si vous cochez se souvenir de moi, votre nom d'utilisateur et votre mot de passe (dans le stockage chiffré d'Android, le keystore Android), vos préférences, et les films et la musique que vous téléchargez
pour les regarder ou les écouter sans connexion. Désinstaller l'application
les supprime. Le déverrouillage par empreinte ou par visage, quand vous
l'activez, est géré par Android : l'application sait seulement s'il a réussi.

## Autorisations { #permissions }

| Autorisation | Pourquoi |
| --- | --- |
| Internet, état du Wi-Fi | Joindre votre serveur, savoir si vous êtes sur le réseau domestique ou à l'extérieur. |
| Appareil photo | Scanner le QR code de la Connexion rapide. Rien n'est enregistré ni envoyé. |
| Notifications, service au premier plan | Montrer où en sont la lecture et les téléchargements, continuer la lecture écran éteint. |
| Biométrie | Verrouillage facultatif de l'application. |
| Modifier les réglages audio | Ajuster le son de la lecture (gestion du volume) pendant un film. |
| Maintien en éveil (wake lock) | Empêcher l'appareil de se mettre en veille pendant la lecture. |
| Vibration | Retour haptique dans l'application. |
| Lecture et écriture du stockage externe (Android 12 et antérieur seulement) | Enregistrer et lire vos téléchargements hors connexion sur ces versions d'Android. |

L'application pour téléphone n'a pas l'autorisation d'utiliser le micro.

## Ce site { #this-website }

`eveyproject.org` est hébergé par GitHub Pages, qui voit l'adresse IP du
visiteur. Le site fournit lui-même ses polices et ses scripts : le navigateur
ne contacte aucun autre service. Il ne dépose aucun cookie propre et ne fait
pas de statistiques de visite.

## Services tiers { #third-party-services }

Ce produit utilise l'API TMDB mais n'est ni approuvé ni certifié par TMDB. Les visuels et métadonnées proviennent de [TMDB](https://www.themoviedb.org/).

## Enfants { #children }

Les applications ne sont pas destinées aux enfants. Les profils enfant sont
créés par la personne qui gère le serveur, et c'est elle qui détient ces
données.

## Contact { #contact }

Questions sur ces règles : [contact@eveyproject.org](mailto:contact@eveyproject.org).
Pour les données gardées sur un serveur Evey, adressez-vous à la personne qui
gère ce serveur.

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
