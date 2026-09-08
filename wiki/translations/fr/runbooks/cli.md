---
source_sha: 82f98fcae7d7d2028b57419f257f377ba2928df3c32c0a66a00c30682eda65c1
---

# La CLI Evey - `evey admin`

Le binaire du serveur Evey fait aussi office de CLI d'exploitation. Invoqué sans
argument, `evey` démarre le serveur HTTP/gRPC ; avec `admin` en premier
argument, il bascule en mode exploitation à coup unique et rend la main dès que
la commande est terminée.

Toutes les commandes lisent leur configuration dans les mêmes variables `EVEY_*`
que le serveur lui-même (`EVEY_DB_USER`, `EVEY_DB_PASSWORD`, `EVEY_DB_HOST`,
`EVEY_DB_PORT`, `EVEY_DB_NAME`, `EVEY_DB_SSLMODE`, `EVEY_BACKUP_DIR`, ...).

## Aide-mémoire

```
evey admin reset-password --user <email>     Génère un nouveau mot de passe et l'affiche une seule fois.
evey admin rotate-jwt-key                    Régénère la clé de signature JWT (tue TOUTES les sessions).
evey admin restore-backup <path>             Restaure la base depuis un dump .sql.gz (le suffixe est obligatoire).
evey admin reindex-search [--library <id>] [--purge]   Reconstruit l'index Meilisearch depuis Postgres (--purge évacue d'abord les documents orphelins ; si Meili est à terre, signale le repli sur Postgres et ne fait rien).
evey admin migrate up [--no-backup]          Applique les migrations en attente.
evey admin migrate down [--no-backup]        Recule d'un cran de migration.
evey admin migrate version                   Affiche la version de schéma courante (sort en 1 si dirty).
evey admin list-users                        Affiche les adresses des utilisateurs, une par ligne.
```

### Maintenance des médias

```
evey admin reindex-no-cue --root <dir> [--apply] [--limit N] [--resync] [--keep-backup=false]
                                             Reconstruit l'index de navigation (les Cues) des fichiers
                                             MKV/WebM qui en sont dépourvus - le correctif de la
                                             navigation et de la reprise lentes.
                                             À BLANC PAR DÉFAUT : passez --apply pour réécrire. Chaque
                                             réécriture est validée, recoupée, et conserve une
                                             sauvegarde .mkvgobak. --limit N pour un premier passage
                                             prudent.
                                             --resync répare en plus les fichiers portant une zone
                                             corrompue que la passe stricte refuse (saute le dégât,
                                             garde le reste, rapporte les octets perdus).
                                             LONG : réécrit chaque fichier en entier, des heures sur
                                             une grande bibliothèque. Affiche une barre de progression
                                             pendant l'application.
evey admin validate-streaming --root <dir> [--errors-only]
                                             Rapport en lecture seule de l'aptitude de chaque fichier à
                                             la diffusion (index Cues, cues sur images clés,
                                             DefaultDuration) - ce sur quoi reposent le palier HLS à la
                                             demande et la navigation.
evey admin artwork-gc [--root <dir>] [--apply]
                                             Supprime du disque les fichiers d'illustration qu'aucune
                                             ligne de la base ne référence. À blanc par défaut. --root
                                             vaut /data/artworks par défaut.
evey admin reclassify <media-id>             Force le recalcul de la classe de complexité d'encodage.
evey admin reclassify --all                  Efface et remet en file tous les éléments classés.
```

### Diagnostic et capacité

```
evey admin doctor [--repair]                 Contrôle de santé : base, schéma, jwt.key, Meilisearch
                                             (dérive de l'index comprise), montages de médias
                                             (appartiennent-ils à l'UID du serveur ?), répertoires de
                                             cache, ffmpeg/mkvgo et disque. --repair n'applique que les
                                             correctifs sans risque (créer et chown les répertoires
                                             manquants qui doivent être accessibles en écriture).
evey admin rollback                          Conseiller en lecture seule : affiche la version qui
                                             tourne, le schéma, la sauvegarde d'avant migration la plus
                                             récente, et les commandes exactes à lancer SUR L'HÔTE pour
                                             revenir en arrière. Le conteneur n'a pas accès à Docker,
                                             par construction : il conseille au lieu d'agir.
evey admin load-test [minimal|standard|max] [--scenario <s>] [--max-viewers N] [--duration S]
                                             Exige un serveur VIVANT (c'est un client HTTP de la
                                             passerelle locale). Fait monter des spectateurs simulés
                                             jusqu'à ce que la machine atteigne son plafond, puis
                                             rapporte le nombre tenu, avec le TTFF et les latences de
                                             segment. Scénarios : transcode, natural, mkvgo, light
                                             (80/20), balanced (50/50, par défaut), heavy (20/80),
                                             mkvgo+N. --max-viewers borne la montée (0 = la valeur
                                             serveur, 12). Passage de vérification rapide :
                                             `load-test --scenario balanced --max-viewers 4`.
                                             `admission_full` est le verdict de plafond (sortie 0), pas
                                             une erreur.
```

> La liste complète, et toujours à jour, s'obtient en lançant `evey admin` sans
> sous-commande - elle affiche 2026-07-23 ; `reindex-no-cue`, la réparation de
> l'index de navigation qui est derrière les incidents récurrents de reprise
> lente, figurait parmi les absentes.

## Comment les invoquer

### Depuis l'hôte, la pile en marche (dev Docker ou production)

Le Makefile enveloppe les opérations les plus courantes dans des appels de shell
au conteneur, pour qu'elles fonctionnent sans sourcer le `.env` et sans pg_dump
sur l'hôte :

```sh
make migrate-version
make migrate-up
make migrate-down
```

Ces cibles appellent `docker exec -w /app evey-server sh -c 'go run ./cmd/evey admin migrate <op>'` :
`evey-server` doit donc tourner - démarrez-le d'abord avec `make dev` si ce n'est
pas le cas.

### Un shell direct dans le conteneur en marche

```sh
docker exec -it evey-server sh
/app # go run ./cmd/evey admin reset-password --user alice@example.com
```

### Sur une installation sans conteneur (hors Docker)

```sh
evey admin reset-password --user alice@example.com
evey admin migrate version
```

Le binaire déjà installé embarque toutes les sous-commandes.

## Le détail des sous-commandes

### `migrate`

- `up` applique dans l'ordre toutes les migrations en attente. Un `pg_dump`
  d'avant migration est pris automatiquement dès qu'`EVEY_BACKUP_DIR` est
  renseignée (par défaut dans l'image Docker : `/data/backups`). Un échec en
  cours d'Up laisse `schema_migrations.dirty = true` ; voir
  `migration-runbook.md`.
- `down` recule d'exactement une migration. C'est chirurgical, pas « on rase
  tout » : il n'existe pas de `down --all`. Prend également un `pg_dump` avant
  le retour arrière.
- `version` affiche la version de schéma courante. Rend un code de sortie 1,
  assorti d'un avertissement `DIRTY`, quand une migration précédente a échoué à
  mi-parcours, c'est-à-dire que certaines instructions `Up` ont tourné mais pas
  toutes.
- `--no-backup` saute le pg_dump, dans un sens comme dans l'autre. Utile sur une
  machine dépourvue de `postgres-client` - le cas courant du poste de
  développement WSL, où le Makefile passe déjà le drapeau.

### `reset-password`, `rotate-jwt-key`, `restore-backup`, `reindex-search`

Voir `docs/runbooks/recovery.md` pour la procédure complète, les prérequis et
l'état obtenu.

## Ce qui n'est *pas* dans `evey admin`

Trois binaires autonomes vivent à côté de `cmd/evey`, parce que ce ne sont pas
des commandes d'exploitation au sens de la récupération ou du schéma :

| Binaire | Rôle | Pourquoi il est à part |
|---|---|---|
| `cmd/intro-backfill` | Rafraîchir la détection de générique sur une tranche du catalogue après un changement de chaîne | **Client d'API** - il parle à `/api/v1/admin/media/{id}/detect-segments` muni d'un jeton porteur, hors du processus serveur. |
| `cmd/intro-diag` | Diagnostiquer pourquoi la détection de générique par pHash a fonctionné (ou non) sur une série donnée | Outil de **débogage pour développeur** - il lit les sprites de trickplay sur le disque et affiche une carte de correspondance de Hamming, image par image. Passe facultative de corrélation croisée audio. |
| `cmd/trickplay-verify` | Vérifier que chaque vignette de trickplay montre bien l'image de l'horodatage enregistré | Outil de **QA/CI** - il réextrait les images source par ffmpeg et les compare à la vignette par aHash. |

Ils sont livrés en binaires distincts parce qu'aucun n'a besoin de toute la
plomberie de configuration du serveur : ce sont des outils cadrés, à usage
unique, qui ne font pas partie de la surface de récupération.

## Runbooks liés

- `migration-runbook.md` - déploiement de schéma, retour arrière, sortie d'un état dirty.
- `recovery.md` - réinitialisation de mot de passe, rotation de la clé JWT, restauration de sauvegarde, réindexation de la recherche.
- `incident-runbook.md` - première réaction quand le serveur déraille.
