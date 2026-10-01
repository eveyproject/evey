import { useEffect, useState } from "react";

/**
 * The time it is in a zone by the SERVER's clock: the instant it reported,
 * advanced by the time elapsed here since. The server applies opening hours
 * from its own clock, so that is the one to show, not the device's.
 * Null until the server time is known, or when the zone is unknown.
 */
export function useServerClock(serverNow: string | undefined, zone: string | undefined, locale: string): string | null {
  const [tick, setTick] = useState(0);
  const [receivedAt] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setTick((n) => n + 1), 15_000);
    return () => clearInterval(id);
  }, []);
  if (!serverNow || !zone) return null;
  const base = Date.parse(serverNow);
  if (Number.isNaN(base)) return null;
  void tick;
  return formatInZone(new Date(base + (Date.now() - receivedAt)), zone, locale);
}

/** "18:42" in the given IANA zone; null when the runtime does not know it. */
export function formatInZone(at: Date, zone: string, locale: string): string | null {
  try {
    return new Intl.DateTimeFormat(locale, { hour: "2-digit", minute: "2-digit", timeZone: zone }).format(at);
  } catch {
    return null;
  }
}
