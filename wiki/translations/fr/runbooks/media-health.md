---
source_sha: 4abf3c80428cd67c34ec53516788e91cc6019eab33ecc17b138c08d22a4bde30
---

# Santé des médias (admin → Media health)

La page Media health inspecte chaque MKV/WebM d'une bibliothèque et donne à
chaque fichier UN verdict. L'objectif : réparer pour de bon ce qui peut l'être,
et cesser de relire éternellement les mêmes avertissements - pas d'entretenir
une liste de 600 constats sur lesquels personne n'agit.

## Les quatre verdicts

| Verdict | Ce que cela veut dire | Ce que le serveur en fait |
|---|---|---|
| **Healthy** | Index, calage audio, conteneur et tailles : tout est bon. | Rien. |
| **Repairable** | Un défaut que le serveur sait corriger seul, sans risque. | **Repair all** les corrige tous en arrière-plan (voir plus bas). |
| **Your call** | Un défaut que seule la source peut corriger, ou une réparation qui se paie d'une vraie contrepartie. | Rien d'automatique. Acquittez-le, ou récupérez le fichier à nouveau. |
| **Info** | Une curiosité sans effet sur la lecture. | Rien. Acquittez-le pour dégager la vue. |

Chaque ligne porte aussi les badges de détail qui expliquent son verdict. Les
badges, groupés par verdict :

**Repairable**

- Index de navigation *manquant*, ou index présent mais inutilisable (calé sur
  la piste audio, référençant des pistes que le fichier n'a plus, ou trop
  clairsemé pour faire atterrir une navigation près de sa cible). La réparation
  reconstruit les Cues : le fichier est réécrit dans une copie temporaire,
  entièrement revalidé (pistes, durée, index), puis échangé. Le symptôme qu'elle
  guérit : navigation et reprise lentes.
- *Audio +Ns* - une piste audio démarre en retard sur la vidéo (un défaut de
  multiplexage, le classique « les lèvres en avance sur le son »). La réparation
  réaligne la piste sur place.
- *Wrong container* - le `.mkv` est en réalité un MP4/MOV. Il n'y a aucun index
  Matroska à construire ; la réparation **le renomme en `.mp4`** et met à jour
  l'entrée de bibliothèque. Rien n'est réécrit.
- *Damaged ~size* - une zone corrompue au milieu du fichier, qu'une réécriture
  stricte refuse. La réparation peut **resynchroniser** : elle saute la zone
  endommagée et resynchronise ce qui suit. Ces octets sont perdus pour de bon,
  et la fenêtre de réparation le demande donc explicitement (*Also resync N
  damaged file(s)*) - case décochée, les fichiers endommagés sont laissés
  tranquilles.

**Your call**

- *Incomplete download - missing tail ~size* - la fin déclarée du fichier
  dépasse sa taille réelle, et le parcours a confirmé la coupure. Aucun outil ne
  récupère cette queue manquante : la taille vous dit s'il s'agit d'un talon de
  9 Ko (invisible à la lecture) ou d'une scène finale absente. Le seul vrai
  correctif est de récupérer la source à nouveau.
- *Audio -Ns* - une piste audio s'arrête N secondes **avant l'image** (mesure du
  contenu contre le contenu, jamais contre la durée déclarée du conteneur, qui
  n'est qu'une affirmation du multiplexeur). La lecture comble la fin par du
  silence ; le son n'est tout simplement pas dans le fichier.
- *Coarse seeking* - l'index a été reconstruit et un trou subsiste. Le détail en
  dit la raison : des images clés que personne n'a indexées (une seconde
  reconstruction referme le trou), une zone totalement dépourvue d'image clé
  (seul un réencodage la rend navigable), ou **une zone totalement dépourvue
  d'image** - la vidéo y gèle quoi que fasse l'index, et le seul remède est la
  source.

**Info**

- *Trailing bytes* - des octets en trop après la fin déclarée du Segment (un
  ajout interrompu, le journal d'un client torrent). Le média est complet et se
  lit très bien ; toute réécriture ultérieure les fera disparaître.

## Acquitter

*Seen, stop flagging* (par fichier) et *Mark all as seen* (par groupe) sortent
un constat de la vue *Awaiting*. L'acquittement est lié à l'identité du fichier
**et à ses octets** : si le fichier change (taille ou date de modification),
l'analyse suivante le diagnostique à neuf et le signalement revient. *Flag
again* annule l'acquittement. Un acquittement posé pendant qu'une analyse tourne
est conservé quand cette analyse s'achève, pour un fichier resté inchangé.

Les vues en haut du tableau (*Awaiting*, *Repairable*, *Your call*, *Info*,
*Seen*, *Backups*, *All*), plus le champ de recherche, sont ce qui permet
d'avancer sur une grande bibliothèque : *Awaiting* est la liste des choses à
faire, *Seen* ce sur quoi vous avez déjà tranché.

## Les analyses, et pourquoi la première après une mise à jour est longue

Une analyse lit chaque fichier en en-tête seul (quelques Ko chacun) et réemploie
le verdict précédent quand le fichier n'a pas changé. Deux choses forcent un
diagnostic neuf de tout :

- le fichier a changé (taille ou date de modification) ;
- **les détecteurs du serveur ont changé** - une nouvelle règle, une référence
  corrigée, un seuil déplacé. Chacun de ces changements incrémente une version
  interne de diagnostic, et l'analyse suivante rediagnostique toute la
  bibliothèque une bonne fois, plutôt que d'afficher un verdict qu'une règle
  plus récente ne rendrait pas. Sur un NAS, comptez des heures pour une grande
  bibliothèque ; les analyses suivantes sont de nouveau rapides.

## Sauvegardes et retour arrière

**Lisez ceci avant de vous jeter sur *Roll back* en plein incident :
l'existence d'une copie physique de l'original dépend de la stratégie de
réparation, et l'une des trois n'en garde aucune.**

| Stratégie | Sauvegarde physique | Comment se passe un retour arrière |
|---|---|---|
| `rewrite` | un `.mkvgobak` à côté du fichier | On restaure la copie |
| `auto` | un `.mkvgobak` (avec repli sur une réécriture) | On restaure la copie |
| `inplace` | **aucune, jamais** | On réapplique le décalage inverse consigné dans le journal de réparation |

`inplace` est un correctif chirurgical - une écriture dans le fichier, sans
remultiplexage - et c'est le chemin qu'emprunte un réalignement audio (le badge
« Audio +Ns »). Rien n'est copié : le chemin du retour, c'est l'entrée de
journal dans `audit_log`, qui porte le décalage appliqué, sachant que le
décalage opposé le défait. Si cette entrée de journal a disparu, le retour
arrière aussi. `admin_media.proto` fait référence pour les trois stratégies ;
`inplace` ignore aussi bien *Keep backups* que le nombre de sauvegardes à
conserver.

Pour les deux stratégies qui gardent bien une copie : *Roll back* restaure un
fichier, *Roll back all* restaure le périmètre sélectionné. Quand les
sauvegardes menacent de déborder le volume, la page le dit et propose de les
désactiver - seules les dernières sont alors gardées pendant la tâche puis
supprimées à la fin, chaque réparation ayant été validée avant l'échange.

Les réparations exigent que les dossiers de médias soient montés **en
lecture-écriture** dans le conteneur (pas de `:ro` sur le montage lié) - la page
vous indique quels fichiers reposent sur un volume en lecture seule.

## Les équivalents en ligne de commande

Le même moteur est accessible depuis la CLI d'exploitation (voir `cli.md`) :

- `evey admin reindex-no-cue --root <dir> [--apply] [--resync]` - la
  reconstruction de l'index, à blanc par défaut ;
- `evey admin validate-streaming --root <dir>` - le rapport, en lecture seule,
  d'aptitude à la diffusion.
