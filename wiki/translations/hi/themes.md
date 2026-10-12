---
source_sha: 53779971ebd8b5f41b11b97d89f9f1102e0d283c08d999b9cc028f3b92f630ca
reviewed: true
---
# थीम

थीम Evey के इंटरफ़ेस का रूप बदलती है: यह `kind: theme` वाला एक पैकेज है जिसमें
कोई बैकएंड प्रोसेस नहीं होता, बस एक stylesheet और, चाहें तो, एक script जो एक छिपे
फ़्रेम में बंद रहती है। यह पेज थीम बनाने वालों के लिए है; यह बताता है कि इन दो
फ़ाइलों में क्या हो सकता है और इंस्टॉलर क्या बदल देता है या मना करता है। थीम
प्लगइन की तरह **एडमिन → प्लगइन** से इंस्टॉल होती हैं, और हर उपयोगकर्ता
**सेटिंग्स → रूप-रंग → थीम** में अपनी थीम चुनता है।

## Manifest

```yaml title="plugin.yaml"
name: aurora               # ज़रूरी; पढ़ने लायक नाम
version: 1.0.0             # ज़रूरी; semver
kind: theme                # ज़रूरी

description: "..."         # वैकल्पिक
author: "..."              # वैकल्पिक

# न language, न entrypoint, न permissions, न capabilities: वरना इंस्टॉल मना होता है
```

ZIP की जड़ में, manifest के बगल में, `theme.css` और/या `theme.js` होती है। जिस
पैकेज में दोनों में से कोई नहीं, वह स्वीकार होता है पर कोई असर नहीं दिखता।
`examples/plugins/theme-dark-blue` (रंगों के वेरिएबल बदलने वाली एक छोटी
`theme.css`) और `examples/plugins/open-workshop` (पूरा नया रूप), दोनों सफ़ाई वाली
जाँच पास करते हैं और शुरुआत के लिए अच्छे हैं।

## theme.css

stylesheet हर पेज के head में एक `<link rel="stylesheet">` के रूप में डाली जाती है
और पूरे दस्तावेज़ पर लागू होती है: custom properties, `@keyframes`, `@media`,
`data:` स्रोत वाले `@font-face`, `@layer` और container queries, सब काम करते हैं।
यह `/api/v1/themes/<id>/theme.css` पर बिना साइन-इन के दी जाती है, ताकि जो थीम
ब्राउज़र याद रखता है, वह साइन-इन से पहले भी लागू हो।

### custom properties से शुरू करें

इंटरफ़ेस अपनी सतहें, टेक्स्ट, accent रंग, किनारे और गोलाई `:root` पर रखी custom
properties के एक समूह से बनाता है; `examples/plugins/theme-dark-blue/theme.css`
हर एक को एक comment के साथ बताती है। इन्हें बदलने से हर स्क्रीन एक साथ बदल जाती
है:

```css title="theme.css"
:root {
  --bg: #0b1120;
  --surface: #162032;
  --text: #e2e8f0;
  --accent: #3b82f6;
  --accent-hover: #60a5fa;
  --radius: 8px;
}
```

### थीम के लिए स्थिर हुक

कुछ सजावटी हिस्सों पर स्थिर class होती हैं, जिन्हें आप utility class पर निर्भर
हुए बिना निशाना बना सकते हैं; utility class रिलीज़ के बीच बदलती रहती हैं। ये एक
अनुबंध हैं: इनके नाम नहीं बदले जाएँगे।

| Selector | हिस्सा | जो बदलना सुरक्षित है |
|---|---|---|
| `.evey-app-header` | सबसे ऊपर की पट्टी | रंग, ऊँचाई, दूरी, blur; अंदर के controls को कभी न छिपाएँ, न हटाएँ |
| `.evey-media-grid` | मीडिया कार्ड के ग्रिड (लाइब्रेरी, कतारें) | कॉलम, `gap`, संरेखण |
| `.evey-media-card` | एक मीडिया कार्ड (पोस्टर और जानकारी) | चौड़ाई, दूरी, hover |
| `.evey-poster` | पोस्टर या artwork की तस्वीर (जानकारी वाले पेज पर भी) | `aspect-ratio`, `border-radius`, छाया, फ़िट |
| `.evey-media-detail` | मीडिया के जानकारी वाले पेज की जड़ | जानकारी वाले हिस्से की बनावट |

```css title="theme.css - bigger posters, denser grid, sharper corners"
.evey-media-card { width: 12rem; }
.evey-poster     { border-radius: 2px; }
.evey-media-grid { gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); }
.evey-media-card:hover { transform: scale(1.03); transition: transform 120ms ease; }
```

### सफ़ाई वाली जाँच क्या बदलती है

इंस्टॉल के समय `theme.css` एक सफ़ाई वाली जाँच से गुज़रती है, जो वह सब बदल देती या
हटा देती है जो थीम नहीं कर सकती, हर बदलाव के लिए एक चेतावनी दर्ज करती है, और
नतीजा वापस लिख देती है: जो दिया जाता है वह साफ़ की गई फ़ाइल है, जो आपकी लिखी फ़ाइल
से अलग हो सकती है। यह पैकेज को कभी मना नहीं करती।

| आपने क्या लिखा | क्या होता है | क्यों |
|---|---|---|
| `@import ...` | हटा दिया जाता है | थीम अपने में पूरी होती है; import नेटवर्क पर जानकारी का रिसाव है। |
| `url(http://...)`, `url(https://...)`, `url(//...)`, `image-set()` में कोई बाहरी विकल्प, ऐसा URL रखने वाली custom property | `none` से बदला जाता है या बेअसर किया जाता है | वही कारण। `data:` URI बिना बदले जाते हैं। |
| `url(javascript:...)`, `expression(...)`, `-moz-binding`, `behavior` | हटा दिया जाता है | कोड चलाने के पुराने रास्ते। |
| कोई rule जिसका selector `[type=password]`, `[type=hidden]`, `.admin-*`, `.token-*` या `.secret-*` से मेल खाता है | पूरा rule block हटा दिया जाता है | थीम को पासवर्ड फ़ील्ड या अधिकार वाले हिस्सों का रूप नहीं बदलना चाहिए। |
| `position: fixed` या `sticky` वाला rule जो स्क्रीन ढक दे: `inset: 0`, चारों तरफ़ `0`, या `top`/`right`/`bottom`/`left: 0`, `width: 100vw`, `height: 100vh` में से कम से कम दो, जिनमें एक viewport का आकार हो | पूरा rule block हटा दिया जाता है | phishing वाली परत का सबसे आम रूप। `top: 0; left: 0` से टिका और अपने कंटेंट जितना बड़ा header बना रहता है। एक ही selector की घोषणाएँ सभी rule में जोड़कर देखी जाती हैं। |
| `button`, `[role=button]`, `a[href*=logout]` या `[data-critical]` को निशाना बनाने वाले rule में `pointer-events: none` | वह घोषणा हटा दी जाती है, बाकी रहता है | किसी अहम control का क्लिक होना छिपाना इंटरफ़ेस से धोखा है। |
| 999 से ऊपर `z-index` | 999 पर रोका जाता है | थीम ऐप्लिकेशन की अपनी modal परत के नीचे रहती है। |

इन जाँचों से पहले escape किए गए अक्षर पढ़कर असली रूप में लाए जाते हैं, इसलिए
escape sequence से इन्हें चकमा नहीं दिया जा सकता।

## theme.js

script वैकल्पिक है। यह साइन-इन किए हुए उपयोगकर्ताओं को
`/api/v1/themes/<id>/theme.js` पर दी जाती है और एक छिपे
`<iframe sandbox="allow-scripts">` में चलती है, जिसकी नीति `connect-src 'none'`
रखती है: होस्ट पेज, उसके स्टोरेज या नेटवर्क तक कोई पहुँच नहीं, उस script के लिए भी
जो इंस्टॉल की जाँच पास कर चुकी है। अकेला रास्ता `window.evey.theme` है, जो script
चलने से पहले डाला जाता है।

### `evey.theme.css(declarations)`

होस्ट पेज को custom property की घोषणाएँ भेजता है, जो उन्हें `:root` पर लागू करता
है। सिर्फ़ यही रूप माना जाता है: `--name: value` की सूची, ज़्यादा से ज़्यादा 200
घोषणाएँ, हर मान ज़्यादा से ज़्यादा 500 अक्षर, बिना `url()`, `image-set`,
`expression()`, `@import`, `javascript:`, `data:`, `http(s):`, `//`, comment या
brace के। बाकी सब छोड़ दिया जाता है और console में बताया जाता है। कोई property
सिर्फ़ वहीं असर करती है जहाँ साफ़ की गई stylesheet उसका इस्तेमाल करती है।

```js title="theme.js"
evey.theme.css("--accent: #e8a33d; --accent-hover: #f0b95c");
```

### `evey.theme.log(level, message)`

ब्राउज़र के console में एक लाइन भेजता है। `level` `"debug"`, `"info"`, `"warn"` या
`"error"` है; संदेश 500 अक्षरों पर काटा जाता है। उपयोगकर्ता को कुछ नहीं दिखता।

`evey.theme.registerComponent` और `evey.theme.observe` object पर मौजूद हैं, पर
लागू नहीं हैं: होस्ट इनका जवाब एक त्रुटि से देता है।

### जाँच क्या मना करती है

इंस्टॉल के समय `theme.js` को टेक्स्ट की तरह जाँचा जाता है, और एक भी मेल मिलने पर
पैकेज कारण के साथ मना हो जाता है। जाँच जानबूझकर सख़्त है: थीम के पास इन नामों का
ज़िक्र करने का कोई जायज़ कारण नहीं है, comment में भी नहीं।

स्रोत में कहीं भी इनमें से कोई भी टुकड़ा:

`document.cookie`, `localStorage`, `sessionStorage`, `indexedDB`,
`XMLHttpRequest`, `navigator.sendBeacon`, `navigator.serviceWorker`,
`WebSocket`, `EventSource`, `importScripts`, `document.write`,
`Element.prototype`, `Object.defineProperty`, `globalThis`, `window.parent`,
`window.top`, `window.opener`, `Reflect.get`, `Reflect.apply`,
`Reflect.construct`, `window.frames`, `frameElement`, `postMessage`।

इनमें से कोई भी पैटर्न:

| पैटर्न | यह क्या पकड़ता है |
|---|---|
| `\b(fetch\|eval\|atob\|btoa\|Function\|setTimeout\|setInterval)\s*\(` | मना किए गए global को सीधे कॉल करना |
| `\bimport\s*\(` | dynamic import |
| `(?:window\|self\|globalThis\|top\|parent\|document\|navigator\|location)\s*\[` | होस्ट के global पर bracket से पहुँच, जैसे `window["fetch"]` |
| `\.cookie\b` | `.cookie` तक कोई भी पहुँच |
| `\.constructor\s*\.\s*constructor` | `[].constructor.constructor` वाला बच निकलने का रास्ता |
| `new\s+Function\b` | कोड बनाना |
| `(?:window\|self\|top\|parent\|globalThis)\s*\.\s*(?:Function\|eval\|fetch\|atob\|btoa\|XMLHttpRequest\|WebSocket\|EventSource)\b` | मना किए गए global की property पढ़ना |
| `(?:window\|top\|parent\|document\|location\|globalThis\|self)\s*\.\s*[A-Za-z_$][\w$]*\s*=[^=]` | होस्ट के global में कुछ लिखना |
| `Object\s*\.\s*prototype` | prototype को दूषित करना |
| `\.register\s*\(\s*['"]\S+\.js` | service worker रजिस्टर करना |

<!-- sources
manifest rules: internal/adapters/driven/pluginrunner/manifest.go - validateManifest (KindTheme)
theme selection: frontend/apps/web/src/features/user/ThemeSettings.tsx ; i18n common.appearance, common.theme ; GET /api/v1/plugins/themes
routes, auth: internal/adapters/driving/http/theme_asset_handler.go (theme.css open, theme.js authMw)
link injection, hidden iframe, connect-src none: frontend/apps/web/src/features/themes/ThemeLoader.tsx
sanitizer: internal/adapters/driven/pluginrunner/css_sanitizer.go - SanitizeCSS, reImport, reURLExternal, reURLExternalUnterminated, reImageSetCandidateExternal, reCustomPropExternalURL, reURLJavascript, reExpression, reMozBinding, reBehavior, reSensitiveSelector, overlaySignals.isOverlay, stripPointerEventsOnInteractive, reZIndex ; written back at install: internal/app/plugin/install.go - validateStagedPlugin
custom properties: examples/plugins/theme-dark-blue/theme.css
stable hooks: frontend/apps/web/src (evey-app-header, evey-media-grid, evey-media-card, evey-poster, evey-media-detail)
theme.js bridge: frontend/apps/web/src/features/themes/theme-bridge.ts - THEME_BRIDGE_SCRIPT ; policy: theme-css-policy.ts - applyThemeCSSPolicy, MAX_DECLARATIONS, MAX_VALUE_LENGTH, FORBIDDEN_IN_VALUE
scanner: internal/pkg/themejs/scan.go - blockedLiterals, blockedRegex, Scan
-->
