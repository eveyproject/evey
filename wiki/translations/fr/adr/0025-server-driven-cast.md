---
source_sha: 7a75b930204b58681f53340dd2596b96a31cc27873540c83876ee6d36759cd60
---

# ADR 0025 : cast piloté par le serveur avec un récepteur web dédié

Date : 2026-07-25
Statut : acceptée

## Contexte

Le cast depuis un navigateur émetteur reposait historiquement sur le récepteur
intégré de Chromium, l'émetteur calculant et envoyant lui-même une URL de média
directe. En pratique, Chromium rapportait `duration=NaN` pour ces URL directes,
ce qui rendait l'envoi côté émetteur peu fiable : le récepteur n'avait aucune
durée digne de confiance sur laquelle se caler pour naviguer ou rapporter une
progression, et l'état de lecture sur les cibles de cast (téléviseurs de classe
Chromecast compris) se désynchronisait de la source de vérité détenue par le
serveur.

Le cast doit en outre porter des comportements propres à Evey que le récepteur
Chromium par défaut ne sait pas fournir : le décodage audio AC-3 par le décodeur
wasm `ac3go` (le décodeur natif du téléviseur ne couvre pas tous les codecs
sources) et les décisions de passthrough selon le profil de capacités audio de
l'appareil.

## Décision

- Le cast est piloté **côté serveur** : l'émetteur appelle `LoadApp` pour confier
  l'intention de lecture au serveur, qui mène la session de cast, au lieu que
  l'émetteur calcule et envoie une URL directement à un récepteur générique.
- Une application **récepteur web dédiée** est enregistrée sous l'identifiant
  d'application Cast `D36A449B`, hébergée sur `cast.eveyproject.org`. Elle fait
  tourner le même décodeur wasm `ac3go` que le reste de la pile, si bien que les
  sources AC-3 se lisent sur le téléviseur sans repli par passthrough ou
  transcodage.
- Trois émetteurs sont câblés sur ce récepteur : le web, le mobile et la TV.

## Conséquences

- C'est le serveur, et non l'émetteur, qui fait autorité sur l'état de lecture du
  cast (position, durée, erreurs), ce qui supprime la famille de désynchronisations
  du type `duration=NaN` entre ce que l'émetteur affiche et ce qui se joue
  réellement sur la cible.
- Evey possède désormais un déploiement de récepteur web (`cast.eveyproject.org`)
  qu'il doit maintenir, en plus du serveur et des trois clients émetteurs : une
  surface de plus à garder synchronisée avec les changements de codec ou de
  décodeur (les mises à jour du wasm `ac3go`, par exemple).
- Les profils de capacités audio d'appareils qui déclarent à tort gérer le
  passthrough (constaté sur au moins un modèle de téléviseur) peuvent encore
  bloquer une session de cast ; cela se suit et se corrige profil par profil, pas
  par cette décision.
