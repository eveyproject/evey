---
source_sha: 8596400054bc5784256222632f8b0b2125ca7a2b8d0fb1cb21fe8fa25398c6f0
reviewed: true
---
# Applications et appareils

Chaque application dialogue avec votre propre serveur. Vous lui indiquez
l'adresse du serveur une fois, vous vous connectez, puis vous choisissez un
profil ; les points de reprise, l'historique et les favoris sont stockés sur
le serveur et vous suivent d'un appareil à l'autre. Cette page s'adresse à la
personne qui installe les applications sur les téléphones et les TV du
foyer : l'adresse à saisir est la première section.

| Application | Provenance | Connexion |
| --- | --- | --- |
| **Web** | Servie par Evey lui-même, à l'adresse du serveur. Rien à installer. | Nom d'utilisateur et mot de passe, ou authentification unique si elle est configurée. |
| **Android** (téléphone, tablette) | L'application Android. | Adresse du serveur, puis nom d'utilisateur et mot de passe. |
| **Android TV** | L'application Android TV. | Adresse du serveur, puis **Connexion rapide** (ci-dessous) ou nom d'utilisateur et mot de passe. |
| **Chromecast** | Pas d'application : on caste depuis l'application web dans Chrome ou depuis l'application Android. | Utilise la session de l'appareil qui caste. |

Le panneau d'administration fait partie de l'application web, pour les
comptes administrateurs.

## L'adresse du serveur à saisir

Les applications Android et Android TV s'ouvrent sur un écran titré
**Connexion au serveur** : un champ (**URL du serveur**, exemple grisé
`https://your-server.com`), un bouton **Connexion** et un lien **Scanner le
réseau local**. Tant qu'aucun serveur n'est enregistré, une ligne au-dessus
du champ explique ce qui est demandé : **Evey tourne sur un serveur que
quelqu'un a installé pour vous. Entrez son adresse web, celle que vous
ouvririez dans un navigateur.**, suivie de **Où trouver l'adresse de mon
serveur ?**, qui mène à cette page (en français quand le téléphone ou la TV
est en français, en anglais sinon). Le téléphone l'ouvre dans le navigateur ;
la TV écrit son adresse à l'écran, à ouvrir sur un autre appareil.

Tapez la **même adresse que celle que vous ouvrez dans un navigateur sur le
même réseau**, avec le préfixe et le port :

```text
http://192.168.1.20:9091
```

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![L'application Android demande l'adresse du serveur](../images/app-android-connect.webp){ loading=lazy data-gallery="connect" }
  <figcaption>L'application Android : l'adresse du serveur à la maison, avec http:// et le port (interface en anglais).</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![L'application Android TV demande l'adresse du serveur](../images/app-tv-connect.webp){ loading=lazy data-gallery="connect" }
  <figcaption>L'application Android TV : la même adresse, tapée à la télécommande ou trouvée par le scan.</figcaption>
</figure>

</div>

| Où se trouve l'appareil | Quoi taper |
| --- | --- |
| À la maison, sur le même Wi-Fi que le serveur | `http://` + l'IP de la machine qui fait tourner Evey + `:9091`, par exemple `http://192.168.1.20:9091`. Son nom fonctionne aussi (`http://nas:9091`) quand votre box ou routeur résout les noms. |
| Hors de chez vous | L'adresse publique en HTTPS renseignée dans **Admin → Accès distant → URL externe**, par exemple `https://evey.example.org`, sans le port (le reverse proxy écoute sur 443). Voir [Accès distant et HTTPS](../administration/remote-access.md). |

D'où viennent les morceaux :

- **L'IP** est celle de l'ordinateur ou du NAS qui fait tourner Docker sur
  votre réseau domestique : dans la liste des appareils de votre box ou
  routeur, ou sur la machine elle-même (`ip a` sous Linux, `ipconfig` sous
  Windows, `ifconfig` sous macOS). Ce n'est jamais l'IP du téléphone.
- **Le port** est le nombre de gauche de la ligne `ports:` de votre fichier
  compose (`"9091:9091"`, voir
  [Installation](../installation.md#the-compose-file)). Avec le fichier de
  la page d'installation, c'est `9091`.
- **Le préfixe** est `http://` à la maison. Le HTTP simple sur le réseau
  domestique est la configuration normale ; `https://` est réservé à
  l'adresse publique.

Rien ne vient après le port : ni `/api`, ni chemin. Un `/` final est retiré.
Quand vous appuyez sur **Connexion**, l'application contacte le serveur,
retient l'adresse et passe à l'écran de connexion.

### Scanner le réseau local : ce qu'il trouve et ce qu'il ne trouve pas

**Scanner le réseau local** essaie `http://` sur chaque adresse de quatre
sous-réseaux domestiques courants (`192.168.1.x`, `192.168.0.x`, `10.0.0.x`
et `172.16.0.x`, hôtes 1 à 254) sur les ports `9091`, `8096` et `8920`,
cinquante à la fois avec un délai de 2 secondes chacun : un balayage complet
peut prendre jusqu'à deux minutes. Chaque serveur trouvé est listé avec son
nom (le **Nom du serveur** choisi à la configuration), son adresse et sa
version ; le toucher ne fait que remplir le champ, il reste à appuyer sur
**Connexion**. Quand rien ne répond, l'écran affiche **Aucun serveur trouvé
sur ce réseau - saisissez l'adresse ci-dessous**.

Le scan ne trouve pas un serveur :

- sur un autre sous-réseau (`192.168.2.x`, `10.1.x.x`...) ou sur un autre
  port ;
- quand le téléphone est en données mobiles, sur un Wi-Fi invité, ou sur un
  réseau différent de celui du serveur ;
- joignable uniquement en HTTPS.

Tapez l'adresse dans ces cas. Les applications n'utilisent ni mDNS ni
Bonjour et ne lisent pas la plage réseau du téléphone : les quatre
sous-réseaux ci-dessus sont fixes.

### Quand la connexion échoue

Chaque échec est une boîte de dialogue titrée **Connexion echouee**. Les
messages détaillés de l'application Android ne sont pas encore traduits et
s'affichent en anglais ; voici ce qu'ils signifient et quoi faire :

| Message (Android) | Cause | Quoi faire |
| --- | --- | --- |
| **Commencez l'adresse par http:// ou https://.** | Pas de préfixe. | Tapez `http://` devant l'IP. |
| **Cette connexion n'est pas chiffrée** | `http://` vers une adresse hors du réseau domestique. Le mot de passe et la vidéo traverseraient internet en clair. | **Annuler** et utiliser l'adresse publique en `https://`, ou **Se connecter quand même** en connaissance de cause. |
| **No response from ... within 8s.** | Rien ne répond à cette adresse. | Vérifiez que le serveur tourne (`docker compose ps`), l'IP et le port, et que le téléphone est sur le même Wi-Fi. |
| **Couldn't reach ...** | Hôte ou port faux, confusion `http`/`https`, ou réseau différent. | Mêmes vérifications ; essayez `http://` à la place de `https://` à la maison, et l'inverse pour une adresse publique. |
| **TLS/certificate problem reaching ...** | Un certificat auto-signé. | Utilisez `http://` à la maison, ou un certificat délivré par une autorité publique pour l'adresse publique. |
| **... answered "HTTP 404 ..." instead of a healthy Evey server.** | Quelque chose répond, mais pas Evey : un chemin après le port, ou un autre service sur ce port. | Retirez tout ce qui suit le port et vérifiez le numéro de port. |
| **... a répondu, mais ce n'est pas un serveur Evey.** | Un portail Wi-Fi, la page du FAI ou un autre service a répondu avec sa propre page. | Vérifiez l'adresse ; sur un Wi-Fi public, connectez-vous d'abord au portail. |
| **Acces distant desactive** | Le serveur est joint depuis l'extérieur alors que son accès distant est désactivé. | Un administrateur l'active dans **Admin → Accès distant** : [Accès distant et HTTPS](../administration/remote-access.md). |

Sur Android TV, les messages sont plus courts : **Server returned 404** pour
une réponse qui n'est pas celle d'Evey, et l'erreur réseau brute quand rien
ne répond ; les causes et les vérifications sont les mêmes. La TV ne prévient
pas d'un `http://` vers une adresse publique, et le préfixe y est tout aussi
obligatoire : un `192.168.1.20:9091` nu échoue.

Quand une application qui connaît déjà un serveur n'arrive pas à le joindre
au lancement, elle revient sur cet écran avec le serveur enregistré dans la
liste ; la TV ajoute **Votre serveur n'a pas répondu. Il est peut-être en
veille ou hors du réseau.** Choisissez à nouveau le serveur une fois qu'il
est démarré.

### Changer de serveur ou en ajouter un plus tard

Les deux applications gardent une liste de serveurs. L'écran de connexion au
serveur l'affiche au-dessus du champ, sous **Vos serveurs**, avec celui en
cours marqué **Actuel** (Android) ou **Actif** (Android TV), et **Ajouter un
autre serveur** en dessous. Toucher un serveur enregistré permet de s'y
reconnecter sans retaper l'adresse. Chaque serveur garde sa propre session.

- **Android.** Chaque serveur enregistré a un bouton de suppression sur
  l'écran de connexion au serveur : une adresse qui ne répond plus peut être
  retirée avant même de vous connecter, après confirmation, et sa session est effacée.
  L'écran de connexion propose **Changer de serveur**. Une fois connecté,
  **Paramètres → Serveurs** ouvre **Gerer les serveurs** : touchez un serveur
  pour basculer dessus, ou utilisez **Renommer** et **Supprimer**. Aux deux
  endroits, le seul serveur enregistré ne peut pas être supprimé.
- **Android TV.** **Paramètres → Changer de serveur** déconnecte la
  télévision et revient à l'écran de connexion au serveur ; l'écran de
  connexion a le même bouton. Sur l'écran de connexion au serveur, **Gerer
  les serveurs** renomme ou supprime un serveur enregistré.

## Connexion rapide : connecter une TV sans taper de mot de passe

Sur l'écran de connexion de la TV, choisissez **Connexion rapide** (marqué
**Recommandé pour la TV**). L'écran affiche un code de 6 caractères, un QR
code et la durée de validité du code. Validez-le depuis un appareil où vous
êtes déjà connecté :

- **Web** : **Paramètres → Connexion rapide**, tapez le code dans **Code de
  l'appareil** et appuyez sur **Autoriser l'appareil**.
- **Application Android** : **Paramètres → Connexion rapide**, puis
  **Scanner le QR code** avec la caméra, ou tapez le code et appuyez sur
  **Autoriser l'appareil**.
- **Appareil photo du téléphone** : scanner le QR code ouvre la page
  Connexion rapide du serveur dans le navigateur. Connecté là, le code est
  déjà rempli ; sinon connectez-vous d'abord et tapez-le.

La TV voit la validation en quelques secondes et se retrouve connectée au
même compte. L'écran de connexion de l'application Android propose la
Connexion rapide de la même manière, pour un téléphone sur lequel vous
préférez ne pas taper de mot de passe.

## Ce que propose chaque application

- **Web.** Des raccourcis clavier dans le lecteur (`?` les affiche), les
  notifications push du navigateur (voir [Notifications](../notifications.md)),
  et le panneau d'administration.
- **Android.**
  - Les **téléchargements** pour regarder hors connexion. Si l'administrateur
    l'a activée (`downloads.quality_choice`, voir
    [Diffusion de la lecture et téléchargements](../environment/playback-delivery.md)),
    l'application propose une copie plus légère préparée par le serveur pour
    un film trop volumineux pour le téléphone. Un téléchargement reste lisible hors
    connexion pendant 30 jours sans contacter le serveur, et seulement pour
    les profils autorisés à le voir : un profil enfant ne peut pas ouvrir un
    film téléchargé par un autre profil au-dessus de son âge maximum.
  - La **diffusion** vers un Chromecast.
- **Android TV.** Une interface pensée pour la télécommande.

**Lire sur**, dans les lecteurs web et Android, envoie ce que vous regardez
vers une autre application Evey ouverte sur le même compte, et la lecture se
poursuit là-bas.

## Garder les applications à jour

Chaque version du serveur indique les versions des applications Android et
Android TV publiées avec elle. Quand l'application installée est plus
ancienne, l'application Android affiche **Mise à jour disponible :** suivi du
numéro de version en haut de l'écran (jamais par-dessus le lecteur), et la TV
l'affiche dans sa barre du haut. Appuyer dessus ouvre la page de l'application
sur le Play Store.

Le Play Store ne met une application à jour tout seul que lorsque l'appareil
est inactif et en Wi-Fi, et cette mise à jour automatique peut être coupée
application par application : un téléphone ou une télévision peut rester
plusieurs versions en arrière sans que personne ne s'en aperçoive, et c'est à
cela que sert cet avis. Un serveur plus ancien que cette fonction, ou une
application déjà à jour, n'affiche rien. L'application web n'a pas besoin de
mise à jour : c'est le serveur qui la sert.

## Quel appareil lit quoi

Chaque application indique ce que son appareil sait décoder, et le serveur
décide pour chaque titre d'envoyer le fichier tel quel ou de l'adapter. Un
même film peut partir intact vers une TV et être réencodé pour un téléphone
plus ancien. Le panneau **Infos du flux** du lecteur affiche la décision et
ses raisons : voir
[Comment Evey choisit le mode de lecture](../playback/how-playback-is-decided.md). La
diffusion suit des règles qui lui sont propres : voir
[Diffusion (Chromecast)](../playback/casting.md).
