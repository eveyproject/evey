---
source_sha: 3b28483f95cf0e01f8af366abbbe130d63e2bfdb7657c49aefe090c25a5ced6f
reviewed: false
---
# Proxy inverso

Evey sirve **la API y la interfaz web juntas** en el puerto **9091** (HTTP sin
cifrar). Un proxy inverso solo tiene que terminar TLS y reenviar todo a
`evey:9091`: **no hay un frontend aparte que compilar ni servir**.

Uses el proxy que uses, indica a Evey su URL pública y en qué proxy confiar. El
sitio más sencillo es **Admin → Acceso remoto**: una **URL externa** en
`https://` y, cuando el proxy es un contenedor, su red en **Proxies de
confianza**. Ambos se aplican sin reiniciar. Las mismas respuestas pueden venir
del entorno, para una instalación descrita por completo como código (el panel
prevalece cuando ambos están definidos):

```env title=".env"
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=172.16.0.0/12       # solo si el proxy es un contenedor; el loopback siempre es de confianza
```

Una dirección `https://` es lo que le dice a Evey que un proxy termina TLS
delante de él: las cookies reciben la marca `Secure`, se envía HSTS y se cree la
cabecera `X-Forwarded-Proto` de un proxy de confianza. No hace falta ningún
interruptor aparte (la antigua `EVEY_FORWARDED_PROTO` se sigue aceptando como
anulación, pero está obsoleta).

## Streaming: lo que los valores por defecto de un proxy le hacen

Evey funciona detrás de un proxy inverso con sus valores por defecto: cada
respuesta de larga duración envía bytes de forma continua (segmentos HLS,
reproducción directa, salida de transcodificación) o hace ping al cliente cada
30 s (los flujos de eventos), así que un tiempo de espera de lectura por defecto
nunca salta. Aun así conviene saber dos cosas:

- **Búfer.** Un proxy que almacena las respuestas en búfer (nginx lo hace por
  defecto) lee un archivo en reproducción directa en sus propios búferes y
  archivos temporales antes de que lo reciba el cliente: funciona, pero una
  película de varios GB pasa entonces por el disco del equipo del proxy.
  `proxy_buffering off` en las rutas de streaming lo evita.
- **Escaneos.** `/api/v1/libraries/*/scan` transmite el progreso mientras dura el
  escaneo; un proxy que limita la duración total de una respuesta (raro: nginx y
  Caddy no lo hacen por defecto) lo cortaría. El escaneo continúa de todos modos
  en el servidor.

Las rutas afectadas:

```
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

El Caddy incluido y los ejemplos de Nginx y Traefik de abajo aplican ambas cosas
(sin búfer, sin tiempo de espera) en estas rutas. Con un proxy gestionado por
interfaz como Nginx Proxy Manager, los valores por defecto sirven y el ajuste es
un fragmento opcional.

## Cabeceras de seguridad y CSP: léelo si tu proxy pone las suyas

Evey emite una Content-Security-Policy completa. Si tu proxy la deja pasar, ya
está.

Muchas configuraciones la eliminan (`header_down -Content-Security-Policy` en
Caddy) y ponen la suya: está bien, pero entonces **tu** política debe conservar
estos tokens o algunas funciones fallan en silencio:

| Token | Directiva | Qué falla sin él |
| --- | --- | --- |
| `'unsafe-inline'` | `script-src` | Todas las interfaces de plugins (iframe en blanco). El paquete de un plugin se descarga con el token del usuario y se inserta en un iframe `srcdoc`, y un documento srcdoc hereda la política de la página que lo incrusta. Lo que acota al plugin es el `sandbox="allow-scripts"` del iframe (origen opaco, sin acceso al DOM ni a las cookies de la página). |
| `'wasm-unsafe-eval'` | `script-src` | La reproducción AC-3/E-AC-3 en el navegador (el descodificador WebAssembly ac3go no puede compilarse → la película se reproduce sin sonido). Es el token acotado que solo permite compilar Wasm, NO `eval()` en general. |
| `https://www.gstatic.com` | `script-src` (+ `frame-src`) | El SDK emisor de Google Cast no carga → nadie tiene botón de envío. Es la única forma de que un navegador descubra los Chromecast de la red local del usuario. |
| `blob:` | `media-src` | La reproducción MSE (hls.js conecta el flujo mediante una URL blob). |
| `blob:` | `worker-src` | hls.js vuelve en silencio a desmultiplexar en el hilo principal (más entrecortado en móvil). |

Una política probada para un proxy que pone la suya:

```
default-src 'self'; base-uri 'self'; object-src 'none';
img-src 'self' https: data: blob:; media-src 'self' blob:;
style-src 'self' 'unsafe-inline';
script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com;
connect-src 'self' https: wss:; worker-src 'self' blob:; font-src 'self' data:;
frame-src https://www.gstatic.com
```

`base-uri` y `object-src` no recurren a `default-src`, así que una política que
los omite deja sin restricción la inyección de `<base href>` y la incrustación
con `<object>`/`<embed>`. Consérvalos si pones tu propia política.

Una página necesita una política más amplia: `/cast-receiver.html`, la página que
carga un Chromecast cuando reproduce desde Evey. El reproductor Cast inserta en
ella estilos, fuentes e imágenes de `www.gstatic.com`, y por eso la aplicación le
envía su propia política. Deja la cabecera de la aplicación en esa ruta, o aplica
esta política solo a esa ruta:

```
default-src 'self'; script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com; style-src 'self' 'unsafe-inline' https://www.gstatic.com; img-src 'self' data: https: blob:; media-src 'self' blob: https:; worker-src 'self' blob:; font-src 'self' data: https://www.gstatic.com; connect-src 'self' https: wss:; frame-src https://www.gstatic.com; base-uri 'self'; object-src 'none'
```

Reglas prácticas:

- **Al navegador debe llegar exactamente una cabecera CSP.** Con dos (por
  ejemplo la de la aplicación Y la de tu proxy, o la de Caddy Y el fragmento
  "Advanced" de Nginx Proxy Manager), los navegadores aplican la *intersección*:
  gana la más estricta y el fallo es silencioso. Compruébalo con
  `curl -sI https://your.domain | grep -ci content-security-policy`
  → debe mostrar `1`.
- **Mantén la política en UNA línea.** Una CSP partida en varias líneas en la
  configuración de un proxy es una corrupción silenciosa clásica: la unión se
  come los caracteres de la costura (`...gstatic.com; connect-src` se convirtió
  una vez en `...gstaticonnect-src`, lo que desactivó el SDK de Cast y fusionó
  dos directivas).
- `'unsafe-inline'` en `script-src` tiene un coste real y hoy es imprescindible
  (interfaces de plugins, ver la tabla). Quitarlo solo es seguro en un despliegue
  que no ejecuta ninguna interfaz de plugin.

Todos los ejemplos incluidos leen el dominio de **un solo sitio**: `EVEY_DOMAIN`
en tu `.env`. Defínelo una vez; Caddy, Nginx y Traefik lo recogen.

```env title=".env"
EVEY_DOMAIN=media.example.com
```

## Caddy (incluido)

El repositorio incluye [`docker/Caddyfile`](../docker/Caddyfile) (convertido en
imagen por [`docker/Dockerfile.caddy`](../docker/Dockerfile.caddy)): obtiene
HTTPS automático para `EVEY_DOMAIN` y reenvía todo a `evey:9091`, con las
excepciones de streaming ya definidas. También pone las cabeceras de seguridad y
la CSP canónicas de la sección anterior (y elimina la CSP de la aplicación para
que llegue exactamente una al navegador). Añade un servicio `caddy` a tu compose
(consulta el servicio `caddy` del [`docker-compose.yml`](../docker-compose.yml)
del repositorio).

### Detrás de un proxy que ya tienes (TLS terminado antes)

En un NAS que ya tiene un proxy inverso (el del fabricante, Nginx Proxy Manager,
un Traefik en una red Docker compartida), **no hace falta Caddy**: Evey emite sus
propias cabeceras de seguridad y una CSP completa, y las reglas de streaming solo
puede aplicarlas el proxy que da la cara al cliente: un segundo proxy detrás no
aporta nada. Pon el servicio evey en la red de ese proxy y apunta a
`evey-server:9091`:

```yaml title="docker-compose.yml"
services:
  evey:
    # sin ports: el proxy es la única entrada
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # acótalo a la subred del proxy cuando la conozcas
    # ...
networks:
  default:
    name: proxy-net
    external: true
```

Después, en ese proxy, para la ruta hacia `evey-server:9091`: reenvía
`X-Forwarded-For` y `X-Forwarded-Proto`, activa el soporte de WebSocket y aplica
las reglas de streaming de arriba (sin búfer, sin tiempo de espera de lectura o
escritura en las rutas de streaming): las secciones de Nginx, Traefik y Nginx
Proxy Manager de abajo muestran las líneas exactas.

## Nginx

Usa [`examples/nginx/evey.conf.template`](../examples/nginx/evey.conf.template)
(hay un compose listo en
[`examples/nginx/docker-compose.nginx.yml`](../examples/nginx/docker-compose.nginx.yml)).
La imagen de nginx la genera al arrancar sustituyendo `EVEY_DOMAIN`: los
certificados deben existir con los nombres de archivo correspondientes. Reenvía a
`evey:9091` con el búfer de streaming desactivado, sin paso de compilación del
frontend.

## Traefik

Consulta [`examples/traefik/docker-compose.traefik.yml`](../examples/traefik/docker-compose.traefik.yml):

```bash
docker compose -f docker-compose.yml -f examples/traefik/docker-compose.traefik.yml up -d
```

## Nginx Proxy Manager

El contenedor evey debe estar en la red que usa NPM (consulta "Detrás de un proxy
que ya tienes" más arriba). Después, **Edit Proxy Host**:

1. **Details**: Scheme `http`, Forward Hostname `evey-server`, Forward Port
   `9091`. **Websockets Support: on** (SyncPlay). **Cache Assets: off**: guardaría
   en caché los segmentos y listas de reproducción con una caducidad fija. Block
   Common Exploits puede quedarse activado (desactívalo si una búsqueda o la
   subida de un plugin responde alguna vez 403 sin motivo).
2. **SSL**: tu certificado, Force SSL, HTTP/2. HSTS es opcional aquí: la
   aplicación ya lo envía.
Eso es todo: NPM reenvía por sí solo `X-Forwarded-For` / `X-Forwarded-Proto` y la
cabecera Host, y sus tiempos de espera por defecto nunca saltan en los flujos de
Evey (consulta la sección de streaming de arriba).

**Opcional**: evitar que NPM almacene los archivos en reproducción directa en su
propio disco: **Advanced** → *Custom Nginx Configuration* (la pestaña `Custom Locations`
no admite la expresión regular):

```nginx
# Subidas de plugins (hasta 100 MB) y restauraciones de copias (hasta 2 GB).
client_max_body_size 2G;

# Streaming: sin búfer, sin tiempo de espera.
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

# Los escaneos de biblioteca transmiten su progreso mientras dura el escaneo.
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
SyncPlay) pasan por el bloque de proxy propio de NPM, que configura la pestaña
Details. Si omites el fragmento no se rompe nada: solo cambia dónde se almacenan
los archivos grandes.

## Direcciones reales de los clientes y el bloqueo del acceso remoto

En cuanto tu proxy reenvía la dirección real del cliente, Evey distingue a un
espectador en casa de uno en internet. Sobre eso actúan el límite de bitrate WAN
y el bloqueo del acceso remoto, así que ambos ajustes solo se comportan bien
juntos:

- Los proxies de confianza (panel o `EVEY_TRUSTED_PROXIES`) deben cubrir el proxy
  (`172.16.0.0/12` cuando es un contenedor en la red de Docker; nada que añadir
  cuando corre en el equipo, el loopback siempre es de confianza). Lo que definas
  se AÑADE a la base del loopback, nunca la sustituye, así que un valor de Docker
  no puede impedir en silencio que el bloqueo vea las direcciones reales.
- Cada proxy de una cadena debe conservar `X-Forwarded-For`. En Caddy es
  `servers { trusted_proxies static private_ranges }`; nginx usa
  `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`.

**Activarlo cambia quién cuenta como remoto.** Antes, cada petición parecía venir
de la dirección privada del propio proxy, así que todo el mundo se clasificaba
como LAN. Después, los clientes remotos reales se identifican correctamente, y si
el acceso remoto está desactivado se rechazan en todas las rutas
(`403 {"code":"remote_access_disabled"}`), aunque no haya cambiado ningún ajuste
de la aplicación.

Evey se protege contra ello: el acceso remoto se activa por defecto cuando hay una
URL pública configurada y no se ha tomado ninguna decisión explícita, el estado se
registra en cada arranque, los rechazos se registran con la IP del cliente,
`evey admin doctor` falla de forma visible ante "publicado pero rechazando", y el
panel de administración muestra un aviso. Si cambias los ajustes de confianza del
proxy, comprueba `evey admin doctor` después.
