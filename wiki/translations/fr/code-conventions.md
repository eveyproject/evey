---
source_sha: 6eccec053950207499f9f9b7847f4db7c800e240981b1a5157aefc14cebc9e63
---

# Conventions de code

Les règles qui s'appliquent quand on écrit du Go dans ce dépôt. Ce ne sont pas
des décisions d'architecture : chacune est réversible fichier par fichier, et la
plupart sont tenues par un test plutôt que par la discipline. C'est pourquoi
elles vivent ici et non dans les [décisions d'architecture](adr/README.md), dont
les entrées consignent des choix coûteux à défaire.

Là où un garde existe, c'est le garde qui fait autorité. Cette page explique ce
qu'il impose et pourquoi il a été ajouté ; c'est le test qui empêche
effectivement une régression.

## Goroutines détachées

Plusieurs gestionnaires rendent une réponse immédiate et poursuivent le vrai
travail en arrière-plan : scan de photos, génération du trickplay, délai de
grâce avant de couper une session, envoi de notifications. Chacune des trois
règles ci-dessous existe parce que son absence a provoqué une panne en
production.

Une goroutine d'arrière-plan est une unité de travail indépendante, pas le
prolongement de la requête. Sa durée de vie, son annulation et sa récupération
d'erreur lui appartiennent.

**1. Se détacher du contexte de la requête.** Ne confiez jamais le contexte du
gestionnaire à une goroutine qui lui survit : le cadre l'annule à l'instant où
le gestionnaire rend la main. Un scan de photos a un jour hérité du contexte de
la requête par un repli mal choisi, s'est fait annuler en pleine exploration et
a silencieusement laissé de côté la moitié d'une médiathèque. Utilisez le
contexte du serveur, injecté au câblage, quand le travail doit s'arrêter à
l'extinction ; utilisez un contexte vierge avec un délai quand le travail est
court et borné par lui-même. Le contexte de la requête n'est jamais un repli
valable.

**2. Borner tous les délais.** Aucun contexte vierge sans délai n'atteint une
entrée-sortie. Une goroutine qui attendait cinq secondes avant d'appeler un
service en aval avec un contexte non borné a fui indéfiniment le jour où cet
appel s'est bloqué. Par défaut : trente secondes pour l'écriture d'une ligne,
cinq minutes pour ffprobe, ffmpeg et la recherche, dix minutes pour un
enrichissement de fond. Une autre durée s'accompagne d'un commentaire qui dit
pourquoi.

**3. Récupérer dans chaque goroutine.** Une panique dans une goroutine détachée
n'a plus d'appelant à qui remonter : elle emporte le serveur.

```go
go func() {
    defer func() {
        if r := recover(); r != nil {
            slog.Error("panic recovered in <operation>",
                slog.Any("panic", r),
                slog.Any("stack", debug.Stack()))
        }
    }()
    // work
}()
```

Journalisez la récupération avec sa pile plutôt que de l'avaler, et nommez
l'opération dans le message pour que la recherche dans les journaux la retrouve.
`recover()` ne fonctionne qu'à l'intérieur d'une fermeture différée : seul, il
ne fait rien.

Nous n'ajoutons délibérément pas de récupérateur global par-dessus. Une
goroutine qui oublie sa récupération doit faire tomber le processus bruyamment
plutôt que de laisser le bug se dissimuler.

Les doublures de test suivent la même règle : elles aussi exécutent leurs
abonnés dans des goroutines, et une doublure qui avalerait les paniques
cacherait ce que la production révèle.

## Constructeurs d'agrégats de domaine

Chaque agrégat du domaine expose exactement un constructeur canonique. Avant ce
motif, les littéraux de structure laissaient passer des états invalides : un
identifiant manquant persistait une ligne à clé primaire vide, découverte des
heures plus tard sous la forme d'une erreur de clé étrangère.

```go
func NewX(id ID, required RequiredParam, opts ...XOption) (*X, error) {
    x := &X{ID: id, RequiredField: required, CreatedAt: time.Now(), Enabled: true}
    for _, opt := range opts {
        opt(x)
    }
    if err := x.Validate(); err != nil {
        return nil, err
    }
    return x, nil
}
```

**L'identité est positionnelle, tout le reste est une option.** Ce sans quoi
l'agrégat ne peut pas exister - son identifiant, ses références de propriétaire -
est un paramètre positionnel, si bien que l'omettre est une erreur de
compilation. Tout le reste est une option, si bien que l'omettre donne une
valeur par défaut qui a du sens. Les options sont découvrables : un `grep` sur
`^func With` les liste toutes.

**Les valeurs par défaut vivent dans le constructeur**, jamais chez l'appelant.
Un seul endroit, une seule source de vérité, aucune copie qui dérive.

**Une violation d'invariant remonte en `InvalidArgument`** à la frontière gRPC,
jamais en `Internal`. Du point de vue du gestionnaire, un agrégat invalide est
toujours une erreur de l'appelant ; un vrai bug serveur aurait paniqué avant de
l'atteindre.

**Ce que le constructeur garantit, et ce qu'il ne garantit pas.** Une structure
entrée dans le système par le constructeur satisfait ses invariants. Une
structure hydratée depuis la base est *présumée* les satisfaire : sur les dépôts
Postgres, deux seulement revalident à la lecture. C'est une lacune connue et non
un oubli - ajouter la vérification partout transformerait une seule ligne
préexistante en violation, laissée par une mise à jour manuelle ou une vieille
migration, en erreur de chargement, ce qui est un pire mode d'échec que celui
qu'elle prévient. Refuser ou non une telle ligne est une décision
d'administration, et elle est suivie comme point ouvert plutôt que glissée sous
couvert de conformité documentaire.

## Gardes

Les règles ci-dessus, ainsi que les règles d'architecture de
[ARCHITECTURE.md](ARCHITECTURE.md), sont adossées à des tests statiques dans
`internal/contractguard/` qui font échouer la compilation quand le code s'en
écarte. Lisez ce paquet avant de supposer qu'une convention de cette page n'est
qu'un conseil.
