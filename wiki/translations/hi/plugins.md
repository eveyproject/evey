---
source_sha: 3e8706d875387de097db25bae84c18a9439cf144e791b450e09bf54c3edf978d
reviewed: true
---
# प्लगइन { #plugins }

यह पेज एडमिन के लिए है: प्लगइन या थीम कैसे इंस्टॉल करें, ऐसा करते समय आप क्या
मंज़ूर करते हैं, और सर्वर उसे आपके होस्ट पर क्या छूने देता है। सब कुछ
**एडमिन → प्लगइन** में होता है; प्लगइन बनाने वालों के पेज
[आखिर में](#writing-a-plugin) दिए गए हैं।

प्लगइन Evey में वह जोड़ता है जो वह खुद नहीं करता: होम पेज पर एक सेक्शन, प्लेयर
के ऊपर कुछ दिखाना, मेटाडेटा का एक स्रोत, एक थीम। हर प्लगइन एक अलग प्रोसेस है जो
एक लोकल socket के ज़रिए सर्वर से संवाद करता है, एक सैंडबॉक्स में सीमित रहता है और सिर्फ़
उन अनुमतियों तक सीमित रहता है जो उसका manifest बताता है। उसका इंटरफ़ेस, अगर है,
पेज के एक अलग फ़्रेम में चलता है।

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![बिना प्लगइन के होम पेज](images/home-no-plugin.webp){ loading=lazy }
  <figcaption>बिना प्लगइन।</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![मौसम वाले प्लगइन के साथ होम पेज](images/home.webp){ loading=lazy }
  <figcaption>उदाहरण वाले मौसम प्लगइन के साथ: सबसे ऊपर एक सेक्शन, उसके अपने फ़्रेम में बना हुआ।</figcaption>
</figure>

</div>

## प्लगइन इंस्टॉल करें { #install-a-plugin }

1. **एडमिन → प्लगइन** में **प्लगइन इंस्टॉल करें** खोलें और **प्लगइन फ़ाइल
   (.zip)** चुनें। अगर बनाने वाला एक सिग्नेचर देता है, तो उसे **हस्ताक्षर फ़ाइल
   (.zip.sig, वैकल्पिक)** में जोड़ें। **URL से** वाला टैब इसकी जगह किसी लिंक से
   इंस्टॉल करता है; डाउनलोड की जाँच करवाने के लिए URL के आखिर में
   `#sha256:<hash>` जोड़ें।
2. **पूर्वावलोकन और इंस्टॉल** पैकेज को इंस्टॉल किए बिना जाँचता है।
   पूर्वावलोकन दिखाता है कि प्लगइन कौन सी **अनुमतियां** और **क्षमताएँ** माँगता
   है, **स्थैतिक विश्लेषण** का नतीजा और, अगर सिग्नेचर दिया गया हो, प्रकाशक के साथ
   **सिग्नेचर** का फ़ैसला। स्लॉट और होस्ट अपडेट के समय और **सुरक्षा डैशबोर्ड** पर
   दिखते हैं।
3. इस सूची को एक अनुबंध की तरह पढ़ें: प्लगइन सिर्फ़ यही चीज़ें कर पाएगा। पुष्टि
   करें, और प्लगइन शुरू हो जाता है।

<figure class="evey-shot" markdown="span">
  ![डेमो सर्वर पर एडमिन → प्लगइन](images/admin-plugins.webp){ loading=lazy }
  <figcaption><strong>एडमिन → प्लगइन</strong>: उपलब्ध runtime, संसाधनों की सीमाएँ, और हर प्लगइन का एक कार्ड (स्क्रीन अंग्रेज़ी में)।</figcaption>
</figure>

जिस पैकेज की जाँच में कोई गंभीर पैटर्न मिलता है, वह मना कर दिया जाता है, आप
चाहे जो जवाब दें। चेतावनियाँ दिखाई जाती हैं और फ़ैसला आप पर छोड़ा जाता है।
प्लगइन की भाषा सर्वर के **उपलब्ध रनटाइम** में होनी चाहिए: डिफ़ॉल्ट इमेज में
Python, Node.js, PHP, Ruby और Lua हैं, `-nodejs` इमेज में Python और Node.js रहते हैं
और `-lean` इमेज में सिर्फ़ Python रहता है।

!!! tip "सिग्नेचर वाले प्लगइन"
    प्लगइन बनाने वाला ZIP पर [cosign](https://docs.sigstore.dev/cosign/installation/)
    से सिग्नेचर कर सकता है; उसकी public key पैकेज के अंदर चलती है। Evey सिग्नेचर
    खुद जाँचता है (`cosign` binary नहीं चाहिए) और जब कुंजी आपकी **भरोसेमंद
    प्रकाशक** सूची में हो तो **<प्रकाशक> द्वारा हस्ताक्षरित** दिखाता है, नहीं तो
    fingerprint के साथ **हस्ताक्षरित, अज्ञात कुंजी** (एक क्लिक से उस पर भरोसा
    होता है), **हस्ताक्षर रहित** या **अमान्य**। जो अपडेट कुंजी बदलता है, या
    सिग्नेचर छोड़ देता है, वह आपकी साफ़ मंज़ूरी का इंतज़ार करता है। बाकी नीति
    तय करती है:

    ```sh
    EVEY_PLUGINS_COSIGN_POLICY=warn      # off | warn (डिफ़ॉल्ट) | enforce
    ```

    `warn` के साथ, फ़ैसला दिखता है और कुछ रोका नहीं जाता; `enforce` के साथ, सिर्फ़
    भरोसेमंद प्रकाशक इंस्टॉल होते हैं। प्लगइन बनाने वालों के लिए: देखें
    [अपने प्लगइन पर सिग्नेचर करें](plugins/dev-setup.md#7-sign-your-plugin)।

## प्लगइन अपडेट करें { #update-a-plugin }

प्लगइन के कार्ड पर, **स्रोत से अपडेट करें…** नई ZIP लेता है (एक रास्ता या एक
URL) और वही पूर्वावलोकन दिखाता है। अपडेट का वर्ज़न इंस्टॉल किए गए वर्ज़न से
ऊँचा होना चाहिए। जब वह नई अनुमतियाँ, क्षमताएँ, स्लॉट या होस्ट माँगता है, तो
पूर्वावलोकन में **यह अपडेट नई अनुमतियाँ माँग रहा है** लिखा आता है और फ़र्क की
सूची दिखती है: जब तक आप **स्वीकृति दें और इंस्टॉल करें** न चुनें, कुछ लागू नहीं
होता। जो पैकेज जाँचा गया, वही इंस्टॉल होता है: अपडेट लागू करते समय उसका hash
फिर से जाँचा जाता है।

## इंस्टॉल किए गए प्लगइन संभालें { #manage-the-installed-plugins }

हर कार्ड प्लगइन की स्थिति (**सक्षम** या **अक्षम**) दिखाता है और ये विकल्प देता
है:

| कार्रवाई | यह क्या करती है |
| --- | --- |
| **सक्षम** / **अक्षम** | प्लगइन का प्रोसेस शुरू या बंद करता है। अक्षम प्लगइन की फ़ाइलें और कॉन्फ़िगरेशन बने रहते हैं। |
| **कॉन्फ़िग** | वे सेटिंग्स जो प्लगइन बताता है (एक API कुंजी, एक शहर, एक इकाई...)। secret वाले फ़ील्ड छिपे हुए दिखते हैं और ब्राउज़र को कभी वापस नहीं भेजे जाते। |
| **लॉग** | वे आखिरी लाइनें जो प्लगइन ने होस्ट के ज़रिए लिखीं। |
| **अनइंस्टॉल** | प्रोसेस, फ़ाइलें और प्लगइन का सहेजा हुआ डेटा हटा देता है। |

पेज के ऊपर **संसाधन सीमाएँ** हर प्लगइन प्रोसेस की सीमा तय करता है। पैनल के मान
डिफ़ॉल्ट हैं; जब पैनल कभी सहेजा न गया हो, तो environment वेरिएबल इन्हें सेट
करता है।

| फ़ील्ड | डिफ़ॉल्ट | Environment वेरिएबल |
| --- | --- | --- |
| **अधिकतम मेमोरी (MiB)** | 256 | `EVEY_PLUGINS_MEMORY_MB` |
| **अधिकतम प्रक्रियाएँ** | 64 | `EVEY_PLUGINS_PIDS_MAX` |
| **अधिकतम डिस्क (MiB)** | 100 | `EVEY_PLUGINS_DISK_MAX_MB` |
| **CPU nice (0-19)** | 10 | `EVEY_PLUGINS_CPU_NICE` |
| **सैंडबॉक्स मोड** | `auto` | `EVEY_PLUGINS_SANDBOX_MODE` |

मेमोरी, प्रोसेस और डिस्क की सीमाएँ निगरानी के अगले दौर में लागू होती हैं, जो इन्हें
पार करने वाले प्लगइन को रोक देता है; CPU प्राथमिकता और सैंडबॉक्स मोड उसके बाद
शुरू होने वाले प्लगइन पर लागू होते हैं। `0` मेमोरी और प्रोसेस के लिए डिफ़ॉल्ट
वापस ला देता है; सिर्फ़ डिस्क की सीमा `0` को "बंद" मानती है।

**सुरक्षा डैशबोर्ड** हर प्लगइन के लिए जाँच में मिली बातें, सिग्नेचर का नतीजा
और आखिरी त्रुटि रखता है। उसकी **होस्ट सैंडबॉक्स** वाली लाइन वह सैंडबॉक्स मोड
दिखाती है जो इस मशीन पर असल में लागू है: अगर उसमें `basic` लिखा है, तो होस्ट
हर रास्ते के हिसाब से फ़ाइल सिस्टम का अलगाव नहीं देता, और
[प्लगइन सैंडबॉक्स सेटअप](plugins/sandbox-deployment.md) बताता है कि क्या बदलना
है।

प्लगइन डिफ़ॉल्ट रूप से चालू हैं। `EVEY_PLUGINS_ENABLED=false` पूरा प्लगइन वाला
हिस्सा बंद कर देता है।

## प्लगइन आपके होस्ट पर क्या कर सकता है { #what-a-plugin-can-do-on-your-host }

उसका manifest कुछ भी कहे, प्लगइन:

- सिर्फ़ अपना फ़ोल्डर देखता है, सर्वर का डेटा या मीडिया फ़ाइलें नहीं;
- के पास सीधा नेटवर्क नहीं होता: `network` अनुमति वाला प्लगइन सिर्फ़ अपने बताए
  होस्ट तक, सर्वर के ज़रिए, `GET` और `HEAD` से पहुँचता है;
- child process शुरू नहीं कर सकता, जब तक वह `subprocess` न बताए;
- ऊपर की संसाधन सीमाओं के अंदर चलता है, और जब होस्ट अनुमति दे, एक अलग
  उपयोगकर्ता के रूप में।

उसका इंटरफ़ेस सिर्फ़ उन स्लॉट तक सीमित है जो manifest बताता है: होम पेज का एक
सेक्शन या सबसे ऊपर का बड़ा हिस्सा, किसी लाइब्रेरी या जानकारी वाले पेज पर साइड
पैनल, प्लेयर के ऊपर दिखने वाली परत, रूप-रंग की सेटिंग्स, नेविगेशन साइडबार का
निचला हिस्सा, या एडमिन मेन्यू में एक एंट्री जो प्लगइन का अपना पेज खोलती है।
प्लगइन साइन-इन, सेटअप, सुरक्षा या मौजूदा एडमिन स्क्रीन में कभी कुछ नहीं बना
सकता। अपने फ़्रेम में उसकी पहुँच पेज, आपकी कुकी या आपके सेशन तक नहीं होती: वह
सर्वर से जो पढ़ता है, वह एक कम समय वाले token से जाता है, जो उन दो अनुमतियों तक
सीमित है जो वह माँग सकता है (`public` मीडिया के टाइटल और पोस्टर की सुरक्षित
झलक के लिए, `prefs` मौजूदा उपयोगकर्ता की पसंद के लिए, उसके अपने हिस्से में)।

प्लगइन जो अनुमतियाँ माँग सकता है, उनकी सूची
[अनुमतियाँ और स्लॉट](plugins/security-model.md) में है। सैंडबॉक्स कैसे काम
करता है, और वह अब भी क्या नहीं रोक सकता, यह
[प्लगइन सैंडबॉक्स और ख़तरा मॉडल](SECURITY.md) में है।

!!! warning "सर्वर यह जाँचता है कि प्लगइन डेटा कहाँ भेजता है, यह नहीं कि क्या भेजता है"
    एक बार कोई होस्ट प्लगइन की सूची में आ जाए, तो सर्वर यह नहीं देख सकता कि
    प्लगइन उसे क्या भेजता है। किसी बाहरी प्लगइन को इंस्टॉल करने से पहले उसे
    परखें, जैसे आप किसी भी सॉफ़्टवेयर को परखते, और उसकी होस्ट सूची को उन पक्षों
    की सूची मानें जिन्हें आपकी लाइब्रेरी के टाइटल मिल सकते हैं।

## थीम { #themes }

थीम `kind: theme` वाला एक पैकेज है जिसमें कोई प्रोसेस नहीं होता: एक stylesheet,
और चाहें तो एक छिपे फ़्रेम तक सीमित script। यह उसी **प्लगइन इंस्टॉल करें** वाले
डायलॉग से इंस्टॉल होती है; इंस्टॉल के समय stylesheet को साफ़ किया जाता है। फिर
हर उपयोगकर्ता इसे **सेटिंग्स → रूप-रंग → थीम** में चुनता है। थीम में क्या हो
सकता है, यह [थीम](themes.md) में बताया गया है।

## प्लगइन लिखना { #writing-a-plugin }

| पेज | क्या है |
| --- | --- |
| [प्लगइन लिखना](plugins/dev-setup.md) | पहला प्लगइन कदम-दर-कदम, इंस्टॉल करके परखने का चक्र, सिग्नेचर, वितरण। यहीं से शुरू करें। |
| [प्लगइन API संदर्भ](plugins/api-reference.md) | manifest, बैकएंड कॉल और डेटा स्टोर, इंटरफ़ेस का `evey.*` bridge। |
| [अनुमतियाँ और स्लॉट](plugins/security-model.md) | manifest क्या बता सकता है, इंस्टॉल के समय क्या जाँचा जाता है, बाहरी डेटा लाने के नियम, स्लॉट। |
| [थीम](themes.md) | CSS और सैंडबॉक्स में चलने वाली script से इंटरफ़ेस का रूप बदलना। |
| [वेबहुक और API कुंजियाँ](webhooks-api-keys.md) | प्लगइन लिखे बिना Evey को दूसरे टूल से जोड़ना। |

<!-- sources
install dialog, from URL with #sha256: i18n admin.plugins.installPlugin, pluginFileZip, sigFileOptional, fromUrl, installFromUrlHint, previewInstall, staticAnalysis ; internal/app/plugin/install.go - downloadIfURL
critical findings refuse, warnings: internal/app/plugin/install.go - validateStagedPlugin, checkEscalatedWarnings ; i18n admin.plugins.blockedByAnalysis, installAnyway
runtimes: internal/adapters/driven/pluginrunner/runtime_availability.go - runtimeBinaries ; i18n admin.plugins.availableRuntimes
cosign policy: internal/config/config.go - CosignPolicy, CosignPublicKey ; internal/adapters/driven/pluginrunner/cosign.go
update, version higher, escalation, hash re-checked: internal/app/plugin/install.go - resolveExistingPlugin ; internal/app/plugin/update.go - ApplyUpdate ; internal/app/plugin/permissions.go - DiffPermissions ; i18n admin.plugin.update, admin.plugins.permissionEscalation.*
card actions: i18n admin.plugin.enabled, disabled, config, logs, uninstall ; secrets masked: internal/domain/plugin/model.go - IsSecretConfigField ; internal/adapters/driving/grpcapi/admin_plugin.go
resource limits defaults and env: internal/config/config.go - Plugins (MemoryMB, PIDsMax, DiskMaxMB, CPUNice, SandboxMode) ; i18n admin.plugins.resourceLimits.*
security dashboard, host sandbox: i18n admin.plugins.security.* ; internal/app/plugin/security_dashboard_service.go
plugins enabled flag: internal/config/config.go - PluginsEnabled
fetch broker GET/HEAD: internal/adapters/driving/pluginhost/fetch.go - Fetch
subprocess: docs/plugins/security-model.md (Permissions catalog)
slots: internal/domain/plugin/slots.go - AllowedSlots
scoped token, public/prefs: docs/adr/0012-plugin-token-deny-by-default.md ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes
theme selection: frontend/apps/web/src/features/user/ThemeSettings.tsx ; i18n common.appearance, common.theme
-->
