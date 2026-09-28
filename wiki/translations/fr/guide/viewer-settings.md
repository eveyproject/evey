---
source_sha: 5eee9b7d91ff4e7f592e19232c8e2bcaa6dbd8516301cccf7b266b23b3a06252
---
# Réglages utilisateur

Ce que chaque compte peut modifier lui-même, sans rien demander à
l'administrateur. L'essentiel se trouve dans **Paramètres** (le menu de
l'avatar, puis Paramètres) ; le reste est dans le lecteur. Les applications
Android et TV proposent les mêmes réglages dans leur propre écran Paramètres.

Les réglages sont attachés au **profil** devant l'écran, pas à l'appareil :
choisissez l'audio anglais sur la télé et le téléphone l'utilisera aussi.

## Apparence et langue

| Réglage | Ce qu'il fait |
| --- | --- |
| **Thème** | Le thème de couleurs, y compris ceux installés par l'administrateur. |
| **Langue** | La langue de l'interface. Six sont disponibles : anglais, français, allemand, espagnol, hindi et chinois. |
| **Région** | Le pays dont les classifications d'âge sont affichées et appliquées. **Automatique** suit le serveur. |
| **Langue audio préférée** | La piste choisie en premier quand un fichier a plusieurs langues. |
| **Langue des sous-titres préférée** | La piste de sous-titres affichée quand l'une correspond ; **Aucune préférence** les laisse éteints sauf si le fichier les impose. |

Les titres et les résumés suivent la langue des métadonnées du serveur,
choisie par l'administrateur.

## Lecture

| Réglage | Par défaut | Ce qu'il fait |
| --- | --- | --- |
| **Épisode suivant automatique** | activé | Lance l'épisode suivant à la fin du précédent. Le bouton Épisode suivant reste disponible dans tous les cas. |
| **Débit maximum en streaming** | Illimité | Un plafond pour vos flux. Abaissez-le sur une connexion lente : le serveur convertit pour tenir dans le débit au lieu de laisser le lecteur caler. |
| **Débit maximum en mobile** (téléphone) | - | Un plafond plus bas, appliqué uniquement en données mobiles. |
| **Codec vidéo préféré** / **Codec audio préféré** | Auto | Laissez sur Auto sauf si un appareil se comporte mal avec un codec : Auto laisse le serveur choisir ce que l'appareil décode le mieux. |
| **Canaux audio maximum** | Auto | Stéréo, 5.1 ou 7.1. Choisissez Stéréo si vous regardez au casque ou sur les haut-parleurs de la télé et que les dialogues sont trop faibles dans les mixages surround. |
| **Incrustation des sous-titres** | Auto | Certains formats de sous-titres (en image, ou très stylés) ne peuvent pas être dessinés par tous les lecteurs ; Auto ne les incruste dans l'image que dans ce cas. **Toujours incruster** et **Jamais incruster** forcent le choix, au prix d'une conversion ou de sous-titres manquants. |
| **Autoriser la lecture directe** / **le flux direct** / **le transcodage** | tous activés | Les trois façons dont un film peut vous parvenir : tel quel, réempaqueté ou converti. En désactiver une est un outil de dépannage, pas un réglage de tous les jours. |

L'administrateur peut fixer un plafond plus bas sur votre compte ou sur les
connexions venant de l'extérieur du domicile ; c'est toujours le plafond le
plus bas qui l'emporte.

## Dans le lecteur

L'icône d'engrenage ouvre les réglages du lecteur :

- **Qualité** : **Auto** s'adapte à votre connexion ; **Original** demande le
  fichier intact quand l'appareil sait le lire ; une hauteur fixe (1080p,
  720p...) force une conversion à cette taille.
- **Audio** et **Sous-titres** : changez de piste à tout moment, sans repartir
  du début. **Téléverser un sous-titre...** ajoute un fichier de sous-titres
  depuis votre ordinateur pour ce titre.
- **Synchro audio** : décale le son par rapport à l'image, en millisecondes
  (positif = son plus tard). Disponible quand le film est converti ; la
  lecture redémarre à la position actuelle.
- **Synchro sous-titres** : décale les sous-titres (positif = plus tard),
  instantanément.
- **Style des sous-titres** : taille, couleur du texte, fond, police, position,
  ombre.
- **Vitesse** et **Minuteur de veille** (un nombre de minutes, ou la fin de
  l'épisode).

Autour de la barre de progression : les **chapitres**, les boutons **passer
l'intro**, **passer le résumé**, **passer le générique** quand le serveur sait
où ils se trouvent, un compte à rebours vers l'épisode suivant, **Caster**,
**Lire sur** (envoyer vers une autre application Evey), **Regarder ensemble**
et **Télécharger** (téléphone).

**Infos du flux** vous dit comment le film vous parvient - lecture directe,
remux ou transcodage - et pourquoi. C'est la première chose à regarder quand
une lecture se passe mal : « Codec audio non lisible par ce lecteur » ou
« Débit au-dessus de la limite autorisée » l'expliquent souvent.

Sur un ordinateur, `?` affiche les raccourcis clavier : `Espace` ou `K` pour
la pause, `J`/`L` ou les flèches gauche et droite pour se déplacer, haut et bas
pour le volume, `F` pour le plein écran, `M` pour couper le son, `C` pour les
sous-titres, `<` et `>` pour la vitesse, `P` pour l'image dans l'image, et `0`
à `9` pour sauter de 0 à 90 % du film.

## Profils et contrôle parental

**Paramètres → Profils** crée, modifie et supprime les profils (le profil par
défaut ne peut pas être supprimé). Chaque profil a :

| Champ | Ce qu'il fait |
| --- | --- |
| **Nom** et **avatar** | Affichés dans l'écran « Qui regarde ? ». |
| **Profil enfant** | S'ouvre sans code PIN ; le quitter pour un autre profil demande le code PIN du compte. |
| **Âge maximum** | Les titres au-dessus ne sont jamais envoyés à ce profil - ni dans les listes, ni dans la recherche, ni au lecteur. Décidé par le serveur : aucune application ne peut le contourner. |
| **Langue** et **Région** | Remplacent, pour ce profil, la langue de l'interface et le pays des classifications. |

**Paramètres → Contrôle parental** définit le **code PIN à 4 chiffres** du
compte. La première fois, il demande le mot de passe du compte ; pour le
changer ensuite, il demande le code actuel. Un code PIN est obligatoire dès
qu'un profil enfant existe.

Si le serveur ne parvient pas à lire les réglages d'âge d'un profil, il
n'affiche rien plutôt que tout : un réglage abîmé n'ouvre jamais la
bibliothèque à un enfant.

## Téléchargements (téléphone)

**Télécharger** sur un film, une saison, une série, une collection ou une
liste de lecture la garde sur le téléphone. Quand un film est trop lourd pour
l'espace libre, l'application propose une copie plus légère préparée par le
serveur, avec la taille attendue de chaque choix. **Téléchargements** liste ce
qui est stocké et la place occupée.

Un téléchargement reste soumis aux règles du profil : un titre au-dessus de la
limite d'âge d'un profil n'est pas lisible depuis ce profil, même si un autre
profil l'a téléchargé. Un badge signale quand un téléchargement a expiré et
doit être récupéré à nouveau.

## Notifications

**Paramètres → Notifications** active les notifications push sur cet appareil
et règle, séparément pour les films, les séries et la musique, ce que vous
voulez recevoir : **Toutes**, **Pertinentes**, **Importantes uniquement** ou
**Désactivé**. Le push demande que l'administrateur l'ait configuré
([Notifications](../notifications.md)).

## Compte et appareils

- **Compte** (carte du profil) : nom d'utilisateur, e-mail, mot de passe (huit
  caractères au moins) et avatar. L'e-mail est ce qui permet de réinitialiser
  un mot de passe oublié, quand le serveur sait envoyer du courrier.
- **Appareils** : les appareils inscrits aux notifications push, que vous
  pouvez retirer.
- **Connexion rapide** : tapez, ou scannez, le code affiché par une télévision
  ou un autre appareil pour le connecter à votre compte sans y taper votre mot
  de passe.
- **Historique de visionnage** (menu du profil) : tout ce que vous avez
  regardé, jour par jour, avec un bouton pour retirer une entrée ou tout
  effacer.
- **Plugins** : quand l'administrateur a installé des extensions qui ont leurs
  propres préférences, elles apparaissent ici.

## Demandes

Si l'administrateur l'autorise, **Demandes de média** permet de chercher un
film, une série ou un artiste absent de la bibliothèque et de le demander.
**Mes demandes** affiche la réponse : acceptée, refusée ou reportée, avec la
note de l'administrateur.
