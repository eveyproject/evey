---
source_sha: ce2a0af8fbd7f01b2b50436f15cba49edd4090091757afa42aa5e7d807ab8239
reviewed: true
---
# Regarder avec Evey

Pour les personnes du foyer qui regardent, pas pour celle qui fait tourner le
serveur : comment vous connecter, trouver quelque chose, le lire à votre goût, et
demander ce qui manque. Chaque section se termine par la page qui entre dans
le détail.

## Se connecter

Quelqu'un fait tourner le serveur et vous donne deux choses : son **adresse**
et un **compte**.

- **L'adresse** est ce que vous tapez dans un navigateur, ou sur le premier
  écran des applications Android et Android TV, par exemple
  `http://192.168.1.20:9091` à la maison.
  [L'adresse du serveur à saisir](apps.md#the-server-address-to-enter)
  explique d'où elle vient et ce que veut dire chaque erreur.
- **Le compte** est un nom d'utilisateur et un mot de passe, ou un lien
  d'invitation avec lequel vous choisissez votre propre mot de passe.
- **Sur une TV**, choisissez **Connexion rapide** : la TV affiche un code, que
  vous approuvez depuis votre téléphone ou un navigateur où vous êtes déjà
  connecté, sans rien taper à la télécommande :
  [Connexion rapide](apps.md#quick-connect-signing-in-a-tv-without-typing-a-password).

## Choisir un profil

Après la connexion, **Qui regarde ?** liste les profils du compte : un par
personne, pour que chacun garde ses points de reprise, son historique, ses
favoris et ses langues. Un profil marqué **Enfant** ne montre que les titres
adaptés à son âge, et le quitter pour un autre profil demande le code PIN à
4 chiffres de ce profil. Comment fonctionnent les profils et le PIN :
[Utilisateurs, profils et contrôle parental](../administration/users.md#profiles-kids-profiles-and-the-pin).

## Trouver quelque chose

- **Accueil** commence par ce que vous avez laissé en cours, prêt à reprendre,
  puis des rangées comme **Ajoutés récemment** et une par bibliothèque ;
  **Personnaliser** choisit les rangées affichées.
- **Bibliothèques** et **Genres** parcourent tout ; **Rechercher** trouve un
  film, une série ou une chanson par son nom.
- **Ajouter aux favoris** sur un titre le range dans **Favoris** ;
  **Historique de lecture** liste ce que ce profil a regardé.

## Lire

- Sur la page d'un titre, **Lecture** le lance, **Reprendre** continue là où
  vous vous êtes arrêté, sur n'importe quel appareil, et **Lire depuis le
  début** recommence.
- Dans le lecteur, le menu des réglages choisit la piste **Audio** et les
  **Sous-titres**. Les langues choisies en premier sont celles de votre
  profil : **Paramètres → Lecture → Langue audio par défaut** et **Langue des
  sous-titres par défaut**. Plus de détails sur les sous-titres, et sur la
  raison pour laquelle le premier affichage de certains prend du temps :
  [Sous-titres](../playback/subtitles.md).
- **Passer l'intro** apparaît pendant le générique d'ouverture d'un épisode
  quand le serveur l'a repéré, et **À suivre** propose l'épisode suivant à la
  fin.
- Quand un titre paraît moins beau que prévu ou démarre lentement, les **Infos
  du flux** du lecteur disent si le fichier est envoyé tel quel ou adapté à
  votre appareil, et pourquoi :
  [Comment Evey choisit le mode de lecture](../playback/how-playback-is-decided.md#reading-stream-info).

## Demander un titre

**Demandes** cherche un film, une série ou un artiste que les bibliothèques
n'ont pas encore ; **Demander** l'envoie aux administrateurs. **Mes demandes**
montre où en est chacune : **En attente**, **Acceptée**, **Refusée** ou
**Plus tard**, avec la note laissée, et vous êtes prévenu quand cela change. Un
titre déjà sur le serveur est marqué **Déjà disponible**, avec **Regarder**.
Quand l'administrateur a désactivé les demandes pour votre compte, l'écran
indique **Les demandes sont désactivées**.

## Sur le téléphone, la TV et le Chromecast

- **Téléchargements** (application Android) garde un titre sur le téléphone
  pour le regarder sans connexion :
  [Ce que propose chaque application](apps.md#what-each-app-adds).
- **Caster** envoie l'image vers un Chromecast :
  [Diffusion (Chromecast)](../playback/casting.md).
- **Lire sur**, dans les lecteurs web et Android, déplace ce que vous regardez
  vers un autre de vos appareils où Evey est ouvert, et la lecture continue là-bas.
- **Regarder ensemble** garde plusieurs personnes au même moment du même
  titre, chacune sur son écran : [Regarder ensemble](../playback/watching-together.md).

## Quand quelque chose ne marche pas

Notez l'heure, le titre et l'appareil, et prévenez la personne qui fait
tourner le serveur. [Dépannage](../operations/troubleshooting.md) lui donne
les causes habituelles, et [Signaler un problème](../operations/troubleshooting.md#reporting-a-problem)
ce qu'il faut mettre dans un signalement quand il faut aller plus loin.
