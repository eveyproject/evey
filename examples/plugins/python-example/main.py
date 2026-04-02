"""
Evey plugin example (Python).

This plugin listens for media events and logs them via the host Log RPC.
It demonstrates the PluginService (host→plugin) and HostService (plugin→host) contract.
"""
import os
import sys
import grpc
from concurrent import futures

# Generated from proto/plugin/v1/plugin_api.proto
# pip install grpcio grpcio-tools
# python -m grpc_tools.protoc -I../../proto --python_out=. --grpc_python_out=. plugin/v1/plugin_api.proto
import plugin_api_pb2 as pb
import plugin_api_pb2_grpc as rpc

PLUGIN_ID = os.environ.get("EVEY_PLUGIN_ID", "unknown")
PLUGIN_SOCK = os.environ.get("EVEY_PLUGIN_SOCK", "/tmp/evey/plugins/python-hello.sock")
HOST_SOCK = os.environ.get("EVEY_HOST_SOCK", "/tmp/evey/plugins/evey-host.sock")


class PluginServicer(rpc.PluginServiceServicer):
    def __init__(self, host_stub):
        self.host = host_stub
        self.config = {}

    def Initialize(self, request, context):
        self.config = dict(request.config)
        self.host.Log(pb.LogRequest(
            level=pb.LOG_LEVEL_INFO,
            message=f"plugin initialized (host {request.host_version})",
        ), metadata=[("x-plugin-id", PLUGIN_ID)])
        return pb.InitResponse(success=True)

    def Shutdown(self, request, context):
        self.host.Log(pb.LogRequest(
            level=pb.LOG_LEVEL_INFO,
            message=f"shutting down: {request.reason}",
        ), metadata=[("x-plugin-id", PLUGIN_ID)])
        return pb.ShutdownResponse()

    def OnEvent(self, request, context):
        self.host.Log(pb.LogRequest(
            level=pb.LOG_LEVEL_INFO,
            message=f"event received: {request.event_type}",
            fields={"source": request.source},
        ), metadata=[("x-plugin-id", PLUGIN_ID)])
        return pb.EventResponse(handled=True)

    def HealthCheck(self, request, context):
        return pb.HealthCheckResponse(healthy=True, status="running")

    def GetCapabilities(self, request, context):
        return pb.GetCapabilitiesResponse(
            api_version="1.0.0",
            capabilities=[
                pb.PluginCapability(
                    name="media.read",
                    description="Logs media events",
                    event_types=["media.added", "media.updated", "media.deleted"],
                ),
            ],
        )


def main():
    host_channel = grpc.insecure_channel(f"unix://{HOST_SOCK}")
    host_stub = rpc.HostServiceStub(host_channel)

    server = grpc.server(futures.ThreadPoolExecutor(max_workers=2))
    rpc.add_PluginServiceServicer_to_server(PluginServicer(host_stub), server)
    server.add_insecure_port(f"unix://{PLUGIN_SOCK}")
    server.start()

    print(f"[python-hello] listening on {PLUGIN_SOCK}", file=sys.stderr)
    server.wait_for_termination()


if __name__ == "__main__":
    main()
