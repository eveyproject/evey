---
source_sha: bba2d9d0a70621281091516a01462cd4178130e57b6654f467dfe62130069342
reviewed: false
---
# OIDC / SSO सेटअप

Evey OpenID Connect के ज़रिए सिंगल साइन-ऑन का समर्थन करता है। उपयोगकर्ता Google, Keycloak, Authentik, Authelia या किसी भी OIDC-अनुरूप प्रदाता से लॉग इन कर सकते हैं।

## कॉन्फ़िगरेशन

```env title=".env"
EVEY_OIDC_ENABLED=true
EVEY_OIDC_ISSUER=https://auth.example.com/realms/main
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=your-client-secret
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=example.com,company.org
```

## प्रदाता सेटअप

### 1. Evey को OIDC क्लाइंट के रूप में पंजीकृत करें

अपने पहचान प्रदाता में एक नया क्लाइंट या एप्लिकेशन बनाएँ:

| फ़ील्ड | मान |
|-------|-------|
| Client ID | `evey` |
| Client Secret | एक बनाएँ |
| Redirect URI | `https://<आपका पता>/api/v1/auth/oidc/callback` |
| Grant Type | Authorization Code |
| Scopes | `openid email profile` |

### 2. Evey कॉन्फ़िगर करें

ऊपर दिए गए एनवायरनमेंट वेरिएबल अपने `.env` में सेट करें। `EVEY_OIDC_ISSUER` आपके OIDC प्रदाता के डिस्कवरी एंडपॉइंट का बेस URL होना चाहिए (`/.well-known/openid-configuration` के बिना)।

### 3. रीस्टार्ट करें

```bash
docker compose restart evey
```

लॉगिन पेज पर एक "SSO से साइन इन करें" बटन दिखेगा।

## यह कैसे काम करता है

1. उपयोगकर्ता लॉगिन पेज पर "SSO से साइन इन करें" पर क्लिक करता है
2. Evey OIDC प्रदाता पर रीडायरेक्ट करता है
3. उपयोगकर्ता प्रदाता के साथ प्रमाणीकरण करता है
4. प्रदाता एक ऑथराइज़ेशन कोड के साथ Evey पर वापस रीडायरेक्ट करता है
5. Evey कोड को टोकन से बदलता है और उपयोगकर्ता खाता बनाता या अपडेट करता है

## डोमेन प्रतिबंध

`EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` सीमित करता है कि कौन-से ई-मेल डोमेन पंजीकरण कर सकते हैं। सभी डोमेन की अनुमति देने के लिए ख़ाली छोड़ें।

## स्वचालित उपयोगकर्ता निर्माण

जो उपयोगकर्ता पहली बार OIDC से लॉग इन करते हैं वे अपने आप इनके साथ बनाए जाते हैं:
- `preferred_username` या ई-मेल के उपसर्ग से बना उपयोगकर्ता नाम
- OIDC टोकन से ई-मेल
- गैर-एडमिन भूमिका
- एक डिफ़ॉल्ट प्रोफ़ाइल

OIDC उपयोगकर्ता स्थानीय पासवर्ड सेट नहीं कर सकते: वे हमेशा प्रदाता के ज़रिए प्रमाणीकरण करते हैं।
