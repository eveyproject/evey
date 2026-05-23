// Red-team probe: data-store isolation + HostService permission enforcement.
//
// This plugin declares NO permissions. It verifies two boundaries of the
// per-plugin data store + capability model:
//   1. The data store is usable for the plugin's OWN namespace with no
//      permission — and there is NO API field to name another plugin, so a
//      plugin physically cannot reach a sibling's data (isolation by design).
//   2. Permissioned HostService RPCs (e.g. GetMediaItem needs media.read) stay
//      DENIED for a plugin that did not declare the permission.
// Logs BYPASSED / BLOCKED per probe, then exits (the value is in the logs).
const grpc = require("@grpc/grpc-js");
const protoLoader = require("@grpc/proto-loader");
const path = require("path");

const HOST_SOCK = process.env.EVEY_HOST_SOCK || "/tmp/evey/plugins/evey-host.sock";
const pkgDef = protoLoader.loadSync(path.join(__dirname, "plugin_api.proto"), {
  keepCase: true, longs: String, enums: String, defaults: true, oneofs: true,
});
const proto = grpc.loadPackageDefinition(pkgDef).plugin.v1;
const host = new proto.HostService(`unix://${HOST_SOCK}`, grpc.credentials.createInsecure());

function report(name, bypassed, detail) {
  console.error("[ESCAPE] " + name + ": " + (bypassed ? "BYPASSED" : "BLOCKED") + " | " + String(detail).slice(0, 120));
}
const call = (fn, req) => new Promise((res) => host[fn](req, (e, r) => res({ e, r })));

(async () => {
  // CONTROL: own data store works without any permission (expected BYPASSED).
  {
    const set = await call("SetData", { key: "redteam", value: "x" });
    report("own_data_store_CONTROL", !set.e, set.e ? set.e.message : "set ok");
  }

  // There is no API field to target another plugin's namespace — the closest a
  // hostile plugin can do is craft a weird key; it still lands in its OWN
  // namespace. Document that cross-plugin access has no surface.
  {
    const get = await call("GetData", { key: "../other-plugin/secret" });
    // "found:false" in OUR namespace — never another plugin's data.
    report("cross_plugin_data_read", get.r && get.r.found === true, get.e ? get.e.message : ("found=" + (get.r && get.r.found)));
  }

  // Permissioned RPC without the permission must be denied (no media.read).
  {
    const g = await call("GetMediaItem", { id: "any" });
    const denied = g.e && /permission|denied|PermissionDenied/i.test(g.e.message || "");
    report("getmediaitem_without_perm", !denied && !g.e, g.e ? g.e.message : "ALLOWED");
  }

  report("DONE", true, "data-store probes complete");
  process.exit(0);
})();
