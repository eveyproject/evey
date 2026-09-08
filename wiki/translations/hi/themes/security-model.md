---
source_sha: 2ec48afdaf5dd44ee661048d21aa84c7ac2b22e7b6454b4270a203bd473922b4
---

# थीम का सुरक्षा मॉडल

## थीम और एक्सटेंशन: बुनियादी फ़र्क

**थीम** सिर्फ़ frontend की चीज़ है। उसकी कोई बैकएंड प्रक्रिया नहीं होती, कोई भाषा runtime नहीं, कोई IPC socket नहीं, और सर्वर की तरफ़ कोई अनुमति नहीं। उसमें इतना ही होता है:

- `theme.css` - एक स्टाइलशीट, जो हर पृष्ठ के `<head>` में डाल दी जाती है
- `theme.js` (वैकल्पिक) - JavaScript, जो एक अलग-थलग परिवेश में सीमित `evey.theme.*` API के साथ चलती है

**एक्सटेंशन** के साथ हमेशा एक बैकएंड प्रक्रिया रहती है (`kind: plugin`, `language`, `entrypoint` ज़रूरी हैं)। जो थीम `language`, `entrypoint` या `permissions` घोषित करने की कोशिश करती है, उसे इंस्टॉल के समय ही मना कर दिया जाता है।

थीम स्वीकार करने से पहले, इंस्टॉल के समय CSS और JS दोनों की static जाँच होती है।

## CSS sanitizer के नियम

इंस्टॉल के समय `theme.css` को `SanitizeCSS` से गुज़ारा जाता है। sanitizer ख़तरनाक रचनाओं को बदल देता है या हटा देता है, और चेतावनियों की एक सूची लौटाता है। वह पूरी फ़ाइल को **ठुकराता नहीं** - साफ़-सुथरी CSS ज्यों की त्यों निकल जाती है।

### जिन रचनाओं पर रोक है

**`@import`** - पूरी तरह हटा दिया जाता है।

कोई भी `@import` डायरेक्टिव निकाल दिया जाता है। वजह: थीम को अपने भीतर पूरी होनी चाहिए; बाहर से स्टाइलशीट मँगाना नेटवर्क के रास्ते जानकारी बाहर जाने का रास्ता है।

**बाहरी `url()`** - `none` से बदल दिया जाता है।

जिन `url()` वैल्यू में `http://`, `https://` या प्रोटोकॉल-निरपेक्ष `//` वाले URL होते हैं, वे बदल दी जाती हैं। `data:` URI (`data:image/*`, `data:font/*`) चलती हैं और ज्यों की त्यों निकल जाती हैं।

**`url()` में `javascript:`** - `none` से बदल दिया जाता है।

**`expression()`** (पुराना IE) - `none` से बदल दिया जाता है।

**`-moz-binding` और `behavior`** - हटा दिए जाते हैं।

ये XBL/HTC वाले पुराने ज़माने के कोड चलाने के रास्ते हैं, पूरी तरह हटा दिए जाते हैं।

**संवेदनशील selector वाले नियम** - पूरा नियम-खंड हटा दिया जाता है।

जिन नियम-खंडों का selector नीचे दिए में से किसी से मेल खाता है, वे पूरे के पूरे निकाल दिए जाते हैं:

- `[type="password"]` या `[type='password']` या `[type=password]`
- `[type="hidden"]`
- `.admin-<कुछ भी>`
- `.token-<कुछ भी>`
- `.secret-<कुछ भी>`

वजह: थीम को पासवर्ड के फ़ील्ड या विशेषाधिकार वाले UI तत्वों की सजावट इस तरह बदलने की छूट नहीं होनी चाहिए जिससे UI का भेस बदला जा सके।

**पूरी स्क्रीन ढकने वाले नियम** - पूरा नियम-खंड हटा दिया जाता है।

नियम-खंड तब हटाया जाता है जब उसमें `position: fixed` या `position: sticky` के साथ इनमें से कम से कम दो हों: `top: 0`, `left: 0`, `width: 100vw`, `height: 100vh`। यही CSS से बनने वाली phishing परत का जाना-पहचाना ढाँचा है।

**संवादमूलक तत्वों पर `pointer-events: none`** - वह घोषणा नियम-खंड से हटा दी जाती है।

जब किसी नियम का selector `button`, `[role="button"]`, `a[href*=logout]` या `[data-critical]` पर लगता है, तो उस नियम की `pointer-events: none` घोषणा हटा दी जाती है। बाकी नियम बना रहता है। वजह: लॉगआउट या ज़रूरी कार्रवाई वाले बटन को क्लिक होने लायक न रहने देना, UI का भेस बदलने वाला हमला है।

**999 से ऊपर का z-index** - 999 पर बाँध दिया जाता है।

999 से बड़ी कोई भी `z-index` वैल्यू 999 कर दी जाती है और एक चेतावनी दर्ज होती है। थीम खुद को ऐप्लिकेशन की अपनी modal या overlay परत के ऊपर नहीं रख सकतीं।

### CSS में थीम क्या-क्या कर सकती है

- पूरे मुखपृष्ठ का लेआउट, hero वाला हिस्सा, कार्ड ग्रिड, sidebar अपने मन का बनाना
- वीडियो प्लेयर के नियंत्रण, प्रगति पट्टी और परतों की सजावट बदलना
- अपने फ़ॉन्ट (`data:` के ज़रिए, या `src: url(data:...)` वाले inline `@font-face` से)
- एनिमेशन और transition (`@keyframes`, `transition`, `animation`)
- Custom properties (`--my-color: #...`), `:root` को बदलना
- Media queries, container queries
- कोई भी ऐसा selector जो पासवर्ड फ़ील्ड, `.admin-*`, `.token-*` या `.secret-*` पर न लगता हो

## थीम की JS जाँचने के नियम

इंस्टॉल के समय `theme.js` को `internal/pkg/themejs/scan.go` जाँचता है। एक भी मेल मिल जाए, तो इंस्टॉल करने से मना कर दिया जाता है और वजह बताने वाली त्रुटि मिलती है। यहाँ चेतावनियाँ होती ही नहीं - जाँचने वाला जान-बूझकर सतर्क रखा गया है (ग़लत पकड़ मंज़ूर है, क्योंकि थीम के पास इन API का ज़िक्र करने की कोई जायज़ वजह नहीं होती)।

### जिन शब्दों पर रोक है

नीचे दिए में से कोई भी टुकड़ा स्रोत में कहीं भी मिल जाए, तो इंस्टॉल करने से मना कर दिया जाता है:

- `document.cookie`
- `localStorage`
- `sessionStorage`
- `indexedDB`
- `XMLHttpRequest`
- `navigator.sendBeacon`
- `navigator.serviceWorker`
- `WebSocket`
- `EventSource`
- `importScripts`
- `document.write`
- `Element.prototype`
- `Object.defineProperty`
- `globalThis`
- `window.parent`
- `window.top`
- `window.opener`
- `Reflect.get`
- `Reflect.apply`
- `Reflect.construct`
- `window.frames`
- `frameElement`
- `postMessage`

### जिन regex पैटर्न पर रोक है

| पैटर्न | यह क्या पकड़ता है |
|---|---|
| `\b(fetch\|eval\|atob\|btoa\|Function\|setTimeout\|setInterval)\s*\(` | मना किए गए global को सीधे बुलाना |
| `\bimport\s*\(` | Dynamic import |
| `(?:window\|self\|globalThis\|top\|parent\|document\|navigator\|location)\s*\[` | होस्ट के global तक bracket वाली पहुँच (`window["fetch"]` पकड़ता है) |
| `\.cookie\b` | `.cookie` तक कोई भी पहुँच, चाहे property descriptor से हो या गिनी हुई |
| `\.constructor\s*\.\s*constructor` | `[].constructor.constructor` वाला निकास |
| `new\s+Function\b` | `new Function(...)` से कोड गढ़ना |
| `(?:window\|self\|top\|parent\|globalThis)\s*\.\s*(?:Function\|eval\|fetch\|atob\|btoa\|XMLHttpRequest\|WebSocket\|EventSource)\b` | मना किए गए global को property के रूप में पढ़ना |
| `(?:window\|top\|parent\|document\|location\|globalThis\|self)\s*\.\s*[A-Za-z_$][\w$]*\s*=[^=]` | होस्ट के global पर property लिखना |
| `Object\s*\.\s*prototype` | Prototype में गंदगी घोलना |
| `\.register\s*\(\s*['"]\S+\.js` | Service worker दर्ज करना |

### थीम की JS का runtime पर अलगाव

चलते समय `theme.js` एक ऐसे iframe के भीतर चलती है जिस पर `sandbox="allow-scripts"` लगा है और `srcdoc` की CSP कसी हुई है। मान लीजिए इंस्टॉल के समय की जाँच को किसी तरह चकमा दे भी दिया जाए, तब भी उस iframe के पास होस्ट दस्तावेज़ तक same-origin पहुँच नहीं है, होस्ट के `window`, `localStorage` या cookies तक पहुँच नहीं है, और बाहर जाने वाला नेटवर्क भी नहीं है (CSP `connect-src 'none'`)।

बातचीत का इकलौता रास्ता वही `evey.theme.*` API है, जिसे होस्ट का पुल डालता है। कौन से तरीके मौजूद हैं, यह [`api-reference.md`](api-reference.md) में देखें।

## theme.js में आप क्या कर सकते हैं

- होस्ट की स्टाइल सतह में CSS डालने के लिए `evey.theme.css(rules)` बुलाना
- ब्राउज़र के console में debug संदेश भेजने के लिए `evey.theme.log(level, msg)` बुलाना
- `evey.theme.registerComponent(slot, render)` से component के render फ़ंक्शन दर्ज करना (V2 - नीचे देखें)
- `evey.theme.observe(stateKey, cb)` से प्लेयर या ऐप की सिर्फ़ पढ़ी जा सकने वाली स्थिति पर नज़र रखना (V2)

## आप क्या नहीं कर सकते

- किसी भी URL से डेटा लाना (`fetch`, `XMLHttpRequest`, `WebSocket`, `navigator.sendBeacon`)
- उपयोगकर्ता के प्रमाण या टोकन पढ़ना (`document.cookie`, `localStorage`, `sessionStorage`)
- iframe के परिवेश से बाहर निकलना (`window.parent`, `window.top`, `frameElement`)
- पूरी viewport ढक लेने वाली परत दिखाना
- CSS से पासवर्ड फ़ील्ड या `.admin-*`/`.token-*`/`.secret-*` selector पर निशाना लगाना
- लॉगआउट या ज़रूरी कार्रवाई वाले बटन को क्लिक होने लायक न रहने देना (`pointer-events: none`)
- ऐप्लिकेशन की अपनी modal के ऊपर चढ़ जाना (`z-index > 999`)
