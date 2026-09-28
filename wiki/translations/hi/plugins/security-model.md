---
source_sha: 80123a8d7d69b50b13baa0a7c638767ebf54582aa06f7b85e176a6fee202d4ca
---
# एक्सटेंशन सुरक्षा मॉडल: लेखक क्या घोषित कर सकता है

> **आपको कौन सा पृष्ठ चाहिए?** यह पृष्ठ एक्सटेंशन लेखकों के लिए है: अनुमतियों की
> सूची, वे नियम जिन्हें किसी बाहरी fetch को पूरा करना होता है, वे स्लॉट जिन पर आप
> बैठ सकते हैं, और अपडेट पर क्या होता है। अगर आप सर्वर चलाते हैं और जानना चाहते हैं
> कि एक्सटेंशन को कैसे क़ैद रखा जाता है और सैंडबॉक्स अब भी क्या नहीं रोक सकता, तो
> इसके बजाय [एक्सटेंशन सैंडबॉक्स और ख़तरा मॉडल](../SECURITY.md) पढ़ें।

Evey के एक्सटेंशन तीसरे पक्ष का कोड हैं जो सर्वर के साथ-साथ अलग OS प्रक्रियाओं के रूप में चलता है। यह दस्तावेज़ बताता है कि एक दुर्भावनापूर्ण एक्सटेंशन क्या कोशिश कर सकता है, Evey हर रास्ते से कैसे बचाव करता है, और मौजूदा अमल की ईमानदार सीमाएँ क्या हैं।

## ख़तरा मॉडल: manifest पर किस चीज़ का भरोसा है

| ख़तरा | विवरण |
|---|---|
| सर्वर RCE | एक्सटेंशन अपने सैंडबॉक्स से निकलकर सर्वर प्रक्रिया के रूप में मनमाने कमांड चलाता है |
| राज़ों की चोरी | एक्सटेंशन फ़ाइल सिस्टम से JWT कुंजियाँ, डेटाबेस क्रेडेंशियल या उपयोगकर्ता टोकन पढ़ता है |
| क्षैतिज बढ़ोतरी | एक्सटेंशन IPC पर किसी दूसरे एक्सटेंशन का रूप धरकर ऐसे RPC बुलाता है जिनकी उसे अनुमति नहीं |
| नेटवर्क से चोरी | एक्सटेंशन बाहर जाने वाले कनेक्शन खोलकर पकड़ा हुआ डेटा हमलावर के होस्ट को भेजता है |
| DoS / संसाधनों का दुरुपयोग | एक्सटेंशन CPU, मेमोरी, PIDs या डिस्क ख़त्म कर देता है (fork bomb, क्रिप्टो माइनिंग, डिस्क भरना) |
| Frontend से फ़िशिंग | एक्सटेंशन उपयोगकर्ता के ब्राउज़र में फ़ुल-स्क्रीन overlay डालकर क्रेडेंशियल चुराता है |
| सप्लाई चेन | लेखक छिपाया हुआ या कंपाइल किया हुआ कोड भेजता है जिसे स्थिर विश्लेषण नहीं जाँच सकता |

## बचाव की परतें

### 1. Manifest जाँच (इंस्टॉल के समय)

हर `plugin.yaml` खोलने से पहले जाँचा जाता है। इंस्टॉल ठुकराया जाता है अगर:

- `kind` नहीं है या `plugin`/`theme` नहीं है
- `kind: plugin` में `language` या `entrypoint` नहीं है
- `kind: theme` `language`, `entrypoint` या `permissions` घोषित करता है
- `frontend[].slot` सर्वर की स्लॉट अनुमत-सूची में नहीं है (देखें [स्लॉट अनुमत-सूची](#slot-whitelist))
- `external_fetch` प्रविष्टियाँ होस्ट अनुमत-सूची के नियम तोड़ती हैं (देखें [बाहरी fetch के नियम](#external-fetch-rules))
- कोई घोषित क्षमता या अनुमति अज्ञात है

सच का स्रोत: `internal/domain/plugin/model.go`, `internal/adapters/driven/pluginrunner/capabilities.go`।

### 2. ZIP को सुरक्षित रूप से खोलना

कोई भी फ़ाइल डिस्क पर लिखने से पहले:

- प्रविष्टियों की सीमा: अधिकतम 5,000 फ़ाइलें
- प्रति फ़ाइल सीमा: 100 MB
- कुल खुले आकार की सीमा: 500 MB
- संपीड़न अनुपात की सीमा: 100:1 (zip bomb पहचान)
- पाथ से बाहर जाने की जाँच: हल हुए पाथ में `..` वाली प्रविष्टियाँ ठुकराई जाती हैं
- Symlinks: बिना शर्त ठुकराए जाते हैं (`ExternalAttrs` के Unix type bits जाँचे जाते हैं)
- setuid/setgid/sticky bits: हर खोली गई फ़ाइल से हटा दिए जाते हैं

स्रोत: `internal/adapters/driven/pluginrunner/zip.go`।

### 3. स्थिर विश्लेषक (इंस्टॉल के समय)

एक्सटेंशन को डेटाबेस में स्वीकार करने से पहले ZIP की हर टेक्स्ट फ़ाइल को regex पर आधारित एक बहुभाषी विश्लेषक स्कैन करता है। पहले चलने वाला एक सामान्यीकरण दौर छिपाने के आम तरीक़ों को बेअसर करता है (alias सुलझाना, string जोड़ को समेटना, f-string फैलाना)।

| श्रेणी | गंभीरता | उदाहरण |
|---|---|---|
| `shell_exec` | CRITICAL | `os.system`, `subprocess.run`, `child_process.exec`, PHP `shell_exec`/`proc_open`/`preg_replace /e`, Ruby backtick/`Kernel.exec`/`open('|cmd')` |
| `network` | WARNING/CRITICAL | `requests.get`, `fetch(`, `net.Dial`, reverse/bind shell के पैटर्न |
| `filesystem` | WARNING/CRITICAL | `/etc/passwd`, `.ssh/`, `rm -rf`, symlink क्रियाएँ, `/proc/<pid>/environ` |
| `obfuscation` | CRITICAL | `eval(`, `base64.b64decode`, hex अनुक्रम, `String.fromCharCode` की शृंखलाएँ, PHP `chr()` शृंखलाएँ, `str_rot13`, `assert(base64_decode(...))` |
| `privilege` | CRITICAL | `setuid`, `chmod 4777`, `sudo`, `os.kill(1, ...)` |
| `cryptomining` | CRITICAL | `stratum+tcp`, `xmrig`, `cryptonight` |
| `runtime_install` | CRITICAL | `pip install`, `npm install`, `curl ... \| sh` |
| `dynamic_import` | CRITICAL | `__import__()`, `importlib.import_module`, `getattr(__builtins__, ...)` |
| `sandbox_escape` | CRITICAL | `vm.runInNewContext`, `process.binding`, `new Function(`, Python `__subclasses__()`, `__mro__` |
| `deserialization` | CRITICAL/WARNING | `pickle.loads`, `marshal.loads`, `__reduce__`; WARNING: Loader के बिना `yaml.load()` |
| `native_code` | CRITICAL/WARNING | `ctypes.CDLL`, `ffi.Library`; WARNING: Go `unsafe.Pointer` |
| `compiled_binary` | WARNING → BLOCK | `.so`, `.dll`, `.exe`, `.wasm`, `.jar` - स्कैनर के लिए अपारदर्शी |
| `embedded_archive` | WARNING → BLOCK | `.zip`, `.tar.gz` - इनमें बिना जाँचा कोड हो सकता है |
| `mime_mismatch` | WARNING (गंभीर माना जाता है) | फ़ाइल का एक्सटेंशन चित्र/फ़ॉन्ट बताता है, सामग्री निष्पादन योग्य है |
| `suspicious_url` | WARNING | URL literal में ग़ैर-HTTP scheme (`ftp://`, `wss://`, `ssh://`) |
| `unvendored_deps` | WARNING | घोषित dependencies वाली `requirements.txt`/`package.json`, पर साथ में मॉड्यूल नहीं |

CRITICAL नतीजे या `mime_mismatch` `Safe = false` कर देते हैं और इंस्टॉल रोकते हैं।

`compiled_binary` और `embedded_archive` इंस्टॉल रोकते हैं, जब तक manifest `native-binary` अनुमति घोषित न करे। स्वीकार करने से पहले व्यवस्थापक Preview चरण में चेतावनी और अनुमति देखता है।

स्रोत: `internal/adapters/driven/pluginrunner/analyzer.go`, `analyzer_normalize.go`।

#### जितना बन पड़े, पक्की रोक नहीं

विश्लेषक regex और पैटर्न पर आधारित है। यह छिपाने की जानी-पहचानी श्रेणियों को बेअसर करता है, पर **जान-बूझकर की गई हर तरकीब को पकड़ने की गारंटी नहीं दे सकता**। श्रेणी-स्तर पर कवरेज के उदाहरण:

| श्रेणी | पकड़ा जाता है | टिप्पणी |
|---|---|---|
| string जोड़ से बना PHP variable-callable | `$f = "sys" . "tem"; $f($cmd)` (एक पंक्ति) और `$x="sys"; $y="tem"; $f=$x.$y; $f($cmd)` (कई चरण, fixed-point तक सुलझाना) | 5 fixed-point दोहराव तक, `$a=$b; $b=$a` जैसे चक्र साफ़ ख़त्म होते हैं |
| नाम से Python builtin ढूँढना | `getattr(__builtins__, "exec")` और `__builtins__.__dict__["exec"]` (literal या सुलझे हुए variable से) | literal ख़तरनाक हो तो dict का bracket indexer Critical चलाता है |
| JS `Function.constructor` से निकलना | `(fn).constructor("...")()` तुरंत कॉल और `const F = (fn).constructor; F("...")` (binding rewrite से संग्रहित संदर्भ ट्रैक होता है) | variable तर्क वाले कॉल `F(code)` को `.constructor(code)` में बदला जाता है और variable-arg पैटर्न पकड़ लेता है |
| कई चरणों में टुकड़ों वाला जोड़ | `$a="s"; $b="y"; $c="s"; $d="t"; $e="e"; $f="m"; $g=$a.$b.$c.$d.$e.$f; $g(...)` | मासूम टुकड़े "ख़तरनाक" मानों से अलग ट्रैक होते हैं; ख़तरे की जाँच सिर्फ़ अंतिम जुड़ी string पर चलती है |

हर तरकीब-श्रेणी का `tests/fixtures/plugins/malicious/` में एक स्थायी regression test है। नई श्रेणी जोड़ने के लिए: (1) fixture commit, (2) विश्लेषक नियम या सामान्यीकरण दौर का विस्तार, (3) पहचान आ जाने पर `installer_security_test.go` `workerANeeded:true → false` करता है।

**दायरे से बाहर (सैंडबॉक्स लागू करता है):** AST-स्तर के गतिशील रूपांतरण - `strrev("metsys")`, `base64.b64decode(...)`, `eval(open("/encoded").read())`, रनटाइम regex.replace। regex विश्लेषक इन्हें नहीं सुलझा सकता; रनटाइम सैंडबॉक्स (Landlock अनुमत-सूची या bwrap mount अनुमत-सूची, साथ में कर्नेल seccomp denylist) + userspace संसाधन मॉनिटर + IPC peer-cred गारंटी देते हैं कि एक्सटेंशन निकल नहीं सकता, `/data` बाहर नहीं भेज सकता, और घोषित अनुमतियों से आगे नहीं बढ़ सकता।

**ज्ञात ग़लत चेतावनियाँ:** विश्लेषक स्रोत कोड की टिप्पणियों और docstrings के भीतर भी मिलान करता है। टिप्पणी में `# pip install foo` वाली पंक्ति `runtime_install` नियम चला देती है। स्वीकार्य: संचालकों से प्रकाशित करने से पहले ऐसी टिप्पणियाँ हटाने को कहना, regex विश्लेषक में हर भाषा के लिए टिप्पणियाँ हटाने से सस्ता है; bwrap mount अनुमत-सूची + userspace संसाधन मॉनिटर रनटाइम रोक को इससे स्वतंत्र बनाते हैं।

#### रनटाइम पर प्रायोगिक पुष्टि

Mount अनुमत-सूची को एक चलते हुए स्टैक पर शुरू से अंत तक एक canary एक्सटेंशन (`tests/fixtures/plugins/canary-sandbox-runtime/`) से जाँचा गया, जो शुरू होते समय सैंडबॉक्स का अपना नज़ारा टटोलता है:

```
ROOT_LIST count=9 dirs=bin,data,dev,etc,lib,proc,sbin,tmp,usr
STAT ok /data type=dir size=60 kids=1 sample=plugins
STAT_BLOCKED /var, /home, /root, /mnt, /media, /opt, /srv, /run (all ENOENT)
STAT_BLOCKED /etc/passwd, /etc/shadow, /data/jwt.key (all ENOENT)
```

`/data` इसलिए नहीं दिखता कि होस्ट का `/data` माउंट है (वह कभी नहीं होता), बल्कि इसलिए कि bubblewrap एक्सटेंशन के अपने पढ़ने-लिखने वाले bind लक्ष्य (`/data/plugins/<id>`) की ख़ाली मूल डायरेक्टरियाँ बना देता है; इसलिए उसमें सिर्फ़ वही एक उप-डायरेक्टरी होती है। कोई `jwt.key`, `backups`, `artworks` नहीं दिखता। `/proc/1/environ` पढ़ा जा सकता है, पर namespace के भीतर PID 1 ख़ुद एक्सटेंशन है (`--unshare-pid`), इसलिए अपना ही environ पढ़ना रिसाव नहीं है।

`runner_bwrap.go` में किसी भी बदलाव के बाद canary फिर चलाएँ, ताकि पुष्टि हो कि mount की बनावट अब भी रोकती है।

**लागू करने की सीमा रनटाइम सैंडबॉक्स (Landlock या bubblewrap) + कर्नेल seccomp + userspace संसाधन मॉनिटर + IPC peer-cred है, विश्लेषक नहीं।** जो एक्सटेंशन कोई CRITICAL पैटर्न विश्लेषक से छिपा ले जाए, वह फिर भी `/data` नहीं पढ़ सकता (Landlock मना करता है / bwrap उसे कभी माउंट नहीं करता), कोई वर्जित syscall नहीं बुला सकता, PID सीमा से आगे fork-bomb नहीं कर सकता, या ऐसे gRPC तरीक़े नहीं बुला सकता जिनकी अनुमति नहीं है। विश्लेषक गहराई वाले बचाव की एक परत है, जो preview के समय साफ़ दुर्भावनापूर्ण कोड सामने लाने और रनटाइम हमले की सतह घटाने में काम आती है - वह चहारदीवारी नहीं है।

### 4. Cosign हस्ताक्षर जाँच

`evey.yaml` की `cosign_policy` कुंजी से सेट होती है:

| नीति | व्यवहार |
|---|---|
| `off` | हस्ताक्षर नहीं जाँचा जाता; `Plugin.Signature` nil है |
| `warn` (डिफ़ॉल्ट) | जाँच विफलता दर्ज होकर व्यवस्थापक को दिखती है पर इंस्टॉल नहीं रोकती; सार्वजनिक कुंजी सेट न हो तो जाँच कुछ नहीं करती |
| `enforce` | जाँच विफलता एक त्रुटि के साथ इंस्टॉल रोक देती है |

स्रोत: `internal/adapters/driven/pluginrunner/cosign.go`।

### 5. CSS सफ़ाई

इंस्टॉल के समय `theme.css` पर लागू होती है। पूरे नियमों के लिए [`docs/themes/security-model.md`](../themes/security-model.md) देखें। सफ़ाई पूरी फ़ाइल ठुकराने के बजाय ख़तरनाक रचनाओं को हटाती या दोबारा लिखती है (चेतावनियाँ दर्ज होती हैं)।

### 6. थीम JS स्कैनर

इंस्टॉल के समय `theme.js` पर लागू होता है। कोई पैटर्न मिल जाए तो इंस्टॉल ठुकराया जाता है। रोके गए पैटर्न की सूची के लिए [`docs/themes/security-model.md`](../themes/security-model.md) देखें।

### 7. रनटाइम सैंडबॉक्स (Landlock / bubblewrap + seccomp + userspace संसाधन मॉनिटर)

> **डिफ़ॉल्ट `landlock` है** (आधिकारिक Docker इमेज पर `sandbox_mode=auto` इसी पर
> पहुँचता है): Landlock LSM की फ़ाइल सिस्टम अनुमत-सूची + `-restrict-net` (किसी भी
> एक्सटेंशन के लिए सीधा IP egress नहीं) + कर्नेल और `LD_PRELOAD` की seccomp
> denylist + `-deny-inet`, सब Docker के डिफ़ॉल्ट seccomp+AppArmor के भीतर, होस्ट में कोई
> बदलाव किए बिना। नक्शे के लिए [`docs/plugins.md`](../plugins.md) और हर मोड की पूरी
> तालिका के लिए [`docs/SECURITY.md`](../SECURITY.md#sandbox-modes) देखें।

नीचे की क़ैद वाली परतें होस्ट की क्षमता के हिसाब से लागू होती हैं। Launcher (और वह
जो कर्नेल seccomp लगाता है) सिर्फ़ **`landlock`** और **`basic`** मोड में इस्तेमाल होता
है: `applySandbox` `bubblewrap`/`strict` को एक अलग bwrap कोड-पथ पर भेजता है जो
launcher को कभी नहीं बुलाता, इसलिए ये दोनों मोड bwrap के अपने namespace अलगाव पर
टिके हैं (और `strict` में LD_PRELOAD shim पर)।

- **UID अलगाव** (`EVEY_PLUGINS_UID_SEPARATION`, डिफ़ॉल्ट `auto`) - सक्रिय होने पर
  हर एक्सटेंशन uid 1001 (`evey-plugin`) पर चला जाता है, जो सर्वर के uid 1000 से
  अलग है, **launcher** पर `cap_setuid` के ज़रिए (सर्वर बिना विशेषाधिकार रहता है)।
  `auto` इसे सिर्फ़ `basic` मोड में (Landlock/bwrap के बिना) चालू करता है, जहाँ DAC
  राज़ों की इकलौती दीवार है; `on` इसे हर जगह ज़बरदस्ती लागू करता है; `off` बंद करता
  है। सक्रिय होने पर `jwt.key` (`0600`) और `/data/{keys,logs,backups}` (सिर्फ़
  मालिक) uid-1001 वाले एक्सटेंशन के लिए DAC से अपठनीय हैं, Landlock के बिना भी।
- **कर्नेल seccomp denylist (सिर्फ़ `landlock`/`basic`)** - `evey-plugin-launcher`
  हर build arch के लिए `execve` से पहले लगाता है, इसलिए यह **स्थिर/कंपाइल किए गए
  एक्सटेंशन बाइनरी को भी** बाँधता है (जो कभी `libevey-sandbox.so` लोड नहीं करते)।
  `ptrace`, `mount`, `unshare`, `setns`, `bpf`, `kexec`, मॉड्यूल लोड, setuid परिवार,
  `capset`, keyring, PID 1 को संकेत आदि रोकता है। `execve` के पार एक्सटेंशन और उसके
  बच्चों में विरासत में जाता है। **`bubblewrap`/`strict` में launcher का यह फ़िल्टर
  लागू नहीं होता** - `strict` सिर्फ़ `LD_PRELOAD` वाला `libevey-sandbox.so` shim
  जोड़ता है (वही denylist, साथ में `execve`/`execveat`, ताकि बिना `subprocess` वाला
  interpreted एक्सटेंशन बच्चे न बना सके), जो सिर्फ़ dynamic एक्सटेंशन के लिए है और
  स्थिर बाइनरी को नहीं ढकता।
- **`-deny-inet`** - **हर** एक्सटेंशन के लिए, नेटवर्क अनुमति हो या न हो, launcher
  कर्नेल पर `socket(AF_INET/AF_INET6)` को EPERM देता है: कोई IP egress नहीं, Landlock
  ABI <4 पर भी। HostService gRPC socket के लिए `AF_UNIX` की अनुमति रहती है।
- **`-deny-fork`** - `subprocess` अनुमति **के बिना** हर एक्सटेंशन के लिए, प्रक्रिया
  बनाना कर्नेल पर EPERM होता है (`fork`, `vfork`, `CLONE_THREAD` के बिना `clone`);
  थ्रेड बनाना अछूता रहता है, इसलिए node, Python और Go के रनटाइम पर असर नहीं पड़ता।
  `clone3` `EPERM` नहीं बल्कि `ENOSYS` लौटाता है, ताकि glibc थ्रेड बनाने में पूरी तरह
  विफल होने के बजाय जाँचने लायक़ `clone(2)` पर लौट आए।
- **सर्वर का `PR_SET_DUMPABLE=0`** - सर्वर शुरू होते ही ख़ुद को non-dumpable चिह्नित
  करता है, ताकि उसी uid वाला एक्सटेंशन `/proc/<server>/{mem,environ,maps}` न पढ़ सके
  (मेमोरी में रखी JWT कुंजी, `DB_PASSWORD`, `MEILI_MASTER_KEY`)।
- **Userspace संसाधन मॉनिटर** - नीचे देखें; cgroups इस्तेमाल नहीं होते।

**फ़ाइल सिस्टम/नेटवर्क backend** क्या जोड़ता है, यह मोड पर निर्भर है:

| मोड | फ़ाइल सिस्टम की क़ैद | नेटवर्क | कब चुना जाता है |
|---|---|---|---|
| `landlock` | Landlock LSM अनुमत-सूची (सिस्टम डायरेक्टरियाँ सिर्फ़ पढ़ने लायक़; एक्सटेंशन डायरेक्टरी + socket डायरेक्टरी पढ़ने-लिखने लायक़; `/data` और उसके भाई-बहन वर्जित) | `-restrict-net` (हर IP egress कटा, पोर्ट/होस्ट के हिसाब से कोई फ़िल्टर नहीं) + `-deny-inet` | `sandbox_mode=landlock` (न हो तो बंद होकर विफल); Landlock वाले कर्नेल पर `auto` - आधिकारिक इमेज की सामान्य स्थिति |
| `bubblewrap` | अलग किए गए namespaces में mount अनुमत-सूची (नीचे देखें) | हमेशा `--unshare-net` (हर एक्सटेंशन, `network` अनुमति से परे) | `sandbox_mode=bubblewrap`; जहाँ Landlock न हो पर bwrap जाँच पास हो वहाँ `auto` (bare-metal / विशेषाधिकार वाला) |
| `strict` | `bubblewrap` जैसा + LD_PRELOAD seccomp | `bubblewrap` जैसा | `sandbox_mode=strict` |
| `basic` | कोई नहीं (सिर्फ़ DAC; `auto` का आख़िरी सहारा) | सिर्फ़ `-deny-inet` | `sandbox_mode=basic`; जब न Landlock न bubblewrap इस्तेमाल हो सके तब `auto` |

`auto` पहली बार किसी एक्सटेंशन के शुरू होने पर एक बार **landlock → bubblewrap →
basic** क्रम में तय होता है और चुना गया मोड लॉग करता है। `basic` का मतलब "कोई अलगाव
नहीं" **नहीं** है: वह फिर भी UID अलगाव, कर्नेल + LD_PRELOAD seccomp, `-deny-inet`,
जितना बन पड़े उतना Landlock, और संसाधन मॉनिटर लागू करता है - वह सिर्फ़ *पाथ के
हिसाब से फ़ाइल सिस्टम की क़ैद* खोता है जब कर्नेल में Landlock न हो (तब एक्सटेंशन सबके
पढ़ने लायक़ ग़ैर-संवेदनशील फ़ाइलें पढ़ सकता है, और `/proc` में प्रक्रियाओं की *सूची*
दिखती है; `/proc/<server>/{mem,environ}` सुरक्षित रहते हैं)।

जब `sandbox_mode` `bubblewrap` या `strict` हो (`evey.yaml` से opt-in), तो हर एक्सटेंशन प्रक्रिया एक bubblewrap namespace के भीतर चलती है, जिसमें:

**Namespace अलगाव (5 अलग):**
- `--unshare-user` - अपना user namespace, UID mapping
- `--unshare-pid` - अलग PID namespace
- `--unshare-uts` - अलग hostname
- `--unshare-ipc` - अलग IPC
- `--unshare-cgroup` - अलग cgroup namespace
- `--unshare-net` - नेटवर्क namespace **हमेशा** अलग, हर एक्सटेंशन के लिए `network` अनुमति से परे; सीधा egress होस्ट की `Fetch` RPC से होकर जाता है

**Mount अनुमत-सूची:**

सैंडबॉक्स रनटाइम के लिए ज़रूरी सिर्फ़-पढ़ने वाले सिस्टम पाथ की एक **चुनी हुई अनुमत-सूची** bind-mount करता है (interpreters, साझा लाइब्रेरी, SSL, DNS), और डेटा पाथ कभी नहीं खोलता। पूरे `/` का bind **नहीं** होता: सिर्फ़ नीचे की डायरेक्टरियाँ माउंट होती हैं, इसलिए namespace के भीतर `/data`, `/var`, `/home`, `/root` और पड़ोसी एक्सटेंशन की डायरेक्टरियाँ होती ही नहीं।

| पाथ | पहुँच | कारण |
|---|---|---|
| `/usr`, `/bin`, `/sbin` | सिर्फ़ पढ़ना | Interpreters और सिस्टम बाइनरी |
| `/lib`, `/lib64` | सिर्फ़ पढ़ना (`--ro-bind-try`) | साझा लाइब्रेरी (जो पाथ न हों, distro के हिसाब से छोड़ दिए जाते हैं) |
| `/etc/ssl`, `/etc/ca-certificates`, `/etc/resolv.conf`, `/etc/nsswitch.conf`, `/etc/{alpine,os}-release` | सिर्फ़ पढ़ना (`--ro-bind-try`) | TLS भरोसा भंडार, DNS समाधान, रनटाइम पहचान। पूरा `/etc` **नहीं**: `/etc/passwd`, `/etc/shadow`, `/etc/ssh` कभी माउंट नहीं होते। |
| `<plugin_dir>` | पढ़ना-लिखना | एक्सटेंशन की अपनी इंस्टॉल डायरेक्टरी |
| `<plugin_sock_dir>` | पढ़ना-लिखना | हर एक्सटेंशन की IPC socket डायरेक्टरी (`/tmp` tmpfs के बाद फिर से bind) |
| `/proc` | Proc-mount | रनटाइम के लिए ज़रूरी |
| `/dev` | कृत्रिम (`--dev`) | उपकरणों का न्यूनतम सेट (null, zero, full, random, urandom, tty, pts) - कोई भौतिक उपकरण नहीं |
| `/tmp` | tmpfs | अस्थायी काम की जगह; होस्ट का `/tmp` नहीं दिखता |

**माउंट न होने वाले** पाथ: `/data`, `/var`, `/home`, `/root`, `/run`, `/etc/passwd`, `/etc/shadow`, `/etc/ssh`, दूसरे एक्सटेंशन की डायरेक्टरियाँ।

स्रोत: `internal/adapters/driven/pluginrunner/runner_bwrap.go` में `buildBwrapCmd`।

**Userspace संसाधन मॉनिटर** (हर एक्सटेंशन के लिए):

संसाधन सीमाएँ userspace के एक sweeper से लागू होती हैं (cgroups से नहीं, जो बिना विशेषाधिकार वाले Docker कंटेनर में बेकार हैं)। मॉनिटर लगभग हर 2 s `/proc/<pid>/{statm,status,stat}` देखता है और कोई सीमा पार होते ही एक्सटेंशन को मार देता है। शुरू करते समय `nice` लगाया जाता है ताकि कोई एक्सटेंशन सर्वर को CPU से कभी भूखा न रखे।

- मेमोरी सीमा (RSS, डिफ़ॉल्ट 256 MiB) - `/proc/<pid>/statm` से
- PID/thread संख्या की सीमा (डिफ़ॉल्ट 64) - `/proc/<pid>/status` से
- CPU प्राथमिकता - `nice` से (डिफ़ॉल्ट 10)
- डिस्क सीमा (इंस्टॉल डायरेक्टरी, डिफ़ॉल्ट 100 MiB) - लगभग हर 30 s एक recursive `filepath.Walk` से

cgroups छोड़ने के कारण के लिए [ADR-0013](../adr/0013-userspace-plugin-resource-monitor.md) देखें।

नोट: नीचे की bwrap namespace सूची का `--unshare-cgroup` एक अलग cgroup _namespace_ बनाता है (एक्सटेंशन को एक नया cgroup क्रम दिखता है) पर संसाधन सीमाएँ **नहीं** लगाता; वे ऊपर के userspace मॉनिटर से आती हैं।

स्रोत: `internal/adapters/driven/pluginrunner/runner_landlock.go`, `runner_bwrap.go`, `runner_resmon.go`; `cmd/evey-plugin-launcher/{main.go,seccomp_linux.go,landlock_linux.go,droppriv_linux.go}`; `cmd/evey/hardening_linux.go`; `sandbox/sandbox.c`।

### 8. (हटाई गई) DNS अनुमत-सूची

इस दस्तावेज़ के पुराने संस्करणों में बताई गई V1 की `/etc/hosts` वाली DNS अनुमत-सूची
**हटा दी गई** है। `runner_dns.go` अब मौजूद नहीं है; `buildBwrapCmd` `/etc/hosts` को
bind-mount नहीं करता (देखें `internal/adapters/driven/pluginrunner/runner_bwrap.go`
की टिप्पणी: "no /etc/hosts whitelist is bind-mounted here anymore")। उसकी जगह एक
सख़्त तरीक़ा आया: हर एक्सटेंशन के लिए सीधा IP egress पूरी तरह कटा है (`landlock`/
`basic` में `-restrict-net` + `-deny-inet`, `bubblewrap`/`strict` में
`--unshare-net`), और नेटवर्क अनुमति वाले एक्सटेंशन सिर्फ़ होस्ट के `Fetch` बिचौलिये से
नेटवर्क तक पहुँचते हैं, जो होस्ट की तरफ़ `external_fetch` अनुमत-सूची और SSRF-सुरक्षित
जुड़ाव लागू करता है (देखें `docs/SECURITY.md` में [एक्सटेंशन का नेटवर्क egress](../SECURITY.md#plugin-network-egress-brokered-since-2026-07-11))।

### 9. IPC peer-credential प्रमाणीकरण

होस्ट हर एक्सटेंशन के लिए `/tmp/evey/plugins/<plugin_id>/host.sock` पर एक Unix socket खोलता है। हर आने वाला gRPC कॉल `getsockopt(SO_PEERCRED)` से सामने वाले का PID पढ़कर प्रमाणित होता है - कर्नेल का दिया हुआ मान, जिसे userspace में जाली नहीं बनाया जा सकता। PID की तुलना `cmd.Start` के ठीक बाद दर्ज मान से होती है।

असर:
- कोई एक्सटेंशन दूसरे का रूप नहीं धर सकता: `x-plugin-id` headers अनदेखे होते हैं; पहचान सुनने वाले socket से आती है।
- जो एक्सटेंशन fork/exec से कोई बच्चा बनाता है, वह होस्ट के कॉल बच्चे से होकर नहीं भेज सकता (अलग PID → `Unauthenticated`)।

स्रोत: `internal/adapters/driving/pluginhost/`।

### 10. Frontend अलगाव

एक्सटेंशन के frontend मॉड्यूल एक `<iframe>` के भीतर चलते हैं, जिसमें:
- `sandbox="allow-scripts"` - same-origin पहुँच नहीं, फ़ॉर्म जमा नहीं, पॉप-अप नहीं
- `srcdoc` के भीतर CSP `<meta>` टैग: `default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src <external_fetch_hosts>; img-src data: blob:; font-src data:`
- होस्ट की तरफ़ लागू postMessage origin
- पुल की दर-सीमाएँ: `toast` अधिकतम 3 संदेश/सेकंड, 200 अक्षर, कोई HTML नहीं
- `navigate` पाथ अनुमत-सूची: `/admin/*`, `/auth/*`, `/setup/*`, `/profile/security/*` ठुकराए जाते हैं
- `resize` सीमित: 50-800 px
- स्लॉट अनुमत-सूची सर्वर पर (इंस्टॉल) और क्लाइंट पर (render) दोनों जगह लागू

## अनुमतियों की सूची

| अनुमति | यह क्या देती है |
|---|---|
| `network` | एक्सटेंशन बाहर जाने वाले नेटवर्क कनेक्शन बना सकता है। इसे ग़ैर-ख़ाली `external_fetch` सूची के साथ **ज़रूर** मिलाना होता है - उसके बिना इंस्टॉल ठुकराया जाता है। |
| `subprocess` | एक्सटेंशन बच्चे प्रक्रियाएँ बना सकता है। इसके बिना दो परतें लागू होती हैं: LD_PRELOAD seccomp shim interpreted एक्सटेंशन के लिए `execve`/`execveat` रोकता है, और launcher का `-deny-fork` कर्नेल में प्रक्रिया बनाना ही रोक देता है (`fork`/`vfork`/`CLONE_THREAD` के बिना `clone`)। थ्रेड हर हाल में चलते रहते हैं। (launcher का कर्नेल seccomp `execve` को मना नहीं करता - उसे एक्सटेंशन चलाना ही है - पर हर बच्चा कर्नेल फ़िल्टर + Landlock domain विरासत में पाता है।) |
| `native-binary` | एक्सटेंशन `.so`/`.dll`/`.exe`/`.wasm`/`.jar` या भीतर रखे archive भेज सकता है। इंस्टॉल preview पर व्यवस्थापक अपारदर्शी कोड देखता है और उसे स्वीकार करना होता है। |
| `filesystem` | एक्सटेंशन की अपनी डायरेक्टरी से आगे फ़ाइल सिस्टम तक पहुँचने का घोषित इरादा। ऑडिट में लागू; अभी सैंडबॉक्स का पक्का नियम नहीं (देखें ज्ञात सीमाएँ)। |
| `media.read` | एक्सटेंशन `MediaRead` RPC बुला सकता है: मीडिया फ़ाइल मेटाडेटा और स्ट्रीम तक पहुँच पढ़ना। |
| `media.write` | एक्सटेंशन `MediaWrite` RPC बुला सकता है: मीडिया रिकॉर्ड बनाना/अपडेट करना। |
| `api.library` | एक्सटेंशन लाइब्रेरी प्रबंधन RPC बुला सकता है: लाइब्रेरी सूचीबद्ध/खोजना, स्कैन शुरू करना। |
| `api.user` | एक्सटेंशन उपयोगकर्ता-दायरे वाले RPC बुला सकता है (अपनी प्रोफ़ाइल, पसंदें पढ़ना)। |
| `api.metadata` | एक्सटेंशन मेटाडेटा RPC बुला सकता है: खोजना और संवर्धन डेटा लिखना। |
| `notification.sender` | एक्सटेंशन सूचना-घटनाएँ प्रकाशित कर सकता है (web push, ईमेल, webhook)। |
| `webhook.receiver` | एक्सटेंशन ख़ुद को webhook लक्ष्य के रूप में दर्ज कर सकता है (बाहरी सेवाओं से HTTP POST पाता है)। |
| `hardware.gpu` | **सबसे ऊँचे जोखिम वाली अनुमति - सिर्फ़ व्यवस्थापक, first-party स्तर।** होस्ट का DRM render node देती है (`-render-dev`, असली `renderD1xx`, कभी card node नहीं, कभी `/dev/dxg`/`/dev/nvidia*`/`/dev/kfd` नहीं): एक कर्नेल ड्राइवर पर हमले की सतह और ट्रांसकोडर के साथ साझा GPU-मेमोरी सीमा (AMD/Apple/Qualcomm GPU पर प्रक्रियाओं के बीच मेमोरी रिसाव संभव; अटका हुआ GPU ट्रांसकोडिंग रोक सकता है)। इस अनुमति के बिना GPU डिवाइस nodes Landlock की डिवाइस अनुमत-सूची से पूरी तरह बाहर रहते हैं (DAC से परे मना)। यह मिलने पर launcher उसी एक फ़ाइल तक सीमित एक स्पष्ट `IOCTL_DEV` Landlock नियम जोड़ता है (ABI v5+); पुराने Landlock ABIs पर ioctls पर Landlock का नियंत्रण ही नहीं होता, इसलिए पढ़ने+लिखने की पहुँच से ही GPU कमांड भेजना संभव हो जाता है। इंस्टॉलर preview पर ज़ोरदार CRITICAL चेतावनी दिखाता है; इसे सिर्फ़ भरोसेमंद, first-party एक्सटेंशन को दें। |

## बाहरी fetch के नियम

manifest के `external_fetch: [...]` में घोषित होते हैं और इंस्टॉल पर
`validateExternalFetch` (`internal/adapters/driven/pluginrunner/manifest.go`) से
जाँचे जाते हैं। सब या कुछ नहीं: एक भी अमान्य प्रविष्टि पूरे इंस्टॉल को ठुकरा देती है।

प्रविष्टि एक **सिर्फ़ hostname** है। scheme उपसर्ग सहा जाता है और हटा दिया जाता है -
सिर्फ़ hostname रखा जाता है।

| प्रविष्टि | मान्य | कारण |
|---|---|---|
| `api.tmdb.org` | हाँ | ठीक-ठीक hostname, सामान्य रूप |
| `https://api.tmdb.org` | हाँ | scheme पढ़कर हटाया गया, hostname रखा गया |
| `*.tmdb.org` | नहीं | wildcard ठुकराए जाते हैं - हर ज़रूरी subdomain अलग से लिखें |
| `*` | नहीं | वही नियम |
| `192.168.1.1`, `::1` | नहीं | IP literals ठुकराए जाते हैं, दोनों परिवार |

रनटाइम पर होस्ट का बिचौलिया अपनी सीमाएँ जोड़ता है, जिन्हें manifest बढ़ा नहीं सकता:
सिर्फ़ `http` और `https`, सिर्फ़ `GET` और `HEAD`, लक्ष्य होस्ट का आपकी सूची से
case-insensitive मिलान, और DNS हल होने के बाद निजी या आरक्षित पते ठुकराए जाते हैं -
redirects को दोनों नियमों से फिर जाँचा जाता है।

> बिचौलिये का मिलानकर्ता `*.example.com` वाला रूप भी समझता है, पर इंस्टॉलर ऐसी
> प्रविष्टि ठुकरा देता है, इसलिए वह कभी वहाँ तक पहुँचती ही नहीं। स्पष्ट hostnames
> लिखें।

## स्लॉट अनुमत-सूची

एक्सटेंशन सिर्फ़ साफ़ तौर पर अनुमत स्लॉट में इंटरफ़ेस डाल सकते हैं। नाज़ुक रास्ते जान-बूझकर बाहर रखे गए हैं।

| स्लॉट | प्रमाणीकरण ज़रूरी | टिप्पणी |
|---|---|---|
| `home.section` | हाँ | मुख्य पृष्ठ का सामग्री क्षेत्र |
| `home.hero` | हाँ | Hero बैनर क्षेत्र |
| `library.sidebar` | हाँ | लाइब्रेरी दृश्य का साइडबार |
| `media.detail.sidebar` | हाँ | मीडिया विवरण का साइडबार |
| `player.overlay` | हाँ | प्लेयर overlay इंटरफ़ेस |
| `settings.user.appearance` | हाँ | उपयोगकर्ता सेटिंग्स में दिखावट खंड |
| `sidebar.bottom` | हाँ | नेविगेशन पट्टी का निचला हिस्सा |
| `admin.nav` | हाँ | व्यवस्थापन पैनल में एक मेनू प्रविष्टि जो एक्सटेंशन का अपना अलग पृष्ठ खोलती है - मौजूदा व्यवस्थापन स्क्रीनों में कुछ नहीं डालती |

**कभी अनुमति नहीं:** `admin.*` (`admin.nav` को छोड़कर), `login.*`, `setup.*`, `profile.security.*` - ये रास्ते प्रमाणीकरण/विशेषाधिकार के लिए नाज़ुक हैं और वहाँ एक्सटेंशन का कुछ डालना विशेषाधिकार बढ़ोतरी या फ़िशिंग का रास्ता है।

`admin.nav` की सामग्री भी बाक़ी स्लॉट की तरह अलग रखी जाती है: वह CSP `default-src 'none'` के साथ एक `<iframe sandbox="allow-scripts">` (बिना `allow-same-origin`) के भीतर दिखती है, इसलिए वह व्यवस्थापन क्षेत्र में दिखाई गई तीसरे पक्ष की सामग्री है, एक्सटेंशन को दी गई कोई व्यवस्थापन क्षमता नहीं।

स्रोत: `internal/domain/plugin/slots.go`।

## अपडेट का क्रम

1. व्यवस्थापक एक नई ZIP अपलोड करता है → `PreviewPluginUpdate` स्थिर विश्लेषक + cosign जाँच चलाता है और इंस्टॉल संस्करण की तुलना में बदली अनुमतियों/स्लॉट/`external_fetch` का अंतर लौटाता है।
2. अगर अंतर ख़ाली नहीं है (नई अनुमतियाँ, नए स्लॉट, बदले बाहरी होस्ट), तो अपडेट लागू होने से पहले व्यवस्थापक को साफ़ तौर पर स्वीकार करना होता है (`requires_ack = true`)।
3. स्रोत का hash preview पर लिया जाता है और लागू करते समय जाँचा जाता है (anti-TOCTOU: जो ZIP जाँची गई, वही इंस्टॉल होती है)।
4. अगर cosign नीति `enforce` है, तो लागू करने के चरण पर मेल न खाता हस्ताक्षर अपडेट रोक देता है, भले preview पास हुआ हो।

## अनुमति मॉडल की ज्ञात सीमाएँ

| सीमा | असर | आज का उपाय |
|---|---|---|
| Landlock और इस्तेमाल लायक़ bubblewrap के बिना होस्ट पर `auto` `basic` पर पहुँच सकता है | **पाथ के हिसाब से** फ़ाइल सिस्टम की क़ैद नहीं: एक्सटेंशन सबके पढ़ने लायक़ ग़ैर-संवेदनशील फ़ाइलें पढ़ सकता है, और `/proc` में प्रक्रियाओं की सूची दिखती है | सर्वर के राज़ सुरक्षित रहते हैं (UID अलगाव + `PR_SET_DUMPABLE=0`); कर्नेल seccomp (स्थिर बाइनरी समेत) + `-deny-inet` लागू रहते हैं; तय हुआ मोड पहले एक्सटेंशन के शुरू होने पर लॉग होता है |
| सारे एक्सटेंशन UID 1001 साझा करते हैं | दुर्भावनापूर्ण एक्सटेंशन अपने भाई-बहनों को संकेत भेज सकता है (PID 1 / सर्वर को संकेत seccomp रोकता है) | `PR_SET_DUMPABLE=0` प्रक्रियाओं के बीच `/proc/<pid>/{environ,mem,maps,fd}` पढ़ना रोकता है; Landlock / bubblewrap एक्सटेंशन की डायरेक्टरियों को एक-दूसरे से पूरी तरह अलग रखते हैं |
| `Fetch` बिचौलिया नियंत्रित करता है कि बाहर जाने वाला कॉल *कहाँ* जा सकता है, *क्या* लेकर जाता है यह नहीं | कोई होस्ट `external_fetch` अनुमत-सूची में आ जाए तो बिचौलिया सामग्री नहीं जाँच सकता, इसलिए अनुमत होस्ट को डेटा भेजना फिर भी संभव है | व्यवस्थापकों को सिर्फ़ उन्हीं होस्ट को अनुमति देनी चाहिए जिन पर वे एक्सटेंशन का डेटा भरोसे से सौंपें; बिचौलिया ग़ैर-अनुमत गंतव्य और निजी/आरक्षित IP फिर भी रोकता है |
| LD_PRELOAD seccomp shim सिर्फ़ dynamic (interpreted) एक्सटेंशन को बाँधता है | launcher के बिना `basic` मोड में स्थिर रूप से link किया गया एक्सटेंशन बाइनरी shim की denylist को पूरी तरह लाँघ जाता है | आधिकारिक इमेज हमेशा launcher के साथ आती है (`landlock`/`basic` में कर्नेल seccomp स्थिर बाइनरी को ढकता है); launcher बाइनरी के बिना वाली डेवलपमेंट इमेज पर अविश्वसनीय एक्सटेंशन नहीं चलाने चाहिए |
| उन्नत छिपाव से regex विश्लेषक को लाँघा जा सकता है | ठाना हुआ हमलावर पैटर्न छिपा सकता है | व्यवस्थापक का Preview + अनुमतियों की समीक्षा; लागू करने की सीमा सैंडबॉक्स + संसाधन मॉनिटर + IPC peer-cred है, विश्लेषक नहीं |

## V2 क्या जोड़ेगा

- `Fetch` बिचौलिये के पीछे एक स्वतंत्र परत के रूप में कंटेनर-स्तर का nftables egress firewall (संचालक की सख़्ती, एक्सटेंशन-सैंडबॉक्स की कोई कमी नहीं - हर एक्सटेंशन के लिए सीधा IP egress पहले ही पूरी तरह कटा है)
- अनुरोध-स्तर पर पूरा नियंत्रण लागू करने के लिए एक egress proxy
- seccomp अनुमत-सूची मोड (सख़्त सैंडबॉक्स: सिर्फ़ syscalls का न्यूनतम सेट अनुमत)
- हर अनुरोध पर नियंत्रण के ज़रिए `external_fetch` की wildcard प्रविष्टियों का समर्थन
