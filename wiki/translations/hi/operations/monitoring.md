---
source_sha: d6335afa014ad627b79829f653197424d1a5c4572cdf3b9832ed19a3f7a079ac
reviewed: true
---
# निगरानी और लॉग

सर्वर क्या कर रहा है, यह कहाँ देखें: उस एडमिन के लिए जो कुछ ठीक करने की जगह
बस उसकी स्थिति देखना चाहता है। अभी क्या चल रहा है से लेकर महीने के आँकड़ों तक, और
बीच में लॉग। किसी लक्षण की जाँच करनी हो, तो
[समस्या का हल](troubleshooting.md) से शुरू करें।

## अभी क्या चल रहा है

**एडमिन → अभी चल रहा है** हर चल रही स्ट्रीम की सूची दिखाता है: कौन, क्या,
किस डिवाइस पर, और कैसे, हर ट्रैक के लिए (कॉपी किया हुआ, ट्रांसकोड किया हुआ, और
कौन सा बैकएंड उसे एनकोड कर रहा है)। एडमिन **रोकें** से किसी सेशन को रोक सकता है,
या **संदेश भेजें** से दर्शक को संदेश भेज सकता है।

डैशबोर्ड का **कौन ऑनलाइन है** कार्ड बताता है कि अभी कौन जुड़ा है, प्रोफ़ाइल के
हिसाब से, और हर ऐप से आखिरी संपर्क कितनी देर पहले हुआ। जिस Chromecast का
टेलीविज़न Evey का receiver लोड रखे रहता है, वह अपना प्लेबैक खत्म होने और
**अभी चल रहा है** से हटने के बाद भी सूची में रहता है; **इस Chromecast को
रोकें** उसे रुकने का आदेश भेजता है, जिसे टेलीविज़न 30 सेकंड के अंदर लागू करता
है।

**एडमिन → सिस्टम स्वास्थ्य** प्रोसेसर, मेमोरी, बैंडविड्थ और स्टोरेज दिखाता है,
और यह कि ट्रांसकोडिंग की क्षमता का कितना हिस्सा इस्तेमाल हो रहा है।

<figure class="evey-shot" markdown="span">
  ![डेमो सर्वर पर एडमिन → सिस्टम स्वास्थ्य](../images/admin-system-health.webp){ loading=lazy }
  <figcaption><strong>एडमिन → सिस्टम स्वास्थ्य</strong>, नीचे चल रहे बैकग्राउंड काम के साथ (स्क्रीन अंग्रेज़ी में)।</figcaption>
</figure>

## लॉग

**एडमिन → लॉग और डीबग** सर्वर के लॉग में खोजता है और उसे लाइव दिखाता है
(**लाइव**), और **क्षेत्र** से किसी एक हिस्से तक सीमित करता है (प्लेयर और
ट्रांसकोड, seek index, सबटाइटल, इंट्रो की पहचान, Chromecast, साइन इन,
इन्फ्रास्ट्रक्चर...)। जब कोई उपयोगकर्ता समस्या बताए, तो देखने की पहली जगह
यही है।

यही लॉग डिस्क पर भी है, हर दिन की एक JSON फ़ाइल `./evey-data/logs/` में
(कंटेनर में `/data/logs/evey-YYYY-MM-DD.log`), जो डिफ़ॉल्ट रूप से 90 दिन रखी
जाती है। `docker compose logs evey` सिर्फ़ वह दिखाता है जो कंटेनर शुरू होते समय
छापता है, और सर्वर चालू न हो तो यही पढ़ना है। देखें
[समस्या का हल](troubleshooting.md#the-logs)।

## प्लेबैक असल में कैसा रहा

**एडमिन → स्ट्रीमिंग गुणवत्ता** उस जानकारी से बनता है जो प्लेयर चलते समय भेजते
हैं (बफ़र का स्तर, दोबारा बफ़रिंग, स्पीड, CMCD फ़ॉर्मेट में): दर्शकों को कितनी
बार इंतज़ार करना पड़ा, कौन से टाइटल सबसे ज़्यादा अटकते हैं, और किस तरह के
डिवाइस को दिक्कत होती है, पिछले एक दिन या एक हफ़्ते में।

सर्वर इस पर कदम भी उठाता है। जब किसी एक तरह का सोर्स दर्शकों को बार-बार
इंतज़ार करवाता है, तो वह कुछ समय के लिए उस तरह के सोर्स की सबसे ऊँची क्वालिटी
घटा देता है, कैलिब्रेट की गई सीमा के ऊपर से, फिर अपने-आप सीमा हटा देता है।
चालू सीमाएँ **स्वचालित गुणवत्ता सीमाएँ** में दिखती हैं, वजह और खत्म होने के
समय के साथ; **रीसेट करें** किसी एक को तुरंत हटा देता है।

## इतिहास और आँकड़े

- **एडमिन → एनालिटिक्स**: आज और इस हफ़्ते कितनी बार चलाया गया, सक्रिय
  उपयोगकर्ता, पिछले 30 दिनों में सबसे ज़्यादा देखे गए टाइटल।
- **एडमिन → गतिविधि**: प्लेबैक और एडमिन के कामों का इतिहास, और चल रहा
  बैकग्राउंड काम।

हर तरह का इतिहास कितने समय रखा जाए, यह **एडमिन → सर्वर कॉन्फ़िग → सर्वर
सेटिंग्स** में तय होता है (प्लेबैक सेशन, audit लॉग, सर्वर के माप, स्ट्रीमिंग
गुणवत्ता)। बदलाव अगली रोज़ की सफ़ाई पर लागू होता है।

## Prometheus metrics

`GET /metrics` सर्वर के counters को Prometheus text फ़ॉर्मेट में देता है, सबके
नाम `evey_...` हैं। यह एडमिन सेशन को जवाब देता है, या एक तय टोकन को जो
`EVEY_METRICS_SCRAPE_TOKEN` में सेट है और `Authorization: Bearer <token>` के रूप
में भेजा जाता है, ताकि Prometheus या Grafana बिना दोबारा साइन इन किए डेटा ले
सकें ([बैकअप, लॉग और निगरानी](../environment/logs-monitoring.md))। पहले चार्ट
के लायक families:

| Metric | यह क्या बताता है |
| --- | --- |
| `evey_transcodes_active`, `evey_transcode_queue_depth` | बैकग्राउंड ट्रांसकोड के काम (लाइव प्लेबैक के बाहर) जो चल रहे हैं, और जो जगह का इंतज़ार कर रहे हैं। |
| `evey_hlsvod_sessions_active`, `evey_hlsvod_sessions_rejected_total` | प्लेबैक के segments बनाने वाले FFmpeg प्रोसेस, और वे शुरुआतें जो मना कर दी गईं क्योंकि सर्वर अपनी सीमा पर था। |
| `evey_hlsvod_cache_size_bytes`, `evey_hlsvod_cache_evicted_bytes_total` | डिस्क पर segment कैश का आकार, और सफ़ाई प्रक्रिया ने कितनी जगह खाली की। |
| `evey_mkvgo_walks_in_flight`, `evey_mkvgo_plan_stats` | सर्वर के अंदर चलने वाला packager: पढ़ी जा रही windows, कतार में लगी reads (`stat="slot_waits"`), देखें [Evey में MKVGo](../playback/mkvgo.md#load-replaced-files-and-updates)। |
| `evey_http_requests_total`, `evey_http_request_duration_seconds` | method और path के हिसाब से अनुरोध और देरी। |
| `evey_watcher_alive`, `evey_scan_queue_depth` | क्या हर लाइब्रेरी के फ़ोल्डर मॉनिटर ने अपनी आखिरी जाँच का जवाब दिया (1 या 0), और स्कैन का इंतज़ार कर रही फ़ाइलें। |

## कमांड लाइन

`evey admin doctor` एक ही रिपोर्ट में पूरे सर्वर की जाँच करता है; जाँच की बाकी
कमांड [कमांड लाइन](../runbooks/cli.md) में दी गई हैं।

<!-- sources
who's online: frontend/apps/web/src/features/admin/DashboardOnline.tsx (presence heartbeat, profile names, idle, SendCastReceiverCommand) ; i18n admin.online.*
logs: internal/pkg/logger/logger.go - New (file only outside DevMode), rotatingWriter ; config EVEY_LOG_RETENTION_DAYS ; docker-compose.yml ./evey-data:/data
labels: i18n admin.sessions.*, admin.systemHealth.*, admin.logs.where.*, admin.logs.live, admin.qoe.*, admin.analytics.*, admin.activity.*, admin.serverSettings.*RetentionDays, retentionSweepHint
automatic limits: i18n admin.qoe.overrides.*
metrics: internal/pkg/metrics/metrics.go (names) ; internal/adapters/driving/http/auth_middleware.go - httpScrapeOrAdminMiddleware (Bearer scrape token or admin JWT) ; docs/adr/0018-cmcd-qoe-pipeline.md
-->
