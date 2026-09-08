---
source_sha: aa2eb3901d0f475fc80f6f02196efd56ffd3beb911d1b5b63e4d270fd3d28c20
---

# Transcodage

Evey transcode les médias à la volée quand le client ne prend pas en charge le format source.

## Comment cela fonctionne

Evey retient tout seul la meilleure méthode de lecture :
1. **Lecture directe** - le format est pris en charge, aucun traitement
2. **Remultiplexage** - le conteneur est ré-emballé (MKV → MP4), sans réencodage
3. **Transcodage** - réencodage en HLS, à débit adaptatif

## Configuration

```env
EVEY_TRANSCODING_MAX_CONCURRENT=2
# auto | off | nvenc | vaapi | qsv - voir « Accélération matérielle » plus bas
EVEY_TRANSCODING_HW_MODE=off
```

## Accélération matérielle

Evey livre **une seule image**, dont le ffmpeg embarque à la fois NVENC, VAAPI et
QSV, tous compilés. Aucun pilote de GPU n'est cuit dans l'image : vous injectez
le pilote à l'exécution en exposant le périphérique, sans le moindre mode
privilégié. La même image reste purement logicielle, ou devient NVENC, ou
VAAPI/QSV, selon ce que vous exposez et rien d'autre. La justification se trouve
dans [ADR-0015](adr/0015-hardware-acceleration.md).

### NVIDIA (NVENC)

Installez le [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html), puis réservez le GPU **et** réclamez la capacité de pilote `video` (sans elle, le toolkit injecte `nvidia-smi` et CUDA, mais pas `libnvidia-encode.so`, et NVENC ne se charge pas) :

```yaml
services:
  evey:
    environment:
      NVIDIA_DRIVER_CAPABILITIES: all   # doit inclure `video` pour NVENC
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: all
              capabilities: [gpu]
```

### Intel / AMD (VAAPI, QSV)

Exposez le nœud de rendu DRM ; libva retient tout seul le bon pilote (iHD, i965
ou radeonsi) d'après l'identifiant PCI du GPU :

```yaml
services:
  evey:
    devices:
      - /dev/dri:/dev/dri
```

Vous pouvez exposer les deux à la fois (la réservation NVIDIA et `/dev/dri`) :
l'image unique servira le moteur que vous aurez ensuite sélectionné.

### L'activer

L'accélération matérielle se pilote depuis **Admin → Server configuration →
Encode calibration** :

- Un interrupteur général **Hardware acceleration**. Coupé = logiciel seul (la
  soupape de secours quand le GPU ou son pilote est capricieux), activé = le GPU
  est employé.
- Une fois activé, un sélecteur de **moteur** : `Auto` (la calibration retient le
  meilleur moteur détecté), ou bien `NVENC`, `VAAPI` ou `QSV` imposés. Si un
  moteur imposé se révèle inutilisable, le serveur se replie sur le logiciel, et
  la carte affiche le moteur réellement actif.

Le choix est persisté et la mesure est refaite à chaud, sans redémarrage. Il peut
aussi être amorcé depuis l'environnement, pour un déploiement sans interface :

```env
# Soit l'ancien drapeau on/off…
EVEY_TRANSCODING_HW_ACCEL_ENABLED=true
# …soit le mode unifié (auto|off|nvenc|vaapi|qsv) ; le réglage du panneau d'administration l'emporte.
EVEY_TRANSCODING_HW_MODE=auto
# Basculer sans attendre vers le logiciel quand le GPU se bloque, passé ce délai (5s par défaut).
EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT=5s
# Lancer un ffmpeg par palier ABR sur le chemin matériel (un encodeur par palier)
# au lieu d'un seul processus à sorties multiples : cela isole les défaillances
# palier par palier et évite le graphe multi-encodage qui fait tomber les pilotes
# VA fragiles. Coupé par défaut ; le logiciel emprunte toujours le graphe à
# processus unique. Voir docs/adr/0015.
EVEY_TRANSCODING_HW_PER_VARIANT=false
# N'encoder que le seul palier ABR que le client réclame réellement (à la demande,
# un encodeur par palier), au lieu d'un ffmpeg qui produit toute l'échelle.
# COUPÉ par défaut (=0, l'échelle multi-variantes) - et c'est BIEN le bon réglage
# pour un NAS modeste : le mode multi-variantes partage UN seul décodage de la
# source entre tous les paliers. Le mode variante unique (=1) redécode la source
# POUR CHAQUE palier : quand le lecteur sonde l'ABR ou navigue d'un palier à
# l'autre, il lance plusieurs décodages simultanés (de 4K, par exemple) qui
# s'affament mutuellement - le classique 360p qui met en tampon sur une machine
# limitée par le décodage. Ne mettez =1 que sur une machine assez puissante pour
# s'offrir un décodage par palier. Compatible avec le cache dans les deux cas.
EVEY_TRANSCODING_SINGLE_VARIANT=0
```

> **Cinq réglages vivent désormais dans le panneau d'administration** (Admin >
> Transcoding configuration > Advanced limits) : `HW_PER_VARIANT`,
> `SINGLE_VARIANT`, `MAX_SESSIONS`, `SOFTWARE_MAX_HEIGHT` et
> `SOFTWARE_HDR_MAX_HEIGHT`. Une valeur enregistrée là est persistée et l'emporte
> sur l'environnement : vous n'avez donc plus à éditer un fichier compose ni à
> recréer le conteneur pour changer un plafond de lecture. Tous les cinq sont lus
> au démarrage, et le panneau le dit : enregistrer mémorise votre choix,
> redémarrer l'applique.

### L'échantillon de calibration de l'encodage

Au démarrage (et sur
`POST /api/v1/admin/transcode/calibration/recalibrate`), le banc d'essai mesure
le plafond d'encodage en temps réel de la machine face à un extrait 4K,
séparément pour le SDR et pour le HDR (`software_max_height` et
`software_hdr_max_height`). C'est du coût de décodage, de tone mapping et
d'encodage de cet extrait que le plafond se déduit : **un extrait de contenu réel
calibre bien plus honnêtement qu'un extrait synthétique**, lequel surestime le
coût et cloue le plafond très bas (symptôme classique : un titre 4K HDR que la
machine sait en réalité lire en 1080p se retrouve plafonné à 480p ou 360p).

Le banc d'essai résout son entrée dans cet ordre de priorité :

1. **Un fichier fourni par l'exploitant** - un vrai extrait que vous déposez. Employé tel quel.
2. **Un téléchargement épinglé** - un extrait de référence à la licence propre, récupéré une seule fois (vérifié par SHA256) puis mis en cache.
3. **Un extrait synthétique** - généré à la volée (`testsrc2` plus un léger grain, à environ 5 Mbit/s). Toujours disponible : c'est le repli des machines coupées du réseau.

```bash
# Faire viser au banc d'essai un vrai fichier (le vôtre, ou un extrait de référence téléchargé) :
EVEY_BENCH_HDR_SAMPLE=/data/bench/my-4k-hdr.hevc
EVEY_BENCH_SDR_SAMPLE=/data/bench/my-4k-sdr.hevc
# …ou un téléchargement épinglé (l'URL doit être en https ; le SHA256 est obligatoire dès qu'une URL est renseignée) :
EVEY_BENCH_HDR_SAMPLE_URL=https://example.org/evey-bench-hdr10.hevc
EVEY_BENCH_HDR_SAMPLE_SHA256=<sha256>
EVEY_BENCH_SDR_SAMPLE_URL=https://example.org/evey-bench-sdr.hevc
EVEY_BENCH_SDR_SAMPLE_SHA256=<sha256>
```

Fabriquez ces extraits de référence à partir d'une source à licence permissive
(les Blender Open Movies, en CC-BY - **pas** un contenu en CC-BY-NC-ND comme le
« Sol Levante » de Netflix) avec `scripts/make-bench-sample.sh <source-4k>` ; le
script affiche le SHA256 à épingler.

### Le cas particulier de WSL2

Sous WSL2, le GPU est atteint par le pilote **d3d12** expérimental de Mesa.
**NVENC fonctionne** (le CUDA passe réellement) ; **VAAPI et QSV, non** : le
pilote VA d3d12 s'effondre sur l'encodage multi-variantes d'Evey. Evey détecte
WSL et grise VAAPI et QSV dans le panneau d'administration. Validez VAAPI et QSV
sur un hôte Linux natif.

## Le son AC-3 / E-AC-3 dans le navigateur (décodeur WebAssembly)

La plupart des navigateurs ne savent pas décoder l'AC-3 ni l'E-AC-3 nativement
(affaire de licences), ce qui force classiquement un réencodage du son sur le
serveur. Evey préfère livrer un décodeur WebAssembly au navigateur : la playlist
maîtresse ne porte que la vidéo, la piste AC-3 est servie comme déclinaison hors
bande (copiée, pas réencodée), et le client la décode en PCM, calé sur l'horloge
de la vidéo. Zéro cycle processeur dépensé sur le serveur pour le son dès lors
que la source s'emballe telle quelle.

- Les navigateurs qui SAVENT décoder l'AC-3 nativement (la famille Safari) sont
  écartés par détection de capacités, et non par leur nom : ils reçoivent le
  passthrough ordinaire.
- **Une exigence côté reverse proxy** : la CSP qui parvient au navigateur doit
  comporter `'wasm-unsafe-eval'` dans `script-src`, faute de quoi le décodeur ne
  peut pas se compiler et le film se joue muet. Voir
  [reverse-proxy.md](reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own).

## Vérifier

Ouvrez **Admin → Server configuration → Encode calibration** : la page affiche
les accélérateurs détectés, le moteur actif, les plafonds mesurés en logiciel et
en matériel, ainsi que tout repli automatique du matériel vers le logiciel. Un
bouton « Recalibrate » relance le banc d'essai après un changement de matériel ou
de pilote.
