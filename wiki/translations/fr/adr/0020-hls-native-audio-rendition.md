---
source_sha: c0e7790697ba9ca7b514f4406c8d3cd4b2e3664897252b2ed321ed25614cb919
---

# ADR-0020 : piste audio native HLS (EXT-X-MEDIA) pour le changement de langue

- Statut : acceptée
- Date : 2026-06-14

## Contexte

Un spectateur veut changer de piste audio - de la version originale au doublage,
ou vers un commentaire - sans retélécharger la vidéo. Dans le modèle multiplexé,
où l'audio est cuit dans chaque variante vidéo, changer de langue impose une
nouvelle session de transcodage du flux **entier** et le retéléchargement d'une
vidéo que le client possède déjà. Les serveurs répandus s'en sortent par un
service d'extraction audio dédié ou en remultiplexant tout le flux.

Nous voulons un changement de langue qui ne retélécharge ni ne retranscode la
vidéo, qui passe par du HLS standard pour que Safari, AVPlayer et le cast le
gèrent nativement, et qui ne dégrade pas le pipeline multiplexé existant dans le
cas courant d'une seule piste audio.

## Décision

L'audio est servi comme un **groupe de pistes** HLS natif au moyen de
`EXT-X-MEDIA`, ses segments étant produits par un pipeline audio dédié, séparé
des variantes vidéo.

**EXT-X-MEDIA plutôt qu'un ffmpeg audio par changement.** Le manifeste maître
déclare les pistes audio comme un groupe indexé sur les flux audio de la source.
Le lecteur change de membre du groupe sans toucher à la playlist vidéo, par le
mécanisme HLS standard que tout client natif implémente déjà. Cela a été retenu
plutôt que de lancer une session ffmpeg audio à chaque changement : le groupe
est déclaratif, c'est le client qui pilote, et le serveur n'a rien à faire au
moment du changement.

**Un chemin de segments audio dédié.** Les segments audio sont produits
indépendamment de l'orchestrateur vidéo, et le générateur de manifeste gagne une
branche pour les pistes. L'URL d'un segment audio est indexée de façon à ce que
chaque piste adresse les siens.

**Le drapeau désactivé par défaut préservait le pipeline multiplexé.** Le chemin
séparé était derrière un drapeau désactivé, pour qu'une installation sans besoin
de multilingue garde le pipeline éprouvé. Voir l'amendement ci-dessous : ce n'est
plus le cas.

**Cela se compose avec les décisions de codec audio.** Le groupe de pistes décrit
*quelles* pistes existent et comment les adresser ; les octets de chaque piste
sont produits par la décision audio habituelle, copie, transcodage ou
passthrough.

## Conséquences

- Changement de langue sans retélécharger la vidéo, par du HLS standard que les
  clients natifs pilotent eux-mêmes, et sans coût serveur au moment du
  changement.
- Un second chemin de production de segments à maintenir à côté du multiplexé, et
  un générateur de manifeste qui porte les deux formes. Qui ajoute un codec audio
  doit comprendre le modèle de groupe de pistes, d'où cette décision.
- Les clients web sans changement natif de groupe audio retombent sur le
  comportement multiplexé ; la fonctionnalité vise d'abord Safari, AVPlayer,
  tvOS et le cast.

## Amendement du 2026-07-23 : le drapeau est désormais actif par défaut

La décision ci-dessus a été écrite quand le chemin par groupe de pistes était
neuf, et elle affirme qu'il est « derrière un drapeau désactivé par défaut ». Ce
n'est plus vrai, et ne l'était plus depuis un moment : la configuration lit ce
drapeau à `true`, donc le chemin séparé est celui de toutes les installations et
le pipeline multiplexé est devenu l'option de repli explicite.

Le fait est consigné plutôt que corrigé sur place : le raisonnement d'origine
tient, seule sa portée a changé. Deux conséquences se lisent maintenant à
l'envers - c'est le chemin séparé que tout le monde exécute, et le multiplexé
qui risque de rouiller faute d'usage. L'argument du repli sûr joue désormais dans
l'autre sens, et le chemin multiplexé demande d'être exercé délibérément pour
rester digne de confiance.
