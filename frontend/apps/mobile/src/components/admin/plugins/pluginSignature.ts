export interface PluginSignatureInfo {
  verified?: boolean;
  keyFingerprint?: string;
  publisher?: string;
  trusted?: boolean;
  continuity?: string;
  error?: string;
}

export type SignatureKind = "trusted" | "untrusted" | "unsigned" | "invalid";

// A key that did not verify with a real error is "invalid"; a package with no
// key or no .sig file is simply "unsigned" (mirror web pluginSignature).
export function signatureKind(sig?: PluginSignatureInfo | null): SignatureKind {
  if (!sig) return "unsigned";
  if (sig.verified) return sig.trusted ? "trusted" : "untrusted";
  if (sig.keyFingerprint && sig.error && !/no signature/i.test(sig.error)) return "invalid";
  return "unsigned";
}

export function shortFingerprint(fp?: string): string {
  if (!fp) return "";
  return `sha256:${fp.replace(/^sha256:/, "").slice(0, 8)}...`;
}
