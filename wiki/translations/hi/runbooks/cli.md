---
source_sha: b5db9ddecc482effde77439df0be753f33fe76e853d0afa1c058a21e0593880d
reviewed: false
---
# Evey CLI - `evey admin`

Evey सर्वर का बाइनरी ऑपरेटर CLI का काम भी करता है। जब `evey` बिना आर्ग्युमेंट के चलाया जाता है, तो यह HTTP/gRPC सर्वर शुरू करता है; पहले आर्ग्युमेंट के रूप में `admin` होने पर यह एक बार चलने वाले ऑपरेटर मोड में चला जाता है और कमांड पूरा होने पर बाहर निकल जाता है।

सभी कमांड कॉन्फ़िगरेशन उन्हीं `EVEY_*` एनवायरनमेंट वेरिएबल से पढ़ते हैं जिनसे सर्वर ख़ुद पढ़ता है (`EVEY_DB_USER`, `EVEY_DB_PASSWORD`, `EVEY_DB_HOST`, `EVEY_DB_PORT`, `EVEY_DB_NAME`, `EVEY_DB_SSLMODE`, `EVEY_BACKUP_DIR`, ...)।

```
evey                          Start the server.
evey admin <subcommand>       One-shot operator tool (below). `evey admin` alone lists them,
                              `evey admin <subcommand> --help` details one.
evey version                  Version, commit, build date - needs no config or database.
evey help                     Command summary.
evey man [--text]             The manual: roff for `man`, plain text with --text.
evey completion bash          The bash completion script.
```

कोई भी दूसरा पहला आर्ग्युमेंट चुपचाप सर्वर शुरू करने की बजाय मना कर दिया जाता है (exit 1)। `cmd/evey/admin_commands.go` में Go तालिका हर सबकमांड का एकमात्र विवरण है: उपयोग सूची, `--help`, मैनुअल, कम्प्लीशन स्क्रिप्ट और कंटेनर शेल सभी `cmd/evey/admin_cli_test.go` द्वारा इसके विरुद्ध जाँचे जाते हैं।

## त्वरित संदर्भ

```
evey admin reset-password --user <email>     Generate a new password, print it once.
evey admin rotate-jwt-key                    Regenerate JWT signing key and end every session.
evey admin restore-backup <path>             Restore the database from a .sql.gz dump (the suffix is required; not the panel's .tar.gz backups).
evey admin reindex-search [--library <id>] [--purge]   Rebuild the Meilisearch index from Postgres (--purge clears orphan docs first; if Meili is down, reports the Postgres fallback and does nothing).
evey admin migrate up [--no-backup]          Apply pending migrations.
evey admin migrate down [--no-backup]        Roll back one migration step.
evey admin migrate version                   Print current schema version (exit 1 if dirty).
evey admin list-users                        Print user emails, one per line (feeds `--user` completion).
evey admin list-libraries                    Print the library root paths, one per line (feeds `--root` completion).
```

### मीडिया रखरखाव

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

### निदान और क्षमता

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

### डेमो इंस्टेंस

```
evey admin demo-seed [--apply] [--media]     Run the demo bootstrap by hand: the demo administrator,
                                             the visitor accounts, the four libraries, and with
                                             --media the freely licensed media (gigabytes). Refuses
                                             unless EVEY_DEMO=1. Dry-run by default.
```

> पूरी और हमेशा अद्यतन सूची है बिना सबकमांड के `evey admin`: यह बाइनरी में मौजूद हर सबकमांड छापता है।

## कैसे चलाएँ

### कंटेनर में (मानक इंस्टॉलेशन)

```sh
docker exec evey-server evey admin doctor
docker exec -it evey-server bash      # एक एडमिन शेल: evey admin ..., man evey
```

इमेज डेटाबेस पासवर्ड फ़ाइल सेट करती है, इसलिए कमांड बिना किसी अतिरिक्त एनवायरनमेंट के बिल्ट-इन डेटाबेस तक पहुँच जाते हैं।

### बेयर-मेटल इंस्टॉलेशन पर (बिना Docker)

```sh
evey admin reset-password --user alice@example.com
evey admin migrate version
```

पहले से इंस्टॉल बाइनरी में हर सबकमांड पहले से मौजूद है।

### डेवलपमेंट स्टैक पर

Makefile माइग्रेशन कमांड को लपेटता है, जो डेवलपमेंट कंटेनर `evey-server` में सोर्स ट्री से चलते हैं:

```sh
make migrate-version
make migrate-up
make migrate-down
```

## टैब-कम्प्लीशन और मैनुअल

बाइनरी अपना bash कम्प्लीशन और मैनुअल साथ रखता है, इसलिए दोनों हमेशा उसके सबकमांड से मेल खाते हैं:

```sh
evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null && sudo mandb -q
```

zsh उपयोगकर्ता: पहले `autoload -U +X bashcompinit && bashcompinit`, फिर वही स्क्रिप्ट लोड करें। कम्प्लीशन सबकमांड, उनके फ़्लैग और फ़्लैग मान (`--mode`, `--backend`, `--scenario`), `--user` के बाद उपयोगकर्ता ई-मेल, `--root` के बाद सर्वर को ज्ञात लाइब्रेरी रूट (और कोई भी डायरेक्टरी), `ladder-bench`, `ladder-probe` और `route-bench` के फ़ाइल आर्ग्युमेंट के लिए केवल मीडिया फ़ाइलें, और `restore-backup` के लिए `.sql.gz` डंप को कवर करता है।

Docker इमेज बिल्ड के समय कम्प्लीशन इंस्टॉल करती है और इसका एडमिन शेल (`docker exec -it evey-server bash`) `man evey` का जवाब `evey man --text` को पेज करके देता है, क्योंकि इमेज में कोई `man` बाइनरी नहीं है।

## सबकमांड का विवरण

### `migrate`

- `up` सभी लंबित माइग्रेशन क्रम से लागू करता है। जब `EVEY_BACKUP_DIR` सेट हो (Docker इमेज में डिफ़ॉल्ट: `/data/backups`) तो माइग्रेशन से पहले अपने आप एक `pg_dump` लिया जाता है। Up के बीच में विफलता `schema_migrations.dirty = true` छोड़ती है; देखें `migration-runbook.md`।
- `down` ठीक एक माइग्रेशन वापस करता है। यह सटीक है, "सब गिरा दो" नहीं: कोई `down --all` नहीं है। यह भी वापस करने से पहले एक `pg_dump` लेता है।
- `version` मौजूदा स्कीमा संस्करण छापता है। जब पिछला माइग्रेशन बीच में विफल हुआ हो (यानी कुछ लेकिन सभी नहीं `Up` स्टेटमेंट चले) तो `DIRTY` चेतावनी के साथ exit code 1 लौटाता है।
- `--no-backup` दोनों दिशाओं में pg_dump छोड़ देता है। ऐसे होस्ट पर उपयोगी जहाँ `postgres-client` इंस्टॉल नहीं है (आम WSL डेवलपमेंट स्थिति, जहाँ Makefile पहले से यह फ़्लैग देता है)।

### `reset-password`, `rotate-jwt-key`, `restore-backup`, `reindex-search`

पूरी प्रक्रिया, पूर्व-शर्तों और बाद की स्थितियों के लिए `docs/runbooks/recovery.md` देखें।

## `evey admin` में क्या *नहीं* है

तीन अलग बाइनरी `cmd/evey` के बगल में रहते हैं क्योंकि वे पुनर्प्राप्ति या स्कीमा के अर्थ में ऑपरेटर कमांड नहीं हैं:

| बाइनरी | उद्देश्य | अलग क्यों |
|---|---|---|
| `cmd/intro-backfill` | पाइपलाइन बदलाव के बाद कैटलॉग के एक हिस्से पर इंट्रो पहचान ताज़ा करना | **API क्लाइंट**: bearer टोकन के साथ `/api/v1/admin/media/{id}/detect-segments` से बात करता है, प्रोसेस के अंदर नहीं। |
| `cmd/intro-diag` | निदान करना कि किसी ख़ास सीरीज़ पर pHash इंट्रो पहचान क्यों मिली (या नहीं मिली) | **डेवलपर डीबग** उपकरण: डिस्क से trickplay स्प्राइट पढ़ता है, फ़्रेम दर फ़्रेम Hamming मिलान का नक्शा छापता है। वैकल्पिक ऑडियो क्रॉस-कोरिलेशन पास। |
| `cmd/trickplay-verify` | जाँचना कि हर trickplay टाइल सचमुच दर्ज टाइमस्टैम्प का फ़्रेम दिखाती है | **QA/CI** उपकरण: ffmpeg से स्रोत फ़्रेम फिर से निकालता है और टाइल से उनका aHash मिलाता है। |

वे अलग बाइनरी के रूप में आते हैं क्योंकि उनमें से किसी को पूरे सर्वर कॉन्फ़िगरेशन की ज़रूरत नहीं: वे सीमित, एक बार चलने वाले उपकरण हैं, पुनर्प्राप्ति सतह का हिस्सा नहीं।

## संबंधित पुस्तिकाएँ

- `migration-runbook.md`: स्कीमा डिप्लॉय, वापसी, dirty स्थिति से पुनर्प्राप्ति।
- `recovery.md`: पासवर्ड रीसेट, JWT कुंजी रोटेशन, बैकअप पुनर्स्थापना, सर्च रीइंडेक्स।
- `incident-runbook.md`: सर्वर के गड़बड़ करने पर पहली प्रतिक्रिया।
