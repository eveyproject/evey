---
source_sha: 4f36e08eb2ed47e2b70b77f7f85f7caa2a9229e3bb755c85c2d54804d13c4031
reviewed: true
---
# प्लगइन API संदर्भ

वह अनुबंध जिसके हिसाब से प्लगइन लिखा जाता है: `plugin.yaml` के फ़ील्ड, वे दो gRPC
सर्विस जिनसे बैकएंड और सर्वर आपस में संवाद करते हैं, और `window.evey` bridge, जो
इंटरफ़ेस मॉड्यूल को उसके फ़्रेम में मिलता है। इनका इस्तेमाल करने वाला पूरा उदाहरण
[प्लगइन लिखना](dev-setup.md) में है; हर अनुमति क्या करने देती है और इंस्टॉलर क्या
मना करता है, यह [अनुमतियाँ और स्लॉट](security-model.md) में है।

## Manifest

हर ZIP के root फ़ोल्डर में एक `plugin.yaml` (या `plugin.yml`) होता है, ज़्यादा से ज़्यादा
1 MB का।

```yaml title="plugin.yaml"
name: my-plugin              # ज़रूरी; अनोखा, kebab-case
version: 1.2.3               # ज़रूरी; semver, X.Y.Z या X.Y.Z-label
kind: plugin                 # ज़रूरी; "plugin" या "theme"
language: nodejs             # प्लगइन के लिए ज़रूरी; इमेज का कोई runtime
entrypoint: node index.js    # प्लगइन के लिए ज़रूरी; वह कमांड लाइन जो सर्वर चलाता है

description: "..."           # वैकल्पिक
author: "..."                # वैकल्पिक
min_evey_version: "1.0.0"    # वैकल्पिक; दर्ज होता है, आज लागू नहीं होता

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
    label: API key
    required: true
  city:
    type: string
    default: Paris
    description: Resolved through geocoding
```

| फ़ील्ड | नियम |
|---|---|
| `name`, `version`, `kind` | ज़रूरी। `version` `X.Y.Z` या `X.Y.Z-label` जैसा होना चाहिए; अपडेट का वर्ज़न इंस्टॉल किए गए वर्ज़न से ऊँचा होना चाहिए। |
| `language` | `kind: plugin` के लिए ज़रूरी: `python`, `nodejs`, `php`, `ruby`, `lua`, `bash`, `sh` या `go`। इससे तय होता है कि कौन सा runtime जाँचा जाए और कौन से stub entrypoint के बगल में कॉपी हों; यह कुछ शुरू नहीं करता। |
| `entrypoint` | `kind: plugin` के लिए ज़रूरी। पहला शब्द interpreter है (या `native-binary` के साथ binary), बाकी उसके arguments; प्रोसेस प्लगइन के फ़ोल्डर में चलता है। |
| `permissions` | हर मान [अनुमतियों की सूची](security-model.md#permissions-catalog) में होना चाहिए। `network` के लिए `external_fetch` खाली नहीं होना चाहिए। |
| `external_fetch` | सिर्फ़ होस्ट के नाम। scheme स्वीकार किया जाता है और हटा दिया जाता है; wildcard या IP लिखने पर इंस्टॉल मना होता है ([नियम](security-model.md#external-fetch-rules))। |
| `capabilities` | `metadata.provider`, `media.read`, `media.write`, `webhook.receiver` या `notification.sender`; बाकी सब मना है। थीम कोई क्षमता नहीं बता सकती। |
| `web_root` | `frontend` खाली न हो तो ज़रूरी; relative, बिना `..`, बिना backslash। `.` प्लगइन का पूरा फ़ोल्डर देता है और सिर्फ़ एक चेतावनी लॉग करता है। |
| `frontend[]` | `slot` [स्लॉट की सूची](#slots) में होना चाहिए; `src` `web_root` के अंदर होना चाहिए। |
| `config_schema` | फ़ील्ड के नाम से `{type, label, description, required, default, min, max, options}` तक का map। टाइप: `string`, `text`, `password`, `boolean`, `integer`, `number`, `select` (`options: [{value, label}]` के साथ)। |

थीम (`kind: theme`) `language`, `entrypoint`, `permissions` या `capabilities` नहीं
बता सकती; उसका फ़ॉर्मैट [थीम](../themes.md) में है।

**Secret।** `type: password` वाला फ़ील्ड, या `secret: true` वाला फ़ील्ड, सामान्य
तरीके से सहेजा जाता है और बैकएंड को दिया जाता है, पर पढ़ने वाले हर route से छिपा
हुआ लौटता है, एडमिन का **कॉन्फ़िग** पैनल भी। पैनल को छिपे हुए मान को बिना बदले
सहेजने पर सहेजा हुआ मान बना रहता है, इसलिए किसी दूसरे फ़ील्ड को बदलने से secret
कभी नहीं मिटता।

## बैकएंड: दो सर्विस

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

### PluginService: सर्वर क्या कॉल करता है

| Method | कब |
|---|---|
| `Initialize(plugin_id, config, host_version)` | जब प्रोसेस अपने socket से जुड़ जाए, एक बार। `config` में `config_schema` के वे मान होते हैं जो एडमिन ने सेट किए। शुरू होने से मना करने के लिए `error` के साथ `success: false` लौटाएँ। |
| `HealthCheck()` | समय-समय पर, ताकि चलता हुआ प्रोसेस अटके हुए प्रोसेस से अलग पहचाना जा सके। जब तक आप जवाब दे सकते हैं, `healthy: true` लौटाएँ। |
| `Shutdown(reason)` | जब प्लगइन अक्षम, अपडेट या अनइंस्टॉल हो, या सर्वर रुके। जवाब देने के बाद जल्दी बंद हो जाएँ। |
| `OnEvent(event_type, payload, source)` | अनुबंध में है, पर सर्वर आज प्लगइन को कोई event नहीं भेजता: इसे `handled: true` लौटाने के लिए लागू करें, और कोई कॉल न आने की उम्मीद रखें। |
| `GetCapabilities()` | अनुबंध में है; सर्वर आज इसे कॉल नहीं करता। |

### HostService: प्लगइन क्या कॉल कर सकता है

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
| `UpdateMetadata` | `api.metadata` या `media.write` | *दिया नहीं जाता।* |
| `GetUserPreferences` | `api.user` | *दिया नहीं जाता।* |
| `SendNotification` | `notification.sender` | *दिया नहीं जाता।* |

### डेटा स्टोर

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

## Frontend API

इंटरफ़ेस मॉड्यूल एक opaque origin वाले फ़्रेम में चलता है, `default-src 'none'` नीति
के साथ: होस्ट पेज, उसकी कुकी या उसके स्टोरेज तक कोई पहुँच नहीं, कोई फ़ॉर्म नहीं,
कोई popup नहीं, और `connect-src` सिर्फ़ `https` पर `external_fetch` के होस्ट तक
सीमित। बाकी सब `window.evey` से होकर जाता है, जो मॉड्यूल चलने से पहले डाला जाता
है। वही मॉड्यूल बिना बदले वेब ऐप में और मोबाइल और टीवी ऐप में चलता है।

### `evey.onInit(callback)`

होस्ट जो context भेजता है, उसके साथ एक बार कॉल होता है; context आने के बाद
रजिस्टर किया गया callback तुरंत कॉल होता है।

```js
evey.onInit(function (data) {
  // data.context         - प्लगइन के कॉन्फ़िगरेशन मान (config_schema)
  // data.locale          - चालू भाषा, जैसे "en-US"
  // data.theme           - चालू थीम की id
  // data.surface         - "web", "mobile" या "tv"
  // data.suggestedHeight - सिर्फ़ home.section: होस्ट की सुझाई ऊँचाई (px)
  // data.viewportHeight  - सिर्फ़ tv: पैनल की ऊँचाई, dp में
});
```

### `evey.navigate(path)`

होस्ट ऐप्लिकेशन को `path` पर ले जाता है। सिर्फ़ `/home`, `/library`, `/media`,
`/player`, `/settings` और `/search` से शुरू होने वाले रास्ते माने जाते हैं; बाकी
सब (एडमिन वाला हिस्सा, साइन-इन, पूरा URL) चुपचाप छोड़ दिया जाता है।

### `evey.toast(level, message)`

होस्ट इंटरफ़ेस में एक toast दिखाता है। `level` `"success"`, `"error"` या `"info"`
है; `message` सादा टेक्स्ट है, 200 अक्षरों पर काटा जाता है; एक सेकंड में ज़्यादा
से ज़्यादा 3 toast दिखते हैं।

### `evey.resize(height)`

स्लॉट को `height` पिक्सेल ऊँचा करने को कहता है, 50 और 800 के बीच रखा जाता है।
`0` स्लॉट को समेट देता है: जिस विजेट के पास दिखाने को कुछ नहीं, वह जगह नहीं
लेता। `home.section` विजेट बिना कुछ किए `data.suggestedHeight` पर खुलता है: होस्ट
जानता है कि उसने आपको किस जगह रखा है और वहाँ होम विजेट की जो ऊँचाई होती है वही
सुझाता है, ताकि हर विजेट एक ही आकार में दिखे। आकार तभी बदलें जब आपके पास इससे
बेहतर जानकारी हो।

### `evey.prefs.*` (अनुमति `prefs`)

मौजूदा उपयोगकर्ता की पसंद, प्लगइन के अपने हिस्से में, उस token के ज़रिए (यह प्लगइन, यह
उपयोगकर्ता) से जुड़ी हुई जो होस्ट फ़्रेम के लिए रखता है। हर कॉल एक promise
लौटाती है; अनुमति न हो तो वह reject होता है।

```js
await evey.prefs.set("units", "celsius");
const { value, found } = await evey.prefs.get("units");
const all = await evey.prefs.list();        // [{ key, value }]
await evey.prefs.delete("units");
```

### `evey.media.*` (अनुमति `public`)

मीडिया आइटम की एक सुरक्षित झलक: id, टाइटल, टाइप, साल, सारांश, पोस्टर का URL।
कभी फ़ाइल का रास्ता, आकार, provider id या stream नहीं।

```js
const item  = await evey.media.get(id);
const page  = await evey.media.list({ libraryId });
const data  = await evey.media.poster(id);  // पोस्टर, data: URL के रूप में
```

`poster` एक `data:` URL लौटाता है, क्योंकि फ़्रेम की नीति सर्वर से सीधे तस्वीर
लोड करने से मना करती है; इसे `<img>` के स्रोत या texture की तरह इस्तेमाल करें।

### `evey.assets.read(path)`

प्लगइन की अपनी `web_root` की कोई फ़ाइल, relative रास्ते से, `data:` URL के रूप
में पढ़ता है: जो फ़ाइल बैकएंड ने चलते समय लिखी, वह cross-origin की चिंता के बिना
फ़्रेम में इस्तेमाल हो सकती है। सिर्फ़ प्लगइन की अपनी फ़ाइलें पहुँच में हैं।

### टीवी पर

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
  `tests/plugins/weather-v2` ऊपर की सभी बातों के लिए नमूने वाला विजेट है।

## स्लॉट

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

एडमिन वाले हिस्से में `admin.nav` अकेला स्लॉट है। यह मौजूदा एडमिन स्क्रीन में कुछ
नहीं डालता: यह एक मेन्यू एंट्री जोड़ता है जो प्लगइन का पेज उसी सैंडबॉक्स वाले
फ़्रेम में खोलती है, एक ऐसे route पर जो सिर्फ़ एडमिन के लिए है। इसे माँगना मंज़ूरी
वाले फ़र्क में दिखता है। बाकी हर `admin.*`, `login.*`, `setup.*` और
`profile.security.*` स्लॉट इंस्टॉल के समय मना होता है।

## क्षमताएँ

क्षमता वह भूमिका बताती है जो प्लगइन निभाने का दावा करता है: `metadata.provider`,
`media.read`, `media.write`, `webhook.receiver` या `notification.sender`। इंस्टॉलर
नाम जाँचता है, एडमिन पूर्वावलोकन में सूची देखता है और अपडेट पर हर नई क्षमता
देखता है, और सुरक्षा डैशबोर्ड उसे दर्ज करता है। सर्वर आज किसी क्षमता के आधार पर
कुछ नहीं भेजता: `metadata.provider` को जानकारी जोड़ने का कोई कॉल नहीं मिलता,
`webhook.receiver` को कोई आने वाला request नहीं मिलता। प्लगइन असल में क्या कर
सकता है, यह उसकी **अनुमतियाँ** तय करती हैं।

## इंटरफ़ेस की अनुमतियाँ

`public` और `prefs` बाकी अनुमतियों की तरह `permissions` में बताई जाती हैं। ये
बैकएंड प्रोसेस को कुछ नहीं देतीं; ये फ़्रेम में `evey.media.*` और `evey.prefs.*`
खोलती हैं। फ़्रेम के पास कभी token नहीं होता: होस्ट (प्लगइन, उपयोगकर्ता) तक
सीमित पाँच मिनट का एक token बनाता है और कॉल आगे भेजता है, और सर्वर डिफ़ॉल्ट रूप
से हर वह method मना करता है जिसका नाम token के दायरे में नहीं है
([ADR-0012](../adr/0012-plugin-token-deny-by-default.md))।

!!! note "प्लगइन कभी क्या नहीं माँग सकता"
    `admin.*`, `auth.*`, `system.*`, `user.write` और `library.write` पूरी तरह बंद
    हैं: इनका नाम लेने वाला manifest मना होता है, कोई token इन्हें कभी नहीं ले
    जा सकता, और एडमिन method प्लगइन token को मना करते हैं, उसका दायरा कुछ भी हो।

## उदाहरण

`examples/plugins/` में मौजूदा manifest फ़ॉर्मैट वाले चलते प्लगइन हैं: Node.js
और Python में शुरुआती प्लगइन (`nodejs-example`, `python-example`), एक मेटाडेटा
provider (`metadata-example`), एक वेबहुक receiver (`webhook-receiver`), build
step वाला एक होम विजेट (`weather`), `public` और `prefs` अनुमतियों और `admin.nav`
स्लॉट के साथ डेटा स्टोर (`data-demo`), `config_schema` के हर फ़ील्ड टाइप का उदाहरण
(`config-demo`), और दो थीम (`theme-dark-blue`, `open-workshop`)। `make plugin-zips`
इन सबको पैक करता है।

<!-- sources
manifest fields, validation: internal/domain/plugin/model.go - PluginManifest, Validate, validateWebRoot, validateSrcUnderWebRoot, IsSecretConfigField ; internal/adapters/driven/pluginrunner/manifest.go - Parse, validateManifest, validateExternalFetch ; capabilities.go - KnownCapabilities, KnownPermissions
runtimes: internal/adapters/driven/pluginrunner/runtime_availability.go ; interpreters: runner_start.go - knownInterpreters
config field types: frontend/apps/web/src/features/admin/PluginCard.tsx ; examples/plugins/config-demo/plugin.yaml
secrets masked: internal/adapters/driving/grpcapi/admin_plugin.go (SecretMask)
env vars, Initialize call: internal/adapters/driven/pluginrunner/runner_start.go ; ipc_client.go - Initialize, Shutdown, HealthCheck, OnEvent ; runner_control.go - SendEvent (no caller)
peer auth, rate limit: internal/adapters/driving/pluginhost/host_server.go - makePluginAuthInterceptor, pluginRPCRatePerMin ; peer_creds.go
host RPCs and permissions: internal/adapters/driving/grpcapi/plugin_interceptor.go ; pluginhost/host_server.go - Log, GetConfig, SetConfig (maxConfigKeys, maxConfigKeyLen, maxConfigValueLen), EmitEvent (pluginEmittedEvent.EventName), GetMediaItem, ListMediaItems, mediaItemToProto, UpdateMetadata, GetUserPreferences (Unimplemented), UnimplementedHostServiceServer ; fetch.go - Fetch, maxFetchBytes, fetchTimeout
proto: proto/plugin/v1/plugin_api.proto - PluginService, HostService, InitRequest, FetchRequest, FetchResponse, MediaItem
frame policy: internal/adapters/driving/http/plugin_frame_handler.go - pluginFramePolicy ; frontend/packages/client/src/plugins/hostProtocol.ts - buildConnectSrc, ALLOWED_NAV_PREFIXES, SLOT_MAX_HEIGHT, clampSlotHeight, TOAST_MAX, TOAST_WINDOW_MS, TOAST_MAX_LENGTH ; web/PluginIframe.tsx - MIN_SLOT_HEIGHT, suggestedSlotHeight ; native/PluginWebView.tsx (surface)
bridge: frontend/packages/client/src/plugins/bridgeScript.ts - EveyBridge ; pluginRpc.ts - pluginRequestFor
slots: internal/domain/plugin/slots.go - AllowedSlots
token, deny by default: internal/app/plugin/token_service.go - PluginTokenTTL ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes, forbiddenPrefix ; docs/adr/0003-plugin-scoped-tokens.md ; docs/adr/0012-plugin-token-deny-by-default.md
examples: examples/plugins/* ; Makefile - plugin-zips
-->
