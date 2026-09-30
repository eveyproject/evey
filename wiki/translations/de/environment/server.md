---
source_sha: d3bbf5c5d256e29ed7bd2108d72166e0906938ea2e32684ab5496cdb6bb7755e
reviewed: false
---
# Server und Netzwerk

Die Adresse, unter der der Server veröffentlicht ist, wem er vertraut, wer ihn
von außen erreichen darf und seine Grenzen für Anfragen.

## Adresse und Ports

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_SERVER_BASE_URL` | `http://localhost:9091` | Die öffentliche Adresse. Eine `https://`-Adresse schaltet `Secure`-Cookies, HSTS und das Vertrauen in `X-Forwarded-Proto` ein. Links in E-Mails, Weiterleitungen des Single Sign-on und Cast-URLs werden daraus gebaut, außer **Admin → Fernzugriff → Externe URL** ist gesetzt, was Vorrang hat. Eine `https://`-Externe-URL schaltet HTTPS auch dann ein, wenn diese hier einfaches HTTP ist. |
| `EVEY_SERVER_PORT` | `9090` | Der interne gRPC-Port. HTTP-API und Weboberfläche lauschen auf diesem Port plus eins (`9091`). |
| `EVEY_GRPC_BIND` | `127.0.0.1` | Die Adresse, auf der der gRPC-Port lauscht. Die Sperre für den Fernzugriff deckt gRPC nicht ab, eine Erweiterung (`0.0.0.0`) protokolliert daher eine Warnung und macht die Firewall zu Ihrer Aufgabe. |
| `EVEY_SERVER_NAME` | `Evey` | Der Name auf der Anmeldeseite, in der Serverliste der Apps und in E-Mails. **Admin → Serverkonfiguration** übersteuert ihn. |

## Reverse Proxy

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_TRUSTED_PROXIES` | nur Loopback | Netzwerke, deren weitergeleiteter Client-Adresse geglaubt wird, kommagetrennt (für einen Proxy in einem Container: `172.16.0.0/12`, eingeengt, sobald bekannt). Wird zu Loopback hinzugefügt, ersetzt es nie. Eine unter **Admin → Fernzugriff → Vertrauenswürdige Proxys** gespeicherte Liste ersetzt diese sofort; das Leeren des Feldes bringt sie zurück. |
| `EVEY_REAL_IP_HEADER` | `X-Forwarded-For` | Der Header, in den ein vertrauenswürdiger Proxy die Client-Adresse schreibt. |
| `EVEY_FORWARDED_PROTO` | abgeleitet | Veraltet: HTTPS wird aus dem Schema von `EVEY_SERVER_BASE_URL` abgeleitet. Als Übersteuerung weiterhin akzeptiert, mit einer Warnung. |

## Fernzugriff

Diese belegen den ersten Start vor; **Admin → Fernzugriff** speichert danach
eigene Werte und wendet sie ohne Neustart an.

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_REMOTE_ACCESS_ENABLED` | nicht gesetzt | Nicht gesetzt: beim Start abgeleitet, an, wenn `EVEY_SERVER_BASE_URL` oder `EVEY_REMOTE_ACCESS_EXTERNAL_URL` öffentlich ist. `true` oder `false`: Ihre ausdrückliche Wahl, nie übersteuert. |
| `EVEY_REMOTE_ACCESS_EXTERNAL_URL` | leer | Die öffentliche Adresse, wenn sie von `EVEY_SERVER_BASE_URL` abweicht. Eine `https://`-Adresse schaltet den HTTPS-Modus ein, wie die Basis-URL. |
| `EVEY_REMOTE_ACCESS_LAN_NETWORKS` | `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16` | Netzwerke, die als Zuhause gelten. Fügen Sie einen VPN- oder CGNAT-Bereich (`100.64.0.0/10`) hinzu, dessen Clients als lokal zählen sollen. |
| `EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS` | `0` | Eine Bitrate-Obergrenze für Clients außerhalb des Zuhauses, in kbit/s; `0` = keine. Eine private Adresse wird nie begrenzt. |

Wie sich die Sperre verhält: [Fernzugriff und HTTPS](../administration/remote-access.md).

## Anfragegrenzen

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_RATELIMIT_LOGIN_MAX` | `10` | Anmeldeversuche pro Adresse und Minute. **Admin → Serverkonfiguration → Servereinstellungen** übersteuert ihn. |
| `EVEY_RATELIMIT_GLOBAL_MAX` | `300` | Anfragen pro Adresse und Minute an die API. Am selben Ort übersteuerbar. |

## Container-Benutzer

Vom Entrypoint gelesen, nicht vom Server.

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `PUID`, `PGID` | Besitzer der `/media`-Einbindungen | Benutzer und Gruppe, unter denen der Server läuft. `1001` wird verweigert (für Plugins reserviert). Einzelheiten in der [Konfigurationsreferenz](../configuration.md#file-ownership-puid-and-pgid). |

## Öffentliche Demo

| Variable | Standard | Wirkung |
| --- | --- | --- |
| `EVEY_DEMO` | `false` | Betreibt den Server als öffentliche Demo: Schreibzugriffe der Verwaltung sind gesperrt, keine E-Mails, kein Push, keine ausgehenden Webhooks. Nicht für eine echte Installation. |
