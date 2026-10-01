---
source_sha: b6d595e92c82851324debcee0de01b789bd1def644e658e011f90ccc857c9de9
reviewed: true
---
# Bibliothèques et métadonnées

Une bibliothèque, c'est un type et un ensemble de dossiers. Evey parcourt ces
dossiers, reconnaît chaque fichier d'après son nom et récupère ses
métadonnées : titres, résumés, affiches, distribution, notes. Les
bibliothèques se gèrent dans **Admin → Bibliothèques**.

<figure class="evey-shot" markdown="span">
  ![Admin → Bibliothèques sur le serveur de démo](../images/admin-libraries.webp){ loading=lazy }
  <figcaption><strong>Admin → Bibliothèques</strong> : une ligne par bibliothèque, avec ses dossiers, son nombre d'éléments et son dernier scan.</figcaption>
</figure>

## Types

| Type | Reconnu comme | Métadonnées issues de |
| --- | --- | --- |
| **Films** | Un film par fichier (ou par dossier) | TMDB, complété par OMDb si une clé est renseignée |
| **Séries** | Séries, saisons, épisodes | TMDB |
| **Musique** | Artistes, albums, titres | Les tags des fichiers ; pochettes depuis les fichiers, le dossier de l'album, Cover Art Archive (MusicBrainz), puis iTunes |
| **Photos** | Albums, photos avec leurs informations de prise de vue | Les fichiers |

Gardez films et séries dans des bibliothèques distinctes : le type indique au
scanner ce qu'il doit chercher.

## Les dossiers sont des chemins du conteneur

Le sélecteur de dossiers affiche ce que voit le conteneur. Avec le fichier
compose de l'installation, votre dossier de médias est `/media` : un dossier
`/srv/media/films` sur la machine devient `/media/films` pour Evey. Un second
dossier monté (`/media2`) s'ajoute de la même façon.

## Nommer les fichiers

Le scanner reconnaît des noms comme ceux-ci :

```text
/media/films/The Mirror (1999)/The Mirror (1999) 1080p.mkv
/media/films/Lantern.2010.1080p.BluRay.x264.mkv
/media/series/The Outpost/Season 01/The.Outpost.S01E05.720p.mkv
/media/series/Quiet Storm/Quiet.Storm.03x10.mkv
```

Pour un film, c'est l'année qui compte le plus. Pour un épisode, `S01E05` ou
`1x05`. Les étiquettes de release (résolution, source, codec, audio, mentions
de langue comme `VOSTFR` ou `MULTI`, éditions comme « Director's Cut ») sont
retirées avant la recherche. Si vos fichiers portent une étiquette que
l'analyseur ne connaît pas, ajoutez-la dans **Admin → Configuration du
serveur → Jetons d'analyse** ; elle s'applique dès le scan suivant.

## Bibliothèques de musique

Le scanner lit les fichiers MP3, FLAC, M4A, OGG, Opus, WAV et AAC, et prend
chaque titre dans ses tags : titre (le nom du fichier s'il manque), artiste,
artiste de l'album, album, année, numéros de piste et de disque, genre. Les
artistes sont regroupés par artiste de l'album, ou par artiste quand ce tag est
vide.

**Un album, c'est un dossier.** Les titres sont regroupés par le dossier qui les
contient, pas par leur tag d'album : deux dossiers portant le même tag d'album
font deux albums, et un album découpé en sous-dossiers `CD1` et `CD2` en fait
deux. Gardez chaque album dans un seul dossier ; ses titres sont classés par
numéro de disque puis de piste, et l'album prend le nom que portent la plupart
de ses titres.

La pochette est l'image intégrée aux fichiers, sinon une image du dossier de
l'album (`cover.jpg`, `folder.jpg` ou `front.jpg`), sinon Cover Art Archive
(trouvé via MusicBrainz), sinon iTunes.

## Bibliothèques de photos

Le scanner lit les fichiers JPEG, PNG, HEIC, WebP, GIF, TIFF et BMP. L'album
d'une photo est son dossier, relatif au dossier de la bibliothèque ; une photo
à la racine n'en a pas, et déplacer un fichier dans un autre dossier le fait
passer dans cet album au scan suivant. La date de prise de vue et le modèle
d'appareil viennent des données EXIF des fichiers JPEG ; les autres formats
sont listés sans. Les miniatures sont redimensionnées sur le serveur et gardées
dans le cache de transcodage.

## Prise en compte des nouveaux fichiers

Chaque bibliothèque a trois options :

| Option | Effet |
| --- | --- |
| **Surveillance en temps réel** | Les fichiers ajoutés, renommés ou supprimés sont pris en compte dès leur apparition. Une vérification périodique (toutes les 2 minutes par défaut) rattrape ce que le système de fichiers n'a pas signalé, ce qui arrive sur les partages réseau. |
| **Scanner au démarrage** | Un scan à chaque démarrage du serveur. |
| **Planification** | Un scan complet toutes les 6 ou 12 heures, chaque jour, chaque semaine, ou selon une expression cron. |

Le serveur vérifie aussi que la surveillance des dossiers reçoit toujours les
événements, en écrivant pendant quelques secondes un fichier de test dans le
dossier de la bibliothèque. Une bibliothèque dont les événements n'arrivent
plus est parcourue plus souvent, jusqu'à ce que la surveillance reprenne.

## Quand un titre est faux ou manquant

- **Identifier les orphelins** (par bibliothèque) relance la recherche pour
  les fichiers jamais reconnus. C'est rapide.
- **Tout réidentifier avec TMDB** refait la recherche pour tous les titres de
  la bibliothèque. C'est lent, et corrige les anciennes erreurs après un gros
  renommage.
- **Modifier les métadonnées**, sur un titre, change ses informations à la
  main ou permet de coller le bon identifiant TMDB ou IMDb. Une valeur
  modifiée est conservée : les actualisations suivantes ne l'écrasent pas.
  **Réidentifier**, sur le titre, abandonne vos modifications et relance la
  recherche à partir du nom de fichier.
- **Réparer les sagas** rattache à leur saga les films qui n'appartiennent à
  aucune collection, quand les autres films de la saga sont déjà dans la
  bibliothèque.

## Clés de métadonnées

Evey embarque une clé TMDB : les métadonnées fonctionnent sans aucune
configuration. Les autres sources sont facultatives ; leurs clés se
renseignent dans **Admin → Configuration du serveur → Clés API**, et une clé
saisie là prime sur la même clé fournie par l'environnement.

| Clé | Apporte |
| --- | --- |
| TMDB (la vôtre) | Votre propre quota de requêtes au lieu du quota partagé. |
| OMDb | Une seconde source pour les films que TMDB connaît mal. |
| OpenSubtitles | La recherche et le téléchargement de sous-titres absents du fichier, par l'API du serveur (les applications ne le proposent pas encore). Voir [Sous-titres](../playback/subtitles.md). |
| TheIntroDB | Les repères d'intro, de résumé et de générique d'une base communautaire : 500 requêtes par jour avec une clé, 100 sans. |

## Intro, résumé et générique

Evey repère l'intro, le résumé des épisodes précédents et le générique des
épisodes et des films, pour que les lecteurs proposent de les passer. Les
repères viennent de bases communautaires (TheIntroDB, puis IntroDB) quand
elles connaissent le titre, et sinon de l'analyse des fichiers par le serveur.
Un administrateur peut ajuster à la main les segments d'un titre, ou relancer
la détection.

Les lecteurs affichent un bouton **Passer** par défaut. **Saut automatique
des intros** (**Admin → Configuration du serveur → Paramètres du serveur**,
désactivé par défaut) fait passer les intros automatiquement dans tous les
lecteurs.
