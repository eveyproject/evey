---
source_sha: 6efce4118fc9a8bfa56a70e2648cc5f619c415c257832c5a9ea6da0e9e98e645
reviewed: true
---
# डेटाबेस और खोज

Postgres डेटाबेस और Meilisearch इंडेक्स के वेरिएबल। डिफ़ॉल्ट रूप से इमेज
दोनों को खुद चलाती है और उनके पासवर्ड खुद बनाती है: सामान्य इंस्टॉलेशन में
इनमें से कुछ भी सेट नहीं करना पड़ता। ये उस इंस्टॉलेशन के काम आते हैं जो अपना
डेटाबेस या सर्च इंजन लाता है, या इमेज में शामिल (embedded) डेटाबेस और सर्च इंजन को ट्यून करता है; तरीके
[एडवांस्ड डिप्लॉयमेंट](../operations/advanced-deployments.md) में हैं। इनमें से
किसी का भी पैनल में कोई विकल्प नहीं है।

## डेटाबेस

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_EMBEDDED_POSTGRES` | सेट नहीं | सेट नहीं: `EVEY_DB_HOST` सेट न हो तो embedded डेटाबेस, वरना बाहरी। साफ़ तय करने के लिए `true` या `false`। इसे entrypoint पढ़ता है। | कोई नहीं |
| `EVEY_POSTGRES_DATA_DIR` | `/data/postgres` | embedded क्लस्टर कहाँ रखा जाता है। यह लोकल फ़ाइल सिस्टम पर होना चाहिए, नेटवर्क शेयर पर नहीं। इसे entrypoint पढ़ता है। | कोई नहीं |
| `EVEY_POSTGRES_IMPORT` | `/data/postgres-import.sql.gz` | gzip किया हुआ एक `pg_dump`, जो embedded क्लस्टर पहली बार बनते समय उसमें रीस्टोर किया जाता है, फिर उसका नाम `*.imported-<date>` कर दिया जाता है। अगर क्लस्टर पहले से है, तो एक संदेश के साथ इसे छोड़ दिया जाता है। इसे entrypoint पढ़ता है। | कोई नहीं |
| `EVEY_DB_HOST` | `localhost` | डेटाबेस का होस्ट। **इसे सेट करने से embedded डेटाबेस बंद हो जाता है।** | कोई नहीं |
| `EVEY_DB_PORT` | `5432` | डेटाबेस का पोर्ट। | कोई नहीं |
| `EVEY_DB_USER` | `evey` | डेटाबेस का लॉगिन। जब यह superuser हो (embedded क्लस्टर और साथ आने वाली compose फ़ाइल), तो सर्वर इसे सिर्फ़ रीस्टोर के लिए रखता है और `EVEY_DB_APP_USER` के रूप में चलता है। | कोई नहीं |
| `EVEY_DB_APP_USER` | `evey_app` | बिना विशेष अधिकार वाला रोल, जिसके रूप में सर्वर और उसके माइग्रेशन चलते हैं जब `EVEY_DB_USER` superuser हो। यह बूट पर बनता है, डेटाबेस और उसकी चीज़ें इसे सौंपी जाती हैं, और हर बूट पर इसे नया रैंडम पासवर्ड मिलता है (जो कहीं लिखा नहीं जाता)। तब कोई SQL injection होस्ट पर प्रोग्राम नहीं चला सकता और फ़ाइलें नहीं पढ़ सकता। | कोई नहीं |
| `EVEY_DB_PRIVILEGE_SPLIT` | `true` | `false` सब कुछ `EVEY_DB_USER` के रूप में चलाता है, superuser भी। अगर कोई डेटाबेस यह बँटवारा न माने, तो यह एक विकल्प है; वरना बूट रुक जाता है और इसी वेरिएबल का नाम बताता है। | कोई नहीं |
| `EVEY_DB_NAME` | `evey` | डेटाबेस का नाम। | कोई नहीं |
| `EVEY_DB_PASSWORD` | अपने-आप बनता है | पासवर्ड। बाहरी डेटाबेस के साथ, मान `evey` होने पर सर्वर शुरू होने से मना करता है, और `changeme` पर चेतावनी देता है। | कोई नहीं |
| `EVEY_DB_PASSWORD_FILE` | `/data/keys/postgres.password` | पासवर्ड वाली फ़ाइल, जो `EVEY_DB_PASSWORD` सेट न होने पर पढ़ी जाती है। इमेज इसे सेट करती है। | कोई नहीं |
| `EVEY_DB_SSLMODE` | loopback पर `disable`, वरना `prefer` | `disable`, `prefer`, `require` या `verify-full`। किसी दूर के होस्ट के साथ `prefer` पर लॉग में चेतावनी आती है: `require` या `verify-full` इस्तेमाल करें। | कोई नहीं |
| `EVEY_DB_MAX_OPEN_CONNS` | `25` | डेटाबेस से खुले कनेक्शन। हर कनेक्शन Postgres की तरफ़ मेमोरी लेता है; इसे सिर्फ़ बड़े डिप्लॉयमेंट के लिए बढ़ाएँ। | कोई नहीं |
| `EVEY_DB_MAX_IDLE_CONNS` | `10` | पूल में खाली रखे गए कनेक्शन। खुले कनेक्शन की संख्या से ज़्यादा नहीं हो सकते। | कोई नहीं |

## खोज

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_EMBEDDED_MEILI` | `true` | `false` embedded Meilisearch की जगह `EVEY_MEILI_URL` पर मौजूद Meilisearch इस्तेमाल करता है। इसे entrypoint पढ़ता है। | कोई नहीं |
| `EVEY_MEILI_URL` | `http://localhost:7700` | Meilisearch का पता। | कोई नहीं |
| `MEILI_MASTER_KEY` | अपने-आप बनता है | Meilisearch की कुंजी। बाहरी इंस्टेंस के साथ, खाली या नमूने वाली कुंजी पर सर्वर शुरू नहीं होता; 16 अक्षरों से छोटी कुंजी पर लॉग में चेतावनी आती है। | कोई नहीं |
| `MEILI_MASTER_KEY_FILE` | `/data/keys/meili.key` | कुंजी वाली फ़ाइल, जो `MEILI_MASTER_KEY` सेट न होने पर पढ़ी जाती है। इमेज इसे सेट करती है। | कोई नहीं |
| `MEILI_DATA_DIR` | `/data/meili` | embedded इंडेक्स कहाँ रखा जाता है। इसे entrypoint पढ़ता है। | कोई नहीं |
| `MEILI_MAX_INDEXING_MEMORY` | कंटेनर की मेमोरी का लगभग 20 %, कम से कम 128 MiB | इंडेक्स बनाने के लिए embedded Meilisearch जितनी मेमोरी ले सकता है। यह कंटेनर की सीमा से निकाला जाता है, मशीन की मेमोरी से नहीं, ताकि कोई स्कैन छोटे कंटेनर की पूरी मेमोरी इस्तेमाल न कर ले। कोई मान साफ़ दिया जाए, तो वही आगे भेजा जाता है। | कोई नहीं |

<!-- sources
internal/adapters/driven/postgres/approle.go - EnsureAppRole, HandOverToAppRole (approle_test.go)
cmd/evey/database.go - appLogin
internal/config/config.go - EVEY_DB_*, defaultSSLMode, envSecret (_FILE), EVEY_MEILI_URL, MEILI_MASTER_KEY ; WarnValidate (password 'evey' / 'changeme', sslmode prefer, idle > open, key length)
docker/entrypoint.sh - EVEY_EMBEDDED_POSTGRES, EVEY_EMBEDDED_MEILI, EVEY_POSTGRES_DATA_DIR, EVEY_POSTGRES_IMPORT, MEILI_DATA_DIR, MEILI_MAX_INDEXING_MEMORY
Dockerfile - EVEY_DB_PASSWORD_FILE, MEILI_MASTER_KEY_FILE
-->
