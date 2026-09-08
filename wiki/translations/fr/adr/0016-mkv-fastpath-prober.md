---
source_sha: fe1320bda5c1091bee71343b8b19b27aa460b90e2b4f93893438335768ff87f5
---

# ADR-0016 : chemin rapide mkvgo en mémoire pour la sonde de scan MKV (ffprobe reste l'autorité)

- Statut : acceptée
- Date : 2026-06-03

> **Mise à jour du 2026-06-23.** Le chemin rapide est désormais **toujours actif**,
> pour les conteneurs EBML (`.mkv` et `.webm`) comme ISO-BMFF (`.mp4`, `.m4v`,
> `.m4a`), y compris pour la sonde de scan musical (`.m4a` en AAC ; `.flac`,
> `.mp3` et ALAC retombent sur ffprobe) ; le drapeau d'environnement
> `EVEY_MKV_FASTPATH` a été retiré (l'équivalence champ par champ a été validée
> sur une vraie bibliothèque, et ffprobe reste le repli). Les mentions de drapeau
> et de valeur par défaut ci-dessous sont historiques.

## Contexte

Chaque fichier que touche le scanner de bibliothèque est sondé pour connaître sa
structure de flux (codecs, langues, drapeaux par défaut et forcés, dimensions,
canaux, colorimétrie et HDR). Ce sondage se fait en lançant `ffprobe` une fois par
fichier. `ffprobe` est correct et fait référence chez nous, mais sur un conteneur
Matroska il travaille bien plus que le scan ne le demande : il ouvre le fichier,
lit environ 1 Mo, et pour la détection HDR va analyser le train binaire. Lors d'un
nouveau scan, d'une passe de réparation, ou sur un NAS modeste via un montage
réseau, ce lancement et cette lecture par fichier dominent la phase de sondage.

`mkvgo` (`github.com/gravity-zero/mkvgo`) est un lecteur Matroska qui travaille en
mémoire, dans le même processus. Ses fonctions `ReadMeta` et `OpenMeta` (v0.5.0)
ne lisent **que** la tête du fichier, `Info` et `Tracks`, en s'arrêtant tôt grâce
au `SeekHead` avant les `Cues` et les `Clusters`, en gros **2 ms et environ 900
octets par fichier**, contre environ 50 ms et 1,2 Mo pour `ffprobe` (mesuré sur 16
vrais fichiers 4K, voir la note de banc d'essai dans la mémoire du dépôt). La
v0.6.0 a ajouté la dérivation de la colorimétrie et du HDR depuis le SPS VUI du
codec (`hvcC`, `avcC`, `av1C`) lorsque le conteneur n'a pas d'élément `Colour`,
et c'est ce qui a débloqué la couverture HDR en HEVC et AV1 : sans cela, ces
fichiers devaient retomber sur `ffprobe`.

La question était comment adopter cela sans rendre la justesse de `ffprobe`
optionnelle.

## Décision

Introduire `internal/adapters/driven/prober`, un **aiguilleur** implémentant
`library.StreamProber` et enveloppant la sonde `ffprobe` :

- Pour un conteneur EBML (famille `.mkv` et `.webm`), il lit les métadonnées dans
  le processus via `mkvgo` et les convertit en `library.ProbeResult` (`mapper.go`).
- Pour tout le reste, et pour tout `.mkv` qu'il ne sait pas convertir de façon
  **équivalente champ par champ** à `ffprobe`, il **délègue à la sonde `ffprobe`**.

`ffprobe` reste l'autorité. Le chemin rapide est une **optimisation** sans
lancement de processus et limitée à la tête du fichier, jamais une exigence de
justesse. Le convertisseur est conservateur par construction : `mapContainer`
renvoie `ok=false` (un signal de flux de contrôle, pas une erreur), donc un repli
sur `ffprobe`, dès qu'il ne peut pas garantir l'équivalence : pistes nulles ou
absentes, type de piste inconnu, codec exotique ou non converti, dimensions vidéo
manquantes, ou codec capable de HDR (hevc, av1, vp9) sans lecture de
colorimétrie. Les vraies erreurs d'entrée-sortie de `ReadMeta` provoquent aussi un
repli. Un `recover` différé à la frontière absorbe toute panique d'analyse sur un
conteneur hostile et replie également.

Câblage (`cmd/evey/wire.go`) :

- Seul le **chemin de scan de bibliothèque** est enveloppé. Le service de commande
  musical garde la sonde `ffprobe` nue (il lui faut le débit audio par piste, que
  `mkvgo` n'expose pas).
- Le chemin rapide est conditionné par le drapeau d'environnement
  **`EVEY_MKV_FASTPATH`** (par défaut **désactivé**). Désactivé, c'est un simple
  passe-plat vers `ffprobe`, sans le moindre changement de comportement.

Un repliement de macrolangues `chi`, `cmn`, `yue`, etc. vers `zh` a été ajouté au
résolveur de piste audio, pour qu'une préférence audio « zh » corresponde de la
même façon selon qu'un fichier a été scanné par `ffprobe` (`chi` en ISO 639-2) ou
par le chemin mkvgo (`cmn` en BCP-47).

## Conséquences

- **L'équivalence est vérifiée, pas supposée.** `parity_realfile_test.go`,
  conditionné par le chemin `EVEY_PARITY_DIR` et sans aucun chemin de fichier réel
  committé, fait passer chaque `.mkv` d'un dossier par les deux sondes et vérifie
  que les champs qui influent sur la décision concordent. Sur le jeu de référence :
  16 fichiers sur 16 en chemin rapide, tous les champs égaux.
- **Le débit vidéo par flux reste nul** sur le chemin rapide, mais `ffprobe`
  lui-même ne rapporte aucun débit vidéo pour du MKV : c'est donc équivalent, pas
  une régression. (Le débit audio peut différer dans de rares cas limites, raison
  de plus pour que la musique garde `ffprobe`.)
- **Le gain est réel là où la phase de sondage domine** : nouveau scan, réparation,
  montages réseau, NAS modeste (environ 35 fois plus rapide, environ 1000 fois
  moins d'entrées-sorties par MKV). Il est **marginal sur un scan initial**, dont
  le goulet d'étranglement est l'appel aux métadonnées TMDB, pas le sondage.
- **Le retour en arrière tient dans un drapeau.** On pose `EVEY_MKV_FASTPATH=0`
  (ou on ne le pose pas) et on redémarre : le scanner revient au `ffprobe` pur,
  sans recompilation.
- La dérivation HDR vit dans l'utilitaire pur `internal/pkg/hdr` (`DetectHDR`), pas
  dans le convertisseur ; celui-ci ne fait que transporter les métadonnées de
  colorimétrie d'un côté à l'autre de la frontière.

## Alternatives envisagées

- **Remplacer purement et simplement `ffprobe`.** Écartée : `mkvgo` ne couvre que
  Matroska, et `ffprobe` est l'autorité multi-format. L'aiguilleur le conserve
  comme plancher.
- **Câbler les `Read` et `Open` complets de `mkvgo`.** Écartée : une lecture
  complète traverse les `Cues` et les `Clusters` et se révèle environ 360 fois
  *plus lente* que `ffprobe` (mesuré). Seul le `ReadMeta` limité à la tête est un
  gain.
- **Actif par défaut.** Écartée pour la première version : livrer derrière un
  drapeau, valider la parité sur le terrain, basculer la valeur par défaut ensuite.
