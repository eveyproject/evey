---
source_sha: fad8d9ff7aa0e8e4c14508e1eb2fe716e13c05c66eca9acb763084c56aec898f
reviewed: true
---
# डेटाबेस माइग्रेशन

अपडेट के दौरान Evey अपने डेटाबेस का स्कीमा कैसे बदलता है, पहले और बाद में क्या
जाँचना है, और जब कोई माइग्रेशन आधे रास्ते में रुक जाए तो बाहर कैसे निकलें। यह
आम इंस्टॉल चलाने वाले ऑपरेटरों के लिए है, जहाँ डेटाबेस `evey-server` कंटेनर के
अंदर चलता है; माइग्रेशन लिखने वाले योगदानकर्ताओं के लिए नियम आखिर में हैं।

## माइग्रेशन कैसे चलते हैं

माइग्रेशन प्रोग्राम के अंदर ही होते हैं और सर्वर शुरू होने पर `golang-migrate`
उन्हें लागू करता है। हालत `schema_migrations` टेबल में रहती है, जिसमें एक पंक्ति
`{version, dirty}` होती है। कुछ भी लागू करने से पहले, सर्वर `EVEY_BACKUP_DIR`
(`/data/backups`, यानी होस्ट पर `./evey-data/backups`) में एक पूरा डंप लिखता है,
जिसका नाम `pre-migration-<version>-<UTC timestamp>.sql.gz` होता है, और सबसे हाल
के 3 स्कीमा वर्ज़न में से हर एक का सबसे नया डंप रखता है।

यही काम हाथ से भी किए जा सकते हैं:

```sh
docker exec evey-server evey admin migrate version   # स्कीमा dirty हो तो exit 1
docker exec evey-server evey admin migrate up        # सारे बाकी माइग्रेशन लागू करें
docker exec evey-server evey admin migrate down      # ठीक एक कदम पीछे जाएँ
```

`up` और `down` पहले एक डंप लेते हैं; `--no-backup` इसे छोड़ देता है। अंदर वाले
डेटाबेस पर SQL शेल खोलने के लिए:

```sh
docker exec -it evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

## सुरक्षित तरीके से अपडेट करना

1. **बैकअप लें**, **एडमिन → बैकअप और पुनर्स्थापना** में (**अभी चलाएं**), या
   किसी स्क्रिप्ट से:
   ```sh
   curl -X POST -H "Authorization: Bearer <admin-token>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   माइग्रेशन से पहले अपने-आप बनने वाला डंप एक दूसरी कॉपी है, इसकी जगह नहीं।

2. **मौजूदा वर्ज़न जाँचें**: `docker exec evey-server evey admin migrate
   version` छापता है `schema version: N`। अगर यह `DIRTY` छापे, तो अपडेट करने से
   पहले [आधे रास्ते में रुका माइग्रेशन](#a-migration-that-stopped-halfway)
   देखें।

3. **बड़े कैटलॉग पर**, सबसे बड़ी टेबल देखें: जो माइग्रेशन इनमें से किसी को दोबारा
   लिखता है उसमें ज़्यादा समय लगता है, और कंटेनर का हेल्थचेक इसके लिए शुरू होने
   में 300 सेकंड देता है।
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

4. **नई इमेज शुरू करें** और लॉग तब तक देखते रहें जब तक माइग्रेशन लागू न हो जाएँ
   और सर्वर जवाब न देने लगे:
   ```sh
   docker compose pull && docker compose up -d
   docker logs -f evey-server
   ```
   सही नतीजा: जब कुछ लागू करना था तो `migrations: pre-migration backup written`
   वाली लाइन, फिर सर्वर का बैनर, फिर `curl -sf http://localhost:9091/healthz`
   जवाब देता है।

5. **इसके बाद**, वर्ज़न की पुष्टि करें और गड़बड़ियाँ खोजें:
   ```sh
   docker exec evey-server evey admin migrate version
   docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
   ```
   अगर इस रिलीज़ ने वीडियो एन्कोड होने का तरीका बदला है, तो ट्रांसकोडिंग कैश
   खाली करें (**एडमिन → ट्रांसकोड कैश → सभी साफ़ करें**)।

## पिछले वर्ज़न पर वापस जाना

`docker exec evey-server evey admin rollback` चल रहा वर्ज़न, स्कीमा वर्ज़न,
माइग्रेशन से पहले का सबसे नया डंप, और होस्ट पर चलाने वाली सटीक कमांड छापता है। यह
कुछ नहीं बदलता: कंटेनर के पास Docker तक पहुँच नहीं है।

वापस जाने का आम तरीका है पिछली इमेज और अपडेट से पहले लिया गया डंप: देखें
[वापस लौटना](../backups.md#going-back)। जब अपडेट ने डेटा हटाया हो (कोई कॉलम हटाया
गया, कोई टाइप बदला गया), तो माइग्रेशन को पीछे ले जाने के बजाय डंप वापस लाएँ।
`evey admin migrate down` इसके बजाय एक बार में एक माइग्रेशन पीछे जाता है, जब उस
कदम की `.down.sql` काफ़ी हो।

!!! warning
    `schema_migrations` को हाथ से न बदलें, सिवाय dirty हालत हटाने के (नीचे
    देखें), और पहले एक डंप ले लें।

## आधे रास्ते में रुका माइग्रेशन

जब लॉग में `Dirty database version N. Fix and force version.` दिखे, तो इसका मतलब
है कि कोई माइग्रेशन बीच में क्रैश हो गया और `golang-migrate` आगे बढ़ने से मना कर
रहा है। स्कीमा को उस हालत में लाएँ जो आप चाहते हैं, फिर यह निशान हटाएँ:

```sql
SELECT version, dirty FROM schema_migrations;

-- up.sql का बाकी हिस्सा हाथ से फिर से लागू करें, या down.sql से वापस लें,
-- फिर वह वर्ज़न दर्ज करें जहाँ आप असल में पहुँचे:
UPDATE schema_migrations SET version = <N>, dirty = false;
```

कंटेनर रीस्टार्ट करें; माइग्रेशन वहीं से आगे चलते हैं। माइग्रेशन से पहले का डंप
वापस लाना ([रिकवरी](recovery.md#restore-a-pre-migration-dump)) बाहर निकलने का
दूसरा रास्ता है।

## माइग्रेशन लिखना

योगदानकर्ताओं के लिए। फ़ाइलें
`internal/adapters/driven/postgres/migrations/NNNNNN_<name>.{up,down}.sql` में
रहती हैं। पहले से जारी हो चुके उदाहरण:

- **`000066_cascade_and_email_unique.up.sql`** - `ALTER TABLE ... DROP
  CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`। ADD चलने तक दोनों टेबल पर
  `ACCESS EXCLUSIVE` लेता है। छोटी टेबल पर सस्ता, `media_items` /
  `watch_progress` पर तकलीफ़देह।
- **`000067_sort_title_index.up.sql`**,
  **`000068_watch_progress_composite_index.up.sql`**,
  **`000069_quick_connect_shortcode_index.up.sql`** - सादा `CREATE INDEX`, जो
  बड़ी टेबल पर लिखना रोक देता है। `CREATE INDEX CONCURRENTLY` माइग्रेशन के
  ट्रांज़ैक्शन के अंदर नहीं चल सकता।
- **`000072_syncplay_queue.up.sql`** - `ALTER TABLE ... ADD COLUMN ... NOT
  NULL DEFAULT '[]'`। PG 11+ पर सिर्फ़ मेटाडेटा बदलता है, लेकिन फिर भी थोड़ी देर
  के लिए `ACCESS EXCLUSIVE` रखता है।

दिशानिर्देश:

- **लंबा ALTER** जो टेबल दोबारा लिखता है (टाइप बदलना, पुराने PG पर ऐसा डिफ़ॉल्ट जो
  स्थिर नहीं है), बड़े कैटलॉग पर सर्वर बंद रहने का समय (downtime) लाता है।
- **DROP COLUMN** सिर्फ़ मेटाडेटा बदलता है, लेकिन बैकअप के बिना कॉलम वापस नहीं आ
  सकता। उसे उसी रिलीज़ में कभी न हटाएँ जो उसे पढ़ना बंद करती है: पढ़ना बंद करें,
  डिप्लॉय करें, अगली रिलीज़ में हटाएँ।
- **बड़ी टेबल पर FK**: `ADD CONSTRAINT ... NOT VALID`, फिर अगले माइग्रेशन में
  `VALIDATE CONSTRAINT` (validate सिर्फ़ `SHARE UPDATE EXCLUSIVE` लेता है)।
- **बैच में UPDATE / backfill**: बड़ी टेबल पर कभी एक ही माइग्रेशन में नहीं। स्कीमा
  का ऐसा माइग्रेशन भेजें जो कुछ न करे, साथ में एक बैकग्राउंड काम।
- **कॉलम बदलना (expand, migrate, contract)**: `ADD COLUMN` + backfill + `SET NOT
  NULL` + `DROP COLUMN` को एक फ़ाइल में न जोड़ें। यह रूप सिर्फ़ इसलिए काम करता है
  क्योंकि Evey एक ही इंस्टेंस में चलता है, और माइग्रेशन शुरू होते समय एक के बाद
  एक लागू होते हैं; रोलिंग डिप्लॉय में जो आधा हिस्सा पीछे होता, उससे गड़बड़ियाँ
  मिलतीं। `000077` और `000098_playlists_profile_id.up.sql` इसी तरह लिखे गए थे
  और जैसे हैं वैसे ही रहेंगे, क्योंकि वे जारी हो चुके हैं। नए के लिए, इसे तीन
  रिलीज़ में बाँटें:
  1. **expand** - नया कॉलम nullable के रूप में जोड़ें, दोनों कॉलम में लिखें,
     पुराने को ही पढ़ते रहें;
  2. **migrate** - नए कॉलम को backfill करें (बैकग्राउंड काम से, माइग्रेशन से
     नहीं), फिर पढ़ना उस पर ले जाएँ;
  3. **contract** - जब कोई डिप्लॉय हुआ कोड पुराने कॉलम को न छुए, तो `SET NOT
     NULL` लागू करें और उसे हटा दें।

<!-- sources
migrations: internal/adapters/driven/postgres/migrate.go - RunMigrations, RollbackLastMigration, CurrentMigrationVersion ; internal/adapters/driven/postgres/migrate_backup.go - dumpDatabase, backupFilePrefix, pruneOldBackups, defaultBackupRetention
cli: cmd/evey/admin.go - adminMigrate, adminMigrateUsage ; cmd/evey/admin_ops.go - adminRollback
embedded database and healthcheck start period: docker/entrypoint.sh (postgres.password, EVEY_DB_PASSWORD_FILE) ; Dockerfile - HEALTHCHECK --start-period
backup route: internal/adapters/driving/http/backup_handler.go (POST /api/v1/admin/backups)
labels: i18n/en/admin.yaml - sidebar.backupRestore, backup.schedule.runNow, transcodeCache.clearAll
-->
