---
source_sha: 8da868d1b798afe178e9ef7c621a9a0513b023c77f535745e4e5d2c44b5dae50
---
# एक्सटेंशन डेवलपर सेटअप

उस व्यक्ति के लिए शुरू से अंत तक की सैर जिसने अभी-अभी Evey क्लोन किया है और कोई एक्सटेंशन या थीम लिखना, जाँचना और भेजना चाहता है। अपने एक्सटेंशन की भाषा से मेल खाता रनटाइम चुनें; बाक़ी सब साझा है।

## 1. पूर्व-शर्तें

Evey होस्ट प्रक्रिया (Go) चलाता है और एक्सटेंशन को ZIP के रूप में भेजता है। एक्सटेंशन का रनटाइम इस पर निर्भर है कि आपका कोड क्या बुलाता है।

### हमेशा ज़रूरी

| औज़ार | किसलिए | इंस्टॉल |
|---|---|---|
| Go ≥ 1.25 | Evey होस्ट बनाने के लिए | `https://go.dev/dl/` |
| Docker + Docker Compose | डिफ़ॉल्ट डेवलपमेंट स्टैक (Postgres, Meilisearch, सर्वर, वेब) | https://docs.docker.com/engine/install/ |
| `bubblewrap` (`bwrap`) | एक्सटेंशन प्रक्रियाओं का सैंडबॉक्स - `bubblewrap` और `strict` मोड के लिए ज़रूरी | `apt install bubblewrap` (Debian/Ubuntu), `dnf install bubblewrap` (Fedora), `pacman -S bubblewrap` (Arch)। macOS: किसी VM में Linux इंस्टॉल करें, bwrap सिर्फ़ Linux पर है। |

`basic` सैंडबॉक्स मोड bwrap के बिना चलता है, पर उसे फेंक देने लायक़ स्थानीय प्रयोगों के बाहर कभी इस्तेमाल न करें - जितनी जल्दी हो सके छोड़ दें।

### भाषा के हिसाब से

वही लें जो आपका एक्सटेंशन सच में इस्तेमाल करता है; सब की ज़रूरत नहीं।

| एक्सटेंशन की भाषा | औज़ार | टिप्पणी |
|---|---|---|
| Python 3 | `python3`, `pip` | होस्ट साफ़ रखने के लिए `pip install --user` इस्तेमाल करें। SDK stubs `sdk/python/` में हैं। |
| Node.js | Node 20+, `npm` या `pnpm` | SDK रनटाइम पर `@grpc/proto-loader` से `plugin_api.proto` लोड करता है। |
| Go | Go ≥ 1.21 | साथ में stubs नहीं - `protoc-gen-go` + `protoc-gen-go-grpc` से बनाएँ; कंपाइल किए गए Go बाइनरी को `native-binary` अनुमति भी चाहिए। |
| Dart / Java / PHP / Ruby / Swift | हर भाषा की अपनी toolchain | stubs पहले से बने हुए `sdk/<lang>/` में हैं। |
| सिर्फ़ CSS वाली थीम | कुछ और नहीं | थीम `kind: theme` + एक `theme.css` वाली ZIP है। |
| JS वाली थीम | कुछ और नहीं (होस्ट आपका JS `sandbox="allow-scripts"` और सख़्त CSP वाले एक अलग `iframe` में चलाता है) | स्थिर स्कैनर `document`, `window`, `fetch`, `localStorage` आदि तक हर पहुँच ठुकराता है - देखें `internal/pkg/themejs/scan.go`। |

### किसी और भाषा के लिए stubs बनाना

ऊपर की भाषाओं के लिए stubs पहले से बने आते हैं, पर एक्सटेंशन का gRPC अनुबंध बस
एक proto फ़ाइल है - आप **किसी भी ऐसी भाषा को निशाना बना सकते हैं जिसे `protoc`
समर्थन देता है** (Rust, C#, Kotlin, Elixir, …)। सच का इकलौता स्रोत है:

```
proto/plugin/v1/plugin_api.proto
```

`protoc` (या `buf`) से बनाएँ, अपनी भाषा के gRPC प्लगइन को इस पर लगाकर:

```bash
# generic shape - exact --*_out flags depend on the language/plugin
protoc --proto_path=proto \
       --<lang>_out=OUT_DIR --grpc_<lang>_out=OUT_DIR \
       proto/plugin/v1/plugin_api.proto
```

साथ आने वाली भाषाओं के आज़माए हुए नुस्ख़े (हर एक को कौन सा gRPC प्लगइन चाहिए:
`grpc_php_plugin`, `protoc-gen-grpc-java`, `protoc-gen-grpc-swift`, …) मूल
`Makefile` के `plugin-sdk` लक्ष्य में और हर `sdk/<lang>/README.md` में हैं (जैसे
`sdk/php/README.md`)। `make plugin-sdk` भेजे जाने वाले stubs फिर से बनाता है।

बने हुए stubs अपने एक्सटेंशन की ZIP के भीतर रखें - होस्ट सिर्फ़ उन्हीं भाषाओं के stubs
अपने आप कॉपी करता है जो वह साथ लाता है (`sdk/<lang>/`)।

### वैकल्पिक पर अनुशंसित

| औज़ार | किसलिए |
|---|---|
| `cosign` (Sigstore) | अपने एक्सटेंशन की ZIP पर हस्ताक्षर करना। `EVEY_PLUGINS_COSIGN_POLICY` डिफ़ॉल्ट रूप से `warn` है (`off` नहीं) - `warn` के साथ और बिना सार्वजनिक कुंजी के, जाँच कुछ नहीं करती (एक चेतावनी लॉग होती है, कुछ नहीं रुकता); हस्ताक्षर सच में जाँचने के लिए `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` सेट करें, या बिना हस्ताक्षर वाले अपलोड ठुकराने के लिए `enforce`। https://docs.sigstore.dev/cosign/installation/ |
| `pre-commit` | एक्सटेंशन जमा करने से पहले साफ़ दिखने वाली समस्याएँ पकड़ना। |
| `golangci-lint` | ज़रूरी अगर आपका एक्सटेंशन Go में है और आप होस्ट जैसी lint सख़्ती चाहते हैं। |

## 2. Evey को स्थानीय रूप से चलाएँ

```sh
git clone git@github.com:Evey-project/evey.git
cd evey
cp .env.dev.example .env.dev   # generates a sane local config
make dev                        # docker compose up: postgres, meilisearch, server (Air), web (Vite)
```

सर्वर स्वस्थ हो जाए तो https://evey.test खोलें (इसे Caddy परोसता है)। डिफ़ॉल्ट क्रेडेंशियल `.env.dev` में हैं - इंस्टेंस को बाहर खोलने से पहले उन्हें बदलें।

अगर `make dev` `bwrap` पर विफल हो:
- पक्का करें कि होस्ट पर `which bwrap` कोई पाथ लौटाता है
- Docker के भीतर डेवलपमेंट कंटेनर में bwrap बाइनरी है; अगर आपने Compose स्टैक बदला है, तो उसे bind-mount करें या *सिर्फ़ डेवलपमेंट कंटेनर के लिए* `EVEY_PLUGINS_SANDBOX_MODE=basic` सेट करें

## 3. अपने पहले एक्सटेंशन का ढाँचा

उदाहरण `examples/plugins/` में हैं: Node.js और Python में शुरुआती ढाँचे (`nodejs-example`, `python-example`), एक मेटाडेटा स्रोत (`metadata-example`), एक webhook रिसीवर (`webhook-receiver`), होम पेज का एक विजेट (`weather`), एक्सटेंशन का कॉन्फ़िगरेशन (`config-demo`) और दो थीम (`theme-dark-blue`, `open-workshop`)। अपने काम के सबसे क़रीब वाला चुनें और उसे किसी काम की डायरेक्टरी में कॉपी करें। डेटा सहेजने और अनुमतियों के लिए **`examples/plugins/data-demo`** देखें - वह हर एक्सटेंशन के अपने डेटा भंडार का इस्तेमाल करता है और `public` / `prefs` अनुमतियाँ तथा `admin.nav` स्लॉट माँगता है।

```sh
cp -r examples/plugins/nodejs-example ~/my-first-plugin
cd ~/my-first-plugin
```

`plugin.yaml` संपादित करें:

- `name`: kebab-case, अनोखा
- `version`: semver - हर रिलीज़ पर बढ़ाएँ
- `kind`: `plugin` (backend ज़रूरी) या `theme` (सिर्फ़ frontend)
- `language`: `python` | `nodejs` | `go` | `php` | `ruby` | `lua` | `bash` | `sh` (manifest का पहचानकर्ता - interpreter बाइनरी का नाम नहीं; `python3`/`node` मान्य `language` मान नहीं हैं, नीचे `entrypoint` देखें)।
- `entrypoint`: वह कमांड लाइन जिसे रनटाइम चलाता है
- `permissions`: हर वह होस्ट API घोषित करें जिसे आप बुलाते हैं। इंस्टॉलर बिना घोषणा वाले कॉल ठुकराता है। पूरी सूची [अनुमतियों की सूची](security-model.md#permissions-catalog) में है।
- `external_fetch`: जब आपको किसी तीसरे पक्ष की API से बात करनी हो। प्रविष्टियाँ सिर्फ़ hostname हैं (`api.example.com`) - कोई scheme न ज़रूरी न इस्तेमाल (`https://` जैसा उपसर्ग पढ़ा जाता है और सिर्फ़ hostname रखा जाता है), कोई wildcard नहीं (`*.example.com` इंस्टॉल पर ठुकराया जाता है: `external_fetch entry %q is a wildcard - use explicit hostnames`), कोई शाब्दिक IP पता नहीं (कोई भी नहीं, सिर्फ़ RFC1918 ही नहीं)। जाँच `internal/adapters/driven/pluginrunner/manifest.go` (`validateExternalFetch`) में है; उसके tests `internal/adapters/driven/pluginrunner/manifest_test.go` में - अपना मान जाँचने के लिए `go test ./internal/adapters/driven/pluginrunner/...` चलाएँ।

Frontend मॉड्यूल (`kind: plugin` के लिए वैकल्पिक, `kind: theme` के लिए वर्जित):

```yaml
frontend:
  - slot: home.section
    src: dist/index.js
```

स्लॉट `internal/domain/plugin/slots.go` में मौजूद होना चाहिए (`home.section`, `home.hero`, `library.sidebar`, `media.detail.sidebar`, `player.overlay`, `sidebar.bottom`, `settings.user.appearance`, और `admin.nav` - व्यवस्थापन क्षेत्र का एकमात्र नियंत्रित स्लॉट, एक मेनू प्रविष्टि जो आपके एक्सटेंशन का अपना अलग किया हुआ पृष्ठ खोलती है)। बाक़ी कुछ भी (`admin.*` injection, `login.*`, `setup.*`, `profile.security.*`) इंस्टॉल पर ठुकरा दिया जाता है।

**डेटा सहेजना।** आपके backend को होस्ट socket पर एक निजी key/value भंडार मिलता है
(`SetData`/`GetData`/`DeleteData`/`ListDataKeys`) - किसी अनुमति की ज़रूरत नहीं,
अनइंस्टॉल पर अपने आप मिट जाता है। अपने iframe से हर उपयोगकर्ता की पसंद के लिए
`prefs` अनुमति माँगें; ग़ैर-संवेदनशील मीडिया (शीर्षक/पोस्टर) पढ़ने के लिए `public`।
देखें [API संदर्भ](api-reference.md#permissions)। व्यवस्थापन, प्रमाणीकरण, सिस्टम और
सामान्य लेखन के दायरे कभी माँगे नहीं जा सकते।

## 4. इंस्टॉल का चक्र

```sh
# zip your plugin
cd ~/my-first-plugin
zip -r my-first-plugin.zip . -x ".*" "node_modules/*" "*.pyc"

# upload through the admin UI
open https://evey.test/admin/plugins
# or via the API
curl -F "file=@my-first-plugin.zip" \
     -H "Authorization: Bearer $EVEY_TOKEN" \
     https://evey.test/api/v1/admin/plugins
```

इंस्टॉलर इस क्रम में चलता है - हर चरण इंस्टॉल ठुकरा सकता है:

1. **Manifest जाँच** - `plugin.yaml` सीधे ZIP से पढ़ा जाता है (नीचे की ZIP-स्तरीय जाँचों से पहले) और schema से जाँचा जाता है। स्लॉट की अनुमत-सूची। अनुमति/क्षमता की अनुमत-सूची। `external_fetch` parser (wildcard और कोई भी शाब्दिक IP ठुकराता है; `http://` जैसा scheme उपसर्ग स्वीकार पर अनदेखा होता है - सिर्फ़ hostname रहता है)। घोषित `language` रनटाइम इमेज में उपलब्ध होना चाहिए।
2. **अनुमति बढ़ोतरी** (सिर्फ़ अपडेट पर) - इंस्टॉलर नए manifest की तुलना पिछले संस्करण से करता है। कोई भी नई माँगी गई अनुमति/क्षमता/स्लॉट/`external_fetch` होस्ट इंस्टॉल को एक त्रुटि के साथ रोक देता है, जिसे बुलाने वाले को साफ़ तौर पर लाँघना पड़ता है (frontend से `accept_permission_changes: true`)। नए इंस्टॉल यह चरण छोड़ देते हैं - तुलना के लिए कुछ है ही नहीं।
3. **Cosign हस्ताक्षर** - जाँचा जाता है जब कोई हस्ताक्षर जाँचक सेट हो; विफलता इंस्टॉल को तभी रोकती है जब `EVEY_PLUGINS_COSIGN_POLICY=enforce` हो। खंड 6 देखें।
4. **ZIP खोलना** - एक्सटेंशन, कुल आकार ≤ 500 MB, हर प्रविष्टि ≤ 100 MB, संपीड़न अनुपात ≤ 100:1, कोई symlink नहीं, कोई path-traversal प्रविष्टि नहीं; फ़ाइलें एक staging डायरेक्टरी में आती हैं (अपग्रेड चल रहे संस्करण को तभी छूते हैं जब बाद के सारे चरण पास हों)।
5. **CSS सफ़ाई** (सिर्फ़ थीम) - `@import` हटाती है, `data:` के अलावा `url()` रोकती है, फ़ुल-स्क्रीन overlay रोकती है, `[type=password]` और `.admin-*` को निशाना बनाने वाले selectors रोकती है, `z-index` को ≤ 999 पर सीमित करती है।
6. **थीम JS स्कैनर** (सिर्फ़ थीम) - `document.cookie`, `localStorage`, `fetch`, `eval`, `globalThis`, होस्ट के globals पर bracket-notation से property पहुँच, `constructor.constructor` आदि का कोई भी ज़िक्र ठुकराता है। थीम सिर्फ़ `evey.theme.*` API इस्तेमाल कर सकती हैं।
7. **स्थिर विश्लेषक** - हर टेक्स्ट फ़ाइल में shell exec, reverse shell, eval, base64-decode-then-exec, छिपाने के पैटर्न, रनटाइम पर इंस्टॉल कमांड, विशेषाधिकार बढ़ोतरी ढूँढता है। गंभीर नतीजे = इंस्टॉल ठुकराया। चेतावनी वाले नतीजे (deserialization, छिपाव, native_code) इनकार बन जाते हैं जब तक मेल खाती अनुमति घोषित न हो।
8. **सहेजना** - staging डायरेक्टरी को जगह पर लाता है, Postgres में पंक्ति लिखता है, और ज़रूरत हो तो रनटाइम शुरू करता है।

कोई भी चरण विफल हो तो व्यवस्थापन इंटरफ़ेस ठीक-ठीक वजह दिखाता है। सर्वर लॉग (`docker compose logs evey`) में पूरा ब्योरा होता है।

## 5. दोहराना

डेवलप करते समय अपने एक्सटेंशन को चलते-चलते फिर लोड करें:

```sh
# edit code → re-zip → re-upload via UI (Update button)
# or, faster: drop the file directly into the plugin directory inside the container
docker compose exec evey ls /var/lib/evey/plugins/<plugin-id>/
```

जब आप `version` बढ़ाते हैं, इंस्टॉलर अपलोड को अपडेट मानता है। अगर आप कोई नई अनुमति जोड़ते हैं, तो आप बढ़ोतरी वाले प्रवाह में पहुँचेंगे - यह डिज़ाइन है, बग नहीं।

एक्सटेंशन प्रक्रिया के लॉग यहाँ आते हैं:
- `/admin/plugins/<id>` - इंटरफ़ेस में आख़िरी N पंक्तियाँ
- `docker compose logs evey | grep <plugin-id>` - पूरी धारा

## 6. अपने एक्सटेंशन पर हस्ताक्षर करें (डेवलपमेंट में वैकल्पिक, वितरण के लिए अनुशंसित)

अगर आप एक्सटेंशन को दूसरे Evey इंस्टेंस तक बाँटने वाले हैं, तो उस पर हस्ताक्षर करें। पाने वाला व्यवस्थापक तब बिना हस्ताक्षर वाले अपलोड सामने लाने के लिए `EVEY_PLUGINS_COSIGN_POLICY=warn` (या `enforce`) चला सकता है।

```sh
# one-time: generate a keypair (keep cosign.key safe - never commit it)
cosign generate-key-pair

# sign the ZIP
cosign sign-blob \
  --key cosign.key \
  --output-signature my-first-plugin.zip.sig \
  my-first-plugin.zip

# distribute: my-first-plugin.zip + my-first-plugin.zip.sig + cosign.pub
```

उस Evey इंस्टेंस पर जो एक्सटेंशन इंस्टॉल करता है:

```sh
# put the public key somewhere readable by the server
sudo cp cosign.pub /etc/evey/cosign.pub

# point Evey at it and switch policy
export EVEY_PLUGINS_COSIGN_PUBLIC_KEY=/etc/evey/cosign.pub
export EVEY_PLUGINS_COSIGN_POLICY=warn   # or "enforce" to refuse unsigned
docker compose restart evey
```

Evey शुरू होते समय कुंजी जाँचता है (PEM parse) - गायब या बिगड़ी कुंजी फ़ाइल पहले इंस्टॉल के बजाय शुरुआत को ही विफल कर देती है। KMS संदर्भ (`gcpkms://...`, `awskms://...`, `k8s://...`) बिना जाँचे स्वीकार किए जाते हैं।

## 7. संसाधन सीमाएँ और सैंडबॉक्स

हर एक्सटेंशन प्रक्रिया पर लागू डिफ़ॉल्ट:

| Env var | डिफ़ॉल्ट | असर |
|---|---|---|
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto` = सबसे अच्छा उपलब्ध चुनता है (आमतौर पर `landlock`)। `landlock` = Landlock LSM अनुमत-सूची + बिना शर्त बाहर जाने वाले TCP पर रोक (`-restrict-net` + seccomp `-deny-inet`, `network` अनुमति के लिए कोई छूट नहीं) + LD_PRELOAD seccomp। `basic` = सिर्फ़ LD_PRELOAD seccomp + UID drop (debug)। `bubblewrap` = पूरे namespaces + root सिर्फ़-पढ़ने लायक़ bind-mount + `--unshare-net` (यह भी बिना शर्त, हर एक्सटेंशन पर, `network` अनुमति से परे)। `strict` = bubblewrap + seccomp अनुमत-सूची। |
| `EVEY_PLUGINS_MEMORY_MB` | 256 | अधिकतम RSS। संसाधन मॉनिटर `/proc` देखता रहता है और इससे ऊपर जाने पर एक्सटेंशन रोक देता है। `0` बंद करता है। |
| `EVEY_PLUGINS_CPU_NICE` | 10 | शुरू करते समय लगाया गया `nice` मान (0-19), ताकि कोई एक्सटेंशन सर्वर को CPU से कभी भूखा न रखे। |
| `EVEY_PLUGINS_PIDS_MAX` | 64 | threads/प्रक्रियाओं की अधिकतम संख्या। fork bomb रोकता है। `0` बंद करता है। |
| `EVEY_PLUGINS_DISK_MAX_MB` | 100 | इंस्टॉल डायरेक्टरी का अधिकतम आकार; मॉनिटर इसे लगभग हर 30 s नापता है और इससे ऊपर जाने पर एक्सटेंशन रोक देता है। `0` बंद करता है। |

नेटवर्क से बात करने वाले एक्सटेंशन को `permissions: [network]` और `external_fetch: [<host>...]` दोनों चाहिए। सीधा बाहरी नेटवर्क हर एक्सटेंशन के लिए सैंडबॉक्स पर बिना शर्त कटा रहता है, उसकी अनुमतियों से परे (ऊपर के सैंडबॉक्स मोड देखें) - एक्सटेंशन प्रक्रिया ख़ुद कभी इंटरनेट की ओर socket नहीं खोल सकती। बाहर जाने का इकलौता रास्ता होस्ट-पक्ष का `Fetch` RPC बिचौलिया है: एक्सटेंशन उसे `EVEY_PLUGIN_SOCK` Unix socket पर बुलाता है, होस्ट लक्ष्य होस्ट को `external_fetch` से मिलाता है, फिर एक SSRF-सुरक्षित client से जुड़ता है (निजी/आरक्षित IP जुड़ते समय ठुकराए जाते हैं, redirects को हल हुए IP और अनुमत होस्ट दोनों से फिर जाँचा जाता है)। देखें `internal/adapters/driving/pluginhost/fetch.go`।

## 8. एक्सटेंशन बाँटना

अभी कोई आधिकारिक कैटलॉग नहीं है: व्यवस्थापक किसी एक्सटेंशन को उसकी ZIP व्यवस्थापन
पैनल में अपलोड करके इंस्टॉल करता है, जहाँ भी आप उसे प्रकाशित करें (GitHub release,
आपकी अपनी साइट)। आज "बाँटने" का मतलब है:

- ZIP, उसकी `.sig` और अपना `cosign.pub` साथ भेजें (खंड 6), ताकि पाने वाला व्यवस्थापक
  अपलोड को `EVEY_PLUGINS_COSIGN_POLICY=warn` या `enforce` से जाँच सके।
- अपने README में अनुमतियाँ और `external_fetch` होस्ट लिखें - व्यवस्थापक इंस्टॉल के
  समय उन्हें देखता है और उसे उन्हें मंज़ूर करना होता है; वहाँ कोई चौंकाने वाली बात
  इंस्टॉल ठुकराए जाने की पहली वजह है।
- अपने एक्सटेंशन को साथ आने वाले उदाहरणों में दिखाने के लिए, स्रोत और एक README के
  साथ `examples/plugins/` पर PR खोलें। उदाहरणों की समीक्षा होती है और वे स्रोत से
  बनाए जाते हैं; PR कोई समर्थन नहीं है और उस पर परियोजना का कोई हस्ताक्षर नहीं होता।

## 9. समस्याएँ सुलझाना

| लक्षण | कारण | उपाय |
|---|---|---|
| `bwrap: ... permission denied` | Docker का डिफ़ॉल्ट seccomp `unshare` रोकता है। | रिपॉज़िटरी अपनी seccomp प्रोफ़ाइल (`docker/seccomp.json`) के साथ आती है। पक्का करें कि आपकी compose फ़ाइल उसे लागू करती है। |
| `forbidden pattern "fetch(" found in theme JS` | थीम JS स्कैनर ने कोई वर्जित API पकड़ी। | थीम सिर्फ़ `evey.theme.*` बुला सकती हैं। क्या रोका जाता है, यह `internal/pkg/themejs/scan.go` में है। |
| `manifest: external_fetch entry "*.tmdb.org" is a wildcard - use explicit hostnames` | इंस्टॉल पर सारे wildcard ठुकराए जाते हैं, बिना अपवाद। | `api.tmdb.org` इस्तेमाल करें (और ज़रूरी हर subdomain अलग से लिखें), `*.tmdb.org` या `cdn.*.tmdb.org` नहीं। |
| `installer: slot "admin.section" not allowed` | विशेषाधिकार वाले क्षेत्रों में एक्सटेंशन के frontend वर्जित हैं। | अनुमत-सूची से कोई ग़ैर-महत्वपूर्ण स्लॉट चुनें या एक्सटेंशन पर फिर सोचें। |
| `cosign verify-blob: failed signature` | सर्वर की सार्वजनिक कुंजी उस निजी कुंजी से मेल नहीं खाती जिससे हस्ताक्षर हुआ। | सही कुंजी से फिर हस्ताक्षर करें, या `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` अपडेट करें। |
| एक्सटेंशन शुरू होकर exit 137 के साथ मर जाता है | cgroup ने मेमोरी ख़त्म होने पर मारा। | मेमोरी खपत घटाएँ या `EVEY_PLUGINS_MEMORY_MB` बढ़ाएँ। |
| Frontend मॉड्यूल नहीं दिखता | एक्सटेंशन चालू नहीं, स्लॉट मेल नहीं खाता, या `frontend.src` में `web_root` उपसर्ग ग़लत। | व्यवस्थापन का Plugins पृष्ठ देखें (स्थिति बैज), पक्का करें कि स्लॉट का नाम किसी असली `<PluginSlot name=...>` से मेल खाता है, और network पैनल में `/api/v1/plugins/{id}/assets/...` कॉल देखें। |

## 10. और आगे

- शुरू से अंत तक सुरक्षा मॉडल: [security-model.md](security-model.md)
- होस्ट API का दायरा: [api-reference.md](api-reference.md)
- उपलब्ध स्लॉट: [api-reference.md](api-reference.md#slots)
- हर भाषा का SDK कोड: `sdk/`
- संदर्भ एक्सटेंशन और थीम: `examples/plugins/`
