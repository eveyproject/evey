---
source_sha: af82a75f9598046f40d3b3b9257d529cb681628d24fcf9fab49c54919727e7fd
---
# Reverse Proxy

Evey liefert **die API und die Weboberfläche gemeinsam** über Port **9091** aus
(unverschlüsseltes HTTP). Ein Reverse Proxy muss lediglich TLS terminieren und
alles an `evey:9091` weiterreichen: **ein eigenes Frontend, das gebaut oder
ausgeliefert werden müsste, gibt es nicht**.

Egal, welchen Proxy Sie einsetzen: Setzen Sie diese Werte, damit Evey seine
öffentliche URL kennt und den weitergereichten Headern des Proxys vertraut:

```env
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=172.16.0.0/12       # nur nötig, wenn der Proxy ein Container ist; Loopback gilt immer als vertrauenswürdig
```

An einer Basis-URL mit `https://` erkennt Evey, dass ein Proxy davor TLS
terminiert: Cookies bekommen das Attribut `Secure`, HSTS wird gesendet, und
`X-Forwarded-Proto` von einem vertrauenswürdigen Proxy wird ausgewertet. Ein
zusätzlicher Schalter ist nicht nötig (das frühere `EVEY_FORWARDED_PROTO` wird
weiterhin als Übersteuerung akzeptiert, gilt aber als veraltet).

## Streaming - was die Standardwerte eines Proxys damit anstellen

Evey läuft auch hinter einem Reverse Proxy, den Sie unangetastet lassen: Jede
langlebige Antwort sendet entweder ununterbrochen Daten (HLS-Segmente,
Direktwiedergabe, Ausgabe der Transkodierung) oder meldet sich alle 30 s beim
Client (die Ereignisströme). Ein voreingestelltes Lesezeitlimit greift also
nie. Zwei Punkte sollten Sie trotzdem kennen:

- **Pufferung.** Ein Proxy, der Antworten puffert (nginx tut das
  standardmäßig), liest eine Datei für die Direktwiedergabe erst vollständig in
  eigene Puffer und temporäre Dateien ein, bevor der Client sie bekommt.
  Funktionieren tut das, aber ein Film von mehreren GB wandert dann über die
  Festplatte des Proxy-Hosts. `proxy_buffering off` auf den Streaming-Pfaden
  verhindert das.
- **Scans.** `/api/v1/libraries/*/scan` überträgt den Fortschritt, solange der
  Scan läuft. Ein Proxy, der die Gesamtdauer einer Antwort begrenzt (selten -
  nginx und Caddy tun das nicht von sich aus), würde die Übertragung
  abschneiden. Der Scan selbst läuft serverseitig in jedem Fall weiter.

Betroffen sind diese Pfade:

```
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

Das mitgelieferte Caddy sowie die Beispiele für Nginx und Traefik weiter unten
setzen beides (keine Pufferung, kein Zeitlimit) auf diesen Pfaden. Bei einem
über die Oberfläche gesteuerten Proxy wie Nginx Proxy Manager genügen die
Standardwerte, dort ist die Feinabstimmung ein optionaler Schnipsel.

## Sicherheits-Header und CSP - lesen Sie das, wenn Ihr Proxy eigene setzt

Evey sendet eine vollständige Content-Security-Policy. Wenn Ihr Proxy sie
durchreicht, sind Sie fertig.

> Bis zum 23.07.2026 stimmte dieser Satz nicht: In der Richtlinie der Anwendung
> fehlten `media-src` und `worker-src`, deshalb fiel `blob:` auf
> `default-src 'self'` zurück und die MSE-Wiedergabe war für alle kaputt, die
> die Richtlinie durchreichten. Aufgefallen ist das niemandem, weil die
> mitgelieferte Caddy-Konfiguration den Header der Anwendung entfernt und einen
> eigenen pflegt. Wenn Sie eine ältere Version hinter Nginx Proxy Manager oder
> Traefik betreiben und Ihr Player nicht startet, ist das der Grund. Pflegen Sie
> dann die untenstehende Richtlinie im Proxy oder aktualisieren Sie.

Viele Installationen entfernen sie stattdessen
(`header_down -Content-Security-Policy` in Caddy) und pflegen eine eigene. Das
ist in Ordnung, aber dann muss **Ihre** Richtlinie diese Token enthalten, sonst
fallen Funktionen stillschweigend aus:

| Token | Direktive | Fällt ohne das Token aus |
| --- | --- | --- |
| `'unsafe-inline'` | `script-src` | Sämtliche Erweiterungsoberflächen (leeres iframe). Das Bundle einer Erweiterung wird mit dem Token des Benutzers geladen und in ein `srcdoc`-iframe eingebettet, und ein srcdoc-Dokument erbt die Richtlinie des einbettenden Dokuments. Begrenzt wird die Erweiterung derweil durch das `sandbox="allow-scripts"` des iframe (undurchsichtiger Ursprung, kein Zugriff auf DOM oder Cookies des Elterndokuments). |
| `'wasm-unsafe-eval'` | `script-src` | Die Wiedergabe von AC-3/E-AC-3 im Browser (der WebAssembly-Decoder ac3go lässt sich nicht kompilieren → der Film läuft stumm). Dieses eng gefasste Token erlaubt ausschließlich das Kompilieren von Wasm, **nicht** allgemeines `eval()`. |
| `https://www.gstatic.com` | `script-src` (und `frame-src`) | Das Sender-SDK von Google Cast wird nie geladen → niemand bekommt eine Cast-Schaltfläche. Nur darüber kann ein Browser Chromecasts im eigenen LAN des Benutzers finden. |
| `blob:` | `media-src` | Die MSE-Wiedergabe (hls.js hängt den Stream über eine blob-URL ein). |
| `blob:` | `worker-src` | hls.js weicht stillschweigend auf das Demuxen im Haupt-Thread aus (ruckeliger auf Mobilgeräten). |

Eine erprobte Richtlinie für einen Proxy, der seine eigene pflegt:

```
default-src 'self'; base-uri 'self'; object-src 'none';
img-src 'self' https: data: blob:; media-src 'self' blob:;
style-src 'self' 'unsafe-inline';
script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com;
connect-src 'self' https: wss:; worker-src 'self' blob:; font-src 'self' data:;
frame-src https://www.gstatic.com
```

`base-uri` und `object-src` fallen nicht auf `default-src` zurück. Eine
Richtlinie, die sie auslässt, lässt daher das Einschleusen von `<base href>`
sowie das Einbetten über `<object>`/`<embed>` unbeschränkt zu. Behalten Sie
beide bei, wenn Sie eine eigene Richtlinie pflegen.

Faustregeln:

- **Beim Browser darf genau ein CSP-Header ankommen.** Bei zweien (etwa dem der
  Anwendung UND dem Ihres Proxys, oder dem von Caddy UND dem Schnipsel unter
  „Advanced“ im Nginx Proxy Manager) erzwingen Browser die *Schnittmenge*: Die
  strengste Angabe gewinnt, und der Ausfall bleibt unbemerkt. Zum Prüfen:
  `curl -sI https://your.domain | grep -ci content-security-policy`
  → muss `1` ausgeben.
- **Schreiben Sie die Richtlinie in EINE Zeile.** Eine über mehrere Zeilen
  verteilte CSP in einer Proxy-Konfiguration ist ein Klassiker unter den
  stillen Verstümmelungen: Beim Zusammenfügen verschwinden die Zeichen an der
  Nahtstelle (aus `...gstatic.com; connect-src` wurde einmal
  `...gstaticonnect-src`, was das Cast-SDK abschaltete und zwei Direktiven
  verschmolz).
- `'unsafe-inline'` in `script-src` hat seinen Preis, und es ist derzeit
  unverzichtbar (Erweiterungsoberflächen, siehe Tabelle). Weglassen können Sie
  es nur gefahrlos, wenn auf der Installation keine Frontends von Erweiterungen
  laufen.

Alle mitgelieferten Beispiele lesen die Domain an **einer einzigen Stelle**:
`EVEY_DOMAIN` in Ihrer `.env`. Einmal setzen genügt, Caddy, Nginx und Traefik
greifen den Wert alle auf.

```env
EVEY_DOMAIN=media.example.com
```

## Caddy (mitgeliefert)

Im Repository liegt [`docker/Caddyfile`](../docker/Caddyfile) (das
[`docker/Dockerfile.caddy`](../docker/Dockerfile.caddy) baut daraus ein Image).
Es besorgt automatisch HTTPS für `EVEY_DOMAIN` und leitet alles an `evey:9091`
weiter, die Ausnahmen fürs Streaming sind bereits gesetzt. Außerdem pflegt es
die maßgeblichen Sicherheits-Header und die CSP aus dem vorigen Abschnitt (und
entfernt die CSP der Anwendung, damit beim Browser genau eine ankommt). Ergänzen
Sie in Ihrer Compose-Datei einen `caddy`-Dienst (siehe den Dienst `caddy` in der
[`docker-compose.yml`](../docker-compose.yml) des Repositorys).

### Hinter einem Proxy, den Sie bereits betreiben (TLS wird vorgelagert terminiert)

Auf einem NAS, auf dem bereits ein Reverse Proxy läuft (der des Herstellers,
Nginx Proxy Manager, ein Traefik in einem gemeinsamen Docker-Netzwerk), brauchen
Sie **kein Caddy**: Evey sendet eigene Sicherheits-Header und eine vollständige
CSP, und die Streaming-Regeln kann ohnehin nur der Proxy durchsetzen, der zum
Client zeigt. Ein zweiter Proxy dahinter bringt nichts. Hängen Sie den
evey-Dienst in das Netzwerk dieses Proxys und lassen Sie ihn auf
`evey-server:9091` zeigen:

```yaml
services:
  evey:
    # keine Ports: der Proxy ist der einzige Weg hinein
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # sobald bekannt, auf das Subnetz des Proxys eingrenzen
    # ...
networks:
  default:
    name: proxy-net
    external: true
```

Richten Sie danach in diesem Proxy die Route zu `evey-server:9091` so ein:
`X-Forwarded-For` und `X-Forwarded-Proto` weiterreichen, WebSocket-Unterstützung
aktivieren und die oben genannten Streaming-Regeln anwenden (keine Pufferung,
kein Lese- oder Schreibzeitlimit auf den Streaming-Pfaden). Die Abschnitte zu
Nginx, Traefik und Nginx Proxy Manager weiter unten zeigen die genauen Zeilen.

## Nginx

Verwenden Sie
[`examples/nginx/evey.conf.template`](../examples/nginx/evey.conf.template) (eine
fertige Compose-Datei liegt unter
[`examples/nginx/docker-compose.nginx.yml`](../examples/nginx/docker-compose.nginx.yml)).
Das nginx-Image erzeugt die Konfiguration beim Start und setzt dabei
`EVEY_DOMAIN` ein. Unter den passenden Dateinamen müssen Zertifikate vorliegen.
Weitergeleitet wird an `evey:9091`, die Pufferung beim Streaming ist
abgeschaltet. Ein Build-Schritt fürs Frontend entfällt.

## Traefik

Siehe [`examples/traefik/docker-compose.traefik.yml`](../examples/traefik/docker-compose.traefik.yml):

```bash
docker compose -f docker-compose.yml -f examples/traefik/docker-compose.traefik.yml up -d
```

## Nginx Proxy Manager

Der evey-Container muss in dem Netzwerk liegen, das NPM benutzt (siehe „Hinter
einem Proxy, den Sie bereits betreiben“ weiter oben). Danach unter **Edit Proxy
Host**:

1. **Details**: Scheme `http`, Forward Hostname `evey-server`, Forward Port
   `9091`. **Websockets Support: an** (SyncPlay). **Cache Assets: aus** - sonst
   würden Mediensegmente und Playlists mit einer festen Ablaufzeit
   zwischengespeichert. Block Common Exploits darf an bleiben (schalten Sie es
   ab, falls eine Suche oder der Upload einer Erweiterung grundlos mit 403
   antwortet).
2. **SSL**: Ihr Zertifikat, Force SSL, HTTP/2. HSTS ist hier optional, die
   Anwendung sendet es bereits.
Mehr ist nicht nötig: NPM reicht `X-Forwarded-For` / `X-Forwarded-Proto` und den
Host-Header von sich aus weiter, und seine voreingestellten Zeitlimits greifen
bei Eveys Streams nie (siehe den Abschnitt zum Streaming weiter oben).

**Optional** - damit NPM Dateien der Direktwiedergabe nicht über die eigene
Festplatte puffert: **Advanced** → *Custom Nginx Configuration* (der Reiter
`Custom Locations` kann den regulären Ausdruck nicht abbilden):

```nginx
# Uploads von Erweiterungen, Wiederherstellung von Sicherungen.
client_max_body_size 100M;

# Streaming - keine Pufferung, kein Zeitlimit.
location ~ ^/api/v1/media/.+/(hls|stream) {
    proxy_pass http://evey-server:9091;
    proxy_buffering off;
    proxy_request_buffering off;
    proxy_read_timeout 0;
    proxy_send_timeout 0;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}

# Bibliotheks-Scans übertragen ihren Fortschritt, solange der Scan läuft.
location ~ ^/api/v1/libraries/.+/scan {
    proxy_pass http://evey-server:9091;
    proxy_buffering off;
    proxy_read_timeout 0;
    proxy_send_timeout 0;
    proxy_http_version 1.1;
    proxy_set_header Connection "";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

Alle übrigen Pfade (die Weboberfläche, der Rest der API, der WebSocket für
SyncPlay) laufen über den eigenen Proxy-Block von NPM, den der Reiter Details
konfiguriert. Lassen Sie den Schnipsel weg, geht nichts kaputt - er ändert nur,
wo große Dateien gepuffert werden.

## Echte Client-IPs und die Sperre für den Fernzugriff

Sobald Ihr Proxy die echte Adresse des Clients weiterreicht, kann Evey einen
Zuschauer zu Hause von einem im Internet unterscheiden. Genau darauf stützen
sich die Bitratenbegrenzung für WAN und die Sperre für den Fernzugriff, deshalb
funktionieren die beiden Einstellungen nur zusammen richtig:

- `EVEY_TRUSTED_PROXIES` muss den Proxy abdecken (`172.16.0.0/12`, wenn er ein
  Container im Docker-Netzwerk ist; nichts einzutragen, wenn er auf dem Host
  läuft, denn Loopback gilt immer als vertrauenswürdig). Was Sie eintragen,
  wird zur Loopback-Grundlage HINZUGEFÜGT und ersetzt sie nie. Ein Docker-Wert
  kann der Sperre also nicht heimlich den Blick auf die echten Client-Adressen
  nehmen.
- Bei mehreren hintereinandergeschalteten Proxys muss jeder einzelne
  `X-Forwarded-For` erhalten. In Caddy geht das über
  `servers { trusted_proxies static private_ranges }`, in nginx über
  `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`.

**Wenn Sie das einschalten, ändert sich, wer als entfernt gilt.** Vorher sah
jede Anfrage so aus, als käme sie von der privaten Adresse des Proxys, also
galten alle als im LAN. Danach werden echte entfernte Clients korrekt erkannt -
und wenn der Fernzugriff abgeschaltet ist, werden sie auf jeder Route
abgewiesen (`403 {"code":"remote_access_disabled"}`). Genau diese Abfolge hat
einen ganzen Haushalt zwei Tage lang von einem funktionierenden Server
ausgesperrt: Die Proxy-Konfiguration hatte sich geändert, keine einzige
Einstellung der Anwendung, und das einzig sichtbare Symptom waren scheiternde
Clients ohne jeden Fehler auf der Serverseite.

Evey schützt inzwischen davor: Der Fernzugriff ist standardmäßig aktiviert,
sobald eine öffentliche URL konfiguriert ist, der Zustand wird bei jedem Start
protokolliert, Abweisungen werden mit der Client-IP protokolliert,
`evey admin doctor` schlägt bei „veröffentlicht, aber abweisend“ laut Alarm, und
der Administrationsbereich zeigt eine Warnung. Wenn Sie die
Vertrauenseinstellungen des Proxys ändern, prüfen Sie danach
`evey admin doctor`.
