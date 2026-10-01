---
source_sha: fb9873d46e2323723ae54ede6035d085a424d9e6c047c3fb833962b8358dd857
reviewed: true
---
# Applications et appareils

Chaque application dialogue avec votre propre serveur. Vous lui indiquez
l'adresse du serveur une fois, vous vous connectez, puis vous choisissez un
profil. Les points de reprise, l'historique et les favoris sont stockés sur le
serveur : ils vous suivent d'un appareil à l'autre.

| Application | Provenance | Connexion |
| --- | --- | --- |
| **Web** | Servie par Evey lui-même, à l'adresse du serveur. Rien à installer. | Nom d'utilisateur et mot de passe, ou authentification unique si elle est configurée. |
| **Android** (téléphone, tablette) | L'application Android. | Adresse du serveur, puis nom d'utilisateur et mot de passe. |
| **Android TV** | L'application Android TV. | Adresse du serveur, puis **Connexion rapide** (ci-dessous) ou nom d'utilisateur et mot de passe. |
| **Chromecast** | Pas d'application : on caste depuis l'application web dans Chrome ou depuis l'application Android. | Utilise la session de l'appareil qui caste. |

Le panneau d'administration fait partie de l'application web, pour les
comptes administrateurs.

## Connexion rapide : connecter une TV sans taper de mot de passe

Sur la TV, choisissez **Connexion rapide**. Elle affiche un code et un QR
code. Sur un téléphone ou un ordinateur déjà connecté, ouvrez
**Paramètres → Connexion rapide**, puis tapez le code ou scannez le QR code
avec l'application Android. La TV est alors connectée au même compte.

## Ce que propose chaque application

- **Web.** Des raccourcis clavier dans le lecteur (`?` les affiche), les
  notifications push du navigateur (voir [Notifications](../notifications.md)),
  et le panneau d'administration.
- **Android.**
  - Les **téléchargements** pour regarder hors connexion. Si l'administrateur
    l'a activée (`downloads.quality_choice`, voir
    [Diffusion de la lecture et téléchargements](../environment/playback-delivery.md)),
    l'application propose une copie plus légère préparée par le serveur pour
    un film trop volumineux pour le téléphone. Un téléchargement reste lisible hors
    connexion pendant 30 jours sans contacter le serveur, et seulement pour
    les profils autorisés à le voir : un profil enfant ne peut pas ouvrir un
    film téléchargé par un autre profil au-dessus de son âge maximum.
  - La **diffusion** vers un Chromecast.
- **Android TV.** Une interface pensée pour la télécommande.

**Lire sur**, dans les lecteurs web et Android, envoie ce que vous regardez
vers une autre application Evey ouverte sur le même compte, et la lecture se
poursuit là-bas.

## Quel appareil lit quoi

Chaque application indique ce que son appareil sait décoder, et le serveur
décide pour chaque titre d'envoyer le fichier tel quel ou de l'adapter. Un
même film peut partir intact vers une TV et être réencodé pour un téléphone
plus ancien. Le panneau **Infos du flux** du lecteur affiche la décision et
ses raisons : voir
[Comment Evey choisit le mode de lecture](../playback/how-playback-is-decided.md). La
diffusion suit des règles qui lui sont propres : voir
[Diffusion (Chromecast)](../playback/casting.md).
