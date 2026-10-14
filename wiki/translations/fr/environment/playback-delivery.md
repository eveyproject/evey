---
source_sha: ebb072bbebfc9bd7bf678229831e531e8fb4937c38720f305ee4e13f6535e311
reviewed: true
---
# Diffusion de la lecture et téléchargements { #playback-delivery-and-downloads }

Les options qui règlent la façon dont les flux sont empaquetés et ce que le
serveur prépare pour les téléchargements. Les valeurs par défaut
correspondent au comportement recommandé : ces options servent à couper une
voie pendant une investigation, ou à activer une voie désactivée par défaut.
Comment un flux est choisi au départ :
[Comment Evey choisit le mode de lecture](../playback/how-playback-is-decided.md).

Une variable qui a une clé de panneau peut aussi se définir par une ligne
portant cette clé dans **Administration → Configuration du serveur → Entrées de
configuration** ; la ligne l'emporte sur l'environnement. Le panneau propose
les clés des téléchargements ; les autres se saisissent telles qu'elles
figurent ici. Toutes sont lues au démarrage : un changement s'applique au
redémarrage suivant, quelle que soit la façon de le faire.

## Empaquetage { #packaging }

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_HLS_ONDEMAND` | activé | Sert un fichier dont les pistes peuvent être copiées en le réempaquetant à la volée, sans ffmpeg et sans rien écrire dans le cache. `off` : ces fichiers passent par la voie de copie ffmpeg. | Entrées de configuration `transcoding.hls_ondemand`, redémarrage |
| `EVEY_REMUX_COPY` | activé | La voie de copie ffmpeg : la vidéo est copiée sans réencodage quand le fichier le permet. `off` : ces fichiers sont transcodés. | Entrées de configuration `transcoding.remux_copy`, redémarrage |
| `EVEY_HLS_AUDIO_RENDITION_ENABLED` | `true` | Propose les pistes audio comme des variantes séparées, pour qu'un lecteur change de langue sans recharger la vidéo. Permet aussi de lire une vidéo copiée avec une piste audio réencodée à part. | aucun |
| `EVEY_HLS_COPY_FALLBACK_RUNG` | `true` | Ajoute une qualité basse transcodée à côté d'un flux copié, pour qu'un lecteur dont la connexion chute brutalement puisse passer à cette qualité au lieu de se figer. Ne coûte rien tant qu'aucun lecteur ne l'utilise. | aucun |
| `EVEY_HLS_ONDEMAND_CONCAT` | activé | La lecture enchaînée sans coupure entre épisodes, quand le lecteur web la demande. Un épisode qui ne peut pas être copié revient au flux normal. | Entrées de configuration `transcoding.hls_ondemand_concat`, redémarrage |
| `EVEY_HLS_ONDEMAND_TRACK_TRIM` | désactivé | Ne met dans chaque flux que la piste audio sélectionnée, au lieu de toutes. | Entrées de configuration `transcoding.hls_ondemand_track_trim`, redémarrage |
| `EVEY_HLS_ONDEMAND_ENCRYPT` | désactivé | Chiffre en AES-128 les flux réempaquetés. Désactive deux choses : le manifeste DASH (HLS seulement) et la lecture enchaînée sans coupure entre épisodes (chaque épisode est lu comme un flux à part). | Entrées de configuration `transcoding.hls_ondemand_encrypt`, redémarrage |
| `EVEY_HLS_KEY_ROTATION_SEGMENTS` | `0` | Avec le chiffrement, change de clé tous les N segments ; `0` = une clé par titre. | Entrées de configuration `transcoding.hls_key_rotation_segments`, redémarrage |
| `EVEY_CAST_WASM_AUDIO_ENABLED` | `false` | Laisse un Chromecast décoder lui-même le Dolby Digital au lieu de recevoir de l'AAC. Désactivé parce que cela surchargeait les récepteurs d'entrée de gamme sans rien économiser. Les navigateurs ne sont pas concernés. | aucun |

## Téléchargements { #downloads }

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_COMPAT_DOWNLOAD` | activé | Prépare un téléchargement dont l'audio est lisible par un téléphone (AAC), vidéo intacte. Seul l'audio est réencodé, ce qui prend quelques minutes. | Entrées de configuration `transcoding.compat_download`, redémarrage |
| `EVEY_DOWNLOAD_QUALITY_CHOICE` | désactivé | Propose une copie plus légère en 720p pour les téléchargements. Elle réencode la vidéo, à peu près en temps réel sur une machine modeste : c'est pourquoi elle reste désactivée tant que vous ne l'activez pas. | Entrées de configuration `downloads.quality_choice`, redémarrage |
| `EVEY_DOWNLOAD_REDUCE_ABOVE_BYTES` | `4294967296` (4 Gio) | La taille au-delà de laquelle l'application propose la copie allégée ; le téléphone la propose aussi quand le film ne tient pas dans l'espace disponible. | Entrées de configuration `downloads.reduce_above_bytes`, redémarrage |

## Lecteur { #player }

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_PLAYBACK_SKIP_FORWARD` | `30` | Secondes sautées par le bouton d'avance. | aucun |
| `EVEY_PLAYBACK_SKIP_BACKWARD` | `30` | Secondes sautées par le bouton de retour. | aucun |
