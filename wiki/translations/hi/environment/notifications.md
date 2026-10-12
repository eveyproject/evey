---
source_sha: 7cc0fb43b6f9e14980a2332dd94e4c79bea3397ef66018840bb68f8b90a78ca2
reviewed: true
---
# ई-मेल और सूचनाएँ

बाहर जाने वाले ई-मेल (सूचनाएँ, पासवर्ड रीसेट) और ब्राउज़र push के पीछे के
वेरिएबल। ई-मेल आम तौर पर पैनल में सेट किया जाता है, जैसा
[ई-मेल और सूचनाएँ](../notifications.md) में बताया गया है; ये वेरिएबल उसका बैकअप
हैं।

## ई-मेल (SMTP)

ये वेरिएबल सर्वर शुरू होते समय पढ़े जाते हैं, और तब तक इस्तेमाल होते हैं जब
तक **एडमिन → सर्वर कॉन्फ़िग** के **ईमेल / SMTP** कार्ड में कोई होस्ट न हो।
वहाँ होस्ट सहेजते ही हर फ़ील्ड कार्ड से आता है और बिना रीस्टार्ट के लागू होता
है।

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_SMTP_HOST` | खाली | मेल सर्वर। खाली: कोई ई-मेल नहीं भेजा जाता। | सर्वर कॉन्फ़िग → ईमेल / SMTP → **SMTP होस्ट**, तुरंत लागू |
| `EVEY_SMTP_PORT` | `587` | उसका पोर्ट। | वही कार्ड → **पोर्ट**, तुरंत लागू |
| `EVEY_SMTP_USERNAME` | खाली | साइन इन के लिए खाता। | वही कार्ड → **उपयोगकर्ता नाम**, तुरंत लागू |
| `EVEY_SMTP_PASSWORD` | खाली | उसका पासवर्ड। | वही कार्ड, तुरंत लागू |
| `EVEY_SMTP_FROM` | खाली | भेजने वाले का पता, जैसे `noreply@example.com`। | वही कार्ड, तुरंत लागू |
| `EVEY_SMTP_TLS` | `starttls` | `starttls`, `ssl` या `none`। | वही कार्ड, तुरंत लागू |

## ब्राउज़र push

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `VAPID_PUBLIC_KEY`, `VAPID_PRIVATE_KEY` | अपने-आप बनती हैं | कुंजियों की वह जोड़ी जो push सूचनाओं पर साइन करती है। न हों, तो सर्वर पहली बार शुरू होने पर एक जोड़ी बनाकर डेटाबेस में रखता है; इन्हें सिर्फ़ तब सेट करें जब आप पहले से इस्तेमाल हो रही जोड़ी रखना चाहते हों, और हमेशा दोनों: सिर्फ़ एक हो तो push एक चेतावनी के साथ बंद हो जाता है। | कोई नहीं |
| `VAPID_SUBJECT` | `mailto:admin@localhost` | वह संपर्क पता जो push सेवाओं को दिखता है। | कोई नहीं |

<!-- sources
internal/config/config.go - EVEY_SMTP_*, VAPID_* ; WarnValidate (one VAPID key alone)
SMTP card wins, env fallback: internal/app/config/smtp.go ResolveSMTPSettings ; i18n admin.serverConfig.emailSmtp, smtpHost, port, smtpUsername
vapid generation: cmd/evey/wire_helpers.go - resolveVAPIDKeys (vapid.public_key / vapid.private_key)
-->
