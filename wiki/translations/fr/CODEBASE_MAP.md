---
source_sha: 6327bfcb083f4f75329d804075903bea93af1550b2cddbc5d00ec5e94d5b4b3e
---

# Carte du code : où vivent les choses et comment les changer

L'orientation de trente minutes pour un nouveau contributeur.
[DEV.md](../DEV.md) met votre environnement en marche ;
[docs/ARCHITECTURE.md](ARCHITECTURE.md) explique la philosophie du découpage en
couches ; CETTE page dit quels fichiers toucher pour les cinq modifications les
plus courantes, dans l'ordre, avec les pièges qui mordent vraiment.

## Le seul schéma qui compte

```
proto/evey/v1/*.proto            the API contract (gRPC + HTTP gateway annotations)
        │  make proto-gen (buf) → gen/ (gitignored)
        ▼
internal/adapters/driving/       "the world calls us"
  grpcapi/                       gRPC handlers (one file per service)
  http/                          raw-HTTP handlers (streaming, HLS, assets, subtitles)
        ▼
internal/app/<domain>/           use cases (command/query services, workers)
        ▼
internal/domain/<name>/          pure models + PORTS (interfaces) + typed events
        ▲ implemented by
internal/adapters/driven/        "we call the world"
  postgres/                      repositories (+ migrations/)
  ffmpeg/  mkvgo/  prober/       media engines
  ...
cmd/evey/wire.go                 composition root: EVERYTHING is wired here
```

Côté interface, le miroir : `frontend/apps/web` (React 19, TanStack Router,
jetons Tailwind), `apps/mobile` (Expo et React Native), `apps/tv`
(react-native-tvos), `packages/api` (client généré par orval depuis l'OpenAPI de
la passerelle, à régénérer avec `pnpm generate` dans ce paquet, à l'intérieur du
conteneur `evey-web`).

## Chemin 1 : ajouter un point d'accès à l'API (gRPC et HTTP)

1. `proto/evey/v1/<service>.proto` : ajoutez le RPC et ses messages, avec une
   annotation `google.api.http` (c'est elle qui engendre la route REST et le
   client orval).
2. `make proto-gen` (ou `buf generate` dans le conteneur `evey-server`), puis
   **redémarrez le serveur de développement** : le rechargement à chaud ne
   surveille pas le code généré.
3. Le gestionnaire dans `internal/adapters/driving/grpcapi/<service>.go`. Gardes
   à ne pas oublier : la présence des revendications, et la vérification d'accès
   sur tout ce qui prend un identifiant de bibliothèque ou de média. C'est le
   motif IDOR, et les relecteurs le cherchent.
4. Le cas d'usage dans `internal/app/<domaine>/` : les gestionnaires restent
   minces.
5. Le câblage dans `cmd/evey/wire.go`, plus les dépendances du serveur gRPC si
   le gestionnaire en réclame une nouvelle.
6. Côté interface : régénérez le client, puis consommez le hook engendré.
7. Les tests : un test de gestionnaire à côté du gestionnaire, les doublures
   vivant dans `internal/domain/<nom>/fakes/`, et un test de service à côté du
   service.

## Chemin 2 : modifier la base de données

1. Une migration `NNNNNN_nom.up.sql` et son `.down.sql` dans
   `internal/adapters/driven/postgres/migrations/`, au numéro suivant ; les
   migrations s'appliquent au démarrage du serveur.
2. **Les identifiants sont des chaînes préfixées (`usr-…`, `lib-…`, `mi-…`),
   jamais des colonnes UUID.** Vérifiez le vrai type de colonne avec `\d <table>`
   avant d'écrire une clé étrangère.
3. Le dépôt dans `internal/adapters/driven/postgres/`, en n'oubliant aucune des
   listes de sélection qui doivent désormais porter votre nouvelle colonne, la
   lecture par identifiant comme les listes : une lecture partielle fait qu'une
   mise à jour de ligne complète efface ensuite le champ en silence.
4. Le champ dans la structure du domaine et la documentation du port dans
   `internal/domain/<nom>/`.
5. Le rechargement à chaud ne rejoue pas les migrations de façon fiable sur un
   conteneur vivant : redémarrez `evey-server` après en avoir ajouté une.

## Chemin 3 : ajouter une page à l'interface web

1. Le fichier de route dans `frontend/apps/web/src/routes/` (TanStack, fondé sur
   les fichiers : `foo.tsx` porte la configuration, `foo.lazy.tsx` le composant).
2. Les données par les hooks engendrés de `@evey/api`, jamais par un `fetch`
   brut : l'enveloppe partagée porte l'authentification, le rafraîchissement et
   la protection CSRF.
3. Les textes ne sont JAMAIS écrits en dur. Les clés vivent côté serveur dans
   `i18n/<langue>/*.yaml`, dans les SIX langues, et doivent aussi être ajoutées à
   la fixture de test anglaise.
4. Le style passe par les jetons de design uniquement : aucune couleur en dur,
   et aucun tiret cadratin nulle part.
5. Les tests, avec vitest et testing-library, à côté de la page, exécutés dans
   le conteneur.

## Chemin 4 : toucher au pipeline de transcodage (à lire deux fois)

> Le flux complet - modèle mental, ordre de lecture, motifs, répartition des
> coûts, où améliorer : [docs/playback-serving.md](playback-serving.md).

Le lecteur est VITAL et le contrat entre le plan et ffmpeg est subtil : trois
désynchronisations audibles ont été livrées le même jour, invisibles pour toutes
les suites unitaires.

- La couche de décision est dans
  `internal/app/transcoding/playback_plan_resolver.go` : décision, échelle et
  empreinte sont résolues UNE fois par requête, l'adaptateur HTTP ne faisant que
  convertir la requête. RÈGLE : toute entrée de la décision doit être inscrite
  dans les URL filles, sinon une interrogation de playlist en cours de lecture
  peut faire basculer la décision sous le lecteur - cette famille de bugs a
  produit des images vertes sur Chromecast.
- L'identité de cache est l'empreinte de décision : des octets différents
  DOIVENT donner une empreinte différente. Les nouvelles dimensions sont hachées
  conditionnellement, seulement quand elles ne valent pas leur défaut, pour ne
  pas orpheliner les caches existants.
- Les arguments ffmpeg sont construits par une fonction **pure**, les sondages
  ayant lieu au démarrage.
- **Le test d'intégration de transcodage est OBLIGATOIRE avant de déclarer une
  modification prête.** Il fait tourner le vrai producteur contre le vrai ffmpeg
  embarqué avec un échantillon hostile et corrèle le CONTENU audio servi. Il est
  branché dans le garde de pré-poussée.

## Chemin 5 : écrire une extension

L'implémentation de référence est `tests/plugins/weather-v2`, sans dépendance
d'exécution ; son script d'empaquetage connaît les pièges de l'analyseur, par
exemple le fait que la chaîne littérale « npm install » n'importe où dans
l'archive provoque un rejet critique. Les parties frontales tournent dans une
iframe isolée sur le web et une WebView sur mobile, avec le pont `window.evey`
(redimensionnement, préférences par profil, lecture des ressources, API de médias
au moyen d'un jeton limité que l'extension ne voit jamais). Le programme
principal ne peut joindre que les hôtes déclarés dans son manifeste et n'écrire
que dans son propre dossier. Documentation complète : [docs/plugins.md](plugins.md).

## Les gardes de qualité, dans l'ordre qui fait gagner du temps

```
go vet <package>                after every Go edit
pnpm tsc --noEmit               after every frontend edit (per app)
go test <package>               before every commit (NEVER go test ./...)
make transcode-itest            if you touched transcode paths
make pre-push                   before declaring anything ready (vet + vuln +
                                build + gosec + lint + arch guards + itest gate)
make doctor                     when your environment misbehaves
```

## Décodeur de vocabulaire

| Terme | Sens |
|---|---|
| palier de copie | qualité HLS qui REMULTIPLEXE la vidéo source au lieu de la réencoder |
| à la demande, `od_` | l'empaqueteur mkvgo qui sert du HLS directement depuis le conteneur, sans ffmpeg |
| rafale | une session ffmpeg née d'une navigation, produisant des segments depuis une image clé en milieu de fichier |
| décision, empreinte | ce qu'il faut servir pour un couple média, capacités du client et préférences, et la clé de cache correspondante |
| remplissage des images clés | le travailleur de fond qui complète la table des images clés pour que le palier de copie puisse découper des segments |
| réparation de l'index de navigation, santé des médias | trier chaque MKV ou WebM en un verdict unique - réparable ici, demande votre intervention, pour information - réparer le premier groupe et acquitter le reste |
| SDUI | le schéma d'interface piloté par le serveur, une fois assaini |
| garde-fou | une assertion d'exécution qui journalise une erreur et une métrique au lieu de corrompre la lecture |
