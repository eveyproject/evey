const COMMON_PORTS = [9091, 8096, 8920];
const TIMEOUT_MS = 2000;

interface DiscoveredServer {
  url: string;
  version?: string;
}

async function probeHost(ip: string, port: number): Promise<DiscoveredServer | null> {
  const url = `http://${ip}:${port}`;
  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), TIMEOUT_MS);
    const res = await fetch(`${url}/healthz`, { signal: controller.signal });
    clearTimeout(timeout);
    if (res.ok) {
      let version: string | undefined;
      try {
        const data = await res.json();
        version = data?.version;
      } catch {}
      return { url, version };
    }
  } catch {}
  return null;
}

export async function scanLocalNetwork(onFound: (server: DiscoveredServer) => void): Promise<void> {
  // Get common local subnets
  const subnets = ["192.168.1", "192.168.0", "10.0.0", "172.16.0"];
  const promises: Promise<void>[] = [];

  for (const subnet of subnets) {
    for (let host = 1; host <= 254; host++) {
      const ip = `${subnet}.${host}`;
      for (const port of COMMON_PORTS) {
        promises.push(
          probeHost(ip, port).then((result) => {
            if (result) onFound(result);
          }),
        );
      }
    }
  }

  // Run in batches to avoid flooding
  const BATCH = 50;
  for (let i = 0; i < promises.length; i += BATCH) {
    await Promise.allSettled(promises.slice(i, i + BATCH));
  }
}
