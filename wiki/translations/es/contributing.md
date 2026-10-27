---
source_sha: cbb77f7bf06443976e7bc56f600217600725f9387abe71900e4c95e24ef4519e
reviewed: true
---
# Contribuir

Por dónde empezar si quieres trabajar en el código de Evey, y qué tiene que
superar una pull request. Los documentos escritos para quien contribuye están
junto al código, en GitHub, y no en este sitio; esta página te lleva a ellos.

## Por dónde empezar

| Quieres | Lee |
| --- | --- |
| Compilar y ejecutar Evey desde el código fuente | [DEV.md](../DEV.md) |
| Conocer las reglas de una contribución: ramas, commits, tests, el acuerdo de licencia | [CONTRIBUTING.md](../CONTRIBUTING.md) |
| Orientarte en el código en media hora | [Mapa del código](CODEBASE_MAP.md) |
| Entender las capas y los contextos delimitados | [Arquitectura](ARCHITECTURE.md) |
| Seguir las convenciones que ya usa el código | [Convenciones de código](code-conventions.md) |
| Trabajar en la interfaz | [Sistema de diseño del frontend](frontend-design-system.md) |
| Saber por qué se tomó una decisión de estructura | [Decisiones de arquitectura](adr/README.md) |
| Publicar una versión | [Guía de publicación de versiones](runbooks/release.md) |

## Por qué un registro de decisión, y dónde

Cada decisión de estructura (la organización hexagonal, cómo se limitan los
tokens de plugin, por qué la instalación es un solo archivo compose, cómo un
único plan de reproducción decide para todos los clientes) tiene un registro
de decisión de arquitectura: el contexto, las opciones que se sopesaron, lo
que se eligió y lo que cuesta. Se guardan en el repositorio, junto al código
que explican, y son el sitio por donde empezar antes de proponer cambiar una
de esas decisiones.

## Antes de abrir una pull request

1. Ejecuta `make pre-push`: es la puerta de control, y ejecuta en local las
   comprobaciones de la CI en este orden: vet, análisis de vulnerabilidades,
   compilación, tests unitarios con `-race`, frontend (comprobación de tipos,
   lint, tests unitarios), análisis de seguridad, lint,
   [guardas de arquitectura](ARCHITECTURE.md#architecture-guards), higiene
   de los commits, búsqueda de secretos, comprobación de cambios
   incompatibles en los proto, una compilación desde una copia limpia, las
   puertas de integración del repositorio y de la transcodificación, y la
   auditoría de TV.
   Un push a `dev` o a una rama de trabajo ejecuta `make pre-push-dev` en su
   lugar: la misma puerta de control sin las suites largas que la CI de la
   pull request vuelve a ejecutar sobre los mismos commits (análisis de
   vulnerabilidades, tests y compilación del frontend, suite del
   repositorio, auditoría de TV); un push a `master` ejecuta la puerta
   completa.
2. Añade tests para cada comportamiento nuevo y para cada corrección de un
   error.
3. Si contribuyes desde fuera, acepta una vez el
   [acuerdo de licencia para contribuidores](../CLA.md): Evey es
   AGPL-3.0-or-later y también puede ofrecerse con una licencia comercial.
   Tú conservas los derechos de autor sobre tu trabajo.

!!! warning "La puerta de control no construye la imagen Docker ni las aplicaciones Android"
    Si tocas el `Dockerfile` o código Kotlin, constrúyelos tú
    (`docker build --target runtime`, la tarea de release de Gradle) antes
    de hacer push.

Los problemas de seguridad no se comunican en público: escribe a
`security@eveyproject.org` (consulta [SECURITY.md](../SECURITY.md)).

<!-- sources
gate: Makefile - pre-push, ci-fast-vet, ci-fast-vuln, ci-fast-build, ci-fast-unit, ci-fast-frontend, ci-fast-sec, ci-local-lint, ci-arch, ci-hygiene, ci-secrets, ci-proto-breaking, ci-clean-checkout, repo-itest-gate, transcode-itest-gate, tv-audit
documents: DEV.md ; CONTRIBUTING.md ; CLA.md ; SECURITY.md ; docs/CODEBASE_MAP.md ; docs/code-conventions.md ; docs/frontend-design-system.md ; docs/adr/README.md ; docs/runbooks/release.md
-->
