---
source_sha: 264e4e9cd6e038d31c6224a57f182539d27ef1f11867974cc2a5393cb2c6d1ca
---

# ADR 0022 : épinglage de la décision HLS et resynchronisation audio-vidéo

Date : 2026-07-09
Statut : acceptée
Portée : implémentée progressivement du 07 au 09-07-2026

## Contexte

Le pipeline HLS choisit, pour chaque lecture, comment servir chaque piste : la
vidéo peut être remultiplexée telle quelle - le palier de copie - ou réencodée,
et l'audio de même. Ce choix, la *décision*, dépend du média, des capacités
déclarées par le client et des préférences du profil actif. Il est calculé une
fois par requête par le résolveur de la couche applicative, et son identité de
cache est son empreinte.

Trois incidents de production distincts ont façonné ce sous-système, et les
invariants ci-dessous existent précisément pour empêcher chacun de se
reproduire.

1. **Images vertes sur Chromecast (07-07-2026).** Une playlist d'événement est
   interrogée en boucle. Un enrichissement d'images clés en tâche de fond est
   arrivé *en cours de visionnage*, si bien qu'entre deux interrogations la
   *même* URL est passée du réencodage au palier de copie. Les segments issus de
   la source ont alors été servis sous une initialisation produite pour le
   réencodage, et le décodeur matériel du Chromecast a affiché n'importe quoi.
2. **Désynchronisation constante sur certains remuxages (08-07-2026).**
   Certains conteneurs portent une piste audio dont le premier horodatage se
   situe environ 0,9 seconde après la vidéo, un défaut du conteneur. Le pipeline
   préserve les horodatages de la source, donc le spectateur entend un décalage
   fixe.
3. **Resynchronisation par l'utilisateur (09-07-2026).** Il faut pouvoir décaler
   l'audio ou les sous-titres quand une source est légèrement fausse, en direct,
   sans rouvrir le fichier.

## Décision

**1. Toute entrée de la décision est inscrite dans les URL filles.** La première
playlist d'une lecture calcule la décision et l'estampille dans chaque URL
descendante : copie ou transcodage, pour la vidéo comme pour l'audio. Les
interrogations suivantes relisent cet épinglage au lieu de redériver, si bien
qu'un enrichissement en tâche de fond ne peut jamais faire basculer le plan
d'une session en cours.

Épingler le transcodage tient toujours : un réencodage est toujours servable.
Épingler la copie ne peut pas être *forcé* : si la décision fraîche ne permet
plus la copie, parce que les images clés ont disparu ou que la vérité HDR a
changé, le plan n'existe structurellement plus, et la session bascule vers le
transcodage AVEC un avertissement journalisé plutôt qu'une erreur 500. Le
lecteur devra peut-être redémarrer ; un avertissement vaut mieux qu'un flux
cassé.

**Règle pour qui ajoute une entrée de décision** : elle DOIT entrer dans le
calcul de l'empreinte, pour que des octets différents produisent une clé de
cache différente, ET voyager dans les URL, pour que toute la session reste
cohérente d'une interrogation à l'autre. Manquer la première fait s'aliaser deux
flux dans le cache ; manquer la seconde fait basculer la décision sous le
lecteur.

**1b. L'empreinte est SCELLÉE dans chaque URI, et un sceau périmé est refusé**
(amendement, incident du 15-07-2026). L'épinglage protège la *décision*, pas la
*forme de l'échelle*. Le numéro de flux est un INDICE dans l'échelle : quand
celle-ci a changé en cours de lecture, la même URL a silencieusement désigné une
*autre* qualité et le lecteur a décodé les octets d'un inconnu, ce qui donnait à
l'écran un arc-en-ciel plein cadre. Le manifeste scelle donc l'empreinte
courante dans chaque URI qu'il émet, et une requête ultérieure dont l'empreinte
fraîche ne correspond plus est refusée par un **410**, avec un en-tête dédié :
le lecteur récupère le manifeste maître et se reconstruit proprement. Une URL
sans sceau, venue d'un cache ou d'un client antérieur, est servie comme avant :
la dégradation se fait dans le sens ouvert. L'avertissement journalisé qui
accompagne chaque refus est le détecteur direct de « l'échelle a bougé sous un
lecteur en cours », situation qui auparavant ne laissait aucune trace.

**2. Le décalage audio de la source est annulé par une seconde entrée décalée.**
Quand le décalage sondé dépasse le plancher de bruit, environ un dixième de
seconde, ET que l'audio est transcodé, le constructeur alimente l'audio depuis
une SECONDE entrée ffmpeg décalée, en laissant l'entrée vidéo et sa comptabilité
de navigation intactes.

Seul l'audio TRANSCODÉ y est éligible. Des paquets audio copiés ne peuvent pas
être rognés au point de navigation, et une seconde entrée y détournait, mesures
à l'appui, le point zéro des sessions de copie : la vidéo atterrissait presque
une seconde à côté du plan et le garde-fou de la ligne de temps se déclenchait.

**3. La resynchronisation utilisateur se compose dans le même décalage net.**
Les sous-titres se décalent entièrement côté client : le lecteur réancre chaque
réplique chargée, sans aller-retour serveur, ce qui fonctionne pour les pistes
intégrées comme externes et reste exactement réversible. L'audio, lui, exige un
réencodage depuis une entrée décalée : le décalage demandé devient donc une
entrée de décision, force le transcodage de l'audio, bifurque l'empreinte, et le
constructeur le combine avec l'annulation du décalage source en UN seul décalage
net. La valeur est quantifiée sur la grille de 100 millisecondes de l'interface,
pour qu'un balayage de décalages voisins ne puisse pas engendrer une flotte
illimitée de sessions ffmpeg.

## Conséquences

- La construction des arguments ffmpeg reste **pure**, les sondages ayant lieu
  au démarrage ; la logique de seconde entrée est partagée par tous les
  constructeurs, si bien que la piste audio décalée n'apparaît jamais sans son
  entrée.
- Le garde-fou de la ligne de temps en copie est le filet d'exécution : il
  journalise une erreur et une métrique plutôt que de corrompre la lecture, si
  une session de copie venait à dériver.
- **Le test d'intégration de transcodage est obligatoire** sur toute
  modification ici : il décode le CONTENU audio servi contre un échantillon
  hostile et le corrèle, dans les deux sens de décalage. Les suites unitaires ne
  peuvent pas attraper un détournement du point zéro.
