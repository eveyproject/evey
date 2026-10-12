---
source_sha: e3a9611908da29d559070f9745961b1f31fad6154f252e1304481c5e371f2c0c
reviewed: true
---
# खाते और साइन-इन

सेशन और सिंगल साइन-ऑन के पीछे के वेरिएबल: एक साइन-इन कितनी देर चलता है,
साइन करने वाली कुंजी कहाँ रहती है, और OIDC प्रदाता। सिंगल साइन-ऑन आम तौर पर
पैनल में सेट किया जाता है, जैसा
[सिंगल साइन-ऑन (OIDC)](../oidc.md) में बताया गया है; यहाँ के वेरिएबल उसका
बैकअप हैं।

## सेशन

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_JWT_PRIVATE_KEY_PATH` | `/data/jwt.key` | वह कुंजी जो access token पर साइन करती है, पहली बार शुरू होने पर बनती है। अगर यह खो जाए, तो अगली बार शुरू होने पर नई बनती है और ऐप अपने token नए कर लेते हैं: कोई साइन आउट नहीं होता। | कोई नहीं |
| `EVEY_JWT_ACCESS_TTL` | `15m` | एक access token की उम्र। ऐप इसे चुपचाप नया कर लेते हैं। | कोई नहीं |
| `EVEY_JWT_REFRESH_TTL` | `168h` (7 दिन) | एक साइन-इन सेशन की उम्र: इतनी देर कोई ऐप इस्तेमाल न करें, तो फिर से साइन इन करना होगा। | कोई नहीं |
| `EVEY_JWT_REFRESH_LONG_TTL` | `4320h` (180 दिन) | उस सेशन की उम्र जिसे कोई ऐप बनाए रखने को कहता है, जैसे क्विक कनेक्ट से साइन इन किया हुआ कोई TV। | कोई नहीं |

## सिंगल साइन-ऑन (OIDC)

**एडमिन → सर्वर कॉन्फ़िग** के **सिंगल साइन-ऑन (SSO)** कार्ड में सहेजा गया हर
फ़ील्ड अपने वेरिएबल की जगह लेता है, बिना रीस्टार्ट के।

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_OIDC_ENABLED` | `false` | सिंगल साइन-ऑन चालू करता है। | सर्वर कॉन्फ़िग → सिंगल साइन-ऑन (SSO), तुरंत लागू |
| `EVEY_OIDC_ISSUER` | खाली | प्रदाता का issuer URL, जैसे `https://auth.example.com/realms/main`। | वही कार्ड, तुरंत लागू |
| `EVEY_OIDC_CLIENT_ID` | खाली | प्रदाता के पास Evey के लिए रजिस्टर किया गया client। | वही कार्ड, तुरंत लागू |
| `EVEY_OIDC_CLIENT_SECRET` | खाली | उसका secret। इसके बिना चालू किया जाए, तो सर्वर शुरू होते समय चेतावनी देता है। | वही कार्ड, तुरंत लागू |
| `EVEY_OIDC_REDIRECT_URL` | खाली | प्रदाता को भेजा जाने वाला callback पता: `https://<your address>/api/v1/auth/oidc/callback`। खाली: सर्वर के सार्वजनिक पते (**एडमिन → रिमोट एक्सेस**) से बनता है। | कोई नहीं |
| `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` | खाली (सभी) | वे ई-मेल डोमेन जिन्हें साइन इन की अनुमति है, कॉमा से अलग। प्रदाता ने पते की पुष्टि की होनी चाहिए। | वही कार्ड, तुरंत लागू |
| `EVEY_OIDC_ALLOW_SIGNUP` | खाली | `true` या `false`: नए SSO उपयोगकर्ताओं के लिए खाते बनाएँ। खाली: सिर्फ़ तब चालू जब ई-मेल डोमेन सेट हों। | वही कार्ड, तुरंत लागू |

<!-- sources
internal/config/config.go - EVEY_JWT_*, EVEY_OIDC_* ; WarnValidate (OIDC enabled without secret)
key regeneration: cmd/evey/wire_infra.go - auth.LoadOrGenerateRSAKey ; refresh tokens in DB: internal/adapters/driven/postgres/refresh_token_repo.go
long ttl: internal/adapters/driving/grpcapi/user_handler.go - refreshTTLOverride ; internal/adapters/driving/grpcapi/interceptors/client_info.go (x-persistent-session)
redirect: internal/app/user/oidc_service.go (oauth2 RedirectURL as given) ; panel over env: internal/app/user/oidc_settings.go ResolveOIDCConfig (OIDCKey*)
panel: i18n admin.sso.title
-->
