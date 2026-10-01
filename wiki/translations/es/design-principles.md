---
source_sha: dffa9823329d7253d7991f9037012a372bdeeb2765f9c4c68f0d73f04efc4dbc
reviewed: false
---
# Principios de diseño

Los principios siguientes explican por qué Evey se comporta como lo hace. Cada
uno es una restricción que sigue el código, con las páginas y los registros de
decisión donde se ve. Cuando dos de ellos chocan, gana el que aparece antes en
esta lista.

## 1. El acceso se deniega cuando no se puede comprobar

Donde una decisión concede acceso, un error se resuelve con la respuesta más
restrictiva. Un perfil cuyo control parental no se puede leer no recibe ningún
contenido; un plugin cuyo código no se puede analizar no se instala; el acceso
remoto sigue una elección explícita en cuanto se ha tomado, y avisa claramente
cuando rechaza a todo el mundo. El acceso remoto y la autenticación son cosas
distintas: el primero es una elección de despliegue, la segunda es lo que
protege la biblioteca.

Consulta [Usuarios, perfiles y control parental](administration/users.md),
[Acceso remoto y HTTPS](administration/remote-access.md),
[ADR-0026](adr/0026-remote-access-inferred-from-a-published-url.md).

## 2. El código de terceros se contiene, no se confía en él

Un plugin recibe los permisos que declara su manifiesto y nada más, se analiza
antes de instalarse, se ejecuta en un sandbox sin acceso directo a la red y tiene
un token que deniega por defecto. Lo que el sandbox todavía no puede impedir se
deja por escrito en lugar de darlo por sobrentendido.

Consulta [Modelo de seguridad y sandbox de plugins](SECURITY.md),
[ADR-0012](adr/0012-plugin-token-deny-by-default.md),
[ADR-0014](adr/0014-landlock-plugin-sandbox.md).

## 3. El original se entrega siempre que se puede

El resultado preferido es el archivo tal cual, después las mismas pistas en otro
contenedor, y después una transcodificación solo de las pistas que lo necesitan.
La calidad no es algo que el espectador tenga que elegir antes de reproducir: el
servidor conserva la calidad de la fuente salvo que una restricción real (el
dispositivo, la conexión, un límite) lo impida.

Consulta [Cómo elige Evey el modo de reproducción](playback/how-playback-is-decided.md).

## 4. Las decisiones se basan en lo demostrado, no en los nombres

Las capacidades de un reproductor vienen de lo que indica su dispositivo, no de
una lista de dispositivos conocidos. Una tarjeta gráfica se usa para lo que
superó durante la medición, no para lo que su modelo debería admitir. Cuando una
capacidad no se puede demostrar, el servidor toma la respuesta prudente: un
receptor de envío desconocido recibe las capacidades del más antiguo, y los
formatos de píxel que los reproductores todavía no indican siguen una regla de
compatibilidad fija. Suponer demasiado da una pantalla negra o colores erróneos;
suponer demasiado poco cuesta una transcodificación.

Consulta [Enviar a Chromecast](playback/casting.md), [Aceleración por hardware](administration/hardware-acceleration.md),
[ADR-0029](adr/0029-transcoding-routes-are-measured-not-assumed.md).

## 5. Una reproducción, una decisión

Una reproducción se decide una vez, al empezar, y cada manifiesto y cada segmento
de la sesión se sirven a partir de esa decisión. El reproductor nunca recibe
segmentos de dos codificaciones, y lo que indica Información del flujo se deriva
del mismo plan que ejecuta el codificador, comprobado por pruebas en ambos
sentidos.

Consulta [ADR-0030](adr/0030-one-plan-one-decision.md),
[ADR-0024](adr/0024-playback-plan-resolver.md).

## 6. El servidor se dimensiona según la máquina

El servidor mide cuánto puede transcodificar y no admite más: una máquina pequeña
reproduce menos flujos transcodificados, en lugar de reproducirlos todos mal. La
reproducción va primero; las tareas en segundo plano se pausan mientras alguien
ve algo. La misma imagen funciona en un NAS y en un servidor con varias tarjetas
gráficas.

Consulta [Transcodificación y capacidad del servidor](playback/transcoding.md),
[Tareas en segundo plano y carga del servidor](operations/background-work.md).

## 7. Pocos ajustes, cada uno en un solo sitio

Lo que debe existir antes de que arranque el servidor (almacenamiento, secretos,
la dirección pública) está en el archivo compose; todo lo que puede cambiar
después está en el panel de administración, se guarda en la base de datos y se
aplica sin reiniciar siempre que nada lo exija. La detección y la medición
sustituyen a los ajustes siempre que pueden: una instalación es un archivo
compose, bibliotecas y usuarios.

Consulta [Instalación](installation.md), [Referencia de configuración](configuration.md),
[ADR-0028](adr/0028-install-is-one-compose-file.md).

## Registros de decisión

Las decisiones estructurales se registran como registros de decisión de
arquitectura, con el contexto, las opciones sopesadas y las consecuencias. Se
guardan con el código: [docs/adr](adr/README.md).
