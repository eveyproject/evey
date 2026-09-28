---
source_sha: d7ed6dd50005bc7e22c522df514264be5d132bc0ca91bd3052d5321a026acad0
---

# एक्सटेंशन तंत्र

Evey के एक्सटेंशन सर्वर में मेटाडेटा प्रदाता, मीडिया की प्रोसेसिंग, webhooks,
सूचनाएँ, frontend मॉड्यूल और CSS थीम जोड़ते हैं। हर एक्सटेंशन एक **अलग-थलग
subprocess** के रूप में चलता है, जो Unix domain socket पर gRPC से होस्ट से बात
करता है; वह जो भी UI साथ लाता है, वह एक **सैंडबॉक्स वाले iframe** में दिखता है और
`postMessage` पुल के ज़रिए होस्ट से बात करता है।

यह पृष्ठ एक नक्शा भर है। ब्योरा तीन अलग दस्तावेज़ों में है:

| दस्तावेज़ | उसमें क्या है |
|---|---|
| **[plugins/dev-setup.md](plugins/dev-setup.md)** | अपना पहला एक्सटेंशन लिखना: पूर्व-शर्तें, ढाँचा, इंस्टॉल और दोहराव का चक्र, हस्ताक्षर, वितरण, गड़बड़ियाँ सुलझाना। **यहीं से शुरू करें।** |
| **[plugins/api-reference.md](plugins/api-reference.md)** | अनुबंध: manifest का ढाँचा, बैकएंड IPC और डेटा स्टोर, `evey.*` वाला frontend पुल, slot की सूची, और हर क्षमता (`metadata.provider`, `media.read/write`, `notification.sender`, `webhook.receiver`, `public`, `prefs`)। |
| **[plugins/security-model.md](plugins/security-model.md)** | ख़तरे का मॉडल और बचाव की परतें: UID अलग करना, कर्नेल और LD_PRELOAD वाली seccomp denylist, सैंडबॉक्स के तरीके (`auto` क्रम से landlock → bubblewrap → basic तय करता है; `strict` bubblewrap के साथ LD_PRELOAD लाइब्रेरी भी जोड़ता है), userspace का संसाधन मॉनिटर, static विश्लेषण, CSS का sanitizer, हस्ताक्षर और संस्करण, slot की allowlist, और जानी-मानी सीमाएँ। |

एक्सटेंशन के टोकन के पीछे का अधिकार-मॉडल (डिफ़ॉल्ट रूप से मना, पक्के तौर पर बँधे
scope, और वह पुल जिसमें टोकन कभी iframe तक नहीं जाता) देखने के लिए
[adr/0012-plugin-token-deny-by-default.md](adr/0012-plugin-token-deny-by-default.md)
पढ़ें; संसाधनों की सीमाओं के लिए
[adr/0013-userspace-plugin-resource-monitor.md](adr/0013-userspace-plugin-resource-monitor.md)।

## वास्तुकला

```
+------------------+      gRPC (Unix socket)      +-------------------+
|   Evey Server    | <--------------------------> |  Plugin Process   |
|                  |   /tmp/evey/plugins/{id}/    |                   |
|  HostService     |   plugin.sock                |  PluginService    |
|  (serves RPCs)   |                              |  (implements)     |
+------------------+                              +-------------------+
        |                                                  |
        v                                                  v
+------------------+                              +-------------------+
|  Permission /    |                              |  SDK stubs        |
|  scope checker   |                              |  (multi-language) |
+------------------+                              +-------------------+
        |
        v
+------------------+     iframe sandbox           +-------------------+
|  Frontend        | <--------------------------> |  Frontend module  |
|  PluginSlot      |   postMessage bridge         |  (in srcdoc)      |
+------------------+                              +-------------------+
```

एक्सटेंशन **किसी भी ऐसी भाषा में** लिखा जा सकता है **जिसे `entrypoint:` चला सके** -
runner इस पर कोई पाबंदी नहीं लगाता। gRPC के तैयार SDK stubs, Python, Node.js, PHP,
Ruby, Dart, Java और Swift के लिए साथ आते हैं (`sdk/<lang>/`, इंस्टॉल के समय अपने
आप नकल हो जाते हैं); Go के लिए साथ में stubs नहीं आते, उन्हें `protoc-gen-go` और
`protoc-gen-go-grpc` से खुद बनाएँ; जिन इंटरप्रेटर भाषाओं के stubs साथ नहीं आते
(Bash, Lua, Perl, …) उनमें इंटरप्रेटर को सीधे चलाकर काम हो जाता है, और कंपाइल की
हुई बाइनरी (Go, Rust, …) `native-binary` अनुमति के साथ चलती हैं। हर भाषा की
तैयारी: [plugins/dev-setup.md](plugins/dev-setup.md#per-language)।

तीन बुनियादी खंभे:

- **क्षमताएँ और scope** - एक्सटेंशन अपने manifest में बता देता है कि उसे कौन सी
  क्षमताएँ चाहिए; व्यवस्थापक उन्हें इंस्टॉल के समय देता है। होस्ट हर कॉल पर
  डिफ़ॉल्ट रूप से मना करते हुए इन्हें लागू करता है। पूरी सूची
  [plugins/api-reference.md](plugins/api-reference.md#capabilities) में है।
- **सैंडबॉक्स** - बैकएंड की प्रक्रियाएँ सैंडबॉक्स में चलती हैं (फ़ाइल-सिस्टम,
  syscall और नेटवर्क की घेराबंदी के साथ) और उन पर userspace का संसाधन मॉनिटर रहता
  है; frontend का कोड कसकर बाँधे गए iframe में चलता है। ब्योरा
  [plugins/security-model.md](plugins/security-model.md) में।
- **भरोसा** - एक्सटेंशन SHA256 से बँधे होते हैं और उन पर Cosign के हस्ताक्षर हो
  सकते हैं; सर्वर को हस्ताक्षर अनिवार्य करने पर लगाया जा सकता है।
  [plugins/security-model.md](plugins/security-model.md#update-workflow) देखें।

## उपतंत्र चालू करना

एक्सटेंशन डिफ़ॉल्ट रूप से चालू रहते हैं (`EVEY_PLUGINS_ENABLED=true`)। सैंडबॉक्स
का तरीका `EVEY_PLUGINS_SANDBOX_MODE` से तय होता है (डिफ़ॉल्ट `auto`, जो आधिकारिक
image पर Landlock पर जाकर टिकता है)। संसाधनों की सीमाएँ (`EVEY_PLUGINS_MEMORY_MB`,
`_PIDS_MAX`, `_DISK_MAX_MB`, `_CPU_NICE`) व्यवस्थापक **Admin → Plugins** से बदल
सकता है। [configuration.md](configuration.md) और
[plugins/dev-setup.md](plugins/dev-setup.md#7-resource-limits-and-sandbox) देखें।
