import { Monitor, Smartphone, Tv } from "lucide-react";

// Device rendering for the online panel, extracted from DashboardOnline so
// the mapping is testable. The Evey apps announce themselves with a product
// UA ("EveyMobile (Android 14; SM-G991B)", "EveyTV (Android TV 12; ...)") -
// without these branches the mobile app rendered as a desktop labeled
// "okhttp/4.12".

// The device model rides between "; " and ")" in the Evey product UAs.
function eveyModel(ua: string): string | undefined {
  return /;\s*([^)]+)\)/.exec(ua)?.[1]?.trim();
}

export function deviceIcon(ua: string) {
  const s = ua.toLowerCase();
  if (s.startsWith("eveytv")) return Tv;
  if (s.startsWith("eveymobile")) return Smartphone;
  if (/(tv|tizen|webos|crkey|appletv)/.test(s)) return Tv;
  if (/(mobile|android|iphone|ipad)/.test(s)) return Smartphone;
  return Monitor;
}

// A short, human label for a raw user-agent (browser + OS best-effort).
export function deviceLabel(ua: string): string {
  if (!ua) return "";
  const s = ua.toLowerCase();
  if (s.startsWith("eveytv")) {
    return ["Evey TV", eveyModel(ua)].filter(Boolean).join(" · ");
  }
  if (s.startsWith("eveymobile")) {
    const os = s.includes("ios") ? "iOS" : "Android";
    return [`Evey ${os}`, eveyModel(ua)].filter(Boolean).join(" · ");
  }
  let browser = "";
  if (s.includes("edg/")) browser = "Edge";
  else if (s.includes("chrome/") && !s.includes("chromium")) browser = "Chrome";
  else if (s.includes("firefox/")) browser = "Firefox";
  else if (s.includes("safari/") && !s.includes("chrome")) browser = "Safari";
  else if (s.startsWith("curl")) browser = "curl";
  let os = "";
  if (s.includes("windows")) os = "Windows";
  else if (s.includes("android")) os = "Android";
  else if (s.includes("iphone") || s.includes("ipad")) os = "iOS";
  else if (s.includes("mac os")) os = "macOS";
  else if (s.includes("linux")) os = "Linux";
  return [browser, os].filter(Boolean).join(" · ") || ua.slice(0, 24);
}
