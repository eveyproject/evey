---
source_sha: ed64eaf04c09d740775060639c8ba122fb90d8ee17950d66a387a572c3fea16d
reviewed: true
---
# लाइब्रेरी और मेटाडेटा

**लाइब्रेरी** एक तरह का मीडिया है (फ़िल्में, सीरीज़, संगीत या फ़ोटो), जो एक
या कई फ़ोल्डर में रखा होता है। Evey इन फ़ोल्डर को पढ़ता है, हर फ़ाइल को उसके
नाम से पहचानता है, और इंटरनेट से उसकी जानकारी लाता है: टाइटल, कहानी, पोस्टर,
कलाकार, रेटिंग। इस जानकारी को **मेटाडेटा** कहते हैं। फ़िल्मों और सीरीज़ के
लिए यह The Movie Database (TMDB) से आता है, जो फ़िल्मों का एक मुफ़्त ऑनलाइन
डेटाबेस है।

इस पेज पर पहले वे तीन काम हैं जो आप सबसे पहले करेंगे: लाइब्रेरी जोड़ना,
फ़ाइलों के नाम रखना, और गलत पहचान ठीक करना। उसके बाद आता है कि स्कैन कैसे
काम करता है, संगीत और फ़ोटो, संग्रह, और मेटाडेटा कहाँ से आता है।

<figure class="evey-shot" markdown="span">
  ![डेमो सर्वर पर एडमिन → लाइब्रेरी](../images/admin-libraries.webp){ loading=lazy }
  <figcaption><strong>एडमिन → लाइब्रेरी</strong>: हर लाइब्रेरी की एक लाइन, उसके फ़ोल्डर, उसमें कितने आइटम हैं, और आखिरी स्कैन (स्क्रीन अंग्रेज़ी में)।</figcaption>
</figure>

## लाइब्रेरी जोड़ें

1. **एडमिन → लाइब्रेरी → लाइब्रेरी जोड़ें**।
2. उसे एक **नाम** और एक **प्रकार** दें। फ़िल्मों और सीरीज़ को अलग-अलग
   लाइब्रेरी में रखें: प्रकार से स्कैनर को पता चलता है कि उसे क्या ढूँढना है।
3. **पथ** में सूची से एक फ़ोल्डर चुनें, या खुद टाइप करने के लिए **कस्टम पथ
   दर्ज करें...** चुनें। **+ पथ जोड़ें** से उसी लाइब्रेरी में दूसरा फ़ोल्डर
   जुड़ता है।
4. **लाइब्रेरी बनाएं** दबाएँ। Evey एक-दो मिनट में फ़ोल्डर पढ़ लेता है। तुरंत
   शुरू करने के लिए लाइब्रेरी की लाइन पर **स्कैन** दबाएँ। पोस्टर और कहानियाँ
   पीछे-पीछे आती रहती हैं, इसलिए बड़ी लाइब्रेरी कुछ मिनटों में धीरे-धीरे भरती
   है।

| प्रकार | किस रूप में पहचाना जाता है | मेटाडेटा कहाँ से |
| --- | --- | --- |
| **फ़िल्में** | हर फ़ाइल (या हर फ़ोल्डर) में एक फ़िल्म | TMDB, और कुंजी सेट हो तो OMDb से पूरा किया जाता है |
| **टीवी शो** | सीरीज़, सीज़न, एपिसोड | TMDB |
| **संगीत** | कलाकार, एल्बम, ट्रैक | फ़ाइलों के टैग; कवर फ़ाइलों से, एल्बम के फ़ोल्डर से, Cover Art Archive (MusicBrainz) से, फिर iTunes से |
| **फ़ोटो** | एल्बम, और फ़ोटो खींचने की जानकारी के साथ तस्वीरें | फ़ाइलें |

!!! note "फ़ोल्डर के नाम वैसे ही दिखते हैं जैसे Evey उन्हें देखता है"
    फ़ोल्डर की सूची में फ़ोल्डर वैसे दिखते हैं जैसे Evey उन्हें अपने कंटेनर
    के अंदर देखता है। [इंस्टॉल](../installation.md) वाली compose फ़ाइल के साथ
    आपका मीडिया फ़ोल्डर `/media` है। तो आपके कंप्यूटर का फ़ोल्डर
    `/srv/media/films` Evey में `/media/films` है। दूसरा मीडिया फ़ोल्डर
    (`/media2`) भी इसी तरह काम करता है।

## फ़ाइलों के नाम ऐसे रखें कि वे पहचानी जाएँ

स्कैनर इस तरह के नाम पहचानता है:

```text
/media/films/The Mirror (1999)/The Mirror (1999) 1080p.mkv
/media/films/Lantern.2010.1080p.BluRay.x264.mkv
/media/series/The Outpost/Season 01/The.Outpost.S01E05.720p.mkv
/media/series/Quiet Storm/Quiet.Storm.03x10.mkv
```

- **फ़िल्म के लिए** सबसे ज़रूरी है साल: `The Mirror (1999)`।
- **एपिसोड के लिए** सीज़न और एपिसोड का नंबर: `S01E05` या `1x05`।
- नाम में बाकी शब्दों पर ध्यान नहीं दिया जाता: रिज़ॉल्यूशन, सोर्स, कोडेक,
  ऑडियो, भाषा वाले टैग जैसे `VOSTFR` या `MULTI`, और एडिशन जैसे "Director's
  Cut"।

अगर आपकी फ़ाइलों में कोई ऐसा शब्द है जिसे Evey अभी अनदेखा नहीं करता, तो उसे
**एडमिन → सर्वर कॉन्फ़िग → पार्सर टोकन** में जोड़ें: एक शब्द (`HDRIP`) के लिए
**अतिरिक्त क्वालिटी टोकन**, और कई शब्दों वाले वाक्यांश के लिए **अतिरिक्त
एडिशन सफ़िक्स**। यह अगले स्कैन से लागू होता है।

## गलत या गायब टाइटल ठीक करें

किसी फ़िल्म पर उसी नाम की दूसरी फ़िल्म का पोस्टर दिख रहा है? सबसे जल्दी का
तरीका:

1. टाइटल खोलें और **मेटाडेटा संपादित करें** चुनें।
2. **TMDB पर पहचानें** दबाएँ, और सही टाइटल खोजें।
3. **यह मेल उपयोग करें** दबाएँ।

**मेटाडेटा संपादित करें** में और विकल्प:

- **कोई फ़ील्ड हाथ से बदलें।** जो मान आप बदलते हैं वह बना रहता है: बाद के
  रिफ़्रेश उसे नहीं बदलते। **TMDB ID** या **IMDB ID** बदलने पर, सेव करते समय
  उस ID की जानकारी लाई जाती है।
- **मेटाडेटा रिफ़्रेश करें** मौजूदा पहचान के लिए जानकारी फिर से लाता है।
- **मेटाडेटा रीसेट करें** और **मेटाडेटा फिर से मिलाएँ** फ़ाइल के नाम से
  फिर से शुरू करते हैं और आपके बदलाव हटा देते हैं। रीसेट तस्वीरें और कलाकार
  भी हटाता है, और सीरीज़ के सीज़न और एपिसोड भी।

पूरी लाइब्रेरी के लिए, **एडमिन → लाइब्रेरी** में उसकी लाइन पर ये विकल्प हैं:

- **केवल बिना पहचाने आइटम मिलाएँ** उन फ़ाइलों के लिए फिर से कोशिश करता है जो कभी
  पहचानी नहीं गईं। यह तेज़ है।
- **TMDB से सभी फिर से मिलाएँ** हर टाइटल को फिर से खोजता है। यह धीमा है।
  बहुत सारी फ़ाइलों के नाम बदलने के बाद इसका इस्तेमाल करें।
- **सागा ठीक करें** उन फ़िल्मों को जो किसी संग्रह में नहीं हैं, उनकी सागा
  से फिर जोड़ता है, जब उस सागा की बाकी फ़िल्में पहले से लाइब्रेरी में हों।

## नई फ़ाइलें कैसे पकड़ी जाती हैं

आपको कुछ नहीं करना है: नई, नाम बदली हुई या हटाई गई फ़ाइल कुछ ही मिनटों में
Evey में दिख जाती है।

इसे तुरंत करवाने के लिए, या नियमित रूप से पूरा स्कैन जोड़ने के लिए,
लाइब्रेरी की लाइन का इस्तेमाल करें:

| बटन | यह क्या करता है |
| --- | --- |
| **स्कैन** | फ़ोल्डर अभी पढ़ता है। लाइन पर **स्कैन हो रहा है...** दिखता है, फिर नतीजा। जो फ़ोल्डर पढ़ा नहीं जा सका, उसका नाम दिखता है। |
| **शेड्यूल** | पूरा स्कैन **हर 6 घंटे**, **हर 12 घंटे**, **दैनिक (सुबह 3 बजे)**, **साप्ताहिक (रविवार सुबह 3 बजे)**, या **क्रॉन एक्सप्रेशन** के हिसाब से। |

??? info "सर्वर फ़ोल्डर कैसे जाँचता है"

    - सर्वर हर बार शुरू होने पर हर लाइब्रेरी पढ़ता है, फिर हर 2 मिनट में
      (**एडमिन → सर्वर कॉन्फ़िग → सर्वर सेटिंग्स → आवधिक स्कैन अंतराल**, जो
      रीस्टार्ट पर लागू होता है)। जब कुछ चल रहा हो, तो वह दो बार पढ़ने के बीच
      15 मिनट रुकता है, ताकि प्लेबैक धीमा न हो।
    - फ़ाइल बदलने पर सिस्टम भी सर्वर को बता सकता है (**एडमिन → सर्वर
      कॉन्फ़िग → सर्वर सेटिंग्स → लाइब्रेरी फ़ाइल मॉनिटर**), उन लाइब्रेरी के
      लिए जिनकी रियलटाइम निगरानी चालू है। यह अब भी काम कर रहा है, यह जाँचने के
      लिए वह कुछ सेकंड के लिए फ़ोल्डर में एक छोटी टेस्ट फ़ाइल लिखता है। जिस
      लाइब्रेरी में यह काम करता साबित हो जाए, उसे हर 2 मिनट की जगह घंटे में
      एक बार पढ़ा जाता है।
    - पैनल से बनाई गई लाइब्रेरी फ़ाइल मॉनिटर का इस्तेमाल नहीं करतीं: उनकी
      फ़ाइलें हर 2 मिनट वाली पढ़ाई में पकड़ी जाती हैं।

!!! note "नेटवर्क शेयर"
    NFS या SMB शेयर पर, किसी दूसरी मशीन से लिखी गई फ़ाइल सर्वर को कोई संकेत
    नहीं भेजती। नियमित पढ़ाई में वह मिल जाती है।

## संगीत लाइब्रेरी

स्कैनर MP3, FLAC, M4A, OGG, Opus, WAV और AAC फ़ाइलें पढ़ता है, और हर ट्रैक की
जानकारी उसके टैग से लेता है: टाइटल (न हो तो फ़ाइल का नाम), कलाकार, एल्बम
कलाकार, एल्बम, साल, ट्रैक और डिस्क नंबर, शैली। कलाकारों को एल्बम कलाकार के
हिसाब से जोड़ा जाता है, और वह टैग खाली हो तो कलाकार के हिसाब से।

**एक एल्बम एक फ़ोल्डर है।** हर एल्बम को एक ही फ़ोल्डर में रखें। Evey ट्रैक को
उनके फ़ोल्डर के हिसाब से जोड़ता है, उनके एल्बम टैग के हिसाब से नहीं। इसलिए एक
ही एल्बम टैग वाले दो फ़ोल्डर दो एल्बम बनाते हैं, और `CD1` और `CD2` सबफ़ोल्डर
में बँटा एल्बम दो एल्बम के रूप में दिखता है। एल्बम के अंदर ट्रैक पहले डिस्क
नंबर, फिर ट्रैक नंबर के क्रम में होते हैं, और एल्बम का नाम वही होता है जो
उसके ज़्यादातर ट्रैक पर लिखा है।

कवर फ़ाइलों के अंदर लगी तस्वीर होती है; वह न हो तो एल्बम के फ़ोल्डर में रखी
तस्वीर (`cover.jpg`, `folder.jpg` या `front.jpg`); वह भी न हो तो Cover Art
Archive (MusicBrainz के ज़रिए मिली); और आखिर में iTunes।

## फ़ोटो लाइब्रेरी

स्कैनर JPEG, PNG, HEIC, WebP, GIF, TIFF और BMP फ़ाइलें पढ़ता है। फ़ोटो का
एल्बम उसका फ़ोल्डर होता है, लाइब्रेरी के फ़ोल्डर के हिसाब से। सबसे ऊपर वाले
स्तर पर रखी फ़ोटो का कोई एल्बम नहीं होता, और किसी फ़ाइल को दूसरे फ़ोल्डर में
ले जाने पर अगले स्कैन में वह उस एल्बम में चली जाती है। फ़ोटो खींचने की
तारीख और कैमरे का मॉडल JPEG फ़ाइलों के EXIF डेटा से आते हैं; बाकी फ़ॉर्मेट
इनके बिना दिखते हैं। थंबनेल सर्वर पर छोटे किए जाते हैं और ट्रांसकोडिंग कैश
में रखे जाते हैं।

## संग्रह

**संग्रह** कई टाइटल को एक नाम के नीचे इकट्ठा करता है, हर खाते के लिए। ये तीन
तरह के होते हैं:

- **TMDB संग्रह**: सागा, जो अपने-आप भरते हैं जब TMDB किसी फ़िल्म को किसी सागा
  में रखता है। **सागा ठीक करें** (ऊपर देखें) उन फ़िल्मों को पकड़ता है जो अपनी
  सागा से छूट गईं।
- **नया संग्रह**: एक सूची जिसे एडमिन हाथ से भरता है, किसी टाइटल या चुने हुए
  टाइटल पर **संग्रह में जोड़ें** से, और उसका क्रम तय करता है।
- **नया स्मार्ट संग्रह**: **शैली**, **वर्ष**, **प्रकार**, **रेटिंग ≥** और
  **लाइब्रेरी** पर नियम, जिन सब पर किसी टाइटल को खरा उतरना होता है। इसकी
  सामग्री हर बार खोलने पर नए सिरे से बनती है, इसलिए नए टाइटल बिना कुछ किए
  दिख जाते हैं।

सिर्फ़ एडमिन संग्रह बनाते और बदलते हैं। उन्हें कोई भी देख सकता है, और वेब पर
**संग्रह डाउनलोड करें** मूल फ़ाइलों को एक ZIP में देता है (1000 टाइटल तक)।
जो चीज़ें खाते की लाइब्रेरी एक्सेस या उम्र की सीमा से छिपी हैं, वे इसमें नहीं
होतीं।

## मेटाडेटा कुंजियाँ

Evey के साथ एक TMDB कुंजी आती है, इसलिए मेटाडेटा बिना किसी सेटअप के काम
करता है। बाकी सोर्स ज़रूरी नहीं हैं। उन्हें इस्तेमाल करने के लिए आप उस सेवा
से एक कुंजी (एक तरह का पासवर्ड) लेते हैं और उसे **एडमिन → सर्वर कॉन्फ़िग →
API कुंजियाँ** में पेस्ट करते हैं। यहाँ सेट की गई कुंजी, environment में दी
गई उसी कुंजी से ऊपर मानी जाती है (देखें
[Environment: लाइब्रेरी और मेटाडेटा](../environment/libraries.md))।

| कुंजी | क्या जोड़ती है |
| --- | --- |
| TMDB (आपकी अपनी) | साझा कोटे की जगह अनुरोधों का आपका अपना कोटा। |
| OMDb | उन फ़िल्मों के लिए दूसरा सोर्स जिनके बारे में TMDB कम जानता है। |
| OpenSubtitles | जो सबटाइटल फ़ाइल में नहीं हैं, उन्हें सर्वर API से खोजना और डाउनलोड करना (ऐप अभी यह नहीं देते)। देखें [सबटाइटल](../playback/subtitles.md)। |
| TheIntroDB | एक कम्युनिटी डेटाबेस से इंट्रो, रीकैप और क्रेडिट्स का समय: कुंजी के साथ दिन में 500 खोज, बिना कुंजी 100। |

## इंट्रो, रीकैप और क्रेडिट्स

Evey एपिसोड और फ़िल्मों का इंट्रो, रीकैप और क्रेडिट्स ढूँढता है, ताकि प्लेयर
उन्हें छोड़ने का विकल्प दे सकें। जब कम्युनिटी डेटाबेस (पहले TheIntroDB, फिर
IntroDB) उस टाइटल को जानते हैं, तो समय वहीं से आता है। नहीं तो सर्वर फ़ाइलों
का विश्लेषण करके खुद समय निकालता है। एडमिन किसी टाइटल के लिए समय हाथ से ठीक
कर सकता है, या पहचान फिर से चला सकता है।

प्लेयर डिफ़ॉल्ट रूप से **इंट्रो छोड़ें**, **रीकैप छोड़ें** और **क्रेडिट्स
छोड़ें** बटन दिखाते हैं। **इंट्रो स्वतः छोड़ें** (**एडमिन → सर्वर कॉन्फ़िग →
सर्वर सेटिंग्स**, डिफ़ॉल्ट रूप से बंद) से हर प्लेयर उन्हें अपने-आप छोड़ देता
है।

<!-- sources
add library: frontend/apps/web/src/features/admin/LibraryForm.tsx (name, type, paths only - no realtime/scan-on-startup toggles) ; i18n admin.libraries.addLibrary, tvShows, paths, enterCustomPath, addPath, createLibrary ; internal/domain/library/model.go NewLibrary (RealtimeMonitoring false by default) ; grpcapi/library_handler.go CreateLibrary (no scan trigger ; only watcher + feed subscribe library.created)
types: i18n admin.libraries.types.* ; internal/adapters/driven/metadata/musicbrainz_provider.go ; internal/app/music/album_art_chain.go
parser: internal/adapters/driven/filesystem/parser.go (reEditionSuffix, knownQualities, SetExtraParserTokens) ; internal/app/library/parser_config.go ApplyParserConfig ; i18n admin.serverConfig.parserTokens*
fix a title: frontend/apps/web/src/features/library/MediaDetailEdit.tsx (save, refresh, reset, rematch, identify) ; IdentifyMetadataModal.tsx ; http/admin_handler_media.go (identify search) ; internal/app/metadata/command_service_overrides.go SetOverride, ResetItemMetadata (overrides, artwork, persons, Catalog=nil, children) ; command_service_refresh.go RefreshItemMetadata ; i18n media.detail.editMetadata, identify.*, tmdbId, imdbId, idChangedWarning, refreshMetadata, resetMetadata, rematchMetadata, rematchConfirm
per library: i18n admin.libraries.matchOrphans, refreshMetadata, relinkCollections (+Tooltip) ; internal/app/metadata/relink_collections.go
scans: internal/app/library/command_service.go StartPeriodicScan (startup pass, ticker, runScanPass) ; scan_service.go playbackScanBackoff 15m, watchedScanInterval 1h, shouldScanNow ; cmd/evey/wire_events.go (library.periodic_scan_interval, all libraries) ; internal/config/config.go EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL 2m ; i18n admin.serverSettings.periodicScanInterval, restartHint, libraryWatcher ; admin.libraries.scan, scanning, scanPartial, schedule, presets.* ; routes/_auth/admin/libraries.lazy.tsx PRESET_SCHEDULES ; internal/app/library/scheduler.go
watcher: internal/app/library/watcher.go Start (RealtimeMonitoring && Enabled) ; watcher_probe.go probeOnce, UnhealthyLibraries (network share blind spot) ; cmd/evey/wire_helpers.go (WatcherEnabled) ; command_service.go WithWatchHealth
music: internal/adapters/driven/filesystem/music_scanner.go (audioExts) ; internal/app/music/command_service.go - parseTrack (AlbumFolder = parent dir, title fallback) ; postgres/music_track_repo.go - GetAlbums GROUP BY album_folder, mode() display_album, ORDER BY disk_number, track_number ; GetArtists COALESCE(album_artist, artist)
album art order: internal/app/music/album_art_folder.go (folderCoverNames, embedded wins) ; cmd/evey/wire_services.go (musicbrainz then itunes)
photos: internal/adapters/driven/filesystem/photo_scanner.go (imageExts) ; internal/app/photo/command_service.go - albumOf, processOnePhoto (EXIF JPEG only) ; http/photo_handler.go - serveTranscoded (cache photos/)
collections: internal/domain/library/model.go Collection (TmdbID, Rules), MaxCollectionItems 1000 ; internal/adapters/driven/postgres/collection_repo.go (genre, type, year, library_id, rating_gte, AND) ; internal/app/collection/query_service.go GetItems ; interceptors/rbac.go CollectionService ; http/collection_handler.go (library access + parental filter) ; i18n media.collections.*, media.library.addToCollection, media.playlist.field*
keys: i18n admin.serverConfig.apiKeys.* ; admin.serverConfig.introProvidersTidbAuthed ; cmd/evey/wire_infra.go resolveAPIKeyWithBuiltin
opensubtitles: internal/app/subtitle/service.go
intro providers: docs/adr/0011-intro-detection-providers.md ; i18n admin.serverSettings.introAutoSkip
labels: i18n admin.libraries.scan, createLibrary, addPath ; media.detail refreshMetadata, resetMetadata, rematchMetadata ; media identify openButton, useThis
-->
