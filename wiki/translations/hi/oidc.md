---
source_sha: b5ebe9aa528d6b195b210870c763195df806015fe80273fefda50e5e3927aad7
---

# OIDC / SSO की स्थापना

Evey में OpenID Connect के ज़रिए Single Sign-On चलता है। उपयोगकर्ता Google, Keycloak, Authentik, Authelia, या OIDC मानक पर चलने वाले किसी भी प्रदाता से लॉगिन कर सकते हैं।

## कॉन्फ़िगरेशन

```env
EVEY_OIDC_ENABLED=true
EVEY_OIDC_ISSUER=https://auth.example.com/realms/main
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=your-client-secret
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=example.com,company.org
```

## प्रदाता की तैयारी

### 1. Evey को OIDC क्लाइंट के रूप में दर्ज करें

अपने पहचान प्रदाता में एक नया क्लाइंट या ऐप्लिकेशन बनाएँ:

| फ़ील्ड | वैल्यू |
|-------|-------|
| Client ID | `evey` |
| Client Secret | एक बनाएँ |
| Redirect URI | `{EVEY_SERVER_BASE_URL}/api/v1/auth/oidc/callback` |
| Grant Type | Authorization Code |
| Scopes | `openid email profile` |

### 2. Evey को कॉन्फ़िगर करें

ऊपर दिए गए env वेरिएबल अपनी `.env` में सेट करें। `EVEY_OIDC_ISSUER` में आपके OIDC प्रदाता के discovery endpoint का base URL आना चाहिए (`/.well-known/openid-configuration` के बिना)।

### 3. दोबारा शुरू करें

```bash
docker compose restart evey
```

लॉगिन पृष्ठ पर "Sign in with SSO" बटन दिखने लगेगा।

## यह काम कैसे करता है

1. उपयोगकर्ता लॉगिन पृष्ठ पर "Sign in with SSO" दबाता है
2. Evey उसे OIDC प्रदाता के पास भेज देता है
3. उपयोगकर्ता प्रदाता के यहाँ अपनी पहचान साबित करता है
4. प्रदाता उसे एक authorization code के साथ वापस Evey पर भेजता है
5. Evey उस code के बदले टोकन लेता है और उपयोगकर्ता खाता बनाता या अपडेट करता है

## डोमेन की पाबंदी

`EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` तय करता है कि किन ईमेल डोमेन से पंजीकरण हो सकता है। इसे खाली छोड़ दें, तो सभी डोमेन चलेंगे।

## खाते अपने आप बनना

जो उपयोगकर्ता पहली बार OIDC से लॉगिन करते हैं, उनका खाता अपने आप बन जाता है:
- उपयोगकर्ता नाम `preferred_username` से, या ईमेल के शुरुआती हिस्से से लिया जाता है
- ईमेल OIDC टोकन से आता है
- भूमिका व्यवस्थापक की नहीं होती
- एक डिफ़ॉल्ट प्रोफ़ाइल बना दी जाती है

OIDC वाले उपयोगकर्ता अपना स्थानीय पासवर्ड नहीं रख सकते - उनकी पहचान हमेशा प्रदाता के ज़रिए ही होती है।
