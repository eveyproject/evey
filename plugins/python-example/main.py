"""
Evey Python Plugin Example

Implements PluginService (host→plugin) over a Unix socket.
Calls HostService (plugin→host) to interact with Evey.

Environment variables set by Evey:
  EVEY_PLUGIN_ID   - unique plugin ID
  EVEY_PLUGIN_SOCK - Unix socket path this plugin must listen on
  EVEY_HOST_SOCK   - Unix socket path of the Evey host service
"""
import os
import signal
import sys
import threading
import time

import grpc

# Import generated stubs (run generate_proto.sh first)
from plugin.v1 import plugin_api_pb2 as pb
from plugin.v1 import plugin_api_pb2_grpc as pb_grpc


PLUGIN_ID = os.environ.get("EVEY_PLUGIN_ID", "python-example")
PLUGIN_SOCK = os.environ.get("EVEY_PLUGIN_SOCK", "/tmp/evey/plugins/python-example.sock")
HOST_SOCK = os.environ.get("EVEY_HOST_SOCK", "/tmp/evey/plugins/evey-host.sock")


def make_host_channel():
    """Connect to the Evey host gRPC service over the Unix socket."""
    return grpc.insecure_channel(f"unix://{HOST_SOCK}")


def host_metadata():
    """gRPC metadata that identifies this plugin to the host."""
    return [("x-plugin-id", PLUGIN_ID)]


class PluginServicer(pb_grpc.PluginServiceServicer):
    """Implements the PluginService interface (called by Evey host)."""

    def Initialize(self, request, context):
        print(f"[{PLUGIN_ID}] Initialize called with config: {dict(request.config)}")
        # Demonstrate calling the host service
        self._fetch_recent_items()
        return pb.InitResponse(success=True)

    def Shutdown(self, request, context):
        print(f"[{PLUGIN_ID}] Shutdown: {request.reason}")
        threading.Thread(target=self._stop_server, daemon=True).start()
        return pb.ShutdownResponse()

    def OnEvent(self, request, context):
        print(f"[{PLUGIN_ID}] Event received: {request.event_type}")
        return pb.EventResponse(handled=True)

    def HealthCheck(self, request, context):
        return pb.HealthCheckResponse(healthy=True, status="ok")

    def GetCapabilities(self, request, context):
        return pb.GetCapabilitiesResponse(
            api_version="1.0.0",
            capabilities=[
                pb.PluginCapability(
                    name="media.read",
                    description="Read media library items",
                    event_types=["media.added", "media.updated"],
                ),
                pb.PluginCapability(
                    name="notification.sender",
                    description="Send notifications via host event bus",
                ),
            ],
        )

    def _fetch_recent_items(self):
        """Example: list 5 items from the library using HostService."""
        try:
            channel = make_host_channel()
            stub = pb_grpc.HostServiceStub(channel)
            resp = stub.ListMediaItems(
                pb.ListMediaItemsRequest(limit=5),
                metadata=host_metadata(),
            )
            print(f"[{PLUGIN_ID}] Found {resp.total} items in library (showing {len(resp.items)})")
            for item in resp.items:
                print(f"  - [{item.type}] {item.title} ({item.year})")
            channel.close()
        except grpc.RpcError as e:
            print(f"[{PLUGIN_ID}] HostService error: {e.code()}: {e.details()}")

    def _stop_server(self):
        time.sleep(0.1)
        _server.stop(grace=2)


_server = None


def main():
    global _server

    if os.path.exists(PLUGIN_SOCK):
        os.remove(PLUGIN_SOCK)

    _server = grpc.server(
        grpc.experimental.gevent.ThreadPoolExecutor(max_workers=4)
        if False
        else __import__("concurrent.futures", fromlist=["ThreadPoolExecutor"]).ThreadPoolExecutor(max_workers=4)
    )
    pb_grpc.add_PluginServiceServicer_to_server(PluginServicer(), _server)
    _server.add_insecure_port(f"unix://{PLUGIN_SOCK}")
    _server.start()
    print(f"[{PLUGIN_ID}] Listening on {PLUGIN_SOCK}")

    def handle_sigterm(*_):
        print(f"[{PLUGIN_ID}] SIGTERM received")
        _server.stop(grace=5)
        sys.exit(0)

    signal.signal(signal.SIGTERM, handle_sigterm)

    try:
        _server.wait_for_termination()
    except KeyboardInterrupt:
        _server.stop(grace=2)


if __name__ == "__main__":
    main()
