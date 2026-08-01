import { Alert } from "react-native";

// Is this server address one that stays on the local network?
//
// Why this lives in TypeScript and not in Android's network_security_config:
// that file discriminates by exact domain or exact IP literal - it has no
// notion of a CIDR range - so "every private address" cannot be expressed
// there at all. Setting cleartextTrafficPermitted="false" would have blocked
// http://192.168.x.x, which IS the normal way to reach a self-hosted Evey.
// The app, on the other hand, can read the host and decide.

/** RFC1918 + loopback + link-local + CGNAT, and the IPv6 equivalents. */
function isPrivateIPv4(host: string): boolean {
  const m = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/.exec(host);
  if (!m) return false;
  const [a, b] = [Number(m[1]), Number(m[2])];
  if (m.slice(1).some((o) => Number(o) > 255)) return false;
  if (a === 10 || a === 127) return true;
  if (a === 192 && b === 168) return true;
  if (a === 172 && b >= 16 && b <= 31) return true;
  if (a === 169 && b === 254) return true; // link-local
  if (a === 100 && b >= 64 && b <= 127) return true; // CGNAT, common on box networks
  return false;
}

function isPrivateIPv6(host: string): boolean {
  const h = host.replace(/^\[|\]$/g, "").toLowerCase();
  if (h === "::1") return true;
  if (h.startsWith("fe80:")) return true; // link-local
  // Unique local addresses: fc00::/7, i.e. fc.. and fd..
  return /^f[cd][0-9a-f]{2}:/.test(h);
}

/**
 * True when reaching this address cannot leave the local network, so plain
 * HTTP carries no exposure beyond the home LAN.
 *
 * A bare hostname with no dot (`nas`, `evey`) counts as local: that is how a
 * machine is addressed on a home network. So do the local-network suffixes
 * used by mDNS and routers.
 */
export function isPrivateHost(host: string): boolean {
  const h = host.trim().toLowerCase().replace(/\.$/, "");
  if (!h) return false;
  if (h === "localhost") return true;
  if (isPrivateIPv4(h) || isPrivateIPv6(h)) return true;
  // Colons mean IPv6, and the checks above already covered every private form
  // of it. This MUST come before the dotless-hostname rule below: an IPv6
  // address has no dots, so "2001:4860::1" - a perfectly public address - was
  // being read as a LAN machine name and waved through without a warning.
  if (h.includes(":")) return false;
  if (!h.includes(".")) return true;
  return /\.(local|lan|home|internal|home\.arpa)$/.test(h);
}

/**
 * True when this server URL would send credentials and video in the clear
 * ACROSS the internet - HTTP towards a host that is not on the local network.
 * HTTP to the LAN is not flagged: it is the normal, intended setup.
 */
export function isCleartextToPublicHost(rawUrl: string): boolean {
  let parsed: URL;
  try {
    parsed = new URL(rawUrl);
  } catch {
    return false; // not parseable: the caller's own validation reports it
  }
  if (parsed.protocol !== "http:") return false;
  return !isPrivateHost(parsed.hostname);
}

/**
 * Asks before connecting to a plain-HTTP server out on the internet. Not a
 * refusal: someone may knowingly run a private VPS without TLS, and Evey is
 * not in a position to forbid it. What it prevents is typing an address by
 * habit and never being told the password will travel in the clear.
 */
export function confirmInsecureServer(
  t: (key: string, fallback: string) => string,
  url: string,
): Promise<boolean> {
  return new Promise<boolean>((resolve) => {
    Alert.alert(
      t("mobile.serverConnect.insecureTitle", "This connection is not encrypted"),
      t(
        "mobile.serverConnect.insecureBody",
        "{{url}} is outside your home network and uses http://. Your password and everything you watch would travel unencrypted. Use https:// if your server supports it.",
      ).replace("{{url}}", url),
      [
        { text: t("common.cancel", "Cancel"), style: "cancel", onPress: () => resolve(false) },
        { text: t("mobile.serverConnect.connectAnyway", "Connect anyway"), onPress: () => resolve(true) },
      ],
      { cancelable: true, onDismiss: () => resolve(false) },
    );
  });
}
