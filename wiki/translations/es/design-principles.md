---
source_sha: be9fbbf14af7dccd420515b33353e89b5ac1154b0fec5dc22468319328e97a6c
reviewed: true
---
# Principios de diseño

Siete reglas que sigue el código, cada una con las páginas y los registros
de decisión donde se ve. Lee esta página para entender por qué Evey se
comporta como se comporta, antes de leer cómo está construido
([Arquitectura](ARCHITECTURE.md)) o por qué se tomó una decisión de
estructura ([registros de decisión](adr/README.md)). Cuando dos principios
entran en conflicto, prevalece el que aparece antes en esta lista.

| Principio | Dónde se ve | Decidido en |
| --- | --- | --- |
| 1. Si no se puede comprobar, se niega el acceso | control parental, instalación de plugins, acceso remoto | [ADR-0026](adr/0026-remote-access-inferred-from-a-published-url.md) |
| 2. El código de terceros se aísla, no se le da confianza | sandbox de plugins | [ADR-0012](adr/0012-plugin-token-deny-by-default.md), [ADR-0014](adr/0014-landlock-plugin-sandbox.md) |
| 3. Se entrega el original siempre que se puede | decisión de reproducción | [ADR-0030](adr/0030-one-plan-one-decision.md) |
| 4. Las decisiones se basan en lo que se ha demostrado, no en nombres | envío a Chromecast, aceleración por hardware | [ADR-0029](adr/0029-transcoding-routes-are-measured-not-assumed.md) |
| 5. Una reproducción, una decisión | plan de reproducción, Información del flujo | [ADR-0030](adr/0030-one-plan-one-decision.md), [ADR-0024](adr/0024-playback-plan-resolver.md) |
| 6. El servidor se ajusta a la máquina | calibración, trabajo en segundo plano | - |
| 7. Pocos ajustes, cada uno en un solo sitio | instalación, panel de administración | [ADR-0028](adr/0028-install-is-one-compose-file.md) |

## 1. Si no se puede comprobar, se niega el acceso

Cuando una decisión da acceso a algo, un error lleva a la respuesta más
restrictiva. Un perfil cuyos ajustes parentales no se pueden leer no recibe
ningún contenido; un plugin cuyo código no se puede analizar no se instala;
el acceso remoto sigue una elección explícita una vez que se ha hecho, y lo
dice claramente cuando rechaza a todo el mundo. El acceso remoto y la
autenticación son cosas distintas: lo primero es una elección de despliegue,
lo segundo es lo que protege la biblioteca.

Consulta [Usuarios, perfiles y control parental](administration/users.md),
[Acceso remoto y HTTPS](administration/remote-access.md),
[ADR-0026](adr/0026-remote-access-inferred-from-a-published-url.md).

## 2. El código de terceros se aísla, no se le da confianza

Un plugin recibe los permisos que declara su manifiesto y nada más, se
analiza antes de instalarse, se ejecuta en un sandbox sin acceso directo a la
red y tiene un token que lo deniega todo por defecto. Lo que el sandbox
todavía no puede impedir está escrito, no se da por sobreentendido.

Consulta [Sandbox de plugins y modelo de amenazas](SECURITY.md),
[ADR-0012](adr/0012-plugin-token-deny-by-default.md),
[ADR-0014](adr/0014-landlock-plugin-sandbox.md).

## 3. Se entrega el original siempre que se puede

Lo preferido es el archivo tal cual; después, las mismas pistas en otro
contenedor; después, transcodificar solo las pistas que lo necesitan. La
calidad no es algo que quien mira tenga que elegir antes de reproducir: el
servidor mantiene la calidad de la fuente salvo que una limitación real (el
dispositivo, la conexión, un límite) lo impida.

Consulta [Cómo elige Evey el modo de reproducción](playback/how-playback-is-decided.md).

## 4. Las decisiones se basan en lo que se ha demostrado, no en nombres

Las capacidades de un reproductor salen de lo que informa su dispositivo, no
de una lista de dispositivos conocidos. Una tarjeta gráfica se usa para lo
que superó durante la medición, no para lo que su modelo debería admitir.
Cuando una capacidad no se puede demostrar, el servidor elige la respuesta
prudente: un receptor de Chromecast desconocido recibe las capacidades del
más antiguo, y los formatos de píxel que los reproductores todavía no
informan siguen una regla de compatibilidad fija. Suponer demasiado da una
pantalla negra o colores incorrectos; suponer demasiado poco cuesta una
transcodificación.

Consulta [Enviar a Chromecast](playback/casting.md), [Aceleración por hardware](administration/hardware-acceleration.md),
[ADR-0029](adr/0029-transcoding-routes-are-measured-not-assumed.md).

## 5. Una reproducción, una decisión

Una reproducción se decide una sola vez, al empezar, y cada manifiesto y cada
segmento de la sesión se sirven a partir de esa decisión. El reproductor
nunca recibe segmentos de dos codificaciones distintas, y lo que muestra
Información del flujo se obtiene del mismo plan de servicio que usa la cadena
de reproducción, comprobado por tests en los dos sentidos.

Consulta [Servir un vídeo - el camino de reproducción HLS](playback-serving.md),
[ADR-0030](adr/0030-one-plan-one-decision.md),
[ADR-0024](adr/0024-playback-plan-resolver.md).

## 6. El servidor se ajusta a la máquina

El servidor mide cuánto puede transcodificar y no acepta más: una máquina
pequeña reproduce menos flujos transcodificados, en lugar de reproducirlos
todos mal. La reproducción va primero; el trabajo en segundo plano se pausa
mientras alguien está viendo algo. La misma imagen funciona en un NAS y en un
servidor con varias tarjetas gráficas.

Consulta [Transcodificación y capacidad del servidor](playback/transcoding.md),
[Trabajo en segundo plano y carga del servidor](operations/background-work.md).

## 7. Pocos ajustes, cada uno en un solo sitio

Lo que tiene que existir antes de que arranque el servidor (el
almacenamiento, los secretos, la dirección pública) está en el archivo
compose; todo lo que puede cambiar después está en el panel de
administración, se guarda en la base de datos y se aplica sin reiniciar
siempre que nada lo exija. La detección y la medición sustituyen a los
ajustes siempre que pueden: una instalación es un archivo compose, unas
bibliotecas y unos usuarios.

Consulta [Instalación](installation.md), [Referencia de configuración](configuration.md),
[ADR-0028](adr/0028-install-is-one-compose-file.md).

## Registros de decisión

Las decisiones de estructura se guardan como registros de decisión de
arquitectura, con el contexto, las opciones que se sopesaron y las
consecuencias. Se guardan junto al código: [docs/adr](adr/README.md).

<!-- sources
fail closed: internal/adapters/driving/http/parental.go - BlockedByParental ; internal/app/plugin/install.go
contained plugins: docs/SECURITY.md ; docs/adr/0012-plugin-token-deny-by-default.md ; docs/adr/0014-landlock-plugin-sandbox.md
original first, one decision: internal/app/transcoding/decision.go ; internal/app/transcoding/route_tonemap_report_test.go ; docs/adr/0030-one-plan-one-decision.md
proved capabilities: frontend/packages/media-caps/src/castUrl.ts - castProfileFor ; docs/adr/0029-transcoding-routes-are-measured-not-assumed.md
sizing: internal/app/transcoding/calibration_service.go ; i18n admin.serverConfig.cpuBudget.globalDesc
-->
