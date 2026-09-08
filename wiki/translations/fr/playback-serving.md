---
source_sha: 8e9b10f4cdc162d51247a1432d8c6b2a3ab2490d0c01a856d52961436aa38556
---

# Servir une vidéo - le chemin de lecture HLS

Voici le **chemin de lecture** : ce qui se passe quand un lecteur réclame une
vidéo, depuis la toute première requête `master.m3u8` jusqu'aux octets d'un
segment. C'est le pendant de la chaîne d'analyse décrite dans
[ARCHITECTURE.md](ARCHITECTURE.md) (le chemin d'écriture, celui qui amorce le
modèle de lecture), et le détail derrière le « Golden path 4 » de
[CODEBASE_MAP.md](CODEBASE_MAP.md).

Le chemin de lecture est **VITAL** et réparti sur environ 8 fichiers de
l'adaptateur `http`. Aucune fonction unique ne déroule tout le flux : ce document
est la carte.

## Le modèle mental : 3 questions, 2 moteurs, 1 clé

Servir une vidéo revient à répondre à trois questions, puis à emballer la
réponse, le tout adressé par une seule empreinte :

- **Q1 - Puis-je lire le fichier à bas coût ?** L'aiguillage du sondage se fait
  par **extension de fichier** : `.mkv/.webm` et `.mp4/.mov` partent vers
  **mkvgo** (en-tête seul, aucun processus lancé) ; tout le reste, ainsi que tout
  échec du chemin rapide, part vers **ffprobe**, qui fait autorité.
- **Q2 - Le client sait-il le lire tel quel ?** C'est la **décision** de lecture,
  prise *piste par piste* : vidéo en `copy` ou en `transcode`, audio en `copy`, en
  `transcode` ou par le chemin wasm AC-3. Vidéo et audio sont tranchées
  **indépendamment** - le cas courant étant « copier la vidéo, réencoder le son en
  AAC ».
- **Q3 - Combien cette machine sait-elle encoder ?** La **capacité** calibrée,
  issue du banc d'essai du démarrage, plus l'**admission** (quel palier de
  l'échelle tient, et reste-t-il une place).

Deux moteurs traversent tout cela :

- **mkvgo** - lit et ré-emballe (remultiplexage, à la demande) sans lancer de processus. **Gratuit.**
- **ffmpeg / ffprobe** - décode, réencode, sonde, en lançant un processus. **Coûteux.**

Et une seule clé : tout - répertoire de cache, session ffmpeg, URL des segments -
est adressé par l'**empreinte de la décision**, un hachage de cette décision.
Mêmes octets ⇒ même hachage ⇒ un seul encodage partagé. Toute la justesse du
système repose sur la **stabilité** de ce hachage entre le manifeste que détient
un lecteur et chacun des segments qu'il ira ensuite chercher.

## Ordre de lecture - suivre une requête à travers le code

Ouvrez ces fichiers dans cet ordre pour tracer une requête de lecture de bout en
bout :

1. **Entrée** · `internal/adapters/driving/http/transcode_handler_vod.go` -
   `serveHTTP` authentifie, puis aiguille vers le master, la variante, le segment
   ou l'audio.
2. **Q2 · Décision** · `internal/app/transcoding/decision.go` -
   `MakePlaybackDecisionWithPrefs` (fonction pure) : le cerveau du copy/transcode,
   piste par piste. Elle est enveloppée par `makeDecision`, dans
   `http/transcode_vod_fingerprint.go`, qui y superpose les épinglages posés au
   moment de la requête (`vd=`, `ad=`), la dérogation de resynchronisation audio,
   etc.
3. **Q3 · Capacité** · `internal/app/transcoding/calibration_service.go` -
   `Current()` rend `domain/transcoding.Derive(report, policy)` : les plafonds de
   la machine, dérivés du dernier banc d'essai.
4. **Mise en forme de l'échelle** · `http/transcode_vod_profiles.go` -
   `profilesForDecision` : classe de complexité → profils producteurs → plafonds
   de la source et de la capacité → plafonds CMCD et AV1 → route et plafond
   d'admission, puis **fige l'échelle ainsi formée** sur l'épinglage d'admission
   (relue telle quelle à chaque appel ultérieur : l'invariant du hachage de cache,
   rendu structurel).
5. **Admission** · `internal/app/transcoding/admission_shaper.go`
   (le `StreamPlan` immuable) et `video_session_limiter.go` : la route (avec le
   débordement du matériel vers le logiciel) et la place en capacité.
6. **Empreinte** · `http/transcode_vod_fingerprint.go` (`buildFingerprint`)
   construit une `DecisionFingerprint` (le type et son `Hash()` vivent dans
   `app/transcoding/segment_cache.go`). Ce hachage est la clé de cache.
7. **Produire les octets** · en copie → **mkvgo**, `app/transcoding/ondemand.go`
   (le port) et `adapters/driven/mkvgo/ondemand.go` ; en transcodage →
   **ffmpeg**, `adapters/driven/ffmpeg/hls_transcoder.go` (`BuildFFmpegArgs` est
   pure ; les segments atterrissent dans `/cache/transcode/segments/<hash>/`).
8. **Emballage** · `app/transcoding/manifest_generator.go` - `master.m3u8` et
   `stream_N.m3u8` (plus `manifest.mpd` sur le chemin mkvgo).

## Les deux moteurs, par leur coût

L'**aiguilleur de sondage** (`adapters/driven/prober/dispatcher.go`) est une
chaîne de responsabilité indexée sur l'extension : il tente mkvgo, et se replie
sur ffprobe au moindre doute (erreur d'analyse, panique, résultat intransposable,
piste média MP4 perdue, profil DoVi 5, colorimétrie illisible). Les deux chemins
rapides sont toujours actifs (portes à nil dans `wire.go`) ; ffprobe est le filet
de sécurité, jamais court-circuité. Un second aiguilleur, identique
(`keyframe_dispatcher.go`), s'occupe de l'index de navigation.

| Opération | mkvgo (gratuit) | ffprobe / ffmpeg (coûteux) |
|---|---|---|
| Sondage des métadonnées (codec, dimensions, HDR, débit, durée) | `.mkv/.webm/.mp4/.mov`, en-tête seul | tout le reste, ou en repli |
| Mode de cadence (cfr/vfr) | `AnalyzeFrameRateMode` | - (migré) |
| Remultiplexage MKV→MP4 (sans réencodage) | `RemuxToMP4` | - |
| HLS/DASH à la demande (palier de copie) | `OnDemandPackager` | - |
| Réencodage vidéo, tone mapping HDR | - | `hls_transcoder.go` |
| Détection d'entrelacement (idet), point d'atterrissage de navigation, chromaprint, DoVi P5 | - (exige un décodage ou des données annexes) | ffmpeg / ffprobe |

Trois forks ffprobe subsistent sur des sources MKV/MP4 dont mkvgo expose déjà les
données (la durée dans `trickplay.go`, le débit et les dimensions dans
`complexity_classifier.go`, le décalage audio de lecture dans `audio_delay.go`) :
ce sont des candidats à la migration, pour un petit gain de processeur, l'essentiel
du travail étant du câblage.

## Le son se décide séparément de la vidéo

`makeDecision` produit indépendamment une `VideoDecision` et une
`AudioDecision`. Le « palier de copie » (`isCopyRequest`) ne dépend que de la
**vidéo** : même une vidéo copiée s'accompagne d'ordinaire d'une piste AAC
réencodée. La décision audio a trois issues : la copie, le transcodage en AAC
(ffmpeg, léger), ou - pour de l'**AC-3 / E-AC-3** sur un navigateur incapable de
le décoder nativement - le **chemin wasm** : le master est réduit à la vidéo,
muni d'une balise `#EVEY-AUDIO-URI`, l'AC-3 est **copié** (par le `od_audio` de
mkvgo si le plan se construit, sinon par un `-c:a copy` de ffmpeg), et
`ac3go.wasm` le décode en PCM dans le navigateur (`WasmAudioEngine.ts`), calé sur
l'horloge de la vidéo. Safari est écarté par capacité, pas par son nom :
`canWasmDecodeAC3()` ne rend `true` que si le navigateur déclare qu'il *ne sait
pas* lire l'AC-3 nativement - or Safari déclare qu'il le sait.

## Emballage : CMAF, HLS, DASH

Le conteneur est toujours du **CMAF** (fMP4) : un `init.mp4` par variante, plus
des segments `.m4s`. (La branche MPEG-TS de `hls_transcoder.go` est morte :
`needsFMP4Segments` rend toujours `true`.) Au-dessus des *mêmes* segments :

- **HLS** (`master.m3u8` → `stream_N.m3u8` → l'init `EXT-X-MAP` puis les `.m4s`) - sur les deux moteurs.
- **DASH** (`manifest.mpd`) - **sur le seul chemin de copie mkvgo**
  (`OnDemandPlan.HasDASH`), à l'octet près, sans un octet de stockage ; absent du
  chemin de transcodage ffmpeg, et désactivé quand le chiffrement AES-128 est en
  service.

Les URL propres à une session sont reforgées
(`transcode_vod_manifest_rebake.go`) pour porter le jeton de flux, en HLS comme
en DASH, y compris pour un Chromecast dépourvu de cookies.

## Les motifs de conception à l'œuvre

- **Ports et adaptateurs (architecture hexagonale)** - la couche applicative ne
  parle qu'à des interfaces (`StreamProber`, `BenchmarkRunner`,
  `OnDemandPackager`, `StreamProducer`), que ffmpeg et mkvgo implémentent. Un
  nouveau moteur, c'est un nouvel adaptateur.
- **Stratégie** - `StreamProducer` (logiciel, matériel, copie), retenu par un
  `ProducerSelector`.
- **Chaîne de responsabilité avec repli** - les aiguilleurs de sondage et
  d'images clés (mkvgo → ffprobe, qui fait autorité).
- **Cache adressé par le contenu** - le hachage de la `DecisionFingerprint` sert
  de clé au cache comme à la session ffmpeg.
- **Cœur pur, coquille impérative** - `Derive`, `MakePlaybackDecision` et le
  modèle de coût sont purs, et testés unitairement sans ffmpeg ni base de
  données.
- **Objet-valeur immuable** - `StreamPlan` : la décision d'admission, émise une
  fois, jamais modifiée, de sorte que l'invariant entre les URL et les octets
  tient par construction.
- **Dégradation en douceur** - un repli sûr à chaque étape : capacité 720p par
  défaut, débordement du matériel vers le logiciel, transcodage à défaut de
  copie, ffprobe à défaut de mkvgo.

## Le cerveau de la décision : le PlanResolver de la couche applicative

Le cerveau qui traite une requête - épinglages de décision, mise en forme de
l'échelle, routage matériel, assemblage de l'empreinte, routage du palier de
copie, choix du producteur - vit dans la couche applicative : `PlanResolver`
(`app/transcoding/playback_plan_resolver.go`), dont l'implémentation concrète est
`PlaybackPlanResolver`
([`playback_plan.go`](../internal/app/transcoding/playback_plan.go)). Chaque
requête est résolue **une seule fois** : l'adaptateur http transpose la
`*http.Request` en une `PlaybackRequest` affranchie du transport
(`http/transcode_vod_plan.go`), puis sert à partir du `PlaybackPlan` immuable
qu'il reçoit en retour. Tous les consommateurs - l'entrée du manifeste, la sonde
de délestage, le scellé des URL périmées, le choix du producteur, la clé de
segment - lisent cette résolution unique : *au sein d'une requête*, l'invariant
du hachage de cache tient donc par construction (et le chemin chaud des segments
est passé d'environ 5 lectures des préférences de profil à une seule). Les
fonctions qui exposent les faits de la source (codec, HDR, SAR, dix bits, …)
vivent à côté, dans `app/transcoding/source_facts.go`, l'adaptateur n'en gardant
que de minces alias. D'une requête à l'autre, en revanche, l'invariant repose
toujours sur ce qui est inscrit dans les URL : **toute entrée de la décision doit
être inscrite dans les URL filles**, faute de quoi une scrutation en cours de
lecture peut retourner le plan sous les pieds du lecteur (`querySuffix` dans
`manifest_generator.go`).
