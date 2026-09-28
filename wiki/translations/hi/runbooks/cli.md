---
source_sha: 0cbab39ffa95c9d5b92240456049d1f9bc27697af69bc1423e9f3c1f3d4a7556
---

# Evey CLI - `evey admin`

Evey सर्वर की बाइनरी ही संचालक की CLI का काम भी करती है। `evey` को बिना किसी
argument के चलाएँ, तो वह HTTP/gRPC सर्वर शुरू कर देती है; पहले argument के तौर पर
`admin` दें, तो वह एक-बार-चलने वाले संचालक मोड में चली जाती है और कमांड पूरी होते
ही बाहर निकल जाती है।

सारी कमांड अपना कॉन्फ़िगरेशन उन्हीं `EVEY_*` env वेरिएबल से पढ़ती हैं जिनसे सर्वर
खुद पढ़ता है (`EVEY_DB_USER`, `EVEY_DB_PASSWORD`, `EVEY_DB_HOST`, `EVEY_DB_PORT`,
`EVEY_DB_NAME`, `EVEY_DB_SSLMODE`, `EVEY_BACKUP_DIR`, ...)।

```
evey                          Start the server.
evey admin <subcommand>       One-shot operator tool (below). `evey admin` alone lists them,
                              `evey admin <subcommand> --help` details one.
evey version                  Version, commit, build date - needs no config or database.
evey help                     Command summary.
evey man [--text]             The manual: roff for `man`, plain text with --text.
evey completion bash          The bash completion script.
```

कोई भी दूसरा पहला आर्ग्युमेंट चुपचाप सर्वर शुरू करने के बजाय अस्वीकार कर दिया
जाता है (exit 1)। `cmd/evey/admin_commands.go` की Go तालिका हर subcommand का
इकलौता विवरण है: usage सूची, `--help`, manual, completion script और container
shell, सब `cmd/evey/admin_cli_test.go` में उसी के सामने जाँचे जाते हैं।

## एक नज़र में

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

### मीडिया का रखरखाव

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

### जाँच और क्षमता

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

> पूरी और हमेशा ताज़ा सूची पाने के लिए `evey admin` बिना किसी subcommand के चलाएँ -
> वह छापता है
> 2026-07-23; `reindex-no-cue`, यानी वह seek-index मरम्मत जो बार-बार होने वाली धीमे
> resume की घटनाओं के पीछे थी, उन्हीं छूटी हुई कमांड में से एक थी।

## चलाने के तरीक़े

### होस्ट से, जब पूरा stack चल रहा हो (Docker dev / उत्पादन)

Makefile सबसे आम कामों को container के shell में लपेट देता है, ताकि वे बिना `.env`
पढ़े और बिना होस्ट पर pg_dump हुए चल जाएँ:

```sh
make migrate-version
make migrate-up
make migrate-down
```

ये असल में
`docker exec -w /app evey-server sh -c 'go run ./cmd/evey admin migrate <op>'`
चलाते हैं, इसलिए `evey-server` चालू होना चाहिए - न चल रहा हो तो पहले `make dev` से
शुरू करें।

### चलते container में सीधे shell खोलकर

```sh
docker exec -it evey-server sh
/app # go run ./cmd/evey admin reset-password --user alice@example.com
```

### कंटेनर के बिना इंस्टॉलेशन पर (Docker के बिना)

```sh
evey admin reset-password --user alice@example.com
evey admin migrate version
```

पहले से इंस्टॉल की हुई बाइनरी में हर subcommand पहले से मौजूद है।

## Tab-completion और manual

बाइनरी अपना bash completion और manual ख़ुद साथ रखती है, इसलिए दोनों हमेशा उन्हीं
subcommands से मेल खाते हैं जो वह लेकर आती है:

```sh
evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null && sudo mandb -q
```

zsh उपयोगकर्ता: पहले `autoload -U +X bashcompinit && bashcompinit`, फिर वही
script source करें। completion में subcommands, उनके flags और flag के मान
(`--mode`, `--backend`, `--scenario`), `--user` के बाद उपयोगकर्ताओं के ईमेल,
`--root` के बाद सर्वर को ज्ञात लाइब्रेरी roots (और कोई भी डायरेक्टरी),
`ladder-bench`, `ladder-probe` और `route-bench` के फ़ाइल आर्ग्युमेंट के लिए सिर्फ़
मीडिया फ़ाइलें, और `restore-backup` के लिए `.sql.gz` dumps आते हैं।

Docker इमेज build के समय completion इंस्टॉल कर देती है, और उसका admin shell
(`docker exec -it evey-server bash`) `man evey` का जवाब `evey man --text` को पन्ना
दर पन्ना दिखाकर देता है, क्योंकि इमेज में `man` बाइनरी नहीं है।

## Subcommand का ब्योरा

### `migrate`

- `up` हर बचे हुए माइग्रेशन को क्रम से लगाता है। `EVEY_BACKUP_DIR` सेट हो, तो
  माइग्रेशन से पहले एक `pg_dump` अपने आप ले लिया जाता है (Docker image में डिफ़ॉल्ट:
  `/data/backups`)। Up के बीच में विफल होने पर `schema_migrations.dirty = true` रह
  जाता है; `migration-runbook.md` देखें।
- `down` ठीक एक माइग्रेशन वापस पलटता है। यह सर्जरी जैसा है, "सब कुछ गिरा दो" जैसा
  नहीं - `down --all` जैसा कुछ है ही नहीं। यह भी पलटने से पहले एक `pg_dump` ले लेता
  है।
- `version` मौजूदा स्कीमा संस्करण छापता है। पिछला माइग्रेशन बीच में विफल हुआ हो
  (यानी `Up` के कुछ statement चले, सब नहीं), तो यह `DIRTY` चेतावनी के साथ exit code
  1 लौटाता है।
- `--no-backup` दोनों दिशाओं में pg_dump छोड़ देता है। यह उन होस्ट पर काम आता है
  जहाँ `postgres-client` इंस्टॉल नहीं है (WSL पर dev का यही आम रास्ता है, जहाँ
  Makefile यह फ़्लैग पहले से देता है)।

### `reset-password`, `rotate-jwt-key`, `restore-backup`, `reindex-search`

पूरी प्रक्रिया, पूर्व-शर्तें और बाद की स्थिति `docs/runbooks/recovery.md` में देखें।

## `evey admin` में जो *नहीं* है

`cmd/evey` के बगल में तीन अलग बाइनरी रहती हैं, क्योंकि वे उबरने या स्कीमा वाले
अर्थ में संचालक की कमांड नहीं हैं:

| बाइनरी | किसलिए | अलग क्यों |
|---|---|---|
| `cmd/intro-backfill` | शृंखला में कोई बदलाव होने के बाद, कैटलॉग के एक हिस्से पर intro पहचान दोबारा चलाना | **API क्लाइंट** - यह bearer टोकन के साथ `/api/v1/admin/media/{id}/detect-segments` से बात करता है, प्रक्रिया के भीतर नहीं चलता। |
| `cmd/intro-diag` | किसी ख़ास शृंखला पर pHash वाली intro पहचान क्यों मिली (या क्यों नहीं मिली), यह जाँचना | **डेवलपर का debug** औज़ार - डिस्क से trickplay sprite पढ़ता है और फ़्रेम-दर-फ़्रेम Hamming मिलान का नक्शा छापता है। साथ में ऑडियो की cross-correlation का एक वैकल्पिक दौर। |
| `cmd/trickplay-verify` | जाँचना कि हर trickplay टाइल सचमुच उसी समय-चिह्न का फ़्रेम दिखाती है जो दर्ज है | **QA/CI** औज़ार - ffmpeg से स्रोत के फ़्रेम दोबारा निकालता है और उन्हें टाइल के सामने aHash करके मिलाता है। |

ये अलग बाइनरी के रूप में इसलिए भेजी जाती हैं कि इनमें से किसी को भी सर्वर का पूरा
कॉन्फ़िगरेशन-तंत्र नहीं चाहिए - ये सीमित दायरे वाले, एक बार चलाने के औज़ार हैं,
उबरने की सतह का हिस्सा नहीं।

## जुड़ी हुई रनबुक

- `migration-runbook.md` - स्कीमा तैनात करना, वापस पलटना, dirty स्थिति से उबरना।
- `recovery.md` - पासवर्ड रीसेट, JWT कुंजी बदलना, बैकअप पुनर्स्थापित करना, खोज का
  index दोबारा बनाना।
- `incident-runbook.md` - सर्वर गड़बड़ करने लगे तो पहला जवाब।
