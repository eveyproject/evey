---
source_sha: b13886b3c7cb855952109b4bf5bdeae9ee280eefbef1cd5d102784c95b2a0863
reviewed: true
---
# HDR और Dolby Vision { #hdr-and-dolby-vision }

**कम शब्दों में:**

- **HDR** ऐसी तस्वीर है जिसमें चमकीले हिस्से ज़्यादा चमकदार और रंग ज़्यादा
  गहरे होते हैं, आम तस्वीर (जिसे **SDR** कहते हैं) के मुकाबले। Dolby Vision
  HDR का ही एक प्रकार है। HDR के लिए ऐसी स्क्रीन चाहिए जो उसे दिखा सके।
- HDR स्क्रीन पर, Android या Android TV ऐप के साथ (और ऐसे डिवाइस पर जो HEVC
  डिकोड करता हो), आपको असली HDR तस्वीर मिलती है, जब तक कोई और चीज़ बदलाव को
  मजबूर न करे।
- किसी भी ऐसी स्क्रीन पर जो HDR नहीं है, सर्वर तस्वीर को SDR में बदल देता है। यह
  जानबूझकर है: SDR स्क्रीन पर दिखाया गया HDR धूसर और फीका दिखता है। SDR डिस्प्ले
  पर वेब ब्राउज़र को SDR मिलता है; जो ब्राउज़र HDR डिस्प्ले बताता है और HEVC डिकोड
  करता है, वह HDR10 और HLG रखता है। Dolby Vision अब भी बदला जाता है।
- HDR को SDR में बदलना वह सबसे भारी काम है जो प्रोसेसर कर सकता है। ग्राफ़िक्स
  कार्ड मदद करता है: देखें
  [हार्डवेयर एक्सेलेरेशन](../administration/hardware-acceleration.md)।

इस पेज का बाकी हिस्सा हर स्थिति समझाता है: असली तस्वीर, ऐसा दोबारा एन्कोड जो
HDR रखता है, या SDR में बदलाव। प्लेबैक के आम नियम
[Evey प्लेबैक का तरीका कैसे चुनता है](how-playback-is-decided.md) में हैं।

## हर डिवाइस पर आपको क्या दिखेगा { #what-you-will-see-by-device }

| आपकी स्क्रीन | क्या पहुँचता है |
| --- | --- |
| Android TV या Android फ़ोन पर HDR स्क्रीन, HEVC डिकोडर के साथ | असली HDR10 या HLG, बिना बदले, जब तक कोई और चीज़ (बिटरेट, रिज़ॉल्यूशन, तस्वीर में जोड़ा गया उपशीर्षक) दोबारा एन्कोड को मजबूर न करे। |
| एक ब्राउज़र | SDR डिस्प्ले पर SDR में बदली हुई तस्वीर। जब ब्राउज़र HDR डिस्प्ले बताता है (`dynamic-range: high`) और HEVC डिकोड करता है, तो HDR10 और HLG रखे जाते हैं; Dolby Vision अब भी बदला जाता है। |
| एक कास्ट रिसीवर | वही, जो रिसीवर ने अपनी स्क्रीन के बारे में बताया: देखें [Chromecast पर कास्ट करना](casting.md)। |
| कहीं भी SDR स्क्रीन | SDR में बदली हुई तस्वीर। |

SDR स्क्रीन पर HDR को बिना बदले दिखाने से रंग फीके और धूसर आते हैं, इसलिए सर्वर
इसे कभी नहीं भेजता। Android ऐप हर प्लेबैक शुरू होने पर डिस्प्ले को जाँचते हैं,
सिर्फ़ इंस्टॉल के समय एक बार नहीं: किसी बॉक्स में HDR टेलीविज़न लगाने का असर अगले
टाइटल, ट्रैक बदलने या क्वालिटी बदलने पर होता है।

## असली तस्वीर कब रखी जाती है { #when-the-original-is-kept }

HDR10 या HLG वीडियो जैसा है वैसा तब भेजा जाता है जब प्लेयर बताए कि उसकी स्क्रीन
HDR दिखाती है और वह कोडेक डिकोड करता है, और कोई दूसरा कारण (बिटरेट,
रिज़ॉल्यूशन, कंटेनर) दोबारा एन्कोड की माँग न करे।

## जब वीडियो को फिर भी दोबारा एन्कोड करना पड़े { #when-the-video-must-be-re-encoded-anyway }

बिटरेट की सीमा, रिज़ॉल्यूशन की सीमा, या तस्वीर में जोड़ना पड़ने वाला उपशीर्षक,
किसी HDR सोर्स को दोबारा एन्कोड करने पर मजबूर कर सकता है। तब HDR रखा जाता है,
अगर ये सारी बातें सच हों:

- प्लेयर HDR स्क्रीन बताता है और HEVC डिकोड करता है;
- सर्वर में एक ग्राफ़िक्स कार्ड है जिसने अपने माप के दौरान साबित किया है कि वह
  HDR के साथ HEVC 10-बिट एन्कोड करता है;
- वह कार्ड इस स्ट्रीम के लिए उपलब्ध है।

नतीजा HEVC 10-बिट होता है, जिसमें सोर्स की HDR जानकारी (BT.2020 रंग, PQ या
HLG ट्रांसफ़र) साथ आती है। HDR10+ का डायनामिक मेटाडेटा साथ नहीं आता: इस रास्ते
पर HDR10+ वाला टाइटल HDR10 के रूप में चलता है।

जब इनमें से कोई शर्त पूरी नहीं होती, तो प्रोसेसर या कार्ड एन्कोड करने से पहले
तस्वीर को टोन-मैप करके SDR में बदल देता है। तब स्ट्रीम जानकारी में **HDR को
SDR में बदला गया (टोन-मैपिंग)** दिखता है।

अगर असली तस्वीर भेजी जा सकती थी, लेकिन सर्वर अभी उसे कॉपी के रूप में नहीं दे
सकता (जैसे, उसका सीक इंडेक्स अभी बन रहा है), तो वीडियो ऐसे कार्ड पर HDR रखते
हुए दोबारा एन्कोड होता है, और स्ट्रीम जानकारी में **कॉपी उपलब्ध नहीं, री-एन्कोड
में HDR बरकरार** दिखता है।

## जब स्क्रीन SDR हो { #when-the-screen-is-sdr }

जो प्लेयर HDR स्क्रीन नहीं बताता, उसे टोन-मैप की हुई SDR तस्वीर मिलती है। टोन
मैपिंग ग्राफ़िक्स कार्ड पर चलती है अगर उसने साबित किया हो कि वह यह कर सकता है
(जैसे, OpenCL के साथ VAAPI), नहीं तो प्रोसेसर पर: अगर ffmpeg बिल्ड में SIMD के
लिए बेहतर बनाया गया फ़िल्टर (AVX2) है तो उससे, नहीं तो एक पोर्टेबल फ़िल्टर चेन से।

!!! note
    प्रोसेसर पर टोन मैपिंग सबसे महँगा ट्रांसकोड है। सर्वर HDR के लिए
    रिज़ॉल्यूशन की एक अलग सीमा नापता है, जो आम तौर पर कम होती है, इसलिए उसी
    मशीन पर HDR टाइटल, SDR टाइटल से कम रिज़ॉल्यूशन पर बदला जा सकता है। देखें
    [ट्रांसकोडिंग](transcoding.md#the-server-measures-itself)।

## Dolby Vision { #dolby-vision }

प्लेयर की रिपोर्ट बताती है कि उसकी स्क्रीन HDR दिखाती है या नहीं, यह नहीं कि वह
Dolby Vision सपोर्ट करती है या नहीं, और Evey कभी HDR सपोर्ट से Dolby Vision
सपोर्ट का अंदाज़ा नहीं लगाता: जो स्क्रीन सिर्फ़ HDR10 संभालती है, उसे Dolby
Vision भेजने से रंग गलत आते हैं। स्ट्रीम सिर्फ़ तभी कॉपी होती है जब उसमें ऐसी
बेस लेयर हो जो किसी Dolby Vision डिकोडिंग के बिना सही दिखे:

| Dolby Vision प्रोफ़ाइल | HDR प्लेयर | SDR प्लेयर |
| --- | --- | --- |
| **8.1** (HDR10 के साथ चलने वाली बेस लेयर) | HDR10 की तरह माना जाता है: ऊपर के नियम लागू होते हैं, असली तस्वीर रखी जा सकती है। | टोन-मैप करके SDR में। |
| **7** (UHD Blu-ray) | HDR10 बेस लेयर को HEVC 10-बिट में HDR रखते हुए दोबारा एन्कोड किया जाता है। स्ट्रीम जानकारी: **Dolby Vision बेस लेयर पुनः एन्कोड (HDR बरकरार)**। | टोन-मैप करके SDR में। |
| **5** (कोई HDR10 बेस लेयर नहीं) | टोन-मैप करके SDR में। | टोन-मैप करके SDR में। |

## क्या हुआ, यह जाँचना { #checking-what-happened }

**स्ट्रीम जानकारी** वीडियो का फ़ैसला दिखाती है और, कारणों में, टोन मैपिंग या रखा
गया HDR। टोन-मैप करने वाला हर रास्ता इसे वहाँ दिखाता है, एक अपवाद के साथ: जब
स्ट्रीम शुरू होते समय ग्राफ़िक्स कार्ड पूरी तरह व्यस्त हो, तो स्ट्रीम जानकारी तैयार होने के
बाद स्ट्रीम प्रोसेसर पर चली जाती है। **एडमिन → अभी चल रहा है** असल में
इस्तेमाल हुआ बैकएंड दिखाता है।

<!-- sources
decision: internal/app/transcoding/decision.go - evaluateVideo (hdr_tonemapping_needed, dovi_base_layer, doViBaseLayerIsHDR10)
re-encode with HDR kept, copy unavailable: internal/app/transcoding/playback_plan_resolver.go - Decide (hdr_copy_unavailable) ; internal/app/transcoding/resolver_route.go - naturalRoute, passthroughWanted, hwTonemapWanted
hdr signalling carried: internal/adapters/driven/ffmpeg/hls_args_hw.go - hwHDRPassthroughArgs (no HDR10+ metadata)
dovi copy rule: internal/app/transcoding/copy_eligibility.go (dovi_profile, DoViBLCompat)
who declares an HDR screen: frontend/packages/client/src/native/displayHdr.ts - probeDisplayHdr ; frontend/packages/client/src/native/playSession.ts (supportsHdr) ; frontend/apps/web/src/cast-receiver/receiverLogic.ts (hdr=1) ; frontend/packages/client/src/web/codec.ts - canDisplayHdr (dynamic-range: high)
tonemapx, software HDR ceiling: internal/adapters/driven/ffmpeg/tonemapx.go ; internal/pkg/hdr/hdr.go ; docs/adr/0017-software-hdr-tonemap-delivery-resolution.md
tests: internal/app/transcoding/route_golden_test.go ; internal/app/transcoding/route_tonemap_report_test.go
labels: i18n player.streamInfo.reason.* ; admin.nowPlaying
plain opening: rules below ; tone mapping cost: internal/app/transcoding/calibration_service.go (separate HDR ceiling)
-->
