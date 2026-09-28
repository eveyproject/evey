---
source_sha: f1ea44197021d6c8d4791aa939ef1b103613cd438a70d3c00ce1af151c0ca580
---
# Premiers pas

Le conteneur tourne et vous avez ouvert `http://<votre-ip>:9091`. Cette page
parcourt les dix premières minutes : l'assistant d'installation, les
bibliothèques, les personnes, et les trois points à vérifier avant de donner
l'adresse à qui que ce soit. Si le serveur ne tourne pas encore, commencez par
l'[installation](../installation.md).

## L'assistant d'installation

Un serveur sans administrateur s'ouvre sur un assistant en quatre étapes.

1. **Langue.** La langue par défaut des nouveaux profils et des métadonnées
   (titres, résumés, genres). Vous pourrez la changer plus tard dans
   **Admin → Configuration du serveur → Langue et région**, avec la région qui
   décide des classifications d'âge appliquées.
2. **Bienvenue.**
3. **Compte administrateur.** Un nom d'utilisateur d'au moins trois caractères
   et un mot de passe d'au moins huit. Le **nom du serveur**, facultatif
   (« Salon »), apparaît sur la page de connexion, dans la liste des serveurs
   des applications et dans les e-mails.
4. **Première bibliothèque.** Un nom et un type. Vous pouvez passer les
   détails ici et terminer dans le panneau d'administration.

## Ajouter vos bibliothèques

**Admin → Bibliothèques → Ajouter une bibliothèque.** Une bibliothèque a un
nom, un type (films, séries, musique ou photos) et un ou plusieurs dossiers.

**Les dossiers sont des chemins à l'intérieur du conteneur**, pas sur votre
machine. Avec le fichier compose par défaut, votre dossier de médias est monté
sur `/media` : une bibliothèque de films rangée dans `/srv/media/films` sur la
machine devient `/media/films` pour Evey. Un second dossier monté (`/media2`,
...) s'ajoute de la même façon. Le sélecteur liste les dossiers qu'il voit.

Gardez films et séries dans des bibliothèques séparées : le type indique au
scanner ce qu'il doit chercher. Evey reconnaît des noms comme ceux-ci :

```text
/media/films/The Mirror (1999)/The Mirror (1999) 1080p.mkv
/media/films/Lantern.2010.1080p.BluRay.x264.mkv
/media/series/The Outpost/Season 01/The.Outpost.S01E05.720p.mkv
/media/series/Quiet Storm/Quiet.Storm.03x10.mkv
```

Pour un film, l'année aide beaucoup. Pour un épisode, c'est `S01E05` ou `1x05`
qui compte. Les étiquettes de release (résolution, source, codec, audio,
marqueurs de langue comme `VOSTFR` ou `MULTI`, éditions comme « Director's
Cut ») sont retirées avant la recherche. Si vos fichiers portent une étiquette
que le scanner ne connaît pas, ajoutez-la dans **Admin → Configuration du
serveur → Jetons d'analyse**.

Pour chaque bibliothèque, vous choisissez aussi :

| Option | Ce qu'elle fait |
| --- | --- |
| **Surveillance en temps réel** | Les fichiers nouveaux, renommés ou supprimés sont pris en compte dès qu'ils apparaissent. Une vérification périodique (toutes les 2 minutes par défaut) rattrape ce que le système de fichiers n'a pas signalé, par exemple sur un partage réseau. |
| **Scanner au démarrage** | Rescanne la bibliothèque à chaque démarrage du serveur. |
| **Planifier** | Un scan complet toutes les 6 ou 12 heures, chaque jour, chaque semaine, ou selon votre propre expression cron. |

Le premier scan démarre tout seul. Affiches et informations arrivent en arrière-plan : une grande bibliothèque se remplit en quelques minutes.

**Quand un titre est faux ou manquant.** Dans la liste des bibliothèques,
**Matcher les orphelins** retente les titres jamais reconnus, et **Re-matcher
tout sur TMDB** refait la recherche pour chaque titre (lent, mais corrige les
anciennes erreurs après un gros renommage). Sur un titre,
**Modifier les métadonnées** permet de corriger le titre ou de coller le bon
identifiant TMDB ou IMDb.

## Sources de métadonnées

Evey est livré avec une clé **TMDB**, la source des titres, affiches,
distributions et collections : les métadonnées fonctionnent dès l'installation.
Les autres sources sont facultatives et leurs clés se saisissent dans
**Admin → Configuration du serveur → Clés API** :

| Clé | Apporte |
| --- | --- |
| TMDB (la vôtre) | Votre propre quota au lieu du quota partagé. |
| OMDb | Une seconde source de métadonnées, pour compléter un titre que TMDB connaît mal. |
| OpenSubtitles | La recherche et le téléchargement de sous-titres qu'un fichier ne contient pas. |
| TheIntroDB | Davantage de repères de générique, de résumé et de générique de fin (500 recherches par jour au lieu de 100). |

Une clé saisie dans le panneau l'emporte sur la même clé passée dans
l'environnement.

## Inviter le foyer

**Admin → Utilisateurs** offre deux portes d'entrée :

- **Créer un utilisateur** : vous choisissez le nom et le mot de passe.
- **Créer un lien d'invitation** : la personne choisit les siens. Un lien peut
  expirer, être limité à un nombre d'utilisations, donner accès à certaines
  bibliothèques seulement, plafonner le nombre de profils que le nouveau
  compte pourra créer, et même créer un administrateur.

Pour chaque utilisateur, vous pouvez ensuite restreindre les
**bibliothèques** qu'il voit, plafonner son **débit de streaming**, limiter
ses **profils** et ses **appareils par profil**, consulter son historique, ou
**vous connecter en tant que** lui pour voir exactement ce qu'il voit.

Les profils, les profils enfants et le code PIN parental sont gérés par chaque
compte lui-même : voir les
[réglages utilisateur](viewer-settings.md#profiles-and-parental-control).

## Laisser le serveur se mesurer

La première fois qu'il démarre sur une machine, puis à chaque changement de
matériel, Evey mesure combien de vidéos il peut convertir en même temps, avec
le processeur et avec la carte graphique s'il y en a une. Cela prend environ
deux minutes, ne se lance que quand personne ne regarde, et un lecteur qui
demande une conversion pendant ce temps est invité à réessayer un peu plus
tard. Le résultat se trouve dans **Admin → Configuration transcodage** : ce
nombre est un plafond que vous pouvez abaisser, jamais relever. Pour utiliser
une carte graphique, voir [Transcodage et matériel](../transcoding.md).

## Avant de partager l'adresse

1. **Accès distant.** Evey ne s'ouvre à Internet que si vous lui avez donné une
   adresse publique. Vérifiez **Admin → Accès distant** : si le serveur est
   joignable depuis l'extérieur alors que l'accès distant est coupé, la page le
   signale en rouge, et tous ceux qui sont hors de votre réseau reçoivent une
   erreur. Voir l'[administration](admin-settings.md#remote-access).
2. **HTTPS.** Evey parle en HTTP simple. Placez-le derrière un reverse proxy
   avant de le publier : [Reverse proxy](../reverse-proxy.md).
3. **Sauvegardes.** Le serveur sauvegarde sa base avant chaque mise à jour.
   Pour des sauvegardes régulières, fixez une planification :
   [Sauvegardes](../backups.md). Gardez `./evey-data` en lieu sûr - il contient
   la base de données et la clé qui signe chaque session.

## Installer les applications

Ouvrez l'application Android ou Android TV, tapez l'adresse du serveur, et
connectez-vous. Sur une télévision, choisissez la **connexion rapide** : la
télé affiche un code et un QR code, que vous approuvez depuis
**Paramètres → Connexion rapide** sur un téléphone ou un ordinateur déjà
connecté. Plus de détails dans [Sur tous les écrans](../discover/apps.md).
