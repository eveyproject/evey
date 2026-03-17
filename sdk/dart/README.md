# Evey Plugin SDK — Dart

Proto file for Evey plugin development.

## Usage

```bash
protoc --dart_out=grpc:lib/src/generated plugin_api.proto
```

Requires `protoc-gen-dart` (`dart pub global activate protoc_plugin`).
