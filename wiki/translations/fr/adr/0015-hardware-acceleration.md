---
source_sha: 8fdb05134996166d5108b81aed29b9c53e43915fa5b4eba796529ecd32599e4c
---

# ADR-0015 : une seule image pour tous les GPU, pilotes injectés à l'exécution

- Statut : acceptée
- Date : 2026-06-01

## Contexte

L'encodage matériel réclame un pilote différent par constructeur. Livrer une
image par backend oblige à en construire, publier et maintenir plusieurs en
phase, et force l'administrateur à choisir la bonne avant même de savoir ce que
sa machine sait faire.

## Décision

**Une seule image, aucun pilote cuit dedans.** L'étage d'exécution embarque un
ffmpeg compilé avec NVENC, VAAPI et QSV, et rien qui soit propre à un
constructeur. Les pilotes arrivent au démarrage : le NVIDIA Container Toolkit
les injecte quand un GPU est réservé, et VAAPI ou QSV fonctionnent à partir d'un
`/dev/dri` monté. La même image est donc logicielle seule, NVENC ou VAAPI/QSV
selon ce que la machine hôte lui expose, rien d'autre.

**La machine est mesurée, pas déclarée.** Un banc d'essai au démarrage relève ce
que la machine sait réellement encoder, et le réglage `transcoding.hw_mode` du
panneau d'administration prime sur ce résultat : `auto`, `off`, ou un backend
imposé. Le changer relance le banc d'essai à chaud, sans redémarrage.

**Un matériel qui lâche en cours de lecture retombe sur le logiciel.** Une
session matérielle qui ne produit aucun segment est rétrogradée et l'échec est
retenu ; celle qui n'a pas écrit son segment d'initialisation au bout de cinq
secondes est tuée et rétrogradée. Cinq secondes parce qu'un GPU qui fonctionne
l'écrit en une à trois secondes, et qu'un GPU mort ne l'écrit jamais. Le cache
de segments ignore quel encodeur les a produits, si bien que la reprise
automatique du client atterrit sur le palier logiciel sans toucher au manifeste.

## Conséquences

- L'administrateur expose un GPU ou n'en expose pas. Il n'y a aucune image à
  choisir, donc aucune configuration à rater.
- Ce dont une machine est capable est découvert plutôt que cru sur parole : un
  pilote qui se casse plus tard dégrade vers le logiciel au lieu de casser la
  lecture.
- `NVIDIA_DRIVER_CAPABILITIES` doit contenir `video`. Sans cela le toolkit
  injecte CUDA mais pas la bibliothèque d'encodage, et NVENC échoue alors que
  `nvidia-smi` répond : c'est le seul réglage contre-intuitif.
- VAAPI et QSV ne fonctionnent pas sous WSL2. Le panneau d'administration le
  détecte et les grise plutôt que de faire semblant ; les contournements ffmpeg
  documentés ont été essayés et plantent toujours. Sous WSL, c'est NVENC ou le
  logiciel.
