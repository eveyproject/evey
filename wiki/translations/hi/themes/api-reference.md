---
source_sha: 72324d88dd91ef30b9ee280c903c4879dad626512f8f23deee264b8281e0538f
---

# थीम API का संदर्भ

यह दस्तावेज़ उन बाहरी डेवलपरों के लिए है जो Evey की थीम लिख रहे हैं। थीम पर लागू होने वाली सुरक्षा बंदिशों के लिए [`security-model.md`](security-model.md) देखें।

## थीम होती क्या है

थीम एक `kind: theme` पैकेज है। इसकी **कोई बैकएंड प्रक्रिया नहीं** होती। इसमें ये हो सकते हैं:

- `theme.css` - sanitize होने के बाद हर पृष्ठ के `<head>` में डाल दी जाती है
- `theme.js` - वैकल्पिक JavaScript, जो एक अलग-थलग परिवेश में सीमित `evey.theme.*` API के साथ लोड होती है

थीम `language`, `entrypoint` या `permissions` घोषित नहीं कर सकतीं। ऐसा करने पर इंस्टॉल करने से मना कर दिया जाता है।

## Manifest का ढाँचा

```yaml
name: aurora               # ज़रूरी; पढ़ने लायक नाम
version: 1.0.0             # ज़रूरी; semver
kind: theme                # ज़रूरी; "theme" ही होना चाहिए

description: "..."         # वैकल्पिक
author: "..."              # वैकल्पिक
min_evey_version: "1.0.0"  # वैकल्पिक

# language, entrypoint, permissions नहीं - थीम के लिए ये अमान्य हैं
```

ZIP की जड़ में `theme.css` या `theme.js`, या दोनों, होने चाहिए। जिस पैकेज में इनमें से कुछ भी न हो, वह स्वीकार तो हो जाता है, पर थीम का कोई असर दिखता नहीं।

## theme.css

यह `/api/v1/themes/<id>/theme.css` पर परोसी जाती है। इंस्टॉल के समय फ़ाइल CSS sanitizer से होकर गुज़रती है (किन रचनाओं पर रोक है, यह [`security-model.md`](security-model.md) में देखें)। सहेजा और परोसा वही जाता है जो sanitize होकर निकलता है - यानी अगर कोई नियम पकड़ा गया, तो आपने जो लिखा और जो असल में डाला गया, उनमें फ़र्क हो सकता है।

स्टाइलशीट `<head>` में `<link rel="stylesheet">` के रूप में डाली जाती है और पूरे होस्ट दस्तावेज़ पर लागू होती है। सामान्य CSS चलती है: custom properties, `@keyframes`, `@media`, `@font-face` (`data:` स्रोतों के साथ), `@layer`, container queries।

### थीम के पक्के ठिकाने (selector का अनुबंध)

Evey कुछ गिने-चुने सजावटी और ग़ैर-संवेदनशील तत्वों पर **पक्की क्लास** लगाता है,
जिन्हें आप बिना उन (बदलती रहने वाली) utility क्लास पर टिके निशाना बना सकते हैं।
ये एक अनुबंध हैं: रिफ़ैक्टर होते रहने पर भी इनके नाम नहीं बदलेंगे।

| Selector | तत्व | किस चीज़ की सजावट बदलना सुरक्षित है |
|---|---|---|
| `.evey-app-header` | सबसे ऊपर की पट्टी | रंग, ऊँचाई, अंतर, blur - उसके भीतर के नियंत्रण **कभी** छिपाएँ या हटाएँ नहीं |
| `.evey-media-grid` | मीडिया कार्ड की ग्रिड (लाइब्रेरी, पंक्तियाँ) | स्तंभ, `gap`, संरेखण |
| `.evey-media-card` | एक मीडिया कार्ड (पोस्टर + विवरण) | चौड़ाई, अंतर, hover |
| `.evey-poster` | पोस्टर या कलाकृति (विवरण पृष्ठ पर भी) | `aspect-ratio`, `border-radius`, छाया, fit |
| `.evey-media-detail` | मीडिया विवरण पृष्ठ की जड़ | विवरण वाले हिस्से का लेआउट |

sanitizer जिस चीज़ पर रोक लगाता है, वह यहाँ भी लागू रहती है (न `@import`, न पूरी
स्क्रीन ढकने वाली परतें, न नियंत्रणों पर `pointer-events:none`, न
`[type=password]` / `.admin-*` / `.token-*` / `.secret-*`, और `z-index` बाँध दिया
जाता है)। थीम इन ठिकानों की सजावट जैसे चाहे बदल सकती है; पर वह हेडर को इस काम पर
**नहीं** लगा सकती कि भरोसेमंद नियंत्रण (लॉगआउट, प्रोफ़ाइल, असली सूचनाएँ) छिप जाएँ
या उनका भेस धर लिया जाए।

### उदाहरण: बड़े पोस्टर, सघन ग्रिड, तीखे कोने

```css
/* सघन ग्रिड पर बड़े पोस्टर */
.evey-media-card { width: 12rem; }
.evey-poster     { border-radius: 2px; }
.evey-media-grid { gap: 0.75rem; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); }

.evey-media-card:hover { transform: scale(1.03); transition: transform 120ms ease; }
```

### उदाहरण: अपनी बनाई प्लेयर थीम

```css
.player-controls {
  background: linear-gradient(to top, rgba(0,0,0,.8), transparent);
  border-radius: 0 0 8px 8px;
}

.player-progress-bar {
  accent-color: var(--color-accent);
  height: 4px;
}
```

## theme.js

यह `/api/v1/themes/<id>/theme.js` पर परोसी जाती है (इसके लिए प्रमाणीकरण ज़रूरी है)। यह एक ऐसे iframe के भीतर लोड होती है जिस पर `sandbox="allow-scripts"` और एक कसी हुई Content Security Policy लगी रहती है - स्क्रिप्ट होस्ट पृष्ठ तक पहुँच नहीं सकती, नेटवर्क अनुरोध नहीं भेज सकती, और भंडारण नहीं पढ़ सकती।

इसके लिए बस एक ही सतह खुली रहती है, `window.evey.theme`, जिसे होस्ट का पुल `theme.js` चलने से पहले डाल देता है।

### `evey.theme.css(rules)`

CSS की एक string को चलते-चलते होस्ट की स्टाइल सतह में डाल देता है। जहाँ सजावट गिनी हुई वैल्यू या थीम के भीतर की उपयोगकर्ता गतिविधि पर निर्भर हो, वहाँ इसका उपयोग करें।

```js
evey.theme.css(`
  .hero-banner {
    background-image: url(data:image/svg+xml,...);
  }
`);
```

### `evey.theme.log(level, msg)`

ब्राउज़र के console में एक लॉग संदेश भेजता है, जिसके आगे `[evey-theme]` लगा रहता है। अंतिम उपयोगकर्ताओं तक यह नहीं पहुँचता।

- `level`: `"debug"` | `"info"` | `"warn"` | `"error"`
- `msg`: सादे पाठ की string

```js
evey.theme.log("info", "Aurora theme loaded.");
```

## V2 का API (आगे की योजना)

नीचे दिए गए तरीके योजना में हैं और V1 में अभी उपलब्ध नहीं हैं:

### `evey.theme.registerComponent(slot, render)` (V2)

किसी नाम वाले slot के लिए एक render फ़ंक्शन दर्ज करता है। slot लगते समय होस्ट `render(props, container)` को बुलाता है। `container` एक DOM नोड है, जो थीम के अपने shadow परिवेश तक सीमित रहता है - `container` में लिखा हुआ होस्ट पृष्ठ में एक ShadowRoot के भीतर दिखता है।

```js
// V2 - अभी उपलब्ध नहीं
evey.theme.registerComponent("home.hero", function(props, container) {
  container.innerHTML = `<div class="my-hero">Welcome, ${props.userName}</div>`;
});
```

### `evey.theme.observe(stateKey, callback)` (V2)

होस्ट की उस स्थिति की सदस्यता लेता है जो सिर्फ़ पढ़ी जा सकती है। जिस कुंजी पर नज़र रखी जा रही है, वह बदलते ही callback बुलाया जाता है।

```js
// V2 - अभी उपलब्ध नहीं
evey.theme.observe("player.playing", function(isPlaying) {
  evey.theme.css(isPlaying ? ".hero { opacity: 0.3; }" : "");
});
```

### `evey.theme.transition(selector, animation)` (V2)

`selector` से मिले होस्ट के किसी तत्व पर, पुल के ज़रिए, नाम वाला एनिमेशन लगाता है (DOM तक सीधी पहुँच नहीं)।

## उदाहरण

एक्सटेंशन के उदाहरणों के साथ दो थीम आती हैं: `examples/plugins/theme-dark-blue` (एक छोटी `theme.css` जो सिर्फ़ रंगों के वेरिएबल बदलती है) और `examples/plugins/open-workshop` (पूरा नया रूप)। शुरुआत के लिए इनमें से किसी को भी कॉपी करें - दोनों पहले से sanitizer पास करती हैं।
