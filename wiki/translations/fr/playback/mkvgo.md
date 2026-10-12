---
source_sha: 91a46caa213d6048de832a4352bd5972b5f2d393a40a66301f693b3d908f084c
reviewed: true
---
# MKVGo dans Evey

**En bref :**

- MKVGo est une partie d'Evey qui lit les fichiers vidéo (MKV, WebM, MP4,
  MOV) et les remet dans un autre format de fichier, sans convertir l'image
  ni le son.
- Quand c'est possible, c'est elle qui sert les lectures en **Remux**, sort
  les sous-titres des fichiers, et lit vos fichiers pendant un scan. Tout ce
  qui demande une vraie conversion passe par FFmpeg, un outil vidéo à part.
- Vous n'avez rien à régler. **Administration → En cours de lecture** montre,
  piste par piste, si une lecture a été servie par MKVGo ou par FFmpeg.

Le reste de cette page s'adresse à qui veut savoir ce que le serveur fait
sans lancer FFmpeg, et où FFmpeg prend encore le relais. MKVGo tourne à
l'intérieur du processus du serveur Evey.

MKVGo travaille sur la structure d'un fichier : ses en-têtes, son index, ses
pistes et leurs paquets. Elle ne décode ni n'encode jamais une image ou un
son ; tout ce qui demande un décodage côté serveur passe par FFmpeg. Elle est
développée comme une bibliothèque Go à part :
[github.com/gravity-zero/mkvgo](https://github.com/gravity-zero/mkvgo).

## Trois niveaux de travail

| Niveau | Ce qui se passe | Ce que cela coûte au serveur |
| --- | --- | --- |
| **Lecture directe** | Le fichier est servi tel quel. | Une lecture de fichier. |
| **MKVGo** | Le travail structurel : lire un fichier, réempaqueter ses pistes pour la diffusion, reconstruire son index, déplacer une piste sur la ligne de temps. | Du code dans le processus du serveur : aucun FFmpeg ni ffprobe lancé par flux, et aucun segment écrit sur le disque pour l'empaquetage à la demande. |
| **FFmpeg** | Les vraies transformations : réencodage, tone mapping, décodage d'une image. Et le repli chaque fois que MKVGo ne sait pas traiter un fichier. | Un processus par tâche. |

Une copie FFmpeg (`-c copy`) ne réencode rien non plus et reste peu coûteuse ;
ce que MKVGo économise en plus, c'est le processus lancé pour chaque session et
les segments écrits dans le cache de transcodage. Le réencodage est le cas
coûteux, et un réencodage vidéo est le seul travail qui occupe un créneau de
transcodage : voir [Transcodage et capacité du serveur](transcoding.md).

Le démarrage et le déplacement n'attendent ni un processus ni un premier
segment sur le disque : le segment est construit à la demande à partir de
l'index du fichier. Qu'un fichier emprunte cette voie dépend de ce que le
lecteur sait décoder :
[Comment Evey choisit le mode de lecture](how-playback-is-decided.md).

## Ce que fait MKVGo

| Travail | Où on le voit | Quand MKVGo ne peut pas |
| --- | --- | --- |
| **Lecture des fichiers** | Pendant l'analyse d'une bibliothèque, codecs, résolution, HDR, débit et durée sont lus dans l'en-tête du fichier. | ffprobe le lit. Il prend aussi le relais au moindre doute : une erreur d'analyse, ou des informations de couleur que MKVGo n'a pas pu lire sur un codec capable de HDR. |
| **Index des images clés** | Les positions d'où la lecture peut partir, lues dans l'index du fichier ; elles permettent au remux et à la copie vidéo de couper les segments sur des images clés. | ffprobe. |
| **Diffusion sans réencodage** | Remux : segments HLS en MP4 fragmenté (CMAF), construits à la demande à partir d'un plan gardé par fichier. Un segment est écrit sur la connexion directement depuis le fichier source, au fil de l'envoi : rien n'est assemblé en mémoire au préalable, rien n'est écrit dans le cache de transcodage. Un manifeste DASH est produit sur les mêmes segments ; les applications utilisent HLS. | FFmpeg copie les pistes, toujours sans réencoder, dans des segments écrits dans le cache. |
| **MP4 pour le navigateur** | Un fichier Matroska qu'un navigateur ne sait pas ouvrir est réécrit une fois en MP4 à démarrage rapide, gardé en cache. | - |
| **Sous-titres** | Les pistes de sous-titres intégrées à un MKV sont extraites à la demande, les pistes texte en WebVTT et les pistes image (PGS) en images. Quand l'index propre du fichier (les Cues) couvre entièrement une piste, le serveur ne lit que les blocs qu'il désigne au lieu du fichier entier, et garde cet index des blocs sous son répertoire de données (`subtitle-index/`) pour la demande suivante. Une piste qui demande encore une lecture complète reçoit une réponse « en préparation » avec un délai de nouvel essai, et les lecteurs réinterrogent jusqu'à ce qu'elle soit prête. | ffmpeg extrait la piste en lisant le fichier de bout en bout. |
| **Images d'aperçu** | L'image clé la plus proche de la position demandée est trouvée grâce à l'index, et FFmpeg ne décode que cette image. Utilisé quand cette image clé est à environ deux secondes ou moins de la demande. | FFmpeg se positionne et décode jusqu'à la position exacte. |
| **Épisodes enchaînés** | Le serveur peut empaqueter plusieurs épisodes en un seul flux continu (`?gapless=1` dans l'adresse du flux). Aucune application ne propose encore l'option. | Chaque épisode est lu séparément. |
| **Santé des médias** | Le diagnostic (index de lecture, début de l'audio, pistes rangées par blocs), l'index reconstruit, la resynchronisation après une zone abîmée, et le décalage d'une piste audio. Voir [Santé des médias](../runbooks/media-health.md). | Le remux des pistes rangées par blocs est fait par FFmpeg. |

## Ce qui reste à FFmpeg

- Tout réencodage, tone mapping HDR compris.
- Le décodage d'images : les miniatures de navigation, l'image d'un aperçu.
- Les analyses qui demandent des données décodées : détection de
  l'entrelacement, empreintes audio pour la détection des génériques.
- Les conteneurs autres que MKV/WebM et MP4/MOV, comme l'AVI ou le MPEG-TS.

## Charge, fichiers remplacés et mises à jour

**Ce qu'il lit à la fois.** Construire un segment lit une fenêtre de la
source, vidéo et audio ensemble : 6 à 10 Mio pour un fichier 1080p, environ
15 Mio pour un remux UHD. Le serveur lit autant de fenêtres à la fois qu'il a
de processeurs, et moins sur une machine qui a peu de mémoire : toutes les
fenêtres en cours doivent tenir dans un quart de la mémoire que le serveur
s'accorde (la moitié de la limite du conteneur, ou de la mémoire de l'hôte).
Les demandes au-delà attendent une place libre, si bien que la mémoire
utilisée ne grandit pas avec l'audience ; sur une petite machine, une foule
qui démarre au même moment voit ses premiers segments arriver les uns après
les autres plutôt que de voir le serveur manquer de mémoire.

**Un fichier remplacé pendant la lecture.** Chaque adresse de segment nomme le
fichier à partir duquel il a été planifié. Quand un fichier est remplacé sur
le disque (une meilleure copie, une réparation), un segment demandé à ce
moment est reconstruit une fois à partir du nouveau fichier, et les adresses
de la liste de lecture de l'ancien fichier répondent `410 Gone` plutôt que de
mélanger les deux fichiers ; rouvrir le titre lit le nouveau.

**Après une mise à jour.** Les adresses des segments portent la version de
MKVGo : un navigateur ou un cache de proxy ne sert jamais un segment construit
par une version plus ancienne, et aucun segment de cette voie n'est gardé sur
le disque, il n'y a donc rien à vider. Les copies MP4 faites pour les
navigateurs portent la version de leur format dans leur nom : quand elle
change, chacune est reconstruite la prochaine fois qu'on la demande.

**Le surveiller.** Sur `GET /metrics` (voir
[Supervision et logs](../operations/monitoring.md#prometheus-metrics)),
`evey_mkvgo_walks_in_flight` donne le nombre de fenêtres en cours de lecture,
et `evey_mkvgo_plan_stats` additionne les compteurs de l'empaqueteur par `stat`.
Un `walks_in_flight` bloqué à son plafond pendant que `stat="slot_waits"`
grimpe signifie que des spectateurs font la queue pour le processeur ;
`stream_fallbacks` compte les segments qui ont dû être assemblés en mémoire
au lieu d'être diffusés directement.

## Voir quel moteur sert un flux

**Administration → Tableau de bord** et **Administration → En cours de lecture** indiquent le
moteur de chaque piste : `mkvgo` ou `ffmpeg`. Un remux peut afficher les deux,
par exemple la vidéo empaquetée par MKVGo et l'audio copié par FFmpeg.
