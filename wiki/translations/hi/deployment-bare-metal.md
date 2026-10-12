---
source_sha: 4aeebbe2feb50999e8470ea3f56b3e47af196ca12cf0100e22527f66ffe278e3
reviewed: true
---
# बिना कंटेनर के

Evey को [Docker इमेज](installation.md) की जगह सीधे किसी होस्ट पर (systemd, किसी
init script से, या हाथ से) कैसे चलाएँ। सुझाया गया रास्ता इमेज ही है: उसमें
प्लगइन सैंडबॉक्स, सुरक्षा की मज़बूती और प्लगइन के runtime पहले से जुड़े हुए
आते हैं। बिना कंटेनर (bare metal) चलाने का मतलब है यह सब खुद दोहराना: सही
binary एक साथ, एक अलग उपयोगकर्ता जो root न हो, वह सैंडबॉक्स बैकएंड जो आपका
kernel देता है, और secret वाले ऐसे फ़ोल्डर जिन्हें सिर्फ़ मालिक पढ़ सके। यह पेज
इन्हें क्रम से बताता है।

## binary बनाएँ और इंस्टॉल करें

दोनों Go binary बनाएँ और उन्हें एक ही फ़ोल्डर में इंस्टॉल करें: plugin runner
launcher को पहले `$EVEY_PLUGIN_LAUNCHER` में ढूँढता है, फिर अपनी executable के
बगल में, फिर `$PATH` पर।

```bash
# रिपॉज़िटरी की जड़ (root) से
CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(git describe --tags --always) -X main.commit=$(git rev-parse HEAD)" -o evey ./cmd/evey
CGO_ENABLED=0 go build -ldflags="-s -w" -o evey-plugin-launcher ./cmd/evey-plugin-launcher
sudo install -m 0755 evey evey-plugin-launcher /opt/evey/

# वैकल्पिक: shell completion और manual page, दोनों binary ही छापती है
/opt/evey/evey completion bash | sudo tee /etc/bash_completion.d/evey >/dev/null
/opt/evey/evey man | sudo tee /usr/local/share/man/man1/evey.1 >/dev/null
```

`evey version` बिना किसी कॉन्फ़िगरेशन के वर्ज़न, commit और build की तारीख छापता
है।

### seccomp shim

`libevey-sandbox.so` बनाएँ (एक C फ़ाइल, C compiler चाहिए) और इसे उस रास्ते पर
इंस्टॉल करें जहाँ runner इसे ढूँढता है, `/app/libevey-sandbox.so`:

```bash
gcc -shared -fPIC -nostartfiles -o libevey-sandbox.so sandbox/sandbox.c
sudo install -D -m 0644 libevey-sandbox.so /app/libevey-sandbox.so
```

यह रास्ता runner में तय है। इस फ़ाइल के बिना प्लगइन शुरू होने से मना कर देते हैं
(fail closed), जब तक सर्वर dev मोड में न चल रहा हो। shim, dynamically linked
प्लगइन के लिए Landlock या bubblewrap के ऊपर एक seccomp मनाही-सूची जोड़ता है।
इमेज इसे SDK के साथ `/app` में रखती है; symlink भी काम करता है।

### Runtime और सर्विस

आपके प्लगइन का `entrypoint:` जो भी चलाता है, उसे इंस्टॉल करें। runner `python3`,
`python`, `node`, `php`, `ruby`, `perl`, `lua`, `bash` और `sh` पहचानता है;
ज़्यादातर कम्युनिटी प्लगइन के लिए `python3` काफ़ी है। फिर वे सर्विस जिनसे सर्वर
बात करता है:

| ज़रूरत | क्यों |
|---|---|
| **ffmpeg** और **ffprobe** | ट्रांसकोडिंग, फ़ाइलों की जाँच; इमेज में VAAPI ड्राइवर और इंट्रो की पहचान के लिए `fpcalc` (chromaprint) भी होते हैं |
| **PostgreSQL** (सर्वर और एक डेटाबेस) और **`pg_dump`** | डेटाबेस; `pg_dump` माइग्रेशन से पहले का अपने-आप वाला dump लिखता है |
| **Meilisearch** | सर्च इंडेक्स; इसके बिना खोज SQL पर चलती है |
| **`bwrap`** (bubblewrap) | सिर्फ़ bubblewrap सैंडबॉक्स बैकएंड के लिए |
| **`setcap`** (libcap) | launcher को उसकी file capability देने के लिए |

अंदर वाला डेटाबेस और सर्च इंजन इमेज के entrypoint की सुविधा है, binary की नहीं:
bare metal पर दोनों आप खुद चलाते हैं और `EVEY_DB_HOST` और `EVEY_MEILI_URL` को
उनकी ओर भेजते हैं।

## एक अलग उपयोगकर्ता से चलाएँ, root से नहीं

सर्वर को कभी root के रूप में न चलाएँ। इमेज की तरह एक ग्रुप और बिना विशेष
अधिकार वाले दो उपयोगकर्ता बनाएँ:

```bash
sudo groupadd --gid 1000 evey
sudo useradd --system --uid 1000 --gid evey --shell /usr/sbin/nologin evey
sudo useradd --system --uid 1001 --gid evey --shell /usr/sbin/nologin evey-plugin
```

- `evey` (UID 1000) सर्वर चलाता है।
- `evey-plugin` (UID 1001) वह उपयोगकर्ता है जिस पर प्लगइन uid separation चालू
  होने पर भेजे जाते हैं, ताकि कोई हैक हुआ प्लगइन एक अलग, बिना विशेष अधिकार वाले
  उपयोगकर्ता के रूप में रहे, जो सामान्य फ़ाइल अनुमतियों की वजह से सर्वर के
  secret नहीं पढ़ सकता।

!!! warning
    ग्रुप id मायने रखती है: launcher प्लगइन को UID 1001 और सर्वर के अपने ग्रुप
    पर भेजता है, और 1000 से नीचे की कोई भी uid या gid मना कर देता है। सिर्फ़
    `useradd --system` से बने ग्रुप को 1000 से नीचे की system id मिलती है, और तब
    `basic` मोड में प्लगइन मना कर दिए जाते हैं।

सर्वर शुरू होते समय खुद को non-dumpable बना लेता है (`PR_SET_DUMPABLE=0`), ताकि
साथ चल रहा कोई प्लगइन `/proc/<server-pid>/{mem,environ}` न पढ़ सके।

## अपने kernel के लिए सैंडबॉक्स बैकएंड चुनें

`EVEY_PLUGINS_SANDBOX_MODE` सेट करें। `auto` (डिफ़ॉल्ट) इस क्रम में देखता है:
landlock, bubblewrap, basic, और वह सबसे मज़बूत चुनता है जो होस्ट पर चलता है।
जब आप fail-closed व्यवहार चाहते हों, तो मोड साफ़ तौर पर सेट करें।

### Landlock (सुझाया गया)

हर रास्ते के हिसाब से फ़ाइल सिस्टम की अनुमति-सूची (प्लगइन अपना फ़ोल्डर और socket
वाला फ़ोल्डर देखता है, आपका डेटा या secret कभी नहीं) और साथ में Landlock v4 का
बाहर जाने वाले TCP का फ़िल्टर। इसके लिए न किसी विशेष अधिकार की ज़रूरत है, न
mount की, न namespace की: launcher इसे खुद पर लागू करता है, फिर प्लगइन चलाता है।

Landlock kernel में compile होना चाहिए और सक्रिय LSM सूची में होना चाहिए। runner
`landlock_create_ruleset` को जाँचता है; यह जाँच तब फ़ेल होती है जब यह syscall
नहीं है (5.13 से पुराना kernel) या जब Landlock सक्रिय सूची में नहीं है। जाँचें:

```bash
uname -r                              # 5.13 या नया चाहिए
cat /sys/kernel/security/lsm          # इसमें "landlock" होना चाहिए
```

अगर उस सूची में `landlock` नहीं है, तो इसे kernel command line में जोड़ें और
reboot करें (मौजूदा एंट्री रखें, यह सूची हर distribution में अलग होती है):

```
# जैसे GRUB_CMDLINE_LINUX में:
lsm=lockdown,capability,yama,apparmor,bpf,landlock
```

फिर या तो `auto` रहने दें, या इसे तय कर दें:

```bash
EVEY_PLUGINS_SANDBOX_MODE=landlock    # fail closed: Landlock लागू न हो सके तो प्लगइन मना कर दिया जाता है
```

`landlock` launcher को `-require-landlock` देता है, जो प्लगइन को बिना घेरे के
चलाने की जगह non-zero के साथ बाहर निकल जाता है।

### bubblewrap

जब Landlock उपलब्ध न हो, तो `bwrap` इंस्टॉल करें। यह हर प्लगइन को user, pid,
uts, ipc और network namespace में रखता है, mount की एक अनुमति-सूची के साथ। इसके
लिए बिना विशेष अधिकार वाले user namespace और `mount` syscall चाहिए, जो ज़्यादातर
bare-metal Linux पर मिलते हैं पर बिना विशेष अधिकार वाले कंटेनर के अंदर बंद होते
हैं, इसीलिए कंटेनर में डिफ़ॉल्ट Landlock है। `auto` bubblewrap को सिर्फ़ तब चुनता
है जब Landlock न हो और `bwrap` की एक परख सफल हो।

```bash
EVEY_PLUGINS_SANDBOX_MODE=bubblewrap  # namespace
EVEY_PLUGINS_SANDBOX_MODE=strict      # namespace और साथ में seccomp shim
```

जाँचें कि होस्ट namespace और mount की यह सेटिंग करने देता है:

```bash
bwrap --unshare-user --unshare-net --ro-bind / / /bin/true && echo "bwrap OK"
```

### basic (आखिरी उपाय)

`EVEY_PLUGINS_SANDBOX_MODE=basic` में uid बदला जाता है और साथ में seccomp shim
होता है; launcher मौजूद हो, तो वह फिर भी अपनी kernel seccomp मनाही-सूची और जितना
हो सके उतना Landlock लगाता है। हर रास्ते के हिसाब से फ़ाइल सिस्टम का कोई घेरा
नहीं है: प्लगइन सिर्फ़ seccomp के साथ UID 1001 पर चलता है। इसे पुराने या कसकर
बंद kernel पर इस्तेमाल करें जहाँ न Landlock चलता है न bubblewrap, और secret
बचाने के लिए नीचे दिए uid separation पर भरोसा करें।

जिस प्लगइन को `subprocess` की अनुमति मिली है, उसे `basic` मोड में भी launcher
चाहिए: shim किसी child process के `exec` के बाद नहीं बचता, सिर्फ़ launcher का
kernel seccomp बचता है। launcher के बिना ऐसा प्लगइन बिना घेरे के चलने की जगह शुरू
होने से मना कर देता है।

## secret को प्लगइन की पहुँच से दूर रखें

uid separation हर प्लगइन को `evey-plugin` (UID 1001) के रूप में चलाता है, जो
सर्वर (UID 1000) से अलग है, ताकि सामान्य फ़ाइल अनुमतियों की वजह से प्लगइन
`jwt.key`, बैकअप या लॉग न पढ़ सके। यह `basic` मोड में मायने रखता है; Landlock या
bubblewrap में फ़ाइल सिस्टम पहले से घिरा होता है, इसलिए वहाँ यह दोहराव है।

`EVEY_PLUGINS_UID_SEPARATION`:

| मान | व्यवहार |
|---|---|
| `auto` (डिफ़ॉल्ट) | सिर्फ़ तब चालू जब सैंडबॉक्स `basic` पर पहुँचे |
| `on` | हमेशा, एक अलग अतिरिक्त परत के रूप में |
| `off` | कभी नहीं |

uid बदलने का काम launcher करता है, सर्वर नहीं। सर्वर root के बिना और बिना किसी
capability के रहता है; launcher के पास एक file capability होती है और वह प्लगइन
चलाने से ठीक पहले UID 1001 पर चला जाता है:

```bash
sudo setcap cap_setuid+ep /opt/evey/evey-plugin-launcher
```

launcher 1000 से नीचे की कोई भी लक्ष्य uid या gid मना कर देता है। जब वह uid नहीं
बदल पाता (capability नहीं है, `no_new_privs` सेट है), तो `basic` मोड वाला प्लगइन
मना कर दिया जाता है, क्योंकि वहाँ uid ही उसकी अकेली दीवार है; Landlock में वह लॉग
में चेतावनी लिखता है और सर्वर की uid पर चलता है।

!!! warning
    uid separation `no_new_privs` के साथ नहीं चलता। अगर सर्वर systemd
    `NoNewPrivileges=yes` के साथ चलता है, तो exec पर file capability अनदेखी हो
    जाती है: `basic` मोड में प्लगइन मना कर दिए जाते हैं, और Landlock में uid
    separation घटकर एक ही uid रह जाता है। एक चुनें।

अगर सर्वर इसकी जगह root के रूप में चलता है (सुझाया नहीं जाता), तो वह प्लगइन को
सीधे 1001 पर भेजता है और capability की ज़रूरत नहीं होती। हर स्थिति की आसान भाषा
में तस्वीर [प्लगइन सैंडबॉक्स सेटअप](plugins/sandbox-deployment.md) में है।

## secret वाली फ़ाइलों की अनुमतियाँ

इमेज की नकल करें: secret वाले फ़ोल्डर सिर्फ़ मालिक के लिए होते हैं, ताकि UID
1001 पर भेजा गया प्लगइन Landlock के बिना भी उन्हें न पढ़ सके।

```bash
sudo chown -R evey:evey /var/lib/evey
# plugins/ सर्वर का है (वह इंस्टॉल वहीं खोलता है); ग्रुप evey,
# जिसमें evey-plugin है, इंस्टॉल की गई फ़ाइलें सिर्फ़ पढ़ता और चलाता है:
sudo chmod 750 /var/lib/evey/plugins
# secret वाले फ़ोल्डर सिर्फ़ मालिक के लिए:
sudo chmod 700 /var/lib/evey/keys /var/lib/evey/logs /var/lib/evey/backups
# JWT पर साइन करने वाली कुंजी खुद (पहले बूट पर 0600 के साथ बनती है):
sudo chmod 600 /var/lib/evey/jwt.key
```

रास्ते वहाँ के हिसाब से बदलें जहाँ आप `EVEY_*_DIR` वेरिएबल और
`EVEY_JWT_PRIVATE_KEY_PATH` भेजते हैं।

## environment कॉन्फ़िगर करें

सारा कॉन्फ़िगरेशन `EVEY_*` environment वेरिएबल से होता है;
[कॉन्फ़िगरेशन रेफ़रेंस](configuration.md) में पूरी टेबल है। bare metal पर चलाने
की खास बातें:

- `EVEY_DB_HOST` और `EVEY_MEILI_URL` को अपने PostgreSQL और Meilisearch की ओर
  भेजें। `EVEY_DB_PASSWORD` डिफ़ॉल्ट `evey` नहीं होना चाहिए।
- रास्तों के Go डिफ़ॉल्ट कंटेनर के लिए चुने गए थे: `EVEY_LOG_DIR` (डिफ़ॉल्ट
  सापेक्ष `logs`), `EVEY_BACKUP_DIR` (`/data/backups`),
  `EVEY_TRANSCODING_CACHE_DIR` (`/data/transcode`), `EVEY_TRICKPLAY_CACHE_DIR`
  (`/cache/trickplay`) और `EVEY_JWT_PRIVATE_KEY_PATH` (`/data/jwt.key`) को अपने
  फ़ोल्डरों पर सेट करें।
- सर्वर सादा HTTP देता है: TLS के लिए आगे एक रिवर्स प्रॉक्सी लगाएँ, `https://`
  वाले **बाहरी URL** के साथ, और प्रॉक्सी को **विश्वसनीय प्रॉक्सी** में डालें
  (**एडमिन → रिमोट एक्सेस**; `EVEY_SERVER_BASE_URL` और `EVEY_TRUSTED_PROXIES`
  बैकअप हैं)। देखें [रिवर्स प्रॉक्सी कॉन्फ़िगरेशन](reverse-proxy.md)।
- **सर्वर दो पोर्ट खोलता है, और सिर्फ़ एक बाहर खुला होना चाहिए।**
  `EVEY_SERVER_PORT` (डिफ़ॉल्ट `9090`) अपना gRPC listener है; जिस HTTP API से
  आपके क्लाइंट और रिवर्स प्रॉक्सी बात करते हैं, वह इस पोर्ट से एक आगे (`9091`)
  है। रिमोट एक्सेस की जाँच, जो तय करती है कि आपके LAN के बाहर के क्लाइंट जुड़
  सकते हैं या नहीं, HTTP वाले रास्ते में लगी है और gRPC listener पर लागू नहीं
  होती, इसीलिए वह डिफ़ॉल्ट रूप से सिर्फ़ loopback पर सुनता है
  (`EVEY_GRPC_BIND=127.0.0.1`)। अगर उसे बाहर खोलने की कोई वजह है, तो
  `EVEY_GRPC_BIND=0.0.0.0` सेट करें; सर्वर बूट पर चेतावनी देता है कि जाँच उस पर
  लागू नहीं होती, और तब उसे फ़ायरवॉल से बचाना आपकी ज़िम्मेदारी है।
- माइग्रेशन सर्वर शुरू होते समय चलते हैं; माइग्रेशन का कोई अलग कदम नहीं है।
- प्लगइन के संसाधनों की सीमाएँ (`EVEY_PLUGINS_MEMORY_MB`, `_PIDS_MAX`,
  `_DISK_MAX_MB`, `_CPU_NICE`) और सैंडबॉक्स मोड चलते-चलते **एडमिन → प्लगइन →
  संसाधन सीमाएँ** से बदले जा सकते हैं; environment के मान शुरुआती डिफ़ॉल्ट हैं।
- एडमिन पैनल से सर्वर रीस्टार्ट इमेज के supervisor पर निर्भर है, और बिना कंटेनर
  चलाने पर खुद को उपलब्ध नहीं बताता: सर्विस को खुद रीस्टार्ट करें।

## सैंडबॉक्स की जाँच करें

सर्वर लॉग में वह बैकएंड लिखता है जो उसने चुना। पहला प्लगइन शुरू होने के बाद
ये देखें:

- एक बार, `auto` के लिए: `plugin sandbox auto-detection complete
  selected_mode=landlock` (या `bubblewrap`, `basic`);
- हर प्लगइन के शुरू होने पर: `plugin sandbox applied sandbox_mode=landlock
  plugin_id=...`;
- वह चेतावनी जो आप नहीं चाहते जब आपको Landlock की उम्मीद थी: `plugin sandbox
  auto-detection fell back to basic mode: no per-path filesystem isolation
  available ...`, यानी न Landlock मिला न चलता हुआ `bwrap`। ऊपर बैकएंड वाला
  हिस्सा फिर से देखें।

kernel की तरफ़ से अलग से पुष्टि करें:

```bash
cat /sys/kernel/security/lsm              # "landlock" दिखाता है
grep Seccomp: /proc/<plugin-pid>/status   # 2 = प्लगइन पर एक seccomp फ़िल्टर सक्रिय है
```

चल रहे प्लगइन प्रोसेस पर `Seccomp: 2` वाली लाइन पुष्टि करती है कि kernel
फ़िल्टर लगा हुआ है। `EVEY_PLUGINS_SANDBOX_MODE=landlock` के साथ और Landlock लागू
न होने पर, प्लगइन शुरू होने से मना कर देता है: यही fail-closed वादा काम कर रहा
है।

बैकअप, अपडेट और रिकवरी की कमांड के लिए कंटेनर वाले पेज जैसे हैं वैसे लागू होते
हैं: [बैकअप और अपडेट](backups.md), [रिकवरी](runbooks/recovery.md) और
[कमांड लाइन](runbooks/cli.md) उसी `evey admin` binary के बारे में हैं।

<!-- sources
launcher lookup: internal/adapters/driven/pluginrunner/runner_landlock.go - launcherPath, launcherBinaryName, resolveAutoMode
shim path and modes: internal/adapters/driven/pluginrunner/runner.go - sandboxLibPath, sandboxModeStrict, pluginUID ; sandbox/sandbox.c ; Dockerfile (gcc line, /app/libevey-sandbox.so)
interpreters: internal/adapters/driven/pluginrunner/runner_start.go - knownInterpreters
basic sandbox, subprocess refusal, uid separation policy: internal/adapters/driven/pluginrunner/runner_sandbox.go - applyBasicSandbox, pluginDropArgs, uidSepApplies, WithUIDSeparation, effectiveMode ("auto-detection complete")
landlock probe: internal/adapters/driven/pluginrunner/landlock_probe_linux.go - landlockAvailable ; bwrap probe: sandbox_probe.go - ProbeSandbox
launcher: cmd/evey-plugin-launcher/main.go (-require-landlock, -uid, -uid-besteffort) ; cmd/evey-plugin-launcher/droppriv_linux.go (uid/gid >= 1000)
hardening: cmd/evey/hardening_linux.go - PR_SET_DUMPABLE
cli: cmd/evey/cli.go (version, completion bash, man)
ports and defaults: internal/config/config.go (EVEY_SERVER_PORT, EVEY_GRPC_BIND, EVEY_LOG_DIR, EVEY_BACKUP_DIR, EVEY_TRANSCODING_CACHE_DIR, EVEY_TRICKPLAY_CACHE_DIR, EVEY_JWT_PRIVATE_KEY_PATH, EVEY_PLUGINS_*, hardFail on password "evey") ; cmd/evey/main.go - httpPort = Port+1
pre-migration dump needs pg_dump: internal/adapters/driven/postgres/migrate_backup.go - dumpDatabase
restart RPC needs the supervisor: docker/entrypoint.sh (EVEY_SUPERVISED)
image permissions: Dockerfile (chmod 750 /data/plugins, chmod 700 keys/logs/backups, setcap)
labels: i18n/en/admin.yaml - remoteAccess.externalUrl, remoteAccess.trustedProxies, plugins.resourceLimits.title
-->
