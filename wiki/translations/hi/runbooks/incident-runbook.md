---
source_sha: 95bca5df52c2505d699a3857e7d305e19067387a32df91af30da948dfa493ae0
reviewed: true
---
# इंसिडेंट रनबुक

Evey की आम रुकावटों में सबसे पहले क्या करें, ड्यूटी पर मौजूद ऑपरेटर के लिए। वह
हिस्सा ढूँढें जो लक्षण से मेल खाता है, उसकी जाँचें चलाएँ, और जो मिला उससे मेल
खाने वाला उपाय लागू करें। हर हिस्से का ढाँचा एक जैसा है: **लक्षण**, **जाँच**,
**उपाय**।

## शुरू करने से पहले

कमांड आम इंस्टॉल के लिए हैं: एक कंटेनर, `evey-server`, जिसके अंदर Postgres और
Meilisearch हैं। उनकी लाइनें सर्वर वाले लॉग में ही आती हैं (डेटाबेस के लिए
`postgres:` से शुरू होती हैं)। रिपॉज़िटरी की `docker-compose.yml`, `evey-postgres`
और `evey-caddy` कंटेनर जोड़ती है; देखें
[उन्नत डिप्लॉयमेंट](../operations/advanced-deployments.md)।

**पोर्ट।** HTTP API `EVEY_SERVER_PORT` + 1 पर सुनता है, डिफ़ॉल्ट रूप से `9091`;
`/healthz` वहीं जवाब देता है। compose की `ports:` लाइन ही इसे बाहर खोलती है।

embedded डेटाबेस पर **SQL शेल**:

```sh
docker exec -it evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

**लॉग।** एक ही आउटपुट दो जगह मिलता है: रोज़ की फ़ाइल
`./evey-data/logs/evey-YYYY-MM-DD.log` (`EVEY_LOG_DIR`, इमेज के अंदर
`/data/logs`) और `docker logs evey-server`। फ़ाइल ही भरोसेमंद है: Docker का
json-file ड्राइवर `docker logs` को रोटेट करता है, और रोटेशन के बाद यह कमांड
कंटेनर से घंटों पहले ही रुक सकती है। जब दोनों में फ़र्क हो, तो फ़ाइल पढ़ें। एडमिन
पैनल का **लॉग और डीबग** पेज इसी आउटपुट में खोजता है
(`GET /api/v1/admin/logs/search?q=<term>&limit=200`); वहाँ bearer, JWT और
`token=` / `password=` के मान `[redacted]` दिखते हैं, इसलिए मीडिया id से खोजें,
टोकन से कभी नहीं।

## सर्वर जवाब नहीं देता

**लक्षण**: क्लाइंट कनेक्ट नहीं हो पाते; `curl http://localhost:9091/healthz`
का समय खत्म हो जाता है या उसे मना कर दिया जाता है।

**जाँच**:

```sh
docker ps --filter name=evey-server
docker inspect --format='{{.State.Health.Status}}' evey-server
docker logs --tail=200 evey-server
ss -ltnp | grep 9091                # क्या पोर्ट सुन भी रहा है?
curl -v http://localhost:9091/healthz
```

**उपाय**:

- कंटेनर बंद हो गया: panic `docker logs evey-server` के आखिर में है। कारण ठीक
  करें, फिर `docker compose up -d evey`।
- हेल्थचेक लाल है क्योंकि डेटाबेस या सर्च इंजन बंद है: देखें
  [डेटाबेस की गड़बड़ियाँ](#database-errors) और
  [सर्च कमज़ोर हो गई है](#search-is-degraded)।
- होस्ट पर पोर्ट का टकराव: compose की `ports:` लाइन में होस्ट वाला हिस्सा बदलें,
  या पोर्ट खाली करें।
- आगे लगा Caddy अनुरोध आगे नहीं भेज रहा (रिपॉज़िटरी वाला ढाँचा): `docker logs
  evey-caddy`, `EVEY_DOMAIN` जाँचें, और यह भी कि `evey` का हेल्थचेक हरा है,
  क्योंकि Caddy उसका इंतज़ार करता है।

## हर लॉगिन 401 देता है

**लक्षण**: हर उपयोगकर्ता को, एडमिन समेत, `/api/v1/auth/*` से 401 मिलता है।

**जाँच**:

```sh
docker logs --since 10m evey-server | grep -iE 'epoch|jwt|auth|signing|private key'
docker exec evey-server ls -l /data/jwt.key
```

सबसे पहले epoch वाली लाइन खोजें। हर प्रमाणित (authenticated) माँग उपयोगकर्ता के टोकन epoch को
डेटाबेस से दोबारा जाँचती है, और यह जाँच विफल होने पर माँग को मना कर देती है: धीमा,
लॉक या पहुँच से बाहर डेटाबेस हर माँग को वही 401 दिलाता है जो एक्सपायर हुआ टोकन
देता है, जो JWT की समस्या जैसा लगता है पर है नहीं। सर्वर यह बताता है:

```
auth rejected: the token epoch could not be read, so every authenticated request is failing - check the database, not the JWT key
```

अगर यह लाइन मौजूद है, तो [डेटाबेस की गड़बड़ियाँ](#database-errors) पर जाएँ। इस
बीच `/healthz` हरा रह सकता है: वह कनेक्शन को पिंग करता है, epoch वाली क्वेरी नहीं
चलाता। Prometheus काउंटर है
`evey_auth_failures_total{reason="epoch_lookup_failed"}`।

**उपाय**:

- **गायब या बदली हुई JWT कुंजी शायद ही कभी कारण होती है।** अगर कुंजी वाली फ़ाइल
  (`EVEY_JWT_PRIVATE_KEY_PATH`, `/data/jwt.key`) पढ़ी नहीं जा सकती, तो सर्वर शुरू
  ही नहीं होता; अगर वह गायब है, तो फिर से बना ली जाती है। तब पुरानी कुंजी से साइन
  हुए एक्सेस टोकन मना कर दिए जाते हैं, लेकिन ऐप उन्हें अपने रिफ़्रेश टोकन से
  नया कर लेते हैं, जो डेटाबेस में जाँचा जाता है, कुंजी से नहीं: उपयोगकर्ता साइन इन
  रहते हैं। पक्का करें कि `./evey-data` वॉल्यूम `/data` पर माउंट है; उसके बिना
  डेटाबेस भी गायब होता है।
- **रेट लिमिटर हर माँग पर लागू हो जाता है**: `EVEY_RATELIMIT_LOGIN_MAX` (हर IP के लिए
  हर मिनट `10`) और `EVEY_RATELIMIT_GLOBAL_MAX` (`300`)। जो रिवर्स प्रॉक्सी सब कुछ
  एक ही पते से आगे भेजता है, वह इस सीमा तक जल्दी पहुँच जाता है: प्रॉक्सी को **एडमिन →
  रिमोट एक्सेस** में **विश्वसनीय प्रॉक्सी** में जोड़ें (रीस्टार्ट के बिना लागू
  होता है; `EVEY_TRUSTED_PROXIES` वैकल्पिक तरीका (fallback) है)। रिपॉज़िटरी की
  `docker-compose.override.yml` डेवलपमेंट के लिए दोनों को `1000` / `10000` कर
  देती है।
- सर्वर और क्लाइंट के बीच **घड़ी का अंतर**: होस्ट पर `date -u`।

## ट्रांसकोडिंग फ़ेल होती है

**लक्षण**: सीधा प्लेबैक काम करता है, जिस प्लेबैक को ट्रांसकोड चाहिए वह फ़ेल होता
है; प्लेयर गड़बड़ी दिखाता है और सर्वर के लॉग में ffmpeg के exit कोड दिखते हैं।

**जाँच**:

```sh
docker exec evey-server which ffmpeg ffprobe
docker exec evey-server ffmpeg -version | head -1
docker logs --since 10m evey-server | grep -iE 'ffmpeg|transcode|hls'
docker exec evey-server ls -ld /cache/transcode /data/trickplay
docker exec evey-server df -h /cache
docker exec evey-server touch /cache/transcode/.write-test && \
  docker exec evey-server rm /cache/transcode/.write-test
```

**उपाय**:

- **ffmpeg गायब है** (कोई अपनी बनाई इमेज): आधिकारिक `Dockerfile` से फिर बिल्ड
  करें।
- **कैश डायरेक्टरी में लिखा नहीं जा सकता**: इमेज
  `EVEY_TRANSCODING_CACHE_DIR=/cache/transcode` (`evey-cache` वॉल्यूम) और
  `EVEY_TRICKPLAY_CACHE_DIR=/data/trickplay` सेट करती है; compose की
  `environment:` वाली एंट्री इन्हें बदल देती है। इमेज के बाहर Go के डिफ़ॉल्ट
  `/data/transcode` और `/cache/trickplay` हैं।
- **कैश भर गया**: देखें [कैश वाली डिस्क भर गई है](#the-cache-disk-is-full)।
- **हार्डवेयर एक्सेलेरेशन गड़बड़ कर रहा है**: **एडमिन → ट्रांसकोडिंग → एनकोड
  कैलिब्रेशन** में **हार्डवेयर एक्सेलेरेशन** को **बंद (केवल सॉफ़्टवेयर)** पर करें
  (तुरंत लागू होता है और फिर से कैलिब्रेट करता है), ताकि हर ट्रांसकोड प्रोसेसर
  पर चले; हार्डवेयर ठीक होने पर वापस **स्वतः (सर्वोत्तम उपलब्ध)** पर कर दें।
- **"सर्वर अन्य स्ट्रीम ट्रांसकोड करने में व्यस्त है"**: लाइव क्षमता नापी जाती
  है, सेट नहीं की जाती; देखें
  [क्षमता बाँटना](../playback/transcoding.md#sharing-the-capacity)।
  `EVEY_TRANSCODING_MAX_CONCURRENT` (**एडमिन → ट्रांसकोड कैश** में **अधिकतम
  समवर्ती**) सिर्फ़ लाइव प्लेबैक के बाहर वाले ट्रांसकोड कामों पर लागू होता है।

### प्लेबैक बफ़र होता है या कटता है, और लॉग वजह नहीं बताता

**लक्षण**: कोई दर्शक बताता है कि ट्रांसकोड हुआ टाइटल अटकता है; लॉग में
`hls-vod: segment timeout` के साथ `err:"context canceled"` दिखता है, जिसका मतलब
है कि क्लाइंट ने कनेक्शन छोड़ दिया, न कि सर्वर का समय खत्म हुआ।

**जाँच**: उससे शुरू करें जो सर्वर पहले ही दर्ज कर चुका है, इसमें कोई अतिरिक्त खर्च नहीं:

```sh
# किसी सेगमेंट की माँग ने पुराना एन्कोडर इस्तेमाल किया या नया शुरू किया, क्यों,
# और प्लेयर को इसकी क्या कीमत पड़ी। सिर्फ़ तभी लॉग होता है जब कोई कीमत हो।
# `over_client_patience: true` ही असली सुराग है।
docker exec evey-server grep '"software: reach decision"' /data/logs/evey-$(date +%F).log | tail -40

# खुद एन्कोडर: शुरू होना, बंद किया जाना, और उसकी वजह।
docker exec evey-server grep -E '"software: session (started|killed)"|per-hash cap' \
  /data/logs/evey-$(date +%F).log | tail -40
```

ये लाइनें बताती हैं कि सर्वर ने क्या फ़ैसला किया। ffmpeg ने सेकंड-दर-सेकंड क्या
हासिल किया, यह देखने के लिए हर एन्कोडर का प्रगति लॉग चालू करें (`$TOKEN` एक
एडमिन JWT है):

```sh
# चालू। अगले एन्कोडर पर लागू होता है, रीस्टार्ट नहीं चाहिए।
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"true"}'

# समस्या दोहराएँ, फिर एक सेशन पढ़ें: out_time = अब तक बनी सामग्री,
# speed = एन्कोडर रियल टाइम से कितना आगे चला।
docker exec evey-server ls -t /data/logs/ffmpeg | head
docker exec evey-server tail -20 /data/logs/ffmpeg/<file>.log

# काम पूरा होने पर बंद।
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"false"}'
```

!!! note
    जाँच के अलावा प्रगति लॉग बंद रखें: हर एन्कोडर के लिए एक फ़ाइल, हर सेशन
    के कुछ MB, मशीन पर हर प्लेबैक के लिए। फ़ाइल के नाम में फ़ैसले का hash, rung
    (`-vN-`) और शुरुआती सेगमेंट (`-sN-`) होता है, ताकि वह JSON लॉग से
    मिलाई जा सके; उसी जगह पर दोबारा शुरू हुए एन्कोडर को अपनी अलग फ़ाइल मिलती है,
    और इसी से एन्कोडर का बार-बार बदलना दिखता है।

**उपाय**:

- **बार-बार थोड़ा आगे जाना (सीक)**: हर बार एक नया एन्कोडर शुरू हो सकता है, और
  हर hash की सीमा (`MaxSessionsPerHash`, डिफ़ॉल्ट रूप से 2, कम मेमोरी वाली
  प्रोफ़ाइल में 1) सबसे कम हाल में इस्तेमाल हुए एन्कोडर को हटा देती है: लॉग में
  `session killed reason:lru_cap`। थोड़ा-बहुत होना सामान्य है।
- **क्लाइंट सर्वर से कम धैर्य रखता है**: सीक तक पहुँचने की window
  `clientSegmentPatience` (6 s) के हिसाब से तय होती है, जबकि सर्वर का अपना
  इंतज़ार का बजट (`WaitBudgetDur`) 60 s है। `over_client_patience: true` वाली
  `reach decision` लाइन का मतलब है कि सर्वर ऐसा इंतज़ार माँगने वाला था जितना कोई
  प्लेयर इंतज़ार नहीं करता।

### `stale_rendition` वाले 410, प्लेयर प्लेबैक के बीच अपना master फिर से लाता है

**लक्षण**: प्लेयर थोड़ी देर अटकता है, फिर ठीक हो जाता है; लॉग में
`hls-vod: stale rendition URL refused (the ladder moved under a live player)`
दिखता है और माँगों का जवाब `410 Gone` के साथ `X-Evey-Error: stale_rendition`
होता है।

यह खुद गड़बड़ी नहीं है, बल्कि एक सुरक्षा है जो चालू हुई है। हर manifest URI में
वह फ़ैसले का hash होता है जिसके तहत वह बना था (`fp=`)। 410 का मतलब है कि किसी
प्लेयर ने ऐसे सेशन का URL दिया जिसका ABR ladder तब से बदल चुका है; उसे जवाब देने
का मतलब होता डिकोडर को किसी दूसरे rendition के बाइट देना। मना करना प्लेयर को
master साफ़ तरीके से फिर से लाने पर मजबूर करता है।

**जाँच**: WARN में `sealed_fp`, `current_fp` और मीडिया id होते हैं।

- **डिप्लॉय, रीस्टार्ट या फिर से कैलिब्रेशन के ठीक बाद एक साथ कई 410** अपेक्षित हैं:
  लाइव सेशन क्षमता के किसी बदलाव के बाद भी चलते रहे। प्लेयर एक पोल के अंदर फिर
  से सिंक हो जाते हैं।
- **आम काम के दौरान लगातार थोड़े-थोड़े 410** का मतलब है कि कोई चीज़ लाइव स्ट्रीम के
  चलते समय ladder बदलती रहती है: क्षमता का बार-बार बदलना खोजें (`docker logs
  evey-server | grep -iE 'calibrat|capability'`), लोड में बार-बार बदलता admission
  shaper, या ऊपर-नीचे होता CMCD सीमा ओवरराइड।
- जो प्लेयर **बार-बार 410 पाते रहते हैं**, उनके पास कैश किया हुआ master है
  (ऐप का पुराना बिल्ड, `.m3u8` कैश करने वाला प्रॉक्सी)। प्रॉक्सी को
  `Cache-Control: no-cache` आगे भेजना चाहिए।

## डेटाबेस की गड़बड़ियाँ

**लक्षण**: लॉग में `dial tcp ... connect: connection refused`, `pq: ...` है, या
क्वेरी का समय खत्म हो जाता है।

**जाँच**:

```sh
docker logs --tail=300 evey-server | grep -iE 'postgres|database'
docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
  SELECT count(*), state
    FROM pg_stat_activity
   WHERE datname = current_database()
   GROUP BY state;"'
df -h ./evey-data
```

**उपाय**:

- `./evey-data` के नीचे **डिस्क भर गई** (क्लस्टर `./evey-data/postgres` में
  रखा जाता है): जगह खाली करें, फिर `docker compose restart evey`।
- **पूल खत्म हो गया** (`too many connections`, लंबे इंतज़ार):
  `EVEY_DB_MAX_OPEN_CONNS` (डिफ़ॉल्ट `25`)। अटके सेशन बंद करें:
  ```sql
  SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
   WHERE state = 'idle in transaction'
     AND now() - state_change > interval '5 minutes';
  ```
- **अलग Postgres कंटेनर** (रिपॉज़िटरी वाला ढाँचा): `docker ps --filter
  name=evey-postgres`, `docker logs --tail=200 evey-postgres`; सर्वर के
  `EVEY_DB_USER` / `EVEY_DB_PASSWORD` / `EVEY_DB_NAME` को `postgres` सर्विस से
  मेल खाना चाहिए।

## सर्च कमज़ोर हो गई है

**लक्षण**: सर्च के नतीजे खराब आते हैं; लॉग में `meilisearch
unavailable, using SQL search fallback` दिखता है।

जब Meilisearch तक पहुँचा नहीं जा सकता, तब भी सर्च SQL से चलती रहती है; सिर्फ़
नतीजों का क्रम कम अच्छा होता है, कोई भी बंद नहीं होता। Meilisearch
`evey-server` के अंदर चलता है (`EVEY_EMBEDDED_MEILI`, डिफ़ॉल्ट रूप से चालू),
कंटेनर के loopback पर, पोर्ट 7700 पर।

**जाँच**:

```sh
docker exec evey-server curl -sf http://127.0.0.1:7700/health
docker logs --tail=300 evey-server | grep -i meili
```

**उपाय**:

- स्वस्थ नहीं: `docker compose restart evey`।
- इंडेक्स खराब हो गया: कंटेनर रोकें, `./evey-data/meili` को हटाकर कहीं और रख दें, फिर
  से शुरू करें, फिर डेटाबेस से इंडेक्स फिर से बनाएँ:
  ```sh
  docker exec evey-server evey admin reindex-search
  ```
  फिर से बनाना एक कमांड है, पैनल का कोई बटन नहीं।

## कैश वाली डिस्क भर गई है

**लक्षण**: ट्रांसकोड `no space left on device` के साथ फ़ेल होते हैं; हेल्थचेक
बार-बार बदलता रहता है।

**जाँच**:

```sh
docker exec evey-server df -h /cache
docker exec evey-server du -sh /cache/transcode /data/trickplay 2>/dev/null
docker volume inspect evey_evey-cache   # compose का प्रीफ़िक्स अलग हो सकता है
```

**उपाय**:

- ट्रांसकोड कैश खाली करें, यह फिर से बन सकता है:
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/transcode/*'
  ```
  इससे उसके अंदर के `photos/` (फ़ोटो के छोटे-बड़े किए गए रूप) और `track-art/`
  (ऑडियो टैग से निकाले गए कवर) भी हट जाते हैं; दोनों अगली माँग पर फिर से बनते
  हैं। **एडमिन → ट्रांसकोड कैश** में **सभी साफ़ करें** पैनल से यही कैश खाली
  करता है।
- Trickplay को फिर से बनाना ज़्यादा महँगा है; उसे सिर्फ़ ज़रूरत हो तभी हटाएँ:
  ```sh
  docker exec evey-server sh -c 'rm -rf /data/trickplay/*'
  ```
- अगर होस्ट का वॉल्यूम तंग है, तो **एडमिन → सर्वर कॉन्फ़िग → सर्वर सेटिंग्स**
  में **ट्रांसकोड कैश बजट (GB)** कम करें (डिफ़ॉल्ट `10`)। बजट जो भी हो, सफ़ाई
  करने वाला काम कैश वॉल्यूम पर 2 GiB खाली रखता है।

## प्लगइन क्रैश हो गया

**लक्षण**: कोई प्लगइन जवाब देना बंद कर देता है; लॉग में प्लगइन runner से panic
दिखते हैं।

**जाँच**:

```sh
docker logs --since 30m evey-server 2>&1 | grep -iE 'plugin|pluginrunner|panic'
# ये खोजें: "panic in plugin runner", "plugin process crashed", "remove plugin sock on crash"
```

**उपाय**:

1. सर्वर को छुए बिना प्लगइन फिर से शुरू करें: **एडमिन → प्लगइन** में उसे फिर
   से **सक्षम करें**, या `POST /api/v1/admin/plugins/<id>/enable`। यह तब काम करता
   है जब प्लगइन सक्षम है लेकिन `Error` में अटका है: शुरू करने का रास्ता सिर्फ़
   उस प्लगइन को मना करता है जो पहले से `Running` या `Initializing` है।
2. अगर इससे ठीक न हो, तो पैनल से प्लगइन को **अक्षम करें**।
3. अगर सर्वर खुद अस्थिर है: `docker compose restart evey`।
4. प्लगइन की फ़ाइलें होस्ट पर `./evey-data/plugins/` में रखी जाती हैं (कंटेनर में
   `/data/plugins`)। खराब प्लगइन की डायरेक्टरी हटाएँ, रीस्टार्ट करें।
5. panic का trace और प्लगइन का id प्लगइन बनाने वाले को भेजें।

## कास्ट अटक गया, आवाज़ चली गई, या फ़ोन पर बस घूमता हुआ निशान दिखता है

यहाँ सब कुछ **एडमिन → लॉग और डीबग** से पढ़ा जा सकता है; मशीन पर किसी शेल की
ज़रूरत नहीं। समय UTC में हैं। हर संकेत (beacon) एक `player-client: playback
event reported` लाइन है, जिसमें `type=` और एक `details=` स्ट्रिंग होती है।

1. **क्या रिसीवर चालू था, और कब तक?** `cast_receiver_alive` खोजें। चलते समय
   रिसीवर हर 30 s पर एक भेजता है; `details` में
   `buf=<behind>/<ahead>/<ranges>` (प्लेहेड के आसपास बफ़र हुए सेकंड) और
   `pos_sec` होते हैं। ठीक चल रहे प्लेबैक में `ahead` दसियों सेकंड में होता है।
   आखिरी संकेत का समय वह है जब रिसीवर बंद हुआ या बंद किया गया: उसके बाद कुछ न
   होने का मतलब है कि टेलीविज़न अब रिसीवर नहीं चला रहा, फ़ोन चाहे जो दिखाए।
2. **यह क्यों रुका?** `cast_receiver_finished` खोजें: `reason=INTERRUPTED` का
   मतलब है कि एक नया LOAD चल रहे LOAD की जगह ले रहा है (कुछ सेकंड पहले का
   `StartStream` खोजें), `reason=ERROR` में प्लेयर की गड़बड़ी होती है। आखिरी
   `alive` के बाद कोई `finished` न होने का मतलब है कि रिसीवर का प्रोसेस गायब हो
   गया, अक्सर एक Chromecast HD जिसे 20+ Mbps वाले copy master पर मेमोरी के लिए
   बंद कर दिया गया।
3. **फ़ोन ने क्या देखा?** `cast_sender_stall` खोजें: भेजने वाला
   `state=playing frozen=<n>s pos=<p>` भेजता है जब Cast SDK अब भी PLAYING कहता है
   लेकिन जगह 20 s से नहीं हिली, और `state=idle reason=<r>` जब रिसीवर ने रुकने
   (idle) की कोई वजह बताई। ये WARN लाइनें हैं, इसलिए पेज का चेतावनी वाला फ़िल्टर
   इन्हें सामने लाता है। इसके `pos` की तुलना रिसीवर के आखिरी `pos_sec` से करें।
4. **क्या सेशन में एक के बाद एक गड़बड़ी हुई?** `refresh rejected` खोजें: रिफ़्रेश
   टोकन का `not found` ऐप को साइन आउट करके फिर से साइन इन कराता है (सहेजी गई
   जानकारी से), जो स्ट्रीम फिर से जारी करता है; लाइन में टोकन का `jti`, `issued`
   और `age` होता है, जिससे पुरानी कॉपी और नकली टोकन में फ़र्क पता चलता है। फिर से
   जारी हुई स्ट्रीम देखने के लिए `StartStream` खोजें।
5. **क्या स्ट्रीम डिवाइस के लिए बहुत बड़ी थी?** `advertises a fallback
   rung` (copy master का सबसे ऊँचा बिटरेट) और `plan fitted to the client's
   line` खोजें (रिसीवर का `maxKbps` माप या उसका `abr=floor` बचाव; उसके बाद आने
   वाले ट्रांसकोड master में वजह `client_line_exceeded` या `abr_floor_requested`
   होती है)। जो सेशन 20+ Mbps वाले copy master पर शुरू हुआ और कुछ सेकंड बाद
   फ़िट किए गए 1080p rung पर आ गया, उसमें बचाव ठीक से काम कर रहा है; copy master पर
   फिर से शुरू होने वाला re-LOAD वह पैटर्न है जो Chromecast HD को बंद कर देता है।
6. **सर्वर की तरफ़ कमी?** `on-demand resource refused` खोजें: लगातार सेगमेंट पर
   `context canceled` वाले कई इनकार एक साथ आगे-पीछे खिसकाना (scrub) है (हर माँग
   अगली माँग से रद्द होती है), सर्वर की गलती नहीं; लंबी अवधि वाली `served` लाइनें
   सर्वर की गलती हैं।

अगर कदम 1-3 दिखाएँ कि रिसीवर चालू है और भेजने वाला चुप है, तो समस्या फ़ोन के Cast
सेशन में है (SDK, TV तक का नेटवर्क)। अगर रिसीवर के संकेत पहले रुकते हैं, तो
टेलीविज़न देखें: मेमोरी, कोडेक, स्ट्रीम का बिटरेट।

## किसी सेशन या असुरक्षित हुए डिवाइस को रोकना

**लक्षण**: **एडमिन → अभी चल रहा है** में कोई अनजान डिवाइस, या कोई उपयोगकर्ता
अपनी सेशन सूची में किसी अनजान डिवाइस की शिकायत करता है।

1. सेशन पहचानें:
   ```sh
   docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
     SELECT id, profile_id, device_name, client_ip, started_at
       FROM play_sessions
      WHERE ended_at IS NULL
      ORDER BY started_at DESC LIMIT 20;"'
   ```
2. इसे **एडमिन → अभी चल रहा है** से रोकें: सेशन पर रोकने वाला आइकन (**सत्र
   रोकें**), फिर पुष्टि के लिए **रोकें**; या gRPC पर सेशन id के साथ
   `AdminSessionService/KillSession`। प्लेयर को पहले रुकने का आदेश मिलता है और
   स्ट्रीम 5 सेकंड बाद खत्म की जाती है। इससे सिर्फ़ प्लेबैक खत्म होता है;
   उपयोगकर्ता साइन इन रहता है।
3. अगर स्ट्रीम फिर भी चलती रहे, तो कंटेनर रीस्टार्ट करें: हर एन्कोडर उसके साथ
   रुक जाता है।
   ```sh
   docker compose restart evey
   ```
4. अगर खाता ही असुरक्षित हो गया है, तो **एडमिन → उपयोगकर्ता → अक्षम करें**:
   रिफ़्रेश टोकन मिटा दिए जाते हैं और एक्सेस टोकन तुरंत रद्द हो जाते हैं, इसलिए
   हर डिवाइस साइन आउट हो जाता है। नया पासवर्ड सेट करें (`evey admin reset-password
   --user <email>`, देखें
   [रिकवरी](recovery.md#give-a-user-a-new-password)), फिर खाते को दोबारा **सक्षम
   करें**।

## लाइब्रेरी स्कैन अटक गया है

**लक्षण**: कोई स्कैन घंटों से चलता हुआ दिखता है, और लॉग में किसी फ़ाइल की प्रगति
नहीं दिखती।

1. जाँचें कि स्कैन अब भी कुछ लॉग करता है या नहीं; अटके स्कैन में CPU खाली होता है
   और हाल की कोई `scan:` लाइन नहीं होती:
   ```sh
   docker logs --tail=500 evey-server | grep 'scan:'
   ```
2. स्कैन का लॉक मेमोरी में होता है, इसलिए रीस्टार्ट उसे हटा देता है:
   ```sh
   docker compose restart evey
   ```
3. अगर एक ही फ़ाइल बार-बार panic कराती है (stack trace में उसका नाम होता है), तो
   उसे लाइब्रेरी के रास्ते से बाहर ले जाएँ, फिर दोबारा स्कैन करें।
4. अगर फ़ाइल मॉनिटर `watcher: inotify failed for path, will use
   periodic scan` लॉग करता है, तो कर्नेल की inotify watch की सीमा खत्म हो गई है:
   ```sh
   echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf
   sudo sysctl -p
   ```

## कमांड की संक्षिप्त सूची

```sh
# हर चीज़ की हालत
docker compose ps
curl -sf http://localhost:9091/healthz && echo healthy

# हर कंटेनर को फ़ॉलो करें
docker compose logs --tail=100 -f

# असल में लागू कॉन्फ़िगरेशन
docker exec evey-server env | grep ^EVEY_ | sort

# डेटाबेस का साइज़
docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
  SELECT pg_size_pretty(pg_database_size(current_database()));"'

# होस्ट की डिस्क
df -h ./evey-data
```

<!-- sources
ports: internal/config/config.go (EVEY_SERVER_PORT, EVEY_GRPC_BIND) ; cmd/evey/main.go - httpPort = Port+1 ; Dockerfile - HEALTHCHECK
logs: Dockerfile ENV EVEY_LOG_DIR ; internal/adapters/driving/http/admin_handler.go (GET /api/v1/admin/logs/search)
epoch fail-closed: internal/adapters/driving/http/auth_middleware.go, internal/adapters/driving/grpcapi/interceptors/auth.go ; internal/pkg/metrics/metrics.go - AuthFailuresTotal
rate limits: internal/config/config.go (EVEY_RATELIMIT_LOGIN_MAX, EVEY_RATELIMIT_GLOBAL_MAX, EVEY_TRUSTED_PROXIES) ; docker-compose.override.yml
cache dirs: Dockerfile ENV EVEY_TRANSCODING_CACHE_DIR / EVEY_TRICKPLAY_CACHE_DIR ; docker-compose.yml ; internal/config/config.go defaults
hardware mode and calibration labels: i18n/en/admin.yaml - transcodeCalibration.hwModeLabel, hwModeOff, hwModeAuto, cardTitle
software producer: internal/app/transcoding/software_producer.go - clientSegmentPatience, WaitBudgetDur, MaxSessionsPerHash, "software: reach decision", over_client_patience ; internal/app/transcoding/software_producer_lifecycle.go - "software: session killed" ; cmd/evey/wire.go - low-memory profile, transcode.ffmpeg_progress_log, logs/ffmpeg
server-config route: proto/evey/v1/server_config.proto - SetServerConfigEntry (PUT /api/v1/admin/server-config/{key})
stale rendition: internal/adapters/driving/http/transcode_handler_vod.go - stale_rendition, sealed_fp
db pool: internal/config/config.go (EVEY_DB_MAX_OPEN_CONNS)
search fallback: cmd/evey/wire_infra.go ; cmd/evey/admin.go - adminReindexSearch ; docker/entrypoint.sh (EVEY_EMBEDDED_MEILI, MEILI_DATA_DIR)
cache budget: internal/config/config.go (EVEY_TRANSCODING_CACHE_MAX_GB) ; internal/app/transcoding/segment_cache_evict.go - DefaultMinFreeBytes ; internal/adapters/driving/http/server.go - track-art under PhotoCacheDir ; cmd/evey/http.go - PhotoCacheDir
plugins: internal/adapters/driven/pluginrunner/runner_start.go (panic/crash lines) ; internal/app/plugin/runtime_service.go - StartPlugin ; proto/evey/v1/admin_plugin.proto - EnablePlugin
cast beacons: internal/adapters/driving/http/player_event_handler.go ; internal/app/user/auth.go - "refresh rejected" ; internal/adapters/driving/http/transcode_ondemand.go ; internal/app/transcoding/resolver_line.go
sessions: internal/adapters/driven/postgres/migrations/000001_init.up.sql - play_sessions ; internal/adapters/driving/grpcapi/admin_audit.go - KillSession (5 s) ; internal/app/user/user_service.go - DisableUser
scan: internal/app/library/scan_service.go - scanning map ; internal/app/library/watcher.go - inotify warning
labels: i18n/en/admin.yaml - sidebar (logs, nowPlaying, remoteAccess, serverConfig, transcodeCache, users, plugins), nowPlaying.stopSession, transcodeCache.clearAll, transcodeCache.maxConcurrent, remoteAccess.trustedProxies, serverSettings.transcodeCacheMaxGB ; i18n/en/common.yaml - enable, disable
-->
