---
source_sha: c71d26ad4217c6fabc0068b559c21f4a78531ce50e7ff7f66989a1bebc1fd517
reviewed: true
---
# Regarder ensemble

Comment regarder le même titre au même instant que d'autres personnes,
chacune sur son propre appareil, et ce que chacun peut s'attendre à contrôler.
Pour les spectateurs ; rien à configurer sur le serveur. **Regarder
ensemble** ouvre une salle qui tient la position, et chaque lecteur la suit.
Le flux de chacun reste décidé pour son propre appareil, comme dans
[Comment Evey choisit le mode de lecture](how-playback-is-decided.md) : une
personne peut être en lecture directe pendant qu'une autre est transcodée.

## Créer et rejoindre une salle

| | Web | Android | Android TV |
| --- | --- | --- | --- |
| Créer une salle | **Regarder ensemble** dans le lecteur | **Regarder ensemble** dans le lecteur | - |
| Rejoindre avec un compte | Le lien de la salle, ou le code dans **Écoute partagée** → **Rejoindre une salle** | Le code dans **Écoute partagée** | Le code dans **Paramètres** → **Regarder ensemble** |
| Rejoindre sans compte | **Regarder ensemble** sur la page de connexion, avec le code | Pareil | - |

La personne qui crée la salle en est l'hôte et voit son code : six
caractères, lettres et chiffres. Le code se saisit sans se soucier des
majuscules, des espaces, ni de la confusion entre `O` et `0`, ou entre `I`,
`L` et `1`.

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
