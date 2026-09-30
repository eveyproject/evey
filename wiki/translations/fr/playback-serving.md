---
source_sha: 2ff1222cb6483b259b30b0a40ec51e6e4e8d1e7a7f4a5877a9761a0186adff2d
reviewed: true
---
# Servir une vidéo : le chemin de lecture HLS

C'est le **chemin de lecture** : ce qui se passe quand un lecteur demande une
vidéo, de la première requête `master.m3u8` jusqu'aux octets d'un segment. C'est
le pendant du pipeline de scan décrit dans [ARCHITECTURE.md](ARCHITECTURE.md) (le
chemin d'écriture, qui prépare le modèle de lecture), et l'explication détaillée
derrière le « Golden path 4 » de [CODEBASE_MAP.md](CODEBASE_MAP.md).

Le chemin de lecture est **VITAL** et réparti sur environ 8 fichiers de
l'adaptateur `http`. Aucune fonction unique ne déroule tout le flux : ce document
en est la carte.

## Le modèle mental : 3 questions, 2 moteurs, 1 clé

Servir une vidéo revient à répondre à trois questions, puis à empaqueter la
réponse, le tout adressé par un seul hachage :

- **Q1 : puis-je lire le fichier à peu de frais ?** L'aiguillage de l'analyse,
  selon **l'extension du fichier** : `.mkv/.webm` et `.mp4/.mov` vont à **mkvgo**
  (lecture de l'en-tête seulement, sans processus) ; tout le reste, et tout échec
  de ce chemin rapide, va à **ffprobe** (qui fait autorité).
- **Q2 : le client peut-il le lire tel quel ?** La **décision** de lecture, prise
  *piste par piste* : vidéo `copy` ou `transcode`, audio `copy`, `transcode` ou
  le chemin AC-3 en wasm. Vidéo et audio sont décidés **indépendamment** : le cas
  courant est « copier la vidéo, réencoder l'audio en AAC ».
- **Q3 : combien cette machine peut-elle encoder ?** La **capacité** calibrée (issue
  du banc d'essai au démarrage) et **l'admission** (quel barreau de l'échelle
  convient, reste-t-il une place).

Deux moteurs traversent l'ensemble :

- **mkvgo** : lit et réempaquette (remux, à la demande) sans lancer de processus.
  **Gratuit.**
- **ffmpeg / ffprobe** : décode, réencode et analyse en lançant un processus.
  **Coûteux.**

Une clé : tout (répertoire du cache, session ffmpeg, URL des segments) est adressé
par **l'empreinte de décision**, un hachage de la décision. Mêmes octets ⇒ même
hachage ⇒ un seul encodage partagé. La justesse de tout le système repose sur la
**stabilité** de ce hachage entre le manifeste que tient un lecteur et chaque
segment qu'il ira chercher ensuite.

## Ordre de lecture : suivre une requête dans le code

Ouvrez ces fichiers dans l'ordre pour suivre une requête de lecture de bout en
bout :

1. **Entrée** · `internal/adapters/driving/http/transcode_handler_vod.go` :
   `serveHTTP` authentifie, puis aiguille vers master / variante / segment /
   audio.
2. **Q2 · Décision** · `internal/app/transcoding/decision.go` :
   `MakePlaybackDecisionWithPrefs` (pure), le cerveau copie/transcodage piste par
   piste. Enveloppée par `makeDecision` dans `http/transcode_vod_fingerprint.go`,
   qui y ajoute les épinglages de la requête (`vd=` / `ad=`), le forçage de la
   resynchronisation audio, etc.
3. **Q3 · Capacité** · `internal/app/transcoding/calibration_service.go` :
   `Current()` renvoie `domain/transcoding.Derive(report, policy)`, les plafonds
   de la machine, déduits du dernier banc d'essai.
4. **Mise en forme de l'échelle** · `http/transcode_vod_profiles.go` :
   `profilesForDecision` : classe de complexité → profils des producteurs →
   plafonds de la source et de la capacité → plafonds CMCD/av1 → route et plafond
   d'admission, puis **fige l'échelle** sur l'épinglage d'admission (relue telle
   quelle à chaque appel suivant : l'invariant du hachage de cache rendu
   structurel).
5. **Admission** · `internal/app/transcoding/admission_shaper.go` (`StreamPlan`
   immuable) + `video_session_limiter.go` : la route (avec bascule du matériel
   vers le logiciel) et la place de capacité.
6. **Empreinte** · `http/transcode_vod_fingerprint.go` (`buildFingerprint`)
   construit une `DecisionFingerprint` (type et `Hash()` dans
   `app/transcoding/segment_cache.go`). Le hachage est la clé du cache.
7. **Produire les octets** · copie → **mkvgo** `app/transcoding/ondemand.go`
   (port) / `adapters/driven/mkvgo/ondemand.go` ; transcodage → **ffmpeg**
   `adapters/driven/ffmpeg/hls_transcoder.go` (`BuildFFmpegArgs` est pure ; les
   segments arrivent dans `/cache/transcode/segments/<hash>/`).
8. **Empaquetage** · `app/transcoding/manifest_generator.go` : `master.m3u8` /
   `stream_N.m3u8` (et `manifest.mpd` sur le chemin mkvgo).

## Les deux moteurs, selon leur coût

**L'aiguilleur d'analyse** (`adapters/driven/prober/dispatcher.go`) est une chaîne
de responsabilité fondée sur l'extension : il essaie mkvgo, et se rabat sur
ffprobe au moindre doute (erreur d'analyse, panique, résultat impossible à
convertir, piste média MP4 perdue, Dolby Vision profil 5, couleurs illisibles).
Les deux chemins rapides sont toujours actifs (portes nil dans `wire.go`) ;
ffprobe est le filet de sécurité, jamais contourné. Un second aiguilleur
identique (`keyframe_dispatcher.go`) gère l'index de lecture.

| Opération | mkvgo (gratuit) | ffprobe / ffmpeg (coûteux) |
|---|---|---|
| Analyse des métadonnées (codec, dimensions, HDR, débit, durée) | `.mkv/.webm/.mp4/.mov`, en-tête seulement | tout le reste, ou en repli |
| Mode de cadence (cfr/vfr) | `AnalyzeFrameRateMode` | - (migré) |
| Remux MKV→MP4 (sans réencodage) | `RemuxToMP4` | - |
| HLS/DASH à la demande (barreau de copie) | `OnDemandPackager` | - |
| Réencodage vidéo / tone mapping HDR | - | `hls_transcoder.go` |
| Détection d'entrelacement (idet), atterrissage des déplacements, chromaprint, DoVi P5 | - (demande un décodage ou des données annexes) | ffmpeg / ffprobe |

Trois appels ffprobe tournent encore sur des sources MKV/MP4 dont mkvgo expose
déjà les données (durée dans `trickplay.go`, débit et dimensions dans
`complexity_classifier.go`, décalage audio de lecture dans `audio_delay.go`) :
des candidats à la migration, pour un petit gain de processeur, surtout du
câblage.

## L'audio est décidé à part de la vidéo

`makeDecision` produit `VideoDecision` et `AudioDecision` indépendamment. Le
« barreau de copie » (`isCopyRequest`) ne dépend que de la **vidéo** ; même une
vidéo copiée s'accompagne le plus souvent d'une piste AAC réencodée. La décision
audio a trois issues : copie, transcodage en AAC (ffmpeg, léger), ou, pour
**l'AC-3 / E-AC-3** sur un navigateur qui ne le décode pas nativement, le
**chemin wasm** : le master est fait en vidéo seule avec une balise
`#EVEY-AUDIO-URI`, l'AC-3 est **copié** (mkvgo `od_audio` si le plan se construit,
sinon ffmpeg `-c:a copy`), et `ac3go.wasm` le décode en PCM dans le navigateur
(`WasmAudioEngine.ts`), calé sur l'horloge de la vidéo. Safari est exclu par
capacité, pas par son nom : `canWasmDecodeAC3()` ne renvoie vrai que si le
navigateur déclare *ne pas* savoir lire l'AC-3 nativement, et Safari déclare le
savoir.

## Empaquetage : CMAF, HLS, DASH

Le conteneur est toujours du **CMAF** (fMP4) : un `init.mp4` par variante, plus
des segments `.m4s`. (La branche MPEG-TS de `hls_transcoder.go` est morte :
`needsFMP4Segments` renvoie toujours vrai.) Sur *les mêmes* segments :

- **HLS** (`master.m3u8` → `stream_N.m3u8` → init `EXT-X-MAP` + `.m4s`) : les deux
  moteurs.
- **DASH** (`manifest.mpd`) : sur le chemin de copie mkvgo, le manifeste propre à
  l'empaqueteur (`OnDemandPlan.HasDASH`, identique à l'octet près, aucun
  stockage, désactivé sous AES-128) ; sur le chemin ffmpeg, un manifeste
  **déclaré à partir du plan** sur les mêmes fragments que ceux des playlists HLS
  (`ManifestGenerator.MPD`), écrit avant même qu'un fragment existe. Ce qui le
  rend fiable, c'est la **grille de segments** (`domain/transcoding.SegmentGrid`) :
  chaque barreau transcodé est découpé sur le même nombre entier d'images (72 à
  23,976 i/s, 75 à 25, 90 à 29,97), si bien qu'un segment est exact dans les tics
  de la piste vidéo et identique dans tout l'ensemble de bascule ; le repli en
  copie déclare son plan d'images clés. Une source dont la cadence est inconnue
  ou variable reste sur l'ancienne horloge de 3 s, garde le HLS, et répond 404 sur
  `manifest.mpd`. Aucun client n'est basculé en DASH : le HLS reste la valeur par
  défaut partout. `DASHFromCMAF` de mkvgo, appliqué aux fragments produits, sert
  d'oracle de parité (`internal/pkg/cmafparity`, `TestDASHGridIntegration` à
  23,976/25/29,97 i/s : à froid, déplacement, bascule entre deux barreaux, audio
  séparé à une trame AAC près).

Les URL de chaque session sont recalculées (`transcode_vod_manifest_rebake.go`)
pour porter le jeton de flux, en HLS comme en DASH, y compris pour un Chromecast
sans cookie.

## Les patrons de conception à l'œuvre

- **Ports et adaptateurs (hexagonal)** : la couche applicative parle à des
  interfaces (`StreamProber`, `BenchmarkRunner`, `OnDemandPackager`,
  `StreamProducer`) ; ffmpeg et mkvgo les implémentent. Un nouveau moteur est un
  nouvel adaptateur.
- **Stratégie** : `StreamProducer` (logiciel / matériel / copie), choisi par un
  `ProducerSelector`.
- **Chaîne de responsabilité avec repli** : les aiguilleurs d'analyse et d'images
  clés (mkvgo → ffprobe qui fait autorité).
- **Cache adressé par le contenu** : le hachage `DecisionFingerprint` sert de clé
  au cache et à la session ffmpeg.
- **Cœur pur, enveloppe impérative** : `Derive`, `MakePlaybackDecision` et le
  modèle de coût sont purs et testés unitairement sans ffmpeg ni base de données.
- **Objet valeur immuable** : `StreamPlan`, la décision d'admission, émise une
  fois et jamais modifiée, si bien que l'invariant URL↔octets tient par
  construction.
- **Dégradation en douceur** : un repli sûr à chaque étape : capacité 720p par
  défaut, bascule du matériel vers le logiciel, de la copie vers le transcodage,
  de mkvgo vers ffprobe.

## Le cerveau de la décision : le PlanResolver de la couche applicative

Le cerveau du traitement des requêtes (épinglages de décision, mise en forme de
l'échelle, routage matériel, assemblage de l'empreinte, routage du barreau de
copie, choix du producteur) vit dans la couche applicative : `PlanResolver`
(`app/transcoding/playback_plan_resolver.go`), le `PlaybackPlanResolver` concret
([`playback_plan.go`](../internal/app/transcoding/playback_plan.go)). Chaque
requête est résolue **une seule fois** : l'adaptateur http convertit le
`*http.Request` en un `PlaybackRequest` indépendant du transport
(`http/transcode_vod_plan.go`) et sert à partir du `PlaybackPlan` immuable
renvoyé. Chaque consommateur (entrée du manifeste, sonde de délestage, scellement
des URL périmées, choix du producteur, clé de segment) lit cette résolution
unique, si bien que *dans une requête* l'invariant du hachage de cache tient par
construction (et le chemin chaud des segments est passé d'environ 5 lectures des
préférences de profil à 1). Les aides sur les caractéristiques de la source
(codec, HDR, SAR, 10 bits…) vivent à côté, dans
`app/transcoding/source_facts.go` ; l'adaptateur garde de simples alias. D'une
requête à l'autre, l'invariant repose toujours sur l'inscription dans les URL :
**chaque entrée de la décision doit être inscrite dans les URL enfants**, sinon
une interrogation en cours de lecture peut changer le plan sous les pieds du
lecteur (`querySuffix` dans `manifest_generator.go`).
