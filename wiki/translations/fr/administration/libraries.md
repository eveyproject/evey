---
source_sha: 5d7cd5222fd548a70f680b24ee25dad0875f225e344cc2d8e24651b5259720a7
reviewed: true
---
# Bibliothèques et métadonnées

Une bibliothèque, c'est un type et un ensemble de dossiers : Evey parcourt ces
dossiers, reconnaît chaque fichier d'après son nom et récupère ses
informations (titres, résumés, affiches, distribution, notes). Cette page
s'adresse à l'administrateur qui en ajoute une : les trois gestes du début
(ajouter une bibliothèque, nommer les fichiers, corriger une mauvaise
identification) viennent en premier ; le déroulement des scans, les
particularités de la musique et des photos, les collections et les sources de
métadonnées suivent.

<figure class="evey-shot" markdown="span">
  ![Admin → Bibliothèques sur le serveur de démo](../images/admin-libraries.webp){ loading=lazy }
  <figcaption><strong>Admin → Bibliothèques</strong> : une ligne par bibliothèque, avec ses dossiers, son nombre d'éléments et son dernier scan.</figcaption>
</figure>

## Ajouter une bibliothèque

1. **Admin → Bibliothèques → Ajouter une bibliothèque**.
2. Donnez-lui un **Nom** et un **Type**. Gardez films et séries dans des
   bibliothèques distinctes : le type indique au scanner ce qu'il doit
   chercher.
3. Sous **Chemins**, choisissez un dossier dans la liste, ou **Entrer un
   chemin personnalisé...** pour le saisir ; **+ Ajouter un chemin** ajoute
   un second dossier à la même bibliothèque.
4. **Créer une bibliothèque**. Le premier parcours des dossiers suit dans les
   deux minutes ; **Scanner** sur la ligne le lance tout de suite. Affiches et
   résumés arrivent en tâche de fond : une grande bibliothèque se remplit en
   quelques minutes.

| Type | Reconnu comme | Métadonnées issues de |
| --- | --- | --- |
| **Films** | Un film par fichier (ou par dossier) | TMDB, complété par OMDb si une clé est renseignée |
| **Séries TV** | Séries, saisons, épisodes | TMDB |
| **Musique** | Artistes, albums, titres | Les tags des fichiers ; pochettes depuis les fichiers, le dossier de l'album, Cover Art Archive (MusicBrainz), puis iTunes |
| **Photos** | Albums, photos avec leurs informations de prise de vue | Les fichiers |

!!! note "Les chemins sont ceux du conteneur"
    Le sélecteur de dossiers affiche ce que voit le conteneur. Avec le fichier
    compose de l'installation, votre dossier de médias est `/media` : un
    dossier `/srv/media/films` sur la machine devient `/media/films` pour
    Evey. Un second dossier monté (`/media2`) s'ajoute de la même façon.

## Nommer les fichiers pour qu'ils soient reconnus

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
retirées avant la recherche.

Si vos fichiers portent une étiquette que l'analyseur ne connaît pas,
ajoutez-la dans **Admin → Configuration du serveur → Jetons d'analyse** :
**Jetons de qualité supplémentaires** pour un mot seul (`HDRIP`), **Suffixes
d'édition supplémentaires** pour une expression. Elle s'applique dès le scan
suivant.

## Corriger un titre faux ou manquant

Pour un seul titre, ouvrez-le et choisissez **Modifier les métadonnées** :

- **Identifier sur TMDB** cherche le titre sur TMDB ; **Choisir ce résultat**
  l'applique. C'est la correction la plus rapide pour un film confondu avec
  un homonyme.
- Les champs modifient les informations à la main. Une valeur modifiée est
  conservée : les actualisations suivantes ne l'écrasent pas. Changer l'**ID
  TMDB** ou l'**ID IMDB** récupère les informations de cet identifiant à
  l'enregistrement.
- **Actualiser les métadonnées** récupère à nouveau les informations de
  l'identification en cours. **Réinitialiser les métadonnées** et
  **Réidentifier** repartent du nom de fichier, en abandonnant vos
  modifications ; la réinitialisation efface aussi les images et la
  distribution, ainsi que les saisons et les épisodes d'une série.

Pour toute une bibliothèque, sa ligne dans **Admin → Bibliothèques** propose :

- **Identifier les orphelins** relance la recherche pour les fichiers jamais
  reconnus. C'est rapide.
- **Tout réidentifier avec TMDB** refait la recherche pour tous les titres.
  C'est lent, et corrige les anciennes erreurs après un gros renommage.
- **Réparer les sagas** rattache à leur saga les films qui n'appartiennent à
  aucune collection, quand les autres films de la saga sont déjà dans la
  bibliothèque.

## Prise en compte des nouveaux fichiers

Le serveur parcourt chaque bibliothèque à chaque démarrage, puis toutes les
2 minutes (**Admin → Configuration du serveur → Paramètres du serveur →
Intervalle d'analyse périodique**, appliqué au redémarrage). Pendant une
lecture, les parcours sont espacés à 15 minutes pour ne pas disputer le
disque au lecteur. Un fichier ajouté, renommé ou supprimé est donc vu en
quelques minutes, sans aucun réglage.

Deux autres façons de scanner, sur la ligne de la bibliothèque :

| Action | Effet |
| --- | --- |
| **Scanner** | Un parcours immédiat. La ligne affiche **Scan en cours...** puis le résultat ; un dossier illisible est nommé. |
| **Planifier** | Un scan complet **Toutes les 6 heures**, **Toutes les 12 heures**, **Quotidien (3h du matin)**, **Hebdomadaire (dimanche 3h du matin)**, ou selon une **Expression cron**. |

Le serveur surveille aussi les dossiers par les événements du système de
fichiers (**Admin → Configuration du serveur → Paramètres du serveur →
Surveillance des fichiers de la bibliothèque**), sur les bibliothèques dont
la surveillance en temps réel est active. Il vérifie que cette surveillance
répond toujours en écrivant pendant quelques secondes un fichier de test dans
le dossier ; une bibliothèque dont la surveillance est prouvée vivante n'est
parcourue qu'une fois par heure au lieu de toutes les 2 minutes, les autres
gardent le parcours périodique. Les bibliothèques créées depuis le panneau ne
sont pas surveillées : c'est le parcours périodique qui prend leurs fichiers
en compte.

!!! note "Partages réseau"
    Sur un partage NFS ou SMB, un fichier écrit par une autre machine ne
    déclenche aucun événement sur le serveur. C'est le parcours périodique
    qui le trouve.

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

## Collections

**Collections** regroupe des titres sous un même nom, pour tous les comptes. Il
en existe trois sortes :

- **Collections TMDB** : les sagas, remplies d'elles-mêmes quand TMDB range un
  film dans l'une d'elles. **Réparer les sagas** (plus haut) rattrape les films
  qui ont manqué la leur.
- **Nouvelle collection** : une liste qu'un administrateur remplit à la main,
  avec **Ajouter à la collection** sur un titre ou sur une sélection, et qu'il
  met en ordre.
- **Nouvelle collection intelligente** : des règles sur **Genre**, **Année**,
  **Type**, **Note ≥** et **Bibliothèque**, qu'un titre doit toutes remplir. Son
  contenu est calculé à chaque ouverture, si bien que les nouveaux titres y
  apparaissent sans rien faire.

Seuls les administrateurs créent et modifient les collections. Tout le monde
peut les parcourir, et **Télécharger la collection** sur le web récupère les
fichiers originaux dans un seul ZIP (jusqu'à 1000 titres), en laissant de côté
ce que masquent l'accès aux bibliothèques ou la limite d'âge du compte.

## Clés de métadonnées

Evey embarque une clé TMDB : les métadonnées fonctionnent sans aucune
configuration. Les autres sources sont facultatives ; leurs clés se
renseignent dans **Admin → Configuration du serveur → Clés API**, et une clé
saisie là prime sur la même clé fournie par l'environnement (voir
[Environnement : bibliothèques et métadonnées](../environment/libraries.md)).

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

Les lecteurs affichent par défaut les boutons **Passer l'intro**, **Passer le
résumé** et **Passer le générique**. **Saut automatique des intros**
(**Admin → Configuration du serveur → Paramètres du serveur**, désactivé par
défaut) fait passer les intros automatiquement dans tous les lecteurs.
