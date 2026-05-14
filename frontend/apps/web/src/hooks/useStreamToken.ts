import { useCallback, useRef, useState } from "react";
import { eveyFetch } from "@evey/api";

interface StreamTokenResponse {
  token: string;
  mediaId: string;
}

export interface StreamTokenResult {
  streamToken: string | null;
  // opts.cast requests a long-lived token for Chromecast playback — the
  // device fetches segments on its own and can't refresh the short
  // sliding token the way the local player's xhrSetup does.
  fetchStreamToken: (mediaId: string, opts?: { cast?: boolean }) => Promise<string>;
  isLoading: boolean;
}

export function useStreamToken(): StreamTokenResult {
  const [streamToken, setStreamToken] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const tokenRef = useRef<string | null>(null);

  const fetchStreamToken = useCallback(async (mediaId: string, opts?: { cast?: boolean }): Promise<string> => {
    setIsLoading(true);
    try {
      const res = await eveyFetch<StreamTokenResponse>({
        url: `/api/v1/media/${mediaId}/stream-token${opts?.cast ? "?purpose=cast" : ""}`,
        method: "POST",
      });
      // A cast token is a parallel, longer-lived token — don't clobber
      // the local player's streamToken state with it.
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
