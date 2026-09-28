---
source_sha: ffa2d81166e7b4ed70cb689b95ea6c53767a79b4a69552e5f0334112ce647605
---

# Transcodage

Evey transcode les médias à la volée quand le client ne prend pas en charge le format source.

**En bref.** La plupart des installations n'ont rien à faire ici : le serveur
mesure ce que la machine sait convertir et admet autant de conversions
simultanées. Pour utiliser une carte graphique, exposez-la au conteneur
(ci-dessous), puis laissez **Accélération matérielle** sur **Auto** dans
**Admin → Transcodage**. Le reste de la page explique ce qui se passe derrière
ce choix.

## Comment cela fonctionne

Evey retient tout seul la meilleure méthode de lecture :
1. **Lecture directe** - le format est pris en charge, aucun traitement
2. **Remultiplexage** - le conteneur est ré-emballé (MKV → MP4), sans réencodage
3. **Transcodage** - réencodage en HLS, à débit adaptatif

## Configuration

Les deux valeurs ci-dessous n'amorcent que le premier démarrage : le panneau
d'administration (**Admin → Transcodage**) l'emporte dès qu'une valeur y est
enregistrée.

```env
EVEY_TRANSCODING_MAX_CONCURRENT=2
# auto | off | nvenc | vaapi | qsv | rkmpp - voir « Accélération matérielle » plus bas
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

```yaml title="docker-compose.yml"
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

```yaml title="docker-compose.yml"
services:
  evey:
    devices:
      - /dev/dri:/dev/dri
```

Vous pouvez exposer les deux à la fois (la réservation NVIDIA et `/dev/dri`) :
l'image unique servira le moteur que vous aurez ensuite sélectionné.

### L'activer

L'accélération matérielle se pilote depuis **Admin → Transcodage → Calibrage de
l'encodage** :

- Un interrupteur général **Accélération matérielle**. Coupé = logiciel seul (la
  soupape de secours quand le GPU ou son pilote est capricieux), activé = le GPU
  est employé.
- Une fois activé, un sélecteur de **moteur** : `Auto` (la calibration retient le
  meilleur moteur détecté), ou bien `NVENC`, `VAAPI`, `QSV` ou `RKMPP` imposés.
  Si un moteur imposé se révèle inutilisable, le serveur se replie sur le
  logiciel, et la carte affiche le moteur réellement actif.

**Ce que fait `Auto`.** Vous exposez le matériel au conteneur (`/dev/dri`, la
réservation NVIDIA, le nœud MPP de Rockchip) ; Evey fait le reste, uniquement à
partir de mesures - jamais d'après le nom d'un processeur ou d'un GPU :

1. **Détecter** : chaque nœud de rendu DRM est sondé pour VAAPI et, sur un nœud
   Intel, pour QSV ; chaque GPU NVIDIA par son index CUDA ; le nœud MPP de
   Rockchip. Chaque encodeur doit réussir un vrai encodage avant de compter.
2. **Certifier** : chaque accélérateur détecté parcourt l'échelle d'encodage
   (720p → 2160p) dans la forme de pipeline qu'emploie l'exécution, garde le
   palier le plus haut qu'il tient à 1,2× le temps réel, puis passe un contrôle
   de justesse visuelle. Sous le temps réel à 720p = rejeté, logiciel seul.
   Chaque moteur certifié est conservé avec **son propre** plafond, sa matrice de
   décodage, ses fonctions et ses routes éprouvées - un accélérateur limité à
   1080p à côté d'un autre qui monte à 2160p sert des échelles 1080p, jamais
   celles de l'autre.
3. **Mesurer les routes** : pour chaque route qu'un moteur a éprouvée (SDR,
   HEVC, AV1, tone mapping GPU, pont CPU, HDR conservé, décodage découplé) et
   pour les routes logicielles, une session de production à chaud est mesurée
   aux paliers de référence et au plafond de la route - si bien qu'un moteur
   certifié seulement en 720p, ou le plancher logiciel de 720p, a lui aussi son
   coût.
4. **Choisir selon la charge** : le résolveur retient, à chaque requête, le
   moteur certifié le moins coûteux qui a éprouvé la route et prend en charge le
   codec (le HDR→SDR peut atterrir sur le tone mapping OpenCL de VAAPI pendant que
   le HEVC prend un QSV plus rapide sur la même machine). L'admission chiffre la
   session d'après la mesure de cette route, face à une réserve CPU et une
   réserve accélérateur ; une réserve pleine déverse un flux GPU vers le CPU, ou
   refuse.

L'échelle logicielle a un plancher de **720p SDR / 480p HDR** : un processeur
qui ne franchit aucun palier avec la marge voulue propose tout de même ces
paliers, en n'admettant qu'une session à la fois. La lecture directe et la
copie/remux ne prennent jamais de place d'encodage et ne sont en rien concernées.

Le choix est persisté et la mesure est refaite à chaud, sans redémarrage. Il peut
aussi être amorcé depuis l'environnement, pour un déploiement sans interface :

```env
# Soit l'ancien drapeau on/off…
EVEY_TRANSCODING_HW_ACCEL_ENABLED=true
# …soit le mode unifié (auto|off|nvenc|vaapi|qsv|rkmpp) ; le réglage du panneau d'administration l'emporte.
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
# ACTIVÉ par défaut (=1). Le mode multi-variantes partage un seul décodage entre
# les paliers, mais encode aussi chaque palier que le spectateur ne regarde
# jamais - mesuré en production, trois encodages inutiles affamaient le
# quatrième. Le calibrage mesure la MÊME forme que celle de l'exécution (un
# palier par sonde en variante unique, l'échelle parallèle cumulée en
# multi-variantes), pour les échelles logicielles ET matérielles, de sorte que
# les plafonds correspondent à ce que coûte réellement une session. Ne mettez =0
# que sur une machine sans décodage matériel ET avec des sources lourdes, pour
# retrouver le décodage partagé. Compatible avec le cache dans les deux cas.
EVEY_TRANSCODING_SINGLE_VARIANT=1
```

> **Cinq réglages vivent désormais dans le panneau d'administration** (Admin →
> Transcodage → Limites avancées) : `HW_PER_VARIANT`,
> `SINGLE_VARIANT`, `MAX_SESSIONS`, `SOFTWARE_MAX_HEIGHT` et
> `SOFTWARE_HDR_MAX_HEIGHT`. Une valeur enregistrée là est persistée et l'emporte
> sur l'environnement : vous n'avez donc plus à éditer un fichier compose ni à
> recréer le conteneur pour changer un plafond de lecture. Tous les cinq sont lus
> au démarrage, et le panneau le dit : enregistrer mémorise votre choix,
> redémarrer l'applique.

### L'échantillon de calibration de l'encodage

Le banc d'essai mesure le plafond d'encodage en temps réel de la machine face à
un extrait 4K, séparément pour le SDR et pour le HDR (`software_max_height` et
`software_hdr_max_height`). **Le démarrage ne le lance jamais** : le serveur
démarre en quelques millisecondes avec ce qu'il a (le calibrage enregistré, ou
un repli logiciel en 720p au tout premier démarrage) et lance le banc au premier
*moment calme* - cinq sondes consécutives, espacées de 4 s, sans aucune lecture
et avec un processeur sous 20 %. Cela tient un banc de 2 à 5 minutes à l'écart
du démarrage et d'une machine occupée par une migration ou un scan de
bibliothèque (un banc mesuré sous charge est marqué *provisoire* et refait au
calme). Pendant que le banc tourne, une nouvelle lecture est priée d'attendre
environ 2 minutes (`calibrating`) ; **Admin → Transcodage → Calibrage de
l'encodage** indique si le calibrage est en attente, en cours ou terminé, et
« Recalibrer » (`POST /api/v1/admin/transcode/calibration/recalibrate`) le lance
tout de suite. C'est du coût de décodage, de tone mapping et
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

### HDR conservé et HDR10+

Sur un GPU sondé pour l'encodage Main10 (`hw_hdr10_encode`), une source HDR lue
par un client compatible HDR qui décode le HEVC est réencodée en HEVC Main10 en
gardant le HDR : la signalisation HDR10 statique (bt2020 / PQ ou HLG, SEI de
l'écran de mastering) est transmise. Les métadonnées dynamiques HDR10+ ne sont
**pas** réémises : un titre HDR10+ est lu comme du HDR10 simple sur ce chemin.
Le Dolby Vision profil 7 se replie de la même façon sur sa couche de base
HDR10 ; le profil 5 est converti en SDR par tone mapping.

La fréquence d'échantillonnage du son n'est jamais une raison de réencoder : une
piste à 96 kHz que la liste de codecs du client accepte est copiée telle quelle.
Un client dont le décodeur s'arrête à 48 kHz doit le dire dans sa déclaration de
codecs ; le serveur ne le suppose pas.

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

Ouvrez **Admin → Transcodage → Calibrage de l'encodage** : la page affiche
les accélérateurs détectés, le moteur actif, les plafonds mesurés en logiciel et
en matériel, ainsi que tout repli automatique du matériel vers le logiciel. Un
bouton « Recalibrer » relance le banc d'essai après un changement de matériel ou
de pilote.
