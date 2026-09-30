---
source_sha: 5c15d0f69d445cc33f1a7038b71f5435492c5280093f9b5d8ce87e76a2707d53
reviewed: true
---
# HDR et Dolby Vision

Une image HDR peut arriver à l'écran de trois façons : l'original, intact ; un
réencodage qui conserve le HDR ; ou un réencodage converti en SDR par tone
mapping. Laquelle s'applique dépend de l'écran du lecteur, du format HDR, et
de ce que le matériel du serveur a prouvé savoir faire.

## Quand l'original est conservé

La vidéo HDR10 ou HLG est envoyée telle quelle quand le lecteur déclare un
écran HDR et décode le codec, et qu'aucune autre raison (débit, résolution,
conteneur) n'impose un réencodage. Les règles générales sont décrites dans
[Comment Evey choisit le mode de lecture](how-playback-is-decided.md).

## Quand la vidéo doit de toute façon être réencodée

Une limite de débit, une limite de résolution ou un sous-titre à incruster
peuvent imposer le réencodage d'une source HDR. Le HDR est alors conservé si
toutes ces conditions sont réunies :

- le lecteur déclare un écran HDR et décode le HEVC ;
- le serveur dispose d'une carte graphique qui a prouvé, lors de sa mesure,
  qu'elle encode du HEVC 10 bits avec HDR ;
- cette carte est disponible pour ce flux.

Le résultat est du HEVC 10 bits avec les informations HDR10 statiques
(espace colorimétrique, fonction de transfert, affichage de mastering)
reportées. Les métadonnées dynamiques HDR10+ ne le sont pas : un titre HDR10+
est lu en HDR10 par cette voie.

Si l'une des conditions manque, le processeur ou la carte convertit l'image
en SDR avant de l'encoder. Les Infos du flux affichent alors **HDR converti
en SDR (tone-mapping)**.

Si l'original aurait pu être envoyé mais que le serveur ne peut pas encore le
servir en copie (son index de navigation est en cours de construction, par
exemple), la vidéo est réencodée en conservant le HDR sur une telle carte, et
les Infos du flux affichent **Copie indisponible, HDR conservé au
réencodage**.

## Quand l'écran est SDR

Un lecteur qui ne déclare pas d'écran HDR reçoit une image convertie en SDR.
Afficher du HDR sur un écran SDR sans conversion donne des couleurs ternes et
grisâtres : le serveur ne l'envoie donc jamais tel quel.

Le tone mapping s'exécute sur la carte graphique quand elle a prouvé qu'elle
en était capable (VAAPI avec OpenCL, par exemple), sinon sur le processeur
avec un filtre optimisé SIMD (AVX2 sur x86-64, NEON sur ARM). Sur le
processeur, c'est le transcodage le plus coûteux qui soit : le serveur mesure
un plafond de résolution distinct pour le HDR, généralement plus bas, si bien
qu'un titre HDR peut être converti dans une résolution inférieure à celle d'un
titre SDR sur la même machine.

## Dolby Vision

La déclaration d'un lecteur dit si son écran affiche le HDR, pas s'il prend en
charge le Dolby Vision, et Evey ne déduit jamais la prise en charge du Dolby
Vision de celle du HDR : envoyer du Dolby Vision à un écran qui ne gère que le
HDR10 donne de fausses couleurs. Le flux n'est copié que s'il porte une couche
de base qui s'affiche correctement sans aucun décodage Dolby Vision :

| Profil Dolby Vision | Lecteur HDR | Lecteur SDR |
| --- | --- | --- |
| **8.1** (couche de base compatible HDR10) | Traité comme du HDR10 : les règles ci-dessus s'appliquent, l'original peut être conservé. | Converti en SDR. |
| **7** (Blu-ray UHD) | La couche de base HDR10 est réencodée en HEVC 10 bits avec HDR conservé. Infos du flux : **Couche de base Dolby Vision réencodée (HDR conservé)**. | Converti en SDR. |
| **5** (sans couche de base HDR10) | Converti en SDR. | Converti en SDR. |

## Vérifier ce qui s'est passé

Les **Infos du flux** montrent la décision vidéo et, parmi les raisons, le
tone mapping ou le HDR conservé. Toutes les routes qui font du tone mapping
l'y signalent, à une exception près : quand la carte graphique est saturée au
moment où un flux démarre, le flux passe sur le processeur après le calcul
des Infos du flux. **Admin → En cours de lecture** indique le backend
réellement utilisé.
