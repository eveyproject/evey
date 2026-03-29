"""
Webhook Receiver Plugin

Flow:
1. Evey starts this process and calls Initialize via gRPC.
2. When an external service sends a webhook to Evey (e.g. POST /api/webhooks/<plugin-id>),
   the host forwards it as an OnEvent with event_type="webhook.incoming" and the
   HTTP body as the payload.
3. This plugin logs the event. A real plugin could trigger a library scan,
   send a notification, update metadata, etc.
"""

import os
import json
import logging
from concurrent import futures

import grpc
import plugin_api_pb2 as pb
import plugin_api_pb2_grpc as rpc

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger("webhook-example")

HOST_ADDR = os.environ.get("EVEY_HOST_ADDR", "localhost:50051")
PLUGIN_PORT = os.environ.get("EVEY_PLUGIN_PORT", "50052")


class WebhookPlugin(rpc.PluginServiceServicer):
    def __init__(self):
        # gRPC channel to call back into the Evey host
        channel = grpc.insecure_channel(HOST_ADDR)
        self.host = rpc.HostServiceStub(channel)

    def Initialize(self, request, context):
        log.info("initialize: plugin_id=%s", request.plugin_id)
        return pb.InitResponse(success=True)

    def Shutdown(self, request, context):
        log.info("shutdown: reason=%s", request.reason)
        return pb.ShutdownResponse()

    def OnEvent(self, request, context):
        if request.event_type != "webhook.incoming":
            return pb.EventResponse(handled=False)

        # Log the raw webhook payload
        payload = {}
        if request.payload:
            for key, val in request.payload.fields.items():
                payload[key] = val.string_value
        log.info("webhook received: %s", json.dumps(payload, indent=2))

        # Example: use the host API to read a media item
        # item = self.host.GetMediaItem(pb.GetMediaItemRequest(id="some-id"))

        return pb.EventResponse(handled=True)

    def HealthCheck(self, request, context):
        return pb.HealthCheckResponse(healthy=True, status="ok")

    def GetCapabilities(self, request, context):
        return pb.GetCapabilitiesResponse(
            api_version="1",
            capabilities=[
                pb.PluginCapability(
                    name="webhook.receiver",
                    description="Receives incoming webhook events",
                    event_types=["webhook.incoming"],
                )
            ],
        )


def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=4))
    rpc.add_PluginServiceServicer_to_server(WebhookPlugin(), server)
    server.add_insecure_port("0.0.0.0:" + PLUGIN_PORT)
    server.start()
    log.info("webhook plugin listening on port %s", PLUGIN_PORT)
    server.wait_for_termination()


if __name__ == "__main__":
    serve()
