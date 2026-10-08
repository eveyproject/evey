---
source_sha: eb0bf4bc0439ca7578ac20f7489cba0718e37fee252faa25b0bdd1888c7bbbb5
reviewed: true
---
# Diffusion (Chromecast)

Ce que le téléviseur lit quand vous diffusez, et pourquoi cela peut différer
de ce que lisait votre téléphone ou votre navigateur. Pour les spectateurs qui
utilisent le bouton **Caster** du lecteur web sous Chrome ou de l'application
Android, vers un Chromecast ou une TV équipée de Chromecast ; l'application
Android TV n'a pas de bouton Caster. Le téléphone ou le navigateur ne fait que
lancer la session : la TV lit le flux directement depuis le serveur, via
l'application récepteur d'Evey, et le serveur décide de ce qu'il envoie en
fonction de la TV, pas de l'appareil qui a lancé la diffusion.

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
