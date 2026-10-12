---
source_sha: 9fad7f52fd0f1b44bc2f65ef902487f1f2929c2f19b89072be7f363968ba8bb8
reviewed: true
---
# Regarder ensemble

Regardez le même film au même moment que d'autres personnes, chacune sur son
écran. Une personne ouvre une salle ; les autres la rejoignent avec un code.
Il n'y a rien à régler sur le serveur.

En bref :

1. Lancez le film dans le lecteur web ou dans l'application Android, et
   appuyez sur **Regarder ensemble**. Vous êtes l'hôte de la salle, et vous
   voyez son code : six lettres et chiffres.
2. Envoyez le code aux personnes que vous voulez avec vous.
3. Elles rejoignent la salle avec le code : depuis leur propre compte (voir
   le tableau ci-dessous) ou, sans compte, avec **Regarder ensemble** sur la
   page de connexion.

Qui peut mettre en pause ou avancer le film pour tout le monde dépend de
l'application : voir [Ce que chaque membre contrôle](#what-each-member-controls).

Chaque personne reçoit quand même la version du film qui convient à son
propre appareil, comme expliqué dans
[Comment Evey choisit le mode de lecture](how-playback-is-decided.md).

## Créer et rejoindre une salle

| | Web | Android | Android TV |
| --- | --- | --- | --- |
| Créer une salle | **Regarder ensemble** dans le lecteur | **Regarder ensemble** dans le lecteur | - |
| Rejoindre avec un compte | Le lien de la salle, ou le code dans **Écoute partagée** → **Rejoindre une salle** | Le code dans **Écoute partagée** | Le code dans **Paramètres** → **Regarder ensemble** |
| Rejoindre sans compte | **Regarder ensemble** sur la page de connexion, avec le code | Pareil | - |

Le code est facile à taper : majuscules ou non, espaces, `O` à la place de
`0`, `I` ou `L` à la place de `1`, tout est accepté.

Rejoindre avec un compte suit les règles habituelles : la bibliothèque doit
être partagée avec ce compte, et le contrôle parental du profil doit autoriser
le titre. Un profil restreint ne peut pas non plus créer de salle sur un titre
qu'il n'a pas le droit de regarder.

## Ce que chaque membre contrôle

| | Hôte | Autres membres |
| --- | --- | --- |
| Android | Lecture, pause et déplacement entraînent toute la salle. | Lecture, pause et déplacement restent sur votre écran. |
| Android TV | Lecture et pause entraînent toute la salle ; un déplacement reste sur la TV. | Comme sur Android. |
| Web | Lecture, pause et déplacement restent sur votre écran, hôte compris. | Pareil. |

Dans le lecteur web, des boutons explicites remplacent le contrôle de l'hôte.
**Tout le monde me rejoint** amène toute la salle à votre position et relance
la lecture ; n'importe quel membre peut s'en servir, pas seulement l'hôte.
**Me resynchroniser** vous ramène là où en est la salle, sans déplacer
personne ; il est proposé sur la page invité (rejointe avec un code, sans
compte), pas dans le lecteur d'un compte connecté.

Une personne qui arrive en cours de route démarre là où en est la salle
maintenant, pas là où elle en était lors de la dernière commande.

Si l'hôte s'en va, le membre présent depuis le plus longtemps devient l'hôte.
Quand le dernier membre part, la salle se ferme, et l'accès invité que son
code avait ouvert est retiré. Une salle où personne n'est jamais entré est
supprimée au bout d'une heure.

## Invités sans compte

Un invité qui saisit un code sur la page de connexion regarde ce seul titre,
et rien d'autre :

- en lecture seulement, jamais en téléchargement ;
- uniquement dans les salles vidéo, pas dans les salles de musique ;
- l'accès dure tant qu'il sert, expire après trois heures sans lecture et
  après 24 heures dans tous les cas, et prend fin dès que la salle se ferme.

!!! warning
    Le code est la seule protection d'une salle : ne le donnez qu'aux
    personnes que vous voulez y voir. Les codes erronés sont limités par
    adresse et sur l'ensemble du serveur, ce qui met leur devinette hors de
    portée.

## Écouter ensemble

Une salle de musique se crée depuis **Écoute partagée**, avec une file
d'albums, de morceaux et de playlists. L'hôte lance, met en pause et avance
dans la file ; chaque membre suit. Les invités sans compte ne peuvent pas
rejoindre une salle de musique.
