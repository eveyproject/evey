---
source_sha: 36caf5ec515d397a772da003df3906205b4127b77b0884c937aea9f89605a500
reviewed: true
---
# Fernzugriff und HTTPS { #remote-access-and-https }

Zu Hause ist Evey unter einer Adresse wie `http://192.168.1.20:9091`
erreichbar. Um von unterwegs zu schauen, brauchen Sie zwei weitere Dinge:

- **Eine öffentliche Adresse**, zum Beispiel `https://evey.example.org`: einen
  Domainnamen, der zu Ihnen nach Hause führt.
- **HTTPS**, damit Passwörter und Videos verschlüsselt durch das Internet
  gehen. Evey macht HTTPS nicht selbst. Das übernimmt ein **Reverse Proxy**:
  ein kleines Programm vor Evey, das die sichere Verbindung herstellt und jede
  Anfrage an Evey weiterreicht.

Diese Seite zeigt, wie Sie den Server veröffentlichen, und danach, was jede
Einstellung bewirkt.

## Den Server in drei Schritten veröffentlichen { #publish-the-server-in-three-steps }

1. **Richten Sie einen Reverse Proxy ein**, der HTTPS übernimmt und alles an
   Eveys Port `9091` weiterleitet. Sie können die Caddy-Einrichtung nehmen,
   die mit Evey kommt, oder einen Proxy, den Sie schon betreiben. Fertige
   Einrichtungen: [Reverse-Proxy-Konfigurationen](../reverse-proxy.md).
2. **Teilen Sie es Evey mit**, unter **Verwaltung → Fernzugriff**:
    - **Externe URL**: Ihre öffentliche `https://`-Adresse.
    - **Vertrauenswürdige Proxys (IP oder CIDR, kommagetrennt)**: nur wenn der
      Proxy in einem anderen Container läuft. Tragen Sie dort sein Netz ein,
      zum Beispiel `172.16.0.0/12` für die Standardnetze von Docker.
3. **Das ist alles.** Sobald der Server eine öffentliche Adresse hat, öffnet
   er den Fernzugriff von selbst.

Zur Kontrolle führen Sie `docker exec evey-server evey admin doctor` aus. Der
Befehl zeigt die verwendeten vertrauenswürdigen Proxys und warnt, wenn ein
veröffentlichter Server Leute von außen abweist.

## Der Fernzugriff ergibt sich aus der öffentlichen Adresse { #remote-access-is-decided-from-the-public-address }

Solange Sie nicht selbst entscheiden, entscheidet Evey beim Start:

- **Der Server hat eine öffentliche Adresse** (**Externe URL** oder die
  Variablen `EVEY_SERVER_BASE_URL` oder `EVEY_REMOTE_ACCESS_EXTERNAL_URL`):
  Der Fernzugriff ist **an**.
- **Er hat keine**: Der Fernzugriff ist **aus**. Nur Geräte in Ihrem Heimnetz
  können sich verbinden.

Sobald Sie **Fernzugriff aktivieren** selbst umschalten, behält Evey Ihre Wahl
und entscheidet nicht mehr für Sie. Ein veröffentlichter Server kann also auf
Wunsch nur zu Hause erreichbar bleiben.

Der Fernzugriff entscheidet nur, wer den Server erreichen kann. Er ist für
sich allein kein Schutz: Jede Seite verlangt trotzdem eine Anmeldung.

**Ist der Fernzugriff aus**, wird jemand außerhalb Ihres Zuhauses auf jeder
Seite abgewiesen (ein Fehler `403`). Die Apps zeigen dann **Fernzugriff
deaktiviert** statt eines unklaren Fehlers. Ist der Server aus dem Internet
erreichbar, während der Fernzugriff aus ist, warnt Evey Sie an mehreren
Stellen:

- **Verwaltung → Fernzugriff** zeigt **Alle entfernten Nutzer sind
  ausgesperrt**;
- das Serverprotokoll meldet es beim Start und protokolliert danach jede
  Abweisung mit der Adresse, von der sie kam;
- `evey admin doctor` meldet es als Fehler.

## Einstellungen { #settings }

Alle unter **Verwaltung → Fernzugriff**. Jede Einstellung gilt ohne Neustart:
Netzwerkeinstellungen ab der nächsten Anfrage, die Bitrate ab der nächsten
Wiedergabe.

<figure class="evey-shot" markdown="span">
  ![Verwaltung → Fernzugriff mit einer HTTPS-Adresse als Externe URL und einem vertrauenswürdigen Proxy](../images/admin-remote-access.webp){ loading=lazy }
  <figcaption>Ein Server, veröffentlicht unter einer <code>https://</code>-Adresse hinter einem Proxy im Docker-Netz (Oberfläche auf Englisch).</figcaption>
</figure>

| Einstellung | Was sie bewirkt |
| --- | --- |
| **Fernzugriff aktivieren** | An: Leute außerhalb Ihres Heimnetzes können sich wie alle anderen verbinden. Aus: Sie werden abgewiesen (`403`). |
| **Externe URL** | Ihre öffentliche Adresse. Links in E-Mails, Single Sign-on und das Streamen auf andere Geräte (Casting) nutzen sie. Eine `https://`-Adresse schaltet außerdem den HTTPS-Modus ein (siehe unten). |
| **Vertrauenswürdige Proxys (IP oder CIDR, kommagetrennt)** | Die Reverse Proxys, deren Angabe zum tatsächlichen Besucher Evey übernimmt. Der Rechner selbst (Loopback) ist immer vertrauenswürdig. Leer: Die Liste kommt aus `EVEY_TRUSTED_PROXIES`. |
| **LAN-Netzwerke (CIDR, kommagetrennt)** | Die Adressbereiche, die Evey als „zu Hause“ behandelt. Private Bereiche sind standardmäßig eingetragen. Nutzen Sie ein VPN (Tailscale, WireGuard) und sollen dessen Nutzer als zu Hause gelten, tragen Sie seinen Bereich hier ein. |
| **Max. Remote-Bitrate (Kbps, 0 = unbegrenzt)** | Eine Qualitätsgrenze für Leute außerhalb Ihres Heimnetzes. Eine private Adresse wird nie begrenzt. Legt **Verwaltung → Transkodierung** ebenfalls eine Grenze für entfernte Zuschauer fest, gilt die niedrigere. Siehe [Wie über die Wiedergabe entschieden wird](../playback/how-playback-is-decided.md#limits-on-the-session). |

## Was Evey über den Proxy wissen muss { #what-evey-needs-to-know-about-the-proxy }

Evey antwortet mit einfachem HTTP auf Port 9091. Der Reverse Proxy übernimmt
HTTPS. Evey muss zwei Dinge über ihn wissen, beide unter **Verwaltung →
Fernzugriff**:

- **Externe URL** mit `https://`. Daran erkennt Evey, dass ein Proxy HTTPS
  übernimmt. Evey schützt dann seine Anmelde-Cookies (Flag `Secure`), sagt
  Browsern, immer HTTPS zu benutzen (HSTS), und übernimmt den Header
  `X-Forwarded-Proto` eines vertrauenswürdigen Proxys.
- **Die vertrauenswürdigen Proxys**, wenn der Proxy nicht auf demselben
  Rechner wie Evey läuft. Für einen Proxy in einem anderen Container gehört
  hier sein Netz hin, zum Beispiel `172.16.0.0/12` für die Standardnetze von
  Docker.

!!! warning "Ein Bereich, der alle Adressen abdeckt, wird abgelehnt"
    `0.0.0.0/0` oder `::/0` als vertrauenswürdiger Proxy würde jedem Besucher
    erlauben, eine beliebige Herkunftsadresse vorzutäuschen. Der Verwaltungsbereich
    und `evey admin doctor` lehnen das beide ab. Tragen Sie stattdessen die
    eigene Adresse oder das Netz des Proxys ein.

### Stattdessen in der Umgebung einstellen { #setting-it-in-the-environment-instead }

Die Variablen `EVEY_SERVER_BASE_URL` und `EVEY_TRUSTED_PROXIES` tun dasselbe
aus der Compose-Datei heraus. Das passt zu einer Einrichtung, die ganz in
Dateien geschrieben ist, oder zu einem Server, der schon beim allerersten
Start über HTTPS antworten muss.

Ist beides gesetzt, hat der Verwaltungsbereich Vorrang. Eine `https://`-Adresse
als **Externe URL** schaltet den HTTPS-Modus ein, auch wenn die Basis-URL
einfaches HTTP ist, und eine gespeicherte Proxy-Liste ersetzt die Variable.
Leeren Sie das Feld, um die Entscheidung wieder der Variable zu überlassen.
Siehe [Server und Netzwerk](../environment/server.md#remote-access).

## Die echte Adresse des Clients { #the-real-client-address }

Damit der Fernzugriff und die Qualitätsgrenze für unterwegs funktionieren,
muss Evey die echte Adresse jedes Besuchers sehen. Zwei Einstellungen arbeiten
zusammen:

1. Die vertrauenswürdigen Proxys legen fest, von wem Evey die Angabe zur
   echten Adresse des Besuchers übernimmt.
2. Die LAN-Netzwerke entscheiden dann, ob diese Adresse „zu Hause“ oder
   „unterwegs“ ist.

Gibt der Proxy die echte Adresse nicht weiter, oder vertraut Evey ihm nicht,
scheint jede Anfrage von der eigenen privaten Adresse des Proxys zu kommen.
Dann gelten alle als zu Hause: Die Einstellung für den Fernzugriff hat keine
Wirkung, und keine Grenze für unterwegs greift.

!!! note "Wer den Proxy korrigiert, ändert, wer als unterwegs gilt"
    War der Fernzugriff aus, werden Leute von außen abgewiesen, sobald der
    Proxy ihre echte Adresse weitergibt. Führen Sie nach jeder Änderung an
    den Proxy-Einstellungen `evey admin doctor` aus: Der Befehl zeigt die
    verwendete Liste und woher sie kommt, warnt vor einem öffentlichen
    Bereich und schlägt bei einem Bereich fehl, der alle Adressen abdeckt.

### Docker und IPv6 { #docker-and-ipv6 }

Docker veröffentlicht einen Port über IPv4 mit einer Firewall-Regel, die die
Adresse des Besuchers beibehält. Über IPv6 nutzt Docker stattdessen ein Relay,
wenn das Netzwerk des Containers kein IPv6 hat: Jeder IPv6-Besucher erreicht
Evey dann vom Gateway des Netzwerks aus, einer privaten Adresse, ganz gleich,
wo er wirklich ist. Diese Adresse gehört zu den lokalen Netzwerken, sodass
diese Besucher selbst bei deaktiviertem Fernzugriff als lokal gelten. Evey
kann sie nicht vom Host selbst unterscheiden, der einen auf `127.0.0.1`
veröffentlichten Port über dasselbe Gateway erreicht: Es lässt eine solche
Anfrage durch, schreibt eine Warnung in sein Protokoll und zeigt eine unter
**Fernzugriff** an. Auf einem Server, der nur zu Hause genutzt oder vom Host
selbst aus geöffnet wird, kann die Warnung ignoriert werden.

Die Compose-Dateien dieser Dokumentation veröffentlichen den Port nur über
IPv4 (`0.0.0.0:9091:9091`), sodass das Relay nie startet. Um auch IPv6 zu
bedienen, geben Sie dem Docker-Netzwerk IPv6: Docker leitet IPv6 dann wie
IPv4 weiter, einschließlich der echten Adresse.

```yaml
services:
  evey:
    ports:
      - "9091:9091"
    networks: [evey]

networks:
  evey:
    enable_ipv6: true
    ipam:
      config:
        - subnet: fd00:e7e7::/64
```

Ältere Docker-Versionen müssen außerdem `ip6tables` in der
Daemon-Konfiguration aktivieren: siehe die IPv6-Dokumentation von Docker.

Ein Reverse-Proxy, der auf dem Host außerhalb von Docker läuft, erreicht Evey
über dasselbe Gateway. Veröffentlichen Sie den Port von Evey auf `127.0.0.1`,
damit auf diesem Weg nichts anderes ankommen kann, und tragen Sie das Gateway
bei den vertrauenswürdigen Proxys ein, damit Evey die Besucheradresse sieht,
die der Proxy weitergibt.

<!-- sources
inference and explicit choice: docs/adr/0026-remote-access-inferred-from-a-published-url.md ; internal/config/remote_access_resolve.go - ResolveRemoteAccess, IsPubliclyReachableURL
gate, live switch and LAN list: cmd/evey/gateway.go - networkMiddleware ; cmd/evey/network_middleware_test.go - TestNetworkMiddleware_LANListChangeAppliesWithoutRestart
remote cap live: internal/adapters/driving/grpcapi/playback_remote_bitrate_test.go
labels: i18n admin.remoteAccess.* (enableRemoteAccess, externalUrl, trustedProxies, lanNetworks, maxRemoteBitrate, lockoutRiskTitle)
proxy settings: docs/reverse-proxy.md
https mode and trusted proxies live: internal/config/network_live.go - EffectiveHTTPS, EffectiveTrustedProxies ; internal/adapters/driving/http/access_handler_live_test.go
docker gateway relay: internal/pkg/network/relay.go - DefaultGateways, IsRelay ; internal/pkg/network/relay_test.go - TestDetectIP_RelayKeepsTheLANListVerdict ; cmd/evey/network_middleware_test.go - TestNetworkMiddleware_GatewayRelayIsReportedNotRefused ; i18n admin.remoteAccess.relayedClientsTitle
catch-all range refused: internal/config/trusted_proxies.go - IsCatchAllProxyRange ; internal/config/network_live.go
doctor check: cmd/evey/admin_ops.go - checkTrustedProxies
exact field labels: i18n admin.remoteAccess.trustedProxies, lanNetworks, maxRemoteBitrate, lockoutRiskTitle ; apps message: i18n auth.remoteAccess.title
-->
