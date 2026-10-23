---
source_sha: e679166aebb377eab1f02f6459bcb9daf6480843df9d2b438037f5909294a541
reviewed: true
---
# समस्या का हल { #troubleshooting }

कुछ काम नहीं कर रहा? जो आपको दिख रहा है, उसे नीचे की टेबल में ढूँढें। बीच वाला
कॉलम बताता है कि सबसे पहले कहाँ देखें; आखिरी कॉलम हल तक ले जाता है। अगर इतना
काफ़ी न हो, तो लॉग और `evey admin doctor` (आगे देखें) ज़्यादा बताते हैं।

## कहाँ देखें, लक्षण के हिसाब से { #where-to-look-by-symptom }

| आपको क्या दिखता है | पहले कहाँ देखें | फिर |
| --- | --- | --- |
| कुछ भी जवाब नहीं देता, घर पर भी नहीं | क्या Evey चल रहा है? `docker compose ps`, फिर उसके आउटपुट की आखिरी लाइनें: `docker compose logs --tail=200 evey` | नीचे [सर्वर शुरू नहीं होता](#the-server-does-not-start); [सर्वर जवाब नहीं देता](../runbooks/incident-runbook.md#the-server-does-not-answer) |
| ऐप में **सर्वर तक पहुंचने में असमर्थ** या **कनेक्शन विफल** लिखा है | ऐप में टाइप किया गया पता | [ऐप कनेक्ट करें](../getting-started/apps.md#the-server-address-to-enter); सिर्फ़ घर से बाहर हो तो: [रिमोट एक्सेस और HTTPS](../administration/remote-access.md) |
| सब साइन आउट हो गए, या कोई साइन इन नहीं कर पाता | **एडमिन → रिमोट एक्सेस**, विश्वसनीय प्रॉक्सी; फिर लॉग | नीचे [कोई साइन इन नहीं कर पाता](#nobody-can-sign-in) |
| फ़िल्म में तस्वीर नहीं आती, शुरू नहीं होती, या रुक जाती है | प्लेयर में **स्ट्रीम जानकारी**, फिर उस टाइटल के लिए **एडमिन → मीडिया स्वास्थ्य** | नीचे [प्लेबैक की समस्याएँ](#playback-problems) |
| बार-बार अटकती है या लोड होती रहती है | **एडमिन → स्ट्रीमिंग गुणवत्ता** (कौन से टाइटल और डिवाइस इंतज़ार कर रहे हैं) और **एडमिन → सिस्टम स्वास्थ्य** (क्या सर्वर बहुत व्यस्त है) | [ट्रांसकोडिंग और सर्वर की क्षमता](../playback/transcoding.md) |
| गलत पोस्टर, गलत टाइटल, या कोई फ़िल्म पहचानी नहीं गई | टाइटल पर **मेटाडेटा संपादित करें** | [गलत या गायब टाइटल ठीक करें](../administration/libraries.md#fix-a-wrong-or-missing-title) |
| पोस्टर गायब हैं या टूटे हुए हैं | **एडमिन → आर्टवर्क कैश** | नीचे [लाइब्रेरी और पोस्टर](#library-and-posters) |
| नई फ़ाइलें नहीं दिखतीं | **एडमिन → लाइब्रेरी** में लाइब्रेरी की लाइन पर स्कैन का नतीजा | नीचे [लाइब्रेरी और पोस्टर](#library-and-posters) |
| ई-मेल नहीं पहुँचते | **एडमिन → सर्वर कॉन्फ़िग → ईमेल / SMTP** में **टेस्ट ईमेल भेजें** | नीचे [ई-मेल नहीं पहुँचते](#e-mails-do-not-arrive) |
| कोई प्लगइन फ़ेल होता है | **एडमिन → प्लगइन** में उसके कार्ड पर उसके **लॉग** | [प्लगइन क्रैश हो गया](../runbooks/incident-runbook.md#a-plugin-crashed) |
| कास्ट जम गया या बिना आवाज़ का हो गया | **एडमिन → अभी चल रहा है**: क्या प्लेबैक अब भी सूची में है? | [कास्ट जम गया, आवाज़ चली गई, या फ़ोन घूमता रहता है](../runbooks/incident-runbook.md#a-cast-froze-went-silent-or-the-phone-keeps-spinning) |
| डिस्क भर जाती है | **एडमिन → ट्रांसकोड कैश** | [कैश वाली डिस्क भर गई है](../runbooks/incident-runbook.md#the-cache-disk-is-full) |

## लॉग { #the-logs }

लॉग में Evey का काम दर्ज होता है: हर एरर वहाँ समय के साथ लिखी जाती है।

**आसान तरीका: एडमिन → लॉग और डीबग।** यह लॉग को एक सर्च बॉक्स के साथ दिखाता
है, एक **लाइव** व्यू जो नई लाइनें आते ही दिखाता है, और किसी एक हिस्से के लिए
**क्षेत्र** फ़िल्टर (प्लेयर और वीडियो बदलना, सबटाइटल, इंट्रो की पहचान,
Chromecast, साइन इन...)। जब कोई समस्या बताए, तो सबसे पहले यहीं देखें, उस समय
के आसपास जो वह बताता है।

??? info "लॉग फ़ाइलें कहाँ हैं"

    - फ़ाइलें `./evey-data/logs/` में हैं, आपकी compose फ़ाइल के पास (कंटेनर
      के अंदर `/data/logs/evey-YYYY-MM-DD.log`)। हर दिन की एक फ़ाइल, JSON में,
      साथ में घंटे के प्रत्यय वाली एक फ़ाइल (`evey-YYYY-MM-DD-HH.log`) जब कोई फ़ाइल
      50 MB पार कर ले (`EVEY_LOG_MAX_SIZE_MB`), डिफ़ॉल्ट रूप से 90 दिन रखी जाती है (`EVEY_LOG_RETENTION_DAYS`)।
    - `docker compose logs evey` कुछ और दिखाता है: कंटेनर शुरू होते समय जो
      छापता है (start-up script की `evey:` वाली लाइनें, डेटाबेस, बैनर)। जब सर्वर
      चालू न हो, तब इसे पढ़ें। चलने के बाद लॉग फ़ाइलों और पैनल में होता है।

## `evey admin doctor` { #evey-admin-doctor }

एक कमांड पूरे सर्वर को जाँचती है। यह हर जाँच की एक लाइन छापती है, `ok`, `WARN`
या `FAIL`, और साथ में क्या करना है:

```bash
docker exec evey-server evey admin doctor
```

??? info "यह क्या जाँचता है"

    डेटाबेस और उसका schema, साइन करने वाली कुंजी, सर्च इंडेक्स (और क्या वह डेटाबेस से
    मेल खाता है), मीडिया फ़ोल्डर (यह भी कि क्या Evey हर लाइब्रेरी में लिख सकता
    है), कैश और लॉग के फ़ोल्डर, `ffmpeg` और `mkvgo`, विश्वसनीय प्रॉक्सी, रिमोट
    एक्सेस, और डिस्क में खाली जगह। `--repair` सिर्फ़ सुरक्षित सुधार लागू करता
    है (गायब फ़ोल्डर बनाना और उन्हें सही मालिक देना)। कोई जाँच फ़ेल हो, तो यह
    एरर कोड के साथ खत्म होता है। बाकी कमांड [कमांड लाइन](../runbooks/cli.md)
    में हैं।

## सर्वर शुरू नहीं होता { #the-server-does-not-start }

| `docker compose logs evey` क्या कहता है | इसका क्या मतलब है |
| --- | --- |
| कॉन्फ़िगरेशन जाँच का कोई संदेश | बाहरी डेटाबेस के साथ सर्वर डिफ़ॉल्ट पासवर्ड `evey` नहीं मानता; बाहरी Meilisearch के साथ खाली या नमूने वाली कुंजी नहीं मानता। संदेश में जिस मान का नाम है, उसे ठीक करें। |
| Postgres के बारे में `evey:` से शुरू होने वाली लाइनें | embedded डेटाबेस ने शुरू होने से मना कर दिया: Postgres के किसी दूसरे major वर्ज़न का cluster ([Postgres major अपग्रेड](advanced-deployments.md#move-to-a-newer-postgres-major)), ऐसी data directory जो Postgres cluster नहीं है, भरी हुई डिस्क, या नेटवर्क शेयर पर `./evey-data/postgres`। |
| `/cache/...` पर `permission denied`, बार-बार रीस्टार्ट के साथ | `evey-cache` volume किसी और उपयोगकर्ता का है, उसका नहीं जिसके रूप में सर्वर चलता है। इमेज अपडेट करें; 2026-09-17 से पहले की इमेज पर एक बार `docker run --rm -v <project>_evey-cache:/c alpine chown -R <PUID>:<PGID> /c` चलाएँ। |

एडमिन का पासवर्ड खो गया, साइन करने वाली कुंजी लीक हो गई, या डेटाबेस खराब हो गया:
[रिकवरी](../runbooks/recovery.md)।

## कोई साइन इन नहीं कर पाता { #nobody-can-sign-in }

दो आम वजहें:

- **डेटाबेस जवाब नहीं देता।** Evey हर साइन इन को उससे जाँचता है, और जब नहीं
  जाँच पाता तो मना कर देता है। लॉग यह बताता है।
- **कोई रिवर्स प्रॉक्सी विश्वसनीय प्रॉक्सी की सूची में नहीं है** (**एडमिन →
  रिमोट एक्सेस**)। तब हर विज़िटर प्रॉक्सी के पते से आता हुआ लगता है, और सब
  साइन इन की कोशिशों की एक ही सीमा बाँटते हैं।

कदम-दर-कदम तरीका:
[हर लॉगिन 401 देता है](../runbooks/incident-runbook.md#every-login-returns-401)।

!!! note
    सिर्फ़ साइन करने वाली कुंजी खोने या बदलने से कोई साइन आउट नहीं होता: ऐप अपने टोकन
    नए कर लेते हैं। जानबूझकर हर सेशन खत्म करना है `evey admin
    rotate-jwt-key`, देखें [रिकवरी](../runbooks/recovery.md#rotate-the-key-that-signs-access-tokens)।

सर्वर घर पर जवाब देता है लेकिन बाहर नहीं, या ऐप कहते हैं कि रिमोट एक्सेस
अक्षम है: [रिमोट एक्सेस और HTTPS](../administration/remote-access.md)।
**एडमिन → रिमोट एक्सेस** लाल रंग में बताता है जब सर्वर इंटरनेट से पहुँचा जा
सकता है लेकिन रिमोट एक्सेस बंद है।

### SSO से साइन इन फ़ेल होता है { #sso-sign-in-fails }

साइन-इन पेज वजह बताता है:

- **डोमेन की अनुमति नहीं**: पता कार्ड के अनुमत ई-मेल डोमेन से बाहर है;
- **कोई खाता मेल नहीं खाता**: साइन-अप बंद है; **नए SSO उपयोगकर्ताओं के लिए
  खाते बनाएँ** चालू करें या खाता बनाएँ;
- **खाता अक्षम है**;
- **खाता है लेकिन जोड़ा नहीं जा सका**: प्रदाता ने ई-मेल की पुष्टि नहीं की,
  जोड़ना बंद है, या खाता किसी दूसरी पहचान से जुड़ा है;
- **उपलब्ध नहीं**: प्रदाता तक पहुँचा नहीं जा सकता; कार्ड की **स्थिति** और
  **कनेक्शन जाँचें** एरर दिखाते हैं।

अगर प्रदाता खुद कोई एरर पेज दिखाता है, तो आम तौर पर इसका मतलब है कि वहाँ दर्ज
redirect URI वह नहीं है जो कार्ड दिखाता है। देखें [OIDC / SSO](../oidc.md)।

## ई-मेल नहीं पहुँचते { #e-mails-do-not-arrive }

**एडमिन → सर्वर कॉन्फ़िग → ईमेल / SMTP** में **टेस्ट ईमेल भेजें** सेव की गई
सेटिंग्स से आपके अपने पते पर एक संदेश भेजता है, और फ़ेल होने पर मेल सर्वर का
जवाब दिखाता है। आम गलती है ऐसा TLS मोड जो पोर्ट से मेल नहीं खाता (`starttls`
के साथ 587, `ssl` के साथ 465)। कार्ड में सेव किया गया होस्ट हर `EVEY_SMTP_*`
वेरिएबल की जगह ले लेता है। फ़ेल हुई सूचनाएँ लॉग में
`email: send notification` के रूप में लिखी जाती हैं। देखें
[सूचनाएँ](../notifications.md)।

## प्लेबैक की समस्याएँ { #playback-problems }

प्लेयर में **स्ट्रीम जानकारी** से शुरू करें। यह बताता है कि फ़िल्म कैसे भेजी
जा रही है (जैसी है वैसी, दूसरे फ़ॉर्मेट में कॉपी करके, या बदलकर), हर ट्रैक के
लिए, और क्यों। अगर कोई टाइटल बदला जा रहा है जबकि आपको उम्मीद थी कि वह जैसा है
वैसा चलेगा: देखें [प्लेबैक कैसे तय होता है](../playback/how-playback-is-decided.md)।

- **एक टाइटल** रुक जाता है, लोड होता रहता है, या किसी दूसरे पल पर जाने में
  धीमा है: **एडमिन → मीडिया स्वास्थ्य** खोलें। आम वजहें हैं: फ़ाइल में आगे-पीछे
  जाने का इंडेक्स नहीं है, कोई हिस्सा खराब है, या ट्रैक बड़े-बड़े टुकड़ों में
  रखे हैं। इनमें से ज़्यादातर ठीक हो सकते हैं। देखें
  [मीडिया स्वास्थ्य](../runbooks/media-health.md)।
- **कई टाइटल, जब कई लोग देख रहे हों**: हो सकता है सर्वर उतना ही बदल पा रहा हो
  जितनी उसकी सीमा है। **एडमिन → सिस्टम स्वास्थ्य** दिखाता है कि वह कितना
  व्यस्त है। देखें [ट्रांसकोडिंग और सर्वर की क्षमता](../playback/transcoding.md)।
- ट्रांसकोड वाला प्लेबैक पूरी तरह फ़ेल होता है:
  [ट्रांसकोडिंग फ़ेल होती है](../runbooks/incident-runbook.md#transcoded-playback-fails)।
- बफ़र होता है और लॉग वजह नहीं बताते:
  [जाँच का तरीका](../runbooks/incident-runbook.md#it-buffers-or-cuts-out-and-the-log-does-not-say-why)।
- सिर्फ़ ब्राउज़र में आवाज़ नहीं, Dolby Digital (AC-3/E-AC-3) ट्रैक के साथ: जो
  रिवर्स प्रॉक्सी अपनी Content-Security-Policy लगाता है, वह वेब प्लेयर का
  डिकोडर रोक सकता है। देखें [ऑडियो](../playback/audio.md#dolby-digital-in-a-browser)।
- किसी एक टाइटल पर आवाज़ तस्वीर से आगे-पीछे: **एडमिन → मीडिया स्वास्थ्य** ऐसा
  ऑडियो पहचानता है जो तस्वीर के बाद शुरू होता है, और ऐसे ट्रैक जो आपस में मेल
  नहीं खाते।
- सबटाइटल नहीं दिखते: [सबटाइटल](../playback/subtitles.md) बताता है कि प्लेयर
  कौन से फ़ॉर्मेट खुद दिखाते हैं और किन्हें सर्वर को तस्वीर में स्थायी रूप से जोड़ना पड़ता है।
- **उपशीर्षक तैयार किए जा रहे हैं…** कई मिनट तक बना रहता है: फ़ाइल में उस
  ट्रैक का इंडेक्स नहीं है, और सर्वर उसे एक बार शुरू से आखिर तक पढ़ता है।
  देखें [पहली बार जब कोई सबटाइटल दिखाया जाता है](../playback/subtitles.md#the-first-time-a-subtitle-is-shown)।
- ट्रांसकोड वाले टाइटल शुरू होने से मना करते हैं या अटक जाते हैं, जबकि पहले
  चल चुके टाइटल चलते रहते हैं, या एडमिन को **उपशीर्षक कैश भर गया** मिलता है:
  डिस्क अपनी खाली जगह की सीमा से नीचे है। देखें
  [ट्रांसकोडिंग कैश](../playback/transcoding.md#the-transcoding-cache)।

## लाइब्रेरी और पोस्टर { #library-and-posters }

पोस्टर और बैकग्राउंड तस्वीरें स्कैन के दौरान सर्वर पर `./evey-data/artworks`
में कॉपी की जाती हैं, और जो नहीं आईं उन्हें हर घंटे फिर से लाने की कोशिश होती
है। **एडमिन → आर्टवर्क कैश** दिखाता है कि कितनी तस्वीरों की अभी कोई लोकल कॉपी
नहीं है; **अनुपस्थित आर्टवर्क डाउनलोड करें** उन्हें अभी लाता है, और **खराब
वैरिएंट साफ़ करें** उन छोटे किए गए पोस्टर को हटाता है जिन्हें छोटा करने की
फ़ेल कोशिश ने खाली छोड़ दिया; ज़रूरत पड़ने पर वे फिर बन जाते हैं।

किसी दूसरी फ़िल्म का पोस्टर या टाइटल, मेटाडेटा की गलत पहचान है, तस्वीर की
समस्या नहीं: [गलत या गायब टाइटल ठीक करें](../administration/libraries.md#fix-a-wrong-or-missing-title)।

नई फ़ाइल आम तौर पर कुछ मिनटों में दिख जाती है (देखें
[नई फ़ाइलें कैसे पहचानी जाती हैं](../administration/libraries.md#how-new-files-are-picked-up))।
अगर नहीं दिखती, तो **एडमिन → लाइब्रेरी** में लाइब्रेरी की लाइन पर **स्कैन**
दबाएँ और नतीजा पढ़ें।

अगर स्कैन **आंशिक स्कैन - ये फ़ोल्डर पढ़े नहीं जा सके:** और उसके बाद कुछ पथ
के साथ खत्म होता है: Evey उन फ़ोल्डर को नहीं पढ़ पाता (अनुमतियाँ, या कोई
नेटवर्क शेयर जिसका कनेक्शन टूट गया)। बाकी लाइब्रेरी स्कैन होती है, और न पढ़े जा सकने वाले
फ़ोल्डर के टाइटल हटाए नहीं जाते, रखे जाते हैं। mount और मालिक जाँचें
(`PUID`/`PGID`, देखें [इंस्टॉल](../installation.md#common-variations)), फिर
दोबारा स्कैन करें।

खोज कुछ नहीं लौटाती: सर्च इंडेक्स को डेटाबेस से फिर बनाएँ:
`docker exec evey-server evey admin reindex-search`। कुछ नहीं खोता: इंडेक्स
हमेशा फिर से बनाया जा सकता है।

## समस्या की रिपोर्ट करना { #reporting-a-problem }

**contact@eveyproject.org** पर लिखें। जिस रिपोर्ट पर काम हो सके, वह बताती है:

- आपने क्या किया, आप क्या उम्मीद कर रहे थे, और उसकी जगह क्या हुआ;
- यह कब हुआ, मिनट तक: लॉग समय से खोजे जाते हैं;
- डिवाइस और ऐप: Android ऐप में **सेटिंग्स → जानकारी → ऐप संस्करण**, वेब ऐप के
  लिए ब्राउज़र और उसका वर्ज़न;
- सर्वर का वर्ज़न: `docker exec evey-server evey version`;
- प्लेबैक की समस्या के लिए, टाइटल और **स्ट्रीम जानकारी** का स्क्रीनशॉट (देखें
  [स्ट्रीम जानकारी पढ़ना](../playback/how-playback-is-decided.md#reading-stream-info)):
  इसमें चुना गया तरीका और उसकी वजहें होती हैं, जो जाँच का ज़्यादातर हिस्सा है।

एडमिन उस पल के आसपास की लॉग लाइनें (**एडमिन → लॉग और डीबग**, देखें
[लॉग](#the-logs)) और [`evey admin doctor`](#evey-admin-doctor) की रिपोर्ट जोड़
सकता है। लॉग में खातों और नेटवर्क पतों के नाम होते हैं: भेजने से पहले उन्हें
पढ़ लें।

सुरक्षा की कमी सार्वजनिक जगह पर नहीं बताई जाती: **security@eveyproject.org**
पर लिखें, और उसके बारे में बात करने से पहले उसे ठीक होने का समय दें।

<!-- sources
symptom table: i18n mobile.serverConnect.unreachable / connectionFailed ; i18n media.rematchMetadata, admin.library.refreshMetadataTooltip
logs: internal/pkg/logger/logger.go - New (file handler only outside DevMode), rotatingWriter.rotate (evey-YYYY-MM-DD.log), StartCleanup (90 days) ; internal/adapters/driving/http/admin_handler_logs.go - currentLogPath ; docker-compose.yml EVEY_LOG_DIR=/data/logs, ./evey-data:/data ; i18n admin.logs.where.*, admin.logs.live
docker compose logs: docker/entrypoint.sh (evey: lines), cmd/evey/banner.go
doctor: cmd/evey/admin_commands.go - doctor ; cmd/evey/admin_ops.go - checkResult names (database, schema, jwt.key, meilisearch, search index, media <root>, trusted proxies, remote access, disk), checkTool ffmpeg/mkvgo
sso: internal/adapters/driving/http/auth_handler.go /login?oidc_error=<reason>, i18n auth.login.ssoError.* ; smtp: internal/adapters/driving/http/smtp_test_handler.go (test send), internal/adapters/driven/email/notification_emailer.go (log)
partial scan: internal/app/library/scan_service.go - scanDirectories (unreadable root = failed path, protected from pruning) ; i18n admin.libraries.scanPartial
posters: internal/app/metadata/command_service_artwork.go (hourly retry) ; i18n admin.artworks.* (/data/artworks, purge variants)
rotate-jwt-key ends sessions: cmd/evey/admin.go - endAllSessions
startup failures: docker/entrypoint.sh ; cache ownership: docker/entrypoint.sh (re-own /cache)
remote access warning: docs/adr/0026-remote-access-inferred-from-a-published-url.md
reporting: docs/privacy.md (contact@eveyproject.org) ; docs/SECURITY.md (security@eveyproject.org) ; app version: frontend/apps/mobile/app/(auth)/(tabs)/settings.tsx, i18n mobile.settings.appVersion ; server version: cmd/evey/cli.go
new files row: i18n admin.libraries.scan, scanPartial ; docs/administration/libraries.md (How new files are picked up) ; e-mail row: i18n admin.serverConfig.sendTestEmail
-->
