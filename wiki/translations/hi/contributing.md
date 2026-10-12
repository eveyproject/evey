---
source_sha: a0eb85f44612fd3d234753208bf63c8a9fec91fc8d1020bfdb8109e199c59b19
reviewed: true
---
# योगदान

जब आप Evey के कोड पर काम करना चाहें, तो कहाँ से शुरू करें, और pull request को कौन
सी जाँचें पास करनी चाहिए। योगदान करने वालों के लिए लिखे दस्तावेज़ इस साइट पर नहीं,
कोड के साथ GitHub पर रखे गए हैं; यह पेज उनकी ओर इशारा करता है।

## कहाँ से शुरू करें

| आप चाहते हैं | पढ़ें |
| --- | --- |
| Evey को स्रोत से build करके चलाना | [DEV.md](../DEV.md) |
| योगदान के नियम जानना: branch, commit, टेस्ट, लाइसेंस समझौता | [CONTRIBUTING.md](../CONTRIBUTING.md) |
| आधे घंटे में कोड की संरचना समझना | [कोड का नक्शा](CODEBASE_MAP.md) |
| परतों और bounded context को समझना | [आर्किटेक्चर](ARCHITECTURE.md) |
| उन तौर-तरीकों का पालन करना जो कोड पहले से इस्तेमाल करता है | [कोड के तौर-तरीके](code-conventions.md) |
| इंटरफ़ेस पर काम करना | [फ़्रंटएंड डिज़ाइन सिस्टम](frontend-design-system.md) |
| जानना कि कोई ढाँचे वाला फ़ैसला क्यों लिया गया | [आर्किटेक्चर के फ़ैसले](adr/README.md) |
| रिलीज़ निकालना | [रिलीज़ रनबुक](runbooks/release.md) |

## फ़ैसले का रिकॉर्ड क्यों, और कहाँ

हर ढाँचे वाले फ़ैसले का - hexagonal बनावट, प्लगइन token का दायरा कैसे तय होता है,
इंस्टॉल एक ही compose फ़ाइल क्यों है, एक प्लेबैक योजना हर क्लाइंट के लिए कैसे तय
करती है - एक architecture decision record होता है: संदर्भ, तौले गए विकल्प, क्या
चुना गया और उसकी कमियाँ क्या हैं। ये repository में, उसी कोड के बगल में रखे जाते हैं
जिसे ये समझाते हैं, और इनमें से किसी फ़ैसले को बदलने का प्रस्ताव देने से पहले
शुरुआत यहीं से करनी चाहिए।

## pull request खोलने से पहले

1. `make pre-push` चलाएँ: यही gate है, और यह CI की जाँचें लोकल रूप से इस क्रम में
   चलाता है: vet, कमज़ोरियों की जाँच, build, `-race` के साथ unit टेस्ट, फ़्रंटएंड
   (typecheck, lint, unit टेस्ट), सुरक्षा जाँच, lint,
   [आर्किटेक्चर की जाँचें](ARCHITECTURE.md#architecture-guards), commit के नियमों की जाँच,
   secret की जाँच, proto में breaking change की जाँच, साफ़ checkout से build,
   repository और ट्रांसकोडिंग के integration gate, और TV audit।
2. नए बर्ताव के लिए और हर bug fix के लिए टेस्ट जोड़ें।
3. बाहर से योगदान के लिए, एक बार
   [योगदानकर्ता लाइसेंस समझौता](../CLA.md) स्वीकार करें: Evey AGPL-3.0-or-later है
   और इसे एक व्यावसायिक लाइसेंस के तहत भी दिया जा सकता है। आपके काम का copyright
   आपके पास ही रहता है।

!!! warning "gate न Docker इमेज बनाता है, न Android ऐप"
    `Dockerfile` या Kotlin कोड छूने के बाद, push करने से पहले इन्हें खुद build करें
    (`docker build --target runtime`, Gradle का release task)।

सुरक्षा की समस्याएँ सार्वजनिक रूप से नहीं बताई जातीं: `security@eveyproject.org` पर
लिखें (देखें [SECURITY.md](../SECURITY.md))।

<!-- sources
gate: Makefile - pre-push, ci-fast-vet, ci-fast-vuln, ci-fast-build, ci-fast-unit, ci-fast-frontend, ci-fast-sec, ci-local-lint, ci-arch, ci-hygiene, ci-secrets, ci-proto-breaking, ci-clean-checkout, repo-itest-gate, transcode-itest-gate, tv-audit
documents: DEV.md ; CONTRIBUTING.md ; CLA.md ; SECURITY.md ; docs/CODEBASE_MAP.md ; docs/code-conventions.md ; docs/frontend-design-system.md ; docs/adr/README.md ; docs/runbooks/release.md
-->
