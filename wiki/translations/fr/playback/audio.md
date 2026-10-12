---
source_sha: 87e08f341b329e7354b9d42ef5972fac5cb564784a1488786bff878e7d74ba8a
reviewed: true
---
# Audio

**En bref :**

- Vous entendez la langue de votre profil quand le film l'a : réglez-la dans
  **Paramètres → Lecture → Langue audio par défaut**. Une piste que vous
  choisissez à la main dans le lecteur passe toujours en premier.
- Le son est envoyé tel quel quand votre appareil sait le lire. Sinon, le
  serveur le convertit dans un format courant (AAC). Souvent, le son est la
  seule chose convertie, et l'image n'est pas touchée.
- Sur le haut-parleur d'un téléphone, un casque ou en Bluetooth,
  l'application Android lit en stéréo. Avec l'application Android ou Android
  TV en HDMI, vous avez autant de canaux que la TV ou l'amplificateur en
  accepte (5.1, par exemple).
- Le son est décalé par rapport à l'image ? Voir
  [Son désynchronisé](#sound-out-of-sync).

Le reste de cette page explique quelle piste est lue, quand elle est copiée
ou convertie, et avec combien de canaux. Les réglages de profil cités ici
sont dans **Paramètres → Lecture**.

## Ce que vous entendrez, selon l'appareil

| Votre appareil | Ce qui arrive |
| --- | --- |
| Un navigateur | La piste telle quelle quand le navigateur la décode ; l'AC-3 et l'E-AC-3 décodés dans la page (ci-dessous) ; sinon de l'AAC. Une piste AAC surround est réduite par le navigateur aux canaux de votre sortie. |
| Un téléphone Android sur son haut-parleur, un casque ou le Bluetooth | De la stéréo. L'application déclare une sortie stéréo, donc une piste surround est réencodée en stéréo par le serveur, avec un mélange normalisé. Le Dolby Digital que l'application décode elle-même est envoyé tel quel et ramené en stéréo par l'application. |
| Un téléphone Android ou une Android TV en HDMI | Autant de canaux que la sortie HDMI déclare en accepter : une piste 5.1 reste en 5.1 sur un ampli 5.1. |
| Un récepteur de diffusion | Ce que le récepteur a déclaré sur sa sortie : voir [Diffusion](casting.md). |

Les applications Android sondent la sortie à chaque début de lecture :
brancher un casque prend effet au titre suivant, au changement de piste ou de
qualité, pas au milieu d'une scène.

## Quelle piste est jouée

Une piste choisie à la main, sur la fiche du titre ou dans le lecteur, prime
toujours. Sinon, les lecteurs suivent la **Langue audio par défaut** du
profil, puis la piste que le fichier marque par défaut, puis la première. Une
piste marquée comme commentaire est écartée au profit d'une piste normale
quand le fichier contient les deux.

Seule la piste écoutée est évaluée : une piste DTS que vous n'écoutez pas ne
provoque jamais de transcodage.

## Copiée ou réencodée

La piste est copiée quand le lecteur décode son codec et que sa sortie
restitue assez de canaux, dans la limite des **Canaux audio maximum** du
profil (**Stéréo (2.0)**, **Surround (5.1)** ou **Surround (7.1)**). Sinon,
elle est réencodée en AAC :

- avec autant de canaux que la source, l'appareil et le profil le
  permettent, jusqu'à 5.1 (une source 7.1 est ramenée en 5.1, que tous les
  lecteurs décodent ; une source stéréo reste en stéréo) ;
- quelle que soit la fréquence d'échantillonnage, qui n'est jamais à elle
  seule une raison de réencoder.

Une piste copiée à l'intérieur d'un flux transcodé ou remuxé doit aussi être
dans un format que le lecteur accepte au sein des segments du flux. L'AAC,
l'AC-3 et l'E-AC-3 le sont toujours ; les autres codecs seulement si le
lecteur l'indique. Sinon, la piste est réencodée et les Infos du flux
affichent **Piste audio non transmissible telle quelle dans le flux** : un
lecteur peut décoder du MP3 dans un fichier et le refuser malgré tout dans un
flux.

## Dolby Digital dans un navigateur

La plupart des navigateurs ne savent pas décoder l'AC-3 ni l'E-AC-3. Plutôt
que de réencoder la piste sur le serveur, le lecteur web la décode lui-même,
avec un décodeur qui s'exécute dans la page : la piste est envoyée telle
quelle et les Infos du flux l'indiquent comme *décodé client*. Les
navigateurs qui décodent nativement le Dolby Digital, comme Safari, la
reçoivent comme une piste ordinaire ; le choix se fait en testant le
navigateur, pas en le reconnaissant. Une piste surround n'est décodée dans la
page que si les **Canaux audio maximum** du profil l'autorisent : un profil
réglé en stéréo reçoit le réencodage AAC à la place, qui peut réduire le nombre
de canaux.

!!! warning
    Un reverse proxy qui impose sa propre Content-Security-Policy doit
    autoriser `'wasm-unsafe-eval'` dans `script-src`, faute de quoi le
    décodeur ne peut pas démarrer et le film se lit sans le son. Voir
    [Configurations de reverse proxy](../reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own).

## Son désynchronisé

**Synchro audio**, dans le lecteur, décale le son du titre en cours (une
valeur positive le joue plus tard). Le réglage est proposé quand le flux vient
du serveur en remux ou en transcodage ; en lecture directe, il affiche
**Disponible uniquement en lecture transcodée**. Appliquer un décalage
redémarre le flux à la position courante. Une piste copiée ne peut pas être
décalée : le serveur la réencode, et les Infos du flux affichent
**Resynchronisation audio demandée**.

Quand le décalage vient du fichier lui-même (une piste audio qui démarre
après l'image, ou des pistes en désaccord entre elles), **Administration → Santé des
médias** le signale mais ne le réécrit jamais de lui-même : une piste qui
démarre en retard est le plus souvent un doublage calé volontairement.
**Vérifier la synchro**, sur la ligne du fichier, permet d'en juger à l'œil et
d'appliquer un décalage, que son historique peut annuler. Voir
[Santé des médias](../runbooks/media-health.md#checking-the-sync-by-eye).
