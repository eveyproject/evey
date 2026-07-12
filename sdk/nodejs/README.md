# Evey Plugin SDK - Node.js

Proto file for Evey plugin development. Loaded at runtime via `@grpc/proto-loader`.

## Usage

```js
const protoLoader = require("@grpc/proto-loader");
const grpc = require("@grpc/grpc-js");
const pkgDef = protoLoader.loadSync("plugin_api.proto", { keepCase: true });
const proto = grpc.loadPackageDefinition(pkgDef).plugin.v1;
```
