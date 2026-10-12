---
source_sha: 3bd3ddf0df0a475d6ddf4417b5aa5b3630c871184888584df522bdae5daee7d4
reviewed: true
---
# प्लेबैक डिलीवरी और डाउनलोड

वे स्विच जो तय करते हैं कि स्ट्रीम कैसे पैक की जाती हैं और सर्वर डाउनलोड के
लिए क्या तैयार करता है। डिफ़ॉल्ट ही सुझाया गया व्यवहार है: ये किसी समस्या की
जाँच करते समय कोई रास्ता बंद करने, या कोई बंद रास्ता चुनकर चालू करने के लिए
हैं। सबसे पहले स्ट्रीम कैसे चुनी जाती है:
[Evey प्लेबैक का तरीका कैसे चुनता है](../playback/how-playback-is-decided.md)।

जिस वेरिएबल की पैनल कुंजी है, उसे **एडमिन → सर्वर कॉन्फ़िग → कॉन्फ़िगरेशन
प्रविष्टियाँ** में उसी कुंजी वाली एक पंक्ति के रूप में भी सेट किया जा सकता है;
पंक्ति environment पर भारी पड़ती है। पैनल डाउनलोड वाली कुंजियाँ सुझाता है;
बाकी को वैसे ही टाइप करें जैसे वे यहाँ लिखी हैं। ये सभी सर्वर शुरू होते समय
पढ़ी जाती हैं, इसलिए बदलाव किसी भी तरीके से किया जाए, अगले रीस्टार्ट पर लागू
होता है।

## पैकेजिंग

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_HLS_ONDEMAND` | चालू | जिस फ़ाइल के ट्रैक कॉपी किए जा सकते हैं, उसे चलते-चलते दोबारा पैक करके भेजता है, ffmpeg के बिना और कैश में कुछ लिखे बिना। `off`: ऐसी फ़ाइलें ffmpeg वाले कॉपी के रास्ते से जाती हैं। | कॉन्फ़िगरेशन प्रविष्टियाँ `transcoding.hls_ondemand`, रीस्टार्ट ज़रूरी |
| `EVEY_REMUX_COPY` | चालू | ffmpeg वाला कॉपी का रास्ता: फ़ाइल अनुमति दे तो वीडियो दोबारा एनकोड किए बिना कॉपी होता है। `off`: ऐसी फ़ाइलें ट्रांसकोड होती हैं। | कॉन्फ़िगरेशन प्रविष्टियाँ `transcoding.remux_copy`, रीस्टार्ट ज़रूरी |
| `EVEY_HLS_AUDIO_RENDITION_ENABLED` | `true` | ऑडियो ट्रैक को अलग rendition के रूप में देता है, ताकि प्लेयर वीडियो दोबारा मँगाए बिना भाषा बदल सके। इससे कॉपी किया गया वीडियो, अलग से दोबारा एनकोड किए गए ऑडियो ट्रैक के साथ भी चल सकता है। | कोई नहीं |
| `EVEY_HLS_COPY_FALLBACK_RUNG` | `true` | कॉपी की गई स्ट्रीम के साथ एक कम क्वालिटी वाला ट्रांसकोड किया हुआ स्तर जोड़ता है, ताकि जिस प्लेयर का कनेक्शन बैठ जाए वह रुकने की जगह नीचे के स्तर पर जा सके। जब तक कोई प्लेयर इसे इस्तेमाल न करे, इसका कोई खर्च नहीं। | कोई नहीं |
| `EVEY_HLS_ONDEMAND_CONCAT` | चालू | एपिसोड के बीच बिना रुकावट का प्लेबैक, जब वेब प्लेयर इसे माँगे। जो एपिसोड कॉपी नहीं हो सकता, वह सामान्य स्ट्रीम पर लौट जाता है। | कॉन्फ़िगरेशन प्रविष्टियाँ `transcoding.hls_ondemand_concat`, रीस्टार्ट ज़रूरी |
| `EVEY_HLS_ONDEMAND_TRACK_TRIM` | बंद | हर स्ट्रीम में सभी ऑडियो ट्रैक की जगह सिर्फ़ चुना हुआ ट्रैक डालता है। | कॉन्फ़िगरेशन प्रविष्टियाँ `transcoding.hls_ondemand_track_trim`, रीस्टार्ट ज़रूरी |
| `EVEY_HLS_ONDEMAND_ENCRYPT` | बंद | दोबारा पैक की गई स्ट्रीम को AES-128 से एन्क्रिप्ट करता है (सिर्फ़ HLS: तब कोई DASH manifest नहीं दिया जाता)। | कॉन्फ़िगरेशन प्रविष्टियाँ `transcoding.hls_ondemand_encrypt`, रीस्टार्ट ज़रूरी |
| `EVEY_HLS_KEY_ROTATION_SEGMENTS` | `0` | एन्क्रिप्शन चालू होने पर हर N सेगमेंट में कुंजी बदलता है; `0` = हर टाइटल के लिए एक कुंजी। | कॉन्फ़िगरेशन प्रविष्टियाँ `transcoding.hls_key_rotation_segments`, रीस्टार्ट ज़रूरी |
| `EVEY_CAST_WASM_AUDIO_ENABLED` | `false` | Chromecast को AAC लेने की जगह Dolby Digital खुद डिकोड करने देता है। बंद है, क्योंकि इससे सस्ते receiver पर बहुत बोझ पड़ा और कोई बचत नहीं हुई। ब्राउज़र पर इसका असर नहीं होता। | कोई नहीं |

## डाउनलोड

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_COMPAT_DOWNLOAD` | चालू | ऐसा डाउनलोड तैयार करता है जिसका ऑडियो फ़ोन चला सके (AAC), वीडियो वैसा ही रहता है। सिर्फ़ ऑडियो दोबारा एनकोड होता है, जिसमें कुछ मिनट लगते हैं। | कॉन्फ़िगरेशन प्रविष्टियाँ `transcoding.compat_download`, रीस्टार्ट ज़रूरी |
| `EVEY_DOWNLOAD_QUALITY_CHOICE` | बंद | डाउनलोड के लिए 720p की एक छोटी कॉपी देता है। यह वीडियो दोबारा एनकोड करता है, कमज़ोर मशीन पर लगभग फ़िल्म की अवधि जितना समय लेकर, इसीलिए यह तब तक बंद रहता है जब तक आप इसे चालू न करें। | कॉन्फ़िगरेशन प्रविष्टियाँ `downloads.quality_choice`, रीस्टार्ट ज़रूरी |
| `EVEY_DOWNLOAD_REDUCE_ABOVE_BYTES` | `4294967296` (4 GiB) | वह आकार जिसके ऊपर ऐप छोटी कॉपी देता है; फ़िल्म जगह में न समाए, तो फ़ोन भी इसे देता है। | कॉन्फ़िगरेशन प्रविष्टियाँ `downloads.reduce_above_bytes`, रीस्टार्ट ज़रूरी |

## प्लेयर

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_PLAYBACK_SKIP_FORWARD` | `30` | आगे वाला बटन कितने सेकंड छोड़ता है। | कोई नहीं |
| `EVEY_PLAYBACK_SKIP_BACKWARD` | `30` | पीछे वाला बटन कितने सेकंड छोड़ता है। | कोई नहीं |

<!-- sources
packaging flags: internal/adapters/driving/http/transcode_vod_wire.go (serverFlagEnabled, resolveServerFlag: hls_ondemand, remux_copy, hls_ondemand_concat, hls_ondemand_track_trim, hls_ondemand_encrypt, hls_key_rotation_segments)
renditions, fallback rung, cast wasm: internal/config/config.go - AudioRenditionEnabled, CopyFallbackRungEnabled, CastWasmAudioEnabled
downloads: cmd/evey/http.go (compat_download, quality_choice, reduce_above_bytes via resolveFlag)
config entries catalogue: frontend/apps/web/src/features/admin/operatorFlags.ts ; i18n admin.serverConfig.entriesTitle, appliedOnRestart
skip: internal/config/config.go - EVEY_PLAYBACK_SKIP_*
-->
