---
source_sha: 7904b99e84aa0e735f5cb7448eda2d769b186a0612f82071abafe35cdfcc473b
---

# ADR-0019 : révocation par époque de jeton pour des JWT sans état (révoquer sans liste noire)

- Statut : acceptée
- Date : 2026-06-14

## Contexte

Les jetons d'accès d'Evey sont des JWT sans état : le serveur valide une
signature et fait confiance aux revendications, sans lecture en base à chaque
requête. C'est rapide, mais cela porte la faiblesse classique du modèle :
**un jeton ne peut pas être révoqué avant son expiration**. Quand un
administrateur désactive un compte, réinitialise un mot de passe ou change un
rôle, les jetons déjà frappés restent valides jusqu'à leur expiration naturelle.

Les deux remèdes de manuel coûtent cher. Une liste noire de jetons réintroduit
une lecture par requête sur un ensemble qui ne cesse de croître, et qu'il faut
conserver jusqu'à l'expiration de chaque jeton : lourd pour le cas courant, où
il n'y a aucune révocation. Des durées de vie très courtes avec rafraîchissement
réduisent la fenêtre sans jamais la fermer, matraquent le chemin de
rafraîchissement, et le jeton de rafraîchissement lui-même reste à révoquer.

## Décision

Chaque utilisateur porte une **époque de jeton** croissante. Tout jeton frappé
embarque l'époque courante, et la validation ne l'accepte que si son époque
correspond à celle de l'utilisateur, lue à travers un cache de courte durée. Une
révocation est un simple **incrément d'époque**, qui invalide d'un seul coup
tous les jetons frappés avant elle.

**Un décorateur, une instance câblée.** Le décorateur enveloppe le générateur de
jetons, et une seule instance est câblée pour tous les sites de frappe, si bien
qu'il n'existe qu'un endroit qui estampille et vérifie l'époque. La frappe pose
l'époque courante, la validation **refuse en cas d'erreur de lecture**, et la
révocation incrémente puis vide l'entrée de cache.

**Un port étroit pour l'époque.** La lecture et l'incrément passent par un port
dédié, délibérément tenu à l'écart de la mise à jour ordinaire d'un utilisateur,
pour qu'une modification de routine ne déplace jamais l'époque par accident.
L'incrément est atomique.

**Trente secondes de cache, le curseur entre latence et coût.** La lecture de
l'époque est mise en cache trente secondes. Cela borne la latence de révocation,
un compte désactivé conservant son accès au plus une trentaine de secondes, tout
en gardant une validation servie par le cache dans le cas très majoritaire où
personne n'a rien révoqué.

**Ce qui incrémente l'époque : des événements de sécurité, PAS la déconnexion.**
Désactiver un compte, changer ou réinitialiser un mot de passe et modifier un
rôle incrémentent. La suppression d'un compte est couverte par la lecture qui
échoue et refuse. Une déconnexion ordinaire n'incrémente pas : elle se contente
de supprimer le jeton de rafraîchissement, car l'époque sert aux révocations de
sécurité et non à l'hygiène de session ; se déconnecter d'un appareil ne doit
pas tuer toutes les autres sessions.

**Les jetons hérités** frappés avant ce mécanisme ne portent pas d'époque et
restent valides jusqu'au premier incrément pour cet utilisateur. Il n'y a pas de
jour de bascule.

## Conséquences

- Une vraie révocation sur un modèle de JWT sans état, en une écriture, avec une
  validation qui reste un accès au cache et aucune liste noire qui grossit.
- Le jeton n'est plus *purement* sans état : la validation consulte l'époque, si
  bien qu'un nœud au cache froid paie une lecture en base par utilisateur et par
  fenêtre de trente secondes. La latence de révocation d'une trentaine de
  secondes est assumée.
- Tout nouveau chemin de frappe DOIT passer par l'unique instance décorée, sans
  quoi il produirait des jetons qui échappent à la révocation.
- La révocation des identifiants n'est délibérément PAS déclenchée par l'arrêt
  d'une session de lecture : couper un flux ne doit pas déconnecter la personne
  de l'application ; bannir un compte est une action distincte.
- Les jetons d'extension gardent leur propre générateur : limités et de courte
  durée, ils sortent du périmètre de ce mécanisme.
