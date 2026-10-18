---
source_sha: 882540a1c223edaceac62b6090fc29c8d6391b2bdbb9f156d5cd7015f46e2bc2
reviewed: true
---
# Reverse-Proxy-Konfigurationen { #reverse-proxy-configurations }

Evey liefert die API und die Weboberfläche zusammen auf Port **9091** aus, mit
einfachem HTTP. Ein Reverse Proxy muss nur TLS übernehmen und alles an
`evey:9091` weiterleiten. Wählen Sie unten Ihren Proxy (mitgeliefertes Caddy,
Nginx, Traefik, Nginx Proxy Manager oder einen, den Sie schon betreiben).
Lesen Sie danach die beiden Abschnitte am Ende, wenn Sie das Streaming
feinabstimmen oder eigene Sicherheits-Header setzen.

## Evey den Proxy mitteilen { #tell-evey-about-the-proxy }

Egal welcher Proxy: Evey braucht seine öffentliche Adresse und muss wissen,
welchem Proxy es vertrauen soll. Beides gehört nach **Verwaltung →
Fernzugriff**: eine **Externe URL** mit `https://` und, wenn der Proxy ein
Container ist, sein Netz unter **Vertrauenswürdige Proxys**. Beides gilt ohne
Neustart. An einer `https://`-Adresse erkennt Evey, dass vor ihm ein Proxy TLS
übernimmt: Cookies bekommen das Flag `Secure`, HSTS wird gesendet, und
`X-Forwarded-Proto` von einem vertrauenswürdigen Proxy wird übernommen. Den
Schalter für den Fernzugriff und die echte Adresse des Clients erklärt
[Fernzugriff und HTTPS](administration/remote-access.md).

Dieselben Angaben können auch aus der Umgebung kommen, unter `environment:` in
der Compose-Datei, für eine Installation, die ganz als Code beschrieben ist
(sind beide gesetzt, hat der Verwaltungsbereich Vorrang):

```yaml title="docker-compose.yml"
services:
  evey:
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 172.16.0.0/12   # nur nötig, wenn der Proxy ein Container ist; Loopback ist immer vertrauenswürdig
```

Das frühere `EVEY_FORWARDED_PROTO` wird als Überschreibung noch angenommen,
ist aber veraltet: Das Schema der Adresse genügt.

Jedes mitgelieferte Beispiel liest die Domain aus `EVEY_DOMAIN`, mit einem
Platzhalter in der Datei, die Sie kopieren: Ersetzen Sie ihn dort durch Ihre
Domain, in der Zeile `EVEY_DOMAIN:` des Proxy-Dienstes (Caddy, Nginx) oder in
der Regel `Host(...)` (Traefik).

## Caddy (mitgeliefert) { #caddy-bundled }

### Dateien { #files }

[`docker/Caddyfile`](https://github.com/eveyproject/evey/blob/master/docker/Caddyfile), zu einem Image gebaut von
[`docker/Dockerfile.caddy`](https://github.com/eveyproject/evey/blob/master/docker/Dockerfile.caddy); der Dienst `caddy`
in der [`docker-compose.yml`](https://github.com/eveyproject/evey/blob/master/docker-compose.yml) des Repositorys.

### Einrichten { #set-it-up }

1. Tragen Sie Ihre Domain in der Zeile `EVEY_DOMAIN:` des Dienstes `caddy`
   ein und sorgen Sie dafür, dass die Ports 80 und 443 des Hosts beim
   Caddy-Container ankommen.
2. Übernehmen Sie den Dienst `caddy` aus der Compose-Datei des Repositorys in
   Ihre und führen Sie `docker compose up -d` aus. Caddy holt sich selbst ein
   Let's-Encrypt-Zertifikat für `EVEY_DOMAIN`.
3. Tragen Sie unter **Verwaltung → Fernzugriff** die **Externe URL** mit
   `https://` ein und `172.16.0.0/12` unter **Vertrauenswürdige Proxys**.

### Was es schon erledigt { #what-it-already-handles }

Leitet alles an `evey:9091` weiter, die Streaming-Pfade ohne Pufferung und
ohne Zeitlimit. Setzt sorgfältig die Sicherheits-Header und die
maßgebliche CSP aus dem
[Abschnitt zur Sicherheit](#security-headers--csp---read-this-if-your-proxy-sets-its-own)
und entfernt die eigene CSP der App, damit genau eine beim Browser ankommt;
die beiden Seiten mit eigener Richtlinie behalten sie.

## Nginx { #nginx }

### Dateien { #files }

[`examples/nginx/evey.conf.template`](https://github.com/eveyproject/evey/blob/master/examples/nginx/evey.conf.template)
und eine fertige Compose-Ergänzung,
[`examples/nginx/docker-compose.nginx.yml`](https://github.com/eveyproject/evey/blob/master/examples/nginx/docker-compose.nginx.yml).

### Einrichten { #set-it-up }

1. Tragen Sie Ihre Domain in der Zeile `EVEY_DOMAIN:` der Ergänzung ein. Das
   nginx-Image füllt die Vorlage beim Start aus und setzt dabei `EVEY_DOMAIN`
   ein.
2. Legen Sie Zertifikat und Schlüssel dort ab, wo die Vorlage sie erwartet,
   unter den passenden Dateinamen (`/etc/ssl/certs/<domain>.pem`,
   `/etc/ssl/private/<domain>.key` im Volume `nginx-certs`): Die Ergänzung
   besorgt keine Zertifikate für Sie.
3. `docker compose -f docker-compose.yml -f examples/nginx/docker-compose.nginx.yml up -d`.
4. Tragen Sie unter **Verwaltung → Fernzugriff** die **Externe URL** mit
   `https://` ein und `172.16.0.0/12` unter **Vertrauenswürdige Proxys**.

### Was es schon erledigt { #what-it-already-handles }

Leitet an `evey:9091` weiter, mit abgeschalteter Pufferung und abgeschalteten
Zeitlimits auf den Streaming-Pfaden, mit dem Upgrade für den
SyncPlay-WebSocket, mit weitergereichtem `X-Forwarded-For` und
`X-Forwarded-Proto` und mit `client_max_body_size 2G` für Plugin-Uploads und
das Wiederherstellen von Backups. Die eigenen Sicherheits-Header und die CSP
der App reicht es unverändert durch.

## Traefik { #traefik }

### Dateien { #files }

[`examples/traefik/docker-compose.traefik.yml`](https://github.com/eveyproject/evey/blob/master/examples/traefik/docker-compose.traefik.yml).

### Einrichten { #set-it-up }

1. Betreiben Sie Traefik mit einem Entrypoint `websecure`, einem
   Zertifikats-Resolver `letsencrypt` und einem externen Docker-Netz namens
   `proxy`; die Labels der Ergänzung setzen diese Namen voraus.
2. Tragen Sie Ihre Domain in die Regel `Host(...)` der Ergänzung ein, dann:

    ```bash
    docker compose -f docker-compose.yml -f examples/traefik/docker-compose.traefik.yml up -d
    ```

3. Tragen Sie unter **Verwaltung → Fernzugriff** die **Externe URL** mit
   `https://` ein und den Bereich des Netzes `proxy` unter
   **Vertrauenswürdige Proxys** (`172.16.0.0/12` deckt die Standardnetze von
   Docker ab).

### Was es schon erledigt { #what-it-already-handles }

Die Ergänzung fügt nur die Router-Labels hinzu und tritt dem Netz `proxy`
bei. Traefik puffert mit seinen Standardeinstellungen weder Antworten noch
begrenzt es die Dauer einer Antwort. Streaming muss also nicht angepasst
werden. Es reicht `X-Forwarded-For` und `X-Forwarded-Proto` von selbst weiter
und lässt die Sicherheits-Header der App durch.

## Nginx Proxy Manager { #nginx-proxy-manager }

### Dateien { #files }

Keine: Alles wird in der Oberfläche von NPM eingestellt. Der Evey-Container
muss im Netz sein, das NPM benutzt, siehe
[Ein Proxy, den Sie schon betreiben](#a-proxy-you-already-run-tls-terminated-upstream).

### Einrichten { #set-it-up }

In **Edit Proxy Host**:

1. **Details**: Scheme `http`, Forward Hostname `evey-server`, Forward Port
   `9091`. **Websockets Support: an** (SyncPlay). **Cache Assets: aus** - sonst
   würde NPM Mediensegmente und Playlists mit einer festen Ablaufzeit
   zwischenspeichern. Block Common Exploits kann an bleiben (schalten Sie es
   aus, falls eine Suche oder ein Plugin-Upload einmal grundlos mit 403
   antwortet).
2. **SSL**: Ihr Zertifikat, Force SSL, HTTP/2. HSTS ist hier freiwillig - die
   App sendet es bereits.
3. Tragen Sie unter **Verwaltung → Fernzugriff** die **Externe URL** mit
   `https://` ein und den Bereich des NPM-Netzes unter **Vertrauenswürdige
   Proxys**.

### Was es schon erledigt { #what-it-already-handles }

NPM reicht `X-Forwarded-For`, `X-Forwarded-Proto` und den Host-Header von
selbst weiter, und seine Standard-Zeitlimits greifen bei Eveys Streams nie.
Mehr ist nicht nötig.

**Freiwillig** - damit NPM Direct-Play-Dateien nicht über seine eigene
Festplatte puffert: **Advanced** → *Custom Nginx Configuration* (der Reiter
`Custom Locations` kann den regulären Ausdruck nicht abbilden):

```nginx
# Plugin-Uploads (bis 100 MB) und Wiederherstellen von Backups (bis 2 GB).
client_max_body_size 2G;

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

# Bibliotheks-Scans senden ihren Fortschritt, solange der Scan läuft.
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

Alle anderen Pfade (die Weboberfläche, der Rest der API, der
SyncPlay-WebSocket) laufen über den eigenen Proxy-Block von NPM, den der Reiter
Details einstellt. Lassen Sie den Ausschnitt weg, geht nichts kaputt - er
ändert nur, wo große Dateien gepuffert werden.

## Ein Proxy, den Sie schon betreiben (TLS davor beendet) { #a-proxy-you-already-run-tls-terminated-upstream }

Auf einem NAS, das schon einen Reverse Proxy hat (den des Herstellers, Nginx
Proxy Manager, ein Traefik in einem gemeinsamen Docker-Netz), wird kein Caddy
gebraucht: Evey sendet eigene Sicherheits-Header und eine vollständige CSP,
und die Streaming-Regeln kann ohnehin nur der Proxy durchsetzen, mit dem der
Client direkt verbunden ist. Ein zweiter Proxy dahinter bringt nichts.

### Einrichten { #set-it-up }

Verbinden Sie den Dienst `evey` mit dem Netz dieses Proxys und leiten Sie an
`evey-server:9091` weiter (der Containername; der Dienstname `evey` funktioniert
auch):

```yaml title="docker-compose.yml"
services:
  evey:
    # keine Ports: Der Proxy ist der einzige Weg hinein
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # auf das Subnetz des Proxys eingrenzen, sobald es bekannt ist
    # ...
networks:
  default:
    name: proxy-net
    external: true
```

Dann in diesem Proxy, für die Route zu `evey-server:9091`: `X-Forwarded-For`
und `X-Forwarded-Proto` weiterreichen, WebSocket-Unterstützung einschalten und
auf den Streaming-Pfaden die Streaming-Regeln unten anwenden - die Abschnitte
zu Nginx und Nginx Proxy Manager oben zeigen die genauen Zeilen.

### Was es schon erledigt { #what-it-already-handles }

Das hängt vom Proxy ab: Prüfen Sie, dass genau ein CSP-Header beim Browser
ankommt (nächste Abschnitte), und führen Sie `evey admin doctor` aus, sobald
die vertrauenswürdigen Proxys eingetragen sind.

## Streaming - was die Standardeinstellungen eines Proxys damit machen { #streaming---what-a-proxys-defaults-do-to-it }

Evey funktioniert hinter einem Reverse Proxy mit Standardeinstellungen: Jede
lang laufende Antwort sendet entweder ständig Bytes (HLS-Segmente, Direct
Play, Ausgabe der Transkodierung) oder meldet sich alle 30 s beim Client (die
Ereignis-Streams). Ein Standard-Zeitlimit beim Lesen greift also nie. Zwei
Dinge sind trotzdem gut zu wissen:

- **Pufferung.** Ein Proxy, der Antworten puffert (nginx tut das
  standardmäßig), liest eine Direct-Play-Datei erst in seine eigenen Puffer
  und temporären Dateien, bevor der Client sie bekommt. Das funktioniert, aber
  ein Film von mehreren GB geht dann über die Festplatte des Proxy-Rechners.
  `proxy_buffering off` auf den Streaming-Pfaden verhindert das.
- **Scans.** `/api/v1/libraries/*/scan` sendet den Fortschritt, solange der
  Scan läuft. Ein Proxy, der die Gesamtdauer einer Antwort begrenzt (selten -
  nginx, Caddy und Traefik tun das standardmäßig nicht), würde ihn vorzeitig
  abschneiden. Der Scan selbst läuft auf dem Server trotzdem weiter.

Die betroffenen Pfade:

```
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

Das mitgelieferte Caddy und das Nginx-Beispiel setzen auf diesen Pfaden
beides (keine Pufferung, kein Zeitlimit). Das Traefik-Beispiel und ein über
eine Oberfläche gesteuerter Proxy wie Nginx Proxy Manager behalten ihre
Standardeinstellungen, und die passen; der NPM-Ausschnitt oben ist freiwillig.

## Sicherheits-Header & CSP - lesen Sie das, wenn Ihr Proxy eigene setzt { #security-headers--csp---read-this-if-your-proxy-sets-its-own }

Evey sendet eine vollständige Content-Security-Policy. Reicht Ihr Proxy sie
durch, sind Sie fertig.

Viele Einrichtungen entfernen sie stattdessen
(`header_down -Content-Security-Policy` in Caddy) und setzen eine eigene. Das
ist in Ordnung, aber dann muss **Ihre** Richtlinie diese Einträge behalten,
sonst fallen Funktionen ohne Fehlermeldung aus:

| Eintrag | Direktive | Fällt ohne ihn aus |
| --- | --- | --- |
| `'self'` | `frame-src` | Jede Plugin-Oberfläche und jedes Theme-Skript (leerer iframe). Sie laufen in `/plugin-frame.html`, einer Seite von Evey, die die App einbettet (siehe unten). |
| `'wasm-unsafe-eval'` | `script-src` | AC-3/E-AC-3-Wiedergabe im Browser (der WebAssembly-Decoder ac3go kann nicht kompiliert werden, der Film läuft also stumm). Das ist der enge Eintrag, der nur das Kompilieren von Wasm erlaubt, kein allgemeines `eval()`. |
| `https://www.gstatic.com` | `script-src` (+ `frame-src`) | Das Google-Cast-Sender-SDK lädt nie, also bekommt niemand eine Cast-Schaltfläche. Nur so kann ein Browser Chromecasts im eigenen LAN des Nutzers finden. |
| `blob:` | `media-src` | MSE-Wiedergabe (hls.js hängt den Stream über eine Blob-URL an). |
| `blob:` | `worker-src` | hls.js weicht ohne Meldung auf Demuxing im Hauptthread aus (ruckeliger auf Handys). |

Eine bewährte Richtlinie für einen Proxy, der seine eigene setzt, nämlich die,
die das mitgelieferte Caddy sendet:

```
default-src 'self'; base-uri 'self'; object-src 'none';
img-src 'self' https: data: blob:; media-src 'self' blob:;
style-src 'self' 'unsafe-inline';
script-src 'self' 'wasm-unsafe-eval' https://www.gstatic.com;
connect-src 'self' https: wss:; worker-src 'self' blob:; font-src 'self' data:;
frame-src 'self' https://www.gstatic.com
```

`base-uri` und `object-src` fallen nicht auf `default-src` zurück. Eine
Richtlinie ohne sie lässt also das Einschleusen von `<base href>` und das
Einbetten über `<object>`/`<embed>` uneingeschränkt zu. Behalten Sie sie, wenn
Sie eine eigene Richtlinie setzen. `script-src` hat kein `'unsafe-inline'`:
Ein in die Seite eingeschleustes Skript läuft nicht. Fügen Sie es nicht wieder
hinzu - nichts in der App braucht es.

Zwei Seiten haben eine eigene Richtlinie, die die App sendet. Lassen Sie auf
diesen beiden Pfaden den Header der App stehen:

- `/plugin-frame.html`, die Seite, in der Plugin-Oberflächen und
  Theme-Skripte laufen. Ihre Richtlinie führt sie in einer Sandbox aus (auch wenn
  sie außerhalb eines iframes geöffnet wird), beschränkt ihren Netzzugriff auf
  die Hosts, die das Manifest des Plugins angibt, und lässt nur die App sie
  einbetten. Deshalb sendet sie auch `X-Frame-Options: SAMEORIGIN`:
  Überschreiben Sie das auf diesem Pfad nicht mit `DENY`, sonst bleibt jede
  Plugin-Oberfläche leer.
- `/cast-receiver.html`, die Seite, die ein Chromecast lädt, wenn er von Evey
  abspielt. Der Cast-Player lädt Stile, Schriften und Bilder von
  `www.gstatic.com` in sie. Lassen Sie auf diesem Pfad den Header der App
  stehen, oder wenden Sie diese Richtlinie nur auf diesen Pfad an:

```
default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com; style-src 'self' 'unsafe-inline' https://www.gstatic.com; img-src 'self' data: https: blob:; media-src 'self' blob: https:; worker-src 'self' blob:; font-src 'self' data: https://www.gstatic.com; connect-src 'self' https: wss:; frame-src https://www.gstatic.com; base-uri 'self'; object-src 'none'
```

!!! warning "Genau ein CSP-Header darf beim Browser ankommen"
    Bei zweien (dem der App und dem Ihres Proxys, oder dem von Caddy und dem
    „Advanced“-Ausschnitt von Nginx Proxy Manager) setzen Browser die
    *Schnittmenge* durch: Die strengere Richtlinie gilt, und der Fehler wird nicht gemeldet.
    Prüfen Sie das mit
    `curl -sI https://your.domain | grep -ci content-security-policy`, das
    `1` ausgeben muss.

Zwei weitere Prüfungen nach jeder Änderung:

- **Halten Sie die Richtlinie in einer Zeile.** Eine CSP, die in einer
  Proxy-Konfiguration über mehrere Zeilen verteilt ist, wird häufig
  unbemerkt beschädigt: Beim Zusammenfügen gehen Zeichen an der Verbindungsstelle verloren
  (`...gstatic.com; connect-src` wurde einmal zu `...gstaticonnect-src`, was
  das Cast-SDK abschaltete und zwei Direktiven verschmolz).
- **Der Plugin-Frame behält seine eigene Richtlinie:**
  `curl -sI https://your.domain/plugin-frame.html | grep -i -e content-security -e x-frame`
  muss `sandbox allow-scripts` und `SAMEORIGIN` zeigen.

## Echte Client-IPs und die Sperre für den Fernzugriff { #real-client-ips-and-the-remote-access-gate }

Sobald der Proxy die echte Adresse des Clients weiterreicht, kann Evey einen
Zuschauer zu Hause von einem im Internet unterscheiden: Darauf wirken
**Max. Remote-Bitrate** und der Schalter für den Fernzugriff. Zwei Regeln:

- Die vertrauenswürdigen Proxys (im Verwaltungsbereich oder
  `EVEY_TRUSTED_PROXIES`) müssen den Proxy abdecken: `172.16.0.0/12`, wenn er
  ein Container im Docker-Netz ist. Läuft er auf dem Host, ist nichts
  hinzuzufügen, denn Loopback ist immer vertrauenswürdig, und was Sie
  eintragen, kommt dazu.
- Hintereinander geschaltete Proxys müssen jeweils `X-Forwarded-For`
  erhalten. In Caddy ist das
  `servers { trusted_proxies static private_ranges }`; nginx nutzt
  `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`.

Das einzuschalten ändert, wer als unterwegs gilt: Ein Server, dessen
Fernzugriff aus war, weist dann echte entfernte Clients ab
(`403 {"code":"remote_access_disabled"}`), obwohl sich keine Einstellung der
Anwendung geändert hat. Evey öffnet den Fernzugriff von selbst, wenn eine
öffentliche Adresse gesetzt ist und niemand eine Wahl getroffen hat,
protokolliert den Zustand bei jedem Start und jede Abweisung mit der Adresse
des Clients, und `evey admin doctor` schlägt bei „veröffentlicht, aber
abweisend“ fehl. Der ganze Mechanismus ist in
[Fernzugriff und HTTPS](administration/remote-access.md#the-real-client-address)
beschrieben.

<!-- sources
one port for API and UI, https mode from the scheme: internal/config/config.go (EVEY_FORWARDED_PROTO deprecated) ; internal/config/network_live.go - EffectiveHTTPS
bundled Caddy: docker/Caddyfile (streaming matcher, @ownPolicy, header_down CSP) ; docker/Dockerfile.caddy ; docker-compose.yml caddy service
nginx: examples/nginx/evey.conf.template ; examples/nginx/docker-compose.nginx.yml (nginx-certs volume, no ACME)
traefik: examples/traefik/docker-compose.traefik.yml (labels + proxy network only)
streaming paths: cmd/evey/gateway.go ; event streams ping 30 s ; scan progress stream
csp and own-policy pages: internal/adapters/driving/http/helpers.go - securityHeadersMiddleware ; /plugin-frame.html ; /cast-receiver.html
trusted proxies, catch-all refused, doctor: internal/config/trusted_proxies.go ; cmd/evey/admin_ops.go - checkTrustedProxies
remote access inferred: docs/adr/0026-remote-access-inferred-from-a-published-url.md
labels: i18n admin.remoteAccess.*
-->
