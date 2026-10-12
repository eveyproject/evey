---
source_sha: 5d6e1c496b373ae579e3de32d6359d6c1c9712b93ef8c17806d17afca066d8f8
reviewed: true
---
# API del servidor

La API HTTP del servidor, descrita en archivos OpenAPI (Swagger 2.0), uno por
servicio. Este índice te dice qué carpeta contiene qué y cómo se autentica
una llamada; los archivos en sí son la referencia.

| Sección | Contenido |
|---------|-------------|
| [API pública](public/README.md) | Lo que puede hacer un usuario conectado: explorar, reproducir, coleccionar, pedir. |
| [API de administración](admin/README.md) | La administración del servidor: configuración, transcodificación, copias de seguridad, metadatos, usuarios. |
| [API de plugins](plugins/README.md) | La gestión de plugins, los webhooks y las preferencias que la interfaz de un plugin guarda para cada usuario. |

Cada archivo `.swagger.json` se puede importar en cualquier herramienta
OpenAPI (Swagger UI, Postman, Insomnia). Las rutas HTTP se generan a partir
de las definiciones gRPC de `proto/evey/v1/`, así que el nombre de un método
en un archivo es el método gRPC que hay detrás.

## Autenticación

Tres tipos de credencial llegan a la API, todos en la cabecera `Authorization`:

| Credencial | Quién | Cómo |
|---|---|---|
| Token de sesión | Una persona conectada desde una aplicación | `Bearer <token>`, se obtiene al iniciar sesión y la aplicación lo renueva. |
| Clave de API | Un script, en nombre del administrador que la creó | `Bearer evey_...` (el prefijo `Bearer` es opcional). Con alcances y revocable: [Webhooks y claves de API](../webhooks-api-keys.md). |
| Token de plugin | La interfaz de un plugin, a través de la página anfitriona | Se emite por cinco minutos para un plugin y un usuario, y solo lleva los alcances `public` y `prefs` que se concedieron al plugin; todos los demás métodos lo rechazan. El marco nunca lo tiene: la página anfitriona hace de intermediaria en las llamadas. |

Un token de plugin nunca puede llegar a un método de administración, tenga
los alcances que tenga
([ADR-0003](../adr/0003-plugin-scoped-tokens.md),
[ADR-0012](../adr/0012-plugin-token-deny-by-default.md)).

## Regeneración

Para quien contribuye: los archivos se regeneran a partir de las definiciones
protobuf.

```bash
buf generate --template buf.gen.local.yaml
python3 docs/api/sync.py
```

<!-- sources
session token, api key, plugin token: internal/adapters/driving/grpcapi/interceptors/auth.go - authenticate, authenticateAPIKey, authorizePluginToken
plugin token: internal/app/plugin/token_service.go - PluginTokenTTL ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes ; docs/adr/0003-plugin-scoped-tokens.md ; docs/adr/0012-plugin-token-deny-by-default.md
generation: docs/api/sync.py ; buf.gen.local.yaml
-->
