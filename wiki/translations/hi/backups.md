---
source_sha: 3529db45dbbd12f6980bca0989017583b6ccad6c3c5a3b54597887c6b09d6285
reviewed: true
---
# बैकअप और अपडेट

दो आदतें आपके सर्वर को सुरक्षित रखती हैं:

- **बैकअप लें।** डेटाबेस का रोज़ का बैकअप चालू करें, और `evey-data` फ़ोल्डर
  की एक कॉपी कहीं और रखें।
- **अपडेट करें।** एक कमांड नया वर्ज़न ले आती है। जब भी कोई अपडेट डेटाबेस को
  बदलता है, Evey पहले खुद डेटाबेस सेव कर लेता है।

यह पेज दोनों दिखाता है, फिर यह कि बैकअप में क्या होता है, उसे वापस कैसे
लाएँ, और पुराने वर्ज़न पर कैसे लौटें।

## बैकअप का शेड्यूल बनाएँ

1. **एडमिन → डेटाबेस बैकअप** पर जाएँ, **शेड्यूल** वाला कार्ड।
2. **क्रॉन एक्सप्रेशन** में बताएँ कि बैकअप कब चले। `0 3 * * *` का मतलब है हर
   दिन सुबह 3 बजे। (क्रम है: मिनट, घंटा, तारीख, महीना, हफ़्ते का दिन।)
3. **प्रतिधारण (दिन)** में बताएँ कि हर बैकअप कितने दिन रखा जाए, जैसे `30`।
   शेड्यूल वाले हर बैकअप के बाद पुराने बैकअप मिटा दिए जाते हैं।
4. **सक्षम** पर टिक लगाएँ, फिर **सहेजें** दबाएँ।

**अभी चलाएं** तुरंत बैकअप बनाता है, और पेज के ऊपर **बैकअप बनाएं** भी।

<figure class="evey-shot" markdown="span">
  ![रोज़ के शेड्यूल के साथ एडमिन → डेटाबेस बैकअप](images/admin-backup.webp){ loading=lazy }
  <figcaption>हर दिन सुबह 3 बजे एक बैकअप, 30 दिन तक रखा जाता है (स्क्रीन अंग्रेज़ी में)।</figcaption>
</figure>

बैकअप `./evey-data/backups` में सेव होते हैं। यह उसी फ़ोल्डर के अंदर है जिसे
आपको वैसे भी किसी दूसरी डिस्क पर कॉपी करना चाहिए (देखें
[बैकअप में क्या होता है](#what-a-backup-contains))।

??? info "कमांड लाइन से"

    होस्ट पर कोई स्क्रिप्ट या cron job यही बैकअप बना सकता है:

    ```bash
    docker exec evey-server evey admin backup
    ```

    यह पैनल के टेलीमेट्री स्विच को मानता है (नीचे देखें);
    `--include-telemetry` सिर्फ़ उस बार के लिए टेलीमेट्री शामिल करता है।

शेड्यूल चाहे जो हो, कोई अपडेट डेटाबेस बदले उससे पहले सर्वर खुद भी डेटाबेस
सेव करता है (देखें [Evey अपडेट करना](#updating-evey))।

## बैकअप में क्या होता है

बैकअप **डेटाबेस** की कॉपी है: खाते, प्रोफ़ाइल, लाइब्रेरी और उनका कैटलॉग, देखने
का इतिहास और जहाँ छोड़ा था वे जगहें, संग्रह, प्लेलिस्ट और पसंदीदा, सेटिंग्स,
प्लगइन के रिकॉर्ड, वेबहुक, API कुंजियाँ, सूचनाएँ और audit लॉग।

इसमें ये **नहीं** होते:

- आपकी मीडिया फ़ाइलें;
- वीडियो कैश (वह अपने-आप फिर भर जाता है);
- सर्च इंडेक्स (डेटाबेस से फिर बन जाता है);
- `./evey-data` में रखी कुंजियाँ: `jwt.key`, जो हर साइन-इन की रक्षा करती है,
  और `keys` फ़ोल्डर (डेटाबेस का पासवर्ड, सर्च की कुंजी);
- इंस्टॉल किए गए प्लगइन की फ़ाइलें।

**इसलिए पूरे `./evey-data` फ़ोल्डर की कॉपी भी रखें।** डेटाबेस बैकअप उसी
इंस्टॉलेशन पर Evey को वापस लाता है। किसी मशीन को शुरू से फिर बनाने के लिए
आपको यह फ़ोल्डर चाहिए।

??? info "टेलीमेट्री, और फ़ाइल का फ़ॉर्मेट"

    प्लेबैक और सर्वर के माप (जो क्वालिटी और लोड के चार्ट के पीछे हैं)
    डिफ़ॉल्ट रूप से छोड़ दिए जाते हैं: ये सबसे तेज़ी से बढ़ते हैं और आपके कैटलॉग
    के बारे में कुछ नहीं बताते। **बैकअप में टेलीमेट्री शामिल करें** (**एडमिन
    → सर्वर कॉन्फ़िग**) इन्हें वापस जोड़ता है, हाथ से बनाए, शेड्यूल वाले और
    कमांड लाइन वाले, तीनों तरह के बैकअप में। बैकअप वापस लाने पर ये टेबल हमेशा
    खाली हो जाती हैं।

    बैकअप एक `.tar.gz` फ़ाइल है, जिसमें डेटाबेस का dump और एक छोटा विवरण
    होता है, उसके checksum (SHA256) और उसके schema वर्ज़न के साथ।

## बैकअप वापस लाएँ

**एडमिन → डेटाबेस बैकअप** बैकअप की सूची दिखाता है। सूची से कोई बैकअप
वापस लाएँ, या **फ़ाइल से पुनर्स्थापित करें** से एक फ़ाइल अपलोड करें। वापस लाने
पर मौजूदा डेटाबेस बदल जाता है: उस बैकअप के बाद का सब कुछ खो जाता है।

!!! warning "सिर्फ़ उसी डेटाबेस बनावट वाले वर्ज़न का बैकअप"
    सर्वर पहले फ़ाइल और डेटाबेस की बनावट (schema वर्ज़न) जाँचता है। अलग
    बनावट वाले वर्ज़न से लिया गया बैकअप वह लोड करने की जगह मना कर देता है।
    किसी पुराने वर्ज़न पर उसके डेटा के साथ लौटने के लिए देखें
    [वापस लौटना](#going-back)।

अगर सर्वर अब शुरू ही नहीं होता, तो कमांड लाइन अपडेट से पहले ली गई सुरक्षा
कॉपियाँ (`pre-migration-*.sql.gz`) वापस ला सकती है, इस पैनल के `.tar.gz` बैकअप
नहीं: देखें [रिकवरी](runbooks/recovery.md)।

## Evey अपडेट करना

उस फ़ोल्डर में जहाँ आपकी compose फ़ाइल है:

```bash
docker compose pull && docker compose up -d
docker compose logs -f evey
```

पूरा अपडेट बस इतना ही है।

- **पहले आपका डेटा सेव होता है।** जब नए वर्ज़न को डेटाबेस बदलना होता है, तो
  वह पहले `./evey-data/backups/pre-migration-*.sql.gz` में एक कॉपी लिखता है
  (सबसे हाल की तीन रखी जाती हैं), फिर शुरू होते समय बदलाव करता है।
- **इसमें कुछ मिनट लग सकते हैं**, बड़े कैटलॉग पर। जब कंटेनर फिर से जवाब देने
  लगता है, तो वह healthy दिखाता है।
- **अपडेट आने पर Evey बताता है।** वह हर घंटे जाँचता है कि कोई नई इमेज आई है
  या नहीं, और एडमिन पैनल में बताता है। वह खुद को कभी अपडेट नहीं करता।
- **कौन सा वर्ज़न चल रहा है:** `docker exec evey-server evey version` वर्ज़न
  और commit छापता है।

??? info "जो वर्ज़न आप चलाते हैं, उसे ठीक-ठीक पिन करें"

    बीटा के दौरान कोई नंबर वाली रिलीज़ नहीं है और कोई `latest` टैग नहीं है।
    `beta-0.0.1` टैग हर अपडेट के साथ बदलता है। जिस सर्वर पर आप निर्भर हैं,
    उसके लिए `image:` लाइन पर एक तय `sha-<commit>` टैग लगाएँ, ताकि एक दिन के
    अंतर पर किए गए दो pull वही कोड चलाएँ। रिलीज़ आने के बाद `latest` सबसे हाल
    की स्थिर रिलीज़ को दर्शाएगा, और रिलीज़ का टैग (`0.2.0`) उसे पिन करेगा।

### वापस लौटना

1. अपनी compose फ़ाइल की `image:` लाइन पर पिछला टैग लगाएँ।
2. अगर नए वर्ज़न ने डेटाबेस बदला था, तो पहले उसकी अपडेट से पहले वाली कॉपी
   वापस लाएँ: `docker exec evey-server evey admin rollback` फ़ाइल के पथ के साथ
   ठीक-ठीक कदम छापता है।
3. `docker compose up -d` चलाएँ।

इमेज टैग, दूसरे रूप और ग्राफ़िक्स कार्ड वाले build
[एडवांस्ड डिप्लॉयमेंट](operations/advanced-deployments.md#image-tags-and-variants)
में बताए गए हैं।

<!-- sources
contents: internal/app/backup/service.go - BackupTables, CreateBackup (sha256 + schema version in manifest), RestoreBackup (checksum + schema guard), CleanupOldBackups ; internal/adapters/driven/filesystem/backup_store.go
schedule: i18n admin.backup.schedule.* (cronLabel, retentionLabel, runNow) ; admin.backup.createBackup, restoreFromFile ; cmd/evey/main.go (DB cron scheduler, env fallback)
telemetry left out by default, emptied on restore: internal/adapters/driven/postgres/backup.go - DumpAll, telemetryTables ; internal/app/backup/service.go - WithIncludeTelemetry ; internal/adapters/driving/http/settings_handler.go - BackupIncludeTelemetryKey ; i18n admin.serverSettings.backupTelemetry, backupTelemetryHint
command-line backup: cmd/evey/admin_backup.go - adminBackup (--include-telemetry, else the panel setting)
keys outside the dump: internal/config/config.go (EVEY_JWT_PRIVATE_KEY_PATH /data/jwt.key, /data/keys)
pre-migration dump at boot, 3 kept: cmd/evey/database.go - RunMigrations BackupOpts Retention 3 ; internal/adapters/driven/postgres/migrate_backup.go (pre-migration-<stamp>.sql.gz, EVEY_BACKUP_DIR default /data/backups)
update check hourly, never self-updates: internal/app/updatecheck/service.go - checkInterval, firstCheck
rollback advisor: cmd/evey/admin_commands.go - adminRollback ; image tags: .github/workflows/ci.yml (beta-0.0.1 + sha-<short>, latest gated on vX.Y.Z)
schedule card controls (Enabled, Save, Run now): frontend/apps/web/src/routes/_auth/admin/backup.lazy.tsx ; i18n admin.backup.schedule.enabledLabel, save, cronHelp
-->
