---
source_sha: 7a1d02609606ef3f71f52ef28cb057c2d7604912d5f5dba6eff03b938cdb2f6f
---

# ADR-0018 : télémétrie de qualité fondée sur CMCD, avec un collecteur non bloquant qui jette quand il est plein

- Statut : acceptée
- Date : 2026-06-14

## Contexte

Le lecteur et le transcodeur ont besoin d'un signal de retour : le client
est-il à sec, en train de remplir son tampon, ou confortablement en avance ?
Sans lui, l'encodeur ne peut pas régler son allure, l'échelle de qualité par
titre n'apprend rien, et l'administrateur n'a aucun tableau de bord de qualité
de service.

Trois approches ont été pesées. Un schéma de télémétrie maison a été écarté : il
réinventerait un standard et obligerait le lecteur à porter une instrumentation
propre à Evey que nul autre outil ne comprend. Une chaîne de métriques externe
est trop lourde pour un serveur mono-processus auto-hébergé, souvent un
Raspberry Pi ou un NAS modeste, et la boucle de rétroaction de l'encodeur a
besoin du signal **dans le processus**, pas après un aller-retour dans une base
temporelle. Restait CMCD, le format standard que le lecteur émet déjà.

Une seconde tension : le chemin d'ingestion est voisin du **chemin chaud des
segments HLS**. La télémétrie ne doit JAMAIS ajouter de latence ni de contre-
pression au service des médias.

## Décision

La qualité de service est un contexte à part entière, alimenté par du CMCD
analysé dans un paquet partagé, avec un collecteur **en mémoire, non bloquant,
qui jette quand il est plein**.

**CMCD plutôt qu'un schéma maison.** Les clients envoient du CMCD sur leurs
requêtes de segment, et l'analyse est bornée par un budget de clés fixe pour
contenir un client hostile. On parle le standard que le lecteur émet déjà :
aucun code client propre à Evey, et des données lisibles par n'importe quel
outil compatible.

**Un canal et un travailleur, jamais une file externe.** L'ingestion remet
l'échantillon à un canal tamponné qu'une goroutine vide en écrivant de façon
asynchrone. Le chemin chaud fait un envoi non bloquant et rend la main
immédiatement. Rester en mémoire garde l'empreinte plate et laisse la boucle de
rétroaction lire le signal vivant sans saut réseau.

**Jeter quand c'est plein, délibérément.** Si le canal est saturé, l'échantillon
est **jeté**, pas attendu. La télémétrie est au mieux par définition : perdre
une mesure est invisible pour l'utilisateur, alors que bloquer une réponse de
segment est une coupure. Ce compromis est toute la raison d'être de cette
conception et ne doit pas être « corrigé » en rendant l'envoi bloquant.

**La boucle de rétroaction respecte le choix manuel de qualité.** L'encodeur lit
le signal pour régler son allure, jamais pour passer outre une qualité choisie
par l'utilisateur : la boucle ajuste le rythme, pas le palier retenu.

## Conséquences

- Aucune instrumentation client propre à Evey ; l'encodeur dispose d'un signal
  de disette vivant ; le chemin chaud n'est jamais bloqué par la télémétrie.
- Sous rafale soutenue, le collecteur perd des échantillons : le tableau de bord
  est un échantillon représentatif, pas un journal garanti complet. C'est voulu.
- Toute évolution qui rendrait l'envoi bloquant, ou qui le ferait passer par une
  écriture synchrone sur le chemin chaud, réintroduirait exactement le risque de
  latence que cette décision existe pour empêcher.
