---
source_sha: 6bbcf7c54732dc7eb7955206675ddd1923616486ecdec3c988c6ad0463cf8a04
reviewed: true
---
# ट्रांसकोडिंग और हार्डवेयर

क्षमता, ग्राफ़िक्स कार्ड, क्वालिटी की सीमाओं और ट्रांसकोडिंग कैश के पीछे के
वेरिएबल। ज़्यादातर इंस्टॉलेशन इनमें से कुछ भी सेट नहीं करते: सर्वर मशीन को
खुद मापता है और हार्डवेयर खुद चुनता है, जैसा
[ट्रांसकोडिंग और सर्वर की क्षमता](../playback/transcoding.md) में समझाया गया
है। जहाँ **एडमिन → ट्रांसकोडिंग** में कोई सेटिंग है, वहाँ सहेजा गया मान भारी
पड़ता है।

## हार्डवेयर एक्सेलेरेशन

GPU को कंटेनर तक पहुँचाना:
[हार्डवेयर एक्सेलेरेशन](../administration/hardware-acceleration.md)।

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_HW_MODE` | खाली | `auto`, `off`, `nvenc`, `vaapi`, `qsv` या `rkmpp`। खाली: नीचे के दो वेरिएबल से तय होता है। | ट्रांसकोडिंग → **हार्डवेयर त्वरण**, तुरंत लागू (फिर से मापता है) |
| `EVEY_TRANSCODING_HW_ACCEL_ENABLED` | `false` | पुराना चालू/बंद स्विच। | ट्रांसकोडिंग → **हार्डवेयर त्वरण**, तुरंत लागू |
| `EVEY_TRANSCODING_HW_BACKEND` | खाली (पहचान) | एक ही बैकएंड (`nvenc`, `vaapi`, `qsv`) तय कर देता है: सिर्फ़ वही मापा जाता है। इससे एक होस्ट पर कई Evey कंटेनर अलग-अलग GPU रास्ता इस्तेमाल कर सकते हैं। | कोई नहीं |
| `EVEY_TRANSCODING_HW_DEVICE` | खाली (सभी) | एक ही डिवाइस तय कर देता है: एक render node (`/dev/dri/renderD129`) या एक CUDA नंबर (`1`)। `EVEY_TRANSCODING_HW_BACKEND` के साथ, कई GPU वाले होस्ट पर एक कंटेनर को एक GPU से बाँध देता है। | कोई नहीं |
| `EVEY_TRANSCODING_HW_FIRST_SEGMENT_TIMEOUT` | `10s` | जिस हार्डवेयर सेशन ने इतने समय में अपना पहला सेगमेंट नहीं बनाया, उसे अटका हुआ GPU माना जाता है और प्रोसेसर पर भेज दिया जाता है। | कोई नहीं |
| `EVEY_TRANSCODING_HW_TONEMAP` | चालू | `0` GPU पर HDR tone mapping बंद करता है और उसके लिए प्रोसेसर इस्तेमाल करता है, ऐसे ड्राइवर के लिए जो गड़बड़ करने के लिए जाना जाता है। | कोई नहीं |
| `EVEY_TRANSCODING_HW_PER_VARIANT` | `false` | GPU वाले रास्ते पर सबके लिए एक प्रोसेस की जगह हर क्वालिटी के लिए एक ffmpeg: गड़बड़ियों को अलग रखता है, और उस कई-आउटपुट वाले ढाँचे से बचाता है जो कमज़ोर VA ड्राइवर को क्रैश कर देता है। | ट्रांसकोडिंग → उन्नत सीमाएँ → **प्रति स्तर एक हार्डवेयर एनकोडर प्रक्रिया**, रीस्टार्ट ज़रूरी |

Intel के low-power एनकोडर और AV1 आउटपुट (स्विच और सीमा) का कोई वेरिएबल नहीं
है: ये सिर्फ़ **एडमिन → ट्रांसकोडिंग** में सेट होते हैं।

## क्षमता और सीमाएँ

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_MAX_SESSIONS` | `0` (CPU की संख्या) | एक साथ चलने वाले कुल ट्रांसकोडिंग सेशन। जो मशीन सॉफ़्टवेयर में मुश्किल से एक 4K HDR ट्रांसकोड संभाल पाती है, उस पर `1` या `2` रखें। | ट्रांसकोडिंग → उन्नत सीमाएँ → **अधिकतम समवर्ती HLS सत्र**, रीस्टार्ट ज़रूरी |
| `EVEY_TRANSCODING_MAX_CONCURRENT` | `2` | लाइव प्लेबैक के बाहर के ट्रांसकोड काम। | ट्रांसकोड कैश → **अधिकतम समवर्ती**, तुरंत लागू |
| `EVEY_TRANSCODING_SINGLE_VARIANT` | `true` | सिर्फ़ वही क्वालिटी एनकोड करता है जो दर्शक देख रहा है। `false` एक ही डिकोड सभी क्वालिटी में बाँटता है, पर वे क्वालिटी भी एनकोड करता है जिन्हें कोई नहीं देख रहा; यह सिर्फ़ उस होस्ट के लिए है जिसमें हार्डवेयर डिकोडिंग न हो और सोर्स भारी हों। | ट्रांसकोडिंग → उन्नत सीमाएँ → **एक समय पर एक ही लैडर स्तर एनकोड करें**, रीस्टार्ट ज़रूरी |
| `EVEY_TRANSCODING_SOFTWARE_MAX_HEIGHT` | `0` (मापा हुआ) | मापी गई सीमा की जगह प्रोसेसर की रिज़ॉल्यूशन सीमा ज़बरदस्ती तय करता है। | ट्रांसकोडिंग → उन्नत सीमाएँ → **सॉफ़्टवेयर ट्रांसकोड ऊँचाई सीमा**, रीस्टार्ट ज़रूरी |
| `EVEY_TRANSCODING_SOFTWARE_HDR_MAX_HEIGHT` | `0` (मापा हुआ) | वही, उन HDR सोर्स के लिए जिनका tone mapping प्रोसेसर पर होता है। | ट्रांसकोडिंग → उन्नत सीमाएँ → **सॉफ़्टवेयर HDR ऊँचाई सीमा**, रीस्टार्ट ज़रूरी |
| `EVEY_TRANSCODING_SOFTWARE_THREADS` | कोई सीमा नहीं | हर सॉफ़्टवेयर एनकोडर के thread (1 से 64; कुछ और हो तो कोई सीमा नहीं)। हार्डवेयर एनकोडर इसे नहीं देखते। | कोई नहीं |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_KBPS` | `0` (सोर्स जैसा) | ट्रांसकोड किए गए वीडियो के बिटरेट की सीमा। | ट्रांसकोडिंग → **वीडियो बिटरेट सीमा**, तुरंत लागू |
| `EVEY_TRANSCODING_MAX_VIDEO_BITRATE_REMOTE_KBPS` | `0` (कोई नहीं) | यही सीमा, घर के नेटवर्क के बाहर के क्लाइंट के लिए। | ट्रांसकोडिंग → **वीडियो बिटरेट सीमा, दूरस्थ दर्शक**, तुरंत लागू |

## कैश

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_TRANSCODING_CACHE_MAX_GB` | `10` | पूरे ट्रांसकोडिंग कैश का आकार (सेगमेंट, रीमक्स, तैयार किए गए डाउनलोड); जो सबसे लंबे समय से इस्तेमाल नहीं हुआ, वह पहले हटता है। | सर्वर कॉन्फ़िग → सर्वर सेटिंग्स → **ट्रांसकोड कैश बजट**, रीस्टार्ट ज़रूरी |
| `EVEY_TRANSCODE_CACHE_MIN_FREE_MB` | `2048` | कैश वाली डिस्क पर बचाकर रखी जाने वाली खाली जगह। इससे नीचे जाने पर पहले बेकार पड़े सेगमेंट हटाए जाते हैं, फिर नए सेगमेंट लिखने की जगह मना कर दिए जाते हैं; निकाले गए सबटाइटल सहेजे जाना बंद हो जाते हैं और एडमिन को सूचना जाती है। `0` यह गुंजाइश बंद कर देता है। देखें [ट्रांसकोडिंग कैश](../playback/transcoding.md#the-transcoding-cache)। | कोई नहीं |
| `EVEY_TRANSCODING_CACHE_DIR` | `/cache/transcode` | कैश कहाँ रहता है। इमेज इसे सेट करती है। | कोई नहीं |
| `EVEY_TRICKPLAY_CACHE_DIR` | `/data/trickplay` | सीक बार के थंबनेल कहाँ रहते हैं। इमेज इसे सेट करती है: इन्हें दोबारा बनाना महँगा है, इसलिए ये `/data` में रहते हैं। | कोई नहीं |

## कैलिब्रेशन क्लिप

माप के लिए एक 4K क्लिप एनकोड की जाती है। पसंद के क्रम में: आपकी फ़ाइल, एक बार
डाउनलोड की गई और उसके hash से जाँची गई क्लिप, या एक बनावटी क्लिप। असली क्लिप
बनावटी क्लिप से ज़्यादा सच्चा माप देती है। इनमें से किसी का भी पैनल में कोई
विकल्प नहीं है।

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_BENCH_HDR_SAMPLE`, `EVEY_BENCH_SDR_SAMPLE` | खाली | आपकी अपनी एक क्लिप, HDR और SDR। | कोई नहीं |
| `EVEY_BENCH_HDR_SAMPLE_URL`, `EVEY_BENCH_SDR_SAMPLE_URL` | खाली | एक बार डाउनलोड करने के लिए क्लिप, `https` पर। | कोई नहीं |
| `EVEY_BENCH_HDR_SAMPLE_SHA256`, `EVEY_BENCH_SDR_SAMPLE_SHA256` | खाली | उस डाउनलोड का SHA256; URL के साथ ज़रूरी। | कोई नहीं |

<!-- sources
cmd/evey/wire.go - EVEY_TRANSCODE_CACHE_MIN_FREE_MB (read at boot, not in config.go) ; internal/config/config.go - EVEY_TRANSCODING_*, EVEY_BENCH_*, EVEY_TRICKPLAY_CACHE_DIR ; struct comments HWBackend, HWDevice, MaxSessions, SingleVariant, SoftwareMaxHeightOverride, SoftwareAV1MaxHeightOverride (panel-only), AV1Enabled, AV1MaxHeight, HWLowPowerEnabled
panel overrides: cmd/evey/main.go - loadOverride (transcoding.*) ; cmd/evey/wire.go (transcoding.hw_mode read directly) ; internal/adapters/driving/http/settings_handler.go
hw tonemap: internal/adapters/driving/http/transcode_vod_wire.go (EVEY_TRANSCODING_HW_TONEMAP)
threads: internal/adapters/driven/ffmpeg/software_threads.go - parseSoftwareThreads
image defaults: Dockerfile (EVEY_TRANSCODING_CACHE_DIR, EVEY_TRICKPLAY_CACHE_DIR)
max concurrent: frontend/apps/web/src/routes/_auth/admin/transcode-cache.lazy.tsx (transcoding.max_concurrent) ; i18n admin.transcodeCache.maxConcurrent
labels: i18n admin.transcoding.advancedTitle, maxSessions, softwareMaxHeight, softwareHDRMaxHeight, singleVariant, hwPerVariant, hwModeLabel, maxBitrateLabel, maxBitrateRemoteLabel ; admin.serverSettings.transcodeCacheMaxGB
-->
