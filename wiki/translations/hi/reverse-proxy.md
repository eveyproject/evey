---
source_sha: 3f509ca8086807c1f4d1c33d492ae026a83e372b7e31aba45a835fc6bbbd56d9
reviewed: false
---
# रिवर्स प्रॉक्सी

Evey पोर्ट **9091** पर (बिना एन्क्रिप्शन वाला HTTP) **API और वेब इंटरफ़ेस दोनों** देता है। रिवर्स प्रॉक्सी को केवल TLS समाप्त करना है और सब कुछ `evey:9091` पर भेजना है: **कोई अलग फ़्रंटएंड बनाने या परोसने की ज़रूरत नहीं**।

आप प्रॉक्सी कहीं भी चलाएँ, Evey को उसका सार्वजनिक URL और यह बताएँ कि किस प्रॉक्सी पर भरोसा करना है। सबसे आसान जगह **Admin → रिमोट एक्सेस** है: एक `https://` **बाहरी URL**, और जब प्रॉक्सी एक कंटेनर हो तो **विश्वसनीय प्रॉक्सी** में उसका नेटवर्क। दोनों बिना रीस्टार्ट के लागू होते हैं। पूरी तरह कोड के रूप में बताए गए इंस्टॉलेशन के लिए वही जवाब एनवायरनमेंट से भी आ सकते हैं (दोनों सेट होने पर पैनल भारी पड़ता है):

```env title=".env"
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=172.16.0.0/12       # केवल तब ज़रूरी जब प्रॉक्सी एक कंटेनर हो; लूपबैक पर हमेशा भरोसा होता है
```

एक `https://` पता ही Evey को बताता है कि उसके आगे कोई प्रॉक्सी TLS समाप्त करता है: कुकी को `Secure` फ़्लैग मिलता है, HSTS भेजा जाता है, और विश्वसनीय प्रॉक्सी से आए `X-Forwarded-Proto` पर भरोसा किया जाता है। किसी अलग स्विच की ज़रूरत नहीं (पुराना `EVEY_FORWARDED_PROTO` अब भी ओवरराइड के रूप में स्वीकार होता है, पर अप्रचलित है)।

## स्ट्रीमिंग: प्रॉक्सी की डिफ़ॉल्ट सेटिंग्स इस पर क्या असर डालती हैं

Evey डिफ़ॉल्ट सेटिंग्स पर छोड़े गए रिवर्स प्रॉक्सी के पीछे काम करता है: हर लंबा चलने वाला जवाब या तो लगातार बाइट भेजता है (HLS सेगमेंट, सीधा प्लेबैक, ट्रांसकोड आउटपुट) या हर 30 सेकंड पर क्लाइंट को पिंग करता है (इवेंट स्ट्रीम), इसलिए डिफ़ॉल्ट रीड टाइमआउट कभी नहीं आता। फिर भी दो बातें जानने लायक़ हैं:

- **बफ़रिंग।** जो प्रॉक्सी जवाबों को बफ़र करता है (nginx डिफ़ॉल्ट रूप से करता है) वह सीधा प्लेबैक वाली फ़ाइल को क्लाइंट तक पहुँचने से पहले अपने बफ़र और अस्थायी फ़ाइलों में पढ़ता है: यह काम करता है, लेकिन तब कई GB की फ़िल्म प्रॉक्सी होस्ट की डिस्क से होकर गुज़रती है। स्ट्रीमिंग पाथ पर `proxy_buffering off` इससे बचाता है।
- **स्कैन।** `/api/v1/libraries/*/scan` स्कैन चलने तक प्रगति भेजता रहता है; जो प्रॉक्सी किसी जवाब की कुल अवधि सीमित करता है (दुर्लभ: nginx और Caddy डिफ़ॉल्ट रूप से नहीं करते) वह इसे बीच में काट देगा। स्कैन ख़ुद सर्वर पर फिर भी चलता रहता है।

संबंधित पाथ:

```
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

साथ आने वाला Caddy और नीचे के Nginx/Traefik उदाहरण इन पाथ पर दोनों बातें सेट करते हैं (कोई बफ़रिंग नहीं, कोई टाइमआउट नहीं)। Nginx Proxy Manager जैसे इंटरफ़ेस से चलने वाले प्रॉक्सी के साथ डिफ़ॉल्ट सेटिंग्स ठीक हैं और ट्यूनिंग एक वैकल्पिक स्निपेट है।

## सुरक्षा हेडर और CSP: अगर आपका प्रॉक्सी अपने हेडर सेट करता है तो इसे पढ़ें

Evey एक पूरी Content-Security-Policy भेजता है। अगर आपका प्रॉक्सी उसे आगे जाने देता है, तो आपका काम हो गया।

कई सेटअप इसके बजाय इसे हटा देते हैं (Caddy में `header_down -Content-Security-Policy`) और अपनी नीति लगाते हैं: यह ठीक है, लेकिन तब **आपकी** नीति में ये टोकन बने रहने चाहिए, वरना कुछ सुविधाएँ चुपचाप टूट जाती हैं:

| टोकन | डायरेक्टिव | इसके बिना क्या टूटता है |
| --- | --- | --- |
| `'unsafe-inline'` | `script-src` | हर प्लगइन इंटरफ़ेस (ख़ाली iframe)। प्लगइन बंडल उपयोगकर्ता के टोकन से लाया जाता है और एक `srcdoc` iframe में इनलाइन किया जाता है, और srcdoc दस्तावेज़ उसे एम्बेड करने वाले की नीति विरासत में लेता है। इस बीच प्लगइन को सीमित रखने का काम iframe का `sandbox="allow-scripts"` करता है (अपारदर्शी ओरिजिन, पैरेंट के DOM और कुकी तक कोई पहुँच नहीं)। |
| `'wasm-unsafe-eval'` | `script-src` | ब्राउज़र में AC-3/E-AC-3 प्लेबैक (ac3go WebAssembly डिकोडर कंपाइल नहीं हो पाता → फ़िल्म बिना आवाज़ के चलती है)। यह संकरा टोकन है जो केवल Wasm कंपाइल करने देता है, सामान्य `eval()` नहीं। |
| `https://www.gstatic.com` | `script-src` (+ `frame-src`) | Google Cast सेंडर SDK कभी लोड नहीं होता → किसी के लिए कास्ट बटन नहीं। ब्राउज़र के लिए उपयोगकर्ता के अपने LAN पर Chromecast खोजने का यही एकमात्र तरीका है। |
| `blob:` | `media-src` | MSE प्लेबैक (hls.js स्ट्रीम को blob URL के ज़रिए जोड़ता है)। |
| `blob:` | `worker-src` | hls.js चुपचाप मुख्य थ्रेड पर डीमक्सिंग करने लगता है (मोबाइल पर ज़्यादा अटकता है)। |

अपनी नीति लगाने वाले प्रॉक्सी के लिए एक भरोसेमंद नीति:

```
default-src 'self'; base-uri 'self'; object-src 'none';
img-src 'self' https: data: blob:; media-src 'self' blob:;
style-src 'self' 'unsafe-inline';
script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com;
connect-src 'self' https: wss:; worker-src 'self' blob:; font-src 'self' data:;
frame-src https://www.gstatic.com
```

`base-uri` और `object-src` `default-src` पर वापस नहीं जाते, इसलिए जो नीति इन्हें छोड़ देती है वह `<base href>` इंजेक्शन और `<object>`/`<embed>` एम्बेडिंग को बिना रोक-टोक छोड़ देती है। अगर आप अपनी नीति लगाते हैं तो इन्हें रखें।

एक पेज को ज़्यादा खुली नीति चाहिए: `/cast-receiver.html`, वह पेज जिसे Chromecast Evey से चलाते समय लोड करता है। Cast प्लेयर इसमें `www.gstatic.com` से स्टाइल, फ़ॉन्ट और तस्वीरें डालता है, और इसी कारण ऐप उस पेज को उसकी अपनी नीति भेजता है। उस पाथ पर ऐप का हेडर रहने दें, या यह नीति केवल उस पाथ पर लागू करें:

```
default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com; style-src 'self' 'unsafe-inline' https://www.gstatic.com; img-src 'self' data: https: blob:; media-src 'self' blob: https:; worker-src 'self' blob:; font-src 'self' data: https://www.gstatic.com; connect-src 'self' https: wss:; frame-src https://www.gstatic.com; base-uri 'self'; object-src 'none'
```

व्यावहारिक नियम:

- **ब्राउज़र तक ठीक एक CSP हेडर पहुँचना चाहिए।** दो होने पर (जैसे ऐप का और आपके प्रॉक्सी का, या Caddy का और Nginx Proxy Manager के "Advanced" स्निपेट का), ब्राउज़र उनका *प्रतिच्छेदन* लागू करते हैं: सबसे सख़्त जीतती है और विफलता चुपचाप होती है। `curl -sI https://your.domain | grep -ci content-security-policy` से जाँचें → इसे `1` दिखाना चाहिए।
- **नीति को एक ही लाइन में रखें।** प्रॉक्सी कॉन्फ़िग में कई लाइनों में बँटी CSP एक क्लासिक चुपचाप होने वाली गड़बड़ी है: जोड़ने पर जोड़ की जगह के अक्षर निगल लिए जाते हैं (`...gstatic.com; connect-src` एक बार `...gstaticonnect-src` बन गया, जिससे Cast SDK बंद हो गया और दो डायरेक्टिव मिल गए)।
- `script-src` में `'unsafe-inline'` की असली क़ीमत है, और अभी यह ज़रूरी है (प्लगइन इंटरफ़ेस, तालिका देखें)। इसे हटाना केवल ऐसे डिप्लॉयमेंट पर सुरक्षित है जो कोई प्लगइन फ़्रंटएंड नहीं चलाता।

साथ आने वाला हर उदाहरण डोमेन **एक ही जगह** से पढ़ता है: आपके `.env` में `EVEY_DOMAIN`। इसे एक बार सेट करें; Caddy, Nginx और Traefik सभी इसे उठा लेते हैं।

```env title=".env"
EVEY_DOMAIN=media.example.com
```

## Caddy (साथ में)

रिपॉज़िटरी में [`docker/Caddyfile`](../docker/Caddyfile) है ([`docker/Dockerfile.caddy`](../docker/Dockerfile.caddy) से एक इमेज में बनाया गया): यह `EVEY_DOMAIN` के लिए स्वचालित HTTPS लेता है और सब कुछ `evey:9091` पर भेजता है, स्ट्रीमिंग के अपवाद पहले से सेट हैं। यह ऊपर वाले सेक्शन के मानक सुरक्षा हेडर और CSP भी लगाता है (और ऐप की अपनी CSP हटा देता है ताकि ब्राउज़र तक ठीक एक पहुँचे)। अपने compose में एक `caddy` सर्विस जोड़ें (रिपॉज़िटरी के [`docker-compose.yml`](../docker-compose.yml) में `caddy` सर्विस देखें)।

### पहले से चल रहे प्रॉक्सी के पीछे (TLS ऊपर समाप्त)

जिस NAS पर पहले से रिवर्स प्रॉक्सी है (निर्माता का, Nginx Proxy Manager, साझा Docker नेटवर्क पर Traefik), वहाँ **Caddy की ज़रूरत नहीं**: Evey अपने सुरक्षा हेडर और पूरी CSP ख़ुद भेजता है, और स्ट्रीमिंग नियम वैसे भी केवल वही प्रॉक्सी लागू कर सकता है जो क्लाइंट के सामने है: उसके पीछे दूसरा प्रॉक्सी कुछ नहीं जोड़ता। evey सर्विस को उस प्रॉक्सी के नेटवर्क पर रखें और `evey-server:9091` को लक्ष्य बनाएँ (कंटेनर का नाम; सर्विस का नाम `evey` भी काम करता है):

```yaml title="docker-compose.yml"
services:
  evey:
    # कोई ports नहीं: प्रॉक्सी ही अंदर आने का एकमात्र रास्ता है
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # पता चलने पर प्रॉक्सी के सबनेट तक सीमित करें
    # ...
networks:
  default:
    name: proxy-net
    external: true
```

फिर उस प्रॉक्सी में, `evey-server:9091` वाले रूट के लिए: `X-Forwarded-For` और `X-Forwarded-Proto` आगे भेजें, WebSocket सपोर्ट चालू करें, और ऊपर के स्ट्रीमिंग नियम लागू करें (स्ट्रीमिंग पाथ पर कोई बफ़रिंग नहीं, कोई रीड/राइट टाइमआउट नहीं): नीचे के Nginx, Traefik और Nginx Proxy Manager सेक्शन सटीक लाइनें दिखाते हैं।

## Nginx

[`examples/nginx/evey.conf.template`](../examples/nginx/evey.conf.template) का इस्तेमाल करें (एक तैयार compose [`examples/nginx/docker-compose.nginx.yml`](../examples/nginx/docker-compose.nginx.yml) में है)। nginx इमेज शुरू होते समय इसे `EVEY_DOMAIN` बदलकर बनाती है: प्रमाणपत्र मेल खाते फ़ाइल नामों से मौजूद होने चाहिए। यह स्ट्रीमिंग बफ़रिंग बंद करके `evey:9091` पर भेजता है, फ़्रंटएंड बनाने का कोई चरण नहीं।

## Traefik

देखें [`examples/traefik/docker-compose.traefik.yml`](../examples/traefik/docker-compose.traefik.yml):

```bash
docker compose -f docker-compose.yml -f examples/traefik/docker-compose.traefik.yml up -d
```

## Nginx Proxy Manager

evey कंटेनर को उस नेटवर्क पर होना चाहिए जिसे NPM इस्तेमाल करता है (ऊपर "पहले से चल रहे प्रॉक्सी के पीछे" देखें)। फिर, **Edit Proxy Host**:

1. **Details**: Scheme `http`, Forward Hostname `evey-server`, Forward Port `9091`। **Websockets Support: on** (SyncPlay)। **Cache Assets: off**: यह मीडिया सेगमेंट और प्लेलिस्ट को एक तय समाप्ति के साथ कैश कर देगा। Block Common Exploits चालू रह सकता है (अगर कोई खोज या प्लगइन अपलोड बिना कारण 403 दे तो इसे बंद करें)।
2. **SSL**: आपका प्रमाणपत्र, Force SSL, HTTP/2। यहाँ HSTS वैकल्पिक है: ऐप इसे पहले ही भेजता है।
बस इतना ही: NPM `X-Forwarded-For` / `X-Forwarded-Proto` और Host हेडर अपने आप भेजता है, और इसके डिफ़ॉल्ट टाइमआउट Evey की स्ट्रीम पर कभी नहीं आते (ऊपर स्ट्रीमिंग सेक्शन देखें)।

**वैकल्पिक**: NPM को सीधा प्लेबैक वाली फ़ाइलें अपनी डिस्क से बफ़र करने से रोकें: **Advanced** → *Custom Nginx Configuration* (`Custom Locations` टैब यह रेगुलर एक्सप्रेशन नहीं लिख सकता):

```nginx
# प्लगइन अपलोड (100 MB तक) और बैकअप पुनर्स्थापना (2 GB तक)।
client_max_body_size 2G;

# स्ट्रीमिंग: कोई बफ़रिंग नहीं, कोई टाइमआउट नहीं।
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

# लाइब्रेरी स्कैन जब तक चलते हैं अपनी प्रगति भेजते रहते हैं।
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

बाक़ी हर पाथ (वेब इंटरफ़ेस, बाक़ी API, SyncPlay WebSocket) NPM के अपने प्रॉक्सी ब्लॉक से जाता है, जिसे Details टैब कॉन्फ़िगर करता है। स्निपेट छोड़ देने पर कुछ नहीं टूटता: इससे केवल यह बदलता है कि बड़ी फ़ाइलें कहाँ बफ़र होती हैं।

## क्लाइंट के असली पते और रिमोट एक्सेस की रोक

जैसे ही आपका प्रॉक्सी क्लाइंट का असली पता भेजता है, Evey घर पर बैठे दर्शक और इंटरनेट पर बैठे दर्शक में फ़र्क़ कर सकता है। WAN बिटरेट सीमा और रिमोट एक्सेस की रोक इसी पर काम करती हैं, इसलिए दोनों सेटिंग्स साथ में ही सही काम करती हैं:

- विश्वसनीय प्रॉक्सी (पैनल या `EVEY_TRUSTED_PROXIES`) में प्रॉक्सी शामिल होना चाहिए (जब वह Docker नेटवर्क पर कंटेनर हो तो `172.16.0.0/12`; जब वह होस्ट पर चलता हो तो कुछ जोड़ने की ज़रूरत नहीं, लूपबैक पर हमेशा भरोसा होता है)। आप जो भी सेट करते हैं वह लूपबैक के आधार में **जोड़ा** जाता है, कभी उसकी जगह नहीं लेता, इसलिए Docker वाला मान चुपचाप रोक को असली क्लाइंट पते देखने से नहीं रोक सकता।
- शृंखला में हर प्रॉक्सी को `X-Forwarded-For` बनाए रखना चाहिए। Caddy में यह `servers { trusted_proxies static private_ranges }` है; nginx `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;` इस्तेमाल करता है।

**इसे चालू करने से बदल जाता है कि कौन रिमोट माना जाता है।** इससे पहले हर अनुरोध प्रॉक्सी के अपने निजी पते से आता दिखता था, इसलिए हर कोई LAN माना जाता था। इसके बाद असली रिमोट क्लाइंट सही पहचाने जाते हैं, और अगर रिमोट एक्सेस बंद है तो उन्हें हर रूट पर मना कर दिया जाता है (`403 {"code":"remote_access_disabled"}`), भले ही ऐप की कोई सेटिंग नहीं बदली।

Evey इससे बचाव करता है: जब कोई सार्वजनिक URL कॉन्फ़िगर हो और कोई स्पष्ट चुनाव न किया गया हो तो रिमोट एक्सेस डिफ़ॉल्ट रूप से चालू रहता है, हर शुरुआत पर स्थिति लॉग होती है, हर मना करना क्लाइंट IP के साथ लॉग होता है, `evey admin doctor` "प्रकाशित पर मना कर रहा" पर साफ़ विफल होता है, और एडमिन पैनल एक चेतावनी दिखाता है। अगर आप प्रॉक्सी भरोसे की सेटिंग्स बदलते हैं, तो बाद में `evey admin doctor` जाँचें।
