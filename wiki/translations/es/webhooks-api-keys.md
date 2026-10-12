---
source_sha: 09be615f048b42a5c91430ee4558b2bc906d1e19a1f0c51eb2577ffa9d579533
reviewed: true
---
# Webhooks y claves de API

Dos maneras de conectar Evey con otras herramientas sin escribir un plugin:
un webhook envía los eventos del servidor a una URL tuya, y una clave de API
permite que un script llame al servidor en tu nombre. Los dos los crea un
administrador en el panel de administración; esta página es para quien los
configura y para quien escribe el receptor o el script.

## Enviar eventos a una URL

**Administración → Webhooks → Crear webhook**: un **Nombre**, una **URL**,
los **Eventos** que se envían y un **Secreto** opcional para firmar.
**Probar** envía un ping y muestra la respuesta.

<figure class="evey-shot" markdown="span">
  ![Un webhook en Administración → Webhooks](images/admin-webhooks.webp){ loading=lazy }
  <figcaption>Un webhook que envía el inicio y el final de cada reproducción (interfaz en inglés).</figcaption>
</figure>

**Eventos.** Un webhook se suscribe a **Todos los eventos** o a una lista
elegida. El formulario ofrece todos los eventos que publica el servidor;
entre ellos:

| Área | Eventos |
| --- | --- |
| Biblioteca | `library.scan.started`, `library.scan.completed`, `media.item.added`, `media.item.updated`, `media.item.removed`, `metadata.refreshed` |
| Reproducción | `playback.started`, `playback.stopped`, `media.marked.watched`, `transcode.started`, `transcode.failed` |
| Cuentas | `user.created`, `user.login.succeeded`, `user.login.failed`, `user.profile.created`, `user.apikey.created` |
| Solicitudes | `media.request.created`, `media.request.answered` |
| Servidor | `backup.created`, `backup.failed`, `plugin.installed`, `plugin.error`, `server.config.changed`, `transcoding.derived_cache_full` (el almacén de subtítulos ha bajado de su margen de espacio libre) |

Un plugin que llama a `EmitEvent` publica `plugin.event.<type>`, que también
recibe un webhook suscrito a **Todos los eventos**.

**Envío.** Cada evento es un `POST` en JSON, con dos cabeceras:

- `X-Evey-Delivery`: un identificador que no cambia entre los reintentos de
  un mismo envío, para que tu receptor pueda ignorar un duplicado;
- `X-Evey-Signature`: `sha256=` seguido del HMAC-SHA256 del cuerpo,
  calculado con el secreto del webhook. Compruébalo con el mismo secreto para
  saber que la petición viene de tu servidor. Sin secreto, la firma no
  demuestra nada.

Cada intento espera una respuesta durante 30 segundos como máximo. Un envío
que falla se reintenta después de 1, 5 y 30 segundos. Cada intento queda
registrado; el historial se consulta por la API (`ListWebhookDeliveries` en
la [API de plugins](api/plugins/README.md)), no en el panel.

!!! warning "Solo direcciones públicas"
    La URL de un webhook debe ser `http` o `https` y apuntar a una dirección
    pública. `localhost`, las direcciones privadas y de enlace local
    (`192.168.x.x`, `10.x.x.x`, `172.16.x.x`...), y un nombre que resuelva a
    una de ellas, se rechazan al guardar el webhook y otra vez cuando el
    servidor se conecta. Así nadie puede usar el servidor para llegar a tu
    red interna; un receptor en tu red local necesita una dirección pública
    o un relé.

## Llamar al servidor desde un script

Una clave de API permite que un script llame a la API del servidor sin
contraseña. Las claves las crea un administrador en
**Administración → Claves API → Nueva clave API**, y pertenecen a ese
administrador.

- **La clave se muestra una sola vez**, al crearla. Guárdala en ese momento;
  no se puede volver a mostrar, solo revocar con **Revocar**.
- **Actúa como su dueño**, con derechos de administrador, dentro de sus
  alcances.
- Los **Alcances** de la clave limitan lo que puede hacer:

| Alcance | Permite |
| --- | --- |
| `media:read` | Leer los metadatos de los medios |
| `media:write` | Modificar los medios y lanzar escaneos |
| `playback:read` | Leer el estado y el historial de reproducción |
| `playback:write` | Controlar la reproducción y el progreso |
| `users:read` | Leer los usuarios |
| `users:write` | Modificar los usuarios |
| `admin:read` | Leer la actividad y las estadísticas de administración |
| `admin:write` | Modificar los ajustes de administración |

Una clave creada sin ningún alcance (**Todos (heredado)**) tiene acceso
completo a los métodos que no son de administración, como su dueño, pero los
métodos de administración siempre exigen su alcance: una clave heredada no
puede llegar a ellos. Elige los alcances que necesita el script.

La clave va en la cabecera `Authorization`, con o sin `Bearer`:

```bash
curl -H "Authorization: Bearer evey_..." https://media.example.com/api/v1/...
```

Una clave deja de funcionar en el acto cuando se revoca, o cuando se
desactiva a su dueño. Los endpoints en sí están en la
[API del servidor](api/README.md), junto con las otras formas de
autenticarse.

<!-- sources
form labels: i18n admin.webhooks.form.* (name, url, secret, events, allEvents), admin.webhooks.createWebhook, common.test, admin.webhooks.testPingSent
event list: frontend/packages/client/src/lib/webhookEvents.ts - WEBHOOK_EVENT_TYPES ; EventName() in internal/domain ; plugin events: internal/adapters/driving/pluginhost/host_server.go - pluginEmittedEvent.EventName
delivery, headers, timeout, retries, signature: internal/app/webhook/service.go - handleAny, attemptDeliver, sign, defaultRetryDelays, deliveryTimeout ; internal/domain/webhook/model.go - MatchesEvent ; history: proto/evey/v1/webhook.proto - ListWebhookDeliveries
public addresses only: internal/pkg/urlvalidator/validator.go - ValidateWebhookURL, isPrivateIP ; internal/pkg/ssrf/dialer.go
api keys: internal/adapters/driving/grpcapi/interceptors/auth.go - authenticateAPIKey (Bearer optional, evey_ prefix), authorize (admin methods always enforce the scope) ; internal/pkg/auth/rbac.go (scopes) ; rbac.go - CreateAPIKey RoleAdmin ; i18n auth.apiKeys.* (newKeyTitle, scopesAll, saveWarning, revoke)
-->
