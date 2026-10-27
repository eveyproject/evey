// Example Metadata Provider Plugin
//
// Flow:
// 1. Evey host starts this process and calls Initialize via gRPC.
// 2. Each time Evey enriches a movie or a series from TMDB, it sends an
//    OnEvent with event_type="metadata.request". The payload carries the
//    item's media_item_id, type ("movie" or "series"), title, original_title,
//    year (absent when unknown), provider_ids ({tmdb, imdb, ...}), language
//    (the locale of the TMDB text) and locales (every locale the server shows).
// 3. This plugin looks the item up in its own source (stubbed here) and
//    answers in the EventResponse result: {locale, title, overview, tagline},
//    plus {translations: {<locale>: {title, overview, tagline}}} for other
//    locales. Leave out a field you do not cover; answer handled=false when
//    you do not know the item.
// 4. Evey keeps the answer beside TMDB's text and shows the fields the
//    administrator lets this plugin replace (the overview and the tagline
//    unless they choose otherwise). TMDB stays the item's identity.

const grpc = require("@grpc/grpc-js");
const protoLoader = require("@grpc/proto-loader");
const path = require("path");

const PROTO_PATH = path.join(__dirname, "plugin_api.proto");
const pkgDef = protoLoader.loadSync(PROTO_PATH, { keepCase: true });
const proto = grpc.loadPackageDefinition(pkgDef).plugin.v1;

const PLUGIN_SOCK = process.env.EVEY_PLUGIN_SOCK;
if (!PLUGIN_SOCK) {
  console.error("EVEY_PLUGIN_SOCK not set");
  process.exit(1);
}

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
        locale: { string_value: "en" },
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
server.bindAsync(`unix://${PLUGIN_SOCK}`, grpc.ServerCredentials.createInsecure(), (err) => {
  if (err) {
    console.error("bind failed:", err);
    process.exit(1);
  }
  console.error(`metadata plugin listening on ${PLUGIN_SOCK}`);
});
