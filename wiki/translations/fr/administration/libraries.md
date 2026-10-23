---
source_sha: c6c0a67f7b17b0139429ba630a95b355b5cc39888b5eb6a2bfd603d38ca8cead
reviewed: true
---
# Bibliothèques et métadonnées { #libraries-and-metadata }

Une **bibliothèque**, c'est un type de média (films, séries, musique ou
photos) rangé dans un ou plusieurs dossiers. Evey lit les dossiers, reconnaît
chaque fichier d'après son nom, et va chercher ses informations sur
Internet : titre, résumé, affiche, distribution, notes. Ces informations
s'appellent les **métadonnées**. Pour les films et les séries, elles viennent
de The Movie Database (TMDB), une base de films gratuite en ligne.

Les trois choses que vous ferez en premier viennent en premier sur cette
page : ajouter une bibliothèque, nommer vos fichiers, corriger un titre mal
reconnu. Le fonctionnement des scans, la musique et les photos, les
collections et les sources de métadonnées viennent ensuite.

<figure class="evey-shot" markdown="span">
  ![Administration → Bibliothèques sur le serveur de démo](../images/admin-libraries.webp){ loading=lazy }
  <figcaption><strong>Administration → Bibliothèques</strong> : une ligne par bibliothèque, avec ses dossiers, son nombre d'éléments et son dernier scan.</figcaption>
</figure>

## Ajouter une bibliothèque { #add-a-library }

1. **Administration → Bibliothèques → Ajouter une bibliothèque**.
2. Donnez-lui un **Nom** et un **Type**. Gardez les films et les séries dans
   des bibliothèques séparées : le type indique au scanner ce qu'il doit
   chercher.
3. Sous **Chemins**, choisissez un dossier dans la liste, ou **Entrer un
   chemin personnalisé...** pour en saisir un ; **+ Ajouter un chemin** ajoute
   un deuxième dossier à la même bibliothèque.
4. Appuyez sur **Créer une bibliothèque**. Evey lit les dossiers dans les
   deux minutes qui suivent ; appuyez sur **Scanner** sur la ligne de la
   bibliothèque pour commencer tout de suite. Les affiches et les résumés
   arrivent en arrière-plan : une grosse bibliothèque se remplit en quelques
   minutes.

| Type | Reconnu comme | Métadonnées venant de |
| --- | --- | --- |
| **Films** | Un film par fichier (ou par dossier) | TMDB, complété par OMDb quand une clé est renseignée |
| **Séries TV** | Séries, saisons, épisodes | TMDB |
| **Musique** | Artistes, albums, morceaux | Les tags des fichiers ; pochettes venant des fichiers, du dossier de l'album, de Cover Art Archive (MusicBrainz), puis d'iTunes |
| **Photos** | Albums, photos avec les informations de prise de vue | Les fichiers |

!!! note "Les dossiers portent le nom qu'ils ont pour Evey"
    La liste montre les dossiers tels qu'Evey les voit, dans son conteneur.
    Avec le fichier compose de l'[installation](../installation.md), votre
    dossier de médias s'appelle `/media`. Un dossier `/srv/media/films` sur
    votre ordinateur devient donc `/media/films` dans Evey. Un deuxième
    dossier de médias (`/media2`) fonctionne de la même façon.

## Nommer les fichiers pour qu'ils soient reconnus { #name-files-so-they-are-recognised }

Le scanner reconnaît des noms comme ceux-ci :

```text
/media/films/The Mirror (1999)/The Mirror (1999) 1080p.mkv
/media/films/Lantern.2010.1080p.BluRay.x264.mkv
/media/series/The Outpost/Season 01/The.Outpost.S01E05.720p.mkv
/media/series/Quiet Storm/Quiet.Storm.03x10.mkv
```

- **Pour un film**, c'est l'année qui compte le plus : `The Mirror (1999)`.
- **Pour un épisode**, le numéro de saison et d'épisode : `S01E05` ou
  `1x05`.
- Les mots en plus dans le nom sont ignorés : résolution, source, codec, son,
  mentions de langue comme `VOSTFR` ou `MULTI`, éditions comme « Director's
  Cut ».

Si vos fichiers portent un mot qu'Evey n'ignore pas encore, ajoutez-le dans
**Administration → Configuration du serveur → Jetons d'analyse** : **Jetons
de qualité supplémentaires** pour un mot seul (`HDRIP`), **Suffixes
d'édition supplémentaires** pour une expression. Cela s'applique dès le scan
suivant.

## Corriger un titre faux ou manquant { #fix-a-wrong-or-missing-title }

Un film montre l'affiche d'un autre film du même nom ? La correction la plus
rapide :

1. Ouvrez le titre et choisissez **Modifier les métadonnées**.
2. Appuyez sur **Identifier sur TMDB**, et cherchez le bon titre.
3. Appuyez sur **Choisir ce résultat**.

Autres possibilités dans **Modifier les métadonnées** :

- **Modifier un champ à la main.** Une valeur que vous changez est gardée :
  les actualisations suivantes ne l'écrasent pas. Changer l'**ID TMDB** ou
  l'**ID IMDB** va chercher les informations de cet identifiant quand vous
  enregistrez.
- **Actualiser les métadonnées** va rechercher les informations pour le titre
  reconnu actuellement.
- **Réinitialiser les métadonnées** et **Réidentifier** repartent du nom du
  fichier et abandonnent vos modifications. La réinitialisation retire aussi
  les images et la distribution, ainsi que les saisons et les épisodes d'une
  série.

Pour toute une bibliothèque, sa ligne dans **Administration →
Bibliothèques** propose :

- **Identifier les orphelins** réessaie pour les fichiers qui n'ont jamais
  été reconnus. C'est rapide.
- **Tout réidentifier avec TMDB** refait la recherche pour tous les titres.
  C'est lent. Servez-vous-en après avoir renommé beaucoup de fichiers.
- **Réparer les sagas** rattache à leur saga les films qui ne sont dans
  aucune collection, quand les autres films de la saga sont déjà dans la
  bibliothèque.

## Prise en compte des nouveaux fichiers { #how-new-files-are-picked-up }

Vous n'avez rien à faire : un fichier nouveau, renommé ou supprimé apparaît
dans Evey en quelques minutes.

Pour forcer la lecture, ou pour ajouter un scan complet régulier, utilisez
la ligne de la bibliothèque :

| Action | Ce qu'elle fait |
| --- | --- |
| **Scanner** | Lit les dossiers tout de suite. La ligne affiche **Scan en cours...**, puis le résultat. Un dossier qui n'a pas pu être lu est nommé. |
| **Planifier** | Un scan complet **Toutes les 6 heures**, **Toutes les 12 heures**, **Quotidien (3h du matin)**, **Hebdomadaire (dimanche 3h du matin)**, ou selon une **Expression cron**. |

??? info "Comment le serveur vérifie les dossiers"

    - Le serveur lit chaque bibliothèque à chaque démarrage, puis toutes les
      2 minutes (**Administration → Configuration du serveur → Bibliothèques → Intervalle d'analyse périodique**, appliqué au
      redémarrage). Pendant une lecture, il attend 15 minutes entre deux
      lectures des dossiers, pour ne pas ralentir la vidéo.
    - Le système peut aussi le prévenir quand un fichier change
      (**Administration → Configuration du serveur → Bibliothèques →
      Surveillance des fichiers de la bibliothèque**), pour les
      bibliothèques dont la surveillance en temps réel est activée. Pour
      vérifier que cela fonctionne toujours, il écrit un petit fichier de test
      dans le dossier pendant quelques secondes. Une bibliothèque où cela
      fonctionne n'est lue qu'une fois par heure au lieu de toutes les
      2 minutes.
    - Les bibliothèques créées depuis le panneau web n'utilisent pas cette
      surveillance : la lecture toutes les 2 minutes trouve leurs fichiers.
      Le formulaire d'administration de l'application mobile propose, pour une
      bibliothèque, un interrupteur **Surveillance en temps réel**.

!!! note "Partages réseau"
    Sur un partage NFS ou SMB, un fichier écrit par une autre machine
    n'envoie aucun signal au serveur. C'est la lecture régulière des
    dossiers qui le trouve.

## Bibliothèques de musique { #music-libraries }

Le scanner lit les fichiers MP3, FLAC, M4A, OGG, Opus, WAV et AAC, et prend
chaque morceau dans ses tags : titre (le nom du fichier s'il manque),
artiste, artiste de l'album, album, année, numéros de piste et de disque,
genre. Les artistes sont regroupés par artiste de l'album, ou par artiste
quand ce tag est vide.

**Un album, c'est un dossier.** Gardez chaque album dans un seul dossier.
Evey regroupe les morceaux selon le dossier où ils sont, pas selon leur tag
d'album. Deux dossiers avec le même tag d'album font donc deux albums, et un
album coupé en sous-dossiers `CD1` et `CD2` s'affiche en deux. Dans un album,
les morceaux sont classés par numéro de disque, puis par numéro de piste, et
l'album prend le nom que portent la plupart de ses morceaux.

La pochette est l'image intégrée aux fichiers, sinon une image du dossier de
l'album (`cover.jpg`, `folder.jpg` ou `front.jpg`), sinon Cover Art Archive
(trouvé via MusicBrainz), sinon iTunes.

## Bibliothèques de photos { #photo-libraries }

Le scanner lit les fichiers JPEG, PNG, HEIC, WebP, GIF, TIFF et BMP.
L'album d'une photo est son dossier, à partir du dossier de la
bibliothèque ; une photo à la racine n'en a pas, et déplacer un fichier dans
un autre dossier le fait passer dans cet album au scan suivant. La date de
prise de vue et le modèle d'appareil viennent des données EXIF des fichiers
JPEG ; les autres formats sont listés sans. Les miniatures sont
redimensionnées sur le serveur et gardées dans le cache de transcodage.

## Collections { #collections }

**Collections** regroupe des titres sous un même nom, pour tous les comptes.
Il y en a trois sortes :

- **Collections TMDB** : les sagas, remplies automatiquement quand TMDB range
  un film dans l'une d'elles. **Réparer les sagas** (plus haut) rattache les
  films qui n'ont pas été ajoutés à la leur.
- **Nouvelle collection** : une liste qu'un administrateur remplit à la main,
  avec **Ajouter à la collection** sur un titre ou sur une sélection, et qu'il
  met dans l'ordre.
- **Nouvelle collection intelligente** : des règles sur **Genre**,
  **Année**, **Type**, **Note ≥** et **Bibliothèque**, qu'un titre doit
  toutes respecter. Son contenu est recalculé à chaque ouverture : les
  nouveaux titres y apparaissent sans rien faire.

Seuls les administrateurs créent et modifient les collections. Tout le monde
peut les parcourir, et **Télécharger la collection** sur le web récupère les
fichiers d'origine dans un seul ZIP (jusqu'à 1000 titres), sauf ce que
l'accès aux bibliothèques ou la limite d'âge du compte masquent.

## Clés de métadonnées { #metadata-keys }

Evey est livré avec une clé TMDB : les métadonnées fonctionnent sans rien
régler. Les autres sources sont facultatives. Pour les utiliser, vous
demandez une clé (une sorte de mot de passe) à ce service et vous la collez
dans **Administration → Configuration du serveur → Clés API**. Une clé
réglée là passe avant la même clé donnée dans l'environnement (voir
[Environnement : bibliothèques et métadonnées](../environment/libraries.md)).

| Clé | Ce qu'elle apporte |
| --- | --- |
| TMDB (la vôtre) | Votre propre quota de requêtes au lieu du quota partagé. |
| OMDb | Une deuxième source pour les films que TMDB connaît mal. |
| OpenSubtitles | Chercher et télécharger des sous-titres qu'un fichier n'a pas, par l'API du serveur (les applications ne le proposent pas encore). Voir [Sous-titres](../playback/subtitles.md). |
| TheIntroDB | Les moments d'intro, de résumé et de générique tirés d'une base communautaire : 500 recherches par jour avec une clé, au lieu de 100 sans. |

## Intro, résumé et générique { #intro-recap-and-credits }

Evey repère l'intro, le résumé des épisodes précédents et le générique des
épisodes et des films, pour que les lecteurs proposent de les passer. Les
moments viennent de bases communautaires (TheIntroDB, puis IntroDB) quand
elles connaissent le titre. Sinon, le serveur les trouve en analysant les
fichiers. Un administrateur peut les ajuster à la main pour un titre, ou
relancer la détection.

Par défaut, les lecteurs affichent les boutons **Passer l'intro**, **Passer
le résumé** et **Passer le générique**. **Saut automatique des intros**
(**Administration → Configuration du serveur → Bibliothèques**,
désactivé par défaut) fait passer automatiquement les intros dans tous
les lecteurs.
