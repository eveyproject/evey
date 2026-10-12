---
source_sha: a631c73d13e67b6253b0aa9e7c5c362475b1e0fcb6153effecea88bd092feca5
reviewed: true
---
# HDR et Dolby Vision

**En bref :**

- Le **HDR** est une image avec des zones claires plus lumineuses et des
  couleurs plus riches que l'image habituelle (appelée **SDR**). Dolby Vision
  est une sorte de HDR. Le HDR demande un écran capable de l'afficher.
- Sur un écran HDR avec l'application Android ou Android TV (et un appareil
  qui décode le HEVC), vous avez l'image HDR d'origine, tant que rien d'autre
  n'oblige à convertir.
- Dans un navigateur web, et sur tout écran qui n'est pas HDR, le serveur
  convertit l'image en SDR. C'est voulu : du HDR affiché sur un écran SDR
  paraît gris et délavé.
- Convertir du HDR en SDR est le travail le plus lourd que puisse faire le
  processeur. Une carte graphique aide : voir
  [Accélération matérielle](../administration/hardware-acceleration.md).

Le reste de cette page explique chaque cas : l'image d'origine, un
réencodage qui garde le HDR, ou une conversion en SDR. Les règles générales
de lecture sont dans
[Comment Evey choisit le mode de lecture](how-playback-is-decided.md).

## Ce que vous verrez, selon l'appareil

| Votre écran | Ce qui arrive |
| --- | --- |
| Un écran HDR sur Android TV ou sur un téléphone Android, avec un décodeur HEVC | L'original HDR10 ou HLG, intact, sauf si autre chose (débit, résolution, sous-titre incrusté) impose un réencodage. |
| Un navigateur | Une conversion en SDR, toujours : le lecteur web ne déclare pas d'écran HDR, le serveur convertit donc pour lui. |
| Un récepteur de diffusion | Ce que le récepteur a déclaré sur son propre écran : voir [Diffusion](casting.md). |
| Un écran SDR, où que ce soit | Une conversion en SDR. |

Afficher du HDR sur un écran SDR sans conversion donne des couleurs ternes et
grisâtres : le serveur ne l'envoie donc jamais tel quel. Les applications
Android sondent l'écran à chaque début de lecture, pas une fois pour toutes à
l'installation : brancher un téléviseur HDR sur un boîtier prend effet au
titre suivant, au changement de piste ou de qualité.

## Quand l'original est conservé

La vidéo HDR10 ou HLG est envoyée telle quelle quand le lecteur déclare un
écran HDR et décode le codec, et qu'aucune autre raison (débit, résolution,
conteneur) n'impose un réencodage.

## Quand la vidéo doit de toute façon être réencodée

Une limite de débit, une limite de résolution ou un sous-titre à incruster
peuvent imposer le réencodage d'une source HDR. Le HDR est alors conservé si
toutes ces conditions sont réunies :

- le lecteur déclare un écran HDR et décode le HEVC ;
- le serveur dispose d'une carte graphique qui a prouvé, lors de sa mesure,
  qu'elle encode du HEVC 10 bits avec HDR ;
- cette carte est disponible pour ce flux.

Le résultat est du HEVC 10 bits avec la signalisation HDR de la source
(couleurs BT.2020, transfert PQ ou HLG) reportée. Les métadonnées dynamiques
HDR10+ ne le sont pas : un titre HDR10+ est lu en HDR10 par cette voie.

Si l'une des conditions manque, le processeur ou la carte convertit l'image
en SDR avant de l'encoder. Les Infos du flux affichent alors **HDR converti
en SDR (tone-mapping)**.

Si l'original aurait pu être envoyé mais que le serveur ne peut pas encore le
servir en copie (son index de lecture est en cours de construction, par
exemple), la vidéo est réencodée en conservant le HDR sur une telle carte, et
les Infos du flux affichent **Copie indisponible, HDR conservé au
réencodage**.

## Quand l'écran est SDR

Un lecteur qui ne déclare pas d'écran HDR reçoit une image convertie en SDR.
Le tone mapping s'exécute sur la carte graphique quand elle a prouvé qu'elle
en était capable (VAAPI avec OpenCL, par exemple), sinon sur le processeur :
avec un filtre optimisé SIMD (AVX2) quand la version de ffmpeg le fournit,
avec une chaîne de filtres portable sinon.

!!! note
    Sur le processeur, le tone mapping est le transcodage le plus coûteux qui
    soit. Le serveur mesure un plafond de résolution distinct pour le HDR,
    généralement plus bas, si bien qu'un titre HDR peut être converti dans une
    résolution inférieure à celle d'un titre SDR sur la même machine. Voir
    [Transcodage](transcoding.md#the-server-measures-itself).

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
des Infos du flux. **Administration → En cours de lecture** indique le backend
réellement utilisé.
