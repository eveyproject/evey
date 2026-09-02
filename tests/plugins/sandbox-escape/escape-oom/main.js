// Grabs memory as fast as it can, forever. There is no malice here - a leak in
// a careless plugin looks identical - and that is the point: the host must cap
// it whatever the intent. Expected: the resource monitor (EVEY_PLUGINS_MEMORY_MB)
// stops THIS process and the server does not so much as hiccup. If the box goes
// down with it, the cap is not holding.
function report(name, detail) {
  console.error("[OOM] " + name + ": " + String(detail).slice(0, 120));
}
const chunks = [];
let mb = 0;
report("balloon_start", "growing until the monitor stops us");
const t = setInterval(function () {
  try {
    // 8 MB of non-compressible, retained bytes per tick.
    chunks.push(Buffer.alloc(8 * 1024 * 1024, mb & 0xff));
    mb += 8;
    if (mb % 64 === 0) report("balloon_progress", mb + " MB retained");
  } catch (e) {
    clearInterval(t);
    report("balloon_alloc_failed", e.message);
  }
}, 50);
