---
source_sha: c8d03e544227da6967bbbab98675cdb62fcd32e683e410544a6fffc79d40c1d5
reviewed: true
---
# Transcodage et capacité du serveur

Le transcodage est l'issue coûteuse de
[la décision de lecture](how-playback-is-decided.md) : la lecture directe et
le remux ne coûtent presque rien au serveur et n'occupent jamais de créneau
de transcodage. Cette page explique comment le serveur sait ce qu'il peut
encoder, comment il répartit cette capacité entre les spectateurs, et ce qui
se passe quand elle est épuisée.

## Le serveur se mesure lui-même

La capacité n'est pas un réglage à estimer. Le serveur la mesure :

- **Quand.** Pas au démarrage : le serveur démarre avec ce qu'il savait déjà
  (ou, la première fois, avec une valeur prudente de 720p sur le processeur),
  et lance la mesure dès qu'il est au repos, quand personne ne regarde et que
  le processeur est presque inactif. Elle prend quelques minutes. Une mesure
  prise sous charge est marquée provisoire et refaite quand la machine est
  au repos.
- **Quoi.** De vrais encodages d'un extrait 4K, en SDR et en HDR, sur le
  processeur et sur chaque accélérateur détecté, selon le même schéma qu'une
  vraie lecture. Le résultat est la résolution la plus haute que chacun tient
  en temps réel, et le coût de chaque type de flux.
- **De nouveau.** Après un changement de matériel, ou avec **Recalibrer**
  dans **Admin → Configuration transcodage → Calibrage de l'encodage**.

Pendant la mesure, une lecture qui a besoin d'un transcodage affiche **Le
serveur se calibre** et demande de réessayer un peu plus tard. La lecture
directe n'est pas concernée.

Sur un processeur qui ne tient aucune des résolutions, le serveur propose
malgré tout du 720p (SDR) et du 480p (HDR), un flux à la fois.

La mesure utilise, dans cet ordre, un extrait que vous fournissez, un extrait
de référence téléchargé une fois et vérifié par son empreinte SHA256, ou un
extrait synthétique généré sur place. Un extrait réel mesure plus fidèlement
que l'extrait synthétique, qui surestime le coût et peut plafonner une
machine plus bas qu'elle ne le mérite :

```yaml title="docker-compose.yml"
services:
  evey:
    environment:
      EVEY_BENCH_HDR_SAMPLE: /data/bench/mon-extrait-4k-hdr.hevc
      EVEY_BENCH_SDR_SAMPLE: /data/bench/mon-extrait-4k-sdr.hevc
```

## Partage de la capacité

La **Capacité de lecture en direct** (**Admin → Configuration transcodage**)
est le nombre de flux vidéo qui peuvent être encodés en même temps. Elle
découle de la mesure et s'ajuste seule quand le matériel change. **Limiter les
flux vidéo simultanés** permet de l'abaisser, jamais de la relever.

- Un transcodage qui ne réencode que l'audio (vidéo copiée) n'occupe aucun
  créneau.
- Un spectateur qui change de qualité conserve un seul créneau.
- Seule la qualité effectivement regardée est encodée, pas toute l'échelle
  des qualités.

Chaque flux part vers la route la moins coûteuse capable de le servir : un
accélérateur qui a prouvé ce dont ce flux a besoin (du tone mapping HDR ou
une sortie HEVC, par exemple), ou le processeur. Le processeur et les
accélérateurs forment deux réserves distinctes. Quand les accélérateurs sont
saturés, un nouveau flux peut passer sur le processeur s'il a de la place ;
quand aucun n'en a, le lecteur affiche **Le serveur est occupé par d'autres
transcodages** et peut réessayer.

Pendant qu'une personne regarde, les tâches de fond du serveur (miniatures,
détection des intros, analyse des médias) se mettent en pause pour laisser la
machine à la lecture : voir
[Tâches de fond et charge du serveur](../operations/background-work.md).

Si une catégorie de sources continue malgré tout de faire attendre les
spectateurs, le serveur abaisse temporairement sa qualité maximale, puis lève
cette limite de lui-même. Voir
[Supervision et logs](../operations/monitoring.md#how-playback-actually-went).

## Ce qui est encodé

| | Par défaut |
| --- | --- |
| Codec vidéo | H.264. HEVC quand le lecteur le décode et que la carte graphique l'encode ; AV1 quand il est activé (**Sortie AV1**, matériel uniquement) et que le lecteur le décode. |
| Résolution | Celle de la source, sauf limite : le maximum du lecteur, la limite de débit, ou ce que le serveur peut encoder en temps réel. |
| Débit | Suit la source, proportionnellement à la résolution. **Plafond de débit vidéo** le limite. |
| Audio | AAC. Voir [Audio](audio.md). |
| Livraison | HLS, en segments MP4 fragmentés. |

<figure class="evey-shot" markdown="span">
  ![Les deux plafonds de débit vidéo dans Admin → Transcodage](../images/admin-transcoding-bitrate.webp){ loading=lazy }
  <figcaption>Les deux plafonds de <strong>Admin → Transcodage</strong> : pour tous les spectateurs, et pour ceux hors des réseaux locaux.</figcaption>
</figure>

## Le cache de transcodage

Les segments encodés sont gardés sur disque dans le volume `evey-cache` et
réutilisés quand un autre spectateur, ou le même plus tard, lit le même titre
avec la même décision. **Admin → Cache de transcodage** affiche sa taille et
permet de le vider ; le **Budget du cache de transcodage** (**Admin →
Configuration du serveur → Paramètres du serveur**) le plafonne. Videz-le après une mise à jour qui modifie la façon dont
les vidéos sont encodées, pour que personne ne reçoive de segments de la
version précédente.

## Vérifier ce qui se passe

- **Admin → En cours de lecture** : chaque flux, son mode de lecture piste par piste,
  et le backend qui l'encode.
- **Admin → Configuration transcodage → Calibrage de l'encodage** : les
  plafonds mesurés et les accélérateurs utilisés.
- **Admin → Qualité streaming** : le déroulement des lectures pour les vrais
  spectateurs.
- **Test de charge** (**Admin → Configuration transcodage**) : simule des
  spectateurs pour mesurer ce que le serveur tient. À lancer uniquement quand
  personne ne regarde.
