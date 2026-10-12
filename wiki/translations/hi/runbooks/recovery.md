---
source_sha: 15f4243e2aa41d1a370efb30c5607eb485799cf11a0be191ea0b7489684f4485
reviewed: true
---
# रिकवरी

यह पेज तब खोलें जब कोई भी साइन इन न कर पा रहा हो, जब एक्सेस टोकन पर साइन करने
वाली की (key) शायद लीक हो गई हो, या जब डेटाबेस को माइग्रेशन से पहले के डंप पर
वापस ले जाना हो। हर तरीका कंटेनर के अंदर चलने वाली एक `evey admin` कमांड है;
बाकी सबकमांड [कमांड लाइन](cli.md) पेज पर हैं।

## शुरू करने से पहले

कमांड वही `EVEY_*` कॉन्फ़िगरेशन पढ़ती हैं जो सर्वर पढ़ता है, इसलिए सर्वर के
कंटेनर में इन्हें चलाने के लिए और कुछ नहीं चाहिए:

```sh
docker exec evey-server evey admin <subcommand>        # सर्वर चल रहा है
docker compose run --rm evey admin <subcommand>        # कंटेनर बंद है
```

दूसरा तरीका पहले अंदर वाला डेटाबेस शुरू करता है, फिर कमांड चलाता है और बंद हो
जाता है। इनमें से कोई भी कमांड सर्वर शुरू नहीं करती।

## किसी उपयोगकर्ता को नया पासवर्ड देना

इसका इस्तेमाल तब करें जब कोई उपयोगकर्ता अंदर न जा पा रहा हो, या जब किसी खाते पर
किसी और के कब्ज़े का शक हो, तुरंत उसके बाद। नया पासवर्ड एक ही बार छपता है और कभी
लॉग में नहीं लिखा जाता: टर्मिनल बंद करने से पहले उसे कॉपी कर लें।

```sh
docker exec evey-server evey admin reset-password --user admin@example.com
```

सही नतीजा ऐसा दिखता है:

```
user:     admin@example.com
user_id:  usr-xxxxxxxx
password: <random 24-byte base64url>

Save this - it is the only time the new password is shown.
Every session of this user has been ended.
```

उस उपयोगकर्ता का हर सेशन एक साथ खत्म हो जाता है: रिफ़्रेश टोकन मिटा दिए जाते
हैं और एक्सेस टोकन रद्द कर दिए जाते हैं। खुला हुआ ऐप लगभग 30 सेकंड के अंदर यह
पकड़ लेता है और फिर से साइन इन करने को कहता है। अगर आपसे आउटपुट खो जाए, तो कमांड
फिर से चलाएँ।

## एक्सेस टोकन पर साइन करने वाली की बदलना

इसका इस्तेमाल तब करें जब की वाली फ़ाइल शायद लीक हो गई हो, या जब सभी उपयोगकर्ताओं
को एक साथ साइन आउट करना हो। किसी एक खाते के लिए **एडमिन → उपयोगकर्ता → अक्षम
करें**, या ऊपर वाला पासवर्ड रीसेट काफ़ी है।

```sh
docker exec evey-server evey admin rotate-jwt-key
```

क्या होता है:

1. की वाली फ़ाइल (`EVEY_JWT_PRIVATE_KEY_PATH`, डिफ़ॉल्ट रूप से `/data/jwt.key`)
   का नाम बदलकर `<path>.rotated-<UTC timestamp>` कर दिया जाता है।
2. हर रिफ़्रेश टोकन मिटा दिया जाता है और हर एक्सेस टोकन रद्द कर दिया जाता है, एक
   ही ट्रांज़ैक्शन में। खुले ऐप लगभग 30 सेकंड के अंदर यह पकड़ लेते हैं; सब लोग
   फिर से साइन इन करते हैं।
3. अगली बार शुरू होने पर सर्वर एक नई 2048-बिट RSA की बनाता है।

इसके बाद कंटेनर रीस्टार्ट करें (`docker compose restart evey`), ताकि नई की
इस्तेमाल हो। अगर डेटाबेस तक पहुँचा नहीं जा सका, तब भी की किनारे कर दी जाती है और
कमांड कहती है `key rotated, but sessions could not be ended
(restart the server, then run this again)`: ठीक यही करें।

## माइग्रेशन से पहले का डंप वापस लाना

इसका इस्तेमाल तब करें जब किसी अपडेट को वापस लेना हो और उसके बाद लिखा गया डेटा खोया
जा सकता हो। कमांड एक `.sql.gz` डंप को `gunzip | psql` से होकर सेट किए गए
डेटाबेस में डालती है।

!!! warning
    वापस लाना मौजूदा पंक्तियों (rows) के ऊपर लिख देता है। अगर ज़रा भी संभावना है
    कि आपको मौजूदा हालत वापस चाहिए होगी, तो पहले उसका बैकअप लें (**एडमिन →
    डेटाबेस बैकअप → अभी चलाएं**)।

```sh
docker compose stop evey
docker compose run --rm evey admin restore-backup /data/backups/pre-migration-152-20260418T220130Z.sql.gz
docker compose up -d
```

क्या होता है:

1. कमांड पहले पूरी फ़ाइल पढ़ती है: वह बिना गड़बड़ी के खुलनी चाहिए और `pg_dump`
   की पूरा होने वाली लाइन पर खत्म होनी चाहिए। अधूरा या खराब डंप कुछ भी हटाने से
   पहले मना कर दिया जाता है (`refusing to restore: ...`)।
2. फिर वह कुछ भी छूने से पहले 5 सेकंड रुकती है: Ctrl+C से रोका जा सकता है।
3. `psql` डंप को `--single-transaction` और `ON_ERROR_STOP=1` के साथ लागू करता
   है। कोई भी गड़बड़ी होने पर ट्रांज़ैक्शन वापस ले लिया जाता है और डेटाबेस जैसा था
   वैसा रहता है।
4. सफल होने पर आखिरी लाइन `[admin] restored backup <path>` होती है।

ज़रूरी बातें:

- फ़ाइल का नाम `.sql.gz` पर खत्म होना चाहिए। **एडमिन → बैकअप और
  पुनर्स्थापना** के `.tar.gz` आर्काइव पैनल से वापस लाए जाते हैं, इस कमांड से नहीं।
- जिस डेटाबेस में डालना है, वह मौजूद होना चाहिए; डंप टेबल हटाकर फिर से बनाता है,
  डेटाबेस नहीं।
- `psql` और `gunzip` PATH में होने चाहिए, जिसकी गारंटी इमेज देती है।

माइग्रेशन से पहले के डंप हर स्कीमा बदलाव से पहले अपने-आप लिखे जाते हैं,
`EVEY_BACKUP_DIR` के अंदर (`/data/backups`, यानी होस्ट पर
`./evey-data/backups`), और उनका नाम
`pre-migration-<schema version>-<UTC timestamp>.sql.gz` होता है। सबसे हाल के 3 स्कीमा वर्ज़न में से हर एक का सबसे नया
डंप रखा जाता है, बाकी हटा दिए जाते हैं।
[डेटाबेस माइग्रेशन](migration-runbook.md) बताता है कि स्कीमा को एक कदम पीछे ले
जाने के बजाय ऐसा डंप कब वापस लाना चाहिए।

<!-- sources
reset-password / rotate-jwt-key: cmd/evey/admin.go - adminResetPassword (DeleteByUserID + BumpTokenEpoch), adminRotateJWTKey, endAllSessions ; epoch cache TTL: internal/app/user/token_epoch.go - epochCacheTTL
restore-backup: cmd/evey/admin.go - adminRestoreBackup, verifyDumpComplete (whole gzip + completion marker before anything runs), 5 s pause, --single-transaction, ON_ERROR_STOP=1, .sql.gz suffix)
admin commands boot the embedded database: docker/entrypoint.sh
refresh validation by stored hash: internal/app/user/auth.go - validateRefreshToken
disable signs out: internal/app/user/user_service.go - DisableUser
dump naming and retention: internal/adapters/driven/postgres/migrate_backup.go - dumpDatabase, backupFilePrefix, pruneOldBackups, defaultBackupRetention
defaults: internal/config/config.go (EVEY_JWT_PRIVATE_KEY_PATH, EVEY_BACKUP_DIR)
-->
