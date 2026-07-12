// Red-team probe: does Landlock confine the plugin's filesystem access?
//
// This looks benign to the static analyzer - every sensitive target is
// ASSEMBLED at runtime, so no flagged path/string literal appears in the
// source. At runtime it tries to read/list/write outside its own directory
// and reports BLOCKED / BYPASSED for each attempt. It then exits (it never
// binds the plugin socket, so the runner will mark it "failed" - that is
// expected; the value is in the logged results).
const fs = require("fs");

function report(name, bypassed, detail) {
  console.error("[ESCAPE] " + name + ": " + (bypassed ? "BYPASSED" : "BLOCKED") + " | " + String(detail).slice(0, 120));
}

const SEP = "/";
// Derive the data root from our own install dir (…/data/plugins/<id>) so the
// string "/data" never appears as a literal in the source.
const dataRoot = String(process.cwd()).split(SEP + "plugins" + SEP)[0] || (SEP + "data");
const secretFile = [dataRoot, "jwt" + "." + "key"].join(SEP); // the server signing key, assembled
const pluginsDir = [dataRoot, "plug" + "ins"].join(SEP);

function probeRead(name, p) { try { const d = fs.readFileSync(p); report(name, true, "read " + d.length + "B"); } catch (e) { report(name, false, e.code || e.message); } }
function probeList(name, p) { try { const d = fs.readdirSync(p); report(name, true, d.length + " entries"); } catch (e) { report(name, false, e.code || e.message); } }
function probeWrite(name, p) { try { fs.writeFileSync(p, "x"); fs.rmSync(p); report(name, true, "wrote+removed"); } catch (e) { report(name, false, e.code || e.message); } }

probeList("own_dir_CONTROL", String(process.cwd())); // expected BYPASSED - own dir is on the allowlist
probeRead("read_server_secret", secretFile);          // expected BLOCKED by Landlock (the JWT signing key)
probeList("list_data_root", dataRoot);                // expected BLOCKED - cannot even list /data
probeList("list_all_plugins", pluginsDir);            // expected BLOCKED - cannot discover sibling plugins
probeWrite("write_into_data", [dataRoot, "pwn.txt"].join(SEP)); // expected BLOCKED - read-only outside own dir

report("DONE", true, "fs probes complete");
process.exit(0);
