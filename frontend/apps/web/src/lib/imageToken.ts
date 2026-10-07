import { useState } from "react";
import { useAuthStore } from "@evey/store";
import { isTokenExpired } from "@/lib/authRefresh";

// The access token an <img src="...?token="> is built with. Subscribing to the
// store re-rendered every grid with a new URL on each proactive rotation
// (~15 min), and a new URL is a cache miss: whole screens of thumbnails were
// downloaded again and backdrops blinked. The token is pinned instead, and
// only moves on when the pinned one has expired - a URL a mounted <img> is
// still showing keeps working as long as its token does, and an <img>
// mounted later gets a token the server will accept.
export function useImageToken(): string | null {
  const live = useAuthStore((s) => s.accessToken);
  const [pinned, setPinned] = useState(live);
  if (pinned !== live && (!pinned || isTokenExpired(pinned) || !live)) {
    setPinned(live);
    return live;
  }
  return pinned;
}
