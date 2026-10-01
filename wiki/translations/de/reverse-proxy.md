---
source_sha: 3b28483f95cf0e01f8af366abbbe130d63e2bfdb7657c49aefe090c25a5ced6f
reviewed: false
---
# Reverse Proxy

Evey liefert **sowohl die API als auch die Weboberfläche** auf Port **9091**
aus (einfaches HTTP). Ein Reverse Proxy muss nur TLS terminieren und alles an
`evey:9091` weiterleiten: Es gibt **kein separates Frontend, das gebaut oder
ausgeliefert werden müsste**.

Wo auch immer Sie einen Proxy betreiben, teilen Sie Evey seine öffentliche URL
mit und welchem Proxy er vertrauen soll. Am einfachsten geht das unter
**Admin → Fernzugriff**: eine `https://`-**Externe URL** und, wenn der Proxy
ein Container ist, sein Netzwerk unter **Vertrauenswürdige Proxys**. Beides
gilt ohne Neustart. Dieselben Antworten können aus der Umgebung kommen, für
eine vollständig als Code beschriebene Installation (der Admin-Bereich hat
Vorrang, wenn beides gesetzt ist):

```env title=".env"
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=172.16.0.0/12       # nur nötig, wenn der Proxy ein Container ist; Loopback ist immer vertrauenswürdig
```

Eine `https://`-Adresse sagt Evey, dass ein Proxy davor TLS terminiert:
Cookies erhalten das `Secure`-Flag, HSTS wird gesendet und `X-Forwarded-Proto`
eines vertrauenswürdigen Proxys wird berücksichtigt. Ein eigener Schalter ist
nicht nötig (das frühere `EVEY_FORWARDED_PROTO` wird als Übersteuerung noch
akzeptiert, ist aber veraltet).

## Streaming - was die Standardeinstellungen eines Proxys damit machen

Evey funktioniert hinter einem Reverse Proxy mit Standardeinstellungen: Jede
langlebige Antwort sendet entweder fortlaufend Bytes (HLS-Segmente,
Direktwiedergabe, Transkodierungsausgabe) oder pingt den Client alle 30 s an
(die Ereignisströme), sodass ein Standard-Lese-Timeout nie auslöst. Zwei Dinge
sind trotzdem gut zu wissen:

- **Pufferung.** Ein Proxy, der Antworten puffert (nginx tut das
  standardmäßig), liest eine Direktwiedergabe-Datei in seine eigenen Puffer und
  temporären Dateien, bevor der Client sie erhält - das funktioniert, aber ein
  mehrere GB großer Film läuft dann über die Festplatte des Proxy-Hosts.
  `proxy_buffering off` auf den Streaming-Pfaden vermeidet das.
- **Scans.** `/api/v1/libraries/*/scan` streamt den Fortschritt, solange der
  Scan läuft; ein Proxy, der die Gesamtdauer einer Antwort begrenzt (selten -
  nginx und Caddy tun es standardmäßig nicht), würde ihn abschneiden. Der Scan
  selbst läuft serverseitig in jedem Fall weiter.

Die betroffenen Pfade:

```
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

Der mitgelieferte Caddy und die Nginx- und Traefik-Beispiele unten setzen
beides (keine Pufferung, kein Timeout) auf diesen Pfaden. Bei einem über eine
Oberfläche bedienten Proxy wie Nginx Proxy Manager passen die
Standardeinstellungen, und die Feinabstimmung ist ein optionaler Schnipsel.

## Sicherheits-Header und CSP - lesen Sie das, wenn Ihr Proxy eigene setzt

Evey sendet eine vollständige Content-Security-Policy. Lässt Ihr Proxy sie
durch, sind Sie fertig.

Viele Installationen entfernen sie stattdessen (`header_down -Content-Security-Policy`
in Caddy) und setzen eine eigene - das ist in
Ordnung, aber dann muss **Ihre** Richtlinie diese Tokens behalten, sonst
brechen Funktionen stillschweigend weg:

| Token | Direktive | Was ohne ihn bricht |
| --- | --- | --- |
| `'unsafe-inline'` | `script-src` | Jede Plugin-Oberfläche (leerer iframe). Ein Plugin-Paket wird mit dem Token des Benutzers geholt und in einen `srcdoc`-iframe eingebettet, und ein srcdoc-Dokument erbt die Richtlinie der einbettenden Seite. Das `sandbox="allow-scripts"` des iframes (opaker Ursprung, kein Zugriff auf DOM oder Cookies der Elternseite) begrenzt das Plugin dabei. |
| `'wasm-unsafe-eval'` | `script-src` | AC-3/E-AC-3-Wiedergabe im Browser (der WebAssembly-Decoder ac3go kann nicht kompilieren → der Film läuft stumm). Das ist der enge Token, der nur die Wasm-Kompilierung erlaubt - NICHT allgemeines `eval()`. |
| `https://www.gstatic.com` | `script-src` (+ `frame-src`) | Das Google-Cast-Sender-SDK lädt nie → für niemanden eine Cast-Schaltfläche. Nur so kann ein Browser Chromecasts im eigenen LAN des Benutzers entdecken. |
| `blob:` | `media-src` | MSE-Wiedergabe (hls.js hängt den Stream über eine Blob-URL an). |
| `blob:` | `worker-src` | hls.js weicht stillschweigend auf Demuxing im Hauptthread aus (ruckeliger auf Mobilgeräten). |

Eine bewährte Richtlinie für einen Proxy, der seine eigene setzt:

```
default-src 'self'; base-uri 'self'; object-src 'none';
img-src 'self' https: data: blob:; media-src 'self' blob:;
style-src 'self' 'unsafe-inline';
script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com;
connect-src 'self' https: wss:; worker-src 'self' blob:; font-src 'self' data:;
frame-src https://www.gstatic.com
```

`base-uri` und `object-src` fallen nicht auf `default-src` zurück: Eine
Richtlinie, die sie weglässt, lässt die Einschleusung von `<base href>` und das
Einbetten per `<object>`/`<embed>` uneingeschränkt. Behalten Sie sie, wenn Sie
Ihre eigene Richtlinie setzen.

Eine Seite braucht eine weiter gefasste Richtlinie: `/cast-receiver.html`, die Seite,
die ein Chromecast lädt, wenn er von Evey abspielt. Der Cast-Player fügt darin
Stile, Schriften und Bilder von `www.gstatic.com` ein, und die Anwendung sendet
dieser Seite deshalb eine eigene Richtlinie. Lassen Sie auf diesem Pfad den
Header der Anwendung durch, oder wenden Sie nur auf diesen Pfad diese Richtlinie
an:

```
default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com; style-src 'self' 'unsafe-inline' https://www.gstatic.com; img-src 'self' data: https: blob:; media-src 'self' blob: https:; worker-src 'self' blob:; font-src 'self' data: https://www.gstatic.com; connect-src 'self' https: wss:; frame-src https://www.gstatic.com; base-uri 'self'; object-src 'none'
```

Faustregeln:

- **Genau ein CSP-Header darf beim Browser ankommen.** Bei zweien (etwa dem der
  Anwendung UND dem Ihres Proxys, oder dem von Caddy UND dem
  „Advanced"-Schnipsel von Nginx Proxy Manager) setzen Browser die
  *Schnittmenge* durch - die strengste gewinnt, und der Fehler bleibt stumm.
  Prüfen Sie mit `curl -sI https://your.domain | grep -ci content-security-policy`
  → muss `1` ausgeben.
- **Halten Sie die Richtlinie in EINER Zeile.** Eine über mehrere Zeilen
  verteilte CSP in einer Proxy-Konfiguration ist eine klassische stille
  Beschädigung: Beim Zusammenfügen verschwinden die Zeichen an der Nahtstelle
  (`...gstatic.com; connect-src` wurde einmal zu `...gstaticonnect-src`, was
  das Cast-SDK abschaltete und zwei Direktiven verschmolz).
- `'unsafe-inline'` in `script-src` hat echte Kosten und ist derzeit tragend
  (Plugin-Oberflächen, siehe Tabelle). Es wegzulassen ist nur auf einer
  Installation sicher, die keine Plugin-Frontends ausführt.

Jedes mitgelieferte Beispiel liest die Domain an **einer Stelle**:
`EVEY_DOMAIN` in Ihrer `.env`. Einmal setzen; Caddy, Nginx und Traefik
übernehmen sie alle.

```env title=".env"
EVEY_DOMAIN=media.example.com
```

## Caddy (mitgeliefert)

Das Repository liefert [`docker/Caddyfile`](../docker/Caddyfile) (in ein Image
gebaut von [`docker/Dockerfile.caddy`](../docker/Dockerfile.caddy)): Es holt
automatisch HTTPS für `EVEY_DOMAIN` und leitet alles an `evey:9091` weiter,
mit den Streaming-Ausnahmen bereits gesetzt. Es setzt außerdem die kanonischen
Sicherheits-Header und die CSP aus dem Abschnitt oben (und entfernt die eigene
CSP der Anwendung, damit genau eine beim Browser ankommt). Fügen Sie Ihrer
Compose-Datei einen `caddy`-Dienst hinzu (siehe den Dienst `caddy` in der
[`docker-compose.yml`](../docker-compose.yml) des Repositorys).

### Hinter einem bereits vorhandenen Proxy (TLS vorgelagert terminiert)

Auf einem NAS, das bereits einen Reverse Proxy hat (den des Herstellers, Nginx
Proxy Manager, einen Traefik in einem gemeinsamen Docker-Netzwerk), **braucht
es keinen Caddy**: Evey sendet seine eigenen Sicherheits-Header und eine
vollständige CSP, und die Streaming-Regeln kann ohnehin nur der Proxy
durchsetzen, der dem Client gegenübersteht - ein zweiter Proxy dahinter bringt
nichts. Hängen Sie den Dienst evey in das Netzwerk dieses Proxys und zielen Sie
auf `evey-server:9091`:

```yaml title="docker-compose.yml"
services:
  evey:
    # keine Ports: der Proxy ist der einzige Zugang
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # auf das Subnetz des Proxys einengen, sobald es bekannt ist
    # ...
networks:
  default:
    name: proxy-net
    external: true
```

Dann, in diesem Proxy, für die Route zu `evey-server:9091`: `X-Forwarded-For`
und `X-Forwarded-Proto` weiterleiten, WebSocket-Unterstützung einschalten und
die Streaming-Regeln oben anwenden (keine Pufferung, kein Lese- oder
Schreib-Timeout auf den Streaming-Pfaden) - die Abschnitte zu Nginx, Traefik
und Nginx Proxy Manager unten zeigen die genauen Zeilen.

## Nginx

Verwenden Sie [`examples/nginx/evey.conf.template`](../examples/nginx/evey.conf.template)
(eine fertige Compose-Datei liegt unter
[`examples/nginx/docker-compose.nginx.yml`](../examples/nginx/docker-compose.nginx.yml)).
Das nginx-Image rendert sie beim Start mit eingesetztem `EVEY_DOMAIN` - die
Zertifikate müssen unter den passenden Dateinamen vorhanden sein. Sie leitet
an `evey:9091` weiter, mit abgeschalteter Streaming-Pufferung - kein
Build-Schritt für ein Frontend.

## Traefik

Siehe [`examples/traefik/docker-compose.traefik.yml`](../examples/traefik/docker-compose.traefik.yml):

```bash
docker compose -f docker-compose.yml -f examples/traefik/docker-compose.traefik.yml up -d
```

## Nginx Proxy Manager

Der Container evey muss im Netzwerk sein, das NPM verwendet (siehe „Hinter
einem bereits vorhandenen Proxy" oben). Dann **Edit Proxy Host**:

1. **Details**: Scheme `http`, Forward Hostname `evey-server`, Forward Port
   `9091`. **Websockets Support: on** (SyncPlay). **Cache Assets: off** - es
   würde Mediensegmente und Playlists mit fester Ablaufzeit cachen. Block
   Common Exploits kann an bleiben (schalten Sie es aus, falls eine Suche oder
   ein Plugin-Upload einmal grundlos mit 403 antwortet).
2. **SSL**: Ihr Zertifikat, Force SSL, HTTP/2. HSTS ist hier optional - die
   Anwendung sendet es bereits.
Das ist alles: NPM leitet `X-Forwarded-For` / `X-Forwarded-Proto` und den
Host-Header von selbst weiter, und seine Standard-Timeouts lösen bei Eveys
Streams nie aus (siehe den Streaming-Abschnitt oben).

**Optional** - verhindern, dass NPM Direktwiedergabe-Dateien über seine eigene
Festplatte puffert: **Advanced** → *Custom Nginx Configuration* (der Reiter
`Custom Locations` kann den regulären Ausdruck nicht ausdrücken):

```nginx
# Plugin-Uploads (bis 100 MB) und Wiederherstellungen von Sicherungen (bis 2 GB).
client_max_body_size 2G;

# Streaming - keine Pufferung, kein Timeout.
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

# Bibliotheksscans streamen ihren Fortschritt, solange der Scan läuft.
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

Jeder andere Pfad (die Weboberfläche, der Rest der API, der SyncPlay-WebSocket)
läuft durch den eigenen Proxy-Block von NPM, den der Reiter Details
konfiguriert. Lassen Sie den Schnipsel weg, bricht nichts - er ändert nur, wo
große Dateien gepuffert werden.

## Echte Client-IPs und die Sperre für den Fernzugriff

Sobald Ihr Proxy die echte Client-Adresse weiterleitet, kann Evey einen
Zuschauer zu Hause von einem im Internet unterscheiden. Darauf wirken die
WAN-Bitrate-Obergrenze und die Sperre für den Fernzugriff, die beiden
Einstellungen verhalten sich also nur zusammen richtig:

- Die vertrauenswürdigen Proxys (Admin-Bereich oder `EVEY_TRUSTED_PROXIES`)
  müssen den Proxy abdecken (`172.16.0.0/12`, wenn er ein Container im
  Docker-Netzwerk ist; nichts hinzuzufügen, wenn er auf dem Host läuft,
  Loopback ist immer vertrauenswürdig). Was Sie festlegen, wird zur
  Loopback-Grundlage HINZUGEFÜGT - es ersetzt sie nie, sodass ein Docker-Wert
  die Sperre nicht stillschweigend daran hindern kann, echte Client-Adressen zu
  sehen.
- Verkettete Proxys müssen jeweils `X-Forwarded-For` erhalten. In Caddy ist das
  `servers { trusted_proxies static private_ranges }`; nginx verwendet
  `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`.

**Das Einschalten ändert, wer als entfernt gilt.** Vorher sah jede Anfrage so
aus, als käme sie von der privaten Adresse des Proxys, und alle wurden als LAN
eingestuft. Danach werden echte entfernte Clients korrekt erkannt - und ist der
Fernzugriff deaktiviert, werden sie auf jeder Route abgelehnt
(`403 {"code":"remote_access_disabled"}`), obwohl sich keine Einstellung der
Anwendung geändert hat.

Evey schützt sich davor: Der Fernzugriff ist standardmäßig aktiviert, sobald
eine öffentliche URL konfiguriert ist und keine ausdrückliche Wahl getroffen
wurde, der Zustand wird bei jedem Start protokolliert, Ablehnungen werden mit
der Client-IP protokolliert, `evey admin doctor` schlägt bei „veröffentlicht,
aber ablehnend" deutlich fehl, und der Admin-Bereich zeigt eine Warnung. Prüfen
Sie nach einer Änderung der Proxy-Vertrauenseinstellungen `evey admin doctor`.
