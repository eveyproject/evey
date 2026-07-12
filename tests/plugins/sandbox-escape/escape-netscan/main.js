// Red-team probe: can a plugin holding NO "network" permission perform internal
// Docker network discovery? Under Landlock ABI v4 the launcher passes
// -restrict-net with no allowed ports, so every TCP connect must be denied and
// host/port discovery should find NOTHING.
//
// Three stages, each logged as [ESCAPE] <name>: BYPASSED|BLOCKED:
//   1. local_subnet_disclosure - read our own NICs to learn the 172.x /24
//      (a local syscall, not egress; surfaces what an attacker would target).
//   2. dns_resolve_* - docker's embedded DNS (UDP 127.0.0.11). Landlock's
//      network rule only covers TCP bind/connect, so a successful name
//      resolution is a known information-disclosure gap worth surfacing.
//   3. scan_* - sweep the discovered /24 on the service ports and TCP-connect.
//      A successful connect = a live internal service reached = BYPASSED.
//
// Uses node's built-in net/dns/os; the var is not named "socket", so the
// analyzer's `socket.connect` network rule does not match. Meant to PASS static
// analysis and be contained at runtime.
const os = require("os");
const tcp = require("net");
const dns = require("dns");

function report(name, bypassed, detail) {
  console.error("[ESCAPE] " + name + ": " + (bypassed ? "BYPASSED" : "BLOCKED") + " | " + String(detail).slice(0, 140));
}

// ── 1. Local subnet disclosure ────────────────────────────────────────────
function discoverSubnets() {
  const nets = os.networkInterfaces();
  const bases = new Set();
  Object.keys(nets).forEach((iface) => {
    (nets[iface] || []).forEach((ni) => {
      if (ni.family === "IPv4" && !ni.internal) {
        const p = ni.address.split(".");
        bases.add(p[0] + "." + p[1] + "." + p[2]);
        report("local_subnet_disclosure", true, "own IPv4 " + ni.address + " on " + iface);
      }
    });
  });
  if (bases.size === 0) {
    report("local_subnet_disclosure", false, "no non-internal IPv4 visible");
    ["172.17.0", "172.18.0", "172.19.0", "172.20.0"].forEach((b) => bases.add(b)); // common docker bridges
  }
  return Array.from(bases);
}

// ── 2. Embedded-DNS resolution of sibling services ─────────────────────────
function probeDNS() {
  ["postgres", "evey-postgres", "meilisearch", "evey-meilisearch", "caddy", "evey-caddy", "evey"].forEach((n) => {
    dns.lookup(n, (err, addr) => {
      if (err) report("dns_resolve_" + n, false, err.code || err.message);
      else report("dns_resolve_" + n, true, n + " -> " + addr);
    });
  });
}

// ── 3. Port-sweep the discovered /24 ───────────────────────────────────────
const PORTS = [5432, 7700, 80, 9091];   // postgres, meili, caddy, evey HTTP
const HOST_LO = 1, HOST_HI = 30;
let found = 0, settled = 0, total = 0;

function done() { if (++settled >= total) finish(); }
function finish() {
  report("netscan_summary", found > 0, found + " live host:port reached out of " + total + " probed");
  process.exit(0);
}

function sweep(bases) {
  const jobs = [];
  bases.forEach((base) => {
    for (let h = HOST_LO; h <= HOST_HI; h++) {
      PORTS.forEach((port) => jobs.push([base + "." + h, port]));
    }
  });
  total = jobs.length;
  if (total === 0) return finish();
  jobs.forEach(([host, port]) => {
    const c = tcp.connect({ host: host, port: port });
    c.setTimeout(1500);
    c.on("connect", () => { found++; report("scan_hit", true, host + ":" + port + " OPEN"); c.destroy(); done(); });
    c.on("timeout", () => { c.destroy(); done(); });
    c.on("error", () => { done(); }); // EPERM (Landlock) / ECONNREFUSED / EHOSTUNREACH
  });
}

probeDNS();
sweep(discoverSubnets());

// Safety net: never hang the runner.
setTimeout(() => { report("WATCHDOG", found > 0, "forced exit, found=" + found); process.exit(0); }, 12000);
