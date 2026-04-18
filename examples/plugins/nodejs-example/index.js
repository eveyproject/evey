'use strict';
/**
 * Evey Node.js Plugin Example
 *
 * Implements PluginService (host→plugin) over a Unix socket.
 * Calls HostService (plugin→host) to interact with Evey.
 *
 * Environment variables set by Evey:
 *   EVEY_PLUGIN_ID   - unique plugin ID
 *   EVEY_PLUGIN_SOCK - Unix socket path this plugin must listen on
 *   EVEY_HOST_SOCK   - Unix socket path of the Evey host service
 */

const path = require('path');
const fs = require('fs');
const grpc = require('@grpc/grpc-js');
const protoLoader = require('@grpc/proto-loader');

const PLUGIN_ID = process.env.EVEY_PLUGIN_ID || 'nodejs-example';
const PLUGIN_SOCK = process.env.EVEY_PLUGIN_SOCK || '/tmp/evey/plugins/nodejs-example.sock';
const HOST_SOCK = process.env.EVEY_HOST_SOCK || '/tmp/evey/plugins/evey-host.sock';

// Load proto from repo root (two levels up from plugins/nodejs-example)
const PROTO_PATH = path.resolve(__dirname, '../../proto/plugin/v1/plugin_api.proto');

const packageDefinition = protoLoader.loadSync(PROTO_PATH, {
  keepCase: true,
  longs: String,
  enums: String,
  defaults: true,
  oneofs: true,
  includeDirs: [path.resolve(__dirname, '../../proto')],
});

const pluginProto = grpc.loadPackageDefinition(packageDefinition)['plugin']['v1'];

/** Create a client stub connecting to the Evey host service. */
function makeHostClient() {
  return new pluginProto.HostService(
    `unix://${HOST_SOCK}`,
    grpc.credentials.createInsecure()
  );
}

// --- PluginService handlers ---

function initialize(call, callback) {
  console.log(`[${PLUGIN_ID}] Initialize called, config:`, call.request.config);
  // Demonstrate fetching library items from host
  fetchRecentItems().finally(() => {
    callback(null, { success: true, error: '' });
  });
}

function shutdown(call, callback) {
  console.log(`[${PLUGIN_ID}] Shutdown: ${call.request.reason}`);
  callback(null, {});
  setTimeout(() => process.exit(0), 200);
}

function onEvent(call, callback) {
  console.log(`[${PLUGIN_ID}] Event: ${call.request.event_type}`);
  callback(null, { handled: true });
}

function healthCheck(call, callback) {
  callback(null, { healthy: true, status: 'ok', details: {} });
}

function getCapabilities(call, callback) {
  callback(null, {
    api_version: '1.0.0',
    capabilities: [
      {
        name: 'metadata.provider',
        description: 'Provide metadata for media items',
        event_types: [],
      },
      {
        name: 'media.read',
        description: 'Read media library items',
        event_types: ['media.added', 'media.updated'],
      },
    ],
  });
}

/** Example: list items from the Evey library via HostService. */
async function fetchRecentItems() {
  return new Promise((resolve) => {
    const client = makeHostClient();
    client.listMediaItems(
      { limit: 5, offset: 0, library_id: '', type: '', parent_id: '' },
      (err, response) => {
        client.close();
        if (err) {
          console.error(`[${PLUGIN_ID}] HostService error:`, err.message);
          return resolve();
        }
        console.log(`[${PLUGIN_ID}] Library has ${response.total} items (showing ${response.items.length})`);
        for (const item of response.items) {
          console.log(`  - [${item.type}] ${item.title} (${item.year})`);
        }
        resolve();
      }
    );
  });
}

// --- gRPC server ---

function main() {
  if (fs.existsSync(PLUGIN_SOCK)) {
    fs.unlinkSync(PLUGIN_SOCK);
  }

  const server = new grpc.Server();
  server.addService(pluginProto.PluginService.service, {
    Initialize: initialize,
    Shutdown: shutdown,
    OnEvent: onEvent,
    HealthCheck: healthCheck,
    GetCapabilities: getCapabilities,
  });

  server.bindAsync(`unix://${PLUGIN_SOCK}`, grpc.ServerCredentials.createInsecure(), (err, port) => {
    if (err) {
      console.error(`[${PLUGIN_ID}] Failed to bind:`, err);
      process.exit(1);
    }
    console.log(`[${PLUGIN_ID}] Listening on ${PLUGIN_SOCK}`);
  });

  process.on('SIGTERM', () => {
    console.log(`[${PLUGIN_ID}] SIGTERM received`);
    server.tryShutdown(() => process.exit(0));
  });

  process.on('SIGINT', () => {
    server.tryShutdown(() => process.exit(0));
  });
}

main();
