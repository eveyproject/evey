---
source_sha: f625c3059af6349cfcff4f14c85f17c7a0a8c26cfeb5772eca562bc3b66c1047
reviewed: true
---
# Server und Netzwerk { #server-and-network }

Die Variablen, die festlegen, unter welcher Adresse der Server erreichbar ist,
welchen Proxys er vertraut, wer ihn von außen erreichen darf und wie viele
Anfragen er annimmt. Die meisten setzen Sie einmal in der Compose-Datei. Was
**Verwaltung → Fernzugriff** danach speichert, hat Vorrang vor ihnen, wie in
der [Konfigurationsreferenz](../configuration.md) erklärt.

## Adresse und Ports { #address-and-ports }

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_SERVER_BASE_URL` | `http://localhost:9091` | Die öffentliche Adresse. Eine `https://`-Adresse schaltet `Secure`-Cookies, HSTS und das Vertrauen in `X-Forwarded-Proto` ein. Links in E-Mails, Weiterleitungen beim Single Sign-on und Cast-URLs werden daraus gebildet. | Fernzugriff → **Externe URL** hat Vorrang, wenn gesetzt, sofort; eine `https://`-Externe-URL schaltet auch HTTPS ein |
| `EVEY_SERVER_PORT` | `9090` | Der interne gRPC-Port. Die HTTP-API und die Weboberfläche lauschen auf diesem Port plus eins (`9091`). | keine |
| `EVEY_GRPC_BIND` | `127.0.0.1` | Die Adresse, auf der der gRPC-Port lauscht. Die Sperre für den Fernzugriff gilt nicht für gRPC. Wer sie weiter öffnet (`0.0.0.0`), bekommt eine Warnung im Protokoll und muss den Port selbst per Firewall schützen. | keine |
| `EVEY_SERVER_NAME` | `Evey` | Der Name auf der Anmeldeseite, in der Serverliste der Apps und in E-Mails. | Serverkonfiguration → Allgemein → **Servername**, sofort |

## Reverse Proxy { #reverse-proxy }

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_TRUSTED_PROXIES` | nur Loopback | Netzwerke, deren weitergegebene Client-Adresse übernommen wird, durch Kommas getrennt (für einen Proxy in einem Container: `172.16.0.0/12`, eingeengt, sobald bekannt). Kommt zu Loopback hinzu, ersetzt es nie. Ein fehlerhafter Eintrag verhindert den Start. | Fernzugriff → **Vertrauenswürdige Proxys** ersetzt diese Liste, sofort; leert man das Feld, gilt wieder die Variable |
| `EVEY_REAL_IP_HEADER` | `X-Forwarded-For` | Der Header, in den ein vertrauenswürdiger Proxy die Client-Adresse schreibt. | keine |
| `EVEY_FORWARDED_PROTO` | abgeleitet | Veraltet: HTTPS wird aus dem Schema von `EVEY_SERVER_BASE_URL` abgeleitet. Wird als Übersteuerung noch akzeptiert, mit einer Warnung. | keine |

Welche Proxy-Einstellungen für das Streaming zählen: [Reverse-Proxy-Konfigurationen](../reverse-proxy.md).

## Fernzugriff { #remote-access }

Diese Variablen liefern die Werte für den ersten Start. Danach speichert
**Verwaltung → Fernzugriff** eigene Werte und wendet sie ohne Neustart an. Wie
die Sperre entscheidet: [Fernzugriff und HTTPS](../administration/remote-access.md).

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_REMOTE_ACCESS_ENABLED` | nicht gesetzt | Nicht gesetzt: wird beim Start abgeleitet, eingeschaltet, wenn `EVEY_SERVER_BASE_URL` oder `EVEY_REMOTE_ACCESS_EXTERNAL_URL` öffentlich ist. `true` oder `false`: Ihre ausdrückliche Wahl, wird nie übersteuert. | Fernzugriff → **Fernzugriff aktivieren**, sofort |
| `EVEY_REMOTE_ACCESS_EXTERNAL_URL` | leer | Die öffentliche Adresse, wenn sie sich von `EVEY_SERVER_BASE_URL` unterscheidet. Eine `https://`-Adresse schaltet den HTTPS-Modus ein, wie die Basis-URL. | Fernzugriff → **Externe URL**, sofort |
| `EVEY_REMOTE_ACCESS_LAN_NETWORKS` | `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16` | Netzwerke, die als Zuhause gelten. Fügen Sie einen VPN- oder CGNAT-Bereich (`100.64.0.0/10`) hinzu, wenn dessen Clients als lokal zählen sollen. | Fernzugriff → **LAN-Netzwerke**, sofort |
| `EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS` | `0` | Eine Obergrenze für die Bitrate von Clients außerhalb des Zuhauses, in kbit/s; `0` = keine. Eine private Adresse wird nie begrenzt. | Fernzugriff → **Max. Remote-Bitrate**, sofort |

## Anfragegrenzen { #request-limits }

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_RATELIMIT_LOGIN_MAX` | `10` | Anmeldeversuche pro Adresse und Minute. | Serverkonfiguration → Konten & Sicherheit → **Anmelde-Ratenlimit**, sofort |
| `EVEY_RATELIMIT_GLOBAL_MAX` | `300` | Anfragen pro Adresse und Minute an die API. | Serverkonfiguration → Konten & Sicherheit → **Globales Ratenlimit**, sofort |

## Benutzer des Containers { #container-user }

Wird vom Startskript gelesen, nicht vom Server.

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `PUID`, `PGID` | Besitzer der `/media`-Mounts | Benutzer und Gruppe, unter denen der Server läuft. `1001` wird abgelehnt (für Plugins reserviert). Einzelheiten in der [Konfigurationsreferenz](../configuration.md#file-ownership-puid-and-pgid). | keine |

## Öffentliche Demo { #public-demo }

| Variable | Standard | Wirkung | Verwaltungsbereich |
| --- | --- | --- | --- |
| `EVEY_DEMO` | `false` | Startet den Server als öffentliche Demo: Änderungen in der Verwaltung sind gesperrt, keine E-Mails, kein Push, keine ausgehenden Webhooks. Nicht für eine echte Installation. | keine |

<!-- sources
internal/config/config.go - EVEY_SERVER_BASE_URL, EVEY_SERVER_PORT, EVEY_GRPC_BIND, EVEY_SERVER_NAME, EVEY_REAL_IP_HEADER, EVEY_RATELIMIT_LOGIN_MAX, EVEY_RATELIMIT_GLOBAL_MAX, EVEY_REMOTE_ACCESS_*, EVEY_DEMO, forwardedProtoFromEnv, EffectiveBaseURL
trusted proxies: internal/config/trusted_proxies.go (loopback baseline, malformed entry is an error) ; cmd/evey/admin_ops.go
remote access inference: internal/config/remote_access_resolve.go ; cmd/evey/main.go (RemoteAccessConfigKey loaded from DB)
panel: internal/adapters/driving/http/settings_handler.go (server.name, ratelimit.* live via SetLimit) ; i18n admin.remoteAccess.*, admin.serverSettings.*
puid/pgid: docker/entrypoint.sh
-->
