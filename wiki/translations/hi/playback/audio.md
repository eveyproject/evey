---
source_sha: c8146d533df94a1909ae91a9bbceb788ab4d86f1860b721414c4dadf8791cff9
reviewed: true
---
# ऑडियो

**कम शब्दों में:**

- अगर फ़िल्म में आपकी प्रोफ़ाइल की भाषा है, तो आप वही सुनते हैं: इसे
  **सेटिंग्स → प्लेबैक → डिफ़ॉल्ट ऑडियो भाषा** में चुनें। प्लेयर में आप जो
  ट्रैक खुद चुनते हैं, वही हमेशा चलता है।
- अगर आपका डिवाइस आवाज़ को चला सकता है, तो वह जैसी है वैसी भेजी जाती है। नहीं
  तो सर्वर उसे एक आम फ़ॉर्मेट (AAC) में बदल देता है। अक्सर सिर्फ़ आवाज़ ही बदली
  जाती है, और तस्वीर जैसी है वैसी रहती है।
- फ़ोन के स्पीकर, हेडफ़ोन या Bluetooth पर, Android ऐप स्टीरियो चलाता है। HDMI
  पर Android या Android TV ऐप के साथ, आपको उतने चैनल मिलते हैं जितने TV या
  एम्प्लिफ़ायर लेता है (जैसे 5.1)।
- आवाज़ तस्वीर से आगे-पीछे है? देखें
  [आवाज़ तस्वीर से मेल नहीं खाती](#sound-out-of-sync)।

इस पेज का बाकी हिस्सा बताता है कि कौन सा ट्रैक चलता है, वह कब कॉपी होता है या
बदला जाता है, और कितने चैनलों के साथ। यहाँ बताई गई प्रोफ़ाइल सेटिंग्स
**सेटिंग्स → प्लेबैक** में हैं।

## हर डिवाइस पर आप क्या सुनेंगे

| आपका डिवाइस | क्या पहुँचता है |
| --- | --- |
| एक ब्राउज़र | ट्रैक जैसा है वैसा, जब ब्राउज़र उसे डिकोड करता हो; AC-3 और E-AC-3 पेज के अंदर डिकोड होते हैं (नीचे देखें); नहीं तो AAC। सराउंड AAC ट्रैक को ब्राउज़र आपके आउटपुट के हिसाब से कम चैनलों में मिला देता है। |
| अपने स्पीकर, हेडफ़ोन या Bluetooth पर एक Android फ़ोन | स्टीरियो। ऐप स्टीरियो आउटपुट बताता है, इसलिए सर्वर सराउंड ट्रैक को स्टीरियो में दोबारा एन्कोड करता है, आवाज़ का स्तर संतुलित रखते हुए। जिस Dolby Digital को ऐप खुद डिकोड करता है, वह जैसा है वैसा भेजा जाता है और ऐप उसे स्टीरियो में मिलाता है। |
| HDMI पर एक Android फ़ोन या Android TV | उतने चैनल जितने HDMI से जुड़ा डिवाइस लेने की बात बताता है: 5.1 एम्प्लिफ़ायर पर 5.1 ट्रैक 5.1 ही रहता है। |
| एक कास्ट रिसीवर | वही, जो रिसीवर ने अपने आउटपुट के बारे में बताया: देखें [Chromecast पर कास्ट करना](casting.md)। |

Android ऐप हर प्लेबैक शुरू होने पर आउटपुट जाँचते हैं: हेडफ़ोन लगाने का असर अगले
टाइटल, ट्रैक बदलने या क्वालिटी बदलने पर होता है, किसी सीन के बीच में नहीं।

## कौन सा ट्रैक चलता है

टाइटल के पेज पर या प्लेयर में खुद चुना गया ट्रैक हमेशा चलता है। नहीं तो प्लेयर
प्रोफ़ाइल की **डिफ़ॉल्ट ऑडियो भाषा** मानते हैं, फिर वह ट्रैक जिसे फ़ाइल डिफ़ॉल्ट
बताती है, फिर पहला ट्रैक। अगर फ़ाइल में कमेंट्री वाला ट्रैक और आम ट्रैक दोनों
हैं, तो कमेंट्री वाले को छोड़कर आम ट्रैक लिया जाता है।

सिर्फ़ उसी ट्रैक को जाँचा जाता है जो चल रहा है: जिस DTS ट्रैक को आप नहीं सुन
रहे, उसकी वजह से कभी ट्रांसकोड नहीं होता।

## कॉपी या दोबारा एन्कोड

ट्रैक तब कॉपी होता है जब प्लेयर उसका कोडेक डिकोड करता हो और उसके आउटपुट में उसके
लिए काफ़ी चैनल हों, प्रोफ़ाइल के **अधिकतम ऑडियो चैनल** के अंदर (**स्टीरियो
(2.0)**, **सराउंड (5.1)** या **सराउंड (7.1)**)। नहीं तो उसे AAC में दोबारा
एन्कोड किया जाता है:

- उतने चैनलों के साथ जितने सोर्स, डिवाइस और प्रोफ़ाइल अनुमति दें, 5.1 तक
  (7.1 सोर्स को 5.1 में मिलाया जाता है, जिसे हर प्लेयर डिकोड करता है; स्टीरियो
  सोर्स स्टीरियो ही रहते हैं);
- सैंपल रेट चाहे जो हो, अकेले वह कभी दोबारा एन्कोड की वजह नहीं होता।

ट्रांसकोड या रीमक्स हुई स्ट्रीम के अंदर कॉपी हुआ ट्रैक ऐसे फ़ॉर्मेट में भी होना
चाहिए जिसे प्लेयर स्ट्रीम के सेगमेंट के अंदर स्वीकार करे। AAC, AC-3 और E-AC-3
हमेशा स्वीकार होते हैं; दूसरे कोडेक तभी, जब प्लेयर ऐसा बताए। जब ऐसा नहीं होता,
तो ट्रैक दोबारा एन्कोड होता है और स्ट्रीम जानकारी कहती है **ऑडियो ट्रैक स्ट्रीम
में बिना बदले नहीं भेजा जा सकता**: कोई प्लेयर किसी फ़ाइल में MP3 डिकोड कर सकता
है, फिर भी स्ट्रीम के अंदर उसे मना कर सकता है।

## ब्राउज़र में Dolby Digital

ज़्यादातर ब्राउज़र AC-3 या E-AC-3 डिकोड नहीं कर सकते। सर्वर पर ट्रैक को दोबारा
एन्कोड करने के बजाय, वेब प्लेयर उसे खुद डिकोड करता है, एक ऐसे डिकोडर से जो पेज के
अंदर चलता है: ट्रैक बिना बदले भेजा जाता है और स्ट्रीम जानकारी उसे *क्लाइंट पर
डिकोड किया गया* दिखाती है। जो ब्राउज़र Dolby Digital खुद डिकोड करते हैं, जैसे
Safari, उन्हें यह एक आम ट्रैक की तरह मिलता है; यह चुनाव ब्राउज़र को जाँचकर किया
जाता है, उसे पहचानकर नहीं। सराउंड ट्रैक पेज में तभी डिकोड होता है जब प्रोफ़ाइल का
**अधिकतम ऑडियो चैनल** इसकी अनुमति दे: स्टीरियो पर सेट प्रोफ़ाइल को इसके बजाय AAC
में दोबारा एन्कोड मिलता है, जो चैनलों को मिला सकता है।

पेज इसे कैसे डिकोड करता है, और जब नहीं कर पाता तब क्या होता है:
[Evey में ac3go](ac3go.md)।

!!! warning
    जो रिवर्स प्रॉक्सी अपनी Content-Security-Policy लगाता है, उसे `script-src`
    में `'wasm-unsafe-eval'` की अनुमति देनी होगी, नहीं तो डिकोडर शुरू नहीं हो
    पाता और फ़िल्म बिना आवाज़ के चलती है। देखें
    [रिवर्स प्रॉक्सी कॉन्फ़िगरेशन](../reverse-proxy.md#security-headers--csp---read-this-if-your-proxy-sets-its-own)।

## आवाज़ तस्वीर से मेल नहीं खाती

प्लेयर में **ऑडियो सिंक** मौजूदा टाइटल के लिए आवाज़ को आगे-पीछे खिसकाता है
(धनात्मक मान आवाज़ को बाद में चलाते हैं)। यह तब मिलता है जब सर्वर से स्ट्रीम
रीमक्स या ट्रांसकोड के रूप में आ रही हो; सीधे प्लेबैक में यह कंट्रोल **केवल
ट्रांसकोडेड प्लेबैक में उपलब्ध** दिखाता है। खिसकाव लागू करने पर स्ट्रीम मौजूदा
जगह से फिर शुरू होती है। कॉपी हुए ट्रैक को खिसकाया नहीं जा सकता, इसलिए सर्वर उसे
दोबारा एन्कोड करता है और स्ट्रीम जानकारी **ऑडियो पुनः-सिंक का अनुरोध किया गया**
दिखाती है।

जब यह अंतर फ़ाइल के अंदर ही हो (कोई ऑडियो ट्रैक जो तस्वीर के बाद शुरू होता है,
या ऐसे ट्रैक जो आपस में मेल नहीं खाते), तो **एडमिन → मीडिया स्वास्थ्य** इसकी
रिपोर्ट करता है, लेकिन अपने-आप उसे कभी दोबारा नहीं लिखता: देर से शुरू होने वाला
अंतर आम तौर पर डबिंग का जानबूझकर किया गया मिलान होता है। फ़ाइल की लाइन पर **सिंक
जाँचें** से आप इसे आँख से परख सकते हैं और खिसकाव लागू कर सकते हैं, जिसे उसका
इतिहास वापस ले सकता है। देखें
[मीडिया स्वास्थ्य](../runbooks/media-health.md#checking-the-sync-by-eye)।

<!-- sources
track choice: internal/app/transcoding/decision.go - selectedAudioSpec ; frontend/packages/client/src/hooks/usePlayerTrackSelection.ts
copy vs aac, channels: internal/app/transcoding/decision.go - evaluateAudio, applyAudioChannelTarget (aacMaxTargetChannels)
device channel ceiling: frontend/packages/client/src/native/outputChannels.ts - resolveMaxOutputChannels (STEREO_ONLY_TYPES, hdmi.maxChannelCount) ; frontend/packages/client/src/native/playSession.ts - playerDecodesCopiedAudio ; internal/adapters/driving/http/client_capabilities.go - maxAudioChannelsQuery
passthrough in segments: internal/app/transcoding/decision.go - applyPackagedAudioPassthrough ; internal/app/transcoding/source_facts.go - AudioCopyCodecAllowedFor ; internal/domain/transcoding/playback_decision.go - ClientProfile.FMP4AudioPassthrough
wasm: internal/app/transcoding/wasm_audio.go - WasmAudioCodecSupported, WasmAudioChannelsOK ; internal/app/transcoding/decision.go - wasmAudioApplies ; frontend/packages/client/src/web/codec.ts
sample rate: internal/app/transcoding/decision.go - evaluateAudio (no sample-rate check)
resync: internal/app/transcoding/playback_plan_resolver.go - Decide (audio_resync_requested) ; frontend/apps/web/src/features/playback/HLSPlayer.tsx (audioOffsetAvailable=transcodeMode) ; frontend/apps/mobile/app/(auth)/player/[id].tsx (audioOffsetAvailable)
media health: i18n admin.syncPreview.btn ; docs/runbooks/media-health.md
labels: i18n auth.settings.playback.maxAudioChannels, channelsStereo, channels51, channels71, audioLanguage ; player.settings.audioSync, audioSyncHint, audioSyncUnavailable ; player.streamInfo.decisionWasm, reason.audio_passthrough_unsupported, reason.audio_resync_requested
plain opening: frontend/packages/client/src/hooks/usePlayerTrackSelection.ts ; i18n auth.settings.playback.audioLanguage
-->
