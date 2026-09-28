---
source_sha: 3ba52125e2272c947b09a156c62f232a40194d67c79158ea4d06ec6075d320b4
---
# Evey API दस्तावेज़

Evey मीडिया सर्वर के OpenAPI (Swagger 2.0) विनिर्देश।

## संरचना

| खंड | विवरण |
|---------|-------------|
| [सार्वजनिक API](public/README.md) | उपयोगकर्ताओं के endpoints - मीडिया ब्राउज़ करना, प्लेबैक, संग्रह, प्लेलिस्ट आदि। |
| [व्यवस्थापन API](admin/README.md) | सर्वर प्रशासन - सिस्टम कॉन्फ़िगरेशन, ट्रांसकोडिंग, बैकअप, मेटाडेटा आदि। |
| [एक्सटेंशन API](plugins/README.md) | एक्सटेंशन का प्रबंधन और webhook एकीकरण। |

## उपयोग

हर फ़ोल्डर में अलग-अलग `.swagger.json` फ़ाइलें हैं, जिन्हें किसी भी OpenAPI-संगत औज़ार (Swagger UI, Postman, Insomnia आदि) में आयात किया जा सकता है।

## फिर से बनाना

ये विनिर्देश protobuf परिभाषाओं से `buf generate` के ज़रिए बनते हैं। अपडेट करने के लिए:

```bash
buf generate --template buf.gen.local.yaml
python3 docs/api/sync.py
```
