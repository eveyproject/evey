---
source_sha: caf11b9849876eb65c290e2bc0934d093760aa2b38f6e3c27b303e9af47472f8
---
# Contribuir

Evey se desarrolla a la vista de todos, y los documentos escritos para quienes
trabajan en su código viven con el código, en GitHub, y no en este sitio. Esta
página los señala.

## Por dónde empezar

| Usted quiere | Lea |
| --- | --- |
| Compilar y ejecutar Evey desde el código fuente | [DEV.md](../DEV.md) |
| Conocer las reglas de una contribución: ramas, commits, pruebas, acuerdo de licencia | [CONTRIBUTING.md](../CONTRIBUTING.md) |
| Orientarse en el código en media hora | [Mapa del código](CODEBASE_MAP.md) |
| Entender las capas y los contextos de negocio | [Arquitectura](ARCHITECTURE.md) |
| Seguir las convenciones que el código ya usa | [Convenciones de código](code-conventions.md) |
| Trabajar en la interfaz | [Sistema de diseño de la interfaz](frontend-design-system.md) |
| Saber por qué se tomó una decisión estructural | [Decisiones de arquitectura](adr/README.md) |

## Por qué un registro de decisiones, y dónde

Cada decisión estructural - la organización hexagonal, el alcance de los tokens
de extensión, por qué la instalación es un único archivo compose, cómo un único
plan de reproducción decide por todos los clientes - tiene su registro de
decisión de arquitectura: el contexto, las opciones sopesadas, lo que se eligió
y lo que cuesta. Se guardan en el repositorio, junto al código que explican, y
son el punto de partida antes de proponer cambiar una de esas decisiones.

## Antes de abrir un pull request

- `make pre-push` es el único punto de control. Reproduce la CI: una ejecución
  en verde ahí es una CI en verde.
- Se esperan pruebas para todo comportamiento nuevo y para cada corrección.
- Las contribuciones externas requieren aceptar una vez el
  [acuerdo de licencia para contribuidores](../CLA.md): Evey está bajo
  AGPL-3.0-or-later y también se ofrece con una licencia comercial. Usted
  conserva los derechos de autor sobre su trabajo.

Los problemas de seguridad no se comunican en público: escriba a
`security@eveyproject.org` (vea [SECURITY.md](../SECURITY.md)).
