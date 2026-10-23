import type { V1Plugin } from "@evey/api";

export type PluginSignature = NonNullable<V1Plugin["signature"]>;

export type SignatureKind = "trusted" | "untrusted" | "unsigned" | "invalid";

// A key that did not verify with a real error is "invalid"; a package with no
// key or no .sig file is simply "unsigned".
export function signatureKind(sig?: PluginSignature | null): SignatureKind {
  if (!sig) return "unsigned";
  if (sig.verified) return sig.trusted ? "trusted" : "untrusted";
  if (sig.keyFingerprint && sig.error && !/no signature/i.test(sig.error)) return "invalid";
  return "unsigned";
}

export function shortFingerprint(fp?: string): string {
  if (!fp) return "";
  const hex = fp.replace(/^sha256:/, "");
  return `sha256:${hex.slice(0, 8)}...`;
}
