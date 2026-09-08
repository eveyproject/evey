---
source_sha: 8e943dfd72345f44add4c70011cd163dc77c351f5f410f69d2165ba7c88ea11d
---

# कंटेनर के बिना इंस्टॉलेशन (Docker के बिना)

यह पृष्ठ Evey को [Docker image](installation.md) के बजाय सीधे किसी होस्ट पर चलाने
के बारे में है (systemd से, किसी init स्क्रिप्ट से, या हाथ से)। सुझाया गया रास्ता
Docker image ही है - उसमें plugin सैंडबॉक्स, हार्डनिंग और सारे runtime पहले से
जुड़े हुए आते हैं। कंटेनर के बिना चलाने का मतलब है कि वही इंतज़ाम **आपको** खुद
दोहराना है: सही बाइनरी अगल-बगल, एक अलग non-root उपयोगकर्ता, आपका कर्नेल जिस
सैंडबॉक्स बैकएंड को सहारा देता है वही, और गुप्त जानकारी वाली डायरेक्टरी सिर्फ़
मालिक के लिए। यह पृष्ठ उसी का क्रमवार तरीका है।

अगर आपको बस आसान इंस्टॉलेशन चाहिए, तो यहीं रुक जाइए और
[installation.md](installation.md) देखिए।

## 1. पूर्वापेक्षाएँ

### बाइनरी (स्रोत से बनाएँ)

दोनों Go बाइनरी बनाएँ और **उन्हें एक ही डायरेक्टरी में रखें**। plugin runner,
launcher को पहले `$EVEY_PLUGIN_LAUNCHER` में देखता है, फिर अपने ही executable के
बगल में (`os.Executable()`), और फिर `$PATH` में - इसलिए दोनों को साथ-साथ रखना
सबसे सरल और सही व्यवस्था है।

```bash
# रिपॉज़िटरी की जड़ से
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags)" -o evey ./cmd/evey
CGO_ENABLED=0 go build -ldflags="-s -w"                                        -o evey-plugin-launcher ./cmd/evey-plugin-launcher

# दोनों को अगल-बगल इंस्टॉल करें, जैसे:
sudo install -m 0755 evey evey-plugin-launcher /opt/evey/
```

`launcherPath()` में खोज का क्रम (`internal/adapters/driven/pluginrunner/runner_landlock.go`):

1. `$EVEY_PLUGIN_LAUNCHER` (यह एक executable फ़ाइल होनी चाहिए) - सीधा override।
2. चल रही `evey` बाइनरी की **उसी डायरेक्टरी** में रखा `evey-plugin-launcher`।
3. `$PATH` पर कहीं मौजूद `evey-plugin-launcher`।

### LD_PRELOAD वाला seccomp shim

`libevey-sandbox.so` shim बनाएँ (छोटी सी एक C फ़ाइल; इसके लिए C कंपाइलर चाहिए):

```bash
gcc -shared -fPIC -nostartfiles -o libevey-sandbox.so sandbox/sandbox.c
sudo install -m 0644 libevey-sandbox.so /app/libevey-sandbox.so
```

runner इसे **`/app/libevey-sandbox.so` इसी तय पथ पर** ढूँढ़ता है (`runner.go` में
`sandboxLibPath`)। यह न मिले तो एक्सटेंशन **शुरू ही नहीं होते** (fail-closed) -
सिवाय dev मोड में चलाने के। यह shim, dynamically linked एक्सटेंशन के लिए एक
seccomp denylist जोड़ता है और Landlock/bubblewrap के ऊपर एक और परत की तरह लगता है।

> Docker image में ओपन-सोर्स बाइनरी, SDK और यह shim `/app` के नीचे रहते हैं (वहाँ
> कोई गुप्त जानकारी नहीं होती)। कंटेनर के बिना चलाते समय आप या तो वही पथ बना सकते
> हैं (`/app/libevey-sandbox.so`), या वहाँ एक symlink रख सकते हैं।

### Runtime

आपके एक्सटेंशन का `entrypoint:` जो भी चलाता है, वह इंस्टॉल करें। runner इन
interpreter को पहचानता है (`runner.go` में `knownInterpreters`): `python3`,
`python`, `node`, `php`, `ruby`, `perl`, `lua`, `bash`, `sh`। कम से कम इतना तो
चाहिए:

- **`python3`** - समुदाय के ज़्यादातर एक्सटेंशन इसी पर चलते हैं।
- **`node`**, **`php`**, **`ruby`**, **`lua`** - सिर्फ़ तभी, जब आपके एक्सटेंशन
  इन्हें इस्तेमाल करते हों।

इनके अलावा बुनियादी runtime निर्भरताएँ:

- **ffmpeg** (ट्रांसकोडिंग के लिए; image में VAAPI ड्राइवर और intro पहचानने के
  लिए `chromaprint` भी साथ आते हैं)।
- **PostgreSQL** (सर्वर और एक डेटाबेस)।
- **Meilisearch** (खोज सूचकांक; या आधिकारिक बाइनरी से इसे embedded चलाएँ)।
- **`bwrap`** (bubblewrap) - सिर्फ़ तभी, जब आप bubblewrap वाला सैंडबॉक्स बैकएंड
  इस्तेमाल करना चाहते हों (नीचे देखें)।
- **`libcap`** (`setcap`) - §4 में बताई गई file capability देने के लिए।

## 2. एक अलग non-root उपयोगकर्ता से चलाएँ

**सर्वर को root से कभी न चलाएँ।** plugin host जिन UID का इस्तेमाल करता है
(`pluginhost/host_server.go` में `pluginUID = 1001`, `pluginGID = 1001`), उनसे
मेल खाते दो बिना-विशेषाधिकार वाले उपयोगकर्ता बनाएँ:

```bash
sudo useradd --system --uid 1000 --shell /usr/sbin/nologin evey
sudo useradd --system --uid 1001 --gid evey --shell /usr/sbin/nologin evey-plugin
```

- सर्वर `evey` (UID 1000) से चलता है।
- `evey-plugin` (UID 1001) वह UID है जिस पर एक्सटेंशन उतार दिए जाते हैं (uid
  separation चालू हो तो launcher यह करता है), ताकि कोई एक्सटेंशन सेंध का शिकार हो
  जाए तो वह एक अलग, बिना-विशेषाधिकार वाले उपयोगकर्ता के रूप में चले और सामान्य
  फ़ाइल-सिस्टम अनुमतियों के बल पर सर्वर की गुप्त जानकारी न पढ़ सके। §4 देखें।

सर्वर शुरू होते ही खुद को **non-dumpable** कर देता है (`PR_SET_DUMPABLE=0`,
`cmd/evey/hardening_linux.go`), ताकि उसी होस्ट पर चलता कोई एक्सटेंशन
`/proc/<server-pid>/{mem,environ}` से कुछ न उठा सके।

## 3. अपने कर्नेल के हिसाब से सैंडबॉक्स बैकएंड चुनें

`EVEY_PLUGINS_SANDBOX_MODE` सेट करें। `auto` (यही डिफ़ॉल्ट है)
**landlock → bubblewrap → basic** के क्रम में देखता है और आपका होस्ट जिन्हें सहारा
देता है उनमें से सबसे मज़बूत चुन लेता है (`runner_landlock.go` में
`resolveAutoMode()`)। fail-closed व्यवहार चाहिए, तो खुद चुनकर बताएँ।

### Landlock (सुझाया गया)

यह हर पथ के लिए फ़ाइल-सिस्टम की allowlist देता है (एक्सटेंशन को सिर्फ़ उसकी अपनी
डायरेक्टरी और socket डायरेक्टरी दिखती है, आपका डेटा या गुप्त जानकारी कभी नहीं),
और साथ में Landlock v4 का TCP-egress फ़िल्टर। इसके लिए **न कोई विशेषाधिकार चाहिए,
न mount, न namespace** - launcher इसे खुद पर लागू करता है और फिर एक्सटेंशन को
`execve` कर देता है।

**कर्नेल की शर्त** - Landlock कर्नेल में कंपाइल भी होना चाहिए *और* सक्रिय LSM
सूची में भी होना चाहिए। runner का `landlockAvailable()`
(`landlock_probe_linux.go`) `landlock_create_ruleset(VERSION)` को आज़माकर देखता
है; syscall मौजूद न हो (कर्नेल `<5.13`) या Landlock सक्रिय LSM सूची में न हो, तो
वह errno लौटाता है। जाँच इस तरह करें:

```bash
uname -r                              # 5.13 या उससे नया चाहिए
cat /sys/kernel/security/lsm          # इसमें "landlock" होना चाहिए
```

अगर उस सूची में `landlock` नहीं है, तो उसे कर्नेल की command line में चालू करें
और सिस्टम दोबारा शुरू करें (सूची हर distro में अलग होती है - मौजूदा प्रविष्टियाँ
वैसी ही रहने दें और आख़िर में `landlock` जोड़ दें):

```
# जैसे GRUB_CMDLINE_LINUX में:
lsm=lockdown,capability,yama,apparmor,bpf,landlock
```

इसके बाद:

```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock    # खुद चुना गया और fail-closed (Landlock लागू न हो पाए तो एक्सटेंशन चलने से मना कर देता है)
# या इसे डिफ़ॉल्ट पर ही रहने दें:
EVEY_PLUGINS_SANDBOX_MODE=auto        # Landlock उपलब्ध हो तो वही चुनता है
```

`landlock` मोड, launcher को `-require-landlock` देता है; तब Landlock लागू न हो
पाने पर launcher एक्सटेंशन को बिना सीमा के चलाने के बजाय गैर-शून्य कोड के साथ
बाहर हो जाता है।

### bubblewrap

अगर Landlock नहीं मिल पा रहा, तो `bwrap` बाइनरी इंस्टॉल करें। bubblewrap हर
एक्सटेंशन को user/pid/uts/ipc/(net) namespace में डाल देता है और साथ में mount की
एक whitelist रखता है। इसके लिए **बिना-विशेषाधिकार वाले user namespace चालू होने
चाहिए** और **`mount` syscall उपलब्ध होना चाहिए** - कंटेनर के बिना चलने वाले
ज़्यादातर Linux पर यह सच है, पर बिना विशेषाधिकार वाले Docker container के भीतर यह
रोक दिया जाता है (कंटेनर में डिफ़ॉल्ट Landlock होने की वजह भी ठीक यही है)। `auto`
तभी bubblewrap चुनता है जब Landlock **मौजूद न हो** और एक आज़माइशी bwrap जाँच
सचमुच सफल हो जाए (`ProbeSandbox` / `sandbox_probe.go`)।

```bash
EVEY_PLUGINS_SANDBOX_MODE=bubblewrap  # bwrap के namespace
# या:
EVEY_PLUGINS_SANDBOX_MODE=strict      # bubblewrap और साथ में LD_PRELOAD वाला seccomp shim
```

जाँच लें कि होस्ट सचमुच namespace और mount वाली यह व्यवस्था होने देता है:

```bash
bwrap --unshare-user --unshare-net --ro-bind / / /bin/true && echo "bwrap OK"
```

### basic (आख़िरी विकल्प)

`EVEY_PLUGINS_SANDBOX_MODE=basic` - इसमें UID उतारा जाता है और LD_PRELOAD वाला
seccomp shim लगता है; launcher फिर भी कर्नेल की seccomp denylist लगाता है और
Landlock मौजूद हो तो जितना बन पड़े उतना उसे भी। इसमें **हर पथ के लिए फ़ाइल-सिस्टम
की कोई सीमा नहीं** होती: एक्सटेंशन UID 1001 पर सिर्फ़ seccomp के साथ चलता है।
इसका इस्तेमाल सिर्फ़ उन पुराने या कसे हुए कर्नेल पर करें जहाँ न Landlock है और न
bubblewrap, और गुप्त जानकारी बचाने के लिए §4 वाले UID separation का सहारा लें।

> **जिस एक्सटेंशन को `subprocess` अनुमति मिली है, उसे launcher चाहिए ही**, basic
> मोड में भी। LD_PRELOAD वाला shim किसी child प्रक्रिया के `exec` के पार नहीं
> टिकता, इसलिए वह उस एक्सटेंशन को सीमा में नहीं रख सकता जिसे subprocess बनाने की
> छूट है - यह काम सिर्फ़ launcher की कर्नेल seccomp कर पाती है (जो `exec` से ठीक
> पहले लगती है और उसके पार भी बनी रहती है)। `evey-plugin-launcher` न मिले (§1
> देखें), तो ऐसा एक्सटेंशन उत्पादन में बिना सीमा के चलने के बजाय **शुरू ही नहीं
> होता** (fail-closed)। जिन एक्सटेंशन के पास यह अनुमति नहीं है, वे ऊपर बताए
> अनुसार LD_PRELOAD वाले shim पर आ जाते हैं।

## 4. UID separation (root के बिना गुप्त जानकारी की सुरक्षा)

UID separation हर एक्सटेंशन को सर्वर (UID 1000) से अलग एक उपयोगकर्ता (UID 1001,
`evey-plugin`) के नीचे चलाता है, ताकि कोई एक्सटेंशन सामान्य फ़ाइल-सिस्टम
अनुमतियों (DAC) के बल पर सर्वर की `jwt.key`, बैकअप या लॉग न पढ़ सके। इसका सबसे
ज़्यादा महत्व **`basic`** मोड में है (जहाँ Landlock वाली पथ-सीमा नहीं होती);
Landlock या bubblewrap के नीचे फ़ाइल-सिस्टम पहले से घिरा होता है, इसलिए वहाँ यह
अतिरिक्त पड़ जाता है।

यह `EVEY_PLUGINS_UID_SEPARATION` से नियंत्रित होता है (`auto` | `on` | `off`,
डिफ़ॉल्ट `auto`):

- `auto` - सिर्फ़ तभी चालू, जब सैंडबॉक्स `basic` पर आकर ठहरे। कंटेनर के बिना चलते
  किसी आधुनिक कर्नेल पर (जहाँ Landlock है) यह जानबूझकर बंद ही रहता है।
- `on` - हमेशा, Landlock/bubblewrap के नीचे भी (एक अलग, अतिरिक्त परत के तौर पर)।
- `off` - कभी नहीं।

**UID उतारने का काम launcher करता है, सर्वर नहीं।** सर्वर non-root ही रहता है और
उसके पास **कोई** capability नहीं होती; छोटी सी `evey-plugin-launcher` बाइनरी के
पास एक ही file capability होती है और वह एक्सटेंशन को exec करने से ठीक पहले खुद को
`setuid` करके 1001 पर ले आती है:

```bash
sudo setcap cap_setuid+ep /opt/evey/evey-plugin-launcher
```

इससे सर्वर पूरी तरह बिना विशेषाधिकार का बना रहता है - सर्वर पर RCE हो जाए तो भी
वह `setuid(0)` नहीं कर सकता, क्योंकि सर्वर के पास कोई cap है ही नहीं; वह सिर्फ़
उस नन्ही launcher के पास है जो कोई इनपुट नहीं लेती (और जो 1000 से नीचे का कोई भी
लक्षित UID मानने से इनकार कर देती है)। यह **जितना बन पड़े उतना** वाला इंतज़ाम है:
launcher UID न उतार पाए (capability न हो, या `no_new_privs` सेट हो), तो वह
चेतावनी लॉग करके एक्सटेंशन को सर्वर के ही UID पर चला देता है, नाकाम नहीं होता -
यानी इससे एक्सटेंशन का शुरू होना कभी नहीं टूटता।

> **`no_new_privs` के साथ यह नहीं चलता।** आप सर्वर को `no-new-privileges` के साथ
> शुरू करते हैं (या systemd में `NoNewPrivileges=yes` के साथ), तो exec के समय file
> capability अनदेखी रह जाती है और uid separation चुपचाप घटकर एक ही UID पर आ जाता
> है (चेतावनी के साथ)। दोनों में से कोई एक ही चुनें।

इसके बजाय सर्वर **root से** चले (यह सुझाया नहीं जाता), तो वह खुद ही एक्सटेंशन को
सीधे 1001 पर उतार देता है और launcher वाली capability की ज़रूरत नहीं पड़ती।

हर परिस्थिति की तस्वीर सरल भाषा वाली गाइड में देखें:
[plugins/sandbox-deployment.md](plugins/sandbox-deployment.md)।

## 5. गुप्त फ़ाइलों की अनुमतियाँ

वही करें जो Docker image करती है (`Dockerfile`): गुप्त जानकारी रखने वाली
डायरेक्टरी **सिर्फ़ मालिक के लिए** रखें, ताकि UID 1001 पर उतारा गया कोई एक्सटेंशन
Landlock के बिना भी उन्हें सामान्य DAC से न पढ़ सके।

```bash
sudo chown -R evey:evey /var/lib/evey
# plugins/ का मालिक सर्वर है (इंस्टॉल वहीं खोले जाते हैं); evey ग्रुप - जिसमें
# evey-plugin भी आता है - इंस्टॉल की गई फ़ाइलें सिर्फ़ पढ़ता और चलाता है:
sudo chmod 750 /var/lib/evey/plugins
# गुप्त डायरेक्टरी पर सिर्फ़ मालिक (न group, न other):
sudo chmod 700 /var/lib/evey/keys /var/lib/evey/logs /var/lib/evey/backups
# खुद JWT हस्ताक्षर कुंजी:
sudo chmod 600 /var/lib/evey/jwt.key      # (पहली बार शुरू होने पर 0600 के साथ अपने आप बनती है)
```

(आपने `EVEY_*_DIR` / `EVEY_JWT_PRIVATE_KEY_PATH` जहाँ भी लगाए हों, पथ उसी हिसाब
से बदल लें।)

## 6. एनवायरनमेंट और कॉन्फ़िगरेशन

चलते समय का सारा कॉन्फ़िगरेशन `EVEY_*` एनवायरनमेंट वेरिएबल से होता है। डेटाबेस,
खोज, भंडारण के पथ, JWT कुंजी का पथ और रिवर्स प्रॉक्सी वाले वेरिएबल ठीक वैसे ही
सेट करें जैसे Docker वाली तैनाती में किए जाते हैं - पूरी तालिका के लिए
[configuration.md](configuration.md) देखें, और डेटाबेस/खोज/रिवर्स प्रॉक्सी की
क्रमवार जानकारी के लिए [installation.md](installation.md)। वह तालिका यहाँ दोहराई
नहीं गई है। कंटेनर के बिना चलाने से जुड़ी कुछ ख़ास बातें:

- `EVEY_DB_HOST` / `EVEY_MEILI_URL` को अपने स्थानीय PostgreSQL / Meilisearch पर
  लगाएँ।
- सर्वर यहाँ भी **सादा HTTP** ही परोसता है - TLS के लिए उसके आगे एक रिवर्स
  प्रॉक्सी लगाएँ (`https://` वाला `EVEY_SERVER_BASE_URL`, और
  `EVEY_TRUSTED_PROXIES`)। [reverse-proxy.md](reverse-proxy.md) देखें।
- **सर्वर दो पोर्ट खोलता है, और उनमें से सिर्फ़ एक ही बाहर खुलना चाहिए।**
  `EVEY_SERVER_PORT` (डिफ़ॉल्ट `9090`) मूल gRPC listener है; आपके क्लाइंट और
  रिवर्स प्रॉक्सी जिस HTTP API से बात करते हैं, वह **उससे एक ज़्यादा** वाले पोर्ट
  पर है (`9091`)। अपना रिवर्स प्रॉक्सी और अपना फ़ायरवॉल सिर्फ़ HTTP पोर्ट पर
  लगाएँ। gRPC पोर्ट भीतरी परिवहन है: दूरस्थ पहुँच का वह नेटवर्क नियंत्रण, जो तय
  करता है कि आपके LAN से बाहर के क्लाइंट जुड़ भी सकते हैं या नहीं, HTTP शृंखला में
  लगा है और **gRPC listener पर लागू नहीं होता**। इसीलिए gRPC listener डिफ़ॉल्ट रूप
  से **सिर्फ़ loopback** पर बँधता है (`EVEY_GRPC_BIND=127.0.0.1`), जिससे कंटेनर के
  बिना चलाते समय वह गलती से बाहर नहीं खुल सकता। उसे बाहर खोलने की कोई वजह हो -
  जैसे किसी दूसरे होस्ट पर चलता कोई मूल gRPC क्लाइंट - तो `EVEY_GRPC_BIND=0.0.0.0`
  सेट करें; सर्वर शुरू होते समय चेतावनी देगा कि उस पर यह नियंत्रण लागू नहीं होता,
  और तब उसे फ़ायरवॉल से बचाना आपकी अपनी ज़िम्मेदारी है।
- माइग्रेशन शुरू होते समय अपने आप चलते हैं; अलग से कोई migrate चरण नहीं है।
- एक्सटेंशन की संसाधन सीमाएँ (`EVEY_PLUGINS_MEMORY_MB`, `_PIDS_MAX`,
  `_DISK_MAX_MB`, `_CPU_NICE`) और सैंडबॉक्स मोड, सर्वर चलते-चलते ही व्यवस्थापक
  Admin → Plugins से बदल सकता है; env वाली वैल्यू सिर्फ़ शुरुआती डिफ़ॉल्ट हैं।

## 7. अपना सैंडबॉक्स जाँचें

सर्वर जिस सैंडबॉक्स बैकएंड पर पहुँचता है, उसे लॉग कर देता है। पहला एक्सटेंशन शुरू
होने के बाद सर्वर का लॉग देखें:

- **auto का नतीजा** (एक ही बार लॉग होता है):
  `plugin sandbox auto-detection complete  selected_mode=landlock`
  (या `bubblewrap` / `basic`)।
- **हर एक्सटेंशन के लिए** (हर बार एक्सटेंशन शुरू होने पर):
  `plugin sandbox applied  sandbox_mode=landlock plugin_id=...`
- **basic पर उतर आया** (यह चेतावनी आपको तब *नहीं* दिखनी चाहिए जब आप Landlock की
  उम्मीद कर रहे हों): `plugin sandbox auto-detection fell back to basic mode: no
  per-path filesystem isolation available …` - इसका मतलब है कि न Landlock मिला,
  न चलता हुआ bwrap। §3 दोबारा देखें।

कर्नेल वाले हिस्से अलग से भी जाँच लें:

```bash
cat /sys/kernel/security/lsm          # Landlock सक्रिय है? (इसमें "landlock" दिखना चाहिए)
grep Seccomp: /proc/<plugin-pid>/status   # 2 = चल रहे एक्सटेंशन पर filter मोड सक्रिय है
```

चल रही किसी एक्सटेंशन प्रक्रिया पर `Seccomp: 2` वाली पंक्ति इस बात की पुष्टि करती
है कि कर्नेल का seccomp फ़िल्टर लगा हुआ है। आपने `EVEY_PLUGINS_SANDBOX_MODE=landlock`
तय कर रखा है और Landlock लागू नहीं हो रहा, तो एक्सटेंशन **शुरू ही नहीं होगा** -
यह fail-closed वाला वादा ठीक वैसे ही काम कर रहा है जैसा उसे करना चाहिए।

---

बाकी सब चीज़ों के लिए (बैकअप, अपडेट, रिकवरी CLI, समस्या निवारण)
[installation.md](installation.md) के Docker वाले खंड यहाँ भी उतने ही लागू होते
हैं - `evey admin` के उप-कमांड और बैकअप की कार्यप्रणाली उसी एक बाइनरी की हैं।
