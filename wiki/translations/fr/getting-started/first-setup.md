---
source_sha: 48225cad93d795d54f36c6e4db0c828700357d5e317bec3f404927fba2c1cdf4
reviewed: true
---
# Première configuration { #first-setup }

Evey tourne, et vous avez ouvert son adresse dans un navigateur, par exemple
`http://192.168.1.20:9091` (vous ne savez pas quelle adresse :
[étape 3 de l'installation](../installation.md#the-address-to-open-next)).

Il reste quatre étapes avant que votre foyer puisse regarder :

1. [L'assistant de configuration](#1-the-setup-wizard) : environ une minute.
2. [Ajouter vos bibliothèques](#2-add-your-libraries) : dire à Evey où sont
   vos médias.
3. [Inviter votre foyer](#3-invite-your-household) : un compte par personne, ou
   un seul compte partagé avec un profil par personne.
4. [Connecter les applications](#4-connect-the-apps) : téléphones, tablettes
   et TV.

## 1. L'assistant de configuration { #1-the-setup-wizard }

Tant que le serveur n'a pas d'administrateur, son adresse ouvre toujours
l'assistant. En haut, il affiche **Étape 1 sur 4 - Langue**, et quatre points
qui se remplissent au fur et à mesure.

### Étape 1 sur 4 - Langue { #step-1-of-4---language }

La carte affiche **Choisissez la langue du serveur**. Choisissez :

- **Langue** : la langue par défaut des nouveaux profils, et des titres et
  résumés qu'Evey va chercher. L'assistant passe tout de suite dans cette
  langue.
- **Région** : remplie d'après la langue. Elle décide quelles classifications
  d'âge sont affichées. **Automatique** est proposé.

Vous pourrez changer les deux plus tard dans le panneau d'administration.
Appuyez sur **Continuer**.

### Étape 2 sur 4 - Bienvenue { #step-2-of-4---welcome }

Rien à remplir. Appuyez sur **Commencer**.

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![Étape 1 de l'assistant : langue et région](../images/setup-wizard-1-language.webp){ loading=lazy data-gallery="setup" }
  <figcaption>Étape 1 : la langue du serveur, et la région utilisée pour les classifications d'âge (interface en anglais).</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Étape 2 de l'assistant : bienvenue](../images/setup-wizard-2-welcome.webp){ loading=lazy data-gallery="setup" }
  <figcaption>Étape 2 : rien à remplir.</figcaption>
</figure>

</div>

### Étape 3 sur 4 - Compte administrateur { #step-3-of-4---admin-account }

La carte affiche **Créer un compte administrateur**. C'est votre propre
compte, celui qui gère le serveur.

| Champ | Ce qu'il faut saisir |
| --- | --- |
| **Nom d'utilisateur** | Au moins 3 caractères. |
| **Mot de passe** | Au moins 8 caractères. |
| **Confirmer le mot de passe** | Le même mot de passe une deuxième fois. |
| **Nom du serveur** | Facultatif. Un nom comme « Salon ». On le voit sur la page de connexion, dans l'onglet du navigateur, dans la liste des serveurs des applications et dans les e-mails. Vous pourrez le changer plus tard dans **Administration → Configuration du serveur → Nom du serveur**. |

Appuyez sur **Continuer**. Evey crée le compte et vous connecte. L'assistant
ne s'affichera plus.

### Étape 4 sur 4 - Première bibliothèque { #step-4-of-4---first-library }

La carte affiche **Ajoutez votre première bibliothèque**. Une bibliothèque,
c'est un type de média rangé dans un ou plusieurs dossiers, par exemple
« Films ».

- **Nom de la bibliothèque** : par exemple « Films ».
- **Type de bibliothèque** : **Films**, **Séries**, **Musique** ou **Photos**.
- **Chemins** : choisissez un dossier dans la liste. Elle montre les dossiers
  qu'Evey peut voir, en commençant par `/media`. Pour saisir un chemin comme
  `/media/movies`, choisissez **Entrer un chemin personnalisé...**.

Appuyez sur **Créer** : Evey commence à lire le dossier. Vous pouvez aussi
appuyer sur **Passer** et ajouter des bibliothèques plus tard. Dans les deux
cas, vous arrivez sur la page d'accueil.

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![Étape 3 de l'assistant : le compte administrateur](../images/setup-wizard-3-admin.webp){ loading=lazy data-gallery="setup" }
  <figcaption>Étape 3 : le compte administrateur ; le nom du serveur est facultatif.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Étape 4 de l'assistant : la première bibliothèque](../images/setup-wizard-4-library.webp){ loading=lazy data-gallery="setup" }
  <figcaption>Étape 4 : un nom, un type, et un dossier tel qu'Evey le voit.</figcaption>
</figure>

</div>

??? note "Si l'assistant n'apparaît pas, ou apparaît deux fois"

    Dès qu'un administrateur existe, l'adresse ouvre la page de connexion, et
    `/setup` vous y renvoie aussi. Si deux personnes ouvrent l'assistant en
    même temps, seule la première obtient le compte administrateur : la
    seconde est renvoyée vers la page de connexion.

<figure class="evey-shot" markdown="span">
  ![Le tableau de bord d'administration du serveur de démo](../images/admin-dashboard.webp){ loading=lazy }
  <figcaption>Le panneau d'administration une fois connecté : le tableau de bord, et toutes les sections dans le menu latéral (interface en anglais).</figcaption>
</figure>

## 2. Ajouter vos bibliothèques { #2-add-your-libraries }

Allez dans **Administration → Bibliothèques → Ajouter une bibliothèque**.
Donnez un nom, un type (Films, Séries, Musique ou Photos) et un ou plusieurs
dossiers.

Les dossiers portent le nom qu'ils ont pour Evey, pas celui qu'ils ont sur
votre ordinateur. Avec le fichier compose de
l'[installation](../installation.md#the-compose-file), votre dossier de médias
s'appelle `/media`. Un dossier `/srv/media/films` sur votre ordinateur devient
donc `/media/films` dans Evey.

Evey commence automatiquement à lire le dossier. Les affiches, les résumés et la
distribution arrivent en arrière-plan : une grosse bibliothèque se remplit en
quelques minutes. Les nouveaux fichiers que vous ajoutez plus tard
apparaissent d'eux-mêmes.

Vos titres n'apparaissent pas, ou ont la mauvaise affiche ? Voir
[Bibliothèques et métadonnées](../administration/libraries.md) : comment
nommer les fichiers, et comment corriger un titre mal reconnu.

## 3. Inviter votre foyer { #3-invite-your-household }

Il y a deux façons de donner un compte à quelqu'un, toutes les deux dans
**Administration → Utilisateurs**.

**Envoyer un lien d'invitation** (la personne choisit son mot de passe) :

1. Ouvrez le panneau d'administration depuis l'adresse que les autres
   utiliseront, par exemple `http://192.168.1.20:9091`, et non
   `http://localhost:9091`. Le lien est construit à partir de l'adresse de
   votre navigateur.
2. Appuyez sur **Créer un lien d'invitation**. Vous pouvez limiter sa durée
   (**Date d'expiration (optionnel)**), le nombre de fois où il peut servir
   (**Nombre max d'utilisations (vide = illimité)**) et les bibliothèques que
   le nouveau compte voit (**Bibliothèques accessibles**).
3. Dans **Liens d'invitation**, appuyez sur **Copier le lien** et envoyez-le
   par message ou par e-mail.
4. La personne ouvre le lien et voit **Vous avez été invité ! Remplissez vos
   informations ci-dessous.** Elle choisit un nom d'utilisateur et un mot de
   passe (au moins 8 caractères), et elle est connectée.

**Ou créer le compte vous-même** : appuyez sur **Créer un utilisateur**,
choisissez le **Nom d'utilisateur** et le **Mot de passe**, puis donnez-les à
la personne.

Chaque compte commence avec un profil, qui porte le nom d'utilisateur. Un
foyer peut partager un seul compte, avec un profil par personne : ajoutez-les
depuis **Paramètres → Profils → Créer un profil**. Pour un enfant, cochez
**Profil enfant** et choisissez un **Âge maximum**. Tous les autres profils de
ce compte demandent alors un **Code PIN (4 chiffres)**, pour que l'enfant ne
puisse pas y passer.

Envoyez ensuite trois choses à chaque personne : l'adresse, son nom
d'utilisateur et son mot de passe (ou le lien d'invitation), et la page écrite
pour elle, [Regarder avec Evey](watching.md).

Plus d'options (limites par compte, horaires autorisés, codes PIN, limites
d'âge) :
[Utilisateurs, profils et contrôle parental](../administration/users.md).

## 4. Connecter les applications { #4-connect-the-apps }

- **Dans un navigateur**, il n'y a rien à installer : c'est l'adresse que
  vous avez déjà ouverte.
- **Sur un téléphone, une tablette ou une TV Android**, l'application demande
  d'abord l'adresse du serveur. Saisissez la **même adresse que dans le
  navigateur**, avec `http://` et le port, par exemple
  `http://192.168.1.20:9091`. Connectez-vous ensuite.
- **Sur une TV**, vous pouvez vous connecter avec un code que vous acceptez
  depuis votre téléphone, au lieu de saisir un mot de passe avec la
  télécommande.

Pas à pas, et ce que veut dire chaque message d'erreur :
[Connecter les applications](apps.md#the-server-address-to-enter).

## Ce que le serveur fait de lui-même { #what-the-server-does-by-itself }

Vous n'avez rien de tout cela à régler :

- **Comment chaque appareil lit un fichier.** Chaque appareil indique au serveur
  ce qu'il sait lire. Pour chaque titre, le serveur envoie alors le fichier
  tel quel, le remet dans un autre format de fichier, ou convertit seulement
  ce que l'appareil ne sait pas lire. Voir
  [Comment Evey choisit le mode de lecture](../playback/how-playback-is-decided.md).
- **Combien de personnes il peut servir.** Dès son premier moment calme, le
  serveur mesure combien de vidéos il peut convertir en même temps, avec le
  processeur et la carte graphique s'il y en a une, et n'en accepte jamais
  plus. Cela prend quelques minutes. Pendant ce temps, une lecture qui a
  besoin d'une conversion doit attendre. Voir
  [Transcodage et capacité du serveur](../playback/transcoding.md).
- **L'accès depuis l'extérieur de chez vous.** Fermé tant que vous ne donnez
  pas d'adresse publique au serveur. Voir
  [Accès distant et HTTPS](../administration/remote-access.md).
- **Une copie de secours de la base de données** avant chaque mise à jour qui
  la modifie. Des sauvegardes régulières ne demandent qu'un réglage :
  [Sauvegardes et mises à jour](../backups.md).

## Avant de partager l'adresse hors de chez vous { #before-you-share-the-address-outside-your-home }

1. Placez le serveur derrière du HTTPS. Evey lui-même ne fournit que du HTTP
   simple : cela convient sur le réseau domestique, pas sur Internet.
   [Accès distant et HTTPS](../administration/remote-access.md).
2. Programmez des sauvegardes, et gardez une copie du dossier `evey-data`. Il
   contient la base de données et la clé qui protège toutes les connexions.
   [Sauvegardes et mises à jour](../backups.md).
