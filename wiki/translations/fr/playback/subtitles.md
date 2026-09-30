---
source_sha: 3fced19f436765baa59845968676caa83737f9a681cd41288d6e854fcde880ae
reviewed: true
---
# Sous-titres

Le sort d'un sous-titre dépend de son format : du texte, ou des images.

| Format | Exemples | Affichage | Effet sur la vidéo |
| --- | --- | --- | --- |
| Texte | SRT, ASS/SSA, WebVTT, sous-titres intégrés au fichier | Envoyé à côté de la vidéo et dessiné par le lecteur | Aucun |
| PGS (Blu-ray) | `hdmv_pgs_subtitle` | Décodé en images une fois par le serveur, mis en cache, puis dessiné par le lecteur | Aucun |
| Autres formats image | VobSub (DVD), DVB, XSUB | Incrusté dans l'image par le serveur | La vidéo est réencodée |

Tous les lecteurs (web, Android, Android TV) dessinent eux-mêmes les
sous-titres texte et PGS : en choisir un ne change jamais la façon dont la
vidéo est livrée. Seule la dernière ligne a un effet : incruster des
sous-titres dans l'image oblige à décoder puis réencoder la vidéo, si bien
qu'un titre lu en lecture directe passe en transcodage. Les Infos du flux
affichent alors **Sous-titres incrustés dans l'image**.

Un sous-titre image n'est incrusté que si vous le sélectionnez réellement. Un
fichier qui se contente d'en contenir un, ou de le marquer par défaut, garde
sa lecture directe.

## Incrustation des sous-titres

Le réglage de profil **Incrustation des sous-titres** (**Paramètres →
Lecture**) s'applique aux formats de la dernière ligne :

- **Auto** et **Toujours incruster** : une piste VobSub, DVB ou XSUB
  sélectionnée est incrustée. Le texte n'est jamais incrusté, quel que soit
  le réglage.
- **Jamais incruster** : la vidéo n'est jamais réencodée pour un sous-titre.
  Une piste VobSub, DVB ou XSUB ne peut alors pas s'afficher.

## Choisir et ajuster

- Les lecteurs sélectionnent le sous-titre de la **Langue des sous-titres par
  défaut** du profil, puis la piste que le fichier marque par défaut (une
  piste forcée, par exemple).
- **Synchro sous-titres**, dans le lecteur, décale les sous-titres du titre
  en cours (une valeur positive les affiche plus tard). L'effet est
  immédiat.
- **Style des sous-titres** règle la taille, la couleur, le fond, la police
  et la position des sous-titres texte.
- **Téléverser un sous-titre…** ajoute un fichier au titre.
- Avec une clé OpenSubtitles (voir
  [Bibliothèques et métadonnées](../administration/libraries.md#metadata-keys)),
  l'API du serveur permet de chercher et télécharger un sous-titre absent du
  fichier (`GET /api/v1/media/{id}/subtitles/search`, puis
  `POST /api/v1/media/{id}/subtitles/download`). Le fichier est enregistré
  dans un dossier `.subtitles` à côté du film. Les applications ne proposent
  pas encore cette recherche.
