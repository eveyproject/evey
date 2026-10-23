---
source_sha: 7be5d2c69f31e5a809a830f40949f2c4242950d173dfc5bf681d26c5be44e87f
reviewed: true
---
# अनुमतियाँ और स्लॉट { #permissions-and-slots }

प्लगइन बनाने वालों के लिए: manifest क्या बता सकता है, इंस्टॉलर क्या जाँचता है, और
चलते समय प्लगइन पर कौन से नियम लागू होते हैं। नीचे की सूची वह है जिसमें से आप
चुनते हैं; सर्वर प्लगइन को कैसे सीमित रखता है, और यह सीमा क्या नहीं रोक सकती, यह
[प्लगइन सैंडबॉक्स और ख़तरा मॉडल](../SECURITY.md) में बताया गया है।

## इंस्टॉल के समय क्या जाँचा जाता है { #what-is-checked-at-install }

प्लगइन इंस्टॉल के समय मना हो जाता है, जब:

- `kind` नहीं है, या न `plugin` है न `theme`;
- `kind: plugin` में `language` या `entrypoint` नहीं है, या `kind: theme` इन्हें
  बताता है, या `permissions` या `capabilities` बताता है;
- कोई अनुमति या क्षमता अनजानी है;
- कोई फ़्रंटएंड मॉड्यूल [स्लॉट की सूची](#slot-whitelist) से बाहर का स्लॉट चुनता है;
- `external_fetch` की कोई एंट्री [fetch के नियम](#external-fetch-rules) तोड़ती है,
  या `network` बिना किसी एंट्री के बताया गया है;
- ZIP [extraction की सीमाएँ](../SECURITY.md#zip-extraction) तोड़ता है;
- स्थैतिक विश्लेषण में कोई गंभीर पैटर्न मिलता है, या कोई ऐसी चेतावनी जिसे
  इंस्टॉलर गंभीर ही मानता है (असुरक्षित deserialization, obfuscation)। compiled
  binary, अंदर रखे आर्काइव और native code मना होते हैं, जब तक manifest
  `native-binary` न बताए।

विश्लेषण से पहले comment हटा दिए जाते हैं, इसलिए comment में बंद की गई लाइन दर्ज
नहीं होती। श्रेणियों की सूची
[प्लगइन सैंडबॉक्स और ख़तरा मॉडल](../SECURITY.md#static-analysis) में है।

इंस्टॉल करने से पहले एडमिन बताई गई अनुमतियाँ और क्षमताएँ देखता है, और यह भी
कि विश्लेषण में क्या मिला; स्लॉट, होस्ट और सिग्नेचर अपडेट के समय और सुरक्षा डैशबोर्ड पर दिखते हैं। प्लगइन जो भी बताता है, उसमें से कुछ भी चुपचाप नहीं
दिया जाता।

## प्लगइन किन सीमाओं में चलता है { #what-the-plugin-runs-under }

manifest कुछ भी बताए:

- प्लगइन अपना फ़ोल्डर देखता है, सर्वर का डेटा नहीं;
- उसके पास सीधा नेटवर्क नहीं होता: `network` अनुमति वाला प्लगइन अपनी
  `external_fetch` सूची के होस्ट तक होस्ट के `Fetch` कॉल से पहुँचता है, और कहीं
  नहीं;
- `subprocess` के बिना वह child process शुरू नहीं कर सकता (thread काम करते हैं);
- वह संसाधनों की उन सीमाओं में चलता है जिन्हें एडमिन बदल सकता है: डिफ़ॉल्ट रूप
  से 256 MiB मेमोरी, 64 प्रोसेस और thread, उसके फ़ोल्डर के लिए 100 MiB डिस्क, और
  कम CPU प्राथमिकता।

डेवलपमेंट के दौरान सिग्नेचर वैकल्पिक है। प्लगइन पर सिग्नेचर कैसे करें, और सर्वर
कौन सी सिग्नेचर नीति लागू कर सकता है: [प्लगइन लिखना](dev-setup.md#7-sign-your-plugin)।

## अनुमतियों की सूची { #permissions-catalog }

अनुमति `permissions` में बताई जाती है; कोई और मान इंस्टॉल मना करवा देता है।
वेबहुक पाना (`webhook.receiver`) और मेटाडेटा देना (`metadata.provider`) क्षमताएँ
हैं, जो `capabilities` में बताई जाती हैं: देखें
[प्लगइन API संदर्भ](api-reference.md#capabilities)।

| अनुमति | यह क्या देती है |
|---|---|
| `network` | बैकएंड `Fetch` कॉल कर सकता है। इसके साथ खाली न हो ऐसी `external_fetch` सूची होनी चाहिए; उसके बिना इंस्टॉल के समय मना। प्रोसेस के पास खुद फिर भी कोई socket नहीं होता। |
| `subprocess` | बैकएंड child process शुरू कर सकता है। इसके बिना launcher का kernel फ़िल्टर `fork`, `vfork` और `CLONE_THREAD` के बिना `clone` को मना करता है, और interpreter वाले प्लगइन के लिए seccomp shim `execve` को मना करता है। thread दोनों हालत में काम करते हैं। एडमिन को एक अलग चेतावनी दिखती है। |
| `native-binary` | पैकेज में `.so`, `.dll`, `.exe`, `.wasm`, `.jar`, अंदर रखे आर्काइव या ऐसा native code हो सकता है जिसे विश्लेषण पढ़ नहीं सकता। एडमिन को ऐसा कोड दिखता है जिसे पढ़ा नहीं जा सकता, और उसे यह स्वीकार करना होता है। `go` प्लगइन के लिए ज़रूरी। |
| `filesystem` | प्लगइन के फ़ोल्डर के बाहर फ़ाइल सिस्टम छूने का बताया गया इरादा। दर्ज होता है और एडमिन को दिखाया जाता है; सैंडबॉक्स प्लगइन के फ़ोल्डर के बाहर कोई फ़ाइल पहुँच नहीं देता, चाहे कुछ भी बताया गया हो। |
| `media.read` | होस्ट पर `GetMediaItem` और `ListMediaItems`: मीडिया का पूरा रिकॉर्ड। |
| `media.write` | मेटाडेटा लिखने वाले कॉल के लिए आरक्षित, जो आज दिए नहीं जाते। |
| `api.library` | `media.read` वाले ही मीडिया पढ़ने के कॉल। |
| `api.user` | `GetUserPreferences` के लिए आरक्षित, जो आज दिया नहीं जाता। |
| `api.metadata` | `UpdateMetadata` के लिए आरक्षित, जो आज दिया नहीं जाता। |
| `notification.sender` | `SendNotification` के लिए आरक्षित, जो आज दिया नहीं जाता। |
| `public` | इंटरफ़ेस मीडिया आइटम की एक सुरक्षित झलक पढ़ सकता है (id, टाइटल, टाइप, साल, सारांश, पोस्टर), कभी रास्ते या stream नहीं। देखें [`evey.media.*`](api-reference.md#eveymedia-permission-public)। |
| `prefs` | इंटरफ़ेस मौजूदा उपयोगकर्ता की पसंद प्लगइन के अपने हिस्से में पढ़ और लिख सकता है। देखें [`evey.prefs.*`](api-reference.md#eveyprefs-permission-prefs)। |
| `hardware.gpu` | प्रोसेस को होस्ट का DRM render node मिलता है (असली `renderD1xx`, कभी card node नहीं)। यह kernel driver पर हमले का एक रास्ता है, और transcoder के साथ बँटी GPU मेमोरी की एक सीमा: अटका हुआ GPU ट्रांसकोडिंग रोक सकता है। इसके बिना GPU node Landlock की अनुमति सूची से पूरी तरह बाहर रहते हैं; इसके साथ launcher उसी एक फ़ाइल तक सीमित एक `IOCTL_DEV` नियम जोड़ता है (Landlock ABI 5 या नया; पुराने kernel पर सिर्फ़ पढ़ने और लिखने की पहुँच से ही command भेजे जा सकते हैं)। प्लगइन इंस्टॉल होने के बाद सुरक्षा डैशबोर्ड एक गंभीर चेतावनी दिखाता है। यह सिर्फ़ उन प्लगइन को दें जिन्हें आप सैंडबॉक्स के बाहर भी चला लेते। |

## External fetch के नियम { #external-fetch-rules }

`external_fetch: [...]` में बताए जाते हैं और इंस्टॉल के समय जाँचे जाते हैं
(`validateExternalFetch`)। सब या कुछ नहीं: एक भी गलत एंट्री पूरा इंस्टॉल मना करवा
देती है।

एंट्री एक **होस्ट का नाम** है। आगे लगा scheme और अंत का path पार्स के समय हटा दिए
जाते हैं; सिर्फ़ होस्ट का नाम रखा जाता है। होस्ट ठीक-ठीक मिलने चाहिए, `*.example.com`
जैसा wildcard इंस्टॉल पर मना हो जाता है और port का suffix अनदेखा किया जाता है।

| एंट्री | मान्य | कारण |
|---|---|---|
| `api.tmdb.org` | हाँ | होस्ट का पूरा नाम, सामान्य रूप |
| `https://api.tmdb.org` | हाँ | scheme और path पार्स के समय हटा दिए जाते हैं, होस्ट का नाम रहता है |
| `*.tmdb.org` | नहीं | wildcard इंस्टॉल पर मना हो जाते हैं; हर होस्ट अलग से लिखें |
| `*` | नहीं | अकेला wildcard मना है |
| `192.168.1.1`, `::1` | नहीं | IP मना हैं, दोनों तरह के |

चलते समय होस्ट का broker अपनी सीमाएँ जोड़ता है, जिन्हें manifest बढ़ा नहीं सकता:
सिर्फ़ `http` और `https`, सिर्फ़ `GET` और `HEAD`, लक्ष्य होस्ट आपकी सूची से
बड़े-छोटे अक्षर का फ़र्क किए बिना मिलाया जाता है (एंट्री में `:port` अंत में हो तो
अनदेखा होता है), DNS से पता निकालने के बाद निजी या आरक्षित पते मना, body 8 MiB
तक और request 20 सेकंड तक सीमित। redirect को सूची और पते वाले नियम, दोनों से
फिर से जाँचा जाता है।

वही होस्ट सूची, आगे `https://` लगाकर, प्लगइन के फ़्रेम का `connect-src` बनती है:
विजेट उन होस्ट को सीधे कॉल कर सकता है, सिर्फ़ `https` पर।

## स्लॉट की अनुमति सूची { #slot-whitelist }

प्लगइन इंटरफ़ेस सिर्फ़ उन स्लॉट में डाल सकते हैं जिनकी साफ़ अनुमति है। संवेदनशील
रास्ते जानबूझकर इसमें नहीं हैं।

| स्लॉट | साइन-इन ज़रूरी | टिप्पणी |
|---|---|---|
| `home.section` | हाँ | मुख्य पेज का कंटेंट वाला हिस्सा |
| `home.hero` | हाँ | सबसे ऊपर वाला बड़ा बैनर |
| `library.sidebar` | हाँ | लाइब्रेरी पेज का साइडबार |
| `media.detail.sidebar` | हाँ | मीडिया के जानकारी वाले पेज का साइडबार |
| `player.overlay` | हाँ | प्लेयर के ऊपर की परत |
| `settings.user.appearance` | हाँ | उपयोगकर्ता सेटिंग्स में रूप-रंग वाला हिस्सा |
| `sidebar.bottom` | हाँ | नेविगेशन साइडबार का निचला हिस्सा |
| `admin.nav` | हाँ | एडमिन पैनल में एक मेन्यू एंट्री, जो प्लगइन का अपना सैंडबॉक्स वाला पेज खोलती है; यह मौजूदा एडमिन स्क्रीन में कुछ नहीं डालती |

**कभी अनुमति नहीं:** `admin.*` (`admin.nav` को छोड़कर), `login.*`, `setup.*`,
`profile.security.*`। ये रास्ते साइन-इन या अधिकारों के लिए अहम हैं, और वहाँ कुछ
डालना अधिकार बढ़ाने या phishing का रास्ता है।

`admin.nav` का कंटेंट भी बाकी स्लॉट की तरह सैंडबॉक्स में है: यह
`allow-same-origin` के बिना एक `<iframe sandbox="allow-scripts">` में बनता है, ऐसे
पेज में जिसकी नीति `default-src 'none'` है, इसलिए यह एडमिन वाले हिस्से में दिखाया
गया किसी तीसरे पक्ष का कंटेंट है, प्लगइन को दिया गया कोई एडमिन अधिकार नहीं।

bridge सीमित करता है कि फ़्रेम पेज से क्या माँग सकता है: `evey.toast` एक सेकंड में
ज़्यादा से ज़्यादा 3 संदेश, 200 अक्षर, कोई HTML नहीं; `evey.navigate` सिर्फ़
`/home`, `/library`, `/media`, `/player`, `/settings` और `/search`; `evey.resize`
50-800 px के बीच। फ़्रेम के पास कोई token नहीं होता: `public` और `prefs` कॉल के
लिए होस्ट (प्लगइन, उपयोगकर्ता) तक सीमित पाँच मिनट का एक token बनाता है, और सर्वर
डिफ़ॉल्ट रूप से हर वह method मना करता है जिसका नाम उस token में नहीं है।

## अपडेट का तरीका { #update-workflow }

1. एडमिन एक नया ZIP अपलोड करता है। `PreviewPluginUpdate` स्थैतिक विश्लेषण और
   सिग्नेचर की जाँच चलाता है, और इंस्टॉल किए गए वर्ज़न से अनुमतियों, क्षमताओं,
   स्लॉट और `external_fetch` का फ़र्क लौटाता है।
2. अगर फ़र्क खाली नहीं है, तो अपडेट लागू होने से पहले एडमिन को उसे स्वीकार करना
   होता है (`requires_ack = true`; अपलोड route से, `accept_permission_changes=true`)।
3. पैकेज का hash पूर्वावलोकन के समय लिया जाता है और लागू करते समय जाँचा जाता है:
   जिस ZIP का विश्लेषण हुआ, वही इंस्टॉल होता है।
4. साइनिंग कुंजी की निरंतरता जाँची जाती है: इंस्टॉल किए गए वर्ज़न से अलग कुंजी
   से साइन किया हुआ, या अब साइन न किया हुआ अपडेट तब तक मना रहता है जब तक एडमिन
   नया प्रकाशक स्वीकार नहीं करता। `EVEY_PLUGINS_COSIGN_POLICY=enforce` में, लागू
   करने वाले कदम पर भरोसे से बाहर या अमान्य सिग्नेचर अपडेट रोक देता है, भले ही
   पूर्वावलोकन पास हो गया हो।

<!-- sources
manifest validation: internal/adapters/driven/pluginrunner/manifest.go - validateManifest, validateExternalFetch ; internal/domain/plugin/model.go - Validate ; capabilities.go - KnownPermissions, KnownCapabilities
escalated warnings, native-binary: internal/app/plugin/install.go - checkEscalatedWarnings, validateStagedPlugin
comments stripped before analysis: docs/SECURITY.md (Static analysis) ; internal/adapters/driven/pluginrunner/analyzer_normalize.go
subprocess, deny-fork, deny-inet, render-dev: cmd/evey-plugin-launcher/main.go ; i18n admin.plugins.subprocessWarning
resource defaults: internal/config/config.go - Plugins
permission per RPC: internal/adapters/driving/grpcapi/plugin_interceptor.go ; served or not: internal/adapters/driving/pluginhost/host_server.go
broker: internal/adapters/driving/pluginhost/fetch.go - Fetch, hostAllowed, maxFetchBytes, fetchTimeout
frame connect-src: frontend/packages/client/src/plugins/hostProtocol.ts - buildConnectSrc ; internal/adapters/driving/http/plugin_frame_handler.go - pluginFramePolicy
slots: internal/domain/plugin/slots.go - AllowedSlots
bridge limits: frontend/packages/client/src/plugins/hostProtocol.ts ; token: internal/app/plugin/token_service.go - PluginTokenTTL ; docs/adr/0012-plugin-token-deny-by-default.md
update workflow: internal/app/plugin/update.go - ApplyUpdate ; internal/app/plugin/install.go - resolveExistingPlugin ; proto/evey/v1/admin_plugin.proto - PreviewPluginUpdate, ApplyPluginUpdate
-->
