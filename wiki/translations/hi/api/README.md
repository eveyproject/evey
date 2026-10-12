---
source_sha: 5d6e1c496b373ae579e3de32d6359d6c1c9712b93ef8c17806d17afca066d8f8
reviewed: true
---
# सर्वर API

सर्वर की HTTP API, OpenAPI (Swagger 2.0) फ़ाइलों में बताई गई, हर सर्विस की एक फ़ाइल।
यह सूची बताती है कि किस फ़ोल्डर में क्या है और कॉल की पहचान कैसे होती है; असली
रेफ़रेंस फ़ाइलें खुद हैं।

| हिस्सा | क्या है |
|---------|-------------|
| [सार्वजनिक API](public/README.md) | साइन-इन किया हुआ उपयोगकर्ता क्या कर सकता है: ब्राउज़ करना, चलाना, सहेजना, अनुरोध करना। |
| [एडमिन API](admin/README.md) | सर्वर का प्रशासन: कॉन्फ़िगरेशन, ट्रांसकोडिंग, बैकअप, मेटाडेटा, उपयोगकर्ता। |
| [प्लगइन API](plugins/README.md) | प्लगइन संभालना, वेबहुक, और वह पसंद जो प्लगइन का इंटरफ़ेस हर उपयोगकर्ता के लिए रखता है। |

हर `.swagger.json` फ़ाइल किसी भी OpenAPI टूल (Swagger UI, Postman, Insomnia) में
import हो जाती है। HTTP route `proto/evey/v1/` की gRPC परिभाषाओं से बनाए जाते हैं,
इसलिए फ़ाइल में किसी method का नाम उसके पीछे वाला gRPC method है।

## पहचान

API तक तीन तरह की पहचान पहुँचती है, सब `Authorization` header में:

| पहचान | कौन | कैसे |
|---|---|---|
| सेशन token | किसी ऐप से साइन-इन किया हुआ व्यक्ति | `Bearer <token>`, साइन-इन करने पर मिलता है और ऐप इसे नया करता रहता है। |
| API कुंजी | एक script, उस एडमिन की ओर से जिसने इसे बनाया | `Bearer evey_...` (`Bearer` लगाना वैकल्पिक है)। दायरे वाली, रद्द की जा सकने वाली: [वेबहुक और API कुंजियाँ](../webhooks-api-keys.md)। |
| प्लगइन token | प्लगइन का इंटरफ़ेस, होस्ट पेज के ज़रिए | एक प्लगइन और एक उपयोगकर्ता के लिए पाँच मिनट को बनाया जाता है, और इसमें सिर्फ़ वे `public` और `prefs` दायरे होते हैं जो प्लगइन को मिले; हर दूसरा method इसे मना करता है। फ़्रेम के पास यह कभी नहीं होता: होस्ट पेज कॉल आगे भेजता है। |

प्लगइन token किसी प्रशासन method तक कभी नहीं पहुँच सकता, उसका दायरा कुछ भी हो
([ADR-0003](../adr/0003-plugin-scoped-tokens.md),
[ADR-0012](../adr/0012-plugin-token-deny-by-default.md))।

## फ़ाइलें फिर से बनाना

योगदान करने वालों के लिए: फ़ाइलें protobuf परिभाषाओं से फिर से बनाई जाती हैं।

```bash
buf generate --template buf.gen.local.yaml
python3 docs/api/sync.py
```

<!-- sources
session token, api key, plugin token: internal/adapters/driving/grpcapi/interceptors/auth.go - authenticate, authenticateAPIKey, authorizePluginToken
plugin token: internal/app/plugin/token_service.go - PluginTokenTTL ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes ; docs/adr/0003-plugin-scoped-tokens.md ; docs/adr/0012-plugin-token-deny-by-default.md
generation: docs/api/sync.py ; buf.gen.local.yaml
-->
