import { useCallback, useRef, useState } from "react";
import { eveyFetch } from "@evey/api";

interface StreamTokenResponse {
  token: string;
  mediaId: string;
}

export interface StreamTokenResult {
  streamToken: string | null;
  // opts.cast requests a long-lived token for Chromecast playback - the device
  // fetches segments on its own and can't refresh the short sliding token the
  // way the local player's xhrSetup does. opts.native requests the same long
  // window for a native <video> HLS element (Safari/iOS), which is likewise
  // unable to re-mint mid-stream; unlike cast it IS the local player's token,
  // so it updates streamToken state.
  fetchStreamToken: (mediaId: string, opts?: { cast?: boolean; native?: boolean }) => Promise<string>;
  isLoading: boolean;
}

export function useStreamToken(): StreamTokenResult {
  const [streamToken, setStreamToken] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const tokenRef = useRef<string | null>(null);

  const fetchStreamToken = useCallback(async (mediaId: string, opts?: { cast?: boolean; native?: boolean }): Promise<string> => {
    setIsLoading(true);
    try {
      const purpose = opts?.cast ? "cast" : opts?.native ? "native" : "";
      const res = await eveyFetch<StreamTokenResponse>({
        url: `/api/v1/media/${mediaId}/stream-token${purpose ? `?purpose=${purpose}` : ""}`,
        method: "POST",
      });
      // A cast token is a parallel, longer-lived token - don't clobber the local
      // player's streamToken state with it. The native token IS the local one.
      if (!opts?.cast) {
        tokenRef.current = res.token;
        setStreamToken(res.token);
      }
      return res.token;
    } finally {
      setIsLoading(false);
    }
  }, []);

  return { streamToken, fetchStreamToken, isLoading };
}
