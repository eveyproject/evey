---
source_sha: 053efa5319e4b48ff12a944ce76f5755b01c4022ff1105d87cf82c5135ac1b4d
reviewed: false
---
# Contribuir

Evey se desarrolla de forma abierta, y los documentos escritos para quienes
trabajan en su código viven con el código, en GitHub, en lugar de en este sitio.
Esta página remite a ellos.

## Por dónde empezar

| Quieres | Lee |
| --- | --- |
| Compilar y ejecutar Evey desde las fuentes | [DEV.md](../DEV.md) |
| Conocer las reglas de una contribución: ramas, commits, pruebas, el acuerdo de licencia | [CONTRIBUTING.md](../CONTRIBUTING.md) |
| Orientarte en el código en media hora | [Mapa del código](CODEBASE_MAP.md) |
| Entender las capas y los contextos delimitados | [Arquitectura](ARCHITECTURE.md) |
| Seguir las convenciones que ya usa el código | [Convenciones de código](code-conventions.md) |
| Trabajar en la interfaz | [Sistema de diseño del frontend](frontend-design-system.md) |
| Saber por qué se tomó una decisión estructural | [Decisiones de arquitectura](adr/README.md) |
| Publicar una versión | [Manual de publicación](runbooks/release.md) |

## Por qué un registro de decisión, y dónde

Cada decisión estructural (la disposición hexagonal, cómo se limitan los tokens
de los plugins, por qué la instalación es un único archivo compose, cómo un único
plan de reproducción decide para todos los clientes) tiene un registro de
decisión de arquitectura: el contexto, las opciones que se sopesaron, lo que se
eligió y lo que cuesta. Se guardan en el repositorio, junto al código que
explican, y son el punto de partida antes de proponer cambiar una de esas
decisiones.

## Antes de abrir una pull request

- `make pre-push` es la única puerta. Reproduce la CI: una ejecución en verde ahí
  significa una canalización en verde.
- Se esperan pruebas para todo comportamiento nuevo y para cada corrección de
  errores.
- Las contribuciones externas necesitan aceptar una vez el
  [acuerdo de licencia de contribuidor](../CLA.md): Evey es AGPL-3.0-or-later y
  también se ofrece con una licencia comercial. Conservas los derechos de autor
  de tu trabajo.

Los problemas de seguridad no se comunican en público: escribe a
`security@eveyproject.org` (consulta [SECURITY.md](../SECURITY.md)).
