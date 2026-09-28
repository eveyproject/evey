---
source_sha: 2afa372fc288cb82c9e90ad0f1bb879056bb0e14682a438cab6ce64d779bf48d
---
# एक्सटेंशन सैंडबॉक्स और ख़तरा मॉडल

> **आपको कौन सा पृष्ठ चाहिए?** यह पृष्ठ बताता है कि किसी एक्सटेंशन को क़ैद रखने के
> लिए *सर्वर* क्या करता है: परतें, सैंडबॉक्स मोड, हर एक अब भी क्या नहीं रोक सकता,
> और उसे कैसे कॉन्फ़िगर करें। अगर आप एक्सटेंशन लिख रहे हैं और जानना चाहते हैं कि आप
> क्या घोषित कर सकते हैं और क्या ठुकराया जाएगा, तो इसके बजाय
> [एक्सटेंशन सुरक्षा मॉडल](plugins/security-model.md) पढ़ें। दोनों पृष्ठ एक ही विषय को
> उलटे सिरों से देखते हैं और जान-बूझकर एक-दूसरे को दोहराते नहीं।

यह दस्तावेज़ बताता है कि Evey तीसरे पक्ष के एक्सटेंशन पर कौन सी बचाव परतें लगाता है, उन बचावों की सीमाएँ क्या हैं, और मज़बूत अलगाव की ओर आगे की योजना क्या है।

## ख़तरा मॉडल: एक शत्रुतापूर्ण एक्सटेंशन क्या कोशिश करेगा

Evey के एक्सटेंशन अलग OS प्रक्रियाओं के रूप में चलते हैं जो Unix sockets पर होस्ट के साथ gRPC का आदान-प्रदान करती हैं। होस्ट डिफ़ॉल्ट रूप से किसी एक्सटेंशन कोड पर भरोसा नहीं करता। जिन जोखिमों से हम बचाव करते हैं, गंभीरता के क्रम में:

1. **होस्ट RPC में रूप धरना** - दुर्भावनापूर्ण एक्सटेंशन किसी दूसरे एक्सटेंशन का रूप धरकर उसकी अनुमतियाँ पाना चाहता है।
2. **विशेषाधिकार बढ़ोतरी** - एक्सटेंशन अपने सैंडबॉक्स से निकलकर सर्वर उपयोगकर्ता के रूप में मनमाने कमांड चलाता है।
3. **फ़ाइल सिस्टम से रिसाव / उसमें लिखना** - एक्सटेंशन अपनी इंस्टॉल डायरेक्टरी के बाहर के पाथ पढ़ता या लिखता है।
4. **नेटवर्क से चोरी** - एक्सटेंशन डेटा घर भेजने के लिए बाहर जाने वाले कनेक्शन खोलता है।
5. **संसाधनों का दुरुपयोग** - क्रिप्टो माइनिंग, fork bomb, fd ख़त्म करना।
6. **सप्लाई चेन** - एक्सटेंशन लेखक छिपाया हुआ या कंपाइल किया हुआ malware भेजता है जिसे स्थिर विश्लेषक नहीं जाँच सकता।

## बचाव की परतें (मौजूदा)

### 1. इंस्टॉल पर स्थिर विश्लेषण (PREVIEW + INSTALL)

हर एक्सटेंशन ZIP एक अस्थायी डायरेक्टरी में खोली जाती है और [`pluginrunner.AnalyzePlugin`](../internal/adapters/driven/pluginrunner/analyzer.go) उससे गुज़रता है। जाँची जाने वाली श्रेणियाँ, गंभीरता के साथ:

| श्रेणी | गंभीरता | उदाहरण |
|---|---|---|
| `shell_exec` | CRITICAL | `os.system`, `subprocess.run`, `exec()`, `child_process.exec`, PHP `system()`/`passthru()`/`shell_exec()`/`proc_open()`, `preg_replace /e`, Ruby backtick/`Kernel.exec`/`open('|cmd')` |
| `network` | WARNING / CRITICAL | `requests.get`, `fetch`, reverse/bind shells |
| `filesystem` | WARNING / CRITICAL | `/etc/passwd`, `.ssh/`, `rm -rf`, symlink क्रियाएँ |
| `obfuscation` | CRITICAL | `eval`, `exec`, base64 decode, hex अनुक्रम, `__builtins__`, PHP `assert(base64_decode(...))`, shell `base64 -d \| bash`, `echo <blob> \| openssl` |
| `privilege` | CRITICAL | `setuid`, `chmod 4777`, `sudo`, PID 1 को संकेत |
| `cryptomining` | CRITICAL | `stratum+tcp`, `xmrig`, `cryptonight` |
| `runtime_install` | CRITICAL | `pip install`, `npm install`, `curl ... \| sh` |
| `dynamic_import` | CRITICAL | `__import__()`, `importlib.import_module` |
| `sandbox_escape` | CRITICAL | `vm.runIn*`, `process.binding`, `new Function`, Python `__subclasses__()`, `__class__.__bases__`, `__mro__` से class क्रम में घूमना |
| `deserialization` | CRITICAL / WARNING | `pickle.loads`/`pickle.load`, `marshal.loads`, `__reduce__`/`__reduce_ex__`; WARNING: स्पष्ट Loader के बिना `yaml.load()` |
| `native_code` | CRITICAL / WARNING | `ctypes.CDLL`, `ffi.Library`; WARNING: Go `unsafe.Pointer`, `unsafe.Slice`, `unsafe.SliceData`, `unsafe.StringData` |
| `compiled_binary` | WARNING → BLOCK | `.so`, `.dll`, `.exe`, `.wasm`, `.jar` (स्कैनर के लिए अपारदर्शी) |
| `embedded_archive` | WARNING → BLOCK | `.zip`, `.tar.gz` (निकाला जा सकने वाला कोड) |
| `mime_mismatch` | WARNING (गंभीर माना जाता है) | फ़ाइल का एक्सटेंशन चित्र बताता है, सामग्री कुछ और है |
| `suspicious_url` | WARNING | URL literal में ग़ैर-HTTP scheme |
| `unvendored_deps` | WARNING | `requirements.txt`, `package.json` मौजूद |

CRITICAL नतीजे या `mime_mismatch` इंस्टॉल रोकते हैं (`Safe = false`)।

`compiled_binary` और `embedded_archive` **डिफ़ॉल्ट रूप से** इंस्टॉल रोकते हैं, जब तक manifest `native-binary` अनुमति घोषित न करे। स्वीकार करने से पहले व्यवस्थापक Preview चरण में चेतावनी और माँगी गई अनुमति दोनों देखता है।

### 2. Manifest अनुमति की चौकी

एक्सटेंशन की `plugin.yaml` उसकी माँगी अनुमतियाँ घोषित करती है। runner इन्हें दो स्तरों पर लागू करता है:

- **`subprocess`** - बच्चे प्रक्रियाएँ बनाने की अनुमति। इसके बिना LD_PRELOAD seccomp shim interpreted एक्सटेंशन के लिए `execve`/`execveat` मना करता है। (launcher का कर्नेल seccomp `execve` को मना *नहीं* करता - उसे एक्सटेंशन चलाना ही है - पर बच्चे फ़िल्टर + Landlock domain फिर भी विरासत में पाते हैं।)
- **`native-binary`** - `.so` / `.dll` / `.exe` / archives भेजने के लिए ज़रूरी। इंस्टॉल के समय व्यवस्थापक को अपारदर्शी कोड स्वीकार करना होता है।
- **API अनुमतियाँ** (`api.library`, `api.user`, `api.metadata`, `media.read`, `media.write`, `notification.sender`) - हर होस्ट RPC पर [`PluginPermissionInterceptor`](../internal/adapters/driving/grpcapi/plugin_interceptor.go) जाँचता है।

### 3. UID अलगाव (DAC)

एक्सटेंशन प्रक्रिया को UID 1001 (`evey-plugin`) पर उतारा जा सकता है, जो सर्वर के UID (1000, `evey`) से अलग है, ताकि सर्वर के राज़ सादे DAC से ही उसके लिए अपठनीय रहें। इसे `EVEY_PLUGINS_UID_SEPARATION` नियंत्रित करता है (`auto` | `on` | `off`, डिफ़ॉल्ट `auto`):

- `auto` - **सिर्फ़ तब उतारना जब सैंडबॉक्स `basic` पर पहुँचे** (Landlock/bubblewrap की फ़ाइल सिस्टम क़ैद नहीं) - वही एक स्थिति जहाँ DAC ही राज़ों की अकेली रक्षा है। landlock/bubblewrap के तहत छोड़ दिया जाता है, जहाँ यह फ़ालतू है।
- `on` - हमेशा (Landlock/bubblewrap के ऊपर एक अतिरिक्त स्वतंत्र परत)।
- `off` - कभी नहीं।

**उतारने का काम launcher करता है, सर्वर नहीं।** सर्वर बिना root और बिना capabilities रहता है; `evey-plugin-launcher` `cap_setuid` रखता है (एक file capability, `Dockerfile` में `setcap cap_setuid+ep`) और exec से ठीक पहले ख़ुद को साझा group + UID 1001 पर `setgid`/`setuid` करता है। वह 1000 से नीचे का हर लक्ष्य UID ठुकराता है। अगर वह उतार न सके (capability नहीं, `no_new_privs` सेट, या launcher बाइनरी ही गायब), तो नतीजा मोड पर निर्भर है: `basic` में, जहाँ UID ही राज़ों की अकेली दीवार है, एक्सटेंशन **ठुकराया** जाता है (बंद होकर विफल); `landlock` के तहत, जहाँ फ़ाइल सिस्टम पहले से घिरा है, वह चेतावनी देकर एक्सटेंशन को सर्वर के UID पर चला देता है। root के रूप में चलता सर्वर `SysProcAttr` से एक्सटेंशन को सीधे उतारता है और उसे किसी capability की ज़रूरत नहीं। सीधी भाषा में गाइड: [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md)।

इससे क्या मिलता है, **Landlock के बिना वाले कर्नेल पर भी** (सक्रिय होने पर):

- सर्वर के राज़ uid-1001 वाले एक्सटेंशन से DAC-सुरक्षित हैं: `jwt.key` (`0600`) और बैकअप (`0700` डायरेक्टरी + `0600` फ़ाइलें) **रनटाइम पर** सिर्फ़-मालिक बनाए जाते हैं, इसलिए bind-mount किए गए `/data` पर भी सुरक्षा टिकती है; uid-1001 वाला एक्सटेंशन उन्हें नहीं पढ़ सकता। (इमेज `/data/{keys,logs,backups}` पर `chmod 700` भी करती है, पर होस्ट से bind-mount किया गया `/data` अपनी अनुमतियाँ रखता है - गारंटी ऊपर के रनटाइम मोड की है।)
- सर्वर पर `PR_SET_DUMPABLE=0` के साथ मिलकर (देखें [§8](#8-server-process-hardening-pr_set_dumpable0)), मेमोरी में रखी JWT कुंजी और environment के राज़ (`DB_PASSWORD`, `MEILI_MASTER_KEY`) `/proc/<server>/{mem,environ,maps}` से अपठनीय हैं।

सारे एक्सटेंशन UID 1001 साझा करते हैं, इसलिए सिर्फ़ DAC से वे *एक-दूसरे* से अलग नहीं हैं - यही फ़ाइल सिस्टम क़ैद की परत (Landlock / bubblewrap) जोड़ती है। `basic`+uid अलगाव के तहत दुर्भावनापूर्ण एक्सटेंशन किसी भाई-बहन की फ़ाइलें पढ़ या बिगाड़ सकता है; Landlock के बिना एक्सटेंशन को एक-दूसरे से अलग करने के लिए *हर एक्सटेंशन का अलग* UID चाहिए। देखें [§4](#4-syscall-confinement-kernel-seccomp--ld_preload), नीचे की हर मोड वाली तालिका, और [ज्ञात सीमाएँ](#known-limits-of-the-sandbox)।

### 4. Syscall क़ैद (कर्नेल seccomp + LD_PRELOAD)

दो seccomp फ़िल्टर लगभग एक ही denylist लागू करते हैं (फ़र्क़ सिर्फ़ signal syscalls में है - तालिका देखें); दोनों पहले `prctl(PR_SET_NO_NEW_PRIVS)` बुलाते हैं, फिर एक BPF प्रोग्राम लगाते हैं जो वर्जित syscalls को EPERM देता है और बाक़ी सब की अनुमति देता है।

1. **कर्नेल पर लागू (मुख्य परत)।** [`evey-plugin-launcher`](../cmd/evey-plugin-launcher) shim एक्सटेंशन के argv के आगे लगाया जाता है और BPF फ़िल्टर ([`seccomp_linux.go`](../cmd/evey-plugin-launcher/seccomp_linux.go)) **`execve` से ठीक पहले** लगाता है, हर build arch के लिए। फ़िल्टर `execve` के पार एक्सटेंशन में और उसके बाद बने हर बच्चे में विरासत में जाता है। चूँकि इसे launcher लगाता है, `LD_PRELOAD` नहीं, यह **स्थिर/कंपाइल किए गए एक्सटेंशन बाइनरी को भी बाँधता है** - जो कभी `libevey-sandbox.so` लोड नहीं करते और पहले shim को पूरी तरह लाँघ जाते थे। `seccomp_data` के ABI फ़ील्ड में arch मेल न खाए तो `SECCOMP_RET_KILL_PROCESS` होता है (i386/x32 syscall-संख्या के भ्रम को नाकाम करता है)।
2. **LD_PRELOAD (dynamic एक्सटेंशन के लिए दोहरी सुरक्षा)।** [`sandbox/sandbox.c`](../sandbox/sandbox.c) interpreted एक्सटेंशन में पहले से लोड किया जाने वाला एक छोटा constructor है; वही denylist, और साथ में यह एक्सटेंशन पर ही `PR_SET_DUMPABLE=0` लगाता है।

**दोनों में से कौन चलता है, यह सैंडबॉक्स मोड पर निर्भर है** (नीचे हर मोड वाली तालिका देखें): launcher का कर्नेल फ़िल्टर `landlock` और `basic` में लगता है; LD_PRELOAD shim `strict` और `basic` में। `bubblewrap`/`strict` launcher **नहीं** चलाते, इसलिए सादे `bubblewrap` के तहत **स्थिर** एक्सटेंशन बाइनरी को कोई syscall denylist नहीं मिलती - वहाँ उसकी क़ैद bwrap namespaces + फ़ाइल सिस्टम अलगाव है, seccomp नहीं।

रोके गए syscalls, श्रेणी के हिसाब से:

| श्रेणी | रोके गए syscalls |
|---|---|
| प्रक्रिया नियंत्रण / debug | `ptrace`, `process_vm_readv`, `process_vm_writev` |
| प्रक्रिया संकेत | `kill` (दोनों फ़िल्टर); launcher `tgkill` को भी सीधे मना करता है, जबकि LD_PRELOAD shim `tkill`/`tgkill` को सिर्फ़ तब मना करता है जब वे PID 1 (सर्वर) पर लक्षित हों |
| फ़ाइल सिस्टम mount | `mount`, `umount2`, `pivot_root`, `chroot` |
| Namespaces | `unshare`, `setns` |
| कर्नेल मॉड्यूल | `init_module`, `finit_module`, `delete_module` |
| BPF / perf | `bpf`, `perf_event_open` |
| हार्डवेयर I/O | `ioperm`, `iopl` (सिर्फ़ LD_PRELOAD shim) |
| Swap / मेमोरी प्रबंधन | `swapon`, `swapoff` |
| सिस्टम नियंत्रण | `syslog`, `reboot`, `settimeofday`, `clock_settime`, `clock_adjtime`, `adjtimex` |
| Kexec | `kexec_load` (दोनों फ़िल्टर); `kexec_file_load` (सिर्फ़ LD_PRELOAD shim) |
| Setuid परिवार | `setuid`, `setgid`, `setreuid`, `setregid`, `setresuid`, `setresgid` |
| Capabilities | `capset` |
| Keyring | `add_key`, `request_key`, `keyctl` |

`execve`/`execveat` **LD_PRELOAD shim में मना** है (ताकि बिना `subprocess` वाला interpreted एक्सटेंशन बच्चे न बना सके) पर **launcher के कर्नेल फ़िल्टर में मना नहीं** - launcher को ख़ुद एक्सटेंशन का `execve` करना होता है। बने हुए बच्चे launcher का फ़िल्टर + Landlock domain विरासत में पाते हैं, इसलिए वे हर हाल में क़ैद रहते हैं।

**सभी एक्सटेंशन के लिए कोई IP egress नहीं (2026-07-11 से)।** हर एक्सटेंशन को - नेटवर्क अनुमति हो या न हो - `-deny-inet` (landlock/basic) या `--unshare-net` (bubblewrap) दिया जाता है: एक्सटेंशन प्रक्रिया कोई भी `socket(AF_INET)` / `socket(AF_INET6)` नहीं खोल सकती। जिस एक्सटेंशन को नेटवर्क चाहिए वह सीधे नहीं जुड़ता; वह अपने `AF_UNIX` HostService socket (जिसकी अनुमति रहती है) से होस्ट के `Fetch` बिचौलिये को बुलाता है, जो होस्ट की तरफ़ `external_fetch` अनुमत-सूची और SSRF-सुरक्षित जुड़ाव लागू करता है। इससे IP-literal egress का वह छेद (जैसे `169.254.169.254`) बंद होता है जिसे हर होस्ट की DNS अनुमत-सूची और हर पोर्ट का कर्नेल फ़िल्टर बंद नहीं कर सकते थे। देखें [ADR-0014](adr/0014-landlock-plugin-sandbox.md)।

सीमाएँ - ध्यान से पढ़ें:

- **Denylist, allowlist नहीं।** `open`, `read`, `write`, `connect`, `socket`, `unlink`, `mmap`, `mprotect`, `clone` आदि सब की अनुमति है। (`strict` मोड bubblewrap के ऊपर LD_PRELOAD परत जोड़ता है; असली syscall allowlist अभी योजना में है।)
- **फ़ाइल सिस्टम अलगाव एक अलग परत से आता है।** seccomp पाथ नहीं बाँधता; वह Landlock (या bubblewrap) करता है। Landlock और bubblewrap दोनों के बिना वाले कर्नेल पर एक्सटेंशन सबके पढ़ने लायक़ कोई भी फ़ाइल पढ़ सकता है (पर सर्वर के राज़ नहीं - वे DAC-सुरक्षित हैं, देखें §3)।
- **नेटवर्क क़ैद।** हर एक्सटेंशन के लिए सीधा IP egress कटा है, नेटवर्क अनुमति हो या न हो: `-restrict-net` (कोई `-net-connect` छूट नहीं) और `-deny-inet` कर्नेल पर सारे `AF_INET`/`AF_INET6` sockets रोकते हैं। हर पोर्ट या होस्ट का कोई कर्नेल फ़िल्टर नहीं है। नेटवर्क अनुमति वाला एक्सटेंशन सिर्फ़ अपने `AF_UNIX` socket से होस्ट के `Fetch` बिचौलिये के ज़रिए नेटवर्क तक पहुँचता है, जो होस्ट की तरफ़ `external_fetch` अनुमत-सूची लागू करता है।
- **Interpreted एक्सटेंशन के लिए बंद होकर विफल।** अगर runner को `libevey-sandbox.so` न मिले और एक्सटेंशन के पास `subprocess` न हो, तो [`Runner.Start`](../internal/adapters/driven/pluginrunner/runner.go) एक्सटेंशन शुरू करने से मना करता है और `plugin.error` event भेजता है। launcher का कर्नेल seccomp जितना बन पड़े वाला है (seccomp के बिना वाला कर्नेल बाक़ी परतों पर आ जाता है)।

### 5. हर एक्सटेंशन का होस्ट gRPC socket + SO_PEERCRED प्रमाणीकरण

होस्ट हर एक्सटेंशन के लिए `/tmp/evey/plugins/<plugin_id>/host.sock` पर एक Unix socket खोलता है। हर socket को एक अलग gRPC सर्वर परोसता है, जिसका interceptor:

1. `getsockopt(SO_PEERCRED)` से सामने वाले का PID पढ़ता है - **यह मान कर्नेल देता है, userspace में इसे जाली नहीं बनाया जा सकता**।
2. उसकी तुलना उस PID से करता है जिसे runner ने `cmd.Start` के ठीक बाद `SetExpectedPID` से दर्ज किया था।
3. एक्सटेंशन की ID एक closure के ज़रिए gRPC संदर्भ पर लगाता है (ID सुनने वाले socket की पहचान से आती है, कभी तार से नहीं)।

असर:

- कोई एक्सटेंशन `x-plugin-id` header भेजकर दूसरे का रूप नहीं धर सकता (header अनदेखा होता है)।
- कोई एक्सटेंशन दूसरे के होस्ट socket तक पहुँचकर उसके RPC नहीं बुला सकता - अगर वह `connect()` कर भी ले, तो PID जाँच `Unauthenticated: PID mismatch` के साथ ठुकरा देती है।
- जो एक्सटेंशन fork/exec से बच्चा बनाता है, वह होस्ट के कॉल बच्चे से होकर नहीं भेज सकता (अलग PID → ठुकराया)। यह डिज़ाइन के मुताबिक़ है और `subprocess` अनुमति की चौकी से मेल खाता है।

### 6. व्यवस्थापक Preview की चौकी

`POST /api/v1/admin/plugins/preview` स्थिर विश्लेषक चलाता है और बिना इंस्टॉल किए manifest + नतीजे लौटाता है। व्यवस्थापक देखता है:

- माँगी गई अनुमतियाँ (`native-binary`, `subprocess`, `network`, `api.*` समेत)।
- गंभीरता, फ़ाइल, पंक्ति और श्रेणी के साथ सारे नतीजे।
- प्रकार के हिसाब से समूहित निकाले गए URL (literal, template, जोड़)।
- स्रोत का SHA256 (आने वाले इंस्टॉल से मेल खाता है)।

ऊँची अनुमतियों के लिए भरोसे की सीमा व्यवस्थापक है - Evey सतह दिखाता है, व्यवस्थापक स्वीकार या अस्वीकार करता है।

### 7. एक्सटेंशन क्रैश की सूचना

runner एक `plugin.error` domain event प्रकाशित करता है जब:

- सैंडबॉक्स लाइब्रेरी गायब हो और एक्सटेंशन के पास `subprocess` न हो।
- एक्सटेंशन प्रक्रिया शून्य से अलग कोड के साथ बंद हो।
- auth interceptor PID-mismatch का कोई प्रयास लॉग करे (सिर्फ़ चेतावनी स्तर का slog - अभी bus से नहीं जुड़ा)।

सूचना सेवा इसकी सदस्यता लेती है और सेट किए गए माध्यम (web push / ईमेल / webhook) से व्यवस्थापक को ख़बर देती है।

### 8. सर्वर प्रक्रिया की सख़्ती (`PR_SET_DUMPABLE=0`)

शुरू होते समय सर्वर `prctl(PR_SET_DUMPABLE, 0)` बुलाता है ([`cmd/evey/hardening_linux.go`](../cmd/evey/hardening_linux.go))। इससे `/proc/<server>/{mem,environ,maps,fd,…}` का मालिक root हो जाता है, इसलिए सर्वर का UID साझा करने वाली प्रक्रिया उन्हें नहीं पढ़ सकती। यह सबसे ज़्यादा Landlock के बिना वाले विकल्प में मायने रखता है (जैसे पुराना Synology कर्नेल), जहाँ वरना कोई एक्सटेंशन `/proc` से सर्वर की मेमोरी में रखी JWT कुंजी और उसके environment राज़ (`DB_PASSWORD`, `MEILI_MASTER_KEY`) निकाल सकता था। Landlock वाले कर्नेल पर एक्सटेंशन को सर्वर का `/proc` पहले ही मना है; यह हर कर्नेल पर काम करने वाली दोहरी सुरक्षा है। इससे सर्वर के अपनी फ़ाइलें पढ़ने पर असर नहीं पड़ता; विफलता लॉग होती है, घातक नहीं।

## सैंडबॉक्स मोड

`sandbox_mode` `evey.yaml` या `EVEY_PLUGINS_SANDBOX_MODE` से सेट होता है (डिफ़ॉल्ट `auto`)। यह क़ैद का **backend** चुनता है, और syscall/egress का तरीक़ा backend पर निर्भर है: `landlock` और `basic` एक्सटेंशन को launcher से चलाते हैं, जो कर्नेल seccomp denylist + `-deny-inet` लगाता है; `bubblewrap`/`strict` इसके बजाय नेटवर्क को एक namespace (`--unshare-net`) से अलग करते हैं और उनमें launcher seccomp नहीं होता (`strict` सिर्फ़ dynamic के लिए LD_PRELOAD shim जोड़ता है)। जो हर मोड में लागू होता है: UID अलगाव, userspace संसाधन मॉनिटर, और यह तथ्य कि **सीधा IP egress कटा है** (`-deny-inet` या `--unshare-net` से), इसलिए नेटवर्क वाला एक्सटेंशन सिर्फ़ होस्ट के `Fetch` बिचौलिये से बाहर पहुँचता है। runner backend [`applySandbox`](../internal/adapters/driven/pluginrunner/runner.go) में चुनता है; `auto` पहले एक्सटेंशन के शुरू होने पर एक बार तय होता है ([`resolveAutoMode`](../internal/adapters/driven/pluginrunner/runner_landlock.go))।

### `auto` (डिफ़ॉल्ट)

होस्ट जो सबसे अच्छा backend सच में समर्थन करता है, उसे इस क्रम में तय करता है: **landlock → bubblewrap → basic**।

- **landlock** को पसंद करता है: इसे किसी विशेषाधिकार की ज़रूरत नहीं, यह Docker के डिफ़ॉल्ट seccomp + AppArmor के तहत होस्ट में कोई बदलाव किए बिना चलता है, और लागू होने के बाद ढीला नहीं किया जा सकता - कंटेनर वाली मुख्य तैनाती के लिए सही डिफ़ॉल्ट।
- **bubblewrap** पर तभी लौटता है जब Landlock न हो *और* एक फेंकने लायक़ bwrap सैंडबॉक्स सच में बन जाए ([`ProbeSandbox`](../internal/adapters/driven/pluginrunner/sandbox_probe.go) - इस्तेमाल लायक़ userns + mount वाला bare-metal / विशेषाधिकार वाला कंटेनर)।
- वरना **basic**। तय हुआ मोड पहले एक्सटेंशन के शुरू होने पर लॉग होता है।

### `landlock`

- **क़ैद करता है:** launcher के लगाए Landlock LSM allowlist से फ़ाइल सिस्टम - सिस्टम डायरेक्टरियाँ (`/usr`, `/lib`, `/lib64`, `/bin`, `/sbin`, `/etc`, `/app`, `/dev`) सिर्फ़ पढ़ना/चलाना; एक्सटेंशन की अपनी इंस्टॉल डायरेक्टरी + उसकी socket डायरेक्टरी पढ़ना-लिखना; बाक़ी सब (ख़ासकर `/data`, पड़ोसी एक्सटेंशन की डायरेक्टरियाँ) मना। नेटवर्क: **किसी भी एक्सटेंशन के लिए सीधा IP egress नहीं** - `-restrict-net` (कोई `-net-connect` छूट नहीं) और `-deny-inet` सारे `AF_INET`/`AF_INET6` sockets रोकते हैं; नेटवर्क अनुमति वाले एक्सटेंशन सिर्फ़ अपने `AF_UNIX` socket से होस्ट के `Fetch` बिचौलिये के ज़रिए नेटवर्क तक पहुँचते हैं। ऊपर से: कर्नेल + LD_PRELOAD seccomp denylist।
- **चाहिए:** सक्रिय LSM सूची में Landlock वाला कर्नेल ≥ 5.13, और `evey-plugin-launcher` बाइनरी। न mount, न unshare, न विशेषाधिकार - Docker की डिफ़ॉल्ट सुरक्षा प्रोफ़ाइल के तहत साफ़ चलता है।
- **कब चुना जाता है:** `sandbox_mode=landlock` (अगर Landlock लागू न हो सके तो एक्सटेंशन बंद होकर विफल) या `auto` इस पर पहुँचे (आधिकारिक इमेज की सामान्य स्थिति)।
- **सीमाएँ:** हर एक्सटेंशन के लिए सीधा IP egress कटा है (`-deny-inet` Landlock ABI <4 पर भी टिकता है); हर होस्ट की अनुमत-सूची अब कर्नेल में नहीं, होस्ट की तरफ़ `Fetch` बिचौलिये में रहती है। फ़ाइल सिस्टम क़ैद हर हाल में टिकती है।

### `bubblewrap`

- **क़ैद करता है:** 6 अलग namespaces (`--unshare-user/-pid/-uts/-ipc/-cgroup/-net` - `--unshare-net` **हर** एक्सटेंशन पर बिना शर्त लागू होता है; सीधा egress बिचौलिये से होकर जाता है) और एक mount allowlist से फ़ाइल सिस्टम + नेटवर्क + प्रक्रिया-दृश्य - सिर्फ़-पढ़ने वाली सिस्टम डायरेक्टरियाँ, लिखने लायक़ एक्सटेंशन डायरेक्टरी + socket डायरेक्टरी, एक कृत्रिम `/proc` और न्यूनतम `/dev`, एक tmpfs `/tmp`; `/data`, `/var`, `/home`, `/root`, पड़ोसी डायरेक्टरियाँ माउंट ही नहीं होतीं। `applySandbox` इस मोड को `applyBwrapSandbox` पर भेजता है, जो launcher को कभी नहीं बुलाता: launcher का कर्नेल seccomp यहाँ लागू **नहीं** होता, और LD_PRELOAD shim भी नहीं (वह सिर्फ़ `strict` में है)। syscall क़ैद सिर्फ़ अलग namespaces से आती है।
- **चाहिए:** `bwrap` बाइनरी और `mount`/`unshare` syscalls; Docker के भीतर इसके लिए ढीली सुरक्षा चाहिए, इसलिए यह असल में सिर्फ़ bare-metal (या विशेषाधिकार वाले कंटेनर) पर है।
- **कब चुना जाता है:** `sandbox_mode=bubblewrap`, या Landlock के बिना वाले होस्ट पर `auto` जहाँ bwrap जाँच पास हो।
- **सीमाएँ:** `--unshare-cgroup` एक cgroup *namespace* है, संसाधन सीमा नहीं (वे userspace मॉनिटर से आती हैं)। इस मोड में कोई कर्नेल seccomp denylist लागू नहीं होती (ऊपर देखें); `-deny-inet` भी इस्तेमाल नहीं होता क्योंकि `--unshare-net` पहले ही सारे IP sockets रोकता है।

### `strict`

- `bubblewrap` जैसा, **साथ में** ऊपर से LD_PRELOAD seccomp लाइब्रेरी। bubblewrap जैसी ही ज़रूरतें और सीमाएँ। (असली syscall *allowlist* अभी योजना में है।)

### `basic`

- **क़ैद करता है:** UID 1001 पर उतारना + कर्नेल seccomp denylist + `-deny-inet` + जितना बन पड़े उतना Landlock - सब एक्सटेंशन को launcher में लपेटकर, जब launcher बाइनरी मौजूद हो (आधिकारिक इमेज में है)। launcher न हो (कुछ डेवलपमेंट इमेज) तो यह सिर्फ़ LD_PRELOAD + UID उतारने पर आ जाता है।
- **कब चुना जाता है:** `sandbox_mode=basic`, या `auto` इस पर लौटे (Landlock नहीं और bubblewrap बेकार - जैसे बंद किया हुआ पुराना कर्नेल)।
- **सीमाएँ:** *फ़ाइल सिस्टम* क़ैद के लिए यह सबसे कमज़ोर मोड है: जब कर्नेल में Landlock न हो तो **पाथ के हिसाब से कोई फ़ाइल सिस्टम क़ैद नहीं**, इसलिए एक्सटेंशन सबके पढ़ने लायक़ ग़ैर-संवेदनशील फ़ाइलें पढ़ सकता है, और `/proc` में प्रक्रियाओं की *सूची* दिखती है। यह "कोई अलगाव नहीं" **नहीं** है: सर्वर के राज़ DAC-सुरक्षित रहते हैं (uid अलगाव + `PR_SET_DUMPABLE=0`), एक्सटेंशन को कोई IP egress नहीं मिलता (`-deny-inet`), ख़तरनाक syscalls कर्नेल पर रोके जाते हैं (स्थिर बाइनरी समेत), और संसाधनों पर मॉनिटर सीमा लगाता है।

### स्थिति का सार

| तैनाती | फ़ाइल सिस्टम क़ैद | नेटवर्क | ख़तरनाक syscalls | राज़ | निष्कर्ष |
|---|---|---|---|---|---|
| Docker + Landlock (`auto`→landlock) | पाथ के हिसाब से allowlist | सीधा egress नहीं (`-deny-inet`); बिचौलिये से | रोके गए (स्थिर समेत) | सुरक्षित | **मज़बूत** |
| Landlock के बिना Docker (जैसे पुराना NAS, `auto`→basic) | कोई नहीं (सबके पढ़ने लायक़ दिखता है) | कोई IP egress नहीं (`-deny-inet`) | रोके गए (स्थिर समेत) | सुरक्षित (DAC + DUMPABLE + seccomp) | ठीक-ठाक, Landlock के बराबर नहीं |
| Bare-metal (`auto`→bubblewrap) | namespace + mount allowlist | नेटवर्क namespace | namespaces (PID/mount/net) अलग करते हैं; `strict` में LD_PRELOAD seccomp - bwrap मोड में launcher का कर्नेल फ़िल्टर लागू *नहीं* होता | सुरक्षित | **सबसे मज़बूत** |

## सैंडबॉक्स की ज्ञात सीमाएँ

| सीमा | असर | आज का उपाय | योजना |
|---|---|---|---|
| `basic` मोड में पाथ के हिसाब से फ़ाइल सिस्टम क़ैद नहीं (Landlock के बिना होस्ट) | एक्सटेंशन सबके पढ़ने लायक़ ग़ैर-संवेदनशील फ़ाइलें पढ़ सकता है; पड़ोसी एक्सटेंशन की डायरेक्टरियाँ भी दिखती हैं | सर्वर के राज़ DAC-सुरक्षित रहते हैं (uid अलगाव + `PR_SET_DUMPABLE=0`); विश्लेषक `/etc/passwd` जैसे पैटर्न चिह्नित करता है; जहाँ उपलब्ध हो `auto` Landlock/bubblewrap को पसंद करता है | - (जवाब Landlock या bubblewrap है; दोनों पहले से आते हैं) |
| सारे एक्सटेंशन UID 1001 साझा करते हैं | दुर्भावनापूर्ण एक्सटेंशन भाई-बहनों को संकेत भेज सकता है (PID 1 / सर्वर को संकेत seccomp रोकता है) | `PR_SET_DUMPABLE=0` एक ही UID की प्रक्रियाओं के बीच `/proc/<pid>/{environ,mem,maps,fd}` पढ़ना बंद करता है; Landlock/bubblewrap एक्सटेंशन की डायरेक्टरियाँ पूरी तरह अलग रखते हैं | bubblewrap के ज़रिए हर एक्सटेंशन का अलग user namespace |
| Seccomp denylist है, allowlist नहीं | एक्सटेंशन खुलकर `open`/`read`/`write`/(AF_UNIX) `socket`/`connect` कर सकता है; ptrace, mount, namespaces, bpf, kexec, मॉड्यूल लोड, setuid-परिवार, keyring, और IP sockets (सारे एक्सटेंशन) कर्नेल पर रोके जाते हैं, स्थिर बाइनरी समेत - देखें [§4](#4-syscall-confinement-kernel-seccomp--ld_preload) | launcher से कर्नेल seccomp (exec से पहले, स्थिर बाइनरी को ढकता है) + `-deny-inet`; `subprocess` अनुमति की चौकी | `strict` एक्सटेंशन के लिए असली syscall allowlist मोड |
| बिचौलिया SSRF सतह को होस्ट पर ले आता है | बिचौलिये की URL/IP जाँच में कोई बग एक्सटेंशन को होस्ट के ज़रिए किसी आंतरिक पते तक पहुँचा सकता है | बिचौलिया Go में `external_fetch` अनुमत-सूची + SSRF-सुरक्षित जुड़ाव लागू करता है (जुड़ते समय निजी/आरक्षित IP ठुकराए जाते हैं); संचालकों को एक स्वतंत्र परत के रूप में कंटेनर egress firewall जोड़नी चाहिए (`169.254.0.0/16` छोड़ें, DB+Meili अनुमत करें) | हर namespace का nftables / बिचौलिये का अलग egress (V2) |
| छिपाव से regex विश्लेषक को लाँघा जा सकता है | ठाना हुआ हमलावर पैटर्न छिपा सकता है | व्यवस्थापक Preview + अनुमतियों की समीक्षा; लागू करने की सीमा सैंडबॉक्स है, विश्लेषक नहीं | सर्वर पर जोखिम-अंक + एक्सटेंशन लेखक की साख की जाँच |

## `sandbox_mode` कॉन्फ़िगरेशन

`evey.yaml` (या `EVEY_PLUGINS_SANDBOX_MODE` env var) से सेट होता है; हर मोड के विस्तृत ब्योरे के लिए ऊपर [सैंडबॉक्स मोड](#sandbox-modes) खंड देखें:

```yaml
sandbox_mode: auto   # default - resolves landlock → bubblewrap → basic for this host
# sandbox_mode: landlock    # Landlock LSM allowlist + no-IP-egress (-deny-inet) + seccomp (fails closed if Landlock absent)
# sandbox_mode: basic       # UID drop + kernel/LD_PRELOAD seccomp + -deny-inet + best-effort Landlock
# sandbox_mode: bubblewrap  # bwrap namespace isolation (bare-metal / privileged only)
# sandbox_mode: strict      # bubblewrap + LD_PRELOAD seccomp lib
```

`EVEY_PLUGINS_UID_SEPARATION` (`auto` | `on` | `off`, डिफ़ॉल्ट `auto`) एक अलग बटन है जो नियंत्रित करता है कि एक्सटेंशन अलग UID पर चलें या नहीं (देखें [§3](#3-uid-separation-dac)): `auto` इसे तभी चालू करता है जब मोड `basic` पर पहुँचे, `on` इसे हर जगह ज़बरदस्ती लागू करता है (अतिरिक्त स्वतंत्र परत), `off` बंद करता है। launcher से चलता है, `basic` में बंद होकर विफल (सिर्फ़ `landlock` के तहत जितना बन पड़े); `no_new_privs` के साथ नहीं चलता। हर स्थिति का ब्योरा: [plugins/sandbox-deployment.md](plugins/sandbox-deployment.md)।

हर मोड का मिलान (सारे मान होस्ट की क्षमता पर निर्भर - जैसे Landlock को सक्रिय LSM सूची में Landlock वाला कर्नेल ≥ 5.13 चाहिए):

| परत | auto | landlock | basic | bubblewrap | strict |
|---|---|---|---|---|---|
| UID अलगाव (1001) - `EVEY_PLUGINS_UID_SEPARATION`, डिफ़ॉल्ट `auto` | सिर्फ़ basic में चालू | बंद (ज़बरदस्ती के लिए `on`) | चालू | userns से¹ | userns से¹ |
| कर्नेल seccomp (launcher, exec से पहले, स्थिर बाइनरी को ढकता है) | हाँ अगर landlock/basic पर पहुँचे; नहीं अगर bubblewrap पर | हाँ | हाँ | नहीं (bwrap में launcher seccomp नहीं; `applySandbox` bubblewrap/strict को `applyBwrapSandbox` पर भेजता है, जो launcher को कभी नहीं बुलाता) | नहीं (strict bwrap के ऊपर सिर्फ़ LD_PRELOAD shim जोड़ता है - सिर्फ़ dynamic एक्सटेंशन, कोई कर्नेल seccomp नहीं) |
| LD_PRELOAD seccomp (interpreted एक्सटेंशन) | हाँ | हाँ | हाँ | नहीं | हाँ |
| `-deny-inet` (किसी भी एक्सटेंशन के लिए कोई IP socket नहीं) | हाँ | हाँ | हाँ | लागू नहीं (net ns) | लागू नहीं (net ns) |
| फ़ाइल सिस्टम क़ैद | सबसे अच्छे उपलब्ध पर तय | Landlock allowlist | कोई नहीं (सिर्फ़ DAC) | mount allowlist | mount allowlist |
| नेटवर्क (सारे एक्सटेंशन: सीधा IP egress नहीं; egress होस्ट के `Fetch` बिचौलिये से) | `-deny-inet` | `-restrict-net` + `-deny-inet` | `-deny-inet` | `--unshare-net` | `--unshare-net` |
| Namespaces (user/pid/uts/ipc/cgroup) | लागू नहीं | लागू नहीं | लागू नहीं | अलग | अलग |
| संसाधन सीमाएँ | userspace मॉनिटर + `nice` | userspace मॉनिटर + `nice` | userspace मॉनिटर + `nice` | userspace मॉनिटर + `nice` | userspace मॉनिटर + `nice` |

¹ `bubblewrap`/`strict` में launcher इस्तेमाल नहीं होता, इसलिए `cap_setuid` से उतारना लागू नहीं होता; इसकी जगह अलग किया गया **user namespace** UID अलगाव देता है।

नोट: `strict` के लिए असली syscall *allowlist* अभी योजना में है; आज `strict` = bubblewrap + LD_PRELOAD denylist।

अमल की निर्भरताएँ:

- `evey-plugin-launcher` (शुद्ध Go, CGO के बिना) सर्वर बाइनरी के साथ आता है - Landlock + कर्नेल seccomp फ़िल्टर लगाता है। `landlock`, `auto` और `basic` इसका इस्तेमाल करते हैं।
- `bubblewrap`, `Dockerfile` के रनटाइम चरण में `apt-get` से इंस्टॉल होता है (Debian पैकेज, ~200 KB); `bubblewrap`/`strict` इसका इस्तेमाल करते हैं।
- bubblewrap के लिए: होस्ट कर्नेल में बिना विशेषाधिकार वाले user namespaces चालू हों (`kernel.unprivileged_userns_clone=1`, जहाँ यह sysctl मौजूद हो)। Docker के भीतर namespace/mount सेटअप के लिए आमतौर पर `--security-opt seccomp=unconfined` या ढीला AppArmor चाहिए - इसीलिए कंटेनर वाली तैनाती के लिए `auto` Landlock को पसंद करता है।

## एक्सटेंशन का नेटवर्क egress (बिचौलिये से, 2026-07-11 से)

एक्सटेंशन के पास **कोई सीधी नेटवर्क पहुँच नहीं है**: सैंडबॉक्स हर एक्सटेंशन के लिए
सारे IP sockets मना करता है (`--unshare-net` / `-deny-inet`), `network` अनुमति से
परे। जिस एक्सटेंशन को नेटवर्क चाहिए, वह अपने `AF_UNIX` होस्ट socket से होस्ट का
`HostService.Fetch` RPC बुलाता है, जो `network` अनुमति से बँधा है। होस्ट
(`internal/adapters/driving/pluginhost/fetch.go`) फिर Go में:

- जाँचता है कि लक्ष्य होस्ट एक्सटेंशन की घोषित `external_fetch` अनुमत-सूची में है
  (अब हर होस्ट का असली अनुबंध, सिर्फ़ DNS का इशारा नहीं) - एक सटीक होस्ट, बड़े-छोटे
  अक्षरों के भेद के बिना मिलाया गया (मिलानकर्ता `*.domain` वाला रूप भी समझता है, पर
  इंस्टॉलर wildcard प्रविष्टियाँ ठुकरा देता है, इसलिए कोई वहाँ तक कभी नहीं पहुँचती);
- scheme को `http`/`https` और तरीक़े को `GET`/`HEAD` तक सीमित करता है;
- SSRF-सुरक्षित client (`internal/pkg/ssrf`) से जुड़ता है, जो जुड़ते समय DNS हल होने
  के **बाद** निजी/आरक्षित/link-local IP ठुकराता है, ताकि DNS-rebinding और redirects
  को फिर जाँचा जाए;
- सर्वर के अपने कोई क्रेडेंशियल नहीं जोड़ता, और जवाब के आकार पर सीमा लगाता है।

**संचालक की सख़्ती (अनुशंसित)।** बिचौलिया SSRF सतह को होस्ट पर ले आता है, इसलिए
`evey-server` कंटेनर पर एक स्वतंत्र egress परत जोड़ें जो कोड पर भरोसा न करे:
`169.254.0.0/16` (क्लाउड मेटाडेटा / link-local) छोड़ दें और आंतरिक subnet को
Postgres + Meilisearch endpoints तक सीमित करें। तब बिचौलिये की जाँच में कोई बग भी
एक नेटवर्क दीवार से टकराता है; दोनों परतें स्वतंत्र रूप से विफल होती हैं।

**बिचौलिया क्या नहीं ढकता।** पुरानी DNS अनुमत-सूची को लाँघने के तरीक़े (IP literals,
अपना resolver साथ लाने वाला एक्सटेंशन, SNI/Host चालों से हल हुए कनेक्शन का दोबारा
इस्तेमाल) सीधे egress से जुड़े थे और उसके साथ ख़त्म हो गए: एक्सटेंशन प्रक्रिया कोई भी
IP socket नहीं खोल सकती। जो बचता है, वह सीमा है जिसे एक्सटेंशन सुरक्षा मॉडल पहले ही
बताता है: बिचौलिया नियंत्रित करता है कि कॉल *कहाँ* जाता है, *क्या* लेकर जाता है यह
नहीं, इसलिए डेटा अनुमत-सूची वाले किसी होस्ट तक फिर भी जा सकता है।

**उपाय के सुझाव**:
- `network` अनुमति को "हाथ से ऑडिट करें" मानें: सिर्फ़ उन्हीं होस्ट को अनुमति दें
  जिन पर आप एक्सटेंशन का डेटा भरोसे से सौंपें, और पढ़ें कि एक्सटेंशन उन्हें क्या
  भेजता है।
- संचालकों को एक्सटेंशन रनटाइम चलाने वाले होस्ट का बाहर जाने वाला ट्रैफ़िक देखते रहना
  चाहिए (Prometheus + flow exporter, या सीधे `ss -tnp | grep <plugin-pid>`)।

**योजना V2** (1.0 के बाद):
- बिचौलिये के पीछे एक स्वतंत्र परत के रूप में कंटेनर-स्तर का nftables egress
  firewall।
- अनुरोध-स्तर की जाँच वाला एक वैकल्पिक egress HTTP proxy।

## नेटवर्क पर खुलापन (एक्सटेंशन ख़तरा मॉडल के बाहर)

ऊपर का सब कुछ एक्सटेंशन से जुड़ा है। इस सर्वर की हमले की सतह का दूसरा आधा यह है कि
उस तक कौन पहुँच सकता है, और वह दो अटल नियमों से तय होता है जिन्हें यहाँ कहना
ज़रूरी है, क्योंकि "Evey को सुरक्षित रूप से कैसे खोलें" ढूँढने वाला पाठक इसी फ़ाइल से
शुरू करता है:

- **प्रकाशित करने के लिए एक ही listener है, और वह gRPC वाला नहीं।** native gRPC
  पोर्ट (`EVEY_SERVER_PORT`, डिफ़ॉल्ट 9090) एक आंतरिक परिवहन है जिसका इकलौता
  client उसी प्रक्रिया के भीतर का HTTP gateway है; HTTP API उस पोर्ट से **एक ज़्यादा**
  है। gRPC listener डिफ़ॉल्ट रूप से loopback पर bind होता है
  (`EVEY_GRPC_BIND=127.0.0.1`) ठीक इसीलिए कि नीचे की रिमोट-एक्सेस चौकी HTTP
  middleware है और उसे नहीं ढकती। उस bind को चौड़ा करना संभव है और शुरू होते समय
  चेतावनी लॉग होती है; तब उसे firewall से बचाना संचालक का काम है।
- **रिमोट-एक्सेस चौकी LAN या WAN का फ़ैसला भरोसेमंद प्रॉक्सी की सीमा पर जाँचे गए
  एक ही client IP से करती है, कभी किसी अनुरोध header से नहीं।** कोई client
  `X-Forwarded-For` से `127.0.0.1` होने का दावा करके स्थानीय नहीं माना जा सकता, और जो
  अनुरोध कभी उस सीमा से गुज़रा ही नहीं वह भरोसे के बजाय WAN (ठुकराया गया) माना जाता
  है। ध्यान दें कि यह एक *तैनाती* नियंत्रण है ("यह इंस्टेंस मेरे अपने नेटवर्क के लिए
  है"), प्रमाणीकरण की परत नहीं - डेटा की रक्षा प्रमाणीकरण करता है।

ब्योरा और वेरिएबल की पूरी सूची: [configuration.md](configuration.md#remote-access-who-may-connect-from-outside-your-network)
और [deployment-bare-metal.md](deployment-bare-metal.md)।

## लॉग में क्या हो सकता है

सर्वर की लॉग फ़ाइल वही है जिसे व्यवस्थापन के लॉग रूट (tail, खोज, लाइव) परोसते हैं,
जो बग रिपोर्ट में पहुँचती है और जिसे बैकअप साथ ले जाता है, इसलिए क्रेडेंशियल **लिखते
समय** साफ़ कर दिए जाते हैं (`internal/pkg/logger/redact.go`): जिस attribute की key
किसी राज़ का नाम हो (`authorization`, `token`, `access_token`, `refresh_token`,
`api_key`, `password`, `pin`, `secret`, ...) उसे पूरा बदल दिया जाता है, और कोई भी
string - संदेश समेत - जिसमें bearer, JWT या `secret=value` जोड़ी हो, साफ़ की जाती है।
व्यवस्थापन के लॉग रूट यही नियम उन पंक्तियों पर बाहर भेजते समय लागू करते हैं जो इस
सफ़ाई के आने से पहले लिखी गई थीं। कोड के लिए दो नतीजे: कोई ग़ैर-गुप्त मान किसी गुप्त
key के नाम से लॉग नहीं होना चाहिए (वह छिप जाएगा - आज `token_info`, `device_pin` नाम
इस्तेमाल होते हैं), और किसी नए तरह के क्रेडेंशियल को कभी लॉग होने से पहले इस सूची में
जोड़ना होगा।

## कमज़ोरी की रिपोर्ट करना

निजी रूप से रिपोर्ट करें, किसी सार्वजनिक issue में कभी नहीं: `security@eveyproject.org`।
यही पता रिपॉज़िटरी की [सुरक्षा नीति](../SECURITY.md) देती है, और यह एक्सटेंशन
प्रमाणीकरण, अनुमतियों और सैंडबॉक्स से निकलने को भी बाक़ी सब की तरह ढकता है। ऐसे
साझा या सार्वजनिक Evey इंस्टेंस पर परीक्षण न करें जो आपके नहीं हैं।
