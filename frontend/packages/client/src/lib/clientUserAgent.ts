// The product User-Agent of the native apps: "EveyMobile/0.0.1-beta.66
// (Android 14; SM-G991B)", "EveyTV/0.1.7 (Android TV 12; MiTV-AESP0)". The
// build sits in the product token so a log line says WHICH build spoke; the
// admin panel recognizes the product by its prefix and reads the model between
// "; " and ")". The television sent no build until 2026-09-24.
export function clientUserAgent(parts: {
  product: "EveyMobile" | "EveyTV";
  version?: string | null;
  os: string;
  release?: string | number;
  model?: string;
}): string {
  const version = parts.version ? `/${parts.version}` : "";
  const release = parts.release != null && parts.release !== "" ? ` ${parts.release}` : "";
  const model = parts.model ? `; ${parts.model}` : "";
  return `${parts.product}${version} (${parts.os}${release}${model})`;
}
