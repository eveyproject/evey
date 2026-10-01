---
source_sha: d950d41e450f1d05a29cf775d0b79159572cfa1353c7e02ac009671bb0bcefc1
reviewed: false
---
# सार्वजनिक API

उपयोगकर्ताओं के लिए एंडपॉइंट, सभी प्रमाणित उपयोगकर्ताओं के लिए उपलब्ध।

## विनिर्देश

| विनिर्देश | विवरण |
|------|-------------|
| [library.swagger.json](library.swagger.json) | लाइब्रेरी, मीडिया आइटम, खोज, लोग |
| [playback.swagger.json](playback.swagger.json) | स्ट्रीमिंग, प्रगति ट्रैकिंग, देखने का इतिहास, फिर से शुरू करना |
| [user.swagger.json](user.swagger.json) | प्रमाणीकरण, प्रोफ़ाइल, API कुंजियाँ, क्विक कनेक्ट |
| [collection.swagger.json](collection.swagger.json) | स्मार्ट और मैनुअल कलेक्शन |
| [favorite.swagger.json](favorite.swagger.json) | पसंदीदा आइटम |
| [feed.swagger.json](feed.swagger.json) | मुखपृष्ठ के सेक्शन (देखना जारी रखें, हाल ही में जोड़े गए आदि) |
| [playlist.swagger.json](playlist.swagger.json) | प्लेलिस्ट (बनाना, क्रम बदलना, साझा करना) |
| [rating.swagger.json](rating.swagger.json) | उपयोगकर्ता रेटिंग |
| [share.swagger.json](share.swagger.json) | सार्वजनिक साझा लिंक |
| [syncplay.swagger.json](syncplay.swagger.json) | तालमेल वाले प्लेबैक कमरे |
| [notification.swagger.json](notification.swagger.json) | ऐप में सूचनाएँ |
| [push.swagger.json](push.swagger.json) | वेब पुश सदस्यताएँ |
| [music.swagger.json](music.swagger.json) | संगीत लाइब्रेरी (कलाकार, एल्बम, ट्रैक) |
| [photo.swagger.json](photo.swagger.json) | फ़ोटो लाइब्रेरी (एल्बम, फ़ोटो) |
| [media_request.swagger.json](media_request.swagger.json) | मीडिया अनुरोध: खोज, भेजना, नीति; एडमिन के जवाब और अनुरोध पहुँच |

## प्रमाणीकरण

`/api/v1/auth/login` और `/api/v1/setup/*` को छोड़कर सभी एंडपॉइंट को `Authorization: Bearer <token>` के ज़रिए एक मान्य JWT टोकन चाहिए।
