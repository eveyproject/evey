---
source_sha: 970d79357da787e635d08003e577bd5a9a2aa1600f1dd6722628b4291f0dea6f
reviewed: true
---

# Questions fréquentes

Des réponses courtes aux questions que l'on pose à la personne qui fait
tourner Evey à la maison. Chacune dit quoi faire d'abord, puis renvoie vers
la page qui explique pourquoi, pour le jour où vous voudrez comprendre.

## Pourquoi le film met-il du temps à démarrer ?

Le plus souvent parce que le serveur l'adapte. Votre télé ou votre téléphone
ne sait pas lire ce fichier exactement tel qu'il est stocké, alors le serveur
le convertit pendant que vous regardez, et les premières secondes servent à
préparer cela. Un film qui démarre presque tout de suite est envoyé tel quel.

Ouvrez **Infos du flux** dans le lecteur (le bouton (i)) : le panneau dit
lequel des deux cas s'applique, piste par piste, et pourquoi. Si un seul titre
est lent à chaque fois alors que les autres vont bien, c'est probablement le
fichier lui-même, et la personne qui fait tourner Evey peut le vérifier dans
**Administration → Santé des médias**.

Pour aller plus loin : [Comment Evey choisit le mode de lecture](../playback/how-playback-is-decided.md#reading-stream-info)
et, pour la personne qui l'administre, [Santé des médias](../runbooks/media-health.md).

## Pourquoi l'image est-elle moins nette sur mon téléphone quand je ne suis pas chez moi ?

Parce qu'un plafond s'applique aux spectateurs hors de la maison. La personne
qui fait tourner Evey peut fixer deux limites dans **Administration →
Configuration transcodage** : une pour tout le monde, et une plus basse pour
les spectateurs hors du réseau local, afin qu'un film continue de passer sur
une connexion mobile. Chaque compte peut aussi avoir son propre **Débit
maximal**. Un titre au-dessus de la limite est converti pour y tenir, et un
peu de détail s'en va avec.

Vous n'avez rien à choisir de votre côté : Evey ne vous demande jamais une
qualité. **Infos du flux** montre quelle limite s'est appliquée.

Pour aller plus loin : [Ce qui est encodé](../playback/transcoding.md#what-is-encoded) et
[Ce qu'un administrateur peut restreindre par compte](../administration/users.md#what-an-administrator-can-restrict-per-account).

## Pourquoi le son est-il dans la mauvaise langue ?

Une piste que vous choisissez à la main, dans le menu des réglages du lecteur,
gagne toujours pour ce titre. Sinon, les lecteurs prennent la langue réglée
dans **Paramètres → Lecture → Langue audio par défaut**, puis la piste que le
fichier marque comme piste par défaut, puis la première. Réglez votre langue
là une fois et chaque titre la suivra. Même chose pour les sous-titres, avec
**Langue des sous-titres par défaut**.

Pour aller plus loin : [Quelle piste est lue](../playback/audio.md#which-track-plays).

## Est-ce que ça marche sur ma télévision ?

Oui, de trois manières :

- **Une Android TV ou une Google TV** fait tourner l'application Evey, pensée
  pour la télécommande. Sur l'écran de connexion, **Connexion rapide** affiche
  un code que vous approuvez depuis votre téléphone ou votre ordinateur, pour
  ne jamais taper un mot de passe à la télécommande.
- **Une télé avec un navigateur web** ouvre le même genre d'écran à l'adresse
  du serveur suivie de `/tv`.
- **Un Chromecast**, ou une télé avec Chromecast intégré, reçoit l'image
  depuis le bouton Cast du lecteur web ou Android.

Pour aller plus loin : [Connecter les applications](apps.md#quick-connect-signing-in-a-tv-without-typing-a-password)
et [Caster](../playback/casting.md).

## Puis-je regarder dans le train, sans connexion ?

Avec l'application Android, oui. Sur un film ou un épisode, l'application peut
en garder une copie sur le téléphone (**Téléchargements**), lisible pendant
30 jours sans contacter le serveur. Quand la personne qui fait tourner Evey
l'active, l'application propose aussi une copie plus petite pour un film trop
gros pour le téléphone. Un profil enfant ne peut pas ouvrir un téléchargement
au-dessus de sa limite d'âge, même si un autre profil l'a fait.

Pour aller plus loin : [Ce que chaque application ajoute](apps.md#what-each-app-adds).

## Puis-je regarder un film avec un ami qui habite ailleurs ?

Oui. Lancez le film dans le lecteur web ou l'application Android et appuyez
sur **Regarder ensemble** : vous obtenez un code de six caractères. Votre ami
le saisit, depuis son propre compte ou, sans compte, avec **Regarder
ensemble** sur la page de connexion. Chacun voit le même instant, sur son
propre écran, dans la version qui convient à son propre appareil.

Pour aller plus loin : [Regarder ensemble](../playback/watching-together.md).

## Comment donner accès à un ami sans ouvrir tout le catalogue ?

Dans **Administration → Utilisateurs**, créez un lien d'invitation et
limitez-le aux bibliothèques que cette personne peut voir : les autres
n'apparaîtront nulle part pour elle, même pas dans la recherche. Vous pouvez
aussi plafonner la qualité qu'elle reçoit (**Débit maximal**) et le nombre
d'écrans qu'elle peut lancer en même temps. Pour un seul film, un **Lien de
téléchargement public** sur sa page permet à quiconque a le lien de
télécharger le fichier pendant 48 heures, sans compte.

Pour aller plus loin : [Créer un compte](../administration/users.md#create-an-account) et
[Liens de téléchargement publics](../administration/users.md#public-download-links).

## Pourquoi un film n'a-t-il pas de jaquette, ou la mauvaise ?

Parce que le nom du fichier ne désignait pas le bon titre. Pour un film,
l'année compte le plus (`The Mirror (1999)`) ; pour un épisode, le numéro de
saison et d'épisode (`S01E05`). Pour corriger un titre, ouvrez-le, choisissez
**Modifier les métadonnées**, puis **Identifier sur TMDB** et prenez la bonne
correspondance. Un fichier nouveau, renommé ou supprimé apparaît tout seul en
quelques minutes ; **Scanner** sur la ligne de la bibliothèque le déclenche tout de suite.

Pour aller plus loin : [Nommer les fichiers pour qu'ils soient reconnus](../administration/libraries.md#name-files-so-they-are-recognised)
et [Corriger un titre faux ou absent](../administration/libraries.md#fix-a-wrong-or-missing-title).

## Les sous-titres disent qu'ils sont en préparation. Quelque chose ne va pas ?

Non. La première fois que quelqu'un choisit des sous-titres, le serveur les
extrait du fichier ; la vidéo continue et ils apparaissent d'eux-mêmes une fois
prêts. Cela n'arrive qu'une fois par piste. Si le message reste plusieurs
minutes, le fichier n'a pas d'index pour cette piste et le serveur le lit en
entier, une fois.

Pour aller plus loin : [La première fois qu'un sous-titre est affiché](../playback/subtitles.md#the-first-time-a-subtitle-is-shown).

## Tout est lent pour tout le monde en même temps. Pourquoi ?

Le serveur convertit un nombre limité de vidéos à la fois, un nombre qu'il a
mesuré sur son propre matériel. Quand la place vient à manquer, il baisse
d'abord la qualité d'un nouveau flux, puis le bascule sur le processeur, et
seulement ensuite affiche **Le serveur est occupé par d'autres
transcodages**. Les films envoyés tels quels ne comptent pas, c'est pourquoi un
catalogue adapté aux appareils de la maison atteint rarement la limite. La
personne qui fait tourner Evey voit la charge dans **Administration → Santé
du système**.

Pour aller plus loin : [Partager la capacité](../playback/transcoding.md#sharing-the-capacity).

## Mes films ou mon historique sortent-ils de la maison ?

Non. Les applications parlent à votre serveur Evey et à rien que le projet
exploite. Ce que vous parcourez, lisez et demandez reste sur ce serveur, et qui
peut le lire dépend de la personne qui l'administre. Les exceptions sont
celles auxquelles on s'attend : une jaquette récupérée à sa source publique,
un Chromecast qui charge la page du récepteur, un lien que vous ouvrez
vous-même.

Pour aller plus loin : [Règles de confidentialité](../privacy.md).

## Comment mettre à jour sans rien casser ?

Pour la personne qui l'administre, dans le dossier qui contient le fichier
compose :

```bash
docker compose pull && docker compose up -d
```

C'est toute la mise à jour. Quand la nouvelle version doit modifier la base de
données, elle en enregistre d'abord une copie dans `./evey-data/backups/`.
Evey signale dans le panneau d'administration qu'une version plus récente
existe ; il ne se met jamais à jour tout seul.

Pour aller plus loin : [Mettre à jour Evey](../backups.md#updating-evey).

## J'ai oublié mon mot de passe

Si la page de connexion affiche **Mot de passe oublié ?**, utilisez-le : un
lien de réinitialisation vous est envoyé par e-mail. Ce lien n'est là que si
la personne qui fait tourner Evey a activé la **Récupération du mot de passe**
et configuré l'e-mail. Sinon, demandez-lui : dans **Administration →
Utilisateurs**, **Modifier l'utilisateur** a un champ **Nouveau mot de passe**.
En dernier recours, quand c'est le mot de passe de l'administrateur lui-même
qui est perdu, `evey admin reset-password` en affiche un nouveau sur le
serveur.

Pour aller plus loin : [E-mail et notifications](../notifications.md) et
[Donner un nouveau mot de passe à un utilisateur](../runbooks/recovery.md#give-a-user-a-new-password).
