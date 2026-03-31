# Transcoding

Evey transcodes media on-the-fly when the client doesn't support the source format.

## How It Works

Evey picks the best playback method automatically:
1. **Direct Play** — format supported, no processing
2. **Remux** — repackage container (MKV → MP4), no re-encoding
3. **Transcode** — re-encode to HLS with adaptive bitrate

## Configuration

```env
EVEY_TRANSCODING_MAX_CONCURRENT=2
EVEY_TRANSCODING_HW_ACCEL_ENABLED=false
```

## Hardware Acceleration

### Intel (VAAPI)

```yaml
# docker-compose.override.yml or add to docker-compose.yml
services:
  evey:
    devices:
      - /dev/dri:/dev/dri
```

```env
EVEY_TRANSCODING_HW_ACCEL_ENABLED=true
```

### NVIDIA (NVENC)

Install the [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html), then:

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

```env
EVEY_TRANSCODING_HW_ACCEL_ENABLED=true
```

### Intel Quick Sync (QSV)

Same as VAAPI — expose `/dev/dri`.

## Verify

Check Admin > System Info for detected hardware devices after starting.
