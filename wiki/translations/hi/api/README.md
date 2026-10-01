---
source_sha: 3ba52125e2272c947b09a156c62f232a40194d67c79158ea4d06ec6075d320b4
reviewed: false
---
# Evey API दस्तावेज़ीकरण

Evey मीडिया सर्वर के लिए OpenAPI (Swagger 2.0) विनिर्देश।

## संरचना

| सेक्शन | विवरण |
|---------|-------------|
| [सार्वजनिक API](public/README.md) | उपयोगकर्ताओं के लिए एंडपॉइंट: मीडिया ब्राउज़ करना, प्लेबैक, कलेक्शन, प्लेलिस्ट आदि। |
| [प्रशासन API](admin/README.md) | सर्वर प्रशासन: सिस्टम कॉन्फ़िग, ट्रांसकोडिंग, बैकअप, मेटाडेटा आदि। |
| [प्लगइन API](plugins/README.md) | प्लगइन प्रबंधन और वेबहुक एकीकरण। |

## उपयोग

हर फ़ोल्डर में अलग-अलग `.swagger.json` फ़ाइलें हैं जिन्हें किसी भी OpenAPI-संगत उपकरण (Swagger UI, Postman, Insomnia आदि) में आयात किया जा सकता है।

## फिर से बनाना

ये विनिर्देश `buf generate` के ज़रिए protobuf परिभाषाओं से बनते हैं। अपडेट करने के लिए:

```bash
buf generate --template buf.gen.local.yaml
python3 docs/api/sync.py
```
