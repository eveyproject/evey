---
source_sha: a960630483d1b2a05980e34cb7d0eacb371e9de2acf7fcd4af72e9609a09a8cf
reviewed: true
---
# प्लगइन API

प्लगइन से जुड़े endpoint: उन्हें इंस्टॉल करना और संभालना (एडमिन), वेबहुक (एडमिन),
और वह पसंद जो प्लगइन का इंटरफ़ेस हर उपयोगकर्ता के लिए रखता है। प्लगइन लिखना एक
अलग अनुबंध है, लोकल socket पर gRPC से: देखें
[प्लगइन लिखना](../../plugins/dev-setup.md) और
[प्लगइन API संदर्भ](../../plugins/api-reference.md)।

| Spec | विवरण |
|------|-------------|
| [admin_plugin.swagger.json](admin_plugin.swagger.json) | प्लगइन इंस्टॉल, सक्षम, अक्षम, अनइंस्टॉल करना; कॉन्फ़िगरेशन, लॉग, संसाधन सीमाएँ, runtime; अपडेट का पूर्वावलोकन और लागू करना; `IssuePluginToken` |
| [webhook.swagger.json](webhook.swagger.json) | वेबहुक बनाना, सूची देखना, बदलना और हटाना; भेजने का इतिहास |
| [plugin_prefs.swagger.json](plugin_prefs.swagger.json) | प्लगइन इंटरफ़ेस की हर उपयोगकर्ता वाली पसंद (`prefs` अनुमति) |

पैकेज अपलोड करना एक multipart `POST /api/v1/admin/plugins/upload` है (फ़ील्ड
`plugin`, वैकल्पिक `sig`, `accept_permission_changes`); `POST
/api/v1/admin/plugins/preview` बिना इंस्टॉल किए विश्लेषण करता है। दोनों एडमिन के
route हैं।

## पहचान

प्लगइन संभालने और वेबहुक के लिए एडमिन चाहिए, [एडमिन API](../admin/README.md) की तरह।
`plugin_prefs` अकेली सर्विस है जो प्लगइन token के लिए बनी है: होस्ट पेज इसे एक
प्लगइन और एक उपयोगकर्ता के लिए बनाता है, और सर्विस सिर्फ़ उसी जोड़े की लाइनें लौटाती
है ([सर्वर API](../README.md#authentication))।

<!-- sources
methods: proto/evey/v1/admin_plugin.proto, webhook.proto, plugin_prefs.proto
upload and preview routes: internal/adapters/driving/http/plugin_asset_handler.go
plugin token on prefs: internal/adapters/driving/grpcapi/interceptors/rbac.go - PluginMethodScopes ; internal/adapters/driving/grpcapi/plugin_prefs_handler.go
-->
