---
source_sha: 3ba52125e2272c947b09a156c62f232a40194d67c79158ea4d06ec6075d320b4
reviewed: false
---
# Documentación de la API de Evey

Especificaciones OpenAPI (Swagger 2.0) del servidor multimedia Evey.

## Estructura

| Sección | Descripción |
|---------|-------------|
| [API pública](public/README.md) | Endpoints para los usuarios: explorar medios, reproducción, colecciones, listas de reproducción, etc. |
| [API de administración](admin/README.md) | Administración del servidor: configuración del sistema, transcodificación, copias de seguridad, metadatos, etc. |
| [API de plugins](plugins/README.md) | Gestión de plugins e integraciones por webhook. |

## Uso

Cada carpeta contiene archivos `.swagger.json` individuales que se pueden importar en cualquier herramienta compatible con OpenAPI (Swagger UI, Postman, Insomnia, etc.).

## Regeneración

Estas especificaciones se generan a partir de las definiciones protobuf mediante `buf generate`. Para actualizarlas:

```bash
buf generate --template buf.gen.local.yaml
python3 docs/api/sync.py
```
