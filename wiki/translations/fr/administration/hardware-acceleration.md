---
source_sha: 1a4e824d130bb84b068353b3024d71ea59b8db2ff04ee70f219e637e02212468
reviewed: true
---
# Accélération matérielle

**En bref :**

- Une carte graphique, ou la partie graphique intégrée à beaucoup de
  processeurs (Intel Quick Sync, par exemple), permet au serveur de convertir
  plus de vidéos à la fois, et en plus haute résolution, que le processeur
  seul.
- Elle ne sert que quand une vidéo est convertie (transcodée). Les films
  envoyés tels quels, ou remis dans un autre format de fichier, ne
  l'utilisent pas.
- Ce que vous faites : ajoutez quelques lignes à votre fichier compose pour
  qu'Evey puisse utiliser la carte, puis laissez le réglage sur **Auto
  (meilleur disponible)**.
- Evey teste la carte avec une vraie conversion avant de lui envoyer la
  moindre lecture. Une carte qui échoue au test n'est pas utilisée.

Les étapes suivent, puis la façon dont le serveur prouve ce que la carte sait
faire.

## L'activer

1. Ouvrez **Administration → Configuration transcodage → Calibrage de l'encodage**.
   **Accélération matérielle** est sur **Auto (meilleur disponible)** par
   défaut, et la ligne **Détecté :** nomme les accélérateurs que la machine
   annonce.
2. Si elle affiche **Aucun accélérateur GPU détecté**, donnez le périphérique
   au conteneur (section suivante) et recréez-le.
3. Cliquez sur **Recalibrer**, ou patientez : le serveur se mesure à son
   premier moment de calme. La carte affiche alors le backend utilisé et les
   résolutions certifiées pour le GPU et pour le processeur.

Il n'y a qu'une image pour tous les cas : aucun pilote propre à une carte n'y
est intégré, et le même conteneur fonctionne sur le processeur seul, avec un GPU
Intel ou AMD, ou avec une carte NVIDIA, selon les périphériques que vous lui
donnez.

## Donner le périphérique au conteneur

=== "Intel / AMD (VAAPI, Quick Sync)"

    ```yaml title="docker-compose.yml"
    services:
      evey:
        devices:
          - /dev/dri:/dev/dri
        group_add:
          - "104"   # le groupe render de la machine : getent group render
    ```

    Le groupe doit être le groupe `render` de la machine hôte
    (`getent group render`, ou `stat -c %g /dev/dri/renderD128`). Une valeur
    fausse n'endommage rien : le GPU est simplement inaccessible et le serveur
    reste sur le processeur.

=== "NVIDIA (NVENC)"

    Installez le NVIDIA Container Toolkit sur la machine hôte, puis réservez le
    GPU et demandez la capacité `video`, sans laquelle la bibliothèque
    d'encodage n'est pas injectée :

    ```yaml title="docker-compose.yml"
    services:
      evey:
        environment:
          NVIDIA_DRIVER_CAPABILITIES: all   # doit inclure "video"
        deploy:
          resources:
            reservations:
              devices:
                - driver: nvidia
                  count: all
                  capabilities: [gpu]
    ```

=== "Rockchip (RKMPP)"

    Montez `/dev/mpp_service`, `/dev/dri` et, si le noyau l'expose,
    `/dev/rga`. Voir [Machines ARM](../operations/advanced-deployments.md#arm-hosts).

Une réservation NVIDIA et `/dev/dri` peuvent être montés en même temps.

## Ce que veut dire « prouvé par un encodage réel »

En **Auto**, le serveur ne se fonde que sur des mesures, jamais sur le nom de
la carte :

1. **Détection.** Chaque nœud de rendu est testé en VAAPI et, sur Intel, en
   Quick Sync ; chaque GPU NVIDIA et l'encodeur Rockchip de même. Un encodeur
   ne compte que si un court encodage réel réussit en passant par lui.
2. **Certification.** Chaque accélérateur encode l'échelle des résolutions
   (de 720p à 2160p) selon le même schéma que la lecture réelle et garde la
   résolution la plus haute qu'il soutient à 1,2 fois le temps réel. Quelques
   images de chaque essai sont redécodées et comparées à la source, parce
   qu'un pilote défaillant peut se terminer proprement tout en produisant des
   images vertes ou brouillées. Un accélérateur incapable de soutenir le 720p
   n'est pas utilisé.
3. **Mesure des routes.** Pour chaque capacité que l'accélérateur a prouvée
   (SDR, HEVC, AV1, tone mapping sur le GPU, HDR conservé), le coût d'une
   vraie session est mesuré.
4. **Choix par flux.** Chaque lecture part vers l'accélérateur le moins
   coûteux qui a prouvé ce dont ce flux a besoin, ou vers le processeur. Voir
   [Transcodage et capacité du serveur](../playback/transcoding.md).

La mesure attend un moment de calme : aucun flux actif et le processeur de la
machine sous 20 % pendant cinq relevés d'affilée, espacés de quatre secondes,
pour qu'un démarrage chargé ou un scan de bibliothèque ne fixe pas les
plafonds trop bas. Elle se relance quand le matériel change. **Recalibrer** la
lance immédiatement : à utiliser après l'ajout d'une carte ou le déplacement
du conteneur sur une autre machine.

!!! note "Un échec au démarrage n'est pas définitif"
    Un GPU qui échoue à s'initialiser dans les premières secondes après le
    démarrage du conteneur est retesté à intervalles courts avant que quoi
    que ce soit ne soit enregistré. Seul un échec qui persiste, ou d'un type
    déterministe (encodeur absent, format non pris en charge), marque
    l'accélérateur comme inutilisable.

## Imposer un backend ou tout désactiver

**Désactivée (processeur uniquement)** garde tous les transcodages sur le
processeur : c'est la solution de repli en cas de pilote défaillant. Choisir un
**Backend** à la main (NVENC, VAAPI, QSV, RKMPP) l'impose ; s'il s'avère
inutilisable, le serveur se replie sur le processeur et la carte affiche le
backend réellement utilisé.

Sur une machine dotée de plusieurs accélérateurs, Auto n'a pas de préférence
figée : chacun est certifié et mesuré, et chaque flux part vers le backend qui
a prouvé la route dont il a besoin, avec la meilleure qualité prouvée et le
coût mesuré le plus bas. Deux flux d'un même serveur peuvent donc utiliser des
backends différents, par exemple le tone mapping HDR sur une carte et un
encodage HEVC sur une autre.

## Vérifier qu'elle est utilisée

**Administration → Configuration transcodage → Calibrage de l'encodage** affiche les
accélérateurs détectés, le backend utilisé, les résolutions certifiées pour
le GPU et pour le processeur, et sous **Replis matériels** les encodages
récents qui se sont repliés sur le processeur, avec leur cause. Pendant une
lecture, **Administration → En cours de lecture** indique le backend qui encode chaque
flux.

Sous WSL2, NVENC fonctionne ; VAAPI et Quick Sync non (le pilote graphique de
WSL échoue sur ces encodages), et le panneau les marque **non supporté en
WSL**.

<!-- sources
auto, certify, routes, recalibrate: docs/adr/0015-hardware-acceleration.md ; docs/adr/0029-transcoding-routes-are-measured-not-assumed.md
detection by real encode: internal/adapters/driven/ffmpeg/hwdetect.go - probeHWEncode
ladder, 1.2x realtime margin, 720p floor: internal/adapters/driven/ffmpeg/benchmark.go - hwBenchLadder, Margin
visual check: internal/adapters/driven/ffmpeg/hw_output_validation.go
cold-boot init retry: internal/adapters/driven/ffmpeg/hw_readiness.go - runHWReadinessProbe, hwReadinessRetryDelays
quiet moment: internal/app/transcoding/calibration_service.go - idleProbes, idleProbeEvery, idleCPUPercent
render group: docker-compose.gpu.yml ; nvidia: docker-compose.gpu-nvidia.yml
panel: frontend/apps/web/src/features/admin/EncodeCalibrationCard.tsx (hw_mode, detected_backends, is_wsl)
labels: i18n admin.calibration.cardTitle, hwModeLabel, hwModeAuto, hwModeOff, hwBackendLabel, detected, detectedNone, recalibrate, hwFallbacks, wslUnsupported ; admin.nav.transcodingConfig, nowPlaying
plain opening: rules below ; i18n admin.calibration.hwModeAuto
-->
