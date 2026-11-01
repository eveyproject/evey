---
source_sha: 44e8e90d77d0eb2abdd259fe7a12d693a5f6af4856125bd2b2da2194a7c6ef42
reviewed: true
---
# वेबहुक और API कुंजियाँ

प्लगइन लिखे बिना Evey को दूसरे टूल से जोड़ने के दो तरीके: वेबहुक सर्वर के event
आपके किसी URL पर भेजता है, और API कुंजी किसी script को आपकी तरह सर्वर को कॉल करने
देती है। दोनों एडमिन पैनल में एडमिन बनाता है; यह पेज उस व्यक्ति के लिए है जो इन्हें
सेट करता है, और उसके लिए जो receiver या script लिखता है।

## event किसी URL पर भेजें

**एडमिन → वेबहुक → वेबहुक बनाएँ**: एक **नाम**, एक **URL**, भेजे जाने वाले
**इवेंट**, और सिग्नेचर के लिए एक वैकल्पिक **सीक्रेट**। **परीक्षण** एक ping भेजता है
और जवाब बताता है।

<figure class="evey-shot" markdown="span">
  ![एडमिन → वेबहुक में एक वेबहुक](images/admin-webhooks.webp){ loading=lazy }
  <figcaption>हर प्लेबैक की शुरुआत और अंत भेजने वाला एक वेबहुक (स्क्रीन अंग्रेज़ी में)।</figcaption>
</figure>

**इवेंट।** वेबहुक **सभी इवेंट** की सदस्यता ले सकता है, या चुनी हुई एक सूची की। फ़ॉर्म हर वह
event दिखाता है जो सर्वर प्रकाशित करता है; उनमें से कुछ:

| हिस्सा | इवेंट |
| --- | --- |
| लाइब्रेरी | `library.scan.started`, `library.scan.completed`, `library.prune.suspended` (किसी रूट से एक साथ बहुत ज़्यादा फ़ाइलें गायब हो गईं, एडमिन की पुष्टि तक हटाना रोका गया), `media.item.added`, `media.item.updated`, `media.item.removed`, `metadata.refreshed` |
| प्लेबैक | `playback.started`, `playback.stopped`, `media.marked.watched`, `transcode.started`, `transcode.failed` |
| खाते | `user.created`, `user.login.succeeded`, `user.login.failed`, `user.profile.created`, `user.apikey.created` |
| अनुरोध | `media.request.created`, `media.request.answered` |
| सर्वर | `backup.created`, `backup.failed`, `plugin.installed`, `plugin.error`, `server.config.changed`, `transcoding.derived_cache_full` (सबटाइटल स्टोर अपनी खाली जगह की सीमा से नीचे) |

जो प्लगइन `EmitEvent` कॉल करता है, वह `plugin.event.<type>` प्रकाशित करता है, जो
**सभी इवेंट** वाले वेबहुक को भी मिलता है।

**भेजना।** हर event एक JSON `POST` है, इन header के साथ:

- `X-Evey-Delivery`: एक पहचान जो एक ही भेजने की सारी कोशिशों में एक जैसी रहती है,
  ताकि आपका receiver दोहराव को अनदेखा कर सके;
- `X-Evey-Signature`: `sha256=` और उसके बाद body का HMAC-SHA256, जो वेबहुक के
  सीक्रेट से निकाला गया है। इसे उसी सीक्रेट से जाँचें, ताकि पता रहे कि request आपके
  सर्वर से आया है। सीक्रेट के बिना सिग्नेचर कुछ साबित नहीं करता। यह सिर्फ़ body को ढकता है,
  इसलिए पकड़े गए request को दोबारा भेजे जाने से नहीं रोकता: नीचे के दो header को चुनें;
- `X-Evey-Timestamp`: कोशिश का समय, Unix सेकंड में। हर नई कोशिश में नया मान होता है;
- `X-Evey-Signature-V2`: `sha256=` और उसके बाद वेबहुक के सीक्रेट से निकाला गया
  HMAC-SHA256, इस पाठ का: `<timestamp>.<delivery id>.<body>` (`X-Evey-Timestamp`
  का मान, एक बिंदु, `X-Evey-Delivery` का मान, एक बिंदु, फिर कच्ची body)।

किसी भेजे गए event को जाँचने के लिए कच्ची body से V2 सिग्नेचर दोबारा निकालें और उसे
constant time में मिलाएँ, फिर अगर timestamp आपकी घड़ी से 5 मिनट से ज़्यादा दूर हो
(किसी भी दिशा में) तो request मना कर दें। अगर आपका receiver उस अवधि में देखे गए
delivery id भी याद रखता है, तो पकड़ा गया request दोबारा नहीं चलाया जा सकता।

हर कोशिश जवाब के लिए 30 सेकंड तक रुकती है। फ़ेल होने पर 1, 5 और 30 सेकंड बाद फिर
कोशिश होती है। हर कोशिश दर्ज होती है; इतिहास API से पढ़ा जाता है
([प्लगइन API](api/plugins/README.md) में `ListWebhookDeliveries`), पैनल में नहीं।

!!! warning "सिर्फ़ सार्वजनिक पते"
    वेबहुक का URL `http` या `https` होना चाहिए और किसी सार्वजनिक पते की ओर होना
    चाहिए। `localhost`, निजी और link-local पते (`192.168.x.x`, `10.x.x.x`,
    `172.16.x.x`...), और ऐसा नाम जो इनमें से किसी पर पहुँचता हो, वेबहुक सहेजते समय
    और फिर सर्वर के जुड़ते समय मना होते हैं। इससे सर्वर को आपके अंदरूनी नेटवर्क तक
    पहुँचने के लिए इस्तेमाल नहीं किया जा सकता; आपके लोकल नेटवर्क पर receiver को
    एक सार्वजनिक पता या एक relay चाहिए।

## script से सर्वर को कॉल करें

API कुंजी किसी script को बिना पासवर्ड के सर्वर API कॉल करने देती है। कुंजियाँ एडमिन
**एडमिन → API कुंजियाँ → नई API कुंजी** में बनाता है, और वे उसी एडमिन की होती हैं।

- **कुंजी एक बार दिखती है**, जब बनाई जाती है। उसी समय उसे सहेज लें; वह फिर से नहीं
  दिखाई जा सकती, सिर्फ़ **रद्द करें** से रद्द की जा सकती है।
- **यह अपने मालिक की तरह काम करती है**, एडमिन के अधिकारों के साथ, अपने दायरे के
  अंदर।
- **दायरे (scope)** सीमित करते हैं कि कुंजी क्या कर सकती है:

| दायरा | क्या करने देता है |
| --- | --- |
| `media:read` | मीडिया का मेटाडेटा पढ़ना |
| `media:write` | मीडिया बदलना और स्कैन शुरू करना |
| `playback:read` | प्लेबैक की स्थिति और इतिहास पढ़ना |
| `playback:write` | प्लेबैक और प्रगति को नियंत्रित करना |
| `users:read` | उपयोगकर्ता पढ़ना |
| `users:write` | उपयोगकर्ता बदलना |
| `admin:read` | प्रशासन की गतिविधि और आँकड़े पढ़ना |
| `admin:write` | प्रशासन की सेटिंग्स बदलना |

बिना किसी दायरे के बनी कुंजी (**सभी (लेगेसी)**) के पास गैर-प्रशासनिक method पर
पूरी पहुँच होती है, अपने मालिक की तरह, पर प्रशासन वाले method हमेशा अपना दायरा
माँगते हैं: लेगेसी कुंजी उन तक नहीं पहुँच सकती। script को जिन दायरों की ज़रूरत है,
वही चुनें।

कुंजी `Authorization` header में जाती है, `Bearer` के साथ या उसके बिना:

```bash
curl -H "Authorization: Bearer evey_..." https://media.example.com/api/v1/...
```

कुंजी रद्द होते ही, या उसका मालिक अक्षम होते ही, तुरंत काम करना बंद कर देती है।
endpoint खुद [सर्वर API](api/README.md) में दिए गए हैं, पहचान के दूसरे तरीकों के
साथ।

<!-- sources
form labels: i18n admin.webhooks.form.* (name, url, secret, events, allEvents), admin.webhooks.createWebhook, common.test, admin.webhooks.testPingSent
event list: frontend/packages/client/src/lib/webhookEvents.ts - WEBHOOK_EVENT_TYPES ; EventName() in internal/domain ; plugin events: internal/adapters/driving/pluginhost/host_server.go - pluginEmittedEvent.EventName
delivery, headers, timeout, retries, signature: internal/app/webhook/service.go - handleAny, attemptDeliver, sign, defaultRetryDelays, deliveryTimeout ; internal/domain/webhook/model.go - MatchesEvent ; history: proto/evey/v1/webhook.proto - ListWebhookDeliveries
public addresses only: internal/pkg/urlvalidator/validator.go - ValidateWebhookURL, isPrivateIP ; internal/pkg/ssrf/dialer.go
api keys: internal/adapters/driving/grpcapi/interceptors/auth.go - authenticateAPIKey (Bearer optional, evey_ prefix), authorize (admin methods always enforce the scope) ; internal/pkg/auth/rbac.go (scopes) ; rbac.go - CreateAPIKey RoleAdmin ; i18n auth.apiKeys.* (newKeyTitle, scopesAll, saveWarning, revoke)
-->
