---
source_sha: afc0329ab4c360a902337ed894a98e2aa467b1063e4ee7fd4453638fb8afd30f
---

# ADR 0024 : résolveur de plan de lecture dans la couche applicative (une résolution par requête)

Date : 2026-07-18
Statut : acceptée

## Contexte

Le chemin de lecture HLS répond, requête par requête, à la question « comment
servons-nous ce flux » : la décision de copie ou de transcodage piste par piste
avec ses épinglages au moment de la requête, l'échelle de paliers ABR façonnée,
la route matérielle, l'aiguillage du palier de copie, le choix du producteur, et
l'empreinte `DecisionFingerprint` qui sert de clé au cache de segments.
Historiquement, ce cerveau était éparpillé sur environ 8 fichiers
`transcode_vod_*.go` dans l'**adaptateur entrant http**, et chaque consommateur
redérivait ses entrées de son côté : une seule requête de segment recalculait la
décision jusqu'à trois fois (vérification de borne, empreinte, choix du
producteur), en rechargeant à chaque fois les préférences de profil depuis
Postgres, soit 5 lectures de préférences par segment et 6 par manifeste maître.

La justesse reposait sur la discipline que toute dérivation produise des entrées
identiques au bit près (l'*invariant de hachage du cache*) : le moindre écart
entre la vision du manifeste et celle d'un segment désynchronise les URL des
octets servis (la famille d'incidents de l'ADR 0022).

## Décision

Résoudre tout le plan **une seule fois par requête**, dans la couche applicative :

- `PlaybackRequest` (sans dépendance au transport) porte tout ce dont la décision
  a besoin : les faits de la source, les capacités du client, les préférences du
  profil et les épinglages du moment. Le seul travail de l'adaptateur http qui
  touche à la décision est de convertir une `*http.Request` en cet objet
  (`marshalPlaybackRequest`, le seul endroit où un paramètre de requête devient
  une entrée de plan).
- `PlanResolver.Resolve(ctx, req) → PlaybackPlan`
  (`internal/app/transcoding/playback_plan_resolver.go`) calcule la décision,
  puis l'échelle, puis l'empreinte, dans cet ordre fixe (le façonnage crée
  l'épinglage d'admission, et l'empreinte s'y lie), et renvoie un plan
  **immuable** : décision, classe d'échelle, échelle façonnée, codec effectif et
  route matérielle, type d'empaqueteur, empreinte et hachage.
- Tous les consommateurs (entrée du manifeste, sonde de délestage, scellé des URL
  périmées, choix du producteur, clé du cache de segments) lisent cette unique
  résolution. À l'intérieur d'une requête, l'invariant de hachage du cache tient
  **par construction** : une dérivation, et non N qui doivent s'accorder. D'une
  requête à l'autre, il repose toujours sur l'inscription dans les URL et sur
  l'échelle gelée (ADR 0022).
- Toutes les dépendances du résolveur tolèrent la valeur nulle : un résolveur à
  zéro reproduit les replis sûrs historiques (plafond logiciel 720p, pas
  d'admission, pas de palier de copie), si bien que les gestionnaires de test
  écrits à la main gardent leur contrat.

Les préférences de profil derrière la résolution sont mises en cache 10 secondes
par profil (même contrat de fraîcheur que le cache de recherche de média), ce qui
fait passer le chemin chaud des segments de 5 allers-retours Postgres à environ
zéro pendant une rafale de lecture.

## Conséquences

- Le cerveau est testable sans `net/http` ; câbler un nouveau codec, un nouvel
  empaqueteur ou un nouveau conteneur tient en une modification du résolveur, au
  lieu d'un ratissage dans l'adaptateur.
- La migration s'est révélée stable au bit près : l'empreinte de décision d'un
  flux en cours est identique avant et après le déplacement (aucune invalidation
  de cache), et la suite de tests d'adaptateur préexistante, environ 10 000
  lignes, tourne sans modification contre des enveloppes qui délèguent.
- Les chemins mkvgo sans processus (ressources à la demande `od_` et `cc_`) sont
  aiguillés avant l'exécution du résolveur ; l'axe des empaqueteurs du résolveur
  ne concerne jamais que les deux empaqueteurs ffmpeg.
- Ordre de lecture pour tout le chemin : [docs/playback-serving.md](../playback-serving.md).
