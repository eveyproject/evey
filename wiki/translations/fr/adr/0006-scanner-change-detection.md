---
source_sha: 18f9d53ab908913b4e875b114eb857308c3c238ca464e7029b76c66d4e53b416
---

# ADR-0006 : contrat de détection de changement du scanner

- Statut : acceptée
- Date : 2026-04-20

## Contexte

Le scanner périodique décide, à chaque passage (toutes les deux minutes), si un
fichier présent sur le disque est déjà représenté par une ligne en base. Un faux
positif, un fichier déclaré « modifié » alors qu'il ne l'est pas, est
catastrophique : le fichier repasse par toute la chaîne, écriture, sondage
ffprobe, réindexation et événements. Sur une médiathèque modeste, cela a occupé
un cœur à 100 % pendant tout un week-end.

Trois sources indépendantes de faux positifs sont apparues la même semaine.

1. **Dérive d'arrondi sur les horodatages.** Postgres stocke les microsecondes
   en arrondissant au plus proche, là où Go tronque vers le bas. Une date de
   modification ext4 au centième de microseconde près est enregistrée arrondie
   puis relue tronquée : les deux valeurs diffèrent, l'égalité échoue, et tout
   fichier ext4 est déclaré modifié à chaque passage.
2. **La date de modification n'était pas écrite à la création.** Le champ
   manquait à la construction de la ligne ; 464 lignes portaient une date nulle,
   et chaque passage voyait un écart entre cette date nulle et la vraie.
3. **L'empreinte n'était écrite qu'après un long sondage.** Le sondage complet
   d'un MKV de 46 Go précédait l'écriture de la ligne. Le délai du travailleur
   expirait pendant le sondage, l'écriture était abandonnée et la nouvelle
   empreinte n'était jamais persistée. Le passage suivant retrouvait l'ancienne
   et repartait dans la même boucle.

Ce ne sont pas des bugs de logique métier : ce sont des violations d'un contrat
implicite sur ce que « modifié » veut dire, et sur l'ordre dans lequel le
scanner doit persister ses décisions.

## Décision

Formaliser le contrat. Tout scanner adossé à des fichiers - séries et films,
photos, musique, livres audio - doit satisfaire les quatre règles suivantes.

**Règle 1 : comparer les dates à la microseconde, avec une tolérance d'une
microseconde.**

```go
func sameMtime(a, b time.Time) bool {
    at := a.Truncate(time.Microsecond)
    bt := b.Truncate(time.Microsecond)
    if at.Equal(bt) {
        return true
    }
    return at.Sub(bt) == time.Microsecond || bt.Sub(at) == time.Microsecond
}
```

Les deux comparaisons unidirectionnelles évitent un débordement quand l'une des
dates est nulle, par exemple sur une ligne fraîchement créée. Ne jamais arrondir
puis comparer : la règle d'arrondi de Postgres n'est pas garantie identique à
celle de Go d'une version de pilote à l'autre.

**Règle 2 : tout champ que le scanner compare doit être écrit dès la création.**
Ce sur quoi le passage suivant se fondera doit exister dès le premier : chemin,
taille, empreinte, date de modification. Un champ manquant, stocké à sa valeur
nulle, force une réécriture à chaque passage.

**Règle 3 : l'état de déduplication est persisté avant tout effet de bord.**

```
1. Mutate in-memory struct (fingerprint, mtime, size, path).
2. s.mediaItems.Update(ctx, item)   ← MUST succeed for next-scan skip to work
3. ProbeAndStoreStreams / extractChapters / IndexMediaItem / publish events
```

D'abord muter la structure en mémoire, puis écrire la ligne, et seulement
ensuite sonder, extraire les chapitres, indexer et publier les événements. Si un
effet de bord échoue, on perd un rafraîchissement récupérable au passage
suivant. Si l'écriture de la ligne échoue parce qu'un effet de bord est passé
devant, le fichier est retraité indéfiniment : c'est le mode d'échec non
récupérable.

**Règle 4 : le délai du travailleur borne le chemin légitime le plus lent, pas
le cas moyen.** Il doit couvrir le sondage et l'écriture du plus gros fichier de
la médiathèque, pas du fichier médian. Trente secondes suffisaient en 1080p et
perdaient la course sur un HEVC 2160p de plus de 40 Go ; le délai est passé à
cinq minutes.

## Conséquences

- **Le contrat est testé.** Deux scans successifs sur le même fichier vérifient
  qu'il est ajouté une fois et pas mis à jour la seconde. L'ancienne assertion,
  qui figeait le comportement fautif, a été inversée.
- **La clé de déduplication stable est complémentaire.** Elle donne une identité
  qui survit à la réécriture du titre par l'enrichissement ; cette décision-ci
  porte sur la détection de changement d'un passage à l'autre.
- **Les nouveaux scanners héritent des règles.** Les scanners photo et musique
  appliquent le même contrat que celui des séries et des films.

## Références

- `internal/app/library/scan_service.go` - la comparaison de dates, la mise à
  jour d'un fichier, le traitement des nouveaux éléments
- `internal/app/photo/command_service.go` et
  `internal/app/music/command_service.go` - le même contrat, dupliqué
- `internal/app/library/scan_service_test.go` - le test des deux scans successifs

## Amendement (2026-05-23)

Les **règles** ci-dessus tiennent toujours, mais deux conséquences ont été
défaites :

- Le raisonnement « la comparaison de dates dupliquée dans trois paquets est
  volontaire, un utilitaire partagé se placerait dans une couche bâtarde » a été
  **renversé**. L'utilitaire a été extrait dans un paquet partagé,
  `internal/pkg/scanindex.SameMtime`, désormais utilisé par les scanners de
  bibliothèque, de photos et de musique. La question de la couche s'est réglée
  en le plaçant dans `pkg/` et non dans `domain/`.
- L'outil de diagnostic photo mentionné plus haut n'a jamais été livré : cette
  référence est caduque.
