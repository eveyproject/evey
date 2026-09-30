---
source_sha: dd11b7fe28f14e3c25cf6db836b2a6f0752722f82e49a8db9ce447b959d38822
reviewed: true
---
# Première configuration

Le serveur tourne et vous avez ouvert `http://<adresse-du-serveur>:9091`.
Il reste quatre étapes avant de pouvoir regarder quoi que ce soit :
l'assistant de configuration, les bibliothèques, les utilisateurs et les
applications.

## 1. L'assistant de configuration

Un serveur sans administrateur s'ouvre sur un assistant en quatre étapes :

1. **Langue** : la langue par défaut des nouveaux profils et des
   métadonnées (titres, résumés, genres).
2. **Bienvenue**.
3. **Compte administrateur** : un nom d'utilisateur d'au moins 3 caractères
   et un mot de passe d'au moins 8. Le nom du serveur, facultatif, apparaît
   sur la page de connexion et dans la liste des serveurs des applications.
4. **Première bibliothèque** : un nom et un type. Vous pouvez passer cette
   étape et ajouter les bibliothèques ensuite.

<figure class="evey-shot" markdown="span">
  ![Le tableau de bord d'administration du serveur de démo](../images/admin-dashboard.webp){ loading=lazy }
  <figcaption>L'administration une fois connecté : le tableau de bord, et chaque section dans le menu latéral. (interface en anglais).</figcaption>
</figure>

## 2. Ajouter vos bibliothèques

**Admin → Bibliothèques → Ajouter une bibliothèque** : un nom, un type
(Films, Séries, Musique ou Photos) et un ou plusieurs dossiers.

Les dossiers sont des chemins **à l'intérieur du conteneur**. Avec le fichier
compose de l'installation, votre dossier de médias est `/media` : un dossier
`/srv/media/films` sur la machine devient `/media/films` pour Evey.

Le premier scan démarre tout seul. Les nouveaux fichiers sont pris en compte
dès leur apparition, et les affiches, résumés et distributions arrivent en
arrière-plan : une grosse bibliothèque se remplit en quelques minutes.

Nommage des fichiers, fonctionnement des scans, correction d'une mauvaise
identification : [Bibliothèques et métadonnées](../administration/libraries.md).

## 3. Ajouter les personnes

**Admin → Utilisateurs** crée un compte avec un mot de passe que vous
choisissez, ou un lien d'invitation avec lequel la personne choisit le sien.
Chaque compte crée ensuite ses profils : un par personne du foyer, profils
enfant compris.

Ce qu'un administrateur peut restreindre par compte, et le fonctionnement
des profils et du contrôle parental :
[Utilisateurs, profils et contrôle parental](../administration/users.md).

## 4. Connecter les applications

L'application web est l'adresse que vous avez déjà ouverte. Les applications
Android et Android TV demandent l'adresse du serveur, puis une connexion ;
une TV peut se connecter avec un code validé depuis un téléphone. Voir
[Applications et appareils](apps.md).

## Ce que le serveur fait tout seul

Rien de ceci ne demande de réglage :

- **La lecture.** Chaque appareil indique ce qu'il sait décoder, et le
  serveur choisit, titre par titre, entre envoyer le fichier tel quel, le
  remuxer, ou réencoder uniquement ce que l'appareil ne sait pas lire. Voir
  [Comment Evey choisit le mode de lecture](../playback/how-playback-is-decided.md).
- **La capacité.** Dès qu'il est au repos, le serveur mesure combien de
  vidéos il peut transcoder simultanément, avec le processeur et avec toute
  carte graphique donnée au conteneur, et n'en accepte jamais davantage. La
  mesure prend quelques minutes ; une lecture qui a besoin d'un transcodage
  pendant ce temps est invitée à patienter. Voir
  [Transcodage et capacité du serveur](../playback/transcoding.md).
- **L'accès distant.** Il s'ouvre quand le serveur a reçu une adresse
  publique, et reste fermé sinon. Voir
  [Accès distant et HTTPS](../administration/remote-access.md).
- **Les sauvegardes.** La base est sauvegardée avant chaque mise à jour qui
  la modifie. Une planification régulière se règle en un écran :
  [Sauvegardes et mises à jour](../backups.md).

## Avant de partager l'adresse hors de chez vous

1. Placez le serveur derrière du HTTPS : Evey sert du HTTP simple.
   [Accès distant et HTTPS](../administration/remote-access.md).
2. Planifiez des sauvegardes, et gardez une copie de `./evey-data` : ce
   dossier contient la base et la clé qui signe toutes les sessions.
   [Sauvegardes et mises à jour](../backups.md).
