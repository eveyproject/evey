---
source_sha: af82a75f9598046f40d3b3b9257d529cb681628d24fcf9fab49c54919727e7fd
---

# Proxy inverso

Evey sirve **tanto la API como la interfaz web** en el puerto **9091** (HTTP sin
cifrar). Al proxy inverso le basta con terminar TLS y reenviarlo todo a
`evey:9091`: **no hay ningún frontend aparte que compilar ni servir**.

Sea cual sea el proxy que utilice, defina estas variables para que Evey conozca
su URL pública y confíe en las cabeceras que el proxy le reenvía:

```env
EVEY_SERVER_BASE_URL=https://media.example.com
EVEY_TRUSTED_PROXIES=172.16.0.0/12       # solo hace falta si el proxy es un contenedor; loopback siempre es de confianza
```

Una URL base en `https://` es lo que le indica a Evey que hay un proxy
terminando TLS por delante: las cookies reciben el atributo `Secure`, se envía
HSTS y se respeta la cabecera `X-Forwarded-Proto` procedente de un proxy de
confianza. No hace falta ningún interruptor aparte (la antigua
`EVEY_FORWARDED_PROTO` se sigue aceptando como anulación, pero está obsoleta).

## Streaming: qué le hacen los valores por defecto de un proxy

Evey funciona detrás de un proxy inverso sin tocarle la configuración de
fábrica: toda respuesta de larga duración o bien envía bytes de forma continua
(segmentos HLS, reproducción directa, salida de la transcodificación) o bien
avisa al cliente cada 30 s (los flujos de eventos), de modo que un tiempo de
espera de lectura por defecto nunca llega a saltar. Aun así, conviene saber dos
cosas:

- **Búfer.** Un proxy que almacena las respuestas en búfer (nginx lo hace por
  defecto) carga el archivo de reproducción directa en sus propios búferes y
  archivos temporales antes de entregárselo al cliente: funciona, pero una
  película de varios GB pasa entonces por el disco de la máquina del proxy.
  `proxy_buffering off` en las rutas de streaming lo evita.
- **Análisis de biblioteca.** `/api/v1/libraries/*/scan` transmite el progreso
  durante todo el tiempo que dure el análisis; un proxy que limite la duración
  total de una respuesta (algo poco frecuente: ni nginx ni Caddy lo hacen por
  defecto) lo cortaría antes de tiempo. El análisis en sí sigue adelante en el
  servidor de todos modos.

Las rutas afectadas:

```
/api/v1/media/*/hls/*
/api/v1/media/*/stream
/api/v1/libraries/*/scan
```

El Caddy incluido y los ejemplos de Nginx y Traefik que vienen a continuación
aplican ambas cosas (sin búfer y sin tiempo de espera) a esas rutas. Con un
proxy que se administra desde una interfaz gráfica, como Nginx Proxy Manager,
los valores por defecto bastan y el ajuste queda en un fragmento opcional.

## Cabeceras de seguridad y CSP: léalo si su proxy define las suyas

Evey emite una Content-Security-Policy completa. Si su proxy la deja pasar tal
cual, no hay nada más que hacer.

> Hasta el 2026-07-23 esa frase era falsa: a la política de la aplicación le
> faltaban `media-src` y `worker-src`, así que `blob:` recaía en
> `default-src 'self'` y la reproducción MSE se rompía para todo aquel que la
> dejara pasar. Pasó desapercibido porque la configuración de Caddy incluida
> elimina la cabecera de la aplicación y compone la suya. Si utiliza una versión
> anterior a esa detrás de Nginx Proxy Manager o de Traefik y el reproductor no
> arranca, la causa es esta: componga en el proxy la política que aparece más
> abajo, o actualice.

Muchas instalaciones prefieren eliminarla (`header_down -Content-Security-Policy`
en Caddy) y componer la suya propia. No hay problema, pero entonces **su**
política tiene que conservar estos valores o algunas funciones dejarán de
funcionar sin previo aviso:

| Valor | Directiva | Qué se rompe sin él |
| --- | --- | --- |
| `'unsafe-inline'` | `script-src` | Todas las interfaces de extensión (iframe en blanco). El paquete de una extensión se descarga con el token del usuario y se inserta en un iframe `srcdoc`, y un documento srcdoc hereda la política de quien lo incrusta. Entretanto, lo que acota a la extensión es el `sandbox="allow-scripts"` del iframe (origen opaco, sin acceso al DOM ni a las cookies del padre). |
| `'wasm-unsafe-eval'` | `script-src` | La reproducción de AC-3/E-AC-3 en el navegador (el descodificador WebAssembly ac3go no puede compilarse → la película se ve sin sonido). Es el valor concreto que permite únicamente compilar Wasm, NO el `eval()` general. |
| `https://www.gstatic.com` | `script-src` (y `frame-src`) | El SDK emisor de Google Cast no llega a cargarse → nadie ve el botón de emisión. Es la única forma que tiene un navegador de descubrir Chromecasts en la red local del usuario. |
| `blob:` | `media-src` | La reproducción MSE (hls.js engancha el flujo mediante una URL blob). |
| `blob:` | `worker-src` | hls.js pasa sin avisar a demultiplexar en el hilo principal (reproducción más entrecortada en el móvil). |

Una política probada para un proxy que compone la suya:

```
default-src 'self'; base-uri 'self'; object-src 'none';
img-src 'self' https: data: blob:; media-src 'self' blob:;
style-src 'self' 'unsafe-inline';
script-src 'self' 'unsafe-inline' 'wasm-unsafe-eval' https://www.gstatic.com;
connect-src 'self' https: wss:; worker-src 'self' blob:; font-src 'self' data:;
frame-src https://www.gstatic.com
```

`base-uri` y `object-src` no recaen en `default-src`, así que una política que
los omita deja sin restricción la inyección de `<base href>` y la incrustación
de `<object>` y `<embed>`. Consérvelos si compone su propia política.

Reglas prácticas:

- **Al navegador debe llegar exactamente una cabecera CSP.** Si llegan dos (por
  ejemplo la de la aplicación Y la de su proxy, o la de Caddy Y el fragmento
  «Advanced» de Nginx Proxy Manager), los navegadores aplican la
  *intersección*: gana la más estricta y el fallo es silencioso. Compruébelo con
  `curl -sI https://your.domain | grep -ci content-security-policy`
  → tiene que imprimir `1`.
- **Mantenga la política en UNA sola línea.** Una CSP repartida en varias líneas
  dentro de la configuración de un proxy es una fuente clásica de corrupción
  silenciosa: al unirlas se pierden los caracteres de la juntura (una vez
  `...gstatic.com; connect-src` quedó como `...gstaticonnect-src`, lo que
  desactivó el SDK de Cast y fusionó dos directivas).
- `'unsafe-inline'` en `script-src` tiene un coste real y hoy por hoy resulta
  imprescindible (las interfaces de extensión, véase la tabla). Quitarlo solo es
  seguro en una instalación que no ejecute ningún frontend de extensión.

Todos los ejemplos incluidos leen el dominio de **un único sitio**:
`EVEY_DOMAIN`, en el archivo `.env`. Defínalo una vez y Caddy, Nginx y Traefik lo
recogerán.

```env
EVEY_DOMAIN=media.example.com
```

## Caddy (incluido)

El repositorio trae [`docker/Caddyfile`](../docker/Caddyfile) (del que
[`docker/Dockerfile.caddy`](../docker/Dockerfile.caddy) construye una imagen):
obtiene HTTPS automático para `EVEY_DOMAIN` y reenvía todo a `evey:9091`, con
las excepciones de streaming ya puestas. También compone las cabeceras de
seguridad y la CSP canónicas de la sección anterior (y elimina la CSP de la
propia aplicación, para que al navegador llegue exactamente una). Añada un
servicio `caddy` a su compose (véase el servicio `caddy` del
[`docker-compose.yml`](../docker-compose.yml) del repositorio).

### Detrás de un proxy que ya tiene en marcha (TLS terminado aguas arriba)

En un NAS que ya cuenta con un proxy inverso (el del fabricante, Nginx Proxy
Manager, un Traefik en una red Docker compartida) **no hace falta Caddy**: Evey
emite sus propias cabeceras de seguridad y una CSP completa, y las reglas de
streaming solo puede aplicarlas el proxy que da la cara al cliente, de manera
que un segundo proxy por detrás no aporta nada. Ponga el servicio evey en la red
de ese proxy y apunte a `evey-server:9091`:

```yaml
services:
  evey:
    # sin ports: el proxy es la única vía de entrada
    environment:
      EVEY_SERVER_BASE_URL: https://media.example.com
      EVEY_TRUSTED_PROXIES: 172.16.0.0/12     # restrínjalo a la subred del proxy cuando la conozca
    # ...
networks:
  default:
    name: proxy-net
    external: true
```

Después, en ese proxy y para la ruta hacia `evey-server:9091`: reenvíe
`X-Forwarded-For` y `X-Forwarded-Proto`, active la compatibilidad con WebSocket
y aplique las reglas de streaming de más arriba (sin búfer y sin tiempo de
espera de lectura ni de escritura en las rutas de streaming). Las secciones de
Nginx, Traefik y Nginx Proxy Manager que siguen muestran las líneas exactas.

## Nginx

Utilice [`examples/nginx/evey.conf.template`](../examples/nginx/evey.conf.template)
(hay un compose listo para usar en
[`examples/nginx/docker-compose.nginx.yml`](../examples/nginx/docker-compose.nginx.yml)).
La imagen de nginx lo genera al arrancar sustituyendo `EVEY_DOMAIN`; los
certificados deben existir con los nombres de archivo correspondientes. Reenvía
a `evey:9091` con el búfer de streaming desactivado, sin ninguna etapa de
compilación del frontend.

## Traefik

Consulte [`examples/traefik/docker-compose.traefik.yml`](../examples/traefik/docker-compose.traefik.yml):

```bash
docker compose -f docker-compose.yml -f examples/traefik/docker-compose.traefik.yml up -d
```

## Nginx Proxy Manager

El contenedor evey tiene que estar en la red que utiliza NPM (véase «Detrás de
un proxy que ya tiene en marcha», más arriba). Después, en **Edit Proxy Host**:

1. **Details**: Scheme `http`, Forward Hostname `evey-server`, Forward Port
   `9091`. **Websockets Support: on** (para SyncPlay). **Cache Assets: off**,
   porque guardaría en caché los segmentos multimedia y las listas de
   reproducción con una caducidad fija. Block Common Exploits puede quedarse
   activado (desactívelo si alguna búsqueda o la subida de una extensión
   responde 403 sin motivo aparente).
2. **SSL**: su certificado, Force SSL y HTTP/2. Aquí HSTS es opcional, ya que la
   aplicación lo envía por su cuenta.
Y con eso basta: NPM reenvía por sí solo `X-Forwarded-For`, `X-Forwarded-Proto`
y la cabecera Host, y sus tiempos de espera por defecto nunca llegan a saltar
con los flujos de Evey (véase la sección sobre streaming, más arriba).

**Opcional**: evitar que NPM haga pasar por su propio disco los archivos de
reproducción directa. Vaya a **Advanced** → *Custom Nginx Configuration* (la
pestaña `Custom Locations` no permite expresar la expresión regular):

```nginx
# Subida de extensiones, restauración de copias de seguridad.
client_max_body_size 100M;

# Streaming: sin búfer y sin tiempo de espera.
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

# Los análisis de biblioteca transmiten su progreso mientras dura el análisis.
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

El resto de rutas (la interfaz web, el resto de la API, el WebSocket de
SyncPlay) pasan por el bloque proxy propio de NPM, el que configura la pestaña
Details. Si omite el fragmento no se rompe nada: lo único que cambia es dónde se
almacenan en búfer los archivos grandes.

## Las IP reales de los clientes y el filtro de acceso remoto

En cuanto el proxy reenvía la dirección real del cliente, Evey puede distinguir
a un espectador que está en casa de otro que está en internet. Sobre esa
distinción actúan el límite de tasa de bits en WAN y el filtro de acceso remoto,
por lo que ambos ajustes solo se comportan como es debido si van juntos:

- `EVEY_TRUSTED_PROXIES` tiene que cubrir el proxy (`172.16.0.0/12` cuando es un
  contenedor de la red Docker; nada que añadir cuando se ejecuta en el host, ya
  que loopback siempre es de confianza). Lo que indique se SUMA a la base
  loopback, nunca la sustituye, así que un valor pensado para Docker no puede
  impedir en silencio que el filtro vea las direcciones reales de los clientes.
- Si encadena varios proxies, cada uno debe conservar `X-Forwarded-For`. En
  Caddy se hace con `servers { trusted_proxies static private_ranges }`; nginx
  usa `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`.

**Activar esto cambia quién cuenta como remoto.** Antes, todas las peticiones
parecían venir de la dirección privada del propio proxy, así que todo el mundo
quedaba clasificado como LAN. Después, los clientes remotos de verdad se
identifican correctamente y, si el acceso remoto está desactivado, se les
rechaza en todas las rutas (`403 {"code":"remote_access_disabled"}`). Esa misma
secuencia dejó a toda una casa fuera de un servidor que funcionaba durante dos
días: cambió la configuración del proxy, no cambió ningún ajuste de la
aplicación y el único síntoma visible fue que los clientes fallaban sin ningún
error del lado del servidor.

Evey ahora se protege de ello: el acceso remoto queda activado por defecto en
cuanto hay una URL pública configurada, el estado se registra en cada arranque,
los rechazos se registran junto con la IP del cliente, `evey admin doctor` falla
de forma bien visible ante un «publicado pero rechazando» y el panel de
administración muestra un aviso. Si modifica los ajustes de confianza del proxy,
ejecute después `evey admin doctor`.
