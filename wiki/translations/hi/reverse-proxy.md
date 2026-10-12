---
source_sha: d47608ac11df56bc518674752f94a79d5498896103d51978e420575983e71cf4
reviewed: true
---
# रिवर्स प्रॉक्सी कॉन्फ़िगरेशन

Evey, API और वेब इंटरफ़ेस दोनों को एक साथ पोर्ट **9091** पर सादे HTTP में
देता है। रिवर्स प्रॉक्सी को बस TLS संभालना है और सब कुछ `evey:9091` पर भेजना
है। नीचे अपना प्रॉक्सी चुनें (साथ आने वाला Caddy, Nginx, Traefik, Nginx Proxy
Manager, या वह जो आप पहले से चला रहे हैं)। अगर आप स्ट्रीमिंग को ट्यून करते हैं
या अपने सुरक्षा हेडर खुद सेट करते हैं, तो आखिर के दो सेक्शन भी पढ़ें।

## Evey को प्रॉक्सी के बारे में बताएँ

प्रॉक्सी कोई भी हो, Evey को उसका सार्वजनिक पता चाहिए और यह भी कि किस प्रॉक्सी
पर भरोसा करना है। दोनों की जगह **एडमिन → रिमोट एक्सेस** है: `https://` वाला
**बाहरी URL**, और जब प्रॉक्सी एक कंटेनर हो, तो **विश्वसनीय प्रॉक्सी** में
उसका नेटवर्क। दोनों बिना रीस्टार्ट के लागू होते हैं। `https://` पते से ही Evey
को पता चलता है कि उसके आगे एक प्रॉक्सी TLS संभाल रहा है: कुकी को `Secure`
फ़्लैग मिलता है, HSTS भेजा जाता है, और विश्वसनीय प्रॉक्सी से आया
`X-Forwarded-Proto` माना जाता है। रिमोट एक्सेस का स्विच और क्लाइंट का असली
पता [रिमोट एक्सेस और HTTPS](administration/remote-access.md) में समझाए गए हैं।

यही जवाब environment से भी आ सकते हैं, उस इंस्टॉलेशन के लिए जो पूरा कोड के
रूप में लिखा है (दोनों सेट हों तो पैनल की बात मानी जाती है):

```env title=".env"
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=172.16.0.0/12       # सिर्फ़ तब ज़रूरी जब प्रॉक्सी एक कंटेनर हो; loopback पर हमेशा भरोसा है
```

पुराना `EVEY_FORWARDED_PROTO` अब भी ओवरराइड के रूप में माना जाता है, लेकिन
पुराना पड़ चुका है: पते का scheme ही काफ़ी है।

साथ आने वाले सभी उदाहरण डोमेन एक ही जगह से पढ़ते हैं: आपकी `.env` में
`EVEY_DOMAIN`। इसे एक बार सेट करें; Caddy, Nginx और Traefik तीनों इसे ले लेते
हैं।

```env title=".env"
EVEY_DOMAIN=media.example.com
```

## Caddy (साथ आने वाला)

### फ़ाइलें

[`docker/Caddyfile`](../docker/Caddyfile), जिसे
[`docker/Dockerfile.caddy`](../docker/Dockerfile.caddy) एक इमेज में बनाता है;
रिपॉज़िटरी की [`docker-compose.yml`](../docker-compose.yml) में `caddy`
सर्विस।

### सेट करें

1. `.env` में `EVEY_DOMAIN` सेट करें, और पक्का करें कि होस्ट के पोर्ट 80 और
   443 Caddy कंटेनर तक पहुँचते हैं।
2. रिपॉज़िटरी की compose फ़ाइल से `caddy` सर्विस अपनी फ़ाइल में जोड़ें और
   `docker compose up -d` चलाएँ। Caddy `EVEY_DOMAIN` के लिए Let's Encrypt
   सर्टिफ़िकेट अपने-आप ले लेता है।
3. **एडमिन → रिमोट एक्सेस** में `https://` वाला **बाहरी URL** सेट करें, और
   **विश्वसनीय प्रॉक्सी** में `172.16.0.0/12`।

### यह पहले से क्या संभालता है

सब कुछ `evey:9091` पर भेजता है, और स्ट्रीमिंग वाले पथ बिना बफ़रिंग और बिना
टाइमआउट के सीधे आगे जाते हैं।
[सुरक्षा वाले सेक्शन](#security-headers--csp---read-this-if-your-proxy-sets-its-own)
के सुरक्षा हेडर और मानक CSP खुद तय करता है, और ऐप का अपना CSP हटा देता है,
ताकि ब्राउज़र तक ठीक एक ही पहुँचे; जिन दो पेज की अपनी नीति है, वे उसे रखते
हैं।

## Nginx

### फ़ाइलें

[`examples/nginx/evey.conf.template`](../examples/nginx/evey.conf.template)
और एक तैयार compose overlay,
[`examples/nginx/docker-compose.nginx.yml`](../examples/nginx/docker-compose.nginx.yml)।

### सेट करें

1. `.env` में `EVEY_DOMAIN` सेट करें। nginx इमेज शुरू होते समय template में
   `EVEY_DOMAIN` भरकर उसे तैयार करती है।
2. सर्टिफ़िकेट और कुंजी वहीं रखें जहाँ template उन्हें ढूँढता है, उसी नाम से
   (`nginx-certs` volume में `/etc/ssl/certs/<domain>.pem`,
   `/etc/ssl/private/<domain>.key`): overlay आपके लिए सर्टिफ़िकेट नहीं लेता।
3. `docker compose -f docker-compose.yml -f examples/nginx/docker-compose.nginx.yml up -d`।
4. **एडमिन → रिमोट एक्सेस** में `https://` वाला **बाहरी URL** सेट करें, और
   **विश्वसनीय प्रॉक्सी** में `172.16.0.0/12`।

### यह पहले से क्या संभालता है

`evey:9091` पर भेजता है: स्ट्रीमिंग वाले पथों पर बफ़रिंग और टाइमआउट बंद,
SyncPlay का WebSocket अपग्रेड होता है, `X-Forwarded-For` और
`X-Forwarded-Proto` आगे भेजे जाते हैं, और प्लगइन अपलोड और बैकअप रिस्टोर के
लिए `client_max_body_size 2G`। यह ऐप के अपने सुरक्षा हेडर और CSP को जैसे का
तैसा आगे जाने देता है।

## Traefik

### फ़ाइलें

[`examples/traefik/docker-compose.traefik.yml`](../examples/traefik/docker-compose.traefik.yml)।

### सेट करें

1. Traefik को `websecure` entrypoint, `letsencrypt` सर्टिफ़िकेट resolver और
   `proxy` नाम के एक बाहरी Docker नेटवर्क के साथ चलाएँ; overlay के labels यही
   नाम मानकर चलते हैं।
2. `.env` में `EVEY_DOMAIN` सेट करें, फिर:

    ```bash
    docker compose -f docker-compose.yml -f examples/traefik/docker-compose.traefik.yml up -d
    ```

3. **एडमिन → रिमोट एक्सेस** में `https://` वाला **बाहरी URL** सेट करें, और
   **विश्वसनीय प्रॉक्सी** में `proxy` नेटवर्क की रेंज (`172.16.0.0/12` Docker
   के डिफ़ॉल्ट नेटवर्क को ढक लेता है)।

### यह पहले से क्या संभालता है

overlay सिर्फ़ router के labels जोड़ता है और `proxy` नेटवर्क से जुड़ता है।
डिफ़ॉल्ट सेटिंग पर Traefik न जवाबों को बफ़र करता है, न किसी जवाब की अवधि पर
सीमा लगाता है, इसलिए स्ट्रीमिंग के लिए कुछ ट्यून नहीं करना पड़ता। यह
`X-Forwarded-For` और `X-Forwarded-Proto` अपने-आप आगे भेजता है, और ऐप के सुरक्षा
हेडर को जैसे का तैसा जाने देता है।

## Nginx Proxy Manager

### फ़ाइलें

कोई नहीं: सब कुछ NPM के इंटरफ़ेस में सेट होता है। evey कंटेनर उसी नेटवर्क पर
होना चाहिए जिसे NPM इस्तेमाल करता है, देखें
[वह प्रॉक्सी जो आप पहले से चला रहे हैं](#a-proxy-you-already-run-tls-terminated-upstream)।

### सेट करें

**Edit Proxy Host** में:

1. **Details**: Scheme `http`, Forward Hostname `evey-server`, Forward Port
   `9091`। **Websockets Support: on** (SyncPlay)। **Cache Assets: off** - नहीं
   तो यह मीडिया के segments और playlists को एक तय मियाद के साथ कैश कर लेगा।
   Block Common Exploits चालू रह सकता है (अगर कभी कोई खोज या प्लगइन अपलोड बिना
   वजह 403 दे, तो इसे बंद करें)।
2. **SSL**: आपका सर्टिफ़िकेट, Force SSL, HTTP/2। HSTS यहाँ ज़रूरी नहीं है - ऐप
   उसे पहले से भेजता है।
3. **एडमिन → रिमोट एक्सेस** में `https://` वाला **बाहरी URL** सेट करें, और
   **विश्वसनीय प्रॉक्सी** में NPM नेटवर्क की रेंज।

### यह पहले से क्या संभालता है

NPM `X-Forwarded-For`, `X-Forwarded-Proto` और Host हेडर अपने-आप आगे भेजता है,
और उसके डिफ़ॉल्ट टाइमआउट Evey की स्ट्रीम पर कभी नहीं लगते। और कुछ ज़रूरी नहीं
है।

**ज़रूरी नहीं** - NPM को direct play वाली फ़ाइलें अपनी डिस्क से बफ़र करने से
रोकें: **Advanced** → *Custom Nginx Configuration* (`Custom Locations` टैब
regex नहीं लिख सकता):

```nginx
# प्लगइन अपलोड (100 MB तक) और बैकअप रिस्टोर (2 GB तक)।
client_max_body_size 2G;

# स्ट्रीमिंग - न बफ़रिंग, न टाइमआउट।
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

# लाइब्रेरी स्कैन जब तक चलता है, अपनी प्रगति भेजता रहता है।
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

बाकी हर पथ (वेब इंटरफ़ेस, बाकी API, SyncPlay का WebSocket) NPM के अपने
प्रॉक्सी ब्लॉक से जाता है, जिसे Details टैब सेट करता है। यह snippet छोड़ दें तो
भी कुछ नहीं टूटता - इससे सिर्फ़ यह बदलता है कि बड़ी फ़ाइलें कहाँ बफ़र होती हैं।

## वह प्रॉक्सी जो आप पहले से चला रहे हैं (TLS पहले ही संभाला गया)

जिस NAS पर पहले से एक रिवर्स प्रॉक्सी है (कंपनी का अपना, Nginx Proxy Manager,
या किसी साझा Docker नेटवर्क पर Traefik), वहाँ Caddy की ज़रूरत नहीं है: Evey
अपने सुरक्षा हेडर और पूरा CSP खुद भेजता है, और स्ट्रीमिंग के नियम वैसे भी
सिर्फ़ वही प्रॉक्सी लागू कर सकता है जो क्लाइंट के सामने है। उसके पीछे दूसरा
प्रॉक्सी कुछ नहीं जोड़ता।

### सेट करें

evey सर्विस को उस प्रॉक्सी के नेटवर्क पर रखें और `evey-server:9091` को
निशाना बनाएँ (कंटेनर का नाम; सर्विस का नाम `evey` भी काम करता है):

```yaml title="docker-compose.yml"
services:
  evey:
    # कोई पोर्ट नहीं: अंदर आने का एकमात्र रास्ता प्रॉक्सी है
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # प्रॉक्सी का subnet पता चलते ही इसे छोटा करें
    # ...
networks:
  default:
    name: proxy-net
    external: true
```

फिर उस प्रॉक्सी में, `evey-server:9091` वाले route के लिए: `X-Forwarded-For`
और `X-Forwarded-Proto` आगे भेजें, WebSocket सपोर्ट चालू करें, और स्ट्रीमिंग
वाले पथों पर नीचे दिए स्ट्रीमिंग नियम लागू करें - ऊपर Nginx और Nginx Proxy
Manager वाले सेक्शन ठीक-ठीक लाइनें दिखाते हैं।

### यह पहले से क्या संभालता है

यह प्रॉक्सी पर निर्भर है: जाँचें कि ब्राउज़र तक ठीक एक ही CSP हेडर पहुँचता है
(अगले सेक्शन देखें), और विश्वसनीय प्रॉक्सी सेट होने के बाद
`evey admin doctor` चलाएँ।

## स्ट्रीमिंग - प्रॉक्सी की डिफ़ॉल्ट सेटिंग इस पर क्या असर डालती है

Evey अपनी डिफ़ॉल्ट सेटिंग पर छोड़े गए रिवर्स प्रॉक्सी के पीछे काम करता है: हर
लंबा चलने वाला जवाब या तो लगातार बाइट भेजता है (HLS segments, direct play,
ट्रांसकोड का आउटपुट), या हर 30 सेकंड में क्लाइंट को ping करता है (event
streams), इसलिए डिफ़ॉल्ट read timeout कभी नहीं लगता। फिर भी दो बातें जानना
अच्छा है:

- **बफ़रिंग।** जो प्रॉक्सी जवाबों को बफ़र करता है (nginx डिफ़ॉल्ट रूप से
  करता है), वह direct play वाली फ़ाइल को क्लाइंट तक पहुँचने से पहले अपने बफ़र
  और अस्थायी फ़ाइलों में पढ़ता है - यह काम करता है, लेकिन तब कई GB की फ़िल्म
  प्रॉक्सी वाले होस्ट की डिस्क से होकर गुज़रती है। स्ट्रीमिंग वाले पथों पर
  `proxy_buffering off` इससे बचाता है।
- **स्कैन।** `/api/v1/libraries/*/scan` स्कैन चलने तक प्रगति भेजता रहता है;
  जो प्रॉक्सी किसी जवाब की कुल अवधि पर सीमा लगाता है (ऐसा कम होता है - nginx,
  Caddy और Traefik डिफ़ॉल्ट रूप से नहीं लगाते), वह इसे बीच में काट देगा। स्कैन
  खुद सर्वर पर फिर भी चलता रहता है।

जिन पथों की बात है:

```
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

साथ आने वाला Caddy और Nginx का उदाहरण इन पथों पर दोनों सेट करते हैं (न
बफ़रिंग, न टाइमआउट)। Traefik का उदाहरण और Nginx Proxy Manager जैसा इंटरफ़ेस से
चलने वाला प्रॉक्सी अपनी डिफ़ॉल्ट सेटिंग रखते हैं, जो ठीक है; ऊपर वाला NPM
snippet ज़रूरी नहीं है।

## सुरक्षा हेडर और CSP - अगर आपका प्रॉक्सी अपने हेडर लगाता है तो यह पढ़ें

Evey एक पूरी Content-Security-Policy भेजता है। अगर आपका प्रॉक्सी उसे आगे जाने
देता है, तो आपका काम हो गया।

कई सेटअप इसकी जगह इसे हटा देते हैं (Caddy में `header_down
-Content-Security-Policy`) और अपनी नीति खुद बनाते हैं - यह ठीक है, लेकिन तब
**आपकी** नीति में ये टोकन रहने चाहिए, नहीं तो कुछ सुविधाएँ चुपचाप टूट जाती
हैं:

| टोकन | Directive | इसके बिना क्या टूटता है |
| --- | --- | --- |
| `'self'` | `frame-src` | हर प्लगइन का इंटरफ़ेस और थीम स्क्रिप्ट (खाली iframe)। ये `/plugin-frame.html` में चलते हैं, जो Evey का एक पेज है जिसे ऐप एक frame में दिखाता है (नीचे देखें)। |
| `'wasm-unsafe-eval'` | `script-src` | ब्राउज़र में AC-3/E-AC-3 प्लेबैक (ac3go का WebAssembly डिकोडर compile नहीं हो पाता, इसलिए फ़िल्म बिना आवाज़ के चलती है)। यह सीमित टोकन सिर्फ़ Wasm compile करने की अनुमति देता है, आम `eval()` की नहीं। |
| `https://www.gstatic.com` | `script-src` (+ `frame-src`) | Google Cast का sender SDK कभी लोड नहीं होता, इसलिए किसी को कास्ट बटन नहीं मिलता। ब्राउज़र के लिए उपयोगकर्ता के अपने LAN पर Chromecast ढूँढने का यही एकमात्र तरीका है। |
| `blob:` | `media-src` | MSE प्लेबैक (hls.js स्ट्रीम को blob URL से जोड़ता है)। |
| `blob:` | `worker-src` | hls.js चुपचाप main thread पर demuxing करने लगता है (मोबाइल पर ज़्यादा अटकता है)। |

अपनी नीति बनाने वाले प्रॉक्सी के लिए एक जाँची-परखी नीति, वही जो साथ आने वाला
Caddy भेजता है:

```
default-src 'self'; base-uri 'self'; object-src 'none';
img-src 'self' https: data: blob:; media-src 'self' blob:;
style-src 'self' 'unsafe-inline';
script-src 'self' 'wasm-unsafe-eval' https://www.gstatic.com;
connect-src 'self' https: wss:; worker-src 'self' blob:; font-src 'self' data:;
frame-src 'self' https://www.gstatic.com
```

`base-uri` और `object-src` के न होने पर `default-src` उनकी जगह नहीं लेता, इसलिए
जो नीति इन्हें छोड़ देती है, उसमें `<base href>` injection और
`<object>`/`<embed>` embedding पर कोई रोक नहीं रहती। अगर आप अपनी नीति खुद बनाते
हैं तो इन्हें रखें। `script-src` में `'unsafe-inline'` नहीं है: पेज में डाली
गई स्क्रिप्ट नहीं चलती। इसे वापस न जोड़ें - ऐप में किसी चीज़ को इसकी ज़रूरत
नहीं है।

दो पेज की अपनी नीति है, जिसे ऐप भेजता है; इन दो पथों पर ऐप का हेडर रहने दें:

- `/plugin-frame.html`, वह पेज जिसमें प्लगइन के इंटरफ़ेस और थीम स्क्रिप्ट चलते
  हैं। इसकी नीति इसे sandbox में रखती है (iframe के बाहर खोलने पर भी), इसके
  नेटवर्क को उन्हीं होस्ट तक सीमित करती है जो प्लगइन का manifest बताता है, और
  सिर्फ़ ऐप को इसे frame में दिखाने देती है, इसलिए यह
  `X-Frame-Options: SAMEORIGIN` भी भेजता है: इस पथ पर इसे `DENY` से न बदलें,
  नहीं तो हर प्लगइन का इंटरफ़ेस खाली रहेगा।
- `/cast-receiver.html`, वह पेज जो Chromecast Evey से चलाते समय लोड करता है।
  Cast प्लेयर इसमें `www.gstatic.com` से styles, fonts और images डालता है। उस
  पथ पर ऐप का हेडर रहने दें, या यह नीति सिर्फ़ उसी पथ पर लगाएँ:

```
default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com; style-src 'self' 'unsafe-inline' https://www.gstatic.com; img-src 'self' data: https: blob:; media-src 'self' blob: https:; worker-src 'self' blob:; font-src 'self' data: https://www.gstatic.com; connect-src 'self' https: wss:; frame-src https://www.gstatic.com; base-uri 'self'; object-src 'none'
```

!!! warning "ब्राउज़र तक ठीक एक ही CSP हेडर पहुँचना चाहिए"
    दो हों (ऐप का और आपके प्रॉक्सी का, या Caddy का और Nginx Proxy Manager के
    "Advanced" snippet का), तो ब्राउज़र दोनों का *साझा हिस्सा* लागू करते हैं:
    सबसे सख्त वाला जीतता है और गड़बड़ी चुपचाप होती है। इससे जाँचें:
    `curl -sI https://your.domain | grep -ci content-security-policy`, जिसे
    `1` छापना चाहिए।

किसी भी बदलाव के बाद दो और जाँचें:

- **नीति को एक ही लाइन में रखें।** प्रॉक्सी की config में कई लाइनों में बँटा
  CSP चुपचाप बिगड़ने का जाना-माना मामला है: जोड़ते समय सिरे वाले अक्षर गायब हो
  जाते हैं (एक बार `...gstatic.com; connect-src` बदलकर `...gstaticonnect-src`
  हो गया, जिससे Cast SDK बंद हो गया और दो directives आपस में मिल गए)।
- **प्लगइन frame अपनी नीति रखता है:**
  `curl -sI https://your.domain/plugin-frame.html | grep -i -e content-security -e x-frame`
  में `sandbox allow-scripts` और `SAMEORIGIN` दिखने चाहिए।

## क्लाइंट के असली IP और रिमोट एक्सेस का दरवाज़ा

जब प्रॉक्सी क्लाइंट का असली पता आगे भेजता है, तो Evey घर के दर्शक और
इंटरनेट के दर्शक में फ़र्क कर पाता है: **अधिकतम रिमोट बिटरेट** और रिमोट
एक्सेस का स्विच इसी पर काम करते हैं। दो नियम:

- विश्वसनीय प्रॉक्सी (पैनल या `EVEY_TRUSTED_PROXIES`) में प्रॉक्सी शामिल होना
  चाहिए: Docker नेटवर्क पर कंटेनर हो तो `172.16.0.0/12`; होस्ट पर चलता हो तो
  कुछ जोड़ने की ज़रूरत नहीं, क्योंकि loopback पर हमेशा भरोसा है और आप जो भी सेट
  करते हैं वह उसमें जुड़ जाता है।
- एक के पीछे एक लगे हर प्रॉक्सी को `X-Forwarded-For` बचाकर रखना होता है। Caddy
  में यह `servers { trusted_proxies static private_ranges }` है; nginx
  `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;` इस्तेमाल करता
  है।

इसे चालू करने से बदल जाता है कि कौन रिमोट गिना जाता है: जिस सर्वर का रिमोट
एक्सेस बंद था, वह असली रिमोट क्लाइंट को मना करने लगता है
(`403 {"code":"remote_access_disabled"}`), जबकि ऐप की कोई सेटिंग नहीं बदली।
जब सार्वजनिक पता सेट हो और कोई चुनाव न किया गया हो, तो Evey रिमोट एक्सेस
अपने-आप खोल देता है, हर बार शुरू होने पर उसकी स्थिति और हर इनकार को क्लाइंट के
पते के साथ लॉग में लिखता है, और `evey admin doctor` "बाहर खुला है लेकिन मना कर
रहा है" पर विफल होता है। पूरा तरीका
[रिमोट एक्सेस और HTTPS](administration/remote-access.md#the-real-client-address)
में बताया गया है।

<!-- sources
one port for API and UI, https mode from the scheme: internal/config/config.go (EVEY_FORWARDED_PROTO deprecated) ; internal/config/network_live.go - EffectiveHTTPS
bundled Caddy: docker/Caddyfile (streaming matcher, @ownPolicy, header_down CSP) ; docker/Dockerfile.caddy ; docker-compose.yml caddy service
nginx: examples/nginx/evey.conf.template ; examples/nginx/docker-compose.nginx.yml (nginx-certs volume, no ACME)
traefik: examples/traefik/docker-compose.traefik.yml (labels + proxy network only)
streaming paths: cmd/evey/gateway.go ; event streams ping 30 s ; scan progress stream
csp and own-policy pages: internal/adapters/driving/http/helpers.go - securityHeadersMiddleware ; /plugin-frame.html ; /cast-receiver.html
trusted proxies, catch-all refused, doctor: internal/config/trusted_proxies.go ; cmd/evey/admin_ops.go - checkTrustedProxies
remote access inferred: docs/adr/0026-remote-access-inferred-from-a-published-url.md
labels: i18n admin.remoteAccess.*
-->
