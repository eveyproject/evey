---
source_sha: f8b9121424daaf442c366f75a6973b21de77d280d45f2cb7e7dadab38b7328be
reviewed: false
---
# Webhooks y claves de API

Dos formas de conectar Evey con otras herramientas sin escribir un plugin: los
webhooks envían los eventos de Evey a una URL tuya, y las claves de API permiten
que tus scripts llamen a Evey.

## Webhooks

**Admin → Webhooks → Crear Webhook**: un nombre, una URL, los eventos que enviar
y un secreto opcional. **Probar** envía un ping e informa de la respuesta.

<figure class="evey-shot" markdown="span">
  ![Un webhook en Admin → Webhooks](images/admin-webhooks.webp){ loading=lazy }
  <figcaption>Un webhook que envía el inicio y el final de cada reproducción (interfaz en inglés).</figcaption>
</figure>

**Eventos.** Un webhook se suscribe a todos los eventos o a una lista elegida.
Los eventos son los del propio servidor, por ejemplo:

| Área | Eventos |
| --- | --- |
| Biblioteca | `library.scan.started`, `library.scan.completed`, `media.item.added`, `media.item.updated`, `media.item.removed` |
| Reproducción | `playback.started`, `playback.stopped`, `media.marked.watched`, `transcode.started`, `transcode.failed` |
| Cuentas | `user.created`, `user.login.succeeded`, `user.login.failed`, `user.profile.created` |
| Solicitudes | `media.request.created`, `media.request.answered` |
| Servidor | `backup.created`, `backup.failed`, `plugin.installed`, `plugin.error`, `server.config.changed` |

**Entrega.** Cada evento es un `POST` JSON. Lleva dos cabeceras:

- `X-Evey-Delivery`: un identificador que no cambia entre los reintentos de una
  misma entrega, para que tu receptor pueda ignorar un duplicado;
- `X-Evey-Signature`: `sha256=` seguido del HMAC-SHA256 del cuerpo, calculado con
  el secreto del webhook. Verifícalo con el mismo secreto para saber que la
  petición viene de tu servidor. Sin secreto, la firma no prueba nada.

Una entrega que falla se reintenta al cabo de 1, 5 y 30 segundos. Cada intento se
anota en el registro de entregas del webhook.

**Solo direcciones públicas.** La URL de un webhook debe ser `http` o `https` y
apuntar a una dirección pública. Las direcciones privadas y locales
(`192.168.x.x`, `10.x.x.x`, `localhost`, un nombre de contenedor que resuelve a
una de ellas) se rechazan, al guardar el webhook y de nuevo cuando el servidor se
conecta. Esto impide que el servidor se use para llegar a tu red interna; un
receptor en tu red local necesita una dirección pública o un relé.

## Claves de API

Una clave de API permite que un script llame a la API del servidor sin
contraseña. Las claves se crean en **Admin → Claves API**, las crea un
administrador y pertenecen a ese administrador.

- **La clave se muestra una sola vez**, al crearla. Guárdala entonces; no se
  puede volver a mostrar, solo revocar.
- **Actúa como su propietario**, con derechos de administrador, dentro de sus
  ámbitos.
- **Los ámbitos** limitan lo que puede hacer la clave:

| Ámbito | Permite |
| --- | --- |
| `media:read` | Leer los metadatos de los medios |
| `media:write` | Modificar medios y lanzar escaneos |
| `playback:read` | Leer el estado y el historial de reproducción |
| `playback:write` | Controlar la reproducción y el progreso |
| `users:read` | Leer los usuarios |
| `users:write` | Modificar los usuarios |
| `admin:read` | Leer la actividad y las estadísticas de administración |
| `admin:write` | Modificar los ajustes de administración |

**Una clave creada sin ningún ámbito tiene acceso completo**, como su
propietario: es el comportamiento heredado, conservado para las claves
existentes. Selecciona los ámbitos que necesita un script.

La clave va en la cabecera `Authorization`, con o sin `Bearer`:

```bash
curl -H "Authorization: Bearer evey_..." https://media.example.com/api/v1/...
```

Una clave deja de funcionar de inmediato cuando se revoca, o cuando se desactiva
a su propietario. Los endpoints se listan en la [API del servidor](api/README.md).
