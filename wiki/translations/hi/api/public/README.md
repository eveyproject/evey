---
source_sha: 58e220cb177619c27f3d55f0ae3d1c6a3b4df50ab6ace660df93aa8ccf6e6df9
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
| [playlist.swagger.json](playlist.swagger.json) | प्लेलिस्ट, हाथ से या नियमों से बनी |
| [rating.swagger.json](rating.swagger.json) | उपयोगकर्ता रेटिंग |
| [share.swagger.json](share.swagger.json) | सार्वजनिक साझा लिंक |
| [syncplay.swagger.json](syncplay.swagger.json) | तालमेल वाले प्लेबैक कमरे |
| [notification.swagger.json](notification.swagger.json) | ऐप में सूचनाएँ |
| [push.swagger.json](push.swagger.json) | वेब पुश सदस्यताएँ |
| [music.swagger.json](music.swagger.json) | संगीत लाइब्रेरी (कलाकार, एल्बम, ट्रैक) |
| [photo.swagger.json](photo.swagger.json) | फ़ोटो लाइब्रेरी (एल्बम, फ़ोटो) |
| [media_request.swagger.json](media_request.swagger.json) | मीडिया अनुरोध: खोज, भेजना, नीति; एडमिन के जवाब और अनुरोध पहुँच |

## प्रमाणीकरण

एंडपॉइंट `Authorization: Bearer <token>` लेते हैं: साइन इन से मिला सेशन टोकन, या एक API कुंजी (देखें [वेबहुक और API कुंजियाँ](../../webhooks-api-keys.md))। इसके बिना खुले: साइन इन और सेशन नवीनीकरण, आमंत्रण से साइन अप, Quick Connect, पासवर्ड रीसेट, सेटअप विज़ार्ड और उसकी स्थिति, और सार्वजनिक डाउनलोड लिंक।
