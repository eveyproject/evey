---
source_sha: 4da6d87a4495603598041edc6216fa889229d188f8579a3799818f7ddbf73246
reviewed: true
---
# कमांड लाइन

सर्वर का प्रोग्राम ही ऑपरेटर की कमांड लाइन का काम भी करता है: `evey admin
<subcommand>` रखरखाव या रिकवरी का एक टूल चलाता है, उसी कॉन्फ़िगरेशन और डेटाबेस
पर जो सर्वर इस्तेमाल करता है, और फिर बंद हो जाता है। यह पेज कमांड की सूची देता है
और बताता है कि उन्हें कंटेनर में कैसे चलाएँ; रोज़ के काम (भूला हुआ पासवर्ड, कुंजी
बदलना, बैकअप से वापस लाना) [रिकवरी](recovery.md) में हैं।

## कमांड चलाना

आम इंस्टॉलेशन में कमांड कंटेनर के अंदर चलती हैं, जिसमें डेटाबेस का पासवर्ड और
`EVEY_*` कॉन्फ़िगरेशन पहले से होते हैं:

```sh
docker exec evey-server evey admin doctor
docker exec -it evey-server bash      # एक एडमिन शेल: evey admin ..., man evey
```

Docker के बिना वाले इंस्टॉल पर प्रोग्राम को सीधे बुलाया जाता है
(`evey admin reset-password --user alice@example.com`); यह वही `EVEY_*`
वेरिएबल पढ़ता है जो सर्वर पढ़ता है (`EVEY_DB_HOST`, `EVEY_DB_USER`,
`EVEY_DB_PASSWORD`, `EVEY_DB_NAME`, `EVEY_BACKUP_DIR`...)। ज़्यादातर कमांड को
डेटाबेस चाहिए; कोई भी कमांड सर्वर शुरू नहीं करती, और ज़्यादातर को सर्वर बंद होने
पर भी चलाना सुरक्षित है।

| कैसे चलाएँ | यह क्या करता है |
| --- | --- |
| `evey` | सर्वर शुरू करता है। |
| `evey admin <subcommand>` | ऑपरेटर का एक टूल चलाता है और बंद हो जाता है। सिर्फ़ `evey admin` सूची छापता है और exit 1 देता है; `evey admin <subcommand> --help` किसी एक का विस्तार बताता है। |
| `evey version` | वर्ज़न, कमिट, बिल्ड की तारीख। इसे न कॉन्फ़िगरेशन चाहिए न डेटाबेस, इसलिए यह उस कंटेनर से भी जवाब देता है जो शुरू नहीं हो रहा। |
| `evey help` | कमांड का सारांश। |
| `evey man [--text]` | मैनुअल: `man` के लिए roff, `--text` के साथ सादा टेक्स्ट। |
| `evey completion bash` | bash की ऑटो-कम्प्लीशन स्क्रिप्ट। |

कोई भी दूसरा पहला आर्ग्युमेंट मना कर दिया जाता है (exit 1), बजाय इसके कि सर्वर
चुपचाप शुरू हो जाए।

## कमांड

नीचे की सूची प्रोग्राम का अपना `--help` टेक्स्ट है, उन्हीं समूहों में जैसे
`evey admin` उसे छापता है। बिना सबकमांड के `evey admin` हमेशा मौजूदा सूची देता है।

### रिकवरी

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
                                             Existing rows are OVERWRITTEN; waits 5 s for a Ctrl-C
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

`reset-password` और `rotate-jwt-key` दोनों सेशन खत्म करते हैं: पहला उस
उपयोगकर्ता के, दूसरा सबके। खुले ऐप लगभग 30 सेकंड के अंदर इसका पता लगा लेते हैं और
फिर से साइन इन करने को कहते हैं। पूरे तरीके [रिकवरी](recovery.md) में हैं।

### स्कीमा

```
evey admin migrate up [--no-backup]          Apply every pending migration.
evey admin migrate down [--no-backup]        Roll back exactly one migration step.
evey admin migrate version                   Print the current schema version; exit 1 when a
                                             previous migration failed halfway (dirty state).
```

`up` और `down` के लिए माइग्रेशन से पहले एक `pg_dump` लिया जाता है, जब
`EVEY_BACKUP_DIR` सेट हो (Docker इमेज में `/data/backups`); `--no-backup` इसे
छोड़ देता है, उन होस्ट के लिए जिनमें `pg_dump` नहीं है। `up` के बीच में फ़ेल
होने पर `schema_migrations.dirty = true` रह जाता है: देखें
[डेटाबेस माइग्रेशन](migration-runbook.md)। डेवलपमेंट स्टैक पर Makefile इन तीनों
को `make migrate-up`, `make migrate-down` और `make migrate-version` के रूप में
चलाता है, `evey-server` डेव कंटेनर में।

### मीडिया का रखरखाव

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

यही इंजन **एडमिन → मीडिया स्वास्थ्य** के पीछे भी चलता है, जो किसी लाइब्रेरी की
मरम्मत का आसान तरीका है: देखें [मीडिया स्वास्थ्य](media-health.md)।

### हर टाइटल की एन्कोडिंग

```
evey admin reclassify <media-id>             Force-recompute the complexity class of one media item.
evey admin reclassify --all                  Wipe every classified row; the background worker then
                                             re-classifies the whole library at its background pace.
```

### जाँच

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
                                             Minutes of full load; never while serving.
evey admin ladder-bench [--scenes 3] [--window 8] [--heights 1080,720] [--crf 18,23,28] [--out rows.csv] [--efficiency --mode full|fast|incremental] <file>...
                                             Encode a few scenes of each title at fixed CRFs and
                                             score encode cost against the ladder's complexity
                                             features. Runs ffmpeg for minutes; never while serving.
evey admin ladder-probe [--window 8] [--cache <dir>] [--out <dir>] <file>...
                                             Run the per-title probe on the given files: probe time,
                                             early-stop rate and points. Reads and encodes only.
```

### क्षमता

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

### कम्प्लीशन के सोर्स

```
evey admin list-users                        Print user emails, one per line (drives --user completion).
evey admin list-libraries                    Print every library root path, one per line (drives --root completion).
```

## टैब-कम्प्लीशन और मैनुअल

प्रोग्राम में अपनी bash कम्प्लीशन और अपना मैनुअल होता है, इसलिए दोनों हमेशा उन
सबकमांड से मेल खाते हैं जो उसमें हैं:

```sh
evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null && sudo mandb -q
```

zsh इस्तेमाल करने वाले: पहले `autoload -U +X bashcompinit && bashcompinit`, फिर
वही स्क्रिप्ट source करें। कम्प्लीशन इन्हें पूरा करती है: सबकमांड, उनके फ़्लैग और
फ़्लैग के मान (`--mode`, `--backend`, `--scenario`), `--user` के बाद उपयोगकर्ताओं
के ईमेल, `--root` के बाद वे लाइब्रेरी रूट जिन्हें सर्वर जानता है (और कोई भी
डायरेक्टरी), `ladder-bench`, `ladder-probe` और `route-bench` के फ़ाइल वाले
आर्ग्युमेंट के लिए सिर्फ़ मीडिया फ़ाइलें, और `restore-backup` के लिए `.sql.gz`
डंप।

Docker इमेज बिल्ड के समय कम्प्लीशन इंस्टॉल करती है, और उसका एडमिन शेल
(`docker exec -it evey-server bash`) `man evey` का जवाब `evey man --text` को
पेज-दर-पेज दिखाकर देता है, क्योंकि इमेज में `man` प्रोग्राम नहीं है।

## `evey admin` में क्या नहीं है

सोर्स कोड में सर्वर के बगल में तीन अलग प्रोग्राम हैं। ये डेवलपर और QA के टूल हैं,
ऑपरेटर की कमांड नहीं, और Docker इमेज में ये नहीं आते:

| प्रोग्राम | काम |
| --- | --- |
| `cmd/intro-backfill` | पाइपलाइन में बदलाव के बाद कैटलॉग के एक हिस्से पर इंट्रो की पहचान फिर से करना। यह bearer टोकन वाला एक API क्लाइंट है, सर्वर के प्रोसेस के अंदर नहीं चलता। |
| `cmd/intro-diag` | डिस्क पर रखे trickplay स्प्राइट से समझाना कि किसी एक सीरीज़ पर इंट्रो की पहचान क्यों मिली, या क्यों नहीं मिली। |
| `cmd/trickplay-verify` | जाँचना कि हर trickplay टाइल अपने दर्ज समय वाला फ़्रेम दिखाती है, ffmpeg से सोर्स फ़्रेम फिर से निकालकर। |

## संबंधित रनबुक

- [रिकवरी](recovery.md): पासवर्ड रीसेट, JWT कुंजी बदलना, बैकअप से वापस लाना।
- [डेटाबेस माइग्रेशन](migration-runbook.md): स्कीमा डिप्लॉय, रोलबैक, dirty हालत से निकलना।
- [इंसिडेंट रनबुक](incident-runbook.md): जब सर्वर ठीक से काम न करे, तब पहले क्या करें।

<!-- sources
dispatch and top-level commands: cmd/evey/cli.go - runCLI (version, help, completion, man, unknown command refused), cmd/evey/main.go
subcommand table, groups, --help text: cmd/evey/admin_commands.go - adminCommands, adminGroups, printAdminUsage, adminUsage (exit 1 without subcommand) ; drift test cmd/evey/admin_cli_test.go
sessions ended: cmd/evey/admin.go - adminResetPassword (DeleteByUserID + BumpTokenEpoch), adminRotateJWTKey, endAllSessions ; epoch cache TTL internal/app/user/token_epoch.go
backup and restore: cmd/evey/admin_backup.go - adminBackup ; cmd/evey/admin.go - adminRestoreBackup, verifyDumpComplete
container: Dockerfile (EVEY_DB_PASSWORD_FILE, EVEY_BACKUP_DIR=/data/backups, completion at build time) ; docker/evey-shell.sh - man()
makefile: Makefile migrate-up / migrate-down / migrate-version
standalone programs: cmd/intro-backfill, cmd/intro-diag, cmd/trickplay-verify
-->
