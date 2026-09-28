---
source_sha: a7cdb2195f99559246f23706d11f64ad40c0cb3dcd88a52f3caf61af02c7ce8c
---
# कॉन्फ़िगरेशन

दो जगहें, एक नियम। **जो सर्वर शुरू होने से पहले मौजूद होना चाहिए** - राज़, कंटेनर की
wiring, रिवर्स प्रॉक्सी से जुड़ी बातें - वह compose फ़ाइल में रहता है। **जो संचालक बाद
में समायोजित करता है** - लॉग स्तर, भाषा, ट्रांसकोडिंग सीमाएँ, हार्डवेयर त्वरण, फ़ाइल
निगरानी, एक्सटेंशन सैंडबॉक्स और कोटा - वह **Admin > सर्वर कॉन्फ़िग** में रहता है,
डेटाबेस में सहेजा जाता है, कंटेनर दोबारा बनने के बाद भी बना रहता है, और ज़्यादातर
बिना पुनरारंभ के लागू होता है।

इनमें से किसी को environment में सेट करना सिर्फ़ पहली शुरुआत का बीज डालता है। इसीलिए
यह पृष्ठ बाइनरी की स्वीकार की हुई हर `EVEY_*` की सूची नहीं देता: बाक़ी पहली शुरुआत के
बीज और आपात रास्ते हैं। अगर आपको पूरी सूची चाहिए तो `internal/config/config.go` पूरा
संदर्भ है।

## 1. जो आपको ख़ुद सेट करना है

डिफ़ॉल्ट इंस्टॉल के लिए कुछ नहीं: मीडिया पाथ और इमेज टैग compose फ़ाइल में ही हैं, और
कोई `.env` नहीं है। रिपॉज़िटरी की अपनी `docker-compose.yml` इसके बजाय `MEDIA_DIR` को
`.env` से पढ़ती है:

```env
MEDIA_DIR=/path/to/your/media
```

दोनों हाल में Postgres और Meilisearch कंटेनर में embedded चलते हैं और पहली शुरुआत पर
अपने राज़ ख़ुद बनाते हैं, जो `./evey-data/keys` के तहत रखे जाते हैं
(`postgres.password`, `meili.key`, 0600)। सर्वर उन्हें `EVEY_DB_PASSWORD_FILE` /
`MEILI_MASTER_KEY_FILE` से पढ़ता है (इमेज में पहले से सेट), इसीलिए
`docker exec evey-server evey admin ...` को कमांड लाइन पर पासवर्ड नहीं चाहिए।
`EVEY_DB_PASSWORD` / `MEILI_MASTER_KEY` को तभी साफ़ तौर पर सेट करें जब आप अपना
डेटाबेस या सर्च इंजन ला रहे हों - साफ़ तौर पर दिया गया मान हमेशा फ़ाइल पर भारी पड़ता है।

मेटाडेटा पहले दिन से काम करता है: एक TMDB कुंजी शामिल है। आपकी अपनी कुंजी
(themoviedb.org पर मुफ़्त) आपको आपका अपना कोटा देती है; इसे **Admin > सर्वर कॉन्फ़िग >
API कुंजियाँ** में डालें, या `EVEY_TMDB_API_KEY` से बीज डालें। सर्वर प्रकाशित हो जाने
पर जोड़ने लायक़ एक ही वेरिएबल `EVEY_SERVER_BASE_URL` है - खंड 2।

## 2. प्रकाशित इंस्टेंस, दो वेरिएबल में

रिवर्स प्रॉक्सी के पीछे, हार्डवेयर ट्रांसकोडिंग के साथ, पूरा environment दो वेरिएबल
है। लोग compose फ़ाइल में जो कुछ और आमतौर पर रखते हैं, वह या तो पहले से इमेज में है या
डिफ़ॉल्ट है:

```yaml
services:
  evey:
    image: gravityz/evey:latest
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 10.0.0.5/32      # your reverse proxy, /32 if you know its address
    volumes:
      - ./evey-data:/data
      - evey-cache:/cache
      - /srv/media:/media
    devices:
      - /dev/dri:/dev/dri                     # hardware transcoding
    group_add:
      - "991"                                 # the group owning /dev/dri/renderD128 on YOUR host
```

पुराने सेटअप से जिन पंक्तियों को कॉपी **नहीं** करना चाहिए:

| पंक्ति | यह ग़ैर-ज़रूरी क्यों है |
| --- | --- |
| `EVEY_LOG_DIR`, `EVEY_TRANSCODING_CACHE_DIR`, `EVEY_TRICKPLAY_CACHE_DIR` | इमेज इन्हें पहले से `/data/logs`, `/cache/transcode`, `/data/trickplay` पर सेट करती है। दोहराने से कुछ नहीं बदलता। |
| `EVEY_FORWARDED_PROTO` | अप्रचलित - `EVEY_SERVER_BASE_URL` के scheme से निकाला जाता है। इसे सेट करने से बस अप्रचलन की चेतावनी लॉग होती है। |
| `EVEY_KEYFRAME_BACKFILL=on` | `off` के अलावा कुछ भी इसे चालू करता है, न होना भी। |
| `LIBVA_DRIVER_NAME` | libva GPU की PCI id से ड्राइवर चुनता है। इसे तभी ज़बरदस्ती करें जब आपके कार्ड को ऐसा ड्राइवर चाहिए जिसे libva नहीं चुनता (`docker exec evey-server vainfo` से जाँचें)। |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` आपके होस्ट की सबसे मज़बूत क़ैद चुनता है। इसे साफ़ तौर पर तभी सेट करें जब आप *बंद होकर विफल* होने की गारंटी चाहते हों - `landlock` ऐसे एक्सटेंशन को चलाने से मना करता है जिसे वह क़ैद न कर सके, जबकि `auto` चेतावनी के साथ पीछे हट जाता है। |
| `/dev/dri/card0` के लिए `group_add` | Evey सिर्फ़ `renderD128` खोलता है। `ls -ln /dev/dri` से जाँचें और सिर्फ़ वही group जोड़ें। |

## 3. वे दो सेटिंग्स जिन्हें व्यवस्थापन पैनल नहीं बदलता

दोनों का **Admin → रिमोट एक्सेस** में एक हमशक्ल है, और दोनों में से कोई भी environment
वेरिएबल को फ़ालतू नहीं बनाता। इससे लोग उलझते हैं, इसलिए इसे साफ़ कहना ज़रूरी है।

फ़र्क़ इस बात में है कि हर एक कहाँ रहता है। रिमोट-एक्सेस वाला हिस्सा (चालू/बंद, बाहरी
URL, LAN श्रेणियाँ, रिमोट बिटरेट सीमा) **डेटाबेस में सहेजा जाता है** - इसके environment
वेरिएबल पहली शुरुआत का बीज डालते हैं और उसके बाद अनदेखे होते हैं। `EVEY_SERVER_BASE_URL`
और `EVEY_TRUSTED_PROXIES` इसका उलटा हैं: ये हर शुरुआत पर environment से पढ़े जाते हैं और
**पैनल में कुछ भी इन्हें कभी नहीं बदलता**। इसलिए पैनल में जो मान दोहराव जैसा दिखता है, वह
इन दो मामलों में एक अलग सेटिंग है जो संयोग से किसी पते का ज़िक्र करती है।

**`EVEY_SERVER_BASE_URL` की जगह "बाहरी URL" नहीं ले सकता।** पैनल का मान उन URLs को
खिलाता है जिन्हें सर्वर *बनाता* है। पर सर्वर `X-Forwarded-Proto` पर भरोसा करता है या
नहीं - और इसलिए वह ख़ुद को HTTPS पर परोसा हुआ मानता है या नहीं - यह **शुरुआत पर, सिर्फ़
environment वेरिएबल से** तय होता है। वेरिएबल हटा दें तो वह `http://localhost:9091` पर
लौट आता है: `Secure` cookies, redirects और हर absolute link `http://` पर गिर जाते हैं,
और share/download URLs localhost की ओर इशारा करते हैं। पैनल का मान मदद करने के लिए बहुत
देर से पढ़ा जाता है।

**`EVEY_TRUSTED_PROXIES` की जगह "LAN नेटवर्क" नहीं ले सकता।** ये दो चरण हैं, एक ही चीज़
के दो नाम नहीं:

1. `EVEY_TRUSTED_PROXIES` तय करता है कि **किसके `X-Forwarded-For` पर भरोसा किया जाए**,
   और इसलिए क्लाइंट को कौन सा पता मिलता है।
2. फिर LAN नेटवर्क **उसी** पते को स्थानीय या रिमोट के रूप में वर्गीकृत करते हैं।

दूसरा पहले के नतीजे पर काम करता है। पहले के बिना हर क्लाइंट प्रॉक्सी के पते के साथ आता है
और सब एक जैसे वर्गीकृत होते हैं - ठीक इसी तरह रिमोट-एक्सेस चौकी चार महीने तक निष्क्रिय
रही (देखें `internal/config/remote_access_resolve.go`)। loopback पर हमेशा भरोसा होता है
और उसे हटाया नहीं जा सकता; आपकी प्रविष्टि उसमें *जुड़ती* है।

## 4. फ़ाइलों का मालिक (PUID/PGID)

**डिफ़ॉल्ट: स्वचालित।** जब `PUID`/`PGID` सेट न हों, entrypoint शुरुआत पर आपके
`/media*` mounts का मालिक देखता है और सर्वर को उसी उपयोगकर्ता के रूप में चलाता है, ताकि
आपकी लाइब्रेरी *में लिखने* वाली सुविधाएँ (seek-index मरम्मत फ़ाइलों को उसी जगह फिर से
बनाती है) पहले दिन से काम करें - न `chown`, न कॉन्फ़िगरेशन। शुरुआती लॉग बताता है कि क्या
चुना गया:

```
evey: media mounts owned by 1026:100 (detected on /media) - running the
server as that user so write features (seek-index repair) work out of the box
```

**अलग-अलग उपयोगकर्ताओं** के मीडिया mounts को जितना बन पड़े उतना सँभाला जाता है: सर्वर
सबसे ज़्यादा mounts वाले मालिक को अपनाता है और बाक़ी mounts के groups में अतिरिक्त
groups के रूप में जुड़ता है, ताकि उन mounts पर मरम्मत तब काम करे जब उनकी फ़ाइलें group
के लिए लिखने लायक़ हों (शुरुआती चेतावनी बताती है कि कौन से mounts अब भी लिखने से मना कर
सकते हैं)। स्वचालित पहचान पूरी तरह तभी पीछे हटती है (चेतावनी के साथ `1000:1000` पर
लौटती है) जब हर मीडिया mount root या आरक्षित uid `1001` का हो - अपने मीडिया को chown
करके किसी सामान्य उपयोगकर्ता को दें, या `PUID`/`PGID` साफ़ तौर पर सेट करें:

```env
PUID=1026   # run the server as this user id  (host: `id -u <media-owner>`)
PGID=100    # ...and this group id
```

साफ़ तौर पर दिए गए मान हमेशा पहचान पर भारी पड़ते हैं।

यह कैसे काम करता है: कंटेनर सिर्फ़ उतनी देर root के रूप में शुरू होता है जितनी उसके
भीतरी उपयोगकर्ता को फिर से map करने (और Evey की अपनी स्थिति वाली डायरेक्टरियों `/data`,
`/cache` का फिर से मालिक बनने - आपके मीडिया mounts का **कभी नहीं**) के लिए चाहिए, फिर
विशेषाधिकार छोड़ देता है। compose के `group_add:` से दिए गए अतिरिक्त groups (जैसे `/dev/dri`
हार्डवेयर ट्रांसकोडिंग के लिए आपके होस्ट का render group) विशेषाधिकार छोड़ने के बाद भी बने
रहते हैं। सर्वर प्रक्रिया ख़ुद कभी root के रूप में नहीं चलती।

बिल्कुल बिना root वाला विकल्प: अगर वह थोड़ी देर का root भी आपको मंज़ूर नहीं, तो इसके बजाय
अपनी compose फ़ाइल में सेवा पर `user: "UID:GID"` सेट करें। entrypoint बिना root वाली
शुरुआत पहचानकर फिर से map करना छोड़ देता है (मीडिया मालिक की पहचान समेत) - तब
`./evey-data` को उस UID के लिए लिखने लायक़ बनाए रखना आपकी ज़िम्मेदारी है।

टिप्पणियाँ:
- `PUID=1001` ठुकराया जाता है: यह भीतरी `evey-plugin` उपयोगकर्ता है, जिसे अलग रखा जाता है
  ताकि एक्सटेंशन कभी सर्वर की पहचान साझा न कर सकें।
- मीडिया डिफ़ॉल्ट रूप से पढ़ने-लिखने के लिए माउंट होता है ताकि मरम्मत सीधे काम करे; अगर
  आप सिर्फ़-पढ़ने वाली लाइब्रेरी चाहते हैं तो मीडिया volume में `:ro` जोड़ें (तब मरम्मत एक
  साफ़ त्रुटि के साथ विफल होगी)।

## 5. वेरिएबल संदर्भ

### सर्वर
```env
EVEY_SERVER_PORT=9090                  # gRPC listen port (default 9090); HTTP API binds at SERVER_PORT+1 = 9091
EVEY_HTTP_PORT=9091                    # compose-only: host port exposed by Docker for the HTTP gateway (not read by the server binary)
EVEY_SERVER_NAME=Evey                  # display name
EVEY_DEMO=false                        # public demo instance only (demo.eveyproject.org): locks admin writes, no mail/push/webhooks
EVEY_VERSION=latest                    # Docker image tag (latest, v0.1.0-beta.1, etc.)
```

### डेटाबेस
```env
# Embedded (default when EVEY_DB_HOST is unset): Postgres 17 inside the container.
EVEY_EMBEDDED_POSTGRES=            # true | false; unset = embedded iff EVEY_DB_HOST is unset
EVEY_POSTGRES_DATA_DIR=/data/postgres   # cluster location inside the container (under ./evey-data)
EVEY_DB_USER=evey
EVEY_DB_NAME=evey
EVEY_DB_PASSWORD=                  # unset = generated on first boot into /data/keys/postgres.password
EVEY_DB_PASSWORD_FILE=/data/keys/postgres.password   # read when EVEY_DB_PASSWORD is unset (image default)

# External: use these INSTEAD of the block above, not alongside it.
# Setting the host turns the embedded database off.
EVEY_DB_HOST=postgres              # the repository compose's sibling service is `postgres`
EVEY_DB_PORT=5432
EVEY_DB_PASSWORD=changeme
EVEY_DB_SSLMODE=prefer             # or disable | require | verify-full
```

Embedded Postgres सिर्फ़ कंटेनर के loopback पर सुनता है और TCP पर पासवर्ड माँगता है;
बनाया गया पासवर्ड वही है जिसे सर्वर (और `evey admin`) इस्तेमाल करते हैं। उसकी डेटा
डायरेक्टरी किसी असली फ़ाइल सिस्टम (ext4, btrfs, xfs, ...) पर होनी चाहिए, नेटवर्क शेयर पर
नहीं। दोनों बनावटों में migrations शुरुआत पर अपने आप चलते हैं।

### प्रमाणीकरण
```env
EVEY_JWT_ACCESS_TTL=15m
EVEY_JWT_REFRESH_TTL=168h              # 7 days
EVEY_RATELIMIT_LOGIN_MAX=10            # per IP per minute
```

### लाइब्रेरी
```env
EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL=2m # fallback polling interval when inotify misses events (default 2m); admin-editable via Admin → Server Config
EVEY_LIBRARY_WATCHER_ENABLED=true      # realtime file watching
```

लाइब्रेरी स्कैन की जाँच `.mkv`/`.webm` और `.mp4`/`.m4v` का मेटाडेटा प्रक्रिया के भीतर
पढ़ती है (mkvgo, सिर्फ़ शुरुआती हिस्सा - दोबारा स्कैन/मरम्मत/NAS पर ~35 गुना तेज़), बिना
किसी कॉन्फ़िगरेशन के: यह हमेशा चालू है। ffprobe ही अंतिम अधिकारी रहता है और हर उस फ़ाइल
को सँभालता है जिसे तेज़ रास्ता फ़ील्ड-दर-फ़ील्ड नहीं ढाल सकता (HDR/Dolby Vision, बिना
map वाले कोडेक, …)। देखें [adr/0016-mkv-fastpath-prober.md](adr/0016-mkv-fastpath-prober.md)।

### ट्रांसकोडिंग
```env
EVEY_TRANSCODING_MAX_CONCURRENT=2
EVEY_TRANSCODING_HW_ACCEL_ENABLED=false
```

हार्डवेयर सेटअप के लिए [transcoding.md](transcoding.md) देखें।

**Chromecast पर Dolby ऑडियो** (`EVEY_CAST_WASM_AUDIO_ENABLED`, डिफ़ॉल्ट `false`)।
Evey cast रिसीवर को मूल AC-3/E-AC-3 ट्रैक देकर उसे टीवी पर WebAssembly में decode करने
दे सकता है, जिससे ट्रांसकोड बचता है। यह **cast रिसीवरों के लिए बंद** है, क्योंकि इस रास्ते
के चलते एक संचालक का कम क्षमता वाला डिवाइस दो बार फ़िल्म के बीच में बंद हो गया - और वहाँ
उसका decoder असल समय का 3.13 गुना नापा गया, यानी उससे कोई फ़ायदा भी नहीं था। बंद होने पर
cast को सामान्य muxed master मिलता है जिसमें AAC ट्रैक होता है: हर डिवाइस पर पहले सेकंड से
आवाज़। ब्राउज़र पर असर **नहीं** पड़ता - वे wasm decoder रखते हैं, जो वहाँ जाँचा हुआ है।
cast के लिए फिर से चालू करने को इसे `true` करें।

**व्यवस्थापन पैनल को प्राथमिकता दें।** एक साथ कितने, हार्डवेयर त्वरण, AV1, cache बजट और
उन्नत सीमाएँ (एक साथ अधिकतम HLS सत्र, सॉफ़्टवेयर और सॉफ़्टवेयर-HDR की ऊँचाई सीमाएँ, एक
बार में एक पायदान एन्कोड करना, हर पायदान के लिए एक हार्डवेयर एन्कोडर) सब **Admin >
ट्रांसकोडिंग** में हैं। वहाँ सहेजे गए मान पुनरारंभ के बाद भी बने रहते हैं और environment
पर प्राथमिकता रखते हैं; पैनल बताता है कि कौन से सिर्फ़ पुनरारंभ के बाद लागू होते हैं।

**पृष्ठभूमि worker के flags** **Admin > सर्वर कॉन्फ़िग** में पंक्तियों के रूप में सेट होते हैं
(पैनल keys सुझाता है, और हर एक के लिए मेल खाता environment वेरिएबल और अपेक्षित मान
दिखाता है)। सब शुरुआत पर पढ़े जाते हैं, इसलिए पुनरारंभ पर लागू होते हैं:

| Key | मान |
| --- | --- |
| `transcoding.keyframe_backfill` | keyframe index backfill बंद करने के लिए `off` (डिफ़ॉल्ट चालू) |
| `transcoding.keyframe_backfill_batch` | हर दौर में आइटम, > 0 |
| `transcoding.keyframe_backfill_pause_ms` | आइटमों के बीच मिलीसेकंड, >= 0 |
| `transcoding.keyframe_backfill_idle_sec` | ख़ाली दौरों के बीच सेकंड, > 0 |
| `transcoding.level_backfill` | H.264/HEVC level backfill बंद करने के लिए `off` (डिफ़ॉल्ट चालू) |
| `transcoding.level_backfill_batch` | हर दौर में आइटम, > 0 |
| `transcoding.level_backfill_pause_ms` | आइटमों के बीच मिलीसेकंड, >= 0 |
| `transcoding.level_backfill_idle_sec` | ख़ाली दौरों के बीच सेकंड, > 0 |
| `transcoding.pause_bg_on_playback` | प्लेबैक के दौरान पृष्ठभूमि का काम चलता रहे, इसके लिए `off` (डिफ़ॉल्ट चालू: वह हट जाता है) |
| `transcoding.snapshot_fastpath` | mkvgo के keyframe तेज़ रास्ते के बजाय सटीक ffmpeg extractor ज़रूरी करने के लिए `off` |

हर एक का environment में भी एक समकक्ष है (`EVEY_KEYFRAME_BACKFILL`,
`EVEY_LEVEL_BACKFILL_BATCH`, ...), पर पैनल की पंक्ति जीतती है।

**पृष्ठभूमि CPU बजट** (Admin > सर्वर कॉन्फ़िग > आवंटित प्रोसेसिंग क्षमता) उस प्रोसेसर को
सीमित करता है जिसे पृष्ठभूमि के सारे काम मिलकर ले सकते हैं, लाइव (बिना पुनरारंभ):

| Key | मान |
| --- | --- |
| `background.cpu_percent` | CPU क्षमता का वह हिस्सा जिससे पृष्ठभूमि का हर काम लेता है: `25` (डिफ़ॉल्ट, हल्का) या `50` (बढ़ावा) |
| `background.cpu_percent.<task>` | किसी भारी काम का अपना हिस्सा, `1`..`80`, उस काम के लिए वैश्विक हिस्से की जगह लेता है: `library_scan`, `trickplay_generation`, `intro_detection`, `complexity_classifier` |

हिस्से कभी जुड़ते नहीं: पृष्ठभूमि में चल रहे हर काम का जोड़ सबसे ऊँचे तय हिस्से के नीचे
रहता है। हिस्से को पूरे cores में बदला जाता है (8 cores का 25 % = 2), कम से कम एक।
पृष्ठभूमि का हर ffmpeg शुरू होने से पहले इस बजट में अपने threads आरक्षित करता है; लाइब्रेरी
स्कैन और इंट्रो पहचान अपनी समानांतरता इसी से तय करते हैं, trickplay बनाना अपने decoder
threads। प्लेबैक फिर भी हर पृष्ठभूमि विश्लेषण को रोक देता है
(`transcoding.pause_bg_on_playback`)। `EVEY_LIBRARY_SCAN_WORKERS` और
`EVEY_INTRO_DETECTION_MAX_PARALLEL` उन दो pools को हाथ से तय करते हैं और उनके लिए बजट को
दरकिनार करते हैं। पुरानी keys `trickplay.adaptive_cpu` (`false` = 50 %) और
`intro_detection.use_all_cores` (`true` = इंट्रो पहचान के लिए 80 %) तब तक मानी जाती हैं जब
तक नई सेट न हों।

**ट्रांसकोड cache बजट** (Admin > सर्वर सेटिंग्स, `transcoding.cache_max_gb`, डिफ़ॉल्ट
`10`, पुनरारंभ पर लागू) डिस्क पर पूरे ट्रांसकोड cache को सीमित करता है: प्लेबैक segments,
तेज़-शुरुआत वाले remux और compat डाउनलोड मिलाकर, एक ही संख्या के तहत। यह जान-बूझकर एक
निरपेक्ष आकार है: cache `evey-cache` volume पर रहता है, उसके साथ जो कुछ भी संचालक वहाँ
चलाता है, और volume का बाक़ी हिस्सा किसलिए है यह उसका फ़ैसला है। एक ही sweeper इसे लागू
करता है, सबसे कम हाल में इस्तेमाल हुआ पहले, सिर्फ़ उसे बख़्शते हुए जिसे कोई दर्शक पढ़ रहा
है। बजट जो भी हो, वह cache volume पर 2 GiB ख़ाली भी रखता है, और ऐसा डाउनलोड हटा देता है
जिसे 7 दिन तक किसी ने नहीं उठाया। copy (remux) सत्र कभी पूरी फ़िल्म नहीं रखता: वह दर्शक के
आसपास segments की एक खिड़की रखता है और उसके बाहर की हर चीज़ के लिए स्रोत में फिर से जाता
है। बिना भंडारण वाला mkvgo रास्ता, जो ज़्यादातर MKV और MP4 स्रोत परोसता है, cache को छूता
ही नहीं।

### एक्सटेंशन (सैंडबॉक्स और सीमाएँ)
```env
EVEY_PLUGINS_SANDBOX_MODE=auto         # auto|landlock|bubblewrap|strict|basic - auto picks the strongest your host supports
EVEY_PLUGINS_UID_SEPARATION=auto       # auto|on|off - run plugins as a separate user to wall off secrets (auto = on only when the effective sandbox resolves to "basic", i.e. neither Landlock nor bubblewrap); incompatible with no-new-privileges
EVEY_PLUGINS_MEMORY_MB=256             # max RAM per plugin (MiB); admin-editable via Admin → Plugins
EVEY_PLUGINS_PIDS_MAX=64               # max processes/threads per plugin; admin-editable
EVEY_PLUGINS_DISK_MAX_MB=100           # max install-dir size per plugin (MiB; 0 = off); admin-editable
EVEY_PLUGINS_CPU_NICE=10               # scheduling nice 0-19 (higher = lower priority); admin-editable
EVEY_PLUGINS_ENABLED=true              # master switch for the plugin subsystem
EVEY_PLUGINS_COSIGN_POLICY=warn        # off|warn|enforce - require a cosign signature on plugin ZIPs
EVEY_PLUGINS_COSIGN_PUBLIC_KEY=        # PEM public-key path (required when policy != off)
```

हर स्थिति की गाइड: [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md)। पूरा मॉडल: [SECURITY.md](SECURITY.md)।

### ईमेल (SMTP)
```env
EVEY_SMTP_HOST=smtp.example.com
EVEY_SMTP_PORT=587
EVEY_SMTP_USERNAME=
EVEY_SMTP_PASSWORD=
EVEY_SMTP_FROM=noreply@example.com
EVEY_SMTP_TLS=starttls                 # starttls, ssl, none
```

देखें [notifications.md](notifications.md)।

### SSO (OIDC)
```env
EVEY_OIDC_ENABLED=false
EVEY_OIDC_ISSUER=https://auth.example.com
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=       # comma-separated, empty = all
```

देखें [oidc.md](oidc.md)।

### बैकअप
```env
EVEY_BACKUP_INTERVAL_HOURS=0           # 0 = disabled, 24 = daily
EVEY_BACKUP_RETENTION_DAYS=30
```

देखें [backups.md](backups.md)।

### रिवर्स प्रॉक्सी
```env
EVEY_TRUSTED_PROXIES=172.16.0.0/12     # added to the always-trusted loopback, not a replacement
EVEY_REAL_IP_HEADER=X-Forwarded-For
EVEY_DOMAIN=media.example.com          # only for bundled Caddy
```

HTTPS व्यवहार (Secure cookies, HSTS, `X-Forwarded-Proto` पर भरोसा)
`EVEY_SERVER_BASE_URL` के scheme के हिसाब से चलता है: `https://` वाला base URL इसे चालू
करता है। `EVEY_FORWARDED_PROTO=true|false` अप्रचलित है और सिर्फ़ override के रूप में रखा
गया है।

देखें [reverse-proxy.md](reverse-proxy.md)।

### इंट्रो पहचान
```env
EVEY_INTRO_TIDB_API_KEY=                # optional Bearer token for TheIntroDB (500 lookups/day vs 100 anonymous)
EVEY_INTRO_FFMPEG_THREADS=1             # threads per ffmpeg process used by intro detection (1-16)
EVEY_INTRO_SKIP_PREVIEW_ENABLED=false   # serve "preview" (next-episode reel) segments to the player so a "Skip Preview" button appears
EVEY_INTRO_AUTO_SKIP_ENABLED=false      # every player auto-skips detected intros (web/mobile read the flag in the segments response; cast receivers get intro segments gated by it)
```

`api.tidb_key`, `intro_providers.enabled`, `intro_providers.tidb_submit_enabled`,
`intro_providers.skip_preview_enabled` और `intro_providers.auto_skip_enabled` को
`/admin/server-config` से भी कॉन्फ़िगर किया जा सकता है (पहले DB, फिर environment विकल्प के
रूप में)। `intro_providers.enabled` बदलने के बाद सर्वर पुनरारंभ करें; पूर्वावलोकन छोड़ने और
इंट्रो अपने आप छोड़ने के बटन लाइव लागू होते हैं (बिना पुनरारंभ)।

देखें [adr/0011-intro-detection-providers.md](adr/0011-intro-detection-providers.md)।

### लॉगिंग
```env
EVEY_LOG_LEVEL=info                    # debug, info, warn, error
EVEY_LOG_DIR=/data/logs                # the image's default; the compose file maps it to ./evey-data/logs
EVEY_LOG_RETENTION_DAYS=90
```

`EVEY_LOG_DIR` के तहत रोज़ की फ़ाइल `evey-YYYY-MM-DD.log` ही आधिकारिक लॉग है।
`docker logs evey-server` वही धारा दिखाता है, पर Docker का json-file ड्राइवर उसे घुमाता है,
और घुमाव के बाद कमांड कंटेनर से घंटों पहले रुकी हुई लग सकती है। दोनों में मतभेद हो तो फ़ाइल
पर भरोसा करें - या root के रूप में कच्ची फ़ाइलों `/var/lib/docker/containers/<id>/*-json.log*`
पर। इमेज के बाहर (कंटेनर के बिना इंस्टॉल) डिफ़ॉल्ट काम की डायरेक्टरी के सापेक्ष एक `logs`
डायरेक्टरी है, इसलिए `EVEY_LOG_DIR` को किसी लिखने लायक़ absolute पाथ पर रखें।

### रिमोट एक्सेस (आपके नेटवर्क के बाहर से कौन जुड़ सकता है)
```env
EVEY_REMOTE_ACCESS_ENABLED=                                              # unset = auto (see below); true/false to decide yourself
EVEY_REMOTE_ACCESS_EXTERNAL_URL=                                         # the public address, if it differs from EVEY_SERVER_BASE_URL
EVEY_REMOTE_ACCESS_LAN_NETWORKS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16  # CIDRs treated as LAN
EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS=0                                    # 0 = no cap for remote (WAN) sessions
```

> ⚠️ यह चौकी **HTTP** listener (`EVEY_SERVER_PORT` + 1) की रक्षा करती है। ख़ुद
> `EVEY_SERVER_PORT` पर native gRPC listener एक आंतरिक परिवहन है और उस पर चौकी नहीं है,
> इसलिए वह **सिर्फ़ loopback** पर bind होता है (`EVEY_GRPC_BIND`, डिफ़ॉल्ट `127.0.0.1`):
> उसका इकलौता client उसी प्रक्रिया के भीतर का HTTP gateway है। उसे जान-बूझकर खोलने के लिए
> `EVEY_GRPC_BIND=0.0.0.0` सेट करें - तब शुरुआत चेतावनी देती है कि रिमोट-एक्सेस चौकी उसे
> नहीं ढकती, और उसे firewall से बचाना आपका काम हो जाता है।

**चौकी।** रिमोट एक्सेस बंद होने पर, जिस client का IP `EVEY_REMOTE_ACCESS_LAN_NETWORKS` में
नहीं है उसे हर रूट पर `403 {"code":"remote_access_disabled"}` के साथ ठुकराया जाता है। यह
तैनाती का चुनाव है ("यह इंस्टेंस मेरे अपने नेटवर्क के लिए है"), सुरक्षा की परत नहीं - आपकी
लाइब्रेरी की रक्षा प्रमाणीकरण करता है।

**डिफ़ॉल्ट रूप से स्वचालित।** `EVEY_REMOTE_ACCESS_ENABLED` सेट न करें तो सर्वर शुरुआत पर
तय करता है: अगर `EVEY_REMOTE_ACCESS_EXTERNAL_URL` या `EVEY_SERVER_BASE_URL` सार्वजनिक रूप
से पहुँचने लायक़ पता है, तो रिमोट एक्सेस **चालू** होता है (जो सर्वर आपने प्रकाशित किया,
वह पहुँचने के लिए ही है); वरना वह LAN तक सीमित रहता है। फ़ैसला शुरुआत पर लॉग होता है, और
`evey admin doctor` उसे बताता है।

साफ़ तौर पर तय करने के लिए वेरिएबल सेट करें - या **Admin → रिमोट एक्सेस** इस्तेमाल करें;
साफ़ चुनाव को अनुमान कभी नहीं बदलता। अगर आप किसी प्रकाशित सर्वर को जान-बूझकर LAN तक
सीमित रखते हैं, तो शुरुआती लॉग में इसकी चेतावनी होती है, क्योंकि वरना आप तक पहुँचने वाला
लक्षण होता है "तुम्हारे फ़ोन पर चलता है, मेरे पर नहीं"।

> रिवर्स प्रॉक्सी के पीछे यह तभी काम करता है जब प्रॉक्सी असली client IP आगे भेजे और
> `EVEY_TRUSTED_PROXIES` प्रॉक्सी को ढके। वरना हर client प्रॉक्सी के अपने पते जैसा दिखता
> है - देखें [reverse-proxy.md](reverse-proxy.md)।

जब `EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS` सेट हो, तो जिन प्लेबैक सत्रों का client IP WAN
वर्गीकृत होता है, उन्हें उस बिटरेट तक सीमित किया जाता है; यह सीमा client/प्रोफ़ाइल की
सीमा को हमेशा सिर्फ़ नीचे ही लाती है। दो सुरक्षा नियम जानने लायक़:

- RFC1918/निजी client IP पर **कभी** सीमा नहीं लगती, भले आप उसकी श्रेणी
  `EVEY_REMOTE_ACCESS_LAN_NETWORKS` में लिखना भूल गए हों - सीमा सिर्फ़ पक्के तौर पर
  सार्वजनिक पते पर लगती है।
- अगर आप असामान्य स्थानीय श्रेणियाँ इस्तेमाल करते हैं (CGNAT `100.64.0.0/10`, कोई
  VPN/Tailscale subnet) और उन दर्शकों पर सीमा **नहीं** चाहते, तो उन्हें
  `EVEY_REMOTE_ACCESS_LAN_NETWORKS` में साफ़ तौर पर लिखें। रिवर्स प्रॉक्सी के पीछे पक्का
  करें कि वह असली client IP (`X-Forwarded-For`) आगे भेजता है, वरना हर सत्र प्रॉक्सी के पते
  जैसा दिखेगा।
