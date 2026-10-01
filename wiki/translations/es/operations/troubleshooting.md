---
source_sha: 358930f5d871a10d9184002e80eb788d14685e06174afd3a569a059380aa6d0b
reviewed: false
---
# Solución de problemas

Empieza por el síntoma. Cada entrada dice qué comprobar primero y dónde está el
procedimiento detallado.

## Primeras comprobaciones

Dos comandos responden a la mayoría de las preguntas sobre un servidor que se
comporta mal:

```bash
docker compose logs --tail=200 evey
docker exec evey-server evey admin doctor
```

`evey admin doctor` comprueba la base de datos y su esquema, la clave de firma,
el índice de búsqueda, los montajes de medios (incluido si el servidor puede
escribir en cada biblioteca), los directorios de caché, ffmpeg, el acceso remoto
y el espacio libre en disco. Muestra una línea por comprobación con un remedio, y
termina con un código distinto de cero cuando una falla.

En el panel de administración, **Logs y depuración** muestra los mismos
registros, con búsqueda y filtrados por área (reproducción, transcodificación,
escaneo...).

## El servidor no arranca

| Lo que dice el registro | Lo que significa |
| --- | --- |
| Un mensaje de validación de la configuración | Con una base de datos externa, el servidor rechaza la contraseña por defecto `evey`; con un Meilisearch externo, una clave vacía o de ejemplo. Corrige el valor que nombra el mensaje. |
| Líneas que empiezan por `evey:` sobre Postgres | La base de datos integrada se negó a arrancar: un clúster de otra versión mayor de Postgres ([Actualización mayor de Postgres](advanced-deployments.md#postgres-major-upgrade)), un directorio de datos que no es un clúster de Postgres, un disco lleno, o `./evey-data/postgres` en un recurso compartido de red. |
| `permission denied` en `/cache/...`, en un bucle de reinicios | El volumen `evey-cache` pertenece a otro usuario distinto del que ejecuta el servidor. Actualiza la imagen; con una imagen anterior al 2026-09-17, ejecuta una vez `docker run --rm -v <project>_evey-cache:/c alpine chown -R <PUID>:<PGID> /c`. |

Recuperar una contraseña de administrador perdida, una clave de firma
comprometida o una base de datos rota: [Recuperación](../runbooks/recovery.md).

## No se puede acceder desde fuera de casa

El servidor responde en casa pero no en otro sitio, o las aplicaciones muestran un
mensaje que dice que el acceso remoto está desactivado: consulta
[Acceso remoto y HTTPS](../administration/remote-access.md). La página de
administración **Acceso remoto** lo indica en rojo cuando el servidor es
accesible desde internet mientras el acceso remoto está desactivado.

Nada responde, ni siquiera en casa: [El servidor no responde](../runbooks/incident-runbook.md#1-server-not-reachable).

## Todo el mundo ha perdido la sesión, o todo inicio de sesión falla

[Todos los inicios de sesión devuelven 401](../runbooks/incident-runbook.md#2-all-logins-return-401).
Las causas habituales son la base de datos (cada petición comprueba la cuenta en
ella y se rechaza cuando no responde; el registro lo indica) y un proxy inverso
que falta en **Proxies de confianza**, que pone a todos bajo un único límite de
inicios de sesión. Una clave de firma perdida o sustituida no cierra la sesión de
nadie por sí sola: las aplicaciones renuevan sus tokens.

## El inicio de sesión SSO falla

La página de inicio de sesión dice «Error de inicio de sesión con SSO»:

- la persona ya tiene una cuenta local con el mismo correo o el mismo nombre de
  usuario;
- su dirección está fuera de `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS`, o el proveedor
  no la marcó como verificada (el registro dice
  `oidc: email domain not allowed` o
  `oidc: unverified email rejected under domain allowlist`).

Una página de error mostrada por el propio proveedor suele significar que la URI
de redirección registrada allí no es la que usa Evey. Consulta
[Configuración de OIDC / SSO](../oidc.md).

## Los correos no llegan

El SMTP se lee de las variables `EVEY_SMTP_*` al arrancar, no de la tarjeta
**Correo / SMTP**. Cuando está activo, el registro de arranque muestra
`SMTP email notifications enabled` con el host. Un envío que falla se registra
como `email: send notification`, con la respuesta del servidor; un modo TLS que
no corresponde al puerto (`starttls` con 587, `ssl` con 465) es el error
habitual. Consulta [Notificaciones](../notifications.md).

## Un título se transcodifica cuando esperabas Reproducción directa

Abre **Información del flujo** en el reproductor: nombra el método y los motivos.
Consulta [Cómo elige Evey el modo de reproducción](../playback/how-playback-is-decided.md)
para interpretarlos.

## La reproducción se para, se queda cargando o no arranca

- Un solo título: abre **Admin → Salud de medios**. Un archivo sin índice de
  posicionamiento, un tramo dañado o pistas guardadas en bloques explican los parones y
  los saltos lentos, y la mayoría se reparan en el sitio. Consulta
  [Salud de medios](../runbooks/media-health.md).
- Varios títulos, cuando varias personas ven algo: el servidor puede estar en su
  capacidad de transcodificación. Consulta
  [Transcodificación y capacidad del servidor](../playback/transcoding.md).
- La reproducción transcodificada falla del todo:
  [La transcodificación falla](../runbooks/incident-runbook.md#3-transcoding-fails--videos-wont-play-transcoded).
- Se queda cargando y los registros no dicen por qué:
  [el procedimiento de diagnóstico](../runbooks/incident-runbook.md#3c-it-buffers--it-cut-out-and-the-logs-do-not-say-why).
- **Admin → Calidad de streaming** muestra, para los espectadores reales durante
  el último día o la última semana, con qué frecuencia esperó la reproducción y
  qué títulos y dispositivos están afectados.

## Sin sonido, o sonido desincronizado

- Solo en un navegador, con una pista Dolby Digital (AC-3/E-AC-3): un proxy
  inverso que pone su propia Content-Security-Policy puede bloquear el
  descodificador que usa el reproductor web. Consulta
  [Audio](../playback/audio.md#dolby-digital-in-a-browser).
- Desincronizado en un título: **Admin → Salud de medios** detecta el audio que
  empieza después de la imagen y las pistas que no coinciden entre sí.

## Los subtítulos no aparecen

Consulta [Subtítulos](../playback/subtitles.md): qué formatos dibujan los
reproductores y cuáles tiene que incrustar el servidor en la imagen.

## Faltan pósteres o están rotos

Los pósteres y los fondos se copian al servidor durante los escaneos, en
`./evey-data/artworks`, y los que faltan se reintentan cada hora. **Admin →
Caché de artworks** muestra cuántos siguen sin copia local; **Descargar
artworks faltantes** los obtiene ahora, y **Purgar variantes corruptas** elimina
los pósteres redimensionados que un redimensionado fallido dejó vacíos, que se
vuelven a generar bajo demanda.

## La búsqueda no devuelve nada

Reconstruye el índice a partir de la base de datos:
`docker exec evey-server evey admin reindex-search`. El índice siempre se puede
reconstruir; no se pierde nada.

## Una sesión de envío se congeló o se quedó sin sonido

[Un envío se congeló, se quedó sin sonido o el teléfono no para de cargar](../runbooks/incident-runbook.md#8-a-cast-froze-went-silent-or-the-phone-keeps-spinning).

## Un plugin dejó de funcionar

[Un plugin se ha caído](../runbooks/incident-runbook.md#7-plugin-crashed). Sus
propios registros están en su tarjeta en **Admin → Plugins**.

## El disco se llena

La causa habitual es la caché de transcodificación:
[Disco de la caché de transcodificación lleno](../runbooks/incident-runbook.md#6-transcode-cache-disk-full).
Se puede vaciar desde **Admin → Caché de transcodificación** en cualquier momento.
