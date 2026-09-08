---
source_sha: 641ed9ad2a04551e1c5e1db07a26cb739f2708ce4ec146e2a5590265cd4216ca
---

# Sistema de extensiones

Las extensiones de Evey amplían el servidor con proveedores de metadatos,
procesamiento multimedia, webhooks, notificaciones, módulos de interfaz y temas
CSS. Una extensión se ejecuta como un **subproceso aislado** que habla con el
anfitrión por gRPC a través de un socket de dominio Unix; la interfaz que traiga,
si la trae, se dibuja dentro de un **iframe con sandbox** y se comunica con el
anfitrión mediante un puente `postMessage`.

Esta página es un mapa. El detalle está en tres documentos monográficos:

| Documento | Qué contiene |
|---|---|
| **[plugins/dev-setup.md](plugins/dev-setup.md)** | Cómo escribir su primera extensión: requisitos previos, andamiaje, el ciclo de instalar e iterar, la firma, el envío al catálogo y la resolución de problemas. **Empiece por aquí.** |
| **[plugins/api-reference.md](plugins/api-reference.md)** | El contrato: esquema del manifiesto, IPC y almacén de datos del backend, el puente `evey.*` de la interfaz, la lista de slots y cada una de las capacidades (`metadata.provider`, `media.read/write`, `notification.sender`, `webhook.receiver`, `public`, `prefs`). |
| **[plugins/security-model.md](plugins/security-model.md)** | Modelo de amenazas y capas de defensa: separación de UID, la lista de denegación seccomp del núcleo y del LD_PRELOAD, los modos de sandbox (`auto` resuelve landlock → bubblewrap → basic; `strict` añade la biblioteca LD_PRELOAD a bubblewrap), el monitor de recursos en espacio de usuario, el análisis estático, el saneador de CSS, la firma y el versionado, la lista blanca de slots y los límites conocidos. |

El modelo de autorización que hay detrás de los tokens de extensión (denegación
por defecto, ámbitos bloqueados en firme, el puente que impide que el token
llegue nunca al iframe) está en
[adr/0012-plugin-token-deny-by-default.md](adr/0012-plugin-token-deny-by-default.md);
los topes de recursos, en [adr/0013-userspace-plugin-resource-monitor.md](adr/0013-userspace-plugin-resource-monitor.md).

## Arquitectura

```
+------------------+      gRPC (Unix socket)      +-------------------+
|   Evey Server    | <--------------------------> |  Plugin Process   |
|                  |   /tmp/evey/plugins/{id}/    |                   |
|  HostService     |   plugin.sock                |  PluginService    |
|  (serves RPCs)   |                              |  (implements)     |
+------------------+                              +-------------------+
        |                                                  |
        v                                                  v
+------------------+                              +-------------------+
|  Permission /    |                              |  SDK stubs        |
|  scope checker   |                              |  (multi-language) |
+------------------+                              +-------------------+
        |
        v
+------------------+     iframe sandbox           +-------------------+
|  Frontend        | <--------------------------> |  Frontend module  |
|  PluginSlot      |   postMessage bridge         |  (in srcdoc)      |
+------------------+                              +-------------------+
```

Una extensión puede escribirse en **cualquier lenguaje que el `entrypoint:` sea
capaz de lanzar**: el ejecutor no impone ninguno. Se distribuyen stubs gRPC del
SDK ya generados para Python, Node.js, PHP, Ruby, Dart, Java y Swift
(`sdk/<lang>/`, que se copian solos al instalar); Go no trae stubs incluidos, así
que hay que generarlos con `protoc-gen-go` y `protoc-gen-go-grpc`; los lenguajes
interpretados sin stubs incluidos (Bash, Lua, Perl, …) funcionan invocando
directamente al intérprete, y los binarios compilados (Go, Rust, …) funcionan con
el permiso `native-binary`. Puesta a punto lenguaje por lenguaje:
[plugins/dev-setup.md](plugins/dev-setup.md#per-language).

Los tres pilares:

- **Capacidades y ámbitos**: la extensión declara en su manifiesto las
  capacidades que necesita y el administrador se las concede al instalarla. El
  anfitrión las aplica con denegación por defecto en cada llamada. Catálogo
  completo en
  [plugins/api-reference.md](plugins/api-reference.md#capabilities).
- **Sandbox**: los procesos de backend se ejecutan acotados (confinamiento del
  sistema de archivos, de las llamadas al sistema y de la red) bajo un monitor de
  recursos en espacio de usuario; el código de la interfaz corre dentro de un
  iframe cerrado a cal y canto. El detalle, en [plugins/security-model.md](plugins/security-model.md).
- **Confianza**: las extensiones quedan fijadas por su SHA256 y pueden ir
  firmadas con Cosign; el servidor se puede configurar para exigir esa firma.
  Véase
  [plugins/security-model.md](plugins/security-model.md#update-workflow).

## Activar el subsistema

Las extensiones vienen activadas de fábrica (`EVEY_PLUGINS_ENABLED=true`). El
modo de sandbox se elige con `EVEY_PLUGINS_SANDBOX_MODE` (por defecto `auto`, que
en la imagen oficial se resuelve como Landlock). Los topes de recursos
(`EVEY_PLUGINS_MEMORY_MB`, `_PIDS_MAX`, `_DISK_MAX_MB`, `_CPU_NICE`) se modifican
desde **Admin → Extensiones**.
Véanse [configuration.md](configuration.md) y [plugins/dev-setup.md](plugins/dev-setup.md#7-resource-limits-and-sandbox).
