// Single source of truth for byte formatting across the admin screens.
// Base-1024 (binary) units: the admin screens previously each rolled their
// own base-1000 formatter, so the same byte count rendered differently
// depending on which screen you were on (e.g. GB on index.tsx vs GB on
// system-health.tsx disagreeing at the same value).
const UNITS = ["B", "KB", "MB", "GB", "TB", "PB"] as const;

export function formatBytes(bytes: number | string | null | undefined): string {
  const n = typeof bytes === "string" ? parseInt(bytes, 10) : (bytes ?? 0);
  if (!Number.isFinite(n) || n <= 0) return "0 B";
  const exponent = Math.min(Math.floor(Math.log(n) / Math.log(1024)), UNITS.length - 1);
  const value = n / Math.pow(1024, exponent);
  return `${value.toFixed(exponent > 0 ? 1 : 0)} ${UNITS[exponent]}`;
}
