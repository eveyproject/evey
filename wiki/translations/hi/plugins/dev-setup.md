---
source_sha: c9ccdddebbef17e96ff8f3da3475c803b0d09dc237cd4c140c897bdfdaf289c5
reviewed: false
---
# प्लगइन लिखना

उस व्यक्ति के लिए शुरू से अंत तक का विवरण जिसने अभी Evey क्लोन किया है और कोई प्लगइन या थीम लिखना, परखना और भेजना चाहता है। अपने प्लगइन की भाषा से मेल खाता रनटाइम चुनें; बाक़ी सब साझा है।

## 1. पूर्व-शर्तें

Evey होस्ट प्रोसेस (Go) चलाता है और प्लगइन ZIP के रूप में भेजता है। प्लगइन रनटाइम इस पर निर्भर करता है कि आपका कोड क्या बुलाता है।

### हमेशा ज़रूरी

| उपकरण | क्यों | इंस्टॉल |
|---|---|---|
| Go ≥ 1.27 | Evey होस्ट बनाने के लिए | `https://go.dev/dl/` |
| Docker + Docker Compose | डिफ़ॉल्ट डेवलपमेंट स्टैक (Postgres, Meilisearch, सर्वर, वेब) | https://docs.docker.com/engine/install/ |
| `bubblewrap` (`bwrap`) | प्लगइन प्रोसेस के लिए सैंडबॉक्स, `bubblewrap` और `strict` मोड के लिए ज़रूरी | `apt install bubblewrap` (Debian/Ubuntu), `dnf install bubblewrap` (Fedora), `pacman -S bubblewrap` (Arch)। macOS: किसी VM में Linux इंस्टॉल करें, bwrap केवल Linux पर है। |

`basic` सैंडबॉक्स मोड bwrap के बिना चलता है लेकिन फेंकने लायक़ स्थानीय प्रयोगों के बाहर कभी इस्तेमाल नहीं होना चाहिए: जितनी जल्दी हो सके इसे छोड़ दें।

### भाषा के अनुसार

वही चुनें जो आपका प्लगइन सचमुच इस्तेमाल करता है; आपको सबकी ज़रूरत नहीं।

| प्लगइन भाषा | उपकरण | टिप्पणियाँ |
|---|---|---|
| Python 3 | `python3`, `pip` | होस्ट को साफ़ रखने के लिए `pip install --user` इस्तेमाल करें। SDK स्टब `sdk/python/` में हैं। |
| Node.js | Node 20+, `npm` या `pnpm` | SDK चलते समय `@grpc/proto-loader` के ज़रिए `plugin_api.proto` लोड करता है। |
| Go | Go ≥ 1.21 | कोई स्टब साथ नहीं: `protoc-gen-go` + `protoc-gen-go-grpc` से बनाएँ; कंपाइल किए गए Go बाइनरी को `native-binary` अनुमति भी चाहिए। |
| Dart / Java / PHP / Ruby / Swift | हर भाषा का टूलचेन | स्टब `sdk/<lang>/` में पहले से बने हैं। |
| केवल-CSS थीम | और कुछ नहीं | थीम `kind: theme` + एक `theme.css` वाली ZIP है। |
| थीम JS | और कुछ नहीं (होस्ट आपका JS `sandbox="allow-scripts"` और बंद CSP वाले एक सैंडबॉक्स किए गए `iframe` के अंदर चलाता है) | स्थैतिक स्कैनर कुकी, स्टोरेज, नेटवर्क कॉल और होस्ट पेज तक पहुँच (`document.cookie`, `localStorage`, `fetch(`, `window.parent`...) मना करता है: देखें [थीम](../themes.md#what-the-scanner-refuses)। |

### किसी दूसरी भाषा के लिए स्टब बनाना

ऊपर की भाषाओं के लिए स्टब पहले से बने आते हैं, लेकिन प्लगइन का gRPC अनुबंध बस एक proto फ़ाइल है: आप **किसी भी भाषा को लक्ष्य बना सकते हैं जिसे `protoc` समर्थित करता है** (Rust, C#, Kotlin, Elixir, …)। सत्य का एकमात्र स्रोत है:

```
proto/plugin/v1/plugin_api.proto
```

`protoc` (या `buf`) से बनाएँ, अपनी भाषा के gRPC प्लगइन को इसकी ओर इंगित करते हुए:

```bash
# सामान्य रूप: सटीक --*_out फ़्लैग भाषा/प्लगइन पर निर्भर हैं
protoc --proto_path=proto \
       --<lang>_out=OUT_DIR --grpc_<lang>_out=OUT_DIR \
       proto/plugin/v1/plugin_api.proto
```

साथ आने वाली भाषाओं के तैयार तरीक़े (हर एक को कौन-सा gRPC प्लगइन चाहिए: `grpc_php_plugin`, `protoc-gen-grpc-java`, `protoc-gen-grpc-swift`, …) जड़ वाले `Makefile` के `plugin-sdk` लक्ष्य में और हर `sdk/<lang>/README.md` में हैं (उदाहरण के लिए `sdk/php/README.md`)। `make plugin-sdk` साथ आने वाले स्टब फिर से बनाता है।

बने हुए स्टब अपनी प्लगइन ZIP के अंदर रखें: होस्ट केवल उन्हीं भाषाओं के स्टब अपने आप कॉपी करता है जिन्हें वह साथ लाता है (`sdk/<lang>/`)।

### वैकल्पिक पर अनुशंसित

| उपकरण | क्यों |
|---|---|
| `cosign` (Sigstore) | अपनी प्लगइन ZIP पर हस्ताक्षर करें। `EVEY_PLUGINS_COSIGN_POLICY` का डिफ़ॉल्ट `warn` है (`off` नहीं): `warn` के साथ और कोई सार्वजनिक कुंजी कॉन्फ़िगर न होने पर जाँच कुछ नहीं करती (एक चेतावनी लॉग होती है, कुछ नहीं रुकता); हस्ताक्षर सचमुच जाँचने के लिए `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` सेट करें, और बिना हस्ताक्षर वाले अपलोड मना करने के लिए नीति को `enforce` पर (बिना सार्वजनिक कुंजी के `enforce` हर प्लगइन को बिना जाँचे स्वीकार करने की बजाय सर्वर को शुरू होते समय रोक देता है)। https://docs.sigstore.dev/cosign/installation/ |
| `pre-commit` | प्लगइन भेजने से पहले स्पष्ट समस्याएँ पकड़ें। |
| `golangci-lint` | अगर आपका प्लगइन Go में है और आप होस्ट जैसी लिंटिंग चाहते हैं तो ज़रूरी। |

## 2. Evey को स्थानीय रूप से चलाएँ

```sh
git clone git@github.com:Evey-project/evey.git
cd evey
cp .env.dev.example .env.dev   # एक समझदार स्थानीय कॉन्फ़िग बनाता है
make dev                        # docker compose up: postgres, meilisearch, सर्वर (Air), वेब (Vite)
```

जब सर्वर स्वस्थ हो, https://evey.test खोलें (Caddy इसे परोसता है)। सेटअप विज़ार्ड पहली बार चलने पर एडमिन खाता बनाता है, जैसा [पहली सेटिंग](../getting-started/first-setup.md) में बताया गया है।

अगर `make dev` `bwrap` पर विफल हो:
- पुष्टि करें कि होस्ट पर `which bwrap` एक पाथ लौटाता है
- Docker के अंदर डेवलपमेंट कंटेनर में bwrap बाइनरी है; अगर आपने Compose स्टैक बदला है, तो उसे माउंट करें या *केवल डेवलपमेंट कंटेनर के लिए* `EVEY_PLUGINS_SANDBOX_MODE=basic` सेट करें

## 3. अपना पहला प्लगइन तैयार करें

उदाहरण `examples/plugins/` में हैं: Node.js और Python में शुरुआती (`nodejs-example`, `python-example`), एक मेटाडेटा प्रदाता (`metadata-example`), एक वेबहुक रिसीवर (`webhook-receiver`), एक मुखपृष्ठ विजेट (`weather`), प्लगइन कॉन्फ़िगरेशन (`config-demo`) और दो थीम (`theme-dark-blue`, `open-workshop`)। अपने उपयोग के सबसे क़रीब वाला चुनें और उसे किसी कार्य डायरेक्टरी में कॉपी करें। स्थायित्व + अनुमति सुविधाओं के लिए **`examples/plugins/data-demo`** देखें: यह हर प्लगइन का डेटा स्टोर इस्तेमाल करता है और `public` / `prefs` अनुमतियाँ + `admin.nav` स्लॉट माँगता है।

```sh
cp -r examples/plugins/nodejs-example ~/my-first-plugin
cd ~/my-first-plugin
```

`plugin.yaml` संपादित करें:

- `name`: kebab-case, अद्वितीय
- `version`: semver, हर रिलीज़ पर बढ़ाएँ
- `kind`: `plugin` (बैकएंड ज़रूरी) या `theme` (केवल फ़्रंटएंड)
- `language`: `python` | `nodejs` | `go` | `php` | `ruby` | `lua` | `bash` | `sh` (मैनिफ़ेस्ट पहचानकर्ता, इंटरप्रेटर बाइनरी का नाम नहीं; `python3`/`node` मान्य `language` मान नहीं हैं, नीचे `entrypoint` देखें)।
- `entrypoint`: वह कमांड लाइन जिसे रनटाइम चलाता है
- `permissions`: हर होस्ट API घोषित करें जिसे आप बुलाते हैं। इंस्टॉलर घोषित न किए गए कॉल मना करता है। पूरी सूची के लिए [अनुमति सूची](security-model.md#permissions-catalog) देखें।
- `external_fetch`: जब आपको किसी तीसरे पक्ष के API से बात करनी हो। प्रविष्टियाँ सादे होस्टनाम हैं (`api.example.com`): न स्कीम ज़रूरी है न इस्तेमाल होती है (कोई भी `https://` उपसर्ग पार्स होता है और केवल होस्टनाम रखा जाता है), कोई वाइल्डकार्ड नहीं (`*.example.com` इंस्टॉल पर मना होता है: `external_fetch entry %q is a wildcard - use explicit hostnames`), कोई शाब्दिक IP पता नहीं (सभी, केवल RFC1918 नहीं)। वैलिडेटर `internal/adapters/driven/pluginrunner/manifest.go` (`validateExternalFetch`) में है; उसके परीक्षण `internal/adapters/driven/pluginrunner/manifest_test.go` में हैं: अपना मान मेल खाता है यह जाँचने के लिए `go test ./internal/adapters/driven/pluginrunner/...` चलाएँ।

फ़्रंटएंड मॉड्यूल (`kind: plugin` के लिए वैकल्पिक, `kind: theme` के लिए वर्जित):

```yaml title="plugin.yaml"
frontend:
  - slot: home.section
    src: dist/index.js
```

स्लॉट `internal/domain/plugin/slots.go` में मौजूद होना चाहिए (`home.section`, `home.hero`, `library.sidebar`, `media.detail.sidebar`, `player.overlay`, `sidebar.bottom`, `settings.user.appearance`, और `admin.nav`, एडमिन क्षेत्र का एकमात्र नियंत्रित स्लॉट, एक मेनू प्रविष्टि जो आपके प्लगइन का अपना सैंडबॉक्स किया गया पेज खोलती है)। कुछ और (`admin.*` में डालना, `login.*`, `setup.*`, `profile.security.*`) इंस्टॉल के समय मना होता है।

**डेटा रखना।** आपके बैकएंड को होस्ट सॉकेट पर एक निजी की/वैल्यू स्टोर मिलता है (`SetData`/`GetData`/`DeleteData`/`ListDataKeys`): कोई अनुमति नहीं चाहिए, अनइंस्टॉल पर अपने आप मिट जाता है। अपने iframe से हर उपयोगकर्ता की पसंदों के लिए `prefs` अनुमति माँगें; गैर-संवेदनशील मीडिया (टाइटल/पोस्टर) तक केवल पढ़ने की पहुँच के लिए `public` माँगें। देखें [API संदर्भ](api-reference.md#permissions)। एडमिन, ऑथ, सिस्टम और सामान्य लेखन दायरे कभी नहीं माँगे जा सकते।

## 4. इंस्टॉल चक्र चलाएँ

```sh
# अपने प्लगइन की zip बनाएँ
cd ~/my-first-plugin
zip -r my-first-plugin.zip . -x ".*" "node_modules/*" "*.pyc"

# एडमिन UI से अपलोड करें
open https://evey.test/admin/plugins
# या API के ज़रिए
curl -F "plugin=@my-first-plugin.zip" \
     -H "Authorization: Bearer $EVEY_TOKEN" \
     https://evey.test/api/v1/admin/plugins/upload
# हस्ताक्षर दूसरे फ़ील्ड में जाता है: -F "sig=@my-first-plugin.zip.sig"
# नई अनुमतियाँ माँगने वाला अपडेट: -F "accept_permission_changes=true"
```

इंस्टॉलर इस क्रम में चलता है, और हर चरण इंस्टॉल मना कर सकता है:

1. **मैनिफ़ेस्ट सत्यापन**: `plugin.yaml` सीधे ZIP से पार्स होता है (नीचे की ZIP-स्तर जाँचों से पहले), स्कीमा जाँची जाती है। स्लॉट श्वेतसूची। अनुमति/क्षमता श्वेतसूची। `external_fetch` पार्सर (वाइल्डकार्ड और कोई भी शाब्दिक IP पता मना करता है; `http://` जैसा स्कीम उपसर्ग स्वीकार पर अनदेखा होता है: केवल होस्टनाम रखा जाता है)। घोषित `language` रनटाइम इमेज में उपलब्ध होना चाहिए।
2. **अनुमति बढ़ोतरी** (केवल अपडेट): इंस्टॉलर नए मैनिफ़ेस्ट की तुलना पिछले संस्करण से करता है। कोई भी नई माँगी गई अनुमति/क्षमता/स्लॉट/external_fetch होस्ट इंस्टॉल को एक त्रुटि के साथ रोकता है जिसे बुलाने वाले को स्पष्ट रूप से ओवरराइड करना होता है (फ़्रंटएंड से `accept_permission_changes: true`)। नए इंस्टॉल यह चरण छोड़ते हैं: तुलना करने को कुछ नहीं।
3. **Cosign हस्ताक्षर**: हस्ताक्षर जाँचने वाला कॉन्फ़िगर होने पर जाँचा जाता है; विफलता केवल तब इंस्टॉल रोकती है जब `EVEY_PLUGINS_COSIGN_POLICY=enforce` हो। सेक्शन 6 देखें।
4. **ZIP निकालना**: एक्सटेंशन, कुल आकार ≤ 500 MB, हर प्रविष्टि का आकार ≤ 100 MB, संपीड़न अनुपात ≤ 100:1, कोई सिमलिंक नहीं, कोई पाथ-ट्रैवर्सल प्रविष्टि नहीं; फ़ाइलें एक स्टेजिंग डायरेक्टरी में जाती हैं (हर बाद वाला चरण पास होने तक अपग्रेड कभी चालू संस्करण को नहीं छूते)।
5. **CSS सैनिटाइज़र** (केवल थीम): `@import` हटाता है, गैर-`data:` `url()` रोकता है, फ़ुलस्क्रीन ओवरले रोकता है, `[type=password]` और `.admin-*` को लक्ष्य करने वाले सेलेक्टर रोकता है, `z-index` को ≤ 999 तक सीमित करता है।
6. **थीम JS स्कैनर** (केवल थीम): `document.cookie`, `localStorage`, `fetch`, `eval`, `globalThis`, होस्ट ग्लोबल पर ब्रैकेट नोटेशन से प्रॉपर्टी पहुँच, `constructor.constructor` आदि के किसी भी उल्लेख को मना करता है। थीम को केवल `evey.theme.*` API इस्तेमाल करनी चाहिए।
7. **स्थैतिक विश्लेषक**: हर टेक्स्ट फ़ाइल में शेल एक्ज़ीक्यूशन, रिवर्स शेल, eval, base64-डिकोड-फिर-एक्ज़ीक्यूट, अस्पष्टीकरण पैटर्न, रनटाइम इंस्टॉल कमांड, विशेषाधिकार बढ़ोतरी खोजता है। गंभीर निष्कर्ष = इंस्टॉल मना। चेतावनी वाले निष्कर्ष (डीसीरियलाइज़ेशन, अस्पष्टीकरण, native_code) तब तक मना में बदल जाते हैं जब तक मेल खाती अनुमति घोषित न हो।
8. **स्थायी करना**: स्टेजिंग डायरेक्टरी को अपनी जगह रखता है, Postgres में पंक्ति लिखता है, ज़रूरत हो तो रनटाइम शुरू करता है।

अगर कोई चरण विफल हो, तो एडमिन UI सटीक कारण दिखाता है। सर्वर लॉग (`docker compose logs evey`) पूरा स्टैक रखते हैं।

## 5. दोहराना

डेवलप करते समय अपने प्लगइन को हॉट-रीलोड करें:

```sh
# कोड बदलें → फिर से zip करें → UI से फिर से अपलोड करें (अपडेट बटन)
# या, तेज़: फ़ाइल सीधे कंटेनर के अंदर प्लगइन डायरेक्टरी में डालें
docker compose exec evey ls /data/plugins/<plugin-id>/
```

अपलोड अपडेट के रूप में केवल तब स्वीकार होता है जब उसका `version` इंस्टॉल वाले से ज़्यादा हो: हर दोहराव पर इसे बढ़ाएँ (बराबर या कम संस्करण मना होता है), या पहले अनइंस्टॉल करें। अगर आप नई अनुमति जोड़ते हैं, तो बढ़ोतरी प्रवाह आएगा: यह डिज़ाइन है, कोई बग नहीं।

प्लगइन प्रोसेस के लॉग यहाँ आते हैं:
- `/admin/plugins/<id>`: UI में आख़िरी N लाइनें
- `docker compose logs evey | grep <plugin-id>`: पूरी स्ट्रीम

## 6. अपने प्लगइन पर हस्ताक्षर करें (डेवलपमेंट में वैकल्पिक, वितरण के लिए अनुशंसित)

अगर आप प्लगइन दूसरे Evey इंस्टेंस को वितरित करना चाहते हैं, तो उस पर हस्ताक्षर करें। पाने वाला एडमिन तब बिना हस्ताक्षर वाले अपलोड दिखाने के लिए `EVEY_PLUGINS_COSIGN_POLICY=warn` (या `enforce`) चला सकता है।

```sh
# एक बार: एक कुंजी जोड़ा बनाएँ (cosign.key सुरक्षित रखें, कभी कमिट न करें)
cosign generate-key-pair

# ZIP पर हस्ताक्षर करें
cosign sign-blob \
  --key cosign.key \
  --output-signature my-first-plugin.zip.sig \
  my-first-plugin.zip

# वितरित करें: my-first-plugin.zip + my-first-plugin.zip.sig + cosign.pub
```

प्लगइन इंस्टॉल करने वाले Evey इंस्टेंस पर:

```sh
# सार्वजनिक कुंजी ऐसी जगह रखें जिसे सर्वर पढ़ सके
sudo cp cosign.pub /etc/evey/cosign.pub

# Evey को उसकी ओर इंगित करें और नीति बदलें
export EVEY_PLUGINS_COSIGN_PUBLIC_KEY=/etc/evey/cosign.pub
export EVEY_PLUGINS_COSIGN_POLICY=warn   # या बिना हस्ताक्षर वालों को मना करने के लिए "enforce"
docker compose restart evey
```

Evey शुरू होते समय कुंजी जाँचता है (PEM पार्स): गुम या ख़राब कुंजी फ़ाइल पहले इंस्टॉल की बजाय बूट को विफल करती है। KMS संदर्भ (`gcpkms://...`, `awskms://...`, `k8s://...`) जैसे हैं वैसे स्वीकार होते हैं।

## 7. संसाधन सीमाएँ और सैंडबॉक्स

हर प्लगइन प्रोसेस पर लागू डिफ़ॉल्ट:

| एनवायरनमेंट वेरिएबल | डिफ़ॉल्ट | असर |
|---|---|---|
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` | `auto` = सबसे अच्छा उपलब्ध चुनता है (आमतौर पर `landlock`)। `landlock` = Landlock LSM अनुमति सूची + बिना शर्त TCP निकास निषेध (`-restrict-net` + seccomp `-deny-inet`, `network` अनुमति के लिए कोई छूट नहीं) + LD_PRELOAD seccomp। `basic` = केवल LD_PRELOAD seccomp + UID उतारना (डीबग)। `bubblewrap` = पूरे नेमस्पेस + केवल पढ़ने वाला बाइंड-माउंट रूट + `--unshare-net` (यह भी बिना शर्त, हर प्लगइन के लिए, `network` अनुमति चाहे जो हो)। `strict` = bubblewrap + LD_PRELOAD seccomp शिम। |
| `EVEY_PLUGINS_MEMORY_MB` | 256 | अधिकतम RSS। संसाधन मॉनिटर `/proc` जाँचता है और इससे आगे जाने पर प्लगइन रोकता है। `0` बंद करता है। |
| `EVEY_PLUGINS_CPU_NICE` | 10 | शुरुआत पर लगाया गया `nice` मान (0-19) ताकि प्लगइन कभी सर्वर को CPU से वंचित न कर सके। |
| `EVEY_PLUGINS_PIDS_MAX` | 64 | अधिकतम थ्रेड/प्रोसेस संख्या। fork बम रोकता है। `0` बंद करता है। |
| `EVEY_PLUGINS_DISK_MAX_MB` | 100 | इंस्टॉल डायरेक्टरी का अधिकतम आकार; मॉनिटर इसे हर ~30s पर मापता है और इससे आगे जाने पर प्लगइन रोकता है। `0` बंद करता है। |

नेटवर्क से बात करने वाले प्लगइन को `permissions: [network]` और `external_fetch: [<host>...]` दोनों चाहिए। सीधा नेटवर्क निकास हर प्लगइन के लिए सैंडबॉक्स पर बिना शर्त काटा जाता है, उसकी अनुमतियाँ चाहे जो हों (ऊपर सैंडबॉक्स मोड देखें): प्लगइन प्रोसेस ख़ुद कभी इंटरनेट की ओर सॉकेट नहीं खोल सकता। एकमात्र रास्ता होस्ट की ओर का `Fetch` RPC ब्रोकर है: प्लगइन इसे `EVEY_PLUGIN_SOCK` Unix सॉकेट पर बुलाता है, होस्ट लक्ष्य होस्ट को `external_fetch` से जाँचता है, फिर एक SSRF-सुरक्षित क्लाइंट से कनेक्ट करता है (कनेक्ट के समय निजी/आरक्षित IP मना, रीडायरेक्ट हल किए गए IP और अनुमत होस्ट दोनों से फिर से जाँचे जाते हैं)। देखें `internal/adapters/driving/pluginhost/fetch.go`।

## 8. प्लगइन वितरित करना

अभी कोई आधिकारिक कैटलॉग नहीं है: एडमिन प्लगइन की ZIP एडमिन पैनल में अपलोड करके उसे इंस्टॉल करता है, जहाँ भी आप उसे प्रकाशित करें (GitHub रिलीज़, आपकी अपनी साइट)। आज "वितरण" का मतलब:

- ZIP, उसका `.sig` और आपका `cosign.pub` साथ भेजें (सेक्शन 6), ताकि पाने वाला एडमिन `EVEY_PLUGINS_COSIGN_POLICY=warn` या `enforce` से अपलोड जाँच सके।
- अपने README में अनुमतियाँ और `external_fetch` होस्ट बताएँ: एडमिन इन्हें इंस्टॉल के समय देखता है और स्वीकार करना होता है; वहाँ कोई अचरज इंस्टॉल मना होने का पहला कारण है।
- अपने प्लगइन को साथ आने वाले उदाहरणों के साथ सूचीबद्ध करवाने के लिए, सोर्स और README के साथ `examples/plugins/` के विरुद्ध एक PR खोलें। उदाहरण समीक्षित होते हैं और सोर्स से बनते हैं; PR कोई समर्थन नहीं और इस पर प्रोजेक्ट का कोई हस्ताक्षर नहीं होता।

## 9. समस्या निवारण

| लक्षण | कारण | समाधान |
|---|---|---|
| `bwrap: ... permission denied` | Docker के अंदर `bubblewrap` मोड: डिफ़ॉल्ट seccomp और AppArmor प्रोफ़ाइल उसके लिए ज़रूरी `unshare` और `mount` कॉल रोकती हैं। | डिफ़ॉल्ट `auto` मोड रखें, जो कंटेनर में Landlock इस्तेमाल करता है। वरना साथ आने वाली प्रोफ़ाइल `docker/seccomp-evey.json` लगाएँ (इमेज में `/etc/evey/seccomp.json` पर भी), जो जान-बूझकर सुरक्षा घटाना है: देखें [प्लगइन सैंडबॉक्स सेटअप](sandbox-deployment.md)। |
| `forbidden pattern "fetch(" found in theme JS` | थीम JS स्कैनर ने एक वर्जित API पकड़ी। | थीम को केवल `evey.theme.*` बुलाना चाहिए। क्या रोका जाता है यह देखने के लिए `internal/pkg/themejs/scan.go` पढ़ें। |
| `manifest: external_fetch entry "*.tmdb.org" is a wildcard - use explicit hostnames` | सभी वाइल्डकार्ड इंस्टॉल पर मना होते हैं, कोई अपवाद नहीं। | `api.tmdb.org` इस्तेमाल करें (और हर ज़रूरी स्पष्ट सबडोमेन सूचीबद्ध करें), `*.tmdb.org` या `cdn.*.tmdb.org` नहीं। |
| `manifest: frontend slot "admin.section" not allowed` | प्लगइन फ़्रंटएंड विशेषाधिकार वाले क्षेत्रों से वर्जित हैं। | श्वेतसूची से कोई गैर-महत्वपूर्ण स्लॉट चुनें या प्लगइन पर फिर से सोचें। |
| `cosign verify-blob: failed signature` | सर्वर की सार्वजनिक कुंजी हस्ताक्षर के लिए इस्तेमाल हुई निजी कुंजी से मेल नहीं खाती। | सही कुंजी से फिर से हस्ताक्षर करें, या `EVEY_PLUGINS_COSIGN_PUBLIC_KEY` अपडेट करें। |
| `plugin already installed at version 1.0.0, new version must be higher` | ZIP में पहले से इंस्टॉल वाला संस्करण है। | `plugin.yaml` में `version` बढ़ाएँ, या पहले प्लगइन अनइंस्टॉल करें। |
| प्लगइन रुकता है, लॉग कहता है `memory quota exceeded (… MB > … MB)` | संसाधन मॉनिटर ने मेमोरी सीमा पार करने पर उसे मार दिया (exit 137)। | मेमोरी का इस्तेमाल घटाएँ, या **Admin → प्लगइन** में उसके कार्ड पर प्लगइन की मेमोरी सीमा बढ़ाएँ (`EVEY_PLUGINS_MEMORY_MB` डिफ़ॉल्ट सेट करता है)। |
| फ़्रंटएंड मॉड्यूल नहीं दिखता | प्लगइन सक्षम नहीं, स्लॉट मेल नहीं खाता, या `frontend.src` में `web_root` उपसर्ग ग़लत। | एडमिन प्लगइन पेज (स्थिति बैज) जाँचें, पुष्टि करें कि स्लॉट स्ट्रिंग किसी असली `<PluginSlot name=...>` माउंट से मेल खाती है, नेटवर्क पैनल में `/api/v1/plugins/{id}/assets/...` कॉल देखें। |

## 10. और आगे

- शुरू से अंत तक सुरक्षा मॉडल: [security-model.md](security-model.md)
- होस्ट API की सतह: [api-reference.md](api-reference.md)
- उपलब्ध स्लॉट: [api-reference.md](api-reference.md#slots)
- हर भाषा का SDK कोड: `sdk/`
- संदर्भ प्लगइन और थीम: `examples/plugins/`
