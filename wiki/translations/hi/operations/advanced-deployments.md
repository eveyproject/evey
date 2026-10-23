---
source_sha: f0e5f75d4befda29fbffbe7da80a4cc45e7a86e9429753abdee557b19bc5a40b
reviewed: true
---
# एडवांस्ड डिप्लॉयमेंट { #advanced-deployments }

[एक कंटेनर वाला इंस्टॉल](../installation.md) ज़्यादातर सर्वर के लिए ठीक है। यह
पेज तब खोलें जब आप डेटाबेस या सर्च इंजन को अलग सर्विस के रूप में चाहते हों, जब
होस्ट ARM हो, जब आपको इमेज का कोई दूसरा रूप चाहिए, या जब प्लगइन सैंडबॉक्स
`landlock` के अलावा कुछ और बताए। Evey को Docker के बिना ही चलाने के लिए देखें
[बिना कंटेनर के](../deployment-bare-metal.md)।

## embedded डेटाबेस { #the-embedded-database }

क्लस्टर `./evey-data/postgres` में रखा जाता है (Postgres 17, पहली बार शुरू होने पर
बनता है, सर्वर के उपयोगकर्ता का होता है)। यह सिर्फ़ कंटेनर के loopback पर सुनता
है और बनाया गया पासवर्ड माँगता है, इसलिए किसी दूसरे उपयोगकर्ता के रूप में चल
रहा प्लगइन इसे नहीं खोल सकता। `docker stop` इसे ठीक से बंद करता है, जब सर्वर
अपने कनेक्शन बंद कर चुका होता है।

डेटा वाला फ़ोल्डर लोकल फ़ाइल सिस्टम (ext4, btrfs, xfs...) पर होना चाहिए, SMB/CIFS
या NFS शेयर पर नहीं। SQL shell खोलने के लिए:

```bash
docker exec -it evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

### Postgres के नए major वर्ज़न पर जाना { #move-to-a-newer-postgres-major }

इमेज embedded डेटाबेस का major वर्ज़न तय रखती है। जो इमेज नया major वर्ज़न
लाती है, वह पुराना क्लस्टर खोलने से मना कर देती है, और एक संदेश इस पेज की ओर
भेजता है, बजाय ऐसे डेटा फ़ोल्डर को चलाने के जिसे वह पढ़ नहीं सकती। नए वर्ज़न पर जाने का
तरीका है पुरानी इमेज से एक dump, और नई इमेज से उसका restore:

```bash
docker compose exec evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_dump -h 127.0.0.1 -U evey -Fc evey' > evey-before-upgrade.dump
docker compose down
mv evey-data/postgres evey-data/postgres.old      # restore की जाँच होने तक इसे रखें
# compose फ़ाइल में नया इमेज tag डालें, फिर:
docker compose up -d                              # नए major पर एक खाली क्लस्टर बनाता है
docker compose exec -T evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_restore -h 127.0.0.1 -U evey -d evey --no-owner' < evey-before-upgrade.dump
docker compose restart evey
```

## Postgres को अलग सर्विस के रूप में चलाएँ { #run-postgres-as-a-separate-service }

डेटाबेस का पूरा रख-रखाव खुद करने के लिए (कोई managed Postgres, तय major वर्ज़न,
dump का अपना तरीका), उसे अपनी अलग सर्विस के रूप में चलाएँ और सर्वर को उससे
जोड़ें। `EVEY_DB_HOST` सेट करने से ही embedded डेटाबेस बंद होता है। डिफ़ॉल्ट
पासवर्ड `evey` के साथ सर्वर शुरू होने से मना करता है।

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... एक कंटेनर वाली फ़ाइल की तरह, और साथ में:
        environment:
          EVEY_DB_HOST: postgres
          EVEY_DB_SSLMODE: disable          # अंदर का नेटवर्क, TLS नहीं
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
    EVEY_DB_PASSWORD=<strong-password>
    ```

## Meilisearch को अलग सर्विस के रूप में चलाएँ { #run-meilisearch-as-a-separate-service }

सर्च इंजन के लिए भी यही तरीका: embedded सर्च इंजन बंद करें, सर्वर को अपने सर्च
इंजन से जोड़ें और कुंजी दोनों को दें। साफ़ दिया गया `MEILI_MASTER_KEY` बनाई गई
`./evey-data/keys/meili.key` से ज़्यादा प्राथमिकता रखता है।

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... और साथ में:
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

इंडेक्स `docker exec evey-server evey admin reindex-search` से डेटाबेस से दोबारा
बनता है, इसलिए `meili-data` को कभी भी हटाया जा सकता है।

## embedded डेटाबेस पर लौटना { #move-back-onto-the-embedded-database }

जो इंस्टॉलेशन Postgres को अलग सर्विस के रूप में चलाता है, वह एक कंटेनर वाले
सेटअप पर आ सकता है। embedded डेटाबेस `./evey-data/postgres-import.sql.gz` पर
मिले dump को एक बार, पहली बार शुरू होने पर बनाए गए क्लस्टर में, सर्वर चलने से
पहले डाल लेता है; फिर सर्वर के माइग्रेशन उसके ऊपर लागू होते हैं।

```bash
# 1. मौजूदा डेटाबेस का dump लें (सादा SQL, gzip किया हुआ)
docker compose exec -T postgres pg_dump -U evey --no-owner --no-privileges evey \
  | gzip > evey-data/postgres-import.sql.gz
docker compose down

# 2. compose फ़ाइल को एक कंटेनर वाली फ़ाइल से बदलें, अपने मीडिया फ़ोल्डर,
#    रिवर्स प्रॉक्सी सेटिंग्स और हार्डवेयर वाली लाइनें रखते हुए।

# 3. पहली शुरुआत: क्लस्टर बनाना, import, माइग्रेशन, सर्वर शुरू करना
docker compose up -d && docker compose logs -f evey     # "evey: import done"

# 4. बाहरी Meilisearch का इंडेक्स साथ नहीं आता: उसे दोबारा बनाएँ
docker exec evey-server evey admin reindex-search
```

`EVEY_DB_PASSWORD`, `MEILI_MASTER_KEY` और `EVEY_DB_HOST` / `EVEY_MEILI_URL` की
सेटिंग्स की अब ज़रूरत नहीं। सर्वर की जाँच करने तक `./postgres-data` रखें। किसी
मौजूदा क्लस्टर के पास छोड़ा गया dump लॉग में बताया जाता है और अनदेखा किया जाता
है, कभी मिलाया नहीं जाता; import फ़ेल हो जाए, तो नया क्लस्टर हटा दिया जाता है
ताकि अगली बार शुरू होने पर dump से फिर कोशिश हो।

## ARM होस्ट { #arm-hosts }

किसी release tag (`0.1.0`, pre-release भी) से बनी इमेज कई
आर्किटेक्चर के लिए होती हैं (`linux/amd64` और `linux/arm64`); Docker सही वाली
चुन लेता है। branch tag सिर्फ़ amd64 के लिए हैं: ARM होस्ट पर
कोई release tag पिन करें।

- **ARM पर हार्डवेयर ट्रांसकोडिंग** RKMPP के ज़रिए सिर्फ़ Rockchip SoC (RK3588,
  RK3576, RK3568 वर्ग) तक सीमित है। `/dev/mpp_service`, `/dev/dri` और, जहाँ
  kernel इसे दिखाता है, `/dev/rga` कंटेनर को दें; सर्वर एनकोडर को इस्तेमाल करने
  से पहले जाँचता और मापता है। प्रोजेक्ट ने RKMPP को अभी असली हार्डवेयर पर
  परखा नहीं है। Raspberry Pi या Realtek वाले ARM NAS में ऐसा कोई हार्डवेयर
  एनकोडर नहीं है जिसे सर्वर इस्तेमाल कर सके: ऐसे होस्ट direct play और
  सॉफ़्टवेयर ट्रांसकोडिंग पर चलते हैं, जिसका आकार बाकी होस्ट की तरह उसी
  कैलिब्रेशन से तय होता है।
- **HDR tone mapping** ARM (NEON) पर भी वही SIMD `tonemapx` फ़िल्टर इस्तेमाल
  करता है जो x86-64 (AVX2) पर।

## इमेज tag और रूप { #image-tags-and-variants }

इमेज `eveyproject/evey` है।

| Tag | किससे प्रकाशित होता है | टिप्पणी |
|---|---|---|
| `X.Y.Z`, `X.Y`, `X`, `latest` | एक release tag `vX.Y.Z` | `latest` हमेशा सबसे नई स्थिर release है |
| एक pre-release tag (हाइफ़न वाला) | एक pre-release tag | सिर्फ़ अपना tag प्रकाशित करता है, `latest` को कभी नहीं बदलता |
| branch का नाम, `sha-<commit>` | branch से बनी इमेज | सिर्फ़ amd64 |

हर tag तीन रूपों में आता है: डिफ़ॉल्ट रूप में प्लगइन के सभी रनटाइम होते हैं
(Python 3, Node.js, PHP, Ruby और Lua), `-nodejs` में Python और Node.js रहते हैं,
`-lean` में सिर्फ़ Python रहता है (जैसे `latest-lean`)। हल्का रूप सिर्फ़ तब चुनें
जब इमेज का आकार मायने रखता हो: installer उस प्लगइन को मना कर देता है जिसकी
बताई गई भाषा इमेज में नहीं है, और संदेश में डिफ़ॉल्ट इमेज का नाम बताता है।

प्रकाशित इमेज में हर समर्थित Intel और AMD पीढ़ी के ड्राइवर होते हैं। खुद बनाई
गई इमेज में एक ही परिवार रखा जा सकता है: `docker build
--build-arg GPU=intel-legacy .` (Intel Gen9 से Gen11), `GPU=intel` (Gen12 और नए),
`GPU=amd` या `GPU=none`। NVIDIA के लिए किसी रूप की ज़रूरत नहीं: NVIDIA
Container Toolkit चलते समय होस्ट का ड्राइवर कंटेनर में डाल देता है।

## किसी असामान्य होस्ट पर प्लगइन सैंडबॉक्स { #the-plugin-sandbox-on-an-unusual-host }

इमेज में प्लगइन को सीमित और अलग रखने के लिए सब कुछ है, privileged मोड के बिना।
डिफ़ॉल्ट सैंडबॉक्स मोड (`auto`) वह सबसे मज़बूत बैकएंड चुनता है जिसकी होस्ट
अनुमति देता है: किसी भी आधुनिक kernel पर Landlock। लागू मोड **एडमिन → प्लगइन**
के **सुरक्षा डैशबोर्ड** में देखें। जो होस्ट `basic` दिखाता है, वह प्लगइन की
फ़ाइलों तक पहुँच को अलग नहीं कर सकता: [प्लगइन सैंडबॉक्स
सेटअप](../plugins/sandbox-deployment.md) बताता है कि इसका क्या मतलब है और क्या
बदलना है। परतें खुद [प्लगइन सैंडबॉक्स और ख़तरा मॉडल](../SECURITY.md) में बताई
गई हैं।

<!-- sources
embedded db, import, major upgrade, embedded meili: docker/entrypoint.sh (PG_MAJOR, PG_IMPORT, "evey: import done", EVEY_EMBEDDED_MEILI, MEILI_MASTER_KEY_FILE)
external db switch and default password refusal: internal/config/config.go (EVEY_DB_HOST, hardFail on "evey")
reindex: cmd/evey/admin.go - adminReindexSearch
arm, variants, GPU families: Dockerfile (TARGETARCH, RUNTIMES, GPU) ; .github/workflows/ci.yml (tags, flavor suffix, platforms) ; internal/adapters/driven/ffmpeg/hwdetect.go - detectRKMPP
sandbox auto: internal/adapters/driven/pluginrunner/runner_landlock.go - resolveAutoMode ; i18n/en/admin.yaml - plugins.security.title
-->
