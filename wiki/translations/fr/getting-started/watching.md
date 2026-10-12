---
source_sha: 01808205a7b486c25cdf287ac650a106e5b49a4e4c1b4e587831593195b62017
reviewed: true
---
# Regarder avec Evey

Cette page est pour les personnes qui regardent : la famille, les amis.
Quelqu'un d'autre fait tourner le serveur Evey ; vous, vous voulez simplement vous
installer et lancer la lecture.

## Se connecter

La personne qui fait tourner Evey vous donne deux choses :

- **Une adresse**, par exemple `http://192.168.1.20:9091`.
- **Un compte** : un nom d'utilisateur et un mot de passe, ou un lien
  d'invitation. Avec le lien, vous choisissez vous-même votre nom
  d'utilisateur et votre mot de passe.

Ensuite :

- **Sur un ordinateur**, ouvrez l'adresse dans votre navigateur et
  connectez-vous.
- **Sur un téléphone, une tablette ou une TV Android**, ouvrez l'application
  Evey. Saisissez l'adresse sur le premier écran, puis connectez-vous. Pas à pas :
  [Connecter les applications](apps.md#the-server-address-to-enter).
- **Sur une TV**, choisissez **Connexion rapide** pour ne pas saisir votre mot
  de passe avec la télécommande. La TV affiche un code ; vous l'acceptez
  depuis votre téléphone ou votre ordinateur :
  [Connexion rapide](apps.md#quick-connect-signing-in-a-tv-without-typing-a-password).

## Choisir un profil

Une fois connecté, **Qui regarde ?** affiche les profils du compte. Choisissez
le vôtre. Chaque personne a son propre profil, qui retient où elle s'est
arrêtée, ce qu'elle a regardé, ses favoris et ses langues.

Un profil marqué **Enfant** ne montre que les titres adaptés à l'âge de
l'enfant. Pour passer de ce profil à un autre, il faut le code PIN à 4
chiffres de cet autre profil. Pour en savoir plus :
[Profils, profils enfant et code PIN](../administration/users.md#profiles-kids-profiles-and-the-pin).

## Trouver quelque chose

- **Accueil** commence par ce que vous avez laissé en cours, prêt à
  reprendre. Viennent ensuite des rangées comme **Ajoutés récemment**, et une
  rangée par bibliothèque. **Personnaliser** permet de choisir les rangées
  affichées.
- **Bibliothèques** et **Genres** vous laissent tout parcourir.
- **Rechercher** trouve un film, une série ou une chanson par son nom.
- **Ajouter aux favoris** sur un titre le range dans **Favoris**.
  **Historique de lecture** liste ce que vous avez regardé avec ce profil.

## Lire

- Sur la page d'un titre, **Lecture** le lance. **Reprendre** continue là où
  vous vous êtes arrêté, même si c'était sur un autre appareil. **Lire depuis
  le début** recommence à zéro.
- Dans le lecteur, le menu des réglages permet de choisir la langue
  **Audio** et les **Sous-titres**. Pour que vos langues soient choisies pour
  vous à chaque fois, réglez-les dans **Paramètres → Lecture → Langue audio
  par défaut** et **Langue des sous-titres par défaut**.
- Certains sous-titres prennent un moment la première fois que quelqu'un les
  choisit. Le lecteur affiche alors **Sous-titres en préparation…** ; la
  vidéo continue, et les sous-titres apparaissent d'eux-mêmes quand ils sont
  prêts. Plus de détails : [Sous-titres](../playback/subtitles.md).
- Dans une série, **Passer l'intro** apparaît pendant le générique de début
  quand Evey l'a trouvé, et **À suivre** propose l'épisode suivant à la fin.

Vous ne choisissez jamais de qualité : Evey adapte de lui-même la vidéo à votre
appareil.

## Demander un titre

Il manque quelque chose ? Allez dans **Demandes** :

1. Cherchez le film, la série ou l'artiste.
2. Appuyez sur **Demander**. Les administrateurs sont prévenus.
3. Suivez votre demande dans **Mes demandes** : **En attente**, **Acceptée**,
   **Refusée** ou **Plus tard**, avec la note qu'ils ont laissée. Vous
   recevez une notification quand elle change.

Un titre déjà sur le serveur est marqué **Déjà disponible**, avec
**Regarder**. Si l'écran affiche **Les demandes sont désactivées**,
l'administrateur a désactivé les demandes pour votre compte.

## Sur le téléphone, la TV et le Chromecast

- **Téléchargements** (application Android) : gardez un film sur le
  téléphone pour le regarder sans connexion, pendant 30 jours :
  [Ce que chaque application ajoute](apps.md#what-each-app-adds).
- **Caster** : envoyez l'image vers un Chromecast, ou vers une TV avec
  Chromecast intégré : [Diffusion (Chromecast)](../playback/casting.md).
- **Lire sur**, dans les lecteurs web et Android : passez ce que vous
  regardez sur un autre de vos appareils où Evey est ouvert, et continuez
  là-bas.
- **Regarder ensemble** : plusieurs personnes regardent le même titre au même
  moment, chacune sur son écran. Des amis sans compte peuvent vous rejoindre
  avec un code : [Regarder ensemble](../playback/watching-together.md).

## Quand quelque chose ne fonctionne pas

Notez trois choses : l'heure, le titre et l'appareil. Puis prévenez la
personne qui fait tourner Evey.

Si un film est moins beau que d'habitude ou met du temps à démarrer, ouvrez
les **Infos du flux** dans le lecteur (le bouton (i) dans le lecteur web).
Elles montrent si le fichier est envoyé tel quel ou adapté à votre appareil,
et pourquoi. Cela aide la personne qui fait tourner Evey :
[Lire les Infos du flux](../playback/how-playback-is-decided.md#reading-stream-info).

Pour elle : [Dépannage](../operations/troubleshooting.md) liste les causes
habituelles, et [Signaler un problème](../operations/troubleshooting.md#reporting-a-problem)
dit quoi mettre dans un signalement.
