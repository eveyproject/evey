---
source_sha: cc4cecd7aee5e32086fae7de125434b98b804bf28bed8ea336efed0bb38c02bb
reviewed: true
---
# एडमिन API

सिर्फ़ एडमिन के लिए रखे गए endpoint: वेब ऐप का **एडमिन** जिन्हें कॉल करता है। हर
सर्विस की एक फ़ाइल, हिस्सों में बँटी हुई।

### सिस्टम
| Spec | विवरण |
|------|-------------|
| [admin_system.swagger.json](admin_system.swagger.json) | डैशबोर्ड, सिस्टम की जानकारी, सेटअप विज़ार्ड |
| [server_config.swagger.json](server_config.swagger.json) | सर्वर कॉन्फ़िगरेशन का key/value स्टोर |
| [remote_access.swagger.json](remote_access.swagger.json) | रिमोट एक्सेस और नेटवर्क की सेटिंग्स |
| [backup.swagger.json](backup.swagger.json) | डेटाबेस का बैकअप और रीस्टोर |
| [backup_schedule.swagger.json](backup_schedule.swagger.json) | तय समय वाले बैकअप: cron, कितने रखें, अभी चलाएँ |
| [activity.swagger.json](activity.swagger.json) | प्लेबैक का इतिहास, साइन-इन का इतिहास, सर्वर के आँकड़े |
| [admin_session.swagger.json](admin_session.swagger.json) | Audit लॉग |
| [admin_task.swagger.json](admin_task.swagger.json) | तय समय पर चलने वाले बैकग्राउंड काम |
| [presence.swagger.json](presence.swagger.json) | अभी कौन जुड़ा है (ऐप की धड़कन और एडमिन वाली सूची) |

### मीडिया और ट्रांसकोडिंग
| Spec | विवरण |
|------|-------------|
| [admin_media.swagger.json](admin_media.swagger.json) | मीडिया स्वास्थ्य (स्कैन, मरम्मत, इतिहास और वापस लेना), मीडिया आइटम हटाना, मेटाडेटा ताज़ा करना, trickplay बनाना |
| [metadata.swagger.json](metadata.swagger.json) | मेटाडेटा provider, खोज, हाथ से बदले गए मान |
| [transcoding.swagger.json](transcoding.swagger.json) | ट्रांसकोड प्रोफ़ाइल, हार्डवेयर की पहचान, चल रहे काम |
| [transcode_cache.swagger.json](transcode_cache.swagger.json) | ट्रांसकोड cache संभालना |
| [mkv.swagger.json](mkv.swagger.json) | MKV container के टूल (ट्रैक, chapter, mux/demux) |
| [admin_intro.swagger.json](admin_intro.swagger.json) | इंट्रो के समय का cache: एक एपिसोड का हटाना, पीछे के एपिसोड भरना |

### उपयोगकर्ता और नीतियाँ
| Spec | विवरण |
|------|-------------|
| [library_access.swagger.json](library_access.swagger.json) | हर उपयोगकर्ता के लिए लाइब्रेरी की पहुँच |
| [parental.swagger.json](parental.swagger.json) | अभिभावक नियंत्रण और PIN |
| [playback_settings.swagger.json](playback_settings.swagger.json) | पूरे सर्वर की प्लेबैक सेटिंग्स |

## पहचान

हर endpoint को एडमिन चाहिए: किसी एडमिन खाते का सेशन token, या एक API कुंजी, जो हमेशा
किसी एडमिन की होती है और उसमें वह दायरा होना चाहिए जो method माँगता है (बिना दायरे
वाली कुंजी किसी एडमिन method तक नहीं पहुँचती)। प्लगइन token यहाँ मना होता है, उसका
दायरा कुछ भी हो ([सर्वर API](../README.md#authentication))।

<!-- sources
admin methods: internal/adapters/driving/grpcapi/interceptors/rbac.go - MethodRoles ; api key scope on admin methods: interceptors/auth.go - authorize ; plugin token floor: auth.go - authorizePluginToken
-->
