---
source_sha: c5dbea370fd3437769623f35f8bb5f32be76bca382bacfd248fac739d7bb12e9
reviewed: true
---
# Accélération matérielle

Une carte graphique ou un GPU intégré permet au serveur de transcoder plus de
flux à la fois, et en plus haute résolution, que le processeur seul. Il n'y a
qu'une image pour tous les cas : aucun pilote propre à une carte n'y est figé,
et le même conteneur fonctionne sur le processeur seul, avec un GPU Intel ou
AMD, ou avec une carte NVIDIA, selon les périphériques que vous lui donnez.

Deux étapes : donner le périphérique au conteneur, puis laisser
**Accélération matérielle** sur **Auto** dans **Admin → Configuration
transcodage → Calibrage de l'encodage**.

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
    fausse ne casse rien : le GPU est simplement inaccessible et le serveur
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

## Ce que fait Auto

En **Auto**, le serveur ne se fonde que sur des mesures, jamais sur le nom de
la carte :

1. **Détection.** Chaque nœud de rendu est testé en VAAPI et, sur Intel, en
   Quick Sync ; chaque GPU NVIDIA et l'encodeur Rockchip de même. Un
   encodeur ne compte que si un encodage réel réussit.
2. **Certification.** Chaque accélérateur encode l'échelle des résolutions
   (de 720p à 2160p) selon le même schéma que la lecture réelle, garde la
   résolution la plus haute qu'il tient à 1,2 fois le temps réel, et passe
   un contrôle visuel. Un accélérateur incapable de tenir le 720p n'est pas
   utilisé.
3. **Mesure des routes.** Pour chaque capacité que l'accélérateur a prouvée
   (SDR, HEVC, AV1, tone mapping sur le GPU, HDR conservé), le coût d'une
   vraie session est mesuré.
4. **Choix par flux.** Chaque lecture part vers l'accélérateur le moins
   coûteux qui a prouvé ce dont ce flux a besoin, ou vers le processeur. Voir
   [Transcodage et capacité du serveur](../playback/transcoding.md).

La mesure s'exécute dès que le serveur est au repos, puis de nouveau après un
changement de matériel. **Recalibrer** la lance immédiatement : à utiliser
après l'ajout d'une carte ou le déplacement du conteneur sur une autre
machine.

**Désactivée (processeur uniquement)** garde tous les transcodages sur le
processeur : c'est la porte de sortie en cas de pilote défaillant. Choisir un
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

**Admin → Configuration transcodage → Calibrage de l'encodage** affiche les
accélérateurs détectés, le backend utilisé, les résolutions certifiées pour
le GPU et pour le processeur, et les éventuels replis sur le processeur.
Pendant une lecture, **Admin → En cours de lecture** indique le backend qui
encode chaque flux.

Sous WSL2, NVENC fonctionne ; VAAPI et Quick Sync non (le pilote graphique de
WSL échoue sur ces encodages), et le panneau d'administration les grise.
