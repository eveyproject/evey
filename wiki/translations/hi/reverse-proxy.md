---
source_sha: af82a75f9598046f40d3b3b9257d529cb681628d24fcf9fab49c54919727e7fd
---

# रिवर्स प्रॉक्सी

Evey पोर्ट **9091** पर **API और वेब UI, दोनों** परोसता है (सादा HTTP)। रिवर्स
प्रॉक्सी को बस TLS समाप्त करना है और बाकी सब कुछ `evey:9091` पर आगे भेज देना है:
**अलग से कोई frontend बनाने या परोसने की ज़रूरत नहीं है**।

आप प्रॉक्सी कहीं भी चलाएँ, ये सेट कर दें ताकि Evey को अपना सार्वजनिक URL पता रहे
और वह प्रॉक्सी के forwarded हेडर पर भरोसा करे:

```env
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=172.16.0.0/12       # सिर्फ़ तभी ज़रूरी जब प्रॉक्सी एक container हो; loopback पर भरोसा हमेशा रहता है
```

`https://` वाला base URL ही Evey को बताता है कि उसके आगे कोई प्रॉक्सी TLS समाप्त
कर रहा है: तब cookies पर `Secure` फ़्लैग लगता है, HSTS भेजा जाता है, और भरोसेमंद
प्रॉक्सी से आया `X-Forwarded-Proto` माना जाता है। इसके लिए अलग से कोई स्विच नहीं
चाहिए (पुराना `EVEY_FORWARDED_PROTO` अब भी override के तौर पर स्वीकार होता है, पर
वह अप्रचलित है)।

## स्ट्रीमिंग - प्रॉक्सी की डिफ़ॉल्ट सेटिंग्स इस पर क्या असर डालती हैं

Evey डिफ़ॉल्ट सेटिंग्स पर छोड़े गए रिवर्स प्रॉक्सी के पीछे भी ठीक चलता है: लंबी
चलने वाली हर प्रतिक्रिया या तो लगातार बाइट भेजती रहती है (HLS सेगमेंट, सीधी
प्लेबैक, ट्रांसकोडिंग का आउटपुट) या हर 30 सेकंड में क्लाइंट को एक संकेत भेजती है
(event stream), इसलिए डिफ़ॉल्ट read timeout कभी नहीं लगता। फिर भी दो बातें जान
लेना ठीक रहेगा:

- **Buffering।** जो प्रॉक्सी प्रतिक्रियाओं को buffer करता है (nginx डिफ़ॉल्ट रूप
  से करता है), वह सीधी प्लेबैक वाली फ़ाइल क्लाइंट तक पहुँचाने से पहले अपने buffer
  और अस्थायी फ़ाइलों में पढ़ लेता है - काम तो चल जाता है, पर कई GB की फ़िल्म इस
  तरह प्रॉक्सी होस्ट की डिस्क से होकर गुज़रती है। स्ट्रीमिंग वाले पथों पर
  `proxy_buffering off` लगाने से यह टल जाता है।
- **स्कैन।** `/api/v1/libraries/*/scan` जब तक स्कैन चलता है, तब तक प्रगति भेजता
  रहता है; जो प्रॉक्सी किसी प्रतिक्रिया की कुल अवधि पर सीमा लगाता है (ऐसा कम ही
  होता है - nginx और Caddy डिफ़ॉल्ट रूप से नहीं लगाते), वह इसे बीच में काट देगा।
  स्कैन खुद तो सर्वर पर वैसे भी चलता रहता है।

जो पथ इससे जुड़े हैं:

```
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

साथ आने वाला Caddy और नीचे दिए गए Nginx/Traefik के उदाहरण, इन पथों पर दोनों बातें
सेट कर देते हैं (न buffering, न timeout)। Nginx Proxy Manager जैसे UI से चलने वाले
प्रॉक्सी में डिफ़ॉल्ट सेटिंग्स से काम चल जाता है और यह ट्यूनिंग एक वैकल्पिक
स्निपेट भर है।

## सुरक्षा हेडर और CSP - अगर आपका प्रॉक्सी अपने हेडर खुद भेजता है तो यह पढ़ें

Evey एक पूरी Content-Security-Policy भेजता है। अगर आपका प्रॉक्सी उसे ज्यों का
त्यों आगे बढ़ा देता है, तो आपका काम यहीं पूरा हो गया।

> 2026-07-23 तक यह बात गलत थी: ऐप की पॉलिसी में `media-src` और `worker-src` थे ही
> नहीं, इसलिए `blob:` के लिए `default-src 'self'` लागू हो जाता था और जो लोग पॉलिसी
> ज्यों की त्यों आगे भेजते थे, उनके यहाँ MSE प्लेबैक टूट जाती थी। यह इसलिए पकड़ में
> नहीं आया क्योंकि साथ आने वाला Caddy कॉन्फ़िगरेशन ऐप का हेडर हटा देता है और अपनी
> पॉलिसी खुद तय करता है। अगर आप उससे पुराना संस्करण Nginx Proxy Manager या Traefik
> के पीछे चला रहे हैं और आपका प्लेयर शुरू ही नहीं होता, तो वजह यही है - नीचे दी गई
> पॉलिसी प्रॉक्सी पर लगाएँ, या संस्करण अपडेट करें।

बहुत से सेटअप इसके बजाय उस हेडर को हटा देते हैं (Caddy में
`header_down -Content-Security-Policy`) और अपनी पॉलिसी खुद तय करते हैं - इसमें कोई
हर्ज नहीं, पर तब **आपकी** पॉलिसी में ये टोकन बचे रहने चाहिए, वरना कुछ सुविधाएँ
चुपचाप काम करना बंद कर देंगी:

| टोकन | डायरेक्टिव | इसके बिना क्या टूटता है |
| --- | --- | --- |
| `'unsafe-inline'` | `script-src` | हर एक्सटेंशन का UI (iframe खाली रह जाता है)। एक्सटेंशन का बंडल उपयोगकर्ता के टोकन से लाया जाता है और `srcdoc` iframe के भीतर रख दिया जाता है, और srcdoc वाला दस्तावेज़ उसे रखने वाले पृष्ठ की पॉलिसी अपना लेता है। इस बीच एक्सटेंशन को सीमा में बाँधने का काम iframe का `sandbox="allow-scripts"` करता है (अपारदर्शी origin, न parent DOM तक पहुँच, न cookie तक)। |
| `'wasm-unsafe-eval'` | `script-src` | ब्राउज़र में AC-3/E-AC-3 की प्लेबैक (ac3go WebAssembly डिकोडर कंपाइल ही नहीं हो पाता → फ़िल्म बिना आवाज़ चलती है)। यह सीमित टोकन सिर्फ़ Wasm कंपाइलेशन की अनुमति देता है - सामान्य `eval()` की नहीं। |
| `https://www.gstatic.com` | `script-src` (+ `frame-src`) | Google Cast का sender SDK कभी लोड नहीं होता → किसी को भी cast बटन नहीं मिलता। उपयोगकर्ता के अपने LAN पर Chromecast खोजने का ब्राउज़र के पास यही एक रास्ता है। |
| `blob:` | `media-src` | MSE प्लेबैक (hls.js स्ट्रीम को blob URL के ज़रिए जोड़ता है)। |
| `blob:` | `worker-src` | hls.js चुपचाप main-thread पर demuxing करने लगता है (मोबाइल पर प्लेबैक ज़्यादा अटकती है)। |

अपनी पॉलिसी खुद तय करने वाले प्रॉक्सी के लिए एक परखी हुई पॉलिसी:

```
default-src 'self'; base-uri 'self'; object-src 'none';
img-src 'self' https: data: blob:; media-src 'self' blob:;
style-src 'self' 'unsafe-inline';
script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com;
connect-src 'self' https: wss:; worker-src 'self' blob:; font-src 'self' data:;
frame-src https://www.gstatic.com
```

`base-uri` और `object-src` के लिए `default-src` पर वापसी नहीं होती, इसलिए जो
पॉलिसी इन्हें छोड़ देती है, वह `<base href>` इंजेक्शन और `<object>`/`<embed>`
एम्बेडिंग को खुला छोड़ देती है। अपनी पॉलिसी खुद तय करें तो इन्हें बनाए रखें।

कुछ बुनियादी नियम:

- **ब्राउज़र तक ठीक एक ही CSP हेडर पहुँचना चाहिए।** दो पहुँचने पर (जैसे ऐप का और
  आपके प्रॉक्सी का, या Caddy का और Nginx Proxy Manager के "Advanced" स्निपेट का)
  ब्राउज़र दोनों का *प्रतिच्छेदन* लागू करते हैं - सबसे सख़्त पॉलिसी जीतती है और
  खराबी चुपचाप होती है। जाँचने के लिए
  `curl -sI https://your.domain | grep -ci content-security-policy`
  → इसे `1` छापना चाहिए।
- **पॉलिसी को एक ही पंक्ति में रखें।** प्रॉक्सी कॉन्फ़िगरेशन में कई पंक्तियों में
  बँटी CSP का चुपचाप बिगड़ जाना जाना-पहचाना है: पंक्तियाँ जोड़ते समय जोड़ वाली
  जगह के अक्षर गायब हो जाते हैं (एक बार `...gstatic.com; connect-src` से
  `...gstaticonnect-src` बन गया था, जिससे Cast SDK बंद हो गया और दो डायरेक्टिव
  आपस में मिल गए)।
- `script-src` में `'unsafe-inline'` की कीमत सचमुच चुकानी पड़ती है, और फ़िलहाल यह
  ज़रूरी भी है (एक्सटेंशन के UI, ऊपर की तालिका देखें)। इसे हटाना सिर्फ़ उसी
  तैनाती में सुरक्षित है जहाँ कोई एक्सटेंशन frontend नहीं चलता।

साथ आने वाला हर उदाहरण डोमेन **एक ही जगह** से पढ़ता है: आपकी `.env` फ़ाइल का
`EVEY_DOMAIN`। इसे एक बार सेट कर दें; Caddy, Nginx और Traefik, तीनों इसे उठा लेंगे।

```env
EVEY_DOMAIN=media.example.com
```

## Caddy (साथ में शामिल)

रिपॉज़िटरी के साथ [`docker/Caddyfile`](../docker/Caddyfile) आता है
([`docker/Dockerfile.caddy`](../docker/Dockerfile.caddy) उससे एक image बनाती है):
यह `EVEY_DOMAIN` के लिए अपने आप HTTPS ले आता है और सब कुछ `evey:9091` पर आगे भेजता
है, और स्ट्रीमिंग वाले अपवाद इसमें पहले से सेट हैं। ऊपर वाले खंड के मानक सुरक्षा
हेडर और CSP भी यही तय करता है (और ऐप की अपनी CSP हटा देता है ताकि ब्राउज़र तक ठीक
एक ही पहुँचे)। अपनी compose फ़ाइल में एक `caddy` सेवा जोड़ें (रिपॉज़िटरी की
[`docker-compose.yml`](../docker-compose.yml) में `caddy` सेवा देखें)।

### अपने पहले से चल रहे प्रॉक्सी के पीछे (TLS ऊपर ही समाप्त होता हो)

जिस NAS पर पहले से कोई रिवर्स प्रॉक्सी चल रहा हो (विक्रेता का अपना, Nginx Proxy
Manager, या किसी साझा Docker नेटवर्क पर चलता Traefik), वहाँ **Caddy की ज़रूरत नहीं
है**: Evey अपने सुरक्षा हेडर और पूरी CSP खुद भेजता है, और स्ट्रीमिंग के नियम वैसे
भी सिर्फ़ वही प्रॉक्सी लागू कर सकता है जो सीधे क्लाइंट के सामने हो - उसके पीछे
दूसरा प्रॉक्सी लगाने से कुछ नहीं मिलता। evey सेवा को उसी प्रॉक्सी के नेटवर्क पर
रखें और लक्ष्य `evey-server:9091` रखें:

```yaml
services:
  evey:
    # कोई ports नहीं: भीतर आने का इकलौता रास्ता प्रॉक्सी है
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # प्रॉक्सी का subnet पता चलते ही इसे उसी तक सीमित कर दें
    # ...
networks:
  default:
    name: proxy-net
    external: true
```

इसके बाद उस प्रॉक्सी में, `evey-server:9091` वाले रूट के लिए: `X-Forwarded-For`
और `X-Forwarded-Proto` आगे भेजें, WebSocket समर्थन चालू करें, और ऊपर बताए
स्ट्रीमिंग के नियम लागू करें (स्ट्रीमिंग वाले पथों पर न buffering, न read/write
timeout) - नीचे Nginx, Traefik और Nginx Proxy Manager वाले खंडों में इसकी सटीक
पंक्तियाँ दी गई हैं।

## Nginx

[`examples/nginx/evey.conf.template`](../examples/nginx/evey.conf.template) का
इस्तेमाल करें (तैयार compose फ़ाइल
[`examples/nginx/docker-compose.nginx.yml`](../examples/nginx/docker-compose.nginx.yml)
में है)। nginx image शुरू होते समय उसमें `EVEY_DOMAIN` भरकर फ़ाइल तैयार करती है -
प्रमाणपत्र उन्हीं से मेल खाते फ़ाइल नामों के साथ मौजूद होने चाहिए। यह `evey:9091`
पर आगे भेजता है और स्ट्रीमिंग के लिए buffering बंद रखता है - frontend बनाने का कोई
चरण नहीं है।

## Traefik

[`examples/traefik/docker-compose.traefik.yml`](../examples/traefik/docker-compose.traefik.yml) देखें:

```bash
docker compose -f docker-compose.yml -f examples/traefik/docker-compose.traefik.yml up -d
```

## Nginx Proxy Manager

evey container उसी नेटवर्क पर होना चाहिए जिसका NPM इस्तेमाल करता है (ऊपर "अपने
पहले से चल रहे प्रॉक्सी के पीछे" देखें)। इसके बाद **Edit Proxy Host** में:

1. **Details**: Scheme `http`, Forward Hostname `evey-server`, Forward Port
   `9091`। **Websockets Support: on** (SyncPlay के लिए)। **Cache Assets: off** -
   वरना यह मीडिया सेगमेंट और प्लेलिस्ट को एक तय समय-सीमा के साथ कैश कर लेगा।
   Block Common Exploits चालू रहने दे सकते हैं (अगर कभी कोई खोज या एक्सटेंशन
   अपलोड बेवजह 403 लौटाए, तो इसे बंद कर दें)।
2. **SSL**: आपका प्रमाणपत्र, Force SSL, HTTP/2। यहाँ HSTS वैकल्पिक है - ऐप इसे
   पहले ही भेज देता है।
बस इतना ही: NPM `X-Forwarded-For` / `X-Forwarded-Proto` और Host हेडर खुद ही आगे
भेज देता है, और उसके डिफ़ॉल्ट timeout, Evey की स्ट्रीम पर कभी नहीं लगते (ऊपर
स्ट्रीमिंग वाला खंड देखें)।

**वैकल्पिक** - NPM को सीधी प्लेबैक वाली फ़ाइलें अपनी डिस्क से होकर buffer करने से
रोकने के लिए: **Advanced** → *Custom Nginx Configuration* (`Custom Locations` टैब
में यह regex लिखा ही नहीं जा सकता):

```nginx
# एक्सटेंशन अपलोड, बैकअप से पुनर्स्थापना।
client_max_body_size 100M;

# स्ट्रीमिंग - न buffering, न timeout।
location ~ ^/api/v1/media/.+/(hls|stream) {
    proxy_pass http://evey-server:9091;
    proxy_buffering off;
    proxy_request_buffering off;
    proxy_read_timeout 0;
    proxy_send_timeout 0;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}

# लाइब्रेरी स्कैन जब तक चलता है, तब तक अपनी प्रगति भेजता रहता है।
location ~ ^/api/v1/libraries/.+/scan {
    proxy_pass http://evey-server:9091;
    proxy_buffering off;
    proxy_read_timeout 0;
    proxy_send_timeout 0;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

बाकी हर पथ (वेब UI, API का शेष हिस्सा, SyncPlay का WebSocket) NPM के अपने proxy
ब्लॉक से होकर जाता है, जिसे Details टैब सेट करता है। यह स्निपेट न लगाएँ तो भी कुछ
नहीं टूटता - इससे सिर्फ़ इतना बदलता है कि बड़ी फ़ाइलें कहाँ buffer होती हैं।

## असली क्लाइंट IP और दूरस्थ पहुँच का नियंत्रण

जैसे ही आपका प्रॉक्सी क्लाइंट का असली पता आगे भेजने लगता है, Evey घर बैठे दर्शक
और इंटरनेट से आए दर्शक में फ़र्क कर पाता है। WAN की बिटरेट सीमा और दूरस्थ पहुँच का
नियंत्रण, दोनों इसी फ़र्क पर काम करते हैं, इसलिए ये दोनों सेटिंग्स तभी सही बरतती
हैं जब साथ-साथ सही हों:

- `EVEY_TRUSTED_PROXIES` में प्रॉक्सी आना चाहिए (Docker नेटवर्क पर container हो तो
  `172.16.0.0/12`; होस्ट पर चलता हो तो कुछ जोड़ने की ज़रूरत नहीं, loopback पर
  भरोसा हमेशा रहता है)। आप जो भी सेट करते हैं वह loopback वाली बुनियाद में जुड़ता
  है, उसकी जगह कभी नहीं लेता - इसलिए Docker वाली कोई वैल्यू इस नियंत्रण को असली
  क्लाइंट पते देखने से चुपचाप रोक नहीं सकती।
- एक के पीछे एक लगे प्रॉक्सियों में से हर एक को `X-Forwarded-For` बनाए रखना होगा।
  Caddy में इसके लिए `servers { trusted_proxies static private_ranges }` है; nginx
  में `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`।

**इसे चालू करने से यह बदल जाता है कि कौन दूरस्थ गिना जाएगा।** इससे पहले हर अनुरोध
ऐसा दिखता था मानो प्रॉक्सी के अपने निजी पते से आया हो, इसलिए सब LAN में गिने जाते
थे। इसके बाद असली दूरस्थ क्लाइंट सही पहचाने जाते हैं - और अगर दूरस्थ पहुँच बंद है,
तो उन्हें हर रूट पर मना कर दिया जाता है (`403 {"code":"remote_access_disabled"}`)।
ठीक इसी क्रम ने एक पूरे परिवार को दो दिन तक अपने ही चालू सर्वर से बाहर कर दिया था:
प्रॉक्सी का कॉन्फ़िगरेशन बदला था, ऐप की कोई सेटिंग नहीं, और बाहर से दिखने वाला
इकलौता लक्षण यह था कि क्लाइंट नाकाम हो रहे थे और सर्वर पर कोई त्रुटि दिख नहीं रही
थी।

अब Evey इससे बचाव करता है: जब भी कोई सार्वजनिक URL कॉन्फ़िगर होता है, दूरस्थ पहुँच
डिफ़ॉल्ट रूप से चालू रहती है, हर बार शुरू होने पर उसकी स्थिति लॉग होती है, मना किए
गए अनुरोध क्लाइंट IP के साथ लॉग होते हैं, "सार्वजनिक तो है पर मना कर रहा है" वाली
हालत में `evey admin doctor` साफ़ शब्दों में नाकाम होता है, और व्यवस्थापन पैनल
चेतावनी दिखाता है। प्रॉक्सी के भरोसे से जुड़ी सेटिंग्स बदलें, तो बाद में
`evey admin doctor` ज़रूर चलाकर देखें।
