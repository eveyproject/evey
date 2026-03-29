// Example Metadata Provider Plugin
//
// Flow:
// 1. Evey host starts this process and calls Initialize via gRPC.
// 2. When a library scan finds new or updated media, the host sends an
//    OnEvent with event_type="metadata.request" and a payload containing
//    the media item's title, year, type, and existing provider IDs.
// 3. This plugin looks up metadata from an external source (stubbed here)
//    and returns updated fields in the EventResponse.
// 4. The host applies the returned metadata via UpdateMetadata.

const grpc = require("@grpc/grpc-js");
const protoLoader = require("@grpc/proto-loader");
const path = require("path");

const PROTO_PATH = path.join(__dirname, "plugin_api.proto");
const pkgDef = protoLoader.loadSync(PROTO_PATH, { keepCase: true });
const proto = grpc.loadPackageDefinition(pkgDef).plugin.v1;

const HOST_ADDR = process.env.EVEY_HOST_ADDR || "localhost:50051";
const PLUGIN_PORT = process.env.EVEY_PLUGIN_PORT || "50052";

// gRPC client to call back into the Evey host
const hostClient = new proto.HostService(HOST_ADDR, grpc.credentials.createInsecure());

// PluginService implementation (host -> plugin)
const pluginImpl = {
  Initialize(call, callback) {
    console.log("initialize: plugin_id=%s", call.request.plugin_id);
    callback(null, { success: true });
  },

  Shutdown(call, callback) {
    console.log("shutdown: reason=%s", call.request.reason);
    callback(null, {});
    setTimeout(() => process.exit(0), 500);
  },

  OnEvent(call, callback) {
    const evt = call.request;
    if (evt.event_type !== "metadata.request") {
      return callback(null, { handled: false });
    }

    const payload = evt.payload ? evt.payload.fields : {};
    const title = payload.title && payload.title.string_value;
    console.log("metadata request for: %s", title);

    // Stub: in a real plugin, query an external API here
    const result = {
      fields: {
        overview: { string_value: "Metadata fetched by example-metadata plugin" },
      },
    };
    callback(null, { handled: true, result: result });
  },

  HealthCheck(_call, callback) {
    callback(null, { healthy: true, status: "ok" });
  },

  GetCapabilities(_call, callback) {
    callback(null, {
      api_version: "1",
      capabilities: [{ name: "metadata.provider", description: "Provides metadata lookups" }],
    });
  },
};

// Start gRPC server
const server = new grpc.Server();
server.addService(proto.PluginService.service, pluginImpl);
server.bindAsync("0.0.0.0:" + PLUGIN_PORT, grpc.ServerCredentials.createInsecure(), (err) => {
  if (err) {
    console.error("bind failed:", err);
    process.exit(1);
  }
  console.log("metadata plugin listening on port %s", PLUGIN_PORT);
});
