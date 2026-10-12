---
source_sha: d47608ac11df56bc518674752f94a79d5498896103d51978e420575983e71cf4
reviewed: true
---
# Configuraciones de proxy inverso

Evey sirve la API y la interfaz web juntas en el puerto **9091**, en HTTP
simple; un proxy inverso solo tiene que encargarse de TLS y reenviarlo todo a
`evey:9091`. Elige tu proxy más abajo (el Caddy incluido, Nginx, Traefik,
Nginx Proxy Manager, o uno que ya tengas funcionando), y lee después las dos
secciones del final si ajustas el streaming o pones tus propias cabeceras de
seguridad.

## Decirle a Evey que hay un proxy

Sea cual sea el proxy, Evey necesita su dirección pública y saber en qué
proxy confiar. Las dos cosas se ponen en **Administración → Acceso remoto**:
una **URL externa** en `https://` y, cuando el proxy es un contenedor, su
red en **Proxies de confianza**. Las dos se aplican sin reiniciar. Una
dirección `https://` es lo que le dice a Evey que delante de él hay un proxy
que se encarga de TLS: las cookies llevan la marca `Secure`, se envía HSTS y
se acepta la cabecera `X-Forwarded-Proto` de un proxy de confianza. El
interruptor de acceso remoto y la dirección real del cliente se explican en
[Acceso remoto y HTTPS](administration/remote-access.md).

Las mismas respuestas pueden venir del entorno, para una instalación descrita
entera como código (el panel tiene prioridad cuando están las dos):

```env title=".env"
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=172.16.0.0/12       # solo hace falta si el proxy es un contenedor; loopback siempre es de confianza
```

La antigua `EVEY_FORWARDED_PROTO` todavía se acepta para forzar el valor,
pero está obsoleta: basta con el esquema de la dirección.

Todos los ejemplos incluidos leen el dominio de un solo sitio, `EVEY_DOMAIN`
en tu `.env`. Ponlo una vez; Caddy, Nginx y Traefik lo usan los tres.

```env title=".env"
EVEY_DOMAIN=media.example.com
```

## Caddy (incluido)

### Archivos

[`docker/Caddyfile`](../docker/Caddyfile), convertido en imagen por
[`docker/Dockerfile.caddy`](../docker/Dockerfile.caddy); el servicio `caddy`
del [`docker-compose.yml`](../docker-compose.yml) del repositorio.

### Configurarlo

1. Pon `EVEY_DOMAIN` en `.env` y comprueba que los puertos 80 y 443 de la
   máquina llegan al contenedor de Caddy.
2. Añade a tu archivo compose el servicio `caddy` del archivo compose del
   repositorio y ejecuta `docker compose up -d`. Caddy obtiene por sí solo un
   certificado de Let's Encrypt para `EVEY_DOMAIN`.
3. En **Administración → Acceso remoto**, pon la **URL externa** en
   `https://` y `172.16.0.0/12` en **Proxies de confianza**.

### Lo que ya resuelve

Lo reenvía todo a `evey:9091`, con las rutas de streaming enviadas sin búfer
y sin tiempo límite. Pone las cabeceras de seguridad y la CSP de referencia
de la [sección de seguridad](#security-headers--csp---read-this-if-your-proxy-sets-its-own),
y quita la CSP propia de la aplicación para que llegue exactamente una al
navegador; las dos páginas que llevan su propia política la conservan.

## Nginx

### Archivos

[`examples/nginx/evey.conf.template`](../examples/nginx/evey.conf.template)
y una capa compose lista para usar,
[`examples/nginx/docker-compose.nginx.yml`](../examples/nginx/docker-compose.nginx.yml).

### Configurarlo

1. Pon `EVEY_DOMAIN` en `.env`. La imagen de nginx genera la configuración a
   partir de la plantilla al arrancar, sustituyendo `EVEY_DOMAIN`.
2. Pon el certificado y la clave donde los espera la plantilla, con los
   nombres de archivo que corresponden (`/etc/ssl/certs/<domain>.pem`,
   `/etc/ssl/private/<domain>.key` en el volumen `nginx-certs`): esta capa
   no obtiene certificados por ti.
3. `docker compose -f docker-compose.yml -f examples/nginx/docker-compose.nginx.yml up -d`.
4. En **Administración → Acceso remoto**, pon la **URL externa** en
   `https://` y `172.16.0.0/12` en **Proxies de confianza**.

### Lo que ya resuelve

Reenvía a `evey:9091` con el búfer y los tiempos límite desactivados en las
rutas de streaming, el WebSocket de SyncPlay actualizado, `X-Forwarded-For` y
`X-Forwarded-Proto` transmitidos, y `client_max_body_size 2G` para subir
plugins y restaurar copias de seguridad. Deja pasar las cabeceras de
seguridad y la CSP propias de la aplicación.

## Traefik

### Archivos

[`examples/traefik/docker-compose.traefik.yml`](../examples/traefik/docker-compose.traefik.yml).

### Configurarlo

1. Ejecuta Traefik con un entrypoint `websecure`, un resolvedor de
   certificados `letsencrypt` y una red Docker externa llamada `proxy`; las
   etiquetas de la capa dan por hecho esos nombres.
2. Pon `EVEY_DOMAIN` en `.env` y después:

    ```bash
    docker compose -f docker-compose.yml -f examples/traefik/docker-compose.traefik.yml up -d
    ```

3. En **Administración → Acceso remoto**, pon la **URL externa** en
   `https://` y el rango de la red `proxy` en **Proxies de confianza**
   (`172.16.0.0/12` cubre las redes por defecto de Docker).

### Lo que ya resuelve

La capa solo añade las etiquetas del router y se une a la red `proxy`.
Traefik, con sus valores por defecto, no guarda las respuestas en búfer ni
limita su duración, así que no hace falta ajustar nada para el streaming;
transmite `X-Forwarded-For` y `X-Forwarded-Proto` por sí solo y deja pasar
las cabeceras de seguridad de la aplicación.

## Nginx Proxy Manager

### Archivos

Ninguno: todo se configura en la interfaz de NPM. El contenedor de evey tiene
que estar en la red que usa NPM, ver
[Un proxy que ya tienes funcionando](#a-proxy-you-already-run-tls-terminated-upstream).

### Configurarlo

En **Edit Proxy Host**:

1. **Details**: Scheme `http`, Forward Hostname `evey-server`, Forward Port
   `9091`. **Websockets Support: on** (SyncPlay). **Cache Assets: off** - si
   no, guardaría en caché los segmentos de vídeo y las listas de
   reproducción con una caducidad fija. Block Common Exploits puede quedarse
   activado (desactívalo si alguna vez una búsqueda o la subida de un plugin
   responde 403 sin motivo).
2. **SSL**: tu certificado, Force SSL, HTTP/2. Aquí HSTS es opcional - la
   aplicación ya lo envía.
3. En **Administración → Acceso remoto**, pon la **URL externa** en
   `https://` y el rango de la red de NPM en **Proxies de confianza**.

### Lo que ya resuelve

NPM transmite `X-Forwarded-For`, `X-Forwarded-Proto` y la cabecera Host por
sí solo, y sus tiempos límite por defecto nunca saltan con los flujos de
Evey. No hace falta nada más.

**Opcional** - evitar que NPM guarde en búfer, en su propio disco, los
archivos en reproducción directa: **Advanced** → *Custom Nginx Configuration*
(la pestaña `Custom Locations` no admite la expresión regular):

```nginx
# Subida de plugins (hasta 100 MB) y restauración de copias de seguridad (hasta 2 GB).
client_max_body_size 2G;

# Streaming - sin búfer, sin tiempo límite.
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

# Los escaneos de biblioteca envían su progreso mientras dura el escaneo.
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

Todas las demás rutas (la interfaz web, el resto de la API, el WebSocket de
SyncPlay) pasan por el bloque de proxy propio de NPM, que se configura en la
pestaña Details. Si omites este fragmento, no deja de funcionar nada - solo cambia
dónde se guardan en búfer los archivos grandes.

## Un proxy que ya tienes funcionando (TLS resuelto antes)

En un NAS que ya tiene un proxy inverso (el del fabricante, Nginx Proxy
Manager, un Traefik en una red Docker compartida), no hace falta Caddy: Evey
envía sus propias cabeceras de seguridad y una CSP completa, y de todos modos
las reglas de streaming solo las puede aplicar el proxy que está de cara al
cliente. Un segundo proxy detrás no aporta nada.

### Configurarlo

Pon el servicio evey en la red de ese proxy y apunta a `evey-server:9091`
(el nombre del contenedor; el nombre del servicio `evey` también funciona):

```yaml title="docker-compose.yml"
services:
  evey:
    # sin puertos: el proxy es la única entrada
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # redúcelo a la subred del proxy cuando la conozcas
    # ...
networks:
  default:
    name: proxy-net
    external: true
```

Después, en ese proxy, para la ruta hacia `evey-server:9091`: transmite
`X-Forwarded-For` y `X-Forwarded-Proto`, activa el soporte de WebSocket, y
aplica las reglas de streaming de abajo en las rutas de streaming - las
secciones de Nginx y de Nginx Proxy Manager de arriba muestran las líneas
exactas.

### Lo que ya resuelve

Depende del proxy: comprueba que llega exactamente una cabecera CSP al
navegador (siguientes secciones) y ejecuta `evey admin doctor` una vez
puestos los proxies de confianza.

## Streaming - lo que le hacen los valores por defecto de un proxy

Evey funciona detrás de un proxy inverso con sus valores por defecto: cada
respuesta larga o bien envía datos sin parar (segmentos HLS, reproducción
directa, salida de la transcodificación), o bien envía una señal de actividad al
cliente cada 30 s (los flujos de eventos), así que un tiempo límite de
lectura por defecto nunca salta. Aun así, conviene saber dos cosas:

- **El búfer.** Un proxy que guarda las respuestas en búfer (nginx lo hace
  por defecto) lee un archivo en reproducción directa en sus propios búferes
  y archivos temporales antes de que lo reciba el cliente - funciona, pero
  una película de varios GB pasa entonces por el disco de la máquina del
  proxy. `proxy_buffering off` en las rutas de streaming lo evita.
- **Los escaneos.** `/api/v1/libraries/*/scan` envía el progreso mientras
  dura el escaneo; un proxy que limita la duración total de una respuesta
  (es raro - nginx, Caddy y Traefik no lo hacen por defecto) lo cortaría
  antes. El escaneo sigue en el servidor de todos modos.

Las rutas afectadas:

```
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

El Caddy incluido y el ejemplo de Nginx aplican las dos cosas (sin búfer, sin
tiempo límite) en estas rutas. El ejemplo de Traefik y un proxy que se
maneja desde una interfaz, como Nginx Proxy Manager, mantienen sus valores
por defecto, que están bien; el fragmento de NPM de arriba es opcional.

## Cabeceras de seguridad y CSP - lee esto si tu proxy pone las suyas

Evey envía una Content-Security-Policy completa. Si tu proxy la deja pasar,
no tienes que hacer nada más.

En cambio, muchas configuraciones la quitan (`header_down
-Content-Security-Policy` en Caddy) y ponen la suya - eso está bien, pero
entonces **tu** política tiene que conservar estos tokens, o algunas
funciones se rompen sin avisar:

| Token | Directiva | Qué se rompe sin él |
| --- | --- | --- |
| `'self'` | `frame-src` | Todas las interfaces de plugin y los scripts de tema (iframe en blanco). Se ejecutan en `/plugin-frame.html`, una página de Evey enmarcada por la aplicación (ver más abajo). |
| `'wasm-unsafe-eval'` | `script-src` | La reproducción AC-3/E-AC-3 en el navegador (el decodificador WebAssembly ac3go no puede compilarse, así que la película se reproduce sin sonido). Es el token limitado que solo permite compilar Wasm, no `eval()` en general. |
| `https://www.gstatic.com` | `script-src` (+ `frame-src`) | El SDK emisor de Google Cast no se carga nunca, así que nadie ve el botón de envío. Es la única forma de que un navegador descubra los Chromecast de la red local del usuario. |
| `blob:` | `media-src` | La reproducción MSE (hls.js conecta el flujo mediante una URL blob). |
| `blob:` | `worker-src` | hls.js pasa sin avisar a separar el flujo en el hilo principal (va a tirones en el móvil). |

Una política que funciona para un proxy que pone la suya, la que envía el
Caddy incluido:

```
default-src 'self'; base-uri 'self'; object-src 'none';
img-src 'self' https: data: blob:; media-src 'self' blob:;
style-src 'self' 'unsafe-inline';
script-src 'self' 'wasm-unsafe-eval' https://www.gstatic.com;
connect-src 'self' https: wss:; worker-src 'self' blob:; font-src 'self' data:;
frame-src 'self' https://www.gstatic.com
```

`base-uri` y `object-src` no recurren a `default-src` cuando faltan, así que
una política que los omite deja sin restricción la inyección de
`<base href>` y la inserción con `<object>`/`<embed>`. Consérvalos si pones
tu propia política. `script-src` no lleva `'unsafe-inline'`: un script
inyectado en la página no se ejecuta. No lo vuelvas a añadir - nada en la
aplicación lo necesita.

Dos páginas llevan su propia política, enviada por la aplicación; deja la
cabecera de la aplicación en esas dos rutas:

- `/plugin-frame.html`, la página en la que se ejecutan las interfaces de
  plugin y los scripts de tema. Su política la aísla (incluso abierta fuera
  de un iframe), limita su red a los servidores que declara el manifiesto del
  plugin, y solo deja que la enmarque la aplicación, así que también envía
  `X-Frame-Options: SAMEORIGIN`: no lo sustituyas por `DENY` en esta ruta, o
  todas las interfaces de plugin se quedarán en blanco.
- `/cast-receiver.html`, la página que carga un Chromecast cuando reproduce
  desde Evey. El reproductor Cast le inyecta estilos, fuentes e imágenes de
  `www.gstatic.com`. Deja la cabecera de la aplicación en esa ruta, o aplica
  esta política solo a esa ruta:

```
default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com; style-src 'self' 'unsafe-inline' https://www.gstatic.com; img-src 'self' data: https: blob:; media-src 'self' blob: https:; worker-src 'self' blob:; font-src 'self' data: https://www.gstatic.com; connect-src 'self' https: wss:; frame-src https://www.gstatic.com; base-uri 'self'; object-src 'none'
```

!!! warning "Al navegador tiene que llegar exactamente una cabecera CSP"
    Con dos (la de la aplicación y la de tu proxy, o la de Caddy y el
    fragmento "Advanced" de Nginx Proxy Manager), los navegadores aplican la
    *intersección*: se impone la más estricta y el fallo no es visible. Compruébalo con
    `curl -sI https://your.domain | grep -ci content-security-policy`, que
    tiene que mostrar `1`.

Dos comprobaciones más después de cualquier cambio:

- **Deja la política en una sola línea.** Una CSP repartida en varias líneas
  en la configuración de un proxy es una corrupción silenciosa clásica: al
  unir las líneas se pierden los caracteres de la unión
  (`...gstatic.com; connect-src` se convirtió una vez en
  `...gstaticonnect-src`, lo que desactivó el SDK de Cast y fusionó dos
  directivas).
- **El marco de plugins conserva su propia política:**
  `curl -sI https://your.domain/plugin-frame.html | grep -i -e content-security -e x-frame`
  tiene que mostrar `sandbox allow-scripts` y `SAMEORIGIN`.

## IP reales de los clientes y el control de acceso remoto

En cuanto el proxy transmite la dirección real del cliente, Evey puede
distinguir a un espectador en casa de uno en internet: sobre eso actúan
**Bitrate remoto máximo** y el interruptor de acceso remoto. Dos reglas:

- Los proxies de confianza (panel o `EVEY_TRUSTED_PROXIES`) tienen que
  cubrir el proxy: `172.16.0.0/12` cuando es un contenedor en la red de
  Docker, nada que añadir cuando funciona en la propia máquina, porque
  loopback siempre es de confianza y lo que pongas se suma a él.
- Cada proxy de una cadena tiene que conservar `X-Forwarded-For`. En Caddy es
  `servers { trusted_proxies static private_ranges }`; nginx usa
  `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`.

Activar esto cambia quién cuenta como remoto: un servidor con el acceso
remoto desactivado empieza a rechazar a los clientes remotos reales
(`403 {"code":"remote_access_disabled"}`) aunque no haya cambiado ningún
ajuste de la aplicación. Evey abre el acceso remoto por sí solo cuando hay
una dirección pública y nadie ha elegido, anota el estado en cada arranque y
cada rechazo con la dirección del cliente, y `evey admin doctor` falla con
"published but refusing". Todo el mecanismo se describe en
[Acceso remoto y HTTPS](administration/remote-access.md#the-real-client-address).

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
