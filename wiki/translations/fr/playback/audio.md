---
source_sha: 671aed4ea8f4b0ef01dac83a6cadd4f8e569eb5206951057955cbf29a9e7b727
reviewed: true
---
# Audio

La piste audio est évaluée indépendamment de la vidéo. Dans la plupart des
transcodages, c'est la seule chose réencodée : la vidéo est copiée et le son
converti dans un format que le lecteur sait lire.

## Quelle piste est jouée

Une piste choisie à la main, sur la fiche du titre ou dans le lecteur, prime
toujours. Sinon, les lecteurs suivent la **Langue audio par défaut** du
profil, puis la piste que le fichier marque par défaut, puis la première. Une
piste marquée comme commentaire est écartée au profit d'une piste normale
quand le fichier contient les deux.

Seule la piste écoutée est évaluée : une piste DTS que vous n'écoutez pas ne
provoque jamais de transcodage.

## Copiée ou réencodée

La piste est copiée quand le lecteur décode son codec et restitue assez de
canaux, dans la limite des **Canaux audio maximum** du profil (Stéréo, 5.1 ou
7.1). Sinon, elle est réencodée en AAC :

- avec autant de canaux que la source, le lecteur et le profil le
  permettent, jusqu'à 5.1 (une source 7.1 est ramenée en 5.1, que tous les
  lecteurs décodent ; une source stéréo reste en stéréo) ;
- quelle que soit la fréquence d'échantillonnage, qui n'est jamais à elle
  seule une raison de réencoder.

Une piste copiée à l'intérieur d'un flux transcodé ou remuxé doit aussi être
dans un format que le lecteur accepte au sein des segments du flux. L'AAC,
l'AC-3 et l'E-AC-3 le sont toujours ; les autres codecs seulement si le
lecteur l'indique. Sinon, la piste est réencodée et les Infos du flux le
signalent : un lecteur peut décoder du MP3 dans un fichier et le refuser
malgré tout dans un flux.

## Dolby Digital dans un navigateur

La plupart des navigateurs ne savent pas décoder l'AC-3 ni l'E-AC-3. Plutôt
que de réencoder la piste sur le serveur, le lecteur web la décode lui-même,
avec un décodeur qui s'exécute dans la page : la piste est envoyée telle
quelle et les Infos du flux l'indiquent comme *décodé client*. Les
navigateurs qui décodent nativement le Dolby Digital, comme Safari, la
reçoivent comme une piste ordinaire ; le choix se fait en testant le
navigateur, pas en le reconnaissant.

Un reverse proxy qui impose sa propre Content-Security-Policy doit autoriser
`'wasm-unsafe-eval'` dans `script-src`, faute de quoi le décodeur ne peut pas
démarrer et le film se lit sans le son. Voir
[Configurations de reverse proxy](../reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own).

## Son désynchronisé

**Synchro audio**, dans le lecteur, décale le son du titre en cours. Une piste
copiée ne peut pas être décalée : le serveur la réencode, et les Infos du
flux affichent **Resynchronisation audio demandée**.

Quand le décalage vient du fichier lui-même (une piste audio qui démarre
après l'image, ou des pistes en désaccord entre elles), **Admin → Santé des
médias** le signale mais ne le réécrit jamais de lui-même : une piste qui
démarre en retard est le plus souvent un doublage calé volontairement.
**Vérifier la synchro**, sur la ligne du fichier, permet d'en juger à l'œil et
d'appliquer un décalage, que son historique peut annuler. Voir
[Santé des médias](../runbooks/media-health.md).
