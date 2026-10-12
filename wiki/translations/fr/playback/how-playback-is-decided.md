---
source_sha: 6edbda5d141c4bdf890a36b49ddaf14c02a6bf242923470295dde702a2d0d3b5
reviewed: true
---
# Comment Evey choisit le mode de lecture

Pourquoi un film est-il lu tel quel sur la TV mais converti sur le téléphone ?

**En bref :**

- Vous ne choisissez jamais de qualité. Quand vous appuyez sur lecture, le
  serveur regarde le fichier, ce que votre appareil sait lire, et sa propre
  charge. Puis il choisit la façon la plus légère qui fonctionne.
- Il y a trois façons : **Lecture directe** (le fichier tel quel), **Remux**
  (la même image et le même son, remis dans un autre format de fichier), et
  **Transcodage** (une partie du film convertie, souvent seulement le son).
- Le même film peut donc partir intact vers un appareil et être converti pour
  un autre, ou être lu différemment sur le même appareil quand le serveur est
  chargé.
- En général, vous n'avez rien à faire. Pour voir quelle façon vous avez eue,
  et pourquoi, ouvrez les **Infos du flux** dans le lecteur : voir
  [Lire les Infos du flux](#reading-stream-info).

Les trois façons et la lecture des **Infos du flux** viennent d'abord. Les
règles exactes que le serveur applique suivent, pour qui veut les comprendre
ou les ajuster.

## Les trois modes de lecture

| Dans les Infos du flux | Ce que reçoit le lecteur | Ce que cela coûte au serveur |
| --- | --- | --- |
| **Lecture directe** | Le fichier d'origine, tel quel. | Une lecture de fichier. |
| **Remux** | La vidéo et l'audio d'origine, placés dans un autre conteneur. Rien n'est réencodé. | Un réempaquetage, fait par [MKVGo](mkvgo.md) dans le serveur quand c'est possible, par une copie FFmpeg sinon. |
| **Transcodage** | Au moins une piste réencodée. Souvent l'audio seul, la vidéo restant celle d'origine. | Un encodage. Seul un réencodage vidéo occupe un [créneau de transcodage](transcoding.md#sharing-the-capacity). |

Le serveur les essaie dans cet ordre et, quand il transcode, ne réencode que
les pistes qui l'exigent.

## Lire les Infos du flux

Les **Infos du flux** (le bouton d'information, (i), des contrôles du
lecteur ; sur la TV, dans le menu d'options du lecteur) affichent la
**Méthode**, puis chaque piste avec sa décision (*copié*, *transcodé*, ou
*décodé client* pour un audio que le lecteur web décode lui-même) et, sous
**Pourquoi pas de lecture directe**, les raisons.

<figure class="evey-shot evey-shot--narrow" markdown="span">
  ![Les Infos du flux dans le lecteur web, pour un remux](../images/player-stream-info.webp){ loading=lazy }
  <figcaption>Remux : l'emballage change, les pistes vidéo et audio restent intactes, et la seule raison pour laquelle le fichier n'est pas lu tel quel. « Copié » décrit les pistes, pas le mode de lecture (interface en anglais).</figcaption>
</figure>

Les raisons se répartissent en quatre catégories :

| Origine | Exemples, tels qu'affichés |
| --- | --- |
| Le lecteur ne peut pas exploiter la piste telle quelle | Codec vidéo non lisible par ce lecteur · Codec audio non lisible par ce lecteur · Résolution au-dessus de la limite du lecteur · Trop de canaux audio pour ce lecteur · Format de pixels que le lecteur ne décode pas (H.264 10 bits, 4:2:2) · Conteneur non lisible en l'état (remis en HLS) |
| Une limite propre à la session | Débit au-dessus de la limite autorisée · Le débit mesuré par le client ne peut pas porter la copie |
| Une conséquence sur l'image ou le son | HDR converti en SDR (tone-mapping) · Sous-titres incrustés dans l'image · Resynchronisation audio demandée |
| Un réglage, ou une décision déjà prise | Lecture directe désactivée dans le profil · Remux désactivé dans le profil · Codec vidéo préféré (réglage du profil) · Choix figé pour cette session |

Seule la piste audio écoutée est prise en compte : un fichier dont la piste de
commentaire, non sélectionnée, est en DTS ne sera pas transcodé pour autant.

Les Infos du flux sont calculées au lancement de la lecture. Un seul cas leur
échappe : une carte graphique saturée au moment précis où le flux démarre. Le
flux bascule alors sur le processeur, et **Administration → En cours de lecture**
indique le backend réellement utilisé.

## Ce qui entre dans la décision

**Le fichier.** Codec, résolution, profondeur de couleur, format HDR, débit,
pistes audio et sous-titres, relevés au moment du scan.

**Le lecteur.** Au lancement, l'application indique ce que son appareil sait
décoder : les familles de codecs prises en charge, les conteneurs qu'il sait
ouvrir, sa résolution maximale, si son écran affiche le HDR, le nombre de
canaux audio qu'il restitue et, pour certains appareils, un plafond ou un
niveau plus bas en H.264. L'application web interroge le navigateur ; les
applications Android et Android TV interrogent les décodeurs et l'écran de
l'appareil. Evey ne tient aucune liste d'appareils compatibles : il s'appuie
sur ce que le lecteur déclare. En diffusion, c'est le récepteur qui se décrit
lui-même quand il le peut, et un profil prudent est utilisé sinon : voir
[Diffusion (Chromecast)](casting.md).

**La session.** Les préférences du profil (débit maximal, nombre maximal de
canaux audio, traitement des sous-titres) et les limites qui s'appliquent à ce
spectateur, détaillées plus bas.

**Le serveur.** Sa capacité à encoder ce flux en temps réel, sur sa carte
graphique ou son processeur, et la disponibilité d'un créneau de transcodage.
Rien de tout cela n'est supposé : c'est mesuré. Voir
[Transcodage et capacité du serveur](transcoding.md).

## Un fichier, trois résultats

Prenons un film 4K en HEVC HDR10 à 70 Mb/s, avec une piste Dolby TrueHD 7.1,
lu sur une Android TV dont le décodeur gère le HEVC et dont l'écran affiche le
HDR :

- **Sans limite de débit.** La TV décode la vidéo et affiche le HDR : la vidéo
  est conservée telle quelle. Elle ne décode pas le TrueHD : seul l'audio est
  réencodé. Les Infos du flux affichent **Transcodage**, avec la vidéo
  *copiée*.
- **Avec une limite à 20 Mb/s et une carte graphique capable d'encoder du HEVC
  10 bits en HDR.** La vidéo doit être réencodée pour respecter la limite. Le
  serveur la réencode en HEVC en conservant le HDR, puisque la TV comme la
  carte en sont capables.
- **Même limite, mais carte graphique saturée ou indisponible pour ce titre.**
  Le processeur prend le relais. Il encode en H.264, qui ne transporte pas de
  HDR ici : l'image est convertie en SDR par tone mapping, à la qualité que le
  processeur tient.

Ni le fichier ni la TV n'ont changé : seules la limite de la session et l'état
du serveur ont varié.

## Les limites de la session

Un titre dont le débit (vidéo et piste audio écoutée) dépasse la plus basse de
ces limites est transcodé pour s'y conformer :

- le maximum déclaré par le lecteur ;
- le **Débit maximal** du compte, fixé par un administrateur ;
- le **Débit maximum en streaming** du profil, et son **Débit maximum en
  mobile** quand un téléphone est sur le réseau cellulaire ;
- le **Débit distant maximal** (**Administration → Accès distant**) et le **Plafond de
  débit vidéo, spectateurs distants** (**Administration → Configuration transcodage**),
  le plus bas des deux, quand l'adresse du spectateur est publique et hors des
  **Réseaux LAN**. Une adresse privée n'est jamais plafonnée.

La piste audio écoutée est réencodée quand elle compte plus de canaux que le
lecteur n'en déclare ou que les **Canaux audio maximum** du profil. Une piste
réencodée garde au plus 5.1 canaux, jamais davantage : voir [Audio](audio.md).

## Les contraintes que le serveur applique lui-même

Certaines limites ne sont pas encore déclarées par les lecteurs. Le serveur
les applique alors comme une règle de compatibilité prudente, identique pour
tous : le H.264 en 10 bits, et toute vidéo en 4:2:2, en 4:4:4 ou en 12 bits,
sont réencodés, alors que le HEVC, le VP9 et l'AV1 jusqu'à 10 bits en 4:2:0
dépendent de la prise en charge du codec par le lecteur. Le Dolby Vision n'est
conservé que sous une forme que le lecteur affiche correctement : voir
[HDR et Dolby Vision](hdr.md).

## Pourquoi le mode change rarement en cours de film

La décision est prise au lancement et conservée pendant toute la session, pour
que le lecteur ne reçoive jamais des segments issus de deux encodages
différents. Une nouvelle décision n'intervient que si vous changez de piste
audio ou de sous-titres d'une façon qui l'exige, ou si vous relancez le titre.

## Imposer un mode de lecture

**Paramètres → Lecture** propose, pour chaque profil, **Autoriser la lecture
directe**, **Autoriser le remux** et **Autoriser le transcodage**. Désactiver
l'une de ces options interdit ce mode au serveur, et les Infos du flux
indiquent pourquoi.

!!! note
    C'est un outil de diagnostic, pas un réglage de qualité : un transcodage
    imposé n'est jamais meilleur que l'original.

La **Qualité**, dans les réglages du lecteur, joue un autre rôle : elle choisit
parmi les qualités d'un flux transcodé. **Auto** s'adapte à la connexion ; une
résolution (1080p, 720p...) fixe cette qualité ; **Original** lève le plafond
de résolution que le lecteur demanderait sinon.

## Pour aller plus loin

- Les critères en détail : [HDR et Dolby Vision](hdr.md) · [Audio](audio.md) ·
  [Sous-titres](subtitles.md) · [Diffusion (Chromecast)](casting.md)
- La répartition de la capacité du serveur :
  [Transcodage et capacité du serveur](transcoding.md)
- Ce qui tourne dans le serveur sans FFmpeg : [MKVGo dans Evey](mkvgo.md)
- L'implémentation : [Chaîne de lecture](../playback-serving.md)
