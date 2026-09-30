---
source_sha: fff4f29a51e1c144db473e7f53efc34660d3fe41ef149263f32a283daf865c54
reviewed: false
---
# Acceso remoto y HTTPS

Llegar al servidor desde fuera de casa implica dos cosas distintas: una dirección
HTTPS pública delante de Evey y el ajuste de acceso remoto del propio Evey, que
decide si se atiende a los clientes de fuera de tu red.

## El acceso remoto se decide a partir de la dirección pública

Evey decide el acceso remoto al arrancar, salvo que lo hayas elegido tú:

- si al servidor se le dio una dirección pública (`EVEY_SERVER_BASE_URL` o
  `EVEY_REMOTE_ACCESS_EXTERNAL_URL`), el acceso remoto está **activado**;
- si no, está **desactivado**, y solo se atiende a los clientes de tus redes
  locales.

En cuanto cambias el ajuste en **Admin → Acceso remoto**, tu elección se
conserva y el servidor deja de deducirla: un servidor publicado puede quedarse
solo en la red local si eso es lo que quieres.

Con el acceso remoto desactivado, un cliente de fuera de las redes locales recibe
un `403` en cada página, y las aplicaciones muestran un mensaje que dice que el
acceso remoto está desactivado en lugar de un error genérico. Si el servidor es
accesible desde internet mientras el acceso remoto está desactivado, la página de
administración muestra **Todos los usuarios remotos están bloqueados**, el
servidor lo registra al arrancar y registra cada rechazo con la dirección del
cliente, y `evey admin doctor` lo informa como un fallo.

El acceso remoto es una elección sobre quién puede llegar al servidor, no una
protección: cada página sigue exigiendo iniciar sesión.

## Ajustes

**Admin → Acceso remoto**. Cada ajuste se aplica sin reiniciar: los de red a la
siguiente petición, el bitrate a la siguiente reproducción.

<figure class="evey-shot" markdown="span">
  ![Admin → Acceso remoto con una URL externa HTTPS y un proxy de confianza](../images/admin-remote-access.webp){ loading=lazy }
  <figcaption>Un servidor publicado en una dirección <code>https://</code> detrás de un proxy en la red de Docker (interfaz en inglés).</figcaption>
</figure>

| Ajuste | Efecto |
| --- | --- |
| **Activar acceso remoto** | Activado: los clientes de fuera de las redes locales se atienden como cualquier otro. Desactivado: reciben un `403`. |
| **URL externa** | La dirección pública. Los enlaces de los correos, las redirecciones del inicio de sesión único y las URL de transmisión se construyen a partir de ella. Una dirección `https://` activa además el modo HTTPS (abajo). |
| **Proxies de confianza** | Los proxies inversos cuya dirección de cliente reenviada cree Evey. El loopback siempre es de confianza. Vacío: la lista viene de `EVEY_TRUSTED_PROXIES`. |
| **Redes LAN** | Los rangos de direcciones considerados como casa. Los rangos privados vienen por defecto. Añade aquí un rango de VPN (Tailscale, WireGuard) si sus clientes deben contar como de casa. |
| **Bitrate remoto máximo** | Un límite de bitrate para los clientes de fuera de las redes locales, en kbit/s (0 = ninguno). Una dirección privada nunca se limita. Cuando **Admin → Transcodificación** fija también un límite para los espectadores remotos, se aplica el menor de los dos. Consulta [Cómo elige Evey el modo de reproducción](../playback/how-playback-is-decided.md#limits-on-the-session). |

## HTTPS: un proxy inverso delante de Evey

Evey sirve HTTP sin cifrar en el puerto 9091, API e interfaz web juntas. HTTPS es
tarea de un proxy inverso, que reenvía todo a `evey-server:9091`. Evey necesita
saber dos cosas de él, ambas en **Admin → Acceso remoto**:

- **URL externa** en `https://`. Es lo que le dice a Evey que un proxy termina
  TLS: las cookies reciben la marca `Secure`, se envía HSTS y se cree la cabecera
  `X-Forwarded-Proto` de un proxy de confianza.
- **Proxies de confianza**, cuando el proxy no está en la misma máquina que Evey.
  Un proxy en otro contenedor necesita aquí su red, por ejemplo `172.16.0.0/12`
  para las redes por defecto de Docker. Se rechaza un rango que cubra todas las
  direcciones (`0.0.0.0/0`, `::/0`): cualquier cliente podría elegir entonces la
  dirección con la que se le ve.

Sin proxy todavía, el repositorio de Evey incluye un servicio Caddy que obtiene
por sí solo un certificado de Let's Encrypt. Configuraciones listas para Caddy,
Nginx, Traefik y Nginx Proxy Manager, y lo que los valores por defecto de un
proxy hacen al streaming: [Configuraciones de proxy inverso](../reverse-proxy.md).

### Definirlo en el entorno

`EVEY_SERVER_BASE_URL` y `EVEY_TRUSTED_PROXIES` dan las mismas respuestas desde
el archivo compose, lo que conviene a una instalación descrita por completo como
código o a un servidor que debe ser accesible por HTTPS desde su primer arranque.
El panel prevalece cuando ambos están definidos: una URL externa `https://` activa
el modo HTTPS aunque la URL base sea HTTP sin cifrar, y una lista de proxies
guardada sustituye a la variable. Vaciar el campo devuelve la respuesta a la
variable. Consulta [Servidor y red](../environment/server.md).

## Los dos pasos de red

Los proxies de confianza y las redes LAN son dos pasos sucesivos, no dos formas
de decir lo mismo. Los proxies de confianza deciden de quién se cree la
dirección reenviada, y por tanto qué dirección recibe cada cliente; las redes
LAN clasifican después esa dirección como local o remota.

## La dirección real del cliente

El acceso remoto y el **Bitrate remoto máximo** dependen de que Evey vea la
dirección real de cada cliente. Un proxy que no la reenvía, o en el que Evey no
confía, hace que cada petición parezca venir de la dirección privada del proxy:
todo el mundo cuenta como de casa, el ajuste de acceso remoto no tiene efecto y
no se aplica ningún límite remoto.

Arreglar el proxy cambia por tanto quién cuenta como remoto. Si el acceso remoto
estaba desactivado, los clientes remotos se rechazan a partir de ese momento.
Comprueba `evey admin doctor` tras cambiar los ajustes del proxy: indica la lista
vigente y de dónde viene, avisa de un rango público y falla ante un rango que
cubra todas las direcciones.
