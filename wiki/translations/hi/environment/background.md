---
source_sha: d0631abae137226c008a3954088887193957c49501d53f8c38282e9314512c14
reviewed: true
---
# बैकग्राउंड का काम

उन दौरों के पीछे के वेरिएबल जो सर्वर अपने-आप चलाता है, और वे प्रोसेसर को आपस
में कैसे बाँटते हैं। हर दौर क्या करता है और प्लेबैक को उससे कैसे बचाया जाता है:
[बैकग्राउंड का काम और सर्वर पर लोड](../operations/background-work.md)।

## प्रोसेसर का हिस्सा

बैकग्राउंड के काम प्रोसेसर का कितना हिस्सा ले सकते हैं, यह **एडमिन → सर्वर
कॉन्फ़िग → आवंटित प्रोसेसिंग क्षमता** में सेट होता है, तुरंत, बिना रीस्टार्ट
के; इसका कोई environment वेरिएबल नहीं है। नीचे के दो वेरिएबल किसी एक pool का
आकार हाथ से तय करते हैं, और उसके लिए यह हिस्सा लागू नहीं होता।

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_LIBRARY_SCAN_WORKERS` | हिस्से से | स्कैन के दौरान एक साथ जाँची जाने वाली फ़ाइलें। | कोई नहीं |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | हिस्से से | इंट्रो और क्रेडिट के लिए एक साथ जाँचे जाने वाले टाइटल। | कोई नहीं |

## दौर

हर वेरिएबल की एक कुंजी है, जो **एडमिन → सर्वर कॉन्फ़िग → कॉन्फ़िगरेशन
प्रविष्टियाँ** में एक पंक्ति के रूप में भी मानी जाती है, और वह स्क्रीन ये
कुंजियाँ सुझाती है; पंक्ति environment पर भारी पड़ती है। उसी स्क्रीन का
**पृष्ठभूमि का काम** कार्ड मुख्य दौर सेट करता है। ये सभी सर्वर शुरू होते समय
पढ़े जाते हैं, इसलिए बदलाव किसी भी तरीके से किया जाए, अगले रीस्टार्ट पर लागू
होता है।

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_PAUSE_BG_ON_PLAYBACK` | चालू | कोई स्ट्रीम चलते समय बैकग्राउंड का काम रोक देता है। `off` उसे चलता रहने देता है। | कॉन्फ़िगरेशन प्रविष्टियाँ `transcoding.pause_bg_on_playback`, रीस्टार्ट ज़रूरी |
| `EVEY_KEYFRAME_BACKFILL` | चालू | स्कैन की गई फ़ाइलों के keyframe का इंडेक्स बनाता है और उनके frame rate की किस्म तय करता है। Direct Play और कॉपी इस पर निर्भर हैं। `off` इसे बंद करता है। | कॉन्फ़िगरेशन प्रविष्टियाँ `transcoding.keyframe_backfill`, रीस्टार्ट ज़रूरी |
| `EVEY_KEYFRAME_BACKFILL_BATCH` | सर्वर का डिफ़ॉल्ट | हर दौर में फ़ाइलें (> 0)। | कॉन्फ़िगरेशन प्रविष्टियाँ `transcoding.keyframe_backfill_batch`, रीस्टार्ट ज़रूरी |
| `EVEY_KEYFRAME_BACKFILL_PAUSE_MS` | सर्वर का डिफ़ॉल्ट | फ़ाइलों के बीच मिलीसेकंड (>= 0)। | कॉन्फ़िगरेशन प्रविष्टियाँ `transcoding.keyframe_backfill_pause_ms`, रीस्टार्ट ज़रूरी |
| `EVEY_KEYFRAME_BACKFILL_IDLE_SEC` | सर्वर का डिफ़ॉल्ट | खाली बैठे होने पर दौरों के बीच सेकंड (> 0)। | कॉन्फ़िगरेशन प्रविष्टियाँ `transcoding.keyframe_backfill_idle_sec`, रीस्टार्ट ज़रूरी |
| `EVEY_LEVEL_BACKFILL` | चालू | उन फ़ाइलों का H.264/HEVC level पढ़ता है जो इसके इकट्ठा होने से पहले स्कैन हुई थीं। `off` इसे बंद करता है। | कॉन्फ़िगरेशन प्रविष्टियाँ `transcoding.level_backfill`, रीस्टार्ट ज़रूरी |
| `EVEY_LEVEL_BACKFILL_BATCH`, `_PAUSE_MS`, `_IDLE_SEC` | सर्वर का डिफ़ॉल्ट | वही रफ़्तार, इस दौर के लिए। | कॉन्फ़िगरेशन प्रविष्टियाँ `transcoding.level_backfill_batch`, `_pause_ms`, `_idle_sec`, रीस्टार्ट ज़रूरी |
| `EVEY_SNAPSHOT_FASTPATH` | चालू | प्रोसेस के अंदर चलने वाले तेज़ डिकोडर से थंबनेल निकालता है। `off` सिर्फ़ सटीक ffmpeg वाला तरीका इस्तेमाल करता है, ताकि जब कोई थंबनेल गलत फ़्रेम दिखाए तो तेज़ डिकोडर को वजह के रूप में खारिज किया जा सके। | कॉन्फ़िगरेशन प्रविष्टियाँ `transcoding.snapshot_fastpath`, रीस्टार्ट ज़रूरी |
| `EVEY_TITLE_PROBE` | चालू | टाइटल को बैकग्राउंड में कम प्राथमिकता से जाँचता है, प्लेबैक के दौरान रुककर। `off` इसे बंद करता है। | कॉन्फ़िगरेशन प्रविष्टियाँ `transcoding.title_probe`, रीस्टार्ट ज़रूरी |

<!-- sources
cpu share: i18n admin.serverConfig.cpuBudget.* ; internal/app/library/scan_service.go - scanWorkersFromEnv ; cmd/evey/wire.go (EVEY_INTRO_DETECTION_MAX_PARALLEL)
passes: cmd/evey/wire.go, cmd/evey/wire_services.go, cmd/evey/wire_helpers.go (resolveFlag) ; frontend/apps/web/src/features/admin/operatorFlags.ts
background work card: i18n admin.serverConfig.backgroundJobs.*
-->
