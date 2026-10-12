---
source_sha: 6f3e4c0be097222fac2048b142e37293374416b753471a96fc1f3936ac437109
reviewed: true
---
# Connecter les applications

Votre téléphone ou votre TV vous demande l'adresse de votre serveur Evey ?
Cette page vous dit quoi taper, et quoi faire quand ça ne marche pas.

Chaque application se connecte à votre propre serveur Evey, à la maison. Vous
tapez son adresse une seule fois, vous vous connectez, puis vous choisissez
votre profil. L'endroit où vous vous êtes arrêté, ce que vous avez regardé et
vos favoris sont gardés sur le serveur : ils vous suivent d'un appareil à
l'autre.

- **Navigateur web** : rien à installer. Ouvrez l'adresse du serveur. Le
  panneau d'administration est là aussi, pour les administrateurs.
- **Android** (téléphone, tablette) et **Android TV** : les applications
  Evey. Elles sont en test et pas encore sur Google Play pour tout le monde.
- **Chromecast** : pas d'application. Vous castez depuis le lecteur web dans
  Chrome, ou depuis l'application Android.

## L'adresse du serveur à saisir

**Demandez l'adresse à la personne qui fait tourner Evey.** Elle ressemble à
ceci :

```text
http://192.168.1.20:9091
```

Ensuite, dans l'application Android ou Android TV :

1. Le premier écran affiche **Connexion au serveur**. Tapez l'adresse dans le
   champ **URL du serveur**, exactement comme on vous l'a donnée : avec
   `http://` au début et `:9091` (ou un autre nombre) à la fin.
2. Appuyez sur **Connexion**.
3. Connectez-vous avec votre nom d'utilisateur et votre mot de passe. Sur une
   TV, vous pouvez utiliser la
   [Connexion rapide](#quick-connect-signing-in-a-tv-without-typing-a-password)
   à la place.
4. Choisissez votre profil.

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![L'application Android demande l'adresse du serveur](../images/app-android-connect.webp){ loading=lazy data-gallery="connect" }
  <figcaption>L'application Android : l'adresse du serveur à la maison, avec http:// et le port (interface en anglais).</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![L'application Android TV demande l'adresse du serveur](../images/app-tv-connect.webp){ loading=lazy data-gallery="connect" }
  <figcaption>L'application Android TV : la même adresse, tapée avec la télécommande ou trouvée par le scan.</figcaption>
</figure>

</div>

L'adresse dépend de l'endroit où vous êtes :

| Où vous êtes | Quoi taper |
| --- | --- |
| À la maison, sur le même Wi-Fi que le serveur | L'adresse de la maison, par exemple `http://192.168.1.20:9091`. Un nom marche aussi si votre box ou votre routeur le connaît, par exemple `http://nas:9091`. |
| Hors de chez vous | L'adresse publique, qui commence par `https://` et n'a en général pas de port, par exemple `https://evey.example.org`. Elle n'existe que si la personne qui fait tourner Evey l'a mise en place. |

Ne tapez rien après le port : pas de `/api`, pas d'autre chemin. Un `/` à la
fin est retiré pour vous.

??? info "Pour la personne qui fait tourner le serveur : d'où vient l'adresse"

    - **L'IP** est celle de l'ordinateur ou du NAS qui fait tourner Evey, sur
      le réseau de la maison. Vous la trouvez dans la liste des appareils de
      votre box ou de votre routeur, ou sur la machine elle-même (`ip a` sous
      Linux, `ipconfig` sous Windows, `ifconfig` sous macOS). Ce n'est jamais
      l'IP du téléphone.
    - **Le port** est le nombre de gauche de la ligne `ports:` de votre
      fichier compose : `9091` avec le fichier de
      l'[installation](../installation.md#the-compose-file).
    - **`http://`** est normal à la maison. `https://` ne sert que pour
      l'adresse publique, réglée dans **Administration → Accès distant → URL
      externe** : voir [Accès distant et HTTPS](../administration/remote-access.md).

Tant qu'aucun serveur n'est enregistré, l'écran de connexion explique aussi
ce qui est demandé (**Evey tourne sur un serveur que quelqu'un a installé pour
vous. Entrez son adresse web, celle que vous ouvririez dans un
navigateur.**) et propose **Où trouver l'adresse de mon serveur ?**, qui mène
à cette page. Le téléphone l'ouvre dans le navigateur ; la TV affiche
l'adresse de la page à l'écran, pour l'ouvrir sur un autre appareil.

### Scanner le réseau local : ce qu'il trouve et ce qu'il ne trouve pas

**Scanner le réseau local**, sous le champ, cherche pour vous un serveur Evey
à la maison. Cela peut prendre jusqu'à deux minutes environ. Chaque serveur
trouvé s'affiche avec son nom, son adresse et sa version. Touchez-en un pour
remplir le champ, puis appuyez sur **Connexion**. Si rien n'est trouvé,
l'écran affiche **Aucun serveur trouvé sur ce réseau - saisissez l'adresse
ci-dessous** : tapez l'adresse vous-même.

Le scan ne trouve pas le serveur quand le téléphone est en données mobiles,
sur un Wi-Fi invité, ou sur un autre réseau que le serveur.

??? info "Ce que le scan essaie exactement"

    Il essaie `http://` sur chaque adresse de quatre réseaux domestiques
    courants (`192.168.1.x`, `192.168.0.x`, `10.0.0.x` et `172.16.0.x`,
    hôtes 1 à 254), sur les ports `9091`, `8096` et `8920`, cinquante à la
    fois, avec 2 secondes d'attente pour chacun. Il rate donc aussi un
    serveur sur une autre plage d'adresses (`192.168.2.x`, `10.1.x.x`...),
    sur un autre port, ou joignable seulement en HTTPS. Les applications
    n'utilisent ni mDNS ni Bonjour, et ne lisent pas la plage d'adresses du
    téléphone : ces quatre plages sont fixes.

### Quand la connexion échoue

Chaque échec ouvre une fenêtre titrée **Connexion echouee**. Cherchez le
message dans ce tableau (certains messages de l'application Android
s'affichent encore en anglais) :

| Message (Android) | Ce que ça veut dire | Quoi faire |
| --- | --- | --- |
| **Commencez l'adresse par http:// ou https://.** | L'adresse n'a pas de `http://`. | Tapez `http://` devant l'adresse. |
| **Cette connexion n'est pas chiffrée** | Vous avez tapé `http://` pour une adresse hors de chez vous. Votre mot de passe et la vidéo traverseraient Internet sans protection. | Appuyez sur **Annuler** et utilisez l'adresse publique en `https://`. **Se connecter quand même** est là si vous savez ce que vous faites. |
| **No response from ... within 8s.** | Rien ne répond à cette adresse. | Vérifiez que le serveur est allumé, que l'adresse et le port sont bons, et que le téléphone est sur le même Wi-Fi. |
| **Couldn't reach ...** | Mauvaise adresse ou mauvais port, `http` et `https` inversés, ou autre réseau. | Mêmes vérifications. À la maison, essayez `http://` au lieu de `https://` ; pour une adresse publique, l'inverse. |
| **TLS/certificate problem reaching ...** | Le certificat HTTPS du serveur n'est pas accepté. | À la maison, utilisez `http://`. Pour une adresse publique, le serveur a besoin d'un vrai certificat. |
| **... answered "HTTP 404 ..." instead of a healthy Evey server.** | Quelque chose répond, mais ce n'est pas Evey : il y a un chemin après le port, ou un autre programme utilise ce port. | Enlevez tout ce qui suit le port, et vérifiez le numéro du port. |
| **... a répondu, mais ce n'est pas un serveur Evey.** | Une autre page a répondu : une page de connexion au Wi-Fi, la page de votre fournisseur d'accès, ou un autre programme. | Vérifiez l'adresse. Sur un Wi-Fi public, connectez-vous d'abord à sa page de connexion. |
| **Acces distant desactive** | Vous êtes hors de chez vous, et le serveur n'accepte pas les connexions de l'extérieur. | Demandez à la personne qui fait tourner Evey. Elle l'active dans **Administration → Accès distant** : [Accès distant et HTTPS](../administration/remote-access.md). |

Sur Android TV, les messages sont plus courts : **Server returned 404** quand
autre chose qu'Evey répond, et l'erreur réseau brute quand rien ne répond.
Les causes et les vérifications sont les mêmes. La TV a aussi besoin du
`http://` (un `192.168.1.20:9091` tout seul échoue), mais elle ne prévient
pas quand vous tapez `http://` pour une adresse publique.

Si le serveur ne répond pas quand vous rouvrez l'application plus tard,
l'application revient sur cet écran et affiche le serveur enregistré. Sur la
TV, elle ajoute **Votre serveur n'a pas répondu. Il est peut-être en veille ou
hors du réseau.** Quand le serveur est rallumé, choisissez-le à nouveau.

### Changer de serveur ou en ajouter un plus tard

Les deux applications peuvent retenir plusieurs serveurs. L'écran de
connexion les affiche sous **Vos serveurs**, avec celui en cours marqué
**Actuel** (Android) ou **Actif** (Android TV), et **Ajouter un autre
serveur** en dessous. Touchez un serveur enregistré pour vous y reconnecter
sans retaper l'adresse. Chaque serveur garde sa propre connexion.

- **Android.** Sur l'écran de connexion au serveur, chaque serveur enregistré
  a un bouton pour le supprimer (vous confirmez, et sa connexion est
  effacée). L'écran où l'on se connecte avec son mot de passe propose
  **Changer de serveur**. Une fois connecté, **Paramètres → Serveurs** ouvre
  **Gerer les serveurs** : touchez un serveur pour passer dessus, ou
  utilisez **Renommer** ou **Supprimer**. Vous ne pouvez pas supprimer le
  seul serveur enregistré.
- **Android TV.** **Paramètres → Changer de serveur** déconnecte la TV et
  revient à l'écran de connexion au serveur ; l'écran où l'on se connecte
  avec son mot de passe a le même bouton. Sur l'écran de connexion au
  serveur, **Gerer les serveurs** renomme ou supprime un serveur enregistré.

## Connexion rapide : connecter une TV sans taper de mot de passe

Taper un mot de passe avec une télécommande, c'est long. La Connexion rapide
vous permet plutôt d'accepter la TV depuis votre téléphone ou votre
ordinateur.

1. Sur l'écran de connexion de la TV, choisissez **Connexion rapide** (marqué
   **Recommandé pour la TV**). La TV affiche un code de 6 caractères, un QR
   code, et la durée pendant laquelle le code reste valable.
2. Acceptez le code depuis un appareil où vous êtes déjà connecté, d'une de
   ces façons :
    - **Dans un navigateur** : **Paramètres → Connexion rapide**, tapez le
      code dans **Code de l'appareil**, appuyez sur **Autoriser
      l'appareil**.
    - **Dans l'application Android** : **Paramètres → Connexion rapide**, puis
      **Scanner le QR code** avec l'appareil photo, ou tapez le code et
      appuyez sur **Autoriser l'appareil**.
    - **Avec l'appareil photo du téléphone** : scannez le QR code. Il ouvre
      la page Connexion rapide du serveur dans le navigateur. Si vous y êtes
      connecté, le code est déjà rempli ; sinon, connectez-vous d'abord, puis
      tapez le code.
3. En quelques secondes, la TV est connectée au même compte.

L'écran de connexion de l'application Android propose aussi la Connexion
rapide, pour un téléphone sur lequel vous préférez ne pas taper votre mot de
passe.

## Ce que chaque application ajoute

- **Web.** Des raccourcis clavier dans le lecteur (appuyez sur `?` pour les
  voir), des notifications dans le navigateur (voir
  [Notifications](../notifications.md)), et le panneau d'administration.
- **Android.**
    - Les **Téléchargements**, pour regarder sans connexion. Un titre
      téléchargé reste lisible pendant 30 jours sans contacter le serveur.
      Il ne s'ouvre que pour les profils qui ont le droit de le voir : un
      profil enfant ne peut pas ouvrir un film téléchargé par un autre profil
      s'il dépasse son âge maximum. Quand l'administrateur l'active,
      l'application propose aussi une copie plus légère, préparée par le
      serveur, pour un film trop gros pour le téléphone
      (`downloads.quality_choice`, voir
      [Diffusion de la lecture et téléchargements](../environment/playback-delivery.md)).
    - **Caster** vers un Chromecast.
- **Android TV.** Un écran pensé pour la télécommande.

**Lire sur**, dans les lecteurs web et Android, envoie ce que vous regardez
vers une autre application Evey ouverte et connectée au même compte. La
lecture continue là-bas.

## Garder les applications à jour

Quand votre application Android ou Android TV est plus ancienne que la
version qui va avec votre serveur, elle vous le dit :

- l'application Android affiche **Mise à jour disponible :** et la version en
  haut de l'écran (jamais par-dessus la vidéo) ;
- la TV l'affiche dans sa barre du haut.

Appuyez dessus pour ouvrir la page de l'application sur le Play Store. Le
Play Store ne met pas toujours les applications à jour tout seul (seulement
quand l'appareil ne sert pas et qu'il est en Wi-Fi, et cela peut être coupé) :
c'est pour ça que cet avis existe. Quand l'application est à jour, ou que le
serveur est plus ancien que cette fonction, rien ne s'affiche. L'application
web n'a jamais besoin de mise à jour : c'est le serveur qui la fournit.

## Quel appareil lit quoi

Chaque application dit au serveur ce que son appareil sait lire. Pour chaque
titre, le serveur décide alors d'envoyer le fichier tel quel ou de l'adapter.
Le même film peut donc partir intact vers une TV et être converti pour un
vieux téléphone. Le panneau **Infos du flux** du lecteur montre ce qui a été
décidé et pourquoi : voir
[Comment Evey choisit le mode de lecture](../playback/how-playback-is-decided.md).
Caster suit ses propres règles : voir
[Diffusion (Chromecast)](../playback/casting.md).
