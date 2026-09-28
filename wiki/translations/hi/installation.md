---
source_sha: ec58362c8a7e791e617d5615c213693bfbd23e8c8494c6d1e5b6bce5ce563cb2
---
# इंस्टॉलेशन

Evey **एक ही कंटेनर** के रूप में चलता है: इमेज API और वेब UI एक ही पोर्ट पर परोसती है
और Postgres व Meilisearch को अपने भीतर चलाती है, बिना root वाले सर्वर उपयोगकर्ता के रूप
में, पहली शुरुआत पर बनाए गए राज़ों के साथ। खंड 1 की compose फ़ाइल ही पूरा इंस्टॉलेशन है।
इसे न `.env` चाहिए, न `--privileged`, न मेमोरी या CPU की कोई सेटिंग।

!!! warning "यह रिपॉज़िटरी की docker-compose.yml नहीं है"

    Git रिपॉज़िटरी के मूल की `docker-compose.yml` वह फ़ाइल **नहीं** है। वह संचालक वाली
    बनावट है (खंड 7): अलग सेवा के रूप में Postgres, साथ आने वाला Caddy, और एक `.env`
    जिसके बिना वह शुरू ही नहीं होती। नीचे वाला ब्लॉक कॉपी करें, रिपॉज़िटरी की फ़ाइल नहीं।

Docker के बिना Evey चलाने के लिए देखें [deployment-bare-metal.md](deployment-bare-metal.md)।

## पूर्व-शर्तें

- Docker + Docker Compose v2 (`docker compose`, पुराना `docker-compose` नहीं)।
- किसी स्थायी डिस्क पर डेटा रखने की जगह (नीचे की compose काम की डायरेक्टरी में bind
  mounts इस्तेमाल करती है)।
- **अनुशंसित:** HTTPS के लिए एक रिवर्स प्रॉक्सी - Evey सिर्फ़ सादा HTTP परोसता है। देखें
  [reverse-proxy.md](reverse-proxy.md)।

## 1. compose फ़ाइल

इसे `docker-compose.yml` के रूप में सहेजें, आख़िरी volume पंक्ति में अपने मीडिया के
पाथ के साथ। राज़ पहली शुरुआत पर `./evey-data/keys` में आ जाते हैं; गढ़ने को कुछ नहीं।

```yaml title="docker-compose.yml"
services:
  evey:
    image: gravityz/evey:latest # (1)!
    container_name: evey-server
    ports:
      - "9091:9091" # (2)!
    # environment:
    #   EVEY_SERVER_BASE_URL: http://<your-ip>:9091
    volumes:
      - ./evey-data:/data # (3)!
      - evey-cache:/cache # (4)!
      - /path/to/your/media:/media # (5)!
    stop_grace_period: 60s # (6)!
    restart: unless-stopped

volumes:
  evey-cache:
```

1. इमेज। यहाँ एक टैग पिन करें (`0.2.0-beta.1`, या न बदलने वाला `sha-1a2b3c4`) ताकि एक दिन के अंतर पर किए गए दो pull एक ही कोड चलाएँ - देखें [Evey अपडेट करना](#4-updating-evey)।
2. API और वेब इंटरफ़ेस, एक ही पोर्ट पर, सादे HTTP में। HTTPS के लिए आगे एक [रिवर्स प्रॉक्सी](reverse-proxy.md) रखें और ठीक नीचे `EVEY_SERVER_BASE_URL` को सार्वजनिक पते के साथ uncomment करें: ई-मेल के लिंक, OIDC redirects और cast URLs उसी से बनते हैं।
3. Evey का सब कुछ: डेटाबेस, JWT कुंजी, बनाए गए राज़, खोज सूचकांक, लॉग, बैकअप, एक्सटेंशन। बैकअप इसी फ़ोल्डर का लेना है।
4. ट्रांसकोड और trickplay cache। मिटाना सुरक्षित है: ज़रूरत पर फिर भर जाता है।
5. आपकी लाइब्रेरी, colon के बाईं ओर। जान-बूझकर पढ़ने-लिखने के लिए: seek-index मरम्मत फ़ाइलों को उसी जगह फिर से बनाती है। सिर्फ़ पढ़ने वाली लाइब्रेरी के लिए `:ro` जोड़ें और मान लें कि वे मरम्मतें विफल होंगी। और फ़ोल्डर `/media2`, `/media3`... के रूप में जुड़ते हैं।
6. `docker stop` पर embedded डेटाबेस सर्वर के बाद व्यवस्थित रूप से बंद होता है; बड़े catalogue को Docker के डिफ़ॉल्ट 10 s से ज़्यादा चाहिए।

!!! tip "Environment न्यूनतम रखें"

    compose फ़ाइल (या राज़ों के लिए `.env`) में सिर्फ़ ये आते हैं: आपके अपने लाए राज़,
    बाहरी होने पर डेटाबेस कनेक्शन, स्टोरेज पाथ, और रिवर्स प्रॉक्सी की बातें। लगभग बाक़ी
    सब - सर्वर का नाम, एक साथ ट्रांसकोड, दर-सीमाएँ, लॉग स्तर, स्कैन और निगरानी
    अंतराल, बैकअप शेड्यूल, API कुंजियाँ - **Admin → सर्वर कॉन्फ़िग से चलते-चलते बदला
    जा सकता है** और compose से बाहर रहता है।

!!! info "फ़ाइलें किसकी हैं: PUID और PGID"

    डिफ़ॉल्ट रूप से सेट नहीं: entrypoint आपके `/media` mounts के मालिक को अपनाता है,
    ताकि उसी जगह होने वाली मरम्मत लिख सके। ख़ुद चुनने के लिए `environment:` के तहत
    `PUID` और `PGID` सेट करें (आपके `id -u` और `id -g`)। कंटेनर सिर्फ़ यह mapping
    लागू करने के लिए root के रूप में शुरू होता है, फिर विशेषाधिकार छोड़ देता है - सर्वर
    ख़ुद कभी root के रूप में नहीं चलता।

???+ note "NAS पर (Synology Container Manager, QNAP Container Station)"

    - डेटा फ़ोल्डर पहली शुरुआत **से पहले** बनाएँ (compose के बगल में `mkdir evey-data`, या
      File Station से): Docker CLI के उलट, ये इंटरफ़ेस ऐसा bind mount नहीं मानते जिसका फ़ोल्डर
      मौजूद न हो। पहले से और कुछ नहीं बनाना - entrypoint `plugins/`, `keys/`, `postgres/`
      ख़ुद बनाता है।
    - `PUID`/`PGID` को उस DSM उपयोगकर्ता पर सेट करें जिसका मीडिया है (SSH पर `id <user>`;
      Synology पर आमतौर पर `1026` के आसपास और group `100` = `users`), स्वचालित पहचान पर
      भरोसा करने के बजाय: DSM share अक्सर `root` का होता है, जिसे entrypoint कभी नहीं अपनाता।
    - Quick Sync वाला Intel NAS: `docker-compose.gpu.yml` की `devices:` और `group_add:`
      पंक्तियाँ जोड़ें (render group का GID `getent group render` या
      `stat -c %g /dev/dri/renderD128` से मिलता है)।
    - कोई `.env` नहीं चाहिए - रिपॉज़िटरी के मूल की `.env` माँगने वाली `docker-compose.yml`
      संचालक वाली बनावट है, यह नहीं।

## 2. पहली शुरुआत

```bash
docker compose up -d
docker compose logs -f evey
```

पहली शुरुआत पर सर्वर:

1. **JWT हस्ताक्षर कुंजी अपने आप बनाता है**, `EVEY_JWT_PRIVATE_KEY_PATH`
   (`/data/jwt.key`, `./evey-data` के भीतर) पर। आप इसे हाथ से नहीं बनाते। `evey-data`
   को सुरक्षित रखें - कुंजी खोने पर सब लॉग आउट हो जाते हैं।
2. **डेटाबेस migrations अपने आप चलाता है** (भीतर शामिल, शुरुआत पर लागू; migration का
   कोई अलग चरण नहीं)। देखें [runbooks/migration-runbook.md](runbooks/migration-runbook.md)।

फिर `http://<आपका-ip>:9091` खोलें। जब तक कोई व्यवस्थापक नहीं है, आप **सेटअप विज़ार्ड**
(`/setup`) पर पहुँचते हैं: पहला व्यवस्थापक खाता बनाएँ। उसके बाद साइन इन करें और `/media`
के तहत किसी पाथ (वहाँ माउंट किया गया आपका फ़ोल्डर) की ओर इशारा करती **लाइब्रेरी
जोड़ें** (Admin → लाइब्रेरी)। पहला स्कैन अपने आप शुरू होता है। विज़ार्ड, लाइब्रेरी, लोगों को
आमंत्रित करना और पता साझा करने से पहले क्या जाँचें - यह सब [पहले कदम](guide/first-steps.md)
में है।

## 3. आम बदलाव

**बदलाव, हर एक उस फ़ाइल में कुछ पंक्तियाँ:**

- *दूसरा मीडिया फ़ोल्डर*: `volumes:` के तहत `- /other/path:/media2` जोड़ें (`/media3`,
  ... इसी तरह)। इन्हें भी पहले वाले की तरह पढ़ने-लिखने के लिए माउंट करें।
- *किसी ऐसे रिवर्स प्रॉक्सी के पीछे जो आप पहले से चलाते हैं* (Nginx Proxy Manager,
  Traefik, ... किसी Docker नेटवर्क पर): `ports:` हटाएँ, प्रॉक्सी के नेटवर्क से जुड़ें और
  सर्वर को उसका सार्वजनिक URL और किस पड़ोसी पर भरोसा करना है, बताएँ:
  ```yaml
  services:
    evey:
      environment:
        EVEY_SERVER_BASE_URL: https://media.example.com
        EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # narrow to the proxy's subnet once known
  networks:
    default:
      name: proxy-net        # the network your proxy is on
      external: true
  ```
  तब प्रॉक्सी `evey-server:9091` को निशाना बनाता है। हर प्रॉक्सी की सेटिंग्स:
  [reverse-proxy.md](reverse-proxy.md)।
- *अभी कोई प्रॉक्सी नहीं, पर HTTPS चाहिए*: [reverse-proxy.md](reverse-proxy.md) से साथ
  आने वाली Caddy सेवा जोड़ें (Let's Encrypt प्रमाणपत्र अपने आप)।
- *Postgres या Meilisearch अपनी अलग सेवा के रूप में*: खंड 7।

**HTTPS और रिवर्स प्रॉक्सी।** Evey 9091 पर सादा HTTP परोसता है। अभी कोई प्रॉक्सी न हो, तो
[reverse-proxy.md](reverse-proxy.md) की साथ आने वाली Caddy सेवा आपके लिए Let's Encrypt
प्रमाणपत्र ले आती है (पोर्ट 80/443 होस्ट तक पहुँचने चाहिए)। अपने प्रॉक्सी (Nginx,
Traefik, Nginx Proxy Manager) के साथ: `EVEY_SERVER_BASE_URL` को अपने सार्वजनिक
`https://` URL पर सेट करें (यही scheme Secure cookies और HSTS चालू करता है) और
`EVEY_TRUSTED_PROXIES` से प्रॉक्सी पर भरोसा करें। स्ट्रीमिंग endpoints को buffering बंद
और कोई timeout नहीं चाहिए - तैयार कॉन्फ़िगरेशन यह सँभालते हैं। पूरे सेटअप:
[reverse-proxy.md](reverse-proxy.md)।

## 4. Evey अपडेट करना

```bash
docker compose pull && docker compose up -d
docker compose logs -f evey      # until "healthy" (a migration on a large catalogue takes minutes)
```

और कुछ नहीं चाहिए। नई इमेज शुरू होते समय बाक़ी migrations लागू करती है और उससे पहले
डेटाबेस का dump लेती है (`./evey-data/backups/pre-migration-*`), कंटेनर की health check
परोसना शुरू होते ही हरी हो जाती है, और `up -d` सिर्फ़ `evey` सेवा को फिर से शुरू करता है -
पड़ोस का डेटाबेस या प्रॉक्सी कभी नहीं छुआ जाता।

**वापस जाना।** पिछला टैग `image:` पंक्ति पर रखें (या `EVEY_VERSION` में, अगर आपकी compose
उसे भरती है) और `docker compose up -d` फिर से चलाएँ। अगर नए संस्करण ने schema migrate
किया था, तो पहले उसका migration से पहले वाला dump वापस लाएँ -
`docker exec evey-server evey admin rollback` dump के पाथ के साथ ठीक-ठीक चरण छापता है।

**जाँचना कि क्या चल रहा है।** `docker exec evey-server evey version` चल रही बाइनरी का
संस्करण और commit छापता है (`/healthz` भी यही बताता है), इसलिए "यह कौन सा कोड है?" कभी
किसी टैग पर निर्भर नहीं करता।

> रिपॉज़िटरी के checkout से काम करने वाले संचालकों के पास यही प्रक्रिया script के रूप में
> है: `scripts/evey-update.sh` (डिफ़ॉल्ट रूप से dry-run; स्थिति, अपडेट से पहले का dump,
> pull, health का इंतज़ार, commit जाँच, rollback पंक्ति) और `scripts/evey-rollback.sh`।
> ये रिपॉज़िटरी की अपनी तैनातियों की सुविधाएँ हैं, किसी इंस्टॉल की ज़रूरत नहीं।

**जो चलाते हैं उसे पिन करें।** `image:` पंक्ति पर एक रिलीज़ टैग (`0.2.0-beta.1`) या एक
न बदलने वाला `sha-<short>` टैग रखें; `beta-0.0.1` जैसा branch टैग हर push के साथ खिसकता
है, इसलिए एक दिन के अंतर पर किए गए दो pull अलग कोड चला सकते हैं और "`beta-0.0.1` पर वापस
जाओ" कहीं नहीं ले जाता। script वह टैग छापती है जिसे उसने शुरू किया, ताकि आप उसे पिन कर सकें।

**इमेज टैग** (`gravityz/evey`):

- रिलीज़ टैग `vX.Y.Z` `X.Y.Z`, `X.Y`, `X` और `latest` प्रकाशित करते हैं। `latest` का
  मतलब हमेशा सबसे नई semver **रिलीज़** होता है।
- branch builds branch का नाम (जैसे `beta-0.0.1`) और `sha-<short>` प्रकाशित करते हैं।
- **रनटाइम रूप** - वही ऐप, साथ में अलग एक्सटेंशन रनटाइम:
  - बिना suffix (डिफ़ॉल्ट / "lean") - सिर्फ़ Python 3।
  - `-nodejs` - JS एक्सटेंशन के लिए Node जोड़ता है।
  - `-full` - साथ में PHP + Ruby + Lua भी।
  `-nodejs` / `-full` तभी चुनें जब आपके एक्सटेंशन को वे रनटाइम चाहिए; वरना डिफ़ॉल्ट सबसे
  छोटा है। जैसे `EVEY_VERSION=latest-nodejs` सेट करें।
- **GPU रूप** - प्रकाशित टैग हर त्वरक परिवार के साथ आते हैं (Intel Gen9 से आज तक, AMD),
  जिसमें अकेले Intel के OpenCL compilers लगभग 900 MB हैं। ख़ुद बनाई गई इमेज सिर्फ़ अपने होस्ट
  का परिवार रख सकती है: `docker build --build-arg GPU=intel-legacy .` (Intel Gen9 से
  Gen11, UHD 6xx), `GPU=intel` (Gen12 और नए), `GPU=amd`, या `GPU=none`। इमेज label
  `org.evey.gpu` चुनाव दर्ज करता है। NVIDIA को किसी रूप की ज़रूरत नहीं: किसी इमेज में
  NVIDIA लाइब्रेरी पकी हुई नहीं है, NVIDIA Container Toolkit रनटाइम पर होस्ट का ड्राइवर
  डालता है (`--gpus all` या compose में device reservation), इसलिए NVENC हर रूप के साथ
  काम करता है, `none` समेत। जिस मशीन में Intel iGPU और NVIDIA कार्ड दोनों हों, वहाँ
  calibration का `auto` मोड पहले VAAPI चुनता है; कार्ड इस्तेमाल करने के लिए व्यवस्थापन के
  ट्रांसकोडिंग पैनल में हार्डवेयर मोड `nvenc` पर पिन करें।

## 5. बैकअप और पुनर्प्राप्ति

निर्धारित DB बैकअप चालू करें (`EVEY_BACKUP_INTERVAL_HOURS`, `EVEY_BACKUP_DIR` →
`/data/backups` में, `evey-data` के भीतर रखे जाते हैं) या व्यवस्थापन पैनल से कोई बैकअप
शुरू करें। बैकअप सारा उपयोगकर्ता डेटा ढकते हैं, आपकी मीडिया फ़ाइलें नहीं। ब्योरा:
[backups.md](backups.md)।

बाहर बंद हो जाने / व्यवस्थापक खो जाने / पुनर्स्थापना की स्थितियों के लिए इमेज में एक
`evey admin` CLI है (`reset-password`, `rotate-jwt-key`, `restore-backup`):
[runbooks/recovery.md](runbooks/recovery.md)। migration की सुरक्षा:
[runbooks/migration-runbook.md](runbooks/migration-runbook.md)।

## 6. समस्याएँ सुलझाना

कुछ ग़लत होने पर पहली जाँचें:

```bash
docker compose ps                       # is every service Up / healthy?
docker compose logs --tail=200 evey     # server logs (migration errors, config validation)
```

- **evey शुरू नहीं होता / कॉन्फ़िगरेशन त्रुटि।** बाहरी डेटाबेस के साथ, अगर
  `EVEY_DB_PASSWORD` अब भी डिफ़ॉल्ट `evey` है तो सर्वर शुरू होने से मना करता है; बाहरी
  Meilisearch के साथ, अगर `MEILI_MASTER_KEY` ख़ाली है या placeholder है (छोटी, ग़ैर-डिफ़ॉल्ट
  कुंजी पर सिर्फ़ चेतावनी)। लॉग हुआ जाँच संदेश पढ़ें और `.env` ठीक करें।
- **Embedded DB शुरू होने से मना करता है।** लॉग के ऊपर की `evey:` पंक्तियाँ वजह बताती हैं:
  किसी दूसरे major का cluster (ऊपर का upgrade खंड देखें), ऐसी डेटा डायरेक्टरी जो Postgres
  cluster नहीं है, या ख़ुद Postgres की `postgres:` उपसर्ग वाली त्रुटि (डिस्क भरी,
  `./evey-data/postgres` पर अनुमतियाँ, नेटवर्क-शेयर वाला फ़ाइल सिस्टम)।
- **लॉग में `/cache/...` पर `permission denied` के साथ restart का चक्कर।** `evey-cache`
  volume उस uid से अलग uid का है जिस पर सर्वर चलता है (`PUID`, या अपनाया गया मीडिया
  मालिक)। 2026-09-17 से पहले की इमेज इसका मालिक तभी बदलती थीं जब `./evey-data` का मालिक
  भी बदलता, इसलिए `PUID` के रूप में पहले से बनाया गया डेटा फ़ोल्डर (ऊपर वाली NAS स्थिति)
  cache को पीछे छोड़ देता था। इमेज अपडेट करें; पुरानी इमेज पर एक बार
  `docker run --rm -v evey_evey-cache:/c alpine chown -R <PUID>:<PGID> /c` चलाएँ (volume
  का नाम `<project>_evey-cache` है)।
- **बाहरी DB तक नहीं पहुँचा जा रहा।** पक्का करें कि Postgres स्वस्थ है और पासवर्ड मेल खाता है:
  ```bash
  docker exec evey-postgres pg_isready -U "$EVEY_DB_USER"
  ```
- **खोज टूटी / ख़ाली।** Meilisearch की सेहत जाँचें, फिर reindex चलाएँ - व्यवस्थापन की खोज
  सेटिंग्स से, या `docker exec evey-server evey admin reindex-search` से। सूचकांक Postgres
  से फिर बनता है, इसलिए `meili-data` मिटाना सुरक्षित है।
- **Health endpoint:** `curl -sf http://localhost:9091/healthz && echo OK`।

### कौन से volumes मिटाना सुरक्षित है

| Volume | सामग्री | मिटाना सुरक्षित? |
|--------|----------|-----------------|
| `evey-data` | डेटाबेस (`postgres/`), JWT कुंजी + बनाए गए राज़ (`keys/`), लॉग, बैकअप, एक्सटेंशन, खोज सूचकांक (`meili/`) | नहीं - डेटा, JWT कुंजी + एक्सटेंशन खो जाते हैं। सिर्फ़ `meili/`: हाँ, `evey admin reindex-search` से फिर बनता है |
| `postgres-data` | डेटाबेस (सिर्फ़ बाहरी बनावट) | नहीं - सारा डेटा खो जाता है |
| `evey-cache` | ट्रांसकोड + trickplay cache | हाँ - ज़रूरत पर फिर बनता है |

## 7. संचालक वाली बनावट: बाहरी Postgres और Meilisearch

**Embedded डेटाबेस कैसे व्यवहार करता है।** cluster `./evey-data/postgres` में रहता है
(Postgres 17, पहली शुरुआत पर `initdb`, सर्वर उपयोगकर्ता का)। यह सिर्फ़ कंटेनर के loopback
पर सुनता है और TCP पर बनाया गया पासवर्ड माँगता है, इसलिए किसी दूसरे uid पर उतारा गया
एक्सटेंशन इसे नहीं खोल सकता। `docker stop` इस तक तब पहुँचता है जब सर्वर अपने कनेक्शन
बंद कर चुका हो, एक व्यवस्थित तेज़ shutdown के रूप में। डेटा डायरेक्टरी किसी असली फ़ाइल
सिस्टम (ext4, btrfs, xfs, ...) पर होनी चाहिए - CIFS/NFS share पर नहीं। बैकअप
(`evey admin`, व्यवस्थापन पैनल, निर्धारित `pg_dump`) ठीक वैसे ही काम करते हैं जैसे बाहरी
डेटाबेस के साथ। SQL shell खोलने के लिए:

```bash
docker exec -it evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

### बाहरी Postgres

जो संचालक डेटाबेस का जीवनचक्र अपने हाथ में चाहते हैं (एक managed Postgres, पिन किया
गया major संस्करण, अलग volume, अपनी शर्तों पर dump+restore), वे इसे पड़ोसी सेवा के रूप
में चलाते हैं: सेवा जोड़ें और सर्वर को उसकी ओर मोड़ें - **`EVEY_DB_HOST` का सेट होना ही
embedded डेटाबेस बंद करता है**, इसीलिए embedded डेटाबेस से पहले का हर इंस्टॉल बिना छुए
चलता रहता है:

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... as above, plus:
        environment:
          EVEY_DB_HOST: postgres
          EVEY_DB_SSLMODE: disable          # internal network, no TLS
        depends_on:
          postgres: { condition: service_healthy }

      postgres:
        image: postgres:16-alpine
        container_name: evey-postgres
        environment:
          POSTGRES_DB: ${EVEY_DB_NAME:-evey}
          POSTGRES_USER: ${EVEY_DB_USER:-evey}
          POSTGRES_PASSWORD: ${EVEY_DB_PASSWORD:?EVEY_DB_PASSWORD must be set in .env}
        volumes:
          - ./postgres-data:/var/lib/postgresql/data
        healthcheck:
          test: ["CMD-SHELL", "pg_isready -U ${EVEY_DB_USER:-evey}"]
          interval: 5s
          timeout: 3s
          retries: 5
        restart: unless-stopped
    ```

=== ".env"

    ```env
    EVEY_DB_PASSWORD=<strong-password>   # must NOT be the default 'evey' - the server refuses to start otherwise
    ```

**बाहरी Meilisearch, वही विचार।** अलग खोज कंटेनर अपना जीवनचक्र और volume रखता है (और
आपको सिर्फ़ सूचकांक रीसेट करने के लिए `docker compose restart meilisearch` करने देता है)।
Embedded वाला बंद करें, सर्वर को इसकी ओर मोड़ें, और कुंजी साझा करें - स्पष्ट
`MEILI_MASTER_KEY` हमेशा बनाई गई `./evey-data/keys/meili.key` पर भारी पड़ता है:

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... plus:
        environment:
          EVEY_EMBEDDED_MEILI: "false"
          EVEY_MEILI_URL: http://meilisearch:7700
        depends_on:
          meilisearch: { condition: service_healthy }

      meilisearch:
        image: getmeili/meilisearch:v1.12
        container_name: evey-meilisearch
        environment:
          MEILI_MASTER_KEY: ${MEILI_MASTER_KEY:?MEILI_MASTER_KEY must be set in .env}
          MEILI_NO_ANALYTICS: "true"
        volumes:
          - ./meili-data:/meili_data
        healthcheck:
          test: ["CMD", "curl", "-sf", "http://localhost:7700/health"]
          interval: 5s
          retries: 5
        restart: unless-stopped
    ```

=== ".env"

    ```env
    MEILI_MASTER_KEY=<run: openssl rand -hex 16>
    ```

हो सके तो उस कंटेनर को बिना root वाला `user:` और एक `mem_limit` दें - आधिकारिक इमेज root
के रूप में चलती है और उसकी resident मेमोरी (memory-mapped सूचकांक) बड़ी दिखती है पर
ज़्यादातर वापस लिया जा सकने वाला page cache है। सूचकांक Postgres से फिर बनता है
(`evey admin reindex-search`), इसलिए `meili-data` हर हाल में छोड़ा जा सकता है।

सिर्फ़ ये env vars infra/राज़ हैं और **compose/.env में ही रहते हैं** - ये कभी व्यवस्थापन से
बदले नहीं जा सकते: `EVEY_DB_PASSWORD`, `MEILI_MASTER_KEY`, `EVEY_DB_HOST`,
`EVEY_DB_USER`, `EVEY_DB_NAME`, `EVEY_DB_SSLMODE`, `EVEY_EMBEDDED_POSTGRES`,
`EVEY_EMBEDDED_MEILI`, `EVEY_MEILI_URL`, `*_DIR` पाथ, `EVEY_JWT_PRIVATE_KEY_PATH`,
`EVEY_SERVER_BASE_URL`, `EVEY_TRUSTED_PROXIES`, और एक्सटेंशन का मुख्य स्विच
`EVEY_PLUGINS_ENABLED`। (`EVEY_PLUGINS_SANDBOX_MODE`, `EVEY_PLUGINS_UID_SEPARATION` और
संसाधन सीमाएँ यहाँ शुरुआती डिफ़ॉल्ट के रूप में सेट की जा सकती हैं, पर रनटाइम पर Admin →
Plugins से बदली जा सकती हैं।) पूरा संदर्भ: [configuration.md](configuration.md)।

### किसी मौजूदा इंस्टॉल को embedded डेटाबेस पर ले जाना

जो इंस्टॉल Postgres को पड़ोसी सेवा के रूप में चलाता है (embedded डेटाबेस से पहले की हर
रिलीज़ की बनावट), वह एक ही कंटेनर पर जा सकता है: डेटा बस एक dump की दूरी पर है,
`./evey-data` की बाक़ी हर चीज़ (JWT कुंजी, एक्सटेंशन, बैकअप, trickplay) जस की तस उठा ली
जाती है। embedded डेटाबेस `./evey-data/postgres-import.sql.gz` पर मिला dump **एक बार,
उस cluster में जिसे वह अपनी पहली शुरुआत पर बनाता है**, सर्वर के चलने से पहले import करता
है - ताकि उसके ऊपर सर्वर की अपनी migrations लागू हों, जैसे किसी भी अपडेट के बाद।

```bash
# 1. dump the current database (plain SQL, gzipped - what the import expects)
docker compose exec -T postgres pg_dump -U evey --no-owner --no-privileges evey \
  | gzip > evey-data/postgres-import.sql.gz
docker compose down

# 2. write the new compose (keep a copy of the old one): the single-service
#    file from section 1, with your variations (extra media folders, the
#    proxy block) and whatever you had in `environment:` (hardware
#    transcoding: `devices: /dev/dri`, `group_add`, LIBVA_DRIVER_NAME).

# 3. first boot: initdb, import, migrations, then the server
docker compose up -d && docker compose logs -f evey     # "evey: import done"

# 4. search index: an external Meilisearch's index is not carried over, rebuild it
docker exec evey-server evey admin reindex-search
```

जिसकी अब ज़रूरत **नहीं**: `EVEY_DB_PASSWORD`, `MEILI_MASTER_KEY` और `EVEY_DB_HOST` /
`EVEY_MEILI_URL` की wiring (राज़ बनाए जाते हैं), `postgres` और `meilisearch` सेवाएँ, और
`.env` से बीज डाली गई API कुंजियाँ (वे पहले से dump की गई `server_config` में हैं)। सर्वर
जाँचने तक `./postgres-data` और `./meili-data` रखें; वापस जाना पड़े तो पुरानी compose
उन्हें अब भी चलाती है। पहले से तैयार cluster के बगल में छोड़ा गया dump बताया और अनदेखा
किया जाता है, कभी मिलाया नहीं जाता; विफल import नया cluster छोड़ देता है ताकि अगली शुरुआत
dump से फिर कोशिश करे।

### Postgres का major upgrade (embedded डेटाबेस)

इमेज embedded Postgres का major पिन करती है (आज 17)। नए major वाली नई इमेज पुराने
cluster को **खोलने से मना करती है**, इस खंड का नाम लेने वाले संदेश के साथ - वह ऐसी डेटा
डायरेक्टरी को चुपचाप कभी नहीं चलाती जिसे वह पढ़ न सके। पार जाने का रास्ता है पुरानी इमेज
से dump और नई से restore:

```bash
docker compose exec evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_dump -h 127.0.0.1 -U evey -Fc evey' > evey-before-upgrade.dump
docker compose down
mv evey-data/postgres evey-data/postgres.old      # keep it until the restore is verified
# set the new EVEY_VERSION in .env, then:
docker compose up -d                              # initialises an empty cluster on the new major
docker compose exec -T evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_restore -h 127.0.0.1 -U evey -d evey --no-owner' < evey-before-upgrade.dump
docker compose restart evey
```

जो संचालक `pg_upgrade` या बरसों तक पिन किया गया major पसंद करते हैं, वे
[बाहरी Postgres वाली बनावट](#7-operator-layout-external-postgres-and-meilisearch)
इस्तेमाल करते हैं।

## 8. ARM होस्ट (x86-64 और ARM64)

स्थिर रिलीज़ (`vX.Y.Z` टैग) `linux/amd64` और `linux/arm64` दोनों के लिए एक **multi-arch**
इमेज प्रकाशित करती हैं; `docker pull`/`compose up` सही वाली अपने आप चुनता है, किसी flag की
ज़रूरत नहीं। प्री-रिलीज़ टैग (`-alpha`/`-beta`) और हर branch के builds **सिर्फ़ amd64** हैं -
ARM होस्ट (Raspberry Pi, ARM NAS, Ampere VPS) के लिए `EVEY_VERSION` में कोई स्थिर
`vX.Y.Z` पिन करें।

ARM पर दो बातें जानने लायक़:

- **ARM पर हार्डवेयर ट्रांसकोडिंग: सिर्फ़ Rockchip, और सिर्फ़ वही जो calibration साबित
  करे।** Evey चार त्वरक backends जोड़ता है - Intel QSV, VAAPI, NVENC, और Rockchip
  **RKMPP** (RK3588 / RK3576 / RK3568 श्रेणी के SoC, arm64 jellyfin-ffmpeg के
  `h264_rkmpp` / `hevc_rkmpp` encoders, `drm_prime` decode, RGA scaling और मौजूद होने पर
  Mali ICD पर OpenCL tone-map के ज़रिए)। QSV और Intel VAAPI ड्राइवर सिर्फ़ x86 के हैं
  (arm64 इमेज में जान-बूझकर इंस्टॉल नहीं); NVENC NVIDIA है। RKMPP इस्तेमाल करने के लिए MPP
  सेवा node और DRM render node को कंटेनर में map करें (`/dev/mpp_service`, `/dev/dri`, और
  जहाँ कर्नेल दे वहाँ `/dev/rga`); Evey उसे पहचानता है, एक असली encode जाँच चलाता है, फिर
  देने से पहले आपके बोर्ड पर हर आकार को नापता और साबित करता है - SoC के नाम से कुछ नहीं माना
  जाता, और जो आकार साबित न हो वह CPU पर रहता है। परियोजना ने RKMPP को अभी असली हार्डवेयर पर
  प्रमाणित नहीं किया है: इसे जितना बन पड़े वाला मानें और **Admin → ट्रांसकोडिंग → एनकोड
  कैलिब्रेशन** में देखें कि क्या प्रमाणित हुआ। Raspberry Pi का **V4L2 M2M** encoder
  **नहीं** जुड़ा है: Pi या Synology-श्रेणी के ARM (Realtek) NAS पर हार्डवेयर encode का कोई
  रास्ता नहीं, इसलिए ARM = **डायरेक्ट प्ले** (क्लाइंट फ़ाइल जस की तस चलाता है, लगभग शून्य
  ख़र्च) और CPU पर **सॉफ़्टवेयर ट्रांसकोड**। क्षमता calibrate की जाती है, अंदाज़े से नहीं:
  कमज़ोर core 720p सॉफ़्टवेयर तल रखता है और एक बार में एक encode मानता है; आप फिर भी
  `transcoding.max_video_sessions` (Admin → ट्रांसकोडिंग) घटा सकते हैं और डायरेक्ट प्ले पर
  टिक सकते हैं।
- **ARM पर भी HDR tone-mapping तेज़ रास्ते पर है।** साथ आने वाला arm64 jellyfin-ffmpeg
  SIMD `tonemapx` फ़िल्टर (NEON) के साथ आता है, जैसे amd64 build अपना AVX2 रूप लाता है -
  इसलिए HDR→SDR tone-mapping hardware-SIMD है, धीमी zscale शृंखला नहीं। (Evey रनटाइम पर
  `tonemapx` जाँचता है और zscale पर तभी लौटता है जब किसी build में वह न हो।)

अगर आप किसी अप्रकाशित arch के लिए ख़ुद इमेज बनाते हैं, तो `docker buildx build
--platform linux/arm64` सीधे काम करता है - Dockerfile x86-विशेष हिस्सों को `TARGETARCH`
के पीछे बचाता है।

## 9. एक्सटेंशन सैंडबॉक्स

एक्सटेंशन अलग प्रक्रियाओं में चलने वाला तीसरे पक्ष का कोड हैं। आधिकारिक इमेज में उन्हें क़ैद
करने के लिए ज़रूरी सब कुछ है - न `--privileged`, न कोई अतिरिक्त Docker सुरक्षा flag - और
`EVEY_PLUGINS_SANDBOX_MODE=auto` (डिफ़ॉल्ट) होस्ट की अनुमति वाला सबसे मज़बूत backend चुनता
है, हर आधुनिक कर्नेल पर Landlock। सर्वर ख़ुद कभी root के रूप में नहीं चलता। परत-दर-परत
ब्योरा (seccomp, uid अलगाव, संसाधन सीमाएँ) [SECURITY.md](SECURITY.md) में है; यह पृष्ठ सिर्फ़
बताता है कि क्या जाँचना है।

पहली शुरुआत के बाद असल में लागू सैंडबॉक्स **Admin → Plugins → Security Dashboard** (होस्ट
सैंडबॉक्स कार्ड) में जाँचें: आधुनिक कर्नेल पर `auto` को `landlock` दिखाना चाहिए; `basic`
का मतलब है कि होस्ट एक्सटेंशन की फ़ाइल सिस्टम पहुँच को अलग नहीं कर सकता - नीचे की तैनाती
गाइड देखें।

आपकी मशीन के लिए कौन सा सेटअप (पुराना NAS, बिना कंटेनर, क्या चालू/बंद करें, सीधी भाषा में जोखिम): [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md)। पूरा ख़तरा मॉडल और हर परत का ब्योरा: [plugins/security-model.md](plugins/security-model.md)।
