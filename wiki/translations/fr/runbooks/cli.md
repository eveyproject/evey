---
source_sha: 80ee38697d8d389ccbe3deabdb838a6d8be5fa0af92f9832ad8d150b574bdcb2
reviewed: true
---
# Ligne de commande { #command-line }

Le binaire du serveur sert aussi de ligne de commande d'exploitation :
`evey admin <sous-commande>` lance un outil de maintenance ou de restauration
avec la même configuration et la même base de données que le serveur, puis
s'arrête. Cette page liste les commandes et la façon de les lancer dans le
conteneur ; les procédures du quotidien (mot de passe perdu, rotation de clé,
restauration) sont dans [Restauration](recovery.md).

## Lancer une commande { #running-a-command }

Dans l'installation standard, les commandes se lancent dans le conteneur, qui
contient déjà le mot de passe de la base et la configuration `EVEY_*` :

```sh
docker exec evey-server evey admin doctor
docker exec -it evey-server bash      # un shell d'administration : evey admin ..., man evey
```

Sur une installation sans Docker, le binaire s'appelle directement
(`evey admin reset-password --user alice@example.com`) ; il lit les mêmes
variables `EVEY_*` que le serveur (`EVEY_DB_HOST`, `EVEY_DB_USER`,
`EVEY_DB_PASSWORD`, `EVEY_DB_NAME`, `EVEY_BACKUP_DIR`...). La plupart des
commandes ont besoin de la base ; aucune ne démarre le serveur, et la plupart
peuvent tourner pendant qu'il est arrêté.

| Invocation | Ce qu'elle fait |
| --- | --- |
| `evey` | Démarre le serveur. |
| `evey admin <sous-commande>` | Lance un outil d'exploitation et s'arrête. `evey admin` seul affiche la liste et sort avec le code 1 ; `evey admin <sous-commande> --help` détaille l'une d'elles. |
| `evey version` | Version, commit, date de construction. N'a besoin ni de configuration ni de base, et répond donc depuis un conteneur qui ne démarre pas. |
| `evey help` | Le résumé des commandes. |
| `evey man [--text]` | Le manuel : au format roff pour `man`, en texte brut avec `--text`. |
| `evey completion bash` | Le script de complétion bash. |

Tout autre premier argument est refusé (code de sortie 1) plutôt que de
démarrer le serveur en silence.

## Les commandes { #the-commands }

La liste ci-dessous reprend le texte `--help` du binaire lui-même, en anglais,
groupé comme `evey admin` l'affiche. `evey admin` sans sous-commande est
toujours la liste à jour.

### Restauration { #recovery }

```
evey admin reset-password --user <email>     Generate a new password for the user, print it once.
                                             Every session of that user is ended at the same time.
evey admin rotate-jwt-key                    Regenerate the JWT signing key. Invalidates ALL active
                                             sessions: the key is moved aside (kept as
                                             <key>.rotated-<timestamp>), the next server boot
                                             generates a new one, and every session is ended in the
                                             database at once.
evey admin backup [--include-telemetry]
                                             Create a database backup in the backup directory, the
                                             same .tar.gz as the panel and the scheduler. Telemetry
                                             follows the panel switch (off by default);
                                             --include-telemetry includes it for this run.
evey admin restore-backup <path-to-dump.sql.gz>
                                             Restore the database from a pg_dump.gz file, in a single
                                             transaction. The whole file is checked first: a truncated
                                             or corrupt dump is refused before anything is dropped.
                                             The dump drops and recreates each table before loading
                                             it, so it can be restored onto a populated database;
                                             waits 5 s for a Ctrl-C
                                             before it starts. The .sql.gz suffix is required (not the
                                             panel's .tar.gz backups).
evey admin reindex-search [--library <id>] [--purge]
                                             Rebuild the Meilisearch index from the database (no
                                             metadata fetch). --purge clears the index first, which
                                             drops orphan documents. If Meilisearch is down, search
                                             is served by the Postgres fallback: the command reports
                                             that and does nothing.
evey admin demo-seed [--apply] [--media]     Run the demo bootstrap by hand: the demo administrator,
                                             the visitor accounts, the four libraries, and with
                                             --media the freely licensed media (gigabytes). Refuses
                                             unless EVEY_DEMO=1. Dry-run by default.
```

`reset-password` et `rotate-jwt-key` mettent tous deux fin à des sessions :
celles de l'utilisateur pour le premier, celles de tout le monde pour le
second. Les applications ouvertes s'en aperçoivent en 30 secondes environ et
redemandent une connexion. Les procédures complètes sont dans
[Restauration](recovery.md).

### Schéma { #schema }

```
evey admin migrate up [--no-backup]          Apply every pending migration.
evey admin migrate down [--no-backup]        Roll back exactly one migration step.
evey admin migrate version                   Print the current schema version; exit 1 when a
                                             previous migration failed halfway (dirty state).
```

Un `pg_dump` d'avant migration est pris pour `up` et `down` quand
`EVEY_BACKUP_DIR` est défini (`/data/backups` dans l'image Docker) ;
`--no-backup` l'omet, pour les machines sans `pg_dump`. Un échec en cours de
`up` laisse `schema_migrations.dirty = true` : voir les
[Procédures de migration](migration-runbook.md). Sur la pile de
développement, le Makefile fournit ces trois commandes sous la forme `make migrate-up`,
`make migrate-down` et `make migrate-version`, exécutées dans le conteneur de
développement `evey-server`.

### Maintenance des médias { #media-maintenance }

```
evey admin reindex-no-cue --root <dir> [--apply] [--limit N] [--mode auto|inplace|rewrite] [--deep-verify] [--resync] [--keep-backup=false]
                                             Rebuild the seek index (Cues) of MKV/WebM files that
                                             lack one, fixing slow resume. DRY-RUN BY DEFAULT: pass
                                             --apply to rewrite. Each rewrite is validated and
                                             cross-checked and keeps a .mkvgobak backup; --limit N for
                                             a cautious first run; --keep-backup=false drops the
                                             backups and enforces --deep-verify instead. --mode auto
                                             takes the surgical in-place patch when the file layout
                                             allows it and rewrites otherwise; inplace patches only
                                             (no backup); rewrite always rebuilds and swaps.
                                             --resync repairs files with a CORRUPTED region the strict
                                             pass refuses: it skips the damage, keeps the rest and
                                             reports the lost bytes. LONG: rewrites each file in full
                                             (hours on a large library); shows a progress bar.
evey admin validate-streaming --root <dir> [--errors-only]
                                             Read-only report of each file's streaming readiness: a
                                             missing Cues index, cue points on a non-video track, cue
                                             times matching no keyframe, video tracks without a
                                             DefaultDuration. A file with errors falls back to the
                                             ffmpeg pipeline. --errors-only skips the warnings.
evey admin artwork-gc [--root <dir>] [--apply]
                                             Delete artwork files on disk that no DB row references.
                                             Dry-run by default. --root defaults to /data/artworks.
```

Le même moteur est utilisé par **Administration → Santé des médias**, qui est la
façon la plus simple de réparer une bibliothèque : voir
[Santé des médias](media-health.md).

### Encodage par titre { #per-title-encoding }

```
evey admin reclassify <media-id>             Force-recompute the complexity class of one media item.
evey admin reclassify --all                  Wipe every classified row; the background worker then
                                             re-classifies the whole library at its background pace.
```

### Diagnostic { #diagnostics }

```
evey admin doctor [--repair]                 Health-check DB, schema, jwt.key, Meilisearch (plus
                                             index drift), media mounts (can the server write each
                                             library root?), cache dirs, ffmpeg/mkvgo, trusted
                                             proxies, remote access and disk. One line per check
                                             (ok / WARN / FAIL) with a remediation hint; exits 1 when
                                             a check fails. --repair applies only the safe fixes
                                             (create and chown the missing writable dirs).
evey admin rollback                          Read-only advisor: running version, schema, newest
                                             pre-migration backup and the exact HOST commands to roll
                                             back. The container has no Docker access, by design, so
                                             it advises rather than acts.
evey admin route-bench [--backend nvenc|vaapi|qsv|none] [--heights 1080,2160] [--fps 24,30,50,60] [--out rows.csv] [--json run.json] [<file>...]
                                             Measure what a session of each transcode route costs on
                                             this host against the calibration and the cost model.
                                             Minutes of full load; never while serving. More flags
                                             (--device --routes --classes --duration --workdir
                                             --capability): the full list is in
                                             `evey admin route-bench --help`.
evey admin ladder-bench [--scenes 3] [--window 8] [--heights 1080,720] [--crf 18,23,28] [--out rows.csv] [--efficiency --mode full|fast|incremental] <file>...
                                             Encode a few scenes of each title at fixed CRFs and
                                             score encode cost against the ladder's complexity
                                             features. Runs ffmpeg for minutes; never while serving.
                                             More flags (--preset --ref-height --ref-crf --cache
                                             --encoders --device): the full list is in
                                             `evey admin ladder-bench --help`.
evey admin ladder-probe [--window 8] [--cache <dir>] [--out <dir>] <file>...
                                             Run the per-title probe on the given files: probe time,
                                             early-stop rate and points. Reads and encodes only.
```

### Capacité { #capacity }

```
evey admin load-test [minimal|standard|max] [--scenario <s>] [--mode <m>] [--max-viewers N] [--duration S] [--cold-cache]
                                             Needs a LIVE server: it is an HTTP client of the local
                                             gateway. Ramps simulated viewers until the box hits its
                                             ceiling, then reports the sustained count and latencies.
                                             Scenarios: transcode, natural, mkvgo, light (80/20),
                                             balanced (50/50, default), heavy (20/80), mkvgo+N.
                                             Modes: ramp (default), flash, seek, endurance. Presets:
                                             minimal (4 viewers), standard (12), max (32); an explicit
                                             --max-viewers wins. --cold-cache keeps evicting the
                                             library from the page cache so the run measures the
                                             storage, not the RAM. admission_full is the ceiling
                                             verdict (exit 0), not an error. Short sanity run:
                                             load-test --scenario balanced --max-viewers 4
```

### Sources de complétion { #completion-sources }

```
evey admin list-users                        Print user emails, one per line (drives --user completion).
evey admin list-libraries                    Print every library root path, one per line (drives --root completion).
```

## Complétion et manuel { #tab-completion-and-the-manual }

Le binaire porte sa propre complétion bash et son propre manuel, qui
correspondent donc toujours aux sous-commandes qu'il contient :

```sh
evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null && sudo mandb -q
```

Sous zsh : lancez d'abord `autoload -U +X bashcompinit && bashcompinit`, puis
chargez le même script. La complétion couvre les sous-commandes, leurs options
et les valeurs de ces options (`--mode`, `--backend`, `--scenario`), les
adresses e-mail des utilisateurs après `--user`, les racines de bibliothèques
que connaît le serveur après `--root` (plus tout dossier), uniquement des
fichiers de médias pour les arguments de fichier de `ladder-bench`,
`ladder-probe` et `route-bench`, et des dumps `.sql.gz` pour `restore-backup`.

L'image Docker installe la complétion à la construction, et son shell
d'administration (`docker exec -it evey-server bash`) répond à `man evey` en
affichant `evey man --text` page par page, puisque l'image ne contient pas de
binaire `man`.

## Ce qui n'est pas dans `evey admin` { #what-is-not-in-evey-admin }

Trois programmes autonomes se trouvent à côté du serveur dans les sources. Ce sont
des outils de développement et de QA, pas des commandes d'exploitation, et
l'image Docker ne les contient pas :

| Programme | Rôle |
| --- | --- |
| `cmd/intro-backfill` | Relancer la détection des intros sur une partie du catalogue après un changement du pipeline. Un client de l'API avec un jeton, pas dans le processus du serveur. |
| `cmd/intro-diag` | Expliquer pourquoi la détection d'intro a trouvé, ou non, une correspondance sur une série, à partir des planches de miniatures sur le disque. |
| `cmd/trickplay-verify` | Vérifier que chaque vignette de miniature montre bien l'image de l'horodatage enregistré, en réextrayant l'image source avec ffmpeg. |

## Runbooks liés { #related-runbooks }

- [Restauration](recovery.md) : réinitialisation de mot de passe, rotation de la clé JWT, restauration d'une sauvegarde.
- [Procédures de migration](migration-runbook.md) : déploiement du schéma, retour en arrière, récupération d'un état « dirty ».
- [Procédures d'incident](incident-runbook.md) : les premiers gestes quand le serveur présente un problème.
