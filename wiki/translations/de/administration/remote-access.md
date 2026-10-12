---
source_sha: b8448a636921ec7091b9ab98200600dd596b087d5b5d02b1d71230f1dbda6d3d
reviewed: true
---
# Fernzugriff und HTTPS

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

## Den Server in drei Schritten veröffentlichen

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

## Der Fernzugriff ergibt sich aus der öffentlichen Adresse

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

## Einstellungen

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

## Was Evey über den Proxy wissen muss

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

### Stattdessen in der Umgebung einstellen

Die Variablen `EVEY_SERVER_BASE_URL` und `EVEY_TRUSTED_PROXIES` tun dasselbe
aus der Compose-Datei heraus. Das passt zu einer Einrichtung, die ganz in
Dateien geschrieben ist, oder zu einem Server, der schon beim allerersten
Start über HTTPS antworten muss.

Ist beides gesetzt, hat der Verwaltungsbereich Vorrang. Eine `https://`-Adresse
als **Externe URL** schaltet den HTTPS-Modus ein, auch wenn die Basis-URL
einfaches HTTP ist, und eine gespeicherte Proxy-Liste ersetzt die Variable.
Leeren Sie das Feld, um die Entscheidung wieder der Variable zu überlassen.
Siehe [Server und Netzwerk](../environment/server.md#remote-access).

## Die echte Adresse des Clients

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

<!-- sources
inference and explicit choice: docs/adr/0026-remote-access-inferred-from-a-published-url.md ; internal/config/remote_access_resolve.go - ResolveRemoteAccess, IsPubliclyReachableURL
gate, live switch and LAN list: cmd/evey/gateway.go - networkMiddleware ; cmd/evey/network_middleware_test.go - TestNetworkMiddleware_LANListChangeAppliesWithoutRestart
remote cap live: internal/adapters/driving/grpcapi/playback_remote_bitrate_test.go
labels: i18n admin.remoteAccess.* (enableRemoteAccess, externalUrl, trustedProxies, lanNetworks, maxRemoteBitrate, lockoutRiskTitle)
proxy settings: docs/reverse-proxy.md
https mode and trusted proxies live: internal/config/network_live.go - EffectiveHTTPS, EffectiveTrustedProxies ; internal/adapters/driving/http/access_handler_live_test.go
catch-all range refused: internal/config/trusted_proxies.go - IsCatchAllProxyRange ; internal/config/network_live.go
doctor check: cmd/evey/admin_ops.go - checkTrustedProxies
exact field labels: i18n admin.remoteAccess.trustedProxies, lanNetworks, maxRemoteBitrate, lockoutRiskTitle ; apps message: i18n auth.remoteAccess.title
-->
