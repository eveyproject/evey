---
source_sha: aacd60e164be25536f5b7ca3fe0a88655d153900e00d2521c8d0795dd2769d24
reviewed: true
---
# भाषाएँ और सुलभता

भाषाओं (i18n) और सुलभता (a11y) के मामले में Evey कहाँ खड़ा है: क्या अनुवादित है
और अनुवादों की सूची हर क्लाइंट तक कैसे पहुँचती है, फिर सहायक तकनीक के लिए क्या
मौजूद है और क्या नहीं। अनुवादों की सूची repository की जड़ में
`i18n/<locale>/*.yaml` में रहती है; `internal/pkg/i18n` इसे देता है और हर ऐप का i18n
मॉड्यूल इसे लोड करता है। यह एक self-hosted प्रोजेक्ट है जिसे एक व्यक्ति चलाता है:
ख़ासकर सुलभता को "जितना हो सके उतना" मानें, प्रमाणित नहीं।

## भाषाएँ (i18n)

### क्या अनुवादित है और क्या नहीं

| Evey जिसका अनुवाद करता है | Evey जिसका अनुवाद नहीं करता |
| --- | --- |
| इंटरफ़ेस का ढाँचा: बटन, मेन्यू, सेटिंग्स, त्रुटि संदेश, push सूचनाओं का टेक्स्ट, सूचना वाले ई-मेल (पाने वाले की भाषा में) | मीडिया का मेटाडेटा (टाइटल, कहानी का सारांश, शैलियाँ), जो मेटाडेटा provider से उस **मेटाडेटा भाषा** में आता है जो सेटअप विज़ार्ड में या **एडमिन → सर्वर कॉन्फ़िग → भाषा और क्षेत्र** में सेट है (`EVEY_METADATA_LANGUAGE` सिर्फ़ डिफ़ॉल्ट है) |
| | आपकी अपनी लाइब्रेरी, फ़ाइलों के नाम, और जो कुछ आप टाइप करते हैं, जैसा है वैसा सहेजा जाता है |

पासवर्ड रीसेट वाले ई-मेल अंग्रेज़ी में भेजे जाते हैं। इंटरफ़ेस की भाषा और मेटाडेटा
की भाषा दो अलग सेटिंग्स हैं: इंटरफ़ेस फ़्रेंच में हो सकता है जबकि फ़िल्म का सारांश
अंग्रेज़ी में हो, या इसका उल्टा।

### इंटरफ़ेस की भाषाएँ

छह: **अंग्रेज़ी (en)**, **फ़्रेंच (fr)**, **जर्मन (de)**, **स्पैनिश (es)**,
**चीनी (zh)**, **हिंदी (hi)**। अंग्रेज़ी स्रोत भाषा है और आख़िरी सहारा भी: किसी दूसरी
भाषा में जो वाक्य न हो, वह खाली जगह या कुंजी दिखाने के बजाय अंग्रेज़ी में दिखता है।

### एक वाक्य स्क्रीन तक कैसे पहुँचता है

- **सच का एक ही स्रोत:** repo की जड़ में [`/i18n/<locale>/*.yaml`](../i18n),
  namespace में बँटा हुआ (`common`, `auth`, `media`, `player`, `admin`, `errors`,
  `notifications`, `mobile`, `downloads`, `requests`)। वही सूची हर जगह इस्तेमाल
  होती है।
- **सर्वर** इसे लोड करता है और `GET /api/v1/i18n/{locale}` पर देता है, और अनुवादित
  API त्रुटि संदेशों और सूचनाओं के टेक्स्ट के लिए इसे सीधे इस्तेमाल करता है।
- **वेब ऐप** अनुवाद उसी endpoint से लोड करता है (HTTP backend के साथ i18next), अगली
  बार शुरू होने के लिए ब्राउज़र के local storage में एक कॉपी रखता है, और
  `<html lang>` को चालू भाषा के साथ मिलाकर रखता है, ताकि सहायक तकनीक सही भाषा
  बोले। **मोबाइल और टीवी ऐप** उसी endpoint से, साझा native i18n मॉड्यूल के ज़रिए
  लोड करते हैं, और उन स्क्रीन के लिए अंग्रेज़ी का एक साथ आने वाला आधार
  (`fallback.en.json`, `offlineStrings.ts`) रखते हैं जो सर्वर के जवाब से पहले
  दिखती हैं।
- भाषा आपके खाते की पसंद से चुनी जाती है; वह न हो तो ब्राउज़र/डिवाइस की भाषा से
  (`Accept-Language` पढ़ा जाता है, क्षेत्र हटा दिया जाता है: `fr-FR` बन जाता है
  `fr`), फिर **एडमिन → सर्वर कॉन्फ़िग → भाषा और क्षेत्र** की **सर्वर भाषा** से (जो
  नए खातों की भाषा भी तय करती है), फिर अंग्रेज़ी।
- वाक्य में मान भरना और एकवचन/बहुवचन i18next से होता है (`{{count}}` वगैरह)।

### अभी क्या नहीं हुआ

- **पूरापन अलग-अलग है।** अंग्रेज़ी आधिकारिक है और हमेशा पूरी होती है; बाकी पाँच
  हाथ से बराबरी पर रखी जाती हैं, एक टेस्ट (`internal/pkg/i18n/parity_test.go`) के
  साथ जो कुंजियों का समूह अलग होने पर फ़ेल होता है, पर कोई बिल्कुल नई सुविधा पहले
  अंग्रेज़ी में आ सकती है और अनुवाद होने तक अंग्रेज़ी में ही दिखती है।
- **दाएँ से बाएँ लिखी जाने वाली कोई भाषा नहीं।** अरबी/हिब्रू/फ़ारसी नहीं है, और
  बनावट RTL के लिए न बनाई गई है न परखी गई है: ऐसी भाषा जोड़ना असली काम है (CSS
  logical properties, `dir` संभालना), सिर्फ़ एक अनुवाद फ़ाइल नहीं।
- **अनुवादों की पेशेवर समीक्षा नहीं हुई है।** ये maintainer के स्तर के हैं।
  सुधारों का स्वागत है।
- **भाषा के हिसाब से फ़ॉर्मैटिंग अधूरी है।** तारीख़ें और संख्याएँ platform के
  `Intl`/ब्राउज़र की भाषा पर निर्भर हैं और हर स्क्रीन पर एक जैसी जुड़ी नहीं हैं।

### भाषा जोड़ना या सुधारना

1. `i18n/en/` को `i18n/<your-locale>/` में कॉपी करें और YAML के मानों का अनुवाद करें
   (कुंजियाँ वैसी ही रखें)।
2. भाषा को [`internal/pkg/i18n/i18n.go`](../internal/pkg/i18n/i18n.go) की अनुमति
   सूची (`supportedLocales`) में जोड़ें।
3. सभी भाषाओं में कुंजियों का समूह एक जैसा रखें: वरना बराबरी वाला टेस्ट फ़ेल होता
   है, और `TestI18nPlaceholdersAgreeAcrossLocales` यह भी जाँचता है कि हर वाक्य के
   `{{placeholders}}` भी मेल खाते हों।

## सुलभता (a11y)

### स्थिति क्या है

Evey semantic HTML, ARIA label और React Native की accessibility props के साथ बना है,
और नीचे की कुछ बातें जानबूझकर की गई हैं। पर WCAG के हिसाब से इसका औपचारिक audit
**नहीं** हुआ है, टेस्ट में **सुलभता की कोई स्वचालित जाँच नहीं** है, और screen
reader के साथ इसे व्यवस्थित ढंग से **नहीं** परखा गया है। इसके कुछ हिस्से शायद
सहायक तकनीक के साथ चल जाते हैं; कुछ शायद नहीं। अगर आप सहायक तकनीक पर निर्भर हैं,
तो कुछ कमियों की उम्मीद रखें और कृपया ठीक-ठीक बताएँ कि क्या गड़बड़ है।

### क्या मौजूद है

| हिस्सा | क्या किया गया है |
| --- | --- |
| कम हलचल | `prefers-reduced-motion: reduce` पूरे ऐप में माना जाता है (`frontend/apps/web/src/styles/main.css`): जब आपका OS कहे, तो animation और transition लगभग बंद हो जाते हैं |
| सहायक तकनीक के लिए भाषा | `<html lang>` इंटरफ़ेस की चालू भाषा के साथ चलता है |
| कीबोर्ड (प्लेयर) | पूरा कीबोर्ड नियंत्रण, स्क्रीन पर shortcuts की एक परत के साथ (`?`, `KeyboardShortcutsOverlay.tsx`); किसी फ़ील्ड में टाइप करते समय shortcuts बंद रहते हैं |
| नाम वाले controls | सिर्फ़ icon वाले बटन और slider पर वेब पर `aria-label` और मोबाइल पर `accessibilityLabel`/`accessibilityRole` होते हैं (interactive controls का बड़ा हिस्सा, सब नहीं) |
| थीम | पाँच थीम साथ आती हैं (डिफ़ॉल्ट डार्क, AMOLED, dark-red, light-default, light-blue); थीम प्लगइन से और भी |
| Android TV | रिमोट से चलाने के लिए D-pad focus संभालना |

### अभी क्या नहीं हुआ

- **screen reader से कोई जाँच नहीं।** NVDA/VoiceOver/TalkBack के रास्ते परखे नहीं
  गए; कुछ ख़ास विजेट (मेन्यू, modal, seek bar, ट्रैक चुनने वाले) शायद अपनी स्थिति या
  भूमिका ठीक से न बताएँ।
- **focus संभालने का audit नहीं हुआ।** dialog और ऊपर आने वाली परतों में
  focus-visible की स्टाइल और focus का क्रम कुछ जगह एक जैसे नहीं हैं।
- **contrast सिर्फ़ साथ आने वाली थीम के लिए मापा जाता है।** एक unit टेस्ट
  (`frontend/packages/theme/src/contrast.test.ts`) साथ आने वाली हर थीम को accent,
  टेक्स्ट के तीन स्तरों और danger रंगों के लिए, पृष्ठभूमि और दोनों सतहों के सामने,
  WCAG AA 4.5:1 पर रखता है; टीवी ऐप अपने निकाले गए token फिर से जाँचता है
  (`ThemeProvider.contrast.test.ts`)। थीम प्लगइन और प्लगइन जो रंग चलते समय डालता है,
  वे नहीं जाँचे जाते: कोई custom थीम कुछ जगह फिर भी फ़ेल हो सकती है। AAA का लक्ष्य
  नहीं है।
- ब्राउज़र/OS zoom के अलावा **ऐप के अंदर टेक्स्ट बड़ा करने की सुविधा नहीं**, और कोई
  अलग high-contrast मोड नहीं।
- **मोबाइल पर सबटाइटल की स्टाइल सीमित है** (नीचे वाले `expo-video` प्लेयर की एक
  सीमा), हालाँकि सबटाइटल/caption ट्रैक खुद चलते हैं।

### समस्या बताना

हमें पेज/स्क्रीन, ठीक कौन सा control, आपने क्या उम्मीद की थी, और इस्तेमाल की गई
सहायक तकनीक और उसका वर्ज़न बताएँ। ठोस रिपोर्ट ठीक होती हैं; "यह सुलभ नहीं है" पर
कुछ किया नहीं जा सकता। सुलभता के सुधार जितना हो सके उतना किए जाते हैं।

<!-- sources
catalogue: i18n/en, de, es, fr, hi, zh ; internal/pkg/i18n/i18n.go - supportedLocales, FallbackLocale, ParseLocale ; internal/pkg/i18n/parity_test.go ; internal/contractguard - TestI18nPlaceholdersAgreeAcrossLocales, TestServerEmittedI18nKeysExistInCatalogue
endpoint: internal/adapters/driving/http/server.go - GET /api/v1/i18n/{locale}
loaders: frontend/apps/web/src/i18n/index.ts - HttpBackend, getCached, documentElement.lang ; frontend/packages/client/src/native/i18n.ts ; frontend/apps/mobile/src/i18n/fallback.en.json ; frontend/apps/tv/src/i18n/offlineStrings.ts
mails: internal/adapters/driven/email/password_reset_emailer.go ; internal/adapters/driven/email/notification_emailer.go - WithNotificationLocale
labels: i18n admin.serverConfig.languageRegion, serverLanguage, metadataLanguage
a11y: frontend/apps/web/src/styles/main.css - prefers-reduced-motion ; frontend/apps/web/src/features/playback/KeyboardShortcutsOverlay.tsx ; frontend/packages/theme/src/themeVars.ts ; frontend/packages/theme/src/contrast.test.ts ; frontend/apps/tv/src/providers/ThemeProvider.contrast.test.ts ; frontend/apps/mobile/package.json - expo-video
-->
