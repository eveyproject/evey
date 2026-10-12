---
source_sha: 91a46caa213d6048de832a4352bd5972b5f2d393a40a66301f693b3d908f084c
reviewed: true
---
# Evey में MKVGo

**कम शब्दों में:**

- MKVGo, Evey का एक हिस्सा है जो वीडियो फ़ाइलें (MKV, WebM, MP4, MOV) पढ़ता है
  और उन्हें दोबारा पैक करता है, तस्वीर या आवाज़ को बदले बिना।
- जहाँ हो सके, **रीमक्स** वाले प्लेबैक यही चलाता है, यही उपशीर्षक निकालता है,
  और स्कैन के समय आपकी फ़ाइलें यही पढ़ता है। जिस काम में असली बदलाव चाहिए, वह
  FFmpeg के पास जाता है, जो वीडियो का एक अलग टूल है।
- आपको कुछ सेट नहीं करना होता। **एडमिन → अभी चल रहा है** हर ट्रैक के लिए
  दिखाता है कि प्लेबैक MKVGo ने चलाया या FFmpeg ने।

इस पेज का बाकी हिस्सा उस पाठक के लिए है जो जानना चाहता है कि सर्वर FFmpeg शुरू
किए बिना क्या करता है, और कहाँ FFmpeg अब भी काम संभालता है। MKVGo, Evey सर्वर
के प्रोसेस के अंदर ही चलता है।

MKVGo फ़ाइल की बनावट पर काम करता है: उसके हेडर, उसका इंडेक्स, उसके ट्रैक और
उनके पैकेट। वह कभी कोई तस्वीर या आवाज़ डिकोड या एन्कोड नहीं करता; सर्वर पर
जिस भी चीज़ को डिकोड करना हो, वह FFmpeg के पास जाती है। इसे एक अलग Go लाइब्रेरी
के रूप में बनाया जाता है:
[github.com/gravity-zero/mkvgo](https://github.com/gravity-zero/mkvgo)।

## काम के तीन स्तर

| स्तर | क्या होता है | सर्वर पर कितना बोझ |
| --- | --- | --- |
| **डायरेक्ट प्ले** | फ़ाइल जैसी है वैसी भेजी जाती है। | बस फ़ाइल पढ़ना। |
| **MKVGo** | बनावट का काम: फ़ाइल पढ़ना, स्ट्रीमिंग के लिए उसके ट्रैक दोबारा पैक करना, उसका इंडेक्स फिर से बनाना, किसी ट्रैक को टाइमलाइन पर खिसकाना। | सर्वर प्रोसेस के अंदर का कोड: हर स्ट्रीम के लिए कोई FFmpeg या ffprobe शुरू नहीं होता, और माँग पर पैकिंग के लिए डिस्क पर कोई सेगमेंट नहीं लिखा जाता। |
| **FFmpeg** | असली बदलाव: दोबारा एन्कोड करना, टोन मैपिंग, तस्वीर डिकोड करना। और जब भी MKVGo किसी फ़ाइल को न संभाल सके, तब का विकल्प (fallback) भी। | हर काम के लिए एक प्रोसेस। |

FFmpeg की कॉपी (`-c copy`) भी कुछ दोबारा एन्कोड नहीं करती और इसमें कम संसाधन लगते हैं;
MKVGo इसके ऊपर जो बचाता है, वह है हर सेशन के लिए शुरू होने वाला प्रोसेस और
ट्रांसकोडिंग कैश में लिखे जाने वाले सेगमेंट। दोबारा एन्कोड करना महँगा काम है, और
वीडियो का दोबारा एन्कोड ही अकेला काम है जो ट्रांसकोडिंग स्लॉट लेता है: देखें
[ट्रांसकोडिंग और सर्वर की क्षमता](transcoding.md)।

शुरू करना और आगे-पीछे जाना (सीक), किसी प्रोसेस का या डिस्क पर पहले सेगमेंट का
इंतज़ार नहीं करते: सेगमेंट माँगने पर फ़ाइल के इंडेक्स से बनाया जाता है। कोई फ़ाइल
यह रास्ता लेती है या नहीं, यह इस पर निर्भर है कि प्लेयर क्या डिकोड करता है:
[Evey प्लेबैक का तरीका कैसे चुनता है](how-playback-is-decided.md)।

## MKVGo क्या करता है

| काम | यह कहाँ दिखता है | जब MKVGo नहीं कर पाता |
| --- | --- | --- |
| **फ़ाइलें पढ़ना** | लाइब्रेरी स्कैन के समय, कोडेक, रिज़ॉल्यूशन, HDR, बिटरेट और अवधि फ़ाइल की शुरुआत से पढ़े जाते हैं। | ffprobe उसे पढ़ता है। किसी भी शक पर भी वही काम संभालता है: पढ़ने में कोई गड़बड़ी, या HDR दिखा सकने वाले कोडेक पर रंग की ऐसी जानकारी जिसे MKVGo पढ़ नहीं सका। |
| **कीफ़्रेम इंडेक्स** | वे जगहें जहाँ से प्लेबैक शुरू हो सकता है, फ़ाइल के इंडेक्स से पढ़ी जाती हैं; इनसे रीमक्स और वीडियो कॉपी, सेगमेंट को कीफ़्रेम पर काट पाते हैं। | ffprobe। |
| **बिना दोबारा एन्कोड किए स्ट्रीमिंग** | रीमक्स: fragmented MP4 (CMAF) में HLS सेगमेंट, जो हर फ़ाइल के लिए रखे एक प्लान से माँगने पर बनते हैं। सेगमेंट भेजते समय सीधे सोर्स फ़ाइल से कनेक्शन पर लिखा जाता है: पहले मेमोरी में कुछ जोड़ा नहीं जाता, ट्रांसकोडिंग कैश में कुछ लिखा नहीं जाता। इन्हीं सेगमेंट पर एक DASH मैनिफ़ेस्ट भी बनता है; ऐप HLS इस्तेमाल करते हैं। | FFmpeg ट्रैक कॉपी करता है, तब भी बिना दोबारा एन्कोड किए, ऐसे सेगमेंट में जो कैश में लिखे जाते हैं। |
| **ब्राउज़र के लिए MP4** | जिस Matroska फ़ाइल को ब्राउज़र नहीं खोल सकता, उसे एक बार fast-start MP4 के रूप में दोबारा लिखा जाता है और कैश में रखा जाता है। | - |
| **उपशीर्षक** | MKV के अंदर के उपशीर्षक ट्रैक माँगने पर निकाले जाते हैं, टेक्स्ट वाले ट्रैक WebVTT में और तस्वीर वाले ट्रैक (PGS) इमेज में। जहाँ फ़ाइल का अपना इंडेक्स (Cues) किसी ट्रैक को पूरा कवर करता है, वहाँ सर्वर पूरी फ़ाइल के बजाय सिर्फ़ वे ब्लॉक पढ़ता है जिनकी ओर इंडेक्स इशारा करता है, और अगली माँग के लिए ब्लॉक का यह इंडेक्स अपनी डेटा डायरेक्टरी में (`subtitle-index/`) रखता है। जिस ट्रैक को अब भी पूरा पढ़ना पड़ता है, उसका जवाब "तैयार हो रहा है" होता है, फिर कोशिश करने के इंतज़ार के समय के साथ, और प्लेयर तैयार होने तक पूछते रहते हैं। | ffmpeg ट्रैक निकालता है, पूरी फ़ाइल पढ़कर। |
| **प्रीव्यू इमेज** | माँगी गई जगह के सबसे पास वाला कीफ़्रेम इंडेक्स से ढूँढा जाता है, और FFmpeg सिर्फ़ वही फ़्रेम डिकोड करता है। यह तब इस्तेमाल होता है जब वह कीफ़्रेम माँगी गई जगह से लगभग दो सेकंड के अंदर हो। | FFmpeg ठीक उस जगह तक जाता है और वहाँ तक डिकोड करता है। |
| **एपिसोड की कड़ी** | सर्वर कई एपिसोड को एक लगातार स्ट्रीम के रूप में पैक कर सकता है (स्ट्रीम के पते पर `?gapless=1`)। अभी कोई ऐप यह विकल्प नहीं देता। | हर एपिसोड अलग चलता है। |
| **मीडिया स्वास्थ्य** | जाँच (सीक इंडेक्स, ऑडियो की शुरुआत, ब्लॉक में रखे ट्रैक), फिर से बना इंडेक्स, खराब हिस्से के बाद दोबारा सिंक, और किसी ऑडियो ट्रैक को किसी ऑफ़सेट से खिसकाना। देखें [मीडिया स्वास्थ्य](../runbooks/media-health.md)। | ब्लॉक में रखे ट्रैक का दोबारा मक्स FFmpeg करता है। |

## FFmpeg के पास क्या रहता है

- हर दोबारा एन्कोड, HDR टोन मैपिंग समेत।
- तस्वीरें डिकोड करना: सीक बार के थंबनेल, प्रीव्यू इमेज का फ़्रेम।
- ऐसा विश्लेषण जिसे डिकोड किया हुआ डेटा चाहिए: इंटरलेस की पहचान, इंट्रो की
  पहचान के लिए ऑडियो फ़िंगरप्रिंट।
- MKV/WebM और MP4/MOV के अलावा दूसरे कंटेनर, जैसे AVI या MPEG-TS।

## लोड, बदली गई फ़ाइलें और अपडेट

**एक बार में कितना पढ़ा जाता है।** एक सेगमेंट बनाने के लिए सोर्स की एक विंडो
पढ़ी जाती है, वीडियो और ऑडियो साथ में: 1080p फ़ाइल के लिए 6 से 10 MiB,
UHD रीमक्स के लिए लगभग 15 MiB। सर्वर एक साथ उतनी विंडो पढ़ता है जितने उसके
पास प्रोसेसर हैं, और कम मेमोरी वाली मशीन पर इससे कम: चल रही सारी विंडो उस
मेमोरी के एक चौथाई में आनी चाहिए जो सर्वर खुद को देता है (कंटेनर की सीमा का
आधा, या होस्ट की मेमोरी का आधा)। इससे ज़्यादा माँगें खाली स्लॉट का इंतज़ार करती
हैं, ताकि इस्तेमाल हुई मेमोरी दर्शकों की गिनती के साथ न बढ़े; छोटे होस्ट पर, अगर
बहुत से लोग एक ही पल में शुरू करें, तो उनके पहले सेगमेंट एक के बाद एक आते हैं,
बजाय इसके कि सर्वर की मेमोरी खत्म हो जाए।

**प्लेबैक के दौरान बदली गई फ़ाइल।** हर सेगमेंट के पते में उस फ़ाइल का नाम होता
है जिससे उसकी योजना बनी थी। जब डिस्क पर कोई फ़ाइल बदली जाती है (बेहतर कॉपी,
मरम्मत), तो उस पल माँगा गया सेगमेंट एक बार नई फ़ाइल से फिर बनाया जाता है, और
पुरानी फ़ाइल की प्लेलिस्ट के पते दोनों फ़ाइलों को मिलाने के बजाय `410 Gone` का
जवाब देते हैं; टाइटल फिर से खोलने पर नई फ़ाइल चलती है।

**अपडेट के बाद।** सेगमेंट के पतों में MKVGo का वर्ज़न होता है, इसलिए ब्राउज़र
या प्रॉक्सी का कैश कभी पुराने वर्ज़न से बना सेगमेंट नहीं देता, और इस रास्ते का
कोई सेगमेंट डिस्क पर नहीं रखा जाता जिसे साफ़ करना पड़े। ब्राउज़र के लिए बनी MP4
कॉपियों के नाम में उनके फ़ॉर्मेट का वर्ज़न होता है: जब वह बदलता है, तो हर कॉपी
अगली बार माँगे जाने पर फिर से बनाई जाती है।

**इस पर नज़र रखना।** `GET /metrics` पर (देखें
[निगरानी और लॉग](../operations/monitoring.md#prometheus-metrics)),
`evey_mkvgo_walks_in_flight` उन विंडो की गिनती है जो अभी पढ़ी जा रही हैं,
और `evey_mkvgo_plan_stats` पैकेजर के काउंटर को `stat` के हिसाब से जोड़ता है।
अगर `walks_in_flight` अपनी सीमा पर टिका रहे और `stat="slot_waits"` बढ़ता जाए,
तो इसका मतलब है कि दर्शक प्रोसेसर के लिए कतार में हैं; `stream_fallbacks` उन
सेगमेंट को गिनता है जिन्हें स्ट्रीम करने के बजाय मेमोरी में जोड़ना पड़ा।

## किस इंजन ने स्ट्रीम चलाई, यह देखना

**एडमिन → डैशबोर्ड** और **एडमिन → अभी चल रहा है** हर ट्रैक का इंजन बताते
हैं: `mkvgo` या `ffmpeg`। एक रीमक्स दोनों दिखा सकता है, जैसे MKVGo से पैक हुआ
वीडियो और FFmpeg से कॉपी हुआ ऑडियो।

<!-- sources
in-process, no ffmpeg, no session state, no on-disk segments: internal/adapters/driven/mkvgo/ondemand.go - OnDemandPackager
probe + fallback reasons: internal/adapters/driven/prober/dispatcher.go (container gate, ReadMeta error, panic) ; internal/adapters/driven/prober/mapper.go (ColourDetermined, VP9 feature metadata) ; docs/adr/0016-mkv-fastpath-prober.md
keyframe index: internal/adapters/driven/prober/keyframe_dispatcher.go
DASH, CMAF, HLS default: docs/playback-serving.md (Delivery) ; OnDemandPlan.HasDASH
streamed segments: internal/app/transcoding/ondemand.go - OnDemandResource (Bytes nil, WriteRange) ; internal/adapters/driving/http/transcode_ondemand_stream.go - servePlanResourceStreamed
progressive MP4: internal/adapters/driving/http/remux_handler.go - RemuxToMP4, remuxCacheName
subtitles: internal/adapters/driving/http/subtitle_handler.go - extractSubtitleVTT ; subtitle_pgs.go ; subtitle_index_cache.go - fromCues, resolveAndStore, newSubtitleIndexCache ; subtitle_preparing.go (202 + Retry-After) ; subtitle_prewarm.go - ensureIndex
preview images: internal/adapters/driving/http/frame_extractor_fastpath.go - maxSnapshotDriftMs ; cmd/evey/wire_helpers.go (transcoding.snapshot_fastpath)
episode chains: internal/adapters/driving/http/transcode_concat.go ; internal/adapters/driven/mkvgo/concat.go - ConcatPackager ; frontend/apps/web/src/hooks/usePlaybackPrefs.ts - SeriesTrackPrefs.gapless (read by HLSPlayer, no UI sets it)
media health: internal/adapters/driven/mkvgo/diagnose.go ; internal/adapters/driven/mkvgo/reindex.go ; internal/adapters/driven/mkvgo/track_retimer.go ; internal/app/maintenance/seek_index_reinterleave.go (Reinterleaver, ffmpeg adapter)
ffmpeg only: docs/playback-serving.md (mkvgo vs ffmpeg table) ; internal/app/library/trickplay_service.go
engine per track: i18n admin.sessions.track.engine (dashboard, now playing)
reads at once: internal/adapters/driven/mkvgo/budget.go - BudgetFor (one walk per CPU, walks within memLimit/4 at 15 MiB each) ; cmd/evey/wire_infra.go (applied after the memory limit) ; cmd/evey/memlimit.go - applyMemoryLimit (half the cgroup or host budget)
replaced file: internal/adapters/driving/http/transcode_ondemand.go - planSourceParam, servePlanSourceGone (410), replay once on a rebuilt plan ; internal/adapters/driving/http/transcode_vod_manifest_rebake.go - stampPlanSourceURIs
after an update: packager version in segment URLs (?v=) ; internal/adapters/driving/http/remux_handler.go - remuxCacheFormat, remuxCacheName
metrics: internal/pkg/metrics/metrics.go - MkvgoWalksInFlight, MkvgoPlanStats ; cmd/evey/wire.go - startMkvgoMetrics
plain opening: rules below ; i18n admin.sessions.track.engine
-->
