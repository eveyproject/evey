import { useCallback, useEffect, useState } from "react";
import { rememberFocusBeforeLeaving } from "../tv/focus";

// A hash router, on purpose: the same build runs under /tv on the server
// and from file:// inside a packaged app, and neither has server-side
// rewrites. Routes are the screens, nothing more.
export type Route =
  | { name: "server" }
  | { name: "login" }
  | { name: "quickconnect" }
  | { name: "profiles"; pick?: boolean }
  | { name: "home"; customize?: boolean }
  | { name: "library"; libraryId: string }
  | { name: "media"; mediaId: string }
  | { name: "player"; mediaId: string; restart?: boolean; audio?: number; subtitle?: number | "off"; roomId?: string }
  | { name: "settings" }
  | { name: "search"; query?: string }
  | { name: "music"; libraryId?: string }
  | { name: "album"; albumId: string; libraryId?: string }
  | { name: "artist"; artistId: string; libraryId?: string }
  | { name: "libraries" }
  | { name: "collections" }
  | { name: "collection"; collectionId: string }
  | { name: "playlists" }
  | { name: "playlist"; playlistId: string }
  | { name: "genres" }
  | { name: "genre"; genre: string }
  | { name: "favorites" }
  | { name: "history" }
  | { name: "notifications" }
  | { name: "person"; personId: string }
  | { name: "photos"; libraryId?: string; albumId?: string }
  | { name: "slideshow"; libraryId?: string; albumId?: string; index?: number }
  | { name: "requests" }
  | { name: "syncplay" }
  | { name: "servers" }
  | { name: "stats" }
  | { name: "playback" }
  | { name: "plugin-settings"; pluginId: string };

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
    case "collection":
      return `#/collection/${encodeURIComponent(route.collectionId)}`;
    case "playlist":
      return `#/playlist/${encodeURIComponent(route.playlistId)}`;
    case "genre":
      return `#/genre/${encodeURIComponent(route.genre)}`;
    case "person":
      return `#/person/${encodeURIComponent(route.personId)}`;
    case "plugin-settings":
      return `#/plugin-settings/${encodeURIComponent(route.pluginId)}`;
    case "photos":
    case "slideshow": {
      const q = new URLSearchParams();
      if (route.libraryId) q.set("lib", route.libraryId);
      if (route.albumId) q.set("album", route.albumId);
      if (route.name === "slideshow" && route.index != null) q.set("i", String(route.index));
      const s = q.toString();
      return `#/${route.name}${s ? `?${s}` : ""}`;
    }
    case "player": {
      const q = new URLSearchParams();
      if (route.restart) q.set("restart", "1");
      if (route.audio !== undefined) q.set("audio", String(route.audio));
      if (route.subtitle !== undefined) q.set("subtitle", String(route.subtitle));
      if (route.roomId) q.set("roomId", route.roomId);
      const s = q.toString();
      return `#/player/${encodeURIComponent(route.mediaId)}${s ? `?${s}` : ""}`;
    }
    case "home":
      return route.customize ? "#/home?customize=1" : "#/home";
    case "profiles":
      return route.pick ? "#/profiles?pick=1" : "#/profiles";
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
      return params.get("pick") === "1" ? { name: "profiles", pick: true } : { name: "profiles" };
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
      if (!parts[1]) return { name: "home" };
      return {
        name: "player",
        mediaId: parts[1],
        restart: params.get("restart") === "1",
        ...(params.get("audio") ? { audio: Number(params.get("audio")) } : {}),
        ...(params.get("subtitle") ? { subtitle: params.get("subtitle") === "off" ? "off" as const : Number(params.get("subtitle")) } : {}),
        ...(params.get("roomId") ? { roomId: params.get("roomId")! } : {}),
      };
    case "libraries":
      return { name: "libraries" };
    case "collections":
      return { name: "collections" };
    case "collection":
      return parts[1] ? { name: "collection", collectionId: parts[1] } : { name: "collections" };
    case "playlists":
      return { name: "playlists" };
    case "playlist":
      return parts[1] ? { name: "playlist", playlistId: parts[1] } : { name: "playlists" };
    case "genres":
      return { name: "genres" };
    case "genre":
      return parts[1] ? { name: "genre", genre: parts[1] } : { name: "genres" };
    case "favorites":
      return { name: "favorites" };
    case "history":
      return { name: "history" };
    case "notifications":
      return { name: "notifications" };
    case "person":
      return parts[1] ? { name: "person", personId: parts[1] } : { name: "home" };
    case "photos":
      return { name: "photos", libraryId: params.get("lib") ?? undefined, albumId: params.get("album") ?? undefined };
    case "slideshow":
      return { name: "slideshow", libraryId: params.get("lib") ?? undefined, albumId: params.get("album") ?? undefined, index: params.get("i") ? Number(params.get("i")) : undefined };
    case "requests":
      return { name: "requests" };
    case "syncplay":
      return { name: "syncplay" };
    case "servers":
      return { name: "servers" };
    case "stats":
      return { name: "stats" };
    case "playback":
      return { name: "playback" };
    case "plugin-settings":
      return parts[1] ? { name: "plugin-settings", pluginId: parts[1] } : { name: "settings" };
    case "home":
      return params.get("customize") === "1" ? { name: "home", customize: true } : { name: "home" };
    default:
      return { name: "home" };
  }
}

export function navigate(route: Route): void {
  // Going back to this screen lands where the viewer left it.
  rememberFocusBeforeLeaving(window.location.hash);
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
