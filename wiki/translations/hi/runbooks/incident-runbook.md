---
source_sha: d6c99c91037371ee077e57625c65e0ac542e80e0737a6e83c3320644603ab562
---

# घटना रनबुक

Evey में आम गड़बड़ियों के लिए पहली प्रतिक्रिया की पुस्तिका। हर अनुभाग का ढाँचा एक ही है: **लक्षण → जाँच → समाधान**।

Container: `evey-server`, `evey-postgres`, `evey-meilisearch`, `evey-caddy` (प्रोडक्शन), `evey-web` + `evey-mailcatcher` (dev override)। डिफ़ॉल्ट इंस्टॉलेशन में **सिर्फ़ `evey-server`** होता है: Postgres और Meilisearch उसी के भीतर चलते हैं - नीचे दिए `docker exec evey-postgres psql -U ...` की जगह `docker exec evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey ...'` लिखें, और उनके लॉग `docker logs evey-server` में `postgres:` तथा Meili वाली पंक्तियाँ हैं।

डिफ़ॉल्ट HTTP पोर्ट: `9091` (`EVEY_HTTP_PORT`)। Health endpoint: `/healthz`।

**लॉग।** एक ही धारा की दो प्रतियाँ हैं: रोज़ बनने वाली फ़ाइल `evey-data/logs/evey-YYYY-MM-DD.log` (`EVEY_LOG_DIR`, image के भीतर `/data/logs`) और `docker logs evey-server`। **फ़ाइल ही प्रामाणिक है**: json-file ड्राइवर `docker logs` को घुमा देता है, और घूमने के बाद यह कमांड container के रुकने से कई घंटे पहले ही ख़त्म हो सकती है - तब किसी load test का `stop_detail` या कोई calibration फ़ैसला ग़ायब लगता है, जबकि वह सिर्फ़ घुमाव में चला गया होता है। दोनों में फ़र्क़ दिखे तो फ़ाइल पढ़ें, या root के तौर पर कच्ची `/var/lib/docker/containers/<id>/*-json.log*` फ़ाइलें। नीचे दी `docker logs` की एक-पंक्ति कमांड यह मानकर चलती हैं कि खिड़की हाल की है और घूमी नहीं है।

## 1. सर्वर तक पहुँच नहीं हो रही

**लक्षण**: क्लाइंट जुड़ नहीं पाते; `curl http://localhost:9091/healthz` या तो टाइम आउट हो जाता है या connection refused देता है।

**जाँच**:
```sh
docker ps --filter name=evey-server
docker logs --tail=200 evey-server
docker inspect --format='{{.State.Health.Status}}' evey-server
ss -ltnp | grep 9091                # port actually listening?
curl -v http://localhost:9091/healthz
```

**आम कारण और समाधान**:
- Container बंद हो गया → panic देखने के लिए `docker logs evey-server`, ठीक करें, फिर `docker compose up -d evey`।
- Healthcheck इसलिए विफल है क्योंकि DB या Meili बंद है → अनुभाग 4 और 5 देखें।
- होस्ट पर पोर्ट की टक्कर → `EVEY_HTTP_PORT` बदलें या पोर्ट ख़ाली करें।
- आगे लगा Caddy रूट नहीं कर रहा → `docker logs evey-caddy`, `EVEY_DOMAIN` जाँचें और देखें कि `evey` का healthcheck हरा है (Caddy उसी पर निर्भर है)।

## 2. हर लॉगिन 401 लौटा रहा है

**लक्षण**: हर उपयोगकर्ता - वे व्यवस्थापक भी जो कल ही लॉगिन हुए थे - को `/api/v1/auth/*` से 401 मिलता है।

**जाँच**:
```sh
docker logs --since 10m evey-server | grep -iE 'epoch|jwt|auth|signing|private key'
docker exec evey-server ls -l /data/jwt.key
```

**पहले epoch वाली पंक्ति खोजें।** प्रमाणित हर अनुरोध उपयोगकर्ता का token epoch डेटाबेस से दोबारा मिलाता है (ADR-0019), और यह जाँच विफल होने पर बंद हो जाती है। इसलिए धीमा, लॉक या अनुपलब्ध डेटाबेस सर्वर के हर अनुरोध से वही 401 निकलवाता है जो एक्सपायर टोकन देता है - पढ़ने में यह बिलकुल JWT की समस्या लगती है, पर है नहीं। सर्वर यह साफ़ कहता है:

```
auth rejected: the token epoch could not be read, so every authenticated
request is failing - check the database, not the JWT key
```

यह पंक्ति मिल जाए तो इस अनुभाग को यहीं छोड़ें और §1 (डेटाबेस) पर जाएँ। यहाँ `/healthz` फिर भी हरा रह सकता है: वह सिर्फ़ कनेक्शन को पिंग करता है, epoch वाली क्वेरी नहीं चलाता। Prometheus का काउंटर `evey_auth_failures_total{reason="epoch_lookup_failed"}` है।

**आम कारण और समाधान**:
- **JWT कुंजी ग़ायब या बदल गई**: `EVEY_JWT_PRIVATE_KEY_PATH` (डिफ़ॉल्ट `/data/jwt.key`) पढ़ी नहीं जा रही, या उसे बदल दिया गया। पुरानी कुंजी से हस्ताक्षरित हर टोकन अब अमान्य है - बैकअप से पुरानी कुंजी लौटाने या हर उपयोगकर्ता से दोबारा लॉगिन कराने के अलावा कोई उपाय नहीं। पक्का करें कि `evey-data` वॉल्यूम `/data` पर सही से माउंट है।
- **हर अनुरोध पर rate limit लग रहा है**: `EVEY_RATELIMIT_LOGIN_MAX` (डिफ़ॉल्ट `10` प्रति IP प्रति मिनट) और `EVEY_RATELIMIT_GLOBAL_MAX` (डिफ़ॉल्ट `300`) देखें। जो reverse proxy सारा ट्रैफ़िक एक ही IP से आता दिखाता है, वह इसे तुरंत छू लेगा - `EVEY_TRUSTED_PROXIES` सेट करें और पुनः आरंभ करें। dev override पहले से `1000` / `10000` रखता है।
- सर्वर और क्लाइंट की घड़ियों में अंतर → होस्ट पर `date -u` चलाएँ।

## 3. ट्रांसकोडिंग विफल है / ट्रांसकोड किए गए वीडियो नहीं चलते

**लक्षण**: सीधी प्लेबैक चलती है, ट्रांसकोड ज़रूरी होते ही विफल हो जाती है। प्लेयर त्रुटि दिखाता है और सर्वर लॉग में ffmpeg के exit code आते हैं।

**जाँच**:
```sh
docker exec evey-server which ffmpeg ffprobe
docker exec evey-server ffmpeg -version | head -1
docker logs --since 10m evey-server | grep -iE 'ffmpeg|transcode|hls'
docker exec evey-server ls -ld /cache/transcode /data/trickplay
docker exec evey-server df -h /cache
```

**आम कारण और समाधान**:
- **ffmpeg बाइनरी ग़ायब है** (उसके बिना बनी कस्टम image): आधिकारिक `Dockerfile` से दोबारा बनाएँ।
- **कैश डायरेक्टरी में लिखा नहीं जा सकता**: कोड में `EVEY_TRANSCODING_CACHE_DIR` का डिफ़ॉल्ट `/data/transcode` है, पर compose उसे `evey-cache` नामक वॉल्यूम के ज़रिए `/cache/transcode` पर मैप करता है। `EVEY_TRICKPLAY_CACHE_DIR` का डिफ़ॉल्ट `/cache/trickplay` है। लिखने की जाँच:
  ```sh
  docker exec evey-server touch /cache/transcode/.write-test && \
    docker exec evey-server rm /cache/transcode/.write-test
  ```
- **कैश भर गया** → अनुभाग 6 देखें।
- **HW accel ग़लत कॉन्फ़िगर है**: सॉफ़्टवेयर पर मजबूर करने के लिए `EVEY_TRANSCODING_HW_ACCEL_ENABLED=false` रखें, पुनः आरंभ करें, और दोबारा चालू करने से पहले पक्का करें कि गड़बड़ी वाक़ई हार्डवेयर से जुड़ी है।
- **एक साथ बहुत ज़्यादा ट्रांसकोड**: `EVEY_TRANSCODING_MAX_CONCURRENT` (डिफ़ॉल्ट `2`)। मशीन में कोर हों तो सावधानी से बढ़ाएँ।

### 3c. « रुक-रुक कर चलता है / बीच में कट गया » और लॉग कारण नहीं बताते

**लक्षण**: कोई दर्शक किसी ट्रांसकोड किए शीर्षक पर रुकावट या कटाव बताता है, और सर्वर लॉग में `hls-vod: segment timeout` के साथ `err:"context canceled"` दिखता है - जिसका अर्थ है कि **क्लाइंट** ने संबंध तोड़ा, सर्वर का समय समाप्त नहीं हुआ।

**जाँच**: सर्वर अपना हर फ़ैसला पहले ही दर्ज कर चुका है। वहीं से शुरू करें, क्योंकि वह मुफ़्त है:

```sh
# Why a segment request reused an encoder or spawned one, and what it cost the
# player. Only logged when it costs something: a wait beyond one segment, or a
# spawn. `over_client_patience: true` is the smoking gun.
docker exec evey-server grep '"software: reach decision"' /data/logs/evey-$(date +%F).log | tail -40

# The encoders themselves: spawns, kills and their reason.
docker exec evey-server grep -E '"software: session (started|killed)"|per-hash cap' \
  /data/logs/evey-$(date +%F).log | tail -40
```

ये पंक्तियाँ बताती हैं कि सर्वर ने क्या *तय किया*। जब आपको यह चाहिए कि ffmpeg ने सेकंड-दर-सेकंड वास्तव में क्या *कर पाया* - यानी समय के साथ सीमा-रेखा की असली स्थिति - तब हर encoder की प्रगति की रिकॉर्डिंग चालू करें:

```sh
# ON. Applies to the NEXT encoder; no restart needed. $TOKEN is an admin JWT.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"true"}'

# Reproduce the problem, then read one session: out_time = content produced so
# far, speed = how far ahead of real time the encoder ran.
docker exec evey-server ls -t /data/logs/ffmpeg | head
docker exec evey-server tail -20 /data/logs/ffmpeg/<file>.log

# OFF again when you are done.
curl -sf -X PUT http://localhost:9091/api/v1/admin/server-config/transcode.ffmpeg_progress_log \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"value":"false"}'
```

**जाँच के बाहर इसे बंद ही रखें।** यह हर encoder के लिए एक फ़ाइल बनाता है, हर सत्र पर कुछ MB, और मशीन पर होने वाली हर प्लेबैक के लिए लिखी जाती है - कमज़ोर NAS पर यह असली डिस्क और असली लेखन है। फ़ाइल के नाम में फ़ैसले का hash, rung (`-vN-`) और आरंभिक segment (`-sN-`) होते हैं, इसलिए वह ऊपर वाले JSON लॉग से मेल खाती है; उसी स्थान पर दोबारा शुरू हुआ encoder पिछली फ़ाइल पर लिखने के बजाय अपनी अलग फ़ाइल पाता है, और यही encoder की उथल-पुथल को दिखने लायक़ बनाता है।

**आम कारण और समाधान**:
- **बार-बार छोटी आगे की छलाँगें**: हर छलाँग एक नया encoder जन्मा सकती है, और per-hash सीमा (`MaxSessionsPerHash`, डिफ़ॉल्ट 2) सबसे पुराने प्रयुक्त encoder को हटा देती है। लॉग में `session killed reason:lru_cap` अपेक्षित है। सीमित मात्रा में यह सामान्य है।
- **क्लाइंट सर्वर से कम धैर्यवान है**: seek-reach खिड़की `clientSegmentPatience` (6 s) के हिसाब से बनी है; AndroidX Media3 लगभग 8 s पर segment छोड़ देता है जबकि सर्वर का `WaitBudgetDur` 60 s है। जिस `reach decision` पंक्ति में `over_client_patience: true` हो, उसका अर्थ है कि सर्वर ऐसी प्रतीक्षा माँगने वाला था जो कोई प्लेयर नहीं झेलता।

### 3b. `stale_rendition` वाले 410 / प्लेयर बीच खेल में अपना master दोबारा माँगता है

**लक्षण**: प्लेयर थोड़ी देर रुकता है फिर सँभल जाता है; लॉग में `hls-vod: stale rendition URL refused (the ladder moved under a live player)` दिखता है और अनुरोधों का उत्तर `410 Gone` के साथ `X-Evey-Error: stale_rendition` होता है।

**इसका अर्थ**: यह *सुरक्षा का चलना* है, विफलता नहीं। हर manifest URI में वह decision hash होता है जिसके तहत वह बना था (`fp=`)। 410 का मतलब है कि प्लेयर ने ऐसे सत्र का URL पेश किया जिसकी ABR सीढ़ी तब से बदल चुकी है - उसे परोसना decoder को किसी दूसरे rendition के बाइट देना होता (वही « rainbow screen » वाली घटना-श्रेणी, ADR 0022 §1b)। इनकार करने से प्लेयर साफ़-सुथरा master दोबारा ले आता है।

**जाँच**: WARN पंक्ति में `sealed_fp` बनाम `current_fp` और media id होते हैं।
- **किसी deploy/restart या व्यवस्थापक की recalibration के तुरंत बाद का झोंका** अपेक्षित है: चालू सत्र capability बदलने के बाद भी जीवित रह गए। कुछ करने की ज़रूरत नहीं; प्लेयर एक poll में फिर मिल जाते हैं।
- **सामान्य संचालन के दौरान लगातार टपकना** बताता है कि कोई चीज़ चालू धाराओं के नीचे सीढ़ियाँ बदलती रहती है - capability का फड़फड़ाना देखें (`docker logs evey-server | grep -iE 'calibrat|capability'`), भार के नीचे मंथन करता admission shaper, या दोलन करता CMCD cap override।
- जो प्लेयर **410 पर लूप करते रहते हैं और सँभलते नहीं**, वे पुराना master पकड़े बैठे हैं (पुराना ऐप बिल्ड, या `.m3u8` को ज़ोर-शोर से कैश करता proxy)। पक्का करें कि proxy manifest कैश नहीं करता (`Cache-Control: no-cache` आर-पार जाना चाहिए)।

## 4. डेटाबेस कनेक्शन अस्वीकृत / त्रुटियाँ

**लक्षण**: सर्वर लॉग में `dial tcp ... connect: connection refused`, `pq: ...` दिखता है, या क्वेरियाँ टाइम आउट होती हैं।

**जाँच**:
```sh
docker ps --filter name=evey-postgres
docker inspect --format='{{.State.Health.Status}}' evey-postgres
docker exec evey-postgres pg_isready -U "$EVEY_DB_USER"
docker logs --tail=200 evey-postgres
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
  SELECT count(*), state
    FROM pg_stat_activity
   WHERE datname = current_database()
   GROUP BY state;"
```

**आम कारण और समाधान**:
- Postgres container बंद है → `docker compose up -d postgres`, और लॉग में corruption या oom देखें।
- ग़लत प्रमाण-पत्र → सर्वर का env `postgres` सेवा के `EVEY_DB_USER` / `EVEY_DB_PASSWORD` / `EVEY_DB_NAME` से मेल खाना चाहिए। डिफ़ॉल्ट: `evey` / `evey` / `evey`।
- Pool ख़ाली नहीं बचा (`too many connections` या लंबा इंतज़ार) → `EVEY_DB_MAX_OPEN_CONNS` (डिफ़ॉल्ट `50`) देखें। अटके सत्र समाप्त करें:
  ```sql
  SELECT pg_terminate_backend(pid)
    FROM pg_stat_activity
   WHERE state = 'idle in transaction'
     AND now() - state_change > interval '5 minutes';
  ```
- `postgres-data/` वाली डिस्क भरी है → `df -h`, जगह ख़ाली करें, पुनः आरंभ करें।

## 5. Meilisearch बंद है

**लक्षण**: खोज कमज़ोर परिणाम देती है, लॉग में `meilisearch unavailable, using SQL search fallback` दिखता है।

**व्यवहार**: Evey में सहनशील विकल्प मौजूद है - खोज SQL के ज़रिए चलती रहती है (`cmd/evey/wire.go` एक resilient search adapter जोड़ता है, जो Meili तक न पहुँच पाने पर `meilisearch unavailable, using SQL search fallback` लिखता है)। ऑनलाइन बने रहने के लिए उपयोगकर्ता को कुछ नहीं करना पड़ता; सिर्फ़ क्रमांकन की गुणवत्ता घटती है।

**परिनियोजन के तरीक़े पर टिप्पणी**: डिफ़ॉल्ट `docker-compose.yml` Meilisearch को **`evey-server` container के भीतर embedded** चलाता है (`EVEY_EMBEDDED_MEILI: "true"`)। अलग `evey-meilisearch` container सिर्फ़ dev override में है (`docker-compose.override.yml`)। embedded तरीक़े में « Meilisearch बंद है » का अर्थ है कि evey-server प्रक्रिया ही अस्वस्थ है - पुनः आरंभ करने को कोई अलग container है ही नहीं।

**जाँच और समाधान (embedded, डिफ़ॉल्ट)**:
```sh
docker ps --filter name=evey-server
curl -sf http://localhost:7700/health          # port forwarded from inside evey-server
docker logs --tail=200 evey-server | grep -i meili
```
- सर्वर अस्वस्थ → `docker compose restart evey-server`।
- सूचकांक भ्रष्ट → evey-server रोकें, `./evey-data/meili-data/` का बैकअप लें, मिटाएँ, पुनः आरंभ करें, और व्यवस्थापन पैनल से पूरा re-index चलाएँ।

**जाँच और समाधान (सिर्फ़ standalone dev override)**:
```sh
docker ps --filter name=evey-meilisearch
curl -sf http://localhost:7700/health
docker logs --tail=200 evey-meilisearch
```
- Container बंद → `docker compose up -d meilisearch`।
- `MEILI_MASTER_KEY` मेल नहीं खाती → सर्वर लॉग में auth विफलताएँ दिखती हैं। दोनों सेवाओं का env एक कर के पुनः आरंभ करें।
- सूचकांक भ्रष्ट → Meili रोकें, `./meili-data/` का बैकअप लें, मिटाएँ, पुनः आरंभ करें, और व्यवस्थापन पैनल से पूरा re-index चलाएँ।

## 6. ट्रांसकोड कैश की डिस्क भर गई

**लक्षण**: ट्रांसकोड `no space left on device` के साथ विफल होते हैं; container का healthcheck डगमगाता है।

**जाँच**:
```sh
docker exec evey-server df -h /cache
docker exec evey-server du -sh /cache/transcode /cache/trickplay /data/trickplay 2>/dev/null
docker volume inspect evey_evey-cache   # compose-mangled name may vary
```

**समाधान**:
- सुरक्षित सफ़ाई - कैश दोबारा बन सकता है:
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/transcode/*'
  ```
  इससे उसके नीचे का `photos/` (फ़ोटो के आकार बदले हुए रूप) और `track-art/` (किसी ट्रैक के टैग में जड़ा कवर, एक बार निकालकर आकार बदला हुआ) भी हटता है - दोनों अगले अनुरोध पर फिर बन जाते हैं। व्यवस्थापन पैनल का « Clear cache » बटन यही सफ़ाई करता है।
- Trickplay दोबारा बनाना महँगा है; ज़रूरी हो तभी मिटाएँ:
  ```sh
  docker exec evey-server sh -c 'rm -rf /cache/trickplay/* /data/trickplay/*'
  ```
- होस्ट का वॉल्यूम तंग हो तो ट्रांसकोड कैश का बजट घटाएँ (Admin > सर्वर सेटिंग्स, डिफ़ॉल्ट `10` GB); बजट जो भी हो, sweeper कैश वॉल्यूम पर 2 GiB ख़ाली रखता है।

## 7. एक्सटेंशन क्रैश हो गया

**लक्षण**: कोई एक्सटेंशन जवाब देना बंद कर देता है; सर्वर लॉग में plugin runner से panic आते हैं।

**जाँच**:
```sh
docker logs --since 30m evey-server 2>&1 | grep -iE 'plugin|pluginrunner|panic'
# Look for: "panic in plugin runner", "plugin process crashed", "remove plugin sock on crash"
```

**समाधान**:
- पहले सर्जिकल पुनरारंभ आज़माएँ, container बंद किए बिना: `POST /api/v1/admin/plugins/<id>/enable`। यह तब भी काम करता है जब एक्सटेंशन पहले से `enabled` हो पर `Error` स्थिति में अटका हो - `StartPlugin` का गार्ड स्थिति के `Running` न होते ही उसे दोबारा चला देता है। पूरे सर्वर को पुनः आरंभ करने के बजाय यही चुनें, ख़ासकर तब जब होस्ट तक SSH पहुँच आसानी से न हो।
- सर्जिकल पुनरारंभ से बात न बने तो व्यवस्थापन पैनल से उस एक्सटेंशन को बंद कर दें।
- सर्वर स्वयं अस्थिर हो तो पुनः आरंभ करें: `docker compose restart evey`।
- एक्सटेंशन की फ़ाइलें `./evey-data/plugins/` (होस्ट) == `/data/plugins` (container) पर देखें। टूटी हुई zip या डायरेक्टरी हटाएँ, फिर पुनः आरंभ करें।
- panic का trace और plugin id एक्सटेंशन के लेखक को भेजें।

## 8. कास्ट जम गया, आवाज़ चली गई, या फ़ोन घूमता ही रहता है

नीचे सब कुछ व्यवस्थापन के Logs पृष्ठ से पढ़ा जा सकता है (या `GET /api/v1/admin/logs/search?q=<term>&limit=200` से): मशीन पर shell की ज़रूरत नहीं। समय UTC में हैं। हर beacon एक `player-client: playback event reported` पंक्ति है, जिसमें `type=` और एक `details=` स्ट्रिंग होती है। इन routes के जवाब में क्रेडेंशियल कभी नहीं दिखते (bearer, JWT और `token=`/`password=` के मान `[redacted]` दिखते हैं), इसलिए cast URL का token लॉग खोजकर नहीं मिलता - token नहीं, मीडिया id खोजें।

1. **receiver ज़िंदा था या नहीं, और कब तक?** `cast_receiver_alive` खोजें। चलते रहने पर receiver हर 30 s में एक भेजता है; `details` में `buf=<behind>/<ahead>/<ranges>` (playhead के पीछे और आगे बफ़र हुए सेकंड) तथा `pos_sec` होते हैं। स्वस्थ प्लेबैक में `ahead` दहाई सेकंडों में रहता है। **आख़िरी beacon का समय ही वह क्षण है जब receiver मरा या मारा गया**: उसके बाद कुछ न होने का अर्थ है कि टेलीविज़न अब receiver नहीं चला रहा, फ़ोन चाहे जो दिखाए।
2. **वह रुका क्यों?** `cast_receiver_finished` खोजें: `reason=INTERRUPTED` का अर्थ है कि किसी नए LOAD ने चालू वाले की जगह ले ली (कुछ सेकंड पहले का `StartStream` देखें), और `reason=ERROR` के साथ प्लेयर की त्रुटि आती है। आख़िरी `alive` के बाद कोई `finished` ही न हो = receiver प्रक्रिया ग़ायब हो गई (अक्सर 20+ Mbps के copy master पर मेमोरी के कारण मारा गया Chromecast HD)।
3. **फ़ोन ने क्या देखा?** `cast_sender_stall` खोजें: जब Cast SDK अब भी PLAYING कहता है पर स्थिति 20 s से हिली नहीं, तब sender `state=playing frozen=<n>s pos=<p>` भेजता है, और receiver के idle कारण बताने पर `state=idle reason=<r>`। ये पंक्तियाँ WARN स्तर पर लिखी जाती हैं, इसलिए Logs पृष्ठ का चेतावनी फ़िल्टर इन्हें सामने ले आता है। यह उसी मिनट का sender वाला पहलू है - इसके `pos` की तुलना receiver के आख़िरी `pos_sec` से करें।
4. **क्या सत्र की शृंखला टूटी?** `refresh rejected` खोजें: « not found » वाला refresh token ऐप को लॉग आउट कर के दोबारा लॉग इन कराता है (सहेजे गए प्रमाण-पत्रों से), जिससे धारा फिर से जारी होती है - अब उस पंक्ति में token का `jti`, `issued` और `age` होते हैं, जिनसे पुरानी प्रति और जाली प्रति में फ़र्क़ किया जा सकता है। दोबारा जारी होना देखने के लिए `StartStream` खोजें; एक ही शीर्षक के लिए दोबारा जारी हुए URL पर sender अब receiver को फिर से लोड नहीं करता।
5. **क्या धारा उपकरण के लिए बहुत बड़ी थी?** `advertises a fallback rung` (copy master की शिखर बिटरेट) और `plan fitted to the client's line` (receiver की `maxKbps` माप या उसका `abr=floor` बचाव, अब plan के इनपुट - उसके बाद का transcode master `client_line_exceeded` या `abr_floor_requested` कारण रखता है) खोजें: जो सत्र 20+ Mbps के copy master पर शुरू हुआ और कुछ सेकंड बाद सीमित 1080p rung पर गिर गया, वहाँ floor rescue अपना काम कर रहा है; पर जो re-LOAD फिर से copy master पर ही शुरू होता है, वही Chromecast HD को मारता है।
6. **सर्वर की तरफ़ भुखमरी?** `on-demand resource refused` खोजें: लगातार segment पर `context canceled` वाले इनकारों का झोंका scrub है (हर अनुरोध अगले ने रद्द किया), सर्वर की ख़राबी नहीं; लंबी अवधि वाली `served` पंक्तियाँ ज़रूर ख़राबी हैं।

अगर चरण 1-3 में receiver ज़िंदा और sender चुप दिखे, तो समस्या फ़ोन के Cast सत्र में है (SDK, या TV तक का नेटवर्क); और अगर receiver के beacon पहले रुकें, तो टेलीविज़न ही देखने की जगह है (मेमोरी, codec, धारा की बिटरेट)।

## किसी संदिग्ध सत्र या उपकरण को समाप्त करें

लक्षण: व्यवस्थापक को **Now Playing** (व्यवस्थापन पैनल) में कोई अनपेक्षित उपकरण दिखता है, या कोई उपयोगकर्ता अपनी सत्र सूची में अनजान उपकरण बताता है।

1. `session_id` पहचानें:
   ```sh
   docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
     SELECT id, user_id, profile_id, device, ip_address, started_at
     FROM play_sessions
     WHERE ended_at IS NULL
     ORDER BY started_at DESC LIMIT 20;"
   ```
2. व्यवस्थापन UI से समाप्त करें: **Admin → Activity → Now Playing → ⋯ → Kill session**, या `session_id` के साथ gRPC `AdminSessionService/KillSession` से। सत्र 5s बाद समाप्त होता है ताकि प्लेयर रुकने का आदेश पा सके।
3. सत्र जवाब न दे तो HLS सत्र को ज़बरन हटाएँ (डायरेक्टरी `<cacheDir>/<mediaItemID>` होती है, डिफ़ॉल्ट `/cache/transcode/<mediaItemID>`):
   ```sh
   docker exec evey-server rm -rf /cache/transcode/<media_item_id>/
   ```
4. अगर refresh token भी रद्द करना हो (पासवर्ड के उजागर होने का संदेह):
   ```sh
   docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
     DELETE FROM refresh_tokens WHERE user_id = '<user_id>';"
   ```
   उस उपयोगकर्ता के सभी उपकरणों को दोबारा लॉग इन करना होगा। पूरी तालाबंदी के लिए `evey admin reset-password --user <username>` से पासवर्ड बदलें।

## अटके हुए लाइब्रेरी स्कैन को छुड़ाएँ

लक्षण: कोई लाइब्रेरी स्कैन घंटों से « in progress » दिखता है पर लॉग में फ़ाइलों की कोई प्रगति नहीं।

1. `scan_service.scanning` map को परोक्ष रूप से देखें - प्रक्रिया अटकी हो तो CPU ख़ाली रहता है और हाल के लॉग में कोई `scan_service` नहीं आता:
   ```sh
   docker compose logs --tail=500 evey-server | grep scan_service
   ```
2. स्कैन का ताला **मेमोरी में** है (कोई DB पंक्ति नहीं), इसलिए सर्वर पुनः आरंभ करने से वह खुल जाता है:
   ```sh
   docker compose restart evey-server
   ```
3. कोई एक फ़ाइल बार-बार panic करा रही हो (stack trace देखें), तो उसे लाइब्रेरी के रास्ते से बाहर हटाकर अलग कर दें, फिर दोबारा स्कैन करें।
4. inotify watch ख़त्म हो गए हों (कर्नेल की सीमा), तो बढ़ाएँ:
   ```sh
   echo fs.inotify.max_user_watches=524288 | sudo tee -a /etc/sysctl.conf
   sudo sysctl -p
   ```

## सामान्य जाँच-सूची

```sh
# Status of everything
docker compose ps
curl -sf http://localhost:9091/healthz && echo healthy

# Tail every container
docker compose logs --tail=100 -f

# Server config dump
docker exec evey-server env | grep ^EVEY_ | sort

# Postgres size / locks
docker exec evey-postgres psql -U "$EVEY_DB_USER" -d "$EVEY_DB_NAME" -c "
  SELECT pg_size_pretty(pg_database_size(current_database()));"

# Host disk
df -h . ./postgres-data ./meili-data ./evey-data
```
