# Server and network

The address the server is published at, who it trusts, who may reach it from
outside, and its limits on requests.

## Address and ports

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_SERVER_BASE_URL` | `http://localhost:9091` | The public address. An `https://` address turns on `Secure` cookies, HSTS and trust in `X-Forwarded-Proto`. E-mail links, single sign-on redirects and cast URLs are built from it, unless **Admin → Remote Access → External URL** is set, which wins. An `https://` External URL also turns HTTPS on when this one is plain HTTP. |
| `EVEY_SERVER_PORT` | `9090` | The internal gRPC port. The HTTP API and the web interface listen on this port plus one (`9091`). |
| `EVEY_GRPC_BIND` | `127.0.0.1` | The address the gRPC port listens on. The remote-access gate does not cover gRPC, so widening it (`0.0.0.0`) logs a warning and makes firewalling it your job. |
| `EVEY_SERVER_NAME` | `Evey` | The name shown on the sign-in page, in the apps' server list and in e-mails. **Admin → Server Config** overrides it. |

## Reverse proxy

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_TRUSTED_PROXIES` | loopback only | Networks whose forwarded client address is believed, comma-separated (for a proxy in a container: `172.16.0.0/12`, narrowed once known). Added to loopback, never replacing it. A list saved in **Admin → Remote Access → Trusted proxies** replaces this one live; emptying the field brings it back. |
| `EVEY_REAL_IP_HEADER` | `X-Forwarded-For` | The header a trusted proxy puts the client address in. |
| `EVEY_FORWARDED_PROTO` | derived | Deprecated: HTTPS is derived from the scheme of `EVEY_SERVER_BASE_URL`. Still accepted as an override, with a warning. |

## Remote access

These seed the first start; **Admin → Remote Access** then stores and applies
its own values without a restart.

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_REMOTE_ACCESS_ENABLED` | unset | Unset: inferred at startup, on when `EVEY_SERVER_BASE_URL` or `EVEY_REMOTE_ACCESS_EXTERNAL_URL` is public. `true` or `false`: your explicit choice, never overridden. |
| `EVEY_REMOTE_ACCESS_EXTERNAL_URL` | empty | The public address, when it differs from `EVEY_SERVER_BASE_URL`. An `https://` address turns HTTPS mode on, like the base URL. |
| `EVEY_REMOTE_ACCESS_LAN_NETWORKS` | `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16` | Networks treated as home. Add a VPN or CGNAT range (`100.64.0.0/10`) whose clients should count as local. |
| `EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS` | `0` | A bitrate ceiling for clients outside the home, in kbit/s; `0` = none. A private address is never capped. |

How the gate behaves: [Remote access and HTTPS](../administration/remote-access.md).

## Request limits

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_RATELIMIT_LOGIN_MAX` | `10` | Sign-in attempts per address per minute. **Admin → Server Config → Server Settings** overrides it. |
| `EVEY_RATELIMIT_GLOBAL_MAX` | `300` | Requests per address per minute on the API. Overridable in the same place. |

## Container user

Read by the entrypoint, not by the server.

| Variable | Default | Effect |
| --- | --- | --- |
| `PUID`, `PGID` | owner of the `/media` mounts | The user and group the server runs as. `1001` is refused (reserved for plugins). Details in the [Configuration reference](../configuration.md#file-ownership-puid-and-pgid). |

## Public demo

| Variable | Default | Effect |
| --- | --- | --- |
| `EVEY_DEMO` | `false` | Runs the server as the public demo: administration writes are locked, no e-mail, push or outgoing webhooks. Not for a real installation. |

<!-- sources
internal/config/config.go - EVEY_SERVER_BASE_URL, EVEY_SERVER_PORT, EVEY_GRPC_BIND, EVEY_SERVER_NAME, EVEY_REAL_IP_HEADER, EVEY_RATELIMIT_LOGIN_MAX, EVEY_RATELIMIT_GLOBAL_MAX, EVEY_REMOTE_ACCESS_*, EVEY_DEMO
trusted proxies: internal/config/trusted_proxies.go
remote access inference: internal/config/remote_access_resolve.go
puid/pgid: docker/entrypoint.sh
-->
