# Server API

The server's HTTP API, described as OpenAPI (Swagger 2.0) files, one per
service. This index tells you which folder holds what and how a call is
authenticated; the files themselves are the reference.

| Section | Contents |
|---------|-------------|
| [Public API](public/README.md) | What a signed-in user can do: browse, play, collect, request. |
| [Admin API](admin/README.md) | Server administration: configuration, transcoding, backups, metadata, users. |
| [Plugin API](plugins/README.md) | Plugin management, webhooks, and the preferences a plugin's interface keeps per user. |

Every `.swagger.json` file imports into any OpenAPI tool (Swagger UI, Postman,
Insomnia). The HTTP routes are generated from the gRPC definitions in
`proto/evey/v1/`, so a method name in a file is the gRPC method behind it.

## Authentication

Three kinds of credential reach the API, all in the `Authorization` header:

| Credential | Who | How |
|---|---|---|
| Session token | A person signed in through an app | `Bearer <token>`, obtained by signing in and renewed by the app. |
| API key | A script, on behalf of the administrator who created it | `Bearer evey_...` (the `Bearer` prefix is optional). Scoped, revocable: [Webhooks and API keys](../webhooks-api-keys.md). |
| Plugin token | A plugin's interface, through the host page | Minted for five minutes for one plugin and one user, carrying only the `public` and `prefs` scopes the plugin was granted; every other method refuses it. The frame never holds it: the host page proxies the calls. |

A plugin token can never reach an administration method, whatever its scopes
([ADR-0003](../adr/0003-plugin-scoped-tokens.md),
[ADR-0012](../adr/0012-plugin-token-deny-by-default.md)).

## Regeneration

For contributors: the files are regenerated from the protobuf definitions.

```bash
buf generate --template buf.gen.local.yaml
python3 docs/api/sync.py
```

<!-- sources
session token, api key, plugin token: internal/adapters/driving/grpcapi/interceptors/auth.go - authenticate, authenticateAPIKey, authorizePluginToken
plugin token: internal/app/plugin/token_service.go - PluginTokenTTL ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes ; docs/adr/0003-plugin-scoped-tokens.md ; docs/adr/0012-plugin-token-deny-by-default.md
generation: docs/api/sync.py ; buf.gen.local.yaml
-->
