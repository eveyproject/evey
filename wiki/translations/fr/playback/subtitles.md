---
source_sha: 03f8a2ec1721f109a072f8ecbb96e6a41ae358c2b550d1000f1987bf6e0a6f8a
reviewed: true
---
# Sous-titres

Comment un sous-titre s'affiche sur votre appareil, et si le choisir change
la façon dont la vidéo est livrée. Pour les spectateurs ; le seul réglage qui
compte est l'**Incrustation des sous-titres** du profil, dans **Paramètres →
Lecture**. En bref : les sous-titres texte et Blu-ray ne touchent jamais à la
vidéo, les sous-titres DVD et de diffusion sont incrustés et imposent un
réencodage.

## Ce que chaque format fait à la vidéo

| Format | Exemples | Affichage | Effet sur la vidéo |
| --- | --- | --- | --- |
| Texte | SRT, ASS/SSA, WebVTT, sous-titres intégrés au fichier | Envoyé à côté de la vidéo et dessiné par le lecteur, avec votre **Style des sous-titres** | Aucun |
| PGS (Blu-ray) | `hdmv_pgs_subtitle` | Décodé en images une fois par le serveur, mis en cache, puis dessiné tel quel par le lecteur | Aucun |
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

| Réglage | Une piste VobSub, DVB ou XSUB sélectionnée | Texte et PGS |
| --- | --- | --- |
| **Auto** | Incrustée, la vidéo est réencodée. | Dessinés par le lecteur. |
| **Toujours incruster** | Comme Auto. | Dessinés par le lecteur : le texte n'est jamais incrusté, quel que soit le réglage. |
| **Jamais incruster** | Ne peut pas s'afficher : la vidéo n'est jamais réencodée pour un sous-titre. | Dessinés par le lecteur. |

## Choisir et ajuster

- Les lecteurs sélectionnent le sous-titre de la **Langue des sous-titres par
  défaut** du profil, puis la piste que le fichier marque par défaut (une
  piste forcée, par exemple). Une piste choisie à la main n'est jamais
  remplacée, et un nouveau titre repart de zéro.
- **Synchro sous-titres**, dans le lecteur, décale les sous-titres du titre
  en cours (une valeur positive les affiche plus tard). L'effet est
  immédiat.
- **Style des sous-titres** règle la taille, la couleur du texte, le fond, la
  police, l'ombre et la position des sous-titres texte. Les images PGS sont
  dessinées telles quelles.
- **Téléverser un sous-titre…** ajoute un fichier au titre.
- Avec une clé OpenSubtitles (voir
  [Bibliothèques et métadonnées](../administration/libraries.md#metadata-keys)),
  l'API du serveur permet de chercher et télécharger un sous-titre absent du
  fichier (`GET /api/v1/media/{id}/subtitles/search`, puis
  `POST /api/v1/media/{id}/subtitles/download`). Le fichier est enregistré
  dans un dossier `.subtitles` à côté du film. Les applications ne proposent
  pas cette recherche.
