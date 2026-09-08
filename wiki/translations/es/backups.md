---
source_sha: 2f1c2cafd1c1e20b2e8d011ec2457b1f01bde072b96b93d077d7694cf6537497
---

# Copias de seguridad

Evey puede copiar la base de datos automáticamente según una programación.

## Configuración

```env
EVEY_BACKUP_DIR=/data/backups          # Dónde se guardan las copias
EVEY_BACKUP_INTERVAL_HOURS=24          # Intervalo de copia automática (0 = desactivado)
EVEY_BACKUP_RETENTION_DAYS=30          # Eliminar las copias más antiguas (0 = conservarlas siempre)
```

## Qué se copia

La copia de seguridad recoge todos los datos de usuario:
- Usuarios, perfiles, roles y permisos
- Bibliotecas, colecciones, listas de reproducción y favoritos
- Progreso de visionado e historial de reproducción
- Extensiones, configuración del servidor y tareas programadas
- Notificaciones, webhooks y registros de auditoría

**Lo que queda fuera**: los archivos multimedia (son suyos) y la caché de transcodificación (se regenera sola).

## Formato de la copia

Cada copia es un `.tar.gz` que contiene:
- `manifest.json` - metadatos, versión y hash SHA256 de integridad
- `dump.sql` - volcado completo de PostgreSQL

## Copia manual

Lance una copia desde el panel de administración o a través de la API:

```bash
curl -X POST -H "Authorization: Bearer <token>" http://localhost:9091/api/v1/admin/backups
```

## Restauración

La restauración a partir de una copia se hace desde la API de administración. Antes de aplicarla se comprueba la integridad del archivo mediante su hash SHA256.

## Almacenamiento persistente

Compruebe que `EVEY_BACKUP_DIR` apunta a un volumen persistente o a una ruta del host. El valor por defecto, `/data/backups`, queda dentro del volumen Docker `evey-data`.

Para guardar las copias en el host:
```yaml
services:
  evey:
    volumes:
      - /path/on/host/backups:/data/backups
```
