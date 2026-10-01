---
source_sha: fff4f29a51e1c144db473e7f53efc34660d3fe41ef149263f32a283daf865c54
reviewed: false
---
# Fernzugriff und HTTPS

Den Server von außerhalb des Zuhauses zu erreichen, umfasst zwei getrennte
Dinge: eine öffentliche HTTPS-Adresse vor Evey und Eveys eigene Einstellung
zum Fernzugriff, die entscheidet, ob Clients außerhalb Ihres Netzwerks bedient
werden.

## Der Fernzugriff wird anhand der öffentlichen Adresse entschieden

Evey entscheidet beim Start über den Fernzugriff, sofern Sie ihn nicht selbst
gewählt haben:

- wurde dem Server eine öffentliche Adresse gegeben (`EVEY_SERVER_BASE_URL`
  oder `EVEY_REMOTE_ACCESS_EXTERNAL_URL`), ist der Fernzugriff **an**;
- sonst ist er **aus**, und nur Clients in Ihren lokalen Netzwerken werden
  bedient.

Sobald Sie die Einstellung unter **Admin → Fernzugriff** ändern, bleibt Ihre
Wahl erhalten und der Server leitet sie nicht mehr ab: Ein veröffentlichter
Server kann auf das LAN beschränkt bleiben, wenn Sie das wollen.

Ist der Fernzugriff aus, erhält ein Client außerhalb der lokalen Netzwerke auf
jeder Seite ein `403`, und die Apps zeigen eine Meldung, dass der Fernzugriff
deaktiviert ist, statt eines allgemeinen Fehlers. Ist der Server aus dem
Internet erreichbar, während der Fernzugriff aus ist, zeigt die Admin-Seite
**Alle entfernten Nutzer sind ausgesperrt**, der Server protokolliert das beim
Start und jede Ablehnung mit der Client-Adresse, und `evey admin doctor`
meldet es als Fehler.

Der Fernzugriff ist eine Entscheidung darüber, wer den Server erreichen darf,
kein Schutz: Jede Seite verlangt weiterhin eine Anmeldung.

## Einstellungen

**Admin → Fernzugriff**. Jede Einstellung gilt ohne Neustart: die
Netzwerkeinstellungen ab der nächsten Anfrage, die Bitrate ab der nächsten
Wiedergabe.

<figure class="evey-shot" markdown="span">
  ![Admin → Fernzugriff mit einer HTTPS-Externe-URL und einem vertrauenswürdigen Proxy](../images/admin-remote-access.webp){ loading=lazy }
  <figcaption>Ein Server, veröffentlicht unter einer <code>https://</code>-Adresse hinter einem Proxy im Docker-Netzwerk (Oberfläche auf Englisch).</figcaption>
</figure>

| Einstellung | Wirkung |
| --- | --- |
| **Fernzugriff aktivieren** | An: Clients außerhalb der lokalen Netzwerke werden wie alle anderen bedient. Aus: Sie erhalten ein `403`. |
| **Externe URL** | Die öffentliche Adresse. Links in E-Mails, Weiterleitungen des Single Sign-on und Cast-URLs werden daraus gebaut. Eine `https://`-Adresse schaltet außerdem den HTTPS-Modus ein (unten). |
| **Vertrauenswürdige Proxys** | Die Reverse Proxys, deren weitergeleiteter Client-Adresse Evey glaubt. Loopback ist immer vertrauenswürdig. Leer: Die Liste kommt aus `EVEY_TRUSTED_PROXIES`. |
| **LAN-Netzwerke** | Die Adressbereiche, die als Zuhause gelten. Private Bereiche sind standardmäßig eingetragen. Fügen Sie hier einen VPN-Bereich hinzu (Tailscale, WireGuard), wenn dessen Clients als Zuhause zählen sollen. |
| **Max. Remote-Bitrate** | Eine Bitrate-Obergrenze für Clients außerhalb der lokalen Netzwerke, in kbit/s (0 = keine). Eine private Adresse wird nie begrenzt. Setzt auch **Admin → Transkodierung** eine Obergrenze für entfernte Zuschauer, gilt die niedrigere der beiden. Siehe [Wie Evey die Wiedergabeart wählt](../playback/how-playback-is-decided.md#limits-on-the-session). |

## HTTPS: ein Reverse Proxy vor Evey

Evey liefert einfaches HTTP auf Port 9091 aus, API und Weboberfläche zusammen.
HTTPS ist Aufgabe eines Reverse Proxys, der alles an `evey-server:9091`
weiterleitet. Evey muss zwei Dinge über ihn wissen, beide unter **Admin →
Fernzugriff**:

- **Externe URL** mit `https://`. Daran erkennt Evey, dass ein Proxy TLS
  terminiert: Cookies erhalten das `Secure`-Flag, HSTS wird gesendet und der
  Header `X-Forwarded-Proto` eines vertrauenswürdigen Proxys wird geglaubt.
- **Vertrauenswürdige Proxys**, wenn der Proxy nicht auf derselben Maschine wie
  Evey läuft. Ein Proxy in einem anderen Container braucht hier sein Netzwerk,
  zum Beispiel `172.16.0.0/12` für Dockers Standardnetzwerke. Ein Bereich, der
  jede Adresse abdeckt (`0.0.0.0/0`, `::/0`), wird verweigert: Jeder Client
  könnte sonst die Adresse wählen, mit der er gesehen wird.

Haben Sie noch keinen Proxy, liefert Eveys Repository einen Caddy-Dienst mit, der sich selbst ein
Let's-Encrypt-Zertifikat besorgt. Fertige Konfigurationen für Caddy, Nginx,
Traefik und Nginx Proxy Manager, und was die Standardeinstellungen eines
Proxys mit dem Streaming machen: [Reverse-Proxy-Konfigurationen](../reverse-proxy.md).

### Stattdessen in der Umgebung festlegen

`EVEY_SERVER_BASE_URL` und `EVEY_TRUSTED_PROXIES` geben dieselben Antworten aus
der Compose-Datei, was zu einer vollständig als Code beschriebenen Installation
passt oder zu einem Server, der ab dem allerersten Start über HTTPS erreichbar
sein muss. Der Admin-Bereich hat Vorrang, wenn beides gesetzt ist: Eine
`https://`-Externe-URL schaltet den HTTPS-Modus ein, auch wenn die Basis-URL
einfaches HTTP ist, und eine gespeicherte Proxy-Liste ersetzt die Variable.
Wird das Feld geleert, geht die Antwort an die Variable zurück. Siehe
[Server und Netzwerk](../environment/server.md).

## Die zwei Netzwerkschritte

Vertrauenswürdige Proxys und LAN-Netzwerke sind zwei aufeinanderfolgende
Schritte, keine zwei Arten, dasselbe zu sagen. Die vertrauenswürdigen Proxys
entscheiden, wessen weitergeleiteter Adresse geglaubt wird, und damit, welche
Adresse jeder Client erhält; die LAN-Netzwerke ordnen diese Adresse dann als
lokal oder entfernt ein.

## Die echte Client-Adresse

Der Fernzugriff und die **Max. Remote-Bitrate** hängen beide davon ab, dass
Evey die echte Adresse jedes Clients sieht. Ein Proxy, der sie nicht
weiterleitet oder dem Evey nicht vertraut, lässt jede Anfrage so aussehen, als
käme sie von der privaten Adresse des Proxys: Alle zählen als Zuhause, die
Einstellung zum Fernzugriff hat keine Wirkung und keine Remote-Obergrenze
greift.

Den Proxy zu korrigieren, ändert daher, wer als entfernt zählt. War der
Fernzugriff aus, werden entfernte Clients ab diesem Moment abgelehnt. Prüfen
Sie nach einer Änderung der Proxy-Einstellungen `evey admin doctor`: Er nennt
die geltende Liste und ihre Herkunft, warnt vor einem öffentlichen Bereich und
schlägt bei einem Bereich fehl, der jede Adresse abdeckt.
