---
source_sha: 86d75370f23016908bec56fe4be6926159cb164b39e15dfd088310fdafc75c5d
reviewed: true
---
# Première configuration

Le serveur tourne et vous avez ouvert `http://<adresse-du-serveur>:9091` dans
un navigateur (quelle adresse :
[L'adresse à ouvrir ensuite](../installation.md#the-address-to-open-next)).
Il reste quatre étapes avant de pouvoir regarder quoi que ce soit :
l'assistant de configuration, les bibliothèques, les utilisateurs et les
applications.

## 1. L'assistant de configuration

Tant que le serveur n'a pas d'administrateur, toute adresse sur ce serveur
s'ouvre sur l'assistant : une seule carte avec le logo Evey, une ligne
**Étape 1 sur 4 - Langue** et quatre points qui se remplissent au fil des
étapes. Il ne pose que quatre questions et prend une minute.

### Étape 1 sur 4 - Langue

La carte est titrée **Choisissez la langue du serveur**. Deux listes
déroulantes :

- **Langue** : devient la langue par défaut des nouveaux profils et des
  métadonnées que le serveur récupère (titres, résumés, genres). L'assistant
  lui-même passe aussitôt dans cette langue.
- **Région** : pré-remplie d'après la langue ; elle sert aux classifications
  de contenu et aux préférences régionales. **Automatique** est proposé.

Les deux se modifient ensuite dans le panneau d'administration. Appuyez sur
**Continuer**.

### Étape 2 sur 4 - Bienvenue

**Bienvenue sur Evey** et une phrase. Appuyez sur **Commencer** (ou
**Retour**).

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

### Étape 3 sur 4 - Compte administrateur

La carte est titrée **Créer un compte administrateur** :

| Champ | Règle |
| --- | --- |
| **Nom d'utilisateur** | Au moins 3 caractères. |
| **Mot de passe** | Au moins 8 caractères. |
| **Confirmer le mot de passe** | Doit être identique. |
| **Nom du serveur** | Facultatif. Affiché sur la page de connexion, dans l'onglet du navigateur, dans la liste des serveurs des applications et dans les e-mails, par exemple « Salon ». Modifiable ensuite dans **Admin → Configuration du serveur → Nom du serveur**. |

Appuyez sur **Continuer**. Le compte est créé et vous êtes connecté : à
partir de là, le serveur a un administrateur et l'assistant ne sera plus
proposé.

### Étape 4 sur 4 - Première bibliothèque

La carte est titrée **Ajoutez votre première bibliothèque** :

- **Nom de la bibliothèque**, par exemple « Films ».
- **Type de bibliothèque** : **Films**, **Séries**, **Musique** ou
  **Photos**.
- **Chemins** : une liste déroulante des dossiers montés dans le conteneur en
  `/media`, `/media2`... ; choisissez-en un, ou **Entrer un chemin
  personnalisé...** pour taper un chemin tel que `/media/movies`. Quand aucun
  dossier `/media` n'est monté, la liste est remplacée par un champ texte.

Appuyez sur **Créer** pour lancer le premier scan, ou sur **Passer** : les
deux mènent à la page d'accueil. Les bibliothèques s'ajoutent à tout moment
depuis le panneau d'administration (section suivante).

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![Étape 3 de l'assistant : le compte administrateur](../images/setup-wizard-3-admin.webp){ loading=lazy data-gallery="setup" }
  <figcaption>Étape 3 : le compte administrateur ; le nom du serveur est facultatif.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![Étape 4 de l'assistant : la première bibliothèque](../images/setup-wizard-4-library.webp){ loading=lazy data-gallery="setup" }
  <figcaption>Étape 4 : un nom, un type, et un dossier vu depuis l'intérieur du conteneur.</figcaption>
</figure>

</div>

??? note "Ouvrir l'adresse une fois l'administrateur créé"

    La même adresse s'ouvre sur la page de connexion, et `/setup` vous y
    renvoie aussi. Si deux personnes arrivent sur l'assistant en même temps,
    le compte de la seconde est refusé et son navigateur passe à la page de
    connexion.

<figure class="evey-shot" markdown="span">
  ![Le tableau de bord d'administration du serveur de démo](../images/admin-dashboard.webp){ loading=lazy }
  <figcaption>L'administration une fois connecté : le tableau de bord, et chaque section dans le menu latéral. (interface en anglais).</figcaption>
</figure>

## 2. Ajouter vos bibliothèques

**Admin → Bibliothèques → Ajouter une bibliothèque** : un nom, un type
(Films, Séries, Musique ou Photos) et un ou plusieurs dossiers.

Les dossiers sont des chemins **à l'intérieur du conteneur**. Avec le fichier
compose de l'installation, votre dossier de médias est `/media` : un dossier
`/srv/media/films` sur la machine devient `/media/films` pour Evey.

Le premier scan démarre tout seul. Les nouveaux fichiers sont pris en compte
dès leur apparition, et les affiches, résumés et distributions arrivent en
arrière-plan : une grosse bibliothèque se remplit en quelques minutes.

Nommage des fichiers, fonctionnement des scans, correction d'une mauvaise
identification : [Bibliothèques et métadonnées](../administration/libraries.md).

## 3. Ajouter les personnes

**Admin → Utilisateurs** crée un compte avec un mot de passe que vous
choisissez, ou un lien d'invitation avec lequel la personne choisit le sien.
Chaque compte crée ensuite ses profils : un par personne du foyer, profils
enfant compris.

Ce qu'un administrateur peut restreindre par compte, et le fonctionnement
des profils et du contrôle parental :
[Utilisateurs, profils et contrôle parental](../administration/users.md).
La page à envoyer aux personnes que vous invitez, écrite pour elles :
[Regarder avec Evey](watching.md).

## 4. Connecter les applications

L'application web est l'adresse que vous avez déjà ouverte. Les applications
Android et Android TV s'ouvrent sur un écran qui demande l'adresse du
serveur : tapez la **même adresse que dans le navigateur**, avec `http://` et
le port, par exemple `http://192.168.1.20:9091`, puis connectez-vous. Une TV
peut se connecter avec un code validé depuis un téléphone plutôt qu'avec un
mot de passe.

Comment trouver l'adresse, ce que fait le scan automatique, et ce que
signifie chaque message d'erreur :
[L'adresse du serveur à saisir](apps.md#the-server-address-to-enter).

## Ce que le serveur fait tout seul

Rien de ceci ne demande de réglage :

- **La lecture.** Chaque appareil indique ce qu'il sait décoder, et le
  serveur choisit, titre par titre, entre envoyer le fichier tel quel, le
  remuxer, ou réencoder uniquement ce que l'appareil ne sait pas lire. Voir
  [Comment Evey choisit le mode de lecture](../playback/how-playback-is-decided.md).
- **La capacité.** Dès qu'il est au repos, le serveur mesure combien de
  vidéos il peut transcoder simultanément, avec le processeur et avec toute
  carte graphique donnée au conteneur, et n'en accepte jamais davantage. La
  mesure prend quelques minutes ; une lecture qui a besoin d'un transcodage
  pendant ce temps est invitée à patienter. Voir
  [Transcodage et capacité du serveur](../playback/transcoding.md).
- **L'accès distant.** Il s'ouvre quand le serveur a reçu une adresse
  publique, et reste fermé sinon. Voir
  [Accès distant et HTTPS](../administration/remote-access.md).
- **Les sauvegardes.** La base est sauvegardée avant chaque mise à jour qui
  la modifie. Une planification régulière se règle en un écran :
  [Sauvegardes et mises à jour](../backups.md).

## Avant de partager l'adresse hors de chez vous

1. Placez le serveur derrière du HTTPS : Evey sert du HTTP simple.
   [Accès distant et HTTPS](../administration/remote-access.md).
2. Planifiez des sauvegardes, et gardez une copie de `./evey-data` : ce
   dossier contient la base et la clé qui signe toutes les sessions.
   [Sauvegardes et mises à jour](../backups.md).
