---
source_sha: ca7f6846927d60865945e4118a9694b18e3af43247d9c41ac5110cf6c4c12f46
reviewed: true
---
# ac3go dans Evey { #ac3go-in-evey }

**En bref :**

- Chrome, Firefox et Edge ne savent pas lire le son Dolby Digital (AC-3) ni
  Dolby Digital Plus (E-AC-3). Plutôt que de le faire convertir par le
  serveur, le lecteur web d'Evey le décode dans la page elle-même, avec
  ac3go, un décodeur qui s'exécute dans le navigateur en WebAssembly.
- Le son vous parvient tel qu'il est dans le fichier, et le serveur n'a
  aucune conversion audio à faire pour lui.
- Vous n'avez rien à régler. Les **Infos du flux** du lecteur affichent le
  son comme *décodé client*, et **Administration → En cours de lecture**
  affiche *ac3go dans le lecteur* pour cette piste.
- Si le décodeur ne peut pas fonctionner, le lecteur passe de lui-même au
  son converti par le serveur (AAC).

La suite de cette page explique quand ce chemin est pris, comment le son
reste synchronisé avec l'image, et quoi vérifier quand un film se lit sans
son. Quelle piste audio est lue, et quand elle est copiée ou convertie en
général, se trouve dans [Audio](audio.md).

ac3go est développé comme une bibliothèque à part, écrite en Go et sans
dépendance : [github.com/gravity-zero/ac3go](https://github.com/gravity-zero/ac3go).

## Quand le navigateur décode le son lui-même { #when-the-browser-decodes-the-sound-itself }

Toutes ces conditions doivent être remplies :

| Condition | Sinon |
| --- | --- |
| Le navigateur ne sait pas lire l'AC-3 ni l'E-AC-3 lui-même, et il exécute le WebAssembly. Le lecteur teste le navigateur sur ces deux points au lieu de le reconnaître à son nom. | Un navigateur qui décode nativement le Dolby Digital, comme Safari, reçoit la piste comme une piste ordinaire. |
| La piste audio lue est en AC-3 ou en E-AC-3. Les autres pistes du film ne comptent pas. | Un autre codec suit les règles de la page [Audio](audio.md). |
| La piste a au plus six canaux (5.1), et pas plus que les **Canaux audio maximum** du profil. Ce chemin transmet la piste telle qu'elle est dans le fichier : il ne peut donc pas réduire le nombre de canaux. | Le serveur convertit la piste en AAC, ce qui permet de réduire les canaux pour un profil en stéréo. Une piste E-AC-3 en 7.1 est toujours convertie. |
| Le film passe par le flux du serveur (Remux ou transcodage), pas par la lecture directe du fichier. | La piste suit la décision prise pour cette lecture. |

## Comment la lecture se déroule { #how-it-plays }

L'image passe par le lecteur habituel du navigateur ; son flux ne porte aucune piste audio, et le volume et la sourdine du lecteur pilotent le gain du moteur de son à la place. Le son
vient d'un flux audio à part, que le serveur indique dans la liste de
lecture : la piste telle qu'elle est dans le fichier, empaquetée par
[MKVGo](mkvgo.md) quand c'est possible, copiée par FFmpeg sinon. Ni l'un ni
l'autre ne la décode.

Dans la page, un traitement en arrière-plan (un *worker*) charge ac3go et
décode chaque segment audio en son brut, à l'écart du code qui affiche
l'image. Le lecteur joue ensuite ce son par la sortie audio du navigateur, au
rythme de l'image : c'est l'image qui donne l'heure, et le son est calé sur
elle. Une pause arrête les deux ensemble. Un déplacement dans le film, ou un
trou après un blocage du flux, recale le son sur la nouvelle position.

Une piste 5.1 est remise au navigateur dans l'ordre de canaux qu'il attend,
pour qu'il puisse la réduire en stéréo pour des haut-parleurs ou un casque.

Au début d'un film, le décodeur met un instant à se charger. Quand le serveur
propose aussi la piste convertie en AAC, c'est ce son qui joue pendant cet
instant, puis ac3go prend le relais dès qu'il est prêt. Le navigateur ne
télécharge le décodeur qu'une fois et le revérifie à chaque utilisation : une
mise à jour du serveur ne mélange donc jamais une ancienne et une nouvelle
copie.

## Si ac3go ne peut pas fonctionner { #if-ac3go-cannot-run }

Quand le décodeur ne se charge pas, échoue sur un segment, ou que le serveur
ne lui a indiqué aucun flux audio, le lecteur cesse de l'utiliser pour le
reste du titre et redemande au serveur le flux normal, avec le son converti
en AAC. Le film garde son son ; les **Infos du flux** affichent alors la
conversion au lieu de *décodé client*.

!!! warning
    Un reverse proxy qui impose sa propre Content-Security-Policy doit autoriser
    `'wasm-unsafe-eval'` dans `script-src`, sinon le décodeur ne peut pas
    démarrer. Il doit aussi servir les fichiers du décodeur sous `/wasm/`,
    comme le reste de l'application web. Voir
    [Configurations de reverse proxy](../reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own).

## Sur un récepteur de diffusion { #on-a-cast-receiver }

Le récepteur Chromecast d'Evey n'utilise pas ac3go. Sur des boîtiers TV à
faible mémoire, les récepteurs qui décodaient le son de cette façon
s'arrêtaient au bout de quelques minutes de lecture. La TV reçoit le Dolby
Digital tel quel quand son récepteur indique qu'il le transmet, et l'AAC du
serveur sinon : voir [Diffusion (Chromecast)](casting.md).

## La bibliothèque et sa licence { #the-library-and-its-licence }

ac3go est publié sous licence GNU AGPLv3, comme Evey. Il est compilé à partir
de ses sources lors de la construction de l'image Evey (en WebAssembly, avec
les outils Go), et sa licence est servie à côté du décodeur, à l'adresse
`/wasm/ac3go.LICENSE.md`.
