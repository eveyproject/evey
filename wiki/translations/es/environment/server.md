---
source_sha: d3bbf5c5d256e29ed7bd2108d72166e0906938ea2e32684ab5496cdb6bb7755e
reviewed: false
---
# Servidor y red

La dirección en la que se publica el servidor, en quién confía, quién puede
llegar a él desde fuera y sus límites de peticiones.

## Dirección y puertos

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_SERVER_BASE_URL` | `http://localhost:9091` | La dirección pública. Una dirección `https://` activa las cookies `Secure`, HSTS y la confianza en `X-Forwarded-Proto`. Los enlaces de los correos, las redirecciones del inicio de sesión único y las URL de envío se construyen a partir de ella, salvo que esté definida **Admin → Acceso remoto → URL externa**, que prevalece. Una URL externa `https://` activa HTTPS aunque esta sea HTTP sin cifrar. |
| `EVEY_SERVER_PORT` | `9090` | El puerto gRPC interno. La API HTTP y la interfaz web escuchan en este puerto más uno (`9091`). |
| `EVEY_GRPC_BIND` | `127.0.0.1` | La dirección en la que escucha el puerto gRPC. El bloqueo del acceso remoto no cubre gRPC, así que ampliarla (`0.0.0.0`) registra un aviso y deja el cortafuegos a tu cargo. |
| `EVEY_SERVER_NAME` | `Evey` | El nombre que se muestra en la página de inicio de sesión, en la lista de servidores de las aplicaciones y en los correos. **Admin → Configuración del servidor** lo anula. |

## Proxy inverso

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_TRUSTED_PROXIES` | solo loopback | Redes cuya dirección de cliente reenviada se cree, separadas por comas (para un proxy en un contenedor: `172.16.0.0/12`, acotada cuando se conozca). Se añade al loopback, nunca lo sustituye. Una lista guardada en **Admin → Acceso remoto → Proxies de confianza** sustituye a esta en caliente; vaciar el campo la recupera. |
| `EVEY_REAL_IP_HEADER` | `X-Forwarded-For` | La cabecera en la que un proxy de confianza pone la dirección del cliente. |
| `EVEY_FORWARDED_PROTO` | deducida | Obsoleta: HTTPS se deduce del esquema de `EVEY_SERVER_BASE_URL`. Se sigue aceptando como anulación, con un aviso. |

## Acceso remoto

Estas inicializan el primer arranque; después **Admin → Acceso remoto** guarda
sus propios valores y los aplica sin reiniciar.

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_REMOTE_ACCESS_ENABLED` | sin definir | Sin definir: se deduce al arrancar, activado si `EVEY_SERVER_BASE_URL` o `EVEY_REMOTE_ACCESS_EXTERNAL_URL` es pública. `true` o `false`: tu elección explícita, nunca anulada. |
| `EVEY_REMOTE_ACCESS_EXTERNAL_URL` | vacía | La dirección pública, cuando difiere de `EVEY_SERVER_BASE_URL`. Una dirección `https://` activa el modo HTTPS, como la URL base. |
| `EVEY_REMOTE_ACCESS_LAN_NETWORKS` | `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16` | Redes consideradas como casa. Añade un rango de VPN o CGNAT (`100.64.0.0/10`) cuyos clientes deban contar como locales. |
| `EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS` | `0` | Un límite de bitrate para los clientes de fuera de casa, en kbit/s; `0` = ninguno. Una dirección privada nunca se limita. |

Cómo se comporta el bloqueo: [Acceso remoto y HTTPS](../administration/remote-access.md).

## Límites de peticiones

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_RATELIMIT_LOGIN_MAX` | `10` | Intentos de inicio de sesión por dirección y minuto. **Admin → Configuración del servidor → Configuración del servidor** lo anula. |
| `EVEY_RATELIMIT_GLOBAL_MAX` | `300` | Peticiones por dirección y minuto a la API. Se puede anular en el mismo sitio. |

## Usuario del contenedor

Las lee el entrypoint, no el servidor.

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `PUID`, `PGID` | propietario de los montajes `/media` | El usuario y el grupo con los que se ejecuta el servidor. `1001` se rechaza (reservado para los plugins). Detalles en la [Referencia de configuración](../configuration.md#file-ownership-puid-and-pgid). |

## Demostración pública

| Variable | Por defecto | Efecto |
| --- | --- | --- |
| `EVEY_DEMO` | `false` | Ejecuta el servidor como la demostración pública: las escrituras de administración están bloqueadas, sin correo, push ni webhooks salientes. No para una instalación real. |
