---
source_sha: faa0ef0aa6b7e0d5694320b07792f4f14f19fdd79e035ec57246a43c7954ffb8
reviewed: true
---
# ई-मेल और सूचनाएँ { #e-mail-and-notifications }

जब नई फ़िल्में, सीरीज़, सीज़न, एपिसोड या एल्बम आते हैं, तो Evey हर प्रोफ़ाइल
को बताता है। वह इन्हें भी बताता है:

- **जिस व्यक्ति ने अनुरोध किया**, जब उसका जवाब दिया जाता है;
- **एडमिन** को, नए अनुरोधों, फ़ेल हुए बैकअप, प्लगइन की गड़बड़ियों और भरे हुए
  सबटाइटल कैश के बारे में।

सूचनाएँ ऐप में बिना कुछ सेट किए दिखती हैं। ई-मेल से तब, जब आप एक मेल सर्वर
सेट कर लें (नीचे देखें)।

## सूचनाएँ कहाँ दिखती हैं { #where-notifications-appear }

- **हर ऐप में**: घंटी, बिना पढ़ी सूचनाओं की गिनती के साथ। हमेशा चालू। एक
  सूचना 30 दिन रखी जाती है। लोगों को सिर्फ़ उन लाइब्रेरी के टाइटल दिखते हैं
  जिन्हें उनका खाता खोल सकता है।
- **ब्राउज़र में (पुश)**: वेब पर हर प्रोफ़ाइल **सेटिंग्स → सूचनाएं** में **पुश
  सूचनाएं** चालू करती है, हर ब्राउज़र में एक बार। ब्राउज़र इसकी अनुमति सिर्फ़
  `https://` पते (या `localhost`) पर देते हैं। बाकी जगह सेटिंग में लिखा होता
  है **पुश सूचनाओं के लिए HTTPS पता चाहिए: इन्हें चालू करने के लिए यह सर्वर
  HTTPS पर खोलें।** Android और TV ऐप को पुश सूचनाएँ नहीं मिलतीं।
- **ई-मेल से**: ई-मेल सेट होने के बाद हर सूचना खाते के ई-मेल पते पर भी भेजी
  जाती है, प्रोफ़ाइल की भाषा में, सर्वर के लिंक के साथ। कई प्रोफ़ाइल वाले खाते
  को हर घटना के लिए एक ई-मेल मिलता है। लोग अपने लिए ई-मेल खुद बंद नहीं कर
  सकते।

??? info "पुश कुंजियाँ"

    ब्राउज़र पुश के लिए ज़रूरी कुंजियाँ सर्वर पहली बार शुरू होने पर खुद बनाता
    है। अपनी कुंजियाँ लाने के लिए देखें
    [ई-मेल और सूचनाएँ](environment/notifications.md#browser-push)।

## ई-मेल सेट करें { #set-up-e-mail }

Evey ऐसे मेल सर्वर से ई-मेल भेजता है जिस पर आपका पहले से खाता है: आपकी ई-मेल
सेवा, या ई-मेल भेजने वाली कोई सेवा। नीचे वाले मान आपकी सेवा के सहायता पेज पर
मिलते हैं।

1. **एडमिन → सर्वर कॉन्फ़िग → ईमेल / SMTP** पर जाएँ।
2. फ़ील्ड भरें:

    | फ़ील्ड | क्या टाइप करें |
    | --- | --- |
    | **SMTP होस्ट**, **पोर्ट** | मेल सर्वर और उसका पोर्ट। आम जोड़ियाँ हैं: `587` के साथ `starttls`, या `465` के साथ `ssl`। |
    | **उपयोगकर्ता नाम**, **पासवर्ड** | वह खाता जिससे Evey साइन इन करता है। |
    | **प्रेषक पता** | भेजने वाले का पता, जो हर संदेश पर दिखता है। |
    | **TLS मोड** | `starttls` या `ssl`, पोर्ट के हिसाब से (ऊपर देखें)। `none` बिना एन्क्रिप्शन के भेजता है। |

3. **SMTP सेटिंग्स सहेजें** दबाएँ। यह तुरंत लागू होता है, बिना रीस्टार्ट के।
4. **टेस्ट ईमेल भेजें** दबाएँ। Evey आपके अपने खाते के पते पर एक छोटा संदेश
   भेजता है। अगर यह फ़ेल हो, तो मेल सर्वर का जवाब दिखाता है।

सबसे आम गलती है ऐसा TLS मोड जो पोर्ट से मेल नहीं खाता।

ई-मेल का इस्तेमाल सूचनाओं और पासवर्ड रीसेट के लिए होता है। न्योते वे लिंक
हैं जो आप खुद भेजते हैं।

??? info "इसकी जगह environment में ई-मेल सेट करना"

    compose फ़ाइल में `environment:` के नीचे लिखे `EVEY_SMTP_*` वेरिएबल सिर्फ़ तब
    इस्तेमाल होते हैं जब कार्ड में कोई होस्ट नहीं है:

    ```yaml title="docker-compose.yml"
    services:
      evey:
        environment:
          EVEY_SMTP_HOST: smtp.example.com
          EVEY_SMTP_PORT: "587"
          EVEY_SMTP_USERNAME: notifications@example.com
          EVEY_SMTP_PASSWORD: your-smtp-password
          EVEY_SMTP_FROM: noreply@example.com
          EVEY_SMTP_TLS: starttls
    ```

    जैसे ही कार्ड में कोई होस्ट सेव होता है, हर फ़ील्ड कार्ड से आती है और
    वेरिएबल अनदेखे कर दिए जाते हैं। इसलिए सर्वर वेरिएबल वाला पासवर्ड कभी पैनल
    में टाइप किए गए मेल सर्वर को नहीं भेजता। वेरिएबल पर लौटने के लिए होस्ट
    खाली करें।

## चुनें कि प्रोफ़ाइल को क्या मिले { #choose-what-a-profile-receives }

**सेटिंग्स → सूचनाएं** में हर प्रोफ़ाइल **फ़िल्में**, **सीरीज़** और
**संगीत** के लिए एक स्तर चुनती है:

| स्तर | किन नए टाइटल की सूचना मिलती है |
| --- | --- |
| **सभी** | हर एक की। यही डिफ़ॉल्ट है। |
| **प्रासंगिक** | सिर्फ़ उन टाइटल की जिनकी कोई शैली वही है जो प्रोफ़ाइल आम तौर पर देखती है। |
| **केवल महत्वपूर्ण** | फ़िलहाल प्रासंगिक जैसा ही। |
| **अक्षम** | किसी की नहीं। |

कोई शैली "आम तौर पर देखी जाने वाली" तब मानी जाती है जब वह प्रोफ़ाइल के आखिरी
200 प्लेबैक में से कम से कम 3 में आई हो। जिस टाइटल की कोई शैली नहीं है, उसकी
सूचना प्रासंगिक और केवल महत्वपूर्ण स्तर पर कभी नहीं आती।

जब एक सीरीज़ के कई एपिसोड 24 घंटे के अंदर आते हैं, तो आपको एक ही सूचना मिलती
है जो उन्हें गिनती है। अगर सीरीज़ या सीज़न की सूचना अभी-अभी दी गई हो, तो कोई
सूचना नहीं भेजी जाती।

## लोगों को अपना पासवर्ड रीसेट करने दें { #let-people-reset-their-password }

1. ई-मेल सेट करें (ऊपर देखें)।
2. **एडमिन → सर्वर कॉन्फ़िग → खाते और सुरक्षा → साइन-इन और सुरक्षा** में **पासवर्ड
   पुनर्प्राप्ति** चालू करें। यह तुरंत लागू होता है।
3. अब साइन-इन पेज पर **पासवर्ड भूल गए?** दिखता है। यह नया पासवर्ड चुनने का
   लिंक ई-मेल से भेजता है।

मेल सर्वर के बिना, विकल्प चालू होने पर भी यह लिंक छिपा रहता है।

<!-- sources
web push support: frontend/apps/web/src/lib/webpush.ts isPushSupported (serviceWorker + PushManager, secure context only) ; NotificationSettings.tsx shows pushNeedsHttps when unsupported
types: internal/domain/notification/model.go (Type*, IsAdminOnly: backup_failed, plugin_error, media_request) ; emitted: internal/app/notification/event_handlers.go, batching.go
retention 30 d: internal/app/notification/service.go defaultRetentionDays ; grouping 24 h: batching.go episodeGroupWindow + shouldSkipEpisodeNotif
list filtered by library access: internal/adapters/driving/grpcapi/notification_handler.go allowedLibraryIDs
bell on TV and mobile: frontend/apps/tv/src/components/TopHeader.tsx ; frontend/apps/mobile/app/(auth)/settings/notifications.tsx
levels: batching.go shouldNotify (relevant == highly_relevant) + fetchFavoriteGenres (GetFavoriteGenresBatch minCount 3, window 200 sessions: watch_history_analyzer.go)
defaults all: internal/adapters/driven/postgres/migrations/000005_notifications.up.sql (notification_settings_global) ; UI: frontend/apps/web/src/features/user/NotificationSettings.tsx ; i18n notificationSettings.level*
email: internal/adapters/driven/email/notification_emailer.go (push title key, recipient locale, one per account per FanoutID, base URL link)
SMTP live from the card, env fallback, panel host = panel only, defaults 587/starttls: internal/app/config/smtp.go (ResolveSMTPSettings, LiveSMTP) ; test send: internal/adapters/driving/http/smtp_test_handler.go
labels: i18n admin.serverConfig emailSmtp, smtpHost, port, smtpUsername, smtpPassword, fromAddress, tlsMode, saveSmtp, sendTestEmail
no native push in the mobile/TV apps (no push client) ; VAPID generated: cmd/evey/wire_helpers.go
password recovery: admin.serverSettings.passwordRecovery ; gated on smtpConfigured (cmd/evey/wire.go) ; auth.login.forgotPassword
admin-only types include the subtitle cache: internal/domain/notification/model.go - IsAdminOnly (TypeCacheFull) ; i18n notifications push.cache_full
labels: i18n admin.serverConfig.port, smtpUsername, smtpPassword, smtpEnvHint ; auth.settings pushNotifications, pushNeedsHttps ; levelAll, levelRelevant, levelImportant, levelDisabled
no invitation e-mail: internal/adapters/driven/email (no invite sender)
-->
