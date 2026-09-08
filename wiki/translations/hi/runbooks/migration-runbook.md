---
source_sha: 6f985b91384f5a0566bd38dfa9962efdfce4a6b07d1697db8a21ab17c08d0469
---

# माइग्रेशन की रनबुक

Evey स्कीमा माइग्रेशन कैसे चलाता है, और उन पर सुरक्षित ढंग से काम कैसे करें।

## माइग्रेशन कैसे चलते हैं

माइग्रेशन बाइनरी में **भीतर ही रखे** हैं (`go:embed`) और सर्वर शुरू होते ही
`internal/adapters/driven/postgres/migrate.go` से `golang-migrate/migrate/v4` की
मदद से अपने आप चल जाते हैं। स्रोत:
`internal/adapters/driven/postgres/migrations/NNNNNN_<name>.{up,down}.sql`।

`evey-server` शुरू होते ही माइग्रेशन अपने आप लग जाते हैं। हाथ से चलाने के लिए एक
CLI **भी** है: `evey admin migrate up | down | version` (कोड `cmd/evey/admin.go`
में; `version` मौजूदा स्कीमा संस्करण छापता है और डेटाबेस dirty हो तो 1 से बाहर
निकलता है)। `Makefile` के `migrate-up` / `migrate-down` लक्ष्य इसे
`./cmd/evey admin migrate` के ज़रिए चलाते हैं। `EVEY_BACKUP_DIR` सेट हो, तो
माइग्रेशन से पहले एक `pg_dump` अपने आप ले लिया जाता है (छोड़ना हो तो `--no-backup`
दें)।

स्थिति `schema_migrations` तालिका में रखी जाती है (जिसे golang-migrate सँभालता है):
`{version, dirty}`।

## माइग्रेशन से पहले की जाँच-सूची

1. **डेटाबेस का बैकअप लें।** Evey में बैकअप का endpoint पहले से है
   (`docs/backups.md` देखें):
   ```sh
   curl -X POST -H "Authorization: Bearer <admin-token>" \
     http://localhost:9091/api/v1/admin/backups
   ```
   या container पर सीधे `pg_dump` चलाएँ:
   ```sh
   docker exec evey-postgres pg_dump -U "$EVEY_DB_USER" "$EVEY_DB_NAME" \
     | gzip > backup-$(date -u +%Y%m%dT%H%M%SZ).sql.gz
   ```
   जाँच लें कि dump का आकार शून्य नहीं है और `gunzip -t` पास हो जाता है।

2. जो माइग्रेशन बड़ी तालिकाओं को छूता हो (media_items, watch_progress, tracks,
   hls_sessions), उसके लिए **तालिकाओं के आकार देखें**:
   ```sql
   SELECT relname,
          pg_size_pretty(pg_total_relation_size(relid)) AS total,
          n_live_tup
     FROM pg_stat_user_tables
    ORDER BY pg_total_relation_size(relid) DESC
    LIMIT 20;
   ```

3. DDL से भरे माइग्रेशन से पहले **चालू सत्र और lock देखें**:
   ```sql
   SELECT pid, state, wait_event_type, wait_event,
          now() - query_start AS duration, left(query, 120)
     FROM pg_stat_activity
    WHERE datname = current_database()
      AND pid <> pg_backend_pid()
    ORDER BY query_start;
   ```
   मज़बूत lock लेने वाला माइग्रेशन चलाने से पहले, देर से चल रहे या
   idle-in-transaction पड़े सत्र ख़त्म कर दें (`SELECT pg_terminate_backend(pid)`)।

4. **मौजूदा माइग्रेशन संस्करण देखें**:
   ```sql
   SELECT version, dirty FROM schema_migrations;
   ```
   `dirty = true` हो, तो कुछ और करने से पहले "उबरना" वाला खंड पढ़ें।

## माइग्रेशन चलाना

माइग्रेशन सर्वर शुरू होने पर चलते हैं। बचे हुए माइग्रेशन लगाने के लिए:

```sh
docker compose pull evey       # नई image तैनात कर रहे हों तो
docker compose up -d evey
docker logs -f evey-server     # शुरुआत का लॉग देखते रहें
```

सफल होने पर: migrate की कोई त्रुटि नहीं, और सर्वर `/healthz` पर 200 देने लगता है।
जाँच लें:

```sh
curl -sf http://localhost:9091/healthz && echo OK
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" \
  -c "SELECT version, dirty FROM schema_migrations;"
```

## वापस पलटना

तैयार लक्ष्य से एक क़दम वापस पलटें:

```sh
make migrate-down     # `evey admin migrate down` चलाता है, ठीक एक क़दम वापस
make migrate-version  # पुष्टि करें कि आप कहाँ पहुँचे
```

और पीछे जाना हो, तो हर क़दम के लिए इसे दोबारा चलाएँ। `EVEY_BACKUP_DIR` सेट हो, तो
माइग्रेशन से पहले एक `pg_dump` अपने आप ले लिया जाता है।

दूसरा रास्ता यह है कि **पिछली image दोबारा तैनात कर दें**, जिसके भीतर रखे माइग्रेशन
संस्करण N-1 पर ही रुक जाते हैं।

> `schema_migrations` को हाथ से मत छेड़ें। इसी रनबुक के एक पुराने रूप में दावा था
> कि `migrate-down` जैसा कोई लक्ष्य है ही नहीं, और संचालकों से कहा गया था कि
> `.down.sql` को `psql` से लगाएँ और फिर हाथ से
> `UPDATE schema_migrations SET version = ...` चलाएँ - वह भी घटना के बीचोंबीच। यह
> बात उसी से नौ पंक्ति ऊपर वाले "माइग्रेशन कैसे चलते हैं" खंड को काटती थी। वह
> लक्ष्य हमेशा से मौजूद है (`Makefile:314`)। हाथ की सर्जरी की ओर सिर्फ़ तभी बढ़ें
> जब डेटाबेस `dirty` लिखा हो और `migrate down` चलने से मना कर दे - और पहले एक dump
> ज़रूर ले लें।

अगर डेटा मिटाने वाले बदलाव गए हों (DROP COLUMN, टाइप बदलना) और बैकअप, उत्पादन के
ट्रैफ़िक के अलग होने से नया हो, तो हमेशा माइग्रेशन से पहले वाले बैकअप से
पुनर्स्थापित करें।

## उबरना - dirty स्थिति

शुरुआत के लॉग में `Dirty database version N. Fix and force version.` दिखे, तो पिछला
माइग्रेशन बीच में ही गिर गया था। स्कीमा को हाथ से उस हाल में लाएँ जो आपको चाहिए, फिर
यह निशान हटाएँ:

```sql
-- देखें: schema_migrations इस समय क्या बता रही है?
SELECT version, dirty FROM schema_migrations;

-- या तो up.sql के statement हाथ से दोबारा लगाएँ, या down.sql से पलट दें।
-- फिर आप असल में जिस संस्करण तक पहुँचे हैं, वहाँ dirty वाला बिट साफ़ कर दें:
UPDATE schema_migrations SET version = <N>, dirty = false;
```

`evey-server` दोबारा शुरू करें। golang-migrate वहीं से आगे चलता रहेगा।

## ख़ास हालात

संदर्भ के लिए कुछ हालिया माइग्रेशन (`internal/adapters/driven/postgres/migrations/`
देखें):

- **`000066_cascade_and_email_unique.up.sql`** -
  `ALTER TABLE ... DROP CONSTRAINT ... ADD CONSTRAINT ... FOREIGN KEY`। ADD चलने
  तक यह दोनों तालिकाओं पर `ACCESS EXCLUSIVE` ले लेता है। छोटी तालिकाओं पर सस्ता
  है, `media_items` / `watch_progress` पर तकलीफ़देह। बड़ी तालिकाओं का तरीक़ा:
  `NOT VALID` के साथ जोड़ें, फिर दूसरे statement में `VALIDATE CONSTRAINT` चलाएँ
  (validate सिर्फ़ `SHARE UPDATE EXCLUSIVE` लेता है)।
- **`000067_sort_title_index.up.sql`, `000068_watch_progress_composite_index.up.sql`,
  `000069_quick_connect_shortcode_index.up.sql`** - सादा `CREATE INDEX`। बड़ी
  तालिकाओं पर यह लिखने का काम रोक देता है। रखरखाव की खिड़की में हाथ से
  `CREATE INDEX CONCURRENTLY` चलाने पर सोचें (यह migrate के लेन-देन के भीतर नहीं
  चल सकता - इसे अलग से लगाएँ और फिर संस्करण दर्ज कर दें)।
- **`000072_syncplay_queue.up.sql`** -
  `ALTER TABLE ... ADD COLUMN ... NOT NULL DEFAULT '[]'`। PG 11 और उसके बाद यह
  सिर्फ़ मेटाडेटा का काम है और तेज़ है। फिर भी थोड़ी देर के लिए
  `ACCESS EXCLUSIVE` पकड़ता है।

कुछ नियम:

- **लंबा ALTER**, जो तालिका दोबारा लिखता हो (टाइप बदलना, पुराने PG पर स्थिर न रहने
  वाला default जोड़ना) → lock की जाँच पहले करना अनिवार्य है, और सेवा बंद रहने की
  उम्मीद रखें।
- **DROP COLUMN** → सिर्फ़ मेटाडेटा का काम है, पर वह कॉलम बिना बैकअप के वापस नहीं
  आ सकता। जिस रिलीज़ में कोड का नया संस्करण जा रहा हो, उसी में कॉलम कभी न गिराएँ -
  दो क़दमों में करें: पहले कॉलम पढ़ना बंद करें, तैनात करें, और अगली रिलीज़ में
  गिराएँ।
- **बड़ी तालिकाओं पर FK** → पहले `ADD CONSTRAINT ... NOT VALID`, फिर अगले माइग्रेशन
  में `VALIDATE CONSTRAINT`। अकेला रोक देने वाला
  `ADD CONSTRAINT ... FOREIGN KEY ...` न चलाएँ।
- **थोक UPDATE या backfill** → बड़ी तालिका पर कभी एक ही माइग्रेशन में नहीं। एक
  ख़ाली स्कीमा माइग्रेशन भेजें और साथ में पृष्ठभूमि का एक काम।
- **कॉलम बदलना (फैलाओ → खिसकाओ → समेटो)** → `ADD COLUMN` + backfill +
  `SET NOT NULL` + पुराने कॉलम का `DROP COLUMN`, इन सबको एक ही माइग्रेशन में मत
  बाँधिए। यह ढाँचा सिर्फ़ इसलिए चल जाता है कि Evey आज एक ही instance पर चलता है और
  माइग्रेशन शुरुआत के समय एक-एक करके लगते हैं: उस खिड़की के दौरान गिराए गए कॉलम को
  पढ़ने वाला पुराना कोड और नए कॉलम को पढ़ने वाला नया कोड साथ-साथ नहीं रह सकते,
  इसलिए धीरे-धीरे होने वाली या कई प्रतियों वाली तैनाती में जो आधा हिस्सा पीछे रह
  जाएगा वही त्रुटियाँ परोसने लगेगा।
  इसी तरह लिखे हुए जो माइग्रेशन पहले से जा चुके हैं, उन्हें वैसा ही रहने दिया गया
  है: `000077` और `000098_playlists_profile_id.up.sql` (`profile_id` जोड़ता है,
  backfill करता है, `NOT NULL` लगाता है, `user_id` गिरा देता है)।
  नया लिखना हो, तो उसे तीन रिलीज़ में बाँटें:
  1. **फैलाओ** - नया कॉलम nullable जोड़ें, *दोनों* कॉलम में लिखें, पढ़ते पुराने से
     ही रहें;
  2. **खिसकाओ** - नए कॉलम को backfill करें (पृष्ठभूमि के काम से, माइग्रेशन से
     नहीं), फिर पढ़ना उसी पर मोड़ दें;
  3. **समेटो** - जब तैनात कोई भी कोड पुराने कॉलम को न छूता हो, तब `SET NOT NULL`
     लगाकर उसे गिरा दें।
  हर क़दम अलग से तैनात भी हो सकता है और पलटा भी जा सकता है - एक ही फ़ाइल वाला रूप
  यह नहीं कर सकता।

## माइग्रेशन के बाद की जाँच

```sh
curl -sf http://localhost:9091/healthz
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" \
  -c "SELECT version, dirty FROM schema_migrations;"
docker logs --since 10m evey-server 2>&1 | grep -iE 'migrat|error'
```

जिन endpoint पर माइग्रेशन का असर पड़ा है, उनमें से कुछ को चलाकर देख लें
(`/api/v1/...`)।
