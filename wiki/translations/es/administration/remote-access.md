---
source_sha: b8448a636921ec7091b9ab98200600dd596b087d5b5d02b1d71230f1dbda6d3d
reviewed: true
---
# Acceso remoto y HTTPS

En casa, Evey funciona en una dirección como `http://192.168.1.20:9091`.
Para ver contenido desde fuera de casa, necesitas dos cosas más:

- **Una dirección pública**, por ejemplo `https://evey.example.org`: un
  nombre de dominio que lleva hasta tu casa.
- **HTTPS**, para que las contraseñas y los vídeos viajen cifrados por
  internet. Evey no hace HTTPS por sí mismo. Lo hace un **proxy inverso**: un
  pequeño programa colocado delante de Evey que se encarga de la conexión
  segura y le pasa cada petición a Evey.

Esta página explica cómo publicar el servidor y después qué hace cada
ajuste.

## Publicar el servidor en tres pasos

1. **Monta un proxy inverso** que se encargue de HTTPS y lo reenvíe todo al
   puerto `9091` de Evey. Puedes usar la configuración de Caddy que viene
   con Evey, o un proxy que ya tengas funcionando. Configuraciones listas
   para usar: [Configuraciones de proxy inverso](../reverse-proxy.md).
2. **Díselo a Evey** en **Administración → Acceso remoto**:
    - **URL externa**: tu dirección pública en `https://`.
    - **Proxies de confianza (IP o CIDR, separados por comas)**: solo cuando
      el proxy funciona en otro contenedor. Pon ahí su red, por ejemplo
      `172.16.0.0/12` para las redes por defecto de Docker.
3. **Eso es todo.** En cuanto tiene una dirección pública, el servidor abre
   el acceso remoto por sí solo.

Para comprobarlo, ejecuta `docker exec evey-server evey admin doctor`.
Muestra los proxies de confianza en uso, y avisa cuando un servidor
publicado está rechazando a la gente de fuera.

## El acceso remoto se decide según la dirección pública

Salvo que lo elijas tú, Evey decide al arrancar:

- **El servidor tiene una dirección pública** (**URL externa**, o las
  variables `EVEY_SERVER_BASE_URL` o `EVEY_REMOTE_ACCESS_EXTERNAL_URL`): el
  acceso remoto está **activado**.
- **No tiene ninguna**: el acceso remoto está **desactivado**. Solo pueden
  conectarse los dispositivos de la red de tu casa.

En cuanto cambias tú mismo **Activar acceso remoto**, Evey respeta tu
elección y deja de decidir por ti. Así, un servidor publicado puede quedarse
solo para casa si lo prefieres.

El acceso remoto solo decide quién puede llegar al servidor. No es una
protección por sí mismo: todas las páginas siguen pidiendo iniciar sesión.

**Cuando el acceso remoto está desactivado**, alguien de fuera de tu casa es
rechazado en todas las páginas (un error `403`). Las apps dicen **Acceso
remoto desactivado** en lugar de un error poco claro. Si el servidor es
accesible desde internet mientras el acceso remoto está desactivado, Evey te
avisa en varios sitios:

- **Administración → Acceso remoto** muestra **Todos los usuarios remotos
  están bloqueados**;
- el registro del servidor lo dice al arrancar, y después anota cada rechazo
  con la dirección de la que vino;
- `evey admin doctor` lo marca como un fallo.

## Ajustes

Todo está en **Administración → Acceso remoto**. Todos los ajustes se
aplican sin reiniciar: los de red desde la siguiente petición, el bitrate
desde la siguiente reproducción.

<figure class="evey-shot" markdown="span">
  ![Administración → Acceso remoto con una URL externa en HTTPS y un proxy de confianza](../images/admin-remote-access.webp){ loading=lazy }
  <figcaption>Un servidor publicado en una dirección <code>https://</code> detrás de un proxy en la red de Docker (interfaz en inglés).</figcaption>
</figure>

| Ajuste | Qué hace |
| --- | --- |
| **Activar acceso remoto** | Activado: la gente de fuera de la red de tu casa puede conectarse como cualquier otra persona. Desactivado: se la rechaza (`403`). |
| **URL externa** | Tu dirección pública. La usan los enlaces de los correos, el inicio de sesión único y el envío a otros dispositivos (cast). Una dirección `https://` también activa el modo HTTPS (más abajo). |
| **Proxies de confianza (IP o CIDR, separados por comas)** | Los proxies inversos en los que Evey confía para indicar quién es el visitante real. La propia máquina (loopback) siempre es de confianza. Vacío: la lista sale de `EVEY_TRUSTED_PROXIES`. |
| **Redes LAN (CIDR, separadas por comas)** | Los rangos de direcciones que Evey considera «casa». Los rangos privados vienen en la lista por defecto. Si usas una VPN (Tailscale, WireGuard) y quieres que sus usuarios cuenten como de casa, añade aquí su rango. |
| **Bitrate remoto máximo (Kbps, 0 = ilimitado)** | Un límite de calidad para la gente de fuera de la red de tu casa. Una dirección privada nunca se limita. Si **Administración → Transcodificación** también fija un límite para los espectadores remotos, se aplica el más bajo. Ver [Cómo se decide la reproducción](../playback/how-playback-is-decided.md#limits-on-the-session). |

## Lo que Evey necesita saber del proxy

Evey responde en HTTP simple en el puerto 9091. El proxy inverso se encarga
de HTTPS. Evey necesita saber dos cosas de él, las dos en **Administración →
Acceso remoto**:

- **URL externa** en `https://`. Esto le dice a Evey que un proxy se encarga
  de HTTPS. Evey protege entonces sus cookies de inicio de sesión (marca
  `Secure`), pide a los navegadores que usen siempre HTTPS (HSTS), y acepta la
  cabecera `X-Forwarded-Proto` de un proxy de confianza.
- **Los proxies de confianza**, cuando el proxy no está en la misma máquina
  que Evey. Un proxy en otro contenedor necesita aquí su red, por ejemplo
  `172.16.0.0/12` para las redes por defecto de Docker.

!!! warning "Se rechaza un rango que cubre todas las direcciones"
    `0.0.0.0/0` o `::/0` como proxy de confianza permitirían a cualquier
    visitante hacerse pasar por alguien de cualquier sitio. El panel y
    `evey admin doctor` lo rechazan. Pon en su lugar la dirección o la red
    del propio proxy.

### Configurarlo en el entorno

Las variables `EVEY_SERVER_BASE_URL` y `EVEY_TRUSTED_PROXIES` hacen lo mismo
desde el archivo compose. Esto encaja con una configuración escrita entera
en archivos, o con un servidor que debe responder por HTTPS desde su primer
arranque.

Cuando están las dos cosas, tiene prioridad el panel. Una **URL externa** en `https://`
activa el modo HTTPS aunque la URL base sea HTTP simple, y una lista de
proxies guardada sustituye a la variable. Vacía el campo para devolverle la
decisión a la variable. Ver
[Servidor y red](../environment/server.md#remote-access).

## La dirección real del cliente

Para que el acceso remoto y el límite de calidad remoto funcionen, Evey tiene
que ver la dirección real de cada visitante. Dos ajustes trabajan juntos:

1. Los proxies de confianza deciden de quién acepta Evey la dirección
   real del visitante.
2. Las redes LAN deciden después si esa dirección es de «casa» o «remota».

Si el proxy no transmite la dirección real, o Evey no confía en él, todas
las peticiones parecen venir de la propia dirección privada del proxy.
Entonces todo el mundo cuenta como de casa: el ajuste de acceso remoto no
tiene efecto y no se aplica ningún límite remoto.

!!! note "Corregir el proxy cambia quién cuenta como remoto"
    Si el acceso remoto estaba desactivado, la gente de fuera es rechazada
    desde el momento en que el proxy transmite su dirección real. Ejecuta
    `evey admin doctor` después de cambiar los ajustes del proxy: muestra la
    lista en uso y de dónde viene, avisa de un rango público, y falla con un
    rango que cubre todas las direcciones.

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
