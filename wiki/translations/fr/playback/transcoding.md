---
source_sha: e039606b33ab2207d3774f9bacb71ee3d18f0d12f52d9dc02bec1e9817e1c91e
reviewed: true
---
# Transcodage et capacité du serveur { #transcoding-and-server-capacity }

**En bref :**

- Le **transcodage**, c'est convertir une vidéo dans une forme que
  l'appareil sait lire. C'est le seul gros travail que fait le serveur : lire
  un fichier tel quel (Lecture directe) ou le remettre dans un autre format
  de fichier (Remux) ne coûte presque rien. Voir
  [Comment Evey choisit le mode de lecture](how-playback-is-decided.md).
- Vous ne réglez pas combien de personnes le serveur peut servir. Il se
  mesure lui-même dès son premier moment calme, en quelques minutes, et n'en
  accepte jamais plus.
- Quand la place manque, une nouvelle lecture reçoit une qualité plus basse,
  ou passe d'une carte graphique occupée au processeur. Si aucun des deux n'a
  de place, le lecteur affiche **Le serveur est occupé par d'autres
  transcodages. Réessayez dans un instant.**
- Ce que vous voudrez peut-être faire : donner une carte graphique au
  serveur ([Accélération matérielle](../administration/hardware-acceleration.md)),
  et garder de la place libre sur le disque qui contient le cache
  ([Le cache de transcodage](#the-transcoding-cache)).

Le reste de cette page s'adresse à l'administrateur qui veut les détails :
comment le serveur mesure ce qu'il sait encoder, comment il partage cette
capacité, et ce que voit un spectateur quand elle est épuisée.

## Le serveur se mesure lui-même { #the-server-measures-itself }

La capacité n'est pas un réglage à estimer. Le serveur la mesure :

- **Quand.** Pas au démarrage : le serveur démarre avec ce qu'il savait déjà
  (ou, la première fois, avec une valeur prudente de 720p sur le processeur),
  et lance la mesure dès qu'il est au repos, quand personne ne regarde et que
  le processeur est presque inactif. Elle prend quelques minutes. Une mesure
  prise sous charge est marquée provisoire et refaite quand la machine est
  au repos.
- **Quoi.** De vrais encodages d'un extrait 4K, en SDR et en HDR, sur le
  processeur et sur chaque accélérateur détecté, selon le même schéma qu'une
  vraie lecture. Le résultat est la résolution la plus haute que chacun soutient
  en temps réel, et le coût de chaque type de flux.
- **De nouveau.** Après un changement de matériel, ou avec **Recalibrer**
  dans **Administration → Configuration transcodage → Calibrage de l'encodage**. Le
  bouton refuse tant que quelqu'un regarde ; seul le nouveau calibrage automatique attend que le serveur soit au repos.

Pendant la mesure, une lecture qui a besoin d'un transcodage affiche **Le
serveur se calibre** et demande de réessayer un peu plus tard. La lecture
directe et le remux ne sont pas concernés.

Sur un processeur qui ne soutient aucune des résolutions, le serveur propose
malgré tout du 720p (SDR) et du 480p (HDR) ; le nombre de flux que cela permet en même temps vient du budget du limiteur de sessions vidéo.

La mesure utilise, dans cet ordre, un extrait que vous fournissez, un extrait
de référence téléchargé une fois et vérifié par son empreinte SHA256, ou un
extrait synthétique généré sur place. Un extrait réel mesure plus fidèlement
que l'extrait synthétique, qui surestime le coût et peut fixer pour une
machine un plafond plus bas que nécessaire :

```yaml title="docker-compose.yml"
services:
  evey:
    environment:
      EVEY_BENCH_HDR_SAMPLE: /data/bench/mon-extrait-4k-hdr.hevc
      EVEY_BENCH_SDR_SAMPLE: /data/bench/mon-extrait-4k-sdr.hevc
```

## Partage de la capacité { #sharing-the-capacity }

La **Capacité de lecture en direct** (**Administration → Configuration transcodage**)
est le nombre de flux vidéo qui peuvent être encodés en même temps. Elle
découle de la mesure et s'ajuste seule quand le matériel change. **Limiter les
flux vidéo simultanés** permet de l'abaisser, jamais de la relever.

- Un transcodage qui ne réencode que l'audio (vidéo copiée) n'occupe aucun
  créneau.
- Un spectateur qui change de qualité conserve un seul créneau.
- Seule la qualité effectivement regardée est encodée, pas toute l'échelle
  des qualités.

Chaque flux part vers la route la moins coûteuse qui a prouvé qu'elle peut le
servir : un accélérateur mesuré sur ce dont ce flux a besoin (du tone mapping
HDR ou une sortie HEVC, par exemple), ou le processeur. Le processeur et les
accélérateurs forment deux réserves distinctes, et un flux sur la carte
graphique consomme encore un peu de processeur pour son audio et son
empaquetage.

Quand la place vient à manquer, le serveur répond dans cet ordre :

1. **Une qualité plus basse.** Un nouveau flux dont la qualité la plus haute
   ne peut pas être assurée est admis à la plus haute possible, et ce plafond vaut pour
   toute la durée du flux.
2. **Le processeur.** Quand les accélérateurs sont saturés, un nouveau flux
   passe sur le processeur s'il a de la place.
3. **Un refus.** Quand aucun n'en a, le lecteur affiche **Le serveur est
   occupé par d'autres transcodages** et peut réessayer.

Un encodage sur carte graphique qui échoue sur un titre envoie ce titre sur
le processeur pendant un moment, pour tous les spectateurs. La carte
elle-même n'est écartée qu'après l'échec d'un encodage de test indépendant du
média, et elle est réessayée plus tard : voir
[Accélération matérielle](../administration/hardware-acceleration.md).

Pendant qu'une personne regarde, les tâches de fond du serveur (miniatures,
détection des intros, analyse des médias) se mettent en pause pour laisser la
machine à la lecture : voir
[Tâches de fond et charge du serveur](../operations/background-work.md).

Si une catégorie de sources continue malgré tout de faire attendre les
spectateurs, le serveur abaisse temporairement sa qualité maximale, puis lève
cette limite de lui-même. Voir
[Supervision et logs](../operations/monitoring.md#how-playback-actually-went).

## Ce qui est encodé { #what-is-encoded }

| | Par défaut |
| --- | --- |
| Codec vidéo | H.264. Sortie HEVC quand une route mesurée le prouve et que le lecteur le décode (la route de passthrough HDR, ou quand le **Codec vidéo préféré** du profil le demande) ; AV1 quand il est activé (**Sortie AV1**, matériel uniquement) et que le lecteur le décode. |
| Résolution | Celle de la source, sauf limite : le maximum du lecteur, la limite de débit, ou ce que le serveur peut encoder en temps réel. |
| Débit | Suit la source, proportionnellement à la résolution. **Plafond de débit vidéo** le limite ; **Plafond de débit vidéo, spectateurs distants** le limite plus bas hors des réseaux locaux. |
| Audio | AAC. Voir [Audio](audio.md). |
| Livraison | HLS, en segments MP4 fragmentés. |

<figure class="evey-shot" markdown="span">
  ![Les deux plafonds de débit vidéo dans Administration → Configuration transcodage](../images/admin-transcoding-bitrate.webp){ loading=lazy }
  <figcaption>Les deux plafonds de <strong>Administration → Configuration transcodage</strong> : pour tous les spectateurs, et pour ceux hors des réseaux locaux.</figcaption>
</figure>

## Le cache de transcodage { #the-transcoding-cache }

Les segments encodés sont gardés sur disque dans le volume `evey-cache` et
réutilisés quand un autre spectateur, ou le même plus tard, lit le même titre
avec la même décision. **Administration → Cache de transcodage** affiche sa taille et
permet de le vider ; le **Budget du cache de transcodage** (**Administration →
Transcodage**) le plafonne. **Tout
vider** supprime aussi les sous-titres extraits de vos fichiers, gardés dans
`./evey-data/subtitles` : ils sont extraits de nouveau à la demande.

**Quand le disque se remplit.** Le serveur garde une marge d'espace libre sur
le disque qui contient le cache : 2 Gio par défaut, réglable avec
`EVEY_TRANSCODE_CACHE_MIN_FREE_MB` (`0` désactive la marge). En dessous, les
segments les plus anciens que personne ne regarde sont évincés d'abord. Si cela
ne libère pas assez, un segment qui n'existe pas encore est refusé avec un
délai au bout duquel le lecteur peut redemander, plutôt que de lancer un
encodeur qui échouerait faute de place ; les segments déjà sur le disque
restent servis. Les sous-titres extraits dans `./evey-data` suivent la même
marge : en dessous, les nouveaux ne sont plus conservés, chaque demande les
extrait de nouveau, et les administrateurs reçoivent **Cache des sous-titres
plein** (au plus une fois par heure, envoyé aussi aux webhooks sous
`transcoding.derived_cache_full`).

!!! tip
    Videz le cache après une mise à jour qui modifie la façon dont les vidéos
    sont encodées, pour que personne ne reçoive de segments de la version
    précédente. Le vider pendant qu'une personne regarde fait remettre son
    flux en mémoire tampon une fois, puis la lecture reprend.

## Vérifier ce qui se passe { #checking-what-happens }

- **Administration → En cours de lecture** : chaque flux, son mode de lecture piste par
  piste, et le backend qui l'encode.
- **Administration → Configuration transcodage → Calibrage de l'encodage** : les
  plafonds mesurés et les accélérateurs utilisés.
- **Administration → Qualité streaming** : le déroulement des lectures pour les vrais
  spectateurs.
- **Test de charge** (**Administration → Configuration transcodage**) : simule des
  spectateurs pour mesurer ce que le serveur supporte. Il refuse de démarrer tant
  que quelqu'un regarde.
