---
source_sha: 423abcebbc76e4563928da41b2a7c832f1c58ae0ac12a6c3ca307b8a59582cb1
reviewed: true
---
# बैकअप, लॉग और निगरानी

डेटाबेस बैकअप, सर्वर लॉग, इतिहास कितनी देर रखा जाता है, और बाहरी निगरानी
सिस्टम के लिए endpoint, इन सबके पीछे के वेरिएबल। रोज़ के काम वाला हिस्सा
[बैकअप और अपडेट](../backups.md) और
[निगरानी और लॉग](../operations/monitoring.md) में है।

## बैकअप

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_BACKUP_DIR` | `/data/backups` | डेटाबेस बैकअप कहाँ लिखे जाते हैं। | कोई नहीं |
| `EVEY_BACKUP_INTERVAL_HOURS` | `0` (बंद) | हर N घंटे में एक बैकअप, पैनल के शेड्यूल से अलग, जो आम तरीका है। | बैकअप और पुनर्स्थापना → **शेड्यूल** (cron expression), तुरंत लागू |
| `EVEY_BACKUP_RETENTION_DAYS` | `30` | ऊपर वाला अंतराल सेट होने पर, इससे पुराने बैकअप हटा दिए जाते हैं, सर्वर शुरू होते समय और उसके हर बार चलने के बाद। पैनल के शेड्यूल का अपना **प्रतिधारण** है। | बैकअप और पुनर्स्थापना → शेड्यूल → **प्रतिधारण**, तुरंत लागू |

## लॉग

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_LOG_LEVEL` | `info` | `debug`, `info`, `warn` या `error`। | सर्वर कॉन्फ़िग → सर्वर सेटिंग्स → **लॉग स्तर**, तुरंत लागू |
| `EVEY_LOG_DIR` | `/data/logs` | जहाँ हर दिन की लॉग फ़ाइल `evey-YYYY-MM-DD.log` लिखी जाती है। इमेज इसे सेट करती है। यही फ़ाइल असली स्रोत है: `docker logs` वही धारा दिखाता है, पर Docker उसे घुमाकर पुराना हिस्सा हटाता रहता है। | कोई नहीं |
| `EVEY_LOG_MAX_SIZE_MB` | `50` | नई फ़ाइल शुरू होने से पहले एक लॉग फ़ाइल का आकार। | कोई नहीं |
| `EVEY_LOG_RETENTION_DAYS` | `90` | लॉग फ़ाइलें कितने दिन रखी जाती हैं। | कोई नहीं |

## इतिहास कितनी देर रखा जाता है

ये अवधियाँ हर दिन की सफ़ाई में फिर से पढ़ी जाती हैं, इसलिए **एडमिन → सर्वर
कॉन्फ़िग → सर्वर सेटिंग्स** में सहेजा गया मान अगली सफ़ाई में लागू होता है।

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_PLAY_SESSION_RETENTION_DAYS` | `0` (हमेशा) | इतिहास में खत्म हो चुके प्लेबैक सेशन। | सर्वर सेटिंग्स → **प्ले सत्र इतिहास प्रतिधारण**, अगली सफ़ाई में |
| `EVEY_AUDIT_LOG_RETENTION_DAYS` | `0` (हमेशा) | एडमिन के कामों का ऑडिट लॉग। | सर्वर सेटिंग्स → **ऑडिट लॉग प्रतिधारण**, अगली सफ़ाई में |
| `EVEY_METRICS_RETENTION_DAYS` | `90` | सर्वर के संसाधनों के नमूने (प्रोसेसर, मेमोरी, बैंडविड्थ)। | सर्वर सेटिंग्स → **सर्वर मेट्रिक्स प्रतिधारण**, अगली सफ़ाई में |
| `EVEY_QOE_RETENTION_DAYS` | `30` | प्लेयर की भेजी स्ट्रीमिंग क्वालिटी की रिपोर्ट। | सर्वर सेटिंग्स → **प्लेबैक गुणवत्ता (QoE) प्रतिधारण**, अगली सफ़ाई में |
| `EVEY_METRICS_SAMPLE_INTERVAL_SEC` | `60` | संसाधनों के वे नमूने कितनी बार लिए जाते हैं। | कोई नहीं |

## बाहरी निगरानी

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_METRICS_SCRAPE_TOKEN` | खाली | एक स्थायी token जो `GET /metrics` पर एडमिन सेशन के अलावा भी माना जाता है, ताकि Prometheus या Grafana बिना लॉगिन नया किए डेटा ले सकें। खाली: सिर्फ़ एडमिन सेशन माना जाता है। | कोई नहीं |
| `EVEY_OTLP_ENDPOINT` | खाली | traces भेजने के लिए एक OpenTelemetry endpoint। खाली: कोई tracing नहीं। | कोई नहीं |
| `EVEY_TRACING_SERVICE_NAME` | `evey` | traces में सेवा का नाम। | कोई नहीं |
| `EVEY_TRACING_SAMPLE_RATIO` | `0` | trace किए जाने वाले अनुरोधों का हिस्सा, 0 से 1 (अनुपात, प्रतिशत नहीं)। | कोई नहीं |

## सिर्फ़ डेवलपमेंट के लिए

| वेरिएबल | डिफ़ॉल्ट | यह क्या करता है | पैनल |
| --- | --- | --- | --- |
| `EVEY_LOG_DEV_MODE` | `false` | डेवलपर मोड: शुरू होते समय की सुरक्षा जाँचें सिर्फ़ चेतावनी देती हैं, और प्लगइन सैंडबॉक्स लाइब्रेरी के बिना चल सकते हैं। असली सर्वर पर कभी नहीं; `EVEY_ENV=production` के साथ सर्वर शुरू होने से मना करता है। | कोई नहीं |

<!-- sources
internal/config/config.go - EVEY_BACKUP_*, EVEY_LOG_*, EVEY_*_RETENTION_DAYS, EVEY_METRICS_*, EVEY_OTLP_ENDPOINT, EVEY_TRACING_* ; ScrapeToken comment ; WarnValidate (DevMode, sample ratio)
overrides: cmd/evey/main.go - loadOverride (log.level, *_retention_days) ; internal/adapters/driving/http/settings_handler.go (log.level live via logger.SetLevel ; retention keys) ; i18n admin.serverSettings.logLevel, *RetentionDays, retentionSweepHint
backup: cmd/evey/main.go (StartScheduler env interval + DB cron scheduler both started) ; internal/app/backup/service.go - StartScheduler (returns when interval <= 0 ; CleanupOldBackups first, then each tick) ; i18n admin.backup.schedule.*
dev mode: internal/adapters/driven/pluginrunner/runner.go ; cmd/evey/wire_infra.go - refuseDevModeInProd
log dir: Dockerfile
-->
