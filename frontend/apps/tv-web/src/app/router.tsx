import { useCallback, useEffect, useState } from "react";

// A hash router, on purpose: the same build runs under /tv on the server
// and from file:// inside a packaged app, and neither has server-side
// rewrites. Routes are the screens, nothing more.
export type Route =
  | { name: "server" }
  | { name: "login" }
  | { name: "quickconnect" }
  | { name: "profiles" }
  | { name: "home" }
  | { name: "library"; libraryId: string }
  | { name: "media"; mediaId: string }
  | { name: "player"; mediaId: string; restart?: boolean }
  | { name: "settings" }
  | { name: "search"; query?: string }
  | { name: "music"; libraryId?: string }
  | { name: "album"; albumId: string; libraryId?: string }
  | { name: "artist"; artistId: string; libraryId?: string };

export function routeToHash(route: Route): string {
  switch (route.name) {
    case "library":
      return `#/library/${encodeURIComponent(route.libraryId)}`;
    case "search":
      return `#/search${route.query ? `?q=${encodeURIComponent(route.query)}` : ""}`;
    case "music":
      return `#/music${route.libraryId ? `?lib=${encodeURIComponent(route.libraryId)}` : ""}`;
    case "album":
      return `#/album/${encodeURIComponent(route.albumId)}${route.libraryId ? `?lib=${encodeURIComponent(route.libraryId)}` : ""}`;
    case "artist":
      return `#/artist/${encodeURIComponent(route.artistId)}${route.libraryId ? `?lib=${encodeURIComponent(route.libraryId)}` : ""}`;
    case "media":
      return `#/media/${encodeURIComponent(route.mediaId)}`;
    case "player":
      return `#/player/${encodeURIComponent(route.mediaId)}${route.restart ? "?restart=1" : ""}`;
    default:
      return `#/${route.name}`;
  }
}

export function parseHash(hash: string): Route {
  const [path, query = ""] = hash.replace(/^#\/?/, "").split("?");
  const parts = path.split("/").filter(Boolean).map(decodeURIComponent);
  const params = new URLSearchParams(query);
  switch (parts[0]) {
    case "server":
      return { name: "server" };
    case "login":
      return { name: "login" };
    case "quickconnect":
      return { name: "quickconnect" };
    case "profiles":
      return { name: "profiles" };
    case "settings":
      return { name: "settings" };
    case "search":
      return { name: "search", query: params.get("q") ?? undefined };
    case "music":
      return { name: "music", libraryId: params.get("lib") ?? undefined };
    case "album":
      return parts[1] ? { name: "album", albumId: parts[1], libraryId: params.get("lib") ?? undefined } : { name: "music" };
    case "artist":
      return parts[1] ? { name: "artist", artistId: parts[1], libraryId: params.get("lib") ?? undefined } : { name: "music" };
    case "library":
      return parts[1] ? { name: "library", libraryId: parts[1] } : { name: "home" };
    case "media":
      return parts[1] ? { name: "media", mediaId: parts[1] } : { name: "home" };
    case "player":
      return parts[1] ? { name: "player", mediaId: parts[1], restart: params.get("restart") === "1" } : { name: "home" };
    default:
      return { name: "home" };
  }
}

export function navigate(route: Route): void {
  window.location.hash = routeToHash(route);
}

export function replace(route: Route): void {
  const url = `${window.location.pathname}${window.location.search}${routeToHash(route)}`;
  window.history.replaceState(null, "", url);
  window.dispatchEvent(new HashChangeEvent("hashchange"));
}

export function goBack(): void {
  window.history.back();
}

export function useRoute(): Route {
  const [route, setRoute] = useState<Route>(() => parseHash(window.location.hash));
  const onChange = useCallback(() => setRoute(parseHash(window.location.hash)), []);
  useEffect(() => {
    window.addEventListener("hashchange", onChange);
    return () => window.removeEventListener("hashchange", onChange);
  }, [onChange]);
  return route;
}
