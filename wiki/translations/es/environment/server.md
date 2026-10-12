---
source_sha: 1632ea5df6d8e43d0a8895c8efa853e83316a37433cb8b556d00d317a9ee4951
reviewed: true
---
# Servidor y red

Las variables que dicen dónde se publica el servidor, de qué proxies se fía,
quién puede llegar a él desde fuera y cuántas peticiones acepta. La mayoría
se definen una sola vez en el archivo compose; lo que guarda después
**Administración → Acceso remoto** tiene prioridad sobre ellas, como explica
la [Referencia de configuración](../configuration.md).

## Dirección y puertos

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_SERVER_BASE_URL` | `http://localhost:9091` | La dirección pública. Una dirección `https://` activa las cookies `Secure`, HSTS y la confianza en `X-Forwarded-Proto`. Los enlaces de los correos, las redirecciones del inicio de sesión único y las URL de envío a Chromecast se construyen a partir de ella. | Acceso remoto → **URL externa** gana si está definida, inmediato; una URL externa en `https://` también activa HTTPS |
| `EVEY_SERVER_PORT` | `9090` | El puerto gRPC interno. La API HTTP y la interfaz web escuchan en este puerto más uno (`9091`). | ninguno |
| `EVEY_GRPC_BIND` | `127.0.0.1` | La dirección en la que escucha el puerto gRPC. El control de acceso remoto no cubre gRPC, así que abrirlo más (`0.0.0.0`) escribe un aviso en el registro y te deja a ti la tarea de protegerlo con un cortafuegos. | ninguno |
| `EVEY_SERVER_NAME` | `Evey` | El nombre que aparece en la página de inicio de sesión, en la lista de servidores de las aplicaciones y en los correos. | Configuración del servidor → Configuración del servidor → **Nombre del servidor**, inmediato |

## Proxy inverso

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_TRUSTED_PROXIES` | solo la interfaz local | Redes cuya dirección de cliente reenviada se cree, separadas por comas (para un proxy en un contenedor: `172.16.0.0/12`, que puedes acotar cuando la conozcas). Se suman a la interfaz local, nunca la sustituyen; una entrada mal escrita impide el arranque. | Acceso remoto → **Proxies de confianza (IP o CIDR, separados por comas)** sustituye esta lista, inmediato; vaciar el campo vuelve a aplicar la variable |
| `EVEY_REAL_IP_HEADER` | `X-Forwarded-For` | La cabecera en la que un proxy de confianza pone la dirección del cliente. | ninguno |
| `EVEY_FORWARDED_PROTO` | deducido | Obsoleta: HTTPS se deduce del esquema de `EVEY_SERVER_BASE_URL`. Se sigue aceptando para forzar el valor, con un aviso. | ninguno |

Qué ajustes del proxy importan para la reproducción:
[Configuraciones de proxy inverso](../reverse-proxy.md).

## Acceso remoto

Estas variables sirven de punto de partida en el primer arranque; después,
**Administración → Acceso remoto** guarda sus propios valores y los aplica
sin reiniciar. Cómo decide el control de acceso:
[Acceso remoto y HTTPS](../administration/remote-access.md).

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_REMOTE_ACCESS_ENABLED` | sin definir | Sin definir: se deduce al arrancar, activo cuando `EVEY_SERVER_BASE_URL` o `EVEY_REMOTE_ACCESS_EXTERNAL_URL` es pública. `true` o `false`: tu elección explícita, nunca se cambia por ti. | Acceso remoto → **Activar acceso remoto**, inmediato |
| `EVEY_REMOTE_ACCESS_EXTERNAL_URL` | vacía | La dirección pública, cuando es distinta de `EVEY_SERVER_BASE_URL`. Una dirección `https://` activa el modo HTTPS, igual que la URL base. | Acceso remoto → **URL externa**, inmediato |
| `EVEY_REMOTE_ACCESS_LAN_NETWORKS` | `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16` | Redes que se consideran de casa. Añade un rango de VPN o de CGNAT (`100.64.0.0/10`) cuyos clientes deban contar como locales. | Acceso remoto → **Redes LAN (CIDR, separadas por comas)**, inmediato |
| `EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS` | `0` | Un tope de bitrate para los clientes de fuera de casa, en kbit/s; `0` = sin tope. Una dirección privada nunca tiene tope. | Acceso remoto → **Bitrate remoto máximo (Kbps, 0 = ilimitado)**, inmediato |

## Límites de peticiones

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_RATELIMIT_LOGIN_MAX` | `10` | Intentos de inicio de sesión por dirección y por minuto. | Configuración del servidor → Configuración del servidor → **Límite de inicio de sesión (sol./min)**, inmediato |
| `EVEY_RATELIMIT_GLOBAL_MAX` | `300` | Peticiones a la API por dirección y por minuto. | Configuración del servidor → Configuración del servidor → **Límite global (sol./min)**, inmediato |

## Usuario del contenedor

Las lee el punto de entrada, no el servidor.

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `PUID`, `PGID` | propietario de los montajes `/media` | El usuario y el grupo con los que se ejecuta el servidor. `1001` se rechaza (reservado para los plugins). Detalles en la [Referencia de configuración](../configuration.md#file-ownership-puid-and-pgid). | ninguno |

## Demo pública

| Variable | Por defecto | Qué hace | Panel |
| --- | --- | --- | --- |
| `EVEY_DEMO` | `false` | Ejecuta el servidor como la demo pública: los cambios de administración quedan bloqueados, sin correos, sin notificaciones push ni webhooks salientes. No es para una instalación real. | ninguno |

<!-- sources
internal/config/config.go - EVEY_SERVER_BASE_URL, EVEY_SERVER_PORT, EVEY_GRPC_BIND, EVEY_SERVER_NAME, EVEY_REAL_IP_HEADER, EVEY_RATELIMIT_LOGIN_MAX, EVEY_RATELIMIT_GLOBAL_MAX, EVEY_REMOTE_ACCESS_*, EVEY_DEMO, forwardedProtoFromEnv, EffectiveBaseURL
trusted proxies: internal/config/trusted_proxies.go (loopback baseline, malformed entry is an error) ; cmd/evey/admin_ops.go
remote access inference: internal/config/remote_access_resolve.go ; cmd/evey/main.go (RemoteAccessConfigKey loaded from DB)
panel: internal/adapters/driving/http/settings_handler.go (server.name, ratelimit.* live via SetLimit) ; i18n admin.remoteAccess.*, admin.serverSettings.*
puid/pgid: docker/entrypoint.sh
-->
