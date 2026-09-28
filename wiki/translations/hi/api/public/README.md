---
source_sha: 4609c608404f7d7614f31c97a0e6a7ce865e34c4d562caf6ca04af7ced28096a
---
# सार्वजनिक API

उपयोगकर्ताओं के endpoints, हर प्रमाणित उपयोगकर्ता के लिए उपलब्ध।

## विनिर्देश

| विनिर्देश | विवरण |
|------|-------------|
| [library.swagger.json](library.swagger.json) | लाइब्रेरी, मीडिया आइटम, खोज, लोग |
| [playback.swagger.json](playback.swagger.json) | स्ट्रीमिंग, प्रगति का हिसाब, देखने का इतिहास, फिर से शुरू करना |
| [user.swagger.json](user.swagger.json) | प्रमाणीकरण, प्रोफ़ाइल, API कुंजियाँ, क्विक कनेक्ट |
| [collection.swagger.json](collection.swagger.json) | स्मार्ट और मैनुअल संग्रह |
| [favorite.swagger.json](favorite.swagger.json) | पसंदीदा आइटम |
| [feed.swagger.json](feed.swagger.json) | होम फ़ीड के खंड (देखना जारी रखें, हाल में जोड़ा गया आदि) |
| [filter.swagger.json](filter.swagger.json) | सहेजे गए फ़िल्टर |
| [playlist.swagger.json](playlist.swagger.json) | प्लेलिस्ट (बनाना, क्रम बदलना, साझा करना) |
| [rating.swagger.json](rating.swagger.json) | उपयोगकर्ताओं की रेटिंग |
| [share.swagger.json](share.swagger.json) | सार्वजनिक साझा लिंक |
| [syncplay.swagger.json](syncplay.swagger.json) | साथ-साथ प्लेबैक के कमरे |
| [notification.swagger.json](notification.swagger.json) | ऐप के भीतर सूचनाएँ |
| [push.swagger.json](push.swagger.json) | वेब पुश सदस्यताएँ |
| [music.swagger.json](music.swagger.json) | संगीत लाइब्रेरी (कलाकार, एल्बम, ट्रैक) |
| [photo.swagger.json](photo.swagger.json) | फ़ोटो लाइब्रेरी (एल्बम, फ़ोटो) |

## प्रमाणीकरण

`/api/v1/auth/login` और `/api/v1/setup/*` को छोड़कर, हर endpoint को `Authorization: Bearer <token>` के ज़रिए एक मान्य JWT टोकन चाहिए।
