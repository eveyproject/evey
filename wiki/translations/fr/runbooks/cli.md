---
source_sha: 7a130827b0ba31330a54269b3d9e83b6ddd534f47a8009c82c6945df914cb007
reviewed: true
---
# La CLI d'Evey : `evey admin`

Le binaire du serveur Evey sert aussi de CLI d'exploitation. Lancé sans
argument, `evey` démarre le serveur HTTP/gRPC ; avec `admin` en premier argument,
il passe en mode outil d'exploitation, exécute une commande et s'arrête.

Toutes les commandes lisent leur configuration dans les mêmes variables `EVEY_*`
que le serveur (`EVEY_DB_USER`, `EVEY_DB_PASSWORD`, `EVEY_DB_HOST`,
`EVEY_DB_PORT`, `EVEY_DB_NAME`, `EVEY_DB_SSLMODE`, `EVEY_BACKUP_DIR`, ...).

```
evey                          Start the server.
evey admin <subcommand>       One-shot operator tool (below). `evey admin` alone lists them,
                              `evey admin <subcommand> --help` details one.
evey version                  Version, commit, build date - needs no config or database.
evey help                     Command summary.
evey man [--text]             The manual: roff for `man`, plain text with --text.
evey completion bash          The bash completion script.
```

Tout autre premier argument est refusé (code de sortie 1) plutôt que de démarrer
le serveur en silence. Le tableau Go de `cmd/evey/admin_commands.go` est la
description unique de chaque sous-commande : la liste d'usage, `--help`, le
manuel, le script de complétion et le shell du conteneur sont vérifiés par
rapport à lui dans `cmd/evey/admin_cli_test.go`.

Les textes d'aide ci-dessous sont ceux que le binaire affiche, en anglais.

## Référence rapide

```
evey admin reset-password --user <email>     Generate a new password, print it once.
evey admin rotate-jwt-key                    Regenerate JWT signing key (kills ALL sessions).
evey admin restore-backup <path>             Restore the database from a .sql.gz dump (the suffix is required).
evey admin reindex-search [--library <id>] [--purge]   Rebuild the Meilisearch index from Postgres (--purge clears orphan docs first; if Meili is down, reports the Postgres fallback and does nothing).
evey admin migrate up [--no-backup]          Apply pending migrations.
evey admin migrate down [--no-backup]        Roll back one migration step.
evey admin migrate version                   Print current schema version (exit 1 if dirty).
evey admin list-users                        Print user emails, one per line (feeds `--user` completion).
evey admin list-libraries                    Print the library root paths, one per line (feeds `--root` completion).
```

### Maintenance des médias

```
evey admin reindex-no-cue --root <dir> [--apply] [--limit N] [--resync] [--keep-backup=false]
                                             Rebuild the seek index (Cues) of MKV/WebM files that
                                             lack one - the fix for slow seek and slow resume.
                                             DRY-RUN BY DEFAULT: pass --apply to rewrite. Each
                                             rewrite is validated, cross-checked, and keeps a
                                             .mkvgobak backup. --limit N for a cautious first run.
                                             --resync also repairs files with a corrupted region the
                                             strict pass refuses (skips the damage, keeps the rest,
                                             reports the lost bytes).
                                             LONG: rewrites each file in full, hours on a large
                                             library. Shows a progress bar while applying.
evey admin validate-streaming --root <dir> [--errors-only]
                                             Read-only report of each file's streaming readiness
                                             (Cues index, keyframe cues, DefaultDuration) - what the
                                             on-demand HLS rung and seeking rely on.
evey admin artwork-gc [--root <dir>] [--apply]
                                             Delete artwork files on disk that no DB row references.
                                             Dry-run by default. --root defaults to /data/artworks.
evey admin reclassify <media-id>             Force-recompute the encoding complexity class.
evey admin reclassify --all                  Wipe and re-queue every classified item.
```

### Diagnostic et capacité

```
evey admin doctor [--repair]                 Health-check DB, schema, jwt.key, Meilisearch (plus
                                             index drift), media mounts (owned by the server UID?),
                                             cache dirs, ffmpeg/mkvgo and disk. --repair applies only
                                             the safe fixes (create + chown missing writable dirs).
evey admin rollback                          Read-only advisor: prints the running version, schema,
                                             newest pre-migration backup and the exact HOST commands
                                             to roll back. The container has no Docker access, by
                                             design, so it advises rather than acts.
evey admin load-test [minimal|standard|max] [--scenario <s>] [--mode <m>] [--max-viewers N] [--duration S] [--cold-cache]
                                             Needs a LIVE server (it is an HTTP client of the local
                                             gateway). Ramps simulated viewers until the box hits its
                                             ceiling, then reports the sustained count with TTFF and
                                             segment latencies. Scenarios: transcode, natural, mkvgo,
                                             light (80/20), balanced (50/50, default), heavy (20/80),
                                             mkvgo+N. --max-viewers caps the ramp (0 = server default
                                             12). Short sanity run:
                                             `load-test --scenario balanced --max-viewers 4`.
                                             `admission_full` is the ceiling verdict (exit 0), not an
                                             error.
evey admin route-bench [--backend nvenc|vaapi|qsv|none] [--heights ...] [--fps ...] [--out rows.csv] [--json run.json] [<file>...]
                                             Measure what a session of each transcode route costs on
                                             this host against the calibration and the admission cost
                                             model. Minutes of full load; never while serving.
evey admin ladder-bench [--scenes 3] [--window 8] [--heights ...] [--crf ...] [--out rows.csv] <file>...
                                             Ladder complexity features on a few scenes per title,
                                             encoded at fixed CRFs and scored. Never while serving.
evey admin ladder-probe [--window 8] [--cache <dir>] [--out <dir>] <file>...
                                             Per-title probe timing and early-stop rate on the files.
```

> La liste complète et toujours à jour est `evey admin` sans sous-commande : il
> affiche toutes les sous-commandes que contient le binaire. Cette page en
> oubliait plusieurs jusqu'au 23 juillet 2026 ; `reindex-no-cue`, la réparation
> de l'index de lecture derrière les incidents récurrents de reprise lente,
> en faisait partie.

## Comment l'appeler

### Depuis la machine hôte, la pile en marche (Docker, dev ou production)

Le Makefile enveloppe les opérations les plus courantes dans des appels au shell
du conteneur, pour qu'elles fonctionnent sans charger le `.env` et sans pg_dump
sur la machine hôte :

```sh
make migrate-version
make migrate-up
make migrate-down
```

Elles exécutent `docker exec -w /app evey-server sh -c 'go run ./cmd/evey admin migrate <op>'` :
`evey-server` doit donc tourner ; lancez-le d'abord avec `make dev` si ce n'est
pas le cas.

### Un shell directement dans le conteneur en marche

```sh
docker exec -it evey-server sh
/app # go run ./cmd/evey admin reset-password --user alice@example.com
```

### Sur une installation sans Docker

```sh
evey admin reset-password --user alice@example.com
evey admin migrate version
```

Le binaire installé contient déjà toutes les sous-commandes.

## Complétion et manuel

Le binaire porte sa propre complétion bash et son propre manuel, qui
correspondent donc toujours aux sous-commandes qu'il contient :

```sh
evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null && sudo mandb -q
```

Sous zsh : lancez d'abord `autoload -U +X bashcompinit && bashcompinit`, puis
chargez le même script. La complétion couvre les sous-commandes, leurs options et
les valeurs de ces options (`--mode`, `--backend`, `--scenario`), les adresses
e-mail des utilisateurs après `--user`, les racines de bibliothèques que connaît
le serveur après `--root` (plus tout dossier), uniquement des fichiers de médias
pour les arguments de fichier de `ladder-bench`, `ladder-probe` et `route-bench`,
et des dumps `.sql.gz` pour `restore-backup`.

L'image Docker installe la complétion à la construction, et son shell
d'administration (`docker exec -it evey-server bash`) répond à `man evey` en
affichant `evey man --text` page par page, puisque l'image ne contient pas de
binaire `man`.

## Détail des sous-commandes

### `migrate`

- `up` applique dans l'ordre chaque migration en attente. Un `pg_dump` d'avant
  migration est pris automatiquement quand `EVEY_BACKUP_DIR` est défini (par
  défaut dans l'image Docker : `/data/backups`). Un échec en cours de route
  laisse `schema_migrations.dirty = true` ; voir `migration-runbook.md`.
- `down` revient exactement d'une migration. Chirurgical, pas « tout
  démolir » : il n'y a pas de `down --all`. Prend aussi un `pg_dump` avant le
  retour en arrière.
- `version` affiche la version actuelle du schéma. Renvoie le code de sortie 1
  avec un avertissement `DIRTY` quand une migration précédente a échoué à
  mi-chemin (certaines instructions `Up` ont tourné, pas toutes).
- `--no-backup` saute le pg_dump dans les deux sens. Utile sur les machines sans
  `postgres-client` (typiquement le développement sous WSL, où le Makefile passe
  déjà l'option).

### `reset-password`, `rotate-jwt-key`, `restore-backup`, `reindex-search`

Voir `docs/runbooks/recovery.md` pour la procédure complète, les prérequis et
l'état obtenu ensuite.

## Ce qui n'est *pas* dans `evey admin`

Trois binaires autonomes vivent à côté de `cmd/evey`, parce que ce ne sont pas
des commandes d'exploitation au sens de la récupération ou du schéma :

| Binaire | Rôle | Pourquoi à part |
|---|---|---|
| `cmd/intro-backfill` | Relancer la détection des génériques sur une partie du catalogue après un changement du pipeline | **Client de l'API** : il appelle `/api/v1/admin/media/{id}/detect-segments` avec un jeton, pas dans le processus du serveur. |
| `cmd/intro-diag` | Comprendre pourquoi la détection de générique par pHash a trouvé (ou non) une correspondance sur une série | Outil de **débogage pour développeurs** : il lit les planches de miniatures de navigation sur le disque et affiche, image par image, la carte des distances de Hamming. Passe facultative de corrélation croisée de l'audio. |
| `cmd/trickplay-verify` | Vérifier que chaque vignette de miniature de navigation montre bien l'image de l'horodatage enregistré | Outil de **QA/CI** : il réextrait les images de la source avec ffmpeg et les compare à la vignette par aHash. |

Ils sont livrés en binaires séparés parce qu'aucun n'a besoin de toute la
configuration du serveur : ce sont des outils ciblés, à usage ponctuel, qui ne
font pas partie de la surface de récupération.

## Runbooks liés

- `migration-runbook.md` : déploiement du schéma, retour en arrière,
  récupération d'un état « dirty ».
- `recovery.md` : réinitialisation de mot de passe, rotation de la clé JWT,
  restauration d'une sauvegarde, réindexation de la recherche.
- `incident-runbook.md` : les premiers gestes quand le serveur se comporte mal.
