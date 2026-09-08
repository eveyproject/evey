---
source_sha: bbebd5707a519a78e76f8f96e2510a99671fb1219bdac2599719410b97a62cc3
---

# Ce qu'Evey cherche à être

Cinq choses guident la conception. Quand un changement doit renoncer à quelque
chose, c'est dans cet ordre qu'il y renonce.

## Exposable sur internet sans crainte

Evey est fait pour être joignable, pas pour se cacher sur un réseau local. Les
valeurs par défaut supposent donc un réseau hostile : l'accès distant se déduit
du fait que vous avez publié une URL ou non, l'authentification refuse quand la
base est lente plutôt que de laisser passer une requête, les jetons d'extension
sont refusés par défaut et n'ouvrent que ce que le manifeste a déclaré, et le
serveur ne tourne jamais en root.

## Installable en quelques lignes

Un seul `docker-compose.yml`, aucun secret à inventer, aucun second conteneur à
câbler. L'image fabrique ses propres identifiants au premier démarrage et fait
tourner sa base de données et son index de recherche en son sein. Tout ce qu'un
administrateur pourrait vouloir changer ensuite vit dans le panneau
d'administration, pas dans des variables d'environnement. Voir
[ADR-0028](adr/0028-install-is-one-compose-file.md).

## Extensible dans le langage que vous voulez

Une extension est un processus que le serveur lance et avec lequel il parle en
gRPC sur une socket locale. C'est tout le contrat, donc le langage vous
appartient : des stubs de SDK pré-générés existent pour sept d'entre eux, et
tout ce que le point d'entrée sait lancer fonctionne même sans stub. Les
extensions sont confinées par le noyau plutôt que crues sur parole, et c'est ce
qui rend raisonnable l'idée de faire tourner le code de quelqu'un d'autre.

## Rapide, et franc sur ce qu'il fait

La lecture préfère envoyer le fichier tel quel, ne transcode que ce que le
client ne sait pas ouvrir, et décide une fois par requête au lieu de recalculer.
Les mesures qui guident ces choix sont prises, pas supposées : une sonde de scan
qui semblait une bonne idée n'a été retenue qu'après avoir été prouvée
équivalente, sur de vrais fichiers, à l'outil qu'elle remplace.

## Pour beaucoup de spectateurs à la fois, sur du matériel modeste

La cible est un NAS qui sert un foyer et ses amis, pas une baie de serveurs. La
concurrence est donc bornée volontairement : les encodeurs sont réutilisés d'une
requête à l'autre, les sessions sont admises contre un budget, la télémétrie est
jetée plutôt que de ralentir un segment, et le serveur dimensionne ses groupes
de workers sur la machine qu'il trouve.

---

Deux choses qu'Evey n'est délibérément pas. Ce n'est pas un catalogue où chaque
type de média serait traité correctement : la lecture est ce qu'il revendique.
Et ce n'est pas un produit qui vous demande de lui faire confiance : ce que le
serveur fait, et ce qu'il ne sait toujours pas empêcher, est écrit dans
[SECURITY.md](SECURITY.md) au lieu d'être sous-entendu.
