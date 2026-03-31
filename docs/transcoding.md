# Transcoding & Hardware Acceleration

Evey transcodes media on-the-fly when the client doesn't support the source format. FFmpeg handles all transcoding.

## Configuration

```env
EVEY_TRANSCODING_MAX_CONCURRENT=2       # Max simultaneous transcode jobs
EVEY_TRANSCODING_CACHE_MAX_GB=50        # Cache size limit in GB
EVEY_TRANSCODING_HW_ACCEL_ENABLED=false # Enable hardware acceleration
```

## How It Works

When a client requests playback, Evey checks codec compatibility:

1. **Direct Play** — client supports the format natively. No transcoding, file served as-is.
2. **Direct Stream** — container remux only (e.g. MKV → MP4). No re-encoding, fast.
3. **Transcode** — full re-encoding to HLS with adaptive bitrate (360p, 480p, 720p, 1080p).

The decision is automatic based on the client's reported capabilities.

## Hardware Acceleration

Enable with:
```env
EVEY_TRANSCODING_HW_ACCEL_ENABLED=true
```

### Intel (VAAPI)

Requires `/dev/dri/renderD128`. Add to `docker-compose.yml`:

```yaml
services:
  evey:
    devices:
      - /dev/dri:/dev/dri
```

### NVIDIA (NVENC)

Requires the NVIDIA Container Toolkit. Add to `docker-compose.yml`:

```yaml
services:
  evey:
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]
```

### Intel Quick Sync (QSV)

Requires `/dev/dri` access, same as VAAPI.

## Checking Detection

After starting Evey, check which hardware was detected:

```bash
curl http://localhost:9091/healthz
# Check Admin > System Info for detected devices
```

Or via the API:
```bash
curl -H "Authorization: Bearer <token>" http://localhost:9091/api/v1/admin/system
# Look at hw_devices in the response
```

## Transcode Cache

Transcoded segments are cached in `/data/transcode` (inside the container). The cache is automatically cleaned when it exceeds `EVEY_TRANSCODING_CACHE_MAX_GB`. Idle transcode sessions are removed after 10 minutes.
