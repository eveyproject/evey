const COMMON_PORTS = [9091, 8096, 8920];
const TIMEOUT_MS = 2000;

interface DiscoveredServer {
  url: string;
  version?: string;
  // Admin-chosen server name from /healthz, so a scan row can read
  // "Salon" rather than a bare IP:port.
  name?: string;
}

async function probeHost(ip: string, port: number, outer?: AbortSignal): Promise<DiscoveredServer | null> {
  const url = `http://${ip}:${port}`;
  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), TIMEOUT_MS);
    // Chain the caller's signal so leaving the screen kills the in-flight
    // probes too, not just the not-yet-started batches.
    const onAbort = () => controller.abort();
    outer?.addEventListener("abort", onAbort);
    if (outer?.aborted) controller.abort();
    let res: Response;
    try {
      res = await fetch(`${url}/healthz`, { signal: controller.signal });
    } finally {
      outer?.removeEventListener("abort", onAbort);
    }
    clearTimeout(timeout);
    if (res.ok) {
      let version: string | undefined;
      let name: string | undefined;
      try {
        const data = await res.json();
        version = data?.version;
        name = typeof data?.name === "string" && data.name.trim() ? data.name.trim() : undefined;
      } catch {
        // A healthz that is not JSON is still a server; the version and
        // name are just unknown.
      }
      return { url, version, name };
    }
  } catch {
    // A probe that times out or is refused is the EXPECTED outcome for
    // ~3000 of the ~3000 addresses a scan tries - not something to log.
  }
  return null;
}

export async function scanLocalNetwork(
  onFound: (server: DiscoveredServer) => void,
  opts?: { signal?: AbortSignal },
): Promise<void> {
  // Get common local subnets
  const subnets = ["192.168.1", "192.168.0", "10.0.0", "172.16.0"];
  // Thunks, not promises: creating a promise STARTS its fetch, so the old
  // array of promises fired all ~3000 probes at once and the "batches" below
  // only awaited them - the flood the batching claimed to avoid.
  const probes: Array<() => Promise<void>> = [];

  for (const subnet of subnets) {
    for (let host = 1; host <= 254; host++) {
      const ip = `${subnet}.${host}`;
      for (const port of COMMON_PORTS) {
        probes.push(() =>
          probeHost(ip, port, opts?.signal).then((result) => {
            // aborted = the screen is gone; a late straggler must not report.
            if (result && !opts?.signal?.aborted) onFound(result);
          }),
        );
      }
    }
  }

  // Run in batches to avoid flooding
  const BATCH = 50;
  for (let i = 0; i < probes.length; i += BATCH) {
    if (opts?.signal?.aborted) return;
    await Promise.allSettled(probes.slice(i, i + BATCH).map((run) => run()));
  }
}
