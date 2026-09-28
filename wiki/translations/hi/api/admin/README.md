---
source_sha: e314c511a93178cdc35b47b8bcd886cb60fd2ea303b7fc7d6f6c3c9ca1f9b6d0
---
# व्यवस्थापन API

प्रशासनिक endpoints, सिर्फ़ व्यवस्थापक अधिकार वाले उपयोगकर्ताओं के लिए।

## विनिर्देश

### सिस्टम
| विनिर्देश | विवरण |
|------|-------------|
| [admin_system.swagger.json](admin_system.swagger.json) | डैशबोर्ड, सिस्टम जानकारी, सेटअप विज़ार्ड |
| [server_config.swagger.json](server_config.swagger.json) | सर्वर कॉन्फ़िगरेशन का key/value भंडार |
| [remote_access.swagger.json](remote_access.swagger.json) | रिमोट एक्सेस और नेटवर्क सेटिंग्स |
| [backup.swagger.json](backup.swagger.json) | डेटाबेस बैकअप और पुनर्स्थापना |
| [activity.swagger.json](activity.swagger.json) | प्लेबैक इतिहास, लॉगिन इतिहास, सर्वर मेट्रिक्स |
| [admin_session.swagger.json](admin_session.swagger.json) | ऑडिट लॉग |
| [admin_task.swagger.json](admin_task.swagger.json) | निर्धारित पृष्ठभूमि कार्य |

### मीडिया और ट्रांसकोडिंग
| विनिर्देश | विवरण |
|------|-------------|
| [admin_media.swagger.json](admin_media.swagger.json) | मीडिया आइटम हटाना, मेटाडेटा रिफ़्रेश करना, trickplay बनाना |
| [metadata.swagger.json](metadata.swagger.json) | मेटाडेटा प्रदाता, खोज, ओवरराइड |
| [transcoding.swagger.json](transcoding.swagger.json) | ट्रांसकोड प्रोफ़ाइल, हार्डवेयर पहचान, सक्रिय काम |
| [transcode_cache.swagger.json](transcode_cache.swagger.json) | ट्रांसकोड कैश का प्रबंधन |
| [mkv.swagger.json](mkv.swagger.json) | MKV कंटेनर औज़ार (ट्रैक, अध्याय, mux/demux) |

### उपयोगकर्ता और नीतियाँ
| विनिर्देश | विवरण |
|------|-------------|
| [library_access.swagger.json](library_access.swagger.json) | हर उपयोगकर्ता के लिए लाइब्रेरी पहुँच नियंत्रण |
| [parental.swagger.json](parental.swagger.json) | अभिभावक नियंत्रण और PIN |
| [playback_settings.swagger.json](playback_settings.swagger.json) | वैश्विक प्लेबैक सेटिंग्स |

## प्रमाणीकरण

हर endpoint को एक मान्य व्यवस्थापक JWT टोकन चाहिए।
