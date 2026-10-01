---
source_sha: cb3a20b8cc9545de2abfb60adb9e7c2272d0ad6fbc8666679ce2cb180603bae4
reviewed: false
---
# घटना पुस्तिका

Evey की आम रुकावटों के लिए पहली प्रतिक्रिया की पुस्तिका। हर सेक्शन: **लक्षण → निदान → समाधान**।

कमांड मानक इंस्टॉलेशन के लिए हैं: एक कंटेनर, `evey-server`, जिसके अंदर Postgres और Meilisearch चलते हैं; उनके लॉग `docker logs evey-server` की `postgres:` और Meilisearch वाली लाइनें हैं। रिपॉज़िटरी का `docker-compose.yml` लेआउट `evey-postgres` और `evey-caddy` कंटेनर जोड़ता है ([उन्नत डिप्लॉयमेंट](../operations/advanced-deployments.md))।

बिल्ट-इन डेटाबेस पर SQL चलाने के लिए:

```sh
docker exec evey-server sh -c \
  'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

डिफ़ॉल्ट HTTP पोर्ट: `9091` (`EVEY_HTTP_PORT`)। हेल्थ एंडपॉइंट: `/healthz`।

**लॉग।** एक ही स्ट्रीम की दो प्रतियाँ: रोज़ की फ़ाइल `evey-data/logs/evey-YYYY-MM-DD.log` (`EVEY_LOG_DIR`, इमेज के अंदर `/data/logs`) और `docker logs evey-server`। फ़ाइल प्रामाणिक है: json-file ड्राइवर `docker logs` को घुमाता है, और घुमाव के बाद कमांड कंटेनर से घंटों पहले ख़त्म हो सकता है; लोड टेस्ट का `stop_detail` या कैलिब्रेशन का निष्कर्ष तब ग़ायब लगता है जबकि वह केवल घुमाव में चला गया होता है। जब दोनों में अंतर हो, तो फ़ाइल पढ़ें, या root के रूप में कच्ची `/var/lib/docker/containers/<id>/*-json.log*` फ़ाइलें। नीचे के `docker logs` कमांड हाल की, बिना घुमाई गई अवधि मानते हैं।

## 1. सर्वर तक नहीं पहुँचा जा सकता

**लक्षण**: क्लाइंट कनेक्ट नहीं कर पाते; `curl http://localhost:9091/healthz` टाइमआउट होता है या connection refused लौटाता है।

**निदान**:

```sh
docker ps --filter name=evey-server
docker logs --tail=200 evey-server
docker inspect --format='{{.State.Health.Status}}' evey-server
ss -ltnp | grep 9091                # क्या पोर्ट सचमुच सुन रहा है?
curl -v http://localhost:9091/healthz
```

**आम कारण + समाधान**:
- कंटेनर बंद हो गया → panic के लिए `docker logs evey-server`, ठीक करें, `docker compose up -d evey`।
- हेल्थचेक विफल है क्योंकि DB या Meili बंद है → सेक्शन 4 और 5 देखें।
- होस्ट पर पोर्ट टकराव → `EVEY_HTTP_PORT` बदलें या पोर्ट ख़ाली करें।
- आगे का Caddy रूट नहीं कर रहा → `docker logs evey-caddy`, `EVEY_DOMAIN` जाँचें और यह कि `evey` हेल्थचेक हरा है (Caddy उस पर निर्भर है)।

## 2. सभी लॉगिन 401 लौटाते हैं

**लक्षण**: हर उपयोगकर्ता, कल लॉग इन करने वाले एडमिन सहित, `/api/v1/auth/*` से 401 पाता है।

**निदान**:

```sh
docker logs --since 10m evey-server | grep -iE 'epoch|jwt|auth|signing|private key'
docker exec evey-server ls -l /data/jwt.key
```

**पहले epoch वाली लाइन खोजें।** हर प्रमाणित अनुरोध उपयोगकर्ता के टोकन epoch को डेटाबेस के विरुद्ध फिर से जाँचता है (ADR-0019), और वह जाँच विफल होने पर मना करती है। इसलिए एक धीमा, लॉक या पहुँच से बाहर डेटाबेस सर्वर के हर अनुरोध को वही 401 लौटवाता है जो समाप्त टोकन लौटाता है, जो बिल्कुल JWT की समस्या जैसा लगता है पर है नहीं। सर्वर यह साफ़ बताता है:

```
auth rejected: the token epoch could not be read, so every authenticated
request is failing - check the database, not the JWT key
```

अगर वह लाइन मौजूद है, तो यह सेक्शन पढ़ना बंद करें और §4 (डेटाबेस) पर जाएँ। `/healthz` यहाँ फिर भी हरा हो सकता है: यह केवल कनेक्शन को पिंग करता है, epoch क्वेरी नहीं चलाता। Prometheus काउंटर है `evey_auth_failures_total{reason="epoch_lookup_failed"}`।

**आम कारण + समाधान**:
- **JWT कुंजी गुम या बदली हुई** शायद ही कारण होती है। न पढ़ा जा सकने वाला या ख़राब `EVEY_JWT_PRIVATE_KEY_PATH` (डिफ़ॉल्ट `/data/jwt.key`) सर्वर को शुरू होने से रोकता है; गुम कुंजी फिर से बनाई जाती है। तब पुरानी कुंजी से हस्ताक्षरित एक्सेस टोकन अस्वीकार होते हैं, पर ऐप उन्हें अपने रिफ़्रेश टोकन से नवीनीकृत कर लेते हैं, जिसकी जाँच डेटाबेस में होती है, कुंजी से नहीं: उपयोगकर्ता साइन इन रहते हैं। पुष्टि करें कि `evey-data` वॉल्यूम `/data` पर माउंट है: उसके बिना डेटाबेस भी गायब होता है।
- **हर अनुरोध पर रेट लिमिट लगना**: `EVEY_RATELIMIT_LOGIN_MAX` (डिफ़ॉल्ट प्रति IP प्रति मिनट `10`) और `EVEY_RATELIMIT_GLOBAL_MAX` (डिफ़ॉल्ट `300`) जाँचें। सारा ट्रैफ़िक एक ही IP के रूप में भेजने वाला रिवर्स प्रॉक्सी इसे जल्दी लगवा देगा: Admin → रिमोट एक्सेस में प्रॉक्सी को **विश्वसनीय प्रॉक्सी** में जोड़ें (बिना रीस्टार्ट के लागू होता है; `EVEY_TRUSTED_PROXIES` भी काम करता है)। डेवलपमेंट ओवरराइड पहले से `1000` / `10000` सेट करता है।
- सर्वर और क्लाइंट के बीच घड़ी का अंतर → होस्ट पर `date -u`।

## 3. ट्रांसकोडिंग विफल होती है / वीडियो ट्रांसकोड होकर नहीं चलते

**लक्षण**: सीधा प्लेबैक काम करता है, ट्रांसकोड ज़रूरी होने पर विफल होता है। प्लेयर त्रुटियाँ, सर्वर लॉग में ffmpeg exit कोड।

**निदान**:

```sh
docker exec evey-server which ffmpeg ffprobe
docker exec evey-server ffmpeg -version | head -1
docker logs --since 10m evey-server | grep -iE 'ffmpeg|transcode|hls'
docker exec evey-server ls -ld /cache/transcode /data/trickplay
docker exec evey-server df -h /cache
```

**आम कारण + समाधान**:
- **ffmpeg बाइनरी गुम** (उसके बिना कस्टम इमेज): आधिकारिक `Dockerfile` से फिर से बनाएँ।
- **कैश डायरेक्टरी में लिखा नहीं जा सकता**: `EVEY_TRANSCODING_CACHE_DIR` कोड में डिफ़ॉल्ट रूप से `/data/transcode` है लेकिन compose इसे `evey-cache` नाम वाले वॉल्यूम के ज़रिए `/cache/transcode` पर मैप करता है। `EVEY_TRICKPLAY_CACHE_DIR` इमेज में `/data/trickplay` है (इमेज के बाहर डिफ़ॉल्ट `/cache/trickplay` है)। लिखने की क्षमता जाँचें:
  ```sh
  docker exec evey-server touch /cache/transcode/.write-test && \
    docker exec evey-server rm /cache/transcode/.write-test
  ```
- **कैश भरा** → सेक्शन 6 देखें।
- **हार्डवेयर एक्सेलेरेशन गड़बड़ कर रहा है**: **Admin → ट्रांसकोडिंग → एनकोड कैलिब्रेशन** में **हार्डवेयर एक्सेलेरेशन** को **बंद (केवल सॉफ़्टवेयर)** पर करें (तुरंत लागू होता है, फिर से मापता है) ताकि हर ट्रांसकोड प्रोसेसर पर रहे, और **स्वतः** पर लौटने से पहले पुष्टि करें कि समस्या हार्डवेयर से जुड़ी है।
- **"सर्वर अन्य स्ट्रीम ट्रांसकोड करने में व्यस्त है"**: लाइव क्षमता मापी जाती है, सेट नहीं की जाती; देखें [ट्रांसकोडिंग और सर्वर क्षमता](../playback/transcoding.md#sharing-the-capacity)। `EVEY_TRANSCODING_MAX_CONCURRENT` (**Admin → ट्रांसकोड कैश** में **अधिकतम समवर्ती**) केवल लाइव प्लेबैक के बाहर के ट्रांसकोड कामों को कवर करता है।

### 3c. "यह बफ़र होता है / यह कट गया" और लॉग कारण नहीं बताते

**लक्षण**: एक दर्शक ट्रांसकोड किए गए टाइटल पर अटकने या कटने की शिकायत करता है, और सर्वर लॉग `err:"context canceled"` के साथ `hls-vod: segment timeout` दिखाते हैं, जिसका मतलब है कि क्लाइंट ने कनेक्शन छोड़ा, सर्वर का टाइमआउट नहीं हुआ।

**निदान**: सर्वर अपने हर निर्णय को पहले से दर्ज करता है। वहीं से शुरू करें, क्योंकि यह मुफ़्त है:

```sh
# किसी सेगमेंट अनुरोध ने एनकोडर फिर से इस्तेमाल किया या नया शुरू किया, और प्लेयर को
# इसकी क्या क़ीमत चुकानी पड़ी। केवल तब लॉग होता है जब कोई क़ीमत हो: एक सेगमेंट से
# ज़्यादा इंतज़ार, या नई शुरुआत। `over_client_patience: true` पक्का सबूत है।
docker exec evey-server grep '"software: reach decision"' /data/logs/evey-$(date +%F).log | tail -40

# एनकोडर ख़ुद: शुरुआत, ख़ात्मा और उनका कारण।
docker exec evey-server grep -E '"software: session (started|killed)"|per-hash cap' \
  /data/logs/evey-$(date +%F).log | tail -40
```

ये लाइनें बताती हैं कि सर्वर ने क्या *तय* किया। जब आपको यह चाहिए कि ffmpeg ने सेकंड दर सेकंड असल में क्या *हासिल* किया (समय के साथ एनकोडिंग सीमा की असली स्थिति), तो हर एनकोडर की प्रगति रिकॉर्डिंग चालू करें:

```sh
# चालू। अगले एनकोडर पर लागू; रीस्टार्ट की ज़रूरत नहीं। $TOKEN एक एडमिन JWT है।
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"true"}'

# समस्या दोहराएँ, फिर एक सेशन पढ़ें: out_time = अब तक बनी सामग्री,
# speed = एनकोडर रीयल-टाइम से कितना आगे चला।
docker exec evey-server ls -t /data/logs/ffmpeg | head
docker exec evey-server tail -20 /data/logs/ffmpeg/<file>.log

# काम पूरा होने पर फिर से बंद करें।
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"false"}'
```

**जाँच के अलावा इसे बंद रखें।** यह हर एनकोडर के लिए एक फ़ाइल है, हर सेशन के कुछ MB, मशीन पर हर प्लेबैक के लिए लिखी जाती है: कमज़ोर NAS पर असली डिस्क और असली लेखन। फ़ाइल के नाम में निर्णय हैश, पायदान (`-vN-`) और शुरुआती सेगमेंट (`-sN-`) होता है, इसलिए यह ऊपर के JSON लॉग से मेल खाता है; उसी स्थिति पर दोबारा शुरू होने पर अपनी फ़ाइल मिलती है बजाय पिछली को अधिलेखित करने के, और यही एनकोडर के बार-बार शुरू होने को दिखाई देने लायक़ बनाता है।

**आम कारण + समाधान**:
- **बार-बार छोटी आगे की सीक**: हर एक नया एनकोडर शुरू कर सकती है, और प्रति हैश सीमा (`MaxSessionsPerHash`, डिफ़ॉल्ट 2) सबसे पहले इस्तेमाल हुए को हटा देती है। लॉग में `session killed reason:lru_cap` की उम्मीद करें। संयम में सामान्य है।
- **क्लाइंट सर्वर से कम धैर्यवान है**: `clientSegmentPatience` (6 s) वह मान है जिसके अनुसार सीक पहुँच की खिड़की तय होती है; AndroidX Media3 किसी सेगमेंट को ~8 s पर छोड़ देता है जबकि सर्वर का `WaitBudgetDur` 60 s है। `over_client_patience: true` वाली `reach decision` लाइन का मतलब है कि सर्वर ऐसा इंतज़ार माँगने वाला था जिसे कोई प्लेयर नहीं झेलता।

### 3b. `stale_rendition` 410 / प्लेयर चलते-चलते अपना master फिर से लाता है

**लक्षण**: प्लेयर थोड़ी देर अटकता है फिर ठीक हो जाता है; लॉग `hls-vod: stale rendition URL refused (the ladder moved under a live player)` दिखाते हैं और अनुरोधों का जवाब `X-Evey-Error: stale_rendition` के साथ `410 Gone` होता है।

**इसका मतलब**: यह एक *सुरक्षा का सक्रिय होना* है, विफलता ख़ुद नहीं। हर मैनिफ़ेस्ट URI उस निर्णय हैश को लिए होता है जिसके तहत वह बना (`fp=`)। 410 का मतलब है कि किसी प्लेयर ने ऐसे सेशन का URL दिया जिसकी ABR सीढ़ी तब से बदल चुकी है: उसे परोसने से डिकोडर को किसी दूसरे रेंडिशन के बाइट मिलते ("इंद्रधनुषी स्क्रीन" वाली घटनाओं की श्रेणी, ADR 0022 §1b)। मना करना इसकी जगह master को साफ़ ढंग से फिर से लाने पर मजबूर करता है।

**निदान**: WARN में `sealed_fp` बनाम `current_fp` और मीडिया id होता है।
- **डिप्लॉय/रीस्टार्ट या एडमिन के फिर से कैलिब्रेट करने के ठीक बाद एक झोंका** अपेक्षित है: लाइव सेशन किसी क्षमता बदलाव के बाद भी चलते रहे। कुछ करने की ज़रूरत नहीं; प्लेयर एक पोल में फिर से तालमेल में आ जाते हैं।
- **सामान्य संचालन के दौरान लगातार टपकना** का मतलब है कि कुछ लाइव स्ट्रीम के नीचे सीढ़ियाँ बदलता रहता है: क्षमता के बार-बार बदलने को खोजें (`docker logs evey-server | grep -iE 'calibrat|capability'`), लोड में डोलता एडमिशन शेपर, या आगे-पीछे होता CMCD सीमा ओवरराइड।
- जो प्लेयर **410 पर बिना ठीक हुए चक्कर लगाते हैं** उनके पास कैश किया हुआ master है (पुराना ऐप संस्करण, `.m3u8` को आक्रामक ढंग से कैश करने वाला प्रॉक्सी)। जाँचें कि प्रॉक्सी मैनिफ़ेस्ट कैश नहीं करता (`Cache-Control: no-cache` आगे जाना चाहिए)।

## 4. डेटाबेस कनेक्शन मना / त्रुटियाँ

**लक्षण**: सर्वर लॉग में `dial tcp ... connect: connection refused`, `pq: ...` होता है, या क्वेरी टाइमआउट होती हैं।

**निदान**:

```sh
docker logs --tail=300 evey-server | grep -iE 'postgres|database'
docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
  SELECT count(*), state
    FROM pg_stat_activity
   WHERE datname = current_database()
   GROUP BY state;"'
df -h ./evey-data
```

**आम कारण + समाधान**:
- `./evey-data` के नीचे **डिस्क भरी** (डेटाबेस `./evey-data/postgres` में रहता है) → जगह ख़ाली करें, फिर `docker compose restart evey`।
- **पूल ख़त्म** (`too many connections` या लंबा इंतज़ार) → `EVEY_DB_MAX_OPEN_CONNS` (डिफ़ॉल्ट `25`)। अटके सेशन ख़त्म करें:
  ```sql
  SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
   WHERE state = 'idle in transaction'
     AND now() - state_change > interval '5 minutes';
  ```
- **अलग Postgres कंटेनर** (रिपॉज़िटरी लेआउट): `docker ps --filter name=evey-postgres`, `docker logs --tail=200 evey-postgres`; सर्वर के `EVEY_DB_USER` / `EVEY_DB_PASSWORD` / `EVEY_DB_NAME` `postgres` सर्विस से मेल खाने चाहिए।

## 5. Meilisearch बंद

**लक्षण**: खोज घटिया नतीजे लौटाती है, लॉग `meilisearch unavailable, using SQL search fallback` दिखाते हैं।

**व्यवहार**: खोज SQL के ज़रिए काम करती रहती है (`cmd/evey/wire.go` एक मज़बूत सर्च एडैप्टर जोड़ता है जो Meilisearch तक न पहुँच पाने पर वह लाइन लॉग करता है)। ऑनलाइन बने रहने के लिए उपयोगकर्ता को कुछ नहीं करना; केवल रैंकिंग की गुणवत्ता घटती है।

Meilisearch `evey-server` के अंदर चलता है (`EVEY_EMBEDDED_MEILI`, डिफ़ॉल्ट रूप से चालू), केवल कंटेनर में `127.0.0.1:7700` पर सुनता है।

**निदान + समाधान**:
```sh
docker exec evey-server curl -sf http://127.0.0.1:7700/health
docker logs --tail=300 evey-server | grep -i meili
```
- अस्वस्थ → `docker compose restart evey`।
- इंडेक्स भ्रष्ट → कंटेनर रोकें, `./evey-data/meili` को अलग हटाएँ, फिर से शुरू करें, फिर डेटाबेस से इंडेक्स फिर से बनाएँ: `docker exec evey-server evey admin reindex-search`।

## 6. ट्रांसकोड कैश डिस्क भरी

**लक्षण**: ट्रांसकोड `no space left on device` के साथ विफल होते हैं; कंटेनर हेल्थचेक डगमगाता है।

**निदान**:

```sh
docker exec evey-server df -h /cache
docker exec evey-server du -sh /cache/transcode /data/trickplay 2>/dev/null
docker volume inspect evey_evey-cache   # compose द्वारा बना नाम अलग हो सकता है
```

**समाधान**:
- सुरक्षित सफ़ाई, कैश फिर से बनाया जा सकता है:
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/transcode/*'
  ```
  यह इसके नीचे के `photos/` (फ़ोटो के छोटे किए गए रूप) और `track-art/` (किसी ट्रैक के टैग में जड़ा कवर, एक बार निकाला और छोटा किया गया) भी हटाता है: दोनों अगले अनुरोध पर फिर से बनते हैं। Admin → ट्रांसकोड कैश में **सभी साफ़ करें** वही सफ़ाई करता है।
- Trickplay फिर से बनाना ज़्यादा महँगा है; केवल ज़रूरत हो तो साफ़ करें:
  ```sh
  docker exec evey-server sh -c 'rm -rf /data/trickplay/*'
  ```
- अगर होस्ट वॉल्यूम तंग है तो **ट्रांसकोड कैश बजट** घटाएँ (**Admin → सर्वर कॉन्फ़िग → सर्वर सेटिंग्स**, डिफ़ॉल्ट `10` GB); बजट चाहे जो हो, सफ़ाई करने वाला कैश वॉल्यूम पर 2 GiB ख़ाली रखता है।

## 7. प्लगइन क्रैश हो गया

**लक्षण**: कोई प्लगइन जवाब देना बंद कर देता है; सर्वर लॉग प्लगइन रनर से panic दिखाते हैं।

**निदान**:

```sh
docker logs --since 30m evey-server 2>&1 | grep -iE 'plugin|pluginrunner|panic'
# खोजें: "panic in plugin runner", "plugin process crashed", "remove plugin sock on crash"
```

**समाधान**:
- पहले एक सटीक रीस्टार्ट आज़माएँ, कंटेनर बंद किए बिना: `POST /api/v1/admin/plugins/<id>/enable`। यह तब भी काम करता है जब प्लगइन पहले से `enabled` है पर `Error` स्थिति में अटका है: जैसे ही स्थिति `Running` न हो, `StartPlugin` की सुरक्षा जाँच उसे फिर से चला देती है। पूरे सर्वर को रीस्टार्ट करने की बजाय इसे पसंद करें, ख़ासकर जब होस्ट तक SSH पहुँच आसानी से उपलब्ध न हो।
- अगर सटीक रीस्टार्ट से ठीक न हो, तो एडमिन पैनल से गड़बड़ प्लगइन अक्षम करें।
- अगर सर्वर ख़ुद अस्थिर है, तो रीस्टार्ट करें: `docker compose restart evey`।
- प्लगइन फ़ाइलें `./evey-data/plugins/` (होस्ट) == `/data/plugins` (कंटेनर) पर देखें। टूटे प्लगइन का zip या डायरेक्टरी हटाएँ, रीस्टार्ट करें।
- panic ट्रेस + प्लगइन id प्लगइन लेखक को भेजें।

## 8. कास्ट अटक गया, आवाज़ चली गई, या फ़ोन घूमता रहता है

नीचे का सब कुछ एडमिन लॉग पेज से पढ़ा जा सकता है (या `GET /api/v1/admin/logs/search?q=<term>&limit=200`): मशीन पर किसी शेल की ज़रूरत नहीं। समय UTC में हैं। हर बीकन `type=` और एक `details=` स्ट्रिंग वाली `player-client: playback event
reported` लाइन है। इन रूटों के लौटाए गए नतीजों में क्रेडेंशियल कभी नहीं आते (bearer, JWT और `token=`/`password=` मान `[redacted]` दिखते हैं), इसलिए कास्ट URL का टोकन लॉग खोज से वापस नहीं मिल सकता: मीडिया id खोजें, टोकन नहीं।

1. **क्या रिसीवर ज़िंदा था, और कब तक?** `cast_receiver_alive` खोजें। रिसीवर चलाते समय हर 30 s पर एक भेजता है; `details` में `buf=<behind>/<ahead>/<ranges>` (प्लेहेड के पीछे और आगे बफ़र किए गए सेकंड) और `pos_sec` होता है। स्वस्थ प्लेबैक में `ahead` दसियों सेकंड में होता है। **आख़िरी बीकन का समय वह है जब रिसीवर मरा या मारा गया**: उसके बाद कुछ नहीं का मतलब है कि टीवी अब रिसीवर नहीं चला रहा, फ़ोन चाहे जो दिखाए।
2. **यह क्यों रुका?** `cast_receiver_finished` खोजें: `reason=INTERRUPTED` एक नया LOAD है जो चल रहे को बदल रहा है (कुछ सेकंड पहले एक `StartStream` देखें), `reason=ERROR` प्लेयर त्रुटि लिए होता है। आख़िरी `alive` के बाद कोई `finished` नहीं = रिसीवर प्रोसेस ग़ायब हो गया (ज़्यादातर, 20+ Mbps कॉपी master पर मेमोरी के कारण मारा गया Chromecast HD)।
3. **फ़ोन ने क्या देखा?** `cast_sender_stall` खोजें: जब Cast SDK अब भी PLAYING कहता है पर स्थिति 20 s से नहीं हिली तो सेंडर `state=playing frozen=<n>s pos=<p>` भेजता है, और जब रिसीवर ने निष्क्रियता का कारण बताया तो `state=idle reason=<r>`। ये लाइनें WARN पर लॉग होती हैं, इसलिए एडमिन लॉग पेज का चेतावनी फ़िल्टर इन्हें दिखाता है। यह उसी मिनट का सेंडर वाला पक्ष है: इसके `pos` की तुलना रिसीवर के आख़िरी `pos_sec` से करें।
4. **क्या सेशन में शृंखला प्रतिक्रिया हुई?** `refresh rejected` खोजें: "not found" रिफ़्रेश टोकन ऐप को लॉग आउट करके फिर लॉग इन करवाता है (सहेजे गए क्रेडेंशियल), जो स्ट्रीम फिर से जारी करता है; लाइन अब टोकन का `jti`, `issued` और `age` बताती है, जो पुरानी प्रति को जाली से अलग करता है। फिर से जारी होने के लिए `StartStream` खोजें; उसी टाइटल के लिए फिर से जारी URL पर सेंडर अब रिसीवर को फिर से लोड नहीं करता।
5. **क्या स्ट्रीम डिवाइस के लिए बहुत बड़ी थी?** `advertises a fallback
   rung` (कॉपी master का अधिकतम बिटरेट) और `plan fitted to the client's
   line` (रिसीवर का `maxKbps` माप या उसका `abr=floor` बचाव, अब योजना का इनपुट: उसके बाद का ट्रांसकोड master `client_line_exceeded` या `abr_floor_requested` कारण लिए होता है) खोजें: जो सेशन 20+ Mbps कॉपी master पर शुरू हुआ और कुछ सेकंड बाद एक मिलाए गए 1080p पायदान पर आ गया, वह न्यूनतम-पायदान बचाव का अपना काम करना है; फिर से कॉपी master पर शुरू होने वाला नया LOAD वही पैटर्न है जो Chromecast HD को मारता है।
6. **सर्वर की ओर संसाधनों की कमी?** `on-demand resource refused` खोजें: लगातार सेगमेंट पर `context canceled` मना करने का झोंका तेज़ी से आगे-पीछे खिसकाना है (हर अनुरोध अगले से रद्द), सर्वर की ग़लती नहीं; लंबी अवधि वाली `served` लाइनें सर्वर की ग़लती हैं।

अगर चरण 1-3 रिसीवर को ज़िंदा और सेंडर को चुप दिखाएँ, तो समस्या फ़ोन के Cast सेशन में है (SDK, टीवी तक का नेटवर्क); अगर रिसीवर के बीकन पहले रुकें, तो टीवी को देखें (मेमोरी, कोडेक, स्ट्रीम का बिटरेट)।

## किसी समझौता किए गए सेशन / डिवाइस को ख़त्म करना

लक्षण: एडमिन को **अभी चल रहा है** (एडमिन पैनल) में कोई अनपेक्षित डिवाइस दिखता है, या कोई उपयोगकर्ता अपनी सेशन सूची में कोई अनजाना डिवाइस बताता है।

1. `session_id` पहचानें:
   ```sh
   docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
     SELECT id, profile_id, device_name, client_ip, started_at
     FROM play_sessions
     WHERE ended_at IS NULL
     ORDER BY started_at DESC LIMIT 20;"'
   ```
2. एडमिन UI से ख़त्म करें: **Admin → अभी चल रहा है**, सेशन पर रोकने का आइकन (**सत्र रोकें**), फिर पुष्टि के लिए **रोकें**; या `session_id` के साथ gRPC `AdminSessionService/KillSession` से। सेशन 5 s बाद ख़त्म होता है ताकि प्लेयर रोकने का आदेश पा सके।
3. अगर स्ट्रीम फिर भी चलती रहे, तो कंटेनर रीस्टार्ट करें: हर एनकोडर उसके साथ रुक जाता है।
   ```sh
   docker compose restart evey
   ```
4. अगर खाता ख़ुद समझौता किया गया है, तो उस पर **Admin → उपयोगकर्ता → अक्षम करें**: यह उसके रिफ़्रेश टोकन हटाता है और उसके एक्सेस टोकन तुरंत अमान्य करता है, इसलिए हर डिवाइस साइन आउट हो जाता है। नया पासवर्ड सेट करें (`evey admin reset-password --user <email>`, जो अपने आप कोई सेशन ख़त्म नहीं करता), फिर खाते को फिर से **सक्षम करें**।

## अटके लाइब्रेरी स्कैन को खोलना

लक्षण: लाइब्रेरी स्कैन घंटों तक "चल रहा है" दिखाता है पर लॉग में कोई फ़ाइल प्रगति नहीं।

1. जाँचें कि क्या स्कैन अब भी कुछ लॉग कर रहा है: अगर प्रोसेस अटका है, तो CPU निष्क्रिय है और हाल के लॉग में कोई `scan:` लाइन नहीं:
   ```sh
   docker logs --tail=500 evey-server | grep 'scan:'
   ```
2. स्कैन लॉक **मेमोरी में** होता है (कोई DB पंक्ति नहीं), इसलिए सर्वर रीस्टार्ट उसे हटा देता है:
   ```sh
   docker compose restart evey
   ```
3. अगर कोई ख़ास फ़ाइल बार-बार panic करवा रही है (स्टैक ट्रेस देखें), तो उसे लाइब्रेरी पाथ से बाहर ले जाकर अलग करें, फिर फिर से स्कैन करें।
4. अगर inotify watch ख़त्म हो गए हैं (कर्नेल सीमा), तो बढ़ाएँ:
   ```sh
   echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf
   sudo sysctl -p
   ```

## सामान्य निदान चीट-शीट

```sh
# हर चीज़ की स्थिति
docker compose ps
curl -sf http://localhost:9091/healthz && echo healthy

# हर कंटेनर के लॉग देखते रहें
docker compose logs --tail=100 -f

# सर्वर कॉन्फ़िग डंप
docker exec evey-server env | grep ^EVEY_ | sort

# डेटाबेस का आकार
docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey -c "
  SELECT pg_size_pretty(pg_database_size(current_database()));"'

# होस्ट डिस्क
df -h ./evey-data
```
