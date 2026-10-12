---
source_sha: 0bcab9ec2d5eb546c48ee32a751bbd41320d9cb2c60cab549b010ec73e26f367
reviewed: true
---
# लाइब्रेरी और मेटाडेटा

स्कैन, मेटाडेटा के स्रोत और इंट्रो की पहचान के पीछे के वेरिएबल। इनमें से
ज़्यादातर का एडमिन पैनल में एक फ़ील्ड है, और वहाँ कोई मान सहेजते ही उसी को
प्राथमिकता मिलती है; पैनल वाला हिस्सा
[लाइब्रेरी और मेटाडेटा](../administration/libraries.md) में बताया गया है।

## स्कैन

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_LIBRARY_WATCHER_ENABLED` | `true` | फ़ाइलों में बदलाव की रीयल-टाइम निगरानी। | सर्वर कॉन्फ़िग → सर्वर सेटिंग्स → **लाइब्रेरी फ़ाइल मॉनिटर**, रीस्टार्ट ज़रूरी |
| `EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL` | `2m` | फ़ोल्डर कितनी बार जाँचे जाते हैं, ताकि वे बदलाव मिलें जो फ़ाइल मॉनिटर से छूट गए (नेटवर्क शेयर)। | सर्वर कॉन्फ़िग → सर्वर सेटिंग्स → **आवधिक स्कैन अंतराल**, रीस्टार्ट ज़रूरी |
| `EVEY_LIBRARY_SCAN_WORKERS` | प्रोसेसर के हिस्से से | स्कैन के दौरान एक साथ जाँची जाने वाली फ़ाइलें। हाथ से सेट करने पर स्कैन के लिए **प्रोसेसिंग क्षमता** का हिस्सा लागू नहीं होता। | कोई नहीं |
| `EVEY_SEEKINDEX_STATE_DIR` | `/data/seek-index` | जहाँ मीडिया स्वास्थ्य का स्कैन अपने नतीजे और चल रहा काम रीस्टार्ट के पार भी रखता है। खाली: कुछ नहीं रखा जाता। | कोई नहीं |

## मेटाडेटा

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_METADATA_LANGUAGE` | `en` | टाइटल और सारांश की भाषा। सेटअप विज़ार्ड में चुनी गई भाषा को इससे ज़्यादा प्राथमिकता मिलती है। | सर्वर कॉन्फ़िग → भाषा और क्षेत्र → **मेटाडेटा भाषा**, तुरंत लागू |
| `EVEY_TMDB_API_KEY` | पहले से डाली हुई कुंजी | आपकी अपनी TMDB कुंजी, आपके अपने कोटे के लिए। | सर्वर कॉन्फ़िग → API कुंजियाँ → **TMDB API कुंजी**, रीस्टार्ट ज़रूरी |
| `EVEY_OMDB_API_KEY` | खाली | एक OMDb कुंजी, फ़िल्मों के लिए दूसरा स्रोत। | सर्वर कॉन्फ़िग → API कुंजियाँ → **OMDB API कुंजी**, रीस्टार्ट ज़रूरी |
| `EVEY_OPENSUBTITLES_API_KEY` | खाली | एक OpenSubtitles कुंजी, API से सबटाइटल खोजने के लिए। | सर्वर कॉन्फ़िग → API कुंजियाँ → **OpenSubtitles API कुंजी**, रीस्टार्ट ज़रूरी |
| `EVEY_OPENSUBTITLES_USERNAME`, `EVEY_OPENSUBTITLES_PASSWORD` | खाली | एक OpenSubtitles खाता, उसके API में साइन इन करने के लिए। | कोई नहीं |

## इंट्रो, रीकैप और क्रेडिट

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_INTRO_PROVIDERS_ENABLED` | `false` | सर्वर की अपनी पहचान से पहले कम्युनिटी डेटाबेस (पहले TheIntroDB, फिर IntroDB) इस्तेमाल करता है। | सर्वर कॉन्फ़िग → इंट्रो स्किप प्रदाता → **बाहरी इंट्रो स्किप प्रदाता सक्षम करें**, रीस्टार्ट ज़रूरी |
| `EVEY_INTRO_TIDB_API_KEY` | खाली | एक TheIntroDB कुंजी: दिन में 100 की जगह 500 lookup। | सर्वर कॉन्फ़िग → API कुंजियाँ → **TheIntroDB API कुंजी**, रीस्टार्ट ज़रूरी |
| `EVEY_INTRO_AUTO_SKIP_ENABLED` | `false` | हर प्लेयर पहचाने गए इंट्रो खुद छोड़ देता है। | सर्वर कॉन्फ़िग → सर्वर सेटिंग्स → **इंट्रो स्वतः छोड़ें**, तुरंत लागू |
| `EVEY_INTRO_SKIP_PREVIEW_ENABLED` | `false` | अगले एपिसोड की झलक छोड़ने का विकल्प भी देता है। | सर्वर कॉन्फ़िग → सर्वर सेटिंग्स → **पूर्वावलोकन सेगमेंट छोड़ें**, तुरंत लागू |
| `EVEY_INTRO_DETECTION_MAX_PARALLEL` | प्रोसेसर के हिस्से से | एक साथ जाँचे जाने वाले टाइटल। हाथ से सेट करने पर इस काम के लिए **प्रोसेसिंग क्षमता** का हिस्सा लागू नहीं होता। | कोई नहीं |
| `EVEY_INTRO_FFMPEG_THREADS` | `1` | पहचान के दौरान हर ffmpeg प्रोसेस के thread (1 से 16)। | कोई नहीं |

<!-- sources
internal/config/config.go - EVEY_LIBRARY_*, EVEY_SEEKINDEX_STATE_DIR, EVEY_METADATA_LANGUAGE, EVEY_INTRO_*
watcher / scan interval: cmd/evey/wire_events.go (loadOverride library.watcher_enabled, library.periodic_scan_interval) ; internal/adapters/driving/http/settings_handler.go (requiresRestart)
scan workers: internal/app/library/scan_service.go - scanWorkersFromEnv
api keys: cmd/evey/wire_infra.go - resolveAPIKeyWithBuiltin, resolveAPIKey ; cmd/evey/wire_services.go (opensubtitles) ; i18n admin.serverConfig.apiKeys.*
language: cmd/evey/wire_infra.go (metadata.language wins) ; internal/app/config/locale_defaults.go
intro: cmd/evey/wire.go (intro_providers.enabled, api.tidb_key, EVEY_INTRO_DETECTION_MAX_PARALLEL) ; internal/adapters/driven/ffmpeg/intro_threads.go ; i18n admin.serverConfig.introProvidersRestart ; admin.serverSettings.introAutoSkip, skipPreview
-->
