---
source_sha: 3c53c703905fd2f00b009c44b39fbc9ac215bbc99556a2b57fda6856122bdae1
reviewed: true
---
# प्लगइन API संदर्भ { #plugin-api-reference }

वह अनुबंध जिसके हिसाब से प्लगइन लिखा जाता है: `plugin.yaml` के फ़ील्ड, वे दो gRPC
सर्विस जिनसे बैकएंड और सर्वर आपस में संवाद करते हैं, और `window.evey` bridge, जो
इंटरफ़ेस मॉड्यूल को उसके फ़्रेम में मिलता है। इनका इस्तेमाल करने वाला पूरा उदाहरण
[प्लगइन लिखना](dev-setup.md) में है; हर अनुमति क्या करने देती है और इंस्टॉलर क्या
मना करता है, यह [अनुमतियाँ और स्लॉट](security-model.md) में है।

## Manifest { #manifest }

हर ZIP के root फ़ोल्डर में एक `plugin.yaml` (या `plugin.yml`) होता है, ज़्यादा से ज़्यादा
1 MB का।

```yaml title="plugin.yaml"
name: my-plugin              # ज़रूरी; अनोखा, kebab-case
version: 1.2.3               # ज़रूरी; semver, X.Y.Z या X.Y.Z-label
kind: plugin                 # ज़रूरी; "plugin" या "theme"
language: nodejs             # प्लगइन के लिए ज़रूरी; कॉपी होने वाले stub चुनता है
entrypoint: node index.js    # प्लगइन के लिए ज़रूरी; वह कमांड लाइन जो सर्वर चलाता है

display_name: My Plugin      # वैकल्पिक; एक स्ट्रिंग, या एक map {en: ..., fr: ...}
description: "..."           # वैकल्पिक
author: "..."                # वैकल्पिक
min_evey_version: "1.0.0"    # वैकल्पिक; सर्वर के वर्शन से ऊपर हो तो इंस्टॉल/अपडेट मना

permissions:                 # बैकएंड और इंटरफ़ेस क्या कॉल कर सकते हैं
  - network
  - media.read
external_fetch:              # "network" के साथ ज़रूरी; सिर्फ़ होस्ट के नाम
  - api.themoviedb.org
capabilities:                # वे भूमिकाएँ जो प्लगइन निभाने का दावा करता है
  - metadata.provider

web_root: public/            # frontend बताया हो तो ज़रूरी; static फ़ाइलों के रूप में दिया जाता है
frontend:
  - slot: home.section       # अनुमति वाले स्लॉट में से एक
    src: public/front.js     # web_root के अंदर होना चाहिए

config_schema:               # प्लगइन के कार्ड पर कॉन्फ़िग पैनल के फ़ील्ड
  api_key:
    type: password           # किसी भी क्लाइंट को कभी वापस नहीं भेजा जाता
    label: API key           # दिखाया नहीं जाता: पैनल फ़ील्ड को उनकी key से नाम देता है
    required: true
  city:
    type: string
    default: Paris
    description: Resolved through geocoding
```

| फ़ील्ड | नियम |
|---|---|
| `name`, `version`, `kind` | ज़रूरी। `version` `X.Y.Z` या `X.Y.Z-label` जैसा होना चाहिए; अपडेट का वर्ज़न इंस्टॉल किए गए वर्ज़न से ऊँचा होना चाहिए। |
| `display_name` | वह नाम जो लोग देखते हैं; `name` वही पहचान रहता है जिससे अपडेट को उसके plugin से जोड़ा जाता है। एक अकेली स्ट्रिंग अंग्रेज़ी मानी जाती है और सभी भाषाओं के लिए चलती है; map की keys भाषा के कोड होते हैं (`en`, `fr`, `pt-BR`)। हर मान 1 से 40 अक्षरों का होता है, आगे-पीछे खाली जगह या control अक्षरों के बिना। अभी इसे थीम चुनने वाली जगह दिखाती है। |
| `language` | `kind: plugin` के लिए ज़रूरी, कोई भी मान। इससे तय होता है कि कौन से stub entrypoint के बगल में कॉपी हों (`sdk/<language>/` वाले)। shebang से शुरू होने वाली स्क्रिप्ट (`entrypoint: main.py`) के लिए `python`, `nodejs`, `php`, `ruby`, `lua`, `bash` और `sh` में इमेज में वह interpreter भी चाहिए। यह कुछ शुरू नहीं करता। |
| `entrypoint` | `kind: plugin` के लिए ज़रूरी। पहला शब्द पैकेज की कोई फ़ाइल है (binary, `native-binary` के साथ, या स्क्रिप्ट) या फिर इमेज की कोई कमांड जो उसके `PATH` में मिलती है (`node`, `python3`, `java`, `dotnet`), बाकी उसके arguments; कमांड न मिले तो इंस्टॉल मना हो जाता है। प्रोसेस प्लगइन के फ़ोल्डर में चलता है। |
| `permissions` | हर मान [अनुमतियों की सूची](security-model.md#permissions-catalog) में होना चाहिए। `network` के लिए `external_fetch` खाली नहीं होना चाहिए। |
| `external_fetch` | होस्ट के नाम। scheme और अंत का path पार्स के समय हटा दिए जाते हैं; होस्ट ठीक-ठीक मिलने चाहिए, `*.example.com` जैसा wildcard इंस्टॉल पर मना हो जाता है; बिना port वाली एंट्री सिर्फ़ port 80 और 443 की अनुमति देती है, `host:port` सिर्फ़ उसी port की; IP लिखने पर इंस्टॉल मना होता है ([नियम](security-model.md#external-fetch-rules))। |
| `capabilities` | `metadata.provider`, `media.read`, `media.write`, `webhook.receiver` या `notification.sender`; बाकी सब मना है। थीम कोई क्षमता नहीं बता सकती। |
| `web_root` | `frontend` खाली न हो तो ज़रूरी; relative, बिना `..`, बिना backslash। `.` प्लगइन का पूरा फ़ोल्डर देता है और सिर्फ़ एक चेतावनी लॉग करता है। |
| `frontend[]` | `slot` [स्लॉट की सूची](#slots) में होना चाहिए; `src` `web_root` के अंदर होना चाहिए। |
| `config_schema` | फ़ील्ड के नाम से `{type, label, description, required, default, min, max, options}` तक का map; `label` दिखाया नहीं जाता, पैनल फ़ील्ड को उनकी key से नाम देता है। टाइप: `string`, `text`, `password`, `boolean`, `integer`, `number`, `select` (`options: [{value, label}]` के साथ)। |

थीम (`kind: theme`) `language`, `entrypoint`, `permissions` या `capabilities` नहीं
बता सकती; उसका फ़ॉर्मैट [थीम](../themes.md) में है।

**Secret।** `type: password` वाला फ़ील्ड, या `secret: true` वाला फ़ील्ड, सामान्य
तरीके से सहेजा जाता है और बैकएंड को दिया जाता है, पर पढ़ने वाले हर route से छिपा
हुआ लौटता है, एडमिन का **कॉन्फ़िग** पैनल भी। पैनल को छिपे हुए मान को बिना बदले
सहेजने पर सहेजा हुआ मान बना रहता है, इसलिए किसी दूसरे फ़ील्ड को बदलने से secret
कभी नहीं मिटता।

## बैकएंड: दो सर्विस { #backend-the-two-services }

बैकएंड प्रोसेस और सर्वर दो Unix socket पर gRPC से संवाद करते हैं, जिनके रास्ते
environment में मिलते हैं:

```
EVEY_PLUGIN_ID=<plugin id>
EVEY_PLUGIN_SOCK=<path>   # आपका प्रोसेस यहाँ सुनता है और PluginService देता है
EVEY_HOST_SOCK=<path>     # आपका प्रोसेस HostService को कॉल करने के लिए यहाँ जुड़ता है
```

सर्वर अपने socket पर प्रोसेस को उस PID से पहचानता है जो kernel बताता है
(`SO_PEERCRED`); प्रोसेस को कोई token नहीं दिया जाता। हर प्लगइन के लिए कॉल 600
प्रति मिनट तक सीमित हैं। `proto/plugin/v1/plugin_api.proto` अनुबंध है; साथ आने
वाली भाषाओं के stub प्लगइन शुरू होने पर entrypoint के बगल में कॉपी हो जाते हैं
([पहले से क्या चाहिए](dev-setup.md#1-prerequisites))।

### PluginService: सर्वर क्या कॉल करता है { #pluginservice-what-the-server-calls }

| Method | कब |
|---|---|
| `Initialize(plugin_id, config, host_version)` | जब प्रोसेस अपने socket से जुड़ जाए, एक बार। `config` में `config_schema` के वे मान होते हैं जो एडमिन ने सेट किए। शुरू होने से मना करने के लिए `error` के साथ `success: false` लौटाएँ। |
| `HealthCheck()` | समय-समय पर, ताकि चलता हुआ प्रोसेस अटके हुए प्रोसेस से अलग पहचाना जा सके। जब तक आप जवाब दे सकते हैं, `healthy: true` लौटाएँ। |
| `Shutdown(reason)` | जब प्लगइन अक्षम, अपडेट या अनइंस्टॉल हो, या सर्वर रुके। जवाब देने के बाद जल्दी बंद हो जाएँ। |
| `OnEvent(event_type, payload, source)` | सिर्फ़ `metadata.request`, उस प्लगइन को जिसके पास `metadata.provider` क्षमता हो (नीचे [मेटाडेटा अनुरोध](#metadata-requests))। किसी दूसरे प्लगइन को कोई event नहीं मिलता: `handled: false` लौटाएँ। |
| `GetCapabilities()` | अनुबंध में है; सर्वर आज इसे कॉल नहीं करता। |

### HostService: प्लगइन क्या कॉल कर सकता है { #hostservice-what-the-plugin-can-call }

जिस कॉल की अनुमति manifest में नहीं बताई गई, वह `PermissionDenied` के साथ मना हो
जाती है। *दिया नहीं जाता* वाले method `Unimplemented` लौटाते हैं; वे आगे के किसी
वर्ज़न के अनुबंध का हिस्सा हैं, और यहाँ इसलिए लिखे हैं ताकि किसी stub से आपको
गलतफ़हमी न हो।

| Method | अनुमति | टिप्पणी |
|---|---|---|
| `Log(level, message, fields)` | कोई नहीं | प्लगइन के लॉग में लिखा जाता है, जो उसके कार्ड पर **लॉग** में दिखता है। |
| `GetConfig(key)` / `SetConfig(key, value)` | कोई नहीं | प्लगइन का कॉन्फ़िगरेशन: **कॉन्फ़िग** पैनल के मान, और वे कुंजियाँ जो प्लगइन खुद लिखता है। ज़्यादा से ज़्यादा 256 कुंजियाँ, 256 बाइट की कुंजी, 16 KiB का मान। |
| `GetData`, `SetData`, `DeleteData`, `ListDataKeys` | कोई नहीं | नीचे वाला [डेटा स्टोर](#data-store)। |
| `EmitEvent(event_type, payload)` | कोई नहीं | सर्वर की event bus पर `plugin.event.<event_type>` के रूप में प्रकाशित होता है, जहाँ वेबहुक इसकी सदस्यता ले सकते हैं। |
| `GetMediaItem(id)`, `ListMediaItems(library_id, type, parent_id, limit, offset)` | `api.library` या `media.read` | मीडिया का पूरा रिकॉर्ड, `file_path`, `provider_ids` और `genres` के साथ। |
| `Fetch(url, method, headers)` | `network` | सैंडबॉक्स से बाहर जाने का अकेला रास्ता: `http` या `https`, `GET` या `HEAD`, `external_fetch` का कोई होस्ट; पता निकालने के बाद और redirect में भी निजी पते मना हैं। body 8 MiB तक सीमित है (`truncated` यह बताता है) और पूरा request 20 सेकंड तक। सर्वर अपनी तरफ़ से कोई credential नहीं जोड़ता। |
| `SearchMedia`, `GetMediaStreams`, `GetChapters`, `GetSubtitles` | `api.library` या `media.read` | *दिया नहीं जाता।* |
| `UpdateMetadata(media_item_id, fields, provider_ids)` | `api.metadata` या `media.write`, और `metadata.provider` क्षमता | बिना पूछे किसी फ़िल्म या सीरीज़ के लिए आपका टेक्स्ट देता है, जैसे [मेटाडेटा जवाब](#metadata-requests) देता। `fields` में `locale` (ज़रूरी) और `title`, `overview`, `tagline` में से कोई भी होता है; टेक्स्ट उस भाषा के लिए आपके पहले दिए टेक्स्ट की जगह लेता है, और सिर्फ़ `locale` वाला `fields` उसे हटा देता है। `provider_ids` मना है: आइटम की पहचान TMDB करता है। एक प्लगइन जो सहेजता है उसकी सीमा है: हर आइटम पर 16 भाषाएँ, कुल 200 000 टेक्स्ट और 128 MiB। इससे ऊपर कॉल `RESOURCE_EXHAUSTED` लौटाता है; पहले दिया गया टेक्स्ट बदलना या हटाना फिर भी संभव रहता है। |
| `GetUserPreferences` | `api.user` | *दिया नहीं जाता।* |
| `SendNotification` | `notification.sender` | *दिया नहीं जाता।* |

### मेटाडेटा अनुरोध { #metadata-requests }

जब भी सर्वर TMDB से किसी फ़िल्म या सीरीज़ का मिलान या रीफ़्रेश करता है, वह हर
सक्षम `metadata.provider` प्लगइन को `event_type` `metadata.request` वाला
`OnEvent` भेजता है, एक के बाद एक प्लगइन को, और हर एक के लिए 15 सेकंड तक इंतज़ार
करता है। सामग्री:

| फ़ील्ड | सामग्री |
|---|---|
| `media_item_id` | आइटम की ID, बाद में `UpdateMetadata` के लिए। |
| `type` | `movie` या `series`। |
| `title`, `original_title` | सर्वर की भाषा में TMDB का शीर्षक, और मूल शीर्षक। |
| `year` | पता न होने पर मौजूद नहीं होता। |
| `provider_ids` | `tmdb`, `imdb` और आइटम की दूसरी ज्ञात ID। |
| `language` | TMDB के टेक्स्ट की भाषा, जैसे `fr`। |
| `locales` | सर्वर जो भी भाषाएँ दिखाता है। |

`handled: true` के साथ इस रूप का `result` लौटाएँ:

```json
{
  "locale": "fr",
  "title": "...",
  "overview": "...",
  "tagline": "...",
  "translations": {
    "de": { "overview": "..." }
  }
}
```

ऊपरी स्तर के फ़ील्ड `locale` की भाषा में होते हैं, या `locale` न हो तो अनुरोध
की `language` में; `translations` दूसरी भाषाएँ जोड़ता है। जो फ़ील्ड आप नहीं देते,
उसे छोड़ दें। भाषा टैग सिर्फ़ भाषा तक छोटे किए जाते हैं (`fr-FR` को `fr` पढ़ा
जाता है), और 8 KiB से बड़ा फ़ील्ड अनदेखा किया जाता है। एक जवाब उस आइटम के लिए
आपके पहले दिए सब कुछ की जगह लेता है; `handled: false`, या बिना टेक्स्ट वाला
जवाब, बताता है कि आप आइटम को नहीं जानते और आपका दिया टेक्स्ट मिटा देता है। कोई error या
समय सीमा पार होने पर आपका पहले दिया टेक्स्ट बना रहता है।

एडमिन चुनता है कि आपका टेक्स्ट `title`, `overview` और `tagline` में से किनकी
जगह ले, और आपसे एक साथ पूरी लाइब्रेरी के बारे में पूछ सकता है
([मेटाडेटा प्लगइन](../plugins.md#metadata-plugins))। सीज़न और एपिसोड नहीं भेजे
जाते।

### डेटा स्टोर { #data-store }

हर प्लगइन के पास एक स्थायी key/value स्टोर होता है, बिना कोई अनुमति बताए: cursor,
cache, आखिरी sync का समय, कुछ भी जो restart के बाद भी याद रखना हो। मान ऐसी
string हैं जिन्हें सर्वर नहीं पढ़ता; JSON खुद encode करें। प्लगइन की id पहचाने गए socket से आती है, इसलिए
प्लगइन सिर्फ़ अपने हिस्से में पढ़ता और लिखता है, और पूरा स्टोर प्लगइन के साथ
मिटा दिया जाता है।

| Method | काम |
|---|---|
| `SetData(key, value)` | मान बनाना या बदलना। |
| `GetData(key)` | `value` और `found` लौटाता है। |
| `DeleteData(key)` | कुंजी हटाना। |
| `ListDataKeys()` | प्लगइन की कुंजियाँ। |

यह स्टोर पूरे प्लगइन के लिए एक है। **हर उपयोगकर्ता का** डेटा (विजेट देखने वाले
व्यक्ति की कोई पसंद) इंटरफ़ेस की तरफ़ रहता है, नीचे दी गई `prefs` अनुमति के ज़रिए।

## Frontend API { #frontend-api }

इंटरफ़ेस मॉड्यूल एक opaque origin वाले फ़्रेम में चलता है, `default-src 'none'` नीति
के साथ: होस्ट पेज, उसकी कुकी या उसके स्टोरेज तक कोई पहुँच नहीं, कोई फ़ॉर्म नहीं,
कोई popup नहीं, और `connect-src` सिर्फ़ `https` पर `external_fetch` के होस्ट तक
सीमित। बाकी सब `window.evey` से होकर जाता है, जो मॉड्यूल चलने से पहले डाला जाता
है। वही मॉड्यूल बिना बदले वेब ऐप में और मोबाइल और टीवी ऐप में चलता है।

### `evey.onInit(callback)` { #eveyoninitcallback }

होस्ट जो context भेजता है, उसके साथ एक बार कॉल होता है; context आने के बाद
रजिस्टर किया गया callback तुरंत कॉल होता है।

```js
evey.onInit(function (data) {
  // data.config          - प्लगइन के कॉन्फ़िगरेशन मान (config_schema)
  // data.context         - होस्ट का संदर्भ
  // data.locale          - चालू भाषा, जैसे "en-US"
  // data.theme           - "dark" या "light"
  // data.surface         - "web", "mobile" या "tv"
  // data.suggestedHeight - सिर्फ़ home.section: होस्ट की सुझाई ऊँचाई (px)
  // data.viewportHeight  - सिर्फ़ tv: पैनल की ऊँचाई, dp में
});
```

### `evey.navigate(path)` { #eveynavigatepath }

होस्ट ऐप्लिकेशन को `path` पर ले जाता है। सिर्फ़ `/home`, `/library`, `/media`,
`/player`, `/settings` और `/search` से शुरू होने वाले रास्ते माने जाते हैं; बाकी
सब (एडमिन वाला हिस्सा, साइन-इन, पूरा URL) चुपचाप छोड़ दिया जाता है।

### `evey.openExternal(url)` { #eveyopenexternalurl }

किसी सार्वजनिक कैटलॉग पेज को ब्राउज़र के नए टैब में खोलता है: `themoviedb.org` या
`imdb.com` का कोई `https` पता, इसके अलावा कुछ नहीं (कोई और पता चुपचाप छोड़ दिया
जाता है)। टैब सिर्फ़ प्लगइन के फ़्रेम में क्लिक के बाद खुलता है, इसलिए कोई प्लगइन
अपने आप पेज नहीं खोल सकता। सिर्फ़ वेब पर: फ़ोन और TV ऐप इसे अनदेखा करते हैं।
पुराने Evey में यह कॉल नहीं है; लिंक दिखाने से पहले
`typeof evey.openExternal === "function"` जाँच लें।

### `evey.toast(level, message)` { #eveytoastlevel-message }

होस्ट इंटरफ़ेस में एक toast दिखाता है। `level` `"success"`, `"error"` या `"info"`
है; `message` सादा टेक्स्ट है, 200 अक्षरों पर काटा जाता है; एक सेकंड में ज़्यादा
से ज़्यादा 3 toast दिखते हैं।

### `evey.resize(height)` { #eveyresizeheight }

स्लॉट को `height` पिक्सेल ऊँचा करने को कहता है, 50 और 800 के बीच रखा जाता है।
`0` स्लॉट को समेट देता है: जिस विजेट के पास दिखाने को कुछ नहीं, वह जगह नहीं
लेता। `home.section` विजेट बिना कुछ किए `data.suggestedHeight` पर खुलता है: होस्ट
जानता है कि उसने आपको किस जगह रखा है और वहाँ होम विजेट की जो ऊँचाई होती है वही
सुझाता है, ताकि हर विजेट एक ही आकार में दिखे। आकार तभी बदलें जब आपके पास इससे
बेहतर जानकारी हो।

### `evey.prefs.*` (अनुमति `prefs`) { #eveyprefs-permission-prefs }

मौजूदा उपयोगकर्ता की पसंद, प्लगइन के अपने हिस्से में, उस token के ज़रिए (यह प्लगइन, यह
उपयोगकर्ता) से जुड़ी हुई जो होस्ट फ़्रेम के लिए रखता है। हर कॉल एक promise
लौटाती है; अनुमति न हो तो वह reject होता है।

```js
await evey.prefs.set("units", "celsius");
const { value, found } = await evey.prefs.get("units");
const all = await evey.prefs.list();        // [{ key, value }]
await evey.prefs.delete("units");
```

### `evey.media.*` (अनुमति `public`) { #eveymedia-permission-public }

मीडिया आइटम की एक सुरक्षित झलक: id, टाइटल, टाइप, साल, सारांश, पोस्टर का URL।
साथ में फ़िल्म या सीरीज़ की सार्वजनिक कैटलॉग id (`tmdbId`, अज्ञात हो तो `0`, और
`imdbId`, अज्ञात हो तो खाली), ताकि प्लगइन उसके TMDB या IMDb पेज का लिंक दे सके।
कभी stream नहीं, और फ़ाइल का रास्ता या आकार भी नहीं, जब तक प्लगइन के पास
`media.files` भी न हो (नीचे देखें)।

```js
const item  = await evey.media.get(id);
const page  = await evey.media.list({ libraryId });  // { items, total }
const data  = await evey.media.poster(id);  // पोस्टर, data: URL के रूप में
```

`list` ज़्यादा से ज़्यादा 500 आइटम का एक पेज लौटाता है, सारे टाइप मिले-जुले
(सीरीज़, सीज़न, एपिसोड...), जब तक `type` उसे सीमित न करे। जिस प्लगइन को पूरी
लाइब्रेरी चाहिए, वह एक टाइप माँगता है और पेज दर पेज तब तक चलता है जब तक उसके पास
`total` आइटम न हो जाएँ:

```js
const page = await evey.media.list({ type: "MEDIA_ITEM_TYPE_MOVIE", page: 2, pageSize: 500 });
```

`media.files` अनुमति के साथ, और सिर्फ़ तब जब देखने वाला एडमिन हो, हर आइटम में
`filePath`, `fileSize` (एक स्ट्रिंग, API के हर 64-बिट नंबर की तरह) और `quality` भी
होते हैं: `width`, `height`, `videoCodec`, `hdrType` (SDR में खाली), `bitDepth`,
`bitrate` (बिट प्रति सेकंड), `durationMs`, `container`, `audioCodec` और
`audioChannels`, पहले वीडियो ट्रैक और डिफ़ॉल्ट ऑडियो ट्रैक से।
बाकी सबके लिए टोकन इस अनुमति के बिना बनता है और दोनों फ़ील्ड खाली रहते हैं: जो
मॉड्यूल सबको दिखता है वह इन पर भरोसा न करे, इनकी जगह `admin.nav` स्लॉट है।

`poster` एक `data:` URL लौटाता है, क्योंकि फ़्रेम की नीति सर्वर से सीधे तस्वीर
लोड करने से मना करती है; इसे `<img>` के स्रोत या texture की तरह इस्तेमाल करें।

### `evey.media.gaps()` (अनुमति `media.gaps`) { #eveymediagaps-permission-mediagaps }

लाइब्रेरी में क्या कम है, Evey का निकाला हुआ: मौजूद सीरीज़ के पहले से प्रसारित
एपिसोड जो नहीं हैं, और शुरू की गई सीरीज़-फ़िल्मों (saga) की रिलीज़ हो चुकी फ़िल्में
जो नहीं हैं। कोई सीरीज़ या saga क्या रखती है, यह Evey ख़ुद TMDB से अपनी रफ़्तार
से जानता है (सहेजा जवाब एक हफ़्ते बाद नया होता है); प्लगइन कभी TMDB को कॉल नहीं
करता, वह नतीजा पढ़ता है।

```js
const { series, sagas } = await evey.media.gaps();
// series: [{ seriesId, title, year, tmdbId, imdbId, fromCatalog, seasons: [{ season,
//   whole, episodes: [{ episode, title, airDate }] }] }]
// sagas: [{ collectionId, name, tmdbId, owned,
//   missing: [{ title, releaseDate, posterUrl, tmdbId }] }]
```

सिर्फ़ जो आ चुका है वही गिना जाता है: बिना तारीख़ का या आज के बाद की तारीख़ वाला
एपिसोड या फ़िल्म अभी कम नहीं है, और स्पेशल (सीज़न 0) अलग रखे जाते हैं। जिस
सीरीज़ को TMDB नहीं जानता, वह अपनी ही नंबरिंग के छेदों से दिखती है
(`fromCatalog` false): सबसे ऊँचे मौजूद एपिसोड से नीचे के कम एपिसोड, सबसे ऊँचे
मौजूद सीज़न से नीचे के कम सीज़न (`whole`, बिना एपिसोड सूची)। सिर्फ़ एडमिन के
सेशन पर, `media.files` की तरह।

`tmdbId` (सीरीज़, सागा, हर कम फ़िल्म) और `imdbId` (सीरीज़) सार्वजनिक कैटलॉग id हैं,
अज्ञात हों तो `0` या खाली, ताकि कोई निष्कर्ष अपने TMDB या IMDb पेज से जोड़ा जा सके।

### `evey.media.trash(ids)` (अनुमति `media.trash`, सिर्फ़ स्लॉट `admin.nav`) { #eveymediatrashids }

Evey से मीडिया फ़ाइलों को उनकी लाइब्रेरी के ट्रैश में ले जाने को कहता है। प्लगइन
सिर्फ़ माँगता है: Evey अपनी ही पुष्टि दिखाता है, हर आइटम के अपने रिकॉर्ड से
(टाइटल, साल, रास्ता, आकार, क्वालिटी), प्लगइन के भेजे से कभी नहीं, और जब तक एडमिन
वहाँ हाँ न कहे, कुछ नहीं हिलता।

```js
const res = await evey.media.trash([id1, id2]);
// { trashed: [{ id, trashedPath, pending }], failed: [{ id, message }] }
```

हाँ कहने पर हर फ़ाइल, अपने सबटाइटल और `.nfo` के साथ, अपनी लाइब्रेरी की जड़ में
`.evey-trash` में उसी relative रास्ते के नीचे चली जाती है, और आइटम Evey से हट जाता
है। डिस्क से कुछ नहीं मिटता: एडमिन वह फ़ोल्डर खाली करता है, और वापस रखी गई फ़ाइल
अगले स्कैन में लौट आती है। सिर्फ़ फ़िल्म या एपिसोड: सीरीज़ एक फ़ोल्डर है। एडमिन के
ना कहने पर promise `cancelled` के साथ reject होता है, और ब्राउज़र में `media.trash`
वाले प्लगइन के `admin.nav` पेज के अलावा कहीं भी `method not permitted` के साथ (फ़ोन
और TV ऐप अभी यह नहीं देते)। किसी एक फ़ाइल का इनकार (रीड-ओनली माउंट, फ़ाइल पहले
ही गायब) `failed` में लौटता है। `pending` तब true होता है जब फ़ाइल बैकग्राउंड में किसी
दूसरे डिस्क के ट्रैश फ़ोल्डर में कॉपी हो रही हो: कॉपी पूरी होने तक आइटम
`evey.media.list` में रहता है, और एडमिन को हर हाल में सूचना मिलती है।

`evey.media.restore(paths)` इसे उसी नियम के तहत पलटता है: `trashedPath` मानों से Evey
हर फ़ाइल और उसके सबटाइटल को उनकी जगह वापस रखता है और लाइब्रेरी स्कैन करता है, ताकि
टाइटल लौट आए, एक नई एंट्री के रूप में जो नाम से दोबारा पहचानी जाती है (पुरानी एंट्री
की प्लेबैक स्थिति वापस नहीं आती)। नतीजा `{ restored: [{ trashedPath, restoredPath }],
failed: [...] }` है; Evey लाइब्रेरी ट्रैश के बाहर का कोई भी रास्ता, और दोबारा भर चुकी
जगह मना करता है। कोई पुष्टि नहीं: यह कुछ नष्ट नहीं करता।

### `evey.assets.read(path)` { #eveyassetsreadpath }

प्लगइन की अपनी `web_root` की कोई फ़ाइल, relative रास्ते से, `data:` URL के रूप
में पढ़ता है: जो फ़ाइल बैकएंड ने चलते समय लिखी, वह cross-origin की चिंता के बिना
फ़्रेम में इस्तेमाल हो सकती है। सिर्फ़ प्लगइन की अपनी फ़ाइलें पहुँच में हैं।

### `evey.config.save(values)` (सिर्फ़ स्लॉट `admin.plugin.config` में) { #eveyconfigsavevalues }

प्लगइन का सर्वर config लिखता है, यानी वे मान जो कोई एडमिन `config_schema` के
हिसाब से तय करता है। दी गई कुंजियाँ सहेजे गए config में मिला दी जाती हैं, बाकी
कुंजियाँ जैसी हैं वैसी रहती हैं, और सर्वर नतीजे को schema के विरुद्ध ठीक वैसे ही
जाँचता है जैसे बने-बनाए फ़ॉर्म को। मान स्ट्रिंग होते हैं, जैसे फ़ॉर्म में। promise
`{ values }` के साथ पूरा होता है, यानी सहेजा गया config (गोपनीय फ़ील्ड छिपे हुए),
और किसी मान के अस्वीकार होने पर सर्वर के संदेश के साथ reject होता है।
`admin.plugin.config` के अलावा कहीं भी यह कॉल reject हो जाती है।

```js
await evey.config.save({ city: "Brest", lat: "48.39", lon: "-4.49" });
```

मौजूदा मान `onInit` में `config` के रूप में आते हैं, हर स्लॉट की तरह।

### टीवी पर { #on-a-television }

टीवी ऐप मॉड्यूल को एक WebView में चलाता है, और कुछ बातें जो ब्राउज़र में सच हैं,
वहाँ सच नहीं रहतीं:

- **अपने viewport से आकार तय न करें।** WebView के अंदर `window.innerHeight` वह
  स्लॉट है जो होस्ट ने अभी आपको दिया। आकार `data.viewportHeight` या
  `data.suggestedHeight` से तय करें।
- **न focus, न pointer।** विजेट को कभी D-pad का focus नहीं मिलता और hover नहीं
  होता; जो भी interactive है, वह वेब और मोबाइल के लिए है।
- **टेक्स्ट किनारों से दूर रखें।** टीवी overscan करते हैं: हर तरफ़ चौड़ाई और
  ऊँचाई का लगभग 5% खाली छोड़ें।
- **WebGL असली डिवाइस पर परखें।** shader डिवाइस के GPU और driver पर चलता है,
  आपके कंप्यूटर पर नहीं; noise के coordinates सीमित रखें।
  [eveyproject/evey-plugin-weather](https://github.com/eveyproject/evey-plugin-weather) ऊपर की सभी बातों के लिए नमूने वाला विजेट है।

## स्लॉट { #slots }

| स्लॉट | कहाँ दिखता है |
|---|---|
| `home.section` | होम पेज पर कंटेंट का एक सेक्शन |
| `home.hero` | होम पेज का सबसे ऊपर वाला बड़ा बैनर |
| `library.sidebar` | लाइब्रेरी वाले पेजों में साइड पैनल |
| `media.detail.sidebar` | मीडिया के जानकारी वाले पेज पर साइड पैनल |
| `player.overlay` | वीडियो प्लेयर के ऊपर एक परत |
| `settings.user.appearance` | उपयोगकर्ता सेटिंग्स का रूप-रंग वाला हिस्सा |
| `sidebar.bottom` | मुख्य नेविगेशन साइडबार का निचला हिस्सा |
| `admin.nav` | एडमिन मेन्यू में एक एंट्री, जो प्लगइन का अपना पेज खोलती है |
| `admin.plugin.config` | प्लगइन का अपना config पैनल, एडमिन की प्लगइन स्क्रीन पर उसके कार्ड में |

एडमिन वाले हिस्से में सिर्फ़ `admin.nav` और `admin.plugin.config` स्लॉट हैं। दोनों में
से कोई भी मौजूदा एडमिन स्क्रीन में कुछ नहीं डालता: `admin.nav` एक मेन्यू एंट्री जोड़ता
है जो प्लगइन का पेज उसी सैंडबॉक्स वाले फ़्रेम में खोलती है, एक ऐसे route पर जो सिर्फ़
एडमिन के लिए है; `admin.plugin.config` `config_schema` से बने फ़ॉर्म की जगह लेता है,
प्लगइन के अपने कार्ड के अंदर, ऐसे मान के लिए जिसे साधारण फ़ील्ड ठीक से नहीं चुन
पाता (जैसे एक ही नाम के शहरों में से कोई एक)। इनमें से किसी को भी माँगना मंज़ूरी
वाले फ़र्क में दिखता है। बाकी हर `admin.*`, `login.*`, `setup.*` और
`profile.security.*` स्लॉट इंस्टॉल के समय मना होता है।

## क्षमताएँ { #capabilities }

क्षमता वह भूमिका बताती है जो प्लगइन निभाने का दावा करता है: `metadata.provider`,
`media.read`, `media.write`, `webhook.receiver` या `notification.sender`। इंस्टॉलर
नाम जाँचता है, एडमिन पूर्वावलोकन में सूची देखता है और अपडेट पर हर नई क्षमता
देखता है, और सुरक्षा डैशबोर्ड उसे दर्ज करता है। सर्वर इनमें से एक के आधार पर
भेजता है: `metadata.provider` को [मेटाडेटा अनुरोध](#metadata-requests) मिलते हैं
और वह `UpdateMetadata` कॉल कर सकता है। बाकी आज कुछ नहीं भेजतीं;
`webhook.receiver` को कोई आने वाला request नहीं मिलता। प्लगइन असल में क्या कर
सकता है, यह उसकी **अनुमतियाँ** तय करती हैं।

## इंटरफ़ेस की अनुमतियाँ { #permissions-of-the-interface }

`public` और `prefs` बाकी अनुमतियों की तरह `permissions` में बताई जाती हैं। ये
बैकएंड प्रोसेस को कुछ नहीं देतीं; ये फ़्रेम में `evey.media.*` और `evey.prefs.*`
खोलती हैं। फ़्रेम के पास कभी token नहीं होता: होस्ट (प्लगइन, उपयोगकर्ता) तक
सीमित पाँच मिनट का एक token बनाता है और कॉल आगे भेजता है, और सर्वर डिफ़ॉल्ट रूप
से हर वह method मना करता है जिसका नाम token के दायरे में नहीं है
([ADR-0012](https://github.com/eveyproject/evey/blob/master/docs/adr/0012-plugin-token-deny-by-default.md))।

!!! note "प्लगइन कभी क्या नहीं माँग सकता"
    `admin.*`, `auth.*`, `system.*`, `user.write` और `library.write` पूरी तरह बंद
    हैं: इनका नाम लेने वाला manifest मना होता है, कोई token इन्हें कभी नहीं ले
    जा सकता, और एडमिन method प्लगइन token को मना करते हैं, उसका दायरा कुछ भी हो।

## उदाहरण { #examples }

`examples/plugins/` में मौजूदा manifest फ़ॉर्मैट वाले चलते प्लगइन हैं: Node.js
और Python में शुरुआती प्लगइन (`nodejs-example`, `python-example`), एक मेटाडेटा
provider (`metadata-example`), एक वेबहुक receiver (`webhook-receiver`), `public`
और `prefs` अनुमतियों और `admin.nav` स्लॉट के साथ डेटा स्टोर (`data-demo`) और
`config_schema` के हर फ़ील्ड टाइप का उदाहरण, `admin.nav` पेज के साथ
(`config-demo`)। `make plugin-zips` इन सबको पैक करता है। प्रोजेक्ट के अपने प्लगइन
और थीम - मौसम विजेट, CShell, Dark Blue और बाकी - हर एक [eveyproject](https://github.com/eveyproject) के तहत अपनी
अलग repository में रहते हैं।

<!-- sources
manifest fields, validation: internal/domain/plugin/model.go - PluginManifest, Validate, validateWebRoot, validateSrcUnderWebRoot, IsSecretConfigField ; internal/adapters/driven/pluginrunner/manifest.go - Parse, validateManifest, validateExternalFetch ; capabilities.go - KnownCapabilities, KnownPermissions
runtimes: internal/adapters/driven/pluginrunner/runtime_availability.go - CheckEntrypoint ; launchers: runner_start.go - resolveCommand ; launcher_resolve.go - lookupLauncher
config field types: frontend/apps/web/src/features/admin/PluginCard.tsx ; examples/plugins/config-demo/plugin.yaml
secrets masked: internal/adapters/driving/grpcapi/admin_plugin.go (SecretMask)
env vars, Initialize call: internal/adapters/driven/pluginrunner/runner_start.go ; ipc_client.go - Initialize, Shutdown, HealthCheck, OnEvent ; runner_control.go - SendEvent (no caller)
peer auth, rate limit: internal/adapters/driving/pluginhost/host_server.go - makePluginAuthInterceptor, pluginRPCRatePerMin ; peer_creds.go
host RPCs and permissions: internal/adapters/driving/grpcapi/plugin_interceptor.go ; pluginhost/host_server.go - Log, GetConfig, SetConfig (maxConfigKeys, maxConfigKeyLen, maxConfigValueLen), EmitEvent (pluginEmittedEvent.EventName), GetMediaItem, ListMediaItems, mediaItemToProto, UpdateMetadata, GetUserPreferences (Unimplemented), UnimplementedHostServiceServer ; fetch.go - Fetch, maxFetchBytes, fetchTimeout
proto: proto/plugin/v1/plugin_api.proto - PluginService, HostService, InitRequest, FetchRequest, FetchResponse, MediaItem
frame policy: internal/adapters/driving/http/plugin_frame_handler.go - pluginFramePolicy ; frontend/packages/client/src/plugins/hostProtocol.ts - buildConnectSrc, ALLOWED_NAV_PREFIXES, SLOT_MAX_HEIGHT, clampSlotHeight, TOAST_MAX, TOAST_WINDOW_MS, TOAST_MAX_LENGTH ; frontend/packages/client/src/web/PluginIframe.tsx - MIN_SLOT_HEIGHT, suggestedSlotHeight ; frontend/packages/client/src/native/PluginWebView.tsx (surface)
bridge: frontend/packages/client/src/plugins/bridgeScript.ts - EveyBridge ; pluginRpc.ts - pluginRequestFor
slots: internal/domain/plugin/slots.go - AllowedSlots
token, deny by default: internal/app/plugin/token_service.go - PluginTokenTTL ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes, forbiddenPrefix ; docs/adr/0003-plugin-scoped-tokens.md ; docs/adr/0012-plugin-token-deny-by-default.md
examples: examples/plugins/* ; Makefile - plugin-zips
-->
