---
source_sha: 2ff1222cb6483b259b30b0a40ec51e6e4e8d1e7a7f4a5877a9761a0186adff2d
---

# वीडियो परोसना - HLS प्लेबैक का रास्ता

यह **पढ़ने का रास्ता** है: कोई प्लेयर वीडियो माँगता है तो क्या-क्या होता है, पहले
`master.m3u8` अनुरोध से लेकर किसी सेगमेंट की बाइट तक। यह
[ARCHITECTURE.md](ARCHITECTURE.md) में बताई गई स्कैन की शृंखला का जोड़ीदार है (वह
लिखने का रास्ता है, जो पढ़ने वाले मॉडल को तैयार करता है), और
[CODEBASE_MAP.md](CODEBASE_MAP.md) के "Golden path 4" के पीछे का गहरा ब्योरा है।

प्लेबैक का रास्ता **बेहद अहम** है और `http` अडैप्टर की क़रीब 8 फ़ाइलों में फैला
हुआ है। ऐसा कोई एक फ़ंक्शन नहीं है जो पूरा प्रवाह चलाता हो - यह दस्तावेज़ ही उसका
नक्शा है।

## सोचने का ढाँचा: 3 सवाल, 2 इंजन, 1 कुंजी

वीडियो परोसना असल में तीन सवालों का जवाब देना है, और फिर उस जवाब को पैक कर देना -
और हर चीज़ का पता एक ही हैश से चलता है:

- **सवाल 1 - क्या फ़ाइल सस्ते में पढ़ी जा सकती है?** probe किसके पास जाए, यह
  **फ़ाइल के एक्सटेंशन** से तय होता है: `.mkv/.webm` और `.mp4/.mov` **mkvgo** के
  पास जाते हैं (सिर्फ़ head पढ़कर, बिना कोई प्रक्रिया चलाए); बाकी सब कुछ, और तेज़
  रास्ते की हर नाकामी, **ffprobe** के पास जाती है (जिसका फ़ैसला अंतिम है)।
- **सवाल 2 - क्या क्लाइंट इसे जस का तस चला सकता है?** प्लेबैक का **फ़ैसला**, जो
  *हर ट्रैक के लिए अलग* लिया जाता है: वीडियो `copy` या `transcode`, ऑडियो `copy`
  या `transcode` या wasm वाला AC-3 रास्ता। वीडियो और ऑडियो का फ़ैसला **एक-दूसरे
  से स्वतंत्र** होता है - सबसे आम हाल यही रहता है कि "वीडियो की नकल कर दो, ऑडियो
  को AAC में दोबारा एन्कोड कर दो"।
- **सवाल 3 - यह होस्ट कितना एन्कोड कर सकता है?** calibrate की हुई **क्षमता**
  (शुरुआत के बेंचमार्क से) और साथ में **प्रवेश** (सीढ़ी का कौन सा पायदान समाता
  है, कोई जगह ख़ाली है या नहीं)।

इस सब में दो इंजन चलते हैं:

- **mkvgo** - बिना कोई प्रक्रिया चलाए पढ़ता है और दोबारा पैक करता है (remux / माँग पर)। **मुफ़्त।**
- **ffmpeg / ffprobe** - एक प्रक्रिया चलाकर decode करता है, दोबारा एन्कोड करता है, probe करता है। **महँगा।**

और एक कुंजी: हर चीज़ (cache डायरेक्टरी, ffmpeg सत्र, सेगमेंट के URL) का पता
**फ़ैसले की छाप** से चलता है - यानी उस फ़ैसले का हैश। वही बाइट ⇒ वही हैश ⇒ एक ही
साझा एन्कोड। पूरे तंत्र का सही चलना इसी बात पर टिका है कि प्लेयर के पास जो
manifest है और उसके बाद वह जो भी सेगमेंट लाता है, उनके बीच यह हैश **स्थिर** रहे।

## पढ़ने का क्रम - एक अनुरोध के पीछे-पीछे कोड में चलें

एक ही प्लेबैक अनुरोध को शुरू से आख़िर तक ढूँढ़ने के लिए इन्हें इसी क्रम में खोलें:

1. **प्रवेश** · `internal/adapters/driving/http/transcode_handler_vod.go` -
   `serveHTTP` पहचान जाँचता है, फिर master / variant / segment / audio में से
   किसी एक की ओर भेज देता है।
2. **सवाल 2 · फ़ैसला** · `internal/app/transcoding/decision.go` -
   `MakePlaybackDecisionWithPrefs` (शुद्ध फ़ंक्शन): हर ट्रैक के लिए copy या
   transcode तय करने वाला दिमाग़। इसे `http/transcode_vod_fingerprint.go` का
   `makeDecision` लपेटता है, जो ऊपर से अनुरोध के समय की पाबंदियाँ (`vd=` / `ad=`),
   ऑडियो को दोबारा मिलाने वाला override वगैरह चढ़ाता है।
3. **सवाल 3 · क्षमता** · `internal/app/transcoding/calibration_service.go` -
   `Current()` वापस `domain/transcoding.Derive(report, policy)` देता है: पिछले
   बेंचमार्क से निकली होस्ट की सीमाएँ।
4. **सीढ़ी गढ़ना** · `http/transcode_vod_profiles.go` - `profilesForDecision`:
   जटिलता की श्रेणी → producer के प्रोफ़ाइल → स्रोत और क्षमता की सीमाएँ →
   CMCD/av1 की सीमाएँ → प्रवेश का रास्ता और उसकी छत, और फिर **गढ़ी हुई सीढ़ी को
   प्रवेश की पाबंदी पर जमा दिया जाता है** (बाद की हर कॉल में वह ज्यों की त्यों
   पढ़ी जाती है - cache-हैश वाला नियम यहाँ ढाँचे में ही बाँध दिया गया है)।
5. **प्रवेश** · `internal/app/transcoding/admission_shaper.go`
   (अपरिवर्तनीय `StreamPlan`) और `video_session_limiter.go`: रास्ता (जिसमें hw से
   सॉफ़्टवेयर पर छलकना शामिल है) और क्षमता की जगह।
6. **छाप** · `http/transcode_vod_fingerprint.go` (`buildFingerprint`) एक
   `DecisionFingerprint` बनाता है (उसका type और `Hash()`
   `app/transcoding/segment_cache.go` में हैं)। यही हैश cache की कुंजी है।
7. **बाइट बनाना** · copy → **mkvgo** `app/transcoding/ondemand.go` (port) /
   `adapters/driven/mkvgo/ondemand.go`; transcode → **ffmpeg**
   `adapters/driven/ffmpeg/hls_transcoder.go` (`BuildFFmpegArgs` शुद्ध फ़ंक्शन है;
   सेगमेंट `/cache/transcode/segments/<hash>/` में आकर गिरते हैं)।
8. **पैक करना** · `app/transcoding/manifest_generator.go` - `master.m3u8` /
   `stream_N.m3u8` (और mkvgo वाले रास्ते पर `manifest.mpd`)।

## दोनों इंजन, ख़र्च के हिसाब से

**probe बाँटने वाला** (`adapters/driven/prober/dispatcher.go`) ज़िम्मेदारी की एक
कड़ी है, जो एक्सटेंशन के हिसाब से चलती है: वह पहले mkvgo आज़माता है, और ज़रा भी
शक होने पर ffprobe पर लौट आता है (parse की त्रुटि, panic, ऐसा नतीजा जिसे मैप न
किया जा सके, MP4 का कोई गिरा हुआ media ट्रैक, DoVi का profile 5, रंग न पढ़ा जाना)।
दोनों तेज़ रास्ते हमेशा चालू रहते हैं (`wire.go` में gate nil हैं); ffprobe
सुरक्षा जाल है, जिसे कभी छोड़ा नहीं जाता। ठीक ऐसा ही एक दूसरा बाँटने वाला
(`keyframe_dispatcher.go`) seek index सँभालता है।

| काम | mkvgo (मुफ़्त) | ffprobe / ffmpeg (महँगा) |
|---|---|---|
| मेटाडेटा probe (codec, नाप, HDR, बिटरेट, अवधि) | `.mkv/.webm/.mp4/.mov` में सिर्फ़ head | बाकी सब, या वापसी के तौर पर |
| फ़्रेम दर का तरीक़ा (cfr/vfr) | `AnalyzeFrameRateMode` | - (हट चुका) |
| MKV→MP4 remux (दोबारा एन्कोड नहीं) | `RemuxToMP4` | - |
| माँग पर HLS/DASH (copy वाला पायदान) | `OnDemandPackager` | - |
| वीडियो दोबारा एन्कोड / HDR tonemap | - | `hls_transcoder.go` |
| Interlace पहचान (idet), seek कहाँ उतरे, chromaprint, DoVi P5 | - (decode या side-data चाहिए) | ffmpeg / ffprobe |

MKV/MP4 स्रोतों पर ffprobe के तीन fork अब भी चलते हैं, जबकि वह डेटा mkvgo पहले ही
दे देता है (`trickplay.go` में अवधि, `complexity_classifier.go` में बिटरेट और नाप,
`audio_delay.go` में प्लेबैक की ऑडियो देरी) - इन्हें हटाया जा सकता है; CPU का
फ़ायदा छोटा है, काम ज़्यादातर तारें जोड़ने का है।

## ऑडियो का फ़ैसला वीडियो से अलग होता है

`makeDecision` एक-दूसरे से स्वतंत्र रूप से `VideoDecision` और `AudioDecision`
बनाता है। "copy वाला पायदान" (`isCopyRequest`) सिर्फ़ **वीडियो** पर टिका है; नक़ल
किए गए वीडियो के साथ भी आम तौर पर दोबारा एन्कोड किया गया AAC ट्रैक चलता है। ऑडियो
के फ़ैसले के तीन नतीजे हो सकते हैं: नक़ल, AAC में transcode (ffmpeg, हल्का), या -
**AC-3 / E-AC-3** के लिए, उस ब्राउज़र पर जो इसे खुद decode नहीं कर सकता - **wasm
वाला रास्ता**: master को `#EVEY-AUDIO-URI` टैग के साथ सिर्फ़ वीडियो का बना दिया
जाता है, AC-3 की **नक़ल** कर दी जाती है (योजना बन जाए तो mkvgo का `od_audio`, वरना
ffmpeg का `-c:a copy`), और `ac3go.wasm` उसे ब्राउज़र में PCM में decode कर देता है
(`WasmAudioEngine.ts`), वीडियो की घड़ी से बँधे रहते हुए। Safari नाम से नहीं, क्षमता
देखकर छाँटा जाता है: `canWasmDecodeAC3()` तभी true लौटाता है जब ब्राउज़र कहे कि वह
AC-3 खुद *नहीं* चला सकता - Safari कहता है कि वह चला सकता है।

## पैक करना: CMAF, HLS, DASH

container हमेशा **CMAF** (fMP4) रहता है: हर variant के लिए एक `init.mp4` और साथ
में `.m4s` सेगमेंट। (`hls_transcoder.go` की MPEG-TS वाली शाखा मर चुकी है -
`needsFMP4Segments` हमेशा true लौटाता है।) *उन्हीं* सेगमेंट के ऊपर:

- **HLS** (`master.m3u8` → `stream_N.m3u8` → `EXT-X-MAP` init + `.m4s`) - दोनों इंजनों पर।
- **DASH** (`manifest.mpd`) - mkvgo के copy वाले रास्ते पर packager का अपना
  manifest (`OnDemandPlan.HasDASH`, बाइट-दर-बाइट वही, बिना कुछ सहेजे, AES-128
  में बंद); ffmpeg वाले रास्ते पर एक manifest जो **plan से घोषित** होता है, उन्हीं
  fragments पर जिन्हें HLS playlists पता करती हैं (`ManifestGenerator.MPD`), और
  जो किसी भी fragment के बनने से पहले लिखा जाता है। इसे ईमानदार बनाता है
  **segment grid** (`domain/transcoding.SegmentGrid`): हर transcoded rung को frames
  की एक ही पूर्ण संख्या पर काटा जाता है (23.976 fps पर 72, 25 पर 75, 29.97 पर
  90), जिससे एक segment वीडियो ट्रैक के ticks में सटीक और पूरे switch set में एक
  जैसा रहता है; copy वाला fallback अपना keyframe plan घोषित करता है। जिस स्रोत का
  frame rate अज्ञात या बदलता हुआ हो, वह पुरानी 3 s वाली घड़ी पर रहता है, HLS
  रखता है और `manifest.mpd` पर 404 देता है। किसी client को DASH पर नहीं बदला गया:
  हर जगह HLS ही डिफ़ॉल्ट है। बने हुए fragments पर mkvgo का `DASHFromCMAF` parity
  oracle है (`internal/pkg/cmafparity`, 23.976/25/29.97 fps पर
  `TestDASHGridIntegration`: cold, seek, दो rung वाला switch set, एक AAC frame के
  भीतर अलग ऑडियो)।

हर सत्र के URL दोबारा पकाए जाते हैं (`transcode_vod_manifest_rebake.go`) ताकि वे
स्ट्रीम टोकन साथ ले जाएँ - HLS और DASH, दोनों के लिए, और बिना cookie वाले
Chromecast के लिए भी।

## जो डिज़ाइन पैटर्न यहाँ चल रहे हैं

- **Ports और adapters (hexagonal)** - ऐप की परत interface से बात करती है
  (`StreamProber`, `BenchmarkRunner`, `OnDemandPackager`, `StreamProducer`); ffmpeg
  और mkvgo उन्हें लागू करते हैं। नया इंजन यानी नया अडैप्टर।
- **Strategy** - `StreamProducer` (सॉफ़्टवेयर / हार्डवेयर / copy), जिसे एक
  `ProducerSelector` चुनता है।
- **वापसी वाली ज़िम्मेदारी की कड़ी** - probe और keyframe बाँटने वाले
  (mkvgo → निर्णायक ffprobe)।
- **सामग्री से पता पाने वाला cache** - `DecisionFingerprint` का हैश ही cache और
  ffmpeg सत्र की कुंजी है।
- **शुद्ध कोर / आदेश देने वाला खोल** - `Derive`, `MakePlaybackDecision` और लागत का
  मॉडल शुद्ध हैं, और बिना ffmpeg या डेटाबेस के यूनिट टेस्ट किए जाते हैं।
- **अपरिवर्तनीय value object** - `StreamPlan`: प्रवेश का फ़ैसला, जो एक ही बार
  निकलता है और कभी बदला नहीं जाता, ताकि URL और बाइट के बीच का नियम अपने आप बना
  रहे।
- **शालीनता से उतरना** - हर क़दम पर एक सुरक्षित वापसी: डिफ़ॉल्ट 720p क्षमता, hw से
  सॉफ़्टवेयर पर छलकना, copy से transcode, mkvgo से ffprobe।

## फ़ैसले का दिमाग़: ऐप परत का PlanResolver

अनुरोध सँभालने वाला दिमाग़ - फ़ैसले की पाबंदियाँ, सीढ़ी गढ़ना, हार्डवेयर की ओर
भेजना, छाप जोड़ना, copy पायदान की राह, producer चुनना - ऐप की परत में रहता है:
`PlanResolver` (`app/transcoding/playback_plan_resolver.go`), और उसका ठोस रूप
`PlaybackPlanResolver`
([`playback_plan.go`](../internal/app/transcoding/playback_plan.go))। हर अनुरोध
**एक ही बार** हल किया जाता है: http अडैप्टर `*http.Request` को transport से मुक्त
`PlaybackRequest` में ढाल देता है (`http/transcode_vod_plan.go`) और लौटे हुए
अपरिवर्तनीय `PlaybackPlan` में से परोसता रहता है। हर उपभोक्ता - manifest का इनपुट,
shed वाली probe, पुराने URL पर मुहर, producer चुनना, सेगमेंट की कुंजी - उसी एक हल
को पढ़ता है, इसलिए *एक अनुरोध के भीतर* cache-हैश वाला नियम अपने आप बना रहता है (और
सेगमेंट का गरम रास्ता प्रोफ़ाइल पसंद के क़रीब 5 अनुरोधों से घटकर 1 पर आ गया)। स्रोत
के तथ्य बताने वाले सहायक (codec, HDR, SAR, ten-bit…) उसी के पास
`app/transcoding/source_facts.go` में रहते हैं; अडैप्टर के पास सिर्फ़ पतले उपनाम
बचे हैं। अनुरोधों के आर-पार यह नियम अब भी URL पकाने पर ही टिका है: **फ़ैसले में
जाने वाली हर चीज़ बच्चा URL में पकी होनी चाहिए**, वरना बीच प्लेबैक में हुई कोई
पूछताछ प्लेयर के नीचे से योजना बदल सकती है (`manifest_generator.go` में
`querySuffix`)।
