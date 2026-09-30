---
source_sha: ae7eadc681e514dab83967eab7a3ef8dfadf53af27b3d1e4c676d5dd2d1176ec
reviewed: true
---
# Diffusion de la lecture et téléchargements

La façon dont les flux sont empaquetés, et ce que le serveur prépare pour les
téléchargements. Les valeurs par défaut correspondent au comportement
recommandé ; ces options servent à couper une voie pendant une investigation,
ou à activer une voie désactivée par défaut.

Une variable qui a une clé peut aussi se définir par une ligne portant cette
clé dans **Admin → Configuration du serveur → Entrées de configuration** ; la
ligne l'emporte sur l'environnement. Le panneau propose les clés des
téléchargements ; les autres se saisissent telles qu'elles figurent ici.
Toutes sont lues au démarrage.

## Empaquetage

| Variable | Clé du panneau | Défaut | Effet |
| --- | --- | --- | --- |
| `EVEY_HLS_ONDEMAND` | `transcoding.hls_ondemand` | activé | Sert un fichier dont les pistes peuvent être copiées en le réempaquetant à la volée, sans ffmpeg et sans rien écrire dans le cache. Désactivé : ces fichiers passent par la voie de copie ffmpeg. |
| `EVEY_REMUX_COPY` | `transcoding.remux_copy` | activé | La voie de copie ffmpeg : la vidéo est copiée sans réencodage quand le fichier le permet. Désactivé : ces fichiers sont transcodés. |
| `EVEY_HLS_AUDIO_RENDITION_ENABLED` | | `true` | Propose les pistes audio comme des variantes séparées, pour qu'un lecteur change de langue sans recharger la vidéo. Permet aussi de lire une vidéo copiée avec une piste audio réencodée à part. |
| `EVEY_HLS_COPY_FALLBACK_RUNG` | | `true` | Ajoute une qualité basse transcodée à côté d'un flux copié, pour qu'un lecteur dont la connexion s'effondre puisse descendre au lieu de se figer. Ne coûte rien tant qu'aucun lecteur ne l'utilise. |
| `EVEY_HLS_ONDEMAND_CONCAT` | `transcoding.hls_ondemand_concat` | activé | La lecture enchaînée sans coupure entre épisodes, quand le lecteur web la demande. Un épisode qui ne peut pas être copié revient au flux normal. |
| `EVEY_HLS_ONDEMAND_TRACK_TRIM` | `transcoding.hls_ondemand_track_trim` | désactivé | Ne met dans chaque flux que la piste audio sélectionnée, au lieu de toutes. |
| `EVEY_HLS_ONDEMAND_ENCRYPT` | `transcoding.hls_ondemand_encrypt` | désactivé | Chiffre en AES-128 les flux réempaquetés (HLS seulement : aucun manifeste DASH n'est alors proposé). |
| `EVEY_HLS_KEY_ROTATION_SEGMENTS` | `transcoding.hls_key_rotation_segments` | `0` | Avec le chiffrement, change de clé tous les N segments ; `0` = une clé par titre. |
| `EVEY_CAST_WASM_AUDIO_ENABLED` | | `false` | Laisse un Chromecast décoder lui-même le Dolby Digital au lieu de recevoir de l'AAC. Désactivé parce que cela surchargeait les récepteurs d'entrée de gamme sans rien économiser. Les navigateurs ne sont pas concernés. |

## Téléchargements

| Variable | Clé du panneau | Défaut | Effet |
| --- | --- | --- | --- |
| `EVEY_COMPAT_DOWNLOAD` | `transcoding.compat_download` | activé | Prépare un téléchargement dont l'audio est lisible par un téléphone (AAC), vidéo intacte. Seul l'audio est réencodé, ce qui prend quelques minutes. |
| `EVEY_DOWNLOAD_QUALITY_CHOICE` | `downloads.quality_choice` | désactivé | Propose une copie plus légère en 720p pour les téléchargements. Elle réencode la vidéo, à peu près en temps réel sur une machine modeste : c'est pourquoi elle reste désactivée tant que vous ne l'activez pas. |
| `EVEY_DOWNLOAD_REDUCE_ABOVE_BYTES` | `downloads.reduce_above_bytes` | `4294967296` (4 Gio) | La taille au-delà de laquelle l'application propose la copie allégée ; le téléphone la propose aussi quand le film ne tient pas. |

## Lecteur

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_PLAYBACK_SKIP_FORWARD` | `30` | Secondes sautées par le bouton d'avance. |
| `EVEY_PLAYBACK_SKIP_BACKWARD` | `30` | Secondes sautées par le bouton de retour. |
