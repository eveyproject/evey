---
source_sha: 8392e2d20e3a5b3c62e2f39d4743e925c172cdcb73dc639a5502e1bf4c16a66d
reviewed: false
---
# प्रशासन API

प्रशासनिक एंडपॉइंट, केवल एडमिन विशेषाधिकार वाले उपयोगकर्ताओं के लिए।

## विनिर्देश

### सिस्टम
| विनिर्देश | विवरण |
|------|-------------|
| [admin_system.swagger.json](admin_system.swagger.json) | डैशबोर्ड, सिस्टम जानकारी, सेटअप विज़ार्ड |
| [server_config.swagger.json](server_config.swagger.json) | सर्वर कॉन्फ़िगरेशन की/वैल्यू स्टोर |
| [remote_access.swagger.json](remote_access.swagger.json) | रिमोट एक्सेस और नेटवर्क सेटिंग्स |
| [backup.swagger.json](backup.swagger.json) | डेटाबेस बैकअप और पुनर्स्थापना |
| [backup_schedule.swagger.json](backup_schedule.swagger.json) | शेड्यूल किए गए बैकअप: cron, प्रतिधारण, अभी चलाएँ |
| [activity.swagger.json](activity.swagger.json) | प्लेबैक इतिहास, लॉगिन इतिहास, सर्वर मेट्रिक्स |
| [admin_session.swagger.json](admin_session.swagger.json) | ऑडिट लॉग |
| [admin_task.swagger.json](admin_task.swagger.json) | शेड्यूल किए गए बैकग्राउंड काम |
| [presence.swagger.json](presence.swagger.json) | अभी कौन जुड़ा है (ऐप की धड़कन और एडमिन सूची) |

### मीडिया और ट्रांसकोडिंग
| विनिर्देश | विवरण |
|------|-------------|
| [admin_media.swagger.json](admin_media.swagger.json) | मीडिया आइटम हटाना, मेटाडेटा ताज़ा करना, trickplay बनाना |
| [metadata.swagger.json](metadata.swagger.json) | मेटाडेटा प्रदाता, खोज, ओवरराइड |
| [transcoding.swagger.json](transcoding.swagger.json) | ट्रांसकोड प्रोफ़ाइल, हार्डवेयर पहचान, सक्रिय काम |
| [transcode_cache.swagger.json](transcode_cache.swagger.json) | ट्रांसकोड कैश प्रबंधन |
| [mkv.swagger.json](mkv.swagger.json) | MKV कंटेनर उपकरण (ट्रैक, अध्याय, mux/demux) |
| [admin_intro.swagger.json](admin_intro.swagger.json) | इंट्रो समय कैश: एक एपिसोड अमान्य करना, बैकफ़िल |

### उपयोगकर्ता और नीतियाँ
| विनिर्देश | विवरण |
|------|-------------|
| [library_access.swagger.json](library_access.swagger.json) | हर उपयोगकर्ता के लिए लाइब्रेरी पहुँच नियंत्रण |
| [parental.swagger.json](parental.swagger.json) | अभिभावक नियंत्रण और PIN |
| [playback_settings.swagger.json](playback_settings.swagger.json) | वैश्विक प्लेबैक सेटिंग्स |

## प्रमाणीकरण

हर एंडपॉइंट को एडमिन चाहिए: किसी एडमिन खाते का सेशन टोकन, या एक API कुंजी, जो हमेशा किसी एडमिन की होती है (उसके scopes उसे सीमित कर सकते हैं)।
