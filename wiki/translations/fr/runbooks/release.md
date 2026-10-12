---
source_sha: f7107d811a1a25b00c55dbd3789c083144308003ef8ab0ea3e52905c56b1bc7c
---

# Publier une version

Comment une version d'Evey passe d'une branche à quelque chose qu'un
administrateur peut épingler. Écrit avant la première version étiquetée, pour
que la première suive une procédure au lieu de l'inventer.

## Ce que la CI fait d'une étiquette (à lire avant d'étiqueter)

La chaîne d'intégration construit et publie des images à chaque poussée et à
chaque étiquette de version, en trois variantes d'environnement d'exécution pour
les extensions - la version allégée, celle avec Node, et la complète - vers les
deux registres publics :

| Ce que vous poussez | Étiquettes d'image publiées (chacune aussi en `-nodejs` et `-full`) |
|---|---|
| une branche (par exemple `beta-0.0.1`) | `beta-0.0.1`, `sha-<court>` |
| l'étiquette `v0.2.0-beta.1` (préversion : elle contient un tiret) | `0.2.0-beta.1`, `sha-<court>` |
| l'étiquette `v0.2.0` (stable) | `0.2.0`, `0.2`, `0`, `latest`, `sha-<court>` |

Trois conséquences.

- **Une étiquette de branche bouge.** Elle désigne ce qui a été poussé en
  dernier : deux récupérations à un jour d'intervalle peuvent faire tourner du
  code différent, et « revenir à `beta-0.0.1` » ne désigne aucune version précise. Un
  administrateur doit épingler une étiquette `sha-<court>` ou une étiquette de
  version, jamais un nom de branche.
- **Une préversion ne modifie jamais `latest`.** `latest` désigne la version
  stable la plus récente ; une préversion de bêta fermée reste invisible pour qui
  suit `latest`. Une préversion ne reçoit pas non plus les étiquettes courtes, réservées
  aux versions stables.
- **Toutes les constructions sont multi-architectures**, amd64 et arm64, si bien
  qu'un administrateur sur ARM peut épingler n'importe laquelle.

Le commit est estampillé dans le binaire et dans les étiquettes de l'image, si
bien que la question « quel code tourne ici » se répond toujours sans faire
confiance à une étiquette.

## Publier une préversion (bêta fermée)

1. **Les contrôles réussis sur le commit exact** : `make pre-push` en local, la CI
   passe les mêmes à la poussée.
2. **Prouvée d'abord sur une vraie installation.** Le commit doit déjà avoir
   tourné sur le déploiement de référence sous son étiquette `sha-<court>`, mis
   à jour puis vérifié par le script prévu. Une étiquette engage envers d'autres
   administrateurs : on la pose après la preuve, pas avant.
3. **Le journal des modifications** : déplacer les entrées non publiées sous la
   nouvelle version avec sa date, en formulations orientées utilisateur.
4. **Étiqueter depuis ce commit**, de façon annotée, et ne pousser que
   l'étiquette :
   ```bash
   git tag -a v0.2.0-beta.1 -m "v0.2.0-beta.1"
   git push origin v0.2.0-beta.1
   ```
5. **Surveiller la publication** par la CI, puis épingler cette étiquette de
   version sur le déploiement de référence, ce qui remplace l'épinglage par
   empreinte de commit.
6. **Annoncer l'étiquette** avec la section correspondante du journal des
   modifications. Les administrateurs épinglent alors cette version sur leur
   ligne `image:`.

Les étiquettes de préversion ne coûtent rien : `-beta.2`, `-beta.3`, autant que
nécessaire. Ce qu'elles ne sont pas, c'est rétractables : une étiquette publiée
reste publiée. Pour corriger une mauvaise version, on en publie une nouvelle ; on ne
redirige jamais une étiquette existante.

## Publier une version stable

Mêmes étapes, avec une étiquette sans suffixe. Une chose à vérifier dont une
préversion se passe : la version majeure de Postgres embarquée dans l'image doit
figurer au journal des modifications, car un administrateur sur la base
embarquée doit suivre la section de montée de version de
[installation.md](../installation.md) quand elle change.

## Revenir en arrière sur une version

Les images restent dans le registre. Un administrateur revient en arrière avec
le script de retour arrière, en épinglant l'étiquette précédente et, au besoin,
en restaurant le dump d'avant mise à jour ; le script de mise à jour affiche la
ligne exacte à utiliser. Rien n'est à défaire côté publication : on publie une
préversion corrigée quand le correctif arrive.
