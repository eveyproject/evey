---
source_sha: 68ae93ace6a6ac1dc58140bf8cdc28c60bd2db3d0b2ce148ec58433940070f68
reviewed: false
---
# डेटाबेस माइग्रेशन

Evey अपडेट के दौरान अपना डेटाबेस स्कीमा कैसे बदलता है, और गड़बड़ होने पर क्या करें। कमांड मानक इंस्टॉलेशन के लिए हैं, जहाँ डेटाबेस `evey-server` कंटेनर के अंदर चलता है।

## माइग्रेशन कैसे चलते हैं

माइग्रेशन बाइनरी में शामिल हैं (`go:embed`) और सर्वर शुरू होते समय `golang-migrate` उन्हें लागू करता है (`internal/adapters/driven/postgres/migrate.go`, फ़ाइलें `internal/adapters/driven/postgres/migrations/NNNNNN_<name>.{up,down}.sql`)। स्थिति `schema_migrations` तालिका है: `{version, dirty}`।

कुछ भी लागू करने से पहले सर्वर `/data/backups/pre-migration-<version>-<time>.sql.gz` (`EVEY_BACKUP_DIR`) में एक पूरा डंप लिखता है। 3 सबसे हाल के स्कीमा संस्करणों में से हर एक का सबसे नया डंप रखा जाता है।

यही काम हाथ से भी किए जा सकते हैं:

```sh
docker exec evey-server evey admin migrate version   # स्कीमा dirty होने पर exit 1
docker exec evey-server evey admin migrate up
docker exec evey-server evey admin migrate down      # ठीक एक चरण
```

`up` और `down` भी पहले एक डंप लेते हैं; `--no-backup` उसे छोड़ देता है।

बिल्ट-इन डेटाबेस पर SQL चलाने के लिए:

```sh
docker exec evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

## अपडेट से पहले

1. **एक बैकअप लें** **Admin → बैकअप और पुनर्स्थापना** में (**अभी चलाएं**), या किसी स्क्रिप्ट से:
   ```sh
   curl -X POST -H "Authorization: Bearer <admin-token>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   माइग्रेशन से पहले का स्वचालित डंप दूसरी प्रति है, विकल्प नहीं।

2. **बड़े कैटलॉग के लिए**, सबसे बड़ी तालिकाओं का आकार देखें: उनमें से किसी को दोबारा लिखने वाला माइग्रेशन ज़्यादा समय लेता है।
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

3. **मौजूदा संस्करण जाँचें**: `evey admin migrate version`। अगर यह dirty स्कीमा बताए, तो पहले [बीच में रुका माइग्रेशन](#a-migration-that-stopped-halfway) देखें।

## माइग्रेशन चलाना

वे नई इमेज शुरू होने पर चलते हैं:

```sh
docker compose pull && docker compose up -d
docker logs -f evey-server
```

माइग्रेशन लागू होने और सर्वर के जवाब देने पर कंटेनर स्वस्थ बताया जाता है:

```sh
curl -sf http://localhost:9091/healthz && echo OK
docker exec evey-server evey admin migrate version
```

## वापस जाना

`evey admin rollback` चल रहा संस्करण, स्कीमा संस्करण, माइग्रेशन से पहले का सबसे नया डंप और होस्ट पर चलाने के सटीक कमांड छापता है। यह कुछ नहीं बदलता: कंटेनर के पास Docker तक पहुँच नहीं है।

वापस जाने का सामान्य तरीका है पिछली इमेज, अपडेट से पहले लिए गए डंप के साथ: बैकअप और अपडेट में [वापस जाना](../backups.md#going-back) देखें। इसकी बजाय `evey admin migrate down` एक बार में एक चरण वापस करता है, जब उस चरण का `.down.sql` काफ़ी हो।

`schema_migrations` को हाथ से न बदलें, सिवाय dirty स्थिति हटाने के (नीचे), और पहले एक डंप लें।

जब अपडेट ने डेटा हटाया हो (हटाया गया कॉलम, प्रकार बदलाव), तो `migrate down` से पीछे जाने की बजाय माइग्रेशन से पहले का डंप बहाल करें।

## बीच में रुका माइग्रेशन

अगर लॉग `Dirty database version N. Fix and force version.` दिखाता है, तो कोई माइग्रेशन बीच में क्रैश हुआ। स्कीमा को उस स्थिति में लाएँ जो आप चाहते हैं, फिर फ़्लैग हटाएँ:

```sql
SELECT version, dirty FROM schema_migrations;

-- up.sql का बाक़ी हिस्सा हाथ से फिर से लागू करें, या down.sql से वापस करें,
-- फिर वह संस्करण दर्ज करें जहाँ आप वास्तव में पहुँचे:
UPDATE schema_migrations SET version = <N>, dirty = false;
```

कंटेनर रीस्टार्ट करें; माइग्रेशन वहीं से जारी रहते हैं। माइग्रेशन से पहले का डंप बहाल करना दूसरा रास्ता है।

## माइग्रेशन लिखना

योगदानकर्ताओं के लिए। उदाहरण `internal/adapters/driven/postgres/migrations/` में:

- **`000066_cascade_and_email_unique.up.sql`**: `ALTER TABLE ... DROP CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`। ADD की अवधि में दोनों तालिकाओं पर `ACCESS EXCLUSIVE` लेता है। छोटी तालिकाओं पर सस्ता, `media_items` / `watch_progress` पर तकलीफ़देह। बड़ी तालिकाओं का पैटर्न: `NOT VALID` के साथ जोड़ें, फिर दूसरे स्टेटमेंट में `VALIDATE CONSTRAINT` (validate केवल `SHARE UPDATE EXCLUSIVE` लेता है)।
- **`000067_sort_title_index.up.sql`, `000068_watch_progress_composite_index.up.sql`, `000069_quick_connect_shortcode_index.up.sql`**: सादा `CREATE INDEX`। बड़ी तालिकाओं पर यह लिखना रोकता है। `CREATE INDEX CONCURRENTLY` migrate ट्रांज़ैक्शन के अंदर नहीं चल सकता।
- **`000072_syncplay_queue.up.sql`**: `ALTER TABLE ... ADD COLUMN ... NOT NULL DEFAULT '[]'`। PG 11+ पर यह केवल मेटाडेटा है और तेज़ है। फिर भी थोड़ी देर `ACCESS EXCLUSIVE` रखता है।

दिशानिर्देश:

- तालिका को दोबारा लिखने वाला **लंबा ALTER** (प्रकार बदलाव, पुराने PG पर गैर-स्थिर डिफ़ॉल्ट जोड़ना) → बड़े कैटलॉग पर डाउनटाइम की उम्मीद करें।
- **DROP COLUMN** → केवल मेटाडेटा, लेकिन बैकअप के बिना कॉलम वापस नहीं लाया जा सकता। उसी रिलीज़ में कभी न हटाएँ जिसमें कोड का नया संस्करण आता है: दो चरण बेहतर हैं, कॉलम पढ़ना बंद करें, डिप्लॉय करें, फिर अगली रिलीज़ में हटाएँ।
- **बड़ी तालिकाओं पर FK** → `ADD CONSTRAINT ... NOT VALID` फिर अगले माइग्रेशन में `VALIDATE CONSTRAINT`। एक ही रोकने वाले `ADD CONSTRAINT ... FOREIGN KEY ...` से बचें।
- **बैच UPDATE / बैकफ़िल** → बड़ी तालिका पर कभी एक ही माइग्रेशन में नहीं। एक ख़ाली स्कीमा माइग्रेशन + एक बैकग्राउंड काम भेजें।
- **कॉलम बदलना (expand → migrate → contract)** → `ADD COLUMN` + बैकफ़िल + `SET NOT NULL` + पुराने का `DROP COLUMN` एक ही माइग्रेशन में न मिलाएँ। यह रूप केवल इसलिए काम करता है क्योंकि Evey आज एक ही इंस्टेंस पर चलता है, और माइग्रेशन शुरू होते समय सिंक्रोनस रूप से लागू होते हैं: इस बीच हटाया गया कॉलम पढ़ने वाला पुराना कोड और नया कॉलम पढ़ने वाला नया कोड साथ नहीं रह सकते, इसलिए रोलिंग या कई रेप्लिका वाला डिप्लॉय पीछे रह गए आधे हिस्से से त्रुटियाँ देगा।
  इस तरह लिखे गए ज्ञात माइग्रेशन, जो पहले ही जारी हो चुके हैं इसलिए जैसे हैं वैसे रखे गए: `000077` और `000098_playlists_profile_id.up.sql` (`profile_id` जोड़ता है, बैकफ़िल करता है, `NOT NULL` सेट करता है, `user_id` हटाता है)।
  नए के लिए, तीन रिलीज़ में बाँटें:
  1. **expand**: नया कॉलम nullable जोड़ें, *दोनों* कॉलम में लिखें, पुराना पढ़ते रहें;
  2. **migrate**: नया कॉलम बैकफ़िल करें (बैकग्राउंड काम, माइग्रेशन नहीं), फिर पढ़ना उस पर ले जाएँ;
  3. **contract**: जब कोई डिप्लॉय किया गया कोड पुराने कॉलम को न छुए, तो `SET NOT NULL` करें और उसे हटाएँ।
  हर चरण अलग से डिप्लॉय और वापस किया जा सकता है, जो एक फ़ाइल वाले संस्करण में संभव नहीं।

## अपडेट के बाद

```sh
curl -sf http://localhost:9091/healthz
docker exec evey-server evey admin migrate version
docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
```

फिर अगर रिलीज़ ने वीडियो एनकोड करने का तरीका बदला हो तो ट्रांसकोडिंग कैश ख़ाली करें (**Admin → ट्रांसकोड कैश**)।
