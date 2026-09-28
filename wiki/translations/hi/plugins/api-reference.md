---
source_sha: 2d10a959b8d809ae197a485f1214dbb6d86d37c8658c1c4d4c12d0222f6593f8
---
# एक्सटेंशन API संदर्भ

यह दस्तावेज़ उन बाहरी डेवलपरों के लिए है जो Evey का एक्सटेंशन लिख रहे हैं। एक्सटेंशन पर लागू सुरक्षा पाबंदियों के लिए [`security-model.md`](security-model.md) देखें।

## Manifest schema

हर एक्सटेंशन ZIP के मूल में एक `plugin.yaml` होना चाहिए।

```yaml
name: my-plugin              # required; human-readable name
version: 1.2.3               # required; semver
kind: plugin                 # required; must be "plugin" (use "theme" for CSS/JS-only themes)
language: nodejs             # required for plugins; nodejs | python | go | php | ruby | lua | bash | sh
entrypoint: node index.js    # required for plugins; the full command line the interpreter runs -
                              # the first token is the interpreter, looked up on PATH; the
                              # remaining tokens are the script/args, path-confined to the ZIP.
                              # "language" is only used to check the runtime is available and to
                              # copy SDK stubs - it does NOT launch the entrypoint.

description: "..."           # optional
author: "..."                # optional
min_evey_version: "1.0.0"   # optional; semver minimum host version

permissions:
  - network                  # see Permissions section
  - media.read

external_fetch:              # required when "network" permission is declared
  - api.themoviedb.org       # see External fetch rules in security-model.md
  - api.example.com          # bare hostname only - no scheme, no wildcard, no literal IP

capabilities:
  - metadata.provider        # see Capabilities section

# Optional: declare a sub-directory whose contents are served as static assets.
# Required when frontend modules are declared.
web_root: public/

# Optional: frontend modules injected into the host UI.
frontend:
  - slot: home.section       # see Slot whitelist in security-model.md
    src: public/front.js     # path relative to ZIP root; must be under web_root

config_schema:               # optional; JSON Schema object describing plugin config keys
  api_key:
    type: string
    secret: true             # or `type: password`: never sent back to any client, admin included
    description: "TMDB API key"
```

`secret: true` से चिह्नित (या `type: password` वाला) फ़ील्ड हमेशा की तरह सहेजा और
एक्सटेंशन को दिया जाता है, पर पढ़ने वाला हर रास्ता - सार्वजनिक भी और व्यवस्थापन का
कॉन्फ़िगरेशन पैनल भी - उसे छिपाकर लौटाता है। जब छिपा हुआ मान बिना बदले वापस भेजा
जाता है, तो व्यवस्थापन पैनल सहेजा हुआ मान बनाए रखता है, इसलिए बाक़ी फ़ील्ड सहेजने से
कोई राज़ कभी नहीं मिटता।

**इंस्टॉल पर लागू जाँच के नियम:**

- `kind: plugin` के लिए `language` और `entrypoint` ज़रूरी हैं।
- `kind: theme` को `language`, `entrypoint` या `permissions` घोषित नहीं करने चाहिए।
- `frontend` ख़ाली न हो तो `web_root` ज़रूरी है।
- `frontend[].src` को `web_root` के भीतर होना चाहिए।
- `frontend[].slot` सर्वर की स्लॉट अनुमत-सूची में होना चाहिए।
- `network` अनुमति के लिए एक ग़ैर-ख़ाली `external_fetch` सूची चाहिए।
- `external_fetch` की हर प्रविष्टि को होस्ट अनुमत-सूची की जाँच पास करनी होगी।
- सारी क्षमताएँ और अनुमतियाँ सर्वर को ज्ञात होनी चाहिए।

## Backend IPC

एक्सटेंशन प्रक्रिया Evey होस्ट से एक Unix domain socket पर gRPC के ज़रिए बात करती है। socket का पाथ शुरू होते समय `EVEY_PLUGIN_SOCK` environment वेरिएबल में दिया जाता है।

होस्ट `cmd.Start` के ठीक बाद `SO_PEERCRED` से उसका PID जाँचकर एक्सटेंशन प्रक्रिया को प्रमाणित करता है - प्रक्रिया को ख़ुद कोई टोकन नहीं दिया जाता।

```
EVEY_PLUGIN_SOCK=/tmp/evey/plugins/<plugin_id>/host.sock
EVEY_PLUGIN_ID=<plugin_id>
EVEY_HOST_SOCK=<path>          # socket the plugin can dial back to reach the host
```

एक अलग, कम उम्र वाला JWT (5 मिनट, `plugin_id` + `user_id` तक सीमित) ज़रूरत पड़ने पर `IssuePluginToken` RPC से जारी होता है - एक्सटेंशन का *frontend iframe* अपने `public`/`prefs` दायरे वाले कॉल प्रमाणित करने के लिए इसे लेता है। इसका ऊपर के backend socket से कोई संबंध नहीं है और यह शुरुआत में एक्सटेंशन प्रक्रिया को कभी नहीं दिया जाता।

gRPC सेवा परिभाषाएँ `proto/plugin/v1/plugin_api.proto` में हैं। भाषा-विशेष SDK stubs उसी proto से बनते हैं और उन्हें एक्सटेंशन ZIP में शामिल किया या साथ रखा जा सकता है। कई भाषाओं के stubs पहले से बने आते हैं (`sdk/<lang>/`); जो भाषा साथ नहीं आती, उसके लिए देखें [dev-setup.md → किसी और भाषा के लिए stubs बनाना](dev-setup.md#generating-stubs-for-another-language)।

होस्ट अनुमतियाँ RPC स्तर पर लागू करता है - ऐसा RPC बुलाने पर जिसे आपके manifest ने घोषित नहीं किया, नतीजा `PermissionDenied` होता है।

### डेटा भंडार

हर एक्सटेंशन को अपना स्थायी key/value भंडार मिलता है, एक्सटेंशन के अपने
namespace में - **कोई अनुमति नहीं चाहिए, एक्सटेंशन अपने डेटा का मालिक है**। इसे
cursors, caches, आख़िरी सिंक के समय, और हर उस चीज़ के लिए इस्तेमाल करें जो
एक्सटेंशन को पुनरारंभ के पार याद रखनी है। मान अपारदर्शी स्ट्रिंग हैं (संरचना चाहिए
तो JSON ख़ुद encode करें)।

| `HostService` RPC | उद्देश्य |
|---|---|
| `SetData(key, value)` | कोई मान बनाना या बदलना। |
| `GetData(key) → {value, found}` | कोई मान पढ़ना। |
| `DeleteData(key)` | कोई key हटाना। |
| `ListDataKeys() → {keys}` | एक्सटेंशन की keys की सूची। |

एक्सटेंशन की id प्रमाणित socket से ली जाती है - कोई एक्सटेंशन सिर्फ़ अपना **ही**
namespace पढ़-लिख सकता है, कभी किसी और का नहीं। एक्सटेंशन अनइंस्टॉल होने पर उसका
पूरा भंडार अपने आप मिट जाता है (`ON DELETE CASCADE`), इसलिए हटाने के बाद कोई पुराना
डेटा नहीं बचता।

यह भंडार पूरे एक्सटेंशन के लिए है। डेटा **हर उपयोगकर्ता** के लिए सहेजना हो (जैसे
अभी देख रहे उपयोगकर्ता की कोई पसंद), तो नीचे `prefs` अनुमति देखें - उस सतह तक
एक्सटेंशन के iframe से पहुँचा जाता है, जहाँ उपयोगकर्ता का संदर्भ मौजूद है।

## Frontend API

जब कोई एक्सटेंशन `frontend` मॉड्यूल घोषित करता है, तो iframe के `srcdoc` वाले HTML में एक पुल-स्क्रिप्ट होती है जो मॉड्यूल के JavaScript को `window.evey` देती है।

Frontend मॉड्यूल को होस्ट पेज के DOM, `window`, `fetch`, `localStorage` या किसी और ब्राउज़र global तक सीधी पहुँच नहीं है, सिवाय उसके जो `window.evey` के ज़रिए साफ़ तौर पर दिया गया है।

### `evey.navigate(path)`

होस्ट ऐप्लिकेशन को `path` पर ले जाता है। पाथ अनुमत रूट होना चाहिए; `/admin/*`, `/auth/*`, `/setup/*` और `/profile/security/*` के अनुरोध चुपचाप छोड़ दिए जाते हैं।

```js
evey.navigate("/library/movies");
evey.navigate("/media/abc123");
```

### `evey.toast(level, message)`

होस्ट इंटरफ़ेस में एक toast सूचना दिखाता है।

- `level`: `"success"` | `"error"` | `"info"`
- `message`: सादा टेक्स्ट, अधिकतम 200 अक्षर, HTML हटा दिया जाता है
- प्रति सेकंड 3 toasts तक सीमित

```js
evey.toast("success", "Metadata enriched.");
evey.toast("error", "API key missing.");
```

### `evey.resize(height)`

होस्ट से iframe स्लॉट को `height` पिक्सेल करने को कहता है। 50-800 तक सीमित। `0` स्लॉट को समेट देता है: जिस एक्सटेंशन के पास दिखाने को कुछ नहीं, वह पेज पर जगह नहीं लेता।

```js
evey.resize(300);
evey.resize(0); // nothing to say: the slot disappears
```

### `evey.onInit(callback)`

एक callback दर्ज करता है जो एक बार चलता है, जब होस्ट एक्सटेंशन को उसका शुरुआती संदर्भ भेजता है। अगर `onInit` बुलाए जाने से पहले होस्ट `init` भेज चुका हो, तो callback सहेजे हुए डेटा के साथ तुरंत बुलाया जाता है।

```js
evey.onInit(function(data) {
  // data.context         - plugin config values (from config_schema)
  // data.locale          - active locale string, e.g. "en-US"
  // data.theme           - active theme id
  // data.surface         - "web", "mobile" or "tv": where the widget landed
  // data.suggestedHeight - home.section only: the height (px) the host recommends there
  // data.viewportHeight  - tv only: the panel height in dp
});
```

होस्ट एक्सटेंशन के `ready` संकेत का 5 सेकंड तक इंतज़ार करता है (यह पुल-स्क्रिप्ट अपने आप करती है)। समय-सीमा पूरी होने पर iframe हटा दिया जाता है और एक त्रुटि दर्ज होती है।

#### ऊँचाई

एक `home.section` विजेट बिना कुछ किए `data.suggestedHeight` पर खुलता है: होस्ट जानता है कि उसने आपको किस सतह पर रखा है - डेस्कटॉप पेज, फ़ोन, टीवी पैनल - और वहाँ होम विजेट की जो ऊँचाई होती है वही सुझाता है, ताकि होम पेज का हर विजेट एक ही आकार में आए। `evey.resize()` तभी बुलाएँ जब आप बेहतर जानते हों; होस्ट आपकी पीठ पीछे आपकी सामग्री का आकार या पैमाना कभी नहीं बदलता।

#### टीवी पर

टीवी ऐप आपका विजेट एक WebView में चलाता है, और ब्राउज़र की कुछ सच्ची बातें वहाँ सच नहीं रहतीं:

- **अपने viewport से आकार न तय करें।** WebView के भीतर `window.innerHeight` वही स्लॉट है जो होस्ट ने आपको अभी दिया - यह गोल-गोल घूमना है। अगर आप अपना आकार बदलते हैं, तो `data.viewportHeight` (पैनल) या `data.suggestedHeight` से तय करें, `innerHeight` से कभी नहीं।
- **न फ़ोकस, न पॉइंटर।** विजेट को कभी D-pad का फ़ोकस नहीं मिलता, और hover होता ही नहीं। जो भी इंटरैक्टिव है, वह वेब और मोबाइल सतहों का है।
- **टेक्स्ट किनारों से दूर रखें।** टीवी overscan करते हैं: हर तरफ़ चौड़ाई और ऊँचाई का लगभग 5 % ख़ाली छोड़ें।
- **WebGL को असली डिवाइस पर, ज़ूम करके जाँचें।** shader डिवाइस के GPU और उसके ड्राइवर पर चलता है, आपके डेस्कटॉप वाले पर नहीं। एक lattice hash जो `fract()` से पहले अपने इनपुट को कुछ सौ से गुणा करता था, Mali GPU पर बादलों की जगह सपाट बहुभुज बनाता था, जबकि ब्राउज़र में ठीक दिखता था; noise के निर्देशांक सीमित रखें (`mod()`) और सीमित मानों का hash करें। ऊपर की सारी बातों के लिए `tests/plugins/weather-v2` संदर्भ विजेट है।

## स्लॉट

| स्लॉट | यह कहाँ दिखता है |
|---|---|
| `home.section` | होम पेज पर सामग्री का एक खंड |
| `home.hero` | होम पेज पर hero बैनर का हिस्सा |
| `library.sidebar` | लाइब्रेरी दृश्यों में साइड पैनल |
| `media.detail.sidebar` | मीडिया विवरण पृष्ठों पर साइड पैनल |
| `player.overlay` | वीडियो प्लेयर के ऊपर की परत |
| `settings.user.appearance` | उपयोगकर्ता सेटिंग्स में दिखावट का उप-खंड |
| `sidebar.bottom` | मुख्य नेविगेशन साइडबार का निचला हिस्सा |
| `admin.nav` | व्यवस्थापन पैनल में एक मेनू प्रविष्टि जो एक्सटेंशन का अपना अलग किया हुआ पृष्ठ खोलती है |

`admin.nav` व्यवस्थापन क्षेत्र का **एकमात्र** स्लॉट है। यह मौजूदा व्यवस्थापन
स्क्रीनों में कोई इंटरफ़ेस नहीं डालता - यह बस एक मेनू प्रविष्टि जोड़ता है जो आपके
एक्सटेंशन का अपना पृष्ठ एक अलग व्यवस्थापन रूट पर, एक अलग किए गए iframe में खोलती
है। इसे माँगना इंस्टॉल की सहमति वाले अंतर में दिखता है।

`admin.*`, `login.*`, `setup.*`, `profile.security.*` के तहत बाक़ी सारे स्लॉट
इंस्टॉल पर ठुकरा दिए जाते हैं।

## क्षमताएँ

क्षमताएँ बताती हैं कि एक्सटेंशन सर्वर की तरफ़ कौन सी भूमिका निभाता है। इनसे तय होता है कि होस्ट एक्सटेंशन तक कौन से RPC भेजता है और अपनी सेवा-सूची में उसे कैसे दर्ज करता है।

### `metadata.provider`

एक्सटेंशन मीडिया आइटम के लिए समृद्ध मेटाडेटा दे सकता है। संवर्धन के लिए कोई अलग RPC नहीं है - होस्ट जिस `PluginService` को बुलाता है वह सिर्फ़ `Initialize` / `Shutdown` / `OnEvent` / `HealthCheck` / `GetCapabilities` देता है (`proto/plugin/v1/plugin_api.proto`)। `metadata.provider` एक्सटेंशन को ज़रूरी घटनाएँ `OnEvent` से मिलती हैं और वह अपनी क्षमता `GetCapabilities` से बताता है; वह संवर्धन का कोई अलग तरीक़ा लागू नहीं करता।

ज़रूरी अनुमति: `api.metadata`।

### `media.read`

एक्सटेंशन मीडिया रिकॉर्ड और स्ट्रीम पढ़ सकता है। manifest की `media.read` अनुमति से मेल खाता है।

### `media.write`

एक्सटेंशन मीडिया रिकॉर्ड बना या अपडेट कर सकता है। `media.write` अनुमति से मेल खाता है।

### `notification.sender`

एक्सटेंशन सूचनाएँ प्रकाशित कर सकता है। होस्ट बाहर जाने वाली सूचना-घटनाओं को एक्सटेंशन से होकर भेजता है। `notification.sender` अनुमति से मेल खाता है।

### `webhook.receiver`

एक्सटेंशन ख़ुद को webhook endpoint के रूप में दर्ज करता है। होस्ट मेल खाते आने वाले HTTP POST अनुरोध gRPC से एक्सटेंशन को भेजता है। क्षमता घोषित करने के अलावा कोई अनुमति नहीं चाहिए।

## अनुमतियाँ

`public` और `prefs` **अनुमतियाँ** हैं, क्षमताएँ नहीं - वे यह नहीं बदलतीं कि
एक्सटेंशन सर्वर की तरफ़ कौन सी भूमिका निभाता है, बल्कि एक्सटेंशन के iframe को
पढ़ने/लिखने के अतिरिक्त दायरे देती हैं। उन्हें manifest में `permissions` के तहत
घोषित करें।

### `public`

एक्सटेंशन के iframe को ग़ैर-संवेदनशील सर्वर डेटा तक सिर्फ़-पढ़ने की पहुँच देता है:
`GetPublicMediaItem` / `ListPublicMediaItems`, जो एक **सुरक्षित प्रक्षेपण** लौटाते
हैं (id, शीर्षक, प्रकार, वर्ष, सारांश, पोस्टर URL) - कभी फ़ाइल पाथ, आकार, प्रदाता
id या स्ट्रीम नहीं। सर्वर की ब्रांडिंग (नाम) पहले से सार्वजनिक है और उसके लिए कोई
अनुमति नहीं चाहिए। इसे माँगने के लिए `permissions` में `public` घोषित करें।

### `prefs`

एक्सटेंशन के iframe को एक्सटेंशन के अपने namespace में **मौजूदा उपयोगकर्ता** की
पसंदों को पढ़ने/लिखने देता है, `PluginUserDataService` (`GetPref` / `SetPref` /
`DeletePref` / `ListPrefs`) के ज़रिए। एक्सटेंशन और उपयोगकर्ता iframe टोकन से बँधे
रहते हैं, इसलिए कोई एक्सटेंशन सिर्फ़ (अपने-ही-एक्सटेंशन, मौजूदा-उपयोगकर्ता) का डेटा
छू सकता है - कभी किसी और एक्सटेंशन का नहीं, कभी किसी और उपयोगकर्ता का नहीं, कभी
पहचान या सुरक्षा के फ़ील्ड नहीं। पंक्तियाँ एक्सटेंशन या उपयोगकर्ता के साथ cascade
होकर मिटती हैं। `permissions` में `prefs` घोषित करें।

> **जो आप कभी नहीं माँग सकते।** `admin.*`, `auth.*` (login/logout/session),
> `system.*`, और सामान्य `user.write` / `library.write` पूरी तरह बंद हैं: उन्हें हर
> टोकन से हटा दिया जाता है और API सीमा पर ठुकराया जाता है, भले कोई manifest उन्हें
> घोषित करने की कोशिश करे। दायरे वाला एक्सटेंशन टोकन सर्वर API पर **डिफ़ॉल्ट रूप से
> अस्वीकृत** है - वह सिर्फ़ उन्हीं तरीक़ों तक पहुँचता है जिन्हें ऊपर दी गई अनुमतियाँ
> साफ़ तौर पर खोलती हैं।

## उदाहरण

`examples/plugins/` में मौजूदा manifest प्रारूप वाले काम करते एक्सटेंशन हैं: Node.js और Python में शुरुआती ढाँचे (`nodejs-example`, `python-example`), एक मेटाडेटा स्रोत (`metadata-example`), एक webhook रिसीवर (`webhook-receiver`), होम पेज का एक विजेट (`weather`), एक्सटेंशन का कॉन्फ़िगरेशन और राज़ (`config-demo`), `public` / `prefs` अनुमतियों और `admin.nav` स्लॉट के साथ डेटा भंडार (`data-demo`), और दो थीम। [dev-setup.md](dev-setup.md#3-scaffold-your-first-plugin) बताता है कि इनमें से किसी को कैसे कॉपी करें।
