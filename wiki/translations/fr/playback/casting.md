---
source_sha: 8c3a42e5732d38c6b13e080b8869dff85cef64c692ff15ed7ef067e6d0e686e9
reviewed: true
---
# Diffusion (Chromecast)

Caster, c'est envoyer un film de votre téléphone ou de votre ordinateur vers
la TV.

1. Lancez le film dans le lecteur web sous Chrome, ou dans l'application
   Android.
2. Appuyez sur **Caster** dans le lecteur.
3. Choisissez votre Chromecast, ou votre TV avec Chromecast intégré.

Le film passe alors sur la TV. L'application Android TV n'a pas de bouton
**Caster** : elle est déjà sur la TV. Firefox n'affiche pas non plus de bouton
**Caster**. Avec Safari, ou Chrome sur un téléphone Android, le bouton marche
autrement : voir
[Navigateurs sans session Cast propre](#browsers-without-a-cast-session-of-their-own).

L'image sur la TV peut être différente de celle de votre téléphone, et c'est
normal. La TV ne reçoit pas ce que lisait votre téléphone : elle va chercher
le film elle-même sur le serveur, et le serveur prépare une version adaptée à
la TV. La suite de cette page explique comment, pour les curieux et pour la
personne qui fait tourner Evey.

## Ce que la TV lit

C'est la TV qui décode le flux : la décision de lecture repose donc sur les
capacités du récepteur, tirées de la source la plus fiable disponible :

1. **Ce que le récepteur a constaté sur lui-même.** Au démarrage, le
   récepteur d'Evey teste son propre appareil (les codecs et résolutions
   qu'il sait afficher, s'il affiche le HDR, s'il transmet l'audio Dolby tel
   quel, combien de canaux audio sa sortie restitue) et le déclare. Quand
   cette déclaration arrive, c'est elle qui est utilisée.
2. **Sinon, le modèle.** Le nom que remonte le SDK Cast (Chromecast avec
   Google TV, Chromecast Ultra, Google TV Streamer...) correspond aux
   capacités documentées de cette famille.
3. **Sinon, un profil prudent.** Un appareil inconnu, un récepteur qui n'a
   pas répondu à temps ou un appareil trouvé sans nom de modèle reçoit les
   capacités du plus ancien Chromecast : H.264 jusqu'en 1080p, SDR, pas de
   Dolby.

Cet ordre suit le coût d'une erreur : prêter à l'appareil une capacité qu'il
n'a pas donne un écran noir, alors qu'en supposer trop peu ne coûte qu'un
transcodage. Concrètement : le HDR n'arrive à la TV que si son récepteur a
déclaré un écran HDR ; le Dolby Digital (AC-3, E-AC-3) n'est envoyé tel quel
qu'à un récepteur qui a déclaré le transmettre nativement, sinon il est
réencodé en AAC. Un récepteur qui s'est dit capable de surround puis n'a pas
réussi à charger le flux recharge une fois avec un plafond stéréo.

## Quand la connexion de la TV est lente

Avant de demander un flux, le récepteur mesure son propre débit et le joint à
la demande. Le serveur conserve la vidéo d'origine quand ce débit la porte
avec une marge de 20 %. Sinon, il choisit la qualité la plus haute que la
connexion supporte. Si la lecture cale malgré tout, le récepteur recharge une
fois sur la qualité la plus basse, à condition qu'elle divise le débit par
deux. Les Infos du flux affichent alors **Le débit mesuré par le client ne
peut pas porter la copie** ou **Palier de survie demandé par le lecteur**.

Les anciens appareils de diffusion ne changent pas de qualité d'eux-mêmes en
cours de lecture : c'est pourquoi le serveur fait ce choix à leur place.

## Navigateurs sans session Cast propre

Dans le lecteur web, le bouton **Caster** apparaît quand le navigateur sait
diffuser : Chrome sur ordinateur, qui ouvre son sélecteur Google Cast, et les
navigateurs qui gèrent la lecture à distance, comme Safari ou Chrome sur
Android. Firefox n'en affiche pas.

Quand le navigateur ne sait pas ouvrir lui-même une session Cast (Safari,
Chrome sur Android), un appui passe par le serveur, qui essaie dans cet ordre
et prend le premier appareil trouvé à chaque étape :

1. un Chromecast sur le réseau de l'appareil que vous tenez, lu par le
   récepteur d'Evey comme ci-dessus ;
2. une TV DLNA sur le réseau du serveur. Elle reçoit le flux que lisait le
   navigateur (le fichier d'origine en lecture directe, le flux transcodé
   sinon) : aucune décision n'est prise pour la TV elle-même ;
3. vos propres applications Evey ouvertes et connectées au même compte : une
   seule reprend le film, comme avec **Lire sur** ; s'il y en a plusieurs, le
   lecteur renvoie vers **Lire sur** pour choisir ;
4. le sélecteur d'appareils du navigateur lui-même.

!!! note
    Les étapes 1 et 2 prennent le premier appareil trouvé, pas celui que vous
    choisissez. Avec plusieurs Chromecast à la maison, diffusez depuis Chrome
    sur ordinateur ou depuis l'application Android, qui affichent tous deux
    un sélecteur.

## Quand une diffusion ne fonctionne pas

[Une diffusion s'est figée, est devenue muette, ou le téléphone tourne en boucle](../runbooks/incident-runbook.md#a-cast-froze-went-silent-or-the-phone-keeps-spinning).
