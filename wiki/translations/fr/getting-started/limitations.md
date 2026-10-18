---
source_sha: 6a8dc35c6cee620adb9bfbba22b808325ea49623051da7c9b0980893e4c7ff09
reviewed: true
---

# Limites connues

Ce qu'Evey ne fait pas encore, dit simplement, pour que personne ne le
découvre le soir où il voulait regarder un film. Chaque point dit quoi
utiliser à la place.

## Pas d'application pour iPhone, iPad ni Apple TV

Il existe une application Evey pour les téléphones et tablettes Android, et
une pour Android TV et Google TV. Il n'y en a aucune pour iPhone, iPad ou
Apple TV. Sur ces appareils, ouvrez l'adresse du serveur dans le navigateur
web.

Pour aller plus loin : [Connecter les applications](apps.md).

## Les applications Android sont encore en test

Les applications Android et Android TV ne sont pas encore sur Google Play
pour tout le monde. Seuls les comptes invités au test peuvent les installer.
L'application web et la page `/tv` dans le navigateur d'une télévision ne
demandent aucune installation et fonctionnent pour tous.

Pour aller plus loin : [Connecter les applications](apps.md).

## Regarder sans connexion : application Android seulement

Seule l'application Android garde une copie d'un film ou d'un épisode sur
l'appareil (**Téléchargements**), lisible pendant 30 jours sans contacter le
serveur. Le lecteur web, l'application Android TV et la page `/tv` lisent
depuis le serveur et ont besoin de la connexion du début à la fin.

Pour aller plus loin : [Ce que chaque application ajoute](apps.md#what-each-app-adds).

## Pas de notifications sur le téléphone quand l'application est fermée

Les applications Android et Android TV affichent leurs notifications dans
leur écran **Notifications**, quand elles sont ouvertes. Elles ne sonnent pas
sur le téléphone ni sur la TV quand elles sont fermées. Les notifications qui
vous parviennent en dehors de l'application viennent du navigateur web (une
fois activées, en `https://`) ou par e-mail, quand la personne qui fait
tourner Evey l'a configuré.

Pour aller plus loin : [Où apparaissent les notifications](../notifications.md#where-notifications-appear).

## Les profils se gèrent sur le web ou sur le téléphone

L'application Android TV passe d'un profil à l'autre et change certains
réglages de celui en cours, mais elle ne peut ni créer ni supprimer un
profil. Faites-le dans l'application web ou dans l'application Android.

Pour aller plus loin : [Utilisateurs, profils et contrôle parental](../administration/users.md#profiles-kids-profiles-and-the-pin).

## Caster : depuis Chrome ou l'application Android

Le bouton **Caster** existe dans le lecteur web sous Chrome et dans
l'application Android. Firefox n'en a pas, et l'application Android TV non
plus, puisqu'elle est déjà sur la TV. Safari, et Chrome sur un téléphone
Android, passent par la lecture à distance du navigateur lui-même, qui
fonctionne autrement.

Pour aller plus loin : [Diffusion (Chromecast)](../playback/casting.md).

## Evey ne se met jamais à jour tout seul

Le serveur vérifie si une version plus récente existe et le signale sur le
tableau de bord d'administration, mais il n'installe rien de lui-même : la
personne qui le fait tourner met à jour quand cela lui convient, en deux
commandes. Les applications Android affichent un message quand elles sont plus
anciennes que le serveur.

Pour aller plus loin : [Mettre à jour Evey](../backups.md#updating-evey) et
[Garder les applications à jour](apps.md#keeping-the-apps-up-to-date).
