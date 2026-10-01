---
source_sha: 8aff69db523d14863f90a67e1d006815586528d8e04ea12732fa6de3856fee4a
reviewed: false
---
# उन्नत डिप्लॉयमेंट

[एक कंटेनर वाला इंस्टॉलेशन](../installation.md) ज़्यादातर सर्वरों के लिए ठीक है। यह पेज उन लेआउट को कवर करता है जिन्हें कोई ऑपरेटर पसंद कर सकता है: डेटाबेस या सर्च इंजन अलग सर्विस के रूप में, ARM होस्ट, इमेज के वेरिएंट, और असामान्य होस्ट पर प्लगइन सैंडबॉक्स। Evey को Docker के बिना चलाने के लिए देखें [कंटेनर के बिना](../deployment-bare-metal.md)।

## बिल्ट-इन डेटाबेस

डेटाबेस क्लस्टर `./evey-data/postgres` में रहता है (Postgres 17, पहली बार चालू होने पर बनाया गया, सर्वर उपयोगकर्ता के स्वामित्व में)। यह केवल कंटेनर के लूपबैक पर सुनता है और बनाया गया पासवर्ड माँगता है, इसलिए दूसरे उपयोगकर्ता के रूप में चल रहा प्लगइन इसे नहीं खोल सकता। सर्वर के अपने कनेक्शन बंद करने के बाद `docker stop` इसे ठीक से बंद करता है।

डेटा डायरेक्टरी किसी स्थानीय फ़ाइल सिस्टम (ext4, btrfs, xfs...) पर होनी चाहिए, SMB/CIFS या NFS शेयर पर नहीं।

SQL शेल खोलने के लिए:

```bash
docker exec -it evey-server sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) psql -h 127.0.0.1 -U evey evey'
```

### Postgres मुख्य संस्करण अपग्रेड

इमेज बिल्ट-इन Postgres का मुख्य संस्करण (17) तय रखती है। नए मुख्य संस्करण वाली इमेज पुराने क्लस्टर को खोलने से मना कर देती है, यहाँ की ओर इशारा करने वाले संदेश के साथ, बजाय ऐसी डेटा डायरेक्टरी चलाने के जिसे वह पढ़ नहीं सकती। पार जाने का तरीका है पुरानी इमेज से डंप और नई से पुनर्स्थापना:

```bash
docker compose exec evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_dump -h 127.0.0.1 -U evey -Fc evey' > evey-before-upgrade.dump
docker compose down
mv evey-data/postgres evey-data/postgres.old      # पुनर्स्थापना की पुष्टि होने तक इसे रखें
# compose फ़ाइल में नया इमेज टैग लिखें, फिर:
docker compose up -d                              # नए मुख्य संस्करण पर एक ख़ाली क्लस्टर बनाता है
docker compose exec -T evey sh -c 'PGPASSWORD=$(cat /data/keys/postgres.password) pg_restore -h 127.0.0.1 -U evey -d evey --no-owner' < evey-before-upgrade.dump
docker compose restart evey
```

## बाहरी Postgres

डेटाबेस का जीवनचक्र ख़ुद संभालने के लिए (प्रबंधित Postgres, तय मुख्य संस्करण, अपनी डंप नीति), इसे अलग सर्विस के रूप में चलाएँ और सर्वर को उसकी ओर इंगित करें। **`EVEY_DB_HOST` सेट करना ही बिल्ट-इन डेटाबेस को बंद करता है।**

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... एक कंटेनर वाली फ़ाइल की तरह, साथ में:
        environment:
          EVEY_DB_HOST: postgres
          EVEY_DB_SSLMODE: disable          # आंतरिक नेटवर्क, TLS नहीं
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
    EVEY_DB_PASSWORD=<strong-password>   # डिफ़ॉल्ट 'evey' के साथ सर्वर शुरू होने से मना करता है
    ```

## बाहरी Meilisearch

सर्च इंजन के लिए भी यही सिद्धांत: बिल्ट-इन बंद करें, सर्वर को अपने वाले की ओर इंगित करें और कुंजी साझा करें। स्पष्ट `MEILI_MASTER_KEY` बनाई गई `./evey-data/keys/meili.key` पर भारी पड़ता है।

=== "docker-compose.yml"

    ```yaml
    services:
      evey:
        # ... साथ में:
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

इंडेक्स डेटाबेस से फिर से बनता है (`evey admin reindex-search`), इसलिए `meili-data` को कभी भी फेंका जा सकता है।

## बिल्ट-इन डेटाबेस पर जाना

जो इंस्टॉलेशन Postgres को अलग सर्विस के रूप में चलाता है वह एक कंटेनर पर जा सकता है। बिल्ट-इन डेटाबेस `./evey-data/postgres-import.sql.gz` पर मिला डंप एक बार, पहली बार चालू होने पर बनाए गए क्लस्टर में, सर्वर चलने से पहले आयात करता है; फिर सर्वर के माइग्रेशन उसके ऊपर लागू होते हैं।

```bash
# 1. मौजूदा डेटाबेस का डंप लें (सादा SQL, gzip किया हुआ)
docker compose exec -T postgres pg_dump -U evey --no-owner --no-privileges evey \
  | gzip > evey-data/postgres-import.sql.gz
docker compose down

# 2. compose फ़ाइल को एक कंटेनर वाली फ़ाइल से बदलें, अपने मीडिया फ़ोल्डर,
#    रिवर्स प्रॉक्सी सेटिंग्स और हार्डवेयर लाइनें रखते हुए।

# 3. पहली शुरुआत: क्लस्टर बनाएँ, आयात करें, माइग्रेट करें, सर्वर शुरू करें
docker compose up -d && docker compose logs -f evey     # "evey: import done"

# 4. बाहरी Meilisearch का इंडेक्स साथ नहीं आता: उसे फिर से बनाएँ
docker exec evey-server evey admin reindex-search
```

`EVEY_DB_PASSWORD`, `MEILI_MASTER_KEY` और `EVEY_DB_HOST` / `EVEY_MEILI_URL` सेटिंग्स की अब ज़रूरत नहीं। सर्वर जाँच लेने तक `./postgres-data` रखें। किसी मौजूदा क्लस्टर के बगल में छोड़ा गया डंप बताया जाता है और अनदेखा किया जाता है, कभी मिलाया नहीं जाता; विफल आयात नया क्लस्टर हटा देता है ताकि अगली शुरुआत फिर से कोशिश करे।

## ARM होस्ट

स्थिर रिलीज़ एक बहु-आर्किटेक्चर इमेज (`linux/amd64` और `linux/arm64`) प्रकाशित करते हैं; Docker सही वाली चुनता है। प्री-रिलीज़ और ब्रांच टैग केवल amd64 हैं: ARM होस्ट पर एक स्थिर रिलीज़ तय करें।

- **ARM पर हार्डवेयर ट्रांसकोडिंग** RKMPP के ज़रिए Rockchip SoC (RK3588, RK3576, RK3568 श्रेणी) तक सीमित है। `/dev/mpp_service`, `/dev/dri` और, जहाँ कर्नेल उसे उपलब्ध कराए, `/dev/rga` मैप करें; Evey इस्तेमाल से पहले एनकोडर को जाँचता और बेंचमार्क करता है। प्रोजेक्ट ने अभी असली हार्डवेयर पर RKMPP की पुष्टि नहीं की है। Raspberry Pi या Realtek-आधारित ARM NAS में ऐसा कोई हार्डवेयर एनकोडर नहीं जिसे Evey इस्तेमाल कर सके: वे सर्वर सीधे प्लेबैक और सॉफ़्टवेयर ट्रांसकोडिंग पर निर्भर रहते हैं, जिनका आकार किसी भी दूसरे होस्ट की तरह उसी कैलिब्रेशन से तय होता है।
- **HDR टोन मैपिंग** ARM (NEON) पर वही SIMD `tonemapx` फ़िल्टर इस्तेमाल करती है जो x86-64 (AVX2) पर।

## इमेज टैग

इमेज `gravityz/evey` है।

- रिलीज़ टैग `vX.Y.Z` `X.Y.Z`, `X.Y`, `X` और `latest` प्रकाशित करते हैं। `latest` हमेशा सबसे नया रिलीज़ है।
- ब्रांच बिल्ड ब्रांच का नाम (उदाहरण के लिए `beta-0.0.1`) और `sha-<commit>` प्रकाशित करते हैं।
- **प्लगइन रनटाइम।** डिफ़ॉल्ट इमेज प्लगइन के लिए Python 3 साथ रखती है। `-nodejs` वेरिएंट Node.js जोड़ता है; `-full` PHP, Ruby और Lua जोड़ता है। वेरिएंट तभी इस्तेमाल करें जब आपके इंस्टॉल किए किसी प्लगइन को उसकी ज़रूरत हो।
- **GPU परिवार।** प्रकाशित इमेज में हर समर्थित Intel और AMD पीढ़ी के ड्राइवर होते हैं। ख़ुद बनाई गई इमेज केवल एक रख सकती है: `docker build --build-arg GPU=intel-legacy .` (Intel Gen9 से Gen11), `GPU=intel` (Gen12 और नए), `GPU=amd` या `GPU=none`। NVIDIA को किसी वेरिएंट की ज़रूरत नहीं: NVIDIA Container Toolkit चलते समय होस्ट का ड्राइवर डालता है।

## प्लगइन सैंडबॉक्स

इमेज में प्रिविलेज्ड मोड के बिना प्लगइन को सीमित रखने के लिए ज़रूरी सब कुछ है। डिफ़ॉल्ट सैंडबॉक्स मोड (`auto`) होस्ट द्वारा अनुमत सबसे मज़बूत बैकएंड चुनता है: किसी भी आधुनिक कर्नेल पर Landlock। वास्तव में लागू मोड **Admin → प्लगइन** के **सुरक्षा डैशबोर्ड** में जाँचें। जो होस्ट `basic` दिखाता है वह प्लगइन की फ़ाइल पहुँच को अलग नहीं कर सकता: इसका क्या मतलब है और क्या बदलना है, इसके लिए [प्लगइन सैंडबॉक्स सेटअप](../plugins/sandbox-deployment.md) देखें। परतें ख़ुद [सुरक्षा मॉडल और प्लगइन सैंडबॉक्स](../SECURITY.md) में बताई गई हैं।
