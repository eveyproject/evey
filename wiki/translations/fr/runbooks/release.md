---
source_sha: 7e38ef34503d1582f3a4a5fc3e6171c1d2f137aa52dae78d9840504057637151
---

# Publier une version

Comment une version d'Evey passe d'une branche à une image qu'un
administrateur peut épingler. Écrit avant la première version étiquetée, pour
que la première suive une procédure au lieu de l'inventer.

## Ce que la CI fait d'une étiquette (à lire avant d'étiqueter)

`.github/workflows/ci.yml` construit et publie des images à chaque poussée et à
chaque étiquette `v*.*.*`, en trois variantes d'environnement d'exécution des
plugins (`` (complète, par défaut), `-nodejs`, `-lean`), vers `docker.io/eveyproject/evey`
et `ghcr.io/eveyproject/evey` :

| Ce que vous poussez | Étiquettes d'image publiées (chacune aussi en `-lean` / `-nodejs`) |
|---|---|
| une branche (par exemple `master`) | `master`, `sha-<short>` |
| l'étiquette `v0.2.0-rc.1` (préversion : elle contient un tiret) | `0.2.0-rc.1`, `sha-<short>` |
| l'étiquette `v0.2.0` (stable) | `0.2.0`, `0.2`, `0`, `latest`, `sha-<short>` |

Trois conséquences :

- **Une étiquette de branche change.** `master` désigne ce qui a été
  poussé en dernier : deux `docker compose pull` à un jour d'intervalle peuvent
  faire tourner du code différent, et un retour « à `master` » ne
  désigne aucune version précise. Un administrateur doit épingler une étiquette
  `sha-<short>` (ou une étiquette de version), jamais un nom de branche : c'est
  ce que `scripts/evey-update.sh` affiche à la fin, à reporter dans `.env`.
- **Une préversion ne modifie jamais `latest`.** `latest` désigne la version
  stable la plus récente ; une préversion de bêta fermée reste invisible pour
  qui suit `latest`. (Une préversion ne reçoit pas non plus les étiquettes
  `X.Y` / `X`, réservées aux versions stables.)
- **Multi-architecture sur les étiquettes de version uniquement.** Une étiquette
  `v*.*.*` (stable ou préversion) construit amd64 + arm64 ; la poussée d'une
  branche ne construit que amd64, sauf si le message de commit contient
  `[arm64]` ou si le workflow est lancé à la main. Un administrateur sur ARM
  épingle une étiquette de version.

Le commit est inscrit dans le binaire (`evey version`, `/healthz`) et dans les
labels de l'image (`org.opencontainers.image.revision`) : on peut donc toujours
savoir quel code tourne sans faire confiance à une étiquette.

## Publier une préversion (bêta fermée)

1. **Contrôles réussis sur le commit exact** : `make pre-push` en local. La CI
   exécute les mêmes contrôles à la poussée.
2. **Vérifiée d'abord sur une vraie installation.** Le commit doit déjà avoir
   tourné sur le déploiement de référence sous une étiquette `sha-<short>`, mis
   à jour avec `scripts/evey-update.sh --tag sha-<short> --apply` et vérifié
   avec `--commit <sha>`. Une étiquette engage envers les autres
   administrateurs : on la crée après cette vérification, pas avant.
3. **`CHANGELOG.md`** : déplacer les entrées `[Unreleased]` sous la nouvelle
   version, avec la date. Format Keep a Changelog, formulations destinées aux
   utilisateurs uniquement.
4. **Étiqueter ce commit**, avec une étiquette annotée, et ne pousser que
   l'étiquette :
   ```bash
   git tag -a v0.2.0-rc.1 -m "v0.2.0-rc.1"
   git push origin v0.2.0-rc.1
   ```
5. **Surveiller la CI** qui publie `0.2.0-rc.1` (+ variantes). Puis, sur le
   déploiement de référence, `scripts/evey-update.sh --tag 0.2.0-rc.1 --apply` :
   l'étiquette de version remplace l'épinglage `sha-`.
6. **Annoncer l'étiquette** avec sa section du changelog. Les administrateurs
   épinglent `eveyproject/evey:0.2.0-rc.1` (ou `-lean`) sur leur ligne
   `image:`.

Créer des étiquettes de préversion ne coûte rien : `-rc.2`, `-rc.3` autant
que nécessaire. En revanche, elles ne peuvent pas être retirées : une étiquette
publiée reste publiée. Pour corriger une mauvaise préversion, on publie la
suivante ; on ne déplace jamais une étiquette.

## Publier une version stable

Mêmes étapes, avec une étiquette `vX.Y.Z` sans suffixe. Un point à vérifier
dont une préversion se passe : la version majeure de Postgres intégrée à
l'image doit figurer dans le changelog, car un administrateur qui utilise la
base intégrée doit suivre la section de mise à niveau de
[installation.md](../installation.md) quand elle change.

## Revenir à une version précédente

Les images restent dans le registre. Un administrateur revient en arrière avec
`scripts/evey-rollback.sh --tag <previous> [--restore <pre-update dump>] --apply`
(le script de mise à jour affiche cette ligne exacte). Rien n'est à annuler
côté publication ; on publie une `-rc.N+1` corrigée quand le correctif est
prêt.
