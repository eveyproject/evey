---
source_sha: d5c0534aa9aa7e7c333f76023148a199fc8b6bf2f16cee4f6c80face79daf6b2
reviewed: true
---
# सार्वजनिक API

वे endpoint जिन्हें साइन-इन किया हुआ हर उपयोगकर्ता कॉल कर सकता है: वही जिनसे ऐप
लाइब्रेरी ब्राउज़ करते हैं, कोई टाइटल चलाते हैं, पसंदीदा और प्लेलिस्ट रखते हैं, और
किसी मीडिया का अनुरोध करते हैं। हर सर्विस की एक फ़ाइल।

| Spec | विवरण |
|------|-------------|
| [library.swagger.json](library.swagger.json) | लाइब्रेरी, मीडिया आइटम, खोज, लोग; वह `public` झलक जिसे प्लगइन का इंटरफ़ेस पढ़ सकता है |
| [playback.swagger.json](playback.swagger.json) | स्ट्रीमिंग, प्रगति का हिसाब, देखने का इतिहास, फिर से शुरू करना |
| [user.swagger.json](user.swagger.json) | पहचान, प्रोफ़ाइल, API कुंजियाँ, Quick Connect |
| [collection.swagger.json](collection.swagger.json) | स्मार्ट और हाथ से बने संग्रह |
| [favorite.swagger.json](favorite.swagger.json) | पसंदीदा आइटम |
| [feed.swagger.json](feed.swagger.json) | होम फ़ीड के सेक्शन (देखना जारी रखें, हाल ही में जोड़े गए...) |
| [playlist.swagger.json](playlist.swagger.json) | प्लेलिस्ट, हाथ से या नियमों से बनी |
| [rating.swagger.json](rating.swagger.json) | उपयोगकर्ता की रेटिंग |
| [share.swagger.json](share.swagger.json) | सार्वजनिक शेयर लिंक |
| [syncplay.swagger.json](syncplay.swagger.json) | साथ में चलने वाले प्लेबैक के कमरे |
| [notification.swagger.json](notification.swagger.json) | ऐप के अंदर की सूचनाएँ |
| [push.swagger.json](push.swagger.json) | वेब push सदस्यताएँ |
| [music.swagger.json](music.swagger.json) | संगीत लाइब्रेरी (कलाकार, एल्बम, ट्रैक) |
| [photo.swagger.json](photo.swagger.json) | फ़ोटो लाइब्रेरी (एल्बम, फ़ोटो) |
| [media_request.swagger.json](media_request.swagger.json) | मीडिया अनुरोध: खोज, भेजना, नीति; एडमिन के जवाब और अनुरोध की पहुँच |

## पहचान

endpoint `Authorization: Bearer <token>` लेते हैं: साइन-इन से मिला सेशन token, या
अपने दायरे के अंदर एक API कुंजी ([सर्वर API](../README.md#authentication))। इनके
बिना खुले हैं: साइन-इन करना और सेशन नया करना, न्योते से खाता बनाना, Quick Connect,
पासवर्ड रीसेट, सेटअप विज़ार्ड और उसकी स्थिति, और सार्वजनिक डाउनलोड लिंक।

<!-- sources
auth: internal/adapters/driving/grpcapi/interceptors/auth.go ; public methods: internal/adapters/driving/grpcapi/interceptors/rbac.go - PublicMethods
public projection: proto/evey/v1/library.proto - GetPublicMediaItem, ListPublicMediaItems
-->
