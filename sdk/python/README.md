# Evey Plugin SDK - Python

Pre-generated gRPC stubs for Evey plugin development.

## Usage

Copy `plugin_api_pb2.py` and `plugin_api_pb2_grpc.py` into your plugin directory, or let Evey copy them automatically at install.

```python
import plugin_api_pb2 as pb
import plugin_api_pb2_grpc as rpc
```

Requires `grpcio` at runtime (provided by the Evey image).
